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

### The submodule constraint

**`go get` alone is not enough to build against this module today.** The VST 3 C
API header is a git submodule (`include/vst3`) rather than a redistributed file,
for the licensing reason described in [LICENSE](../LICENSE). Git submodule
content is not part of the repository tree that the Go module proxy serves, so a
module downloaded from the proxy has an empty `include/vst3` and every cgo
package in it fails to compile:

```
bridge.h:4:10: fatal error: ../include/vst3/vst3_c_api.h: No such file or directory
```

`CGO_CFLAGS` does not rescue this: the header is included by relative path from
several different directory depths, so no single `-I` satisfies all of them.

Until that is resolved, depend on a checkout rather than on the proxy:

```bash
git clone --recurse-submodules git@github.com:CWBudde/vst3go.git
# or, in an existing clone:
git submodule update --init --recursive
```

and point the consuming module at it:

```
// go.mod
require github.com/cwbudde/vst3go v0.0.0
replace github.com/cwbudde/vst3go => ../vst3go
```

CI in a downstream repo needs the same treatment — check out the fork with
submodules as a sibling directory, or add the header fetch as a build step.

### Versioning

No versions are tagged yet. When they are, the intent is:

- `v0.x` while the runtime API is still moving. Minor bumps may break API; that
  is what pre-1.0 is for, and each break is recorded in this file.
- `v1.0.0` only once the public surface of `pkg/plugin`, `pkg/framework/*` and
  `pkg/midi` is stable enough that a downstream repo can upgrade without
  reading a changelog first.

Pin an exact version rather than tracking a branch. Because of the submodule
constraint above, a tag is a git reference to check out, not something to
`go get`.

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

| Before | After |
| --- | --- |
| `midi.NoteOnEvent{...}` | `midi.NewNoteOn(channel, note, velocity, offset)` |
| `midi.ControlChangeEvent{...}` | `midi.NewControlChange(channel, controller, value, offset)` |
| `switch e := ev.(type)` | `switch ev.Type` |
| `e.(midi.NoteOnEvent).Note` | `ev.Note()` |

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

| Deprecated | Use |
| --- | --- |
| `Parameter.GetValue` | `Parameter.GetNormalized` |
| `Parameter.SetValue` | `Parameter.SetNormalized` |
| `Parameter.GetPlainValue` | `Parameter.GetPlain` |
| `Parameter.SetPlainValue` | `Parameter.SetPlain` |

The result-code aliases `vst3.ResultOk` and `vst3.ResultTrue` are likewise
compatibility-only; `vst3.ResultOK` is canonical.
