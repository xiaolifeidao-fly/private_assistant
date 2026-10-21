// Package weather 是天气和「宝宝穿衣」提醒（设计文档 10）。
//
//   - 天气：中国天气网（百度搜「青浦天气」那张卡片的数据，不用 key）为主，Open-Meteo 备用（cma.go / openmeteo.go）；
//   - 穿衣：按体感、雨、风、预警、空气和当天送接安排，用规则算出穿什么、几点前到家（outfit.go，纯函数）；
//   - 提醒：每天早 / 中 / 接之前 / 晚，模型把规则结果写成家常话，推给我和奶奶（brief.go）。
//
// 数字和时间只由代码算，模型写的话里的温度、钟点对不上就改用模板。全家共用，不分私有。
package weather

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	dateLayout = "2006-01-02"
	cacheTTL   = 20 * time.Minute
	// staleLimit 两个源都取不到时，缓存里最近一次超过这么久就不用了
	staleLimit = 3 * time.Hour
)

var ErrUnavailable = errors.New("天气暂时查不到")

var shanghai = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}()

// Location 是查天气的地方。Code 是中国天气网的城市编码（青浦 101020800），Lat / Lon 给备用的 Open-Meteo 用。
type Location struct {
	Name string  `json:"name"`
	Code string  `json:"code"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// Now 是实况。
type Now struct {
	Temp      float64   `json:"temp"`
	Feels     float64   `json:"feels"` // 体感，代码按温度、湿度、风算（feels.go）
	Humidity  float64   `json:"humidity"`
	Text      string    `json:"text"`
	Wind      string    `json:"wind"`      // 西北风 2级
	WindSpeed float64   `json:"windSpeed"` // m/s
	Rain1h    float64   `json:"rain1h"`
	AQI       int       `json:"aqi"` // 0 = 没有
	At        time.Time `json:"at"`
}

// Hour 是逐小时预报的一个小时（At 是这个小时的开始）。
type Hour struct {
	At        time.Time `json:"at"`
	Temp      float64   `json:"temp"`
	Feels     float64   `json:"feels"`
	Humidity  float64   `json:"humidity"`
	Text      string    `json:"text"`
	Rain      float64   `json:"rain"` // 降水量 mm
	Wet       bool      `json:"wet"`  // 下雨 / 下雪（天气现象或降水量）
	WindLevel int       `json:"windLevel"`
	WindSpeed float64   `json:"windSpeed"`
}

// Day 是按天的预报。
type Day struct {
	Date      string  `json:"date"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	DayText   string  `json:"dayText"`
	NightText string  `json:"nightText"`
	Sunrise   string  `json:"sunrise"` // 06:01
	Sunset    string  `json:"sunset"`  // 17:19
}

// Warning 是气象预警。
type Warning struct {
	Type  string `json:"type"`  // 寒潮、大风、暴雨……
	Level string `json:"level"` // 蓝色、黄色、橙色、红色
	Text  string `json:"text"`
	At    string `json:"at"`
}

// Title 「大风黄色预警」
func (w Warning) Title() string { return w.Type + w.Level + "预警" }

// Serious 黄色及以上。
func (w Warning) Serious() bool {
	return strings.Contains(w.Level, "黄") || strings.Contains(w.Level, "橙") || strings.Contains(w.Level, "红")
}

// Snapshot 是一次取到的全部天气。
type Snapshot struct {
	Location  Location  `json:"location"`
	Source    string    `json:"source"` // cma / open-meteo
	Now       Now       `json:"now"`
	Hours     []Hour    `json:"hours"`
	Days      []Day     `json:"days"`
	Warnings  []Warning `json:"warnings"`
	Clothing  string    `json:"clothing"` // 中国天气网穿衣指数，只作参考
	FetchedAt time.Time `json:"fetchedAt"`
	Stale     bool      `json:"stale"` // 取不到新的，用的是缓存里旧的
}

// Day 返回 date 那天的按天预报。
func (s Snapshot) Day(date string) (Day, bool) {
	for _, day := range s.Days {
		if day.Date == date {
			return day, true
		}
	}
	return Day{}, false
}

// Between 返回 [from, to) 之间的小时（含 from 所在的那个小时）。
func (s Snapshot) Between(from, to time.Time) []Hour {
	start := from.Truncate(time.Hour)
	var result []Hour
	for _, hour := range s.Hours {
		if !hour.At.Before(start) && hour.At.Before(to) {
			result = append(result, hour)
		}
	}
	return result
}

// Provider 是一个天气源。
type Provider interface {
	Name() string
	Fetch(ctx context.Context, location Location) (Snapshot, error)
}

// Status 是天气源的情况，管理端看。
type Status struct {
	Source      string    `json:"source"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastError   string    `json:"lastError"`
	LastErrorAt time.Time `json:"lastErrorAt"`
	Calls       int       `json:"calls"` // 今天（本进程）请求了几次
	CallsDate   string    `json:"callsDate"`
}

type cacheEntry struct {
	snapshot Snapshot
	at       time.Time
}

type Service struct {
	db        *gorm.DB
	providers []Provider
	family    FamilyPort
	llm       Chatter
	reminders ReminderPort
	accounts  func(ctx context.Context, roles []string) []Recipient

	mu     sync.Mutex
	cache  map[string]cacheEntry
	status Status
	// fetching 同一个地方同时只取一次
	fetching sync.Mutex
}

func New(database *gorm.DB, familyService FamilyPort, gateway Chatter) *Service {
	return &Service{db: database, family: familyService, llm: gateway,
		providers: []Provider{newCMA(), newOpenMeteo()}, cache: map[string]cacheEntry{}}
}

// SetNotifier 接上推送：reminders 建提醒，accounts 按角色返回手机端账号。由 wire 注入，没接就只存不推。
func (s *Service) SetNotifier(reminders ReminderPort, accounts func(ctx context.Context, roles []string) []Recipient) {
	s.reminders, s.accounts = reminders, accounts
}

// Snapshot 返回配置的地方现在的天气（20 分钟缓存）。
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	return s.SnapshotAt(ctx, s.Config(ctx).Location, false)
}

// SnapshotAt 返回 location 的天气。fresh=true 时不用缓存（管理端「重新取」）。
func (s *Service) SnapshotAt(ctx context.Context, location Location, fresh bool) (Snapshot, error) {
	if location.Code == "" && location.Lat == 0 {
		return Snapshot{}, errors.New("还没设置查哪里的天气")
	}
	key := location.Code + "|" + fmt.Sprintf("%.3f,%.3f", location.Lat, location.Lon)
	s.fetching.Lock()
	defer s.fetching.Unlock()
	s.mu.Lock()
	entry, ok := s.cache[key]
	s.mu.Unlock()
	if ok && !fresh && time.Since(entry.at) < cacheTTL {
		return entry.snapshot, nil
	}
	var errs []string
	for _, provider := range s.providers {
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		snapshot, err := provider.Fetch(fetchCtx, location)
		cancel()
		s.count()
		if err == nil {
			snapshot.Location, snapshot.Source, snapshot.FetchedAt = location, provider.Name(), time.Now()
			s.mu.Lock()
			s.cache[key] = cacheEntry{snapshot: snapshot, at: time.Now()}
			s.status.Source, s.status.LastSuccess = provider.Name(), time.Now()
			s.mu.Unlock()
			return snapshot, nil
		}
		log.Printf("天气（%s，%s）: %v", provider.Name(), location.Name, err)
		errs = append(errs, provider.Name()+"："+err.Error())
	}
	s.mu.Lock()
	s.status.LastError, s.status.LastErrorAt = strings.Join(errs, "；"), time.Now()
	s.mu.Unlock()
	if ok && time.Since(entry.at) < staleLimit {
		stale := entry.snapshot
		stale.Stale = true
		return stale, nil
	}
	return Snapshot{}, fmt.Errorf("%w：%s", ErrUnavailable, strings.Join(errs, "；"))
}

func (s *Service) count() {
	today := time.Now().In(shanghai).Format(dateLayout)
	s.mu.Lock()
	if s.status.CallsDate != today {
		s.status.CallsDate, s.status.Calls = today, 0
	}
	s.status.Calls++
	s.mu.Unlock()
}

// Status 返回天气源的情况。
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// SearchLocations 按名字搜地方（中国天气网城市搜索），给设置页选。
func (s *Service) SearchLocations(ctx context.Context, query string) ([]Location, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Location{}, nil
	}
	return searchCMA(ctx, query)
}

func today() time.Time {
	now := time.Now().In(shanghai)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, shanghai)
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return today(), nil
	}
	parsed, err := time.ParseInLocation(dateLayout, value, shanghai)
	if err != nil {
		return time.Time{}, fmt.Errorf("日期格式应为 YYYY-MM-DD：%s", value)
	}
	return parsed, nil
}

// at 把 date 那天的 "15:04" 变成时间；格式不对返回零值。
func at(date time.Time, clock string) time.Time {
	parsed, err := time.Parse("15:04", strings.TrimSpace(clock))
	if err != nil {
		return time.Time{}
	}
	return time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), 0, 0, shanghai)
}
