package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// The acquisition side (capture/devices/mount/guiding/logbook) left the fork (E01 card 0014); stale
// clients must get a 404. Kept neighbours are pinned alive so a route-block trim can't overshoot.
func TestCaptureEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
		want               int
	}{
		{"capture start", http.MethodPost, "/api/capture/start", http.StatusNotFound},
		{"capture status", http.MethodGet, "/api/capture/status", http.StatusNotFound},
		{"capture sessions", http.MethodGet, "/api/capture/sessions", http.StatusNotFound},
		{"capture sequences", http.MethodGet, "/api/capture/sequences", http.StatusNotFound},
		{"calibration plan", http.MethodPost, "/api/capture/calibration/plan", http.StatusNotFound},
		{"filter slots", http.MethodGet, "/api/capture/filters", http.StatusNotFound},
		{"tracking report", http.MethodGet, "/api/tracking/report/1", http.StatusNotFound},
		{"tracking sessions", http.MethodGet, "/api/tracking/sessions", http.StatusNotFound},
		{"goto align stars", http.MethodGet, "/api/sky/align", http.StatusNotFound},
		{"device status", http.MethodGet, "/api/device/status", http.StatusNotFound},
		// Kept neighbours, probed with a wrong method so no handler runs on the bare test Server:
		// 405 proves the route is still registered (equipment = E02 rig fields; align-points = a
		// planetary stacking knob that merely shares the word "align").
		{"equipment stays", http.MethodPatch, "/api/equipment", http.StatusMethodNotAllowed},
		{"planetary align-points stays", http.MethodGet, "/api/planetary/align-points", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
