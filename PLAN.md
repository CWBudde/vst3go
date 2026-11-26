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
  - [x] Click the EeeekU graph in REAPER on Linux and verify a keyboard
        gesture changes the host-visible frequency from 500 to 520 Hz.
  - [x] Exercise HWND attach, remove, reopen and 1.5× scaling in Win64
        REAPER 7.86 under Wine. Pin the Go DLL through host unload.
  - [ ] Run the native Windows and macOS editor checks, including focus and
        keyboard interaction.
- [x] Wire editor gestures through the retained component handler
      (begin, perform, end). Send host automation and preset changes back to the
      editor. Deliver meters and plots without locking or allocating on the audio
      thread. Exercise two open instances and close a view during playback.
- [x] Keep skins, fonts, knob strips, control layout and product-specific
      drawing in the companion repo. Validate its editor with a real port.

### 6.2 Processing and host contracts

- [x] Exercise existing bus negotiation with optional sidechains, mono,
      stereo, quad and 5.1 arrangements. Verify routing with channel-isolated
      impulses, sidechain disconnect/reconnect and inactive buses.
- [x] Verify latency and tail reporting for lookahead, FIR, convolution and
      feedback. Support host notification when latency changes, and check delay
      compensation across reconfiguration and preset restore.
  - [x] Exercise retained-handler latency notifications from the editor and
        on processor-state restore in the C host fixture.
  - [x] Let processors react to direct controller and editor parameter
        writes on the UI thread. Verify same-value writes and keep
        sample-offset automation on its existing path.
  - [x] Verify 238 direct controller latency restarts with Linear Phase
        Butterworth and live REAPER compensation for both filter classes.
  - [x] Verify sample-exact delay compensation after reopening a delay probe
        at 48 and 96 kHz in REAPER 7.86.
  - [x] Verify the Lookahead Limiter delay and tail. Reopen and render
        16- and 258-sample settings with sample-exact compensation in REAPER.
  - [x] Measure 768-sample latency and 1023-sample tail on Linear Phase HP
        and LP. Check compensation after reopening at 48 and 96 kHz.
  - [x] Verify ConvoFXLite latency and tails and reopen compensated projects
        with 64, 2048 and 65536 samples of delay in REAPER.
  - [x] Verify zero latency and no audio tail in Simple Feedback Compressor.
        Reopen settings and render the impulse at frame zero in REAPER.
- [x] Check sample-offset MIDI and automation across block boundaries, bypass
      and transport reset.
  - [x] Resolve the smoothing contract before a port relies on it. The DSP
        caller advances each smoother once per sample frame.
  - [x] Send notes from the first event output bus to the host. Verify
        sample offsets across automation chunks and inactive bus handling.
  - [x] Expose optional MIDI controller assignments and verify repeated
        mapped parameter values at their sample offsets on Linux and Win64/Wine.
- [x] Test the Sidechain Compressor with a loaded detector plugin in the
      original nested processing path, including reconfiguration and saved state.
  - [x] Forward host processing start and stop independently of activation so
        a nested detector can receive both lifecycle transitions.
  - [x] Forward final component teardown to processors that own nested modules.
  - [x] Compare the original and the port using matching Windows and Linux
        builds of the same VST2 detector. Reopen both Linux instances with
        their saved child state in REAPER.

### 6.3 Programs, state and host validation

- [x] Support factory program enumeration and selection where the Delphi
      product supplies programs. Preserve parameter IDs, units, display curves and
      defaults. Round-trip processor and editor state in a saved host session.
  - [x] Verify root-unit program lists, selection, sample offsets and separate
        processor and editor state through the runtime and C host fixture.
  - [x] Support the original DAV logarithmic normalization for continuous
        controls such as Electri-Q frequency and bandwidth.
  - [x] Save and reopen EeeekU factory programs and edited parameters in
        REAPER 7.86.
  - [x] Reopen the EeeekU session in a fresh REAPER process and verify
        independent selected-band editor state in the two live instances.
  - [x] Preserve editable factory slots in Graphic EQ, Noname EQ, Lookahead
        Limiter and Simple Sample Delay. Switch and restore edits in REAPER.
- [ ] Validate the affected runtime interfaces using the C host smoke tests
      and Steinberg validator. Repeat in a real host with a family bundle,
      an instrument, a sidechain and a native editor before declaring coverage.
  - [x] Run the C host suite and Steinberg Validator 3.7.2 on Linux. In REAPER
        7.86, load a three-class test bundle with a sidechain, instrument and
        X11 editor, reopen its session, and compare the sidechain impulse render.
  - [x] Verify the EeeekU family with native editors and the Sidechain
        Compressor with a native VST2 detector in REAPER on Linux.
  - [x] Verify Simple Sine Synth as an instrument in REAPER on Linux. Check
        its keyboard, voice timing, playback reopen and saved state.
  - [ ] Repeat port validation on the remaining supported platforms.

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
  - [x] Implement stereo and mono RBJ and factory programs. Compare 180
        impulse renders across the three released binaries and verify
        parameter state after reopening a REAPER project.
  - [x] Implement the source-defined Orfanidis mode on channel zero. The
        companion pins the tested revisions from [algo-dsp PR #43](https://github.com/CWBudde/algo-dsp/pull/43)
        and [PR #44](https://github.com/CWBudde/algo-dsp/pull/44).
  - [x] Verify RBJ complex response and harmonics at three rates and three
        input levels. Verify Linux editor controls, 2× scaling and playback
        reopen in REAPER.
  - [x] Reopen both native Windows editors and saved state in Win64 REAPER
        under Wine. Verify keyboard, focus and 2× resize in the C fixture.
  - [ ] Finish native Windows and macOS host checks and compare the released
        Orfanidis response when a reference mode can be selected.
- [ ] [Electri-Q](../vst3go-plugins/plugins/eq/electri-q/README.md)
  - [ ] Commercial edition
    - [ ] Establish exact source correspondence and capture the final 1.8.8
          release with an existing authorized license.
  - [ ] Posihfopit edition
    - [x] Measure all 32 released programs and 64-band state restore.
          Compare 154 reference renders for the verified filter paths.
    - [x] Match all 32 untouched factory programs in 192 additional renders.
          Verify the seven-mode, 19-selector pilot and variable-band state in REAPER.
    - [x] Match 2,106 renders through all nine modes and 39 selectors. Reopen
          a project from the earlier pilot and save the expanded controls.
    - [x] Verify all 80 retained selectors across the nine modes. Match 4,308
          finite captures and keep the 60 dB/oct lowpass finite in the two
          British modes where the original returned NaNs. Reopen both the
          earlier selector state and the newly edited slope in REAPER.
    - [ ] Verify the original editor, authorize factory-bank distribution
          and test additional native host platforms.
- [ ] [Graphic EQ](../vst3go-plugins/plugins/eq/graphic-eq/README.md)
  - [x] Compare all 22 band controls and channel isolation across 207 audible
        release captures. Reopen the stereo program and edits in REAPER.
  - [ ] Verify the custom editor, older program import and remaining hosts.
- [ ] [Noname EQ](../vst3go-plugins/plugins/eq/noname-eq/README.md)
  - [ ] Original
    - [x] Compare all eight filter types and both rate setup orders in 153
          float32 stereo renders. Reopen an edited Init program in REAPER.
    - [ ] Check the older binary, mono and float64 paths, and native editor.
  - [ ] Split and oversampled edition
    - [x] Capture seven programs and the two EQ parameter groups in both
          distinct split release binaries.
    - [x] Compare the nine SNEQ 1100 modes at three rates and two block sizes.
          Verify the seven editable slots and reopen a 2× Spin session in REAPER.
    - [x] Match 2,088 SNo2 and SNEQ renders across factors 1–16, including
          timed modulation and the early oscillator's suspend/resume phase.
          Verify high-factor REAPER projects and sample-offset queues.
    - [ ] Verify the original native editor and additional hosts.
- [ ] [Parametri-Q (lite)](../vst3go-plugins/plugins/eq/parametri-q-lite/README.md)
  - [x] Match 249 January 2010 release cases, including selector order and
        running edits. Verify the four slots and reopen an edited project.
  - [ ] Verify the earlier ZIP's distinct transitions, native editor,
        public float64 processing and other native hosts.
- [ ] [ParametricEQ](../vst3go-plugins/plugins/eq/parametriceq/README.md)
  - [x] Confirm 63 no-write cases in the published x64 callback. Compare
        the audible checkout PE32 build against the later Lite baseline.
  - [x] Port the retained headless PE32 edition and match 135 release and
        checkout captures. Reopen blank and edited slots in REAPER.
  - [ ] Verify other platforms and obtain an audible published x64 baseline.
- [ ] [Posihfopit](../vst3go-plugins/plugins/eq/posihfopit/README.md)
  - [ ] Original Posihfopit
    - [x] Match the released processing modes, filter detents, editable
          programs and variable band count. Reopen edited state in REAPER.
    - [ ] Verify the graph, FFT and skin controls, native VST2 chunk import
          and additional native hosts.
  - [ ] Armyfopit
    - [x] Compare 189 release renders across all eight programs, three input
          levels and timed controls. Reopen independently edited slots in REAPER.
    - [ ] Verify the original graphical editor, alternate coefficient design
          and additional native hosts.
  - [x] Match Smallifopit 4-band mono and stereo DSP and editable programs.
  - [x] Match Smallifopit 8-band mono and stereo DSP and editable programs.
  - [x] Reproduce 2784 renders across both releases and the four shell classes,
        including running edits. Reopen the six-class bundle in Linux REAPER.
  - [ ] Verify the Smallifopit native editors and remaining hosts.
- [ ] [Pushtec 5+1A](../vst3go-plugins/plugins/eq/pushtec-5-1a/README.md)
  - [x] Confirm attribution and the original archive's redistribution terms.
  - [ ] Obtain the contributors' VST3 port authorization.
- [ ] [Soul EQ](../vst3go-plugins/plugins/eq/soul-eq/README.md)
  - [x] Match 281 release captures across the three listening-test modes,
        rate setup orders and timed edits. Reopen edited Init slots in REAPER.
  - [ ] Verify the original editor and its six snapshots, additional native
        hosts and the exact same-value write in an audio-only parameter queue.
- [ ] [Tiny-Q](../vst3go-plugins/plugins/eq/tiny-q/README.md)
  - [x] Compare 363 renders across the website release and two matching
        developer checkout builds. Restore the four editable Init slots.
  - [ ] Verify the native editor, its snapshots and other native hosts.
- [ ] [UNIQUE EQ](../vst3go-plugins/plugins/eq/unique-eq/README.md)
  - [x] Match 180 renders from both releases, including fractional Power
        and Pad behavior. Reopen all seven controls and the editable Init slot.
  - [ ] Verify the original editor and additional native hosts.
- [ ] [Valuable EQ](../vst3go-plugins/plugins/eq/valuable-eq/README.md)
  - [x] Compare Valueable and Valueable Stereo against five released
        DLLs. Match all 772 renders on the channels written by the originals.
  - [x] Verify explicit Mono selection and fresh REAPER reopen for both classes.
  - [ ] Verify original editors and remaining hosts. The port uses
        separately supplied, hash-checked response tables.

#### Filters and crossovers

- [ ] [Bugpass](../vst3go-plugins/plugins/filter/bugpass/README.md)
  - [x] Compare 54 original renders and verify 3072-sample compensation
        at 48 and 96 kHz. Reopen editable programs in REAPER.
  - [ ] Complete the native editor, soft bypass, aliased manual cutoff
        endpoint and additional native hosts.
- [ ] [Bugpass Lite](../vst3go-plugins/plugins/filter/bugpass-lite/README.md)
  - [x] Compare both released binaries across 108 renders. Verify editable
        programs and the modern 768-sample compensation in REAPER.
  - [ ] Verify original editor and additional native hosts. The older ZIP
        advertises 1024 samples for the same measured 768-sample delay.
- [ ] [Butterworth Filters](../vst3go-plugins/plugins/filter/butterworth-filters/README.md)
  - [ ] High-pass mono and stereo
  - [ ] Low-pass mono and stereo
  - [ ] Frequency splitter
  - [ ] Butterworth²HP oversampled edition
  - [x] Compare 509 release renders across nine VST3 classes. Verify mono
        and stereo routing, split summation, oversampling and saved sessions
        on Linux.
  - [ ] Finish editor and platform checks, float64 processing and exact
        live oversampler selector transitions.
- [ ] [Chebyshev Filters](../vst3go-plugins/plugins/filter/chebyshev-filters/README.md)
  - [ ] Type I high-pass and low-pass, including mono editions
    - [x] Match the modern stereo HP and LP, the HP mono wrapper and the
          2008 HP. Verify response, automation and saved Linux sessions.
    - [x] Match 78 retained Type I LP mono source captures. Reopen and
          render the mono source class at two rates in REAPER.
    - [ ] Resolve the 2008 LP release's nonfinite order-three response.
  - [ ] Type II high-pass
    - [x] Compare both published HP editions and frequency correction.
          Reopen and render the two editions in REAPER.
    - [ ] Verify original editor and additional native hosts.
  - [ ] Type II low-pass
    - [x] Match 96 unchanged Pascal source renders. Reopen and render the
          stereo source class with frequency correction in REAPER.
    - [ ] Verify the exact retained x64 build on a supported Windows host.
          The website archive contains only the license and Wine reports
          error 998 during binary initialization.
- [ ] [Dual Linkwitz-Riley Filters](../vst3go-plugins/plugins/filter/dual-linkwitz-riley-filters/README.md)
  - [x] Compare 78 release renders across the four modes and timed changes.
        Verify both editable slots and reopen bandpass output in REAPER.
  - [ ] Verify the original editor and remaining native hosts.
- [ ] [Hum Removal](../vst3go-plugins/plugins/filter/hum-removal/README.md)
  - [x] Match 187 audible release traces, including fixed-rate capture,
        filter transitions and harmonic attenuation. Reopen the edited
        programs and capture profile in Linux REAPER.
  - [ ] Verify the native editor and remaining hosts. The archived x64
        callback leaves output buffers untouched.
- [ ] [Lavaland](../vst3go-plugins/plugins/filter/lavaland/README.md)
  - [x] Match 78 release renders across the five oversampling settings,
        live edits and rail saturation. Reopen both tested settings in REAPER.
  - [ ] Verify the original editor, repeated same-value setter behavior
        and remaining native hosts.
- [ ] [Linear Phase Butterworth](../vst3go-plugins/plugins/filter/linear-phase-butterworth/README.md)
  - [x] Match HP and LP audio and adaptive latency across 252 release renders.
        Verify manual edits and restore compensation in fresh REAPER renders.
  - [ ] Verify audio-queue latency transitions, live FIR history, the
        original editors and additional native hosts.
- [ ] [Linear Phase Frequency Crossover](../vst3go-plugins/plugins/filter/linear-phase-frequency-crossover/README.md)
  - [x] Match 432 release renders, including timed cutoff edits. Reopen the
        mono input and two output routes with 1024-sample PDC at 48 and 96 kHz.
  - [ ] Verify the native editor and additional hosts. The 2009 release
        reports zero latency for essentially the same response.
- [ ] [MoogVST](../vst3go-plugins/plugins/filter/moogvst/README.md)
  - [x] Match 120 normal release renders and the six high-resonance cases
        with explicit controls. Reopen Init and driven settings in REAPER.
  - [ ] Verify other native hosts and the separate retained checkout build.
        The VST3 uses the displayed zero resonance at startup.
- [ ] [Phase Rotator](../vst3go-plugins/plugins/filter/phase-rotator/README.md)
  - [x] Match 240 audible captures and verify unity magnitude, group delay
        and six timed controls. Reopen the edited stereo setting in REAPER.
  - [x] Match 12 additional captures of MIDI CC70–72 across channels and
        verify the same controls in a reopened REAPER MIDI session.
  - [ ] Verify the original editor and other native hosts.
        The published headless x64 replacing callback leaves buffers unchanged.
- [ ] [QR-Filter](../vst3go-plugins/plugins/filter/qr-filter/README.md)
  - [x] Match 117 released stereo renders, including running Q and R edits.
        Reopen independent damping and latched spacing in Linux REAPER.
  - [ ] Verify the original custom editor and additional native hosts.
- [ ] [Rubberfilter](../vst3go-plugins/plugins/filter/rubberfilter/README.md)
  - [x] Match 222 renders across the 2007 and 2010 releases. Verify order 64,
        the edition-specific Power behavior and edited programs in REAPER.
  - [ ] Verify the original editor, remaining automation paths and additional
        native hosts.
- [ ] [Simple Linear Phase Filter](../vst3go-plugins/plugins/filter/simple-linear-phase-filter/README.md)
  - [ ] Linear Phase LP
    - [x] Compare all 22 windows, saved state, tail and compensation on Linux.
  - [ ] Linear Phase HP
    - [x] Compare both released HP binaries, all windows, state and compensation on Linux.
  - [x] Match 138 LinearPhaseFilterFFT split captures outside the original
        uninitialized High startup. Verify live and reopened compensation
        for the measured delay, which differs from the original 1024 report.
- [ ] [Simple Linear Phase Linkwitz-Riley Filter](../vst3go-plugins/plugins/filter/simple-linear-phase-linkwitz-riley-filter/README.md)
  - [x] Match the HP and LP filter audio and reported latency across 846
        captures. Reopen eight compensated REAPER projects at 48 and 96 kHz.
  - [ ] Verify live convolution history, audio-queue latency automation, the
        original editors and additional native hosts.
- [ ] [Simple Linkwitz-Riley Crossover](../vst3go-plugins/plugins/filter/simple-linkwitz-riley-crossover/README.md)
  - [x] Match the x86 LinkwitzRiley and LinkwitzRileyStereo across 162 renders.
        Verify unity summation and reopen their two- and four-channel routes.
  - [ ] Validate additional native hosts and recover an intact original
        macOS binary. The archived x64 replacing and accumulating paths
        leave the output buffers untouched.
- [ ] [SimpleFilter](../vst3go-plugins/plugins/filter/simplefilter/README.md)
  - [x] Port the audible Win32 checkout edition and match 162 float32
        captures. Reopen its edited and untouched presets in REAPER.
  - [ ] Verify other hosts and recover an audible reference for the published
        x64 edition. Both checked x64 builds left output buffers untouched.
- [ ] [Weighting Filter](../vst3go-plugins/plugins/filter/weighting-filter/README.md)
  - [x] Match 176 mono renders bit for bit across both distinct releases.
        Verify the original host controls and reopen edited banks in REAPER.
  - [ ] Verify additional host platforms.

### 7.2 Dynamics, bass and distortion

- [ ] Port dynamics processors, waveshapers and bass effects. Measure static
      curves, attack/release, lookahead, sidechain routing, harmonics and aliasing.
      Match intentional saturation and original oversampling modes.

#### Bass

- [ ] [Bass Baron](../vst3go-plugins/plugins/bass/bass-baron/README.md)
  - [x] Match 132 captures from the published x64 release, covering float32,
        float64, live algorithm and response changes, lowcut and program recall.
        Reopen independently edited programs in REAPER. The port uses the
        pinned DAV soft-knee gain from [algo-dsp PR #48](https://github.com/CWBudde/algo-dsp/pull/48).
  - [ ] Resolve the published x86 callback fault and verify the native editor
        and other host platforms.
- [ ] [Bass Extender](../vst3go-plugins/plugins/bass/bass-extender/README.md)
  - [x] Match the divider, crossover, detector and sample-offset edits against
        360 audible x86 captures. Validate and reopen four REAPER projects,
        including mid-only routing and independently edited program slots.
  - [ ] Verify the x64 callback, which left outputs untouched under Wine, and
        test the original editor and additional native hosts. Reconcile the
        original zero tail report with the measured sustained shaped output.
- [ ] [SubBoost](../vst3go-plugins/plugins/bass/subboost/README.md)
  - [x] Match 102 published Win32 renders and compare 102 retained Win32
        renders. Verify sample-offset edits, effective Tune and Order zero
        state and four reopened REAPER projects.
  - [ ] Verify the native editor, retained Win64 audio and additional native
        hosts. The VST3 keeps the dry mix where legacy Key Osc overflows.

#### Dynamics

- [ ] [Adhesive](../vst3go-plugins/plugins/dynamics/adhesive/README.md)
  - [x] Match 108 release captures for compression, mix, clip, bypass and
        the mono sidechain. Reopen six REAPER sessions with edited slots,
        routed and disconnected auxiliary inputs.
  - [ ] Verify the native editor, original sidechain edit timing and other
        native hosts. The port selects the sidechain immediately and the
        retained x64 DLL failed to load under Wine.
- [ ] [Audiocation Compressor](../vst3go-plugins/plugins/dynamics/audiocation-compressor/README.md)
  - [x] Port the released DVL1 mono and DVL2 stereo classes. Match 196
        transformer, detector, automation, finite expansion and rate-change
        captures. Reopen both layouts and edited programs in REAPER.
        Use the pinned DAV curve and timing from [algo-dsp PR #49](https://github.com/CWBudde/algo-dsp/pull/49).
  - [ ] Check original graphics, meters, retained editions and additional
        native hosts. The port underflows safely at extreme expansion.
- [ ] [EnhancedGate](../vst3go-plugins/plugins/dynamics/enhancedgate/README.md)
  - [x] Match all 114 x86 release captures bit for bit, including the +40 dB
        startup gain, explicit +1 dB endpoint write and Power bypass. Reopen
        and render six REAPER sessions with the endpoint state preserved.
  - [ ] Verify the original editor and additional native hosts. The archived
        wrapper leaves the gate detector silent, and the retained x64 DLL
        failed to load under Wine.
- [ ] [Hard Knee Compressor](../vst3go-plugins/plugins/dynamics/hard-knee-compressor/README.md)
  - [x] Match 72 x86 release captures, including independent stereo channels,
        startup control values and the 0.01 ms attack. Verify sample-offset
        edits and four reopened REAPER projects.
  - [ ] Verify the original editor, retained Win64 audio, additional native
        hosts and redistribution permission.
- [ ] [LA4029](../vst3go-plugins/plugins/dynamics/la4029/README.md)
  - [x] Port the 2008 and 2010 releases separately. Compare both sample
        widths in 192 captures, including the linked detector, wet mix and
        live highpass changes. Reopen and render four REAPER projects.
        Use the two measured gain curves from [algo-dsp PR #50](https://github.com/CWBudde/algo-dsp/pull/50).
  - [ ] Verify the original meters and native editors, additional host
        platforms, and permission to redistribute the original assets.
- [ ] [Lightweight Dynamics](../vst3go-plugins/plugins/dynamics/lightweight-dynamics/README.md)
  - [x] Match 48 x86 release renders through the gate, compressor, limiter
        and Soft Clip paths, including the original right-only Soft Clip
        detector and Auto MakeUp latch. Reopen edited programs in REAPER.
  - [ ] Verify the original editor, gain readout and additional native hosts.
        The published VST2 double callback left its output buffers untouched.
- [ ] [Lightweight Feedback Compressor](../vst3go-plugins/plugins/dynamics/lightweight-feedback-compressor/README.md)
  - [x] Match 156 finite comparisons per class for the 2010 archive/installer,
        September 2009 Series and March 2009 ZIP releases. Verify the different
        clipping and adaptive feedback paths and reopen 12 REAPER projects.
  - [ ] Verify the native editor and additional native hosts. The ports keep
        audio finite at the measured legacy ratio and makeup overflows.
- [ ] [Lightweight Feedback Limiter](../vst3go-plugins/plugins/dynamics/lightweight-feedback-limiter/README.md)
  - [x] Match 252 x86 captures from both distributions, including minimum
        timing, auto-makeup, channel routing and timed edits. Reopen five
        REAPER projects with all editable slots and the pinned feedback
        timing from [algo-dsp PR #51](https://github.com/CWBudde/algo-dsp/pull/51).
  - [ ] Verify the original editor, additional native hosts and permission
        to redistribute original assets.
- [ ] [Lightweight Multiband Compressor](../vst3go-plugins/plugins/dynamics/lightweight-multiband-compressor/README.md)
  - [x] Port the archive/installer, Series and ZIP variants. Match 288 finite
        captures and verify finite output in 24 zero-ratio runs. Reopen six
        REAPER projects with independently edited splits and band controls.
  - [ ] Verify the native editor, meters and additional host platforms.
- [ ] [Lightweight Soft Knee Compressor](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-compressor/README.md)
  - [x] Port three release editions and match 159 captures across the
        detector timings, clip curves, stereo routing and editable programs.
        Verify independent edits in a reopened REAPER project.
  - [ ] Verify the original editor, characteristic plot and additional native
        hosts. The published VST2 double callback left outputs untouched.
- [ ] [Lightweight Soft Knee Gate](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-gate/README.md)
  - [x] Verify 132 cases per release class, including the different timing
        curves, sample-offset edits and factory knees above 10 dB. Reopen
        and render 12 REAPER sessions with edited and untouched slots.
  - [ ] Verify the native editor and additional hosts. The port evaluates
        finite underflow where the released x87 exponent wraps.
- [ ] [Lightweight Soft Knee Limiter](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-limiter/README.md)
  - [x] Match 576 renders from four distributions, retaining the 2010 and
        2009 clipping curves in two classes. Verify sample-offset edits,
        eleven editable slots and five reopened REAPER projects.
  - [ ] Verify the native editor, retained Win64 provenance, additional hosts
        and redistribution permission for original assets.
- [ ] [Lightweight Upward Compressor](../vst3go-plugins/plugins/dynamics/lightweight-upward-compressor/README.md)
  - [x] Match 252 captures from the archive and Series releases. Verify both
        clip curves, upward gain, latched makeup, all programs and four
        reopened REAPER projects using the curve from [algo-dsp PR #52](https://github.com/CWBudde/algo-dsp/pull/52).
  - [ ] Verify the native editor, characteristic display and other host
        platforms. The port bounds gain and output at exponent overflow.
- [ ] [Limiter](../vst3go-plugins/plugins/dynamics/limiter/README.md)
  - [x] Match all 72 renders from the three audible retained binaries sample
        for sample. Reopen the unassigned timing and explicit minimum timing
        states and the edited Default program in REAPER.
  - [ ] Verify an audible published x64 callback in another host and test
        additional native VST3 hosts. The released x64 callbacks left the
        float32 and float64 output buffers untouched under Wine.
- [ ] [Lookahead Limiter](../vst3go-plugins/plugins/dynamics/lookahead-limiter/README.md)
  - [x] Match the measured audio and program values and verify saved delay
        and compensation in REAPER on Linux.
  - [ ] Reproduce the original editor and verify live delay changes and other host platforms.
- [ ] [Multiband Compressor](../vst3go-plugins/plugins/dynamics/multiband-compressor/README.md)
  - [x] Match 132 original x86 captures for the linked three-band paths,
        startup aliases, crossover and dynamics edits. Reopen six REAPER
        projects, including the untouched and edited high crossover settings.
  - [ ] Verify the original editor, retained Win64 callback and additional
        native hosts. The retained x64 DLL failed to load under Wine.
- [ ] [Sidechain Compressor](../vst3go-plugins/plugins/dynamics/sidechain-compressor/README.md)
  - [x] Compare all 13 released programs and the loaded detector path.
        Validate the Linux class and restore the nested state in REAPER.
  - [ ] Finish native detector selection, editor behavior and other host platforms.
- [ ] [Simple Compressor](../vst3go-plugins/plugins/dynamics/simple-compressor/README.md)
  - [x] Match 72 stable January 2010 Win32 renders, including the measured
        RMS window, queued edits and four reopened REAPER projects.
  - [ ] Confirm the 2008 ZIP and published x64 audio in other native hosts,
        and verify the original editor. The ZIP returned nonfinite samples,
        and the published x64 build returned silence. The VST3 resumes finite
        compression after silence.
- [ ] [Simple Feedback Compressor](../vst3go-plugins/plugins/dynamics/simple-feedback-compressor/README.md)
  - [x] Compare the audible 2010 edition across three sample rates and two
        block sizes. Verify zero latency, tail and saved settings on Linux.
  - [ ] Verify the custom editor and remaining host platforms.
- [ ] [Simple Gate](../vst3go-plugins/plugins/dynamics/simple-gate/README.md)
  - [x] Port both audible releases. Compare 270 bit-exact renders against
        five original binaries at 44.1, 48 and 96 kHz, including live changes
        and the ZIP's displayed 0 dB and effective −40 dB startup threshold.
        Verify explicit same-value edits and six independently reopened REAPER
        projects, SDK validation for both classes and Windows x64 cross-build.
  - [ ] Verify the 54 untouched-buffer renders from the published headless
        x64 binary in another host, and compare the original editor and
        physical Windows and macOS host behavior.
- [ ] [Simple Limiter](../vst3go-plugins/plugins/dynamics/simple-limiter/README.md)
  - [x] Compare the published four-control and retained three-control
        editions. Match 60 published renders within 1.20e−7 and all 24 retained
        renders exactly. Verify all 72 plain Limiter regression renders,
        SDK validation and REAPER save and reopen for both classes.
  - [ ] Compare each original editor, including the retained editor's Attack
        update on opening, and verify the remaining native host platforms.
- [ ] [Smooth Multiband Compressor](../vst3go-plugins/plugins/dynamics/smooth-multiband-compressor/README.md)
  - [x] Match 114 Win32 captures within 5.97e−7, including queued crossover
        edits, the Smooth mode from [algo-dsp PR #53](https://github.com/CWBudde/algo-dsp/pull/53),
        Limit and the original frame-16 impulse peak with zero reported latency.
        Validate the class and reopen four REAPER projects within 1.20e−7.
  - [ ] Compare the original editor and its auto-makeup controls, verify Windows
        and macOS hosts, and confirm permission to redistribute original assets.
- [ ] [Soft Knee Feedback Compressor](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-compressor/README.md)
- [ ] [Soft Knee Feedback Limiter](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-limiter/README.md)
  - [x] Compare the audible September 2008 ZIP and January 2010 SKLDM
        builds. Match 228 renders within 4.77e−7 on finite samples and
        eight reopened REAPER projects within 2.99e−8. Map the ZIP's
        cold zero-input NaNs to silence. Validate both classes and cross-build
        them for Windows x64.
  - [ ] Locate an audible binary or executable DSP for the separately
        named SKFBLimiter prototype. The retained implementation is disabled.
  - [ ] Check the retained x64 binary, which fails to load with Wine error
        998, and verify the original editors and remaining native hosts.
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
  - [x] Compare all three VST3 classes to the audible x86 releases.
        Verify quad routing, program state and independent controls in REAPER.
  - [ ] Verify the native editors, the x64 release discrepancy and other hosts.
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
  - [x] Compare delay, feedback and timed resize against 72 x86 renders.
        Reopen the three editable Init slots and render exact taps in REAPER.
  - [ ] Verify the original editor and other host platforms.
- [ ] [StkEcho](../vst3go-plugins/plugins/delay/stkecho/README.md)
  - [x] Match 60 stereo renders, including sample-offset edits. Verify all
        five editable programs and exact saved-session taps in Linux REAPER.
  - [ ] Verify the native editor and additional host platforms.
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
  - [x] Compare the ten local release IR pairs, finite settings and tails.
        Reopen settings and verify three latency orders in REAPER on Linux.
  - [ ] Verify the custom editor and live latency changes, and resolve asset
        redistribution provenance and host checks on other platforms.
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
  - [x] Measure both 2009 binary releases. Port the audible oscillator from
        the retained Delphi source and verify MIDI, keyboard and state on Linux.
  - [x] Verify CC126 at its sample offset in the ABI and a reopened REAPER
        session. Confirm that CC123 leaves held notes active.
  - [ ] Verify native Windows and macOS editors.
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
