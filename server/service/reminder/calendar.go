package reminder

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

var shanghai = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}()

func atClock(day time.Time, hour int) time.Time {
	day = day.In(shanghai)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, shanghai)
}

func parseMonthDay(value string) (month, day int, leap bool, err error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToUpper(value), "L") {
		leap, value = true, value[1:]
	}
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf("日期 %q 应为 MM-DD 或 LMM-DD", value)
	}
	month, err = strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	day, err = strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 || day < 1 || day > 30 {
		return 0, 0, false, fmt.Errorf("无效的月日 %q", value)
	}
	return
}

func nextSolarAnnual(month, day int, from time.Time) time.Time {
	from = from.In(shanghai)
	for year := from.Year(); year <= from.Year()+1; year++ {
		candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, shanghai)
		if candidate.Month() == time.Month(month) && !candidate.Before(atClock(from, 0)) {
			return candidate
		}
	}
	return time.Time{}
}

func nextLunarAnnual(value string, from time.Time) (time.Time, error) {
	month, day, leap, err := parseMonthDay(value)
	if err != nil {
		return time.Time{}, err
	}
	if leap {
		month = -month
	}
	lunarYear := calendar.NewLunarFromDate(from.In(shanghai)).GetYear()
	for year := lunarYear; year <= lunarYear+2; year++ {
		lunar := calendar.NewLunarFromYmd(year, month, day)
		solar := lunar.GetSolar()
		candidate := time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, shanghai)
		if !candidate.Before(atClock(from, 0)) {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法换算农历日期 %s", value)
}

func lunarToday(now time.Time) string {
	lunar := calendar.NewLunarFromDate(now.In(shanghai))
	return fmt.Sprintf("农历%s月%s", lunar.GetMonthInChinese(), lunar.GetDayInChinese())
}

func LunarToday(now time.Time) string { return lunarToday(now) }
