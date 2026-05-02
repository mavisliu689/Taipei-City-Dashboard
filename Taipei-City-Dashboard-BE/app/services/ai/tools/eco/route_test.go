// Package eco route tests cover the plan_eco_route tool. Phase 1 returns
// 3 candidate variants (shortest / balanced / greenest) by selecting POIs
// from the loaded dataset within the corridor between origin and destination.
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanEcoRoute_Should_ReturnThreeCandidates(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, err := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	assert.NoError(t, err)
	assert.Len(t, res.Routes, 3)
	ids := make([]string, 3)
	for i, r := range res.Routes {
		ids[i] = r.RouteID
	}
	assert.ElementsMatch(t, []string{"shortest", "balanced", "greenest"}, ids)
}

func TestPlanEcoRoute_Should_HaveShortestPathWithFewestStops(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	var shortest, greenest RouteVariant
	for _, r := range res.Routes {
		if r.RouteID == "shortest" {
			shortest = r
		}
		if r.RouteID == "greenest" {
			greenest = r
		}
	}
	assert.LessOrEqual(t, len(shortest.GreenPointsPassed), len(greenest.GreenPointsPassed))
	assert.LessOrEqual(t, shortest.DistanceM, greenest.DistanceM)
}

func TestPlanEcoRoute_Should_ReturnError_When_OriginUnknown(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	_, err := PlanEcoRoute(pts, "南極點", "新北市政府", PlanOptions{})
	assert.ErrorIs(t, err, ErrUnknownPlace)
}

func TestPlanEcoRoute_Should_PopulateShortestDistanceM(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	// 雙北市府大圓 ~10 km = 10000 m
	assert.InDelta(t, 10000, res.ShortestDistanceM, 500)
}

func TestPlanEcoRoute_Should_AnnotateGreenPointType(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	for _, r := range res.Routes {
		for _, p := range r.GreenPointsPassed {
			assert.NotEmpty(t, p.Name)
			assert.NotEmpty(t, p.Type)
			assert.Greater(t, p.Weight, 0)
		}
	}
}

func TestPlanEcoRoute_Should_HaveDistinctDurations_ForThreeRoutes(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	var shortest, balanced, greenest RouteVariant
	for _, r := range res.Routes {
		switch r.RouteID {
		case "shortest":
			shortest = r
		case "balanced":
			balanced = r
		case "greenest":
			greenest = r
		}
	}
	// shortest 不繞路也不停留 → 最短時間
	assert.Less(t, shortest.EstimatedMinutes, balanced.EstimatedMinutes)
	// greenest 繞最多 + 停最多 → 最長
	assert.Less(t, balanced.EstimatedMinutes, greenest.EstimatedMinutes)
	// 每個 POI 至少加 5 分停留
	gap := greenest.EstimatedMinutes - shortest.EstimatedMinutes
	assert.GreaterOrEqual(t, gap, len(greenest.GreenPointsPassed)*5)
}

func TestPlanEcoRoute_Should_HaveDistinctDistances_ForThreeRoutes(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{})
	var shortest, balanced, greenest RouteVariant
	for _, r := range res.Routes {
		switch r.RouteID {
		case "shortest":
			shortest = r
		case "balanced":
			balanced = r
		case "greenest":
			greenest = r
		}
	}
	// 每個綠點至少 200m round-trip detour，所以差距明顯
	assert.Less(t, shortest.DistanceM, balanced.DistanceM)
	assert.Less(t, balanced.DistanceM, greenest.DistanceM)
}

func TestPlanEcoRoute_Should_HonorRequiredCategory_OnBalancedAndGreenest(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	// 走廊夠長, fixture 內 5 類各 10 個都會落入 corridor; required=restaurant
	// 預期 balanced/greenest 兩條路線各至少有 1 個 restaurant POI。
	res, err := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{
		RequiredCategories: []string{"restaurant"},
	})
	assert.NoError(t, err)
	for _, r := range res.Routes {
		if r.RouteID == "shortest" {
			continue // shortest 設計上零繞路, 不應受 required 影響
		}
		hasRestaurant := false
		for _, gp := range r.GreenPointsPassed {
			if gp.Type == "restaurant" {
				hasRestaurant = true
				break
			}
		}
		assert.True(t, hasRestaurant,
			"route %q expected to include >=1 restaurant when required, got %+v", r.RouteID, r.GreenPointsPassed)
	}
}

func TestPlanEcoRoute_Should_LeaveShortestUnchanged_WhenRequiredGiven(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	res, _ := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{
		RequiredCategories: []string{"ubike"},
	})
	for _, r := range res.Routes {
		if r.RouteID == "shortest" {
			assert.Empty(t, r.GreenPointsPassed,
				"shortest route should remain zero-detour, but has %d green points", len(r.GreenPointsPassed))
		}
	}
}

func TestPlanEcoRoute_Should_ReportUnmetRequired_WhenCategoryAbsent(t *testing.T) {
	pts, _ := LoadPOIs(fixturePath)
	// fixture 沒有 ubike 類別 → 應在 UnmetRequired 回報
	res, err := PlanEcoRoute(pts, "台北市政府", "新北市政府", PlanOptions{
		RequiredCategories: []string{"ubike", "park"},
	})
	assert.NoError(t, err)
	assert.Contains(t, res.UnmetRequired, "ubike")
	assert.NotContains(t, res.UnmetRequired, "park", "park exists in fixture, should not be unmet")
}

func TestPickWithRequired_Should_PrioritizeRequiredCategoryFirst(t *testing.T) {
	// 直接構造 candidates 排序 (Detour 升序), 確保 required 即使 detour 較高也會被選入。
	mk := func(cat string, detour float64) scoredPOI {
		lat, lng := 25.0, 121.5
		return scoredPOI{POI: POI{Name: cat + "-x", Category: cat, Lat: &lat, Lng: &lng}, Detour: detour}
	}
	candidates := []scoredPOI{
		mk("park", 0.1),
		mk("park", 0.2),
		mk("restaurant", 0.5), // detour 最高, 但 required 應強制入選
	}
	picks := pickWithRequired(candidates, 2, []string{"restaurant"})
	cats := make(map[string]int)
	for _, p := range picks {
		cats[p.POI.Category]++
	}
	assert.Equal(t, 1, cats["restaurant"], "restaurant required → 至少 1 個")
	assert.Len(t, picks, 2)
}

func TestPickWithRequired_Should_BehaveLikeTopN_WhenNoRequired(t *testing.T) {
	mk := func(detour float64) scoredPOI {
		return scoredPOI{POI: POI{Name: "x", Category: "park"}, Detour: detour}
	}
	candidates := []scoredPOI{mk(0.1), mk(0.2), mk(0.3)}
	picks := pickWithRequired(candidates, 2, nil)
	assert.Len(t, picks, 2)
	assert.Equal(t, 0.1, picks[0].Detour)
	assert.Equal(t, 0.2, picks[1].Detour)
}
