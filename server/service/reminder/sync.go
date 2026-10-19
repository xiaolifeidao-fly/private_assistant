package reminder

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"common/viewer"
	"service/memory"
)

type festival struct {
	Key, Name, Lunar string
	BeforeDays       int
}

var festivals = []festival{
	{Key: "spring", Name: "春节", Lunar: "01-01", BeforeDays: 14},
	{Key: "dragon_boat", Name: "端午节", Lunar: "05-05", BeforeDays: 7},
	{Key: "mid_autumn", Name: "中秋节", Lunar: "08-15", BeforeDays: 7},
}

// SyncDerived 把人物、记忆和行程中的日期同步为可投递提醒。重复执行是幂等的。
func (s *Service) SyncDerived(ctx context.Context, familyKey string, now time.Time) error {
	// 不管是谁触发的（比如建行程），都按系统账号同步，免得把别人看不到的提醒当成过期取消掉
	ctx = viewer.AsSystem(ctx)
	now = now.In(shanghai)
	if err := s.syncBirthdays(ctx, familyKey, now); err != nil {
		return err
	}
	if err := s.syncDatedMemories(ctx, familyKey, now); err != nil {
		return err
	}
	if err := s.syncFestivals(ctx, familyKey, now); err != nil {
		return err
	}
	if err := s.syncTrips(ctx, familyKey, now); err != nil {
		return err
	}
	return s.syncDaily(ctx, familyKey, now)
}

func (s *Service) syncBirthdays(ctx context.Context, familyKey string, now time.Time) error {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return err
	}
	for _, person := range persons {
		var event time.Time
		if person.BirthdayBy == "lunar" && person.LunarBirthday != "" {
			event, err = nextLunarAnnual(person.LunarBirthday, now)
		} else if person.Birthday != "" {
			birthday, parseErr := time.ParseInLocation("2006-01-02", person.Birthday, shanghai)
			if parseErr != nil {
				continue
			}
			event = nextSolarAnnual(int(birthday.Month()), birthday.Day(), now)
		} else if person.LunarBirthday != "" {
			event, err = nextLunarAnnual(person.LunarBirthday, now)
		} else {
			continue
		}
		if err != nil || event.IsZero() {
			continue
		}
		calendarLabel := "公历"
		if person.BirthdayBy == "lunar" || person.Birthday == "" {
			calendarLabel = "农历 " + person.LunarBirthday
		}
		eventAt := atClock(event, 8)
		for _, lead := range []int{7, 0} {
			remindAt := atClock(event.AddDate(0, 0, -lead), 9)
			if lead == 0 {
				remindAt = atClock(event, 8)
			}
			if remindAt.Before(atClock(now, 0)) {
				continue
			}
			title, body := person.RoleLabel+"快过生日了", fmt.Sprintf("%s的生日是 %s，还有 %d 天。记得准备礼物和安排。", person.RoleLabel, event.Format("1月2日"), lead)
			if lead == 0 {
				title, body = "今天是"+person.RoleLabel+"的生日", fmt.Sprintf("今天是%s生日（%s），别忘了问候和安排。", person.RoleLabel, calendarLabel)
			}
			_, err = s.upsert(ctx, familyKey, reminderSeed{Kind: "birthday", Title: title, Body: body, EventAt: &eventAt,
				RemindAt: remindAt, SourceType: "person", SourceKey: person.PersonKey,
				DedupeKey: fmt.Sprintf("birthday:%s:%d:%d", person.PersonKey, event.Year(), lead), TargetURL: "/person/" + person.PersonKey})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) syncDatedMemories(ctx context.Context, familyKey string, now time.Time) error {
	items, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{
		Types: []string{"commitment", "need", "quote"}, Statuses: []string{memory.StatusActive}, Size: 200,
	})
	if err != nil {
		return err
	}
	activeKeys := make([]string, 0, len(items))
	for _, item := range items {
		activeKeys = append(activeKeys, item.MemoryKey)
	}
	stale := s.db.WithContext(ctx).Model(&reminderRow{}).
		Where("family_key = ? AND source_type = 'memory' AND status IN ?", familyKey, []string{StatusScheduled, StatusFailed})
	if len(activeKeys) > 0 {
		stale = stale.Where("source_key NOT IN ?", activeKeys)
	}
	if err := stale.Updates(map[string]any{"status": StatusCancelled, "updated_at": time.Now()}).Error; err != nil {
		return err
	}
	for _, item := range items {
		due, ok := attrDue(item)
		if !ok {
			s.db.WithContext(ctx).Model(&reminderRow{}).
				Where("family_key = ? AND source_type = 'memory' AND source_key = ? AND status IN ?", familyKey, item.MemoryKey, []string{StatusScheduled, StatusFailed}).
				Updates(map[string]any{"status": StatusCancelled, "updated_at": time.Now()})
			continue
		}
		// 截止日变更后，取消旧日期的未投递提醒。
		s.db.WithContext(ctx).Model(&reminderRow{}).
			Where("family_key = ? AND source_type = 'memory' AND source_key = ? AND event_at <> ? AND status IN ?", familyKey, item.MemoryKey, due, []string{StatusScheduled, StatusFailed}).
			Updates(map[string]any{"status": StatusCancelled, "updated_at": time.Now()})
		if due.Before(now.AddDate(0, 0, -1)) {
			continue
		}
		kind := "dated"
		prefix := "家里提过的事"
		if item.Type == "commitment" {
			kind, prefix = "commitment", "你答应过的事"
		}
		for _, lead := range []int{1, 0} {
			remindAt := atClock(due.AddDate(0, 0, -lead), 9)
			if remindAt.Before(atClock(now, 0)) {
				continue
			}
			title := fmt.Sprintf("%s：%s", prefix, item.Title)
			if lead == 0 {
				title = "今天到期：" + item.Title
			}
			body := item.Content
			if body == "" {
				body = item.Title
			}
			eventAt := due
			_, err = s.upsert(ctx, familyKey, reminderSeed{Kind: kind, Title: title, Body: body, EventAt: &eventAt,
				RemindAt: remindAt, SourceType: "memory", SourceKey: item.MemoryKey, Owner: item.OwnerAccount, Visibility: item.Visibility,
				DedupeKey: fmt.Sprintf("memory:%s:%s:%d", item.MemoryKey, due.Format("20060102"), lead), TargetURL: "/today?memory=" + item.MemoryKey})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) syncFestivals(ctx context.Context, familyKey string, now time.Time) error {
	for _, item := range festivals {
		event, err := nextLunarAnnual(item.Lunar, now)
		if err != nil {
			continue
		}
		remindAt := atClock(event.AddDate(0, 0, -item.BeforeDays), 9)
		if remindAt.Before(atClock(now, 0)) {
			continue
		}
		eventAt := atClock(event, 0)
		body := fmt.Sprintf("距%s还有 %d 天。可以先确定去哪边、礼物红包和行程，再回顾两边老家的规矩。", item.Name, item.BeforeDays)
		if _, err := s.upsert(ctx, familyKey, reminderSeed{Kind: "festival", Title: item.Name + "要提前安排了", Body: body,
			EventAt: &eventAt, RemindAt: remindAt, SourceType: "festival", SourceKey: item.Key,
			DedupeKey: fmt.Sprintf("festival:%s:%d", item.Key, event.Year()), TargetURL: "/family"}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) syncTrips(ctx context.Context, familyKey string, now time.Time) error {
	trips, err := s.ListTrips(ctx, familyKey, false)
	if err != nil {
		return err
	}
	for _, trip := range trips {
		if trip.StartAt.Before(now.AddDate(0, 0, -1)) {
			continue
		}
		brief, briefErr := s.BuildBrief(ctx, familyKey, trip.TripKey)
		if briefErr != nil {
			brief = "回老家前记得查看称呼、雷区和当地规矩。"
		}
		eventAt := trip.StartAt
		remindAt := atClock(trip.StartAt.AddDate(0, 0, -2), 9)
		if remindAt.Before(atClock(now, 0)) {
			remindAt = now
		}
		if _, err := s.upsert(ctx, familyKey, reminderSeed{Kind: "trip", Title: trip.Title + "·出发前简报", Body: brief,
			EventAt: &eventAt, RemindAt: remindAt, SourceType: "trip", SourceKey: trip.TripKey,
			DedupeKey: "trip:" + trip.TripKey, TargetURL: "/trip/" + trip.TripKey}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) syncDaily(ctx context.Context, familyKey string, now time.Time) error {
	body := s.dailyBody(ctx, familyKey, now)
	remindAt := atClock(now, 8)
	_, err := s.upsert(ctx, familyKey, reminderSeed{Kind: "daily", Title: "今日家庭提醒", Body: body,
		RemindAt: remindAt, DedupeKey: "daily:" + now.Format("20060102"), TargetURL: "/today"})
	return err
}

func (s *Service) dailyBody(ctx context.Context, familyKey string, now time.Time) string {
	items, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Types: []string{"commitment", "need"}, Statuses: []string{memory.StatusActive}, Size: 100})
	if err != nil {
		return "打开「今天」看看待办和提醒。"
	}
	open, due := 0, 0
	for _, item := range items {
		var attrs struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(item.Attrs, &attrs)
		if attrs.Status == "done" || attrs.Status == "addressed" {
			continue
		}
		open++
		if at, ok := attrDue(item); ok && !at.After(now.AddDate(0, 0, 7)) {
			due++
		}
	}
	if open == 0 {
		return "今天没有尚未完成的承诺或诉求。"
	}
	return fmt.Sprintf("还有 %d 件承诺/诉求没办完，其中 %d 件在 7 天内到期。", open, due)
}
