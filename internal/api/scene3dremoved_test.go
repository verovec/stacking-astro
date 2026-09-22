package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// The run star-annotation overlay and the 3D scene left the fork (E01 card 0016).
func TestSceneAndStarsEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
		want               int
	}{
		{"compute stars", http.MethodPost, "/api/jobs/1/stars", http.StatusNotFound},
		{"get stars", http.MethodGet, "/api/jobs/1/stars", http.StatusNotFound},
		{"scene3d", http.MethodGet, "/api/jobs/1/scene3d", http.StatusNotFound},
		{"galaxy points", http.MethodGet, "/api/galaxy/points", http.StatusNotFound},
		// The job record itself stays reachable (405 = the route family survives, no handler ran).
		{"job detail stays", http.MethodPatch, "/api/jobs/1", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
