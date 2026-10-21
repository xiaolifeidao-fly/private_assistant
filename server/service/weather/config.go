package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"service/identity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 配置存在 pa_setting（setting_key = weather），读的时候和出厂值合并，管理端可改、可恢复默认。
const settingKey = "weather"

// Config 是宝宝穿衣的设置。
type Config struct {
	Enabled  bool     `json:"enabled"`
	ChildKey string   `json:"childKey"`
	Location Location `json:"location"`
	Slots    Slots    `json:"slots"`
	// SchoolDays 哪几天上幼儿园：1 = 周一 … 5 = 周五，0 = 周日
	SchoolDays []int     `json:"schoolDays"`
	School     School    `json:"school"`
	Weekend    []Outing  `json:"weekend"` // 不上学的日子平常出去玩的时段
	Thresholds Threshold `json:"thresholds"`
	Levels     []Level   `json:"levels"`
	Alerts     bool      `json:"alerts"`    // 新的黄色以上预警立刻推
	AlertMax   int       `json:"alertMax"`  // 临时提醒一天最多几条
	QuietFrom  string    `json:"quietFrom"` // 这段时间不推临时提醒
	QuietTo    string    `json:"quietTo"`
	Recipients []string  `json:"recipients"` // 按角色：admin / member / spouse / elder
}

// Slots 每天推送的时间。PrePickup 接之前多少分钟再看一眼（0 = 不看）。
type Slots struct {
	Morning   string `json:"morning"`
	Noon      string `json:"noon"`
	Evening   string `json:"evening"`
	PrePickup int    `json:"prePickup"`
}

// School 平常上学日的送接。Walk 接到以后走回家要多久（分钟），算「接到直接回」几点到家。
type School struct {
	Drop   string `json:"drop"`
	Pickup string `json:"pickup"`
	Home   string `json:"home"`
	Walk   int    `json:"walk"`
}

// Outing 一段出去玩的时间。
type Outing struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}

// Threshold 回家线：体感低于它就该回家了。GoOut 体感低于它不建议出去玩。
type Threshold struct {
	BackBy        float64 `json:"backBy"`
	BackByInfant  float64 `json:"backByInfant"` // 1 岁以内
	GoOut         float64 `json:"goOut"`
	ColdMinutes   int     `json:"coldMinutes"`   // cold 档在外面最多多久
	FreezeMinutes int     `json:"freezeMinutes"` // freezing 档在外面最多多久
	AQIMask       int     `json:"aqiMask"`       // 超过它戴口罩、别在外面玩
	AQIStay       int     `json:"aqiStay"`       // 超过它尽量别出门
}

// Level 是一档穿衣：体感不低于 Min 就是这一档（从高往低找）。
type Level struct {
	Key     string   `json:"key"` // hot / warm / mild / cool / cold / chilly / freezing
	Label   string   `json:"label"`
	Min     float64  `json:"min"`
	Layers  int      `json:"layers"`
	Clothes []string `json:"clothes"`
	Tip     string   `json:"tip"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:    true,
		ChildKey:   "daughter",
		Location:   Location{Name: "上海 青浦", Code: "101020800", Lat: 31.1539, Lon: 121.1141},
		Slots:      Slots{Morning: "07:00", Noon: "12:00", Evening: "18:30", PrePickup: 30},
		SchoolDays: []int{1, 2, 3, 4, 5},
		School:     School{Drop: "08:00", Pickup: "15:50", Home: "16:30", Walk: 20},
		Weekend:    []Outing{{From: "09:30", To: "11:00"}, {From: "15:00", To: "16:30"}},
		Thresholds: Threshold{BackBy: 10, BackByInfant: 12, GoOut: 5, ColdMinutes: 90, FreezeMinutes: 40, AQIMask: 150, AQIStay: 200},
		Levels: []Level{
			{Key: "hot", Label: "热", Min: 28, Layers: 1, Clothes: []string{"短袖", "短裤"}, Tip: "透气，避开中午暴晒，带水"},
			{Key: "warm", Label: "暖和", Min: 23, Layers: 1, Clothes: []string{"短袖或薄长袖"}, Tip: "早晚带件薄外套"},
			{Key: "mild", Label: "舒服", Min: 18, Layers: 2, Clothes: []string{"长袖", "薄外套"}},
			{Key: "cool", Label: "有点凉", Min: 13, Layers: 3, Clothes: []string{"长袖", "卫衣或薄毛衣", "外套"}},
			{Key: "cold", Label: "冷", Min: 8, Layers: 3, Clothes: []string{"秋衣秋裤", "毛衣", "厚外套", "帽子"}},
			{Key: "chilly", Label: "很冷", Min: 3, Layers: 4, Clothes: []string{"保暖内衣", "毛衣", "羽绒服", "帽子", "围巾"}},
			{Key: "freezing", Label: "冻", Min: -99, Layers: 4, Clothes: []string{"保暖内衣", "毛衣", "羽绒服", "帽子", "手套", "围巾"}, Tip: "在外面尽量短"},
		},
		Alerts: true, AlertMax: 2, QuietFrom: "21:30", QuietTo: "06:30",
		Recipients: []string{identity.RoleAdmin, identity.RoleMember, identity.RoleElder},
	}
}

// Config 返回当前生效的设置（和出厂值合并）。
func (s *Service) Config(ctx context.Context) Config {
	config := DefaultConfig()
	var row settingRow
	if err := s.db.WithContext(ctx).Where("setting_key = ?", settingKey).First(&row).Error; err == nil {
		var saved Config
		if json.Unmarshal([]byte(row.Value), &saved) == nil {
			config = merge(config, saved, []byte(row.Value))
		}
	}
	return config
}

// merge 库里写了的字段覆盖出厂值；列表整个覆盖。
func merge(base, saved Config, raw []byte) Config {
	var present map[string]json.RawMessage
	_ = json.Unmarshal(raw, &present)
	if _, ok := present["enabled"]; ok {
		base.Enabled = saved.Enabled
	}
	if saved.ChildKey != "" {
		base.ChildKey = saved.ChildKey
	}
	if saved.Location.Code != "" || saved.Location.Lat != 0 {
		base.Location = saved.Location
	}
	if _, ok := present["slots"]; ok {
		base.Slots = saved.Slots
	}
	if _, ok := present["schoolDays"]; ok && saved.SchoolDays != nil {
		base.SchoolDays = saved.SchoolDays
	}
	if _, ok := present["school"]; ok && saved.School.Pickup != "" {
		base.School = saved.School
	}
	if _, ok := present["weekend"]; ok && saved.Weekend != nil {
		base.Weekend = saved.Weekend
	}
	if _, ok := present["thresholds"]; ok {
		base.Thresholds = saved.Thresholds
	}
	if len(saved.Levels) > 0 {
		base.Levels = saved.Levels
	}
	if _, ok := present["alerts"]; ok {
		base.Alerts = saved.Alerts
	}
	if _, ok := present["alertMax"]; ok {
		base.AlertMax = saved.AlertMax
	}
	if saved.QuietFrom != "" {
		base.QuietFrom, base.QuietTo = saved.QuietFrom, saved.QuietTo
	}
	if saved.Recipients != nil {
		base.Recipients = saved.Recipients
	}
	return base
}

// SaveConfig 校验并保存设置。
func (s *Service) SaveConfig(ctx context.Context, config Config) (Config, error) {
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return Config{}, err
	}
	row := settingRow{SettingKey: settingKey, Value: string(data), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
		return Config{}, err
	}
	return s.Config(ctx), nil
}

// ResetConfig 回到出厂值。
func (s *Service) ResetConfig(ctx context.Context) (Config, error) {
	if err := s.db.WithContext(ctx).Where("setting_key = ?", settingKey).Delete(&settingRow{}).Error; err != nil {
		return Config{}, err
	}
	return DefaultConfig(), nil
}

// SetLocation 只改查哪里的天气（App 上切「宝宝现在在哪」）。
func (s *Service) SetLocation(ctx context.Context, location Location) (Config, error) {
	config := s.Config(ctx)
	config.Location = location
	return s.SaveConfig(ctx, config)
}

func validClock(value string) bool {
	_, err := time.Parse("15:04", strings.TrimSpace(value))
	return err == nil
}

func (c *Config) validate() error {
	c.Location.Name, c.Location.Code = strings.TrimSpace(c.Location.Name), strings.TrimSpace(c.Location.Code)
	if c.Location.Code == "" && c.Location.Lat == 0 {
		return errors.New("要设置查哪里的天气")
	}
	if c.Location.Code != "" && cmaDataCode(c.Location.Code) == "" {
		return errors.New("城市编码应该是数字，比如青浦 101020800")
	}
	for name, value := range map[string]string{"早上推送": c.Slots.Morning, "中午推送": c.Slots.Noon, "晚上推送": c.Slots.Evening} {
		if value != "" && !validClock(value) {
			return fmt.Errorf("%s时间格式应为 HH:MM", name)
		}
	}
	if c.Slots.PrePickup < 0 || c.Slots.PrePickup > 180 {
		return errors.New("接之前提前 0-180 分钟")
	}
	if !validClock(c.School.Drop) || !validClock(c.School.Pickup) || !validClock(c.School.Home) {
		return errors.New("送、接、到家时间格式应为 HH:MM")
	}
	if c.School.Home <= c.School.Pickup {
		return errors.New("到家时间要晚于接的时间")
	}
	if c.School.Walk <= 0 || c.School.Walk > 120 {
		c.School.Walk = 20
	}
	for _, outing := range c.Weekend {
		if !validClock(outing.From) || !validClock(outing.To) || outing.To <= outing.From {
			return errors.New("出去玩的时段格式应为 HH:MM，结束晚于开始")
		}
	}
	for _, day := range c.SchoolDays {
		if day < 0 || day > 6 {
			return errors.New("上学日用 0-6 表示（0 = 周日）")
		}
	}
	t := c.Thresholds
	if t.BackBy < -10 || t.BackBy > 25 || t.BackByInfant < t.BackBy-5 || t.BackByInfant > 25 {
		return errors.New("回家线应在 -10 到 25 度之间")
	}
	if t.ColdMinutes <= 0 || t.FreezeMinutes <= 0 || t.AQIMask <= 0 || t.AQIStay < t.AQIMask {
		return errors.New("时长、空气阈值要大于 0，别出门的空气阈值不低于戴口罩的")
	}
	if len(c.Levels) == 0 {
		return errors.New("穿衣档位不能为空")
	}
	sort.SliceStable(c.Levels, func(i, j int) bool { return c.Levels[i].Min > c.Levels[j].Min })
	for i := range c.Levels {
		c.Levels[i].Key, c.Levels[i].Label = strings.TrimSpace(c.Levels[i].Key), strings.TrimSpace(c.Levels[i].Label)
		if c.Levels[i].Label == "" || len(c.Levels[i].Clothes) == 0 {
			return errors.New("每一档都要有名字和衣服")
		}
		if c.Levels[i].Key == "" {
			c.Levels[i].Key = fmt.Sprintf("level%d", i)
		}
	}
	c.Levels[len(c.Levels)-1].Min = -99 // 最冷那档兜底
	if c.AlertMax < 0 || c.AlertMax > 5 {
		return errors.New("临时提醒一天 0-5 条")
	}
	if !validClock(c.QuietFrom) || !validClock(c.QuietTo) {
		return errors.New("免打扰时间格式应为 HH:MM")
	}
	for _, role := range c.Recipients {
		switch role {
		case identity.RoleAdmin, identity.RoleMember, identity.RoleSpouse, identity.RoleElder:
		default:
			return fmt.Errorf("不认识的角色 %s", role)
		}
	}
	return nil
}

type settingRow struct {
	SettingKey string `gorm:"primaryKey"`
	Value      string
	UpdatedAt  time.Time
}

func (settingRow) TableName() string { return "pa_setting" }

// ---------------------------------------------------------------- 当天的送接安排

// DayPlan 是某一天的送接安排（默认 + 当天改的）。
type DayPlan struct {
	Date      string   `json:"date"`
	School    bool     `json:"school"` // 这天去幼儿园
	Drop      string   `json:"drop,omitempty"`
	Pickup    string   `json:"pickup,omitempty"`
	Home      string   `json:"home,omitempty"`
	Outings   []Outing `json:"outings"`
	Note      string   `json:"note,omitempty"`
	Override  bool     `json:"override"` // 这天临时改过
	UpdatedBy string   `json:"updatedBy,omitempty"`
}

// DayInput 改某一天。NoSchool 不去幼儿园；Drop / Pickup / Home 空 = 用默认；Outings 这天出去玩的时段（替换默认的周末时段）。
type DayInput struct {
	Date     string   `json:"date"`
	NoSchool bool     `json:"noSchool"`
	Drop     string   `json:"drop"`
	Pickup   string   `json:"pickup"`
	Home     string   `json:"home"`
	Outings  []Outing `json:"outings"`
	Note     string   `json:"note"`
}

type dayRow struct {
	ID        int64
	FamilyKey string
	ChildKey  string
	Day       string // DATE 按字符串读写，不受连接时区影响
	NoSchool  bool
	DropAt    string // drop 是 MySQL 保留字
	PickupAt  string
	HomeBy    string
	Outings   string // JSON
	Note      string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (dayRow) TableName() string { return "pa_weather_day" }

func (c Config) isSchoolDay(date time.Time) bool {
	for _, day := range c.SchoolDays {
		if int(date.Weekday()) == day {
			return true
		}
	}
	return false
}

// DayPlan 返回 date 那天的安排。
func (s *Service) DayPlan(ctx context.Context, familyKey, date string) (DayPlan, error) {
	day, err := parseDate(date)
	if err != nil {
		return DayPlan{}, err
	}
	config := s.Config(ctx)
	var row dayRow
	err = s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND day = ?", familyKey, config.ChildKey, day.Format(dateLayout)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return planOf(config, day, nil), nil
	}
	if err != nil {
		return DayPlan{}, err
	}
	return planOf(config, day, &row), nil
}

func planOf(config Config, day time.Time, row *dayRow) DayPlan {
	plan := DayPlan{Date: day.Format(dateLayout), School: config.isSchoolDay(day), Outings: []Outing{}}
	if plan.School {
		plan.Drop, plan.Pickup, plan.Home = config.School.Drop, config.School.Pickup, config.School.Home
	} else {
		plan.Outings = append(plan.Outings, config.Weekend...)
	}
	if row == nil {
		return plan
	}
	plan.Override, plan.Note, plan.UpdatedBy = true, row.Note, row.UpdatedBy
	if row.NoSchool {
		plan.School, plan.Drop, plan.Pickup, plan.Home = false, "", "", ""
	} else if row.DropAt != "" || row.PickupAt != "" || row.HomeBy != "" {
		// 改了送接时间的，就当这天要去
		plan.School = true
		plan.Drop, plan.Pickup, plan.Home = firstNonEmpty(row.DropAt, config.School.Drop), firstNonEmpty(row.PickupAt, config.School.Pickup), firstNonEmpty(row.HomeBy, config.School.Home)
	}
	if plan.School && plan.Home <= plan.Pickup {
		plan.Home = at(day, plan.Pickup).Add(time.Duration(config.School.Walk+20) * time.Minute).Format("15:04")
	}
	var outings []Outing
	if row.Outings != "" && json.Unmarshal([]byte(row.Outings), &outings) == nil && outings != nil {
		plan.Outings = outings
	} else if plan.School {
		plan.Outings = []Outing{}
	}
	return plan
}

// SaveDay 改某一天的安排。谁都能改（奶奶最清楚今天怎么安排）。
func (s *Service) SaveDay(ctx context.Context, familyKey string, input DayInput, account string) (DayPlan, error) {
	day, err := parseDate(input.Date)
	if err != nil {
		return DayPlan{}, err
	}
	if day.Before(today()) {
		return DayPlan{}, errors.New("过去的日子不用改")
	}
	for _, value := range []string{input.Drop, input.Pickup, input.Home} {
		if value != "" && !validClock(value) {
			return DayPlan{}, errors.New("时间格式应为 HH:MM")
		}
	}
	for _, outing := range input.Outings {
		if !validClock(outing.From) || !validClock(outing.To) || outing.To <= outing.From {
			return DayPlan{}, errors.New("出去玩的时段格式应为 HH:MM，结束晚于开始")
		}
	}
	config := s.Config(ctx)
	row := dayRow{FamilyKey: familyKey, ChildKey: config.ChildKey, Day: day.Format(dateLayout), NoSchool: input.NoSchool,
		DropAt: strings.TrimSpace(input.Drop), PickupAt: strings.TrimSpace(input.Pickup), HomeBy: strings.TrimSpace(input.Home),
		Note: strings.TrimSpace(input.Note), UpdatedBy: account, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if input.Outings != nil {
		data, _ := json.Marshal(input.Outings)
		row.Outings = string(data)
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "family_key"}, {Name: "child_key"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"no_school", "drop_at", "pickup_at", "home_by", "outings", "note", "updated_by", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return DayPlan{}, err
	}
	return s.DayPlan(ctx, familyKey, day.Format(dateLayout))
}

// ResetDay 这天恢复默认安排。
func (s *Service) ResetDay(ctx context.Context, familyKey, date string) (DayPlan, error) {
	day, err := parseDate(date)
	if err != nil {
		return DayPlan{}, err
	}
	config := s.Config(ctx)
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND day = ?", familyKey, config.ChildKey, day.Format(dateLayout)).
		Delete(&dayRow{}).Error; err != nil {
		return DayPlan{}, err
	}
	return planOf(config, day, nil), nil
}

// ListDays 列出 from 起 days 天的安排（管理端「安排」一览）。
func (s *Service) ListDays(ctx context.Context, familyKey, from string, days int) ([]DayPlan, error) {
	start, err := parseDate(from)
	if err != nil {
		return nil, err
	}
	if days <= 0 || days > 31 {
		days = 7
	}
	config := s.Config(ctx)
	var rows []dayRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND day >= ? AND day < ?", familyKey, config.ChildKey,
		start.Format(dateLayout), start.AddDate(0, 0, days).Format(dateLayout)).Find(&rows).Error; err != nil {
		return nil, err
	}
	byDay := map[string]*dayRow{}
	for i := range rows {
		byDay[dateOnly(rows[i].Day)] = &rows[i]
	}
	result := make([]DayPlan, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		result = append(result, planOf(config, day, byDay[day.Format(dateLayout)]))
	}
	return result, nil
}

// dateOnly DATE 列读出来可能是 2026-10-21 或 2026-10-21T00:00:00Z，取前 10 位。
func dateOnly(value string) string {
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
