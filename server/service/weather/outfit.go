package weather

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// 穿衣规则：纯函数，只看输入，不查库不联网，好测。数字和钟点都在这里算定，模型只负责把它说成家常话。

const (
	SlotMorning   = "morning"
	SlotNoon      = "noon"
	SlotPrePickup = "prepickup"
	SlotEvening   = "evening"
	SlotAlert     = "alert"
	SlotNow       = "now" // 页面上「现在看一下」，不存不推
)

var slotLabels = map[string]string{SlotMorning: "早上", SlotNoon: "中午", SlotPrePickup: "接之前", SlotEvening: "晚上", SlotAlert: "临时提醒", SlotNow: "现在"}

func SlotLabel(slot string) string {
	if strings.HasPrefix(slot, SlotAlert) {
		return slotLabels[SlotAlert]
	}
	if label, ok := slotLabels[slot]; ok {
		return label
	}
	return slot
}

// Input 是算一次穿衣建议要的全部东西。
type Input struct {
	Slot      string
	Now       time.Time
	Snapshot  Snapshot
	Plan      DayPlan  // 今天的安排
	Tomorrow  *DayPlan // 晚上那次用：明天的安排
	AgeMonths int      // 不知道生日时为 -1
	Config    Config
}

// Part 是一段要在外面的时间：早上送、下午接、出去玩、明早送。
type Part struct {
	Kind       string     `json:"kind"` // drop / pickup / outing
	Label      string     `json:"label"`
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	FeelsMin   float64    `json:"feelsMin"`
	FeelsMax   float64    `json:"feelsMax"`
	TempMin    float64    `json:"tempMin"`
	TempMax    float64    `json:"tempMax"`
	Level      Level      `json:"level"`
	Layers     int        `json:"layers"`
	Clothes    []string   `json:"clothes"`
	Extras     []string   `json:"extras"`
	Leave      *time.Time `json:"leave,omitempty"`
	BackBy     *time.Time `json:"backBy,omitempty"`
	BackReason string     `json:"backReason,omitempty"`
	PlayUntil  *time.Time `json:"playUntil,omitempty"`
	DirectHome bool       `json:"directHome"`
	Skip       bool       `json:"skip"`
	SkipReason string     `json:"skipReason,omitempty"`
	RainFrom   *time.Time `json:"rainFrom,omitempty"`
	Notes      []string   `json:"notes"`
	Estimated  bool       `json:"estimated"` // 没有逐小时预报，按实况 / 当天最高最低估的
}

// Night 是夜里睡觉。
type Night struct {
	Low     float64  `json:"low"`
	Bedding string   `json:"bedding"`
	Tips    []string `json:"tips"`
}

// Advice 是一次穿衣建议的全部结论。
type Advice struct {
	Slot      string    `json:"slot"`
	Date      string    `json:"date"`
	Place     string    `json:"place"`
	Source    string    `json:"source"`
	Stale     bool      `json:"stale"`
	FetchedAt time.Time `json:"fetchedAt"`
	Now       Now       `json:"now"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	HasRange  bool      `json:"hasRange"`
	Sunset    string    `json:"sunset"`
	Warnings  []Warning `json:"warnings"`
	Clothing  string    `json:"clothing"`
	Infant    bool      `json:"infant"`
	Parts     []Part    `json:"parts"`
	Night     *Night    `json:"night,omitempty"`
	Tomorrow  *Day      `json:"tomorrow,omitempty"`
	Notes     []string  `json:"notes"`
	Headline  string    `json:"headline"`
}

// Advise 算穿衣建议。
func Advise(in Input) Advice {
	snapshot, config := in.Snapshot, in.Config
	date, _ := time.ParseInLocation(dateLayout, in.Plan.Date, shanghai)
	if in.Plan.Date == "" {
		date = time.Date(in.Now.Year(), in.Now.Month(), in.Now.Day(), 0, 0, 0, 0, shanghai)
	}
	advice := Advice{Slot: in.Slot, Date: date.Format(dateLayout), Place: snapshot.Location.Name, Source: snapshot.Source,
		Stale: snapshot.Stale, FetchedAt: snapshot.FetchedAt, Now: snapshot.Now, Clothing: snapshot.Clothing,
		Infant: in.AgeMonths >= 0 && in.AgeMonths < 12, Parts: []Part{}, Notes: []string{}, Warnings: []Warning{}}
	if day, ok := snapshot.Day(advice.Date); ok {
		advice.High, advice.Low, advice.HasRange, advice.Sunset = day.High, day.Low, true, day.Sunset
	}
	for _, warning := range snapshot.Warnings {
		if warning.Serious() {
			advice.Warnings = append(advice.Warnings, warning)
		}
	}
	rules := ruleSet{config: config, snapshot: snapshot, infant: advice.Infant, now: in.Now}

	switch in.Slot {
	case SlotEvening:
		advice.Night = rules.night(in.Now)
		if in.Tomorrow != nil {
			tomorrow, _ := time.ParseInLocation(dateLayout, in.Tomorrow.Date, shanghai)
			if day, ok := snapshot.Day(in.Tomorrow.Date); ok {
				advice.Tomorrow = &day
			}
			if part, ok := rules.firstPart(tomorrow, *in.Tomorrow, "明天"); ok {
				advice.Parts = append(advice.Parts, part)
			}
		}
	case SlotMorning:
		if part, ok := rules.firstPart(date, in.Plan, ""); ok {
			advice.Parts = append(advice.Parts, part)
		}
		advice.Parts = append(advice.Parts, rules.afternoonParts(date, in.Plan)...)
	default: // noon / prepickup / alert / now：还没过去的那一段
		for _, part := range append(rules.morningParts(date, in.Plan), rules.afternoonParts(date, in.Plan)...) {
			if part.End.After(in.Now) {
				advice.Parts = append(advice.Parts, part)
			}
		}
	}
	if advice.HasRange && advice.High-advice.Low >= 10 && in.Slot != SlotEvening {
		advice.Notes = append(advice.Notes, fmt.Sprintf("今天早晚温差大（%s–%s），穿能脱的", deg(advice.Low), deg(advice.High)))
	}
	if aqi := snapshot.Now.AQI; aqi > config.Thresholds.AQIMask && in.Slot != SlotEvening {
		advice.Notes = append(advice.Notes, fmt.Sprintf("空气不好（AQI %d），出门戴口罩", aqi))
	}
	if advice.Infant {
		advice.Notes = append(advice.Notes, "摸后颈，温热不出汗就合适")
	}
	advice.Headline = headline(advice)
	return advice
}

type ruleSet struct {
	config   Config
	snapshot Snapshot
	infant   bool
	now      time.Time
}

func (r ruleSet) backLine() float64 {
	if r.infant {
		return r.config.Thresholds.BackByInfant
	}
	return r.config.Thresholds.BackBy
}

func (r ruleSet) level(feels float64) Level {
	levels := append([]Level(nil), r.config.Levels...)
	sort.SliceStable(levels, func(i, j int) bool { return levels[i].Min > levels[j].Min })
	for _, level := range levels {
		if feels >= level.Min {
			return level
		}
	}
	if len(levels) == 0 {
		return Level{Key: "unknown", Label: "不确定", Clothes: []string{"按平时穿"}}
	}
	return levels[len(levels)-1]
}

func isCold(level Level) bool {
	switch level.Key {
	case "cold", "chilly", "freezing":
		return true
	}
	return false
}

// stats 统计 [from, to) 这段时间的体感、温度、雨、风。没有逐小时预报就用实况 / 当天最高最低估。
func (r ruleSet) stats(part *Part, from, to time.Time) []Hour {
	hours := r.snapshot.Between(from, to)
	if len(hours) == 0 {
		part.Estimated = true
		now := r.snapshot.Now
		feels, temp := now.Feels, now.Temp
		if day, ok := r.snapshot.Day(from.Format(dateLayout)); ok && from.Sub(r.now) > 3*time.Hour {
			// 离现在远：早上 / 晚上按最低温估，白天按平均估
			temp = (day.High + day.Low) / 2
			if from.Hour() < 9 || from.Hour() >= 17 {
				temp = day.Low + (day.High-day.Low)*0.2
			}
			feels = apparent(temp, 70, 3)
		}
		part.FeelsMin, part.FeelsMax, part.TempMin, part.TempMax = feels, feels, temp, temp
		return nil
	}
	part.FeelsMin, part.FeelsMax, part.TempMin, part.TempMax = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, hour := range hours {
		part.FeelsMin, part.FeelsMax = math.Min(part.FeelsMin, hour.Feels), math.Max(part.FeelsMax, hour.Feels)
		part.TempMin, part.TempMax = math.Min(part.TempMin, hour.Temp), math.Max(part.TempMax, hour.Temp)
	}
	return hours
}

// dress 按这段时间最低的体感定穿什么。
func (r ruleSet) dress(part *Part, hours []Hour) {
	part.Level = r.level(part.FeelsMin)
	part.Layers = part.Level.Layers
	part.Clothes = append([]string(nil), part.Level.Clothes...)
	if r.infant {
		part.Layers++
		part.Clothes = append(part.Clothes, "多加一件薄的")
	}
	if part.Level.Tip != "" {
		part.Notes = append(part.Notes, part.Level.Tip)
	}
	maxWind := 0
	for _, hour := range hours {
		if hour.WindLevel > maxWind {
			maxWind = hour.WindLevel
		}
		if hour.Wet && part.RainFrom == nil {
			when := hour.At
			if when.Before(part.Start) {
				when = part.Start
			}
			part.RainFrom = &when
		}
	}
	if part.RainFrom != nil {
		part.Extras = append(part.Extras, "雨伞或雨衣", "防水鞋")
	}
	if maxWind >= 6 {
		part.Extras = append(part.Extras, "挡风的外套")
		part.Notes = append(part.Notes, "风大，别在空旷的地方久待")
	} else if maxWind >= 4 && part.FeelsMin < 18 {
		part.Extras = append(part.Extras, "挡风的外套")
	}
	if part.Level.Key == "hot" {
		part.Extras = append(part.Extras, "遮阳帽", "水")
	}
	if aqi := r.snapshot.Now.AQI; aqi > r.config.Thresholds.AQIMask {
		part.Extras = append(part.Extras, "口罩")
	}
}

// stay 预警、空气很差时不该在外面玩。
func (r ruleSet) stay() (bool, string) {
	for _, warning := range r.snapshot.Warnings {
		if warning.Serious() {
			return true, "有" + warning.Title()
		}
	}
	if aqi := r.snapshot.Now.AQI; aqi > r.config.Thresholds.AQIStay {
		return true, fmt.Sprintf("空气很差（AQI %d）", aqi)
	}
	return false, ""
}

// limit 从 from 往后找该回家的时刻：体感低于回家线、开始下雨（提前半小时）、天黑后半小时，取最早的。
func (r ruleSet) limit(date, from time.Time, horizon time.Duration) (time.Time, string) {
	var best time.Time
	var reason string
	consider := func(when time.Time, why string) {
		if !when.IsZero() && (best.IsZero() || when.Before(best)) {
			best, reason = when, why
		}
	}
	line := r.backLine()
	for _, hour := range r.snapshot.Between(from, from.Add(horizon)) {
		if hour.Feels < line {
			when := hour.At
			if when.Before(from) {
				when = from
			}
			consider(when, fmt.Sprintf("%s 以后体感降到 %s", clock(when), deg(hour.Feels)))
			break
		}
	}
	for _, hour := range r.snapshot.Between(from, from.Add(horizon)) {
		if hour.Wet {
			consider(hour.At.Add(-30*time.Minute), fmt.Sprintf("%s 左右开始%s", clock(hour.At), rainWord(hour.Text)))
			break
		}
	}
	if day, ok := r.snapshot.Day(date.Format(dateLayout)); ok {
		if sunset := at(date, day.Sunset); !sunset.IsZero() {
			consider(sunset.Add(30*time.Minute), fmt.Sprintf("%s 天黑", clock(sunset)))
		}
	}
	return best, reason
}

func rainWord(text string) string {
	if strings.Contains(text, "雪") {
		return "下雪"
	}
	return "下雨"
}

// morningParts 早上送 / 上午出去玩。
func (r ruleSet) morningParts(date time.Time, plan DayPlan) []Part {
	var parts []Part
	if plan.School {
		if drop := at(date, plan.Drop); !drop.IsZero() {
			parts = append(parts, r.dropPart(date, plan, drop, ""))
		}
	}
	for _, outing := range plan.Outings {
		if at(date, outing.From).Hour() < 12 {
			parts = append(parts, r.outingPart(date, outing, ""))
		}
	}
	return parts
}

// afternoonParts 下午接 / 下午出去玩。
func (r ruleSet) afternoonParts(date time.Time, plan DayPlan) []Part {
	var parts []Part
	if plan.School {
		if pickup := at(date, plan.Pickup); !pickup.IsZero() {
			parts = append(parts, r.pickupPart(date, plan, pickup))
		}
	}
	for _, outing := range plan.Outings {
		if at(date, outing.From).Hour() >= 12 {
			parts = append(parts, r.outingPart(date, outing, ""))
		}
	}
	return parts
}

// firstPart 这一天第一次出门（明早送、早上送，或者第一段出去玩）。
func (r ruleSet) firstPart(date time.Time, plan DayPlan, prefix string) (Part, bool) {
	if plan.School {
		if drop := at(date, plan.Drop); !drop.IsZero() {
			return r.dropPart(date, plan, drop, prefix), true
		}
	}
	if len(plan.Outings) > 0 {
		return r.outingPart(date, plan.Outings[0], prefix), true
	}
	return Part{}, false
}

func (r ruleSet) dropPart(date time.Time, plan DayPlan, drop time.Time, prefix string) Part {
	part := Part{Kind: "drop", Label: prefix + "早上送", Start: drop, End: drop.Add(30 * time.Minute), Notes: []string{}, Extras: []string{}}
	if prefix != "" {
		part.Label = "明早送"
	}
	part.Leave = &part.Start
	hours := r.stats(&part, part.Start, part.End)
	r.dress(&part, hours)
	// 在幼儿园一整天：看到接的时候
	pickup := at(date, plan.Pickup)
	if pickup.IsZero() {
		pickup = drop.Add(8 * time.Hour)
	}
	day := r.snapshot.Between(part.Start, pickup)
	warmest := part.FeelsMax
	for _, hour := range day {
		warmest = math.Max(warmest, hour.Feels)
		if hour.Wet && part.RainFrom == nil {
			when := hour.At
			part.RainFrom = &when
			part.Extras = append(part.Extras, "雨伞（放书包里）")
		}
	}
	if warmest-part.FeelsMin >= 8 {
		part.Notes = append(part.Notes, fmt.Sprintf("早上 %s、中午到 %s，穿能脱的，外套进教室脱", deg(part.FeelsMin), deg(warmest)))
		part.Extras = append(part.Extras, "一件备用的薄衣服（放书包里，出汗湿了能换）")
	}
	return part
}

func (r ruleSet) pickupPart(date time.Time, plan DayPlan, pickup time.Time) Part {
	home := at(date, plan.Home)
	if !home.After(pickup) {
		home = pickup.Add(40 * time.Minute)
	}
	part := Part{Kind: "pickup", Label: "下午接", Start: pickup, End: home, Notes: []string{}, Extras: []string{}}
	part.Leave = &part.Start
	hours := r.stats(&part, pickup, home)
	r.dress(&part, hours)
	walk := time.Duration(r.config.School.Walk) * time.Minute
	if walk <= 0 {
		walk = 20 * time.Minute
	}
	direct := pickup.Add(walk)
	stay, stayReason := r.stay()
	limit, limitReason := r.limit(date, pickup, 4*time.Hour)
	// 接的那一刻已经低于回家线才直接回；之后才降下来的，由 limit 定几点到家
	atPickup := part.FeelsMin
	if len(hours) > 0 {
		atPickup = hours[0].Feels
	}

	switch {
	case stay:
		part.DirectHome, part.BackReason = true, stayReason
	case atPickup < r.backLine():
		part.DirectHome, part.BackReason = true, fmt.Sprintf("接的时候体感只有 %s", deg(atPickup))
	case part.RainFrom != nil:
		part.DirectHome, part.BackReason = true, fmt.Sprintf("%s 左右有%s", clock(*part.RainFrom), strings.TrimPrefix(rainWord(textAt(hours, *part.RainFrom)), "下"))
	case part.Level.Key == "freezing":
		part.DirectHome, part.BackReason = true, "太冷了"
	}
	if part.DirectHome {
		part.BackBy = &direct
		return part
	}
	backBy := home
	if !limit.IsZero() && limit.Before(home) {
		backBy = floor5(limit)
		if backBy.Before(direct) {
			backBy = direct
		}
		part.BackReason = limitReason
		part.DirectHome = !backBy.After(direct.Add(5 * time.Minute))
	}
	if isCold(part.Level) {
		capped := pickup.Add(time.Duration(r.config.Thresholds.ColdMinutes) * time.Minute)
		if part.Level.Key == "freezing" {
			capped = pickup.Add(time.Duration(r.config.Thresholds.FreezeMinutes) * time.Minute)
		}
		if capped.Before(backBy) {
			backBy, part.BackReason = capped, "天冷，在外面别太久"
		}
	}
	part.BackBy = &backBy
	// 天气好：想多玩可以到几点（不超过回家线、天黑，最多接后两个半小时）
	if backBy.Equal(home) && !isCold(part.Level) {
		until := pickup.Add(150 * time.Minute)
		if !limit.IsZero() && limit.Before(until) {
			until = limit
		}
		until = floor5(until)
		if until.After(home.Add(15 * time.Minute)) {
			part.PlayUntil = &until
		}
	}
	return part
}

func (r ruleSet) outingPart(date time.Time, outing Outing, prefix string) Part {
	from, to := at(date, outing.From), at(date, outing.To)
	label := fmt.Sprintf("%s出去玩 %s–%s", prefix, outing.From, outing.To)
	if outing.Note != "" {
		label = fmt.Sprintf("%s%s %s–%s", prefix, outing.Note, outing.From, outing.To)
	}
	part := Part{Kind: "outing", Label: label, Start: from, End: to, Notes: []string{}, Extras: []string{}}
	part.Leave = &part.Start
	hours := r.stats(&part, from, to)
	r.dress(&part, hours)
	if stay, reason := r.stay(); stay {
		part.Skip, part.SkipReason = true, reason+"，今天别出去玩了"
		return part
	}
	if part.FeelsMax < r.config.Thresholds.GoOut {
		part.Skip, part.SkipReason = true, fmt.Sprintf("体感最高才 %s，别出去玩了，在家活动", deg(part.FeelsMax))
		return part
	}
	if part.RainFrom != nil && !part.RainFrom.After(from) {
		part.Skip, part.SkipReason = true, "这段时间在下雨，改在室内玩"
		return part
	}
	backBy := to
	if limit, reason := r.limit(date, from, to.Sub(from)+time.Hour); !limit.IsZero() && limit.Before(backBy) {
		backBy, part.BackReason = floor5(limit), reason
	}
	if isCold(part.Level) {
		minutes := r.config.Thresholds.ColdMinutes
		if part.Level.Key == "freezing" {
			minutes = r.config.Thresholds.FreezeMinutes
		}
		if capped := from.Add(time.Duration(minutes) * time.Minute); capped.Before(backBy) {
			backBy, part.BackReason = capped, "天冷，在外面别太久"
		}
	}
	if !backBy.After(from.Add(15 * time.Minute)) {
		part.Skip, part.SkipReason = true, part.BackReason+"，这段时间别出去了"
		return part
	}
	part.BackBy = &backBy
	if part.Level.Key == "hot" && from.Hour() < 15 && to.Hour() >= 11 {
		part.Notes = append(part.Notes, "中午太晒，能换到早上或傍晚更好")
	}
	return part
}

// night 今晚睡觉：按到明早 7 点的最低气温（不是体感，屋里没风）。
func (r ruleSet) night(now time.Time) *Night {
	tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, shanghai).AddDate(0, 0, 1)
	hours := r.snapshot.Between(now, tomorrow)
	low := math.Inf(1)
	for _, hour := range hours {
		low = math.Min(low, hour.Temp)
	}
	if math.IsInf(low, 1) {
		if day, ok := r.snapshot.Day(tomorrow.Format(dateLayout)); ok {
			low = day.Low
		} else {
			low = r.snapshot.Now.Temp
		}
	}
	night := &Night{Low: low, Tips: []string{}}
	switch {
	case low >= 26:
		night.Bedding = "毛巾被，开空调 26 度左右，别对着吹"
	case low >= 21:
		night.Bedding = "薄被"
	case low >= 16:
		night.Bedding = "春秋被，穿长袖睡衣"
	case low >= 10:
		night.Bedding = "厚一点的被子，长袖长裤睡衣"
	default:
		night.Bedding = "厚被子 + 防踢睡袋，开暖气别太干"
	}
	if day, ok := r.snapshot.Day(now.Format(dateLayout)); ok && day.High-low >= 8 {
		night.Tips = append(night.Tips, "夜里比白天凉不少，半夜看一眼有没有踢被子")
	}
	return night
}

func textAt(hours []Hour, when time.Time) string {
	for _, hour := range hours {
		if !hour.At.After(when) && hour.At.Add(time.Hour).After(when) {
			return hour.Text
		}
	}
	return "雨"
}

func floor5(value time.Time) time.Time {
	return value.Truncate(5 * time.Minute)
}

func clock(value time.Time) string { return value.In(shanghai).Format("15:04") }

func deg(value float64) string { return fmt.Sprintf("%d°", int(math.Round(value))) }

// headline 一句话结论，推送标题用。
func headline(advice Advice) string {
	if len(advice.Warnings) > 0 {
		var titles []string
		for _, warning := range advice.Warnings {
			titles = append(titles, warning.Title())
		}
		action := "今天少出门"
		for _, part := range advice.Parts {
			if part.Kind == "pickup" && part.BackBy != nil {
				action = "接到直接回，" + clock(*part.BackBy) + " 前到家"
			}
		}
		return strings.Join(titles, "、") + "，" + action
	}
	if advice.Slot == SlotEvening {
		if len(advice.Parts) > 0 {
			part := advice.Parts[0]
			return fmt.Sprintf("%s体感 %s，穿%s", part.Label, deg(part.FeelsMin), strings.Join(part.Clothes, "+"))
		}
		if advice.Night != nil {
			return fmt.Sprintf("今晚最低 %s，盖%s", deg(advice.Night.Low), firstPhrase(advice.Night.Bedding))
		}
	}
	for _, part := range advice.Parts {
		if part.Kind == "pickup" && part.BackBy != nil && (part.DirectHome || part.BackReason != "") {
			prefix := "今天冷"
			if part.RainFrom != nil {
				prefix = "下午有雨"
			} else if !isCold(part.Level) && part.FeelsMin >= 10 {
				prefix = "下午变天"
			}
			if part.DirectHome {
				return fmt.Sprintf("%s，接到直接回，%s 前到家", prefix, clock(*part.BackBy))
			}
			return fmt.Sprintf("%s，%s 前到家", prefix, clock(*part.BackBy))
		}
		if part.Kind == "outing" && part.Skip {
			return part.SkipReason
		}
	}
	if len(advice.Parts) > 0 {
		part := advice.Parts[0]
		text := fmt.Sprintf("%s体感 %s，穿%s", part.Label, deg(part.FeelsMin), strings.Join(part.Clothes, "+"))
		if advice.Slot == SlotMorning && advice.HasRange {
			text = fmt.Sprintf("今天 %s–%s，", deg(advice.Low), deg(advice.High)) + text
		}
		return text
	}
	return fmt.Sprintf("现在 %s，体感 %s，%s", deg(advice.Now.Temp), deg(advice.Now.Feels), advice.Now.Text)
}

func firstPhrase(text string) string {
	if index := strings.IndexAny(text, "，,"); index > 0 {
		return text[:index]
	}
	return text
}
