// Package models stores the models for the postgreSQL databases.
//
// green.go 定義 /api/v1/green 群組(park/restaurant/hotel/walkpath/recycle)的
// 唯讀資料模型。實際表結構由 Data Engineering 端 Airflow ETL 透過 pandas to_sql
// 寫入 DBDashboard,此處 GORM struct 僅作為 BE 讀取與 AutoMigrate 對齊用。
//
// json tag 完全對齊 controllers/green.go 既有的回應 struct,
// 重構為讀 DB 後對前端的回應 JSON 形狀不變。
package models

import "time"

/* ----- Park (parks.gov.taipei) ----- */

type GreenPark struct {
	ID             int64  `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	SeqNo          string `json:"SeqNo" gorm:"column:seq_no;type:varchar"`
	Name           string `json:"pm_name" gorm:"column:pm_name;type:varchar"`
	NameEng        string `json:"pm_name_eng" gorm:"column:pm_name_eng;type:varchar"`
	Overview       string `json:"pm_overview" gorm:"column:pm_overview;type:text"`
	Longitude      string `json:"pm_Longitude" gorm:"column:pm_longitude;type:varchar"`
	Latitude       string `json:"pm_Latitude" gorm:"column:pm_latitude;type:varchar"`
	Unit           string `json:"pm_unit" gorm:"column:pm_unit;type:varchar"`
	ConstYear      string `json:"pm_const_year" gorm:"column:pm_const_year;type:varchar"`
	Location       string `json:"pm_location" gorm:"column:pm_location;type:varchar"`
	LandPublicArea string `json:"pm_LandPublicArea" gorm:"column:pm_land_public_area;type:varchar"`
	OpeningStart   string `json:"pm_opening_s" gorm:"column:pm_opening_s;type:varchar"`
	OpeningEnd     string `json:"pm_opening_e" gorm:"column:pm_opening_e;type:varchar"`
	Libie          string `json:"pm_libie" gorm:"column:pm_libie;type:text"`
	Phone          string `json:"pm_phone" gorm:"column:pm_phone;type:varchar"`
	Sports         string `json:"pm_sports" gorm:"column:pm_sports;type:text"`
	Recreation     string `json:"pm_recreation" gorm:"column:pm_recreation;type:text"`
	Service        string `json:"pm_service" gorm:"column:pm_service;type:text"`
	Other          string `json:"pm_other" gorm:"column:pm_other;type:text"`
	Transit        string `json:"pm_transit" gorm:"column:pm_transit;type:text"`
	Ecology        string `json:"pm_ecology" gorm:"column:pm_ecology;type:text"`
	Type           string `json:"pm_type" gorm:"column:pm_type;type:varchar"`
	PlayType       string `json:"pm_playtype" gorm:"column:pm_playtype;type:varchar"`
	PlayArea       string `json:"pm_playarea" gorm:"column:pm_playarea;type:varchar"`
	Description    string `json:"pm_description" gorm:"column:pm_description;type:text"`
	PlayEquipment  string `json:"pm_playeq" gorm:"column:pm_playeq;type:text"`

	UpdatedAt time.Time `json:"-" gorm:"column:updated_at;type:timestamp with time zone"`
}

func (GreenPark) TableName() string { return "green_parks" }

/* ----- Restaurant (gis_p_11) ----- */

type GreenRestaurant struct {
	ID        int64  `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	RestID    string `json:"restid" gorm:"column:restid;type:varchar"`
	Name      string `json:"name" gorm:"column:name;type:varchar"`
	Address   string `json:"address" gorm:"column:address;type:varchar"`
	Phone     string `json:"phone" gorm:"column:phone;type:varchar"`
	Mobile    string `json:"mobile" gorm:"column:mobile;type:varchar"`
	Latitude  string `json:"latitude" gorm:"column:latitude;type:varchar"`
	Longitude string `json:"longitude" gorm:"column:longitude;type:varchar"`
	City      string `json:"city" gorm:"column:city;type:varchar"`

	UpdatedAt time.Time `json:"-" gorm:"column:updated_at;type:timestamp with time zone"`
}

func (GreenRestaurant) TableName() string { return "green_restaurants" }

/* ----- Hotel (gp_p_43) ----- */

type GreenHotel struct {
	ID           int64  `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	SerialNumber string `json:"serialnumber" gorm:"column:serialnumber;type:varchar"`
	Name         string `json:"name" gorm:"column:name;type:varchar"`
	Address      string `json:"address" gorm:"column:address;type:varchar"`
	Phone        string `json:"phone" gorm:"column:phone;type:varchar"`
	Latitude     string `json:"latitude" gorm:"column:latitude;type:varchar"`
	Longitude    string `json:"longitude" gorm:"column:longitude;type:varchar"`
	Note         string `json:"note" gorm:"column:note;type:varchar"`
	County       string `json:"county" gorm:"column:county;type:varchar"`
	Town         string `json:"town" gorm:"column:town;type:varchar"`
	Village      string `json:"village" gorm:"column:village;type:varchar"`

	UpdatedAt time.Time `json:"-" gorm:"column:updated_at;type:timestamp with time zone"`
}

func (GreenHotel) TableName() string { return "green_hotels" }

/* ----- Walkpath (data.taipei 登山步道) ----- */

type GreenWalkpath struct {
	ID                 int64   `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	SerialNumber       int     `json:"serial_number" gorm:"column:serial_number"`
	District           string  `json:"district" gorm:"column:district;type:varchar"`
	Route              string  `json:"route" gorm:"column:route;type:varchar"`
	TotalLengthM       int     `json:"total_length_m" gorm:"column:total_length_m"`
	OneWayMinutes      int     `json:"one_way_minutes" gorm:"column:one_way_minutes"`
	Grade              string  `json:"grade" gorm:"column:grade;type:varchar"`
	StartPoint         string  `json:"start_point" gorm:"column:start_point;type:varchar"`
	StartLongitude     float64 `json:"start_longitude" gorm:"column:start_longitude"`
	StartLatitude      float64 `json:"start_latitude" gorm:"column:start_latitude"`
	StartIsStairs      bool    `json:"start_is_stairs" gorm:"column:start_is_stairs"`
	EndPoint           string  `json:"end_point" gorm:"column:end_point;type:varchar"`
	EndLongitude       float64 `json:"end_longitude" gorm:"column:end_longitude"`
	EndLatitude        float64 `json:"end_latitude" gorm:"column:end_latitude"`
	EndIsStairs        bool    `json:"end_is_stairs" gorm:"column:end_is_stairs"`
	HasTrailGate       bool    `json:"has_trail_gate" gorm:"column:has_trail_gate"`
	WheelchairFriendly bool    `json:"wheelchair_friendly" gorm:"column:wheelchair_friendly"`
	WheelchairSlope    string  `json:"wheelchair_slope" gorm:"column:wheelchair_slope;type:varchar"`
	WheelchairLengthM  int     `json:"wheelchair_length_m" gorm:"column:wheelchair_length_m"`
	MobileSignal       string  `json:"mobile_signal" gorm:"column:mobile_signal;type:varchar"`
	HasMobileToilet    bool    `json:"has_mobile_toilet" gorm:"column:has_mobile_toilet"`
	ToiletLocation     string  `json:"toilet_location" gorm:"column:toilet_location;type:varchar"`
	AccessibleToilet   bool    `json:"accessible_toilet" gorm:"column:accessible_toilet"`

	UpdatedAt time.Time `json:"-" gorm:"column:updated_at;type:timestamp with time zone"`
}

func (GreenWalkpath) TableName() string { return "green_walkpaths" }

/* ----- Recycle Point (北市 + 新北 整合) ----- */

type GreenRecycle struct {
	ID        int64   `json:"-" gorm:"column:id;autoincrement;primaryKey"`
	Source    string  `json:"source" gorm:"column:source;type:varchar"` // taipei / new_taipei
	City      string  `json:"city" gorm:"column:city;type:varchar"`
	District  string  `json:"district" gorm:"column:district;type:varchar"`
	Village   string  `json:"village" gorm:"column:village;type:varchar"`
	Code      string  `json:"code" gorm:"column:code;type:varchar"`
	Name      string  `json:"name" gorm:"column:name;type:varchar"`
	Address   string  `json:"address" gorm:"column:address;type:varchar"`
	Phone     string  `json:"phone" gorm:"column:phone;type:varchar"`
	Mobile    string  `json:"mobile" gorm:"column:mobile;type:varchar"`
	OpenTime  string  `json:"open_time" gorm:"column:open_time;type:varchar"`
	State     string  `json:"state" gorm:"column:state;type:varchar"`
	Longitude float64 `json:"longitude" gorm:"column:longitude"`
	Latitude  float64 `json:"latitude" gorm:"column:latitude"`

	UpdatedAt time.Time `json:"-" gorm:"column:updated_at;type:timestamp with time zone"`
}

func (GreenRecycle) TableName() string { return "green_recycles" }

/* ----- Handlers ----- */

func GetAllGreenParks() ([]GreenPark, error) {
	var rows []GreenPark
	err := DBDashboard.Order("id").Find(&rows).Error
	return rows, err
}

func GetAllGreenRestaurants() ([]GreenRestaurant, error) {
	var rows []GreenRestaurant
	err := DBDashboard.Order("id").Find(&rows).Error
	return rows, err
}

func GetAllGreenHotels() ([]GreenHotel, error) {
	var rows []GreenHotel
	err := DBDashboard.Order("id").Find(&rows).Error
	return rows, err
}

func GetAllGreenWalkpaths() ([]GreenWalkpath, error) {
	var rows []GreenWalkpath
	err := DBDashboard.Order("serial_number").Find(&rows).Error
	return rows, err
}

func GetAllGreenRecycles() ([]GreenRecycle, error) {
	var rows []GreenRecycle
	err := DBDashboard.Order("id").Find(&rows).Error
	return rows, err
}
