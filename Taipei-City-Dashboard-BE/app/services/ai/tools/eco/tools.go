// Package eco tools wraps the pure functions into the ToolFunc signature
// expected by the AI tool registry: func(ctx, jsonArgs) (jsonResult, error).
package eco

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// dataPath holds the location of the unified eco-point dataset. It can be
// overridden via SetDataPath() for tests or via the ECO_DATA_PATH env var.
// Default: ../Taipei-City-Dashboard-FE/public/mockData/eco-route/all_points.json
// (assumes BE process is launched from Taipei-City-Dashboard-BE directory).
var dataPath = resolveDefaultDataPath()

func resolveDefaultDataPath() string {
	if env := os.Getenv("ECO_DATA_PATH"); env != "" {
		return env
	}
	candidates := []string{
		"../Taipei-City-Dashboard-FE/public/mockData/eco-route/all_points.json",
		"./Taipei-City-Dashboard-FE/public/mockData/eco-route/all_points.json",
		"/app/mockData/eco-route/all_points.json", // 容器內常見路徑
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0] // 沒找到也回第一條，後續報錯訊息會帶該路徑
}

// SetDataPath overrides the dataset location (call once at startup).
func SetDataPath(path string) {
	dataPath = path
}

// PlanEcoRouteTool wraps PlanEcoRoute for AI tool calling.
// args JSON: { "origin": "...", "destination": "...", "max_hop_km": 1.5,
//   "origin_coord": {"lat":..,"lng":..}, "destination_coord": {...},
//   "required_categories": ["ubike"]  // optional; balanced/greenest 路線將強制經過 }
func PlanEcoRouteTool(_ context.Context, args string) (string, error) {
	var in struct {
		Origin             string   `json:"origin"`
		Destination        string   `json:"destination"`
		MaxHopKm           float64  `json:"max_hop_km"`
		OriginCoord        *Coord   `json:"origin_coord"`
		DestinationCoord   *Coord   `json:"destination_coord"`
		RequiredCategories []string `json:"required_categories"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("plan_eco_route: invalid args: %w", err)
	}
	pts, err := LoadPOIs(dataPath)
	if err != nil {
		return "", fmt.Errorf("plan_eco_route: load dataset: %w", err)
	}
	res, err := PlanEcoRoute(pts, in.Origin, in.Destination, PlanOptions{
		CorridorBufferKm:   in.MaxHopKm,
		OriginCoord:        in.OriginCoord,
		DestCoord:          in.DestinationCoord,
		RequiredCategories: in.RequiredCategories,
	})
	if err != nil {
		return "", err
	}
	// 給 LLM 的版本拿掉 geometry (每條路線可能上千座標, 會撐爆 16k context)。
	// FE 另外透過 /eco/plan-route REST endpoint 拿到含 geometry 的完整版本。
	slim := *res
	slim.Routes = make([]RouteVariant, len(res.Routes))
	for i, r := range res.Routes {
		v := r
		v.Geometry = nil
		slim.Routes[i] = v
	}
	out, err := json.Marshal(slim)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FindEcoPOIsTool wraps FindEcoPOIs for AI tool calling.
// args JSON: {
//   "center": {"lat":..,"lng":..}, "radius_km": 1,
//   "categories": ["park"],
//   "districts": ["大安區","信義區"],   // optional, 嚴格行政區比對
//   "limit": 10
// }
func FindEcoPOIsTool(_ context.Context, args string) (string, error) {
	var in struct {
		Center     *LatLng  `json:"center"`
		RadiusKm   float64  `json:"radius_km"`
		Categories []string `json:"categories"`
		Districts  []string `json:"districts"`
		Limit      int      `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("find_eco_pois: invalid args: %w", err)
	}
	pts, err := LoadPOIs(dataPath)
	if err != nil {
		return "", fmt.Errorf("find_eco_pois: load dataset: %w", err)
	}
	// LLM context 16k token, full POI 物件帶 extra/tags/source 動輒 ~300 token/筆,
	// 5-10 筆就把 context 撐爆。給 LLM 的版本拿掉 extra/tags/source/_en 欄位 + 強制 limit ≤ 5
	// (LLM 文字回覆本來就只該列 5 筆給使用者看, 多的送過去也是浪費)。
	limit := in.Limit
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	res := FindEcoPOIs(pts, FindOptions{
		Center:     in.Center,
		RadiusKm:   in.RadiusKm,
		Categories: in.Categories,
		Districts:  in.Districts,
		Limit:      limit,
	})
	type slimPOI struct {
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
		Address  string  `json:"address,omitempty"`
		District string  `json:"district,omitempty"`
	}
	slim := make([]slimPOI, 0, len(res))
	for _, p := range res {
		var lat, lng float64
		if p.Lat != nil {
			lat = *p.Lat
		}
		if p.Lng != nil {
			lng = *p.Lng
		}
		slim = append(slim, slimPOI{
			Name: p.Name, Category: p.Category, Lat: lat, Lng: lng,
			Address: p.Address, District: p.District,
		})
	}
	out, err := json.Marshal(slim)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CalcCarbonSavingTool wraps CalcCarbonSavingGrams for AI tool calling.
// args JSON: { "legs": [ {"mode":"mrt","distance_km":10}, ... ] }
func CalcCarbonSavingTool(_ context.Context, args string) (string, error) {
	var in struct {
		Legs []Leg `json:"legs"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("calc_carbon_saving: invalid args: %w", err)
	}
	saved, err := CalcCarbonSavingGrams(in.Legs)
	if err != nil {
		return "", err
	}
	out := map[string]interface{}{
		"saved_grams":       saved,
		"saved_kg":          saved / 1000,
		"equivalent_trees":  EquivalentTrees(saved),
		"baseline_mode":     "car",
		"baseline_grams_km": carBaseline,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
