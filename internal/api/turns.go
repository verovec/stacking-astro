package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/verove-jordan/astronomy/internal/turns"
)

// agentTurnEvents streams a turn's steps (thinking / tool calls / results / confirmation requests /
// final answer) to the browser over SSE, backlog-first so a late reader sees the whole run. Turns are
// produced by the job manager's supervised finishes. GET /api/agent/turns/{id}/events
func (s *Server) agentTurnEvents(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		serverError(w, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	backlog, live, cancel, ok := s.agentTurns.Subscribe(turnID)
	if !ok {
		sendTurnEvent(w, flusher, turns.Event{Kind: "done"}) // unknown/expired turn → let the client stop
		return
	}
	defer cancel()
	for _, e := range backlog {
		sendTurnEvent(w, flusher, e)
		if e.Kind == "done" {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-live:
			if !open {
				return
			}
			sendTurnEvent(w, flusher, e)
			if e.Kind == "done" {
				return
			}
		}
	}
}

// agentTurnConfirm delivers the user's answer to a pending confirmation/choice, unblocking the turn's
// loop. POST /api/agent/turns/{id}/confirm  {call_id, approve, choice?}
func (s *Server) agentTurnConfirm(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("id")
	var body struct {
		CallID  string `json:"call_id"`
		Approve bool   `json:"approve"`
		Choice  string `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	ok := s.agentTurns.Resolve(turnID, body.CallID, body.Approve, body.Choice)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
}

// agentTurnMessage delivers a free-text nudge and/or a stop request to a running turn — used to steer a
// supervised finish between iterations ("boost saturation" / "stop, keep this one"). Unlike confirm it
// never blocks: the producer drains the mailbox on its own schedule.
// POST /api/agent/turns/{id}/message  {text?, stop?}
func (s *Server) agentTurnMessage(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("id")
	var body struct {
		Text string `json:"text"`
		Stop bool   `json:"stop"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	ok := s.agentTurns.PostMessage(turnID, body.Text, body.Stop)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
}

// sendTurnEvent writes one SSE frame for a turn event (mirrors sendEvent for job events).
func sendTurnEvent(w http.ResponseWriter, f http.Flusher, e turns.Event) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}
