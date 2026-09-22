// cfaprobe.go answers "is this frame genuinely a colour mosaic?" from the PIXELS rather than from
// the FILTER card.
//
// The question matters because clearSpuriousBayer has to tell two situations apart that look
// identical in the header:
//
//   - an ASI1600MM (monochrome) behind a filter wheel, whose older ASICAP build stamps a BAYERPAT
//     card anyway. The card is meaningless and must be cleared, or the mono session routes down the
//     one-shot-colour path and loses every frame.
//   - an ASI2600MC (colour) behind a DUAL-BAND CLIP FILTER, whose capture program writes
//     FILTER='L-eXtreme'. That name is not a wheel slot, the BAYERPAT is real, and clearing it
//     destroys the session.
//
// A name cannot separate them — a user may legitimately call a clip filter "Ha" — so the pixels do.
// A real CFA puts its four sub-lattices under different dyes, so skyglow and flat-field QE split
// them by tens of percent, and crucially the two GREENS (same dye) stay together. A monochrome
// sensor is one response: its four sub-lattices differ only by noise, and a vignette or a gradient
// moves all four at once rather than splitting R from B.
package inspect

import (
	"math"

	"github.com/verove-jordan/astronomy/internal/fits"
)

// cfaVerdict is the probe's answer. cfaMaybe is a genuine third state, not an error: it keeps
// today's behaviour rather than guessing, exactly as the filter-set classifier declines.
type cfaVerdict string

const (
	cfaYes   cfaVerdict = "cfa"
	cfaNo    cfaVerdict = "mono"
	cfaMaybe cfaVerdict = "unknown"
)

const (
	// cfaMinColorSplit is the relative separation between the colour sub-lattices (R vs B vs the
	// green pair) above which the frame is a genuine mosaic. Real sensors split by 5–40 % on
	// skyglow; this sits well under that and well over sensor noise.
	cfaMinColorSplit = 0.03
	// cfaMaxFlatSplit is the separation below which the four sub-lattices are one response — a mono
	// sensor. Between the two is the declined band.
	cfaMaxFlatSplit = 0.012
	// cfaMaxGreenSplit bounds how far the two greens may differ and still read as a mosaic. They sit
	// under the SAME dye, so a frame that separates them as much as it separates R from B is showing
	// a gradient or a column artefact, not colour.
	cfaMaxGreenSplit = 0.5
	// cfaProbeFrames bounds the I/O, like the filter-set sampler.
	cfaProbeFrames = 3
)

// probeCFAStructure measures one frame. It needs a single-plane image of at least one full 2×2 cell;
// an already-debayered three-plane frame is not a mosaic question and is declined.
func probeCFAStructure(im *fits.Image) cfaVerdict {
	if im == nil || im.C != 1 || len(im.Pix) != 1 || im.W < 2 || im.H < 2 {
		return cfaMaybe
	}
	m00, m10, m01, m11, ok := subLatticeMeans(im.Pix[0], im.W, im.H)
	if !ok {
		return cfaMaybe
	}
	// In every Bayer pattern the two greens are the diagonal pair (1,0)/(0,1) of the cell; the other
	// diagonal holds R and B. That is true for RGGB, BGGR, GRBG and GBRG alike, which is why the
	// probe never needs to know WHICH pattern it is looking at.
	green := (m10 + m01) / 2
	mean := (m00 + m10 + m01 + m11) / 4
	if mean <= 0 {
		return cfaMaybe
	}
	colorSplit := spread(m00, green, m11) / mean
	greenSplit := math.Abs(m10-m01) / mean

	switch {
	case colorSplit >= cfaMinColorSplit && greenSplit <= colorSplit*cfaMaxGreenSplit:
		return cfaYes
	case colorSplit <= cfaMaxFlatSplit:
		return cfaNo
	default:
		return cfaMaybe
	}
}

// probeFramesCFA measures a bounded sample and requires them to AGREE. One frame is a weak witness:
// a passing cloud, a lens cap or a satellite trail can each flatten or fake the structure, and the
// cost of a wrong verdict here is a destroyed session.
func probeFramesCFA(frames []*Frame, load imageLoader) cfaVerdict {
	if len(frames) == 0 || load == nil {
		return cfaMaybe
	}
	verdict := cfaMaybe
	for _, fr := range sampleFrames(frames, cfaProbeFrames) {
		im, err := load(fr.Path)
		if err != nil || im == nil {
			return cfaMaybe // an unreadable sample is not evidence either way
		}
		v := probeCFAStructure(im)
		if v == cfaMaybe {
			return cfaMaybe
		}
		if verdict != cfaMaybe && v != verdict {
			return cfaMaybe // the samples disagree
		}
		verdict = v
	}
	return verdict
}

// subLatticeMeans returns the mean of each position in the 2×2 Bayer cell.
func subLatticeMeans(pix []float32, w, h int) (m00, m10, m01, m11 float64, ok bool) {
	if len(pix) < w*h || w < 2 || h < 2 {
		return 0, 0, 0, 0, false
	}
	var s00, s10, s01, s11 float64
	var n int
	for y := 0; y+1 < h; y += 2 {
		for x := 0; x+1 < w; x += 2 {
			s00 += float64(pix[y*w+x])
			s10 += float64(pix[y*w+x+1])
			s01 += float64(pix[(y+1)*w+x])
			s11 += float64(pix[(y+1)*w+x+1])
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0, 0, false
	}
	f := float64(n)
	return s00 / f, s10 / f, s01 / f, s11 / f, true
}

// spread is max−min over the given values.
func spread(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals[1:] {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	return hi - lo
}
