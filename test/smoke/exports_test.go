//go:build smoke

// This file guards the one defect class that no amount of Go testing catches
// and no compiler warns about: an entry point that is present in the object
// code but missing from the shared library's export table. The module then
// links cleanly, loads cleanly, and every host reports it as invalid, because
// dlsym/GetProcAddress cannot find GetPluginFactory.
//
// That is exactly what a C-side __declspec(dllexport) produces on Windows: for
// a c-shared build the Go linker generates a .def file whose EXPORTS section is
// built solely from //export symbols and hands it to ld, so anything exported
// only from C is dropped silently. The fix — defining the entry points in Go —
// is invisible in the source, so it is pinned here instead.
//
// The Windows half runs as a cross-build from any host with a MinGW-w64
// toolchain, which is what makes Windows support checkable in CI without a
// Windows runner.

package smoke

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requiredExports lists the symbols a VST3 host resolves by name after loading
// a module. GetPluginFactory is mandatory everywhere; the entry/exit pair is
// optional per the SDK's loader but hosts that call one call the other, and
// their names differ per platform.
func requiredExports(goos string) []string {
	switch goos {
	case "windows":
		return []string{"GetPluginFactory", "InitDll", "ExitDll"}
	case "darwin":
		return []string{"GetPluginFactory", "bundleEntry", "bundleExit"}
	default:
		return []string{"GetPluginFactory", "ModuleEntry", "ModuleExit"}
	}
}

// TestModuleExportsTheHostEntryPoints checks the module built for the host
// platform.
func TestModuleExportsTheHostEntryPoints(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skipf("no export-table reader wired up for %s", runtime.GOOS)
	}

	root := repoRoot(t)
	lib := filepath.Join(t.TempDir(), "module"+sharedLibExt(runtime.GOOS))

	run(t, root, nil, "go", "build",
		"-buildvcs=false",
		"-buildmode=c-shared",
		"-o", lib,
		"./test/smoke/plugin",
	)

	assertExports(t, lib, runtime.GOOS)
}

// TestWindowsCrossBuildExportsTheHostEntryPoints builds the Windows DLL from
// whatever host is running the test and inspects its PE export table. It skips
// when no MinGW-w64 cross compiler is installed.
func TestWindowsCrossBuildExportsTheHostEntryPoints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by TestModuleExportsTheHostEntryPoints on this host")
	}

	const cc = "x86_64-w64-mingw32-gcc"
	if _, err := exec.LookPath(cc); err != nil {
		t.Skipf("no %s on PATH; install MinGW-w64 to check the Windows build", cc)
	}

	root := repoRoot(t)
	dll := filepath.Join(t.TempDir(), "module.dll")

	run(t, root,
		[]string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=1", "CC=" + cc},
		"go", "build",
		"-buildvcs=false",
		"-buildmode=c-shared",
		"-o", dll,
		"./test/smoke/plugin",
	)

	assertExports(t, dll, "windows")
	assertNoToolchainRuntimeImports(t, dll)
}

// assertNoToolchainRuntimeImports checks the DLL does not depend on MinGW's
// own runtime DLLs.
//
// This is the second Windows failure mode that only shows up on someone else's
// machine: a build that picks up libgcc_s_seh-1.dll or libwinpthread-1.dll
// loads fine on the box that compiled it, where those sit next to the
// compiler, and fails to load for every user who does not have a MinGW
// installation. The current build imports nothing beyond the OS, so no
// -static-libgcc style link flags are needed; this pins that so a future flag
// change cannot quietly reintroduce the dependency.
func assertNoToolchainRuntimeImports(t *testing.T, dll string) {
	t.Helper()

	f, err := pe.Open(dll)
	if err != nil {
		t.Fatalf("open %s: %v", dll, err)
	}
	defer f.Close()

	syms, err := f.ImportedSymbols()
	if err != nil {
		t.Fatalf("read imports: %v", err)
	}

	for _, sym := range syms {
		// ImportedSymbols returns entries shaped as "symbol:library.dll".
		_, lib, ok := strings.Cut(sym, ":")
		if !ok {
			continue
		}
		switch lower := strings.ToLower(lib); {
		case strings.HasPrefix(lower, "libgcc"),
			strings.HasPrefix(lower, "libwinpthread"),
			strings.HasPrefix(lower, "libstdc++"):
			t.Errorf("the DLL imports %s; it would fail to load on a machine without the MinGW runtime", lib)
		}
	}
}

func sharedLibExt(goos string) string {
	switch goos {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	default:
		return ".so"
	}
}

func assertExports(t *testing.T, lib, goos string) {
	t.Helper()

	exports, err := readExports(lib, goos)
	if err != nil {
		t.Fatalf("read exports from %s: %v", lib, err)
	}

	for _, name := range requiredExports(goos) {
		if !exports[name] {
			t.Errorf("%s is not exported from the %s module; a host would reject it as an invalid plug-in", name, goos)
		}
	}
	if t.Failed() {
		t.Logf("exports found: %v", sortedKeys(exports))
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func readExports(path, goos string) (map[string]bool, error) {
	switch goos {
	case "windows":
		return readPEExports(path)
	case "darwin":
		return readMachOExports(path)
	default:
		return readELFExports(path)
	}
}

func readELFExports(path string) (map[string]bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	syms, err := f.DynamicSymbols()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, s := range syms {
		// Undefined entries are imports, not exports.
		if s.Section == elf.SHN_UNDEF {
			continue
		}
		out[s.Name] = true
	}
	return out, nil
}

func readMachOExports(path string) (map[string]bool, error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if f.Symtab == nil {
		return nil, fmt.Errorf("no symbol table")
	}
	out := map[string]bool{}
	for _, s := range f.Symtab.Syms {
		// Mach-O prefixes C symbols with an underscore.
		out[strings.TrimPrefix(s.Name, "_")] = true
	}
	return out, nil
}

// readPEExports walks the PE export directory by hand. debug/pe parses the
// headers but does not expose the export table, and the walk is short: locate
// the export data directory, find the section holding it, then read the name
// pointer array it points at.
func readPEExports(path string) (map[string]bool, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var dir pe.DataDirectory
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		dir = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	case *pe.OptionalHeader32:
		dir = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	default:
		return nil, fmt.Errorf("unrecognized PE optional header")
	}
	if dir.VirtualAddress == 0 || dir.Size == 0 {
		return nil, fmt.Errorf("the DLL has no export directory at all")
	}

	sect := sectionForRVA(f, dir.VirtualAddress)
	if sect == nil {
		return nil, fmt.Errorf("no section contains the export directory (RVA %#x)", dir.VirtualAddress)
	}
	data, err := sect.Data()
	if err != nil {
		return nil, err
	}

	// IMAGE_EXPORT_DIRECTORY: NumberOfNames at offset 24, AddressOfNames at 32.
	at := func(rva uint32) (uint32, error) {
		off := rva - sect.VirtualAddress
		if uint64(off)+4 > uint64(len(data)) {
			return 0, fmt.Errorf("RVA %#x is outside section %s", rva, sect.Name)
		}
		return binary.LittleEndian.Uint32(data[off:]), nil
	}

	numNames, err := at(dir.VirtualAddress + 24)
	if err != nil {
		return nil, err
	}
	namesRVA, err := at(dir.VirtualAddress + 32)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	for i := uint32(0); i < numNames; i++ {
		nameRVA, err := at(namesRVA + i*4)
		if err != nil {
			return nil, err
		}
		name, err := cstringAt(data, sect.VirtualAddress, nameRVA)
		if err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, nil
}

func sectionForRVA(f *pe.File, rva uint32) *pe.Section {
	for _, s := range f.Sections {
		if rva >= s.VirtualAddress && rva < s.VirtualAddress+s.VirtualSize {
			return s
		}
	}
	return nil
}

func cstringAt(data []byte, base, rva uint32) (string, error) {
	off := int(rva - base)
	if off < 0 || off >= len(data) {
		return "", fmt.Errorf("string RVA %#x is outside the section", rva)
	}
	end := off
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[off:end]), nil
}
