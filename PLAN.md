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
        the validator against their _own_ bus/parameter topology, allocation
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
        captured while writing it: `param.Registry` _is_ mutex-guarded, so it is
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
  - [x] **Open problem found while writing that:** a tagged release is not
        `go get`-able. `include/vst3` is a git submodule, submodule content is
        absent from what the module proxy serves, and every cgo package then
        fails on a missing `vst3_c_api.h` (verified against `git archive HEAD`).
        `CGO_CFLAGS` cannot paper over it: the header is included by relative
        path from several directory depths, so no single `-I` satisfies all of
        them. Documented as a `replace`-directive workaround for now. Deciding
        the real fix — vendor the header and accept the licensing implication,
        generate a shim header at build time, or accept checkout-only
        consumption — belongs to Phase 8 release scope.

        Decided: vendor it. The header is now three regular files in
            `include/vst3`, and the same `git archive` check that found the problem
            confirms the fix — a source-only tree compiles every cgo package. The
            licensing implication is accepted explicitly rather than in passing:
            redistribution is permitted only by Steinberg's GPLv3 alternative, since
            the proprietary license forbids distributing the SDK in parts without a
            signed agreement, so the copy this repository hands out is GPLv3.
            `LICENSE`'s third-party notice says so, and says what it does not change
            — a plug-in built against these headers was always a combined work, on
            exactly these terms. Taking the GPLv3 route also means shipping the
            GPLv3: section 4 requires whoever conveys the work to give recipients a
            copy of the License, and a URL is not a copy, so the full text is in
            `COPYING`. The shim-header option was not taken: hand-writing
            the declarations avoids redistribution but makes an ABI mistake a silent
            memory-layout bug instead of a compile error, which is a poor trade for
            a runtime whose whole job is matching a host's vtable layout.

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

The headline finding: `GetPluginFactory` was never exported from the Windows
DLL, and nothing in the build said so. For a `c-shared` build the Go linker
generates a `.def` file whose `EXPORTS` section is built solely from `//export`
symbols and passes it to `ld`, which then exports exactly that set — a C-side
`__declspec(dllexport)` is dropped without a diagnostic. The module compiled,
linked and loaded, and every host would have rejected it as invalid.

- [x] Update the build system for Windows.
  - [x] Detect Windows cleanly.
  - [x] Produce the correct library format and bundle layout. `pkg/bundle` now
        owns the bundle layout for all three platforms — including the detail
        that the Windows binary keeps the `.vst3` extension rather than `.dll`,
        and the `x86_64-win`/`arm64-win` subdirectory names. The validator test
        consumes it instead of carrying its own copy of the table, so the two
        cannot drift.
  - [x] `just build-windows`, `just test-smoke` and `just test-validator`
        recipes; the first cross-builds with MinGW-w64.
- [x] Extend the bridge for Windows entry points.
  - [x] Add required DLL entry handling. `InitDll`/`ExitDll` on Windows,
        `bundleEntry`/`bundleExit` on macOS (which were missing entirely — only
        Linux had an entry pair), `ModuleEntry`/`ModuleExit` on Linux.
  - [x] Export `GetPluginFactory` — see above. It and the entry pairs are now
        Go `//export` functions in `pkg/plugin/cbridge`, because that is the
        only export mechanism that reaches the export table on all three
        platforms.
- [x] Review platform-specific CGO directives.
  - [x] Add Windows-specific flags only where necessary. None were: the DLL
        imports only `KERNEL32.dll` and `msvcrt.dll`, so no `-static-libgcc`
        style flags are needed. A test pins that, since picking up
        `libwinpthread-1.dll` would only fail on a user's machine.
  - [x] Minimize platform divergence. Net negative: moving the entry points to
        Go removed every `#ifdef` from `bridge/bridge.c`, and the platform
        split is now three small build-tagged Go files.
- [x] Validate on real toolchains and hosts.
  - [x] Start with MinGW-w64. The cross-built `windows/amd64` module was run
        end to end under Wine against the existing C driver: factory lifecycle,
        IID dispatch, refcount release/re-acquire and a multi-bus `process()`
        call, 20/20 assertions.
  - [x] Keep MSVC as a later enhancement if justified. Not justified: cgo
        cannot drive MSVC, and nothing here needs it. Recorded as a
        non-goal in `docs/windows.md`.
  - [x] `test/smoke/exports_test.go` reads the real export table out of the
        built module (PE, ELF and Mach-O) and asserts the host-visible entry
        points are present. The Windows half is a cross-build, so it runs from
        the Linux CI runner and does not depend on a Windows host.
  - [x] CI: `windows-latest` added to the build matrix and to the smoke-test
        job; the C bridge is now also compiled `-Werror` with MinGW-w64,
        because `SMTG_STDMETHODCALLTYPE` is `__stdcall` there and empty
        elsewhere.
  - Not covered: Steinberg's `validator` on Windows (Linux only), any real DAW
    on Windows, and `windows/arm64`, which has a bundle layout but no CI build.
    `windows/386` is refused outright — the SDK's `PLUGIN_API` is `__stdcall`
    there and cgo emits `cdecl` for `//export`.
- [x] Capture Windows-specific risks. All in `docs/windows.md`.
  - [x] Shared-library behavior with Go: the runtime has no teardown path, so
        `FreeLibrary` does not restore a clean process and hosts that unload
        and reload during a scan can crash; each loaded plug-in carries its own
        runtime, GC and threads; the runtime installs a process-wide vectored
        exception handler that also sees the host's and other plug-ins'
        exceptions.
  - [x] Scheduler differences: coarser default timer resolution, and Go threads
        do not register with MMCSS, so the audio callback does not get the
        priority class a native audio thread would ask for.
  - [x] File locking and reload friction: Windows locks a loaded DLL, so the
        development loop is build → close host → install → reopen, not
        build-over-the-top.
  - [x] Path-length edge cases: `MAX_PATH` is 260 and the bundle layout is deep.

## Phase 6: Runtime prerequisites for the pcjv.de ports

The product catalog and per-family Delphi references live in
[`vst3go-plugins`](../vst3go-plugins/CATALOG.md). Keep product implementations
there and reusable DSP in the sibling `algo-*` libraries.

### 6.0 Multiple classes per bundle

- [x] Extend the single-plugin factory in `pkg/plugin/wrapper.go` to enumerate
      classes in a stable order and construct the processor for the requested CID.
      Preserve single-class registration. Test unknown IDs, multiple live instances,
      isolation of state and parameters, and discovery of every class in a bundle.

### 6.1 Native editor and host automation

- [ ] Implement the controller's `createView` and the `IPlugView` lifecycle
      for supported native host windows. Exercise attach, remove, reopen, resize,
      focus, keyboard input and display scaling in real hosts. Keep the generic
      host editor available for originals without a custom view.
  - [x] Implement the optional editor bridge and cover lifecycle, input,
        scaling and generic-editor fallback in the C host fixture.
  - [x] Exercise native X11 attach, remove, reopen, resize and 2× scaling in
        REAPER 7.86.
  - [ ] Exercise focus and keyboard in a real host and run the native Windows
        and macOS editor checks.
- [x] Wire editor gestures through the retained component handler
      (begin, perform, end). Send host automation and preset changes back to the
      editor. Deliver meters and plots without locking or allocating on the audio
      thread. Exercise two open instances and close a view during playback.
- [ ] Keep skins, fonts, knob strips, control layout and product-specific
      drawing in the companion repo. Validate its editor with a real port.

### 6.2 Processing and host contracts

- [x] Exercise existing bus negotiation with optional sidechains, mono,
      stereo, quad and 5.1 arrangements. Verify routing with channel-isolated
      impulses, sidechain disconnect/reconnect and inactive buses.
- [ ] Verify latency and tail reporting for lookahead, FIR, convolution and
      feedback. Support host notification when latency changes, and check delay
      compensation across reconfiguration and preset restore.
  - [x] Exercise retained-handler latency notifications from the editor and
        on processor-state restore in the C host fixture.
  - [ ] Measure reported latency, tails and host compensation with the ports.
- [x] Check sample-offset MIDI and automation across block boundaries, bypass
      and transport reset.
  - [x] Resolve the smoothing contract before a port relies on it. The DSP
        caller advances each smoother once per sample frame.
- [ ] Test the Sidechain Compressor with a loaded detector plugin in the
      original nested processing path, including reconfiguration and saved state.

### 6.3 Programs, state and host validation

- [ ] Support factory program enumeration and selection where the Delphi
      product supplies programs. Preserve parameter IDs, units, display curves and
      defaults. Round-trip processor and editor state in a saved host session.
  - [x] Verify root-unit program lists, selection, sample offsets and separate
        processor and editor state through the runtime and C host fixture.
  - [ ] Save and reopen factory programs and editor-specific state in a real
        host session with a port.
- [ ] Validate the affected runtime interfaces using the C host smoke tests
      and Steinberg validator. Repeat in a real host with a family bundle,
      an instrument, a sidechain and a native editor before declaring coverage.
  - [x] Run the C host suite and Steinberg Validator 3.7.2 on Linux. In REAPER
        7.86, load a three-class test bundle with a sidechain, instrument and
        X11 editor, reopen its session, and compare the sidechain impulse render.
  - [ ] Repeat with the published port fixtures on supported host platforms.

## Phase 7: Faithful pcjv.de VST3 ports

Use the [product checklist](../vst3go-plugins/CATALOG.md) for the full
inventory, editions and outstanding identification work. Each family owns its
parameters, programs, original editor references and audio comparisons in
`../vst3go-plugins/plugins/<category>/<product>/`.

### 7.1 Filters and equalizers

- [ ] Port EQs, pass filters, crossovers and linear-phase variants. Compare
      impulse responses, magnitude, phase, channel summation and reported latency.
      Check response plots against the actual output at the same control settings.

#### Equalizers

- [ ] [EeeekU](../vst3go-plugins/plugins/eq/eeeeku/README.md)
  - [x] Implement stereo RBJ and factory programs. The companion pins the
        tested allpass resampler from
        [algo-dsp PR #43](https://github.com/CWBudde/algo-dsp/pull/43).
  - [ ] Reconcile the mono RBJ output with the installer release.
  - [ ] Compare renders from the three original release binaries and verify
        the editor, including its channel-zero Orfanidis mode.
- [ ] [Electri-Q](../vst3go-plugins/plugins/eq/electri-q/README.md)
  - [ ] Commercial edition
  - [ ] Posihfopit edition
- [ ] [Graphic EQ](../vst3go-plugins/plugins/eq/graphic-eq/README.md)
- [ ] [Noname EQ](../vst3go-plugins/plugins/eq/noname-eq/README.md)
  - [ ] Original
  - [ ] Split and oversampled edition
- [ ] [Parametri-Q (lite)](../vst3go-plugins/plugins/eq/parametri-q-lite/README.md)
- [ ] [ParametricEQ](../vst3go-plugins/plugins/eq/parametriceq/README.md)
- [ ] [Posihfopit](../vst3go-plugins/plugins/eq/posihfopit/README.md)
  - [ ] Original Posihfopit
  - [ ] Armyfopit
  - [ ] Smallifopit 4-band mono
  - [ ] Smallifopit 4-band stereo
  - [ ] Smallifopit 8-band mono
  - [ ] Smallifopit 8-band stereo
- [ ] [Pushtec 5+1A](../vst3go-plugins/plugins/eq/pushtec-5-1a/README.md) — confirm contributor scope before porting.
- [ ] [Soul EQ](../vst3go-plugins/plugins/eq/soul-eq/README.md)
- [ ] [Tiny-Q](../vst3go-plugins/plugins/eq/tiny-q/README.md)
- [ ] [UNIQUE EQ](../vst3go-plugins/plugins/eq/unique-eq/README.md)
- [ ] [Valuable EQ](../vst3go-plugins/plugins/eq/valuable-eq/README.md)
  - [ ] Valueable
  - [ ] Valueable Stereo

#### Filters and crossovers

- [ ] [Bugpass](../vst3go-plugins/plugins/filter/bugpass/README.md)
- [ ] [Bugpass Lite](../vst3go-plugins/plugins/filter/bugpass-lite/README.md)
- [ ] [Butterworth Filters](../vst3go-plugins/plugins/filter/butterworth-filters/README.md)
  - [ ] High-pass mono and stereo
  - [ ] Low-pass mono and stereo
  - [ ] Frequency splitter
  - [ ] Butterworth²HP oversampled edition
- [ ] [Chebyshev Filters](../vst3go-plugins/plugins/filter/chebyshev-filters/README.md)
  - [ ] Type I high-pass and low-pass, including mono editions
  - [ ] Type II high-pass
  - [ ] Type II low-pass (source present, download contains only the license)
- [ ] [Dual Linkwitz-Riley Filters](../vst3go-plugins/plugins/filter/dual-linkwitz-riley-filters/README.md)
- [ ] [Hum Removal](../vst3go-plugins/plugins/filter/hum-removal/README.md)
- [ ] [Lavaland](../vst3go-plugins/plugins/filter/lavaland/README.md)
- [ ] [Linear Phase Butterworth](../vst3go-plugins/plugins/filter/linear-phase-butterworth/README.md)
  - [ ] Linear Phase Butterworth HP
  - [ ] Linear Phase Butterworth LP
- [ ] [Linear Phase Frequency Crossover](../vst3go-plugins/plugins/filter/linear-phase-frequency-crossover/README.md)
- [ ] [MoogVST](../vst3go-plugins/plugins/filter/moogvst/README.md)
- [ ] [Phase Rotator](../vst3go-plugins/plugins/filter/phase-rotator/README.md)
- [ ] [QR-Filter](../vst3go-plugins/plugins/filter/qr-filter/README.md)
- [ ] [Rubberfilter](../vst3go-plugins/plugins/filter/rubberfilter/README.md)
- [ ] [Simple Linear Phase Filter](../vst3go-plugins/plugins/filter/simple-linear-phase-filter/README.md)
  - [ ] Linear Phase LP
  - [ ] Linear Phase HP
  - [ ] LinearPhaseFilterFFT edition
- [ ] [Simple Linear Phase Linkwitz-Riley Filter](../vst3go-plugins/plugins/filter/simple-linear-phase-linkwitz-riley-filter/README.md)
  - [ ] Linear Phase Linkwitz-Riley HP
  - [ ] Linear Phase Linkwitz-Riley LP
- [ ] [Simple Linkwitz-Riley Crossover](../vst3go-plugins/plugins/filter/simple-linkwitz-riley-crossover/README.md)
  - [ ] LinkwitzRiley
  - [ ] LinkwitzRileyStereo
- [ ] [SimpleFilter](../vst3go-plugins/plugins/filter/simplefilter/README.md)
- [ ] [Weighting Filter](../vst3go-plugins/plugins/filter/weighting-filter/README.md)

### 7.2 Dynamics, bass and distortion

- [ ] Port dynamics processors, waveshapers and bass effects. Measure static
      curves, attack/release, lookahead, sidechain routing, harmonics and aliasing.
      Match intentional saturation and original oversampling modes.

#### Bass

- [ ] [Bass Baron](../vst3go-plugins/plugins/bass/bass-baron/README.md)
- [ ] [Bass Extender](../vst3go-plugins/plugins/bass/bass-extender/README.md)
- [ ] [SubBoost](../vst3go-plugins/plugins/bass/subboost/README.md)

#### Dynamics

- [ ] [Adhesive](../vst3go-plugins/plugins/dynamics/adhesive/README.md)
- [ ] [Audiocation Compressor](../vst3go-plugins/plugins/dynamics/audiocation-compressor/README.md)
  - [ ] Mono compressor
  - [ ] Stereo compressor
- [ ] [EnhancedGate](../vst3go-plugins/plugins/dynamics/enhancedgate/README.md)
- [ ] [Hard Knee Compressor](../vst3go-plugins/plugins/dynamics/hard-knee-compressor/README.md)
- [ ] [LA4029](../vst3go-plugins/plugins/dynamics/la4029/README.md)
- [ ] [Lightweight Dynamics](../vst3go-plugins/plugins/dynamics/lightweight-dynamics/README.md)
- [ ] [Lightweight Feedback Compressor](../vst3go-plugins/plugins/dynamics/lightweight-feedback-compressor/README.md)
- [ ] [Lightweight Feedback Limiter](../vst3go-plugins/plugins/dynamics/lightweight-feedback-limiter/README.md)
- [ ] [Lightweight Multiband Compressor](../vst3go-plugins/plugins/dynamics/lightweight-multiband-compressor/README.md)
- [ ] [Lightweight Soft Knee Compressor](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-compressor/README.md)
- [ ] [Lightweight Soft Knee Gate](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-gate/README.md)
- [ ] [Lightweight Soft Knee Limiter](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-limiter/README.md)
- [ ] [Lightweight Upward Compressor](../vst3go-plugins/plugins/dynamics/lightweight-upward-compressor/README.md)
- [ ] [Limiter](../vst3go-plugins/plugins/dynamics/limiter/README.md)
- [ ] [Lookahead Limiter](../vst3go-plugins/plugins/dynamics/lookahead-limiter/README.md)
- [ ] [Multiband Compressor](../vst3go-plugins/plugins/dynamics/multiband-compressor/README.md)
- [ ] [Sidechain Compressor](../vst3go-plugins/plugins/dynamics/sidechain-compressor/README.md)
- [ ] [Simple Compressor](../vst3go-plugins/plugins/dynamics/simple-compressor/README.md)
- [ ] [Simple Feedback Compressor](../vst3go-plugins/plugins/dynamics/simple-feedback-compressor/README.md)
- [ ] [Simple Gate](../vst3go-plugins/plugins/dynamics/simple-gate/README.md)
- [ ] [Simple Limiter](../vst3go-plugins/plugins/dynamics/simple-limiter/README.md)
- [ ] [Smooth Multiband Compressor](../vst3go-plugins/plugins/dynamics/smooth-multiband-compressor/README.md)
- [ ] [Soft Knee Feedback Compressor](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-compressor/README.md)
- [ ] [Soft Knee Feedback Limiter](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-limiter/README.md)
  - [ ] Feedback build (SKFBLimiter)
  - [ ] Soft Knee Limiter build (SKLDM)
- [ ] [Transient Bug](../vst3go-plugins/plugins/dynamics/transient-bug/README.md)
  - [ ] BugWars release
  - [ ] TrBug release

#### Distortion

- [ ] [2-Band Distortion](../vst3go-plugins/plugins/distortion/2-band-distortion/README.md)
- [ ] [Advanced Clipper](../vst3go-plugins/plugins/distortion/advanced-clipper/README.md)
- [ ] [Amp Sim](../vst3go-plugins/plugins/distortion/amp-sim/README.md)
- [ ] [Analogue Mojo](../vst3go-plugins/plugins/distortion/analogue-mojo/README.md)
- [ ] [Chebyshev Waveshaper](../vst3go-plugins/plugins/distortion/chebyshev-waveshaper/README.md)
- [ ] [Christortion](../vst3go-plugins/plugins/distortion/christortion/README.md)
- [ ] [Decimator](../vst3go-plugins/plugins/distortion/decimator/README.md)
- [ ] [Dynamite](../vst3go-plugins/plugins/distortion/dynamite/README.md)
- [ ] [Exciter](../vst3go-plugins/plugins/distortion/exciter/README.md)
- [ ] [Hysteresis](../vst3go-plugins/plugins/distortion/hysteresis/README.md)
- [ ] [OpAmp](../vst3go-plugins/plugins/distortion/opamp/README.md)
- [ ] [Oversampled Tanh](../vst3go-plugins/plugins/distortion/oversampled-tanh/README.md)
- [ ] [Scary](../vst3go-plugins/plugins/distortion/scary/README.md)
- [ ] [TanhWaveshaper](../vst3go-plugins/plugins/distortion/tanhwaveshaper/README.md)
- [ ] [Toadstunnel](../vst3go-plugins/plugins/distortion/toadstunnel/README.md)
  - [ ] Toadstunnel
  - [ ] Toadstunnel²
- [ ] [Tubster](../vst3go-plugins/plugins/distortion/tubster/README.md)

### 7.3 Modulation, delay, pitch, spectral processing and reverb

- [ ] Compare modulation speed/depth and channel phase, delay taps and
      feedback, pitch ratio and transients, noise profiles, impulse responses
      and reverb decay. Use envelope or spectral comparisons for random processes.

#### Modulation

- [ ] [Barberpole Flanger](../vst3go-plugins/plugins/modulation/barberpole-flanger/README.md)
- [ ] [Flanger](../vst3go-plugins/plugins/modulation/flanger/README.md)
- [ ] [ITA Phaser](../vst3go-plugins/plugins/modulation/ita-phaser/README.md)
- [ ] [Phase-O-Matic](../vst3go-plugins/plugins/modulation/phase-o-matic/README.md)
- [ ] [Ring Modulator](../vst3go-plugins/plugins/modulation/ring-modulator/README.md)
  - [ ] Mono edition
  - [ ] Stereo edition
  - [ ] Quad edition
- [ ] [Simple Chorus](../vst3go-plugins/plugins/modulation/simple-chorus/README.md)
- [ ] [Spin Bug Lite](../vst3go-plugins/plugins/modulation/spin-bug-lite/README.md)
- [ ] [StkChorus](../vst3go-plugins/plugins/modulation/stkchorus/README.md)
- [ ] [Vibrato](../vst3go-plugins/plugins/modulation/vibrato/README.md)

#### Delay

- [ ] [DegradingDelay](../vst3go-plugins/plugins/delay/degradingdelay/README.md)
- [ ] [FeedBUG](../vst3go-plugins/plugins/delay/feedbug/README.md)
- [ ] [ModDelay](../vst3go-plugins/plugins/delay/moddelay/README.md)
- [ ] [Party-Delay](../vst3go-plugins/plugins/delay/party-delay/README.md)
- [ ] [Simple Sample Delay](../vst3go-plugins/plugins/delay/simple-sample-delay/README.md)
- [ ] [StkEcho](../vst3go-plugins/plugins/delay/stkecho/README.md)
- [ ] [VariableDelay](../vst3go-plugins/plugins/delay/variabledelay/README.md)

#### Pitch

- [ ] [Bode Frequency Shifter](../vst3go-plugins/plugins/pitch/bode-frequency-shifter/README.md)
  - [ ] Mono edition
  - [ ] Stereo edition
- [ ] [Granular Pitch Shifter](../vst3go-plugins/plugins/pitch/granular-pitch-shifter/README.md)
- [ ] [SoundTouch based Pitch Shifter](../vst3go-plugins/plugins/pitch/soundtouch-based-pitch-shifter/README.md)
- [ ] [Split Harmonizer](../vst3go-plugins/plugins/pitch/split-harmonizer/README.md)
- [ ] [StkPitchShift](../vst3go-plugins/plugins/pitch/stkpitchshift/README.md)

#### Reverb

- [ ] [ConvoFX](../vst3go-plugins/plugins/reverb/convofx/README.md)
- [ ] [ConvoFXLite](../vst3go-plugins/plugins/reverb/convofxlite/README.md)
- [ ] [Dynamic Convolution Plugin](../vst3go-plugins/plugins/reverb/dynamic-convolution-plugin/README.md) — resolve the discontinued release and reference build.
- [ ] [Freeverb](../vst3go-plugins/plugins/reverb/freeverb/README.md)
- [ ] [Plate Reverb](../vst3go-plugins/plugins/reverb/plate-reverb/README.md)
- [ ] [StkReverb](../vst3go-plugins/plugins/reverb/stkreverb/README.md)

#### Spectral processing

- [ ] [Noise Reduction](../vst3go-plugins/plugins/spectral/noise-reduction/README.md)
- [ ] [Spectral Gate](../vst3go-plugins/plugins/spectral/spectral-gate/README.md)

### 7.4 Spatial processing, instruments and MIDI

- [ ] Check channel order, matrix coefficients, HRTF selection and IRs.
      Reproduce instrument note handling, velocity, voice allocation, sample
      loading, tuning and envelopes. Check MIDI-generating effects at block edges.

#### Spatial processing

- [ ] [Digital Crosstalk Cancellator (DCC)](../vst3go-plugins/plugins/spatial/digital-crosstalk-cancellator-dcc/README.md)
- [ ] [DualWideBug](../vst3go-plugins/plugins/spatial/dualwidebug/README.md)
- [ ] [Headphone Mix](../vst3go-plugins/plugins/spatial/headphone-mix/README.md)
- [ ] [HRTF 3D](../vst3go-plugins/plugins/spatial/hrtf-3d/README.md)
- [ ] [PhaseBug](../vst3go-plugins/plugins/spatial/phasebug/README.md)
  - [ ] PhaseBug
  - [ ] PhaseBug Mono
- [ ] [Quadrophonics](../vst3go-plugins/plugins/spatial/quadrophonics/README.md)
  - [ ] SQ encoder
  - [ ] SQ decoder
  - [ ] QS encoder
  - [ ] QS decoder
- [ ] [Quadropolis](../vst3go-plugins/plugins/spatial/quadropolis/README.md)
- [ ] [Simple Crosstalk Cancellation](../vst3go-plugins/plugins/spatial/simple-crosstalk-cancellation/README.md)
- [ ] [Spinbug](../vst3go-plugins/plugins/spatial/spinbug/README.md)
- [ ] [Surround to Stereo](../vst3go-plugins/plugins/spatial/surround-to-stereo/README.md)
- [ ] [Tina-Turner](../vst3go-plugins/plugins/spatial/tina-turner/README.md)

#### Instruments and vocoders

- [ ] [2-Oszillator Synth](../vst3go-plugins/plugins/synth/2-oszillator-synth/README.md)
- [ ] [FunKEYS](../vst3go-plugins/plugins/synth/funkeys/README.md)
- [ ] [Simple Sampler](../vst3go-plugins/plugins/synth/simple-sampler/README.md)
- [ ] [Simple Sine Synth](../vst3go-plugins/plugins/synth/simple-sine-synth/README.md)
- [ ] [Vocoder](../vst3go-plugins/plugins/synth/vocoder/README.md)

### 7.5 Analysis, utilities, educational, hosting and other plugins

- [ ] Validate meter scales, detector timing and readouts against known
      signals. Exercise actual host/external I/O, scripts, GUIs and interactive
      behavior as applicable. Record host or hardware dependencies per product.

#### Analysis

- [ ] [Barberpole Tuner](../vst3go-plugins/plugins/analysis/barberpole-tuner/README.md)
- [ ] [BetaMeta](../vst3go-plugins/plugins/analysis/betameta/README.md)
- [ ] [StereoMeterVST](../vst3go-plugins/plugins/analysis/stereometervst/README.md)
- [ ] [Third-Octave Analyser (VST-Plugin)](../vst3go-plugins/plugins/analysis/third-octave-analyser-vst-plugin/README.md)
- [ ] [TunaBUG](../vst3go-plugins/plugins/analysis/tunabug/README.md)

#### Utilities and MIDI

- [ ] [Audio2Midi Trigger](../vst3go-plugins/plugins/utility/audio2midi-trigger/README.md)
- [ ] [Dither & Noiseshaper](../vst3go-plugins/plugins/utility/dither-noiseshaper/README.md)
- [ ] [PhaseAdjustment](../vst3go-plugins/plugins/utility/phaseadjustment/README.md)
- [ ] [VST Host Opcode Logger](../vst3go-plugins/plugins/utility/vst-host-opcode-logger/README.md)
- [ ] [VST Pascal Script](../vst3go-plugins/plugins/utility/vst-pascal-script/README.md)

#### Hosting

- [ ] [VST interfaced ASIO-Host](../vst3go-plugins/plugins/hosting/vst-interfaced-asio-host/README.md)

#### Other effects

- [ ] [Ather](../vst3go-plugins/plugins/misc/ather/README.md)
- [ ] [Audio Amelioration](../vst3go-plugins/plugins/misc/audio-amelioration/README.md)

#### Educational processors

- [ ] [ChunkDemo](../vst3go-plugins/plugins/education/chunkdemo/README.md)
- [ ] [Compander](../vst3go-plugins/plugins/education/compander/README.md)
- [ ] [Sigma Delta](../vst3go-plugins/plugins/education/sigma-delta/README.md)

#### Games

- [ ] [Tetris](../vst3go-plugins/plugins/game/tetris/README.md)

### 7.6 Acceptance for every published class

- [ ] Trace the original Delphi project, processor, form resources and
      factory programs for each edition. Compare each `algo-*` candidate against
      the original topology and coefficients. Implement missing reusable DSP
      upstream and verify it against reference vectors.
- [ ] Capture original renders with source version, input, preset, rate, block
      size, routing and latency alignment. Set tolerances per measured behavior.
      Compare null residuals for deterministic paths, and use spectral/time
      measures and listening checks where exact cancellation is inappropriate.
- [ ] Exercise silence, tails, bypass, automation, reload and non-finite
      output checks at multiple supported rates and block sizes. Measure audio
      callback allocations and CPU at the most demanding supported setting.
- [ ] Compare the original and ported editor at matched size and parameter
      state, including meters, interaction, readouts and HiDPI behavior. Record
      validation results for each class and supported host platform.
- [ ] Document any unresolved source identity, original assets, reference
      binaries or differences before marking the corresponding port complete.

## Phase 8: Release Readiness

- [x] Document caller-driven parameter smoothing in `pkg/framework/param`.
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
