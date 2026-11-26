//go:build smoke

package smoke

import (
	"path/filepath"
	"testing"
)

func TestWindowsModuleRemainsLoadedAfterHostRelease(t *testing.T) {
	cc := findCC(t)
	root := repoRoot(t)
	tmp := t.TempDir()
	module := filepath.Join(tmp, "module.dll")
	driver := filepath.Join(tmp, "unload.exe")

	run(t, root, nil, "go", "build", "-buildvcs=false", "-buildmode=c-shared", "-o", module, "./test/smoke/plugin")
	run(t, root, nil, cc, "-I"+filepath.Join(root, "include"),
		"-o", driver, filepath.Join(root, "test", "smoke", "testdata", "unload_windows.c"))

	for _, entry := range []string{"init", "factory"} {
		t.Run(entry, func(t *testing.T) {
			run(t, tmp, nil, driver, module, entry)
		})
	}
}
