package pipeline

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/verove-jordan/astronomy/internal/mode"
	"github.com/verove-jordan/astronomy/internal/postprocess"
	"github.com/verove-jordan/astronomy/internal/starnet"
	"golang.org/x/image/tiff"
)

// fakeStarnet writes a stub StarNet executable that copies its (positional) input to its output and
// appends one line per invocation to a log, so the tier plumbing is exercised — including the
// "exactly one star-removal pass" contract — without the real, GPU-bound tool.
func fakeStarnet(t *testing.T, dir string) (bin, callLog string) {
	t.Helper()
	bin = filepath.Join(dir, "fake-starnet")
	callLog = filepath.Join(dir, "starnet-calls.log")
	script := "#!/bin/sh\necho run >> " + callLog + "\nexec cp \"$1\" \"$2\"\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin, callLog
}

// starnetCalls counts the recorded invocations of the stub (0 when it never ran).
func starnetCalls(t *testing.T, callLog string) int {
	t.Helper()
	b, err := os.ReadFile(callLog)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return len(strings.Fields(string(b)))
}

// writeFinalTIFF writes a small 16-bit TIFF (and a matching PNG) standing in for the finished
// composite the tier set is derived from.
func writeFinalTIFF(t *testing.T, path string) {
	t.Helper()
	img := image.NewGray16(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetGray16(x, y, color.Gray16{Y: uint16(1000 * (x + 1))})
		}
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, tiff.Encode(f, img, nil))
	require.NoError(t, f.Close())
}

// tierOpts is an Options wired with the stub StarNet and star tiers enabled.
func tierOpts(bin string, tiers bool) Options {
	return Options{
		Preset:  &mode.Preset{StarTiers: tiers},
		Starnet: starnet.NewVariant(bin, starnet.VariantPositional),
	}
}

// finalResult is a Result whose finish already produced final.{tif,png} in outDir.
func finalResult(t *testing.T, outDir string) *Result {
	t.Helper()
	writeFinalTIFF(t, filepath.Join(outDir, "final.tif"))
	img, err := postprocess.DecodeImage(filepath.Join(outDir, "final.tif"))
	require.NoError(t, err)
	require.NoError(t, postprocess.WritePNG(filepath.Join(outDir, "final.png"), img))
	return &Result{Final: &postprocess.Result{
		Mode:    "mono",
		Outputs: []string{filepath.Join(outDir, "final.xcf"), filepath.Join(outDir, "final.tif"), filepath.Join(outDir, "final.png")},
	}}
}

func TestEmitStarTiers_NamesAndResult(t *testing.T) {
	dir := t.TempDir()
	bin, callLog := fakeStarnet(t, dir)
	outDir := t.TempDir()
	res := finalResult(t, outDir)

	emitStarTiers(context.Background(), tierOpts(bin, true), res, outDir)

	// The five star-presence deliverables, by exact name.
	for _, name := range []string{"final.png", "final-starless.png", "final-25-stars.png", "final-50-stars.png", "final-75-stars.png"} {
		assert.FileExists(t, filepath.Join(outDir, name), "deliverable %s", name)
	}
	assert.FileExists(t, filepath.Join(outDir, "final-starless.tif"), "starless TIFF kept for the GIMP blend / 3D backdrop")

	// One typed entry per star level, ascending, 0 = starless … 100 = the untouched final.
	require.Len(t, res.Final.StarTiers, 5)
	var percents []int
	for _, st := range res.Final.StarTiers {
		percents = append(percents, st.Percent)
		assert.FileExists(t, st.Png)
	}
	assert.Equal(t, []int{0, 25, 50, 75, 100}, percents)
	assert.Equal(t, "starless", res.Final.StarTiers[0].Kind)
	assert.Equal(t, "stars", res.Final.StarTiers[1].Kind)
	assert.Equal(t, "final", res.Final.StarTiers[4].Kind)
	assert.Equal(t, filepath.Join(outDir, "final.png"), res.Final.StarTiers[4].Png, "100% is the existing final, not a copy")

	// The viewer contract: the FIRST .png in Outputs is still the hero final.
	var firstPng string
	for _, o := range res.Final.Outputs {
		if strings.HasSuffix(o, ".png") {
			firstPng = o
			break
		}
	}
	assert.Equal(t, filepath.Join(outDir, "final.png"), firstPng)
	assert.Contains(t, res.Final.Outputs, filepath.Join(outDir, "final-50-stars.png"), "tier files are downloadable")
	assert.Empty(t, res.Warnings)
	assert.Equal(t, 1, starnetCalls(t, callLog), "exactly one star-removal pass per run")
}

// TestEmitStarTiers_ReusesExistingStarless pins the shared single pass: when the star_reduce blend
// already produced a starless for THIS final, the tier set reuses it instead of re-running StarNet.
func TestEmitStarTiers_ReusesExistingStarless(t *testing.T) {
	dir := t.TempDir()
	bin, callLog := fakeStarnet(t, dir)
	outDir := t.TempDir()
	res := finalResult(t, outDir)
	// Simulate reduceStarsAI having just produced the starless from this same final.
	writeFinalTIFF(t, filepath.Join(outDir, "final-starless.tif"))

	emitStarTiers(context.Background(), tierOpts(bin, true), res, outDir)

	assert.Equal(t, 0, starnetCalls(t, callLog), "the fresh starless is reused, StarNet is not re-run")
	assert.FileExists(t, filepath.Join(outDir, "final-50-stars.png"))
	require.Len(t, res.Final.StarTiers, 5)
}

// TestEmitStarTiers_NoDuplicateOutputs pins that the shared starless TIFF is listed ONCE even when
// the star-reduction blend already registered it — a duplicate means a doubled download entry and a
// doubled S3 upload of a full-size render.
func TestEmitStarTiers_NoDuplicateOutputs(t *testing.T) {
	dir := t.TempDir()
	bin, _ := fakeStarnet(t, dir)
	outDir := t.TempDir()
	res := finalResult(t, outDir)
	starlessTif := filepath.Join(outDir, "final-starless.tif")
	writeFinalTIFF(t, starlessTif)
	// reduceStarsAI lists the starless it produced before the deferred tier set runs.
	res.Final.Outputs = append(res.Final.Outputs, starlessTif)

	emitStarTiers(context.Background(), tierOpts(bin, true), res, outDir)

	count := 0
	for _, o := range res.Final.Outputs {
		if o == starlessTif {
			count++
		}
	}
	assert.Equal(t, 1, count, "the shared starless TIFF is listed once: %v", res.Final.Outputs)
}

func TestEmitStarTiers_NoStarnetKeepsFinalOnly(t *testing.T) {
	outDir := t.TempDir()
	res := finalResult(t, outDir)
	before, err := os.ReadFile(filepath.Join(outDir, "final.png"))
	require.NoError(t, err)
	outputsBefore := append([]string(nil), res.Final.Outputs...)

	opts := tierOpts(filepath.Join(t.TempDir(), "does-not-exist"), true)
	emitStarTiers(context.Background(), opts, res, outDir)

	assert.Empty(t, res.Final.StarTiers, "no tiers recorded without StarNet")
	assert.Equal(t, outputsBefore, res.Final.Outputs, "outputs untouched")
	for _, name := range []string{"final-starless.png", "final-25-stars.png", "final-50-stars.png", "final-75-stars.png"} {
		assert.NoFileExists(t, filepath.Join(outDir, name))
	}
	after, err := os.ReadFile(filepath.Join(outDir, "final.png"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "the full-stars final is byte-identical")
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "star tiers skipped")
}

func TestEmitStarTiers_Gating(t *testing.T) {
	dir := t.TempDir()
	bin, callLog := fakeStarnet(t, dir)
	tests := []struct {
		name  string
		opts  func(outDir string) Options
		res   func(t *testing.T, outDir string) *Result
		cause string
	}{
		{"knob off", func(string) Options { return tierOpts(bin, false) }, finalResult, "star_tiers disabled"},
		{"no preset", func(string) Options { return Options{Starnet: starnet.NewVariant(bin, starnet.VariantPositional)} }, finalResult, "no preset"},
		{"no starnet runner", func(string) Options { return Options{Preset: &mode.Preset{StarTiers: true}} }, finalResult, "no runner"},
		{"finish produced nothing", func(string) Options { return tierOpts(bin, true) }, func(*testing.T, string) *Result { return &Result{} }, "no final"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()
			res := tt.res(t, outDir)
			emitStarTiers(context.Background(), tt.opts(outDir), res, outDir)
			if res.Final != nil {
				assert.Empty(t, res.Final.StarTiers, tt.cause)
			}
			assert.NoFileExists(t, filepath.Join(outDir, "final-50-stars.png"), tt.cause)
		})
	}
	assert.Equal(t, 0, starnetCalls(t, callLog), "a gated-off tier set never shells out")
}

// TestEmitStarTiers_CancelledRun pins that a cancelled run does not start a star-removal pass.
func TestEmitStarTiers_CancelledRun(t *testing.T) {
	dir := t.TempDir()
	bin, callLog := fakeStarnet(t, dir)
	outDir := t.TempDir()
	res := finalResult(t, outDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	emitStarTiers(ctx, tierOpts(bin, true), res, outDir)

	assert.Empty(t, res.Final.StarTiers)
	assert.Equal(t, 0, starnetCalls(t, callLog))
}
