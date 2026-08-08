//go:build validator

// This file adds the second half of the validation story. smoke_test.go proves
// the module loads and answers a handful of calls the way we think a host makes
// them; this runs Steinberg's own `validator` against the same plugin, which
// checks the parts of the specification we would otherwise be grading ourselves
// on — bus arrangements, parameter ranges and flags, state round-trips, the
// processing contract and the required class metadata.
//
// It is behind its own build tag because it needs a validator binary, which is
// not part of this repository (the submodule carries only the C API headers,
// not the full C++ SDK). Point VST3_VALIDATOR at one, or put `validator` on
// PATH, and run:
//
//	go test -tags validator ./test/smoke/...
//
// Without a validator the test skips rather than fails, so a contributor
// without the SDK is never blocked. CI builds one and sets VST3_VALIDATOR.

package smoke

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/cwbudde/vst3go/pkg/bundle"
)

// The build helpers shared with smoke_test.go live in helpers_test.go, which
// carries both build tags.

// bundleName is the plugin bundle the validator is pointed at.
const bundleName = "VST3GoSmoke"

// findValidator locates Steinberg's validator, preferring an explicit path.
func findValidator(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("VST3_VALIDATOR"); path != "" {
		if _, err := os.Stat(path); err != nil {
			// An explicitly configured path that does not exist is a
			// misconfiguration, not a reason to skip silently.
			t.Fatalf("VST3_VALIDATOR=%q: %v", path, err)
		}
		return path
	}

	path, err := exec.LookPath("validator")
	if err != nil {
		t.Skip("no VST3 validator found; set VST3_VALIDATOR to its path")
	}
	return path
}

// buildBundle compiles the smoke plugin and lays it out as a VST3 bundle, which
// is the only form the SDK's module loader accepts. The layout itself comes
// from pkg/bundle, so this test and any downstream build script agree on it by
// construction rather than by two copies of the same table.
func buildBundle(t *testing.T, root, dir string) string {
	t.Helper()

	layout, err := bundle.For(bundleName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("%v", err)
	}

	binary, err := layout.Create(dir)
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	run(t, root, nil, "go", "build",
		"-buildvcs=false",
		"-buildmode=c-shared",
		"-o", binary,
		"./test/smoke/plugin",
	)

	return layout.Path(dir)
}

// TestValidatorAcceptsThePlugin is the closest thing to a real host in CI: it is
// Steinberg's own conformance suite, so it grades the runtime against the
// specification rather than against our reading of it.
func TestValidatorAcceptsThePlugin(t *testing.T) {
	validator := findValidator(t)

	root := repoRoot(t)
	bundle := buildBundle(t, root, t.TempDir())

	cmd := exec.Command(validator, bundle)
	out, err := cmd.CombinedOutput()
	t.Logf("%s %s\n%s", validator, bundle, out)

	if err != nil {
		t.Fatalf("validator rejected the plugin: %v", err)
	}

	// The validator exits 0 on success, but say so explicitly: a build that
	// somehow ran no tests at all would also exit 0.
	if !strings.Contains(string(out), "Result:") {
		t.Error("validator produced no result summary; it may not have run any tests")
	}
}
