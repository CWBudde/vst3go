# Consuming this fork, and what changed

This document covers two things a downstream repo — `synthkit`, or any plugin
project — needs from `vst3go`: how to depend on it, and what has broken since
the fork diverged.

## Consuming the fork

### Module path

The module is `github.com/cwbudde/vst3go`. The original project is
`github.com/justyntemme/vst3go`; this fork is not API-compatible with it and
does not track it. Nothing resolves the old path to the new one, so the import
path has to be changed by hand:

```bash
go mod edit -droprequire github.com/justyntemme/vst3go
go get github.com/cwbudde/vst3go
grep -rl justyntemme/vst3go --include='*.go' . | xargs sed -i 's|justyntemme/vst3go|cwbudde/vst3go|g'
```

### The submodule constraint, and its removal

**This is resolved. `go get` is now enough.** If you are carrying the `replace`
workaround below, this section is how to drop it.

The VST 3 C API header used to be a git submodule (`include/vst3`) rather than a
redistributed file. Git submodule content is not part of the repository tree that
the Go module proxy serves, so a module downloaded from the proxy had an empty
`include/vst3` and every cgo package in it failed to compile:

```
bridge.h:4:10: fatal error: ../include/vst3/vst3_c_api.h: No such file or directory
```

`CGO_CFLAGS` could not rescue this: the header is included by relative path from
several different directory depths, so no single `-I` satisfies all of them. The
only fix that survives a proxy download is for the header to be inside the module,
so it is vendored now, under Steinberg's GPLv3 alternative — the proprietary
license does not permit redistribution. [LICENSE](../LICENSE) has the reasoning
and what it does and does not change for you.

To migrate, delete the workaround:

```bash
go mod edit -dropreplace github.com/cwbudde/vst3go
go get github.com/cwbudde/vst3go@latest
go mod tidy
```

A downstream repository's CI can drop the same scaffolding: no sibling checkout,
no `submodules: recursive`, no header fetch step. If your plug-in sources are
behind a build tag that exists only to keep the rest of the repository buildable
while this module was not consumable, that tag has no reason to survive the
migration either.

### Versioning

No versions are tagged yet. When they are, the intent is:

- `v0.x` while the runtime API is still moving. Minor bumps may break API; that
  is what pre-1.0 is for, and each break is recorded in this file.
- `v1.0.0` only once the public surface of `pkg/plugin`, `pkg/framework/*` and
  `pkg/midi` is stable enough that a downstream repo can upgrade without
  reading a changelog first.

Pin an exact version rather than tracking a branch. A tag is now something to
`go get`, not just a git reference to check out.

## Breaking changes

Three breaks have accumulated since the fork diverged. None of them is behind a
build tag or a compatibility shim.

### 1. Preset format is now v2

`pkg/framework/state` writes format version 2, which length-prefixes the custom
payload so a reader with no custom loader can skip it. Version 1 wrote the
payload unframed, running to the end of the stream.

- **Reading is compatible.** `Manager.Load` still reads v1 presets.
- **Writing is not.** A v2 preset loaded by a build that predates the framing
  fails with an unsupported-version error. Presets move forward, not backward.

Nothing is required of a downstream repo unless it ships builds that have to
interoperate both ways — in that case, roll the plugin's own version forward at
the same time. The full wire format is documented in the `pkg/framework/state`
package documentation.

### 2. `midi.Event` is one struct, not an interface

The per-message types (`NoteOnEvent`, `ControlChangeEvent`, …) and `BaseEvent`
are gone. There is now a single concrete `midi.Event` struct with a `Type` tag.
This is why the audio path is allocation-free: an interface boxed every event
once per block.

Migration:

| Before                         | After                                                       |
| ------------------------------ | ----------------------------------------------------------- |
| `midi.NoteOnEvent{...}`        | `midi.NewNoteOn(channel, note, velocity, offset)`           |
| `midi.ControlChangeEvent{...}` | `midi.NewControlChange(channel, controller, value, offset)` |
| `switch e := ev.(type)`        | `switch ev.Type`                                            |
| `e.(midi.NoteOnEvent).Note`    | `ev.Note()`                                                 |

The accessors (`Note`, `Velocity`, `Controller`, `Value`, `Pressure`,
`Program`) are type-guarded: they return zero for an event of the wrong
kind rather than panicking, so a missed case in a rewritten type switch is
silent. Check `ev.Type` first. Pitch bend is the exception: it is the `Bend`
field, read either raw or through `NormalizedBend`, which maps it onto
[-1, 1).

`EventQueue.OffsetEvents` no longer returns an error. Callers that checked it
will not compile.

### 3. Only one bypass parameter per plugin

`BypassParameter` now sets the `IsBypass` flag, which is what makes a host treat
it as the plugin's bypass. VST 3 permits exactly one such parameter per plugin,
and the validator rejects a second one.

Two builders in this repo were using `BypassParameter` for controls that are not
the plugin bypass and were switched to a plain `Choice`:

- `RegisterCompressorControls` — "Auto Gain"
- `RegisterEQBand` — "Band N Enable"

Any downstream code that built a per-section enable out of `BypassParameter`
needs the same change, or the plugin will fail validation.

## Deprecations

The four `Parameter` value accessors below still work and still delegate to
their replacements, but they are marked deprecated and the runtime no longer
calls them. The Value spelling never said which of the two value domains it
meant.

| Deprecated                | Use                       |
| ------------------------- | ------------------------- |
| `Parameter.GetValue`      | `Parameter.GetNormalized` |
| `Parameter.SetValue`      | `Parameter.SetNormalized` |
| `Parameter.GetPlainValue` | `Parameter.GetPlain`      |
| `Parameter.SetPlainValue` | `Parameter.SetPlain`      |

The result-code aliases `vst3.ResultOk` and `vst3.ResultTrue` are likewise
compatibility-only; `vst3.ResultOK` is canonical.
