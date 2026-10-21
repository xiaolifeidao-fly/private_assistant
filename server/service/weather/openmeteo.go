package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Open-Meteo（api.open-meteo.com）：备用源，不用 key，正规 JSON。没有预警和穿衣指数。
// 中国天气网取不到或者解析失败时用它，按经纬度查。
type openMeteo struct{ client *http.Client }

func newOpenMeteo() *openMeteo { return &openMeteo{client: &http.Client{Timeout: 15 * time.Second}} }

func (*openMeteo) Name() string { return "open-meteo" }

func (p *openMeteo) Fetch(ctx context.Context, location Location) (Snapshot, error) {
	if location.Lat == 0 && location.Lon == 0 {
		return Snapshot{}, errors.New("没有经纬度")
	}
	query := url.Values{}
	query.Set("latitude", fmt.Sprintf("%.4f", location.Lat))
	query.Set("longitude", fmt.Sprintf("%.4f", location.Lon))
	query.Set("current", "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m,precipitation")
	query.Set("hourly", "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m,precipitation")
	query.Set("daily", "temperature_2m_max,temperature_2m_min,weather_code,sunrise,sunset")
	query.Set("wind_speed_unit", "ms")
	query.Set("timezone", "Asia/Shanghai")
	query.Set("forecast_days", "7")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.open-meteo.com/v1/forecast?"+query.Encode(), nil)
	if err != nil {
		return Snapshot{}, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return Snapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var data struct {
		Current struct {
			Time        string  `json:"time"`
			Temperature float64 `json:"temperature_2m"`
			Humidity    float64 `json:"relative_humidity_2m"`
			Code        int     `json:"weather_code"`
			Wind        float64 `json:"wind_speed_10m"`
			Rain        float64 `json:"precipitation"`
		} `json:"current"`
		Hourly struct {
			Time        []string  `json:"time"`
			Temperature []float64 `json:"temperature_2m"`
			Humidity    []float64 `json:"relative_humidity_2m"`
			Code        []int     `json:"weather_code"`
			Wind        []float64 `json:"wind_speed_10m"`
			Rain        []float64 `json:"precipitation"`
		} `json:"hourly"`
		Daily struct {
			Time    []string  `json:"time"`
			Max     []float64 `json:"temperature_2m_max"`
			Min     []float64 `json:"temperature_2m_min"`
			Code    []int     `json:"weather_code"`
			Sunrise []string  `json:"sunrise"`
			Sunset  []string  `json:"sunset"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return Snapshot{}, fmt.Errorf("解析失败: %w", err)
	}
	const layout = "2006-01-02T15:04"
	current := data.Current
	snapshot := Snapshot{Now: Now{Temp: current.Temperature, Humidity: current.Humidity, Text: wmoWeather(current.Code),
		WindSpeed: current.Wind, Rain1h: current.Rain, Wind: fmt.Sprintf("%d级", levelOfSpeed(current.Wind))}}
	snapshot.Now.Feels = apparent(current.Temperature, current.Humidity, current.Wind)
	snapshot.Now.At, _ = time.ParseInLocation(layout, current.Time, shanghai)
	hourly := data.Hourly
	for i, value := range hourly.Time {
		when, err := time.ParseInLocation(layout, value, shanghai)
		if err != nil || i >= len(hourly.Temperature) || i >= len(hourly.Humidity) || i >= len(hourly.Code) || i >= len(hourly.Wind) || i >= len(hourly.Rain) {
			continue
		}
		hour := Hour{At: when, Temp: hourly.Temperature[i], Humidity: hourly.Humidity[i], Text: wmoWeather(hourly.Code[i]),
			Rain: hourly.Rain[i], WindSpeed: hourly.Wind[i], WindLevel: levelOfSpeed(hourly.Wind[i])}
		hour.Feels = apparent(hour.Temp, hour.Humidity, hour.WindSpeed)
		hour.Wet = isWet(hour.Text) || hour.Rain >= 0.3
		snapshot.Hours = append(snapshot.Hours, hour)
	}
	if len(snapshot.Hours) == 0 {
		return Snapshot{}, errors.New("逐小时预报是空的")
	}
	daily := data.Daily
	for i, value := range daily.Time {
		if i >= len(daily.Max) || i >= len(daily.Min) || i >= len(daily.Code) || i >= len(daily.Sunrise) || i >= len(daily.Sunset) {
			break
		}
		clock := func(value string) string {
			if parsed, err := time.ParseInLocation(layout, value, shanghai); err == nil {
				return parsed.Format("15:04")
			}
			return ""
		}
		snapshot.Days = append(snapshot.Days, Day{Date: value, High: daily.Max[i], Low: daily.Min[i], DayText: wmoWeather(daily.Code[i]),
			Sunrise: clock(daily.Sunrise[i]), Sunset: clock(daily.Sunset[i])})
	}
	return snapshot, nil
}

// geocode 用 Open-Meteo 的地名搜索按拼音找经纬度，只认同一个省的结果。先找本名，找不到再找上一级。
func geocode(ctx context.Context, province string, names ...string) (float64, float64) {
	client := &http.Client{Timeout: 6 * time.Second}
	province = strings.TrimSuffix(strings.TrimSuffix(province, "市"), "省")
	for _, name := range names {
		name = strings.TrimSpace(name)
		for _, suffix := range []string{"zhen", "xiang", "jiedao"} {
			if trimmed := strings.TrimSuffix(strings.ToLower(name), suffix); trimmed != strings.ToLower(name) && len(trimmed) > 2 {
				name = trimmed
				break
			}
		}
		if name == "" {
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://geocoding-api.open-meteo.com/v1/search?count=10&language=zh&countryCode=CN&name="+url.QueryEscape(name), nil)
		if err != nil {
			continue
		}
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		var data struct {
			Results []struct {
				Admin1    string  `json:"admin1"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"results"`
		}
		err = json.NewDecoder(response.Body).Decode(&data)
		response.Body.Close()
		if err != nil {
			continue
		}
		for _, result := range data.Results {
			if province == "" || strings.Contains(result.Admin1, province) {
				return result.Latitude, result.Longitude
			}
		}
	}
	return 0, 0
}
