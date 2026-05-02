// Package eco geocode resolves common 雙北 place names to (lat, lng).
// Lookup order:
//  1. In-memory dictionary (zero latency, top destinations + 雙北捷運站)
//  2. Mapbox Forward Geocoding API (sub-second, restricted to 雙北 bbox)
//  3. Cached results from #2 are kept for the process lifetime
package eco

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ErrUnknownPlace is returned when neither dictionary nor Mapbox API
// could resolve the query.
var ErrUnknownPlace = errors.New("eco: unknown place")

type placeCoord struct {
	Lat float64
	Lng float64
}

var (
	taipeiCityHall    = placeCoord{Lat: 25.0376, Lng: 121.5644}
	newTaipeiCityHall = placeCoord{Lat: 25.0124, Lng: 121.4664}
	taipeiStation     = placeCoord{Lat: 25.0478, Lng: 121.5170}
	banqiaoStation    = placeCoord{Lat: 25.0145, Lng: 121.4636}
)

// In-memory dictionary for top destinations (instant lookup).
var placeDictionary = map[string]placeCoord{
	// 雙北市府
	"台北市政府": taipeiCityHall,
	"市府":    taipeiCityHall,
	"北市府":   taipeiCityHall,
	"新北市政府":  newTaipeiCityHall,
	"新北市府":   newTaipeiCityHall,
	"板橋市政府":  newTaipeiCityHall,

	// 主要車站
	"台北車站":   taipeiStation,
	"臺北車站":   taipeiStation,
	"台北火車站":  taipeiStation,
	"臺北火車站":  taipeiStation,
	"板橋車站":   banqiaoStation,
	"板橋火車站":  banqiaoStation,
	"松山火車站":  {Lat: 25.0498, Lng: 121.5777},
	"南港火車站":  {Lat: 25.0535, Lng: 121.6072},
	"萬華車站":   {Lat: 25.0337, Lng: 121.4998},
	"萬華火車站":  {Lat: 25.0337, Lng: 121.4998},
	"板橋高鐵站":  banqiaoStation, // HSR 與台鐵同一棟
	"高鐵板橋站":  banqiaoStation,
	"南港高鐵站":  {Lat: 25.0535, Lng: 121.6072},
	"高鐵南港站":  {Lat: 25.0535, Lng: 121.6072},
	"台北高鐵站":  taipeiStation, // HSR 與台鐵同一棟
	"高鐵台北站":  taipeiStation,
	"汐止車站":   {Lat: 25.0688, Lng: 121.6624},
	"汐止火車站":  {Lat: 25.0688, Lng: 121.6624},
	"七堵車站":   {Lat: 25.0884, Lng: 121.7136},
	"樹林車站":   {Lat: 24.9933, Lng: 121.4205},
	"樹林火車站":  {Lat: 24.9933, Lng: 121.4205},

	// 常見地標 / 區中心
	"信義區":   {Lat: 25.0330, Lng: 121.5654},
	"大安區":   {Lat: 25.0260, Lng: 121.5430},
	"中山區":   {Lat: 25.0640, Lng: 121.5270},
	"中正區":   {Lat: 25.0320, Lng: 121.5180},
	"松山區":   {Lat: 25.0510, Lng: 121.5780},
	"內湖區":   {Lat: 25.0697, Lng: 121.5889},
	"南港區":   {Lat: 25.0540, Lng: 121.6070},
	"北投區":   {Lat: 25.1320, Lng: 121.5010},
	"士林區":   {Lat: 25.0880, Lng: 121.5260},
	"文山區":   {Lat: 24.9890, Lng: 121.5700},
	"大同區":   {Lat: 25.0631, Lng: 121.5152},
	"萬華區":   {Lat: 25.0320, Lng: 121.5000},
	"板橋":    {Lat: 25.0145, Lng: 121.4636},
	"中和":    {Lat: 24.9930, Lng: 121.4990},
	"永和":    {Lat: 25.0040, Lng: 121.5150},
	"三重":    {Lat: 25.0610, Lng: 121.4830},
	"新店":    {Lat: 24.9670, Lng: 121.5380},
	"淡水":    {Lat: 25.1670, Lng: 121.4400},

	// 知名地標
	"台北101":   {Lat: 25.0337, Lng: 121.5645},
	"101":     {Lat: 25.0337, Lng: 121.5645},
	"中正紀念堂":   {Lat: 25.0344, Lng: 121.5215},
	"國父紀念館":   {Lat: 25.0400, Lng: 121.5600},
	"大安森林公園":  {Lat: 25.0263, Lng: 121.5358},
	"故宮博物院":   {Lat: 25.1023, Lng: 121.5485},
	"龍山寺":    {Lat: 25.0370, Lng: 121.4998},
	"西門町":    {Lat: 25.0421, Lng: 121.5080},
	"信義商圈":    {Lat: 25.0330, Lng: 121.5654},
	"南港車站":    {Lat: 25.0535, Lng: 121.6072},
	"松山車站":    {Lat: 25.0498, Lng: 121.5777},
	"南港展覽館":   {Lat: 25.0560, Lng: 121.6175},
}

// dynamic cache for Mapbox + mock data results
var (
	dynCache = make(map[string]placeCoord)
	dynMu    sync.RWMutex
	httpCli  = &http.Client{Timeout: 4 * time.Second}
)

// 表示「使用者目前位置」之類無法 geocode 的關鍵字 — LLM 應該主動詢問起點
var currentLocationKeywords = []string{
	"我的位置", "我現在的地方", "目前位置", "現在位置", "當前位置",
	"我這裡", "我所在", "current location", "my location", "here",
}

// ErrCurrentLocation 提示 LLM 應該詢問使用者具體起點
var ErrCurrentLocation = errors.New("eco: query refers to user's current location; please ask for a concrete origin")

// 雙北邊界 (left,bottom,right,top) — Mapbox 用此限定搜尋範圍
const taipeiBBox = "121.40,24.90,121.75,25.30"

// 偏好點 (台北車站附近) — Mapbox 用此排序，相同名稱優先選最近的
const taipeiProximity = "121.5170,25.0478"

// Geocode resolves a place name to (lat, lng).
// Lookup order:
//  1. 拒絕「我目前位置」之類關鍵字 (回 ErrCurrentLocation)
//  2. 內建 dictionary (~30 條雙北重點)
//  3. Mock data POI 名稱比對 (~5,700 筆精準座標, 含 YouBike/公園/餐廳/旅館/回收)
//  4. Mapbox Forward Geocoding (含 sanity check: 結果必須在雙北 bbox)
//  5. ErrUnknownPlace
func Geocode(name string) (lat, lng float64, err error) {
	trimmed := strings.TrimSpace(name)
	for _, kw := range currentLocationKeywords {
		if strings.Contains(trimmed, kw) {
			return 0, 0, ErrCurrentLocation
		}
	}
	if c, ok := placeDictionary[trimmed]; ok {
		return c.Lat, c.Lng, nil
	}
	// 模糊匹配 dictionary: 去掉「火」字 (X火車站 -> X車站) / 簡轉繁 (台↔臺)
	for _, variant := range nameVariants(trimmed) {
		if c, ok := placeDictionary[variant]; ok {
			return c.Lat, c.Lng, nil
		}
	}
	dynMu.RLock()
	c, ok := dynCache[trimmed]
	dynMu.RUnlock()
	if ok {
		return c.Lat, c.Lng, nil
	}
	if c, ok := lookupInMockData(trimmed); ok {
		dynMu.Lock()
		dynCache[trimmed] = c
		dynMu.Unlock()
		return c.Lat, c.Lng, nil
	}
	c, err = mapboxForwardGeocode(trimmed)
	if err != nil {
		return 0, 0, err
	}
	if !insideTaipeiBBox(c) {
		return 0, 0, fmt.Errorf("%w: mapbox returned out-of-bbox result", ErrUnknownPlace)
	}
	dynMu.Lock()
	dynCache[trimmed] = c
	dynMu.Unlock()
	return c.Lat, c.Lng, nil
}

func insideTaipeiBBox(c placeCoord) bool {
	return c.Lng >= 121.40 && c.Lng <= 121.75 && c.Lat >= 24.90 && c.Lat <= 25.30
}

// nameVariants 產生輸入名稱的常見變體用於 dictionary 比對。
//   "松山火車站" -> ["松山車站"]   去掉多餘「火」字
//   "臺北車站"   -> ["台北車站"]   臺/台
//   "高鐵XX"     -> ["XX高鐵站"]   高鐵在前/後
func nameVariants(s string) []string {
	out := []string{}
	if strings.Contains(s, "火車站") {
		out = append(out, strings.Replace(s, "火車站", "車站", 1))
	}
	if strings.Contains(s, "臺") {
		out = append(out, strings.ReplaceAll(s, "臺", "台"))
	}
	if strings.Contains(s, "台") {
		out = append(out, strings.ReplaceAll(s, "台", "臺"))
	}
	if strings.HasPrefix(s, "高鐵") {
		// 「高鐵板橋站」 -> 「板橋高鐵站」
		base := strings.TrimPrefix(s, "高鐵")
		base = strings.TrimSuffix(base, "站")
		out = append(out, base+"高鐵站", base+"車站")
	}
	if strings.HasSuffix(s, "高鐵站") {
		base := strings.TrimSuffix(s, "高鐵站")
		out = append(out, base+"車站")
	}
	return out
}

// lookupInMockData scans the loaded eco-points for a POI whose name matches
// the query (exact or substring). Returns the first match's coordinates.
// Reuses the dataset path resolved by tools.go (SetDataPath / ECO_DATA_PATH).
func lookupInMockData(query string) (placeCoord, bool) {
	pts, err := LoadPOIs(dataPath)
	if err != nil {
		return placeCoord{}, false
	}
	// 先找精確比對
	for _, p := range pts {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		if p.Name == query {
			return placeCoord{Lat: *p.Lat, Lng: *p.Lng}, true
		}
	}
	// 再找 substring (查詢字串含於 POI 名稱)
	for _, p := range pts {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		if strings.Contains(p.Name, query) || strings.Contains(query, p.Name) {
			return placeCoord{Lat: *p.Lat, Lng: *p.Lng}, true
		}
	}
	return placeCoord{}, false
}

func mapboxForwardGeocode(name string) (placeCoord, error) {
	token := os.Getenv("MAPBOX_TOKEN")
	if token == "" {
		// 容器中可能沿用前端同名 env
		token = os.Getenv("VITE_MAPBOXTOKEN")
	}
	if token == "" {
		return placeCoord{}, fmt.Errorf("%w: MAPBOX_TOKEN not configured", ErrUnknownPlace)
	}
	q := url.PathEscape(name)
	// types=poi 優先商家/景點; proximity 偏向 雙北市中心，避免抓到外縣市同名店
	endpoint := fmt.Sprintf(
		"https://api.mapbox.com/geocoding/v5/mapbox.places/%s.json?country=tw&bbox=%s&proximity=%s&types=poi,address,place&language=zh-TW&limit=1&access_token=%s",
		q, taipeiBBox, taipeiProximity, url.QueryEscape(token),
	)
	resp, err := httpCli.Get(endpoint)
	if err != nil {
		return placeCoord{}, fmt.Errorf("%w: mapbox http: %v", ErrUnknownPlace, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return placeCoord{}, fmt.Errorf("%w: mapbox status %d", ErrUnknownPlace, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Features []struct {
			Center [2]float64 `json:"center"` // [lng, lat]
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return placeCoord{}, fmt.Errorf("%w: mapbox parse: %v", ErrUnknownPlace, err)
	}
	if len(data.Features) == 0 {
		return placeCoord{}, ErrUnknownPlace
	}
	return placeCoord{Lat: data.Features[0].Center[1], Lng: data.Features[0].Center[0]}, nil
}
