// Package eco tools_test verifies the JSON wrappers used by the registry.
package eco

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcCarbonSavingTool_Should_ReturnJSONWithSavedGrams(t *testing.T) {
	args := `{"legs":[{"mode":"mrt","distance_km":10}]}`
	out, err := CalcCarbonSavingTool(context.Background(), args)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.InDelta(t, 1590.0, got["saved_grams"], 0.001)
	assert.InDelta(t, 1.59, got["saved_kg"], 0.001)
	assert.NotZero(t, got["equivalent_trees"])
}

func TestPlanEcoRouteTool_Should_ReturnJSONWithThreeRoutes(t *testing.T) {
	SetDataPath(fixturePath)
	defer SetDataPath("Taipei-City-Dashboard-FE/public/mockData/eco-route/all_points.json")

	args := `{"origin":"台北市政府","destination":"新北市政府"}`
	out, err := PlanEcoRouteTool(context.Background(), args)
	require.NoError(t, err)

	var res PlanResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Len(t, res.Routes, 3)
	assert.Equal(t, "台北市政府", res.StartName)
	assert.Equal(t, "新北市政府", res.EndName)
}

func TestFindEcoPOIsTool_Should_ReturnJSONArray(t *testing.T) {
	SetDataPath(fixturePath)
	defer SetDataPath("Taipei-City-Dashboard-FE/public/mockData/eco-route/all_points.json")

	args := `{"center":{"Lat":25.0376,"Lng":121.5644},"radius_km":50,"categories":["park"],"limit":3}`
	out, err := FindEcoPOIsTool(context.Background(), args)
	require.NoError(t, err)

	var pts []POI
	require.NoError(t, json.Unmarshal([]byte(out), &pts))
	assert.LessOrEqual(t, len(pts), 3)
	for _, p := range pts {
		assert.Equal(t, "park", p.Category)
	}
}

func TestPlanEcoRouteTool_Should_ReturnError_When_BadJSON(t *testing.T) {
	_, err := PlanEcoRouteTool(context.Background(), "not json")
	assert.Error(t, err)
}
