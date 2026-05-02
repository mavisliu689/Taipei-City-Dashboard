package controllers

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

const parksAPIURL = "https://parks.gov.taipei/parks/api/"

const restaurantsAPIURL = "https://data.moenv.gov.tw/api/v2/gis_p_11?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"

const hotelsAPIURL = "https://data.moenv.gov.tw/api/v2/gp_p_43?api_key=e75b1660-e564-4107-aad5-a8be1f905dd9&limit=1000&sort=ImportDate%20desc&format=XML"

const walkpathsCSVURL = "https://data.taipei/api/dataset/b5726297-d172-4ba7-b5c4-31de38e184e1/resource/0d1d7db3-efc1-40d1-ad24-5a1a1f88e06b/download"

const (
	recycleTaipeiCSVURL    = "https://data.taipei/api/dataset/1acf38f3-1509-4cb1-898a-9b1d4f31a3af/resource/0263f0ce-403a-45ed-a407-c69285b6cad2/download"
	recycleNewTaipeiCSVURL = "https://data.ntpc.gov.tw/api/datasets/a381e1f4-86d0-4575-adb4-8d9b6a75e3c4/csv/file"
)

const ubikeCSVURL = "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"

// Park 對應 parks.gov.taipei 公園資料的單筆紀錄
type Park struct {
	SeqNo          string `json:"SeqNo"`
	Name           string `json:"pm_name"`
	NameEng        string `json:"pm_name_eng"`
	Overview       string `json:"pm_overview"`
	Longitude      string `json:"pm_Longitude"`
	Latitude       string `json:"pm_Latitude"`
	Unit           string `json:"pm_unit"`
	ConstYear      string `json:"pm_const_year"`
	Location       string `json:"pm_location"`
	LandPublicArea string `json:"pm_LandPublicArea"`
	OpeningStart   string `json:"pm_opening_s"`
	OpeningEnd     string `json:"pm_opening_e"`
	Libie          string `json:"pm_libie"`
	Phone          string `json:"pm_phone"`
	Sports         string `json:"pm_sports"`
	Recreation     string `json:"pm_recreation"`
	Service        string `json:"pm_service"`
	Other          string `json:"pm_other"`
	Transit        string `json:"pm_transit"`
	Ecology        string `json:"pm_ecology"`
	Type           string `json:"pm_type"`
	PlayType       string `json:"pm_playtype"`
	PlayArea       string `json:"pm_playarea"`
	Description    string `json:"pm_description"`
	PlayEquipment  string `json:"pm_playeq"`
}

var (
	parksCache      []Park
	parksCacheTime  time.Time
	parksCacheMutex sync.Mutex
)

const parksCacheTTL = 10 * time.Minute

func fetchParks() ([]Park, error) {
	parksCacheMutex.Lock()
	defer parksCacheMutex.Unlock()

	if parksCache != nil && time.Since(parksCacheTime) < parksCacheTTL {
		return parksCache, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(parksAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parks API returned status %d", resp.StatusCode)
	}

	var parks []Park
	if err := json.NewDecoder(resp.Body).Decode(&parks); err != nil {
		return nil, err
	}

	parksCache = parks
	parksCacheTime = time.Now()
	return parks, nil
}

/*
ListParks 從台北市政府公園管理處取得全部公園資料
GET /api/v1/green/park
*/
func ListParks(c *gin.Context) {
	parks, err := fetchParks()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(parks), "data": parks})
}

// Restaurant 對應環境部 gis_p_11 資料集的單筆環保餐廳紀錄
type Restaurant struct {
	RestID    string `xml:"restid" json:"restid"`
	Name      string `xml:"name" json:"name"`
	Address   string `xml:"address" json:"address"`
	Phone     string `xml:"phone" json:"phone"`
	Mobile    string `xml:"mobile" json:"mobile"`
	Latitude  string `xml:"latitude" json:"latitude"`
	Longitude string `xml:"longitude" json:"longitude"`
	City      string `xml:"city" json:"city"`
}

type restaurantsXML struct {
	XMLName xml.Name     `xml:"gis_p_11"`
	Data    []Restaurant `xml:"data"`
}

var (
	restaurantsCache      []Restaurant
	restaurantsCacheTime  time.Time
	restaurantsCacheMutex sync.Mutex
)

const restaurantsCacheTTL = 30 * time.Minute

func fetchRestaurants() ([]Restaurant, error) {
	restaurantsCacheMutex.Lock()
	defer restaurantsCacheMutex.Unlock()

	if restaurantsCache != nil && time.Since(restaurantsCacheTime) < restaurantsCacheTTL {
		return restaurantsCache, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(restaurantsAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("restaurants API returned status %d", resp.StatusCode)
	}

	var parsed restaurantsXML
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	restaurantsCache = parsed.Data
	restaurantsCacheTime = time.Now()
	return parsed.Data, nil
}

/*
ListRestaurants 從環境部 gis_p_11 資料集取得環保餐廳清單
GET /api/v1/green/restaurant
*/
func ListRestaurants(c *gin.Context) {
	restaurants, err := fetchRestaurants()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(restaurants), "data": restaurants})
}
// Hotel 對應環境部 gp_p_43 環保旅宿資料集的單筆紀錄
type Hotel struct {
	SerialNumber string `xml:"serialnumber" json:"serialnumber"`
	Name         string `xml:"name" json:"name"`
	Address      string `xml:"address" json:"address"`
	Phone        string `xml:"phone" json:"phone"`
	Latitude     string `xml:"latitude" json:"latitude"`
	Longitude    string `xml:"longitude" json:"longitude"`
	Note         string `xml:"note" json:"note"`
	County       string `xml:"county" json:"county"`
	Town         string `xml:"town" json:"town"`
	Village      string `xml:"village" json:"village"`
}

type hotelsXML struct {
	XMLName xml.Name `xml:"gp_p_43"`
	Data    []Hotel  `xml:"data"`
}

var (
	hotelsCache      []Hotel
	hotelsCacheTime  time.Time
	hotelsCacheMutex sync.Mutex
)

const hotelsCacheTTL = 30 * time.Minute

func fetchHotels() ([]Hotel, error) {
	hotelsCacheMutex.Lock()
	defer hotelsCacheMutex.Unlock()

	if hotelsCache != nil && time.Since(hotelsCacheTime) < hotelsCacheTTL {
		return hotelsCache, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(hotelsAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hotels API returned status %d", resp.StatusCode)
	}

	var parsed hotelsXML
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	hotelsCache = parsed.Data
	hotelsCacheTime = time.Now()
	return parsed.Data, nil
}

/*
ListHotels 從環境部 gp_p_43 資料集取得環保旅宿清單
GET /api/v1/green/hotel
*/
func ListHotels(c *gin.Context) {
	hotels, err := fetchHotels()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(hotels), "data": hotels})
}
// Walkpath 對應「臺北市登山步道」CSV 資料集的單筆步道紀錄
type Walkpath struct {
	SerialNumber       int     `json:"serial_number"`
	District           string  `json:"district"`
	Route              string  `json:"route"`
	TotalLengthM       int     `json:"total_length_m"`
	OneWayMinutes      int     `json:"one_way_minutes"`
	Grade              string  `json:"grade"`
	StartPoint         string  `json:"start_point"`
	StartLongitude     float64 `json:"start_longitude"`
	StartLatitude      float64 `json:"start_latitude"`
	StartIsStairs      bool    `json:"start_is_stairs"`
	EndPoint           string  `json:"end_point"`
	EndLongitude       float64 `json:"end_longitude"`
	EndLatitude        float64 `json:"end_latitude"`
	EndIsStairs        bool    `json:"end_is_stairs"`
	HasTrailGate       bool    `json:"has_trail_gate"`
	WheelchairFriendly bool    `json:"wheelchair_friendly"`
	WheelchairSlope    string  `json:"wheelchair_slope"`
	WheelchairLengthM  int     `json:"wheelchair_length_m"`
	MobileSignal       string  `json:"mobile_signal"`
	HasMobileToilet    bool    `json:"has_mobile_toilet"`
	ToiletLocation     string  `json:"toilet_location"`
	AccessibleToilet   bool    `json:"accessible_toilet"`
}

var (
	walkpathsCache      []Walkpath
	walkpathsCacheTime  time.Time
	walkpathsCacheMutex sync.Mutex
)

const walkpathsCacheTTL = 1 * time.Hour

// cleanField 收斂前後空白與內嵌換行,合併連續空白,並去除 UTF-8 BOM
func cleanField(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
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

func fetchWalkpaths() ([]Walkpath, error) {
	walkpathsCacheMutex.Lock()
	defer walkpathsCacheMutex.Unlock()

	if walkpathsCache != nil && time.Since(walkpathsCacheTime) < walkpathsCacheTTL {
		return walkpathsCache, nil
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(walkpathsCSVURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("walkpaths API returned status %d", resp.StatusCode)
	}

	// 上游 CSV 是 Big5 編碼,需先解碼成 UTF-8 再交給 csv.Reader
	utf8Reader := transform.NewReader(resp.Body, traditionalchinese.Big5.NewDecoder())

	csvReader := csv.NewReader(utf8Reader)
	csvReader.LazyQuotes = true     // 容忍欄位中出現未跳脫的引號
	csvReader.FieldsPerRecord = -1  // 容忍欄位數不一致(以單列防呆過濾)

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	walkpaths := make([]Walkpath, 0, len(records))
	for i, row := range records {
		if i == 0 {
			continue // skip header
		}
		if len(row) < 22 {
			continue
		}
		serial := parseIntSafe(row[0])
		if serial == 0 {
			continue // 序號異常的列直接濾掉
		}
		walkpaths = append(walkpaths, Walkpath{
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

	walkpathsCache = walkpaths
	walkpathsCacheTime = time.Now()
	return walkpaths, nil
}

/*
ListWalkpaths 從 data.taipei 下載並解析臺北市登山步道 CSV(Big5)
GET /api/v1/green/walkpath
*/
func ListWalkpaths(c *gin.Context) {
	walkpaths, err := fetchWalkpaths()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(walkpaths), "data": walkpaths})
}
// RecyclePoint 整合臺北市/新北市資源回收據點的統一格式
type RecyclePoint struct {
	Source    string  `json:"source"` // "taipei" 或 "new_taipei"
	City      string  `json:"city"`
	District  string  `json:"district"`
	Village   string  `json:"village"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Phone     string  `json:"phone"`
	Mobile    string  `json:"mobile"`
	OpenTime  string  `json:"open_time"`
	State     string  `json:"state"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

var (
	recyclesCache      []RecyclePoint
	recyclesCacheTime  time.Time
	recyclesCacheMutex sync.Mutex
)

const recyclesCacheTTL = 1 * time.Hour

// fetchRecycleTaipei 解析臺北市清潔隊資源回收站 CSV(Big5)
func fetchRecycleTaipei() ([]RecyclePoint, error) {
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

	points := make([]RecyclePoint, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 7 {
			continue
		}
		district := cleanField(row[0])
		if district == "" {
			continue
		}
		points = append(points, RecyclePoint{
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

// fetchRecycleNewTaipei 解析新北市資源回收個體戶 CSV(UTF-8 with BOM)
func fetchRecycleNewTaipei() ([]RecyclePoint, error) {
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

	points := make([]RecyclePoint, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 12 {
			continue
		}
		seq := cleanField(row[0])
		if seq == "" {
			continue
		}
		// recycle_address 為實際回收地點,優先使用;空字串時回退至 address(住家)
		address := cleanField(row[9])
		if address == "" {
			address = cleanField(row[5])
		}
		// 將分機合併到電話欄位
		phone := cleanField(row[6])
		if ext := cleanField(row[7]); ext != "" {
			phone = phone + "#" + ext
		}
		points = append(points, RecyclePoint{
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

func fetchRecycles() ([]RecyclePoint, error) {
	recyclesCacheMutex.Lock()
	defer recyclesCacheMutex.Unlock()

	if recyclesCache != nil && time.Since(recyclesCacheTime) < recyclesCacheTTL {
		return recyclesCache, nil
	}

	type result struct {
		points []RecyclePoint
		err    error
	}
	ch := make(chan result, 2)
	go func() { p, e := fetchRecycleTaipei(); ch <- result{p, e} }()
	go func() { p, e := fetchRecycleNewTaipei(); ch <- result{p, e} }()

	combined := make([]RecyclePoint, 0, 400)
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

	recyclesCache = combined
	recyclesCacheTime = time.Now()
	return combined, nil
}

/*
ListRecycles 整合臺北市清潔隊與新北市個體戶資源回收據點
GET /api/v1/green/recycle
*/
func ListRecycles(c *gin.Context) {
	points, err := fetchRecycles()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(points), "data": points})
}
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
ListUblikes 從 data.ntpc.gov.tw 取得新北市 YouBike2.0 站點即時資料
GET /api/v1/green/ublike
*/
func ListUblikes(c *gin.Context) {
	stations, err := fetchUbikeStations()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(stations), "data": stations})
}
