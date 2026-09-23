package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/fits"
)

// readPlaneConst reads a single-plane FITS written from a constant and returns that constant.
func readPlaneConst(t *testing.T, path string) float64 {
	t.Helper()
	im, err := fits.ReadImage(path)
	require.NoError(t, err)
	require.NotEmpty(t, im.Pix)
	require.NotEmpty(t, im.Pix[0])
	return float64(im.Pix[0][0])
}

// TestDualSetChannels is the second half of the card: once the dual-band master has been registered
// onto the broadband grid, it stops being a "colour channel" and becomes the two emission channels
// it always was.
//
// The broadband lane survives untouched as the base — the runbook's "broadband base + emission
// screens" composite — and the dual-band lane DISAPPEARS from the map. Leaving it in would offer
// resolvePalette a second colour channel it has no meaning for, and the finish would have to guess
// which of the two was the real RGB.
func TestDualSetChannels(t *testing.T) {
	dir := t.TempDir()
	writeColourMaster(t, dir, "aligned_RGB", 0.50, 0.50, 0.50)
	// Dual-band: Hα on red, [OIII] split across green/blue with green the stronger.
	writeColourMaster(t, dir, "aligned_RGB-dualband", 0.40, 0.20, 0.10)
	channels := map[string]string{
		filters.Color:                      "aligned_RGB",
		filters.Color + dualbandLaneSuffix: "aligned_RGB-dualband",
	}

	out, note := dualSetChannels(channels, dir)

	require.NotEmpty(t, note, "a split this consequential is never silent")
	assert.Equal(t, "aligned_RGB", out[filters.Color], "the broadband base is untouched")
	assert.NotContains(t, out, filters.Color+dualbandLaneSuffix, "the lane is consumed, not left behind")
	assert.Equal(t, duobandHaBase, out["Ha"])
	assert.Equal(t, duobandOIIIBase, out["OIII"])
	assert.FileExists(t, filepath.Join(dir, duobandHaBase+".fits"))
	assert.FileExists(t, filepath.Join(dir, duobandOIIIBase+".fits"))

	// The caller's map is never mutated — every other channel helper in this package keeps that
	// contract, and finishAligned's has/cpath closures capture the variable.
	assert.Contains(t, channels, filters.Color+dualbandLaneSuffix)
}

// TestDualSetChannels_SplitsTheREGISTEREDMaster pins WHICH file is split. The dual-band master has
// to be separated AFTER it lands on the broadband grid: pseudo-Hα and pseudo-[OIII] are composited
// pixel-for-pixel against the broadband base, so splitting the unregistered master would bake the
// two lanes' pointing difference into the emission layers — the exact misalignment the master-level
// registration exists to remove.
func TestDualSetChannels_SplitsTheREGISTEREDMaster(t *testing.T) {
	dir := t.TempDir()
	writeColourMaster(t, dir, "aligned_RGB", 0.50, 0.50, 0.50)
	writeColourMaster(t, dir, "aligned_RGB-dualband", 0.40, 0.20, 0.10) // registered
	writeColourMaster(t, dir, "master_RGB-dualband", 0.99, 0.99, 0.99)  // unregistered — must be ignored

	out, _ := dualSetChannels(map[string]string{
		filters.Color:                      "aligned_RGB",
		filters.Color + dualbandLaneSuffix: "aligned_RGB-dualband",
	}, dir)

	ha := readPlaneConst(t, filepath.Join(dir, out["Ha"]+".fits"))
	assert.InDelta(t, 0.40, ha, 1e-6, "Hα came from the registered master, not the raw one")
}

// TestDualSetChannels_NoDualbandLaneIsANoOp: every capture that did not split — mono, broadband-only,
// dual-band-only, anything unclassified — must come through with the map it already had.
func TestDualSetChannels_NoDualbandLaneIsANoOp(t *testing.T) {
	dir := t.TempDir()
	channels := map[string]string{"L": "aligned_L", "R": "aligned_R"}

	out, note := dualSetChannels(channels, dir)

	assert.Empty(t, note)
	assert.Equal(t, channels, out)
}

// TestDualSetChannels_UnsplittableMasterDegrades: a dual-band lane whose master cannot be read or is
// not three-plane must not sink the run. The broadband base is a complete image on its own, so the
// honest degradation is to finish on it and say why.
func TestDualSetChannels_UnsplittableMasterDegrades(t *testing.T) {
	dir := t.TempDir()
	writeColourMaster(t, dir, "aligned_RGB", 0.5, 0.5, 0.5)
	channels := map[string]string{
		filters.Color:                      "aligned_RGB",
		filters.Color + dualbandLaneSuffix: "missing_master",
	}

	out, note := dualSetChannels(channels, dir)

	assert.Contains(t, note, "skipped")
	assert.Equal(t, "aligned_RGB", out[filters.Color], "the broadband base still finishes")
	assert.NotContains(t, out, "Ha")
	assert.NotContains(t, out, filters.Color+dualbandLaneSuffix, "a lane that cannot be split is still not a colour channel")
}

// TestDualSetChannels_NoBroadbandBaseKeepsTheColourRun is the guard for a half-failed split. If the
// broadband lane produced no master — too few survivors, a corrupt group, a failed stack — the
// dual-band lane is all the run has left.
//
// Splitting it then would be the worst outcome available: the map would hold Ha and OIII and NO
// colour channel, so the finish would have nothing to screen the emission lines ONTO. The honest
// answer is that this run is now an ordinary dual-band capture, which the pipeline has always known
// how to finish — so the lane goes back to being the colour channel and duobandChannels decides the
// split on the palette, exactly as it does for an unsplit capture.
func TestDualSetChannels_NoBroadbandBaseKeepsTheColourRun(t *testing.T) {
	dir := t.TempDir()
	writeColourMaster(t, dir, "aligned_RGB-dualband", 0.40, 0.20, 0.10)

	out, note := dualSetChannels(map[string]string{
		filters.Color + dualbandLaneSuffix: "aligned_RGB-dualband",
	}, dir)

	assert.Equal(t, "aligned_RGB-dualband", out[filters.Color],
		"the surviving lane becomes the colour channel")
	assert.NotContains(t, out, filters.Color+dualbandLaneSuffix)
	assert.NotContains(t, out, "Ha", "nothing to screen onto — do not pre-split")
	assert.Contains(t, note, "broadband")
}
