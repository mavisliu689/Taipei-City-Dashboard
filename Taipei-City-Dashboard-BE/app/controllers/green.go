package controllers

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/app/services/geocode"
	"TaipeiCityDashboardBE/logs"

	"github.com/gin-gonic/gin"
)

// upstreamFetchError 標記「DB 為空且外部 fetch 失敗」的錯誤;
// handler 看到此型別會回 502,其餘錯誤(DB 連線/查詢)回 500。
type upstreamFetchError struct{ err error }

func (e *upstreamFetchError) Error() string { return e.err.Error() }
func (e *upstreamFetchError) Unwrap() error { return e.err }

// /api/v1/green 群組:
//   - park / restaurant / hotel / recycle:
//     優先讀 DBDashboard;若 DB 為空(例如 Airflow ETL 還沒跑過第一次),
//     觸發 fallback:從外部 API/CSV 抓資料、寫回 DB、回應給 client。
//     fallback 受 mutex 保護,並發進入時只會抓一次,後續請求會看到 DB 已有資料。
//   - ubike: 30 秒 TTL 即時資料,維持外部抓取 + in-memory cache,不寫 DB。

// === 外部資料源 URL ===

const (
	parksAPIURL              = "https://parks.gov.taipei/parks/api/"
	parksNtpcRiversideCSVURL = "https://data.ntpc.gov.tw/api/datasets/c3867812-6188-4b0a-a487-03bb4d93238d/csv"
	restaurantsAPIURL        = "https://data.moenv.gov.tw/api/v2/gis_p_11?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"
	hotelsAPIURL             = "https://data.moenv.gov.tw/api/v2/gp_p_43?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"
	recycleTaipeiCSVURL      = "https://data.taipei/api/dataset/1acf38f3-1509-4cb1-898a-9b1d4f31a3af/resource/0263f0ce-403a-45ed-a407-c69285b6cad2/download"
	recycleNewTaipeiCSVURL   = "https://data.ntpc.gov.tw/api/datasets/a381e1f4-86d0-4575-adb4-8d9b6a75e3c4/csv/file"
	ubikeCSVURL              = "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"
	ubikeTaipeiJSONURL       = "https://tcgbusfs.blob.core.windows.net/dotapp/youbike/v2/youbike_immediate.json"
)

// bomChar 用 rune 構造,避免在原始碼中出現會讓 Go 編譯器拒絕的中段 BOM bytes
var bomChar = string(rune(0xFEFF))

// === 共用工具 ===

// cleanField 收斂前後空白與內嵌換行,合併連續空白,並去除 UTF-8 BOM
func cleanField(s string) string {
	s = strings.TrimPrefix(s, bomChar)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func parseIntSafe(s string) int {
	n, _ := strconv.Atoi(cleanField(s))
	return n
}

func parseFloatSafe(s string) float64 {
	f, _ := strconv.ParseFloat(cleanField(s), 64)
	return f
}

// === Fallback fetch mutexes (per dataset) ===

var (
	parksFallbackMutex       sync.Mutex
	restaurantsFallbackMutex sync.Mutex
	hotelsFallbackMutex      sync.Mutex
	recyclesFallbackMutex    sync.Mutex
)

// === Parks fetchers (台北市 JSON + 新北市 河濱 + 新北市 鄰里) ===

// fetchParksTaipei 從台北市公園處 JSON 抓全公園資料(25 欄 metadata)。
func fetchParksTaipei() ([]models.GreenPark, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(parksAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parks TPE API returned status %d", resp.StatusCode)
	}

	parks := make([]models.GreenPark, 0)
	if err := json.NewDecoder(resp.Body).Decode(&parks); err != nil {
		return nil, err
	}
	for i := range parks {
		parks[i].City = "臺北市"
	}
	return parks, nil
}

// fetchParksNtpcRiverside 從新北市開放資料平台抓河濱公園 CSV(3 欄: name, longitude, latitude)。
func fetchParksNtpcRiverside() ([]models.GreenPark, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(parksNtpcRiversideCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parks NTPC riverside CSV returned status %d", resp.StatusCode)
	}

	csvReader := csv.NewReader(resp.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	parks := make([]models.GreenPark, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 3 {
			continue
		}
		name := cleanField(row[0])
		if name == "" {
			continue
		}
		parks = append(parks, models.GreenPark{
			Name:      name,
			Longitude: cleanField(row[1]),
			Latitude:  cleanField(row[2]),
			City:      "新北市",
		})
	}
	return parks, nil
}

// fetchParksFromAPI 並行抓兩個來源、合併。
// 容忍部分失敗:任一邊成功仍回該邊資料(失敗的那邊只 log);兩邊都失敗才回 error。
func fetchParksFromAPI() ([]models.GreenPark, error) {
	type result struct {
		label string
		parks []models.GreenPark
		err   error
	}
	ch := make(chan result, 2)
	go func() { p, e := fetchParksTaipei(); ch <- result{"TPE", p, e} }()
	go func() { p, e := fetchParksNtpcRiverside(); ch <- result{"NTPC-River", p, e} }()

	combined := make([]models.GreenPark, 0)
	var lastErr error
	gotAny := false
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			lastErr = r.err
			logs.FError("parks %s fetch failed: %v", r.label, r.err)
			continue
		}
		gotAny = true
		combined = append(combined, r.parks...)
	}
	if !gotAny {
		return nil, lastErr
	}
	return combined, nil
}

// === Restaurants fetcher ===

type restaurantsXMLWrap struct {
	XMLName xml.Name                 `xml:"gis_p_11"`
	Data    []models.GreenRestaurant `xml:"data"`
}

// isTaipeiOrNewTaipei 判斷 city 字串是否屬於台北市或新北市,
// 容忍前後空白與「臺/台」的字形差異。
func isTaipeiOrNewTaipei(city string) bool {
	s := strings.TrimSpace(city)
	return s == "臺北市" || s == "台北市" || s == "新北市"
}

func fetchRestaurantsFromAPI() ([]models.GreenRestaurant, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(restaurantsAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("restaurants API returned status %d", resp.StatusCode)
	}

	var parsed restaurantsXMLWrap
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	// 上游含全國資料,只保留台北市/新北市
	filtered := make([]models.GreenRestaurant, 0, len(parsed.Data))
	for _, r := range parsed.Data {
		if isTaipeiOrNewTaipei(r.City) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// === Hotels fetcher ===

type hotelsXMLWrap struct {
	XMLName xml.Name            `xml:"gp_p_43"`
	Data    []models.GreenHotel `xml:"data"`
}

func fetchHotelsFromAPI() ([]models.GreenHotel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(hotelsAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hotels API returned status %d", resp.StatusCode)
	}

	var parsed hotelsXMLWrap
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	// 上游含全國資料,只保留台北市/新北市(此資料集縣市欄位叫 county)
	filtered := make([]models.GreenHotel, 0, len(parsed.Data))
	for _, h := range parsed.Data {
		if isTaipeiOrNewTaipei(h.County) {
			filtered = append(filtered, h)
		}
	}
	return filtered, nil
}

// === Recycles fetchers (兩個 CSV 合併) ===

func fetchRecycleTaipei() ([]models.GreenRecycle, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(recycleTaipeiCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recycle TPE returned status %d", resp.StatusCode)
	}

	// 注意: 台北市 recycle CSV 原始為 Big5; 此 fallback 路徑僅在 DB 為空時觸發,
	// 正式運作以 DE pipeline (green_recycles.py, 已處理編碼) 灌入 DB 為主。
	// 若 fresh DB + 觸發此 fallback, 中文欄位會 mojibake — 屬可接受 trade-off。
	csvReader := csv.NewReader(resp.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	points := make([]models.GreenRecycle, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 7 {
			continue
		}
		district := cleanField(row[0])
		if district == "" {
			continue
		}
		points = append(points, models.GreenRecycle{
			Source:    "taipei",
			City:      "臺北市",
			District:  district,
			Name:      cleanField(row[1]),
			Phone:     cleanField(row[2]),
			Address:   cleanField(row[3]),
			OpenTime:  cleanField(row[4]),
			Longitude: parseFloatSafe(row[5]),
			Latitude:  parseFloatSafe(row[6]),
		})
	}
	return points, nil
}

func fetchRecycleNewTaipei() ([]models.GreenRecycle, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(recycleNewTaipeiCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recycle NTPC returned status %d", resp.StatusCode)
	}

	csvReader := csv.NewReader(resp.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	points := make([]models.GreenRecycle, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 12 {
			continue
		}
		seq := cleanField(row[0])
		if seq == "" {
			continue
		}
		// recycle_address(row[9])為實際回收地點優先,空字串時回退至 address(row[5],住家)
		address := cleanField(row[9])
		if address == "" {
			address = cleanField(row[5])
		}
		// 將分機合併到電話欄位
		phone := cleanField(row[6])
		if ext := cleanField(row[7]); ext != "" {
			phone = phone + "#" + ext
		}
		points = append(points, models.GreenRecycle{
			Source:   "new_taipei",
			City:     "新北市",
			District: cleanField(row[1]),
			Village:  cleanField(row[2]),
			Code:     cleanField(row[3]),
			Name:     cleanField(row[4]),
			Address:  address,
			Phone:    phone,
			Mobile:   cleanField(row[8]),
			OpenTime: cleanField(row[10]),
			State:    cleanField(row[11]),
		})
	}

	// 上游無經緯度 → 用 Mapbox forward geocoding 補上(token 缺則跳過,維持 0/0)。
	// 主路徑由 DE DAG 寫好座標進 DB,此處是 fallback 也能自洽。
	if token := geocode.Token(); token != "" && len(points) > 0 {
		addrs := make([]string, len(points))
		for i, p := range points {
			addrs[i] = p.Address
		}
		coords := geocode.BatchGeocode(addrs, token)
		ok := 0
		for i, c := range coords {
			if c.Longitude != 0 || c.Latitude != 0 {
				points[i].Longitude = c.Longitude
				points[i].Latitude = c.Latitude
				ok++
			}
		}
		logs.FInfo("[recycle NTPC] mapbox geocoded %d/%d", ok, len(points))
	}

	return points, nil
}

func fetchRecyclesFromAPI() ([]models.GreenRecycle, error) {
	type result struct {
		points []models.GreenRecycle
		err    error
	}
	ch := make(chan result, 2)
	go func() { p, e := fetchRecycleTaipei(); ch <- result{p, e} }()
	go func() { p, e := fetchRecycleNewTaipei(); ch <- result{p, e} }()

	combined := make([]models.GreenRecycle, 0, 400)
	var firstErr error
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		combined = append(combined, r.points...)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return combined, nil
}

// === Ensure helpers (DB 優先,空則 fallback fetch + save) ===
//
// 模式為「double-checked locking」:先讀 DB,若空再進 mutex,進去後再讀一次 DB
// (避免 mutex 等待期間另一個 goroutine 已經寫好);仍空才實際 fetch+save。
// fetch 失敗會回 error 由 handler 轉 502;save 失敗只 log warning,response 仍正常回。

func ensureParksData() ([]models.GreenPark, error) {
	rows, err := models.GetAllGreenParks()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	parksFallbackMutex.Lock()
	defer parksFallbackMutex.Unlock()

	rows, err = models.GetAllGreenParks()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchParksFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenParks(fetched); saveErr != nil {
		logs.FError("SaveGreenParks after fallback fetch failed: %v", saveErr)
	}
	return fetched, nil
}

func ensureRestaurantsData() ([]models.GreenRestaurant, error) {
	rows, err := models.GetAllGreenRestaurants()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	restaurantsFallbackMutex.Lock()
	defer restaurantsFallbackMutex.Unlock()

	rows, err = models.GetAllGreenRestaurants()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchRestaurantsFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenRestaurants(fetched); saveErr != nil {
		logs.FError("SaveGreenRestaurants after fallback fetch failed: %v", saveErr)
	}
	return fetched, nil
}

func ensureHotelsData() ([]models.GreenHotel, error) {
	rows, err := models.GetAllGreenHotels()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	hotelsFallbackMutex.Lock()
	defer hotelsFallbackMutex.Unlock()

	rows, err = models.GetAllGreenHotels()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchHotelsFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenHotels(fetched); saveErr != nil {
		logs.FError("SaveGreenHotels after fallback fetch failed: %v", saveErr)
	}
	return fetched, nil
}

func ensureRecyclesData() ([]models.GreenRecycle, error) {
	rows, err := models.GetAllGreenRecycles()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	recyclesFallbackMutex.Lock()
	defer recyclesFallbackMutex.Unlock()

	rows, err = models.GetAllGreenRecycles()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchRecyclesFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenRecycles(fetched); saveErr != nil {
		logs.FError("SaveGreenRecycles after fallback fetch failed: %v", saveErr)
	}
	return fetched, nil
}

// === Handlers ===
//
// 三種失敗類型:
//   - DB 連線/查詢錯誤      -> 500 internal error
//   - DB 空且外部 API 失敗  -> 502 upstream error(此資料來源仍未可用)
//   - 兩者皆成功            -> 200 + data
// 所有錯誤細節只進 log,對外只給通用訊息。

// parseCityFilter 讀取 query string 的 "city" 參數,正規化成三選一:
//   - "origin" => 台北市
//   - "new"    => 新北市
//   - "both" / 其他 / 空 => 雙北(預設)
func parseCityFilter(c *gin.Context) string {
	switch strings.ToLower(strings.TrimSpace(c.Query("city"))) {
	case "origin":
		return "origin"
	case "new":
		return "new"
	default:
		return "both"
	}
}

// matchesCityFilter 判斷 city 欄位字串是否符合 filter,容忍「臺/台」字形差異與前後空白。
// 給有 city/county 欄位的資料集(restaurant/hotel/recycle/ubike)使用。
func matchesCityFilter(cityField, filter string) bool {
	switch filter {
	case "origin":
		s := strings.TrimSpace(cityField)
		return s == "臺北市" || s == "台北市"
	case "new":
		return strings.TrimSpace(cityField) == "新北市"
	default: // both
		return true
	}
}

func handleGreenError(c *gin.Context, handlerName string, err error) {
	logs.FError("%s failed: %v", handlerName, err)
	var ufe *upstreamFetchError
	if errors.As(err, &ufe) {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "upstream data source unavailable"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "internal error"})
}

/*
ListParks 從 DBDashboard 讀取公園資料;若 DB 為空則 fallback 到 parks.gov.taipei API
GET /api/v1/green/park
*/
func ListParks(c *gin.Context) {
	parks, err := ensureParksData()
	if err != nil {
		handleGreenError(c, "ListParks", err)
		return
	}
	filter := parseCityFilter(c)
	filtered := make([]models.GreenPark, 0, len(parks))
	for _, p := range parks {
		if matchesCityFilter(p.City, filter) {
			filtered = append(filtered, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(filtered), "data": filtered})
}

/*
ListRestaurants 從 DBDashboard 讀取環保餐廳;若 DB 為空則 fallback 到 MOENV gis_p_11
GET /api/v1/green/restaurant
*/
func ListRestaurants(c *gin.Context) {
	restaurants, err := ensureRestaurantsData()
	if err != nil {
		handleGreenError(c, "ListRestaurants", err)
		return
	}
	filter := parseCityFilter(c)
	filtered := make([]models.GreenRestaurant, 0, len(restaurants))
	for _, r := range restaurants {
		if matchesCityFilter(r.City, filter) {
			filtered = append(filtered, r)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(filtered), "data": filtered})
}

/*
ListHotels 從 DBDashboard 讀取環保旅宿;若 DB 為空則 fallback 到 MOENV gp_p_43
GET /api/v1/green/hotel
*/
func ListHotels(c *gin.Context) {
	hotels, err := ensureHotelsData()
	if err != nil {
		handleGreenError(c, "ListHotels", err)
		return
	}
	// hotel 資料集縣市欄位叫 county
	filter := parseCityFilter(c)
	filtered := make([]models.GreenHotel, 0, len(hotels))
	for _, h := range hotels {
		if matchesCityFilter(h.County, filter) {
			filtered = append(filtered, h)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(filtered), "data": filtered})
}

/*
ListRecycles 從 DBDashboard 讀取回收據點;若 DB 為空則 fallback 到雙北 CSV
GET /api/v1/green/recycle
*/
func ListRecycles(c *gin.Context) {
	points, err := ensureRecyclesData()
	if err != nil {
		handleGreenError(c, "ListRecycles", err)
		return
	}
	filter := parseCityFilter(c)
	filtered := make([]models.GreenRecycle, 0, len(points))
	for _, p := range points {
		if matchesCityFilter(p.City, filter) {
			filtered = append(filtered, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(filtered), "data": filtered})
}

// === ubike (DB-first + fallback fetch) ===
//
// ubike 跟其他 5 個 green endpoint 一致:優先讀 DB,DB 空才 fallback 抓兩個外部資料源
// (台北市 JSON + 新北市 CSV),寫回 DB 並回應。30 秒 in-memory cache 不再需要(DB 直讀夠快)。
//
// 寫入端用 active flag 過濾掉停用站點(active != "1" / 1 的不存)。
// schema 不含即時欄位(available_bikes/spots/mday/yb2/eyb/total_quantity)。

// taipeiUbikeRecord 對應台北市 YouBike2.0 JSON 的單筆紀錄(欄位皆為 string)
type taipeiUbikeRecord struct {
	Sno       string  `json:"sno"`
	Sna       string  `json:"sna"`
	Sarea     string  `json:"sarea"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Ar        string  `json:"ar"`
	Sareaen   string  `json:"sareaen"`
	Snaen     string  `json:"snaen"`
	Aren      string  `json:"aren"`
	Act       string  `json:"act"`
}

var ubikesFallbackMutex sync.Mutex

// fetchUbikeNewTaipei 從新北市開放資料平台 CSV 抓 YouBike2.0 站點(僅保留 active=1)。
func fetchUbikeNewTaipei() ([]models.GreenUbike, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(ubikeCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ubike NTPC API returned status %d", resp.StatusCode)
	}

	csvReader := csv.NewReader(resp.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	stations := make([]models.GreenUbike, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 18 {
			continue
		}
		sno := parseIntSafe(row[8])
		if sno == 0 {
			continue
		}
		// active 在 row[15],值為 "1" 表示啟用;非啟用直接過濾
		if parseIntSafe(row[15]) != 1 {
			continue
		}
		stations = append(stations, models.GreenUbike{
			Sno:       sno,
			City:      cleanField(row[0]),
			CityEn:    cleanField(row[1]),
			Name:      cleanField(row[2]),
			Area:      cleanField(row[3]),
			Address:   cleanField(row[4]),
			NameEn:    cleanField(row[5]),
			AreaEn:    cleanField(row[6]),
			AddressEn: cleanField(row[7]),
			Latitude:  parseFloatSafe(row[12]),
			Longitude: parseFloatSafe(row[13]),
		})
	}
	return stations, nil
}

// fetchUbikeTaipei 從臺北市政府 tcgbusfs blob 抓 YouBike2.0 JSON(僅保留 act=1)。
func fetchUbikeTaipei() ([]models.GreenUbike, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(ubikeTaipeiJSONURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ubike TPE API returned status %d", resp.StatusCode)
	}

	var records []taipeiUbikeRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	stations := make([]models.GreenUbike, 0, len(records))
	for _, r := range records {
		sno := parseIntSafe(r.Sno)
		if sno == 0 {
			continue
		}
		// act 在 JSON 中為 string "1" 表示啟用
		if parseIntSafe(r.Act) != 1 {
			continue
		}
		stations = append(stations, models.GreenUbike{
			Sno:       sno,
			City:      "臺北市",
			CityEn:    "Taipei City",
			Name:      cleanField(r.Sna),
			NameEn:    cleanField(r.Snaen),
			Area:      cleanField(r.Sarea),
			AreaEn:    cleanField(r.Sareaen),
			Address:   cleanField(r.Ar),
			AddressEn: cleanField(r.Aren),
			Latitude:  r.Latitude,
			Longitude: r.Longitude,
		})
	}
	return stations, nil
}

// fetchUbikesFromAPI 並行抓台北市 + 新北市,合併後回傳。
// 容忍部分失敗:有任一邊成功仍回該邊資料(失敗的那邊只 log);兩邊都失敗才回 error。
func fetchUbikesFromAPI() ([]models.GreenUbike, error) {
	type result struct {
		label    string
		stations []models.GreenUbike
		err      error
	}
	ch := make(chan result, 2)
	go func() { s, e := fetchUbikeNewTaipei(); ch <- result{"NTPC", s, e} }()
	go func() { s, e := fetchUbikeTaipei(); ch <- result{"TPE", s, e} }()

	combined := make([]models.GreenUbike, 0, 3000)
	var lastErr error
	gotAny := false
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			lastErr = r.err
			logs.FError("ubike %s fetch failed: %v", r.label, r.err)
			continue
		}
		gotAny = true
		combined = append(combined, r.stations...)
	}
	if !gotAny {
		return nil, lastErr
	}
	return combined, nil
}

// ensureUbikesData DB 優先,空則 fallback fetch + save(double-checked locking)。
func ensureUbikesData() ([]models.GreenUbike, error) {
	rows, err := models.GetAllGreenUbikes()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	ubikesFallbackMutex.Lock()
	defer ubikesFallbackMutex.Unlock()

	rows, err = models.GetAllGreenUbikes()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchUbikesFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenUbikes(fetched); saveErr != nil {
		logs.FError("SaveGreenUbikes after fallback fetch failed: %v", saveErr)
	}
	return fetched, nil
}

/*
ListUbikes 從 DBDashboard 讀取 YouBike2.0 站點目錄(台北市 + 新北市,僅 active);
若 DB 為空則 fallback 到外部 API(台北 JSON + 新北 CSV)
GET /api/v1/green/ubike
*/
func ListUbikes(c *gin.Context) {
	rows, err := ensureUbikesData()
	if err != nil {
		handleGreenError(c, "Listubikes", err)
		return
	}
	filter := parseCityFilter(c)
	filtered := make([]models.GreenUbike, 0, len(rows))
	for _, r := range rows {
		if matchesCityFilter(r.City, filter) {
			filtered = append(filtered, r)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(filtered), "data": filtered})
}
