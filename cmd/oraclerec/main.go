// Command oraclerec records the legacy engine's observable decisions as oracles for the Swift port
// (kappa-sigma card 0044): it runs pipeline.Process with the deterministic golden preset on four
// cases and writes, per case, the inventory, the calibration actually used, every Siril script in
// emission order, grades, registration and selection, data-unit hashes of the masters and bundle
// layers, and the normalized run.json. One-shot tooling: it never changes engine code.
//
//	go run ./cmd/oraclerec --case all              # a, b, c, d
//	go run ./cmd/oraclerec --case a --out /tmp/x   # one case elsewhere
//
// Output: <out>/<case>/{in,out}/ (frames and outputs, pixels — never committed) and
// <out>/<case>/record/ (text only; the kappa-sigma repo commits record/ of the synthetic cases).
//
// Siril scripts are captured without touching the engine: the recorder hands siril.New its own
// executable, which, run with ORACLEREC_SHIM_DIR set, behaves as Siril (see shim.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// recordTZ pins the night keys (inspect.NightKey uses time.Local) so recordings do not depend on
// the recording machine's timezone.
const recordTZ = "Europe/Paris"

func main() {
	if os.Getenv(envShimDir) != "" {
		os.Exit(runShim(os.Args[1:]))
	}
	home, _ := os.UserHomeDir()
	which := flag.String("case", "all", "case to record: a, b, c, d or all")
	out := flag.String("out", filepath.Join(home, "Documents/astro/fixtures/oracle"), "fixture root")
	captures := flag.String("captures", filepath.Join(home, "Documents/astro/captures"), "captures root")
	siril := flag.String("siril", "/Applications/Siril.app/Contents/MacOS/siril-cli", "real siril-cli")
	keepWork := flag.Bool("keep-work", false, "keep the engine scratch dir (tens of GB on real cases)")
	flag.Parse()

	loc, err := time.LoadLocation(recordTZ)
	if err != nil {
		fail(err)
	}
	time.Local = loc

	cases, err := selectCases(*which, *captures)
	if err != nil {
		fail(err)
	}
	for _, c := range cases {
		start := time.Now()
		fmt.Printf("oraclerec: case %s (%s)\n", c.name, c.title)
		cfg := recordConfig{root: filepath.Join(*out, c.name), sirilBin: *siril,
			capturesRoot: *captures, keepWork: *keepWork}
		if err := recordCase(context.Background(), c, cfg); err != nil {
			fail(fmt.Errorf("case %s: %w", c.name, err))
		}
		fmt.Printf("oraclerec: case %s done in %s\n", c.name, time.Since(start).Round(time.Second))
	}
}

func selectCases(which, captures string) ([]oracleCase, error) {
	all := allCases(captures)
	if which == "all" {
		return all, nil
	}
	var picked []oracleCase
	for _, key := range strings.Split(which, ",") {
		found := false
		for _, c := range all {
			if c.key == key || c.name == key {
				picked = append(picked, c)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown case %q (a, b, c, d, all)", key)
		}
	}
	return picked, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "oraclerec:", err)
	os.Exit(1)
}
