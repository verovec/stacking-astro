package inspect

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// TestClassifyFilterSet pins the two measured discriminators against the numbers in the runbooks
// (ngc7000-narrowband-plus-broadband §0, session-intake §2):
//
//	sky above the bias pedestal, per 120 s : dual-band 2–6 ADU   · broadband 30–45 ADU
//	background colour                      : dual-band R>G>B     · broadband G/B-dominant
//
// BOTH must agree. One signal alone is not enough: amplitude alone is fooled by a bright sky or a
// slower lens, and colour alone is fooled by a red-heavy light-pollution gradient. When they
// disagree the answer is unknown — the whole point is that a wrong verdict silently mis-stacks a
// night, while an honest "unknown" just falls back to today's behaviour.
func TestClassifyFilterSet(t *testing.T) {
	tests := []struct {
		name       string
		sky        ChannelSky
		exposureMs int64
		want       filters.FilterSet
	}{
		{
			name:       "dual-band at 120s — faint, red-dominant",
			sky:        ChannelSky{R: 6, G: 3, B: 2},
			exposureMs: 120_000,
			want:       filters.FilterSetDualband,
		},
		{
			name:       "dual-band at the faint end of the measured band",
			sky:        ChannelSky{R: 2.4, G: 1.3, B: 0.9},
			exposureMs: 120_000,
			want:       filters.FilterSetDualband,
		},
		{
			name:       "broadband at 120s — bright, blue-dominant",
			sky:        ChannelSky{R: 30, G: 38, B: 42},
			exposureMs: 120_000,
			want:       filters.FilterSetBroadband,
		},
		{
			name:       "broadband at 120s — green-dominant",
			sky:        ChannelSky{R: 28, G: 40, B: 35},
			exposureMs: 120_000,
			want:       filters.FilterSetBroadband,
		},
		{
			// session-intake §2: at 60 s the sky is 2–4 ADU and integer medians cannot resolve the
			// colour, so the caller measures MEANS. Normalizing to 120 s must recover the same verdict.
			name:       "dual-band at 60s — normalized to the 120s band",
			sky:        ChannelSky{R: 3, G: 1.8, B: 1.2},
			exposureMs: 60_000,
			want:       filters.FilterSetDualband,
		},
		{
			name:       "broadband at 60s — normalized to the 120s band",
			sky:        ChannelSky{R: 14, G: 19, B: 21},
			exposureMs: 60_000,
			want:       filters.FilterSetBroadband,
		},
		{
			name:       "broadband at 300s — long subs normalize down",
			sky:        ChannelSky{R: 70, G: 95, B: 105},
			exposureMs: 300_000,
			want:       filters.FilterSetBroadband,
		},
		{
			// A red light-pollution gradient on a broadband night: bright sky, yet R leads.
			name:       "signals disagree — bright but red-dominant",
			sky:        ChannelSky{R: 45, G: 35, B: 30},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
		{
			// A very dark broadband site: faint sky, but the colour is not dual-band's.
			name:       "signals disagree — faint but green-dominant",
			sky:        ChannelSky{R: 2, G: 4, B: 3},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
		{
			name:       "amplitude falls in the gap between the two bands",
			sky:        ChannelSky{R: 20, G: 12, B: 10},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
		{
			// R highest but B is NOT the lowest — session-intake is explicit that only the OIII window
			// feeds blue, so a dual-band night always bottoms out in B.
			name:       "red-dominant but blue is not the floor",
			sky:        ChannelSky{R: 6, G: 2, B: 3},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
		{
			name:       "no exposure — cannot normalize, so no verdict",
			sky:        ChannelSky{R: 6, G: 3, B: 2},
			exposureMs: 0,
			want:       filters.FilterSetUnknown,
		},
		{
			name:       "all zero — an unmeasured or fully-subtracted set",
			sky:        ChannelSky{},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
		{
			name:       "negative sky (over-subtracted floor) is not evidence",
			sky:        ChannelSky{R: -2, G: -3, B: -4},
			exposureMs: 120_000,
			want:       filters.FilterSetUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyFilterSet(tt.sky, tt.exposureMs, 1.0))
		})
	}
}

// TestChannelSky_Dominance documents the colour half on its own, so a future threshold change to the
// amplitude half cannot quietly alter what "red-dominant" means.
func TestChannelSky_Dominance(t *testing.T) {
	assert.True(t, ChannelSky{R: 6, G: 3, B: 2}.redDominant(), "R highest and B lowest")
	assert.False(t, ChannelSky{R: 6, G: 2, B: 3}.redDominant(), "B must be the floor")
	assert.False(t, ChannelSky{R: 3, G: 6, B: 2}.redDominant(), "R must lead")

	assert.True(t, ChannelSky{R: 28, G: 40, B: 35}.greenBlueDominant())
	assert.True(t, ChannelSky{R: 30, G: 38, B: 42}.greenBlueDominant())
	assert.False(t, ChannelSky{R: 45, G: 35, B: 30}.greenBlueDominant())
}

// ASI2600MC conversion factors, from the EGAIN card of the user's own captures.
const (
	egainGain0   = 0.768 // e-/ADU at analogue gain 0
	egainGain100 = 0.243 // e-/ADU at analogue gain 100
)

// The bug this conversion exists for: an ADU threshold measures the analogue gain as much as the
// sky. A real M31 dual-band capture shot at gain 100 read 27.5 ADU per 120 s — over the broadband
// bound — and was classified BROADBAND, confidently and wrongly. In electrons it is 6.7.
func TestClassifyFilterSet_HighGainDualbandIsNotBroadband(t *testing.T) {
	// Measured on input/M31-DB: green-dominant, because [OIII] lands on green and two of the four
	// Bayer photosites are green — so the colour test abstains on a galaxy.
	sky := ChannelSky{R: 23.6, G: 37.2, B: 20.8} // amplitude 27.2 ADU per 120 s

	got := ClassifyFilterSet(sky, 120_000, egainGain100)

	assert.NotEqual(t, filters.FilterSetBroadband, got,
		"27.2 ADU at gain 100 is 6.6 electrons — a dual-band sky, not a broadband one")
	assert.Equal(t, filters.FilterSetUnknown, got,
		"the colour signal disagrees on a galaxy, so the honest answer is no verdict")
}

// A sky that lands in the ADU dead band at high gain is comfortably dual-band in electrons. This is
// the IC 1848 case: its brighter frames read 19.4 ADU per 120 s and the night went unclassified.
func TestClassifyFilterSet_DeadBandInADUClassifiesInElectrons(t *testing.T) {
	sky := ChannelSky{R: 25.6, G: 19.4, B: 13.2} // amplitude 19.4 ADU per 120 s, red-dominant

	assert.Equal(t, filters.FilterSetDualband, ClassifyFilterSet(sky, 120_000, egainGain100),
		"19.4 ADU at gain 100 is 4.7 electrons — inside the dual-band band")
}

// The property the conversion buys: the SAME sky measured at two analogue gains must classify the
// same. In ADU the two differ by the gain ratio and can straddle a threshold.
func TestClassifyFilterSet_GainInvariant(t *testing.T) {
	tests := []struct {
		name      string
		electrons float64
		redLeads  bool
		want      filters.FilterSet
	}{
		{"a dual-band sky", 6.0, true, filters.FilterSetDualband},
		{"a broadband sky", 35.0, false, filters.FilterSetBroadband},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, eg := range []float64{egainGain0, egainGain100} {
				adu := tt.electrons / eg // the same sky, as this gain records it
				sky := ChannelSky{R: adu, G: adu, B: adu}
				if tt.redLeads {
					sky = ChannelSky{R: adu * 1.5, G: adu, B: adu * 0.5}
				} else {
					sky = ChannelSky{R: adu * 0.5, G: adu * 1.5, B: adu}
				}
				assert.Equal(t, tt.want, ClassifyFilterSet(sky, 120_000, eg),
					"EGAIN %.3f: %.1f ADU is the same %.1f electrons", eg, adu, tt.electrons)
			}
		})
	}
}

// Without EGAIN the sky can only be stated in ADU, which is not a unit the thresholds can compare
// against. Declining costs nothing; guessing in the wrong unit mis-stacks a whole night.
func TestClassifyFilterSet_NoEGainIsNoVerdict(t *testing.T) {
	dualband := ChannelSky{R: 6, G: 3, B: 2}
	broadband := ChannelSky{R: 30, G: 38, B: 42}

	for _, eg := range []float64{0, -1} {
		assert.Equal(t, filters.FilterSetUnknown, ClassifyFilterSet(dualband, 120_000, eg))
		assert.Equal(t, filters.FilterSetUnknown, ClassifyFilterSet(broadband, 120_000, eg))
	}
}
