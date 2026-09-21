package postprocess

// Native star blend: mix a starless render back with the with-stars original to keep a chosen
// FRACTION of the original star brightness. It is the arithmetic of internal/gimp ReduceStars —
// out = starless·(1−ratio) + original·ratio — done in-process, so the star-tier deliverables cost
// one decode + one encode per tier instead of a GIMP round-trip (and stay testable without GIMP).

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	_ "golang.org/x/image/tiff" // register the TIFF decoder: StarNet writes 16-bit LZW TIFFs
)

// ErrBlendInputs reports unusable blend inputs (missing image, mismatched geometry).
var ErrBlendInputs = errors.New("star blend inputs")

// BlendStars returns starless·(1−ratio) + original·ratio at 16 bits per channel, so ratio is the
// fraction of the original star brightness kept (0 = starless, 1 = the original). Out-of-range
// ratios clamp. Both images must share their bounds.
func BlendStars(original, starless image.Image, ratio float64) (image.Image, error) {
	if original == nil || starless == nil {
		return nil, fmt.Errorf("%w: both the original and the starless image are required", ErrBlendInputs)
	}
	if original.Bounds() != starless.Bounds() {
		return nil, fmt.Errorf("%w: size mismatch %v vs %v", ErrBlendInputs, original.Bounds(), starless.Bounds())
	}
	r := clamp01(ratio)
	b := original.Bounds()
	out := image.NewRGBA64(b)
	mix := func(o, s uint32) uint16 { return uint16(float64(s)*(1-r) + float64(o)*r + 0.5) }
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			or, og, ob, _ := original.At(x, y).RGBA()
			sr, sg, sb, _ := starless.At(x, y).RGBA()
			out.SetRGBA64(x, y, color.RGBA64{R: mix(or, sr), G: mix(og, sg), B: mix(ob, sb), A: 65535})
		}
	}
	return out, nil
}

// DecodeImage reads one image file, naming the file in any error (these run deep inside a finish).
// Exported so a caller writing SEVERAL blends of the same pair decodes each source once — the tier
// set would otherwise re-read two full-size 16-bit images per level.
func DecodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

// WritePNG encodes img to path, removing a partial file if the encode fails so a failed deliverable
// never leaves a truncated file behind. BestSpeed is deliberate: these are large 16-bit renders and
// the slower levels buy a few percent of size for several seconds per image.
func WritePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// clamp01 confines a ratio to 0..1.
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
