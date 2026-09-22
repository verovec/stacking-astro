package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// The observation-planning half left the fork (E01 card 0015). The Mosaic planner's dependencies are
// pinned KEPT via 405 method probes (routes exist; no handler runs on the bare test Server).
func TestPlannerEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
		want               int
	}{
		{"sky targets", http.MethodGet, "/api/sky/targets", http.StatusNotFound},
		{"sky events", http.MethodGet, "/api/sky/events", http.StatusNotFound},
		{"sky event series", http.MethodGet, "/api/sky/series", http.StatusNotFound},
		{"sky geocode", http.MethodGet, "/api/sky/geocode", http.StatusNotFound},
		{"sky point", http.MethodGet, "/api/sky/point", http.StatusNotFound},
		{"light pollution", http.MethodGet, "/api/sky/lightpollution", http.StatusNotFound},
		{"dark sites", http.MethodGet, "/api/sky/darksites", http.StatusNotFound},
		{"sky nights", http.MethodGet, "/api/sky/nights", http.StatusNotFound},
		{"canopy atlas", http.MethodGet, "/api/sky/canopy/atlas", http.StatusNotFound},
		{"solar system bodies", http.MethodGet, "/api/solarsystem/bodies", http.StatusNotFound},
		{"weather", http.MethodGet, "/api/sky/weather", http.StatusNotFound},
		{"sky search stays", http.MethodPost, "/api/sky/search", http.StatusMethodNotAllowed},
		{"sky starfield stays", http.MethodPost, "/api/sky/starfield", http.StatusMethodNotAllowed},
		{"mosaic plans stay", http.MethodPatch, "/api/mosaic/plans", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
