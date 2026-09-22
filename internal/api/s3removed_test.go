package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// The S3 mirroring / transfer / backup surface left the fork (E01 card 0012); a stale client must get
// a 404, not a half-alive handler.
func TestS3AndBackupEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
	}{
		{"s3 status", http.MethodGet, "/api/s3/status"},
		{"s3 transfer", http.MethodPost, "/api/s3/transfer"},
		{"s3 browse", http.MethodGet, "/api/s3/browse"},
		{"s3 import", http.MethodPost, "/api/s3/import"},
		{"s3 connections", http.MethodGet, "/api/s3/connections"},
		{"s3 manage objects", http.MethodGet, "/api/s3/manage/objects"},
		{"backup create", http.MethodPost, "/api/backup"},
		{"backup list", http.MethodGet, "/api/backup"},
		{"backup restore", http.MethodPost, "/api/backup/restore"},
		{"backup appstate", http.MethodGet, "/api/backup/appstate"},
		{"library s3 sync", http.MethodPost, "/api/library/s3-sync"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}
