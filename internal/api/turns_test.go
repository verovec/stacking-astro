package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/turns"
)

func doReq(s *Server, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	s.Handler().ServeHTTP(rec, r)
	return rec
}

func TestAgentTurnEvents_StreamsBacklogToDone(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}

	// Unknown turn → a single terminal "done" frame so the client stops cleanly.
	rec := doReq(s, http.MethodGet, "/api/agent/turns/nope/events", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"kind":"done"`)

	// Known turn: the backlog published before the subscriber arrived streams in order, then done.
	id := s.agentTurns.Start()
	s.agentTurns.Publish(id, turns.Event{Kind: "thinking", Text: "measuring"})
	s.agentTurns.Publish(id, turns.Event{Kind: "final", Text: "kept iteration 2"})
	s.agentTurns.Finish(id)
	body := doReq(s, http.MethodGet, "/api/agent/turns/"+id+"/events", "").Body.String()
	thinking := strings.Index(body, `"kind":"thinking"`)
	final := strings.Index(body, `"kind":"final"`)
	done := strings.Index(body, `"kind":"done"`)
	require.NotEqual(t, -1, thinking)
	require.NotEqual(t, -1, final)
	require.NotEqual(t, -1, done)
	assert.Less(t, thinking, final)
	assert.Less(t, final, done)
}

func TestAgentTurnConfirm_ResolvesPendingAsk(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}

	// Unknown turn / call → ok:false (nothing pending).
	rec := doReq(s, http.MethodPost, "/api/agent/turns/nope/confirm", `{"call_id":"c1","approve":true}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":false`)

	// A pending Await is resolved by the endpoint with the caller's approval + choice.
	id := s.agentTurns.Start()
	got := make(chan struct {
		approve bool
		choice  string
		ok      bool
	}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		approve, choice, ok := s.agentTurns.Await(ctx, id, "c1")
		got <- struct {
			approve bool
			choice  string
			ok      bool
		}{approve, choice, ok}
	}()
	require.Eventually(t, func() bool {
		rec := doReq(s, http.MethodPost, "/api/agent/turns/"+id+"/confirm", `{"call_id":"c1","approve":true,"choice":"tier_a"}`)
		return strings.Contains(rec.Body.String(), `"ok":true`)
	}, 2*time.Second, 10*time.Millisecond, "confirm should resolve once the Await is registered")
	r := <-got
	assert.True(t, r.ok)
	assert.True(t, r.approve)
	assert.Equal(t, "tier_a", r.choice)
}

func TestAgentTurnMessage_Steers(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}

	// Unknown turn → ok:false (nothing to steer).
	rec := doReq(s, http.MethodPost, "/api/agent/turns/nope/message", `{"text":"more saturation"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":false`)

	// Known turn → ok:true, and the mailbox carries the nudge + the sticky stop for the loop to drain.
	id := s.agentTurns.Start()
	rec = doReq(s, http.MethodPost, "/api/agent/turns/"+id+"/message", `{"text":"boost saturation","stop":true}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)
	texts, stop := s.agentTurns.DrainMessages(id)
	assert.Equal(t, []string{"boost saturation"}, texts)
	assert.True(t, stop)
}

func TestAgentChatEndpoints_Removed(t *testing.T) {
	s := &Server{agentTurns: turns.NewSessions()}
	tests := []struct {
		name, method, path string
	}{
		{"chat", http.MethodPost, "/api/agent/chat"},
		{"status", http.MethodGet, "/api/agent/status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(s, tt.method, tt.path, `{}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}
