// filterset.go answers a question a one-shot-color rig cannot answer from its headers: was this night
// shot through a dual-band clip filter or unfiltered?
//
// A colour camera has no filter wheel, so there is no FILTER card and no filename token. The same body
// shoots broadband one night and through an Ha/OIII clip the next, and the two are not
// interchangeable — a dual-band frame carries two emission lines on a near-black sky, a broadband
// frame carries a full continuum. Stacking them together, or calibrating one with the other's flats,
// is wrong. The only witness is the pixels.
//
// Two signals are measured, and BOTH must agree or the answer is unknown. That conservatism is the
// design: a wrong verdict silently mis-stacks a whole night, whereas "unknown" costs nothing because
// every consumer falls back to today's behaviour.
package inspect

import (
	"math"
	"sort"
	"strings"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// Measured thresholds, from the two runbooks (ngc7000-narrowband-plus-broadband §0 and
// session-intake §2): sky above the bias pedestal per 120 s is 2–6 ADU through a dual-band clip and
// 30–45 ADU unfiltered. The gap between the bands is deliberately left as "no verdict" rather than
// split down the middle — a sky that lands there is genuinely ambiguous (a bright dual-band night
// under a moon, a very dark broadband site) and guessing would be worse than declining.
const (
	dualbandMaxPer120   = 10.0 // measured 2–6, with headroom for a brighter-than-typical night
	broadbandMinPer120  = 20.0 // measured 30–45, with headroom for a darker-than-typical site
	normalizeExposureMs = 120_000.0
)

// The sky level is read as a SYMMETRIC trimmed mean rather than a plain mean.
//
// A mean of every photosite reports the stars as well as the sky. On a dense field that inflation
// is worth a couple of ADU, and since the thresholds above are only 10 ADU apart it is enough to
// push a genuine dual-band night out of its band and into the "no verdict" gap — measured on a real
// IC 1848 capture: 9.1 ADU per 120 s read robustly, 10.9 read as a mean, against the 10.0 bound.
// The night then finished as plain broadband colour, which is exactly the silent mis-stack this
// file exists to prevent.
//
// A mean, not a median, because a median of 16-bit integers cannot resolve a 2–4 ADU sky at 60 s —
// that is why the plain mean was chosen in the first place, and a trimmed mean keeps the sub-ADU
// resolution while dropping the tails. SYMMETRIC, because the same reader measures the bias
// pedestal, whose noise is symmetric: trimming only the bright tail would drag the floor down and
// read every sky as brighter than it is. On real frames the symmetric trim leaves the floor
// unmoved (499.82 ADU either way) and only the star-contaminated lights change.
const (
	skyTrimLo = 0.10 // drop the darkest tenth…
	skyTrimHi = 0.90 // …and the brightest tenth, which is where the stars are
	// skyTrimMinSamples is the sample count below which the trim is skipped and a plain mean
	// returned. A trim over a handful of values is noise, not robustness; real frames bring
	// millions, so this only ever exempts synthetic fixtures.
	skyTrimMinSamples = 64
	// skySampleCap bounds the per-channel sample the trimmed mean sorts. A quarter of a million
	// photosites pins the mean far tighter than the 1 ADU the thresholds need, and it keeps inspect
	// interactive on folders of 24 MP subs.
	skySampleCap = 250_000
)

// ChannelSky is a set's per-channel sky level, in ADU ABOVE the scan's bias/dark floor. Floats, not
// integers: session-intake §2 notes that at 60 s the sky is 2–4 ADU and an integer median cannot
// resolve the colour at all, so the caller measures means.
type ChannelSky struct {
	R, G, B float64 `json:"-"`
}

// amplitude is the scalar the sky-rate thresholds are written against: the mean of the three
// channels. A mean rather than a single channel, because which channel is brightest is the OTHER
// signal — reading amplitude off one channel would make the two tests the same test.
func (s ChannelSky) amplitude() float64 { return (s.R + s.G + s.B) / 3 }

// redDominant is the dual-band colour signature: Ha feeds red hardest, and only the OIII window
// feeds blue at all, so blue is the floor. Both halves are required — "R is highest" alone also
// describes a red light-pollution gradient on a broadband night.
func (s ChannelSky) redDominant() bool {
	return s.R > s.G && s.G > s.B
}

// greenBlueDominant is the broadband signature: the continuum plus a typical sky puts green or blue
// on top. Stated as "red does not lead" so it stays the exact complement of the test above.
func (s ChannelSky) greenBlueDominant() bool {
	return s.G >= s.R || s.B >= s.R
}

// ClassifyFilterSet decides a set's filter set from its per-channel sky and its exposure. Returns
// FilterSetUnknown unless the amplitude and the colour independently agree — see the package comment.
func ClassifyFilterSet(sky ChannelSky, exposureMs int64) filters.FilterSet {
	if exposureMs <= 0 {
		return filters.FilterSetUnknown
	}
	// Normalize to the 120 s the thresholds are written for. Sky accumulates linearly with exposure,
	// so this is the one conversion that makes a 60 s and a 300 s night comparable.
	scale := normalizeExposureMs / float64(exposureMs)
	rate := ChannelSky{R: sky.R * scale, G: sky.G * scale, B: sky.B * scale}
	amp := rate.amplitude()
	if amp <= 0 || math.IsNaN(amp) || math.IsInf(amp, 0) {
		return filters.FilterSetUnknown // unmeasured, or a floor subtracted past the signal
	}

	switch {
	case amp <= dualbandMaxPer120 && rate.redDominant():
		return filters.FilterSetDualband
	case amp >= broadbandMinPer120 && rate.greenBlueDominant():
		return filters.FilterSetBroadband
	default:
		return filters.FilterSetUnknown
	}
}

// bayerOffsets maps a BAYERPAT to the (x,y) position of each primary inside the 2×2 mosaic cell.
// A CFA frame reaches us as ONE plane — the colour is carried by position, not by a channel index —
// so the per-channel sky cannot be read without knowing the pattern. An unrecognised pattern yields
// no offsets and the set is simply left unclassified.
var bayerOffsets = map[string]struct{ r, g1, g2, b [2]int }{
	"RGGB": {r: [2]int{0, 0}, g1: [2]int{1, 0}, g2: [2]int{0, 1}, b: [2]int{1, 1}},
	"BGGR": {r: [2]int{1, 1}, g1: [2]int{1, 0}, g2: [2]int{0, 1}, b: [2]int{0, 0}},
	"GRBG": {r: [2]int{1, 0}, g1: [2]int{0, 0}, g2: [2]int{1, 1}, b: [2]int{0, 1}},
	"GBRG": {r: [2]int{0, 1}, g1: [2]int{0, 0}, g2: [2]int{1, 1}, b: [2]int{1, 0}},
}

// cfaChannelSky reads the per-channel sky of an undebayered mosaic by sampling each primary at its
// own position in the 2×2 cell. ok is false for an unknown pattern or a frame too small to sample.
func cfaChannelSky(pix []float32, w, h int, pattern string) (ChannelSky, bool) {
	off, known := bayerOffsets[strings.ToUpper(strings.TrimSpace(pattern))]
	if !known || w < 2 || h < 2 || len(pix) < w*h {
		return ChannelSky{}, false
	}
	stride := cellStride(w/2, h/2)
	var rs, gs, bs []float64
	for y := 0; y+1 < h; y += 2 * stride {
		for x := 0; x+1 < w; x += 2 * stride {
			at := func(o [2]int) float64 { return float64(pix[(y+o[1])*w+(x+o[0])]) }
			rs = append(rs, at(off.r))
			gs = append(gs, at(off.g1), at(off.g2))
			bs = append(bs, at(off.b))
		}
	}
	if len(rs) == 0 || len(gs) == 0 || len(bs) == 0 {
		return ChannelSky{}, false
	}
	return ChannelSky{R: trimmedMean(rs), G: trimmedMean(gs), B: trimmedMean(bs)}, true
}

// cellStride subsamples the mosaic so each channel contributes at most skySampleCap values, keeping
// the sort bounded on a 24 MP frame. The sky is a property of the whole frame, so an evenly spread
// subsample measures it exactly as well as every pixel would.
func cellStride(cellsX, cellsY int) int {
	cells := cellsX * cellsY
	if cells <= skySampleCap || cells <= 0 {
		return 1
	}
	return int(math.Ceil(math.Sqrt(float64(cells) / float64(skySampleCap))))
}

// planeChannelSky reads the per-channel sky of an already-debayered RGB frame (three planes).
func planeChannelSky(planes [][]float32) (ChannelSky, bool) {
	if len(planes) < 3 {
		return ChannelSky{}, false
	}
	sky := func(p []float32) (float64, bool) {
		if len(p) == 0 {
			return 0, false
		}
		step := 1
		if len(p) > skySampleCap {
			step = len(p) / skySampleCap
		}
		vals := make([]float64, 0, len(p)/step+1)
		for i := 0; i < len(p); i += step {
			vals = append(vals, float64(p[i]))
		}
		return trimmedMean(vals), true
	}
	r, okR := sky(planes[0])
	g, okG := sky(planes[1])
	b, okB := sky(planes[2])
	if !okR || !okG || !okB {
		return ChannelSky{}, false
	}
	return ChannelSky{R: r, G: g, B: b}, true
}

// trimmedMean is the mean of vals between the skyTrimLo and skyTrimHi quantiles — the sky level,
// with the star tail (and the symmetric dark tail) removed. See the constants for why it is a
// trimmed MEAN and why the trim is symmetric. Samples below skyTrimMinSamples are meaned whole.
func trimmedMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	lo, hi := 0, len(vals)
	if len(vals) >= skyTrimMinSamples {
		sort.Float64s(vals)
		lo, hi = int(float64(len(vals))*skyTrimLo), int(float64(len(vals))*skyTrimHi)
		if lo >= hi { // a degenerate window measures nothing — fall back to the whole sample
			lo, hi = 0, len(vals)
		}
	}
	var sum float64
	for _, v := range vals[lo:hi] {
		sum += v
	}
	return sum / float64(hi-lo)
}

// subtractFloor removes the bias/dark pedestal the thresholds are written ABOVE. The floor is a
// single scalar (the scan's dark floor) rather than a per-channel one: a bias pedestal is an
// electronic offset, identical on every photosite, so subtracting one number cannot invent a colour
// imbalance that was not in the photons.
func subtractFloor(sky ChannelSky, floor float64) ChannelSky {
	return ChannelSky{R: sky.R - floor, G: sky.G - floor, B: sky.B - floor}
}
