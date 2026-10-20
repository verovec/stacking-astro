package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/verove-jordan/astronomy/internal/mode"
	"github.com/verove-jordan/astronomy/internal/pipeline"
	"github.com/verove-jordan/astronomy/internal/siril"
)

// oracleCase is one recording: synthetic frames generated into <root>/in, or real capture folders.
type oracleCase struct {
	key, name, title string
	synth            func(in string) error
	inputDirs        []string
	// goldenDir, when set, holds the legacy repo's pinned steps.txt + run.json this case must match.
	goldenDir string
}

func allCases(captures string) []oracleCase {
	sub := func(capture string, dirs ...string) []string {
		var out []string
		for _, d := range dirs {
			out = append(out, filepath.Join(captures, capture, d))
		}
		return out
	}
	calib := []string{"lights", "darks", "flats", "bias"}
	return []oracleCase{
		{key: "a", name: "golden-single-night", title: "synthetic one-night L/R/G/B",
			synth: buildGoldenCapture, goldenDir: "internal/pipeline/testdata/golden_single_night"},
		{key: "b", name: "golden-two-night", title: "synthetic two nights, second rotated 35°",
			synth: func(in string) error { return buildTwoNightCapture(in, 35) }},
		{key: "c", name: "m31-db", title: "M31-DB, one real dual-band night",
			inputDirs: sub("M31-DB", calib...)},
		{key: "d", name: "m31-bb-db", title: "M31-BB + M31-DB, real multi-night",
			inputDirs: append(sub("M31-DB", calib...), sub("M31-BB", calib...)...)},
	}
}

type recordConfig struct {
	root, sirilBin, capturesRoot string
	keepWork                     bool
}

// recordCase runs the engine once on the case and writes <root>/record/.
func recordCase(ctx context.Context, c oracleCase, cfg recordConfig) error {
	in, work, out := filepath.Join(cfg.root, "in"), filepath.Join(cfg.root, "work"), filepath.Join(cfg.root, "out")
	shimDir, rec := filepath.Join(cfg.root, "shim"), filepath.Join(cfg.root, "record")
	for _, d := range []string{work, out, shimDir, rec} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	opts := pipeline.Options{WorkDir: work, OutputDir: out, Preset: goldenPreset()}
	if c.synth != nil {
		if err := os.RemoveAll(in); err != nil {
			return err
		}
		if err := c.synth(in); err != nil {
			return fmt.Errorf("generate frames: %w", err)
		}
		opts.InputDir = in
	} else {
		opts.InputDir, opts.InputDirs = c.inputDirs[0], c.inputDirs
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	os.Setenv(envShimDir, shimDir)
	os.Setenv(envRealBin, cfg.sirilBin)
	opts.Runner = siril.New(self, siril.Limits{})
	steps := &stepLog{seen: map[string]bool{}}
	opts.OnProgress = steps.add

	res, err := pipeline.Process(ctx, opts)
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	norm := newNormalizer(res.RunID, cfg.root, cfg.capturesRoot, c.synth == nil)
	if err := writeRecord(rec, shimDir, res, steps, norm); err != nil {
		return err
	}
	if err := writeManifest(rec, c, cfg, goldenStatus(c, rec)); err != nil {
		return err
	}
	if !cfg.keepWork {
		if err := os.RemoveAll(work); err != nil {
			return err
		}
	}
	return os.RemoveAll(shimDir)
}

// stepLog keeps each distinct "index/total step" once, in first-seen order, like the golden test.
type stepLog struct {
	lines []string
	seen  map[string]bool
}

func (s *stepLog) add(p pipeline.Progress) {
	line := fmt.Sprintf("%d/%d %s", p.Index, p.Total, p.Step)
	if !s.seen[line] {
		s.seen[line] = true
		s.lines = append(s.lines, line)
	}
}

// goldenPreset mirrors internal/pipeline/golden_live_test.go goldenPreset: every external-tool and
// model-driven step off, Go denoise and trail mask off, so masters are Siril's bytes. The real
// cases use it too, which makes them oracles of the Siril path, not of the full deepsky preset.
func goldenPreset() *mode.Preset {
	p := mode.For(mode.Deepsky)
	p.BackgroundAI = false
	p.CombinedBackgroundAI = false
	p.ColorDenoiseAI = false
	p.ColorCalibration = false
	p.Previews = false
	p.AutoFixStars = false
	p.Supervise = false
	p.StarReduce = 0
	p.TrailMaskK = 0
	p.DenoiseChroma, p.DenoiseLum = 0, 0
	return &p
}

// goldenStatus compares case (a) with the legacy repo's pinned golden (the normalized run.json and
// first-seen steps the live golden test asserts) and says which files differ. It informs, it does
// not fail: on 2026-10-20 the pin predates host Siril 1.4.4 and the golden test itself fails, while
// re-pinning it here yields files byte-equal to this recording.
func goldenStatus(c oracleCase, rec string) string {
	if c.goldenDir == "" {
		return ""
	}
	var differ []string
	for _, name := range []string{"run.json", "steps.txt"} {
		want, err := os.ReadFile(filepath.Join(c.goldenDir, name))
		if err != nil {
			return fmt.Sprintf("golden unreadable (%v); run from the stacking-astro repo root", err)
		}
		got, err := os.ReadFile(filepath.Join(rec, name))
		if err != nil || !bytes.Equal(want, got) {
			differ = append(differ, name)
		}
	}
	status := "matches " + c.goldenDir
	if len(differ) > 0 {
		status = "differs from " + c.goldenDir + ": " + strings.Join(differ, ", ")
	}
	fmt.Printf("oraclerec: case %s %s\n", c.key, status)
	return status
}
