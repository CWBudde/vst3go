# Validation

This document draws the line between what `vst3go` validates on its own behalf
and what a repo built on top of it — `synthkit`, or any plugin project — has to
validate for itself. The short version: **this repo proves the runtime obeys the
VST3 contract; it cannot prove that your plugin does.**

## What this repo validates

Run everything with:

```bash
just test                                   # unit tests, -race
go test -tags smoke ./test/smoke/...        # loads a real module, drives it from C
go test -tags validator ./test/smoke/...    # Steinberg's conformance suite
```

CI runs all three on every push, plus `go vet`, `golangci-lint` and a
`-Werror` compile of the C bridge.

### Unit tests

Per-package tests covering the runtime primitives: parameter normalization and
step quantization, preset save/load round-trips (including the v1 → v2 format
migration), MIDI queue ordering and chunk-boundary delivery, bus configuration,
and the process context.

Two categories are worth calling out because they encode invariants rather than
behaviour, and are easy to break without noticing:

- **Allocation guards.** `testing.AllocsPerRun` asserts **0 allocations** for the
  audio path: `Context`, `getComponent`, `processChunk`, `processSampleAccurate`,
  MIDI event construction and range reads, note/frequency conversion, and the
  smoother's per-sample advance. A block carrying 32 MIDI events must allocate
  exactly as much as a silent one, which is nothing.
- **Race coverage.** Everything runs under `-race`, and the places where a
  control thread and the audio thread genuinely meet have dedicated contention
  tests: the event queue, `Smoother.SetTarget` against `Next`, `SetBusActive`
  against audio-thread bus reads, and `state.Manager.Save` against concurrent
  parameter registration.

### Smoke test (`-tags smoke`)

Builds the runtime into an actual `c-shared` module and drives it from a C
driver the way a host would. Nothing else in the repository links `pkg/plugin`
into a loadable library, so without this the entire cgo wrapper surface is only
ever checked by the compiler.

It covers the factory lifecycle, COM refcounting (including releasing the
factory and re-acquiring it, which used to return freed memory), `queryInterface`
dispatch by IID, and a `process()` call with more than one input bus — the case
that used to panic inside a recovered callback and emit garbage rather than
crash.

### Steinberg validator (`-tags validator`)

The `validator` from Steinberg's SDK: 47 conformance tests covering bus
arrangements, parameter flags and ranges, state round-trips, the processing
contract, sample-accurate automation and the required class metadata. This is
the one check that grades the runtime against the specification instead of
against our reading of it.

The validator is not vendored — the `include/vst3` submodule carries only the C
API headers, not the C++ SDK. Build one, then point the test at it:

```bash
VST3_VALIDATOR=/path/to/validator go test -tags validator ./test/smoke/...
```

Without `VST3_VALIDATOR` (and with no `validator` on `PATH`) the test skips, so
contributors without the SDK are never blocked. CI builds it from a pinned SDK
commit and caches it.

## What downstream repos must validate themselves

The smoke and validator tests here run against a deliberately minimal plugin:
one class, one parameter, one stereo bus pair, and a pass-through
`ProcessAudio`. That is the right shape for proving the *runtime* works, and the
wrong shape for proving *your plugin* works. Everything below is outside what
this repo can see.

### 1. Run the validator against your own plugin

This is the single highest-value thing to copy. The runtime passing conformance
with a trivial plugin says nothing about a plugin with sidechains, many
parameters, a bypass, or a non-trivial state blob. `test/smoke/validator_test.go`
is written to be lifted: swap the bundle it builds for yours.

Configurations worth adding that the minimal plugin cannot exercise:

- more than one input or output bus (sidechain, multi-out)
- more than 16 channels on a bus
- an instrument plugin (event input, no audio input)
- a plugin that reports non-zero latency or tail
- a real bypass parameter — note that VST3 permits exactly **one** parameter
  with `IsBypass` per plugin, so an on/off control that is not the plugin's
  bypass must be a plain `Choice`

### 2. Realtime safety of your DSP

The runtime is allocation-free up to the point it calls `ProcessAudio`, and its
tests keep it that way. From there it is yours. On the audio thread your code
must not allocate, lock, block, or make syscalls — no `append` that can grow, no
map access, no `fmt` anything, no channel operations, no `sync.Mutex`.

Apply the same guard to your own processors:

```go
allocs := testing.AllocsPerRun(200, func() { proc.ProcessAudio(ctx) })
if allocs != 0 {
    t.Errorf("ProcessAudio allocated %.1f times per block, want 0", allocs)
}
```

Call it once before measuring so lazily-built state is not counted as a
steady-state allocation — and assert that the warm-up actually did some work,
or the measurement can pass on an empty code path.

### 3. DSP correctness

Entirely out of scope here. `vst3go` has no opinion about what your filter
sounds like. Worth covering downstream: impulse and step responses, coefficient
stability across the full sample-rate range, denormal behaviour, and that
`SetActive(false)` genuinely clears every piece of internal state so a
re-activated plugin does not emit a tail from its previous life.

### 4. Preset compatibility

The runtime serializes registered parameters and hands you a callback for
anything else. It cannot know whether *your* custom blob survives a version
change.

Two specifics:

- The preset format is at **version 2** (length-prefixed custom payload). v1
  presets still load; presets written now cannot be read by an older build.
- If you register a custom state loader, test the round-trip and test what
  happens when an *older* plugin build meets a *newer* preset, and vice versa.
  The runtime skips a custom payload it has no loader for, but it cannot
  interpret one it does not understand.

### 5. Host testing

No automated suite substitutes for loading the plugin in a real DAW. The
validator will not catch a host that calls things in an order it does not model.
At minimum, in Reaper, Bitwig or Live: instantiate; automate a parameter and
listen for dropouts; toggle bypass; save, close and reload the project; add a
sidechain; and change the block size and sample rate while playing.
