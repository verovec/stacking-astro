package gimp

import "math"

// The [OIII] soft-shoulder boost. Its knee and ceiling are the values the NGC7000 runbook settled on
// and are deliberately shared with coreShoulderLUT — one highlight roll-off in this package, not two
// that can drift apart.
const (
	oiiiBoostKnee = 0.60
	oiiiBoostCeil = 0.98
)

// oiiiBoost lifts one [OIII] sample by factor f without ever letting it clip.
//
// A plain multiply is what this replaces, and it failed in a specific, instructive way (runbook v8,
// the "fake cyan plate"): pushing the layer linearly drove its bright cores to 1.0, where they went
// FLAT — every pixel in the core equal to every other and equal to white. A flat core does not read
// as light, it reads as a pasted plate of colour, and no amount of opacity tuning afterwards
// recovers the structure that was destroyed.
//
// So the value is multiplied, and then anything past the knee is rolled off with a tanh that
// asymptotes to the ceiling: the faint mid-tone [OIII] the boost exists for gets the full factor,
// while the cores merely approach 0.98 and stay ordered among themselves. Prefer this over pushing
// oiii_screen, which raises the rims along with the cores.
//
// f <= 1 is the identity: the knob is off by default and costs nothing.
func oiiiBoost(v, f float64) float64 {
	if f <= 1 {
		return v
	}
	x := v * f
	if x <= oiiiBoostKnee {
		return x
	}
	span := oiiiBoostCeil - oiiiBoostKnee
	return oiiiBoostKnee + span*math.Tanh((x-oiiiBoostKnee)/span)
}

// oiiiBoostLUT samples oiiiBoost into the explicit curve GIMP applies to the [OIII] layer, at the
// same resolution as every other curve in this package.
func oiiiBoostLUT(f float64) []float64 {
	lut := make([]float64, coreShoulderSamples)
	for i := range lut {
		lut[i] = oiiiBoost(float64(i)/float64(coreShoulderSamples-1), f)
	}
	return lut
}

// nbBlendEpsilon is the opacity below which an emission screen is dropped rather than written at a
// rounded-to-zero strength: the script prints opacity as a whole percent, so anything under half a
// percent would emit "(gimp-layer-set-opacity oiii 0)" — a loaded, inert layer instead of an absent one.
const nbBlendEpsilon = 0.005

// nbBlended scales one emission screen opacity by the run's narrowband weight.
//
// blend <= 0 is UNSET and returns the opacity untouched. That asymmetry is deliberate: the zero value
// of a new field must leave every existing composite exactly as it was, and "blend at zero" is
// reachable with any small positive value, which nbBlendEpsilon then turns into a dropped layer.
func nbBlended(blend, opacity float64) float64 {
	if blend <= 0 || blend >= 1 {
		return opacity
	}
	if scaled := opacity * blend; scaled >= nbBlendEpsilon {
		return scaled
	}
	return 0
}
