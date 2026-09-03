package eew

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// EEWEvent WebSocket / HTTP JSON 推送体模型
type EEWEvent struct {
	Type         string   `json:"type"`
	Title        string   `json:"Title"`
	EventID      string   `json:"EventID"`
	ReportNum    int      `json:"ReportNum"`
	ReportTime   string   `json:"ReportTime"`
	OriginTime   string   `json:"OriginTime"`
	HypoCenter   string   `json:"HypoCenter"`
	Hypocenter   string   `json:"Hypocenter"`
	Latitude     float64  `json:"Latitude"`
	Longitude    float64  `json:"Longitude"`
	Magnitude    float64  `json:"Magnitude"`
	Magunitude   float64  `json:"Magunitude"`
	Depth        *float64 `json:"Depth"`
	MaxIntensity any      `json:"MaxIntensity"`
	IsFinal      bool     `json:"isFinal"`
	IsCancel     bool     `json:"isCancel"`
	OriginalText string   `json:"OriginalText"`
	Ver          int      `json:"ver"`
	ID           any      `json:"id"`
	Timestamp    any      `json:"timestamp"`
}

func (e *EEWEvent) GetMagnitude() float64 {
	if e.Magnitude > 0 {
		return e.Magnitude
	}
	return e.Magunitude
}

func (e *EEWEvent) GetLocation() string {
	if e.HypoCenter != "" {
		return e.HypoCenter
	}
	if e.Hypocenter != "" {
		return e.Hypocenter
	}
	if e.Title != "" {
		return e.Title
	}
	return "未知震源地"
}

func (e *EEWEvent) GetDepthStr() string {
	if e.Depth == nil || *e.Depth <= 0 {
		return "不明"
	}
	return fmt.Sprintf("%.1f km", *e.Depth)
}

func (e *EEWEvent) GetMaxIntensityStr() string {
	if e.MaxIntensity == nil {
		return "不明"
	}
	switch v := e.MaxIntensity.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" || s == "-" || s == "未知" || s == "不明" || s == "0" {
			return "不明"
		}
		if !strings.HasSuffix(s, "度") {
			return s + " 度"
		}
		return s
	case float64:
		if v <= 0 {
			return "不明"
		}
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d 度", int64(v))
		}
		return fmt.Sprintf("%.1f 度", v)
	case int:
		if v <= 0 {
			return "不明"
		}
		return fmt.Sprintf("%d 度", v)
	case int64:
		if v <= 0 {
			return "不明"
		}
		return fmt.Sprintf("%d 度", v)
	default:
		s := fmt.Sprintf("%v", v)
		if s == "" || s == "0" {
			return "不明"
		}
		return s + " 度"
	}
}

func (e *EEWEvent) GetCoordinateStr() string {
	return FormatCoordinate(e.Latitude, e.Longitude)
}

func (e *EEWEvent) GetIntensityNum() int {
	if e.MaxIntensity == nil {
		return 0
	}
	switch v := e.MaxIntensity.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// GetEventTime 超强全兼容时间解析：支持多种时间格式及 EventID (如 202607130727.0001_1) 时间前缀解析
func (e *EEWEvent) GetEventTime() time.Time {
	// 1. 优先尝试从 ReportTime 或 OriginTime 解析
	for _, timeStr := range []string{e.ReportTime, e.OriginTime} {
		timeStr = strings.TrimSpace(timeStr)
		if timeStr == "" {
			continue
		}
		timeStr = strings.ReplaceAll(timeStr, "/", "-")

		formats := []string{
			"2006-01-02 15:04:05",
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04",
			"2006-01-02T15:04:05Z07:00",
		}
		for _, fmtStr := range formats {
			if t, err := time.ParseInLocation(fmtStr, timeStr, time.Local); err == nil {
				return t
			}
		}

		if sec, err := strconv.ParseInt(timeStr, 10, 64); err == nil && sec > 1000000000 {
			return time.Unix(sec, 0)
		}
	}

	// 2. 从 EventID 提取年月日时分 (如 202607130727.0001_1 提取前12位数字 202607130727)
	eventID := strings.TrimSpace(e.EventID)
	if len(eventID) >= 12 {
		var dateDigits strings.Builder
		for _, r := range eventID {
			if r >= '0' && r <= '9' {
				dateDigits.WriteRune(r)
				if dateDigits.Len() == 12 {
					break
				}
			} else {
				break
			}
		}
		if dateDigits.Len() == 12 {
			if t, err := time.ParseInLocation("200601021504", dateDigits.String(), time.Local); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// CENCEQItem 中国地震台网 历史速报项
type CENCEQItem struct {
	Type       string `json:"type"`
	EventID    string `json:"EventID"`
	Time       string `json:"time"`
	ReportTime string `json:"ReportTime"`
	Location   string `json:"location"`
	PlaceName  string `json:"placeName"`
	Magnitude  string `json:"magnitude"`
	Depth      string `json:"depth"`
	Latitude   string `json:"latitude"`
	Longitude  string `json:"longitude"`
	Intensity  string `json:"intensity"`
}

func (c *CENCEQItem) GetDepthStr() string {
	d := strings.TrimSpace(c.Depth)
	if d == "" || d == "-" || d == "未知" || d == "不明" {
		return "不明"
	}
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(d, "km"), "KM"))
	if val, err := strconv.ParseFloat(trimmed, 64); err == nil {
		if val <= 0 {
			return "不明"
		}
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d km", int64(val))
		}
		return fmt.Sprintf("%.1f km", val)
	}
	return "不明"
}

func (c *CENCEQItem) GetIntensityStr() string {
	val := strings.TrimSpace(c.Intensity)
	if val == "" || val == "-" || val == "未知" || val == "不明" || val == "0" {
		return "不明"
	}
	if !strings.HasSuffix(val, "度") {
		return val + " 度"
	}
	return val
}

func (c *CENCEQItem) GetCoordinateStr() string {
	lat, err1 := strconv.ParseFloat(c.Latitude, 64)
	lng, err2 := strconv.ParseFloat(c.Longitude, 64)
	if err1 != nil || err2 != nil {
		return "不明"
	}
	return FormatCoordinate(lat, lng)
}

// WeatherRankItem 气象排行元素
type WeatherRankItem struct {
	Province string `json:"province"`
	City     string `json:"city"`
	Value    string `json:"value"`
}

// WeatherHourRank 每小时三大气象排行
type WeatherHourRank struct {
	TempRank  []WeatherRankItem `json:"tempRank"`
	RainRank  []WeatherRankItem `json:"rainRank"`
	WindSRank []WeatherRankItem `json:"windSRank"`
}

// CalcDistance 使用 Haversine 公式计算两个经纬度坐标之间的球面大圆距离（单位：公里 km）
func CalcDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	rad := math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	lat1Rad := lat1 * rad
	lat2Rad := lat2 * rad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	dist := earthRadiusKm * c
	return math.Round(dist*10) / 10
}

// CalcLocalIntensity 依据中国地震烈度衰减关系（国家标准/汪素云中国大陆经验衰减公式）推算本地预估烈度
// 公式形式: I = 4.493 + 1.454 * M - 1.792 * ln(R + 16)，其中 R = sqrt(D^2 + h^2)
// depthKm 若 <= 0 或不明，按浅源地震默认深度 10.0 km 计算
func CalcLocalIntensity(mag float64, distKm float64, depthKm float64) float64 {
	if mag <= 0 {
		return 0.0
	}
	if depthKm <= 0 {
		depthKm = 10.0
	}
	// 空间震源距 R
	hypoDist := math.Sqrt(distKm*distKm + depthKm*depthKm)
	intensity := 4.493 + 1.454*mag - 1.792*math.Log(hypoDist+16.0)
	if intensity < 0 {
		intensity = 0.0
	}
	return math.Round(intensity*10) / 10
}

// GetIntensityDesc 获取地震烈度的体感/影响等级描述
func GetIntensityDesc(intensity float64) string {
	switch {
	case intensity < 1.0:
		return "无感"
	case intensity < 3.0:
		return "轻微有感"
	case intensity < 5.0:
		return "明显震感"
	case intensity < 7.0:
		return "强烈震感"
	default:
		return "破坏性震感"
	}
}

// FormatCoordinate 格式化经纬度坐标为易读格式（如 28.51°N, 104.67°E）
func FormatCoordinate(lat, lng float64) string {
	if lat == 0 && lng == 0 {
		return "不明"
	}
	latDir := "N"
	if lat < 0 {
		latDir = "S"
		lat = -lat
	}
	lngDir := "E"
	if lng < 0 {
		lngDir = "W"
		lng = -lng
	}
	return fmt.Sprintf("%.2f°%s, %.2f°%s", lat, latDir, lng, lngDir)
}
