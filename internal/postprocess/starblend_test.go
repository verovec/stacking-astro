package postprocess

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// solid builds a 1-colour RGBA64 image of the given size (opaque, so At().RGBA() is exact).
func solid(w, h int, r, g, b uint16) *image.RGBA64 {
	img := image.NewRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA64(x, y, color.RGBA64{R: r, G: g, B: b, A: 65535})
		}
	}
	return img
}

func TestBlendStars_Ratios(t *testing.T) {
	const (
		orig     = uint16(32768) // the with-stars render
		starless = uint16(8192)  // the same pixel with the star taken out
	)
	tests := []struct {
		name  string
		ratio float64
		want  uint16
	}{
		{"0 keeps the starless value", 0, 8192},
		{"25% of the original star brightness", 0.25, 14336},
		{"50% halves the star", 0.5, 20480},
		{"75% keeps most of the star", 0.75, 26624},
		{"1 reproduces the original", 1, 32768},
		{"below range clamps to starless", -0.5, 8192},
		{"above range clamps to original", 1.5, 32768},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BlendStars(solid(4, 3, orig, orig/2, orig/4), solid(4, 3, starless, starless/2, starless/4), tt.ratio)
			require.NoError(t, err)
			assert.Equal(t, image.Rect(0, 0, 4, 3), got.Bounds())
			r, g, b, a := got.At(2, 1).RGBA()
			assert.Equal(t, uint32(tt.want), r, "red channel")
			assert.Equal(t, uint32(tt.want/2), g, "green channel")
			assert.Equal(t, uint32(tt.want/4), b, "blue channel")
			assert.Equal(t, uint32(65535), a, "blend stays opaque")
		})
	}
}

func TestBlendStars_Errors(t *testing.T) {
	tests := []struct {
		name               string
		original, starless image.Image
	}{
		{"size mismatch", solid(4, 3, 1, 1, 1), solid(4, 2, 1, 1, 1)},
		{"nil original", nil, solid(4, 3, 1, 1, 1)},
		{"nil starless", solid(4, 3, 1, 1, 1), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BlendStars(tt.original, tt.starless, 0.5)
			assert.Error(t, err)
		})
	}
}

// TestBlendStars_RoundTripsThroughFiles pins the decode → blend → encode path the tier set uses:
// a PNG written by WritePNG decodes back through DecodeImage with the blended value intact.
func TestBlendStars_RoundTripsThroughFiles(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "final.png")
	starlessPath := filepath.Join(dir, "final-starless.png")
	outPath := filepath.Join(dir, "final-50-stars.png")
	require.NoError(t, WritePNG(origPath, solid(8, 8, 32768, 32768, 32768)))
	require.NoError(t, WritePNG(starlessPath, solid(8, 8, 8192, 8192, 8192)))

	original, err := DecodeImage(origPath)
	require.NoError(t, err)
	starless, err := DecodeImage(starlessPath)
	require.NoError(t, err)
	blended, err := BlendStars(original, starless, 0.5)
	require.NoError(t, err)
	require.NoError(t, WritePNG(outPath, blended))

	f, err := os.Open(outPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	img, format, err := image.Decode(f)
	require.NoError(t, err)
	assert.Equal(t, "png", format)
	r, _, _, _ := img.At(4, 4).RGBA()
	assert.Equal(t, uint32(20480), r)
}

func TestDecodeImage_MissingOrUnreadable(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "not-an-image.png")
	require.NoError(t, os.WriteFile(junk, []byte("definitely not a PNG"), 0o644))
	tests := []struct{ name, path string }{
		{"missing file", filepath.Join(dir, "nope.png")},
		{"undecodable file", junk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeImage(tt.path)
			assert.Error(t, err)
		})
	}
}
