// Package starnet drives a host-installed StarNet (https://www.starnetastro.com) headlessly to
// remove stars from a stretched image, producing a "starless" frame used for star-reduced
// finishing and the star-tier deliverables (see internal/pipeline finishWithGimp / emitStarTiers).
//
// StarNet is an optional host tool, invoked like Siril/GIMP: we shell out to the user's own
// install (set via STARNET_BIN) and stream its stdout. It is never vendored. When absent, the
// finish keeps full stars.
//
// Two CLI generations are in the wild and they take incompatible arguments — see Variant.
package starnet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/verove-jordan/astronomy/internal/sysmon"
)

// Variant selects how the binary's arguments are rendered. The two generations are mutually
// incompatible: StarNet++ v2 takes bare positional arguments, while StarNet2 (v2.5+) uses a
// flag-style CLI and *rejects* positional ones, so guessing wrong fails every run.
type Variant string

const (
	// VariantAuto probes the binary once (see resolveVariant) and falls back to positional.
	VariantAuto Variant = "auto"
	// VariantPositional is StarNet++ v2: `starnet++ in.tif out.tif [stride]`.
	VariantPositional Variant = "positional"
	// VariantFlags is StarNet2 v2.5+: `starnet2 -i in.tif -o out.tif [-s stride]`.
	VariantFlags Variant = "flags"
)

// machineInfoFlag asks a StarNet2 CLI to describe itself as JSON and exit; machineInfoSchema is the
// schema id that JSON carries. Older StarNet++ builds reject the flag, which is the signal we want.
const (
	machineInfoFlag   = "--machine-info"
	machineInfoSchema = "starnetastro.cli.machine-info"
)

// DefaultBinCandidates are the known StarNet executable names, newest generation first. Config
// resolves an unset STARNET_BIN to the first of these found on PATH, so an install of either
// generation is picked up without configuration.
var DefaultBinCandidates = []string{"starnet2", "starnet++"}

// Runner executes StarNet via its command-line interface.
type Runner struct {
	bin     string
	variant Variant

	mu     sync.Mutex // guards probed
	probed Variant    // memoized result of the auto-probe ("" until it has concluded)
}

// New returns a Runner for the given StarNet binary path, auto-detecting its CLI generation on
// first use. An empty path yields a Runner that reports Unavailable, so "not configured" and
// "not installed" are handled identically.
func New(bin string) *Runner { return NewVariant(bin, VariantAuto) }

// NewVariant returns a Runner pinned to a CLI generation. Anything other than a known variant
// (including "" and "auto") means auto-detect.
func NewVariant(bin string, v Variant) *Runner { return &Runner{bin: bin, variant: v} }

// resolveVariant returns the CLI generation to render arguments for: the configured one when it is
// explicit, otherwise the memoized probe. A probe that could not conclude (binary absent, context
// cancelled) yields positional WITHOUT being memoized, so a later run can still detect properly.
func (r *Runner) resolveVariant(ctx context.Context) Variant {
	if r.variant == VariantPositional || r.variant == VariantFlags {
		return r.variant
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.probed != "" {
		return r.probed
	}
	out, err := exec.CommandContext(ctx, r.bin, machineInfoFlag).Output()
	v := variantFromMachineInfo(out, err)
	if err == nil {
		r.probed = v
	}
	return v
}

// variantFromMachineInfo reads the `--machine-info` probe: a StarNet2 CLI answers with its JSON
// product description (possibly after unrelated preamble lines), anything else is the older
// positional StarNet++. Pure, so the decision table is unit-testable without a binary.
func variantFromMachineInfo(out []byte, err error) Variant {
	if err != nil {
		return VariantPositional
	}
	start := bytes.IndexByte(out, '{')
	if start < 0 {
		return VariantPositional
	}
	var info struct {
		Schema string `json:"schema"`
	}
	if json.Unmarshal(out[start:], &info) != nil {
		return VariantPositional
	}
	if strings.HasPrefix(info.Schema, machineInfoSchema) {
		return VariantFlags
	}
	return VariantPositional
}

// Progress is one line of StarNet++ output with any embedded percentage extracted. When Sample is
// non-nil the Progress carries a live resource reading instead of a log line (Line is empty).
type Progress struct {
	Line    string
	Percent int // -1 when the line carried no percentage
	Sample  *sysmon.Sample
}

// Options tune star removal.
type Options struct {
	Stride int // tile stride in px; <=0 → StarNet++ default (256)
}

var percentRe = regexp.MustCompile(`(\d+)\s?%`)

// Available reports whether the StarNet++ binary can be found and executed. Soft check: callers log
// the error and keep full stars rather than aborting the run.
func (r *Runner) Available(_ context.Context) error {
	if r == nil || r.bin == "" {
		return fmt.Errorf("starnet binary path is empty (set STARNET_BIN)")
	}
	if _, err := exec.LookPath(r.bin); err != nil {
		return fmt.Errorf("starnet binary %q not found: %w", r.bin, err)
	}
	return nil
}

// RemoveStars runs StarNet++ on inTIFF (a 16-bit TIFF), writing the starless image to outTIFF.
// Progress lines are streamed to onProgress (may be nil).
func (r *Runner) RemoveStars(ctx context.Context, inTIFF, outTIFF string, opts Options, onProgress func(Progress)) error {
	if err := r.Available(ctx); err != nil {
		return err
	}
	return r.run(ctx, removeArgs(r.resolveVariant(ctx), inTIFF, outTIFF, opts), onProgress)
}

// removeArgs builds the StarNet CLI args for one generation. Kept pure and central so both forms
// (which vary across StarNet builds — verify with the binary's usage) are easy to adjust and
// unit-test; an unknown variant renders the positional form.
func removeArgs(v Variant, inTIFF, outTIFF string, opts Options) []string {
	if v == VariantFlags {
		args := []string{"-i", inTIFF, "-o", outTIFF}
		if opts.Stride > 0 {
			args = append(args, "-s", strconv.Itoa(opts.Stride))
		}
		return args
	}
	args := []string{inTIFF, outTIFF}
	if opts.Stride > 0 {
		args = append(args, strconv.Itoa(opts.Stride))
	}
	return args
}

// run executes StarNet++ with the given args, streaming each output line to onProgress and folding
// stderr into the same stream. A non-zero exit returns an error with the captured log attached.
func (r *Runner) run(ctx context.Context, args []string, onProgress func(Progress)) error {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start starnet: %w", err)
	}

	// emit serializes callbacks so the monitor goroutine's samples can't race the scan loop's lines.
	var emitMu sync.Mutex
	emit := func(p Progress) {
		if onProgress == nil {
			return
		}
		emitMu.Lock()
		defer emitMu.Unlock()
		onProgress(p)
	}

	mon := sysmon.Start(ctx, cmd.Process.Pid, 0, func(s sysmon.Sample) {
		emit(Progress{Sample: &s})
	})
	defer mon.Stop()

	var log strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		log.WriteString(line)
		log.WriteByte('\n')
		emit(Progress{Line: line, Percent: parsePercent(line)})
	}

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("starnet failed (exit %d): %w\n%s", exitErr.ExitCode(), err, log.String())
		}
		return fmt.Errorf("starnet: %w", err)
	}
	return nil
}

func parsePercent(line string) int {
	if m := percentRe.FindStringSubmatch(line); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n > 100 {
			return -1
		}
		return n
	}
	return -1
}
