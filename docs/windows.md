# Windows support

This document covers what building a `vst3go` plug-in for Windows requires, and
the platform-specific risks that come with hosting the Go runtime inside a DAW's
process.

## Toolchain

cgo needs a GCC-compatible compiler, so **MinGW-w64** is the supported
toolchain. MSVC is not usable: cgo cannot drive it, and there is no plan to add
that here — the runtime has no MSVC-only dependency that would justify it.

Targets: `windows/amd64` and `windows/arm64`.

`windows/386` is deliberately unsupported. The SDK's `PLUGIN_API` is `__stdcall`
on 32-bit Windows, and cgo emits `cdecl` for `//export` functions; the two only
coincide on 64-bit, where Windows has a single calling convention.
`bundle.For` returns an error for it rather than producing a layout that would
load and then misbehave.

### Building natively

```powershell
go build -buildmode=c-shared -o MyPlugin.vst3/Contents/x86_64-win/MyPlugin.vst3 ./cmd/myplugin
```

### Cross-compiling from Linux

```bash
sudo apt install gcc-mingw-w64-x86-64   # Debian/Ubuntu

GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -buildmode=c-shared -o MyPlugin.vst3/Contents/x86_64-win/MyPlugin.vst3 ./cmd/myplugin
```

`just build-windows` runs exactly this against the smoke plugin as a build
check.

## Bundle layout

Windows uses the same bundle-shaped layout as the other platforms, with one
detail that reliably trips people up: **the DLL keeps the `.vst3` extension**,
not `.dll`.

```
MyPlugin.vst3/
  Contents/
    x86_64-win/
      MyPlugin.vst3      <- the DLL
```

`pkg/bundle` computes this — including the `arm64-win` variant and the macOS and
Linux equivalents — so build scripts do not have to re-derive it:

```go
l, _ := bundle.For("MyPlugin", "windows", "amd64")
binary, _ := l.Create(outputDir)   // returns the path to build the DLL at
```

## Entry points, and why they are defined in Go

A host resolves `GetPluginFactory` from the loaded module by name, plus
`InitDll`/`ExitDll` on Windows (`bundleEntry`/`bundleExit` on macOS,
`ModuleEntry`/`ModuleExit` on Linux).

These are defined in Go with `//export`, in `pkg/plugin/cbridge`, rather than in
the C bridge. That is not a stylistic choice. For a `c-shared` build the Go
linker generates a `.def` file whose `EXPORTS` section is built **solely** from
`//export` symbols and hands it to `ld`, which then exports exactly that set. A
C-side `__declspec(dllexport)` is dropped with no diagnostic at all: the DLL
compiles, links, and loads, and every host reports it as an invalid plug-in
because `GetProcAddress` cannot find the factory.

Because the export mechanism has to be Go's on Windows and works equally well on
ELF and Mach-O, all three platforms use it — which is why `bridge/bridge.c`
carries no `#ifdef` for platform entry points at all.

`test/smoke/exports_test.go` checks this by reading the real export table out of
the built module (PE, ELF and Mach-O), including a Windows cross-build from any
host with MinGW-w64 installed. It is the only thing standing between a
refactor and a silently unloadable plug-in.

## Risks

### The module stays loaded until the process exits

Go's `c-shared` runtime can leave threads and a vectored exception handler
active after the VST3 host releases the module. Both `InitDll` and
`GetPluginFactory` pin the DLL before returning success, so the host can call
`ExitDll` and `FreeLibrary` and then reopen the plug-in safely. The Windows
C smoke test exercises both entry paths. [Go issue #32497](https://github.com/golang/go/issues/32497)

### One Go runtime per module

Each loaded plug-in DLL carries its own copy of the Go runtime, with its own
GC, scheduler and OS threads. Loading many Go plug-ins in one DAW multiplies
that cost. It is correct, but it is not free, and it is a different profile from
a C++ plug-in where the marginal cost of the tenth instance is close to zero.

### Exception handling

The Go runtime installs a vectored exception handler at load time. It is
process-wide, so it sees exceptions raised by the host and by other plug-ins,
not only by Go code. This is normally transparent, but it is the first thing to
suspect if a host's own crash reporter starts behaving differently once a Go
plug-in is installed.

### Scheduler and GC against the audio thread

The audio callback arrives on a host thread that Go did not create. Go's
garbage collector can still stop it: a stop-the-world pause suspends every
goroutine-running thread, including one currently inside `Process`. The runtime
packages here are allocation-free on the audio path precisely so that a plug-in
can avoid triggering GC at all, but that guarantee only holds as far as the
plug-in's own DSP keeps it — see [validation.md](validation.md).

Windows-specific aggravations: the default timer resolution is coarser than on
Linux, and Go threads do not register with MMCSS, so they do not get the
audio-priority scheduling class a native audio thread would ask for.

### File locking and the reload loop

Windows locks a DLL while it is loaded. A build that writes over an installed
plug-in can fail with a sharing violation until the DAW and any scanner
processes that loaded it have exited. Build into a staging directory, close
those processes, install the updated bundle and reopen the host.

### Path length

The default `MAX_PATH` limit is 260 characters, and the bundle layout is deep:

```
C:\Program Files\Common Files\VST3\MyPlugin.vst3\Contents\x86_64-win\MyPlugin.vst3
```

That is fine on its own, but a long vendor sub-directory plus a long plug-in
name gets close, and the failure mode is an installer or build script that
half-succeeds. Long-path support is opt-in per system and cannot be assumed.

### Antivirus

An unsigned, freshly built DLL appearing in a scanned directory is exactly the
shape of thing endpoint protection quarantines. When a plug-in "disappears"
after a build on Windows, check the quarantine before debugging the loader.

## Verification

CI covers, for Windows:

- `go build` / `go vet` / `go test -race` on `windows-latest`
- the smoke test on `windows-latest`: a real `c-shared` module driven from C
- a `-Werror` MinGW-w64 compile of the C bridge
- the cross-built DLL's export table and import table, from the Linux runner

Win64 REAPER 7.86 under Wine scanned the pinned module, opened two native
editor windows at 144 DPI, closed and reopened one editor and exited cleanly.
The loader trace showed one module load and no access violations.

Pending coverage:

- Steinberg's `validator` on Windows (it runs on Linux only)
- a DAW on native Windows
- `windows/arm64`, which has a defined bundle layout but no CI build

Downstream repos should read [validation.md](validation.md) for what they still
have to validate themselves.
