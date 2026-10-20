package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The shim: siril.Runner runs `<bin> -d <dir> -s <dir>/_astrostack.ssf` and overwrites that script
// on every call, and the engine deletes most per-channel dirs afterwards. Handing the runner this
// executable as <bin> captures each script, in emission order, before the real Siril runs it.
const (
	envShimDir  = "ORACLEREC_SHIM_DIR"
	envRealBin  = "ORACLEREC_REAL_SIRIL"
	shimScripts = "scripts"
)

// runShim copies the script, then replaces itself with the real Siril (same arguments, same
// PID): output and exit code are Siril's, and the runner's cancellation and CPU/RAM sampling
// reach Siril itself rather than an intermediate process. It returns only on failure.
func runShim(args []string) int {
	dir, script := argAfter(args, "-d"), argAfter(args, "-s")
	if script != "" {
		if err := captureScript(os.Getenv(envShimDir), dir, script); err != nil {
			fmt.Fprintln(os.Stderr, "oraclerec shim:", err)
			return 97
		}
	}
	bin := os.Getenv(envRealBin)
	err := syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
	fmt.Fprintln(os.Stderr, "oraclerec shim: exec", bin+":", err)
	return 98
}

// captureScript stores the script as scripts/NNNN_<base of -d>.ssf, where NNNN is the run-wide
// emission number: it is reserved with an O_EXCL marker under seq/ (independent of the folder
// name), so numbering stays global and safe even if Siril calls ever overlapped.
func captureScript(shimDir, workDir, script string) error {
	body, err := os.ReadFile(script)
	if err != nil {
		return err
	}
	target, markers := filepath.Join(shimDir, shimScripts), filepath.Join(shimDir, "seq")
	for _, d := range []string{target, markers} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	for n := 1; n < 100000; n++ {
		marker, err := os.OpenFile(filepath.Join(markers, fmt.Sprintf("%04d", n)), os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := marker.Close(); err != nil {
			return err
		}
		name := fmt.Sprintf("%04d_%s.ssf", n, filepath.Base(workDir))
		return os.WriteFile(filepath.Join(target, name), body, 0o644)
	}
	return fmt.Errorf("too many scripts under %s", target)
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// shimScriptNames lists the captured scripts in emission order.
func shimScriptNames(shimDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(shimDir, shimScripts))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ssf") {
			names = append(names, e.Name())
		}
	}
	return names, nil // ReadDir sorts by name; NNNN_ is zero-padded
}
