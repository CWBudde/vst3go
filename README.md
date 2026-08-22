# VST3Go

`vst3go` is the low-level Go interface and runtime layer for VST3.

This fork is intentionally different from the original `justyntemme/vst3go` project. It is being kept deliberately narrow as a clean VST3 binding/runtime layer, while the higher-level DSP and synth-oriented functionality lives in the companion `synthkit` repository.

It contains:

- VST3 C API headers and C bridge glue
- Go bindings for VST3 types and process structures
- The plugin wrapper/runtime used to expose Go processors as VST3 plugins
- Generic plugin authoring primitives such as buses, parameters, process context, state, and MIDI/event transport

The higher-level audio and synth layer now lives in the companion `synthkit` repository.

## Scope

`vst3go` keeps the VST3-facing pieces:

- `bridge/`
- `include/vst3/`
- `pkg/vst3/`
- `pkg/plugin/`
- `pkg/midi/`
- `pkg/bundle`
- `pkg/framework/bus`
- `pkg/framework/param`
- `pkg/framework/plugin`
- `pkg/framework/process`
- `pkg/framework/state`

The companion `synthkit` repo owns:

- `pkg/dsp/`
- `pkg/framework/dsp`
- `pkg/framework/debug`
- `pkg/framework/voice`
- `examples/`

## Companion Repo

Recommended name: `synthkit`

Purpose:

- DSP building blocks
- instrument/effect showcase plugins
- higher-level audio helpers built on top of `vst3go`

## Platform support

| Platform                | Builds | Smoke test | Validator |
| ----------------------- | ------ | ---------- | --------- |
| `linux/amd64`           | CI     | CI         | CI        |
| `darwin` (amd64, arm64) | CI     | —          | —         |
| `windows/amd64`         | CI     | CI         | —         |
| `windows/arm64`         | —      | —          | —         |

A plug-in is not a bare shared library on any of these: it is a `.vst3` bundle
directory with a per-architecture subdirectory inside. `pkg/bundle` computes
that layout, so build scripts do not have to hard-code it:

```go
l, err := bundle.For("MyPlugin", runtime.GOOS, runtime.GOARCH)
binary, err := l.Create(outputDir)  // where to write the c-shared build
```

Windows needs MinGW-w64 (MSVC cannot be driven by cgo) and has enough
platform-specific behaviour — DLL file locking, the Go runtime's unload
constraints, `MAX_PATH` — to warrant its own page:
[docs/windows.md](docs/windows.md).

## Consuming this repo

The module path is `github.com/cwbudde/vst3go`, and `go get` is enough:

```bash
go get github.com/cwbudde/vst3go
```

The VST 3 C API header is vendored in `include/vst3`, so a copy downloaded from
the module proxy compiles. It did not until recently, and a `replace` directive
pointing at a checkout was the documented workaround; if you are carrying one,
[docs/migration.md](docs/migration.md) has the removal. That file also covers
the versioning scheme, the accumulated breaking changes (preset format v2, the
`midi.Event` rework, the bypass-flag rule), and the deprecated parameter
accessors.

Vendoring the header means this repository redistributes it, which only
Steinberg's GPLv3 alternative permits. See [License](#license) below --- it
changes nothing about the terms for a plug-in you build, which were always the
same, but it is worth reading once.

## Development

Core checks in this repo:

```bash
just fmt
just fmt-check
just test
```

Two heavier checks sit behind build tags. The first builds the runtime into a
real loadable module, drives it from C, and checks the module actually exports
the entry points a host resolves by name; the second runs Steinberg's own
conformance suite against it.

```bash
just test-smoke                                   # go test -tags smoke ./test/smoke/...
VST3_VALIDATOR=/path/to/validator just test-validator
```

With a MinGW-w64 toolchain installed, `just test-smoke` also cross-builds the
Windows DLL and inspects its export and import tables. `just build-windows`
does the cross-build on its own.

[docs/validation.md](docs/validation.md) explains what each of these covers, and
— more importantly — what a downstream repo such as `synthkit` still has to
validate on its own.

## License

This project's own source is licensed under the MIT License — see [LICENSE](LICENSE).

The VST 3 C API headers **are** redistributed here, in `include/vst3`, under
Steinberg's dual license (the VST 3 License Agreement _or_ GPLv3). Only the
GPLv3 alternative permits that redistribution — the proprietary license forbids
distributing the SDK in parts without a signed agreement — so the copy you get
from this repository comes to you under the GPLv3, whose full text is in
[COPYING](COPYING). Three license files, for three different things: `COPYING`
is the GPLv3 the headers come under, [LICENSE](LICENSE) is this project's own
MIT license, and `include/vst3/LICENSE.txt` is Steinberg's dual-license notice,
carried verbatim with the headers.

That does not change the terms for a plug-in you build. A compiled plug-in is a
combined work with those headers either way, so the terms under which you may
distribute a binary come from whichever Steinberg license you have accepted —
without a signed agreement, that is the GPLv3. See the third-party notice in
[LICENSE](LICENSE) for details.
