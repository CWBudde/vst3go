package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
// #include "../../bridge/bridge.h"
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/framework/state"
	"github.com/cwbudde/vst3go/pkg/midi"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

// Sentinel errors for the process path: constructing an error there would
// allocate on the realtime thread.
var (
	errNilProcessData  = errors.New("process: nil process data")
	errSampleSize      = errors.New("process: host requested an unsupported sample size")
	errNoProcessConfig = errors.New("process: processing not set up")
)

// componentImpl wraps a Processor to implement VST3 interfaces.
//
// Everything the audio callback reads is held in an atomic so that Process
// never blocks on a lifecycle call from the host's UI or scanner thread. mu
// only serializes those lifecycle calls against each other.
type componentImpl struct {
	processor  Processor
	processCtx atomic.Pointer[process.Context]

	// sampleRateBits is the current sample rate as math.Float64bits.
	sampleRateBits atomic.Uint64
	processing     atomic.Bool

	maxBlockSize int32
	active       bool
	mu           sync.Mutex
	wrapper      *componentWrapper // Reference to wrapper for notifications

	// Audio-thread scratch. Reused across blocks so the chunk loop and the
	// event fan-out stay allocation-free.
	chunkInput  [][]float32
	chunkOutput [][]float32
	blockEvents []midi.Event

	// Out-parameters for cgo calls. Taking the address of a local would make it
	// escape to the heap on every call; a field is allocated once with the
	// component. None of these contain Go pointers, so passing them to C is safe.
	cSampleOffset C.int32_t
	cValue        C.double
	cEvent        C.struct_Steinberg_Vst_Event
}

func (c *componentImpl) parameterRegistry() (*param.Registry, error) {
	if c.processor == nil {
		return nil, fmt.Errorf("component has no processor")
	}

	params := c.processor.GetParameters()
	if params == nil {
		return nil, fmt.Errorf("processor has no parameter registry")
	}

	return params, nil
}

func (c *componentImpl) stateManager() (*state.Manager, error) {
	params, err := c.parameterRegistry()
	if err != nil {
		return nil, err
	}

	manager := state.NewManager(params)
	if stateful, ok := c.processor.(StatefulProcessor); ok {
		manager.SetCustomLoadFunc(stateful.LoadCustomState)
		manager.SetCustomSaveFunc(stateful.SaveCustomState)
	}

	return manager, nil
}

// newComponent creates a new component implementation
func newComponent(processor Processor) *componentImpl {
	params := processor.GetParameters()
	c := &componentImpl{
		processor:    processor,
		maxBlockSize: 8192,
	}
	c.processCtx.Store(process.NewContext(8192, params)) // Default max block size
	return c
}

// IComponent implementation
func (c *componentImpl) Initialize(_ interface{}) error {
	return c.processor.Initialize(48000, c.maxBlockSize) // Default sample rate
}

func (c *componentImpl) Terminate() error {
	return nil
}

func (c *componentImpl) GetControllerClassID() [16]byte {
	// Return same ID - we're a single component
	return [16]byte{}
}

func (c *componentImpl) SetIOMode(_ int32) error {
	return nil
}

func (c *componentImpl) GetBusCount(mediaType, direction int32) int32 {
	buses := c.processor.GetBuses()
	return buses.GetBusCount(bus.MediaType(mediaType), bus.Direction(direction))
}

func (c *componentImpl) GetBusInfo(mediaType, direction, index int32) (*vst3.BusInfo, error) {
	buses := c.processor.GetBuses()
	info := buses.GetBusInfo(bus.MediaType(mediaType), bus.Direction(direction), index)
	if info == nil {
		return nil, vst3.ErrNotImplemented
	}

	flags := uint32(1) // Default active
	if !info.IsActive {
		flags = 0
	}

	return &vst3.BusInfo{
		MediaType:    int32(info.MediaType),
		Direction:    int32(info.Direction),
		ChannelCount: info.ChannelCount,
		Name:         info.Name,
		BusType:      int32(info.BusType),
		Flags:        flags,
	}, nil
}

func (c *componentImpl) GetRoutingInfo(inInfo, outInfo interface{}) error {
	return vst3.ErrNotImplemented
}

func (c *componentImpl) ActivateBus(mediaType, direction, index int32, state bool) error {
	return nil
}

func (c *componentImpl) SetActive(active bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.active = active
	if ctx := c.processCtx.Load(); !active && ctx != nil {
		ctx.ResetParameterChanges()
		ctx.ClearAllEvents()
	}
	return c.processor.SetActive(active)
}

func (c *componentImpl) SetState(stateData []byte) error {
	stateManager, err := c.stateManager()
	if err != nil {
		return fmt.Errorf("set component state: %w", err)
	}

	buf := bytes.NewReader(stateData)
	if err := stateManager.Load(buf); err != nil {
		return fmt.Errorf("set component state: %w", err)
	}
	return nil
}

func (c *componentImpl) GetState() ([]byte, error) {
	stateManager, err := c.stateManager()
	if err != nil {
		return nil, fmt.Errorf("get component state: %w", err)
	}

	var buf bytes.Buffer
	if err := stateManager.Save(&buf); err != nil {
		return nil, fmt.Errorf("get component state: %w", err)
	}

	return buf.Bytes(), nil
}

// IAudioProcessor implementation
func (c *componentImpl) SetBusArrangements(inputs, outputs []int64) error {
	return nil
}

func (c *componentImpl) GetBusArrangement(direction, index int32) (int64, error) {
	// Return stereo by default
	return int64(3), nil // Left + Right
}

func (c *componentImpl) CanProcessSampleSize(symbolicSampleSize int32) error {
	// We only support 32-bit float
	if symbolicSampleSize == 0 { // kSample32
		return nil
	}
	return vst3.ErrNotImplemented
}

func (c *componentImpl) GetLatencySamples() uint32 {
	return uint32(c.processor.GetLatencySamples())
}

func (c *componentImpl) SetupProcessing(setup *vst3.ProcessSetup) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if setup == nil {
		return fmt.Errorf("setup processing: nil process setup")
	}

	c.sampleRateBits.Store(math.Float64bits(setup.SampleRate))
	if setup.MaxSamplesPerBlock > 0 {
		c.maxBlockSize = setup.MaxSamplesPerBlock
		// Recreate process context with new max block size
		params := c.processor.GetParameters()
		c.processCtx.Store(process.NewContext(int(c.maxBlockSize), params))
	}

	if err := c.processor.Initialize(setup.SampleRate, c.maxBlockSize); err != nil {
		return fmt.Errorf("setup processing: initialize processor: %w", err)
	}
	return nil
}

func (c *componentImpl) SetProcessing(state bool) error {
	c.processing.Store(state)
	return nil
}

func (c *componentImpl) Process(data unsafe.Pointer) error {
	if data == nil {
		return errNilProcessData
	}

	if !c.processing.Load() {
		return nil
	}

	ctx := c.processCtx.Load()
	if ctx == nil {
		return errNoProcessConfig
	}

	// Get raw process data struct
	processData := (*C.struct_Steinberg_Vst_ProcessData)(data)

	// In double-precision mode the host hands us double[] buffers. Reading
	// those as float[] would yield garbage and read only half the block, so
	// refuse the block rather than misinterpret it.
	if processData.symbolicSampleSize != C.Steinberg_Vst_SymbolicSampleSizes_kSample32 {
		return errSampleSize
	}

	c.prepareProcessContext(ctx)
	c.updateTransport(ctx, processData.processContext)
	c.mapAudioBuffers(ctx, processData)
	c.collectInputEvents(processData.inputEvents)
	c.collectParameterChanges(ctx, unsafe.Pointer(processData.inputParameterChanges))

	// Process audio with sample-accurate parameter automation
	if ctx.HasParameterChanges() {
		// Sort parameter changes by sample offset
		ctx.SortParameterChanges()

		// Process audio in chunks between parameter changes
		c.processSampleAccurate(ctx)
	} else {
		// No parameter changes - process entire block
		c.publishInputEvents(ctx, 0, math.MaxInt32)
		c.processor.ProcessAudio(ctx)
	}

	return nil
}

func (c *componentImpl) prepareProcessContext(ctx *process.Context) {
	ctx.SampleRate = math.Float64frombits(c.sampleRateBits.Load())
	ctx.ResetParameterChanges()
	ctx.ClearAllEvents()
	ctx.Input = ctx.Input[:0]
	ctx.Output = ctx.Output[:0]
}

func (c *componentImpl) updateTransport(processCtx *process.Context, ctx *C.struct_Steinberg_Vst_ProcessContext) {
	transport := processCtx.Transport
	*transport = process.TransportInfo{}

	if ctx == nil {
		return
	}

	transport.IsPlaying = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kPlaying) != 0
	transport.IsRecording = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kRecording) != 0
	transport.IsCycling = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kCycleActive) != 0

	transport.HasTempo = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kTempoValid) != 0
	if transport.HasTempo {
		transport.Tempo = float64(ctx.tempo)
	}

	transport.HasTimeSignature = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kTimeSigValid) != 0
	if transport.HasTimeSignature {
		transport.TimeSigNumerator = int32(ctx.timeSigNumerator)
		transport.TimeSigDenominator = int32(ctx.timeSigDenominator)
	}

	transport.HasMusicalTime = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kProjectTimeMusicValid) != 0
	if transport.HasMusicalTime {
		transport.ProjectTimeMusic = float64(ctx.projectTimeMusic)
	}

	transport.HasBarPosition = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kBarPositionValid) != 0
	if transport.HasBarPosition {
		transport.BarPositionMusic = float64(ctx.barPositionMusic)
	}

	transport.HasCycle = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kCycleValid) != 0
	if transport.HasCycle {
		transport.CycleStartMusic = float64(ctx.cycleStartMusic)
		transport.CycleEndMusic = float64(ctx.cycleEndMusic)
	}

	transport.ProjectTimeSamples = int64(ctx.projectTimeSamples)
	transport.ContinuousTimeSamples = int64(ctx.continousTimeSamples)

	if (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kClockValid) != 0 {
		transport.SamplesToNextClock = int32(ctx.samplesToNextClock)
	}
}

func (c *componentImpl) mapAudioBuffers(ctx *process.Context, processData *C.struct_Steinberg_Vst_ProcessData) {
	numSamples := int(processData.numSamples)

	if processData.numInputs > 0 && processData.inputs != nil {
		inputBuses := unsafe.Slice(processData.inputs, int(processData.numInputs))
		c.appendAudioBuffers(&ctx.Input, inputBuses, numSamples)
	}

	if processData.numOutputs > 0 && processData.outputs != nil {
		outputBuses := unsafe.Slice(processData.outputs, int(processData.numOutputs))
		c.appendAudioBuffers(&ctx.Output, outputBuses, numSamples)
	}
}

func (c *componentImpl) appendAudioBuffers(dst *[][]float32, buses []C.struct_Steinberg_Vst_AudioBusBuffers, numSamples int) {
	for i := range buses {
		bus := &buses[i]
		table := (**float32)(unsafe.Pointer(getChannelBuffers32(bus)))
		*dst = appendBusChannels(*dst, table, int(bus.numChannels), numSamples)
	}
}

// appendBusChannels maps one bus's channel pointer table onto Go slices that
// alias the host's memory. Split out from appendAudioBuffers so it is reachable
// from tests, which cgo does not allow to name C types.
func appendBusChannels(dst [][]float32, table **float32, numChannels, numSamples int) [][]float32 {
	if table == nil || numChannels <= 0 {
		return dst
	}

	channels := unsafe.Slice(table, numChannels)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		dst = append(dst, unsafe.Slice(channel, numSamples))
	}

	return dst
}

func (c *componentImpl) collectParameterChanges(ctx *process.Context, inputParameterChanges unsafe.Pointer) {
	if inputParameterChanges == nil {
		return
	}

	paramCount := C.getParameterChangeCount(inputParameterChanges)
	for i := C.int32_t(0); i < paramCount; i++ {
		paramQueue := C.getParameterData(inputParameterChanges, i)
		if paramQueue == nil {
			continue
		}

		paramID := C.getParameterId(paramQueue)
		pointCount := C.getPointCount(paramQueue)
		for j := C.int32_t(0); j < pointCount; j++ {
			result := C.getPoint(paramQueue, j, &c.cSampleOffset, &c.cValue)
			if result == 0 {
				ctx.AddParameterChange(uint32(paramID), float64(c.cValue), int(c.cSampleOffset))
			}
		}
	}
}

func (c *componentImpl) GetTailSamples() uint32 {
	return uint32(c.processor.GetTailSamples())
}

// collectInputEvents converts the host's events into blockEvents, keeping the
// host's block-absolute offsets. Publication into the process context is
// deferred so the chunk loop can hand each chunk only its own events.
func (c *componentImpl) collectInputEvents(eventList *C.struct_Steinberg_Vst_IEventList) {
	c.blockEvents = c.blockEvents[:0]

	if eventList == nil {
		return
	}

	// Get event count using helper
	eventCount := C.getEventCount(unsafe.Pointer(eventList))
	if eventCount <= 0 {
		return
	}

	for i := C.int32_t(0); i < eventCount; i++ {
		if C.getEvent(unsafe.Pointer(eventList), i, &c.cEvent) != 0 { // not kResultOk
			continue
		}
		if event, ok := convertEvent(&c.cEvent); ok {
			c.blockEvents = append(c.blockEvents, event)
		}
	}
}

// convertEvent converts a VST3 event to our MIDI event format.
func convertEvent(event *C.struct_Steinberg_Vst_Event) (midi.Event, bool) {
	// Use helper function to get event type
	switch C.getEventType(event) {
	case C.Steinberg_Vst_Event_EventTypes_kNoteOnEvent:
		noteOn := C.getNoteOnEvent(event)
		return midi.NoteOnEvent{
			BaseEvent: midi.BaseEvent{
				EventChannel: uint8(noteOn.channel),
				Offset:       int32(event.sampleOffset),
			},
			NoteNumber: uint8(noteOn.pitch),
			Velocity:   velocityToMIDI(float64(noteOn.velocity)),
		}, true

	case C.Steinberg_Vst_Event_EventTypes_kNoteOffEvent:
		noteOff := C.getNoteOffEvent(event)
		return midi.NoteOffEvent{
			BaseEvent: midi.BaseEvent{
				EventChannel: uint8(noteOff.channel),
				Offset:       int32(event.sampleOffset),
			},
			NoteNumber: uint8(noteOff.pitch),
			Velocity:   velocityToMIDI(float64(noteOff.velocity)),
		}, true

		// Add more event types as needed
	}

	return nil, false
}

// velocityToMIDI maps VST3's normalized 0..1 velocity onto MIDI's 0..127.
// Rounding rather than truncating keeps values just below 1.0 from dropping a
// step, and clamping matters because an out-of-range value from the host would
// otherwise wrap silently in the uint8 conversion.
func velocityToMIDI(velocity float64) uint8 {
	scaled := math.Round(velocity * 127)
	if !(scaled > 0) { // also catches NaN
		return 0
	}
	if scaled > 127 {
		return 127
	}
	return uint8(scaled)
}

// publishInputEvents hands the context the events falling in [start, end),
// rebased so their offsets match the chunk-relative audio buffers the user's
// DSP sees. Without the range filter a note-on would retrigger once per chunk.
func (c *componentImpl) publishInputEvents(ctx *process.Context, start, end int32) {
	ctx.ClearInputEvents()

	for _, event := range c.blockEvents {
		offset := event.SampleOffset()
		if offset < start || offset >= end {
			continue
		}
		ctx.AddInputEvent(shiftEvent(event, -start))
	}
}

// shiftEvent rebases a single event's offset. midi.EventQueue.OffsetEvents does
// the same job for a whole queue, but process.Context does not expose its
// queue, and shifting after insertion would also move events from other chunks.
// Only the types convertEvent produces need handling here.
func shiftEvent(event midi.Event, delta int32) midi.Event {
	if delta == 0 {
		return event
	}

	switch e := event.(type) {
	case midi.NoteOnEvent:
		e.Offset += delta
		return e
	case midi.NoteOffEvent:
		e.Offset += delta
		return e
	}

	return event
}

// IEditController implementation
func (c *componentImpl) SetComponentState(state []byte) error {
	return nil
}

func (c *componentImpl) GetParameterCount() int32 {
	return c.processor.GetParameters().Count()
}

func (c *componentImpl) GetParameterInfo(index int32) (*vst3.ParameterInfo, error) {
	p := c.processor.GetParameters().GetByIndex(index)
	if p == nil {
		return nil, vst3.ErrInvalidArgument
	}

	return &vst3.ParameterInfo{
		ID:           p.ID,
		Title:        p.Name,
		ShortTitle:   p.ShortName,
		Units:        p.Unit,
		StepCount:    p.StepCount,
		DefaultValue: p.DefaultValue,
		UnitID:       p.UnitID,
		Flags:        int32(p.Flags),
	}, nil
}

func (c *componentImpl) GetParamStringByValue(id uint32, value float64) (string, error) {
	if p := c.processor.GetParameters().Get(id); p != nil {
		result := p.FormatValue(value)
		// fmt.Printf("Component.GetParamStringByValue: id=%d, value=%.3f -> '%s'\n", id, value, result)
		return result, nil
	}
	return "", vst3.ErrInvalidArgument
}

func (c *componentImpl) GetParamValueByString(id uint32, str string) (float64, error) {
	if p := c.processor.GetParameters().Get(id); p != nil {
		return p.ParseValue(str)
	}
	return 0, vst3.ErrInvalidArgument
}

func (c *componentImpl) NormalizedParamToPlain(id uint32, normalized float64) float64 {
	if p, ok := c.processor.GetParameters().GetOK(id); ok {
		return p.Denormalize(normalized)
	}
	return normalized
}

func (c *componentImpl) PlainParamToNormalized(id uint32, plain float64) float64 {
	if p, ok := c.processor.GetParameters().GetOK(id); ok {
		return p.Normalize(plain)
	}
	return plain
}

func (c *componentImpl) GetParamNormalized(id uint32) float64 {
	if value, ok := c.processor.GetParameters().GetNormalized(id); ok {
		return value
	}
	return 0
}

func (c *componentImpl) SetParamNormalized(id uint32, value float64) error {
	if p, ok := c.processor.GetParameters().GetOK(id); ok {
		p.SetNormalized(value)
		return nil
	}
	return vst3.ErrInvalidArgument
}

func (c *componentImpl) SetComponentHandler(handler interface{}) error {
	return nil
}

func (c *componentImpl) CreateView(name string) (interface{}, error) {
	return nil, vst3.ErrNotImplemented
}

// SetParamNormalizedWithNotification sets a parameter value and notifies the
// host that the plugin changed it internally.
//
// UI THREAD ONLY. beginEdit/performEdit/endEdit are IComponentHandler calls,
// which VST3 permits only from the host's UI thread; invoking them from
// Process would violate the realtime contract and can deadlock a host. A
// processor that needs to emit parameter values from the audio callback must
// write them into ProcessData.outputParameterChanges instead. Nothing on the
// Process path calls this today, and it must stay that way.
func (c *componentImpl) SetParamNormalizedWithNotification(id uint32, value float64) error {
	if p, ok := c.processor.GetParameters().GetOK(id); ok {
		// Notify host of parameter change
		if c.wrapper != nil {
			c.wrapper.notifyParamBeginEdit(id)
			p.SetNormalized(value)
			c.wrapper.notifyParamPerformEdit(id, value)
			c.wrapper.notifyParamEndEdit(id)
		} else {
			// Fallback if no wrapper available
			p.SetNormalized(value)
		}
		return nil
	}
	return vst3.ErrInvalidArgument
}

// processSampleAccurate processes audio in chunks delimited by parameter
// changes, so each change takes effect on exactly the sample the host asked for.
func (c *componentImpl) processSampleAccurate(ctx *process.Context) {
	changes := ctx.GetParameterChanges()
	numSamples := ctx.NumSamples()
	lastOffset := 0

	// Store original buffers
	origInput := ctx.Input
	origOutput := ctx.Output

	for _, change := range changes {
		if change.SampleOffset > lastOffset {
			end := change.SampleOffset
			if end > numSamples {
				end = numSamples
			}
			c.processChunk(ctx, origInput, origOutput, lastOffset, end)
			lastOffset = change.SampleOffset
		}

		// Apply the parameter change
		ctx.ApplyParameterChange(change)
	}

	// Process final chunk if there are samples remaining
	if lastOffset < numSamples {
		c.processChunk(ctx, origInput, origOutput, lastOffset, numSamples)
	}

	// Restore original buffers
	ctx.Input = origInput
	ctx.Output = origOutput
}

// processChunk points the context at [start, end) of the block and runs one
// pass of user DSP over it. The chunk slice headers live in reusable scratch so
// the loop keeps ProcessAudio's zero-allocation promise.
func (c *componentImpl) processChunk(ctx *process.Context, input, output [][]float32, start, end int) {
	c.chunkInput = sliceChannels(c.chunkInput[:0], input, start, end)
	c.chunkOutput = sliceChannels(c.chunkOutput[:0], output, start, end)
	ctx.Input = c.chunkInput
	ctx.Output = c.chunkOutput

	c.publishInputEvents(ctx, int32(start), int32(end))

	c.processor.ProcessAudio(ctx)
}

// sliceChannels appends the [start, end) window of every channel in src to dst,
// which the caller supplies pre-truncated so no allocation occurs after warmup.
func sliceChannels(dst, src [][]float32, start, end int) [][]float32 {
	for _, channel := range src {
		if start >= len(channel) {
			continue
		}

		stop := end
		if stop > len(channel) {
			stop = len(channel)
		}
		dst = append(dst, channel[start:stop])
	}

	return dst
}
