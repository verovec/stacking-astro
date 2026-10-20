package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/verove-jordan/astronomy/internal/pipeline"
)

// volatileKeys are dropped from every recorded JSON, exactly as the live golden drops them.
var volatileKeys = map[string]bool{
	"timings": true, "engine": true, "duration_ms": true, "elapsed_ms": true, "seconds": true,
	"created_at": true, "date_obs_ms": true,
}

// normalizer replaces machine- and run-specific substrings with placeholders, in order.
type normalizer struct{ pairs [][2]string }

func newNormalizer(runID, root, captures string, real bool) normalizer {
	var n normalizer
	if real {
		n.pairs = append(n.pairs, [2]string{captures, "<CAPTURES>"})
	}
	n.pairs = append(n.pairs, [2]string{root, "<ROOT>"}, [2]string{runID, "<RUN>"})
	return n
}

func (n normalizer) str(s string) string {
	for _, p := range n.pairs {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// value walks a decoded JSON value: volatile keys dropped, every string normalized.
func (n normalizer) value(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if !volatileKeys[k] {
				out[k] = n.value(val)
			}
		}
		return out
	case []any:
		for i := range x {
			x[i] = n.value(x[i])
		}
		return x
	case string:
		return n.str(x)
	default:
		return v
	}
}

func (n normalizer) writeJSON(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return err
	}
	out, err := json.MarshalIndent(n.value(generic), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// writeRecord writes every pixel-free file of the case under rec.
func writeRecord(rec, shimDir string, res *pipeline.Result, steps *stepLog, n normalizer) error {
	if err := os.MkdirAll(filepath.Join(rec, "scripts"), 0o755); err != nil {
		return err
	}
	scripts, err := copyScripts(rec, shimDir, n)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(rec, "steps.txt"), []byte(strings.Join(steps.lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	files := map[string]any{
		"run.json":          res,
		"inventory.json":    res.Inventory,
		"calibration.json":  calibrationOf(res),
		"grades.json":       gradesOf(res),
		"selection.json":    selectionOf(res, scripts),
		"registration.json": registrationOf(res, scripts),
	}
	for name, v := range files {
		if err := n.writeJSON(filepath.Join(rec, name), v); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := writeHashes(filepath.Join(rec, "masters.sha256"), masterPaths(res), n); err != nil {
		return err
	}
	return writeHashes(filepath.Join(rec, "bundle_layers.sha256"), layerPaths(res.OutputDir), n)
}

// recordedScript is one captured Siril script: its normalized file name and command lines.
type recordedScript struct {
	name  string
	lines []string
}

func copyScripts(rec, shimDir string, n normalizer) ([]recordedScript, error) {
	names, err := shimScriptNames(shimDir)
	if err != nil {
		return nil, fmt.Errorf("no captured scripts (did the shim run?): %w", err)
	}
	var scripts []recordedScript
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(shimDir, shimScripts, name))
		if err != nil {
			return nil, err
		}
		text, target := n.str(string(body)), n.str(name)
		if err := os.WriteFile(filepath.Join(rec, "scripts", target), []byte(text), 0o644); err != nil {
			return nil, err
		}
		scripts = append(scripts, recordedScript{name: target, lines: strings.Split(text, "\n")})
	}
	return scripts, nil
}

func masterPaths(res *pipeline.Result) []string {
	var paths []string
	for _, m := range res.Masters {
		paths = append(paths, m.Path)
	}
	for _, ch := range res.Channels {
		if ch.OutputPath != "" {
			paths = append(paths, ch.OutputPath)
		}
	}
	return paths
}

// layerPaths lists the bundle-relevant layers of a run's output dir (precedence combine_ >
// aligned_ > master_ > syn_, plus rgb_base and the linear/ steps); masters are in masters.sha256.
func layerPaths(outDir string) []string {
	var paths []string
	for _, pattern := range []string{"rgb_base*.fits", "syn_*.fits", "aligned_*.fits",
		"combine_*.fits", "trim_*.fits", "linear/*.fits"} {
		found, _ := filepath.Glob(filepath.Join(outDir, pattern))
		paths = append(paths, found...)
	}
	return paths
}

// writeHashes writes "<sha256 of the FITS data unit>  <normalized path>" lines, sorted by path.
func writeHashes(path string, files []string, n normalizer) error {
	seen := map[string]bool{}
	var lines []string
	for _, f := range files {
		if seen[f] {
			continue
		}
		seen[f] = true
		sum, err := fitsDataSHA256(f)
		if err != nil {
			return fmt.Errorf("hash %s: %w", f, err)
		}
		lines = append(lines, sum+"  "+n.str(f))
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// writeManifest records how the case was produced and the hash of every recorded file.
func writeManifest(rec string, c oracleCase, cfg recordConfig, golden string) error {
	version, _ := exec.Command(cfg.sirilBin, "--version").Output()
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	entries, err := os.ReadDir(rec)
	if err != nil {
		return err
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(rec, e.Name()))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		files[e.Name()] = hex.EncodeToString(sum[:])
	}
	manifest := map[string]any{
		"case": c.name, "title": c.title, "timezone": recordTZ,
		"siril": strings.TrimSpace(string(version)), "legacy_commit": strings.TrimSpace(string(commit)),
		"go": runtime.Version(), "files": files, "legacy_golden": golden,
		"preset":  "deepsky golden preset: AI, external tools, colour calibration, previews, supervisor off; denoise 0; trail mask 0",
		"hashes":  "masters.sha256 and bundle_layers.sha256 hash the FITS data unit only: Siril stamps DATE in every header",
		"scripts": "scripts/NNNN_<siril -d dir>.ssf in emission order; paths normalized to <ROOT>/<CAPTURES>, run id to <RUN>",
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(rec, "manifest.json"), append(raw, '\n'), 0o644)
}
