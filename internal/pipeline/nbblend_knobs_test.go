package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/mode"
)

// TestNBKnobs_ClampAndRoundTrip: both knobs travel the same road every tunable does — wire JSON →
// supervisePatch → preset → back out to the value surface — and both are CLAMPED, because they reach
// GIMP as an opacity and a curve where an out-of-range value is not a worse picture but a broken one.
func TestNBKnobs_ClampAndRoundTrip(t *testing.T) {
	tests := []struct {
		name          string
		nbBlend       float64
		oiiiBoost     float64
		wantNBBlend   float64
		wantOIIIBoost float64
	}{
		{"documented values pass through", 0.6, 1.35, 0.6, 1.35},
		{"the runbook's over-cooked end is still reachable", 1.0, 1.6, 1.0, 1.6},
		{"a negative blend clamps to silence", -0.5, 1.2, 0, 1.2},
		{"a blend above full clamps to full", 4, 1.2, 1, 1.2},
		{"a boost below 1 clamps to off — never attenuate through the boost", 0.5, 0.2, 0.5, 1},
		{"a boost past the shoulder's useful range clamps", 0.5, 99, 0.5, 1.6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mode.For(mode.Deepsky)
			patch := supervisePatch{NBBlend: &tt.nbBlend, OIIIBoost: &tt.oiiiBoost}

			p = clampPreset(patch.apply(p)) // the idiom every caller uses

			assert.InDelta(t, tt.wantNBBlend, p.NBBlend, 1e-9)
			assert.InDelta(t, tt.wantOIIIBoost, p.OIIIBoost, 1e-9)
			// The value surface is what the UI reads back; a knob the user set must be visible there.
			vals := deepskyParams(p)
			assert.InDelta(t, tt.wantNBBlend, vals["nb_blend"], 1e-9)
			assert.InDelta(t, tt.wantOIIIBoost, vals["oiii_boost"], 1e-9)
		})
	}
}

// TestNBKnobs_DefaultsAreOff: every deep-sky preset must ship with the boost off and the blend at
// full, so a run that says nothing about them composites exactly as it did before they existed.
func TestNBKnobs_DefaultsAreOff(t *testing.T) {
	for _, m := range []mode.Mode{mode.Deepsky, mode.Nebula} {
		p := mode.For(m)
		assert.InDelta(t, 1.0, p.NBBlend, 1e-9, "%s: the narrowband weight starts at full", m)
		assert.InDelta(t, 1.0, p.OIIIBoost, 1e-9, "%s: the boost starts off", m)
	}
}

// TestNBKnobs_AreTierA is what makes re-mixing cheap, and it is acceptance criterion 4 of the card.
// Both knobs act at COMPOSITE time, on layers the Tier-A checkpoint already persisted (base + ha +
// oiii under linear/), so changing either must classify as Tier A — a re-mix that re-stacked the
// frames would take the run from seconds to hours and defeat the point of the slider.
func TestNBKnobs_AreTierA(t *testing.T) {
	base := mode.For(mode.Deepsky)

	for _, tc := range []struct {
		name   string
		mutate func(p *mode.Preset)
	}{
		{"nb_blend", func(p *mode.Preset) { p.NBBlend = 0.4 }},
		{"oiii_boost", func(p *mode.Preset) { p.OIIIBoost = 1.35 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := base
			tc.mutate(&next)
			require.NotEqual(t, base, next, "the fixture must actually change something")
			assert.Equal(t, tierA, tierOf(base, next), "%s must re-composite, never re-stack", tc.name)
			assert.True(t, composeChanged(base, next), "%s must register as a composite change", tc.name)
		})
	}
}
