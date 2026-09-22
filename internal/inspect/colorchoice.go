// colorchoice.go turns the user's answer to "is this a monochrome or a colour stack?" into a narrowed
// inventory. It is the REQUEST-level knob that sits upstream of the scan verdict (ColorModel, decided
// in colorModel) and upstream of the resolved live switch (mode.Preset.Color).
//
// Why a knob at all: the verdict is inferred from the pixels and the headers, which is right almost
// always and unrecoverable when it is wrong. A header-less camera, a folder holding two rigs' sessions,
// a capture program that stamps BAYERPAT on a mono sensor — each misroutes a whole run with no way for
// the user to say otherwise. An explicit choice is an ASSERTION: it narrows the lights to the ones that
// match, and fails loudly when none do rather than stacking the wrong half of the folder.
package inspect

import (
	"errors"
	"fmt"
	"strings"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// ColorChoice is the run request's colour-model knob. It is NOT ColorModel: there is no "mixed"
// choice (a user never asks for an unstackable folder) and there IS an "auto", which defers to the
// scan verdict exactly as every run did before the knob existed.
type ColorChoice string

const (
	// ChoiceAuto defers to the scan's own verdict. The default, and what an absent wire value means.
	ChoiceAuto ColorChoice = "auto"
	// ChoiceMono asserts a monochrome filter-wheel stack: colour lights are dropped.
	ChoiceMono ColorChoice = "mono"
	// ChoiceOSC asserts a one-shot-color stack: monochrome lights are dropped.
	ChoiceOSC ColorChoice = "osc"
)

// ErrNoLightsForColorModel is returned when the asserted colour model matches none of the scan's
// lights. Callers branch on it with errors.Is to tell "you picked the wrong knob" apart from a real
// scan failure.
var ErrNoLightsForColorModel = errors.New("no light frames match the requested color model")

// ParseColorChoice validates a wire value against the closed enum. The empty string is auto, so every
// job stored before the knob existed — and every client that never sends it — keeps today's routing.
func ParseColorChoice(s string) (ColorChoice, error) {
	switch ColorChoice(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return ChoiceAuto, nil
	case ChoiceAuto:
		return ChoiceAuto, nil
	case ChoiceMono:
		return ChoiceMono, nil
	case ChoiceOSC:
		return ChoiceOSC, nil
	default:
		return "", fmt.Errorf("unknown color_model %q (want: auto, mono, osc)", s)
	}
}

// ResolveColorModel applies the user's colour choice to a scanned inventory.
//
// For ChoiceAuto it is a deliberate NO-OP. Each mode entry still owns its own reading of the scan
// verdict and those readings genuinely differ — deepsky drops the colour lights of a mixed folder and
// warns, mosaic does the same for anything that is not pure OSC, comet keeps every light, and the
// per-stage rerun drops silently to mirror the run it is replaying. Resolving auto here would change
// all four at once.
//
// For an explicit choice it drops the contradicting LIGHTS, rewrites the verdict to what the user
// asserted, and reports what it removed. Because the verdict is then never Mixed, each mode entry's
// existing branch sees a pure mono or pure colour scan and routes it exactly as it always has —
// the knob needs no per-mode special case downstream.
//
// Only lights are filtered. Calibration frames carry no colour identity in the matcher (it keys on
// gain/offset/bin, exposure and temperature, guarded by KeepMatchingDims' sensor-dimension check) and
// clearSpuriousBayer actively strips BAYERPAT from them whenever the scan shows filter-wheel evidence.
// Dropping "every mono frame" would therefore delete a colour rig's own darks — silently, and exactly
// in the mixed folder where the user reached for the knob in the first place.
func ResolveColorModel(inv *Inventory, c ColorChoice) error {
	if inv == nil || c == ChoiceAuto || c == "" {
		return nil
	}

	want := ColorOSC
	if c == ChoiceMono {
		want = ColorMono
	}
	hadLights := countLights(inv.Frames) > 0

	var removed int
	if want == ColorMono {
		// Exactly what every mode entry already does to a mixed folder, so an explicit "mono" and an
		// auto-resolved mixed scan produce ONE behaviour rather than two that can drift apart. Dropping
		// colour calibration frames along with the lights is safe: a frame that is still genuinely CFA
		// after the spurious-BAYERPAT veto belongs to the colour rig, not to this mono stack.
		removed = inv.ExcludeColor()
	} else {
		// The reverse is NOT a mirror, and the asymmetry is in the data rather than in the design.
		// clearSpuriousBayer deliberately strips BAYERPAT from calibration frames whenever the scan
		// shows filter-wheel evidence, and plenty of OSC capture software never writes the card on
		// darks at all — so "drop every monochrome frame" would delete this colour rig's own darks,
		// silently, in exactly the mixed folder where the user reached for the knob. Lights only.
		removed = inv.excludeFrames(func(fr *Frame) bool { return fr.Type == Light && !fr.IsColor() })
	}
	if hadLights && countLights(inv.Frames) == 0 {
		return fmt.Errorf("%w: this run was submitted as a %s stack but the folder holds %s (%d light frame(s) dropped)",
			ErrNoLightsForColorModel, colorChoiceLabel(c), noneFoundLabel(c), removed)
	}

	inv.ColorModel = want
	renamed := nameColorLights(inv)
	if removed > 0 || renamed > 0 {
		inv.Sets = buildSets(inv.Frames) // sets key on Filter, so re-key after either change
	}
	if removed > 0 {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf(
			"%d %s light frame(s) excluded — this run was submitted as a %s", removed, droppedLabel(c), colorChoiceLabel(c)))
	}
	return nil
}

// nameColorLights gives the surviving colour lights the canonical channel name and returns how many
// it renamed. A mixed folder promoted to colour needs it: nameColorChannel runs at scan time and only
// for an already-OSC verdict, so without this the frames reach the channel machinery with an empty
// Filter, which reads as "no filter" everywhere downstream.
//
// It REPLACES each frame with a renamed copy instead of assigning through the pointer. Frames are
// shared read-only with the ScanCache (see cache.go — "a filter override would mutate them in
// place"), and this runs at run time, long after the scan that cached them: an in-place rename would
// leak into a later, unrelated inspection of the same folder. Only this Inventory's own slice is
// written, which is the same invariant excludeFrames keeps.
func nameColorLights(inv *Inventory) int {
	renamed := 0
	for i, fr := range inv.Frames {
		if fr.Type != Light || !fr.IsColor() || fr.Filter != "" {
			continue
		}
		clone := *fr
		clone.Filter = filters.Color
		inv.Frames[i] = &clone
		renamed++
	}
	return renamed
}

// colorChoiceLabel names the choice the way the warning and error text reads it.
func colorChoiceLabel(c ColorChoice) string {
	if c == ChoiceMono {
		return "monochrome stack"
	}
	return "colour stack"
}

// droppedLabel names the frames an explicit choice removes.
func droppedLabel(c ColorChoice) string {
	if c == ChoiceMono {
		return "one-shot-color"
	}
	return "monochrome"
}

// noneFoundLabel names what the folder turned out to hold instead.
func noneFoundLabel(c ColorChoice) string {
	if c == ChoiceMono {
		return "no monochrome light frames"
	}
	return "no one-shot-color light frames"
}

// countLights counts the light frames — the only frames a colour choice filters.
func countLights(frames []*Frame) int {
	n := 0
	for _, fr := range frames {
		if fr.Type == Light {
			n++
		}
	}
	return n
}
