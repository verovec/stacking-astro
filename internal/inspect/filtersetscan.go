// filtersetscan.go turns the pure classifier in filterset.go into a per-set verdict over real files:
// pick a bounded sample of each one-shot-color light set, measure its per-channel sky, subtract the
// scan's own bias pedestal, and classify. Everything here fails soft — an unreadable frame, a missing
// bias, an unknown Bayer pattern all yield "unknown", never a guess.
package inspect

import (
	"sort"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/fits"
)

const (
	// filterSetSampleFrames bounds the I/O: the sky level of a set is a property of the night, not of
	// a particular sub, so a handful of frames settles it. Inspect runs interactively on folders with
	// hundreds of subs and must stay fast.
	filterSetSampleFrames = 3
	// adu16Scale converts normalized [0,1] float pixels to the 16-bit ADU the thresholds are written
	// in. Siril writes 32-bit float FITS normalized to [0,1]; a camera writes 16-bit integers.
	adu16Scale = 65535.0
)

// imageLoader reads a frame's pixels. Injected so the annotation is testable without FITS files.
type imageLoader func(path string) (*fits.Image, error)

// annotateFilterSets stamps a filter set on every one-shot-color LIGHT set of the inventory.
//
// The bias pedestal is REQUIRED, not optional. The thresholds are written for sky ABOVE the pedestal,
// and an uncalibrated ASI frame sits at ~500 ADU of pure offset — twenty times the broadband sky —
// so classifying without subtracting it would read every set as broadband, confidently and wrongly.
// With no bias or dark in the scan there is no honest answer, and every set stays unknown.
//
// Overrides win outright and are applied even when nothing could be measured: the user looked at the
// stack and knows. Detection only fills the blanks — the same precedence the info.txt manifest keeps
// over the header.
func annotateFilterSets(inv *Inventory, overrides map[string]filters.FilterSet, load imageLoader) {
	if inv == nil {
		return
	}
	verdicts := map[string]filters.FilterSet{}
	if inv.ColorModel == ColorOSC {
		if floor, ok := biasFloorADU(inv, load); ok {
			for _, set := range inv.Sets {
				if set.Key.Type != Light {
					continue
				}
				if v := classifySet(set, floor, load); v.Known() {
					verdicts[set.Key.ID()] = v
				}
			}
		}
	}
	for id, v := range overrides { // user's word is final
		if v.Known() {
			verdicts[id] = v
		}
	}
	if len(verdicts) == 0 {
		inv.FilterSets = nil
		applyFilterSets(inv)
		return
	}
	inv.FilterSets = verdicts
	applyFilterSets(inv)
}

// applyFilterSets copies the inventory's per-set verdicts onto the Set values. It is idempotent and
// must be re-run after anything rebuilds Sets, because buildSets returns fresh structs — the map, not
// the Set field, is the durable record.
func applyFilterSets(inv *Inventory) {
	for i := range inv.Sets {
		inv.Sets[i].FilterSet = inv.FilterSets[inv.Sets[i].Key.ID()]
	}
}

// NightFilterSet reports the clip filter a capture NIGHT was shot through, read from the LIGHT sets
// of that night. It is how a frame that cannot be classified on its own inherits a verdict: a flat
// is an evenly-illuminated panel, not a sky, so ClassifyFilterSet has nothing to measure on it — but
// the clip filter sits in the optical train for the whole night, so the night's lights answer for it.
//
// One dissenting light set collapses the verdict to unknown. Two different verdicts on one night mean
// the clip was swapped mid-session, and nothing in the night key says which flat belongs to which
// half; unknown is the honest answer and leaves matching exactly as it is today. Sets with no verdict
// abstain rather than dissent — an unclassifiable set is silence, not disagreement.
//
// It reads Inventory.FilterSets, the durable record, because several code paths rebuild Sets from
// Frames and lose the projected Set.FilterSet (see applyFilterSets).
func (inv *Inventory) NightFilterSet(session string) filters.FilterSet {
	if inv == nil {
		return filters.FilterSetUnknown
	}
	verdict := filters.FilterSetUnknown
	for _, set := range inv.Sets {
		if set.Key.Type != Light || set.Key.Session != session {
			continue
		}
		v := inv.FilterSets[set.Key.ID()]
		if !v.Known() {
			continue
		}
		if verdict.Known() && v != verdict {
			return filters.FilterSetUnknown
		}
		verdict = v
	}
	return verdict
}

// classifySet measures a bounded sample of the set and classifies the median of the per-frame skies.
// The median rather than the mean: one frame ruined by a passing cloud or a car headlight should not
// drag a whole night's verdict across a threshold.
func classifySet(set Set, floor float64, load imageLoader) filters.FilterSet {
	var rs, gs, bs []float64
	for _, fr := range sampleFrames(set.Frames, filterSetSampleFrames) {
		sky, ok := frameChannelSky(fr, load)
		if !ok {
			continue
		}
		rs = append(rs, sky.R)
		gs = append(gs, sky.G)
		bs = append(bs, sky.B)
	}
	if len(rs) == 0 {
		return filters.FilterSetUnknown
	}
	above := subtractFloor(ChannelSky{R: median(rs), G: median(gs), B: median(bs)}, floor)
	return ClassifyFilterSet(above, set.Key.ExposureMs, setEGain(set))
}

// setEGain is the sensor conversion factor (electrons per ADU) the set was shot at, read from the
// frames' EGAIN cards. A set is one body at one analogue gain — SetKey carries Gain — so the frames
// agree; the median only guards against a stray frame with a missing or garbled card. Returns 0 when
// no frame carries EGAIN, which ClassifyFilterSet turns into "no verdict" rather than a guess.
func setEGain(set Set) float64 {
	var vals []float64
	for _, fr := range set.Frames {
		if fr != nil && fr.EGain > 0 {
			vals = append(vals, fr.EGain)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	return median(vals)
}

// biasFloorADU measures the scan's electronic pedestal from its BIAS frames, falling back to DARKs
// (a dark is a bias plus thermal signal — slightly high, which biases the verdict toward "fainter
// sky", i.e. toward dual-band; it is the conservative direction only for the amplitude test, so the
// colour test still has to agree).
func biasFloorADU(inv *Inventory, load imageLoader) (float64, bool) {
	for _, typ := range []FrameType{Bias, Dark} {
		var levels []float64
		for _, set := range inv.Sets {
			if set.Key.Type != typ {
				continue
			}
			for _, fr := range sampleFrames(set.Frames, filterSetSampleFrames) {
				if sky, ok := frameChannelSky(fr, load); ok {
					levels = append(levels, sky.amplitude())
				}
			}
		}
		if len(levels) > 0 {
			return median(levels), true
		}
	}
	return 0, false
}

// frameChannelSky reads one frame's per-channel mean, in 16-bit ADU. A CFA mosaic is sampled at the
// Bayer positions; an already-debayered frame by plane.
func frameChannelSky(fr *Frame, load imageLoader) (ChannelSky, bool) {
	if fr == nil || load == nil {
		return ChannelSky{}, false
	}
	im, err := load(fr.Path)
	if err != nil || im == nil || im.W <= 0 || im.H <= 0 || len(im.Pix) == 0 {
		return ChannelSky{}, false
	}
	var sky ChannelSky
	var ok bool
	if im.C >= 3 {
		sky, ok = planeChannelSky(im.Pix)
	} else {
		sky, ok = cfaChannelSky(im.Pix[0], im.W, im.H, fr.Bayer)
	}
	if !ok {
		return ChannelSky{}, false
	}
	if normalizedPixels(im) {
		sky = ChannelSky{R: sky.R * adu16Scale, G: sky.G * adu16Scale, B: sky.B * adu16Scale}
	}
	return sky, true
}

// normalizedPixels reports whether the image is float-normalized to [0,1] (Siril's 32-bit output)
// rather than raw 16-bit ADU. Judged on the observed maximum: a real astronomical frame always has
// SOMETHING above 1 ADU — a star, a hot pixel, the bias pedestal itself — so an all-below-1 frame is
// normalized, not black.
func normalizedPixels(im *fits.Image) bool {
	var max float32
	for _, plane := range im.Pix {
		for _, v := range plane {
			if v > max {
				max = v
			}
		}
	}
	return max <= 1.0
}

// sampleFrames picks up to n frames spread across the set rather than the first n, so a sequence
// whose first subs were shot through cloud or during a meridian flip cannot define the night.
func sampleFrames(frames []*Frame, n int) []*Frame {
	if len(frames) <= n {
		return frames
	}
	out := make([]*Frame, 0, n)
	step := float64(len(frames)) / float64(n)
	for i := 0; i < n; i++ {
		out = append(out, frames[int(float64(i)*step)])
	}
	return out
}

// median of a non-empty slice (the caller guarantees it).
func median(vals []float64) float64 {
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}
