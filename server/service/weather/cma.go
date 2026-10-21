package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 中国天气网（www.weather.com.cn）网页自己用的数据接口：百度搜「青浦天气」那张卡片就是这份数据。不用 key。
// 没有公开文档、字段是缩写，以后可能改版：解析写得宽松，哪个字段解析不出就当没有，整份失败交给备用源。
//
//	sk_2d/{code}.html          实况 dataSK
//	wap_180h/{code}.html       逐小时 fc180.jh：ja 天气代码、jb 温度、jc 风力代码、jd 风向代码、je 湿度、jf 时间、jg 降水量
//	wap_40d/{code}.html        按天 fc40：001/002 白天/夜里天气代码、003/004 最高/最低、009 日期、014/015 日出/日落
//	dingzhi/{code}.html        今天概况 + 预警 alarmDZ.w：w5 类型、w7 级别、w8 时间、w9 内容
//	weather_index/{code}.html  生活指数 dataZS.zs：ct_hint / ct_des_s 穿衣指数
const (
	cmaHost    = "http://d1.weather.com.cn"
	cmaReferer = "http://www.weather.com.cn/"
	cmaAgent   = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
)

type cma struct{ client *http.Client }

func newCMA() *cma { return &cma{client: &http.Client{Timeout: 12 * time.Second}} }

func (*cma) Name() string { return "cma" }

func (p *cma) Fetch(ctx context.Context, location Location) (Snapshot, error) {
	code := cmaDataCode(location.Code)
	if code == "" {
		return Snapshot{}, errors.New("没有中国天气网的城市编码")
	}
	var snapshot Snapshot
	// 实况和逐小时是必须的，其余取不到不影响
	sk, err := p.get(ctx, "sk_2d", code)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Now, err = parseCMANow(sk); err != nil {
		return Snapshot{}, err
	}
	hourly, err := p.get(ctx, "wap_180h", code)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Hours, err = parseCMAHours(hourly); err != nil {
		return Snapshot{}, err
	}
	if daily, err := p.get(ctx, "wap_40d", code); err == nil {
		snapshot.Days, _ = parseCMADays(daily)
	}
	if body, err := p.get(ctx, "dingzhi", code); err == nil {
		snapshot.Warnings = parseCMAWarnings(body)
	}
	if body, err := p.get(ctx, "weather_index", code); err == nil {
		snapshot.Clothing = parseCMAClothing(body)
	}
	return snapshot, nil
}

func (p *cma) get(ctx context.Context, kind, code string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%s.html?_=%d", cmaHost, kind, code, time.Now().UnixMilli()), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Referer", cmaReferer)
	request.Header.Set("User-Agent", cmaAgent)
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("var ")) {
		return nil, fmt.Errorf("%s: HTTP %d", kind, response.StatusCode)
	}
	return body, nil
}

// cmaDataCode 镇一级的编码（12 位）取数据时用所在区县的 9 位编码。
func cmaDataCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) > 9 {
		code = code[:9]
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return code
}

// jsVar 取 `var name = {...}` 里的 JSON。name 后面可能跟着城市编码（cityDZ101020800）。
func jsVar(body []byte, name string, target any) error {
	pattern := regexp.MustCompile(`var\s+` + regexp.QuoteMeta(name) + `\w*\s*=\s*`)
	location := pattern.FindIndex(body)
	if location == nil {
		return fmt.Errorf("没有 %s", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(body[location[1]:]))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s 解析失败: %w", name, err)
	}
	return nil
}

func number(value string) (float64, bool) {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, "℃"), "%"))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed >= 999 || parsed <= -999 {
		return 0, false
	}
	return parsed, true
}

func parseCMANow(body []byte) (Now, error) {
	var sk map[string]string
	if err := jsVar(body, "dataSK", &sk); err != nil {
		return Now{}, err
	}
	temp, ok := number(sk["temp"])
	if !ok {
		return Now{}, errors.New("实况没有温度")
	}
	now := Now{Temp: temp, Text: sk["weather"]}
	now.Humidity, _ = number(firstNonEmpty(sk["SD"], sk["sd"]))
	if speed, ok := number(strings.TrimSuffix(sk["wse"], "km/h")); ok {
		now.WindSpeed = speed / 3.6
	} else {
		now.WindSpeed = 2
	}
	now.Wind = strings.TrimSpace(sk["WD"] + " " + sk["WS"])
	now.Rain1h, _ = number(sk["rain"])
	if aqi, ok := number(sk["aqi"]); ok {
		now.AQI = int(aqi)
	}
	now.Feels = apparent(now.Temp, now.Humidity, now.WindSpeed)
	if clock, err := time.Parse("15:04", strings.TrimSpace(sk["time"])); err == nil {
		day := today()
		now.At = time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, shanghai)
		if now.At.After(time.Now().Add(time.Hour)) { // 23:50 的实况在 00:10 取到
			now.At = now.At.AddDate(0, 0, -1)
		}
	} else {
		now.At = time.Now()
	}
	return now, nil
}

func parseCMAHours(body []byte) ([]Hour, error) {
	var data struct {
		JH []map[string]string `json:"jh"`
	}
	if err := jsVar(body, "fc180", &data); err != nil {
		return nil, err
	}
	var hours []Hour
	for _, item := range data.JH {
		when, err := time.ParseInLocation("200601021504", item["jf"], shanghai)
		if err != nil {
			continue
		}
		temp, ok := number(item["jb"])
		if !ok {
			continue
		}
		hour := Hour{At: when, Temp: temp, Text: cmaWeather[item["ja"]]}
		hour.Humidity, _ = number(item["je"])
		hour.Rain, _ = number(item["jg"])
		level, _ := strconv.Atoi(item["jc"])
		hour.WindSpeed = windLevelSpeed(level)
		hour.WindLevel = levelOfSpeed(hour.WindSpeed)
		hour.Feels = apparent(temp, hour.Humidity, hour.WindSpeed)
		hour.Wet = isWet(hour.Text) || hour.Rain >= 0.3
		hours = append(hours, hour)
	}
	if len(hours) == 0 {
		return nil, errors.New("逐小时预报是空的")
	}
	return hours, nil
}

func parseCMADays(body []byte) ([]Day, error) {
	var data []map[string]string
	if err := jsVar(body, "fc40", &data); err != nil {
		return nil, err
	}
	var days []Day
	for _, item := range data {
		date, err := time.ParseInLocation("20060102", item["009"], shanghai)
		if err != nil {
			continue
		}
		day := Day{Date: date.Format(dateLayout), DayText: cmaWeather[item["001"]], NightText: cmaWeather[item["002"]],
			Sunrise: item["014"], Sunset: item["015"]}
		high, okHigh := number(item["003"])
		low, okLow := number(item["004"])
		if !okHigh && !okLow {
			continue
		}
		if !okHigh {
			high = low
		}
		if !okLow {
			low = high
		}
		day.High, day.Low = high, low
		days = append(days, day)
		if len(days) >= 7 {
			break
		}
	}
	return days, nil
}

func parseCMAWarnings(body []byte) []Warning {
	var data struct {
		W []map[string]any `json:"w"`
	}
	if err := jsVar(body, "alarmDZ", &data); err != nil {
		return nil
	}
	var warnings []Warning
	for _, item := range data.W {
		text := func(key string) string {
			value, _ := item[key].(string)
			return strings.TrimSpace(value)
		}
		warning := Warning{Type: strings.TrimSuffix(text("w5"), "预警"), Level: strings.TrimSuffix(text("w7"), "预警"), At: text("w8"), Text: text("w9")}
		if warning.Type == "" {
			continue
		}
		warnings = append(warnings, warning)
	}
	return warnings
}

func parseCMAClothing(body []byte) string {
	var data struct {
		ZS map[string]string `json:"zs"`
	}
	if err := jsVar(body, "dataZS", &data); err != nil {
		return ""
	}
	hint, detail := strings.TrimSpace(data.ZS["ct_hint"]), strings.TrimSpace(data.ZS["ct_des_s"])
	if hint == "" {
		return detail
	}
	return strings.TrimSpace(hint + "：" + detail)
}

// searchCMA 用中国天气网的城市搜索找地方，再用 Open-Meteo 的地名搜索补上经纬度（备用源要用）。
func searchCMA(ctx context.Context, query string) ([]Location, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://toy1.weather.com.cn/search?callback=cb&cityname="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Referer", cmaReferer)
	request.Header.Set("User-Agent", cmaAgent)
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("搜索地方失败: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	start, end := bytes.IndexByte(body, '('), bytes.LastIndexByte(body, ')')
	if start < 0 || end <= start {
		return []Location{}, nil
	}
	var items []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body[start+1:end], &items); err != nil {
		return nil, fmt.Errorf("搜索结果解析失败: %w", err)
	}
	result := []Location{}
	for _, item := range items {
		// 101020800009~shanghai~赵巷镇~zhaoxiangzhen~青浦~qingpu~021~201700~shanghai~上海
		parts := strings.Split(item.Ref, "~")
		if len(parts) < 10 || cmaDataCode(parts[0]) == "" || strings.Contains(parts[9], "景点") {
			continue
		}
		name := parts[2]
		if parts[4] != "" && parts[4] != parts[2] {
			name = parts[4] + " " + parts[2]
		}
		if !strings.HasPrefix(name, parts[9]) {
			name = parts[9] + " " + name
		}
		location := Location{Name: name, Code: parts[0]}
		location.Lat, location.Lon = geocode(ctx, parts[9], parts[3], parts[5])
		result = append(result, location)
		if len(result) >= 8 {
			break
		}
	}
	return result, nil
}
