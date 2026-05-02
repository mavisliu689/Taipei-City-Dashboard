// Package eco score implements the deterministic objective scoring for
// score_eco_routes. Go computes the numbers (verifiable, testable). The
// LLM consumes the result and writes the narrative + ranking.
package eco

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// ScoreBreakdown holds the four score components per route.
type ScoreBreakdown struct {
	GreenRichness int `json:"green_richness"` // 0..40
	Efficiency    int `json:"efficiency"`     // 0..30
	Diversity     int `json:"diversity"`      // 0..20
	Health        int `json:"health"`         // 0..10
}

// ScoredRoute is one objectively-scored route plus the original route data.
type ScoredRoute struct {
	RouteID           string         `json:"route_id"`
	Label             string         `json:"label"`
	DistanceM         float64        `json:"distance_m"`
	EstimatedMinutes  int            `json:"estimated_minutes"`
	GreenPointsPassed []GreenPoint   `json:"green_points_passed"`
	GreenScoreTotal   int            `json:"green_score_total"`
	Score             int            `json:"score"`
	Grade             string         `json:"grade"`
	Breakdown         ScoreBreakdown `json:"score_breakdown"`
	Rank              int            `json:"rank"`
}

// ScoreResult bundles the ranked routes plus echo of input metadata.
type ScoreResult struct {
	StartName             string        `json:"start_name"`
	EndName               string        `json:"end_name"`
	ShortestDistanceM     float64       `json:"shortest_distance_m"`
	RankedRoutes          []ScoredRoute `json:"ranked_routes"`
	OverallRecommendation string        `json:"overall_recommendation"`
}

// ----- Per-dimension scoring -----

// scoreGreenRichness maps total green weight to 0..40 points.
//   0    → 0
//   1-14 → 10..24 (linear)
//   15-29 → 25..35 (linear)
//   ≥30  → 40
func scoreGreenRichness(total int) int {
	switch {
	case total <= 0:
		return 0
	case total >= 30:
		return 40
	case total >= 15:
		return 25 + (total-15)*10/14 // 15→25 .. 29→35
	default:
		return 10 + (total-1)*14/13 // 1→10 .. 14→24
	}
}

// scoreEfficiency maps detour ratio to 0..30 points.
//   ≤1.2 → 30
//   1.2..1.5 → 20..29
//   1.5..1.8 → 10..19
//   >1.8 → 0..9
func scoreEfficiency(detourRatio float64) int {
	switch {
	case detourRatio <= 1.2:
		return 30
	case detourRatio <= 1.5:
		return 29 - int((detourRatio-1.2)/0.3*9)
	case detourRatio <= 1.8:
		return 19 - int((detourRatio-1.5)/0.3*9)
	case detourRatio <= 2.5:
		return 9 - int((detourRatio-1.8)/0.7*9)
	default:
		return 0
	}
}

// scoreDiversity maps distinct category count to 0..20 points.
func scoreDiversity(categoryCount int) int {
	switch {
	case categoryCount <= 0:
		return 0
	case categoryCount == 1:
		return 5
	case categoryCount == 2:
		return 10
	case categoryCount == 3:
		return 15
	default:
		return 20
	}
}

// scoreHealth maps walk minutes to 0..10 points.
func scoreHealth(min int) int {
	switch {
	case min >= 15 && min <= 40:
		return 10
	case (min >= 10 && min < 15) || (min > 40 && min <= 50):
		return 6
	default:
		return 3
	}
}

// scoreObjective computes the full per-route objective scoring.
func scoreObjective(r RouteVariant, shortestDistM float64) ScoredRoute {
	categories := uniqueCategories(r.GreenPointsPassed)

	br := ScoreBreakdown{
		GreenRichness: scoreGreenRichness(r.GreenScoreTotal),
		Efficiency:    scoreEfficiency(detourRatio(r.DistanceM, shortestDistM)),
		Diversity:     scoreDiversity(len(categories)),
		Health:        scoreHealth(r.EstimatedMinutes),
	}
	score := br.GreenRichness + br.Efficiency + br.Diversity + br.Health

	return ScoredRoute{
		RouteID:           r.RouteID,
		Label:             r.Label,
		DistanceM:         r.DistanceM,
		EstimatedMinutes:  r.EstimatedMinutes,
		GreenPointsPassed: r.GreenPointsPassed,
		GreenScoreTotal:   r.GreenScoreTotal,
		Score:             score,
		Grade:             gradeFor(score),
		Breakdown:         br,
	}
}

func detourRatio(dist, shortest float64) float64 {
	if shortest <= 0 {
		return 1.0
	}
	return dist / shortest
}

func uniqueCategories(pts []GreenPoint) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, p := range pts {
		if !seen[p.Type] {
			seen[p.Type] = true
			out = append(out, p.Type)
		}
	}
	return out
}

func gradeFor(score int) string {
	switch {
	case score >= 90:
		return "S"
	case score >= 75:
		return "A"
	case score >= 50:
		return "B"
	default:
		return "C"
	}
}

// ----- Tool wrapper -----

type scoreInput struct {
	StartName         string         `json:"start_name"`
	EndName           string         `json:"end_name"`
	ShortestDistanceM float64        `json:"shortest_distance_m"`
	Routes            []RouteVariant `json:"routes"`
}

// ScoreEcoRoutesTool implements the score_eco_routes tool. Returns ranked
// scored routes plus the recommended overall route_id, ready for LLM to
// turn into narrative.
func ScoreEcoRoutesTool(_ context.Context, args string) (string, error) {
	var in scoreInput
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("score_eco_routes: invalid args: %w", err)
	}

	scored := make([]ScoredRoute, 0, len(in.Routes))
	for _, r := range in.Routes {
		scored = append(scored, scoreObjective(r, in.ShortestDistanceM))
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	for i := range scored {
		scored[i].Rank = i + 1
	}

	overall := ""
	if len(scored) > 0 {
		overall = scored[0].RouteID
	}

	res := ScoreResult{
		StartName:             in.StartName,
		EndName:               in.EndName,
		ShortestDistanceM:     in.ShortestDistanceM,
		RankedRoutes:          scored,
		OverallRecommendation: overall,
	}
	out, _ := json.Marshal(res)
	return string(out), nil
}
