package diet

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Summary 把一段日子的计划和实际按天、按餐写成文字，给对话里的助理（diet_overview 工具）和每周对照用。
func (s *Service) Summary(ctx context.Context, familyKey, childKey, from string, days int) (string, error) {
	overview, err := s.Overview(ctx, familyKey, childKey, from, days, Actor{})
	if err != nil {
		return "", err
	}
	return formatOverview(overview), nil
}

func formatOverview(overview Overview) string {
	start, _ := time.ParseInLocation(dateLayout, overview.From, time.Local)
	weekdays := []string{"日", "一", "二", "三", "四", "五", "六"}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s的饮食，%s 起 %d 天（计划 = 爸爸妈妈排的，实际 = 家里人记的；没记不代表没吃）\n", overview.ChildLabel, overview.From, overview.Days)
	for offset := 0; offset < overview.Days; offset++ {
		day := start.AddDate(0, 0, offset)
		key := day.Format(dateLayout)
		var plans []Plan
		for _, plan := range overview.Plans {
			if plan.Date == key {
				plans = append(plans, plan)
			}
		}
		var records []Record
		for _, record := range overview.Records {
			if record.EatenAt.In(time.Local).Format(dateLayout) == key {
				records = append(records, record)
			}
		}
		fmt.Fprintf(&builder, "\n%s（周%s）", key, weekdays[day.Weekday()])
		if len(plans) == 0 && len(records) == 0 {
			builder.WriteString("：没有计划，也没有记录\n")
			continue
		}
		builder.WriteString("\n")
		for _, plan := range plans {
			if plan.Meal == MealDay {
				fmt.Fprintf(&builder, "- 全天要求：%s\n", joinNonEmpty("；", plan.Foods, plan.Note))
			}
		}
		for _, meal := range Meals {
			var plan *Plan
			for index := range plans {
				if plans[index].Meal == meal.Key {
					plan = &plans[index]
				}
			}
			var eaten []string
			for _, record := range records {
				if record.Meal != meal.Key {
					continue
				}
				line := record.EatenAt.In(shanghai).Format("15:04") + " " + record.Foods
				if label, ok := Amounts[record.Amount]; ok {
					line += "，" + label
				}
				if record.Note != "" {
					line += "（" + record.Note + "）"
				}
				eaten = append(eaten, line)
			}
			if plan == nil && len(eaten) == 0 {
				continue
			}
			fmt.Fprintf(&builder, "- %s　计划：", meal.Label)
			if plan != nil {
				builder.WriteString(plan.Foods)
				if plan.Note != "" {
					fmt.Fprintf(&builder, "（要求：%s）", plan.Note)
				}
			} else {
				builder.WriteString("无")
			}
			builder.WriteString("；实际：")
			if len(eaten) == 0 {
				builder.WriteString("没记")
			} else {
				builder.WriteString(strings.Join(eaten, "；"))
			}
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func joinNonEmpty(separator string, values ...string) string {
	var parts []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

// adviceHistoryDays 出主意时看最近多少天实际吃了什么。
const adviceHistoryDays = 14

// AdviceContext 给「某天该吃什么」准备材料（diet_overview 的 for_date）：
// 孩子年龄、那天（以及今天到那天之间）已经排的计划、最近两周的计划和实际、最近一次每周对照。
// 建议由对话里的助理自己写，这里只给事实。
func (s *Service) AdviceContext(ctx context.Context, familyKey, childKey, date string) (string, error) {
	person, err := s.childPerson(ctx, familyKey, childKey)
	if err != nil {
		return "", err
	}
	today, _ := parseDate("")
	target, err := parseDate(date)
	if err != nil {
		return "", err
	}
	if target.Before(today) {
		target = today
	}
	if target.After(today.AddDate(0, 0, maxDays)) {
		return "", fmt.Errorf("只能给 %d 天以内的日子出主意", maxDays)
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "孩子：%s", person.RoleLabel)
	if age := ageOf(person.Birthday, today); age != "" {
		fmt.Fprintf(&builder, "，%s", age)
	}
	fmt.Fprintf(&builder, "\n今天：%s；要出主意的是：%s\n", today.Format(dateLayout), target.Format(dateLayout))

	// 要出主意的那天（和今天之间的日子）排了什么；那天就是今天的话，下面的最近两周里已经有了
	if target.After(today) {
		ahead := int(target.Sub(today).Hours()/24 + 0.5)
		upcoming, err := s.Overview(ctx, familyKey, person.PersonKey, today.AddDate(0, 0, 1).Format(dateLayout), ahead, Actor{})
		if err != nil {
			return "", err
		}
		builder.WriteString("\n【接下来已经排的计划】\n")
		builder.WriteString(formatOverview(upcoming))
	}

	history, err := s.Overview(ctx, familyKey, person.PersonKey, today.AddDate(0, 0, 1-adviceHistoryDays).Format(dateLayout), adviceHistoryDays, Actor{})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&builder, "\n【最近 %d 天，含今天】\n", adviceHistoryDays)
	builder.WriteString(formatOverview(history))

	// 最近一次每周对照里有「下周建议」，这周的还没写就看上周的
	for _, day := range []time.Time{today, today.AddDate(0, 0, -7)} {
		review, err := s.GetReview(ctx, familyKey, person.PersonKey, day.Format(dateLayout))
		if err != nil || review == nil {
			continue
		}
		fmt.Fprintf(&builder, "\n【%s 那周的计划对照（含建议）】\n%s\n", review.WeekStart, review.Content)
		break
	}
	return builder.String(), nil
}
