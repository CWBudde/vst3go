//go:build smoke

package smoke

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestMIDIAndAutomationFromHost(t *testing.T) {
	libName, driverName := "vst3go_midi.so", "midi_host"
	switch runtime.GOOS {
	case "linux":
	case "darwin":
		libName = "vst3go_midi.dylib"
	case "windows":
		libName, driverName = "vst3go_midi.dll", "midi_host.exe"
	default:
		t.Skipf("MIDI smoke test not wired up for %s", runtime.GOOS)
	}
	cc := findCC(t)
	root, tmp := repoRoot(t), t.TempDir()
	lib, driver := filepath.Join(tmp, libName), filepath.Join(tmp, driverName)
	run(t, root, nil, "go", "build", "-buildvcs=false", "-buildmode=c-shared", "-o", lib, "./test/smoke/midifixture")
	args := []string{
		"-Wall", "-Wextra", "-Werror", "-o", driver,
		filepath.Join(root, "test", "smoke", "testdata", "midi.c"),
		lib, "-I" + filepath.Join(root, "include"),
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-Wl,-rpath,"+tmp)
	}
	run(t, root, nil, cc, args...)
	t.Log(run(t, tmp, []string{"LD_LIBRARY_PATH=" + tmp, "DYLD_LIBRARY_PATH=" + tmp}, driver))
}
