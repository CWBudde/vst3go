package process

import (
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/midi"
)

func TestContextEventProcessing(t *testing.T) {
	registry := param.NewRegistry()
	ctx := NewContext(512, registry)

	// Set up input/output buffers first
	ctx.Input = [][]float32{make([]float32, 512)}
	ctx.Output = [][]float32{make([]float32, 512)}

	// Test adding input events
	noteOn := midi.NewNoteOn(0, 60, 100, 100)
	noteOff := midi.NewNoteOff(0, 60, 0, 200)

	ctx.AddInputEvent(noteOn)
	ctx.AddInputEvent(noteOff)

	// Test getting all input events
	events := ctx.GetAllInputEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 input events, got %d", len(events))
	}

	// Test HasInputEvents
	if !ctx.HasInputEvents() {
		t.Error("Expected HasInputEvents to return true")
	}

	// Test getting events in range
	events = ctx.GetInputEvents(0, 150)
	if len(events) != 1 {
		t.Errorf("Expected 1 event in range [0, 150), got %d", len(events))
	}

	events = ctx.GetInputEvents(150, 300)
	if len(events) != 1 {
		t.Errorf("Expected 1 event in range [150, 300), got %d", len(events))
	}

	// Test clearing input events
	ctx.ClearInputEvents()
	if ctx.HasInputEvents() {
		t.Error("Expected no input events after clear")
	}
}

func TestContextOutputEvents(t *testing.T) {
	registry := param.NewRegistry()
	ctx := NewContext(512, registry)

	// Add output events
	cc := midi.NewControlChange(0, midi.CCVolume, 100, 50)
	pitchBend := midi.NewPitchBend(0, 1000, 75)

	ctx.AddOutputEvent(cc)
	ctx.AddOutputEvent(pitchBend)

	// Get output events
	outputEvents := ctx.GetOutputEvents()
	if len(outputEvents) != 2 {
		t.Errorf("Expected 2 output events, got %d", len(outputEvents))
	}

	// Clear output events
	ctx.ClearOutputEvents()
	outputEvents = ctx.GetOutputEvents()
	if len(outputEvents) != 0 {
		t.Error("Expected no output events after clear")
	}
}

type testProcessor struct {
	receivedEvents []midi.Event
}

func (p *testProcessor) ProcessEvent(event midi.Event) {
	p.receivedEvents = append(p.receivedEvents, event)
}

func TestContextProcessEvents(t *testing.T) {
	registry := param.NewRegistry()
	ctx := NewContext(512, registry)
	ctx.Input = [][]float32{make([]float32, 512)}

	// Add events at different offsets
	events := []midi.Event{
		midi.NewNoteOn(0, 60, 100, 50),
		midi.NewNoteOn(0, 61, 100, 150),
		midi.NewNoteOn(0, 62, 100, 250),
		midi.NewNoteOn(0, 63, 100, 350),
	}

	for _, e := range events {
		ctx.AddInputEvent(e)
	}

	// Process events in chunks
	processor := &testProcessor{}

	// Process first chunk [0, 200)
	ctx.ProcessEvents(processor, 0, 200)
	if len(processor.receivedEvents) != 2 {
		t.Errorf("Expected 2 events in first chunk, got %d", len(processor.receivedEvents))
	}

	// Process second chunk [200, 400)
	processor.receivedEvents = nil
	ctx.ProcessEvents(processor, 200, 400)
	if len(processor.receivedEvents) != 2 {
		t.Errorf("Expected 2 events in second chunk, got %d", len(processor.receivedEvents))
	}
}

func TestContextClearAllEvents(t *testing.T) {
	registry := param.NewRegistry()
	ctx := NewContext(512, registry)

	// Add both input and output events
	ctx.AddInputEvent(midi.NewNoteOn(0, 60, 100, 100))
	ctx.AddOutputEvent(midi.NewControlChange(0, midi.CCVolume, 100, 50))

	// Clear all events
	ctx.ClearAllEvents()

	if ctx.HasInputEvents() {
		t.Error("Expected no input events after ClearAllEvents")
	}

	if len(ctx.GetOutputEvents()) != 0 {
		t.Error("Expected no output events after ClearAllEvents")
	}
}

func TestContextParameterAccessorsHandleMissingParameters(t *testing.T) {
	ctx := NewContext(128, param.NewRegistry())

	if got := ctx.Param(999); got != 0 {
		t.Fatalf("Param(999) = %f, want 0", got)
	}

	if got := ctx.ParamPlain(999); got != 0 {
		t.Fatalf("ParamPlain(999) = %f, want 0", got)
	}
}

// HasInputEvents used to build a full copy of the block's events just to test
// emptiness, which allocates on the audio thread.
func TestHasInputEventsDoesNotAllocate(t *testing.T) {
	ctx := NewContext(512, param.NewRegistry())
	ctx.Input = [][]float32{make([]float32, 512)}

	for i := range int32(16) {
		ctx.AddInputEvent(midi.NewNoteOn(0, 60, 100, i*8))
	}

	if !ctx.HasInputEvents() {
		t.Fatal("expected input events")
	}

	if allocs := testing.AllocsPerRun(100, func() {
		_ = ctx.HasInputEvents()
	}); allocs != 0 {
		t.Fatalf("expected zero allocations, got %f", allocs)
	}

	ctx.ClearInputEvents()
	if allocs := testing.AllocsPerRun(100, func() {
		_ = ctx.HasInputEvents()
	}); allocs != 0 {
		t.Fatalf("expected zero allocations when empty, got %f", allocs)
	}
}
