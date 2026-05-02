// Package eco directions calls Mapbox Directions API to get true road
// distance + duration + geometry for a sequence of waypoints. This replaces
// the naive Haversine + fixed-pace estimate which underestimated time
// significantly in urban areas.
package eco

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// WalkingRoute is the response from Mapbox Directions API for walking mode.
type WalkingRoute struct {
	DistanceM float64    `json:"-"`
	Duration  float64    `json:"-"` // seconds
	Geometry  GeoJSONGeo `json:"-"`
}

// GeoJSONGeo is a minimal LineString GeoJSON geometry passed back to FE.
type GeoJSONGeo struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// FetchWalkingRoute returns the road-following walking route for the
// given waypoints. Returns nil if the API fails or token is missing.
func FetchWalkingRoute(waypoints [][]float64) *WalkingRoute {
	if len(waypoints) < 2 {
		return nil
	}
	token := os.Getenv("MAPBOX_TOKEN")
	if token == "" {
		token = os.Getenv("VITE_MAPBOXTOKEN")
	}
	if token == "" {
		return nil
	}
	parts := make([]string, 0, len(waypoints))
	for _, p := range waypoints {
		if len(p) < 2 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.6f,%.6f", p[0], p[1]))
	}
	endpoint := fmt.Sprintf(
		"https://api.mapbox.com/directions/v5/mapbox/walking/%s?geometries=geojson&overview=full&access_token=%s",
		strings.Join(parts, ";"), url.QueryEscape(token),
	)
	resp, err := httpCli.Get(endpoint)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Routes []struct {
			Distance float64    `json:"distance"`
			Duration float64    `json:"duration"`
			Geometry GeoJSONGeo `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}
	if len(data.Routes) == 0 {
		return nil
	}
	r := data.Routes[0]
	return &WalkingRoute{DistanceM: r.Distance, Duration: r.Duration, Geometry: r.Geometry}
}
