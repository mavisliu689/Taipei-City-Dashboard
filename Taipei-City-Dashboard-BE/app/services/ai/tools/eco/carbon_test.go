// Package eco carbon tests cover the carbon-saving calculator that compares
// transport modes against a private-car baseline.
package eco

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalcCarbonSaving_Should_ReturnZero_When_LegsEmpty(t *testing.T) {
	saved, err := CalcCarbonSavingGrams(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0.0, saved)
}

func TestCalcCarbonSaving_Should_ReturnExpected_When_SingleMRTLeg(t *testing.T) {
	// 10 km MRT vs car baseline = (192 - 33) * 10 = 1590 g
	legs := []Leg{{Mode: "mrt", DistanceKm: 10}}
	saved, err := CalcCarbonSavingGrams(legs)
	assert.NoError(t, err)
	assert.InDelta(t, 1590.0, saved, 0.001)
}

func TestCalcCarbonSaving_Should_SumMultipleLegs(t *testing.T) {
	// walk 0.5km + mrt 9km + walk 0.5km
	legs := []Leg{
		{Mode: "walk", DistanceKm: 0.5},
		{Mode: "mrt", DistanceKm: 9},
		{Mode: "walk", DistanceKm: 0.5},
	}
	saved, err := CalcCarbonSavingGrams(legs)
	assert.NoError(t, err)
	// walk: (192-0)*0.5 = 96 (twice = 192) + mrt: (192-33)*9 = 1431 = 1623
	assert.InDelta(t, 1623.0, saved, 0.001)
}

func TestCalcCarbonSaving_Should_ReturnError_When_UnknownMode(t *testing.T) {
	_, err := CalcCarbonSavingGrams([]Leg{{Mode: "spaceship", DistanceKm: 1}})
	assert.ErrorIs(t, err, ErrUnknownMode)
}

func TestCalcCarbonSaving_Should_ReturnNegative_When_CarLegOnly(t *testing.T) {
	// Car vs car baseline = 0 saving
	saved, _ := CalcCarbonSavingGrams([]Leg{{Mode: "car", DistanceKm: 10}})
	assert.Equal(t, 0.0, saved)
}

func TestEquivalentTrees_Should_DivideBySequestrationRate(t *testing.T) {
	// 21,770 g/year/tree → 21770 g 應為 1 棵
	assert.InDelta(t, 1.0, EquivalentTrees(21770), 0.001)
	assert.InDelta(t, 0.5, EquivalentTrees(10885), 0.001)
}
