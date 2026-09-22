package job

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/pipeline"
)

func TestResumeCheckpoint_PipelineResume(t *testing.T) {
	compute := resumeCheckpoint{Phase: phaseCompute, RunID: "20260101_120000", OutDir: "/out/M101/20260101_120000"}
	got := compute.pipelineResume()
	require.NotNil(t, got)
	assert.Equal(t, &pipeline.ResumeState{RunID: "20260101_120000", OutDir: "/out/M101/20260101_120000"}, got)

	assert.Nil(t, resumeCheckpoint{Phase: phaseCompute}.pipelineResume(), "no run id/dir yet → nothing to reuse")
}

// laneFor must route a resumed job to the same worker lane a fresh one would use, so Continue can't land a
// sequential job in the wrong pool.
func TestLaneFor(t *testing.T) {
	m := &Manager{
		queue:    make(chan int64, 1),
		seqQueue: make(chan int64, 1),
	}
	assert.Equal(t, m.queue, m.laneFor(RunRequest{}), "default run → main lane")
	assert.Equal(t, m.seqQueue, m.laneFor(RunRequest{Sequential: true}), "sequential run → seq lane")
}

// resultBlob keeps a prior result's bytes intact (json.RawMessage round-trips) and maps nil → nil so a
// compute-phase pause leaves the stored result untouched.
func TestResultBlob(t *testing.T) {
	assert.Nil(t, resultBlob(nil))
	raw := json.RawMessage(`{"object":"M101","run_id":"x"}`)
	assert.JSONEq(t, string(raw), string(resultBlob(raw)))
}
