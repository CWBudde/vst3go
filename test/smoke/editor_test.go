//go:build smoke

package smoke

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativeEditorHostCallbacks(t *testing.T) {
	libName, driverName := "vst3go_editor.so", "editor_host"
	switch runtime.GOOS {
	case "linux":
	case "darwin":
		libName = "vst3go_editor.dylib"
	case "windows":
		libName, driverName = "vst3go_editor.dll", "editor_host.exe"
	default:
		t.Skipf("editor smoke test not wired up for %s", runtime.GOOS)
	}
	cc := findCC(t)
	root, tmp := repoRoot(t), t.TempDir()
	lib, driver := filepath.Join(tmp, libName), filepath.Join(tmp, driverName)
	run(t, root, nil, "go", "build", "-buildvcs=false", "-buildmode=c-shared", "-o", lib, "./test/smoke/editorfixture")
	args := []string{
		"-Wall", "-Wextra", "-Werror", "-o", driver,
		filepath.Join(root, "test", "smoke", "testdata", "editor.c"),
		lib, "-I" + filepath.Join(root, "include"),
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-Wl,-rpath,"+tmp)
	}
	run(t, root, nil, cc, args...)
	run(t, tmp, []string{"LD_LIBRARY_PATH=" + tmp, "DYLD_LIBRARY_PATH=" + tmp}, driver)
}
