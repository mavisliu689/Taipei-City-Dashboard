// Package controllers eco hosts direct REST endpoints that wrap eco tools
// without going through the LLM. Used by FE for deterministic map rendering
// and POI listings that should not depend on LLM hallucination.
package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"TaipeiCityDashboardBE/app/services/ai/tools/eco"
)

type planRouteInput struct {
	Origin           string     `json:"origin"`
	Destination      string     `json:"destination"`
	MaxHopKm         float64    `json:"max_hop_km"`
	OriginCoord      *eco.Coord `json:"origin_coord"`
	DestinationCoord *eco.Coord `json:"destination_coord"`
}

// PlanEcoRoute is a thin REST wrapper around eco.PlanEcoRouteTool so the FE
// can reliably fetch structured route data (3 candidates) for map rendering.
// Accepts origin/destination as a name OR a {lat, lng} pair (used when user
// picks / drags points on the map).
func PlanEcoRoute(c *gin.Context) {
	var in planRouteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if in.Origin == "" && in.OriginCoord == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "origin or origin_coord required"})
		return
	}
	if in.Destination == "" && in.DestinationCoord == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "destination or destination_coord required"})
		return
	}
	if in.Origin == "" {
		in.Origin = "📍 自訂起點"
	}
	if in.Destination == "" {
		in.Destination = "🏁 自訂終點"
	}
	args, _ := json.Marshal(in)
	out, err := eco.PlanEcoRouteTool(context.Background(), string(args))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	var data interface{}
	_ = json.Unmarshal([]byte(out), &data)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

type findPoisInput struct {
	Center     *eco.LatLng `json:"center"`
	RadiusKm   float64     `json:"radius_km"`
	Categories []string    `json:"categories"`
	Districts  []string    `json:"districts"`
	Limit      int         `json:"limit"`
}

// FindEcoPOIs is a thin REST wrapper around eco.FindEcoPOIsTool.
func FindEcoPOIs(c *gin.Context) {
	var in findPoisInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	args, _ := json.Marshal(in)
	out, err := eco.FindEcoPOIsTool(context.Background(), string(args))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	var data interface{}
	_ = json.Unmarshal([]byte(out), &data)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}
