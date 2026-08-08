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

## Consuming this repo

The module path is `github.com/cwbudde/vst3go`. Note that `go get` alone is not
enough to build against it: the VST 3 C API header is a git submodule rather
than a redistributed file, and submodule content is not part of what the Go
module proxy serves, so a proxy-downloaded copy fails to compile. Depend on a
checkout instead:

```bash
git clone --recurse-submodules git@github.com:CWBudde/vst3go.git
```

```
// go.mod in the consuming repo
require github.com/cwbudde/vst3go v0.0.0
replace github.com/cwbudde/vst3go => ../vst3go
```

No versions are tagged yet. [docs/migration.md](docs/migration.md) covers this
in full, along with the intended versioning scheme, the accumulated breaking
changes (preset format v2, the `midi.Event` rework, the bypass-flag rule), and
the deprecated parameter accessors.

## Development

Core checks in this repo:

```bash
just fmt
just fmt-check
just test
```

Two heavier checks sit behind build tags. The first builds the runtime into a
real loadable module and drives it from C; the second runs Steinberg's own
conformance suite against it.

```bash
go test -tags smoke ./test/smoke/...
VST3_VALIDATOR=/path/to/validator go test -tags validator ./test/smoke/...
```

[docs/validation.md](docs/validation.md) explains what each of these covers, and
— more importantly — what a downstream repo such as `synthkit` still has to
validate on its own.

## License

This project's own source is licensed under the MIT License — see [LICENSE](LICENSE).

The VST 3 C API headers are **not** redistributed here; they are a git submodule
(`include/vst3`) under Steinberg's dual license (the VST 3 License Agreement *or*
GPLv3). A compiled plug-in is a combined work with those headers, so the terms
under which you may distribute a binary come from whichever Steinberg license you
have accepted — without a signed agreement, that is the GPLv3. See the third-party
notice in [LICENSE](LICENSE) for details.
