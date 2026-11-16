package repository

import (
	_ "embed"
	"encoding/json"
	"log"
	"math"
	"sync"
)

// chonburiCentroidsRaw embeds the 92-subdistrict centroid lookup table at compile time (Zero I/O, Zero network).
//
//go:embed assets/chonburi_subdistrict_centroids.json
var chonburiCentroidsRaw []byte

// subdistrictCentroid holds the geographic centroid of a subdistrict in Chonburi province.
type subdistrictCentroid struct {
	Subdistrict string  `json:"subdistrict"`
	District    string  `json:"district"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
}

// spatialResolver resolves a geographic coordinate (lat, lon) to the nearest
// (subdistrict, district) pair using Haversine nearest-centroid algorithm.
// It is initialized once at startup and is safe for concurrent use.
type spatialResolver struct {
	centroids []subdistrictCentroid
	once      sync.Once
}

var globalSpatialResolver = &spatialResolver{}

func init() {
	globalSpatialResolver.load()
}

// load parses the embedded centroids JSON exactly once.
func (s *spatialResolver) load() {
	s.once.Do(func() {
		var list []subdistrictCentroid
		if err := json.Unmarshal(chonburiCentroidsRaw, &list); err != nil {
			log.Printf("❌ [SpatialResolver] Failed to parse chonburi_subdistrict_centroids.json: %v", err)
			return
		}
		s.centroids = list
		log.Printf("✅ [SpatialResolver] Loaded %d Chonburi subdistrict centroids (Zero I/O, In-memory)", len(list))
	})
}

// Resolve returns the nearest (subdistrict, district) for the given (lat, lon).
// If no centroids are loaded or coords are invalid, it returns ("ชลบุรี", "ชลบุรี") as a safe fallback.
//
// Performance: O(n) over 92 entries ≈ sub-millisecond per call.
func (s *spatialResolver) Resolve(lat, lon float64) (subdistrict, district string) {
	if len(s.centroids) == 0 || lat == 0 || lon == 0 {
		return "ชลบุรี", "ชลบุรี"
	}

	minDist := math.MaxFloat64
	best := s.centroids[0]

	for _, c := range s.centroids {
		d := haversineKm(lat, lon, c.Lat, c.Lon)
		if d < minDist {
			minDist = d
			best = c
		}
	}

	return best.Subdistrict, best.District
}

// ResolveSubdistrict exposes Resolve globally across repositories and use cases
func ResolveSubdistrict(lat, lon float64) (string, string) {
	return globalSpatialResolver.Resolve(lat, lon)
}

// haversineKm calculates the great-circle distance in kilometres between two geographic coordinates.
// Uses Earth radius = 6371 km (WGS-84 mean).
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

