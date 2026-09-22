package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// Camera polar alignment left the fork (E01 card 0013); a stale client must get a 404.
func TestPolarEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
	}{
		{"start", http.MethodPost, "/api/capture/polar/start"},
		{"rough", http.MethodPost, "/api/capture/polar/rough"},
		{"next", http.MethodPost, "/api/capture/polar/next"},
		{"adjust", http.MethodPost, "/api/capture/polar/adjust"},
		{"refresh", http.MethodPost, "/api/capture/polar/refresh"},
		{"stop", http.MethodPost, "/api/capture/polar/stop"},
		{"status", http.MethodGet, "/api/capture/polar"},
		{"sky polar reticle", http.MethodGet, "/api/sky/polar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}
