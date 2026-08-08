# VST3Go TODO

## Repo Boundary

- [x] Keep `vst3go` scoped as the VST3 binding/runtime layer.
- [x] Move DSP packages, showcase plugins, and synth-product planning out to `synthkit`.
- [x] Rename the module to `github.com/cwbudde/vst3go`.
- [x] Update the README to state that this fork intentionally differs from the original project.
- [x] Review the retained package boundary once more.
  - [x] `pkg/vst3`
  - [x] `pkg/plugin`
  - [x] `pkg/midi`
  - [x] `pkg/framework/{bus,param,plugin,process,state}`
  - [x] Confirm no higher-level audio/product concerns have drifted back in.

## Rules

- [x] Keep the C bridge minimal and direct.
- [x] Keep business logic in Go rather than in C.
- [x] Keep DSP and product-specific logic out of this repo.
- [x] Keep example/showcase plugin ownership out of this repo.
- [x] Keep these rules reflected in docs and future planning updates.

## Current Baseline

- [x] Plugin implementations can import `pkg/plugin/cbridge` instead of including bridge code directly.
- [x] Core runtime packages exist for buses, parameters, process context, state, MIDI/events, plugin runtime, and VST3 bindings.
- [x] Sample-accurate parameter automation is implemented.
- [x] MIDI/event handling is implemented.
- [x] Advanced bus support is implemented.
- [x] Parameter IDs are standardized around typed `uint32` constants with `ParamXxx` naming.
- [x] Most runtime reset paths are in place on deactivation.
- [x] `go test ./pkg/...` passes after the split and module rename.
- [x] Clean up the remaining bridge warning.
  - [x] Address the `const` qualifier mismatch around `GoCreateInstance` in `bridge.c` / `bridge.h`.

## Phase 1: Runtime API Cleanup

- [x] Resolve naming inconsistencies.
  - [x] Eliminate duplicate constant spellings and casing variants by making `ResultOK` canonical and keeping aliases compatibility-only.
  - [x] Standardize normalized/plain parameter naming semantics by adding explicit `GetNormalized`/`SetNormalized` and `GetPlain`/`SetPlain` accessors.
  - [x] Standardize canonical bus configuration entry points by adding primary constructors such as `Stereo`, `Mono`, `EffectStereo`, `Generator`, and `MIDIEffect`.
  - Residual, tracked in Phase 4: the four `Parameter` alias pairs still carry no
    `// Deprecated:` marker, and the runtime itself still calls the alias form in
    `process/context.go` and `param/smoother.go`.
- [x] Clarify interface hierarchy.
  - [x] Document the relationships between plugin, processor, and optional stateful processor behavior in code comments.
  - [x] Keep optional interfaces explicit rather than implicit.
- [x] Tighten parameter and registry APIs.
  - [x] Audit and clean up the obvious unsafe parameter access patterns in retained runtime code.
  - [x] Prefer checked access where nil is possible by adding safe registry helpers such as `GetOK`, `Has`, `GetNormalized`, and `GetPlain`.
  - [x] Clarify duplicate-registration behavior and failure modes by making `Registry.Add` fail explicitly on duplicate IDs.
- [x] Tighten error handling.
  - [x] Replace silent failures where normal error returns are appropriate in the state/registry layer.
  - [x] Wrap errors with useful context in state save/load and core runtime setup paths.
  - Sentinels now exist in `param/errors.go` and `state/manager.go`
    (`ErrDuplicateID`, `ErrDuplicateName`, `ErrNilRegistry`, `ErrInvalidFormat`,
    `ErrUnsupportedVersion`, `ErrNoCustomLoader`), and save/load wraps every
    failure with the operation and the parameter it was on.

## Phase 2: Runtime Architecture Hardening

- [x] Reassess package boundaries.
  - [x] Keep VST3-facing code, bridge glue, and generic runtime primitives clearly separated.
  - [x] Remove mixed-responsibility code where low-level and higher-level concerns are entangled.
- [x] Review lifecycle behavior.
  - [x] Audit `SetActive(false)` and related reset behavior in retained runtime code.
  - [x] Ensure state, parameter, and event handling reset deterministically in retained runtime paths.
- [x] Review process-layer structure.
  - [x] Refactor oversized runtime processing functions where readability is poor.
  - [x] Split transport updates, buffer mapping, and automation collection into smaller runtime helpers.
- [x] Keep helper abstractions honest.
  - [x] Only add base helpers if they reduce boilerplate without hiding core VST3 mechanics.

## Phase 3: Testing, Validation, And Performance

Coverage went from 34.3% to 62.1% overall. `golangci-lint` went from 104 issues
to 0, and `go vet ./...` is clean for the first time.

- [x] Expand automated coverage for retained packages.
  - [x] `pkg/framework/param` (58.6% -> 67.5%)
  - [x] `pkg/framework/process` (24.6% -> 94.9%)
  - [x] `pkg/framework/state` (58.3% -> 83.8%)
  - [x] `pkg/midi` (64.8% -> 96.9%)
  - [x] `pkg/plugin` where practical (no test files -> 17.3%; the cgo entry
        points are covered end-to-end by the smoke test and the validator
        instead, neither of which reports Go coverage)
  - [x] `pkg/vst3` (0.0% -> 80.0%), unblocked by the new in-memory `IBStream`
  - [x] `pkg/framework/plugin` (22.2% -> 100%)
- [x] Add race-detector coverage where thread-safety assumptions are non-trivial.
  - [x] `EventQueue` under concurrent writer/reader/mutator.
  - [x] `Smoother.SetTarget` (control thread) against `Next` (audio thread).
  - [x] `SetBusActive` against audio-thread bus reads.
  - [x] `state.Manager.Save` against concurrent parameter registration.
- [x] Keep allocation-sensitive paths measurable.
  - [x] Benchmark process-context internals.
  - [x] Benchmark runtime hot paths where useful. `testing.AllocsPerRun`
        asserts 0 allocations for the `Context` audio path, `getComponent`,
        `processChunk`, `processSampleAccurate` (both with and without MIDI),
        MIDI event construction and range reads, note frequency conversion,
        and the smoother's per-sample advance.
  - [x] Revisit fixed-size internal buffers where limits may be too rigid. The
        128-entry automation buffer now scales with the block size and
        coalesces on overflow instead of silently dropping changes.
  - [x] `midi.Event` is now a concrete tagged struct rather than an interface,
        so events no longer box once each per block. The audio path is
        allocation-free end to end; `TestProcessSampleAccurateWithEventsDoesNotAllocate`
        pins it with a block carrying 32 MIDI events.
        `process.Context.ProcessEvents` also collects into a retained scratch
        instead of allocating a snapshot.
- [x] Keep validation expectations explicit.
  - [x] Define minimal validation expectations for the runtime layer itself.
        `test/smoke` builds a real `c-shared` module and drives it from C
        (`go test -tags smoke ./test/smoke/...`), covering the factory
        lifecycle, IID dispatch, refcount release/re-acquire and a multi-bus
        `process()` call. Run in CI.
  - [x] Steinberg's `validator` runs in CI on top of the smoke test
        (`go test -tags validator ./test/smoke/...`): 47 conformance tests, 0
        failures. The validator is built from a pinned SDK commit and cached;
        the test skips when no validator is available, so contributors without
        the C++ SDK are not blocked.
  - [x] `docs/validation.md` records what this repo validates and what
        downstream repos such as `synthkit` must validate on their own:
        the validator against their *own* bus/parameter topology, allocation
        guards on their DSP, DSP correctness, custom preset-payload
        compatibility, and real-host testing.
- [x] Add CI (`.github/workflows/ci.yml`): build/vet/`-race` on Linux and
      macOS, golangci-lint, gofmt, a `-Werror` C bridge compile, the smoke
      test, and the Steinberg validator.

## Phase 4: Documentation And Consumer Experience

- [x] Document retained scope clearly.
  - [x] State the split boundary in README and PLAN.
  - [x] The README's "Scope" and "Companion Repo" sections list what belongs
        here against what `synthkit` owns.
- [x] Improve API documentation.
  - [x] package-level docs — `pkg/vst3/doc.go` now covers the binding layer,
        its untrusted-input caps, and the cgo constraint on tests.
  - [x] lifecycle expectations — `pkg/plugin`'s package doc tabulates the host
        call order against the hooks it invokes, and calls out the two traps:
        `Initialize` runs twice (the first time with a placeholder 48 kHz rate,
        the real one arriving with `SetupProcessing`), and there is no
        `Terminate` hook, so `SetActive(false)` is where teardown belongs. Which
        hook may allocate is stated per method.
  - [x] thread-safety guarantees — `pkg/plugin`, `pkg/framework/param`,
        `pkg/framework/process`, `pkg/framework/bus`, `pkg/midi` and `pkg/vst3`
        each have a Threading section splitting their surface into
        audio-thread-safe, control-thread-only, and both. Notable correction
        captured while writing it: `param.Registry` *is* mutex-guarded, so it is
        memory-safe but still wrong for the audio thread (a lookup can block on
        the control thread, and `All` allocates) — resolve `*Parameter` pointers
        during setup and hold them.
  - [x] state/persistence expectations — `pkg/framework/state` documents the v2
        wire format field by field, the v1/v2 compatibility direction, why
        values are stored normalized (so ranges can change without invalidating
        presets) and why parameter IDs must therefore be stable, and what a
        custom payload has to guarantee on its own.
- [x] Add migration guidance for the fork.
  - [x] Clarify intentional differences from the original project.
  - [x] Clarify the new module path.
  - [x] Document how downstream repos should consume this fork once versions are
        tagged — `docs/migration.md`, plus a README section.
  - [ ] **Open problem found while writing that:** a tagged release is not
        `go get`-able. `include/vst3` is a git submodule, submodule content is
        absent from what the module proxy serves, and every cgo package then
        fails on a missing `vst3_c_api.h` (verified against `git archive HEAD`).
        `CGO_CFLAGS` cannot paper over it: the header is included by relative
        path from several directory depths, so no single `-I` satisfies all of
        them. Documented as a `replace`-directive workaround for now. Deciding
        the real fix — vendor the header and accept the licensing implication,
        generate a shim header at build time, or accept checkout-only
        consumption — belongs to Phase 6 release scope.
- [x] Add a `LICENSE` file and state the Steinberg dual-license implication for
      distributed binaries.
- [x] Write the breaking-change notes for downstream consumers. All three are
      now in `docs/migration.md`, with the migration table for each:
  - [x] Preset format: `state` now writes v2 (length-prefixed custom payload).
        v1 presets still load; v2 cannot be read by older builds.
  - [x] `midi.Event`: the per-message types (`NoteOnEvent`, `ControlChangeEvent`,
        …) and `BaseEvent` are gone, replaced by one `midi.Event` struct with
        constructors (`NewNoteOn`, …) and type-guarded accessors (`Note`,
        `Velocity`, `Controller`, …). `EventQueue.OffsetEvents` no longer returns
        an error. Code that type-switched on events switches on `Event.Type`.
        The notes flag that the accessors return zero rather than panicking on a
        mismatched type, so a missed case in a rewritten type switch is silent.
  - [x] `BypassParameter` now sets `IsBypass`, and VST3 permits only one such
        parameter per plugin. `RegisterCompressorControls` ("Auto Gain") and
        `RegisterEQBand` ("Band N Enable") were switched to plain `Choice`.
- [x] Mark the deprecated `Parameter` aliases. `GetValue`/`SetValue`/
      `GetPlainValue`/`SetPlainValue` now carry `// Deprecated:` markers, the two
      internal callers (`process/context.go`, `param/smoother.go`) use the
      canonical forms, and the tests moved with them — the aliases keep a
      dedicated `TestDeprecatedAliases` that pins them to their replacements.
      Closes the residual noted under Phase 1.
- [x] Keep build and validation instructions accurate for the retained runtime
      repo. The README's Development section now covers `just` targets, both
      tagged test suites, and links `docs/validation.md`.

## Phase 5: Cross-Platform Windows Support

- [ ] Update the build system for Windows.
  - [ ] Detect Windows cleanly.
  - [ ] Produce the correct library format and bundle layout.
- [ ] Extend the bridge for Windows entry points.
  - [ ] Add required DLL entry handling.
  - [ ] Export `GetPluginFactory`.
- [ ] Review platform-specific CGO directives.
  - [ ] Add Windows-specific flags only where necessary.
  - [ ] Minimize platform divergence.
- [ ] Validate on real toolchains and hosts.
  - [ ] Start with MinGW-w64.
  - [ ] Keep MSVC as a later enhancement if justified.
- [ ] Capture Windows-specific risks.
  - [ ] Shared-library behavior with Go.
  - [ ] Scheduler differences.
  - [ ] File locking and reload friction.
  - [ ] Path-length edge cases.

## Phase 6: Release Readiness

- [ ] Decide who drives parameter smoothing. `Parameter.EnableSmoothing` attaches
      a smoother and `SetNormalized` retargets it, but nothing in
      `pkg/framework/process` or `pkg/plugin` calls `Advance()`, so smoothing is
      inaudible unless the user's DSP advances it per sample. Either the runtime
      advances registered smoothers inside the process loop, or the contract is
      documented as caller-driven — but shipping it ambiguous is worse than
      either.
- [ ] Keep architectural guardrails enforced.
- [ ] Keep retained runtime packages tested and documented.
- [ ] Confirm the public API boundary is stable enough for downstream repos.
- [ ] Decide supported release scope explicitly.
  - [ ] Linux/macOS only, or include Windows.
  - [ ] Runtime-only, with higher-level features delegated to companion repos.

## Ongoing Rules

- [ ] If work belongs in `synthkit`, remove it from this file instead of duplicating it here.
- [ ] Avoid standalone roadmap documents; merge planning back here.
- [ ] Before adding a new abstraction, check whether it belongs in `vst3go` or in a higher-level companion repo.
