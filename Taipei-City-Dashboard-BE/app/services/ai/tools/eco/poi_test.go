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
