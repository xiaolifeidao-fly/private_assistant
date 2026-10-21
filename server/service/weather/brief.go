package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"common/keys"
	"common/viewer"
	"service/family"
	"service/identity"
	"service/llm"
	"service/reminder"

	"gorm.io/gorm/clause"
)

// 每天早 / 中 / 接之前 / 晚：算穿衣建议 → 模型写成两三句家常话（对不上就用模板）→ 存 pa_weather_brief → 推给我和奶奶。
const (
	taskBrief     = "weather.brief"
	schedulerTick = time.Minute
	retryAfter    = 5 * time.Minute
	slotWindow    = time.Hour // 过了推送时间一小时就不补了
)

type Chatter interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
}

// ReminderPort 用来推到手机（service/reminder）。
type ReminderPort interface {
	CreateManual(ctx context.Context, familyKey string, input reminder.ManualInput) (reminder.Reminder, error)
}

// Recipient 是要推的手机端账号。
type Recipient struct {
	Account string
	Role    string
}

type briefRow struct {
	ID        int64
	BriefKey  string
	FamilyKey string
	ChildKey  string
	PlaceCode string
	BriefDate string // DATE 按字符串读写，不受连接时区影响
	Slot      string
	Snapshot  *string
	Advice    *string
	Content   *string
	Source    string
	PushedTo  *string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (briefRow) TableName() string { return "pa_weather_brief" }

// Brief 是一条穿衣提醒。
type Brief struct {
	BriefKey  string    `json:"briefKey"`
	Date      string    `json:"date"`
	Slot      string    `json:"slot"`
	SlotLabel string    `json:"slotLabel"`
	Headline  string    `json:"headline"`
	Content   string    `json:"content"`
	Source    string    `json:"source"` // llm / template / skipped（没变化没推）
	PushedTo  []string  `json:"pushedTo"`
	Advice    *Advice   `json:"advice,omitempty"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (row briefRow) toDTO() Brief {
	brief := Brief{BriefKey: row.BriefKey, Date: dateOnly(row.BriefDate), Slot: row.Slot, SlotLabel: SlotLabel(row.Slot),
		Source: row.Source, PushedTo: []string{}, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.Content != nil {
		brief.Content = *row.Content
	}
	if row.PushedTo != nil {
		_ = json.Unmarshal([]byte(*row.PushedTo), &brief.PushedTo)
	}
	if row.Advice != nil {
		var advice Advice
		if json.Unmarshal([]byte(*row.Advice), &advice) == nil {
			brief.Advice, brief.Headline = &advice, advice.Headline
		}
	}
	return brief
}

func strPtr(value string) *string { return &value }

// childInfo 孩子的人物 key 和月龄（不知道生日为 -1）。
func (s *Service) childInfo(ctx context.Context, familyKey, childKey string) (string, string, int) {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return childKey, "宝宝", -1
	}
	for _, person := range persons {
		if person.PersonKey != childKey {
			continue
		}
		label := firstNonEmpty(person.Name, person.RoleLabel, "宝宝")
		born, err := time.ParseInLocation(dateLayout, strings.TrimSpace(person.Birthday), shanghai)
		if err != nil {
			return childKey, label, -1
		}
		now := time.Now().In(shanghai)
		months := (now.Year()-born.Year())*12 + int(now.Month()) - int(born.Month())
		if now.Day() < born.Day() {
			months--
		}
		return childKey, label, months
	}
	return childKey, "宝宝", -1
}

// Advise 按现在的天气和 date 那天的安排算 slot 的穿衣建议。date 不是今天时，按那天 slot 的推送时间算。
func (s *Service) Advise(ctx context.Context, familyKey, slot, date string) (Advice, Snapshot, error) {
	day, err := parseDate(date)
	if err != nil {
		return Advice{}, Snapshot{}, err
	}
	config := s.Config(ctx)
	snapshot, err := s.SnapshotAt(ctx, config.Location, false)
	if err != nil {
		return Advice{}, Snapshot{}, err
	}
	plan, err := s.DayPlan(ctx, familyKey, day.Format(dateLayout))
	if err != nil {
		return Advice{}, Snapshot{}, err
	}
	now := time.Now().In(shanghai)
	if !day.Equal(today()) {
		now = slotTime(config, day, plan, slot)
		if now.IsZero() {
			now = day.Add(7 * time.Hour)
		}
	}
	input := Input{Slot: slot, Now: now, Snapshot: snapshot, Plan: plan, Config: config}
	_, _, input.AgeMonths = s.childInfo(ctx, familyKey, config.ChildKey)
	if slot == SlotEvening {
		tomorrow, err := s.DayPlan(ctx, familyKey, day.AddDate(0, 0, 1).Format(dateLayout))
		if err == nil {
			input.Tomorrow = &tomorrow
		}
	}
	return Advise(input), snapshot, nil
}

// slotTime 这一天 slot 该推的时间；接之前那次看当天接不接。
func slotTime(config Config, day time.Time, plan DayPlan, slot string) time.Time {
	switch slot {
	case SlotMorning:
		return at(day, config.Slots.Morning)
	case SlotNoon:
		return at(day, config.Slots.Noon)
	case SlotEvening:
		return at(day, config.Slots.Evening)
	case SlotPrePickup:
		if !plan.School || config.Slots.PrePickup <= 0 {
			return time.Time{}
		}
		if pickup := at(day, plan.Pickup); !pickup.IsZero() {
			return pickup.Add(-time.Duration(config.Slots.PrePickup) * time.Minute)
		}
	}
	return time.Time{}
}

// Preview 生成但不存不推（管理端「试一下」）。
func (s *Service) Preview(ctx context.Context, familyKey, slot, date string) (Brief, error) {
	advice, _, err := s.Advise(ctx, familyKey, slot, date)
	if err != nil {
		return Brief{}, err
	}
	content, source := s.write(ctx, advice, "")
	return Brief{Date: advice.Date, Slot: slot, SlotLabel: SlotLabel(slot), Headline: advice.Headline, Content: content,
		Source: source, PushedTo: []string{}, Advice: &advice, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

// Send 手动生成并推送（覆盖这一天这个时段已有的）。
func (s *Service) Send(ctx context.Context, familyKey, slot, date, by string) (Brief, error) {
	switch slot {
	case SlotMorning, SlotNoon, SlotPrePickup, SlotEvening:
	default:
		return Brief{}, errors.New("时段应为 morning / noon / prepickup / evening")
	}
	day, err := parseDate(date)
	if err != nil {
		return Brief{}, err
	}
	return s.generate(ctx, familyKey, slot, day, by, true)
}

var errAlreadyDone = errors.New("这个时段已经推过了")

// generate 生成一条提醒并推送。force=false（定时任务）时这一天这个时段有了就不再生成。
func (s *Service) generate(ctx context.Context, familyKey, slot string, day time.Time, by string, force bool) (result Brief, err error) {
	config := s.Config(ctx)
	now := time.Now()
	row := briefRow{BriefKey: keys.New("wbrf"), FamilyKey: familyKey, ChildKey: config.ChildKey, PlaceCode: config.Location.Code,
		BriefDate: day.Format(dateLayout), Slot: slot, Source: "llm", CreatedBy: by, CreatedAt: now, UpdatedAt: now}
	// 先占位：两个进程都跑定时任务时只有一个能插进去
	claim := s.db.WithContext(ctx).Clauses(clause.Insert{Modifier: "IGNORE"}).Create(&row)
	if claim.Error != nil {
		return Brief{}, claim.Error
	}
	if claim.RowsAffected > 0 {
		defer func() {
			if err != nil {
				s.db.WithContext(context.WithoutCancel(ctx)).Where("id = ? AND content IS NULL", row.ID).Delete(&briefRow{})
			}
		}()
	} else {
		if !force {
			return Brief{}, errAlreadyDone
		}
		if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND brief_date = ? AND slot = ?", familyKey, config.ChildKey,
			day.Format(dateLayout), slot).First(&row).Error; err != nil {
			return Brief{}, err
		}
	}

	advice, snapshot, adviseErr := s.Advise(ctx, familyKey, strings.TrimRight(slot, "0123456789"), day.Format(dateLayout))
	var content, source string
	if adviseErr != nil {
		if !errors.Is(adviseErr, ErrUnavailable) {
			return Brief{}, adviseErr
		}
		// 天气取不到：照样提醒一声，不瞎编
		log.Printf("宝宝穿衣（%s %s）: %v", day.Format(dateLayout), slot, adviseErr)
		advice = Advice{Slot: slot, Date: day.Format(dateLayout), Place: config.Location.Name, Headline: "天气暂时查不到", Parts: []Part{}, Notes: []string{}, Warnings: []Warning{}}
		content, source = "天气暂时查不到，出门前看一眼窗外，给宝宝备一件外套。", "template"
	} else {
		if slot == SlotPrePickup && !force {
			if same, err := s.sameAsNoon(ctx, familyKey, config.ChildKey, day, advice); err == nil && same {
				content, source = "和中午说的一样，没有推送。", "skipped"
			}
		}
		if content == "" {
			content, source = s.write(ctx, advice, row.BriefKey)
		}
	}

	adviceJSON, _ := json.Marshal(advice)
	snapshotJSON, _ := json.Marshal(trimSnapshot(snapshot, day))
	var pushed []string
	if source != "skipped" {
		pushed = s.push(ctx, familyKey, advice, content)
	}
	pushedJSON, _ := json.Marshal(pushed)
	updates := map[string]any{"content": content, "source": source, "advice": string(adviceJSON), "snapshot": string(snapshotJSON),
		"pushed_to": string(pushedJSON), "place_code": config.Location.Code, "created_by": by, "updated_at": time.Now()}
	if err := s.db.WithContext(ctx).Model(&briefRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return Brief{}, err
	}
	row.Content, row.Source, row.Advice, row.PushedTo, row.CreatedBy, row.UpdatedAt =
		strPtr(content), source, strPtr(string(adviceJSON)), strPtr(string(pushedJSON)), by, time.Now()
	return row.toDTO(), nil
}

// sameAsNoon 接之前再看一眼：和中午那次比，到家时间没提前 30 分钟以上、没新的雨和预警、体感没低 3 度以上，就算没变化。
func (s *Service) sameAsNoon(ctx context.Context, familyKey, childKey string, day time.Time, advice Advice) (bool, error) {
	var row briefRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND brief_date = ? AND slot = ?", familyKey, childKey,
		day.Format(dateLayout), SlotNoon).First(&row).Error; err != nil || row.Advice == nil {
		return false, err
	}
	var noon Advice
	if err := json.Unmarshal([]byte(*row.Advice), &noon); err != nil {
		return false, err
	}
	if len(advice.Warnings) > len(noon.Warnings) {
		return false, nil
	}
	pickup := func(a Advice) *Part {
		for i := range a.Parts {
			if a.Parts[i].Kind == "pickup" {
				return &a.Parts[i]
			}
		}
		return nil
	}
	before, after := pickup(noon), pickup(advice)
	if before == nil || after == nil {
		return before == nil && after == nil, nil
	}
	if after.RainFrom != nil && before.RainFrom == nil {
		return false, nil
	}
	if after.DirectHome && !before.DirectHome {
		return false, nil
	}
	if before.BackBy != nil && after.BackBy != nil && before.BackBy.Sub(*after.BackBy) >= 30*time.Minute {
		return false, nil
	}
	if before.FeelsMin-after.FeelsMin >= 3 {
		return false, nil
	}
	return true, nil
}

// trimSnapshot 只存当天和第二天早上的逐小时，够回头看当时依据就行。
func trimSnapshot(snapshot Snapshot, day time.Time) Snapshot {
	trimmed := snapshot
	trimmed.Hours = snapshot.Between(day, day.Add(36*time.Hour))
	if len(trimmed.Days) > 3 {
		trimmed.Days = trimmed.Days[:3]
	}
	return trimmed
}

// push 给每个收件人各建一条只提醒自己的提醒。奶奶点开到长辈模式的页面。
func (s *Service) push(ctx context.Context, familyKey string, advice Advice, content string) []string {
	pushed := []string{}
	if s.reminders == nil || s.accounts == nil {
		return pushed
	}
	config := s.Config(ctx)
	title := "👕 " + advice.Headline
	if runes := []rune(title); len(runes) > 40 {
		title = string(runes[:40]) + "…"
	}
	body := content
	if runes := []rune(body); len(runes) > 180 {
		body = string(runes[:180]) + "…"
	}
	for _, recipient := range s.accounts(ctx, config.Recipients) {
		target := "/weather"
		if recipient.Role == identity.RoleElder {
			target = "/elder/weather"
		}
		userCtx := viewer.With(ctx, viewer.Viewer{Account: recipient.Account})
		if _, err := s.reminders.CreateManual(userCtx, familyKey, reminder.ManualInput{
			Kind: "weather", Title: title, Body: body, RemindAt: time.Now(), TargetURL: target, Visibility: viewer.Private,
		}); err != nil {
			log.Printf("推送宝宝穿衣给 %s: %v", recipient.Account, err)
			continue
		}
		pushed = append(pushed, recipient.Account)
	}
	return pushed
}

// ---------------------------------------------------------------- 写成家常话

const briefSystem = `你帮家里写「宝宝穿衣」提醒，推到手机上。读的人是带孩子的爸爸和奶奶，奶奶那边会读出来给她听。
给你的是已经算好的结论（几度、穿什么、几点前到家），你只负责把它说成两到四句顺口的家常话。

要求：
- 第一句说最要紧的：有预警先说预警；要接到直接回、要早点到家的，第一句就说几点前到家和原因。
- 然后说穿什么、要带什么；有注意事项挑最要紧的一两条。
- 只用给你的数字和钟点，不要自己换算、估计或编新的温度和时间；钟点照抄（16:10 这种写法），温度写成「12度」。
- 口语、简短，像家里人提醒一声；不用 Markdown、不用表情、不分点、不客套，不说「根据天气数据」。
- 不评价带孩子的人，不说「奶奶要记得……」，只说宝宝该怎么穿、几点回。`

// write 让模型写；模型不可用、写出来的数字和钟点对不上，就用模板。
func (s *Service) write(ctx context.Context, advice Advice, refKey string) (string, string) {
	fallback := Template(advice)
	if s.llm == nil {
		return fallback, "template"
	}
	writeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, err := s.llm.Chat(writeCtx, llm.Request{
		Task:     taskBrief,
		System:   []llm.SystemPart{{Text: briefSystem, Cache: true}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: facts(advice)}}}},
		RefKey:   refKey,
	}, nil)
	if err != nil {
		log.Printf("宝宝穿衣写提醒（%s）: %v", advice.Slot, err)
		return fallback, "template"
	}
	text := strings.TrimSpace(response.Message.Text())
	if text == "" || len([]rune(text)) > 300 {
		return fallback, "template"
	}
	if problem := checkNumbers(text, advice); problem != "" {
		log.Printf("宝宝穿衣写提醒（%s）：模型的话里%s，改用模板", advice.Slot, problem)
		return fallback, "template"
	}
	return text, "llm"
}

// facts 给模型看的结论。
func facts(advice Advice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "时段：%s（%s）\n地点：%s\n", SlotLabel(advice.Slot), advice.Date, advice.Place)
	fmt.Fprintf(&b, "现在：%s，体感 %s，%s\n", deg(advice.Now.Temp), deg(advice.Now.Feels), advice.Now.Text)
	if advice.HasRange {
		fmt.Fprintf(&b, "今天：%s–%s", deg(advice.Low), deg(advice.High))
		if advice.Sunset != "" {
			fmt.Fprintf(&b, "，%s 天黑", advice.Sunset)
		}
		b.WriteString("\n")
	}
	if advice.Stale {
		fmt.Fprintf(&b, "（天气是 %s 取到的旧数据，提一句）\n", clock(advice.FetchedAt))
	}
	for _, warning := range advice.Warnings {
		fmt.Fprintf(&b, "预警：%s\n", warning.Title())
	}
	if advice.Infant {
		b.WriteString("孩子不到 1 岁，已经多算了一层\n")
	}
	b.WriteString("\n结论：\n")
	b.WriteString(Template(advice))
	b.WriteString("\n\n一句话结论：" + advice.Headline)
	return b.String()
}

// Template 按规则结果拼的提醒，模型不可用时直接推这个。
func Template(advice Advice) string {
	var lines []string
	if advice.Stale {
		lines = append(lines, fmt.Sprintf("（用的是 %s 的天气）", clock(advice.FetchedAt)))
	}
	for _, warning := range advice.Warnings {
		lines = append(lines, fmt.Sprintf("有%s，少出门。", warning.Title()))
	}
	if advice.Night != nil {
		line := fmt.Sprintf("今晚最低 %s，%s。", deg(advice.Night.Low), advice.Night.Bedding)
		if len(advice.Night.Tips) > 0 {
			line += strings.Join(advice.Night.Tips, "；") + "。"
		}
		lines = append(lines, line)
	}
	for _, part := range advice.Parts {
		if part.Skip {
			lines = append(lines, fmt.Sprintf("%s：%s。", part.Label, part.SkipReason))
			continue
		}
		line := fmt.Sprintf("%s（%s 体感 %s）：穿%s", part.Label, clock(part.Start), deg(part.FeelsMin), strings.Join(part.Clothes, "、"))
		if len(part.Extras) > 0 {
			line += "，带" + strings.Join(part.Extras, "、")
		}
		line += "。"
		if part.BackBy != nil && part.Kind != "drop" {
			switch {
			case part.DirectHome:
				line += fmt.Sprintf("接到直接回，%s 前到家", clock(*part.BackBy))
			default:
				line += fmt.Sprintf("%s 前到家", clock(*part.BackBy))
			}
			if part.BackReason != "" {
				line += "（" + part.BackReason + "）"
			}
			line += "。"
			if part.PlayUntil != nil {
				line += fmt.Sprintf("天气好，想多玩可以到 %s。", clock(*part.PlayUntil))
			}
		}
		if len(part.Notes) > 0 {
			line += strings.Join(part.Notes, "；") + "。"
		}
		lines = append(lines, line)
	}
	if len(advice.Notes) > 0 {
		lines = append(lines, strings.Join(advice.Notes, "；")+"。")
	}
	if len(lines) == 0 {
		lines = append(lines, fmt.Sprintf("现在 %s，体感 %s，%s。", deg(advice.Now.Temp), deg(advice.Now.Feels), advice.Now.Text))
	}
	return strings.Join(lines, "\n")
}

var (
	clockPattern = regexp.MustCompile(`(\d{1,2})\s*[:：点]\s*(半|\d{1,2})?`)
	tempPattern  = regexp.MustCompile(`(零下|-)?\s*(\d{1,2})(?:\.\d)?\s*(?:°|度|℃)`)
)

// checkNumbers 模型话里的钟点、温度都要在结论里出现过（温度允许差 1 度）。对不上返回问题描述。
func checkNumbers(text string, advice Advice) string {
	clocks := map[int]bool{}
	addClock := func(value time.Time) {
		if !value.IsZero() {
			local := value.In(shanghai)
			clocks[(local.Hour()%12)*60+local.Minute()] = true
		}
	}
	temps := []float64{advice.Now.Temp, advice.Now.Feels}
	if advice.HasRange {
		temps = append(temps, advice.High, advice.Low)
		if sunset, err := time.Parse("15:04", advice.Sunset); err == nil {
			addClock(sunset)
		}
	}
	if advice.Night != nil {
		temps = append(temps, advice.Night.Low)
	}
	if advice.Stale {
		addClock(advice.FetchedAt)
	}
	for _, part := range advice.Parts {
		temps = append(temps, part.FeelsMin, part.FeelsMax, part.TempMin, part.TempMax)
		for _, value := range []*time.Time{&part.Start, &part.End, part.Leave, part.BackBy, part.PlayUntil, part.RainFrom} {
			if value != nil {
				addClock(*value)
			}
		}
		for _, value := range numbersIn(part.BackReason + part.SkipReason + strings.Join(part.Notes, "")) {
			temps = append(temps, value)
		}
	}
	for _, note := range advice.Notes {
		temps = append(temps, numbersIn(note)...)
	}
	for _, match := range clockPattern.FindAllStringSubmatch(text, -1) {
		hour, _ := strconv.Atoi(match[1])
		minute := 0
		if match[2] == "半" {
			minute = 30
		} else if match[2] != "" {
			minute, _ = strconv.Atoi(match[2])
		}
		if hour > 24 || minute > 59 || !clocks[(hour%12)*60+minute] {
			return "出现了结论里没有的时间 " + match[0]
		}
	}
	for _, match := range tempPattern.FindAllStringSubmatch(text, -1) {
		value, _ := strconv.ParseFloat(match[2], 64)
		if match[1] != "" {
			value = -value
		}
		ok := false
		for _, allowed := range temps {
			if math.Abs(math.Round(allowed)-value) <= 1 {
				ok = true
				break
			}
		}
		if !ok {
			return "出现了结论里没有的温度 " + match[0]
		}
	}
	return ""
}

func numbersIn(text string) []float64 {
	var values []float64
	for _, match := range tempPattern.FindAllStringSubmatch(text, -1) {
		value, _ := strconv.ParseFloat(match[2], 64)
		if match[1] != "" {
			value = -value
		}
		values = append(values, value)
	}
	return values
}

// ---------------------------------------------------------------- 查看

// Briefs 列出 from 到 to（含）的提醒，新的在前。
func (s *Service) Briefs(ctx context.Context, familyKey, from, to string) ([]Brief, error) {
	start, err := parseDate(from)
	if err != nil {
		return nil, err
	}
	end, err := parseDate(to)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		start, end = end, start
	}
	if end.Sub(start) > 62*24*time.Hour {
		start = end.AddDate(0, 0, -62)
	}
	var rows []briefRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND brief_date >= ? AND brief_date <= ? AND content IS NOT NULL", familyKey,
		start.Format(dateLayout), end.Format(dateLayout)).Order("brief_date DESC, created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Brief, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

// Today 是页面要的全部：天气、今天和明天的安排、现在的建议、今天的提醒。
type Today struct {
	Location Location `json:"location"`
	Snapshot Snapshot `json:"snapshot"`
	Plan     DayPlan  `json:"plan"`
	Tomorrow DayPlan  `json:"tomorrow"`
	Advice   *Advice  `json:"advice"`
	Evening  *Advice  `json:"evening"` // 今晚 + 明早
	Briefs   []Brief  `json:"briefs"`
	Slots    Slots    `json:"slots"`
	Child    string   `json:"child"`
	Error    string   `json:"error,omitempty"`
}

func (s *Service) Today(ctx context.Context, familyKey string) (Today, error) {
	config := s.Config(ctx)
	day := today()
	result := Today{Location: config.Location, Slots: config.Slots, Briefs: []Brief{}}
	_, result.Child, _ = s.childInfo(ctx, familyKey, config.ChildKey)
	var err error
	if result.Plan, err = s.DayPlan(ctx, familyKey, day.Format(dateLayout)); err != nil {
		return Today{}, err
	}
	if result.Tomorrow, err = s.DayPlan(ctx, familyKey, day.AddDate(0, 0, 1).Format(dateLayout)); err != nil {
		return Today{}, err
	}
	if result.Briefs, err = s.Briefs(ctx, familyKey, day.Format(dateLayout), day.Format(dateLayout)); err != nil {
		return Today{}, err
	}
	advice, snapshot, err := s.Advise(ctx, familyKey, SlotNow, "")
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Snapshot, result.Advice = trimSnapshot(snapshot, day), &advice
	if evening, _, err := s.Advise(ctx, familyKey, SlotEvening, ""); err == nil {
		result.Evening = &evening
	}
	return result, nil
}

// ---------------------------------------------------------------- 定时

// RunScheduler 每分钟看一次：到了推送时间（一小时内）还没推的就推；每 15 分钟看一次有没有新的预警。
func (s *Service) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()
	failed := map[string]time.Time{}
	var mu sync.Mutex
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		tick++
		config := s.Config(ctx)
		if !config.Enabled {
			continue
		}
		var families []string
		if err := s.db.WithContext(ctx).Table("pa_family").Pluck("family_key", &families).Error; err != nil {
			continue
		}
		now := time.Now().In(shanghai)
		day := today()
		for _, familyKey := range families {
			done, err := s.doneSlots(ctx, familyKey, config.ChildKey, day)
			if err != nil {
				continue
			}
			plan, err := s.DayPlan(ctx, familyKey, day.Format(dateLayout))
			if err != nil {
				continue
			}
			for _, slot := range []string{SlotMorning, SlotNoon, SlotPrePickup, SlotEvening} {
				when := slotTime(config, day, plan, slot)
				if when.IsZero() || now.Before(when) || now.Sub(when) > slotWindow || done[slot] {
					continue
				}
				key := familyKey + "|" + day.Format(dateLayout) + "|" + slot
				mu.Lock()
				last := failed[key]
				mu.Unlock()
				if time.Since(last) < retryAfter {
					continue
				}
				jobCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
				if _, err := s.generate(jobCtx, familyKey, slot, day, "", false); err != nil && !errors.Is(err, errAlreadyDone) {
					log.Printf("宝宝穿衣（%s %s）: %v", day.Format(dateLayout), slot, err)
					mu.Lock()
					failed[key] = time.Now()
					mu.Unlock()
				}
				cancel()
			}
			if config.Alerts && tick%15 == 0 && !quiet(config, now) {
				s.checkAlerts(ctx, familyKey, config, day, done)
			}
		}
	}
}

func quiet(config Config, now time.Time) bool {
	clock := now.Format("15:04")
	if config.QuietFrom <= config.QuietTo {
		return clock >= config.QuietFrom && clock < config.QuietTo
	}
	return clock >= config.QuietFrom || clock < config.QuietTo
}

// doneSlots 今天已经有（或正在生成）的时段。
func (s *Service) doneSlots(ctx context.Context, familyKey, childKey string, day time.Time) (map[string]bool, error) {
	var slots []string
	if err := s.db.WithContext(ctx).Model(&briefRow{}).Where("family_key = ? AND child_key = ? AND brief_date = ?", familyKey, childKey,
		day.Format(dateLayout)).Pluck("slot", &slots).Error; err != nil {
		return nil, err
	}
	done := map[string]bool{}
	for _, slot := range slots {
		done[slot] = true
	}
	return done, nil
}

// checkAlerts 有今天还没提过的黄色以上预警，立刻推一条临时提醒（一天最多 AlertMax 条）。
func (s *Service) checkAlerts(ctx context.Context, familyKey string, config Config, day time.Time, done map[string]bool) {
	snapshot, err := s.SnapshotAt(ctx, config.Location, false)
	if err != nil || snapshot.Stale {
		return
	}
	var serious []Warning
	for _, warning := range snapshot.Warnings {
		if warning.Serious() {
			serious = append(serious, warning)
		}
	}
	if len(serious) == 0 {
		return
	}
	var rows []briefRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND brief_date = ? AND advice IS NOT NULL", familyKey, config.ChildKey,
		day.Format(dateLayout)).Find(&rows).Error; err != nil {
		return
	}
	told := map[string]bool{}
	for _, row := range rows {
		var advice Advice
		if json.Unmarshal([]byte(*row.Advice), &advice) == nil {
			for _, warning := range advice.Warnings {
				told[warning.Title()] = true
			}
		}
	}
	fresh := false
	for _, warning := range serious {
		if !told[warning.Title()] {
			fresh = true
		}
	}
	if !fresh {
		return
	}
	for i := 1; i <= config.AlertMax; i++ {
		slot := fmt.Sprintf("%s%d", SlotAlert, i)
		if done[slot] {
			continue
		}
		jobCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		if _, err := s.generate(jobCtx, familyKey, slot, day, "", false); err != nil && !errors.Is(err, errAlreadyDone) {
			log.Printf("宝宝穿衣临时提醒: %v", err)
		}
		cancel()
		return
	}
}

// ---------------------------------------------------------------- 对话工具

// Describe 给对话工具 weather_now：现在的天气、接下来几小时、今天和明天、预警，以及按今天安排算的穿衣结论。
func (s *Service) Describe(ctx context.Context, familyKey string, hours int) (string, error) {
	if hours <= 0 || hours > 48 {
		hours = 12
	}
	advice, snapshot, err := s.Advise(ctx, familyKey, SlotNow, "")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "地点：%s（数据：%s，%s 取的）\n", snapshot.Location.Name, snapshot.Source, clock(snapshot.FetchedAt))
	now := snapshot.Now
	fmt.Fprintf(&b, "现在：%s，体感 %s，%s，%s，湿度 %.0f%%", deg(now.Temp), deg(now.Feels), now.Text, now.Wind, now.Humidity)
	if now.AQI > 0 {
		fmt.Fprintf(&b, "，AQI %d", now.AQI)
	}
	b.WriteString("\n")
	for _, warning := range snapshot.Warnings {
		fmt.Fprintf(&b, "预警：%s %s\n", warning.Title(), warning.Text)
	}
	for i, day := range snapshot.Days {
		if i >= 3 {
			break
		}
		fmt.Fprintf(&b, "%s：%s–%s，白天%s，夜里%s，日落 %s\n", day.Date, deg(day.Low), deg(day.High), day.DayText, day.NightText, day.Sunset)
	}
	b.WriteString("接下来逐小时：\n")
	for _, hour := range snapshot.Between(time.Now(), time.Now().Add(time.Duration(hours)*time.Hour)) {
		fmt.Fprintf(&b, "  %s %s 体感 %s %s", hour.At.Format("01-02 15:04"), deg(hour.Temp), deg(hour.Feels), hour.Text)
		if hour.Rain > 0 {
			fmt.Fprintf(&b, " 降水 %.1fmm", hour.Rain)
		}
		b.WriteString("\n")
	}
	if snapshot.Clothing != "" {
		fmt.Fprintf(&b, "穿衣指数（参考）：%s\n", snapshot.Clothing)
	}
	b.WriteString("\n按今天的送接安排算的宝宝穿衣（数字和钟点以这里为准）：\n")
	b.WriteString(Template(advice))
	return b.String(), nil
}

// SetOutingTool 给对话工具 set_outing 用：改某一天的安排，返回改后的安排说明。
func (s *Service) SetOutingTool(ctx context.Context, familyKey string, input DayInput, reset bool, account string) (string, error) {
	var plan DayPlan
	var err error
	if reset {
		plan, err = s.ResetDay(ctx, familyKey, input.Date)
	} else {
		plan, err = s.SaveDay(ctx, familyKey, input, account)
	}
	if err != nil {
		return "", err
	}
	return "已改好。" + DescribePlan(plan), nil
}

// DescribePlan 「10-21：去幼儿园，08:00 送，16:10 接，17:00 前到家」
func DescribePlan(plan DayPlan) string {
	var parts []string
	if plan.School {
		parts = append(parts, fmt.Sprintf("去幼儿园，%s 送，%s 接，%s 前到家", plan.Drop, plan.Pickup, plan.Home))
	} else {
		parts = append(parts, "不去幼儿园")
	}
	for _, outing := range plan.Outings {
		text := fmt.Sprintf("%s–%s 出去", outing.From, outing.To)
		if outing.Note != "" {
			text += "（" + outing.Note + "）"
		}
		parts = append(parts, text)
	}
	if plan.Note != "" {
		parts = append(parts, plan.Note)
	}
	result := plan.Date + "：" + strings.Join(parts, "，")
	if plan.Override {
		result += "（这天临时改过）"
	}
	return result
}
