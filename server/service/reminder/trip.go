package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"
	"service/memory"

	"gorm.io/gorm"
)

type TripInput struct {
	PlaceKey string     `json:"placeKey"`
	Title    string     `json:"title"`
	StartAt  time.Time  `json:"startAt"`
	EndAt    *time.Time `json:"endAt"`
	People   []string   `json:"people"`
	Notes    string     `json:"notes"`
}

func (s *Service) CreateTrip(ctx context.Context, familyKey string, input TripInput) (Trip, error) {
	if input.PlaceKey == "" || input.StartAt.IsZero() {
		return Trip{}, errors.New("目的地和出发时间不能为空")
	}
	place, err := s.family.GetPlace(ctx, familyKey, input.PlaceKey)
	if err != nil {
		return Trip{}, err
	}
	if input.EndAt != nil && input.EndAt.Before(input.StartAt) {
		return Trip{}, errors.New("结束时间不能早于出发时间")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "回" + place.Name
	}
	now := time.Now()
	row := tripRow{TripKey: keys.New("trip"), FamilyKey: familyKey, PlaceKey: input.PlaceKey, Title: title,
		StartAt: input.StartAt.In(shanghai), EndAt: input.EndAt, People: db.StringList(cleanStrings(input.People)), Notes: strings.TrimSpace(input.Notes),
		Status: "planned", CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Trip{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) ListTrips(ctx context.Context, familyKey string, includeHistory bool) ([]Trip, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if !includeHistory {
		query = query.Where("status = 'planned' AND start_at >= ?", time.Now().AddDate(0, 0, -1))
	}
	var rows []tripRow
	if err := query.Order("start_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Trip, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) GetTrip(ctx context.Context, familyKey, tripKey string, withBrief bool) (Trip, error) {
	var row tripRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND trip_key = ?", familyKey, tripKey).First(&row).Error; err != nil {
		return Trip{}, ErrNotFound
	}
	trip := row.toDTO()
	if withBrief {
		trip.Brief, _ = s.BuildBrief(ctx, familyKey, tripKey)
	}
	return trip, nil
}

func (s *Service) CancelTrip(ctx context.Context, familyKey, tripKey string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&tripRow{}).Where("family_key = ? AND trip_key = ?", familyKey, tripKey).
			Updates(map[string]any{"status": "cancelled", "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Model(&reminderRow{}).Where("family_key = ? AND source_type = 'trip' AND source_key = ? AND status IN ?", familyKey, tripKey, []string{StatusScheduled, StatusFailed}).
			Updates(map[string]any{"status": StatusCancelled, "updated_at": time.Now()}).Error
	})
}

func (s *Service) BuildBrief(ctx context.Context, familyKey, tripKey string) (string, error) {
	trip, err := s.GetTrip(ctx, familyKey, tripKey, false)
	if err != nil {
		return "", err
	}
	place, err := s.family.GetPlace(ctx, familyKey, trip.PlaceKey)
	if err != nil {
		return "", err
	}
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return "", err
	}
	relations, _ := s.family.ListRelations(ctx, familyKey, "me")
	relationByPerson := map[string]string{}
	for _, relation := range relations {
		other := relation.ToPerson
		if other == "me" {
			other = relation.FromPerson
		}
		line := relation.Relation
		if relation.AddressAs != "" {
			line += "，我叫「" + relation.AddressAs + "」"
		}
		if relation.Notes != "" {
			line += "。要点：" + relation.Notes
		}
		if relation.Sensitivities != "" {
			line += "。⚠️ " + relation.Sensitivities
		}
		relationByPerson[other] = line
	}
	selected := map[string]bool{}
	for _, key := range trip.People {
		selected[key] = true
	}
	var peopleLines []string
	for _, person := range persons {
		if person.PersonKey == "me" || (!selected[person.PersonKey] && person.HomePlaceKey != trip.PlaceKey) {
			continue
		}
		line := "- " + person.RoleLabel
		if person.Name != "" && person.Name != person.RoleLabel {
			line += "（" + person.Name + "）"
		}
		if relationByPerson[person.PersonKey] != "" {
			line += "：" + relationByPerson[person.PersonKey]
		}
		peopleLines = append(peopleLines, line)
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "## %s\n%s 出发，目的地：%s。\n", trip.Title, trip.StartAt.In(shanghai).Format("1月2日 15:04"), place.Name)
	builder.WriteString("\n## 这次可能会见到\n")
	if len(peopleLines) == 0 {
		builder.WriteString("（还没指定人，档案里也没有这里的常住人。）\n")
	} else {
		builder.WriteString(strings.Join(peopleLines, "\n") + "\n")
	}
	builder.WriteString("\n## 当地规矩和准备\n")
	for _, line := range []string{place.Customs, place.TravelTips} {
		if strings.TrimSpace(line) != "" {
			builder.WriteString("- " + strings.TrimSpace(line) + "\n")
		}
	}
	if place.Customs == "" && place.TravelTips == "" {
		builder.WriteString("（档案里还没有当地规矩。）\n")
	}

	complaints, _, _ := s.memory.List(ctx, familyKey, memory.ListFilter{
		Types: []string{"quote", "conflict", "taboo", "need"}, Person: "wife",
		Statuses: []string{memory.StatusActive, memory.StatusSuperseded}, Size: 100,
	})
	builder.WriteString("\n## 老婆之前在意的事\n")
	added := 0
	for _, item := range complaints {
		if item.PlaceKey != trip.PlaceKey && !strings.Contains(item.Content, place.Name) && !strings.Contains(item.Title, place.Name) {
			continue
		}
		text := item.Content
		if text == "" {
			text = item.Title
		}
		fmt.Fprintf(&builder, "- %s [%s]\n", oneLine(text, 220), item.MemoryKey)
		added++
		if added == 5 {
			break
		}
	}
	if added == 0 {
		builder.WriteString("（没有找到和这次地点直接关联的不满或诉求。）\n")
	}
	if trip.Notes != "" {
		builder.WriteString("\n## 这次的备注\n" + trip.Notes + "\n")
	}
	return builder.String(), nil
}

func cleanStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
