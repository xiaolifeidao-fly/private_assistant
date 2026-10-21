package weather

import (
	"math"
	"strings"
)

// apparent 体感温度。
//
//   - 26 度以上：Steadman 的 Apparent Temperature（澳大利亚气象局用的那个），又潮又热时更闷；
//   - 26 度以下：只算风，风速每超过 1.5 m/s 一米，冷 0.7 度；10 度以下又很潮（≥ 80%）再冷 1 度。
//
// 上海常年潮，凉快天也把湿度算进去会把 25 度算成 29 度，穿衣就偏少了，所以不热的时候不算湿度。
// 中国天气网实况没给体感，两个源都按这个算，口径一致。
func apparent(temp, humidity, windSpeed float64) float64 {
	if humidity <= 0 || humidity > 100 {
		humidity = 60
	}
	if windSpeed < 0 {
		windSpeed = 0
	}
	if temp >= 26 {
		vapor := humidity / 100 * 6.105 * math.Exp(17.27*temp/(237.7+temp))
		return math.Round(math.Max(temp+0.33*vapor-0.70*windSpeed-4.00, temp)*10) / 10
	}
	feels := temp - 0.7*math.Max(windSpeed-1.5, 0)
	if temp < 10 && humidity >= 80 {
		feels--
	}
	return math.Round(feels*10) / 10
}

// windLevelSpeed 风力等级代码（中国天气网 0 = <3 级，1 = 3-4 级……）对应的大概风速 m/s。
func windLevelSpeed(code int) float64 {
	speeds := []float64{2, 5.5, 7.5, 9.5, 12, 15.5, 19, 22.5, 26.5, 30.5}
	if code < 0 {
		return 2
	}
	if code >= len(speeds) {
		return speeds[len(speeds)-1]
	}
	return speeds[code]
}

// levelOfSpeed 风速 m/s 对应的蒲福风级。
func levelOfSpeed(speed float64) int {
	limits := []float64{0.3, 1.6, 3.4, 5.5, 8.0, 10.8, 13.9, 17.2, 20.8, 24.5, 28.5, 32.7}
	for level, limit := range limits {
		if speed < limit {
			return level
		}
	}
	return 12
}

// cmaWeather 中国气象局天气现象代码。
var cmaWeather = map[string]string{
	"00": "晴", "01": "多云", "02": "阴", "03": "阵雨", "04": "雷阵雨", "05": "雷阵雨伴有冰雹", "06": "雨夹雪",
	"07": "小雨", "08": "中雨", "09": "大雨", "10": "暴雨", "11": "大暴雨", "12": "特大暴雨", "13": "阵雪",
	"14": "小雪", "15": "中雪", "16": "大雪", "17": "暴雪", "18": "雾", "19": "冻雨", "20": "沙尘暴",
	"21": "小到中雨", "22": "中到大雨", "23": "大到暴雨", "24": "暴雨到大暴雨", "25": "大暴雨到特大暴雨",
	"26": "小到中雪", "27": "中到大雪", "28": "大到暴雪", "29": "浮尘", "30": "扬沙", "31": "强沙尘暴",
	"32": "浓雾", "49": "强浓雾", "53": "霾", "54": "中度霾", "55": "重度霾", "56": "严重霾", "57": "大雾", "58": "特强浓雾",
	"301": "雨", "302": "雪",
}

// cmaWindDir 风向代码。
var cmaWindDir = []string{"无持续风向", "东北风", "东风", "东南风", "南风", "西南风", "西风", "西北风", "北风", "旋转风"}

// cmaWindLevel 风力代码。
var cmaWindLevel = []string{"<3级", "3-4级", "4-5级", "5-6级", "6-7级", "7-8级", "8-9级", "9-10级", "10-11级", "11-12级"}

// wmoWeather Open-Meteo 用的 WMO 天气代码。
func wmoWeather(code int) string {
	switch {
	case code == 0:
		return "晴"
	case code == 1 || code == 2:
		return "多云"
	case code == 3:
		return "阴"
	case code == 45 || code == 48:
		return "雾"
	case code >= 51 && code <= 57:
		return "毛毛雨"
	case code == 61 || code == 80:
		return "小雨"
	case code == 63 || code == 81:
		return "中雨"
	case code == 65 || code == 82:
		return "大雨"
	case code == 66 || code == 67:
		return "冻雨"
	case code == 71 || code == 85:
		return "小雪"
	case code == 73:
		return "中雪"
	case code == 75 || code == 86:
		return "大雪"
	case code == 77:
		return "雪粒"
	case code >= 95:
		return "雷阵雨"
	}
	return "未知"
}

// isWet 天气现象是不是在下雨 / 下雪。
func isWet(text string) bool {
	for _, word := range []string{"雨", "雪", "冰雹"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}
