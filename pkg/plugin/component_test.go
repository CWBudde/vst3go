package plugin

import (
	"math"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/midi"
	// The C bridge defines the helpers component.go calls; without it the test
	// binary has nothing to link those symbols against.
	_ "github.com/cwbudde/vst3go/pkg/plugin/cbridge"
)

func TestComponentRegistry(t *testing.T) {
	first := &componentWrapper{}
	second := &componentWrapper{}

	firstID := registerComponent(first)
	secondID := registerComponent(second)

	if firstID == 0 || secondID == 0 {
		t.Fatalf("IDs must never be zero: got %d and %d", firstID, secondID)
	}
	if firstID == secondID {
		t.Fatalf("IDs must be unique: both are %d", firstID)
	}
	if first.id != firstID || second.id != secondID {
		t.Errorf("wrapper.id not set: %d/%d, want %d/%d", first.id, second.id, firstID, secondID)
	}

	if got := getComponent(firstID); got != first {
		t.Errorf("getComponent(%d) = %p, want %p", firstID, got, first)
	}
	if got := getComponent(secondID); got != second {
		t.Errorf("getComponent(%d) = %p, want %p", secondID, got, second)
	}

	if got := unregisterComponent(firstID); got != first {
		t.Errorf("unregisterComponent returned %p, want %p", got, first)
	}

	// A removed ID must read as absent, never as a stale pointer.
	if got := getComponent(firstID); got != nil {
		t.Errorf("getComponent after removal = %p, want nil", got)
	}
	if got := getComponent(secondID); got != second {
		t.Errorf("removing one component disturbed another: %p, want %p", got, second)
	}

	// Removing twice is harmless.
	if got := unregisterComponent(firstID); got != nil {
		t.Errorf("second unregisterComponent = %p, want nil", got)
	}

	unregisterComponent(secondID)
}

func TestComponentRegistryRejectsUnknownIDs(t *testing.T) {
	if got := getComponent(0); got != nil {
		t.Errorf("getComponent(0) = %p, want nil", got)
	}
	// Well past the end of the table: must not panic or read out of bounds.
	if got := getComponent(^uintptr(0)); got != nil {
		t.Errorf("getComponent(max) = %p, want nil", got)
	}
	if got := unregisterComponent(0); got != nil {
		t.Errorf("unregisterComponent(0) = %p, want nil", got)
	}
	if got := unregisterComponent(1 << 30); got != nil {
		t.Errorf("unregisterComponent(huge) = %p, want nil", got)
	}
}

func TestGetComponentDoesNotAllocate(t *testing.T) {
	id := registerComponent(&componentWrapper{})
	defer unregisterComponent(id)

	allocs := testing.AllocsPerRun(100, func() {
		if getComponent(id) == nil {
			t.Fatal("component vanished")
		}
	})
	if allocs != 0 {
		t.Errorf("getComponent allocated %.0f times per call, want 0", allocs)
	}
}

func TestVelocityToMIDI(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want uint8
	}{
		{"zero", 0, 0},
		{"full", 1, 127},
		// Truncation would drop this to 126.
		{"just below full", 0.999, 127},
		{"half", 0.5, 64},
		{"one step", 1.0 / 127.0, 1},
		// Out-of-range values wrap catastrophically in a bare uint8 conversion.
		{"above range", 1.5, 127},
		{"far above range", 1000, 127},
		{"negative", -0.5, 0},
		{"far below range", -1000, 0},
		{"nan", math.NaN(), 0},
		{"inf", math.Inf(1), 127},
		{"neg inf", math.Inf(-1), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := velocityToMIDI(tt.in); got != tt.want {
				t.Errorf("velocityToMIDI(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSliceChannels(t *testing.T) {
	src := [][]float32{make([]float32, 10), make([]float32, 10)}
	for ch := range src {
		for i := range src[ch] {
			src[ch][i] = float32(ch*100 + i)
		}
	}

	dst := sliceChannels(nil, src, 4, 8)
	if len(dst) != 2 {
		t.Fatalf("channel count = %d, want 2", len(dst))
	}
	for ch := range dst {
		if len(dst[ch]) != 4 {
			t.Fatalf("channel %d length = %d, want 4", ch, len(dst[ch]))
		}
		if dst[ch][0] != float32(ch*100+4) {
			t.Errorf("channel %d starts at %v, want %v", ch, dst[ch][0], float32(ch*100+4))
		}
	}

	// Windows must clamp to the channel length rather than slice past it.
	dst = sliceChannels(dst[:0], src, 8, 20)
	if len(dst[0]) != 2 {
		t.Errorf("clamped length = %d, want 2", len(dst[0]))
	}

	// A start beyond the channel yields nothing rather than panicking.
	if got := sliceChannels(dst[:0], src, 10, 12); len(got) != 0 {
		t.Errorf("out-of-range window produced %d channels, want 0", len(got))
	}
}

// recordingProcessor captures what each ProcessAudio pass observes.
type recordingProcessor struct {
	params *param.Registry
	buses  *bus.Configuration

	chunkSizes  []int
	chunkFirst  []float32
	chunkEvents [][]midi.Event
}

func (p *recordingProcessor) Initialize(float64, int32) error { return nil }
func (p *recordingProcessor) SetActive(bool) error            { return nil }
func (p *recordingProcessor) GetLatencySamples() int32        { return 0 }
func (p *recordingProcessor) GetTailSamples() int32           { return 0 }
func (p *recordingProcessor) GetBuses() *bus.Configuration    { return p.buses }
func (p *recordingProcessor) GetParameters() *param.Registry  { return p.params }

func (p *recordingProcessor) ProcessAudio(ctx *process.Context) {
	p.chunkSizes = append(p.chunkSizes, ctx.NumSamples())
	p.chunkFirst = append(p.chunkFirst, ctx.Input[0][0])

	events := ctx.GetAllInputEvents()
	captured := make([]midi.Event, len(events))
	copy(captured, events)
	p.chunkEvents = append(p.chunkEvents, captured)
}

// nullProcessor does nothing, so allocation tests measure only the wrapper.
type nullProcessor struct{ recordingProcessor }

func (p *nullProcessor) ProcessAudio(*process.Context) {}

func newTestComponent(t *testing.T, processor Processor, blockSize int) (*componentImpl, *process.Context) {
	t.Helper()

	c := &componentImpl{processor: processor}
	ctx := process.NewContext(blockSize, processor.GetParameters())
	c.processCtx.Store(ctx)
	return c, ctx
}

func fillBlock(ctx *process.Context, numChannels, numSamples int) {
	for ch := 0; ch < numChannels; ch++ {
		in := make([]float32, numSamples)
		out := make([]float32, numSamples)
		for i := range in {
			in[i] = float32(i)
		}
		ctx.Input = append(ctx.Input, in)
		ctx.Output = append(ctx.Output, out)
	}
}

func TestProcessSampleAccurateChunkBoundaries(t *testing.T) {
	const numSamples = 512

	processor := &recordingProcessor{params: param.NewRegistry()}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 2, numSamples)

	ctx.AddParameterChange(1, 0.25, 128)
	ctx.AddParameterChange(1, 0.75, 384)
	ctx.SortParameterChanges()

	c.processSampleAccurate(ctx, nil)

	wantSizes := []int{128, 256, 128}
	if len(processor.chunkSizes) != len(wantSizes) {
		t.Fatalf("chunk count = %d, want %d", len(processor.chunkSizes), len(wantSizes))
	}
	for i, want := range wantSizes {
		if processor.chunkSizes[i] != want {
			t.Errorf("chunk %d size = %d, want %d", i, processor.chunkSizes[i], want)
		}
	}

	// Each chunk must see its own window of the block, not the block start.
	wantFirst := []float32{0, 128, 384}
	for i, want := range wantFirst {
		if processor.chunkFirst[i] != want {
			t.Errorf("chunk %d first sample = %v, want %v", i, processor.chunkFirst[i], want)
		}
	}

	// The full-block buffers must be restored for the next call.
	if len(ctx.Input[0]) != numSamples || len(ctx.Output[0]) != numSamples {
		t.Errorf("buffers not restored: input %d, output %d", len(ctx.Input[0]), len(ctx.Output[0]))
	}
}

func TestProcessSampleAccurateReslicesEvents(t *testing.T) {
	const numSamples = 512

	processor := &recordingProcessor{params: param.NewRegistry()}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 2, numSamples)

	// Block-absolute offsets, as the host delivers them.
	c.blockEvents = []midi.Event{
		midi.NewNoteOn(0, 60, 100, 10),
		midi.NewNoteOn(0, 64, 100, 300),
		midi.NewNoteOff(0, 60, 0, 500),
	}

	ctx.AddParameterChange(1, 0.5, 128)
	ctx.AddParameterChange(1, 0.5, 384)
	ctx.SortParameterChanges()

	c.processSampleAccurate(ctx, nil)

	if len(processor.chunkEvents) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(processor.chunkEvents))
	}

	// Chunk [0,128): note 60 at absolute 10, chunk-relative 10.
	assertEvents(t, "chunk 0", processor.chunkEvents[0], []int32{10}, []uint8{60})
	// Chunk [128,384): note 64 at absolute 300, chunk-relative 172.
	assertEvents(t, "chunk 1", processor.chunkEvents[1], []int32{172}, []uint8{64})
	// Chunk [384,512): note off at absolute 500, chunk-relative 116.
	assertEvents(t, "chunk 2", processor.chunkEvents[2], []int32{116}, []uint8{60})
}

func assertEvents(t *testing.T, name string, got []midi.Event, wantOffsets []int32, wantNotes []uint8) {
	t.Helper()

	if len(got) != len(wantOffsets) {
		t.Fatalf("%s: event count = %d, want %d (%v)", name, len(got), len(wantOffsets), got)
	}
	for i, event := range got {
		if event.Offset != wantOffsets[i] {
			t.Errorf("%s event %d: offset = %d, want %d", name, i, event.Offset, wantOffsets[i])
		}

		switch event.Type {
		case midi.EventTypeNoteOn, midi.EventTypeNoteOff:
		default:
			t.Fatalf("%s event %d: unexpected type %v", name, i, event.Type)
		}
		if note := event.Note(); note != wantNotes[i] {
			t.Errorf("%s event %d: note = %d, want %d", name, i, note, wantNotes[i])
		}
	}
}

func TestPublishInputEventsWholeBlock(t *testing.T) {
	const numSamples = 256

	processor := &recordingProcessor{params: param.NewRegistry()}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 1, numSamples)

	c.blockEvents = []midi.Event{
		midi.NewNoteOn(0, 60, 0, 0),
		midi.NewNoteOn(0, 61, 0, 255),
	}

	c.publishInputEvents(ctx, 0, math.MaxInt32)

	events := ctx.GetAllInputEvents()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Offset != 0 || events[1].Offset != 255 {
		t.Errorf("whole-block offsets were shifted: %d, %d", events[0].Offset, events[1].Offset)
	}
}

func TestPublishInputEventsClearsPreviousChunk(t *testing.T) {
	const numSamples = 256

	processor := &recordingProcessor{params: param.NewRegistry()}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 1, numSamples)

	c.blockEvents = []midi.Event{
		midi.NewNoteOn(0, 60, 0, 10),
	}

	c.publishInputEvents(ctx, 0, 128)
	c.publishInputEvents(ctx, 128, 256)

	// The note belongs to the first chunk only; re-delivering it would
	// retrigger the voice once per chunk.
	if events := ctx.GetAllInputEvents(); len(events) != 0 {
		t.Errorf("second chunk saw %d events, want 0", len(events))
	}
}

func TestProcessChunkDoesNotAllocate(t *testing.T) {
	const numSamples = 512

	processor := &nullProcessor{recordingProcessor{params: param.NewRegistry()}}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 8, numSamples)

	input, output := ctx.Input, ctx.Output

	// Warm up the scratch slices; the first call is allowed to allocate them.
	c.processChunk(ctx, input, output, 0, numSamples, nil)

	allocs := testing.AllocsPerRun(200, func() {
		c.processChunk(ctx, input, output, 0, 128, nil)
		c.processChunk(ctx, input, output, 128, 384, nil)
		c.processChunk(ctx, input, output, 384, numSamples, nil)
	})
	if allocs != 0 {
		t.Errorf("chunk loop allocated %.1f times per block, want 0", allocs)
	}
}

// MIDI events used to box into the midi.Event interface, which allocated once
// per event per block on the audio thread. Event is now a concrete struct, so a
// block carrying MIDI must be exactly as allocation-free as a silent one.
func TestProcessSampleAccurateWithEventsDoesNotAllocate(t *testing.T) {
	const numSamples = 512

	processor := &nullProcessor{recordingProcessor{params: param.NewRegistry()}}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 8, numSamples)

	events := make([]midi.Event, 0, 64)
	for i := range int32(32) {
		events = append(events, midi.NewNoteOn(0, uint8(i), 100, i*16))
	}

	prime := func() {
		c.blockEvents = append(c.blockEvents[:0], events...)
		ctx.ResetParameterChanges()
		ctx.AddParameterChange(1, 0.5, 128)
		ctx.AddParameterChange(1, 0.5, 384)
		ctx.SortParameterChanges()
	}

	prime()
	c.processSampleAccurate(ctx, nil)

	// Guard against a vacuous measurement: the events must actually be reaching
	// the context, otherwise this would pass on an empty code path.
	if !ctx.HasInputEvents() {
		t.Fatal("the final chunk saw no input events; the alloc measurement would be meaningless")
	}

	allocs := testing.AllocsPerRun(200, func() {
		prime()
		c.processSampleAccurate(ctx, nil)
	})
	if allocs != 0 {
		t.Errorf("a block with 32 MIDI events allocated %.1f times, want 0", allocs)
	}
}

func TestProcessSampleAccurateDoesNotAllocate(t *testing.T) {
	const numSamples = 512

	processor := &nullProcessor{recordingProcessor{params: param.NewRegistry()}}
	c, ctx := newTestComponent(t, processor, numSamples)
	fillBlock(ctx, 8, numSamples)

	prime := func() {
		ctx.ResetParameterChanges()
		ctx.AddParameterChange(1, 0.5, 128)
		ctx.AddParameterChange(1, 0.5, 384)
		ctx.SortParameterChanges()
	}

	prime()
	c.processSampleAccurate(ctx, nil)

	allocs := testing.AllocsPerRun(200, func() {
		prime()
		c.processSampleAccurate(ctx, nil)
	})
	if allocs != 0 {
		t.Errorf("processSampleAccurate allocated %.1f times per block, want 0", allocs)
	}
}
