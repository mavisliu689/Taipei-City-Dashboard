// Package eco hosts AI tools for the carbon-reduction route assistant.
package eco

import "math"

const earthRadiusKm = 6371.0

// Haversine returns the great-circle distance in kilometres between two
// (lat, lng) pairs in decimal degrees.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	lat1Rad := degToRad(lat1)
	lat2Rad := degToRad(lat2)
	dLat := degToRad(lat2 - lat1)
	dLng := degToRad(lng2 - lng1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

func degToRad(deg float64) float64 {
	return deg * math.Pi / 180
}
