//go:build smoke

// Package smoke builds the runtime into an actual loadable VST3 module and
// drives it from C the way a host would.
//
// Nothing else in the repository links pkg/plugin into a shared library, so
// without this the entire cgo wrapper surface — the factory, the COM refcounts,
// queryInterface dispatch and the audio callback — is only ever exercised by
// the compiler. It is behind the "smoke" build tag because it needs a C
// toolchain and takes a few seconds:
//
//	go test -tags smoke ./test/smoke/...
package smoke

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPluginLoadsAndAnswersHost builds the minimal plugin as a C shared library
// and runs the C driver against it. The driver's assertions cover the defects
// that used to make the plugin unusable in a real DAW: a factory that could not
// be re-acquired after release, createInstance ignoring the requested IID,
// getControllerClassId claiming a controller class that does not exist, and
// process() panicking as soon as a host presented more than one input bus.
func TestPluginLoadsAndAnswersHost(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin":
	default:
		t.Skipf("smoke test not wired up for %s", runtime.GOOS)
	}

	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no C compiler available")
	}

	root := repoRoot(t)
	tmp := t.TempDir()

	libName := "vst3go_smoke.so"
	if runtime.GOOS == "darwin" {
		libName = "vst3go_smoke.dylib"
	}
	lib := filepath.Join(tmp, libName)

	// -buildvcs=false: the module may be built from a dirty or detached tree.
	run(t, root, nil, "go", "build",
		"-buildvcs=false",
		"-buildmode=c-shared",
		"-o", lib,
		"./test/smoke/plugin",
	)

	driver := filepath.Join(tmp, "driver")
	run(t, root, nil, "cc",
		"-o", driver,
		filepath.Join(root, "test", "smoke", "testdata", "driver.c"),
		lib,
		"-I"+filepath.Join(root, "include"),
		"-Wl,-rpath,"+tmp,
	)

	out := run(t, tmp, []string{"LD_LIBRARY_PATH=" + tmp, "DYLD_LIBRARY_PATH=" + tmp}, driver)
	t.Log("\n" + out)

	if !strings.Contains(out, "ALL PASSED") {
		t.Fatalf("driver reported failures:\n%s", out)
	}
}
