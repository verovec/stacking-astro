package starnet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RemoteRequest is the JSON body the engine POSTs to the host StarNet service (cmd/starnet-host). Only
// paths + knobs travel: the engine and the host share the same absolute file paths (the compose bind
// mounts), so the TIFF bytes never cross the wire. Exported so cmd/starnet-host shares the wire type.
//
// The CLI generation is deliberately NOT part of the request: the host service owns its own binary and
// probes it (STARNET_CLI / the --machine-info auto-probe), so the engine never has to know which
// StarNet it is talking to.
type RemoteRequest struct {
	In     string `json:"in"`
	Out    string `json:"out"`
	Stride int    `json:"stride,omitempty"`
}

// ResultPrefix marks the final line of a /run stream: "<prefix>ok" on success, "<prefix>error:<msg>"
// otherwise. StarNet's own output lines never start with it, so it unambiguously terminates the stream
// (a plain EOF without it means the service died mid-run).
const ResultPrefix = "__STARNET_RESULT__:"

// errPrefix separates a failed result from "ok" inside a ResultPrefix line.
const errPrefix = "error:"

// ResultLine renders the terminal line of a /run stream for the given outcome — cmd/starnet-host writes
// exactly what this returns. The message is QUOTED because a StarNet failure carries its whole captured
// log: raw, its newlines would spill past the sentinel and the client would re-read the tail of the
// diagnosis as progress lines, losing the one thing that explains the failure.
func ResultLine(err error) string {
	if err == nil {
		return ResultPrefix + "ok"
	}
	return ResultPrefix + errPrefix + strconv.Quote(err.Error())
}

// resultError decodes what ResultLine encoded, tolerating an unquoted payload so a hand-written or
// older service still produces a readable error rather than a mangled one.
func resultError(payload string) error {
	msg := strings.TrimPrefix(payload, errPrefix)
	if unquoted, err := strconv.Unquote(msg); err == nil {
		msg = unquoted
	}
	return fmt.Errorf("host StarNet: %s", msg)
}

// remotePing checks the host service is reachable (used by Available in offload mode). Short timeout:
// /health is a cheap binary lookup on the host, not a star-removal pass.
func (r *Runner) remotePing(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("host StarNet service %s unreachable (run `just run-starnet-service`): %w", r.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("host StarNet service unhealthy: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

// runRemote POSTs one star removal to the host service and streams its StarNet output back through the
// same onProgress contract as a local run, so the job log looks identical whether StarNet ran locally or
// on the host. The passed ctx bounds the whole call (a large frame takes minutes) — there is no client
// timeout, matching the local exec path.
func (r *Runner) runRemote(ctx context.Context, rr RemoteRequest, onProgress func(Progress)) error {
	body, err := json.Marshal(rr)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/run", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("host StarNet %s: %w", r.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("host StarNet (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var result error
	gotResult := false
	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := strings.CutPrefix(line, ResultPrefix); ok {
			gotResult = true
			if payload != "ok" {
				result = resultError(payload)
			}
			continue
		}
		if onProgress != nil {
			onProgress(Progress{Line: line, Percent: parsePercent(line)})
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("host StarNet stream: %w", err)
	}
	if !gotResult {
		return fmt.Errorf("host StarNet: connection closed before the run completed")
	}
	return result
}
