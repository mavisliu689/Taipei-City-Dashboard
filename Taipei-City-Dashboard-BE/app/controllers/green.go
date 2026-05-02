package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const parksAPIURL = "https://parks.gov.taipei/parks/api/"

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

func ListRestaurants(c *gin.Context) {}
func ListHotels(c *gin.Context)      {}
func ListWalkpaths(c *gin.Context)   {}
func ListRecycles(c *gin.Context)    {}
func ListUblikes(c *gin.Context)    {}
