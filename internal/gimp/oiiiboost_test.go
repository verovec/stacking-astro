package gimp

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOIIIBoostCurve pins the soft-shoulder boost the runbook settled on after the "fake cyan plate"
// (v8): a LINEAR multiply on the [OIII] layer drives its bright cores flat against white, and a flat
// core reads as a pasted plate of colour rather than as light. The shoulder lifts the mid tones —
// where the faint teal actually lives — and rolls the top off asymptotically, so the core is always
// brighter than its surroundings and never equal to them.
func TestOIIIBoostCurve(t *testing.T) {
	const knee, ceil = oiiiBoostKnee, oiiiBoostCeil

	t.Run("below the shoulder the boost is exactly the multiply", func(t *testing.T) {
		// Nothing is compressed until the multiplied value passes the knee — the faint [OIII] the
		// boost exists for must get the full factor, not a fraction of it.
		for _, f := range []float64{1.25, 1.35, 1.6} {
			for _, v := range []float64{0.0, 0.05, 0.2, 0.35} {
				if v*f > knee {
					continue
				}
				assert.InDelta(t, v*f, oiiiBoost(v, f), 1e-12, "f=%v v=%v", f, v)
			}
		}
	})

	t.Run("off by default — 1.0 is the identity", func(t *testing.T) {
		for _, v := range []float64{0, 0.1, 0.5, 0.6, 0.85, 1.0} {
			assert.InDelta(t, v, oiiiBoost(v, 1.0), 1e-12, "v=%v", v)
		}
	})

	t.Run("monotone in the input", func(t *testing.T) {
		for _, f := range []float64{1.0, 1.25, 1.35, 1.6} {
			prev := -1.0
			for i := 0; i <= 2000; i++ {
				v := float64(i) / 2000
				got := oiiiBoost(v, f)
				require.GreaterOrEqual(t, got, prev, "f=%v not monotone at v=%v", f, v)
				prev = got
			}
		}
	})

	t.Run("monotone in the factor once the boost is on", func(t *testing.T) {
		for _, v := range []float64{0.1, 0.3, 0.5, 0.75, 1.0} {
			prev := -1.0
			for _, f := range []float64{1.01, 1.1, 1.25, 1.35, 1.5, 1.6} {
				got := oiiiBoost(v, f)
				require.GreaterOrEqual(t, got, prev, "v=%v f=%v", v, f)
				prev = got
			}
		}
	})

	t.Run("switching the boost ON engages the shoulder — a deliberate step, and only above the knee",
		func(t *testing.T) {
			// f=1 is a pass-through: the layer keeps whatever highlights it had, shoulder included or
			// not, so a default run is byte-identical to before this knob existed. The moment the knob
			// moves, the roll-off engages — which means a core sitting at 1.0 comes DOWN (1.0 → ~0.93
			// at f=1.1) even though the knob says "boost".
			//
			// That is the feature, not a wart: the knob's whole purpose is that bright [OIII] stops
			// clipping flat, and compressing the top is how. What must never happen is the step
			// touching the faint end, where the boost is supposed to be a clean multiply.
			assert.InDelta(t, 0.0, oiiiBoost(0, 1.1), 1e-12, "black stays black at any factor")
			for _, v := range []float64{0.1, 0.3, 0.5, 0.59} {
				assert.InDelta(t, v, oiiiBoost(v, 1.0), 1e-12, "v=%v moved with the boost off", v)
				assert.Greater(t, oiiiBoost(v, 1.1), v, "v=%v below the knee must gain, not lose", v)
			}
			assert.Less(t, oiiiBoost(1.0, 1.1), oiiiBoost(1.0, 1.0),
				"a saturated core is pulled off white the moment the shoulder engages")
		})

	t.Run("bounded below the ceiling — the core NEVER clips flat", func(t *testing.T) {
		// The whole point. A saturated core at 1.0, pushed by the most aggressive factor the knob can
		// reach, must still land under the ceiling with room to spare.
		for _, f := range []float64{1.25, 1.35, 1.6} {
			got := oiiiBoost(1.0, f)
			assert.Less(t, got, ceil, "f=%v reached the ceiling", f)
			assert.Less(t, got, 1.0, "f=%v clipped to white", f)
		}
		// Beyond the knob's range the tanh saturates in float64 and the curve TOUCHES the ceiling.
		// That is the asymptote doing its job, and 0.98 is still not white — the guarantee that
		// matters (never clipping) holds for any factor, however absurd.
		for _, f := range []float64{10, 1e6} {
			got := oiiiBoost(1.0, f)
			assert.LessOrEqual(t, got, ceil, "f=%v exceeded the ceiling", f)
			assert.Less(t, got, 1.0, "f=%v clipped to white", f)
		}
	})

	t.Run("zero clipped pixels on a saturated core", func(t *testing.T) {
		// A synthetic core: a bright plateau surrounded by mid tones. After the boost every pixel must
		// still be strictly below 1.0, and the plateau must stay strictly ABOVE its surroundings —
		// a core that merely stops clipping but flattens into its halo is the same visual failure.
		for _, f := range []float64{1.25, 1.35, 1.6} {
			core, halo := oiiiBoost(1.0, f), oiiiBoost(0.75, f)
			assert.Less(t, core, 1.0, "f=%v", f)
			assert.Greater(t, core, halo, "f=%v flattened the core into its halo", f)
		}
	})

	t.Run("the curve is the one in coreShoulderLUT, not a new shape", func(t *testing.T) {
		// At f=1 the boost must reproduce the package's existing shoulder exactly — one shape, so a
		// future change to the highlight roll-off cannot silently diverge between the two.
		span := ceil - knee
		for _, v := range []float64{0.61, 0.7, 0.9, 1.0} {
			want := knee + span*math.Tanh((v-knee)/span)
			assert.InDelta(t, want, oiiiBoost(v, 1.0+1e-9), 1e-6, "v=%v", v)
		}
	})
}

// TestOIIIBoostLUT_FeedsGimp: the boost reaches GIMP as an explicit curve, so it has to be sampled
// into the same LUT resolution every other curve in this package uses.
func TestOIIIBoostLUT_FeedsGimp(t *testing.T) {
	lut := oiiiBoostLUT(1.35)

	require.Len(t, lut, coreShoulderSamples)
	assert.InDelta(t, 0.0, lut[0], 1e-12, "black stays black")
	assert.Less(t, lut[len(lut)-1], oiiiBoostCeil, "white lands under the ceiling")
	for i := 1; i < len(lut); i++ {
		require.GreaterOrEqual(t, lut[i], lut[i-1], "LUT must be monotone at %d", i)
	}
}
