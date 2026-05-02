package controllers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"TaipeiCityDashboardBE/app/models"

	"github.com/gin-gonic/gin"
)

// /api/v1/green 群組:
//   - park / restaurant / hotel / walkpath / recycle: 由 Airflow ETL 寫入 DBDashboard,
//     BE 直接讀 DB 回應(原本即時抓外部 API + in-memory cache 的邏輯已移除)。
//   - ublike: 30 秒 TTL 的即時資料,維持外部抓取 + in-memory cache 的舊邏輯。

const ubikeCSVURL = "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"

// cleanField 收斂前後空白與內嵌換行,合併連續空白,並去除 UTF-8 BOM
func cleanField(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
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

/*
ListParks 從 DBDashboard 讀取公園資料(原資料源:parks.gov.taipei,由 Airflow 寫入)
GET /api/v1/green/park
*/
func ListParks(c *gin.Context) {
	parks, err := models.GetAllGreenParks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(parks), "data": parks})
}

/*
ListRestaurants 從 DBDashboard 讀取環保餐廳清單(原資料源:環境部 gis_p_11)
GET /api/v1/green/restaurant
*/
func ListRestaurants(c *gin.Context) {
	restaurants, err := models.GetAllGreenRestaurants()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(restaurants), "data": restaurants})
}

/*
ListHotels 從 DBDashboard 讀取環保旅宿清單(原資料源:環境部 gp_p_43)
GET /api/v1/green/hotel
*/
func ListHotels(c *gin.Context) {
	hotels, err := models.GetAllGreenHotels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(hotels), "data": hotels})
}

/*
ListWalkpaths 從 DBDashboard 讀取臺北市登山步道(原資料源:data.taipei CSV)
GET /api/v1/green/walkpath
*/
func ListWalkpaths(c *gin.Context) {
	walkpaths, err := models.GetAllGreenWalkpaths()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "total": len(walkpaths), "data": walkpaths})
}

/*
ListRecycles 從 DBDashboard 讀取整合後的資源回收據點(臺北市 + 新北市)
GET /api/v1/green/recycle
*/
func ListRecycles(c *gin.Context) {
	points, err := models.GetAllGreenRecycles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
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
ListUblikes 從 data.ntpc.gov.tw 取得新北市 YouBike2.0 站點即時資料(30 秒 cache)
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
