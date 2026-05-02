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
