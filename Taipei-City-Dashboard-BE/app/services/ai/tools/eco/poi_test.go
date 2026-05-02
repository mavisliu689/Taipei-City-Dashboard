// Package eco poi tests cover loading the unified mock dataset and the
// proximity / category filters used by find_eco_pois.
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const fixturePath = "testdata/points_sample.json"

func TestLoadPOIs_Should_LoadFixtureFromDisk(t *testing.T) {
	pts, err := LoadPOIs(fixturePath)
	assert.NoError(t, err)
	assert.NotEmpty(t, pts)
	assert.Equal(t, 50, len(pts))
}

func TestFindEcoPOIs_Should_FilterByCategory(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	got := FindEcoPOIs(pts, FindOptions{
		Categories: []string{"park"},
		Limit:      100,
	})
	for _, p := range got {
		assert.Equal(t, "park", p.Category)
	}
}

func TestFindEcoPOIs_Should_FilterByRadius(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	// 中心 = 台北市政府, 半徑 5 km
	got := FindEcoPOIs(pts, FindOptions{
		Center:    &LatLng{Lat: 25.0376, Lng: 121.5644},
		RadiusKm:  5,
		Limit:     100,
	})
	for _, p := range got {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		d := Haversine(25.0376, 121.5644, *p.Lat, *p.Lng)
		assert.LessOrEqual(t, d, 5.0)
	}
}

func TestFindEcoPOIs_Should_RespectLimit(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	got := FindEcoPOIs(pts, FindOptions{Limit: 3})
	assert.LessOrEqual(t, len(got), 3)
}

func TestFindEcoPOIs_Should_ResolveLiToDistrict_ForPark(t *testing.T) {
	// park 資料的 District 欄存「里」名 (例「長安里」). 即便 LLM 傳
	// districts=["大安區"], 嚴格比對也應透過 li→區 mapping 還原命中。
	saved := liToDistrict
	defer func() { liToDistrict = saved }()
	liToDistrict = map[liKey]string{
		{City: "臺北市", Li: "錦泰里"}: "大安區",
	}
	lat, lng := 25.026, 121.543
	pts := []POI{
		{ID: "p1", Name: "錦安公園", Category: "park", Lat: &lat, Lng: &lng,
			City: "臺北市", District: "錦泰里"},
	}
	got := FindEcoPOIs(pts, FindOptions{
		Categories: []string{"park"},
		Districts:  []string{"大安區"},
		Limit:      10,
	})
	assert.Len(t, got, 1)
	assert.Equal(t, "錦安公園", got[0].Name)
}

func TestFindEcoPOIs_Should_FallbackToAddressDistrict(t *testing.T) {
	// 表還沒涵蓋的 (city,里) — 改靠 address 含「XX區」也要能命中。
	saved := liToDistrict
	defer func() { liToDistrict = saved }()
	liToDistrict = map[liKey]string{} // 空表
	lat, lng := 25.038, 121.515
	pts := []POI{
		{ID: "p1", Name: "二二八和平公園", Category: "park", Lat: &lat, Lng: &lng,
			City: "臺北市", District: "黎明里",
			Address: "中正區凱達格蘭大道3號"},
	}
	got := FindEcoPOIs(pts, FindOptions{
		Categories: []string{"park"},
		Districts:  []string{"中正區"},
		Limit:      10,
	})
	assert.Len(t, got, 1)
}

func TestFindEcoPOIs_Should_RejectMismatchedDistrict(t *testing.T) {
	// 防呆: District 解析後若不在請求的 districts 裡, 必須過濾掉。
	saved := liToDistrict
	defer func() { liToDistrict = saved }()
	liToDistrict = map[liKey]string{
		{City: "臺北市", Li: "長安里"}: "北投區",
	}
	lat, lng := 25.136, 121.502
	pts := []POI{
		{ID: "p1", Name: "七虎公園", Category: "park", Lat: &lat, Lng: &lng,
			City: "臺北市", District: "長安里"},
	}
	got := FindEcoPOIs(pts, FindOptions{
		Categories: []string{"park"},
		Districts:  []string{"大安區"},
		Limit:      10,
	})
	assert.Empty(t, got)
}

func TestFindEcoPOIs_Should_FilterByCategory_Ubike(t *testing.T) {
	lat1, lng1 := 25.033, 121.564
	lat2, lng2 := 25.040, 121.560
	pts := []POI{
		{ID: "ubike-tpe-1", Name: "YouBike2.0_市府站", Category: "ubike",
			Lat: &lat1, Lng: &lng1, City: "臺北市", District: "信義區"},
		{ID: "park-1", Name: "信義公園", Category: "park",
			Lat: &lat2, Lng: &lng2, City: "臺北市", District: "信義區"},
	}
	got := FindEcoPOIs(pts, FindOptions{
		Categories: []string{"ubike"},
		Limit:      10,
	})
	assert.Len(t, got, 1)
	assert.Equal(t, "ubike", got[0].Category)
	assert.Equal(t, "YouBike2.0_市府站", got[0].Name)
}

func TestFindEcoPOIs_Should_SkipNullCoord(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	got := FindEcoPOIs(pts, FindOptions{
		Center:   &LatLng{Lat: 25.0, Lng: 121.5},
		RadiusKm: 50,
		Limit:    1000,
	})
	for _, p := range got {
		assert.NotNil(t, p.Lat)
		assert.NotNil(t, p.Lng)
	}
}
