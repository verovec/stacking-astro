package inspect

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/fits"
)

// lattice builds an image whose four Bayer sub-lattices sit at the given means, with a little noise
// so nothing depends on the values being exactly constant.
func lattice(m00, m10, m01, m11 float64) *fits.Image {
	const w, h = 16, 16
	pix := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v float64
			switch {
			case y%2 == 0 && x%2 == 0:
				v = m00
			case y%2 == 0 && x%2 == 1:
				v = m10
			case y%2 == 1 && x%2 == 0:
				v = m01
			default:
				v = m11
			}
			// A deterministic ripple: mean-preserving over each sub-lattice, so the probe cannot be
			// passing merely because the planes are perfectly flat.
			v += math.Sin(float64(x*7+y*13)) * 0.5
			pix[y*w+x] = float32(v)
		}
	}
	return &fits.Image{W: w, H: h, C: 1, Pix: [][]float32{pix}}
}

// TestCFAStructureProbe asks the question the FILTER card cannot answer: is this frame genuinely a
// colour mosaic?
//
// A real CFA separates its four sub-lattices — R, G1, G2 and B sit under different dyes and see
// different quantum efficiency, so skyglow and a flat field both split them by tens of percent. A
// MONOCHROME sensor whose capture program stamped a spurious BAYERPAT has no such structure: its
// four sub-lattices are the same sensor, and differ only by noise.
func TestCFAStructureProbe(t *testing.T) {
	tests := []struct {
		name string
		im   *fits.Image
		want cfaVerdict
	}{
		{
			// A colour sensor on skyglow: green is roughly twice red, blue lower still.
			name: "genuine mosaic — sub-lattices separate strongly",
			im:   lattice(520, 560, 560, 505),
			want: cfaYes,
		},
		{
			name: "genuine mosaic — dual-band, red far above the rest",
			im:   lattice(530, 504, 504, 502),
			want: cfaYes,
		},
		{
			// The ASI1600MM case: one sensor, one response, a BAYERPAT card that means nothing.
			name: "mono sensor carrying a spurious BAYERPAT",
			im:   lattice(500, 500, 500, 500),
			want: cfaNo,
		},
		{
			name: "mono sensor with a touch of gain drift between columns",
			im:   lattice(500, 501, 500, 501),
			want: cfaNo,
		},
		{
			// Between the bands: could be a very neutral mosaic or a slightly uneven mono sensor.
			// 0005's discipline — decline rather than guess.
			name: "ambiguous separation",
			im:   lattice(500, 510, 510, 500),
			want: cfaMaybe,
		},
		{
			name: "an empty frame says nothing",
			im:   &fits.Image{W: 0, H: 0, C: 1, Pix: [][]float32{{}}},
			want: cfaMaybe,
		},
		{
			// Already-debayered RGB: three planes, no mosaic to probe.
			name: "three-plane frame is not a mosaic question",
			im:   &fits.Image{W: 4, H: 4, C: 3, Pix: [][]float32{make([]float32, 16), make([]float32, 16), make([]float32, 16)}},
			want: cfaMaybe,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, probeCFAStructure(tt.im))
		})
	}
}

// TestCFAStructureProbe_GreensPair is the sharper discriminator, and the reason the probe splits
// FOUR sub-lattices rather than three channels: in every Bayer pattern the two greens sit under the
// SAME dye, so a genuine mosaic separates R from B while keeping G1≈G2. Vignetting or a gradient
// moves all four together and must not be mistaken for colour structure.
func TestCFAStructureProbe_GreensPair(t *testing.T) {
	// A smooth left-to-right gradient hits both greens equally and does not fake a mosaic.
	assert.Equal(t, cfaNo, probeCFAStructure(lattice(500, 502, 500, 502)))
	// A real mosaic: the greens agree with each other and differ from R and B.
	assert.Equal(t, cfaYes, probeCFAStructure(lattice(520, 560, 560, 505)))
}

// TestProbeFramesCFA requires AGREEMENT across the sampled frames. One frame is a weak witness: a
// passing cloud, a satellite, or a frame shot with the lens cap on can all flatten the structure.
func TestProbeFramesCFA(t *testing.T) {
	cfa := lattice(520, 560, 560, 505)
	mono := lattice(500, 500, 500, 500)
	ambiguous := lattice(500, 510, 510, 500)

	load := func(images ...*fits.Image) imageLoader {
		byPath := map[string]*fits.Image{}
		for i, im := range images {
			byPath[pathOf(i)] = im
		}
		return func(p string) (*fits.Image, error) { return byPath[p], nil }
	}
	frames := func(n int) []*Frame {
		out := make([]*Frame, n)
		for i := range out {
			out[i] = &Frame{Path: pathOf(i), Type: Light, Bayer: "RGGB"}
		}
		return out
	}

	t.Run("every sampled frame agrees it is a mosaic", func(t *testing.T) {
		assert.Equal(t, cfaYes, probeFramesCFA(frames(3), load(cfa, cfa, cfa)))
	})

	t.Run("every sampled frame agrees it is not", func(t *testing.T) {
		assert.Equal(t, cfaNo, probeFramesCFA(frames(3), load(mono, mono, mono)))
	})

	t.Run("one dissenting frame withdraws the verdict", func(t *testing.T) {
		assert.Equal(t, cfaMaybe, probeFramesCFA(frames(3), load(cfa, cfa, mono)))
	})

	t.Run("an ambiguous frame withdraws the verdict", func(t *testing.T) {
		assert.Equal(t, cfaMaybe, probeFramesCFA(frames(3), load(cfa, ambiguous, cfa)))
	})

	t.Run("nothing readable says nothing", func(t *testing.T) {
		assert.Equal(t, cfaMaybe, probeFramesCFA(frames(2), func(string) (*fits.Image, error) {
			return nil, assert.AnError
		}))
	})

	t.Run("no frames at all says nothing", func(t *testing.T) {
		require.Equal(t, cfaMaybe, probeFramesCFA(nil, load(cfa)))
	})
}

func pathOf(i int) string { return string(rune('a'+i)) + ".fits" }
