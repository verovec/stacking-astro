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

// cfaChannelSky reads the per-channel mean of an undebayered mosaic by sampling each primary at its
// own position in the 2×2 cell. ok is false for an unknown pattern or a frame too small to sample.
func cfaChannelSky(pix []float32, w, h int, pattern string) (ChannelSky, bool) {
	off, known := bayerOffsets[strings.ToUpper(strings.TrimSpace(pattern))]
	if !known || w < 2 || h < 2 || len(pix) < w*h {
		return ChannelSky{}, false
	}
	var rSum, gSum, bSum float64
	var rN, gN, bN int
	for y := 0; y+1 < h; y += 2 {
		for x := 0; x+1 < w; x += 2 {
			at := func(o [2]int) float64 { return float64(pix[(y+o[1])*w+(x+o[0])]) }
			rSum += at(off.r)
			rN++
			gSum += at(off.g1) + at(off.g2)
			gN += 2
			bSum += at(off.b)
			bN++
		}
	}
	if rN == 0 || gN == 0 || bN == 0 {
		return ChannelSky{}, false
	}
	return ChannelSky{R: rSum / float64(rN), G: gSum / float64(gN), B: bSum / float64(bN)}, true
}

// planeChannelSky reads the per-channel mean of an already-debayered RGB frame (three planes).
func planeChannelSky(planes [][]float32) (ChannelSky, bool) {
	if len(planes) < 3 {
		return ChannelSky{}, false
	}
	mean := func(p []float32) (float64, bool) {
		if len(p) == 0 {
			return 0, false
		}
		var sum float64
		for _, v := range p {
			sum += float64(v)
		}
		return sum / float64(len(p)), true
	}
	r, okR := mean(planes[0])
	g, okG := mean(planes[1])
	b, okB := mean(planes[2])
	if !okR || !okG || !okB {
		return ChannelSky{}, false
	}
	return ChannelSky{R: r, G: g, B: b}, true
}

// subtractFloor removes the bias/dark pedestal the thresholds are written ABOVE. The floor is a
// single scalar (the scan's dark floor) rather than a per-channel one: a bias pedestal is an
// electronic offset, identical on every photosite, so subtracting one number cannot invent a colour
// imbalance that was not in the photons.
func subtractFloor(sky ChannelSky, floor float64) ChannelSky {
	return ChannelSky{R: sky.R - floor, G: sky.G - floor, B: sky.B - floor}
}
