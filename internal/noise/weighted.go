package noise

import (
	"fmt"

	"github.com/verove-jordan/astronomy/internal/fits"
)

// DenoiseWeighted runs the starlet denoiser in place with a per-pixel threshold weight
// (len == W·H): the effective threshold at pixel i is K[j]·Strength·weight[i]·sigma(i), so weight 1
// behaves like Denoise and larger weights denoise harder — the seam noise equalization ramps the
// weight up where fewer frames were stacked. Pixels with weight 0 stay BYTE-IDENTICAL (the
// reconstruction is copied back only where weight > 0 — reconstruction float dust must not touch
// the full-depth core). A nil weight delegates to Denoise. Errors on a length mismatch; otherwise
// soft-fails per plane exactly like Denoise.
//
// A 3-plane COLOUR image is denoised in LUMINANCE ONLY, per-pixel chroma preserved exactly.
// Denoising the planes independently under a spatially-varying weight reshapes local chroma
// wherever the weight ramps — regional colour patches following the ramp's geometry (the
// blue-grey blotches of every multi-night OSC master, root-caused 2026-09-24). It is the same
// principle pipeline/noisefinish.go states for the per-channel denoise: three independent
// smoothing fields cannot cancel in the combine. Mono planes keep the historical path unchanged.
func DenoiseWeighted(im *fits.Image, o Options, weight []float32) error {
	if im == nil || o.Strength <= 0 || im.W <= 0 || im.H <= 0 {
		return nil
	}
	if weight == nil {
		Denoise(im, o)
		return nil
	}
	if len(weight) != im.W*im.H {
		return fmt.Errorf("denoise weight plane has %d samples, image is %dx%d", len(weight), im.W, im.H)
	}
	o = withDefaults(o)
	if im.C == 3 && len(im.Pix) >= 3 {
		denoiseColourWeightedLuminance(im, o, weight)
		return nil
	}
	for ch := 0; ch < im.C && ch < len(im.Pix); ch++ {
		denoisePlaneWeighted(im.Pix[ch], im.W, im.H, o, weight)
	}
	return nil
}

// denoiseColourWeightedLuminance denoises m=(R+G+B)/3 with the weighted pass and writes back
// c' = m' + (c − m) per channel: the luminance carries the correction, the per-pixel chroma is
// byte-exact by construction (the chromaSmoothRGB identity). Where weight is 0 the denoised m
// equals m (masked copy-back in denoisePlaneWeighted), so c' == c and the zero-weight
// byte-identity contract holds for colour too.
func denoiseColourWeightedLuminance(im *fits.Image, o Options, weight []float32) {
	n := len(im.Pix[0])
	mOld := make([]float32, n)
	for i := 0; i < n; i++ {
		mOld[i] = (im.Pix[0][i] + im.Pix[1][i] + im.Pix[2][i]) / 3
	}
	m := append([]float32(nil), mOld...)
	denoisePlaneWeighted(m, im.W, im.H, o, weight)
	for c := 0; c < 3; c++ {
		plane := im.Pix[c]
		for i := 0; i < n; i++ {
			if m[i] == mOld[i] {
				continue // untouched luminance (weight 0, or soft-fail): keep the plane byte-identical
			}
			plane[i] = m[i] + (plane[i] - mOld[i])
		}
	}
}

// denoisePlaneWeighted mirrors denoisePlane with the weight folded into every threshold and a
// masked copy-back.
func denoisePlaneWeighted(pix []float32, w, h int, o Options, weight []float32) {
	if !allFinite(pix) {
		return
	}
	cJ, wcoef := Decompose(pix, w, h, o.Scales)
	sigmaMap, mask, ok := buildMaps(pix, cJ, wcoef, w, h, o)
	if !ok {
		return
	}
	for j := range wcoef {
		kj := o.K[j]
		if kj <= 0 {
			continue
		}
		thresholdScaleWeighted(wcoef[j], sigmaMap, mask, weight, kj*o.Strength*scaleSigma(j))
	}
	out := Reconstruct(cJ, wcoef)
	if !allFinite(out) {
		return // soft-fail: keep the original plane
	}
	for i := range pix {
		if weight[i] > 0 {
			pix[i] = out[i]
		}
	}
}

// thresholdScaleWeighted is thresholdScale with the per-pixel threshold weight applied.
func thresholdScaleWeighted(w, sigmaMap, mask, weight []float32, base float64) {
	parallelRows(len(w), func(i0, i1 int) {
		for i := i0; i < i1; i++ {
			wv := float64(w[i])
			thr := softThreshold(wv, base*float64(sigmaMap[i])*float64(weight[i]))
			m := float64(mask[i])
			w[i] = float32(m*wv + (1-m)*thr)
		}
	})
}
