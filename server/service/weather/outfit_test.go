package weather

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// day 是测试用的那天：2026-11-20（周五，上学）。
var testDay = time.Date(2026, 11, 20, 0, 0, 0, 0, shanghai)

func hourAt(h int) time.Time { return testDay.Add(time.Duration(h) * time.Hour) }

// snapshotWith 造一天的逐小时：feels(h) 给每小时体感（温度就当体感），wet 里的小时下雨。
func snapshotWith(feels func(h int) float64, wet map[int]bool, sunset string) Snapshot {
	var hours []Hour
	for h := 0; h < 36; h++ {
		value := feels(h % 24)
		text := "多云"
		if wet[h] {
			text = "小雨"
		}
		hours = append(hours, Hour{At: hourAt(h), Temp: value, Feels: value, Text: text, Wet: wet[h], WindLevel: 2})
	}
	return Snapshot{
		Location: Location{Name: "上海 青浦"},
		Now:      Now{Temp: feels(7), Feels: feels(7), Text: "多云"},
		Hours:    hours,
		Days: []Day{{Date: "2026-11-20", High: 20, Low: 8, Sunset: sunset},
			{Date: "2026-11-21", High: 18, Low: 7, Sunset: sunset}},
	}
}

func schoolPlan() DayPlan {
	return DayPlan{Date: "2026-11-20", School: true, Drop: "08:00", Pickup: "15:50", Home: "16:30", Outings: []Outing{}}
}

func findPart(advice Advice, kind string) *Part {
	for i := range advice.Parts {
		if advice.Parts[i].Kind == kind {
			return &advice.Parts[i]
		}
	}
	return nil
}

func TestMildAfternoonGoesHomeAsUsualAndCanPlay(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 20 }, nil, "17:30")
	advice := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	pickup := findPart(advice, "pickup")
	if pickup == nil || pickup.BackBy == nil {
		t.Fatalf("没有下午接: %+v", advice.Parts)
	}
	if clock(*pickup.BackBy) != "16:30" || pickup.DirectHome {
		t.Fatalf("天气好应照常 16:30 到家，得到 %s direct=%v", clock(*pickup.BackBy), pickup.DirectHome)
	}
	if pickup.PlayUntil == nil || clock(*pickup.PlayUntil) != "18:00" {
		t.Fatalf("天气好可以玩到天黑后半小时 18:00，得到 %v", pickup.PlayUntil)
	}
	if pickup.Level.Key != "mild" {
		t.Fatalf("20 度应是 mild，得到 %s", pickup.Level.Key)
	}
}

func TestColdAtPickupGoesStraightHome(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 {
		if h >= 15 {
			return 8
		}
		return 13
	}, nil, "17:00")
	advice := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	pickup := findPart(advice, "pickup")
	if pickup == nil || !pickup.DirectHome || clock(*pickup.BackBy) != "16:10" {
		t.Fatalf("接的时候体感 8 度应接到直接回、16:10 到家，得到 %+v", pickup)
	}
	if !strings.Contains(advice.Headline, "接到直接回，16:10 前到家") {
		t.Fatalf("结论没说直接回: %s", advice.Headline)
	}
}

func TestDropsBelowLineDuringPlayEndsEarly(t *testing.T) {
	// 15、16 点 12 度，17 点开始 9 度：照常 16:30 前到家，不该叫人多玩
	snapshot := snapshotWith(func(h int) float64 {
		if h >= 17 {
			return 9
		}
		return 12
	}, nil, "17:20")
	plan := schoolPlan()
	plan.Home = "17:30"
	advice := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: plan, AgeMonths: 42, Config: DefaultConfig()})
	pickup := findPart(advice, "pickup")
	if pickup == nil || pickup.BackBy == nil || clock(*pickup.BackBy) != "17:00" {
		t.Fatalf("17 点体感降到 9 度，应 17:00 前到家，得到 %+v", pickup)
	}
	if !strings.Contains(pickup.BackReason, "17:00") {
		t.Fatalf("原因应提到 17:00: %s", pickup.BackReason)
	}
}

func TestRainAfterPickupGoesStraightHome(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 18 }, map[int]bool{16: true}, "17:20")
	advice := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	pickup := findPart(advice, "pickup")
	if pickup == nil || !pickup.DirectHome || pickup.RainFrom == nil {
		t.Fatalf("16 点有雨应接到直接回，得到 %+v", pickup)
	}
	if !strings.Contains(strings.Join(pickup.Extras, ""), "雨") {
		t.Fatalf("应该带雨具: %v", pickup.Extras)
	}
	if !strings.HasPrefix(advice.Headline, "下午有雨") {
		t.Fatalf("结论: %s", advice.Headline)
	}
}

func TestInfantGetsExtraLayerAndHigherLine(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 11 }, nil, "17:20")
	config := DefaultConfig()
	child := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: config})
	baby := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 8, Config: config})
	if findPart(child, "pickup").DirectHome {
		t.Fatalf("3 岁半 11 度高于回家线 10 度，不必直接回")
	}
	part := findPart(baby, "pickup")
	if !part.DirectHome || part.Layers != findPart(child, "pickup").Layers+1 {
		t.Fatalf("不到 1 岁：回家线 12 度应直接回、多一层，得到 %+v", part)
	}
}

func TestWarningMeansStraightHome(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 20 }, nil, "17:20")
	snapshot.Warnings = []Warning{{Type: "大风", Level: "黄色"}, {Type: "雷电", Level: "蓝色"}}
	advice := Advise(Input{Slot: SlotMorning, Now: hourAt(7), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	if len(advice.Warnings) != 1 || !strings.HasPrefix(advice.Headline, "大风黄色预警") {
		t.Fatalf("只算黄色以上，结论先说预警: %v %s", advice.Warnings, advice.Headline)
	}
	if !findPart(advice, "pickup").DirectHome {
		t.Fatalf("有预警下午接到直接回")
	}
}

func TestMorningSwingSuggestsLayers(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 {
		if h >= 11 && h <= 14 {
			return 22
		}
		return 10
	}, nil, "17:20")
	advice := Advise(Input{Slot: SlotMorning, Now: hourAt(7), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	drop := findPart(advice, "drop")
	if drop == nil || drop.Level.Key != "cold" || !strings.Contains(strings.Join(drop.Notes, ""), "能脱") {
		t.Fatalf("早上 10 度中午 22 度：穿冷档、提醒能脱，得到 %+v", drop)
	}
	if findPart(advice, "pickup") == nil {
		t.Fatalf("早上那条也要预告下午接")
	}
}

func TestWeekendOutingSkippedWhenFreezing(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 2 }, nil, "17:00")
	plan := DayPlan{Date: "2026-11-20", Outings: []Outing{{From: "09:30", To: "11:00"}, {From: "15:00", To: "16:30"}}}
	advice := Advise(Input{Slot: SlotMorning, Now: hourAt(7), Snapshot: snapshot, Plan: plan, AgeMonths: 42, Config: DefaultConfig()})
	if len(advice.Parts) != 2 || !advice.Parts[0].Skip || !advice.Parts[1].Skip {
		t.Fatalf("体感 2 度低于出门下限 5 度，两段都别出去: %+v", advice.Parts)
	}
}

func TestEveningCoversNightAndTomorrow(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 { return 12 }, nil, "17:20")
	tomorrow := DayPlan{Date: "2026-11-21", School: true, Drop: "08:00", Pickup: "15:50", Home: "16:30", Outings: []Outing{}}
	advice := Advise(Input{Slot: SlotEvening, Now: hourAt(18), Snapshot: snapshot, Plan: schoolPlan(), Tomorrow: &tomorrow, AgeMonths: 42, Config: DefaultConfig()})
	if advice.Night == nil || advice.Night.Low != 12 {
		t.Fatalf("今晚最低应是 12: %+v", advice.Night)
	}
	if len(advice.Parts) != 1 || advice.Parts[0].Label != "明早送" {
		t.Fatalf("晚上说明早送: %+v", advice.Parts)
	}
}

func TestCheckNumbers(t *testing.T) {
	snapshot := snapshotWith(func(h int) float64 {
		if h >= 15 {
			return 8
		}
		return 13
	}, nil, "17:00")
	advice := Advise(Input{Slot: SlotNoon, Now: hourAt(12), Snapshot: snapshot, Plan: schoolPlan(), AgeMonths: 42, Config: DefaultConfig()})
	good := "今天下午冷，体感只有8度，接到直接回，4点10分前到家。穿秋衣秋裤、毛衣、厚外套，戴帽子。"
	if problem := checkNumbers(good, advice); problem != "" {
		t.Fatalf("应该通过: %s", problem)
	}
	if problem := checkNumbers("接到直接回，16:20 前到家", advice); problem == "" {
		t.Fatalf("16:20 不在结论里，应该拦下")
	}
	if problem := checkNumbers("体感只有 3 度", advice); problem == "" {
		t.Fatalf("3 度不在结论里，应该拦下")
	}
	if template := Template(advice); checkNumbers(template, advice) != "" {
		t.Fatalf("模板自己应该通过: %s / %s", template, checkNumbers(template, advice))
	}
}

func TestPlanOverride(t *testing.T) {
	config := DefaultConfig()
	friday := testDay
	plan := planOf(config, friday, &dayRow{PickupAt: "16:10"})
	if !plan.School || plan.Pickup != "16:10" || plan.Drop != "08:00" || plan.Home != "16:30" || !plan.Override {
		t.Fatalf("只改接的时间: %+v", plan)
	}
	plan = planOf(config, friday, &dayRow{NoSchool: true, Outings: `[{"from":"15:00","to":"18:00","note":"去外婆家"}]`})
	if plan.School || len(plan.Outings) != 1 || plan.Outings[0].Note != "去外婆家" {
		t.Fatalf("不去幼儿园、去外婆家: %+v", plan)
	}
	saturday := planOf(config, friday.AddDate(0, 0, 1), nil)
	if saturday.School || len(saturday.Outings) != 2 {
		t.Fatalf("周六按周末时段: %+v", saturday)
	}
}

func TestParseCMA(t *testing.T) {
	sk := []byte(`var dataSK={"cityname":"青浦","temp":"20","WD":"西风","WS":"1级","wse":"5km\/h","SD":"93%","time":"23:25","rain":"0.1","aqi":"25","weather":"阴"}`)
	now, err := parseCMANow(sk)
	if err != nil || now.Temp != 20 || now.AQI != 25 || now.Humidity != 93 || now.Feels == 0 {
		t.Fatalf("实况: %+v %v", now, err)
	}
	hourly := []byte(`var fc180={"f0":"2026102020","jh":[{"ja":"07","jb":"22","jc":"0","jd":"8","je":"83","jf":"202610202000","jg":"0.2"},{"ja":"01","jb":"999","jf":"202610202100"}]};`)
	hours, err := parseCMAHours(hourly)
	if err != nil || len(hours) != 1 || hours[0].Text != "小雨" || !hours[0].Wet || hours[0].At.Hour() != 20 {
		t.Fatalf("逐小时: %+v %v", hours, err)
	}
	daily := []byte(`var fc40=[{"001":"01","002":"08","003":"25","004":"18","009":"20261020","014":"06:01","015":"17:19"}];var index3d={}`)
	days, err := parseCMADays(daily)
	if err != nil || len(days) != 1 || days[0].Sunset != "17:19" || days[0].High != 25 || days[0].NightText != "中雨" {
		t.Fatalf("按天: %+v %v", days, err)
	}
	alarm := []byte(`var cityDZ101020800 ={"weatherinfo":{}};var alarmDZ101020800 ={"w":[{"w5":"大风","w7":"黄色","w9":"预计……"}]}`)
	if warnings := parseCMAWarnings(alarm); len(warnings) != 1 || !warnings[0].Serious() || warnings[0].Title() != "大风黄色预警" {
		t.Fatalf("预警: %+v", warnings)
	}
	index := []byte(`var dataZS ={"zs":{"ct_name":"穿衣指数","ct_hint":"较舒适","ct_des_s":"建议穿薄外套。"}}`)
	if clothing := parseCMAClothing(index); clothing != "较舒适：建议穿薄外套。" {
		t.Fatalf("穿衣指数: %s", clothing)
	}
}

func TestApparent(t *testing.T) {
	if value := apparent(5, 80, 5.5); value > 2 || value < -2 {
		t.Fatalf("5 度、4 级风体感应在 0 度上下，得到 %.1f", value)
	}
	if value := apparent(32, 80, 1); value < 36 {
		t.Fatalf("32 度又潮又闷体感应更高，得到 %.1f", value)
	}
}

// TestLiveCMA 真的去中国天气网取青浦的天气：WEATHER_LIVE=1 go test -run Live -v
func TestLiveCMA(t *testing.T) {
	if os.Getenv("WEATHER_LIVE") == "" {
		t.Skip("WEATHER_LIVE=1 才联网")
	}
	location := DefaultConfig().Location
	for _, provider := range []Provider{newCMA(), newOpenMeteo()} {
		snapshot, err := provider.Fetch(context.Background(), location)
		if err != nil {
			t.Fatalf("%s: %v", provider.Name(), err)
		}
		snapshot.Location, snapshot.Source = location, provider.Name()
		t.Logf("%s 现在 %.0f° 体感 %.1f° %s %s AQI %d；逐小时 %d 条（%s 起）；按天 %d 条；预警 %d；穿衣指数 %s",
			provider.Name(), snapshot.Now.Temp, snapshot.Now.Feels, snapshot.Now.Text, snapshot.Now.Wind, snapshot.Now.AQI,
			len(snapshot.Hours), snapshot.Hours[0].At.Format("01-02 15:04"), len(snapshot.Days), len(snapshot.Warnings), snapshot.Clothing)
		plan := planOf(DefaultConfig(), today(), nil)
		advice := Advise(Input{Slot: SlotMorning, Now: time.Now(), Snapshot: snapshot, Plan: plan, AgeMonths: 42, Config: DefaultConfig()})
		t.Logf("结论：%s\n%s", advice.Headline, Template(advice))
	}
	locations, err := searchCMA(context.Background(), "赵巷")
	if err != nil || len(locations) == 0 {
		t.Fatalf("搜索: %v %v", locations, err)
	}
	t.Logf("搜索赵巷: %+v", locations)
}
