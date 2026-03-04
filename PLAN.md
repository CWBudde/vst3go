# VST3Go Plan

## Purpose

This file is the single planning document for VST3Go. It consolidates the previous roadmap, cleanup notes, Windows support plan, synth roadmap, and pattern documentation into one phased plan.

As of the repo split, `vst3go` owns the VST3 binding/runtime layer. The DSP library, showcase plugins, and synth-focused roadmap moved to the companion `synthkit` repository.

## Current Baseline

### Completed Foundation

- Minimal C bridge architecture is in place, and plugins use `pkg/plugin/cbridge` instead of direct CGO imports.
- Core framework packages exist for plugin lifecycle, parameters, process context, buses, state, and MIDI/events.
- Zero-allocation processing is a project-wide requirement and the major known audio-path allocations called out in prior cleanup notes were removed.
- Sample-accurate parameter automation and MIDI/event handling are implemented in the framework.
- Advanced bus support is implemented.
- Parameter ID usage has been standardized around typed `uint32` constants with `ParamXxx` naming.
- Most plugins now reset runtime state correctly when deactivated.

### Companion Repo Boundary

The following areas are no longer planned inside `vst3go`:

- DSP packages
- voice-management helpers intended for synth engines
- showcase effect and instrument plugins
- MassiveSynth and related synth-product work

Those now belong in `synthkit`.

### Completed Work To Keep Brief

- Audio-path allocation cleanup in examples.
- Removal of production debug prints from known hot paths.
- C bridge abstraction cleanup.
- Parameter pattern cleanup and documentation.
- Most hardcoded sample-rate and DSP constants replaced with named values.

## Principles

1. Minimal C bridge, with framework internals hidden from plugin authors.
2. Zero allocations in real-time audio paths.
3. Idiomatic Go APIs over C++-style wrappers.
4. Predictable plugin lifecycle, state handling, and host interoperability.
5. Strong developer experience for the common plugin cases.

## Guardrails

- Keep the C bridge minimal.
  - No business logic in C.
  - No framework features in the bridge layer.
  - Keep the mapping from VST3 C API to Go direct and easy to reason about.
- Keep the Go framework rich, but not rigid.
  - Provide conveniences, builders, templates, and helpers.
  - Preserve escape hatches to lower layers when advanced use cases require them.
- Do not duplicate framework logic in examples.
  - Parameter boilerplate, state handling, event handling patterns, and common DSP belong in packages, not in each plugin.
  - Examples should focus on plugin-specific behavior and serve as canonical usage references.
- Prefer complete implementations over placeholders.
  - Unsupported extensions should be omitted or explicitly return unsupported behavior, not partial TODO scaffolding.
- Prefer direct refactors over compatibility cruft for internal architecture work.
  - Use aliases or migration notes only where they materially help users.
  - Avoid preserving broken patterns just for the sake of internal compatibility.
- Use `just`-based workflows and keep validator-based testing in the release process.
- Keep all examples buildable and representative of the current best practice.

## Deferred Items

- GUI support remains deferred until explicitly prioritized.
- Broad SIMD work should not start before a measured hotspot review.
- Large DSP-chain rewrites should only happen where profiling shows a real benefit.

## Phase 1: Architecture Hardening And Cleanup

### Goal

Close the known architectural gaps so the framework is internally consistent and examples model the right patterns.

### Tasks

- Strengthen processor state persistence.
  - Audit plugins that should implement `StatefulProcessor`.
  - Add save/load coverage for delay state, filter state, synth state, and presets where appropriate.
  - Add regression tests for preset recall and DAW project restore.
- Resolve API inconsistencies with explicit canonical patterns.
  - Eliminate duplicate constant spellings and casing variants.
  - Standardize normalized/plain parameter naming semantics.
  - Standardize package-function versus method usage by domain:
    - pure value transforms as package functions
    - stateful DSP as methods on types
    - builders as fluent methods
  - Standardize bus configuration around clear primary factory APIs, with builders for complex layouts.
- Tighten initialization and error handling.
  - Audit constructors and initialization paths for silent failures.
  - Return actionable wrapped errors from DSP and plugin setup code.
  - Document expected failure surfaces for plugin authors.
  - Replace silent duplicate handling and panic-prone behavior where normal error returns are more appropriate.
- Finish lifecycle cleanup.
  - Audit `SetActive(false)` implementations across examples and framework wrappers.
  - Ensure DSP state, temporary buffers, event queues, and voice state reset deterministically.
  - Evaluate a reusable `BaseProcessor` and state-reset helpers if they reduce repeated lifecycle code without hiding core concepts too aggressively.
- Clean up process-layer ergonomics.
  - Refactor oversized processing functions, especially in `pkg/plugin/component.go`.
  - Extract transport updates, buffer mapping, and event handling into smaller units.
  - Revisit sample-accurate sub-buffer processing for reuse and clarity.
- Harden parameter access patterns.
  - Audit unsafe `params.Get(...).SetValue(...)` call sites.
  - Prefer checked access or helper APIs where nil is possible.
  - Keep typed parameter IDs and canonical naming as the only accepted pattern.
- Remove remaining magic numbers where they obscure intent.
  - Replace ad hoc musical values, channel counts, and buffer constants with named values.
  - Centralize default process constants where shared behavior matters.
- Reassess process helpers versus direct loops.
  - Identify where `process.Context` helpers improve clarity without harming hot-path performance.
  - Keep manual loops where they are clearer or measurably faster.
- Decide, don’t assume, on low-level optimizations.
  - Profile DSP-chain ordering ideas before refactoring.
  - Profile SIMD candidates before adding platform-specific complexity.
- Clarify package layering and ownership.
  - Reassess whether bridge-related code is cleanly separated from Go framework abstractions.
  - Remove mixed-responsibility packages where Layer 1 and higher-level concerns are entangled.
  - Document the interface hierarchy for plugin, processor, and optional stateful behavior.

## Phase 2: Quality, Validation, And Performance Guardrails

### Goal

Turn the current implementation into a repeatable, measurable engineering baseline.

### Tasks

- Expand automated test coverage.
  - Unit tests for DSP edge cases and parameter/state behavior.
  - Integration tests for host interaction, MIDI/event flow, and preset recall.
  - Cross-check sample-accurate automation behavior under dense parameter changes.
  - Add race-detector coverage where thread-safety assumptions are non-trivial.
- Strengthen performance verification.
  - Add or extend benchmarks for hot DSP paths and process-context internals.
  - Verify zero-allocation guarantees in benchmarks and debug tooling.
  - Revisit the parameter-change buffer size strategy without introducing runtime allocations.
- Improve validation workflow.
  - Keep validator-passing examples as a release gate.
  - Add documented DAW test coverage expectations for effects and instruments.
  - Define a lightweight release checklist for build, validation, and smoke testing.
- Document non-functional requirements.
  - Performance guidelines for plugin authors.
  - Thread-safety expectations.
  - Real-time safe coding rules for framework contributors and examples.
  - Validation expectations for examples and release candidates.

## Phase 3: Developer Experience And Documentation

### Goal

Make the framework easier to adopt without weakening the architecture.

### Tasks

- Build the project generator.
  - `vst3go new effect`
  - `vst3go new instrument`
  - Templates should encode the current best practices for buses, parameters, state, and bridge imports.
- Improve debugging and observability.
  - Replace ad hoc debug prints with a controlled debug/logging approach.
  - Support debug-only instrumentation that stays out of the audio path in production builds.
- Reduce recurring plugin boilerplate where it helps.
  - Evaluate base processor helpers, registry helpers, common parameter sets, and state-reset utilities.
  - Keep convenience APIs opt-in and transparent rather than magical.
- Consolidate and expand docs.
  - Plugin authoring best practices.
  - Parameter conventions and examples.
  - State/preset handling guidance.
  - Performance and architecture notes.
  - ADR-style notes for major design decisions when tradeoffs matter.
  - Interface hierarchy and lifecycle expectations.
  - Thread-safety guide.
  - Migration guidance when API cleanup renames or removes inconsistent patterns.
- Keep examples as canonical references.
  - Ensure each example demonstrates one or two patterns clearly.
  - Avoid examples depending on internal bridge details or outdated parameter styles.

## Phase 4: Generic Runtime Features For Instruments And Advanced Plugins

### Goal

Round out the framework features needed for serious instrument development and advanced plugin workflows.

### Tasks

- Mature MIDI/event infrastructure beyond the current baseline.
  - Fill any remaining event-type gaps.
  - Add MIDI output workflows for MIDI effects where needed.
  - Evaluate MIDI learn support.
  - Evaluate MPE support after the core instrument workflows are stable.
- Extend voice-management capabilities.
  - Keep runtime/event interfaces sufficient for external voice-management layers.
  - Add only the generic support that belongs in the VST3 runtime surface.
- Improve process context capabilities where justified.
  - Maintain clear event input/output APIs.
  - Ensure helper APIs are stable for instrument authors.
  - Keep transport and musical timing access easy to use and easy to test.

## Phase 5: Cross-Platform Windows Support

### Goal

Extend the current Linux/macOS focus to Windows without compromising the minimal-bridge design.

### Tasks

- Update the build system.
  - Detect Windows cleanly.
  - Produce the correct library format and bundle layout.
  - Support practical install targets for development and testing.
- Extend the bridge for Windows entry points.
  - Add the required DLL entry handling.
  - Export `GetPluginFactory`.
  - Keep Unix and Windows behavior clearly separated where needed.
- Review CGO and platform-specific build directives.
  - Add Windows-specific flags only where necessary.
  - Minimize divergence between platforms.
- Normalize path handling and scripts.
  - Prefer cross-platform path assembly.
  - Add Windows install and validator scripts if shell parity is not enough.
- Validate on real toolchains and hosts.
  - Start with MinGW-w64.
  - Keep MSVC as a later enhancement if it materially improves integration.
  - Verify exports, bundle structure, validator behavior, and DAW loading.
- Capture Windows-specific risks.
  - Shared-library behavior with Go.
  - Scheduler and real-time behavior.
  - File locking and development reload friction.
  - Path-length edge cases.

## Phase 6: Release Readiness

### Goal

Define what "ready" means for a stable public milestone.

### Tasks

- Keep validator compliance as mandatory.
- Keep zero-allocation guarantees in the audio path.
- Keep architectural guardrails enforced in examples and templates.
- Finish documentation needed for first-time adopters.
- Confirm example coverage for effect, bus, automation, and instrument workflows.
- Decide the release scope explicitly:
  - Linux/macOS only, or include Windows.
  - Audio-only, or broader feature surface.
  - MassiveSynth included, or released later as a showcase package.

## Ongoing Maintenance Rules

- Update this file when priorities or status materially change.
- When a phase is completed, move it into the baseline summary and compress it.
- Avoid creating new standalone roadmap files unless they are temporary working notes that will be merged back here.
- Before adding a framework feature to an example, check whether it belongs in a shared package instead.
- Before adding low-level abstractions, confirm they preserve access to underlying VST3 concepts.
- Before changing audio-path structure for performance reasons, measure first.
