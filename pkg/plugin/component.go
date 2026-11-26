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
	programs   *factoryPrograms
	processCtx atomic.Pointer[process.Context]
	busConfig  *bus.Configuration

	// sampleRateBits is the current sample rate as math.Float64bits.
	sampleRateBits atomic.Uint64
	processing     atomic.Bool

	maxBlockSize int32
	active       bool
	mu           sync.Mutex
	wrapper      *componentWrapper // Reference to wrapper for notifications

	// Audio-thread scratch. Reused across blocks so the chunk loop and the
	// event fan-out stay allocation-free.
	chunkInput       [][]float32
	chunkOutput      [][]float32
	blockEvents      []midi.Event
	chunkInputBuses  []process.BusBuffers
	chunkOutputBuses []process.BusBuffers

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
	if err := c.processor.Initialize(48000, c.maxBlockSize); err != nil {
		return err
	}
	c.busConfig = c.processor.GetBuses()
	return nil
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

func (c *componentImpl) buses() *bus.Configuration {
	if c.busConfig != nil {
		return c.busConfig
	}
	return c.processor.GetBuses()
}

func (c *componentImpl) GetBusCount(mediaType, direction int32) int32 {
	buses := c.buses()
	if buses == nil {
		return 0
	}
	return buses.GetBusCount(bus.MediaType(mediaType), bus.Direction(direction))
}

func (c *componentImpl) GetBusInfo(mediaType, direction, index int32) (*vst3.BusInfo, error) {
	buses := c.buses()
	if buses == nil {
		return nil, vst3.ErrNotImplemented
	}
	info := buses.GetBusInfo(bus.MediaType(mediaType), bus.Direction(direction), index)
	if info == nil {
		return nil, vst3.ErrNotImplemented
	}

	var flags uint32
	if info.DefaultActive() {
		flags = C.Steinberg_Vst_BusInfo_BusFlags_kDefaultActive
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
	if c.busConfig == nil {
		return bus.ErrBusNotFound
	}
	return c.busConfig.SetBusActive(bus.MediaType(mediaType), bus.Direction(direction), index, state)
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
	if c.programs != nil {
		c.programs.selection.And(programsRunning)
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
	for direction, arrangements := range [][]int64{inputs, outputs} {
		if len(arrangements) != int(c.GetBusCount(int32(bus.MediaTypeAudio), int32(direction))) {
			return vst3.ErrInvalidArgument
		}
		for i, requested := range arrangements {
			declared, err := c.GetBusArrangement(int32(direction), int32(i))
			if err != nil || requested != declared {
				return vst3.ErrInvalidArgument
			}
		}
	}
	return nil
}

func (c *componentImpl) GetBusArrangement(direction, index int32) (int64, error) {
	buses := c.buses()
	if buses == nil {
		return 0, bus.ErrBusNotFound
	}
	info := buses.GetBusInfo(bus.MediaTypeAudio, bus.Direction(direction), index)
	if info == nil {
		return 0, bus.ErrBusNotFound
	}
	return speakerArrangement(info.ChannelCount), nil
}

// speakerArrangement maps a declared channel count to the fixed layout the
// component advertises. ProcessAudio receives channels in the host's order.
func speakerArrangement(channels int32) int64 {
	// These masks match SpeakerArr in the VST3 interface header.
	switch channels {
	case 1:
		return 1 << 19 // kMono
	case 2:
		return 0x03 // kStereo
	case 4:
		return 0x33 // k40Music (quad)
	case 5:
		return 0x37 // k50
	case 6:
		return 0x3f // k51
	case 7:
		return 0x637 // k70Music
	case 8:
		return 0x63f // k71Music
	default:
		return (int64(1) << channels) - 1
	}
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
	}

	if err := c.processor.Initialize(setup.SampleRate, c.maxBlockSize); err != nil {
		return fmt.Errorf("setup processing: initialize processor: %w", err)
	}
	if c.busConfig == nil {
		c.busConfig = c.processor.GetBuses()
	}
	ctx := process.NewContext(int(c.maxBlockSize), c.processor.GetParameters())
	if c.busConfig != nil {
		ctx.Buses = process.NewMultiBusContext(ctx, c.busConfig)
		var inChannels, outChannels int
		ctx.Buses.InputBuses, inChannels = busBufferViews(c.busConfig, bus.DirectionInput)
		ctx.Buses.OutputBuses, outChannels = busBufferViews(c.busConfig, bus.DirectionOutput)
		ctx.Input = make([][]float32, 0, inChannels)
		ctx.Output = make([][]float32, 0, outChannels)
		c.chunkInput = make([][]float32, 0, inChannels)
		c.chunkOutput = make([][]float32, 0, outChannels)
		c.chunkInputBuses = make([]process.BusBuffers, len(ctx.Buses.InputBuses))
		c.chunkOutputBuses = make([]process.BusBuffers, len(ctx.Buses.OutputBuses))
	}
	c.processCtx.Store(ctx)
	return nil
}

func busBufferViews(buses *bus.Configuration, direction bus.Direction) ([]process.BusBuffers, int) {
	views := make([]process.BusBuffers, buses.GetBusCount(bus.MediaTypeAudio, direction))
	var channels int
	for i := range views {
		info := buses.GetBusInfo(bus.MediaTypeAudio, direction, int32(i))
		views[i].BusInfo = info
		channels += int(info.ChannelCount)
	}
	return views, channels
}

func (c *componentImpl) SetProcessing(state bool) error {
	if state && c.programs != nil {
		c.programs.selection.Or(programsRunning)
	}
	c.processing.Store(state)
	if !state && c.programs != nil {
		c.programs.selection.And(^programsRunning)
	}
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
	ctx.SetNumSamples(int(processData.numSamples))
	c.updateTransport(ctx, processData.processContext)
	c.mapAudioBuffers(ctx, processData)
	c.collectInputEvents(processData.inputEvents)
	c.collectParameterChanges(ctx, unsafe.Pointer(processData.inputParameterChanges))
	if c.programs != nil {
		c.programs.applyPending(ctx.GetParameterChanges())
	}

	// Process audio with sample-accurate parameter automation
	if ctx.HasParameterChanges() {
		// Sort parameter changes by sample offset
		ctx.SortParameterChanges()

		// Process audio in chunks between parameter changes
		c.processSampleAccurate(ctx)
	} else {
		// No parameter changes - process entire block
		c.publishInputEvents(ctx, 0, int32(ctx.NumSamples()))
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
	if ctx.Buses != nil {
		for i := range ctx.Buses.InputBuses {
			ctx.Buses.InputBuses[i].Channels = nil
		}
		for i := range ctx.Buses.OutputBuses {
			ctx.Buses.OutputBuses[i].Channels = nil
		}
	}
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
	transport.HasContinuousTime = (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kContTimeValid) != 0
	if transport.HasContinuousTime {
		transport.ContinuousTimeSamples = int64(ctx.continousTimeSamples)
	}

	if (ctx.state & C.Steinberg_Vst_ProcessContext_StatesAndFlags_kClockValid) != 0 {
		transport.SamplesToNextClock = int32(ctx.samplesToNextClock)
	}
}

func (c *componentImpl) mapAudioBuffers(ctx *process.Context, processData *C.struct_Steinberg_Vst_ProcessData) {
	if ctx.Buses == nil {
		return
	}
	numSamples := ctx.NumSamples()

	if processData.numInputs > 0 && processData.inputs != nil {
		inputBuses := unsafe.Slice(processData.inputs, int(processData.numInputs))
		ctx.Input = appendAudioBuffers(ctx.Input, ctx.Buses.InputBuses, inputBuses, numSamples)
	}

	if processData.numOutputs > 0 && processData.outputs != nil {
		outputBuses := unsafe.Slice(processData.outputs, int(processData.numOutputs))
		ctx.Output = appendAudioBuffers(ctx.Output, ctx.Buses.OutputBuses, outputBuses, numSamples)
	}
}

func appendAudioBuffers(dst [][]float32, views []process.BusBuffers, buses []C.struct_Steinberg_Vst_AudioBusBuffers, numSamples int) [][]float32 {
	for i := range min(len(views), len(buses)) {
		view, buffers := &views[i], &buses[i]
		if !view.BusInfo.Active() || buffers.numChannels != C.int32_t(view.BusInfo.ChannelCount) {
			continue
		}
		first := len(dst)
		table := (**float32)(unsafe.Pointer(getChannelBuffers32(buffers)))
		dst = appendBusChannels(dst, table, int(buffers.numChannels), numSamples)
		if len(dst)-first != int(view.BusInfo.ChannelCount) {
			// A missing channel in an active bus is malformed. Exclude the
			// entire bus so later speakers cannot shift into its position.
			dst = dst[:first]
			continue
		}
		view.Channels = dst[first:len(dst):len(dst)]
	}
	return dst
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
		return midi.NewNoteOn(
			uint8(noteOn.channel),
			uint8(noteOn.pitch),
			velocityToMIDI(float64(noteOn.velocity)),
			int32(event.sampleOffset),
		), true

	case C.Steinberg_Vst_Event_EventTypes_kNoteOffEvent:
		noteOff := C.getNoteOffEvent(event)
		return midi.NewNoteOff(
			uint8(noteOff.channel),
			uint8(noteOff.pitch),
			velocityToMIDI(float64(noteOff.velocity)),
			int32(event.sampleOffset),
		), true

		// Add more event types as needed
	}

	return midi.Event{}, false
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

	// midi.EventQueue.OffsetEvents does the same rebase for a whole queue, but
	// process.Context does not expose its queue, and shifting after insertion
	// would also move events belonging to other chunks.
	for _, event := range c.blockEvents {
		if event.Offset < start || event.Offset >= end {
			continue
		}
		ctx.AddInputEvent(event.ShiftOffset(-start))
	}
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
	if c.programs != nil && id == c.programs.list.ParameterID {
		return c.programs.selectFromController(value)
	}
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

	for first := 0; first < len(changes); {
		offset := min(changes[first].SampleOffset, numSamples)
		if offset > lastOffset {
			c.processChunk(ctx, origInput, origOutput, lastOffset, offset)
			lastOffset = offset
		}

		next := first + 1
		for next < len(changes) && changes[next].SampleOffset == changes[first].SampleOffset {
			next++
		}
		c.applyParameterChanges(ctx, changes[first:next])
		first = next
	}

	// Process final chunk if there are samples remaining
	if lastOffset < numSamples {
		c.processChunk(ctx, origInput, origOutput, lastOffset, numSamples)
	}

	// Restore original buffers
	ctx.Input = origInput
	ctx.Output = origOutput
}

func (c *componentImpl) applyParameterChanges(ctx *process.Context, changes []process.ParameterChange) {
	if programs := c.programs; programs != nil {
		// A preset establishes base values at this offset. Automation for
		// individual controls at the same offset overrides those values.
		for _, change := range changes {
			if change.ParamID == programs.list.ParameterID {
				if index, ok := programs.index(change.Value); ok {
					programs.applyIndex(index)
				}
			}
		}
	}
	for _, change := range changes {
		if c.programs == nil || change.ParamID != c.programs.list.ParameterID {
			ctx.ApplyParameterChange(change)
		}
	}
}

// processChunk points the context at [start, end) of the block and runs one
// pass of user DSP over it. The chunk slice headers live in reusable scratch so
// the loop keeps ProcessAudio's zero-allocation promise.
func (c *componentImpl) processChunk(ctx *process.Context, input, output [][]float32, start, end int) {
	fullBlockSamples := ctx.NumSamples()
	ctx.SetNumSamples(end - start)
	defer ctx.SetNumSamples(fullBlockSamples)

	if buses := ctx.Buses; buses != nil {
		fullInputBuses, fullOutputBuses := buses.InputBuses, buses.OutputBuses
		c.chunkInput = sliceBusBuffers(c.chunkInputBuses, fullInputBuses, c.chunkInput[:0], start, end)
		c.chunkOutput = sliceBusBuffers(c.chunkOutputBuses, fullOutputBuses, c.chunkOutput[:0], start, end)
		buses.InputBuses, buses.OutputBuses = c.chunkInputBuses, c.chunkOutputBuses
		defer func() {
			buses.InputBuses, buses.OutputBuses = fullInputBuses, fullOutputBuses
		}()
	} else {
		c.chunkInput = sliceChannels(c.chunkInput[:0], input, start, end)
		c.chunkOutput = sliceChannels(c.chunkOutput[:0], output, start, end)
	}
	ctx.Input = c.chunkInput
	ctx.Output = c.chunkOutput

	c.publishInputEvents(ctx, int32(start), int32(end))

	transport := *ctx.Transport
	if start > 0 && transport.IsPlaying {
		ctx.Transport.ProjectTimeSamples += int64(start)
		if transport.HasContinuousTime {
			ctx.Transport.ContinuousTimeSamples += int64(start)
		}
		if samplesPerBeat := transport.GetSamplesPerBeat(ctx.SampleRate); transport.HasMusicalTime && samplesPerBeat > 0 {
			ctx.Transport.ProjectTimeMusic += float64(start) / samplesPerBeat
		}
		defer func() { *ctx.Transport = transport }()
	}

	c.processor.ProcessAudio(ctx)
}

func sliceBusBuffers(dst, src []process.BusBuffers, channels [][]float32, start, end int) [][]float32 {
	for i, view := range src {
		first := len(channels)
		channels = sliceChannels(channels, view.Channels, start, end)
		dst[i] = process.BusBuffers{BusInfo: view.BusInfo}
		if first != len(channels) {
			dst[i].Channels = channels[first:len(channels):len(channels)]
		}
	}
	return channels
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
