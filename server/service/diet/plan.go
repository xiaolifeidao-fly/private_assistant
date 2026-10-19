package diet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"
)

// 说一句 / 拍张照 → 改饮食计划（我和老婆用）。
// 「下周每天早上一个鸡蛋」「周三晚饭吃鱼」「幼儿园这周的食谱（拍照）」「按她最近吃的排一下下周」：
// 把话、照片、这段日子已有的计划、最近两周实际吃了什么一起交给模型，模型给出要改的餐，直接写进计划；
// 不满意再说一句改，或者在页面上手动改。
const taskPlan = "diet.plan"

const planHistoryDays = 14

type PlanCaptureInput struct {
	ChildKey string
	Text     string
	Images   []Image
	// From / Days 当前在看的日子，「这周」「明天」按它和今天理解；默认今天起 7 天
	From string
	Days int
}

type PlanCaptureResult struct {
	// Changed 改过的餐（清掉的也在里面，foods 和 note 为空）
	Changed []Plan `json:"changed"`
	Reply   string `json:"reply"`
}

type planChanges struct {
	Changes []struct {
		Date  string `json:"date"`
		Meal  string `json:"meal"`
		Foods string `json:"foods"`
		Note  string `json:"note"`
		Clear bool   `json:"clear"`
	} `json:"changes"`
	Summary string `json:"summary"`
}

func planSchema() map[string]any {
	meals := []string{MealDay}
	for _, meal := range Meals {
		meals = append(meals, meal.Key)
	}
	str := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	item := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"date":  str("哪天 YYYY-MM-DD"),
			"meal":  map[string]any{"type": "string", "enum": meals, "description": "哪一餐；day 表示这一天的总体要求"},
			"foods": str("这一餐打算吃什么（改完后的完整内容，不是增量）；clear 时填空串"),
			"note":  str("要求、注意（少盐、不给糖、要吃完）；没有填空串"),
			"clear": map[string]any{"type": "boolean", "description": "true 表示取消这一餐的计划"},
		},
		"required": []string{"date", "meal", "foods", "note", "clear"},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"changes": map[string]any{"type": "array", "items": item},
			"summary": str("一两句话说改了什么；如果有需要家长注意的（营养明显不均衡、和要求冲突），简短提一句"),
		},
		"required": []string{"changes", "summary"},
	}
}

const planSystem = `你帮爸爸妈妈安排孩子的饮食计划。奶奶照着计划做饭、喂饭，所以计划要写得具体、好照着做。
家长会说一句话（可能是语音转的文字），或者拍一张照片（幼儿园食谱、医生的饮食建议、写在纸上的菜单），告诉你要怎么安排或者怎么改。

规则：
- 只输出要改的餐。没提到的日子、餐次不要动。
- foods 写改完以后这一餐的完整内容（原来有、家长没说去掉的要保留）。
- 「每天」「这周」「下周」「工作日」这类说法展开成具体日期，按"今天"和"正在看的日子"理解。
- 全天的要求（一天一杯奶、不吃糖、多喝水）放在 meal=day。
- 家长让你帮忙排菜时，参考孩子年龄、最近实际吃的情况（吃得少的、挑的、重复太多的），荤素搭配，做法家常，别排太复杂的菜。
- 照片是幼儿园食谱时，幼儿园管的那几餐（通常是午饭、下午加餐）照着写，note 写「幼儿园」，家里的餐另外按家长的话排。
- 家长说的不是安排饮食（问问题、说别的事），返回空的 changes，summary 里简短回答或说明。
- 不要编造家长没说、照片里也没有的要求。`

// CapturePlan 用一句话 / 一张照片改计划。只有能改计划的账号能用。
func (s *Service) CapturePlan(ctx context.Context, familyKey string, input PlanCaptureInput, actor Actor) (PlanCaptureResult, error) {
	if !actor.CanPlan() {
		return PlanCaptureResult{}, ErrForbidden
	}
	person, err := s.childPerson(ctx, familyKey, input.ChildKey)
	if err != nil {
		return PlanCaptureResult{}, err
	}
	text := clip(input.Text)
	if text == "" && len(input.Images) == 0 {
		return PlanCaptureResult{}, errors.New("说一句怎么安排，或者拍张食谱")
	}
	blocks, err := imageBlocks(input.Images)
	if err != nil {
		return PlanCaptureResult{}, err
	}
	today, _ := parseDate("")
	from, err := parseDate(input.From)
	if err != nil {
		return PlanCaptureResult{}, err
	}
	if from.Before(today) {
		from = today
	}
	days := input.Days
	if days <= 0 || days > maxDays {
		days = 7
	}
	// 「下周」可能落在正在看的日子之外，已有计划多给一周
	current, err := s.Overview(ctx, familyKey, person.PersonKey, from.Format(dateLayout), days+7, actor)
	if err != nil {
		return PlanCaptureResult{}, err
	}
	history, err := s.Overview(ctx, familyKey, person.PersonKey, today.AddDate(0, 0, -planHistoryDays).Format(dateLayout), planHistoryDays, actor)
	if err != nil {
		return PlanCaptureResult{}, err
	}

	var parsed planChanges
	header := planHeader(today, from, days, person.RoleLabel, ageOf(person.Birthday, today), text, len(blocks), current, history)
	if err := s.askJSON(ctx, taskPlan, planSystem, planSchema(), blocks, header, keys.New("dplanc"), &parsed); err != nil {
		return PlanCaptureResult{}, fmt.Errorf("没整理出来：%w", err)
	}

	result := PlanCaptureResult{Changed: []Plan{}}
	latest := today.AddDate(0, 0, maxDays*2)
	for _, change := range parsed.Changes {
		date, err := parseDate(change.Date)
		if err != nil || date.Before(today) || date.After(latest) {
			continue // 过去的日子、太远的日子不改
		}
		foods, note := change.Foods, change.Note
		if change.Clear {
			foods, note = "", ""
		}
		plan, err := s.SavePlan(ctx, familyKey, PlanInput{ChildKey: person.PersonKey, Date: date.Format(dateLayout), Meal: change.Meal,
			Foods: foods, Note: note}, actor)
		if err != nil {
			continue
		}
		if plan == nil {
			plan = &Plan{ChildKey: person.PersonKey, Date: date.Format(dateLayout), Meal: change.Meal}
		}
		result.Changed = append(result.Changed, *plan)
	}
	summary := strings.TrimSpace(parsed.Summary)
	switch {
	case len(result.Changed) == 0 && summary == "":
		result.Reply = "没看出要改哪一餐，换个说法再说一次？"
	case len(result.Changed) == 0:
		result.Reply = summary
	default:
		result.Reply = fmt.Sprintf("改好了 %d 餐。%s", len(result.Changed), summary)
	}
	return result, nil
}

func (s *Service) childPerson(ctx context.Context, familyKey, childKey string) (personInfo, error) {
	childKey = strings.TrimSpace(childKey)
	if childKey == "" {
		childKey = DefaultChild
	}
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return personInfo{}, err
	}
	for _, person := range persons {
		if person.PersonKey == childKey {
			return personInfo{PersonKey: person.PersonKey, RoleLabel: person.RoleLabel, Birthday: person.Birthday}, nil
		}
	}
	return personInfo{}, fmt.Errorf("家庭档案里没有 %s，请先在管理端添加", childKey)
}

type personInfo struct {
	PersonKey, RoleLabel, Birthday string
}

// ageOf 返回「3 岁 5 个月」这样的年龄；没有生日返回空串。
func ageOf(birthday string, today time.Time) string {
	born, err := time.Parse(dateLayout, strings.TrimSpace(birthday))
	if err != nil || born.After(today) {
		return ""
	}
	months := (today.Year()-born.Year())*12 + int(today.Month()) - int(born.Month())
	if today.Day() < born.Day() {
		months--
	}
	if months < 12 {
		return fmt.Sprintf("%d 个月", months)
	}
	return fmt.Sprintf("%d 岁 %d 个月", months/12, months%12)
}

func planHeader(today, from time.Time, days int, childLabel, age, text string, photoCount int, current, history Overview) string {
	var builder strings.Builder
	weekdays := []string{"日", "一", "二", "三", "四", "五", "六"}
	day := func(date time.Time) string {
		return fmt.Sprintf("%s（周%s）", date.Format(dateLayout), weekdays[date.Weekday()])
	}
	fmt.Fprintf(&builder, "今天：%s\n正在看的日子：%s 起 %d 天\n孩子：%s", day(today), day(from), days, childLabel)
	if age != "" {
		fmt.Fprintf(&builder, "，%s", age)
	}
	builder.WriteString("\n\n已有的计划：\n")
	if len(current.Plans) == 0 {
		builder.WriteString("（还没有）\n")
	}
	for _, plan := range current.Plans {
		date, _ := time.Parse(dateLayout, plan.Date)
		fmt.Fprintf(&builder, "- %s %s：%s", day(date), MealLabel(plan.Meal), plan.Foods)
		if plan.Note != "" {
			fmt.Fprintf(&builder, "（要求：%s）", plan.Note)
		}
		builder.WriteString("\n")
	}
	fmt.Fprintf(&builder, "\n最近 %d 天实际吃的：\n", planHistoryDays)
	if len(history.Records) == 0 {
		builder.WriteString("（没有记录）\n")
	}
	for _, record := range history.Records {
		fmt.Fprintf(&builder, "- %s %s：%s", record.EatenAt.In(shanghai).Format("01-02"), MealLabel(record.Meal), record.Foods)
		if label, ok := Amounts[record.Amount]; ok {
			fmt.Fprintf(&builder, "，%s", label)
		}
		if record.Note != "" {
			fmt.Fprintf(&builder, "（%s）", record.Note)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n家长这次")
	if photoCount > 0 {
		fmt.Fprintf(&builder, "拍了 %d 张照片（在上面）", photoCount)
	}
	if text != "" {
		if photoCount > 0 {
			builder.WriteString("，")
		}
		fmt.Fprintf(&builder, "说：\n%s\n", text)
	} else {
		builder.WriteString("，没有说话，按照片安排。\n")
	}
	return builder.String()
}
