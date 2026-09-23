package pipeline

// duoband.go lets a ONE-SHOT-COLOUR duo-band capture reach the narrowband palettes.
//
// A dual-band filter (L-eXtreme, L-Ultimate and friends) in front of a Bayer sensor already performs
// the separation a filter wheel gives the mono path: Hα at 656 nm lands on the RED pixels, [OIII] at
// 500.7 nm on the GREEN and BLUE ones. The physical information is there — it just arrives as three
// planes of one exposure instead of three exposures. The palette engine could not see it, because
// resolvePalette short-circuits every colour run to a pass-through (paletteSpecs is written in
// filter names a colour sensor does not have), so the Hubble-style renditions this data is actually
// shot for were unreachable and it could only ever be finished as muddy broadband RGB.
//
// Splitting the stacked master into pseudo-Hα and pseudo-[OIII] channel files puts the run back on
// the ordinary narrowband road: hoo/foraxx resolve, rgbcomp composes them, and Foraxx's GExpr pixel
// math works unchanged.
//
// [OIII] is the per-pixel MAXIMUM of green and blue, not their mean. The 500.7 nm line sits on the
// shoulder between the two Bayer passbands, so each catches a different fraction of it; averaging
// folds the weaker one into the signal and costs SNR exactly where the [OIII] is faintest, while the
// maximum keeps whichever pixel actually caught the line. Nothing is invented — both planes are
// measurements of the same line.
//
// Only Hα and [OIII] are synthesized. A duo-band filter passes no [SII], so "sho" cannot be honoured
// from this data and is left to the palette engine's own fallback chain (→ hoo, with its note). The
// Hubble-like rendition available here is "foraxx", whose dynamic green is the geometric mean of the
// two real lines.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/fits"
	"github.com/verove-jordan/astronomy/internal/inspect"
	"github.com/verove-jordan/astronomy/internal/mode"
)

// Basenames of the synthesized channel files, in the same Siril-relative form as every other entry
// in the channels map (outDir is the working directory and `setext fits` appends the extension).
const (
	duobandHaBase   = "syn_Ha"
	duobandOIIIBase = "syn_OIII"
)

// wantsNarrowbandPalette reports whether the preset asks for a palette that maps emission lines to
// R/G/B. It is what gates the split: a colour run that wants its natural rendition must keep the
// untouched pass-through it has always had.
func wantsNarrowbandPalette(p *mode.Preset) bool {
	if p == nil {
		return false
	}
	spec, ok := paletteSpecs[strings.ToLower(strings.TrimSpace(p.Palette))]
	return ok && spec.Narrowband
}

// measuredBroadband reports whether the scan's pixels say a dual-band clip was NOT in the train:
// at least one LIGHT set classified broadband, and none classified dual-band.
//
// This is the guard the split was missing. Everything above assumes the three planes are two
// emission lines, and on a real duo-band capture they are. On a BROADBAND capture they are a
// continuum: pseudo-Hα becomes plain red continuum, and pseudo-[OIII] becomes the per-pixel maximum
// of two INDEPENDENT noisy planes. A maximum of independents is not a measurement of one line, it
// is a noise rectifier — it biases the sky upward wherever the noise is locally correlated, so the
// palette paints those places as [OIII] and the finish shows grey-green blotches on the red. A
// galaxy shot unfiltered and finished in "foraxx" went down exactly this road.
//
// Only a POSITIVE broadband verdict blocks the split, mirroring splitsFilterSets: an unclassified
// set is silence, not evidence, and a scan the pixels could not settle keeps today's behaviour.
// One dual-band set anywhere is enough to allow it — a mixed capture's dual-band lane is real
// emission data, and filtersetlanes.go has already consumed it by the time this runs.
func measuredBroadband(inv *inspect.Inventory) bool {
	if inv == nil {
		return false
	}
	broadband := false
	for _, set := range inv.Sets {
		if set.Key.Type != inspect.Light {
			continue
		}
		switch inv.FilterSets[set.Key.ID()] {
		case filters.FilterSetDualband:
			return false
		case filters.FilterSetBroadband:
			broadband = true
		}
	}
	return broadband
}

// splitDuoband writes pseudo-Hα and pseudo-[OIII] channel files beside the colour master and returns
// their basenames. masterPath is the stacked one-shot-colour master; it is never modified.
func splitDuoband(masterPath, outDir string) (haBase, oiiiBase string, err error) {
	im, err := fits.ReadImage(masterPath)
	if err != nil {
		return "", "", fmt.Errorf("read colour master: %w", err)
	}
	if im.C < 3 {
		return "", "", fmt.Errorf("colour master has %d plane(s), need 3 to separate Hα from [OIII]", im.C)
	}
	ha := fits.NewImage(im.W, im.H, 1)
	copy(ha.Pix[0], im.Pix[0])

	oiii := fits.NewImage(im.W, im.H, 1)
	g, b, o := im.Pix[1], im.Pix[2], oiii.Pix[0]
	for i := range o {
		if g[i] > b[i] {
			o[i] = g[i]
		} else {
			o[i] = b[i]
		}
	}
	if err := ha.WriteFITS(filepath.Join(outDir, duobandHaBase+".fits")); err != nil {
		return "", "", fmt.Errorf("write Hα channel: %w", err)
	}
	if err := oiii.WriteFITS(filepath.Join(outDir, duobandOIIIBase+".fits")); err != nil {
		return "", "", fmt.Errorf("write [OIII] channel: %w", err)
	}
	return duobandHaBase, duobandOIIIBase, nil
}

// duobandChannels returns a channels map with the synthesized Hα/[OIII] entries added, leaving the
// caller's map untouched, plus a note for the run. It is a no-op (nil note) when the run is not a
// colour run, when no narrowband palette was asked for, or when the master cannot be split — in
// every one of those cases the caller keeps the map it already had and the ordinary colour
// pass-through applies. inv (may be nil) is the run's scan, consulted only to refuse the split on a
// capture measured BROADBAND — see measuredBroadband.
func duobandChannels(p *mode.Preset, inv *inspect.Inventory, channels map[string]string, outDir string) (map[string]string, string) {
	if !isColorRun(p) || !wantsNarrowbandPalette(p) {
		return channels, ""
	}
	// No emission lines were captured, so there are none to separate. Leaving Ha/OIII out lets
	// resolvePalette fall back down its own chain to the natural rendition this data actually is.
	if measuredBroadband(inv) {
		return channels, fmt.Sprintf(
			"%q palette not honoured: these lights measured BROADBAND (no dual-band clip filter), so there "+
				"are no emission lines to separate — a pseudo-[OIII] taken as max(green,blue) of a continuum "+
				"rectifies noise into colour blotches instead of recovering signal. Finishing as colour",
			strings.ToLower(strings.TrimSpace(p.Palette)))
	}
	// A mixed-filter-set run has already produced these from its dual-band lane (filtersetlanes.go),
	// against the right master. Splitting again would overwrite them with the BROADBAND stack's
	// planes — pure continuum sold as emission lines.
	if _, ok := channels["Ha"]; ok {
		return channels, ""
	}
	src := oscSource(channels)
	if src == "" {
		return channels, ""
	}
	ha, oiii, err := splitDuoband(filepath.Join(outDir, src+".fits"), outDir)
	if err != nil {
		return channels, "duo-band split skipped, finishing as colour: " + err.Error()
	}
	out := make(map[string]string, len(channels)+2)
	for k, v := range channels {
		out[k] = v
	}
	out["Ha"], out["OIII"] = ha, oiii
	return out, fmt.Sprintf(
		"duo-band one-shot-colour: separated pseudo-Hα (red pixels) and pseudo-[OIII] (max of green/blue) "+
			"from the colour master so the %q palette can map them", strings.ToLower(strings.TrimSpace(p.Palette)))
}
