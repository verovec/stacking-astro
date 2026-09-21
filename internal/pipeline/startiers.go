package pipeline

// The star-presence set: every image run ships the finish at five star levels — starless (0 %), the
// 25/50/75 % blends, and the untouched final (100 %) — so choosing how present the stars are is a
// screen-side choice instead of a re-process (tasks-context/ngc7000-narrowband-plus-broadband.md §6).
//
// One StarNet pass per run produces the starless master, SHARED with the StarReduce blend (see
// starlessTIFF); the intermediate levels are then pure arithmetic in Go (postprocess.BlendStars), so
// each extra level costs a decode+encode rather than a GIMP round-trip. Everything is soft-fail: no
// StarNet, no tiers — the full-stars final is never touched, and a run never fails over a tier.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/verove-jordan/astronomy/internal/postprocess"
	"github.com/verove-jordan/astronomy/internal/siril"
	"github.com/verove-jordan/astronomy/internal/starnet"
)

// starTierPercents are the intermediate star levels, as a percentage of the original star
// brightness kept. 0 (starless) and 100 (the final itself) bracket them and are added separately.
var starTierPercents = []int{25, 50, 75}

// starlessBase is the shared name of the star-removal output: <outDir>/final-starless.{tif,png}.
const starlessBase = "final-starless"

// starTierName is the deliverable name for one intermediate level, e.g. final-50-stars.png.
func starTierName(percent int) string { return fmt.Sprintf("final-%d-stars.png", percent) }

// emitStarTiers renders the star-presence set next to the finished composite. Registered as a defer
// in finishAligned so it runs once, on whichever finish path set res.Final (supervised / GIMP /
// Siril fallback) and AFTER any promotion of a repaired winner to final.*, so the tiers always match
// the final that shipped. Produced files are APPENDED to res.Final.Outputs (after final.png, which
// stays the gallery hero) and recorded in res.Final.StarTiers for the results UI.
func emitStarTiers(ctx context.Context, opts Options, res *Result, outDir string) {
	if res == nil || res.Final == nil || opts.Preset == nil || opts.Starnet == nil || ctx.Err() != nil {
		return
	}
	if !opts.Preset.StarTiers || len(res.Final.StarTiers) > 0 {
		return
	}
	finalTif := filepath.Join(outDir, "final.tif")
	finalPng := filepath.Join(outDir, "final.png")
	if !fileExists(finalTif) || !fileExists(finalPng) {
		warnLive(opts, res, "star tiers skipped: the finish produced no final.tif/final.png")
		return
	}

	starless, err := starlessTIFF(ctx, opts, finalTif, outDir, opts.beginStep("star tiers (StarNet)"))
	if err != nil {
		warnLive(opts, res, "star tiers skipped: "+err.Error())
		return
	}

	// Decode both sources ONCE — every level blends the same pair, and these are full-size 16-bit
	// renders (re-reading them per level was the bulk of the tier cost).
	withStars, err := postprocess.DecodeImage(finalTif)
	if err != nil {
		warnLive(opts, res, "star tiers skipped: "+err.Error())
		return
	}
	starlessImg, err := postprocess.DecodeImage(starless)
	if err != nil {
		warnLive(opts, res, "star tiers skipped: "+err.Error())
		return
	}

	// 0 % — the starless render itself, transcoded to a viewable PNG next to the TIFF.
	starlessPng := filepath.Join(outDir, starlessBase+".png")
	if err := postprocess.WritePNG(starlessPng, starlessImg); err != nil {
		warnLive(opts, res, "star tiers skipped: "+err.Error())
		return
	}
	tiers := []postprocess.StarTier{{Kind: "starless", Percent: 0, Png: starlessPng, Tif: starless}}

	// 25/50/75 % — the intermediate blends, in the same 16-bit domain as the final TIFF.
	for _, pct := range starTierPercents {
		out := filepath.Join(outDir, starTierName(pct))
		blended, err := postprocess.BlendStars(withStars, starlessImg, float64(pct)/100)
		if err == nil {
			err = postprocess.WritePNG(out, blended)
		}
		if err != nil {
			warnLive(opts, res, fmt.Sprintf("star tier %d%% skipped: %s", pct, err.Error()))
			continue
		}
		tiers = append(tiers, postprocess.StarTier{Kind: "stars", Percent: pct, Png: out})
	}

	// 100 % — the finish that already shipped; listed so the UI switcher is the full ladder, but NOT
	// re-appended to Outputs (it is already there, first, as the gallery hero).
	tiers = append(tiers, postprocess.StarTier{Kind: "final", Percent: 100, Png: finalPng, Tif: finalTif})

	res.Final.StarTiers = tiers
	for _, st := range tiers {
		if st.Percent == 100 {
			continue // already first in Outputs as the gallery hero
		}
		res.Final.Outputs = appendUniquePath(res.Final.Outputs, st.Png)
		if st.Tif != "" {
			res.Final.Outputs = appendUniquePath(res.Final.Outputs, st.Tif)
		}
	}
	res.Final.Notes = append(res.Final.Notes, "star-presence set: starless + 25/50/75 % star blends")
}

// starlessTIFF returns the starless render of withStarsTif, running StarNet at most ONCE per final:
// an existing <outDir>/final-starless.tif that is at least as new as the final it came from is
// reused, so the StarReduce blend and the tier set share a single (slow) pass whichever runs first.
// A promotion that rewrites final.tif (star-fix, supervised winner) makes the old starless stale and
// is therefore re-run — the tiers can never belong to a superseded final.
func starlessTIFF(ctx context.Context, opts Options, withStarsTif, outDir string, onProgress func(siril.Progress)) (string, error) {
	starless := filepath.Join(outDir, starlessBase+".tif")
	if fresh, err := isFresherThan(starless, withStarsTif); err == nil && fresh {
		return starless, nil
	}
	fwd := func(p starnet.Progress) {
		if onProgress != nil {
			onProgress(siril.Progress{Line: p.Line, Percent: p.Percent, Sample: p.Sample})
		}
	}
	if err := opts.Starnet.RemoveStars(ctx, withStarsTif, starless, starnet.Options{}, fwd); err != nil {
		return "", fmt.Errorf("StarNet star removal: %w", err)
	}
	if !fileExists(starless) {
		return "", fmt.Errorf("StarNet star removal: no output produced")
	}
	return starless, nil
}

// appendUniquePath adds path to list unless it is already there. The starless TIFF is SHARED with
// the star-reduction blend, which lists it the moment it produces one — appending blindly would
// give the run a doubled download entry and a doubled S3 upload of a full-size render.
func appendUniquePath(list []string, path string) []string {
	for _, p := range list {
		if p == path {
			return list
		}
	}
	return append(list, path)
}

// isFresherThan reports whether path exists and is no older than ref (both must exist).
func isFresherThan(path, ref string) (bool, error) {
	a, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	b, err := os.Stat(ref)
	if err != nil {
		return false, err
	}
	return !a.ModTime().Before(b.ModTime()), nil
}
