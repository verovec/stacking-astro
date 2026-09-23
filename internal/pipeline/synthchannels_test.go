package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// TestRegisterSynthesizedChannels closes a gap the card names: syn_Ha/syn_OIII were written to disk
// and handed to the finish, but never recorded in res.Channels. Nothing downstream that reads the
// RESULT — the refine panel, a Tier-A re-mix, run.json — could see that the run had emission
// channels at all, so a mixed capture could be finished once and never re-tuned.
func TestRegisterSynthesizedChannels(t *testing.T) {
	tests := []struct {
		name     string
		prior    []ChannelResult
		channels map[string]string
		want     []string // filters expected in res.Channels afterwards
	}{
		{
			name:     "the emission channels join the broadband base",
			prior:    []ChannelResult{{Filter: filters.Color, Object: "NGC7000", StackedFrames: 40}},
			channels: map[string]string{filters.Color: "aligned_RGB", "Ha": duobandHaBase, "OIII": duobandOIIIBase},
			want:     []string{filters.Color, "Ha", "OIII"},
		},
		{
			// A run that never synthesized anything must keep exactly the channels it stacked.
			name:     "an ordinary run is untouched",
			prior:    []ChannelResult{{Filter: "L"}, {Filter: "R"}},
			channels: map[string]string{"L": "aligned_L", "R": "aligned_R"},
			want:     []string{"L", "R"},
		},
		{
			// Idempotent: a re-entry that re-runs the finish must not append a second Ha.
			name: "already registered — no duplicate",
			prior: []ChannelResult{
				{Filter: filters.Color}, {Filter: "Ha", Synthesized: true},
			},
			channels: map[string]string{filters.Color: "aligned_RGB", "Ha": duobandHaBase},
			want:     []string{filters.Color, "Ha"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &Result{Channels: tt.prior}

			registerSynthesizedChannels(res, tt.channels)

			got := make([]string, 0, len(res.Channels))
			for _, ch := range res.Channels {
				got = append(got, ch.Filter)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

// TestRegisterSynthesizedChannels_MarksThemSynthesized: a pseudo-Hα separated from a colour master is
// not the same thing as an Hα master stacked from frames shot through a 3 nm filter. Anything that
// reports integration time or frame counts has to be able to tell them apart, or a mixed run would
// claim narrowband integration it never captured.
func TestRegisterSynthesizedChannels_MarksThemSynthesized(t *testing.T) {
	res := &Result{Channels: []ChannelResult{{Filter: filters.Color, StackedFrames: 40, ExposureMs: 120_000}}}

	registerSynthesizedChannels(res, map[string]string{
		filters.Color: "aligned_RGB", "Ha": duobandHaBase, "OIII": duobandOIIIBase,
	})

	for _, ch := range res.Channels {
		if ch.Filter == filters.Color {
			continue
		}
		assert.True(t, ch.Synthesized, "%s is derived, not captured", ch.Filter)
		assert.Zero(t, ch.StackedFrames, "%s stacked no frames of its own", ch.Filter)
		assert.Zero(t, ch.ExposureMs, "%s has no exposure of its own", ch.Filter)
	}
}

// TestReconstructChannelsFromDisk_FindsSynthesized: registering the channel is only half of it. The
// refine path rebuilds the channel map from the FILES, and a synthesized channel is written as
// syn_<tag> — none of the combine_/aligned_/master_ prefixes it used to look for.
func TestReconstructChannelsFromDisk_FindsSynthesized(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aligned_RGB.fits"), []byte("f"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, duobandHaBase+".fits"), []byte("f"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, duobandOIIIBase+".fits"), []byte("f"), 0o644))

	got := reconstructChannelsFromDisk(dir, []ChannelResult{
		{Filter: filters.Color}, {Filter: "Ha", Synthesized: true}, {Filter: "OIII", Synthesized: true},
	})

	assert.Equal(t, "aligned_RGB", got[filters.Color])
	assert.Equal(t, duobandHaBase, got["Ha"])
	assert.Equal(t, duobandOIIIBase, got["OIII"])
}

// A real Ha master must still win over a synthesized one of the same name — a mono narrowband run
// writes master_Ha.fits, and that is captured data, not a separation.
func TestReconstructChannelsFromDisk_RealMasterBeatsSynthesized(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aligned_Ha.fits"), []byte("f"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, duobandHaBase+".fits"), []byte("f"), 0o644))

	got := reconstructChannelsFromDisk(dir, []ChannelResult{{Filter: "Ha"}})

	assert.Equal(t, "aligned_Ha", got["Ha"], "captured data outranks a separation")
}
