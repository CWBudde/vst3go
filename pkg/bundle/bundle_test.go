package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestForKnownPlatforms pins the layout for every platform this repo claims to
// support. These strings come from the VST3 format specification, not from
// convention: a host that does not find the binary at exactly this path treats
// the bundle as if it were not there, with no error message anywhere.
func TestForKnownPlatforms(t *testing.T) {
	tests := []struct {
		goos, goarch    string
		archDir, binary string
		extraFiles      []string
	}{
		{"linux", "amd64", "Contents/x86_64-linux", "MyPlugin.so", nil},
		{"linux", "arm64", "Contents/aarch64-linux", "MyPlugin.so", nil},
		{"windows", "amd64", "Contents/x86_64-win", "MyPlugin.vst3", nil},
		{"windows", "arm64", "Contents/arm64-win", "MyPlugin.vst3", nil},
		{
			"darwin", "amd64", "Contents/MacOS", "MyPlugin",
			[]string{"Contents/Info.plist", "Contents/PkgInfo"},
		},
		{
			"darwin", "arm64", "Contents/MacOS", "MyPlugin",
			[]string{"Contents/Info.plist", "Contents/PkgInfo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			l, err := For("MyPlugin", tt.goos, tt.goarch)
			if err != nil {
				t.Fatalf("For: %v", err)
			}
			if l.Dir != "MyPlugin.vst3" {
				t.Errorf("Dir = %q, want MyPlugin.vst3", l.Dir)
			}
			if got := filepath.ToSlash(l.ArchDir); got != tt.archDir {
				t.Errorf("ArchDir = %q, want %q", got, tt.archDir)
			}
			if l.Binary != tt.binary {
				t.Errorf("Binary = %q, want %q", l.Binary, tt.binary)
			}
			if len(l.Files) != len(tt.extraFiles) {
				t.Errorf("Files has %d entries, want %d: %v", len(l.Files), len(tt.extraFiles), l.Files)
			}
			for _, want := range tt.extraFiles {
				if _, ok := l.Files[filepath.FromSlash(want)]; !ok {
					t.Errorf("Files is missing %q", want)
				}
			}
		})
	}
}

// TestWindowsBinaryKeepsTheVST3Extension guards the detail most likely to be
// "corrected" by someone assuming a DLL must be named .dll.
func TestWindowsBinaryKeepsTheVST3Extension(t *testing.T) {
	l, err := For("MyPlugin", "windows", "amd64")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if filepath.Ext(l.Binary) != Extension {
		t.Errorf("Binary = %q, want a %s extension", l.Binary, Extension)
	}
}

func TestForAcceptsNameWithExtension(t *testing.T) {
	with, err := For("MyPlugin.vst3", "linux", "amd64")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	without, err := For("MyPlugin", "linux", "amd64")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if with.BinaryPath("/out") != without.BinaryPath("/out") {
		t.Errorf("%q != %q", with.BinaryPath("/out"), without.BinaryPath("/out"))
	}
}

func TestForRejectsUnsupportedTargets(t *testing.T) {
	tests := []struct{ goos, goarch string }{
		{"windows", "386"},
		{"linux", "riscv64"},
		{"freebsd", "amd64"},
		{"js", "wasm"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			if _, err := For("MyPlugin", tt.goos, tt.goarch); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestForRejectsEmptyName(t *testing.T) {
	for _, name := range []string{"", ".vst3"} {
		if _, err := For(name, "linux", "amd64"); err == nil {
			t.Errorf("For(%q): expected an error, got nil", name)
		}
	}
}

// TestCreateWritesTheTree checks the layout actually lands on disk, including
// the macOS metadata, and that Create reports the same path BinaryPath does.
func TestCreateWritesTheTree(t *testing.T) {
	root := t.TempDir()

	l, err := For("MyPlugin", "darwin", "arm64")
	if err != nil {
		t.Fatalf("For: %v", err)
	}

	binary, err := l.Create(root)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if binary != l.BinaryPath(root) {
		t.Errorf("Create returned %q, BinaryPath says %q", binary, l.BinaryPath(root))
	}

	fi, statErr := os.Stat(filepath.Dir(binary))
	if statErr != nil || !fi.IsDir() {
		t.Fatalf("binary directory not created: %v", statErr)
	}

	plist, err := os.ReadFile(filepath.Join(l.Path(root), "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("read Info.plist: %v", err)
	}
	// CFBundleExecutable is how the macOS loader finds the binary; if it does
	// not match the file name the bundle loads as empty.
	if !strings.Contains(string(plist), "<string>MyPlugin</string>") {
		t.Errorf("Info.plist does not name the executable:\n%s", plist)
	}
}

// TestCreateIsIdempotent matters because build scripts re-run into an existing
// output directory far more often than into a clean one.
func TestCreateIsIdempotent(t *testing.T) {
	root := t.TempDir()
	l, err := For("MyPlugin", "linux", "amd64")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if _, err := l.Create(root); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := l.Create(root); err != nil {
		t.Fatalf("second Create: %v", err)
	}
}
