// Package starnet drives a host-installed StarNet (https://www.starnetastro.com) headlessly to
// remove stars from a stretched image, producing a "starless" frame used for star-reduced
// finishing and the star-tier deliverables (see internal/pipeline finishWithGimp / emitStarTiers).
//
// StarNet is an optional host tool, invoked like Siril/GIMP: we shell out to the user's own
// install (set via STARNET_BIN) and stream its stdout. It is never vendored. When absent, the
// finish keeps full stars.
//
// A CONTAINERIZED engine cannot exec it at all: StarNet publishes Linux x64, Windows x64 and both
// macOS builds, but no linux/arm64 one, so on an Apple-Silicon host the engine image (linux/arm64)
// has no runnable StarNet — and a macOS binary bind-mounted into it is a Mach-O that Linux will
// never execute. For that case the runner OFFLOADS to a native host HTTP service
// (cmd/starnet-host, ASTRO_STARNET_URL); see offload for when each transport is chosen.
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
	"net/http"
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

// Runner executes StarNet, either by exec'ing the host-installed binary or — when there is no runnable
// local one and a host-service URL is set — by offloading each pass to a native StarNet HTTP service
// (cmd/starnet-host) over that URL. Both modes share this type, so callers never branch.
type Runner struct {
	bin     string
	variant Variant
	url     string // optional host StarNet service base URL; used only when bin does not resolve
	hc      *http.Client

	mu     sync.Mutex // guards probed
	probed Variant    // memoized result of the auto-probe ("" until it has concluded)
}

// New returns a Runner for the given StarNet binary path and optional host-service URL, auto-detecting
// the CLI generation on first use. An empty bin AND empty url yields a Runner that reports Unavailable,
// so "not configured" and "not installed" are handled identically.
func New(bin, url string) *Runner { return NewVariant(bin, VariantAuto, url) }

// NewVariant returns a Runner pinned to a CLI generation. Anything other than a known variant
// (including "" and "auto") means auto-detect. url is the optional host-service base URL.
func NewVariant(bin string, v Variant, url string) *Runner {
	return &Runner{bin: bin, variant: v, url: strings.TrimRight(url, "/"), hc: &http.Client{}}
}

// offload reports whether this runner talks to the host service instead of exec'ing a binary. A usable
// LOCAL binary always wins: unlike the GraXpert offload — which exists to reach the host GPU, and so
// lets an explicit URL beat a working local install — this one exists only because some platforms have
// no runnable StarNet build at all. Where one does run it is strictly the better path (no HTTP, no
// second process), which also keeps a Linux x64 server that bind-mounts its own StarNet on the local
// path even when the compose default hands it a URL.
func (r *Runner) offload() bool {
	if r == nil || r.url == "" {
		return false
	}
	return !r.localUsable()
}

// Endpoint returns the host-service URL when this runner offloads, and "" when it exec's a local
// binary. Status surfaces use it to name what StarNet will ACTUALLY be for the next run, rather than
// printing a binary path the engine is never going to reach.
func (r *Runner) Endpoint() string {
	if r.offload() {
		return r.url
	}
	return ""
}

// localUsable reports whether the configured binary resolves to something executable.
func (r *Runner) localUsable() bool {
	if r == nil || r.bin == "" {
		return false
	}
	_, err := exec.LookPath(r.bin)
	return err == nil
}

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

// Available reports whether StarNet can run: the binary can be found and executed, or — in offload
// mode — the host service answers. Soft check: callers log the error and keep full stars rather than
// aborting the run.
func (r *Runner) Available(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("starnet runner is nil")
	}
	if r.offload() {
		return r.remotePing(ctx) // the service being reachable IS the availability answer
	}
	if r.bin == "" {
		return fmt.Errorf("starnet binary path is empty (set STARNET_BIN, or ASTRO_STARNET_URL " +
			"to offload to a host service)")
	}
	if _, err := exec.LookPath(r.bin); err != nil {
		return fmt.Errorf("starnet binary %q not found: %w", r.bin, err)
	}
	return nil
}

// RemoveStars runs StarNet++ on inTIFF (a 16-bit TIFF), writing the starless image to outTIFF.
// Progress lines are streamed to onProgress (may be nil).
func (r *Runner) RemoveStars(ctx context.Context, inTIFF, outTIFF string, opts Options, onProgress func(Progress)) error {
	if r.offload() {
		// The host service owns its binary, so it resolves the CLI generation too — nothing local to probe.
		return r.runRemote(ctx, RemoteRequest{In: inTIFF, Out: outTIFF, Stride: opts.Stride}, onProgress)
	}
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
