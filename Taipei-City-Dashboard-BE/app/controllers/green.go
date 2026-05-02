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
	"TaipeiCityDashboardBE/logs"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// upstreamFetchError 標記「DB 為空且外部 fetch 失敗」的錯誤;
// handler 看到此型別會回 502,其餘錯誤(DB 連線/查詢)回 500。
type upstreamFetchError struct{ err error }

func (e *upstreamFetchError) Error() string { return e.err.Error() }
func (e *upstreamFetchError) Unwrap() error { return e.err }

// /api/v1/green 群組:
//   - park / restaurant / hotel / walkpath / recycle:
//     優先讀 DBDashboard;若 DB 為空(例如 Airflow ETL 還沒跑過第一次),
//     觸發 fallback:從外部 API/CSV 抓資料、寫回 DB、回應給 client。
//     fallback 受 mutex 保護,並發進入時只會抓一次,後續請求會看到 DB 已有資料。
//   - ublike: 30 秒 TTL 即時資料,維持外部抓取 + in-memory cache,不寫 DB。

// === 外部資料源 URL ===

const (
	parksAPIURL            = "https://parks.gov.taipei/parks/api/"
	restaurantsAPIURL      = "https://data.moenv.gov.tw/api/v2/gis_p_11?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"
	hotelsAPIURL           = "https://data.moenv.gov.tw/api/v2/gp_p_43?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"
	walkpathsCSVURL        = "https://data.taipei/api/dataset/b5726297-d172-4ba7-b5c4-31de38e184e1/resource/0d1d7db3-efc1-40d1-ad24-5a1a1f88e06b/download"
	recycleTaipeiCSVURL    = "https://data.taipei/api/dataset/1acf38f3-1509-4cb1-898a-9b1d4f31a3af/resource/0263f0ce-403a-45ed-a407-c69285b6cad2/download"
	recycleNewTaipeiCSVURL = "https://data.ntpc.gov.tw/api/datasets/a381e1f4-86d0-4575-adb4-8d9b6a75e3c4/csv/file"
	ubikeCSVURL            = "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"
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

func parseTaiwanBool(s string) bool { return cleanField(s) == "是" }

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
	walkpathsFallbackMutex   sync.Mutex
	recyclesFallbackMutex    sync.Mutex
)

// === Parks fetcher ===

func fetchParksFromAPI() ([]models.GreenPark, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(parksAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parks API returned status %d", resp.StatusCode)
	}

	parks := make([]models.GreenPark, 0)
	if err := json.NewDecoder(resp.Body).Decode(&parks); err != nil {
		return nil, err
	}
	return parks, nil
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

// === Walkpaths fetcher ===

func fetchWalkpathsFromAPI() ([]models.GreenWalkpath, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(walkpathsCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("walkpaths API returned status %d", resp.StatusCode)
	}

	// 上游 CSV 是 Big5 編碼,先解碼成 UTF-8 再交給 csv.Reader
	utf8Reader := transform.NewReader(resp.Body, traditionalchinese.Big5.NewDecoder())
	csvReader := csv.NewReader(utf8Reader)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	walkpaths := make([]models.GreenWalkpath, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 22 {
			continue
		}
		serial := parseIntSafe(row[0])
		if serial == 0 {
			continue
		}
		walkpaths = append(walkpaths, models.GreenWalkpath{
			SerialNumber:       serial,
			District:           cleanField(row[1]),
			Route:              cleanField(row[2]),
			TotalLengthM:       parseIntSafe(row[3]),
			OneWayMinutes:      parseIntSafe(row[4]),
			Grade:              cleanField(row[5]),
			StartPoint:         cleanField(row[6]),
			StartLongitude:     parseFloatSafe(row[7]),
			StartLatitude:      parseFloatSafe(row[8]),
			StartIsStairs:      parseTaiwanBool(row[9]),
			EndPoint:           cleanField(row[10]),
			EndLongitude:       parseFloatSafe(row[11]),
			EndLatitude:        parseFloatSafe(row[12]),
			EndIsStairs:        parseTaiwanBool(row[13]),
			HasTrailGate:       parseTaiwanBool(row[14]),
			WheelchairFriendly: parseTaiwanBool(row[15]),
			WheelchairSlope:    cleanField(row[16]),
			WheelchairLengthM:  parseIntSafe(row[17]),
			MobileSignal:       cleanField(row[18]),
			HasMobileToilet:    parseTaiwanBool(row[19]),
			ToiletLocation:     cleanField(row[20]),
			AccessibleToilet:   parseTaiwanBool(row[21]),
		})
	}
	return walkpaths, nil
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

	utf8Reader := transform.NewReader(resp.Body, traditionalchinese.Big5.NewDecoder())
	csvReader := csv.NewReader(utf8Reader)
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

func ensureWalkpathsData() ([]models.GreenWalkpath, error) {
	rows, err := models.GetAllGreenWalkpaths()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	walkpathsFallbackMutex.Lock()
	defer walkpathsFallbackMutex.Unlock()

	rows, err = models.GetAllGreenWalkpaths()
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	fetched, err := fetchWalkpathsFromAPI()
	if err != nil {
		return nil, &upstreamFetchError{err}
	}
	if saveErr := models.SaveGreenWalkpaths(fetched); saveErr != nil {
		logs.FError("SaveGreenWalkpaths after fallback fetch failed: %v", saveErr)
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(parks), "data": parks})
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(restaurants), "data": restaurants})
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(hotels), "data": hotels})
}

/*
ListWalkpaths 從 DBDashboard 讀取登山步道;若 DB 為空則 fallback 到 data.taipei CSV
GET /api/v1/green/walkpath
*/
func ListWalkpaths(c *gin.Context) {
	walkpaths, err := ensureWalkpathsData()
	if err != nil {
		handleGreenError(c, "ListWalkpaths", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(walkpaths), "data": walkpaths})
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(points), "data": points})
}

// === ublike (即時資料,維持原邏輯) ===

// UbikeStation 對應新北市 YouBike2.0 站點即時狀態的單筆紀錄
type UbikeStation struct {
	Sno            int     `json:"sno"`
	Name           string  `json:"name"`
	NameEn         string  `json:"name_en"`
	City           string  `json:"city"`
	CityEn         string  `json:"city_en"`
	Area           string  `json:"area"`
	AreaEn         string  `json:"area_en"`
	Address        string  `json:"address"`
	AddressEn      string  `json:"address_en"`
	TotalQuantity  int     `json:"total_quantity"`
	AvailableBikes int     `json:"available_bikes"`
	AvailableSpots int     `json:"available_spots"`
	Yb2Quantity    int     `json:"yb2_quantity"`
	EybQuantity    int     `json:"eyb_quantity"`
	Active         bool    `json:"active"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	UpdatedAt      string  `json:"updated_at"`
}

var (
	ubikeCache      []UbikeStation
	ubikeCacheTime  time.Time
	ubikeCacheMutex sync.Mutex
	ubikeTzOnce     sync.Once
	ubikeTz         *time.Location
)

const ubikeCacheTTL = 30 * time.Second

// parseUbikeMday 將 "20060102T150405" 字串以台北時區解析為 RFC3339,失敗時回傳原字串
func parseUbikeMday(s string) string {
	s = cleanField(s)
	if s == "" {
		return ""
	}
	ubikeTzOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Taipei")
		if err != nil {
			loc = time.FixedZone("CST", 8*3600)
		}
		ubikeTz = loc
	})
	t, err := time.ParseInLocation("20060102T150405", s, ubikeTz)
	if err != nil {
		return s
	}
	return t.Format(time.RFC3339)
}

func fetchUbikeStations() ([]UbikeStation, error) {
	ubikeCacheMutex.Lock()
	defer ubikeCacheMutex.Unlock()

	if ubikeCache != nil && time.Since(ubikeCacheTime) < ubikeCacheTTL {
		return ubikeCache, nil
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(ubikeCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ubike API returned status %d", resp.StatusCode)
	}

	csvReader := csv.NewReader(resp.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	stations := make([]UbikeStation, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 18 {
			continue
		}
		sno := parseIntSafe(row[8])
		if sno == 0 {
			continue
		}
		stations = append(stations, UbikeStation{
			Sno:            sno,
			City:           cleanField(row[0]),
			CityEn:         cleanField(row[1]),
			Name:           cleanField(row[2]),
			Area:           cleanField(row[3]),
			Address:        cleanField(row[4]),
			NameEn:         cleanField(row[5]),
			AreaEn:         cleanField(row[6]),
			AddressEn:      cleanField(row[7]),
			TotalQuantity:  parseIntSafe(row[9]),
			AvailableBikes: parseIntSafe(row[10]),
			UpdatedAt:      parseUbikeMday(row[11]),
			Latitude:       parseFloatSafe(row[12]),
			Longitude:      parseFloatSafe(row[13]),
			AvailableSpots: parseIntSafe(row[14]),
			Active:         parseIntSafe(row[15]) == 1,
			Yb2Quantity:    parseIntSafe(row[16]),
			EybQuantity:    parseIntSafe(row[17]),
		})
	}

	ubikeCache = stations
	ubikeCacheTime = time.Now()
	return stations, nil
}

/*
ListUblikes 從 data.ntpc.gov.tw 取得新北市 YouBike2.0 站點即時資料(30 秒 cache)
GET /api/v1/green/ublike
*/
func ListUblikes(c *gin.Context) {
	stations, err := fetchUbikeStations()
	if err != nil {
		logs.FError("ListUblikes upstream fetch failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": "upstream data source unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(stations), "data": stations})
}
