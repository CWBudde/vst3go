// Package plugin provides the runtime-facing VST3 plugin interfaces.
//
// This package is the contract consumed by the VST3 wrapper/runtime layer:
// callers implement [Plugin], which produces a [Processor], and may optionally
// implement [StatefulProcessor] when parameter state alone is not sufficient.
//
// # Lifecycle
//
// The host drives the following order. The left column is the VST3 call, the
// right column the hook this package invokes:
//
//	IPluginFactory::createInstance   Plugin.CreateProcessor
//	IComponent::initialize           Processor.Initialize(48000, maxBlockSize)
//	IAudioProcessor::setupProcessing Processor.Initialize(sampleRate, maxBlockSize)
//	IComponent::setActive(true)      Processor.SetActive(true)
//	IAudioProcessor::process         Processor.ProcessAudio(ctx)   (repeatedly)
//	IComponent::setActive(false)     Processor.SetActive(false)
//	IComponent::terminate            (no hook)
//
// Two consequences of that table are easy to miss:
//
// Initialize is called more than once, and the first call is a lie. The host
// has not announced a sample rate at IComponent::initialize, so the runtime
// passes a 48000 placeholder; the authoritative sample rate and block size
// arrive with setupProcessing, which calls Initialize again. Initialize must
// therefore be idempotent and must recompute anything rate-dependent on every
// call rather than only on the first.
//
// There is no Terminate hook. A Processor releases nothing explicitly: it is
// garbage collected with the component. Work that must stop when the host
// stops belongs in SetActive(false), which the runtime also uses to reset the
// process context's pending parameter changes and event queues.
//
// The host may call state save/load (IComponent::getState / setState) at any
// point after initialize, including while the plugin is active.
//
// # Which hook may allocate
//
// Initialize and SetActive run on the host's UI or scanner thread and may
// allocate freely — that is where buffers, tables and any per-rate state
// should be built.
//
// ProcessAudio runs on the audio thread and must not allocate, lock, block, or
// call into the host. Everything it needs must already exist by the time
// SetActive(true) returns. The repository's own tests pin this with
// testing.AllocsPerRun over the whole runtime process path; a downstream
// processor is expected to hold the same line for its DSP (see
// docs/validation.md).
//
// # Threading
//
// Three threads reach a Processor:
//
//   - The audio thread calls only ProcessAudio.
//   - The host's UI/control thread calls Initialize, SetActive,
//     GetParameters, GetBuses, GetLatencySamples, GetTailSamples, and the
//     [StatefulProcessor] save/load hooks.
//   - The host's scanner thread may query metadata (GetInfo, GetParameters,
//     GetBuses) before anything is active.
//
// The runtime serializes lifecycle calls against each other with a mutex and
// publishes everything the audio callback reads through atomics, so
// ProcessAudio never blocks on a lifecycle call. It does not, however,
// serialize a Processor's own fields: any state a processor mutates from both
// SetActive and ProcessAudio is the processor's own to synchronize, and the
// audio side of that must stay lock-free.
//
// Parameter values are the supported way to cross the boundary: [param.Parameter]
// stores its value atomically, so the control thread may write while the audio
// thread reads. See the pkg/framework/param and pkg/framework/process package
// documentation for the per-type rules.
package plugin

import (
	"io"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/plugin"
	"github.com/cwbudde/vst3go/pkg/framework/process"
)

// Plugin is the top-level runtime interface implemented by a plugin entrypoint.
//
// GetInfo returns the metadata shared with the host, while CreateProcessor
// constructs a fresh Processor instance for actual audio/event handling.
type Plugin interface {
	// GetInfo returns plugin metadata
	GetInfo() plugin.Info

	// CreateProcessor creates a new instance of the audio processor
	CreateProcessor() Processor
}

// Processor is the core runtime contract consumed by the VST3 wrapper.
//
// A Processor owns:
//   - initialization and activation lifecycle
//   - audio/event processing via process.Context
//   - parameter and bus configuration
//   - latency/tail reporting
//
// The wrapper/runtime creates a Processor from Plugin.CreateProcessor and
// delegates all host-facing lifecycle and processing calls through it.
type Processor interface {
	// Initialize prepares the processor for the given sample rate and block
	// size. It is called once at component initialization with a placeholder
	// sample rate and again from setupProcessing with the host's real values,
	// so it must be idempotent. Control thread; may allocate.
	Initialize(sampleRate float64, maxBlockSize int32) error

	// ProcessAudio processes one block. Audio thread: no allocation, no
	// locking, no blocking, no calls into the host.
	ProcessAudio(ctx *process.Context)

	// GetParameters returns the parameter registry. The registry itself must
	// not change after Initialize; individual parameter values may.
	GetParameters() *param.Registry

	// GetBuses returns the bus configuration
	GetBuses() *bus.Configuration

	// SetActive is called when the host activates or deactivates the plugin.
	// Control thread; may allocate. SetActive(false) is the place to stop and
	// release anything the audio thread was using.
	SetActive(active bool) error

	// GetLatencySamples returns the plugin's latency in samples
	GetLatencySamples() int32

	// GetTailSamples returns the tail length in samples
	GetTailSamples() int32
}

// ProcessingStateProcessor receives the host's processing start and stop
// separately from component activation. A processor hosting a nested plugin
// can forward these transitions to it. The host may call SetProcessing on
// its audio thread. Avoid allocation and blocking on this path.
type ProcessingStateProcessor interface {
	Processor
	SetProcessing(processing bool) error
}

// TerminatingProcessor releases resources that must live across activation
// cycles, such as a loaded detector module. Terminate runs on the host's
// control thread during component termination or final release. It may run
// after partial initialization. The runtime retries failures at final release
// and invokes no further termination callbacks after success.
type TerminatingProcessor interface {
	Processor
	Terminate() error
}

// LatencyChangeProcessor is an optional extension for processors that can
// change their latency while a plug-in is in use. The runtime supplies one
// notifier per instance before initialization. Call it on the host UI thread
// after changing a setting that affects delay compensation. The host can
// synchronously deactivate and reactivate the processor, so release any
// processor locks first. Never call it from ProcessAudio.
//
// The runtime also detects latency changes for these processors during state
// restoration. A host-driven processing setup lets the host query latency.
type LatencyChangeProcessor interface {
	Processor
	SetLatencyChangeNotifier(notify func())
}

// ControllerParameterProcessor receives direct controller and native-editor
// parameter writes after the runtime stores the clamped normalized value.
// Repeated writes are delivered even when the value stays the same.
//
// This runs on the host UI thread and may allocate. It can run during
// playback, so publish any state ProcessAudio uses without blocking that
// thread. Sample-offset queues, program selectors, and state restoration use
// their existing processing and state hooks.
type ControllerParameterProcessor interface {
	Processor
	ControllerParameterChanged(id uint32, normalized float64)
}

// StatefulProcessor extends Processor with custom state save/load hooks.
//
// Implement this only when parameter state is insufficient to restore runtime
// behavior, for example when additional processor-owned state must be persisted.
//
// Both hooks run on the host's control thread and may allocate. They may be
// called while the plugin is active, so a processor that shares state with the
// audio thread must publish it safely rather than reading it raw.
//
// The payload written here is opaque to the runtime and is the plugin's own
// versioning problem: the preset format frames it with a length prefix but
// says nothing about its contents. See the pkg/framework/state package
// documentation for what that guarantees and what it does not.
type StatefulProcessor interface {
	Processor

	// SaveCustomState saves additional state beyond parameters
	// This is called after all parameters have been saved
	SaveCustomState(w io.Writer) error

	// LoadCustomState loads additional state beyond parameters
	// This is called after all parameters have been loaded
	LoadCustomState(r io.Reader) error
}
