package calib

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// oscLight is a one-shot-colour light set on a given night.
func oscLight(night string) inspect.SetKey {
	return inspect.SetKey{
		Type: inspect.Light, Object: "NGC7000", Filter: filters.Color,
		ExposureMs: 120_000, Gain: 100, Offset: 50, Bin: 1, Session: night, Color: true,
	}
}

// oscFlat is a per-night flat master, optionally carrying a measured filter set.
func oscFlat(night string, fs filters.FilterSet, frames int) Master {
	return Master{
		Type: MasterFlat, Filter: "", ExposureMs: 2000, Gain: 100, Offset: 50, Bin: 1,
		FrameCount: frames, Session: night, FilterSet: fs,
		Path: "flat_" + night + "_" + string(fs) + ".fits",
	}
}

// TestPickFlat_FilterSet is the heart of the card: a dual-band night must never be flat-fielded with
// a broadband flat.
//
// The bug it closes is quiet. Calibration frames carry an empty Filter and one-shot-colour lights
// read "RGB", so bestFlat's filter-matched pass ALWAYS misses and falls through to the filter-blind
// pass — which happily hands a broadband flat to a dual-band night. The illumination profile of a
// dual-band clip is nothing like an unfiltered one, so that flat corrupts the colour response of
// every frame it divides, and the only trace was a generic cross-filter note.
func TestPickFlat_FilterSet(t *testing.T) {
	const nightA, nightB = "2026-07-29", "2026-08-02"

	tests := []struct {
		name       string
		light      inspect.SetKey
		lightSet   filters.FilterSet
		masters    []Master
		wantPath   string // "" = no flat at all
		wantNote   string // substring required in the notes ("" = none required)
		forceCalib bool
	}{
		{
			name:     "both sides known and equal — the same-set flat is used",
			light:    oscLight(nightA),
			lightSet: filters.FilterSetDualband,
			masters: []Master{
				oscFlat(nightB, filters.FilterSetBroadband, 50),
				oscFlat(nightB, filters.FilterSetDualband, 20),
			},
			wantPath: "flat_2026-08-02_dualband.fits",
		},
		{
			name:     "a cross-set flat is not a candidate, even when it is the only one",
			light:    oscLight(nightA),
			lightSet: filters.FilterSetDualband,
			masters:  []Master{oscFlat(nightB, filters.FilterSetBroadband, 50)},
			wantPath: "",
			wantNote: "dualband",
		},
		{
			// The whole point of excluding it: a deeper, same-night broadband flat would otherwise win
			// every tiebreak against the correct one.
			name:     "a deeper same-night cross-set flat still loses to the correct set",
			light:    oscLight(nightA),
			lightSet: filters.FilterSetBroadband,
			masters: []Master{
				oscFlat(nightA, filters.FilterSetDualband, 200),
				oscFlat(nightB, filters.FilterSetBroadband, 5),
			},
			wantPath: "flat_2026-08-02_broadband.fits",
		},
		{
			name:     "light unknown — today's ranking, unchanged",
			light:    oscLight(nightA),
			lightSet: filters.FilterSetUnknown,
			masters: []Master{
				oscFlat(nightB, filters.FilterSetBroadband, 5),
				oscFlat(nightA, filters.FilterSetDualband, 5),
			},
			wantPath: "flat_2026-07-29_dualband.fits", // same night wins, as before
		},
		{
			name:     "flat unknown — today's ranking, unchanged",
			light:    oscLight(nightA),
			lightSet: filters.FilterSetDualband,
			masters: []Master{
				oscFlat(nightA, filters.FilterSetUnknown, 5),
				oscFlat(nightB, filters.FilterSetDualband, 50),
			},
			wantPath: "flat_2026-07-29_unknown.fits", // same night still wins
		},
		{
			// force_calibration_frames drops the other gates; it must drop this one too, or the user's
			// "apply my masters anyway" would silently keep refusing.
			name:       "force applies a cross-set flat, mismatch and all",
			light:      oscLight(nightA),
			lightSet:   filters.FilterSetDualband,
			masters:    []Master{oscFlat(nightB, filters.FilterSetBroadband, 50)},
			forceCalib: true,
			wantPath:   "flat_2026-08-02_broadband.fits",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := LightRef{Key: tt.light, FilterSet: tt.lightSet}
			sel := MatchForRef(ref, tt.masters, nil, tt.forceCalib)

			if tt.wantPath == "" {
				assert.Nil(t, sel.Flat, "expected no flat, got %+v", sel.Flat)
			} else {
				require.NotNil(t, sel.Flat, "expected %s", tt.wantPath)
				assert.Equal(t, tt.wantPath, sel.Flat.Path)
			}
			if tt.wantNote != "" {
				assert.Contains(t, strings.ToLower(strings.Join(sel.Notes, " | ")), tt.wantNote)
			}
		})
	}
}

// TestMatchForLight_FilterSetUnknownIsByteIdentical is the regression guard for every existing
// caller: the old entry points must behave exactly as before, which they do by asserting nothing
// about the filter set.
func TestMatchForLight_FilterSetUnknownIsByteIdentical(t *testing.T) {
	light := oscLight("2026-07-29")
	masters := []Master{
		oscFlat("2026-08-02", filters.FilterSetBroadband, 50),
		oscFlat("2026-07-29", filters.FilterSetDualband, 5),
	}

	legacy := MatchForLight(light, masters)
	viaRef := MatchForRef(LightRef{Key: light}, masters, nil, false)

	require.NotNil(t, legacy.Flat)
	require.NotNil(t, viaRef.Flat)
	assert.Equal(t, legacy.Flat.Path, viaRef.Flat.Path)
	assert.Equal(t, legacy.Notes, viaRef.Notes)
}

// TestPickFlat_FilterSet_DarksUnaffected: a dark and a bias are closed-shutter exposures. No light
// reaches the sensor, so the clip filter in the train cannot matter — gating them on it would strand
// perfectly good masters.
func TestPickFlat_FilterSet_DarksUnaffected(t *testing.T) {
	light := oscLight("2026-07-29")
	dark := Master{
		Type: MasterDark, ExposureMs: 120_000, Gain: 100, Offset: 50, Bin: 1,
		FrameCount: 30, Path: "dark.fits", FilterSet: filters.FilterSetBroadband,
	}
	bias := Master{
		Type: MasterBias, Gain: 100, Offset: 50, Bin: 1,
		FrameCount: 50, Path: "bias.fits", FilterSet: filters.FilterSetBroadband,
	}

	sel := MatchForRef(LightRef{Key: light, FilterSet: filters.FilterSetDualband},
		[]Master{dark, bias}, nil, false)

	require.NotNil(t, sel.Dark, "a dual-band light must still take a broadband-tagged dark")
	assert.Equal(t, "dark.fits", sel.Dark.Path)
	require.NotNil(t, sel.Bias)
	assert.Equal(t, "bias.fits", sel.Bias.Path)
}
