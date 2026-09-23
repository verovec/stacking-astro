package starnet

// Live contract test against the host StarNet install: that the auto-probe classifies the real
// binary, and that a RemoveStars call actually produces a decodable starless TIFF of the same
// geometry. Skipped when no StarNet is installed (e.g. Linux CI), like internal/siril's live tests.

import (
	"context"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/tiff"
)

// starnetBin resolves the host StarNet the same way the engine does (STARNET_BIN, else the known
// install names), skipping the test when none is installed.
func starnetBin(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("STARNET_BIN"); bin != "" {
		if p, err := exec.LookPath(bin); err == nil {
			return p
		}
		t.Skipf("no starnet at STARNET_BIN=%s", bin)
	}
	for _, name := range DefaultBinCandidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skipf("no starnet binary installed (tried %v)", DefaultBinCandidates)
	return ""
}

// writeStarField writes a 16-bit mono TIFF: a smooth background gradient plus bright Gaussian
// "stars" — enough structure for StarNet to have something to separate.
func writeStarField(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewGray16(image.Rect(0, 0, w, h))
	stars := [][3]float64{{80, 90, 3.0}, {200, 150, 2.2}, {310, 260, 3.6}, {130, 330, 2.6}, {400, 80, 2.0}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Broad nebulosity the starless render must KEEP.
			v := 0.18 + 0.12*math.Sin(float64(x)/float64(w)*math.Pi)*math.Sin(float64(y)/float64(h)*math.Pi)
			for _, s := range stars {
				dx, dy := float64(x)-s[0], float64(y)-s[1]
				v += 0.75 * math.Exp(-(dx*dx+dy*dy)/(2*s[2]*s[2]))
			}
			img.SetGray16(x, y, color.Gray16{Y: uint16(math.Min(v, 1) * 65535)})
		}
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, tiff.Encode(f, img, nil))
	require.NoError(t, f.Close())
}

// maxLuma is the brightest sample in an image, 0..1.
func maxLuma(t *testing.T, path string) float64 {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	require.NoError(t, err)
	b := img.Bounds()
	peak := 0.0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if v := float64(max3(r, g, bl)) / 65535; v > peak {
				peak = v
			}
		}
	}
	return peak
}

func max3(a, b, c uint32) uint32 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func TestRemoveStars_Live(t *testing.T) {
	bin := starnetBin(t)
	ctx := context.Background()

	// The installed binary must be classified by the probe, not guessed.
	r := New(bin, "")
	variant := r.resolveVariant(ctx)
	assert.Contains(t, []Variant{VariantFlags, VariantPositional}, variant)
	t.Logf("starnet %s detected as %s", bin, variant)

	dir := t.TempDir()
	in := filepath.Join(dir, "stars.tif")
	out := filepath.Join(dir, "starless.tif")
	writeStarField(t, in, 512, 512)

	var sawLine bool
	require.NoError(t, r.RemoveStars(ctx, in, out, Options{}, func(p Progress) {
		if p.Line != "" {
			sawLine = true
		}
	}))

	require.FileExists(t, out)
	f, err := os.Open(out)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	require.NoError(t, err, "starless output must be a decodable image")
	assert.Equal(t, image.Rect(0, 0, 512, 512), img.Bounds(), "starless keeps the input geometry")

	// Removing stars can only ever lower the peak — a directional invariant that holds for any
	// working starless render without depending on how much of a synthetic star it eats.
	assert.LessOrEqual(t, maxLuma(t, out), maxLuma(t, in)+1e-6)
	assert.True(t, sawLine, "StarNet streams progress/log lines")
}
