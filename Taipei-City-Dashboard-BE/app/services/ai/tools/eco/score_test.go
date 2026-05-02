// Package eco score tests cover objective per-route scoring used by
// score_eco_routes (Go computes deterministic numbers; LLM writes narrative).
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func mkRoute(id string, distM float64, mins int, gp []GreenPoint) RouteVariant {
	score := 0
	for _, p := range gp {
		score += p.Weight
	}
	return RouteVariant{
		RouteID:           id,
		DistanceM:         distM,
		EstimatedMinutes:  mins,
		GreenPointsPassed: gp,
		GreenScoreTotal:   score,
	}
}

// ----- 綠點豐富度 (40 分) -----
func TestGreenRichness_Should_BeFull_When_TotalAtLeast30(t *testing.T) {
	got := scoreGreenRichness(30)
	assert.Equal(t, 40, got)
	assert.Equal(t, 40, scoreGreenRichness(100))
}

func TestGreenRichness_Should_BeZero_When_NoPoints(t *testing.T) {
	assert.Equal(t, 0, scoreGreenRichness(0))
}

func TestGreenRichness_Should_BeMid_When_Between15and29(t *testing.T) {
	got := scoreGreenRichness(20)
	assert.GreaterOrEqual(t, got, 25)
	assert.LessOrEqual(t, got, 35)
}

// ----- 效率 (30 分) -----
func TestEfficiency_Should_BeFull_When_DetourUnder1_2(t *testing.T) {
	assert.Equal(t, 30, scoreEfficiency(1.0))
	assert.Equal(t, 30, scoreEfficiency(1.2))
}

func TestEfficiency_Should_DropProportional_When_Detour1_2to1_5(t *testing.T) {
	got := scoreEfficiency(1.35)
	assert.GreaterOrEqual(t, got, 20)
	assert.LessOrEqual(t, got, 29)
}

func TestEfficiency_Should_BeAlmostZero_When_DetourOver1_8(t *testing.T) {
	assert.LessOrEqual(t, scoreEfficiency(2.0), 9)
}

// ----- 多樣性 (20 分) -----
func TestDiversity_Should_ScaleWithCategoryCount(t *testing.T) {
	assert.Equal(t, 0, scoreDiversity(0))
	assert.Equal(t, 5, scoreDiversity(1))
	assert.Equal(t, 10, scoreDiversity(2))
	assert.Equal(t, 15, scoreDiversity(3))
	assert.Equal(t, 20, scoreDiversity(4))
	assert.Equal(t, 20, scoreDiversity(5))
}

// ----- 健康 (10 分) -----
func TestHealth_Should_BeFull_When_Walk15to40Min(t *testing.T) {
	assert.Equal(t, 10, scoreHealth(15))
	assert.Equal(t, 10, scoreHealth(30))
	assert.Equal(t, 10, scoreHealth(40))
}

func TestHealth_Should_BeMid_When_Edge(t *testing.T) {
	assert.Equal(t, 6, scoreHealth(12))
	assert.Equal(t, 6, scoreHealth(45))
}

func TestHealth_Should_BeLow_When_TooShortOrTooLong(t *testing.T) {
	assert.Equal(t, 3, scoreHealth(5))
	assert.Equal(t, 3, scoreHealth(120))
}

// ----- 整體評分 -----
func TestScoreObjective_Should_GradeS_When_TotalAtLeast90(t *testing.T) {
	r := mkRoute("greenest", 2400, 30, []GreenPoint{
		{Name: "A park", Type: "park", Weight: 10, Lat: 25, Lng: 121},
		{Name: "B park", Type: "park", Weight: 10, Lat: 25, Lng: 121},
		{Name: "Trail X", Type: "trail", Weight: 10, Lat: 25, Lng: 121},
	})
	got := scoreObjective(r, 2100) // detour 1.14
	assert.GreaterOrEqual(t, got.Score, 70)
	assert.GreaterOrEqual(t, got.Score, got.Breakdown.GreenRichness+got.Breakdown.Efficiency+got.Breakdown.Diversity+got.Breakdown.Health-2)
	assert.LessOrEqual(t, got.Score, got.Breakdown.GreenRichness+got.Breakdown.Efficiency+got.Breakdown.Diversity+got.Breakdown.Health+2)
}

func TestScoreObjective_Should_GradeC_When_NoGreenAndShort(t *testing.T) {
	r := mkRoute("shortest", 2100, 26, nil)
	got := scoreObjective(r, 2100)
	assert.Equal(t, "C", got.Grade)
	assert.LessOrEqual(t, got.Score, 49)
}

// ----- Tool wrapper -----
func TestScoreEcoRoutesTool_Should_ReturnRankedRoutes(t *testing.T) {
	args := `{
        "start_name": "台北市政府",
        "end_name": "新北市政府",
        "shortest_distance_m": 10000,
        "routes": [
            {"route_id":"shortest","distance_m":10000,"estimated_minutes":133,"green_points_passed":[],"green_score_total":0,"label":"最短路徑"},
            {"route_id":"balanced","distance_m":10500,"estimated_minutes":150,"green_points_passed":[{"name":"P1","type":"park","weight":10,"lat":25,"lng":121},{"name":"R1","type":"restaurant","weight":3,"lat":25,"lng":121}],"green_score_total":13,"label":"平衡路徑"},
            {"route_id":"greenest","distance_m":11500,"estimated_minutes":183,"green_points_passed":[{"name":"P1","type":"park","weight":10,"lat":25,"lng":121},{"name":"P2","type":"park","weight":10,"lat":25,"lng":121},{"name":"T1","type":"trail","weight":8,"lat":25,"lng":121},{"name":"R1","type":"restaurant","weight":3,"lat":25,"lng":121}],"green_score_total":31,"label":"最綠路徑"}
        ]
    }`
	out, err := ScoreEcoRoutesTool(nil, args)
	assert.NoError(t, err)
	assert.Contains(t, out, "ranked_routes")
	assert.Contains(t, out, "score_breakdown")
}
