// Package bundle describes the on-disk layout a compiled VST3 module must have
// before a host will load it.
//
// A VST3 plug-in is not a bare shared library. On every platform it is a
// directory named "<Name>.vst3" containing a "Contents" directory, and inside
// that a per-architecture subdirectory holding the binary. The subdirectory
// name, the binary's file name and whether extra metadata files are required
// all differ per platform, and getting any of them wrong produces the same
// symptom: the host silently ignores the plug-in.
//
// This package holds that knowledge in one tested place so the layout is not
// re-derived by every build script. It only computes and creates the layout —
// compiling the binary is the caller's job:
//
//	l, err := bundle.For("MyPlugin", runtime.GOOS, runtime.GOARCH)
//	if err != nil { ... }
//	binary, err := l.Create("/path/to/output")
//	// then: go build -buildmode=c-shared -o binary ./cmd/myplugin
//
// Threading: everything here is plain build-time file manipulation. None of it
// belongs anywhere near the audio thread.
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Extension is the directory suffix that marks a VST3 bundle. Hosts scan for
// it; a bundle without it is not found.
const Extension = ".vst3"

// The GOOS and GOARCH values this package knows a layout for.
const (
	osDarwin  = "darwin"
	osLinux   = "linux"
	osWindows = "windows"

	archAMD64 = "amd64"
	archARM64 = "arm64"
)

// Layout is the resolved bundle layout for one platform.
type Layout struct {
	// Name is the plug-in name, without any extension.
	Name string

	// Dir is the bundle directory name, e.g. "MyPlugin.vst3".
	Dir string

	// ArchDir is the path inside the bundle that holds the binary, relative to
	// the bundle directory, e.g. "Contents/x86_64-win".
	ArchDir string

	// Binary is the file name of the shared library inside ArchDir. Note that
	// this is not the same on every platform: Windows keeps the .vst3
	// extension on the DLL itself, Linux uses .so, and macOS uses no extension
	// at all.
	Binary string

	// Files holds extra metadata files the platform requires, keyed by their
	// path relative to the bundle directory. It is empty except on macOS,
	// where it carries Contents/Info.plist and Contents/PkgInfo. Callers that
	// need their own bundle identifier or version can replace an entry before
	// calling Create.
	Files map[string]string
}

// archDirs maps a platform to its VST3 architecture subdirectory names.
//
// Only 64-bit targets are listed. 32-bit VST3 is effectively extinct, and on
// windows/386 the SDK's PLUGIN_API calling convention (__stdcall) does not
// match what cgo emits for //export functions, so claiming support would be
// dishonest.
var archDirs = map[string]map[string]string{
	osWindows: {
		archAMD64: "x86_64-win",
		archARM64: "arm64-win",
	},
	osLinux: {
		archAMD64: "x86_64-linux",
		archARM64: "aarch64-linux",
	},
}

// For resolves the bundle layout for a plug-in name and a GOOS/GOARCH pair.
//
// name may be given with or without the .vst3 extension. Passing
// runtime.GOOS/runtime.GOARCH gives the layout for the host; passing them
// explicitly gives the layout for a cross-compilation target.
func For(name, goos, goarch string) (Layout, error) {
	base := strings.TrimSuffix(name, Extension)
	if base == "" {
		return Layout{}, fmt.Errorf("bundle: empty plug-in name %q", name)
	}

	l := Layout{
		Name:  base,
		Dir:   base + Extension,
		Files: map[string]string{},
	}

	switch goos {
	case osDarwin:
		// macOS bundles are architecture-independent by convention: the binary
		// is a universal one under a fixed directory name.
		l.ArchDir = filepath.Join("Contents", "MacOS")
		l.Binary = base
		// Without an Info.plist the bundle does not load at all, and
		// CFBundleExecutable is what points the loader at the binary.
		l.Files[filepath.Join("Contents", "Info.plist")] = infoPlist(base)
		l.Files[filepath.Join("Contents", "PkgInfo")] = "BNDL????"
	case osWindows, osLinux:
		arch, ok := archDirs[goos][goarch]
		if !ok {
			return Layout{}, fmt.Errorf("bundle: no VST3 layout defined for %s/%s", goos, goarch)
		}
		l.ArchDir = filepath.Join("Contents", arch)
		if goos == osWindows {
			// The DLL keeps the .vst3 extension; that is the specification,
			// not a typo. On Linux it is a normal shared object.
			l.Binary = base + Extension
		} else {
			l.Binary = base + ".so"
		}
	default:
		return Layout{}, fmt.Errorf("bundle: no VST3 layout defined for %s/%s", goos, goarch)
	}

	return l, nil
}

// Path returns the bundle directory inside root.
func (l Layout) Path(root string) string {
	return filepath.Join(root, l.Dir)
}

// BinaryPath returns the full path the compiled shared library must be written
// to, inside root.
func (l Layout) BinaryPath(root string) string {
	return filepath.Join(root, l.Dir, l.ArchDir, l.Binary)
}

// Create builds the bundle's directory tree under root and writes any
// platform-required metadata files. It returns the path the compiled binary
// must be written to, which is the same value BinaryPath reports.
//
// Create is safe to call on an existing bundle: directories are created as
// needed and metadata files are overwritten.
//
// The permissions are world-readable on purpose. A plug-in bundle is normally
// installed into a shared location (~/.vst3, /usr/lib/vst3, the Windows common
// VST3 directory) and has to be traversable and readable by whatever user the
// host process runs as, which is not necessarily the user that built it.
func (l Layout) Create(root string) (string, error) {
	const (
		dirPerm  = 0o755
		filePerm = 0o644
	)

	dir := filepath.Join(root, l.Dir, l.ArchDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", fmt.Errorf("bundle: create %s: %w", dir, err)
	}

	for rel, content := range l.Files {
		path := filepath.Join(root, l.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			return "", fmt.Errorf("bundle: create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
			return "", fmt.Errorf("bundle: write %s: %w", path, err)
		}
	}

	return l.BinaryPath(root), nil
}

func infoPlist(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>` + name + `</string>
	<key>CFBundleIdentifier</key>
	<string>com.vst3go.` + strings.ToLower(name) + `</string>
	<key>CFBundleName</key>
	<string>` + name + `</string>
	<key>CFBundlePackageType</key>
	<string>BNDL</string>
	<key>CFBundleVersion</key>
	<string>1.0.0</string>
</dict>
</plist>
`
}
