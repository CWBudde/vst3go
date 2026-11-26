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
	t.Run("single class", func(t *testing.T) { testPluginLoadsAndAnswersHost(t, false) })
	t.Run("multiple classes", func(t *testing.T) { testPluginLoadsAndAnswersHost(t, true) })
}

func testPluginLoadsAndAnswersHost(t *testing.T, multiClass bool) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skipf("smoke test not wired up for %s", runtime.GOOS)
	}

	cc := findCC(t)

	root := repoRoot(t)
	tmp := t.TempDir()

	libName := "vst3go_smoke.so"
	driverName := "driver"
	switch runtime.GOOS {
	case "darwin":
		libName = "vst3go_smoke.dylib"
	case "windows":
		libName = "vst3go_smoke.dll"
		driverName = "driver.exe"
	}
	lib := filepath.Join(tmp, libName)

	// -buildvcs=false: the module may be built from a dirty or detached tree.
	buildArgs := []string{
		"build",
		"-buildvcs=false",
		"-buildmode=c-shared",
		"-o", lib,
	}
	ldflags := "-X main.runtimeSmoke=1"
	if multiClass {
		ldflags += " -X main.multiClass=1"
	}
	run(t, root, nil, "go", append(buildArgs, "-ldflags="+ldflags, "./test/smoke/plugin")...)

	driver := filepath.Join(tmp, driverName)
	args := []string{
		"-o", driver,
		filepath.Join(root, "test", "smoke", "testdata", "driver.c"),
		lib,
		"-I" + filepath.Join(root, "include"),
	}
	if multiClass {
		args = append(args, "-DMULTI_CLASS")
	}
	if runtime.GOOS != "windows" {
		// PE has no rpath; Windows resolves the DLL from the executable's own
		// directory, which is where both of these land.
		args = append(args, "-Wl,-rpath,"+tmp)
	}
	run(t, root, nil, cc, args...)

	out := run(t, tmp, []string{"LD_LIBRARY_PATH=" + tmp, "DYLD_LIBRARY_PATH=" + tmp}, driver)
	t.Log("\n" + out)

	if !strings.Contains(out, "ALL PASSED") {
		t.Fatalf("driver reported failures:\n%s", out)
	}
}

// findCC locates a C compiler. Windows toolchains generally ship gcc without
// the "cc" alias that is conventional on Unix.
func findCC(t *testing.T) string {
	t.Helper()

	for _, name := range []string{"cc", "gcc", "clang"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	t.Skip("no C compiler available")
	return ""
}
