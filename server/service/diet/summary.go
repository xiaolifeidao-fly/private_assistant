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
