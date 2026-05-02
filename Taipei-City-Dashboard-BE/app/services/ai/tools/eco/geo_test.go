// Package eco geo tests cover Haversine distance and place-name geocoding
// used by the eco-route AI tools.
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHaversine_Should_Return0_When_SamePoint(t *testing.T) {
	got := Haversine(25.0376, 121.5644, 25.0376, 121.5644)
	assert.Equal(t, 0.0, got)
}

func TestHaversine_Should_Match_TaipeiCityHallToNewTaipeiCityHall(t *testing.T) {
	// 台北市政府 (25.0376, 121.5644) → 新北市政府 (25.0124, 121.4664)
	// 大圓距離約 10.0 km (空中直線)，誤差容忍 ±0.3 km
	got := Haversine(25.0376, 121.5644, 25.0124, 121.4664)
	assert.InDelta(t, 10.0, got, 0.3)
}

func TestGeocode_Should_ReturnTaipeiCityHallCoord_When_QueryByName(t *testing.T) {
	lat, lng, err := Geocode("台北市政府")
	assert.NoError(t, err)
	assert.InDelta(t, 25.0376, lat, 0.005)
	assert.InDelta(t, 121.5644, lng, 0.005)
}

func TestGeocode_Should_ReturnNewTaipeiCityHallCoord_When_QueryByName(t *testing.T) {
	lat, lng, err := Geocode("新北市政府")
	assert.NoError(t, err)
	assert.InDelta(t, 25.0124, lat, 0.005)
	assert.InDelta(t, 121.4664, lng, 0.005)
}

func TestGeocode_Should_ReturnErrUnknownPlace_When_NameNotInDictionary(t *testing.T) {
	_, _, err := Geocode("不存在的地點 XYZ")
	assert.ErrorIs(t, err, ErrUnknownPlace)
}

func TestGeocode_Should_AcceptCommonAliases(t *testing.T) {
	cases := map[string]struct{ lat, lng float64 }{
		"市府":    {25.0376, 121.5644}, // 簡稱台北市政府
		"北市府":   {25.0376, 121.5644},
		"新北市府":  {25.0124, 121.4664},
		"板橋市政府": {25.0124, 121.4664}, // 新北市政府位於板橋
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			lat, lng, err := Geocode(name)
			assert.NoError(t, err)
			assert.InDelta(t, want.lat, lat, 0.005)
			assert.InDelta(t, want.lng, lng, 0.005)
		})
	}
}
