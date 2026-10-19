// Package diet 是孩子的饮食：预先的计划（pa_diet_plan）和实际吃了什么（pa_diet_record）。
//
//   - 计划：哪天哪一餐打算吃什么、有什么要求。只有我和老婆的账号能改（CanPlan），奶奶的账号只看；
//   - 实际：谁都能记、能改。奶奶是喂饭最多的人，她那边拍张照、说一句就行（capture.go 交给模型整理）。
//
// 全家共用，不分私有。删除是软删，留痕。
package diet

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"common/keys"
	"service/family"
	"service/identity"

	"gorm.io/gorm"
)

const (
	dateLayout    = "2006-01-02"
	statusActive  = "active"
	statusDeleted = "deleted"
	// DefaultChild 没指定孩子时用家里的女儿
	DefaultChild = "daughter"
	maxDays      = 31
	maxText      = 1000
)

var (
	ErrNotFound  = errors.New("这条饮食记录不存在")
	ErrForbidden = errors.New("饮食计划只有爸爸妈妈能改")
)

// Meal 是一餐。day 只用于计划：这一天的总体要求（一天一杯奶、不吃糖……）。
type Meal struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Clock 记录没说几点时按这个时间算
	Clock string `json:"clock"`
	// Until 这一餐大概到几点为止，用来按当前时间猜是哪一餐
	Until string `json:"-"`
}

var Meals = []Meal{
	{Key: "breakfast", Label: "早饭", Clock: "07:30", Until: "09:30"},
	{Key: "morning_snack", Label: "上午加餐", Clock: "10:00", Until: "11:00"},
	{Key: "lunch", Label: "午饭", Clock: "12:00", Until: "14:00"},
	{Key: "afternoon_snack", Label: "下午加餐", Clock: "15:30", Until: "17:00"},
	{Key: "dinner", Label: "晚饭", Clock: "18:00", Until: "20:00"},
	{Key: "night_snack", Label: "睡前", Clock: "20:30", Until: "24:00"},
}

const MealDay = "day"

// Amounts 吃了多少。
var Amounts = map[string]string{"all": "吃完了", "most": "吃了大半", "half": "吃了一半", "little": "吃了几口", "none": "没吃"}

func mealOf(key string) (Meal, bool) {
	for _, meal := range Meals {
		if meal.Key == key {
			return meal, true
		}
	}
	return Meal{}, false
}

// MealLabel 返回一餐的中文名；day 是「全天」。
func MealLabel(key string) string {
	if key == MealDay {
		return "全天"
	}
	if meal, ok := mealOf(key); ok {
		return meal.Label
	}
	return key
}

// GuessMeal 按时间猜是哪一餐。
func GuessMeal(at time.Time) string {
	clock := at.Format("15:04")
	for _, meal := range Meals {
		if clock < meal.Until {
			return meal.Key
		}
	}
	return Meals[len(Meals)-1].Key
}

// Actor 是谁在操作：账号和角色（identity.Role*）。
type Actor struct {
	Account string
	Role    string
}

// CanPlan 我（admin / member）和老婆（spouse）能改计划，奶奶（elder）只看。
func (a Actor) CanPlan() bool {
	switch a.Role {
	case identity.RoleAdmin, identity.RoleMember, identity.RoleSpouse, "":
		return true
	}
	return false
}

// Storage 存照片，由 common/objectstore 实现。
type Storage interface {
	Save(path string, data []byte) error
	Load(path string) ([]byte, error)
}

type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
}

type Service struct {
	db      *gorm.DB
	family  FamilyPort
	llm     Chatter
	storage Storage
	// 每周点评的推送（review.go，SetNotifier 注入）
	reminders    ReminderPort
	planAccounts func(ctx context.Context) []string
}

func New(database *gorm.DB, familyService FamilyPort, gateway Chatter, storage Storage) *Service {
	return &Service{db: database, family: familyService, llm: gateway, storage: storage}
}

type Plan struct {
	PlanKey   string    `json:"planKey"`
	ChildKey  string    `json:"childKey"`
	Date      string    `json:"date"`
	Meal      string    `json:"meal"`
	Foods     string    `json:"foods"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"createdBy"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Photo struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
}

type Record struct {
	RecordKey  string    `json:"recordKey"`
	ChildKey   string    `json:"childKey"`
	EatenAt    time.Time `json:"eatenAt"`
	Meal       string    `json:"meal"`
	Foods      string    `json:"foods"`
	Amount     string    `json:"amount"`
	Note       string    `json:"note"`
	Source     string    `json:"source"`
	RawText    string    `json:"rawText"`
	PhotoCount int       `json:"photoCount"`
	CaptureKey string    `json:"captureKey"`
	CreatedBy  string    `json:"createdBy"`
	UpdatedBy  string    `json:"updatedBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Overview 是一段日子的计划和实际，App 按天、按餐对照着显示。
type Overview struct {
	ChildKey   string   `json:"childKey"`
	ChildLabel string   `json:"childLabel"`
	From       string   `json:"from"`
	Days       int      `json:"days"`
	CanPlan    bool     `json:"canPlan"`
	Meals      []Meal   `json:"meals"`
	Plans      []Plan   `json:"plans"`
	Records    []Record `json:"records"`
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		now := time.Now().In(shanghai)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local), nil
	}
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("日期格式应为 YYYY-MM-DD：%s", value)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.Local), nil
}

// child 校验孩子是家里的人，返回 key 和称呼。
func (s *Service) child(ctx context.Context, familyKey, childKey string) (string, string, error) {
	person, err := s.childPerson(ctx, familyKey, childKey)
	return person.PersonKey, person.RoleLabel, err
}

// Overview 返回从 from 起 days 天的计划和实际。
func (s *Service) Overview(ctx context.Context, familyKey, childKey, from string, days int, actor Actor) (Overview, error) {
	childKey, label, err := s.child(ctx, familyKey, childKey)
	if err != nil {
		return Overview{}, err
	}
	start, err := parseDate(from)
	if err != nil {
		return Overview{}, err
	}
	if days <= 0 || days > maxDays {
		days = 1
	}
	end := start.AddDate(0, 0, days)
	var plans []planRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND status = ? AND plan_date >= ? AND plan_date < ?",
		familyKey, childKey, statusActive, start.Format(dateLayout), end.Format(dateLayout)).Order("plan_date, id").Find(&plans).Error; err != nil {
		return Overview{}, err
	}
	var records []recordRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND status = ? AND eaten_at >= ? AND eaten_at < ?",
		familyKey, childKey, statusActive, start, end).Order("eaten_at, id").Find(&records).Error; err != nil {
		return Overview{}, err
	}
	result := Overview{ChildKey: childKey, ChildLabel: label, From: start.Format(dateLayout), Days: days, CanPlan: actor.CanPlan(),
		Meals: Meals, Plans: make([]Plan, 0, len(plans)), Records: make([]Record, 0, len(records))}
	for _, row := range plans {
		result.Plans = append(result.Plans, row.toDTO())
	}
	for _, row := range records {
		result.Records = append(result.Records, row.toDTO())
	}
	return result, nil
}

type PlanInput struct {
	ChildKey string `json:"childKey"`
	Date     string `json:"date"`
	Meal     string `json:"meal"`
	Foods    string `json:"foods"`
	Note     string `json:"note"`
}

// SavePlan 写某天某一餐的计划；吃什么和要求都清空就是删掉这一餐的计划。
func (s *Service) SavePlan(ctx context.Context, familyKey string, input PlanInput, actor Actor) (*Plan, error) {
	if !actor.CanPlan() {
		return nil, ErrForbidden
	}
	childKey, _, err := s.child(ctx, familyKey, input.ChildKey)
	if err != nil {
		return nil, err
	}
	if _, ok := mealOf(input.Meal); !ok && input.Meal != MealDay {
		return nil, fmt.Errorf("不认识的餐次：%s", input.Meal)
	}
	date, err := parseDate(input.Date)
	if err != nil {
		return nil, err
	}
	foods, note := clip(input.Foods), clip(input.Note)
	now := time.Now()
	var existing planRow
	err = s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND plan_date = ? AND meal = ? AND status = ?",
		familyKey, childKey, date.Format(dateLayout), input.Meal, statusActive).First(&existing).Error
	switch {
	case err == nil:
		updates := map[string]any{"foods": foods, "note": note, "updated_by": actor.Account, "updated_at": now}
		if foods == "" && note == "" {
			updates = map[string]any{"status": statusDeleted, "updated_by": actor.Account, "updated_at": now}
		}
		if err := s.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
			return nil, err
		}
		if foods == "" && note == "" {
			return nil, nil
		}
		existing.Foods, existing.Note, existing.UpdatedBy, existing.UpdatedAt = foods, note, actor.Account, now
		plan := existing.toDTO()
		return &plan, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	if foods == "" && note == "" {
		return nil, nil
	}
	row := planRow{PlanKey: keys.New("dplan"), FamilyKey: familyKey, ChildKey: childKey, PlanDate: date, Meal: input.Meal,
		Foods: foods, Note: note, Status: statusActive, CreatedBy: actor.Account, UpdatedBy: actor.Account, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	plan := row.toDTO()
	return &plan, nil
}

type CopyInput struct {
	ChildKey string `json:"childKey"`
	From     string `json:"from"`
	To       string `json:"to"`
	Days     int    `json:"days"`
}

// CopyPlans 把 from 起 days 天的计划照搬到 to 起的同样几天（比如上周的抄到这周）。
// 目标那天那一餐已经有计划的会被覆盖，源里没有的餐不动。返回抄了几餐。
func (s *Service) CopyPlans(ctx context.Context, familyKey string, input CopyInput, actor Actor) (int, error) {
	if !actor.CanPlan() {
		return 0, ErrForbidden
	}
	childKey, _, err := s.child(ctx, familyKey, input.ChildKey)
	if err != nil {
		return 0, err
	}
	from, err := parseDate(input.From)
	if err != nil {
		return 0, err
	}
	to, err := parseDate(input.To)
	if err != nil {
		return 0, err
	}
	if input.Days <= 0 || input.Days > maxDays {
		input.Days = 7
	}
	if from.Equal(to) {
		return 0, errors.New("抄到的日子和原来的一样")
	}
	var plans []planRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND status = ? AND plan_date >= ? AND plan_date < ?",
		familyKey, childKey, statusActive, from.Format(dateLayout), from.AddDate(0, 0, input.Days).Format(dateLayout)).Find(&plans).Error; err != nil {
		return 0, err
	}
	offset := int(math.Round(to.Sub(from).Hours() / 24))
	count := 0
	for _, plan := range plans {
		target := plan.PlanDate.AddDate(0, 0, offset)
		if _, err := s.SavePlan(ctx, familyKey, PlanInput{ChildKey: childKey, Date: target.Format(dateLayout), Meal: plan.Meal,
			Foods: plan.Foods, Note: plan.Note}, actor); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

type RecordInput struct {
	ChildKey string     `json:"childKey"`
	EatenAt  *time.Time `json:"eatenAt"`
	Meal     string     `json:"meal"`
	Foods    string     `json:"foods"`
	Amount   string     `json:"amount"`
	Note     string     `json:"note"`
}

func (input *RecordInput) normalize() error {
	input.Foods, input.Note = clip(input.Foods), clip(input.Note)
	if input.Foods == "" && input.Note == "" {
		return errors.New("写一下吃了什么")
	}
	if _, ok := Amounts[input.Amount]; !ok {
		input.Amount = ""
	}
	if input.EatenAt == nil || input.EatenAt.IsZero() {
		now := time.Now()
		input.EatenAt = &now
	}
	if _, ok := mealOf(input.Meal); !ok {
		input.Meal = GuessMeal(input.EatenAt.In(shanghai))
	}
	return nil
}

// CreateRecord 手动记一条实际吃了什么。谁都能记。
func (s *Service) CreateRecord(ctx context.Context, familyKey string, input RecordInput, actor Actor) (Record, error) {
	childKey, _, err := s.child(ctx, familyKey, input.ChildKey)
	if err != nil {
		return Record{}, err
	}
	if err := input.normalize(); err != nil {
		return Record{}, err
	}
	now := time.Now()
	row := recordRow{RecordKey: keys.New("drec"), FamilyKey: familyKey, ChildKey: childKey, EatenAt: *input.EatenAt, Meal: input.Meal,
		Foods: input.Foods, Amount: input.Amount, Note: input.Note, Source: "manual", Status: statusActive,
		CreatedBy: actor.Account, UpdatedBy: actor.Account, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Record{}, err
	}
	return row.toDTO(), nil
}

// UpdateRecord 改一条记录（模型整理错了、记错了）。谁都能改。
func (s *Service) UpdateRecord(ctx context.Context, familyKey, recordKey string, input RecordInput, actor Actor) (Record, error) {
	row, err := s.getRecord(ctx, familyKey, recordKey)
	if err != nil {
		return Record{}, err
	}
	if input.EatenAt == nil {
		input.EatenAt = &row.EatenAt
	}
	if err := input.normalize(); err != nil {
		return Record{}, err
	}
	updates := map[string]any{"eaten_at": *input.EatenAt, "meal": input.Meal, "foods": input.Foods, "amount": input.Amount,
		"note": input.Note, "updated_by": actor.Account, "updated_at": time.Now()}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return Record{}, err
	}
	row, err = s.getRecord(ctx, familyKey, recordKey)
	return row.toDTO(), err
}

// DeleteRecord 软删一条记录。
func (s *Service) DeleteRecord(ctx context.Context, familyKey, recordKey string, actor Actor) error {
	row, err := s.getRecord(ctx, familyKey, recordKey)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": statusDeleted, "updated_by": actor.Account, "updated_at": time.Now()}).Error
}

func (s *Service) getRecord(ctx context.Context, familyKey, recordKey string) (recordRow, error) {
	var row recordRow
	err := s.db.WithContext(ctx).Where("family_key = ? AND record_key = ? AND status = ?", familyKey, recordKey, statusActive).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

// PhotoData 读一条记录的第 index 张照片。
func (s *Service) PhotoData(ctx context.Context, familyKey, recordKey string, index int) (string, []byte, error) {
	row, err := s.getRecord(ctx, familyKey, recordKey)
	if err != nil {
		return "", nil, err
	}
	photos := row.photos()
	if index < 0 || index >= len(photos) {
		return "", nil, ErrNotFound
	}
	data, err := s.storage.Load(photos[index].Path)
	if err != nil {
		return "", nil, err
	}
	return photos[index].MediaType, data, nil
}

func clip(value string) string {
	value = strings.TrimSpace(value)
	if runes := []rune(value); len(runes) > maxText {
		value = string(runes[:maxText])
	}
	return value
}

var shanghai = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}()
