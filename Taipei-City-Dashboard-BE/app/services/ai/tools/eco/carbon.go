// Package eco carbon implements the carbon-saving calculator. Emission
// factors are gCO2e per passenger-kilometre, sourced from Taiwan EPA /
// 北捷年報. Baseline is 自小客車 (gasoline private car).
package eco

import "errors"

// ErrUnknownMode is returned when a leg uses a transport mode that is not
// in the emission factor table.
var ErrUnknownMode = errors.New("eco: unknown transport mode")

// Leg is one segment of a route, used as both calculator input and the
// shape returned to the LLM via tool call.
type Leg struct {
	Mode       string  `json:"mode"`
	DistanceKm float64 `json:"distance_km"`
}

// emission factor table: gCO2e per passenger-km
var emissionFactor = map[string]float64{
	"car":     192,
	"scooter": 78,
	"bus":     49,
	"mrt":     33,
	"youbike": 0,
	"walk":    0,
}

const (
	carBaseline           = 192.0  // gCO2e/km，作為比較基準
	treeSequestrationGram = 21770  // 一棵成熟樹一年固碳量 (公克)
)

// CalcCarbonSavingGrams returns total grams of CO2e saved compared with
// driving the same distance in a private car. Returns ErrUnknownMode if any
// leg uses a mode not in the emission factor table.
func CalcCarbonSavingGrams(legs []Leg) (float64, error) {
	total := 0.0
	for _, l := range legs {
		factor, ok := emissionFactor[l.Mode]
		if !ok {
			return 0, ErrUnknownMode
		}
		total += (carBaseline - factor) * l.DistanceKm
	}
	return total, nil
}

// EquivalentTrees converts saved grams of CO2e into equivalent number of
// mature trees needed to sequester that amount in one year.
func EquivalentTrees(savedGrams float64) float64 {
	return savedGrams / treeSequestrationGram
}
