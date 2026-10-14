// Package family 管家庭档案：人物、人物之间的关系（称呼、亲疏、雷区）、地点（三个家）。
//
// 这是记忆系统的「骨架」：记忆挂在人和地点上，检索时靠这里的别名认人。
package family

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("记录不存在")

type Service struct{ db *gorm.DB }

func New(database *gorm.DB) *Service { return &Service{db: database} }

// ---------------------------------------------------------------- 人物

type Person struct {
	PersonKey     string   `json:"personKey"`
	Name          string   `json:"name"`
	RoleLabel     string   `json:"roleLabel"`
	Aliases       []string `json:"aliases"`
	Gender        string   `json:"gender"`
	Birthday      string   `json:"birthday"`      // YYYY-MM-DD 公历
	LunarBirthday string   `json:"lunarBirthday"` // MM-DD 农历
	BirthdayBy    string   `json:"birthdayBy"`    // solar / lunar
	HomePlaceKey  string   `json:"homePlaceKey"`
	Side          string   `json:"side"`
	Importance    int      `json:"importance"`
	IsCore        bool     `json:"isCore"`
	Profile       string   `json:"profile"`
	Avatar        string   `json:"avatar"`
	Sort          int      `json:"sort"`
	Status        string   `json:"status"`
}

func (s *Service) ListPersons(ctx context.Context, familyKey string) ([]Person, error) {
	var rows []personRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND status = 'active'", familyKey).
		Order("sort, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Person, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) GetPerson(ctx context.Context, familyKey, personKey string) (Person, error) {
	var row personRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND person_key = ?", familyKey, personKey).First(&row).Error; err != nil {
		return Person{}, notFound(err)
	}
	return row.toDTO(), nil
}

func (s *Service) SavePerson(ctx context.Context, familyKey string, input Person) (Person, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || strings.TrimSpace(input.RoleLabel) == "" {
		return Person{}, errors.New("名字和身份不能为空")
	}
	birthday, err := parseDate(input.Birthday)
	if err != nil {
		return Person{}, err
	}
	now := time.Now()
	row := personRow{
		FamilyKey: familyKey, Name: input.Name, RoleLabel: strings.TrimSpace(input.RoleLabel),
		Aliases: db.StringList(cleanList(input.Aliases)), Gender: input.Gender, Birthday: birthday,
		LunarBirthday: input.LunarBirthday, BirthdayBy: defaultString(input.BirthdayBy, "solar"),
		HomePlaceKey: input.HomePlaceKey, Side: defaultString(input.Side, "self"),
		Importance: clamp(input.Importance, 1, 5, 3), IsCore: boolInt(input.IsCore), Profile: input.Profile,
		Avatar: input.Avatar, Sort: input.Sort, Status: "active", UpdatedAt: now,
	}
	if input.PersonKey == "" {
		row.PersonKey = keys.New("per")
		row.CreatedAt = now
		err = s.db.WithContext(ctx).Create(&row).Error
	} else {
		row.PersonKey = input.PersonKey
		err = s.db.WithContext(ctx).Model(&personRow{}).
			Where("family_key = ? AND person_key = ?", familyKey, input.PersonKey).
			Select("name", "role_label", "aliases", "gender", "birthday", "lunar_birthday", "birthday_by",
				"home_place_key", "side", "importance", "is_core", "profile", "avatar", "sort", "updated_at").
			Updates(&row).Error
	}
	if err != nil {
		return Person{}, err
	}
	return s.GetPerson(ctx, familyKey, row.PersonKey)
}

// ArchivePerson 软删：人物被大量记忆引用，物理删除会让记忆变成孤儿。
func (s *Service) ArchivePerson(ctx context.Context, familyKey, personKey string) error {
	if personKey == "me" {
		return errors.New("不能删除「我」")
	}
	return s.db.WithContext(ctx).Model(&personRow{}).Where("family_key = ? AND person_key = ?", familyKey, personKey).
		Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error
}

// ResolvePerson 按 key、名字、身份或别名认人，认不出返回 ErrNotFound。
// 抽取和工具调用里模型给的往往是「老婆」「大舅」这种叫法而不是 key。
func (s *Service) ResolvePerson(ctx context.Context, familyKey, mention string) (Person, error) {
	mention = strings.TrimSpace(mention)
	if mention == "" {
		return Person{}, ErrNotFound
	}
	persons, err := s.ListPersons(ctx, familyKey)
	if err != nil {
		return Person{}, err
	}
	for _, person := range persons {
		if person.PersonKey == mention || person.Name == mention || person.RoleLabel == mention {
			return person, nil
		}
	}
	for _, person := range persons {
		for _, alias := range person.Aliases {
			if alias == mention {
				return person, nil
			}
		}
	}
	return Person{}, ErrNotFound
}

// ---------------------------------------------------------------- 关系

type Relation struct {
	RelationKey    string `json:"relationKey"`
	FromPerson     string `json:"fromPerson"`
	ToPerson       string `json:"toPerson"`
	Relation       string `json:"relation"`
	AddressAs      string `json:"addressAs"`
	ChildAddressAs string `json:"childAddressAs"`
	Closeness      int    `json:"closeness"`
	Notes          string `json:"notes"`
	Sensitivities  string `json:"sensitivities"`
}

// ListRelations 返回某人作为起点或终点的全部关系；personKey 为空返回全家的。
func (s *Service) ListRelations(ctx context.Context, familyKey, personKey string) ([]Relation, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if personKey != "" {
		query = query.Where("(from_person = ? OR to_person = ?)", personKey, personKey)
	}
	var rows []relationRow
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Relation, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) SaveRelation(ctx context.Context, familyKey string, input Relation) (Relation, error) {
	if input.FromPerson == "" || input.ToPerson == "" || strings.TrimSpace(input.Relation) == "" {
		return Relation{}, errors.New("关系的两端和关系名不能为空")
	}
	if input.FromPerson == input.ToPerson {
		return Relation{}, errors.New("不能和自己建立关系")
	}
	for _, key := range []string{input.FromPerson, input.ToPerson} {
		if _, err := s.GetPerson(ctx, familyKey, key); err != nil {
			return Relation{}, fmt.Errorf("人物 %s 不存在", key)
		}
	}
	now := time.Now()
	row := relationRow{
		FamilyKey: familyKey, FromPerson: input.FromPerson, ToPerson: input.ToPerson,
		Relation: strings.TrimSpace(input.Relation), AddressAs: input.AddressAs, ChildAddressAs: input.ChildAddressAs,
		Closeness: clamp(input.Closeness, 1, 5, 3), Notes: input.Notes, Sensitivities: input.Sensitivities, UpdatedAt: now,
	}
	var err error
	if input.RelationKey == "" {
		row.RelationKey = keys.New("rel")
		row.CreatedAt = now
		err = s.db.WithContext(ctx).Create(&row).Error
	} else {
		row.RelationKey = input.RelationKey
		err = s.db.WithContext(ctx).Model(&relationRow{}).
			Where("family_key = ? AND relation_key = ?", familyKey, input.RelationKey).
			Select("from_person", "to_person", "relation", "address_as", "child_address_as", "closeness", "notes", "sensitivities", "updated_at").
			Updates(&row).Error
	}
	if err != nil {
		return Relation{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) DeleteRelation(ctx context.Context, familyKey, relationKey string) error {
	return s.db.WithContext(ctx).Where("family_key = ? AND relation_key = ?", familyKey, relationKey).Delete(&relationRow{}).Error
}

// ---------------------------------------------------------------- 地点

type Place struct {
	PlaceKey    string `json:"placeKey"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Address     string `json:"address"`
	Dialect     string `json:"dialect"`
	Description string `json:"description"`
	Customs     string `json:"customs"`
	TravelTips  string `json:"travelTips"`
	Sort        int    `json:"sort"`
}

func (s *Service) ListPlaces(ctx context.Context, familyKey string) ([]Place, error) {
	var rows []placeRow
	if err := s.db.WithContext(ctx).Where("family_key = ?", familyKey).Order("sort, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Place, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) GetPlace(ctx context.Context, familyKey, placeKey string) (Place, error) {
	var row placeRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND place_key = ?", familyKey, placeKey).First(&row).Error; err != nil {
		return Place{}, notFound(err)
	}
	return row.toDTO(), nil
}

func (s *Service) SavePlace(ctx context.Context, familyKey string, input Place) (Place, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Place{}, errors.New("地点名称不能为空")
	}
	now := time.Now()
	row := placeRow{
		FamilyKey: familyKey, Name: strings.TrimSpace(input.Name), Kind: defaultString(input.Kind, "other"),
		Address: input.Address, Dialect: input.Dialect, Description: input.Description, Customs: input.Customs,
		TravelTips: input.TravelTips, Sort: input.Sort, UpdatedAt: now,
	}
	var err error
	if input.PlaceKey == "" {
		row.PlaceKey = keys.New("plc")
		row.CreatedAt = now
		err = s.db.WithContext(ctx).Create(&row).Error
	} else {
		row.PlaceKey = input.PlaceKey
		err = s.db.WithContext(ctx).Model(&placeRow{}).
			Where("family_key = ? AND place_key = ?", familyKey, input.PlaceKey).
			Select("name", "kind", "address", "dialect", "description", "customs", "travel_tips", "sort", "updated_at").
			Updates(&row).Error
	}
	if err != nil {
		return Place{}, err
	}
	return s.GetPlace(ctx, familyKey, row.PlaceKey)
}

func (s *Service) DeletePlace(ctx context.Context, familyKey, placeKey string) error {
	var count int64
	s.db.WithContext(ctx).Model(&personRow{}).Where("family_key = ? AND home_place_key = ? AND status = 'active'", familyKey, placeKey).Count(&count)
	if count > 0 {
		return errors.New("还有人物归属这个地点，先改掉人物的所属地点")
	}
	return s.db.WithContext(ctx).Where("family_key = ? AND place_key = ?", familyKey, placeKey).Delete(&placeRow{}).Error
}

// ---------------------------------------------------------------- helpers

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func parseDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, fmt.Errorf("日期格式应为 YYYY-MM-DD: %s", value)
	}
	return &parsed, nil
}

func cleanList(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func clamp(value, low, high, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
