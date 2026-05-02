// Package eco poi loads the unified eco-point dataset and provides
// filtering for the find_eco_pois tool.
package eco

import (
	"encoding/json"
	"os"
	"sort"
)

// POI is one node in the eco-route graph. Lat/Lng may be nil (e.g. some
// 新北 recycling stations have no geocoded coordinates).
type POI struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Category string                 `json:"category"`
	Lat      *float64               `json:"lat"`
	Lng      *float64               `json:"lng"`
	Address  string                 `json:"address"`
	City     string                 `json:"city"`
	District string                 `json:"district"`
	Tags     []string               `json:"tags"`
	Extra    map[string]interface{} `json:"extra"`
	Source   string                 `json:"source"`
}

// LatLng is a simple coordinate pair used for query parameters.
type LatLng struct {
	Lat float64
	Lng float64
}

// FindOptions configures FindEcoPOIs.
type FindOptions struct {
	Center     *LatLng
	RadiusKm   float64
	Categories []string
	Districts  []string // 若有, 只回傳 District 完全等於其中之一的 POI
	Limit      int
}

// LoadPOIs reads the unified mock dataset (or any compatible JSON array).
func LoadPOIs(path string) ([]POI, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pts []POI
	if err := json.Unmarshal(raw, &pts); err != nil {
		return nil, err
	}
	return pts, nil
}

// FindEcoPOIs filters and ranks POIs by distance from center.
//
// Radius is hard-capped at 2 km to avoid LLM passing huge radii (e.g. 10 km)
// that would sweep across the whole city for queries like "沿途有什麼回收站".
func FindEcoPOIs(pts []POI, opt FindOptions) []POI {
	if opt.RadiusKm > 2.0 {
		opt.RadiusKm = 2.0
	}
	type scored struct {
		POI  POI
		Dist float64
	}
	var matched []scored

	catSet := make(map[string]bool, len(opt.Categories))
	for _, c := range opt.Categories {
		catSet[c] = true
	}
	distSet := make(map[string]bool, len(opt.Districts))
	for _, d := range opt.Districts {
		distSet[d] = true
	}

	for _, p := range pts {
		if len(catSet) > 0 && !catSet[p.Category] {
			continue
		}
		if len(distSet) > 0 {
			// park 的 p.District 存「里」名 (例「長安里」), 必須先還原成「區」
			// 才能跟 LLM 傳入的 districts=["XX區"] 比對
			resolved := resolveDistrict(p)
			if resolved == "" || !distSet[resolved] {
				continue
			}
		}
		// 需要距離過濾或排序時，跳過缺座標的點
		if (opt.Center != nil || opt.RadiusKm > 0) && (p.Lat == nil || p.Lng == nil) {
			continue
		}
		dist := 0.0
		if opt.Center != nil {
			dist = Haversine(opt.Center.Lat, opt.Center.Lng, *p.Lat, *p.Lng)
			if opt.RadiusKm > 0 && dist > opt.RadiusKm {
				continue
			}
		}
		matched = append(matched, scored{POI: p, Dist: dist})
	}

	if opt.Center != nil {
		sort.Slice(matched, func(i, j int) bool {
			return matched[i].Dist < matched[j].Dist
		})
	}

	limit := opt.Limit
	if limit <= 0 || limit > len(matched) {
		limit = len(matched)
	}
	out := make([]POI, limit)
	for i := 0; i < limit; i++ {
		out[i] = matched[i].POI
	}
	return out
}
