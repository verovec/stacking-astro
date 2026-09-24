package noise

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/fits"
)

// noisyPlane builds a deterministic sky+noise plane for the weighted-denoise tests.
func noisyPlane(w, h int, seed uint32) []float32 {
	p := newPlane(w, h, 0.1)
	addNoise(p, newRNG(seed), 0.01)
	return p
}

// planeSigma measures the robust noise of a sub-rectangle via the package's own tile estimator on a
// copy (Measure works on whole images; here a plain std-dev is enough for a pure-noise fixture).
func planeSigma(p []float32, w int, x0, x1, h int) float64 {
	var sum, sum2 float64
	n := 0
	for y := 0; y < h; y++ {
		for x := x0; x < x1; x++ {
			v := float64(p[y*w+x])
			sum += v
			sum2 += v * v
			n++
		}
	}
	mean := sum / float64(n)
	return sum2/float64(n) - mean*mean // variance (comparisons only need monotonicity)
}

func TestDenoiseWeighted_NilMatchesDenoise(t *testing.T) {
	const w, h = 256, 256
	a := monoImage(w, h, noisyPlane(w, h, 7))
	b := monoImage(w, h, noisyPlane(w, h, 7))

	Denoise(a, DefaultOptions())
	require.NoError(t, DenoiseWeighted(b, DefaultOptions(), nil))
	assert.Equal(t, a.Pix[0], b.Pix[0], "nil weight must be byte-identical to Denoise")
}

func TestDenoiseWeighted_ZeroWeightIsByteIdentical(t *testing.T) {
	const w, h = 128, 128
	orig := noisyPlane(w, h, 11)
	im := monoImage(w, h, append([]float32(nil), orig...))

	require.NoError(t, DenoiseWeighted(im, DefaultOptions(), make([]float32, w*h)))
	assert.Equal(t, orig, im.Pix[0], "weight 0 everywhere must not touch a single byte")
}

func TestDenoiseWeighted_HalfPlaneReducesSigmaOnlyThere(t *testing.T) {
	const w, h = 256, 256
	orig := noisyPlane(w, h, 23)
	im := monoImage(w, h, append([]float32(nil), orig...))
	weight := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			weight[y*w+x] = 1.5
		}
	}

	require.NoError(t, DenoiseWeighted(im, DefaultOptions(), weight))

	for y := 0; y < h; y++ {
		left := im.Pix[0][y*w : y*w+w/2]
		wantLeft := orig[y*w : y*w+w/2]
		require.Equal(t, wantLeft, left, "zero-weight half must stay byte-identical (row %d)", y)
	}
	before := planeSigma(orig, w, w/2+8, w-8, h)
	after := planeSigma(im.Pix[0], w, w/2+8, w-8, h)
	assert.Less(t, after, before*0.8, "weighted half must be measurably denoised")
}

func TestDenoiseWeighted_LengthMismatchErrors(t *testing.T) {
	im := monoImage(64, 64, noisyPlane(64, 64, 3))
	err := DenoiseWeighted(im, DefaultOptions(), make([]float32, 7))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "weight plane")
}

// colourImage builds a 3-plane image around a shared mean plane m with a deterministic
// antisymmetric chroma field c: R = m + c, G = m − c/2, B = m − c/2 — so (R+G+B)/3 == m exactly
// and the per-pixel chroma (channel − mean) is known in closed form.
func colourImage(w, h int, m, c []float32) *fits.Image {
	im := &fits.Image{W: w, H: h, C: 3, Pix: make([][]float32, 3)}
	for p := 0; p < 3; p++ {
		im.Pix[p] = make([]float32, w*h)
	}
	for i := range m {
		im.Pix[0][i] = m[i] + c[i]
		im.Pix[1][i] = m[i] - c[i]/2
		im.Pix[2][i] = m[i] - c[i]/2
	}
	return im
}

// chromaField is a smooth deterministic colour pattern (the kind a real dual-band sky carries).
func chromaField(w, h int) []float32 {
	c := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c[y*w+x] = 0.004 * float32(x%64) / 64
		}
	}
	return c
}

// The multi-night seam noise equalization runs this weighted denoise on the merged COLOUR master.
// Denoising the three planes independently under a spatially-varying weight reshapes the local
// chroma wherever the weight ramps — regional colour patches following the coverage footprints,
// the blue-grey blotches of every multi-night OSC master (root-caused 2026-09-24; the principle is
// stated in pipeline/noisefinish.go). A colour image must therefore be denoised in LUMINANCE only,
// with per-pixel chroma preserved exactly.
func TestDenoiseWeighted_ColourPreservesChroma(t *testing.T) {
	const w, h = 256, 256
	m := noisyPlane(w, h, 31)
	c := chromaField(w, h)
	im := colourImage(w, h, append([]float32(nil), m...), c)
	weight := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			weight[y*w+x] = 2.0
		}
	}

	require.NoError(t, DenoiseWeighted(im, DefaultOptions(), weight))

	for i := 0; i < w*h; i++ {
		mean := (im.Pix[0][i] + im.Pix[1][i] + im.Pix[2][i]) / 3
		assert.InDelta(t, float64(c[i]), float64(im.Pix[0][i]-mean), 1e-5,
			"per-pixel chroma must survive the pass exactly (pixel %d)", i)
		if t.Failed() {
			return // one pixel is proof enough; do not flood the log
		}
	}
	meanAfter := make([]float32, w*h)
	for i := range meanAfter {
		meanAfter[i] = (im.Pix[0][i] + im.Pix[1][i] + im.Pix[2][i]) / 3
	}
	before := planeSigma(m, w, w/2+8, w-8, h)
	after := planeSigma(meanAfter, w, w/2+8, w-8, h)
	assert.Less(t, after, before*0.8, "the luminance must still be measurably denoised where weighted")
}

// The zero-weight byte-identity contract holds for colour images too.
func TestDenoiseWeighted_ColourZeroWeightIsByteIdentical(t *testing.T) {
	const w, h = 128, 128
	m := noisyPlane(w, h, 41)
	im := colourImage(w, h, m, chromaField(w, h))
	var orig [3][]float32
	for p := 0; p < 3; p++ {
		orig[p] = append([]float32(nil), im.Pix[p]...)
	}

	require.NoError(t, DenoiseWeighted(im, DefaultOptions(), make([]float32, w*h)))

	for p := 0; p < 3; p++ {
		assert.Equal(t, orig[p], im.Pix[p], "plane %d must not change under zero weight", p)
	}
}

// A mono image keeps the historical single-plane path byte-for-byte: DenoiseWeighted on one plane
// must equal a direct denoisePlaneWeighted call — the colour fix must not leak into mono.
func TestDenoiseWeighted_MonoMatchesDirectPlanePass(t *testing.T) {
	const w, h = 128, 128
	orig := noisyPlane(w, h, 53)
	viaAPI := monoImage(w, h, append([]float32(nil), orig...))
	direct := append([]float32(nil), orig...)
	weight := make([]float32, w*h)
	for i := range weight {
		weight[i] = 1.2
	}

	require.NoError(t, DenoiseWeighted(viaAPI, DefaultOptions(), weight))
	denoisePlaneWeighted(direct, w, h, withDefaults(DefaultOptions()), weight)

	assert.Equal(t, direct, viaAPI.Pix[0])
}
