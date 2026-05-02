// Package eco route implements plan_eco_route. Phase 1 strategy: build a
// straight-line corridor between origin and destination, then greedily pick
// nearby POIs to form three variants:
//
//	shortest  — origin → destination (no detour)
//	balanced  — pick top 1~2 POIs near corridor midpoint
//	greenest  — pick all POIs within corridor buffer, sorted by detour cost
package eco

import "sort"

// PlanOptions configures PlanEcoRoute. All fields optional; sensible
// defaults are applied when zero.
type PlanOptions struct {
	CorridorBufferKm float64 // default 1.0
	MaxGreenPoints   int     // default 6
	// 若提供，會覆寫 origin/destination 的 geocoding 結果。
	// 用於使用者在地圖上手動點選或拖曳起終點時。
	OriginCoord *Coord
	DestCoord   *Coord
	// RequiredCategories 強制 balanced/greenest 路線至少各包含 1 個指定類別 POI。
	// 例: 使用者「經過一個 youbike 站」→ ["ubike"]; 「順路找咖啡廳」→ ["restaurant"]。
	// shortest 保留 0 繞路語意, 不受此參數影響。
	// 走廊內若無對應類別 POI, 該類別會被跳過, 透過 UnmetRequired 回傳。
	RequiredCategories []string
}

const (
	// 進出每個綠點的 round-trip 步行距離（真實情境下，要走離主路線才能進入景點）
	perPOIRoundTripKm = 0.2
	// 每個綠點停留時間（觀光、用餐、休憩平均）
	perPOIStopMinutes = 5
)

// GreenPoint is one POI passed by a route.
type GreenPoint struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Weight int     `json:"weight"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
}

// RouteVariant is one candidate path returned by PlanEcoRoute.
type RouteVariant struct {
	RouteID           string       `json:"route_id"`
	Label             string       `json:"label"`
	DistanceM         float64      `json:"distance_m"`
	EstimatedMinutes  int          `json:"estimated_minutes"`
	GreenPointsPassed []GreenPoint `json:"green_points_passed"`
	GreenScoreTotal   int          `json:"green_score_total"`
	Geometry          *GeoJSONGeo  `json:"geometry,omitempty"`
}

// PlanResult bundles 3 candidates plus metadata. StartCoord/EndCoord allow
// the FE to render polylines without needing its own geocoder.
type PlanResult struct {
	StartName         string         `json:"start_name"`
	EndName           string         `json:"end_name"`
	StartCoord        Coord          `json:"start_coord"`
	EndCoord          Coord          `json:"end_coord"`
	ShortestDistanceM float64        `json:"shortest_distance_m"`
	Routes            []RouteVariant `json:"routes"`
	// UnmetRequired 列出走廊內找不到 POI 的 RequiredCategories, 給 LLM 誠實回覆使用者。
	// 例: 使用者要求 ["ubike"] 但走廊 1km 內無 ubike 站, 此欄會是 ["ubike"]。
	UnmetRequired []string `json:"unmet_required,omitempty"`
}

// Coord is a (lat, lng) pair returned to the frontend.
type Coord struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// 各類別綠點權重（影響評分中「綠點豐富度」項目）
var categoryWeight = map[string]int{
	"park":       10,
	"hotel":      6,
	"restaurant": 3,
	"ubike":      3,
	"recycle":    2,
}

// PlanEcoRoute returns 3 candidate routes between origin and destination.
// If opt.OriginCoord / DestCoord are set, they override the geocoded result.
func PlanEcoRoute(pts []POI, origin, destination string, opt PlanOptions) (*PlanResult, error) {
	var oLat, oLng, dLat, dLng float64
	var err error

	if opt.OriginCoord != nil {
		oLat, oLng = opt.OriginCoord.Lat, opt.OriginCoord.Lng
	} else {
		oLat, oLng, err = Geocode(origin)
		if err != nil {
			return nil, err
		}
	}
	if opt.DestCoord != nil {
		dLat, dLng = opt.DestCoord.Lat, opt.DestCoord.Lng
	} else {
		dLat, dLng, err = Geocode(destination)
		if err != nil {
			return nil, err
		}
	}

	if opt.CorridorBufferKm == 0 {
		opt.CorridorBufferKm = 1.0
	}
	if opt.MaxGreenPoints == 0 {
		opt.MaxGreenPoints = 6
	}

	directDist := Haversine(oLat, oLng, dLat, dLng)
	candidates := poisAlongCorridor(pts, oLat, oLng, dLat, dLng, opt.CorridorBufferKm)

	originPt := []float64{oLng, oLat}
	destPt := []float64{dLng, dLat}

	required := normalizeRequired(opt.RequiredCategories)
	unmet := findUnmetCategories(candidates, required)

	shortest := buildShortestRoute(directDist, originPt, destPt)
	balanced := buildBalancedRoute(directDist, candidates, required, originPt, destPt)
	greenest := buildGreenestRoute(directDist, candidates, opt.MaxGreenPoints, required, originPt, destPt)

	// 用真實步行距離 (shortest variant) 作為最短參考
	shortestM := shortest.DistanceM
	if shortestM <= 0 {
		shortestM = directDist * 1000
	}

	return &PlanResult{
		StartName:         origin,
		EndName:           destination,
		StartCoord:        Coord{Lat: oLat, Lng: oLng},
		EndCoord:          Coord{Lat: dLat, Lng: dLng},
		ShortestDistanceM: shortestM,
		Routes:            []RouteVariant{shortest, balanced, greenest},
		UnmetRequired:     unmet,
	}, nil
}

// normalizeRequired lower-cases & dedupes required categories.
func normalizeRequired(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, c := range in {
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// findUnmetCategories returns required categories with zero candidates in the corridor.
func findUnmetCategories(candidates []scoredPOI, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	have := make(map[string]bool, len(required))
	for _, c := range candidates {
		have[c.POI.Category] = true
	}
	var unmet []string
	for _, c := range required {
		if !have[c] {
			unmet = append(unmet, c)
		}
	}
	return unmet
}

func poisAlongCorridor(pts []POI, oLat, oLng, dLat, dLng, bufferKm float64) []scoredPOI {
	var out []scoredPOI
	corridorLen := Haversine(oLat, oLng, dLat, dLng)
	for _, p := range pts {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		dFromOrigin := Haversine(oLat, oLng, *p.Lat, *p.Lng)
		dToDest := Haversine(*p.Lat, *p.Lng, dLat, dLng)
		// 三角不等式: 若 (起→P) + (P→終) 約等於 (起→終) + buffer，則 P 在走廊上
		detour := (dFromOrigin + dToDest) - corridorLen
		if detour > bufferKm*2 {
			continue
		}
		out = append(out, scoredPOI{POI: p, Detour: detour})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Detour < out[j].Detour })
	return out
}

type scoredPOI struct {
	POI    POI
	Detour float64
}

func buildShortestRoute(distKm float64, origin, dest []float64) RouteVariant {
	v := RouteVariant{
		RouteID:           "shortest",
		Label:             "最短路徑",
		GreenPointsPassed: []GreenPoint{},
		GreenScoreTotal:   0,
	}
	applyDirections(&v, [][]float64{origin, dest}, distKm, 0)
	return v
}

func buildBalancedRoute(distKm float64, candidates []scoredPOI, required []string, origin, dest []float64) RouteVariant {
	return assembleVariant("balanced", "平衡路徑", distKm, pickWithRequired(candidates, 2, required), origin, dest)
}

func buildGreenestRoute(distKm float64, candidates []scoredPOI, maxPts int, required []string, origin, dest []float64) RouteVariant {
	return assembleVariant("greenest", "最綠路徑", distKm, pickWithRequired(candidates, maxPts, required), origin, dest)
}

// assembleVariant builds a route variant. Real road distance/duration come
// from Mapbox Directions when reachable; falls back to Haversine + estimate.
func assembleVariant(id, label string, baseDistKm float64, picks []scoredPOI, origin, dest []float64) RouteVariant {
	gp, score := toGreenPoints(picks)
	v := RouteVariant{
		RouteID:           id,
		Label:             label,
		GreenPointsPassed: gp,
		GreenScoreTotal:   score,
	}
	wp := make([][]float64, 0, len(picks)+2)
	wp = append(wp, origin)
	for _, p := range picks {
		if p.POI.Lat != nil && p.POI.Lng != nil {
			wp = append(wp, []float64{*p.POI.Lng, *p.POI.Lat})
		}
	}
	wp = append(wp, dest)

	// fallback estimate (Haversine + 4.5 km/h + per-POI overhead)
	count := len(picks)
	fallbackDistKm := baseDistKm + sumDetour(picks) + float64(count)*perPOIRoundTripKm
	fallbackStops := count * perPOIStopMinutes
	applyDirections(&v, wp, fallbackDistKm, fallbackStops)
	return v
}

// applyDirections fills DistanceM / EstimatedMinutes / Geometry on v using
// Mapbox Directions when possible, falling back to the supplied estimate.
func applyDirections(v *RouteVariant, waypoints [][]float64, fallbackDistKm float64, stopMinutes int) {
	r := FetchWalkingRoute(waypoints)
	if r != nil {
		v.DistanceM = r.DistanceM
		v.EstimatedMinutes = int(r.Duration/60+0.5) + stopMinutes
		geom := r.Geometry
		v.Geometry = &geom
		return
	}
	v.DistanceM = fallbackDistKm * 1000
	v.EstimatedMinutes = estimateMinutes(fallbackDistKm, stopMinutes)
}

func pickTopN(candidates []scoredPOI, n int) []scoredPOI {
	if len(candidates) <= n {
		return candidates
	}
	return candidates[:n]
}

// pickWithRequired guarantees each required category contributes at least 1
// pick (lowest detour of that category) before falling back to top-N by detour.
// Caller must pass `candidates` already sorted by Detour ascending. The result
// preserves detour-ascending order for downstream geometry/score consistency.
// If no required categories are given, behaves identical to pickTopN.
func pickWithRequired(candidates []scoredPOI, n int, required []string) []scoredPOI {
	if len(required) == 0 {
		return pickTopN(candidates, n)
	}
	if n <= 0 || len(candidates) == 0 {
		return nil
	}

	chosen := make(map[int]struct{}, n)
	// step 1: 為每個 required 類別挑一個最低 detour 的 POI (找不到就跳過)
	for _, cat := range required {
		for i, c := range candidates {
			if _, used := chosen[i]; used {
				continue
			}
			if c.POI.Category == cat {
				chosen[i] = struct{}{}
				break
			}
		}
		if len(chosen) >= n {
			break
		}
	}
	// step 2: 剩餘名額用 top-N by detour 補滿
	for i := range candidates {
		if len(chosen) >= n {
			break
		}
		if _, used := chosen[i]; used {
			continue
		}
		chosen[i] = struct{}{}
	}
	// step 3: 依原本的 detour 升序輸出 (candidates 已預排, 用 index 順序即可)
	out := make([]scoredPOI, 0, len(chosen))
	for i := range candidates {
		if _, used := chosen[i]; used {
			out = append(out, candidates[i])
		}
	}
	return out
}

func sumDetour(picks []scoredPOI) float64 {
	total := 0.0
	for _, p := range picks {
		total += p.Detour
	}
	return total
}

func toGreenPoints(picks []scoredPOI) ([]GreenPoint, int) {
	out := make([]GreenPoint, 0, len(picks))
	score := 0
	for _, p := range picks {
		w := categoryWeight[p.POI.Category]
		if w == 0 {
			w = 1
		}
		out = append(out, GreenPoint{
			Name:   p.POI.Name,
			Type:   p.POI.Category,
			Weight: w,
			Lat:    *p.POI.Lat,
			Lng:    *p.POI.Lng,
		})
		score += w
	}
	return out, score
}

// estimateMinutes assumes 4.5 km/h walking pace and adds stop time for POI visits.
func estimateMinutes(distKm float64, stopMinutes int) int {
	if distKm <= 0 {
		return stopMinutes
	}
	walkMin := int(distKm/4.5*60 + 0.5) // 四捨五入
	return walkMin + stopMinutes
}
