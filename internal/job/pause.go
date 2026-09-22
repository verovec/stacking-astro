package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/verove-jordan/astronomy/internal/pipeline"
	"github.com/verove-jordan/astronomy/internal/store"
)

// Resume phases — where a paused run picks up from.
const (
	phaseCompute = "compute" // paused mid-stack (manual pause); resume reuses per-channel masters on disk
)

// Pause causes — WHY a job paused. Only the manual cause remains now that the S3 transfer lanes (whose
// transient errors auto-paused jobs) left the fork: a pause is always the user's, resumed by Continue.
const (
	causeManual = "manual" // the user clicked Pause — stays paused until the user Continues
)

// pauseGate is a one-shot cooperative pause signal for one running job. Pause() flips it; the pipeline
// polls requested() at safe boundaries and stops when it is set.
type pauseGate struct{ flag atomic.Bool }

func (g *pauseGate) request()        { g.flag.Store(true) }
func (g *pauseGate) requested() bool { return g.flag.Load() }

// resumeCheckpoint is the job-layer view of a paused run, persisted as the job's `resume` JSONB. Phase says
// where to pick up; RunID/OutDir (compute phase only) let the run reuse the output dir so its already-
// stacked per-channel masters are found and skipped.
type resumeCheckpoint struct {
	Phase  string `json:"phase"`
	RunID  string `json:"run_id,omitempty"`
	OutDir string `json:"out_dir,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Cause is always "manual" now (user Pause; never auto-resumed).
	Cause string `json:"cause,omitempty"`
}

// pipelineResume maps the checkpoint to the pipeline's resume handle (nil unless we have a run id/dir to
// reuse — i.e. a compute-phase pause; a pull-phase pause computed nothing yet).
func (c resumeCheckpoint) pipelineResume() *pipeline.ResumeState {
	if c.RunID == "" && c.OutDir == "" {
		return nil
	}
	return &pipeline.ResumeState{RunID: c.RunID, OutDir: c.OutDir}
}

// Pause asks a running job to pause at its next safe boundary. The multi-channel deep-sky path stops after
// the current channel (its stacked master is kept on disk, so Continue reuses it); other modes finish the
// current compute first. Returns false if the job is not running in this process (queued/terminal → Cancel).
func (m *Manager) Pause(id int64) bool {
	m.mu.Lock()
	gate, ok := m.pauses[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	gate.request()
	m.publish(Event{JobID: id, Status: store.JobRunning, Step: "pause requested — will pause at the next safe point"})
	return true
}

// Continue resumes a paused job by re-enqueueing the SAME job id onto its lane. run() reads the job's
// resume checkpoint and picks up where it left off. Refuses a job that is not paused.
func (m *Manager) Continue(ctx context.Context, id int64) error {
	j, err := m.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if j.Status != store.JobPaused {
		return fmt.Errorf("job %d is %s, not paused", id, j.Status)
	}
	var req RunRequest
	if err := json.Unmarshal(j.Params, &req); err != nil {
		return fmt.Errorf("job %d has invalid params: %w", id, err)
	}
	select {
	case m.laneFor(req) <- id:
		return nil
	default:
		return fmt.Errorf("job queue is full")
	}
}

// laneFor picks the worker lane a run belongs to: the sequential lane for chained stacked jobs, else the
// main pool. Shared by Enqueue and Continue so a resumed job lands where a fresh one would.
func (m *Manager) laneFor(req RunRequest) chan int64 {
	if req.Sequential {
		return m.seqQueue
	}
	return m.queue
}

// pauseJob parks a running job in the resumable paused state with its checkpoint, keeping progress/step and
// (optionally) the result computed so far. Publishes a paused event so the UI swaps in the Continue action.
func (m *Manager) pauseJob(id int64, cp resumeCheckpoint, result json.RawMessage) {
	blob, err := json.Marshal(cp)
	if err != nil {
		log.Printf("astrostack: marshal resume checkpoint for job %d: %v", id, err)
		return
	}
	if err := m.store.SetJobPaused(context.Background(), id, blob, result, cp.Reason); err != nil {
		log.Printf("astrostack: pause job %d: %v", id, err)
		return
	}
	m.publish(Event{JobID: id, Status: store.JobPaused, Step: cp.Reason, Done: true})
}

// finishTerminal writes a terminal (failed/cancelled) status with a fresh context so it persists even if
// the run context was cancelled, publishes the done event, and closes any conversation turn. The error
// also lands in the live journal (and stdout) as a ✗ line — the Step field alone never reached the log.
func (m *Manager) finishTerminal(id int64, status string, err error) {
	_ = m.store.FinishJob(context.Background(), id, status, nil, err.Error())
	line := fmt.Sprintf("✗ job %s: %s", status, err.Error())
	m.publish(Event{JobID: id, Status: status, Line: line, Ts: time.Now().UnixMilli()})
	log.Printf("astrostack: job %d %s", id, line)
	m.publish(Event{JobID: id, Status: status, Step: err.Error(), Done: true})
	m.closeTurn(id, status, err.Error())
}

// finishSucceeded writes the successful terminal status + result, publishes done, closes the turn, and
// advances any agent series.
func (m *Manager) finishSucceeded(id int64, p RunRequest, result json.RawMessage) {
	_ = m.store.FinishJob(context.Background(), id, store.JobSucceeded, result, "")
	m.publish(Event{JobID: id, Status: store.JobSucceeded, Progress: 100, Step: "done", Done: true})
	m.closeTurn(id, store.JobSucceeded, "Finished — kept the best pass as the final image.")
	m.maybeContinueSeries(id, p)
}

// resultBlob marshals a pipeline result for persistence; nil stays nil (leave the stored result untouched).
// A json.RawMessage marshals back to its own bytes, so this also works to re-persist a prior result.
// A marshal failure means a non-finite float (encoding/json refuses NaN/±Inf): zero those and retry
// rather than silently storing no result at all — the empty-result half of task #353. (The deepsky
// path normally sanitizes in writeRunJSON already; this covers results that never pass through it,
// e.g. a cancelled run's partial result.)
func resultBlob(res any) json.RawMessage {
	if res == nil {
		return nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		if paths := pipeline.SanitizeNonFinite(res); len(paths) > 0 {
			log.Printf("job result contained non-finite numbers (zeroed): %d field(s)", len(paths))
			b, err = json.Marshal(res)
		}
		if err != nil {
			log.Printf("job result not serializable: %v", err)
			return nil
		}
	}
	return b
}
