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
  - [x] Reserve optional dense event output capacity before processing.
        Verify Audio2Midi Trigger emits all 2,046 events in its busiest
        tested block without allocating.
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
  - [x] Record released, retained and gated editions for all 30 filter and
        equalizer entries. Verify audio and recall for the usable paths.
  - [ ] Complete the remaining live transitions, response displays,
        commercial references, editor and native host checks by product.

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
        eight separately supplied, hash-checked response tables subject
        to the author's noncommercial-use terms.

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
  - [ ] Locate the original Pascal entry point. Verify audio-queue latency
        transitions, live FIR history, the original editors and native hosts.
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
  - [ ] Verify the HP and LP editors, time-varying automation and VST2
        preset import. Verify native Windows and macOS hosts for all classes.
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

- [x] Port the reproducible dynamics processors, waveshapers and bass
      effects. Measure static curves, attack/release, lookahead, sidechain
      routing, harmonics, aliasing, saturation and oversampling.
- [ ] Close the remaining per-product reference, editor, rights and native
      host checks below.

#### Bass

- [ ] [Bass Baron](../vst3go-plugins/plugins/bass/bass-baron/testdata/reference/CAPTURE.md)
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
- [ ] [Audiocation Compressor](../vst3go-plugins/plugins/dynamics/audiocation-compressor/testdata/reference/CAPTURE.md)
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
- [ ] [Lightweight Dynamics](../vst3go-plugins/plugins/dynamics/lightweight-dynamics/testdata/reference/CAPTURE.md)
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
- [ ] [Lightweight Soft Knee Compressor](../vst3go-plugins/plugins/dynamics/lightweight-soft-knee-compressor/testdata/reference/CAPTURE.md)
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
- [ ] [Limiter](../vst3go-plugins/plugins/dynamics/limiter/testdata/reference/CAPTURE.md)
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
        and verify the original editor and its graphics permissions. The ZIP
        returned nonfinite samples, and the published x64 build returned
        silence. The VST3 resumes finite compression after silence.
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
- [ ] [Simple Limiter](../vst3go-plugins/plugins/dynamics/simple-limiter/testdata/reference/CAPTURE.md)
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
- [ ] [Soft Knee Feedback Compressor](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-compressor/testdata/reference/CAPTURE.md)
  - [x] Port both releases and compare 180 original captures. The 2010 class
        matches within 4.77e−7. The 2008 ZIP matches 74 renders within 1.20e−7.
        Its 16 high-feedback traces differ by up to 0.02771 peak and 1.31%
        relative RMS. Verify eight reopened REAPER renders within 2.39e−7,
        both classes in the SDK Validator, and a Windows x64 cross-build.
  - [ ] Verify the original editors and additional native hosts. The ZIP
        uses the original fixed-rate feedback circuit and its displayed zero
        values only replace the effective settings when explicitly edited.
- [ ] [Soft Knee Feedback Limiter](../vst3go-plugins/plugins/dynamics/soft-knee-feedback-limiter/testdata/reference/CAPTURE.md)
  - [x] Compare the audible September 2008 ZIP and January 2010 SKLDM
        builds. Match 228 renders within 4.77e−7 on finite samples and
        eight reopened REAPER projects within 2.99e−8. Map the ZIP's
        cold zero-input NaNs to silence. Validate both classes and cross-build
        them for Windows x64.
  - [ ] Locate an audible binary or executable DSP for the separately
        named SKFBLimiter prototype. The retained implementation is disabled.
  - [ ] Check the retained x64 binary, which fails to load with Wine error
        998, and verify the original editors and remaining native hosts.
- [ ] [Transient Bug](../vst3go-plugins/plugins/dynamics/transient-bug/testdata/reference/CAPTURE.md)
  - [x] Match every finite sample across 96 BugWars renders. Use the finite
        zero-Attack limit where the original emits NaNs.
  - [x] Match 240 TrBug renders within 1.61e−22, including all selector modes,
        both styles, the noise gate and the editable `init` program.
  - [x] Verify 24 queued-automation renders and save and reopen both classes
        in REAPER. Validate both classes in the SDK Validator.
  - [ ] Compare the original BugWars editor and verify remaining native hosts.

#### Distortion

- [ ] [2-Band Distortion](../vst3go-plugins/plugins/distortion/2-band-distortion/testdata/reference/CAPTURE.md)
  - [x] Compare 522 original captures across both modern builds and the 2008
        ZIP. Match modern renders within 5.97e−8 and the ZIP within 3.52e−6.
        Verify 18 reopened REAPER sessions, including edited program slots and
        the ZIP's factory silence versus explicit zero drive. Validate all
        three classes and cross-build them for Windows x64.
  - [ ] Compare the original editor and physical Windows and macOS hosts.
        The published and retained standalone x64 copies left output untouched,
        and four other x64 artifacts failed to load in Wine.
- [ ] [Advanced Clipper](../vst3go-plugins/plugins/distortion/advanced-clipper/README.md)
  - [x] Match 432 audible captures from both Win32 releases within 5.97e−8.
        Verify sample-offset automation and the 2008 fixed-rate filter
        behavior using the float64 path from [algo-dsp PR #54](https://github.com/CWBudde/algo-dsp/pull/54).
        Validate both VST3 classes and match five reopened REAPER projects
        within 1.33e−19.
  - [ ] Verify x64 audibility in an independent Windows host, the original
        float64 callback and the original editor and graphics permissions.
        The 144 x64 Wine captures were silent. Native VST3 host checks remain open.
- [ ] [Amp Sim](../vst3go-plugins/plugins/distortion/amp-sim/testdata/reference/CAPTURE.md)
  - [x] Compare 378 renders from the audible Win32 archive and both installer
        builds. Verify all seven cabinets, stereo and summed routing, both
        presets, live controls and noise RMS within 1.05%. Confirm the cold
        44.1 kHz D.I. behavior and the trim changes tied to parameter writes.
  - [x] Reopen four REAPER projects with both editable slots. Compare D.I.
        from frame zero and Mesa-Boogie after the 2048-frame bias settling
        window. Validate the class and cross-build it for Windows x64.
  - [ ] Verify the original editor and physical Windows and macOS hosts.
        Check the headless x64 archive in another host. Its float32 and
        float64 callbacks left the output untouched in Wine.
- [ ] [Analogue Mojo](../vst3go-plugins/plugins/distortion/analogue-mojo/testdata/reference/CAPTURE.md)
  - [x] Compare 660 finite renders across five verified profiles, including
        120 fresh-process retained x86 renders. Verify sample error within
        9.54e−7 and harmonic amplitude error within 2.56e−8, SDK validation
        and all five class states after reopening a REAPER project.
  - [ ] Verify retained x86 repeat-instance gain, the January x64 no-write
        callback, the older x86 build's nonfinite output and the i386 Mac
        build. Compare original editors and native hosts.
- [ ] [Chebyshev Waveshaper](../vst3go-plugins/plugins/distortion/chebyshev-waveshaper/testdata/reference/CAPTURE.md)
  - [x] Port the 2010 and 2008 editions with 24 signed harmonics and an
        unclamped evaluator from [algo-dsp PR #55](https://github.com/CWBudde/algo-dsp/pull/55).
        Compare 732 original renders and eight reopened REAPER projects.
        Validate both classes and cross-build them for Windows x64.
  - [ ] Verify the no-editor builds, which faulted before metadata, and the
        x64 originals with untouched buffers or load errors under Wine.
        Compare original editors and physical Windows and macOS hosts.
- [ ] [Christortion](../vst3go-plugins/plugins/distortion/christortion/testdata/reference/CAPTURE.md)
  - [x] Compare 1,320 captures from four published releases and the retained
        Win64 build. Verify all sixteen editable slots, both 2006 oversampling
        paths, signed phase interpolation, DC and the retained 2 dB Carnaval
        alias. Peak finite sample error stays within 2.39e−7.
  - [x] Reopen and render ten REAPER projects within 1.82e−12. Verify each
        edited inactive slot, validate all five classes and cross-build for
        Windows x64.
  - [ ] Verify the original editors and physical Windows and macOS hosts.
        The native VST3 runtime still exposes only float32 processing.
- [ ] [Decimator](../vst3go-plugins/plugins/distortion/decimator/README.md)
  - [x] Match all 120 comparable original renders bit for bit, including the
        fractional hold phase, right-channel high-pass coupling, three
        editable programs and six MIDI CC mappings at their queued sample offsets.
        Validate the VST3 and reopen three repeatable REAPER projects.
  - [ ] Compare the original editor and physical Windows and macOS hosts,
        confirm graphics redistribution and support the measured original
        float64 input path. REAPER's variable silent pre-roll affects hold phase.
- [ ] [Dynamite](../vst3go-plugins/plugins/distortion/dynamite/PORT.md)
  - [x] Compare 108 archived Win32 renders for the four-step, 12-tap stereo
        classic profile within 1.20e−7. Verify synthetic profile import,
        both edited program slots after REAPER reopen and SDK validation.
  - [ ] Verify the loose Win32 build, which failed during the ordinary render,
        additional profile formats, native editor and hosts. The installer
        remains encrypted and the retained notice restricts distribution.
- [ ] [Exciter](../vst3go-plugins/plugins/distortion/exciter/testdata/reference/CAPTURE.md)
  - [x] Match 342 renders across the 2008, 2010 and 2011 editions. Verify
        the 2011 reset on resume and the older editions' retained history,
        both editable slots, and twelve reopened REAPER projects within
        5.39e−7. Recheck all 522 2-Band Distortion traces after sharing the
        legacy filter. Validate all classes and cross-build for Windows x64.
  - [ ] Verify the retained Win32 pre-metadata failure and the x64 no-write
        or load failures in another host. Compare original editors and
        physical Windows and macOS hosts.
- [ ] [Hysteresis](../vst3go-plugins/plugins/distortion/hysteresis/testdata/reference/CAPTURE.md)
  - [x] Match 468 renders across all six Win32 and Win64 originals bit for
        bit with the Preisach processor from [algo-dsp PR #56](https://github.com/CWBudde/algo-dsp/pull/56).
        Verify independent loops, saturation, nearest-even thresholds,
        rate and activation transitions and updates at the displayed 0 dB.
  - [x] Verify the host parameter view, all six reopened REAPER renders,
        SDK validation, and the Windows x64 cross-build.
  - [ ] Verify physical Windows and macOS VST3 hosts.
- [ ] [OpAmp](../vst3go-plugins/plugins/distortion/opamp/README.md)
  - [x] Match all 96 Win32 Digital OpAmp Simulation traces bit for bit using
        the saturation function from [algo-dsp PR #57](https://github.com/CWBudde/algo-dsp/pull/57).
        Verify queued gain changes and both edited slots through the VST3 ABI,
        four reopened REAPER renders, and SDK validation.
  - [ ] Verify the original editor, graphics redistribution permissions and
        physical Windows and macOS hosts.
- [ ] [Oversampled Tanh](../vst3go-plugins/plugins/distortion/oversampled-tanh/PORT.md)
  - [x] Compare 540 Win32 and Win64 traces across the published and retained
        classes. Verify 16 and zero samples of latency, transition resets on
        same-value writes and saved overrides in REAPER, and SDK validation.
        Use the allocation-free coefficient updates from
        [algo-dsp PR #58](https://github.com/CWBudde/algo-dsp/pull/58).
  - [ ] Verify the native editor, redistribution permissions, and physical
        Windows and macOS hosts.
- [ ] [Scary](../vst3go-plugins/plugins/distortion/scary/PORT.md)
  - [x] Match all 288 captures across the 2005 website, retained Betabugs
        and retained Waveshaper classes. Verify cold settings, same-value
        writes, live changes, rate handling and all four editable slots.
  - [x] Verify nine reopened REAPER renders, SDK validation and the
        Windows x64 cross-build.
  - [ ] Compare the original editors and verify graphics permissions and
        physical Windows and macOS VST3 hosts.
- [ ] [TanhWaveshaper](../vst3go-plugins/plugins/distortion/tanhwaveshaper/testdata/reference/CAPTURE.md)
  - [x] Compare 396 captures per class for the audible 2009 stereo, retained
        stereo and Plugin Match mono editions. Verify curve selection,
        oversampling, alias rejection, and twelve reopened REAPER projects.
        Validate all three classes and cross-build the Windows x64 bundle.
  - [ ] Verify the x64 stereo no-write and load failures, the Plugin Match
        x86 first-block fault, and physical Windows and macOS VST3 hosts.
- [ ] [Toadstunnel](../vst3go-plugins/plugins/distortion/toadstunnel/README.md)
  - [x] Port the 2003 Toadstunnel class. Match 48 undithered renders exactly
        and compare high-pass dither energy and timing in 24 cases. Verify
        four editable slots, queued changes, SDK and four REAPER reopens.
  - [x] Port Toadstunnel². Verify all eleven routing modes, crossover,
        oversampling, and 544 finite comparisons. Match three queued
        automation renders and reopen four REAPER projects. Use valid
        child depths where six original presets produced nonfinite audio.
  - [ ] Confirm the six nonfinite factory recalls in a native Win32 host.
  - [ ] Verify the native editors, artwork permissions, and physical Windows
        and macOS hosts. The 2003 Wine original emits an integer sentinel
        at the start of dithered blocks. Compare this on native Win32.
- [ ] [Tubster](../vst3go-plugins/plugins/distortion/tubster/PORT.md)
  - [x] Verify the original editor-open capture path and the eight harmonic
        controls. Match 300 original float32 renders within 3.82e−6 and
        preserve the active-harmonic mask across REAPER reopen. Validate
        the VST3 class and the original's working double callback.
  - [ ] Identify the original Pascal entry point. Verify VST3 double audio,
        the original editor and physical Windows and macOS VST3 hosts.

### 7.3 Modulation, delay, pitch, spectral processing and reverb

- [x] Compare modulation speed/depth and channel phase, delay taps and
      feedback, pitch ratio and transients, noise profiles, impulse responses
      and reverb decay across the reproduced editions. Use envelope or spectral
      comparisons for random processes. Keep outstanding edition, transition,
      asset and native host checks with their products.

#### Modulation

- [ ] [Barberpole Flanger](../vst3go-plugins/plugins/modulation/barberpole-flanger/testdata/reference/CAPTURE.md)
  - [x] Match 126 renders bit for bit across all four audible Win32 binaries.
        Verify modulation modes, stereo offsets, editable programs, six
        reopened REAPER renders, SDK validation, and a Windows x64 cross-build.
  - [ ] Verify the x64 original that left its buffers untouched in Wine,
        the original editor and graphics permissions, and native hosts.
- [ ] [Flanger](../vst3go-plugins/plugins/modulation/flanger/PORT.md)
  - [x] Compare 774 renders across the 2010, later editor and headless
        classes. Verify the fixed oscillator clock, variable delay, Mix
        behavior, and edition-specific activation using
        [algo-dsp PR #59](https://github.com/CWBudde/algo-dsp/pull/59).
  - [x] Verify nine phase-aligned reopened REAPER renders, SDK validation,
        and the Windows x64 cross-build.
  - [ ] Verify the original editor and artwork permissions, early
        headless x64 no-write builds, the i386 Mac release, VST3 double
        audio, and physical Windows and macOS hosts.
- [ ] [ITA Phaser](../vst3go-plugins/plugins/modulation/ita-phaser/testdata/reference/CAPTURE.md)
  - [x] Match all 126 published Win32 renders exactly. Verify the notch
        sweep, stereo traces, startup Mix history, six reopened REAPER
        renders, SDK validation and the Windows x64 cross-build.
  - [ ] Locate the published ITAP Pascal entry point. Compare the original
        editor and verify physical Windows and macOS VST3 hosts.
- [ ] [Phase-O-Matic](../vst3go-plugins/plugins/modulation/phase-o-matic/PORT.md)
  - [x] Match 1,248 original captures bit for bit across the website
        editor, later editor and retained headless classes. Verify rate
        assignment order, sample-offset edits, and saved Default state in
        REAPER. Validate all three classes.
  - [ ] Resolve the early 2008 control range and published x64 no-write
        outcomes. Verify VST3 double audio, native editors and hosts.
- [ ] [Ring Modulator](../vst3go-plugins/plugins/modulation/ring-modulator/README.md)
  - [x] Mono edition audio and state.
  - [x] Stereo edition audio and state.
  - [x] Quad edition audio and state.
  - [x] Compare all three VST3 classes to the audible x86 releases.
        Verify quad routing, program state and independent controls in REAPER.
  - [ ] Verify the native editors, the x64 release discrepancy and other hosts.
- [ ] [Simple Chorus](../vst3go-plugins/plugins/modulation/simple-chorus/testdata/reference/CAPTURE.md)
  - [x] Compare 114 scenarios per class across the 2009, later editor
        and retained host-editor builds. Verify settled dry taps, stereo
        drift statistics, and repeated renders from the saved seed in
        twelve reopened REAPER projects. Validate and cross-build.
  - [ ] Resolve the x64 no-write and load outcomes and the retained Win32
        callback fault. Verify original editors, VST3 double audio and
        physical Windows and macOS hosts.
- [ ] [Spin Bug Lite](../vst3go-plugins/plugins/modulation/spin-bug-lite/README.md)
  - [x] Compare 198 Win32 renders across ten routes and six editable slots.
        Verify queued changes, fixed LFO timing, SDK validation, and four
        reopened REAPER projects.
  - [ ] Verify the published Win64 callbacks that rendered silence,
        the original editor, and physical Windows and macOS hosts.
- [ ] [StkChorus](../vst3go-plugins/plugins/modulation/stkchorus/PORT.md)
  - [x] Compare 192 renders across the released and retained Win32
        binaries. Verify stereo delay, rate and depth changes, the Long
        preset's 600 ms value, and three phase-aligned REAPER reopens.
        Validate the class and cross-build for Windows x64.
  - [ ] Verify the original editor, artwork permissions, VST3 double
        processing and physical Windows and macOS hosts.
- [ ] [Vibrato](../vst3go-plugins/plugins/modulation/vibrato/PORT.md)
  - [x] Compare 654 renders across all six audible Win32 and Win64
        originals. Verify pitch excursion, the fixed LFO clock, rate and
        depth changes, and three phase-aligned REAPER reopens. Validate
        the VST3 class and cross-build for Windows x64.
  - [ ] Verify the early Win64 no-write release, original editor,
        artwork permissions, VST3 double audio and native hosts.

#### Delay

- [ ] [DegradingDelay](../vst3go-plugins/plugins/delay/degradingdelay/PORT.md)
  - [x] Match 234 stereo renders exactly, including crossed routing,
        the held feedback tap, rate-order effects and live delay resize.
        Verify saved feedback clocks in REAPER and validate the VST3.
  - [ ] Compare the original editor and native Windows and macOS hosts.
- [ ] [FeedBUG](../vst3go-plugins/plugins/delay/feedbug/testdata/reference/CAPTURE.md)
  - [x] Compare 114 captures per class across the 2004, May 2005 and
        retained April 2005 editions. Verify cabinet morphing from the
        original externally supplied resource, feedback, editable banks,
        and twelve reopened REAPER projects. Validate and cross-build.
  - [ ] Resolve the retained February edition, resource redistribution,
        native editor and physical platform checks.
- [ ] [ModDelay](../vst3go-plugins/plugins/delay/moddelay/README.md)
  - [x] Compare 630 renders across five audible retained editions.
        Verify tap timing, feedback polarity, filtering, the initial
        modulation-rate alias, queued edits, five REAPER reopens and SDK.
  - [ ] Verify the published x64 no-write callback and native hosts.
- [ ] [Party-Delay](../vst3go-plugins/plugins/delay/party-delay/PORT.md)
  - [x] Compare 186 renders across its four bands, nine filters and
        Bode settings. Verify both editable slots, below-range saved
        values, four reopened REAPER projects and SDK validation.
        Use the shared shifter from [algo-dsp PR #60](https://github.com/CWBudde/algo-dsp/pull/60).
  - [ ] Resolve the variable and nonfinite original Bode captures and
        verify the original editor, artwork permissions and native hosts.
- [ ] [Simple Sample Delay](../vst3go-plugins/plugins/delay/simple-sample-delay/README.md)
  - [x] Compare delay, feedback and timed resize against 72 x86 renders.
        Reopen the three editable Init slots and render exact taps in REAPER.
  - [ ] Verify the original editor and other host platforms.
- [ ] [StkEcho](../vst3go-plugins/plugins/delay/stkecho/README.md)
  - [x] Match 60 stereo renders, including sample-offset edits. Verify all
        five editable programs and exact saved-session taps in Linux REAPER.
  - [ ] Verify the native editor and additional host platforms.
- [ ] [VariableDelay](../vst3go-plugins/plugins/delay/variabledelay/PORT.md)
  - [x] Match 864 stereo captures across six audible original builds
        exactly. Verify zero-delay behavior, sample-rate setup, all
        three editable Init slots, REAPER reopen and SDK validation.
  - [ ] Verify the published x64 no-write build, VST3 double processing,
        the original editor and physical Windows and macOS hosts.

#### Pitch

- [ ] [Bode Frequency Shifter](../vst3go-plugins/plugins/pitch/bode-frequency-shifter/testdata/reference/CAPTURE.md)
  - [x] Port the Win32 mono core and the stereo wrappers. Compare 1,008
        captures, including routing, both sidebands and timed edits. The two
        published wrappers are identical and resolve an alternate core filename.
        Initialized frames match within 1.073e-6. Clear both Hilbert histories
        for deterministic output where the originals leave one uncleared.
  - [x] Verify both classes in eight reopened Linux REAPER projects, validate
        all 94 SDK cases and cross-build Windows x64.
  - [ ] Resolve the original x64 no-write and load failures. Verify the
        original custom editor and physical Windows and macOS hosts.
- [ ] [Granular Pitch Shifter](../vst3go-plugins/plugins/pitch/granular-pitch-shifter/README.md)
  - [x] Match 486 stereo captures across three Win32/Win64 originals,
        including sub-minimum factory grain values, effective startup settings
        and sample-offset automation. 480 renders are sample-exact and the
        remaining peak difference is 3.73e-9. Verify all four factory slots and
        one edit in reopened Linux REAPER projects, SDK validation and Win64
        cross-build.
  - [ ] Verify the original editor, double-processing capability and native
        Windows and macOS hosts.
- [ ] [SoundTouch based Pitch Shifter](../vst3go-plugins/plugins/pitch/soundtouch-based-pitch-shifter/PORT.md)
  - [x] Match 264 renders from the 2010 stereo edition within 2.39e-7,
        including unison, both octave endpoints and automation. Use an
        opt-in, version-checked external SoundTouch 1.4.1 engine. Verify
        SDK validation, REAPER state reopen and 40 native allocation probes.
  - [ ] Resolve the 2009 mono unity and transition differences and obtain
        a matching engine for the retained x64 binary. Verify the original
        editor and host platforms beyond Linux amd64.
- [ ] [Split Harmonizer](../vst3go-plugins/plugins/pitch/split-harmonizer/PORT.md)
  - [x] Identify and port separate nine-control 2009 and fifteen-control
        2010 classes for Linux amd64. Out of 288 release captures,
        match 216 settled windows within 2.608e-7. Verify both reported
        delays and edited programs in four sample-exact dry REAPER
        reopens, 94 SDK checks, and bounded external engine settings.
  - [ ] Resolve the 2010 mid/side and manual short-window differences,
        live transitions, original double audio and in-place behavior,
        and the full ±12,000-cent range. Verify the retained no-space
        builds, original editors and other physical host platforms.
- [ ] [StkPitchShift](../vst3go-plugins/plugins/pitch/stkpitchshift/testdata/reference/CAPTURE.md)
  - [x] Match 194 stereo captures from the published and retained Win32
        builds within 1.193e-7. Verify the unreported 512-sample unity delay,
        two controls, all five editable slots and history through rate and
        activation changes. Three Linux REAPER reopens render sample-exact.
        Pass SDK validation and cross-build Windows x64.
  - [ ] Verify the original editor and native hosts. Both originals process
        double-precision audio at the same unity delay. The port uses float32.

#### Reverb

- [ ] [ConvoFX](../vst3go-plugins/plugins/reverb/convofx/README.md)
  - [x] Verify all 40 stereo IRs against the website binaries and match 64
        audible cases within 1.863e-9. Reopen four sessions in Linux REAPER
        and verify 64, 2,048 and 65,536 samples of delay compensation.
        Validate the SDK class, cross-build Windows x64 and keep all 32
        ConvoFXLite regression renders sample-exact.
  - [ ] Verify original live IR and damping transitions and live latency
        changes, retained editions that fail under Wine, the original
        editor and native Windows and macOS hosts. Keep IR redistribution
        provenance open. The port reads the original IRs from a local path.
- [ ] [ConvoFXLite](../vst3go-plugins/plugins/reverb/convofxlite/README.md)
  - [x] Compare the ten local release IR pairs, finite settings and tails.
        Reopen settings and verify three latency orders in REAPER on Linux.
  - [ ] Verify the custom editor and live latency changes, and resolve asset
        redistribution provenance and host checks on other platforms.
- [ ] [Dynamic Convolution Plugin](../vst3go-plugins/plugins/reverb/dynamic-convolution-plugin/README.md)
  - [x] Identify the January 2005 ZIP, installer, and separate retained
        profiler DLL. Match 246 original renders sample-exact, including
        84 original profile imports, and verify four Linux REAPER recalls,
        both SDK configurations, and Windows x64 cross-build.
  - [ ] Resolve the discontinued release's distribution permissions and
        verify the custom editor and native Windows and macOS hosts.
- [ ] [Freeverb](../vst3go-plugins/plugins/reverb/freeverb/testdata/reference/CAPTURE.md)
  - [x] Port the 2008 ZIP and 2009 release classes. Replay 567 comparable
        windows from six Win32 originals, with 198 for the two released
        classes within 9.32e-10. Match 12 timed Freeze and damp cases
        sample-exact. Verify eight Linux REAPER reopens, both SDK classes,
        and Windows x64 cross-build. Use algo-dsp PR #61.
  - [ ] Resolve original x64 no-write and load failures, the native editor,
        and physical Windows and macOS hosts. The port initializes resized
        delay history and caps damping where the original emits non-finite
        audio.
- [ ] [Plate Reverb](../vst3go-plugins/plugins/reverb/plate-reverb/PORT.md)
  - [x] Port all three modulation variants. Match 388 stereo captures
        from four originals within 2.84e-7, including automation,
        rate changes, activation and in-place buffers. Verify six
        Linux REAPER reopens at the tank clock, 141 SDK checks, and the
        Windows x64 cross-build.
  - [ ] Verify original editor interaction and native Windows and macOS
        hosts. The original double callbacks leave buffers untouched.
- [ ] [StkReverb](../vst3go-plugins/plugins/reverb/stkreverb/PORT.md)
  - [x] Match all five algorithms on 420 installer, retained, in-place
        and pre-open captures within 1.193e-7. Confirm stereo tap timing and
        tails, valid setup ordering, and active and inactive program edits
        in a reopened Linux REAPER project. Pass SDK checks and Windows
        x64 cross-build.
  - [ ] Verify the original editors, high-rate pre-open failure, original
        double processing, and physical Windows and macOS hosts.

#### Spectral processing

- [ ] [Noise Reduction](../vst3go-plugins/plugins/spectral/noise-reduction/README.md)
  - [x] Identify three published containers and replay 198 original
        captures. Port the audible Win32 edition, including training,
        22 windows and four editable slots. Verify 84 aligned Go
        comparisons, 42 exact VST3 block pairs, six direct ABI renders,
        three Linux REAPER reopens at 24,576 samples of compensation,
        SDK checks and Windows x64 cross-build.
  - [ ] Resolve exact trained-signal parity, including the Rectangle
        case at 0.02644 peak and 3.58% relative RMS. Verify the distinct
        retained Win32 response, the original Win64 no-write callbacks,
        the original editor and native Windows and macOS hosts. Resolve
        redistribution permission for the original editor artwork.
- [ ] [Spectral Gate](../vst3go-plugins/plugins/spectral/spectral-gate/PORT.md)
  - [x] Match 114 original captures at recorded tolerances. Verify all
        22 windows, seven controls, fixed latency, stereo isolation, Linux
        REAPER state, the SDK class, and Windows x64 cross-build. Use
        the measured gain and detector timing from algo-dsp PRs #62 and #63.
  - [ ] Resolve live FFT-order transitions and the quantified fast-control
        and mixed DC/tone differences. Verify the original custom editor
        and physical Windows and macOS hosts.

### 7.4 Spatial processing, instruments and MIDI

- [x] Check channel order, matrix coefficients, HRTF selection and IRs.
      Reproduce instrument note handling, velocity, voice allocation, sample
      loading, tuning and envelopes. Check MIDI behavior at block edges.
      Edition-specific source, editor and host limits are recorded below.

#### Spatial processing

- [ ] [Digital Crosstalk Cancellator (DCC)](../vst3go-plugins/plugins/spatial/digital-crosstalk-cancellator-dcc/PORT.md)
  - [x] Identify the matching 2007 installer and retained Win32 DLL.
        Verify independent Left and Right editor controls, zero latency,
        and 36 sample-exact audible captures. Reopen all four routing
        states in Linux REAPER, pass SDK checks and cross-build Windows x64.
  - [ ] Locate the original Pascal entry point and verify native editor
        and physical Windows and macOS host behavior. Original float32
        replacing and double callbacks leave the probed buffers untouched.
- [ ] [DualWideBug](../vst3go-plugins/plugins/spatial/dualwidebug/README.md)
  - [x] Port the published and retained FIRs with respective 27- and
        22-sample dry delays. Match 216 original captures within 2.981e-8.
        Verify three editable programs, five Linux REAPER reopens, both
        SDK classes, and Windows x64 cross-build.
  - [ ] Verify the retained Win32 host-setter discrepancy on native hosts,
        original editor and artwork, and physical Windows and macOS hosts.
- [ ] [Headphone Mix](../vst3go-plugins/plugins/spatial/headphone-mix/PORT.md)
  - [x] Verify stereo crossfeed in the installer and right-input duplication
        in the retained Win32 and Win64 editions. Match 288 captures within
        5.961e-8. Verify three models and polarity in four Linux REAPER
        reopens, both SDK classes, and Windows x64 cross-build.
  - [ ] Resolve the older retained Win32 host-setter behavior and verify
        physical Windows and macOS hosts.
- [ ] [HRTF 3D](../vst3go-plugins/plugins/spatial/hrtf-3d/PORT.md)
  - [x] Verify the 2007, 2009 and retained legacy classes against external
        checksum-verified impulse sets. Match 67 original captures both
        in-place and out-of-place within 3.726e-8. Reopen six Linux REAPER
        projects, verify 64- and 512-sample compensation, pass 141 SDK
        checks and cross-build Windows x64. Live direction changes use
        [algo-dsp PR #64](https://github.com/CWBudde/algo-dsp/pull/64).
  - [ ] Verify the later VSTPack Win32 and Win64 editions, which fail
        during Wine entry, and native editors and hosts.
- [ ] [PhaseBug](../vst3go-plugins/plugins/spatial/phasebug/PORT.md)
  - [x] Port the stereo and mono installer classes. Compare 84 installer
        captures within 2.43e-7 and the 84 collection captures within
        3.78e-5. Reopen eight Linux REAPER projects with 3,072-sample
        compensation, pass 94 SDK checks and cross-build Windows x64.
  - [ ] Verify the original 316 × 309 editor and ring interactions,
        native Windows and macOS hosts, and old project migration.
- [ ] [Quadrophonics](../vst3go-plugins/plugins/spatial/quadrophonics/PORT.md)
  - [x] SQ encode and decode. Match 60 published references within
        2.981e-7 and verify the 768-sample compensated quad/stereo routing.
  - [x] QS encode and retained decode. Match 60 audible captures within
        3.577e-7. Verify the encoder's 768-sample compensation and the
        retained decoder's explicit zero-latency four-channel matrix.
        Reopen all four classes in Linux REAPER, pass 188 SDK checks
        and cross-build Windows x64.
  - [ ] Verify native hosts, the static original editor and the alternate
        retained SQ decoder. Both published QS decode callbacks leave all
        probed output buffers untouched.
- [ ] [Quadropolis](../vst3go-plugins/plugins/spatial/quadropolis/PORT.md)
  - [x] Verify the published 2004 right-ear-only and 2010 binaural classes
        using external checksum-verified HRTFs. Match 270 reference
        captures within 5.018e-8 and verify 512-sample compensation in
        both Linux REAPER reopens. Pass 94 SDK checks and cross-build
        Windows x64.
  - [ ] Verify native editors, physical ASIO and native hosts. Both retained
        VSTPack binaries fail during ordinary Wine effOpen.
- [ ] [Simple Crosstalk Cancellation](../vst3go-plugins/plugins/spatial/simple-crosstalk-cancellation/PORT.md)
  - [x] Match 432 float32 captures from the published Win32 and retained
        Win32 and Win64 DLLs within 4.173e-7. Verify 18 original double
        captures against the widened float32 model, two editable programs,
        four Linux REAPER reopens, 47 SDK checks and the Windows x64 build.
  - [ ] Verify the original 665 × 281 editor and native Windows and macOS
        hosts. The VST3 runtime exposes float32 audio.
- [ ] [Spinbug](../vst3go-plugins/plugins/spatial/spinbug/PORT.md)
  - [x] Port the public installer, BetaBugs collection and retained
        loose classes. Match 840 complete finite renders and six
        prefixes within 7.898e-7, verify timed changes in the ABI and
        four Linux REAPER reopens, pass 141 SDK checks and cross-build
        Windows x64. Recheck Spin Bug Lite after sharing its cascade.
  - [ ] Verify the retained installer's non-finite Old One behavior
        on a native host and reproduce the original editor and native
        Windows and macOS host sessions.
- [ ] [Surround to Stereo](../vst3go-plugins/plugins/spatial/surround-to-stereo/PORT.md)
  - [x] Port the 2005 and 2006 six-input editions using five local
        checksum-verified profiles. Match 134 unchanged release captures
        and 32 profile comparisons within 1.491e-8. Verify 512-sample
        compensation in three Linux REAPER reopens, 94 SDK checks and
        the Windows x64 cross-build.
  - [ ] Verify native host and editor behavior and custom HRTF import.
        The 2006 original changes the displayed program while keeping
        Setup 1 active. The port applies the selected local setup.
- [ ] [Tina-Turner](../vst3go-plugins/plugins/spatial/tina-turner/PORT.md)
  - [x] Identify the single-Angle 2005 installer. Match 252 original
        replacements and accumulating captures within 4.553e-21. Verify
        untouched silence and explicit 0° across three Linux REAPER
        project reopens, 47 SDK checks and the Windows x64 cross-build.
  - [ ] Verify the original 245 × 269 editor, native Windows and macOS
        hosts, and the physical crossed rear-speaker wiring.

#### Instruments and vocoders

- [ ] [2-Oszillator Synth](../vst3go-plugins/plugins/synth/2-oszillator-synth/PORT.md)
  - [x] Verify the byte-identical 2009 installer and archive. Match
        240 deterministic original renders and 36 direct VST3 ABI
        renders within 6.706e-8, with separate RMS comparisons for noise.
        Reopen four Linux REAPER projects and verify timestamped MIDI,
        inactive factory slot edits, 47 SDK checks and Windows x64.
  - [ ] Verify the original 498 × 189 editor and native Windows and macOS
        hosts. The original double callbacks leave the buffers untouched.
- [ ] [FunKEYS](../vst3go-plugins/plugins/synth/funkeys/PORT.md)
  - [x] Identify the byte-identical website and retained archive. Port
        the retained Wurley voice with local checksum-verified wavetables,
        eight-voice stealing and sample-offset CC126. Verify the reopened
        Linux REAPER MIDI render, 47 SDK checks and Windows x64 cross-build.
  - [ ] Verify published audio parity and the original editor on native
        hosts. Both Win32 callbacks raise a range exception at the second
        output, including on the first silent block under Wine.
- [ ] [Simple Sampler](../vst3go-plugins/plugins/synth/simple-sampler/PORT.md)
  - [x] Match 18 published Win32 and 12 VST3 ABI renders bit-for-bit.
        Reopen the sine and loaded WAV sessions in Linux REAPER after
        moving the source WAV. Verify the file picker, persisted sample,
        47 SDK checks and Windows x64 cross-build.
  - [ ] Verify the retained Win64 callback and native hosts, original
        keyboard and full format support. AIFF is silent in the original
        Wine capture and AU selection stalled.
- [ ] [Simple Sine Synth](../vst3go-plugins/plugins/synth/simple-sine-synth/README.md)
  - [x] Measure both 2009 binary releases. Port the audible oscillator from
        the retained Delphi source and verify MIDI, keyboard and state on Linux.
  - [x] Verify CC126 at its sample offset in the ABI and a reopened REAPER
        session. Confirm that CC123 leaves held notes active.
  - [ ] Verify native Windows and macOS editors.
- [ ] [Vocoder](../vst3go-plugins/plugins/synth/vocoder/PORT.md)
  - [x] Recover the published 32-band filter bank and MIDI carrier.
        Match 54 captures within 1.193e-7 and verify the dry and wet
        Linux REAPER reopens, CC126, 47 SDK checks and Windows x64.
  - [ ] Verify the original editor and native Windows and macOS hosts.
        The later Win32 and Win64 builds raise Delphi exceptions
        before providing a descriptor under Wine.

### 7.5 Analysis, utilities, educational, hosting and other plugins

- [ ] Validate meter scales, detector timing and readouts against known
      signals. Exercise actual host/external I/O, scripts, GUIs and interactive
      behavior as applicable. Record host or hardware dependencies per product.
  - [x] Compare the working release paths against original meters, audio,
        event and script traces. Check Linux REAPER recall and run saved-state
        and editor probes where the product provides those interfaces.
  - [ ] Verify physical ASIO I/O, native Windows and macOS DAWs, and the
        original controls and assets listed in the product notes.

#### Analysis

- [ ] [Barberpole Tuner](../vst3go-plugins/plugins/analysis/barberpole-tuner/PORT.md)
  - [x] Match 384 published mono captures within 3.726e-9 and compare
        the 256-column phase history. Verify the 274 × 90 X11 strip,
        H-string gesture and two reopened REAPER beat magnitudes,
        47 SDK checks and Windows x64 cross-build.
  - [ ] Verify retained DLLs that timed out in Wine, native Windows
        and macOS hosts, and the original textured editor and fonts.
- [ ] [BetaMeta](../vst3go-plugins/plugins/analysis/betameta/PORT.md)
  - [x] Identify the 2005 Win32 release and archived copies. Match 72
        stereo passthrough cases, 63 calibrated positions on each meter,
        and 16 RMS steps. Exercise the 454 × 187 X11 view and reopen
        Peak and RMS projects in Linux REAPER with exact audio, 47 SDK
        checks and a Windows x64 cross-build.
  - [ ] Verify original skins and native Windows and macOS meter views.
- [ ] [StereoMeterVST](../vst3go-plugins/plugins/analysis/stereometervst/PORT.md)
  - [x] Identify the website release and the misnamed retained
        Surround to Stereo payload. Verify 252 stereo captures, visible
        program settings, four plot directions and stretch in the
        original editor. Reopen a Linux REAPER session with the
        200 × 220 X11 plot, 47 SDK checks and Windows x64 build.
  - [ ] Verify native Windows and macOS views, original pixel smoothing,
        artwork, blend and alternate update modes.
- [ ] [Third-Octave Analyser (VST-Plugin)](../vst3go-plugins/plugins/analysis/third-octave-analyser-vst-plugin/PORT.md)
  - [x] Port the published and retained measurement paths and compare
        90 captures. Published readouts match exactly. Retained Win64
        matches within 0.00271 dB. Verify both Linux X11 views and saved
        settings, 94 SDK checks and Windows x64 cross-build.
  - [ ] Verify native live chart motion, Windows and macOS views, and
        the retained Win32 floor and decay discrepancy.
- [ ] [TunaBUG](../vst3go-plugins/plugins/analysis/tunabug/PORT.md)
  - [x] Verify 90 exact audio passthrough cases and 96 original tuner
        positions within two pixels. Exercise both custom controls and
        the live 300 × 192 X11 needle in a reopened Linux REAPER session,
        pass 47 SDK checks and cross-build Windows x64.
  - [ ] Verify the 96 kHz low-string case that faults in Wine, native
        Windows and macOS hosts, standalone ASIO and Winamp.

#### Utilities and MIDI

- [ ] [Audio2Midi Trigger](../vst3go-plugins/plugins/utility/audio2midi-trigger/PORT.md)
  - [x] Compare 98 original audio and MIDI cases. Preserve duplicate
        ordered Note Ons at offset zero, including zero velocity, and
        verify note and velocity recall in REAPER. Pass 47 SDK checks and
        cross-build Windows x64.
  - [ ] Verify native Windows and macOS hosts. The published Win64 VST2
        callback left output untouched and emitted no events under Wine.
- [ ] [Dither & Noiseshaper](../vst3go-plugins/plugins/utility/dither-noiseshaper/PORT.md)
  - [x] Port the published IIR and retained Win32 IIR and FIR editions.
        Verify 552 original captures, 12 sample-exact seeded sequences
        and three sample-exact REAPER reopens. Pass 141 SDK checks and
        cross-build Windows x64.
  - [ ] Verify native Windows and macOS hosts and the retained Win64 FIR
        no-write behavior outside Wine. Reconcile the recorded rounding
        differences in the retained IIR binaries.
- [ ] [PhaseAdjustment](../vst3go-plugins/plugins/utility/phaseadjustment/PORT.md)
  - [x] Port the working retained Win32 edition with six editable slots
        and measured latency of 1,152 samples. Match 84 reference captures
        and five compensated REAPER reopens. Pass 47 SDK checks and
        cross-build Windows x64.
  - [ ] Recover the published Win64 and other retained DSP paths that
        failed before the descriptor under Wine. Verify custom graphics
        and native Windows and macOS hosts.
- [ ] [VST Host Opcode Logger](../vst3go-plugins/plugins/utility/vst-host-opcode-logger/PORT.md)
  - [x] Identify the released DVOL and retained DVOl editions. Check
        108 original float32 buffers and three double callback probes,
        dated local logs, and ordered VST3 lifecycle and parameter calls.
        Reopen both Linux REAPER projects, exercise the 374 × 277 X11
        slider and readout, pass 94 SDK checks and cross-build Windows x64.
  - [ ] Verify native Windows and macOS views and the retained Win32
        audio capture failure. Published and retained Win64 VST2
        buffers remain untouched. VST3 provides silent output.
- [ ] [VST Pascal Script](../vst3go-plugins/plugins/utility/vst-pascal-script/PORT.md)
  - [x] Compare 90 compiled-script captures across five original files.
        Verify the Linux X11 chooser and compile controls and two
        sample-exact REAPER reopens after moving the source. Pass
        47 SDK checks and cross-build Windows x64.
  - [x] Verify dry-state recall in an open view. Destroy the X11 parent
        before the child and verify timer cleanup, resize rejection and
        reattachment in a C host.
  - [ ] Verify broader Delphi PascalScript syntax, original edit widget
        styling, original VST2 chunk import and native Windows and macOS.
        VST3 recompiles saved source on recall. The original stayed dry
        after chunk restore until recompilation.

#### Hosting

- [ ] [VST interfaced ASIO-Host](../vst3go-plugins/plugins/hosting/vst-interfaced-asio-host/PORT.md)
  - [x] Identify five distributed editions. Verify the published Wine
        editor, 45 latency cases and a software stereo exchange with
        zero callback allocations.
  - [ ] Establish a working native Windows driver and physical loopback
        before registering a VST3 class. One retained driver rejects
        buffer creation. The other starts in Wine without delivering
        any callbacks.

#### Other effects

- [ ] [Ather](../vst3go-plugins/plugins/misc/ather/PORT.md)
  - [x] Verify four measured editions and 92 original captures, including
        release-specific Preview and gain. Match 12 VST3 automation traces
        within 1.49e-8 and four REAPER renders sample-for-sample. Pass 188
        SDK checks and cross-build the Windows HWND editor.
  - [ ] Verify native Windows and macOS DAWs and original skin details.
- [ ] [Audio Amelioration](../vst3go-plugins/plugins/misc/audio-amelioration/PORT.md)
  - [x] Compare 550 original captures and port the published and retained
        chains. Match published audio within 3.58e-7 and retained Win64
        within 1.63e-5. Reopen speaker and headphone projects in REAPER
        with exact audio. Pass 94 SDK checks and cross-build Windows x64.
  - [ ] Verify the retained Win32 ambience clipping in native Windows and
        the original display and controls on native Windows and macOS.

#### Educational processors

- [ ] [ChunkDemo](../vst3go-plugins/plugins/education/chunkdemo/PORT.md)
  - [x] Match 198 published and retained renders within 2.87e-7. Verify
        the shared granular core against its 486 existing references.
        Restore four edited controls and Preset 2 in Linux REAPER. Pass
        47 SDK checks and cross-build Windows x64.
  - [ ] Verify the original 285 × 123 four-dial editor and native Windows
        and macOS DAWs. The port saves active controls. The original
        echoed preset chunks without updating its live DSP.
- [ ] [Compander](../vst3go-plugins/plugins/education/compander/PORT.md)
  - [x] Match 360 original buffers bit for bit. Verify both quantisation
        modes and the native 611 × 226 controls in Linux REAPER reopens
        and in a Win64 HWND fixture under Wine. Pass 47 SDK checks.
  - [ ] Verify the original artwork, HiDPI scaling and physical Windows
        and macOS DAWs.
- [ ] [Sigma Delta](../vst3go-plugins/plugins/education/sigma-delta/README.md)

#### Games

- [ ] Tetris
  - [x] Identify the published source and the website archives
        `VST/Tetris.zip` and `VST/7z/Tetris.7z`. The
        catalog identifies this as a clone of the existing game, and
        `TetrisUnit.pas` credits its game core to Arnaud Celermajer.
  - [ ] A faithful recreation of the existing Tetris game remains
        outside the implementation scope. No VST3 game class is registered.

### 7.6 Acceptance for every published class

- [ ] Trace the original Delphi project, processor, form resources and
      factory programs for each edition. Compare each `algo-*` candidate against
      the original topology and coefficients. Implement missing reusable DSP
      upstream and verify it against reference vectors.
  - [x] Reconcile the 136 product entries with the published and retained
        evidence in their port notes. Record measured source identities,
        usable editions and upstream dependencies, including the pinned
        algo-dsp revisions through PR #64.
  - [ ] Resolve the separately listed source, licensed reference and
        author permissions where the retained evidence is incomplete.
- [ ] Capture original renders with source version, input, preset, rate, block
      size, routing and latency alignment. Set tolerances per measured behavior.
      Compare null residuals for deterministic paths, and use spectral/time
      measures and listening checks where exact cancellation is inappropriate.
  - [x] Record capture and replay conditions, per-edition comparisons and
        selected deterministic, spectral and timing measurements in the
        product notes and reference tests.
  - [ ] Repeat the original cases that faulted, returned nonfinite samples
        or left buffers untouched on the native platforms listed by product.
- [ ] Exercise silence, tails, bypass, automation, reload and non-finite
      output checks at multiple supported rates and block sizes. Measure audio
      callback allocations and CPU at the most demanding supported setting.
  - [x] Run the combined runtime race, C host and SDK suite, vet and lint.
        Verify Windows x64 exports. Run the companion tagged race and vet
        suite with the recent captured reference sets.
  - [x] Measure the available Linux companion audio benchmarks. The
        measured callback cases report zero allocations.
  - [ ] Fill the remaining class-specific stress and worst-case performance
        coverage, including settings and hosts with open parity gates.
- [ ] Compare the original and ported editor at matched size and parameter
      state, including meters, interaction, readouts and HiDPI behavior. Record
      validation results for each class and supported host platform.
  - [x] Record Linux REAPER and isolated Wine checks with their tested
        editors, programs, host versions and original pixel dimensions.
  - [ ] Complete the remaining native Windows and macOS editor comparisons
        and hardware-dependent checks listed by product.
- [x] Document known unresolved source identity, original assets, reference
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
