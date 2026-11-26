// Package process provides the audio processing context handed to a plugin's
// ProcessAudio callback.
//
// A [Context] carries one block: the input and output channel slices (which
// alias the host's buffers directly, without copying), the transport state,
// the parameter changes the host scheduled for this block, and the MIDI events
// that fall inside it. The runtime owns the Context, reuses it across blocks,
// and fills it in before each call.
//
// # Threading
//
// A Context belongs to the audio thread. Every method on it is intended to be
// called from ProcessAudio and from nowhere else: none of them is synchronized,
// and its buffers are only valid for the duration of the call that delivered
// them. A Context, or any slice obtained from one, must never be retained past
// the end of ProcessAudio or handed to another goroutine.
//
// The exceptions are owned by the runtime, not by plugin code:
// [NewContext], [Context.ReserveOutputEvents], and the reset paths run on the
// control thread while the audio thread is stopped.
//
// Within ProcessAudio the whole surface is allocation-free: the parameter-change
// buffer is sized from the block size at construction and coalesces on
// overflow, and event collection reads into retained scratch rather than
// allocating a snapshot.
package process

import (
	"cmp"
	"math"
	"slices"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/midi"
)

const (
	// quarterNotesPerWholeNote is the denominator basis of a time signature: a
	// "/4" signature means the beat is one quarter note, "/8" an eighth, and so
	// on, so quarter notes per beat is this value divided by the denominator.
	quarterNotesPerWholeNote = 4.0

	// secondsPerMinute converts a tempo in beats per minute into seconds per beat.
	secondsPerMinute = 60.0

	// minParameterChanges is the floor for the automation buffer so that even a
	// tiny block size tolerates a burst of automation points.
	minParameterChanges = 128

	// maxParameterChanges caps the automation buffer: a host reporting an absurd
	// block size must not make us reserve unbounded memory.
	maxParameterChanges = 16384

	// eventScratchCapacity pre-sizes the reusable event scratch so emptiness
	// checks on the audio thread never allocate for a normal MIDI block.
	eventScratchCapacity = 64

	// maxScratchChannels pre-sizes the per-sample scratch buffers. It matches the
	// per-bus channel limit in pkg/framework/bus, so ProcessSamples never has to
	// allocate on the audio thread.
	maxScratchChannels = 32
)

// ParameterChange represents a parameter change at a specific sample offset
type ParameterChange struct {
	ParamID      uint32
	Value        float64
	SampleOffset int
}

// TransportInfo provides musical timing and transport state information
type TransportInfo struct {
	// Transport state
	IsPlaying   bool // Transport is playing
	IsRecording bool // Transport is recording
	IsCycling   bool // Loop/cycle is active

	// Tempo and timing
	Tempo              float64 // Current tempo in BPM (0 if not available)
	TimeSigNumerator   int32   // Time signature numerator (e.g., 4 for 4/4)
	TimeSigDenominator int32   // Time signature denominator (e.g., 4 for 4/4)

	// Musical position
	ProjectTimeMusic float64 // Musical position in quarter notes
	BarPositionMusic float64 // Bar position in quarter notes

	// Cycle/loop points
	CycleStartMusic float64 // Cycle start in quarter notes
	CycleEndMusic   float64 // Cycle end in quarter notes

	// Sample position at the start of the current ProcessAudio call.
	ProjectTimeSamples    int64 // Project time in samples
	ContinuousTimeSamples int64 // Continuous time in samples (doesn't reset on loop)

	// Clock information
	SamplesToNextClock int32 // Samples until next clock/beat

	// Validity flags
	HasTempo          bool // Tempo field is valid
	HasTimeSignature  bool // Time signature fields are valid
	HasMusicalTime    bool // Musical time fields are valid
	HasBarPosition    bool // Bar position is valid
	HasCycle          bool // Cycle points are valid
	HasContinuousTime bool // ContinuousTimeSamples is valid
}

// GetBarsBeats returns the current position in bars and beats
func (t *TransportInfo) GetBarsBeats() (bars int, beats float64) {
	if !t.HasMusicalTime || !t.HasTimeSignature || t.TimeSigDenominator == 0 {
		return 0, 0
	}

	// Convert quarter notes to beats based on time signature
	beatsPerBar := float64(t.TimeSigNumerator)
	quarterNotesPerBeat := quarterNotesPerWholeNote / float64(t.TimeSigDenominator)
	totalBeats := t.ProjectTimeMusic / quarterNotesPerBeat

	bars = int(totalBeats / beatsPerBar)
	beats = totalBeats - float64(bars)*beatsPerBar

	return bars, beats
}

// GetSamplesPerBeat returns the number of samples per beat at current tempo
func (t *TransportInfo) GetSamplesPerBeat(sampleRate float64) float64 {
	if !t.HasTempo || t.Tempo <= 0 {
		return 0
	}

	// 60 seconds per minute / tempo = seconds per beat
	// seconds per beat * sample rate = samples per beat
	return (secondsPerMinute / t.Tempo) * sampleRate
}

// GetBeatPosition returns the current position within the current beat (0-1)
func (t *TransportInfo) GetBeatPosition() float64 {
	if !t.HasMusicalTime || !t.HasBarPosition {
		return 0
	}

	// Calculate position within current bar in quarter notes
	positionInBar := t.ProjectTimeMusic - t.BarPositionMusic

	// Convert to beat position based on time signature
	if t.HasTimeSignature && t.TimeSigDenominator > 0 {
		quarterNotesPerBeat := quarterNotesPerWholeNote / float64(t.TimeSigDenominator)
		beatPosition := (positionInBar / quarterNotesPerBeat)
		return beatPosition - float64(int(beatPosition)) // Get fractional part
	}

	// Default to quarter note = beat
	return positionInBar - float64(int(positionInBar))
}

// IsOnBeat returns true if the current position is at the start of a beat
func (t *TransportInfo) IsOnBeat(threshold float64) bool {
	beatPos := t.GetBeatPosition()
	return beatPos < threshold || beatPos > (1.0-threshold)
}

// Context provides a clean API for audio processing with zero allocations
type Context struct {
	// Input and Output contain the flattened channels of active buses in host order.
	Input      [][]float32
	Output     [][]float32
	SampleRate float64

	// Buses provides channel views by host bus index. Inactive or omitted buses
	// have no channels. It is populated by the plugin runtime during processing.
	Buses *MultiBusContext

	numSamples    int
	hasNumSamples bool

	// Pre-allocated work buffers
	workBuffer []float32
	tempBuffer []float32
	sampleIn   []float32
	sampleOut  []float32

	// Parameter access
	params *param.Registry

	// Sample-accurate automation
	paramChanges   []ParameterChange // Pre-allocated slice for parameter changes
	changeCount    int               // Number of active parameter changes
	coalescedCount int               // Changes folded into an existing entry (buffer full)
	droppedCount   int               // Changes discarded entirely (buffer full, no entry to fold into)

	// Transport and timing information
	Transport *TransportInfo

	// MIDI event processing
	eventBuffer *midi.EventBuffer

	// eventScratch and dispatchScratch let HasInputEvents and ProcessEvents
	// collect events without allocating. They are separate buffers on purpose:
	// ProcessEvents iterates its scratch while user code runs, and that code is
	// free to call HasInputEvents, which would otherwise overwrite the events
	// not yet dispatched.
	eventScratch    []midi.Event
	dispatchScratch []midi.Event
}

// NewContext creates a new process context with pre-allocated buffers.
//
// Everything the audio thread can reach is sized here: the automation buffer
// scales with the block size because a dense automation lane can carry close to
// one point per sample, and the per-sample scratch buffers are allocated up
// front so the very first audio callback is already allocation-free.
func NewContext(maxBlockSize int, params *param.Registry) *Context {
	if maxBlockSize < 0 {
		maxBlockSize = 0
	}

	changeCapacity := min(max(maxBlockSize, minParameterChanges), maxParameterChanges)

	return &Context{
		workBuffer:   make([]float32, maxBlockSize),
		tempBuffer:   make([]float32, maxBlockSize),
		sampleIn:     make([]float32, 0, maxScratchChannels),
		sampleOut:    make([]float32, 0, maxScratchChannels),
		params:       params,
		paramChanges: make([]ParameterChange, changeCapacity),
		changeCount:  0,
		Transport:    &TransportInfo{}, // Initialize transport info
		eventBuffer:  midi.NewEventBuffer(),

		eventScratch:    make([]midi.Event, 0, eventScratchCapacity),
		dispatchScratch: make([]midi.Event, 0, eventScratchCapacity),
	}
}

// Param returns the current value of a parameter (0-1 normalized)
func (c *Context) Param(id uint32) float64 {
	if c.params == nil {
		return 0
	}
	value, ok := c.params.GetNormalized(id)
	if !ok {
		return 0
	}
	return value
}

// ParamPlain returns the current plain value of a parameter
func (c *Context) ParamPlain(id uint32) float64 {
	if c.params == nil {
		return 0
	}
	value, ok := c.params.GetPlain(id)
	if !ok {
		return 0
	}
	return value
}

// SetNumSamples sets the frame count supplied by the host, including for
// event-only blocks. The plugin runtime calls this before processing.
func (c *Context) SetNumSamples(numSamples int) {
	c.numSamples = max(0, numSamples)
	c.hasNumSamples = true
}

// NumSamples returns the current block or chunk's frame count. A context built
// without a host frame count derives it from the input or output buffers.
func (c *Context) NumSamples() int {
	if c.hasNumSamples {
		return c.numSamples
	}
	if len(c.Input) > 0 && len(c.Input[0]) > 0 {
		return len(c.Input[0])
	}
	if len(c.Output) > 0 && len(c.Output[0]) > 0 {
		return len(c.Output[0])
	}
	return 0
}

// NumInputChannels returns the number of input channels
func (c *Context) NumInputChannels() int {
	return len(c.Input)
}

// NumOutputChannels returns the number of output channels
func (c *Context) NumOutputChannels() int {
	return len(c.Output)
}

// WorkBuffer returns a slice of the pre-allocated work buffer sized to the
// current block size - no allocation for any block up to the size declared at
// construction.
//
// A host that hands us a larger block than it declared is violating its own
// contract; growing here allocates (not RT-safe) but is preferable to panicking
// across the cgo boundary into the C++ host. The grown buffer is retained, so
// the cost is paid at most once per new high-water mark.
func (c *Context) WorkBuffer() []float32 {
	numSamples := c.NumSamples()
	if numSamples > len(c.workBuffer) {
		c.workBuffer = make([]float32, numSamples)
	}
	return c.workBuffer[:numSamples]
}

// TempBuffer returns a slice of the pre-allocated temp buffer sized to the
// current block size. See WorkBuffer for the oversized-block behavior.
func (c *Context) TempBuffer() []float32 {
	numSamples := c.NumSamples()
	if numSamples > len(c.tempBuffer) {
		c.tempBuffer = make([]float32, numSamples)
	}
	return c.tempBuffer[:numSamples]
}

// PassThrough copies input to output (for bypass)
func (c *Context) PassThrough() {
	numChannels := c.NumInputChannels()
	if c.NumOutputChannels() < numChannels {
		numChannels = c.NumOutputChannels()
	}

	for ch := 0; ch < numChannels; ch++ {
		copy(c.Output[ch], c.Input[ch])
	}
}

// Clear zeros the output buffers
func (c *Context) Clear() {
	for ch := range c.Output {
		for i := range c.Output[ch] {
			c.Output[ch][i] = 0
		}
	}
}

// SetParameterAtOffset sets a parameter value at a specific sample offset within the current block
//
// Deprecated: Use AddParameterChange for sample-accurate automation
func (c *Context) SetParameterAtOffset(paramID uint32, value float64, _ int) {
	if c.params == nil {
		return
	}

	if p, ok := c.params.GetOK(paramID); ok {
		// For backward compatibility, apply the change immediately
		p.SetNormalized(value)
	}
}

// AddParameterChange adds a parameter change for sample-accurate processing.
// This method is used during the parameter change collection phase.
//
// The buffer is sized for the declared block size, but a host is free to send
// more points than that. Rather than silently dropping the tail - which leaves
// the parameter stuck at a mid-block value for the rest of the block - an
// overflowing change is folded into the newest entry for the same parameter, so
// the block still ends on the value the host asked for. Only a change for a
// parameter that has no entry at all is discarded.
func (c *Context) AddParameterChange(paramID uint32, value float64, sampleOffset int) {
	if c.changeCount < len(c.paramChanges) {
		c.paramChanges[c.changeCount] = ParameterChange{
			ParamID:      paramID,
			Value:        value,
			SampleOffset: sampleOffset,
		}
		c.changeCount++
		return
	}

	for i := c.changeCount - 1; i >= 0; i-- {
		if c.paramChanges[i].ParamID == paramID {
			c.paramChanges[i].Value = value
			c.paramChanges[i].SampleOffset = sampleOffset
			c.coalescedCount++
			return
		}
	}

	c.droppedCount++
}

// ResetParameterChanges clears the parameter change list for the next processing block
func (c *Context) ResetParameterChanges() {
	c.changeCount = 0
	c.coalescedCount = 0
	c.droppedCount = 0
}

// CoalescedParameterChanges returns how many changes in this block had to be
// folded into an existing entry because the automation buffer was full.
func (c *Context) CoalescedParameterChanges() int {
	return c.coalescedCount
}

// DroppedParameterChanges returns how many changes in this block were discarded
// outright. A non-zero value means automation was lost and the buffer is
// undersized for the host.
func (c *Context) DroppedParameterChanges() int {
	return c.droppedCount
}

// SortParameterChanges sorts parameter changes by sample offset for processing.
//
// slices.SortStableFunc is used instead of sort.Slice because the latter builds
// a reflect-based swapper on every call, which allocates on the audio thread.
// Stability keeps the host's ordering for changes sharing a sample offset, so
// the last change to a parameter still wins.
func (c *Context) SortParameterChanges() {
	if c.changeCount > 1 {
		// Sort only the active portion of the slice
		slices.SortStableFunc(c.paramChanges[:c.changeCount], func(a, b ParameterChange) int {
			return cmp.Compare(a.SampleOffset, b.SampleOffset)
		})
	}
}

// GetParameterChanges returns the active parameter changes for this block
func (c *Context) GetParameterChanges() []ParameterChange {
	return c.paramChanges[:c.changeCount]
}

// HasParameterChanges returns true if there are parameter changes in this block
func (c *Context) HasParameterChanges() bool {
	return c.changeCount > 0
}

// ApplyParameterChange applies a parameter change immediately
func (c *Context) ApplyParameterChange(change ParameterChange) {
	if c.params == nil {
		return
	}
	if p := c.params.Get(change.ParamID); p != nil {
		p.SetNormalized(change.Value)
	}
}

// Event processing methods

// AddInputEvent adds a MIDI event to the input queue
func (c *Context) AddInputEvent(event midi.Event) {
	c.eventBuffer.AddInputEvent(event)
}

// ReserveOutputEvents ensures capacity for at least count queued output events.
// The runtime calls it during setup, before publishing the context to the audio thread.
func (c *Context) ReserveOutputEvents(count int) {
	c.eventBuffer.ReserveOutputEvents(count)
}

// AddOutputEvent adds a MIDI event to the output queue
func (c *Context) AddOutputEvent(event midi.Event) {
	c.eventBuffer.AddOutputEvent(event)
}

// GetInputEvents returns input events in the specified sample range
func (c *Context) GetInputEvents(startSample, endSample int32) []midi.Event {
	return c.eventBuffer.GetInputEvents(startSample, endSample)
}

// GetAllInputEvents returns all input events for the current block
func (c *Context) GetAllInputEvents() []midi.Event {
	return c.eventBuffer.GetInputEvents(0, c.numSamplesInt32())
}

// numSamplesInt32 converts the block size for the int32-based event API. A block
// that large cannot occur in practice, but the conversion must saturate rather
// than wrap into a negative range that would silently match no events.
func (c *Context) numSamplesInt32() int32 {
	numSamples := c.NumSamples()
	if numSamples < 0 {
		return 0
	}
	if numSamples > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(numSamples)
}

// GetOutputEvents returns all output events generated during processing
func (c *Context) GetOutputEvents() []midi.Event {
	return c.eventBuffer.GetOutputEvents()
}

// AppendOutputEvents appends sorted output events to caller-owned scratch.
// It allocates only when dst has insufficient capacity.
func (c *Context) AppendOutputEvents(dst []midi.Event) []midi.Event {
	return c.eventBuffer.AppendOutputEvents(dst)
}

// ClearInputEvents clears the input event queue
func (c *Context) ClearInputEvents() {
	c.eventBuffer.ClearInput()
}

// ClearOutputEvents clears the output event queue
func (c *Context) ClearOutputEvents() {
	c.eventBuffer.ClearOutput()
}

// ClearAllEvents clears both input and output event queues
func (c *Context) ClearAllEvents() {
	c.eventBuffer.ClearAll()
}

// ProcessEvents hands every input event in the half-open range
// [startSample, endSample) to processor in time order.
//
// It collects into a retained scratch slice, so it allocates nothing on the
// audio thread. The scratch is filled before the first callback runs, which
// means processor may safely add or clear events without disturbing the
// iteration.
func (c *Context) ProcessEvents(processor midi.EventProcessor, startSample, endSample int32) {
	c.dispatchScratch = c.eventBuffer.AppendInputEvents(c.dispatchScratch[:0], startSample, endSample)
	for _, event := range c.dispatchScratch {
		processor.ProcessEvent(event)
	}
}

// HasInputEvents returns true if there are input events in the current block.
//
// It appends into a retained scratch slice rather than calling
// GetAllInputEvents, which would allocate a full copy of the block's events just
// to test emptiness.
func (c *Context) HasInputEvents() bool {
	c.eventScratch = c.eventBuffer.AppendInputEvents(c.eventScratch[:0], 0, c.numSamplesInt32())
	return len(c.eventScratch) > 0
}
