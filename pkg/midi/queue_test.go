package midi

import (
	"errors"
	"sync"
	"testing"
)

func TestEventQueue(t *testing.T) {
	q := NewEventQueue()

	// Test empty queue
	if !q.IsEmpty() {
		t.Error("Expected queue to be empty")
	}
	if q.Size() != 0 {
		t.Errorf("Expected size 0, got %d", q.Size())
	}

	// Add events
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 60, Velocity: 100})
	q.Add(NoteOffEvent{BaseEvent: BaseEvent{Offset: 200}, NoteNumber: 60, Velocity: 0})
	q.Add(ControlChangeEvent{BaseEvent: BaseEvent{Offset: 50}, Controller: CCSustain, Value: 127})

	if q.IsEmpty() {
		t.Error("Expected queue to not be empty")
	}
	if q.Size() != 3 {
		t.Errorf("Expected size 3, got %d", q.Size())
	}
}

func TestEventQueueSorting(t *testing.T) {
	q := NewEventQueue()

	// Add events out of order
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 300}, NoteNumber: 62, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 60, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 200}, NoteNumber: 61, Velocity: 100})

	events := q.GetAllEvents()
	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	// Check that events are sorted by offset
	offsets := []int32{100, 200, 300}
	for i, event := range events {
		if event.SampleOffset() != offsets[i] {
			t.Errorf("Event %d: expected offset %d, got %d", i, offsets[i], event.SampleOffset())
		}
	}
}

func TestGetEventsInRange(t *testing.T) {
	q := NewEventQueue()

	// Add events
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 0}, NoteNumber: 60, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 50}, NoteNumber: 61, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 62, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 150}, NoteNumber: 63, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 200}, NoteNumber: 64, Velocity: 100})

	// Test different ranges
	tests := []struct {
		start    int32
		end      int32
		expected int
	}{
		{0, 100, 2},   // Events at 0 and 50
		{50, 150, 2},  // Events at 50 and 100
		{100, 200, 2}, // Events at 100 and 150
		{0, 250, 5},   // All events
		{250, 300, 0}, // No events
		{-50, 0, 0},   // Before first event
	}

	for _, tt := range tests {
		events := q.GetEventsInRange(tt.start, tt.end)
		if len(events) != tt.expected {
			t.Errorf("Range [%d, %d): expected %d events, got %d",
				tt.start, tt.end, tt.expected, len(events))
		}
	}
}

func TestRemoveProcessedEvents(t *testing.T) {
	q := NewEventQueue()

	// Add events
	for i := int32(0); i < 5; i++ {
		q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: i * 50}, NoteNumber: 60 + uint8(i), Velocity: 100})
	}

	// Remove events up to sample 125 (should remove first 3 events: 0, 50, 100)
	q.RemoveProcessedEvents(125)

	if q.Size() != 2 {
		t.Errorf("Expected 2 events remaining, got %d", q.Size())
	}

	remaining := q.GetAllEvents()
	if len(remaining) != 2 {
		t.Fatalf("Expected 2 remaining events, got %d", len(remaining))
	}

	// Check that remaining events are at offsets 150 and 200
	if remaining[0].SampleOffset() != 150 {
		t.Errorf("Expected first remaining event at offset 150, got %d", remaining[0].SampleOffset())
	}
	if remaining[1].SampleOffset() != 200 {
		t.Errorf("Expected second remaining event at offset 200, got %d", remaining[1].SampleOffset())
	}
}

func TestOffsetEvents(t *testing.T) {
	q := NewEventQueue()

	// Add various event types
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 60, Velocity: 100})
	q.Add(NoteOffEvent{BaseEvent: BaseEvent{Offset: 200}, NoteNumber: 60, Velocity: 0})
	q.Add(ControlChangeEvent{BaseEvent: BaseEvent{Offset: 50}, Controller: CCSustain, Value: 127})
	q.Add(PitchBendEvent{BaseEvent: BaseEvent{Offset: 150}, Value: 1000})

	// Offset all events by 100
	if err := q.OffsetEvents(100); err != nil {
		t.Fatalf("OffsetEvents() error = %v", err)
	}

	events := q.GetAllEvents()
	expectedOffsets := []int32{150, 200, 250, 300} // Sorted order

	for i, event := range events {
		if event.SampleOffset() != expectedOffsets[i] {
			t.Errorf("Event %d: expected offset %d, got %d",
				i, expectedOffsets[i], event.SampleOffset())
		}
	}
}

func TestEventBuffer(t *testing.T) {
	buffer := NewEventBuffer()

	// Test input events
	buffer.AddInputEvent(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 60, Velocity: 100})
	buffer.AddInputEvent(NoteOffEvent{BaseEvent: BaseEvent{Offset: 200}, NoteNumber: 60, Velocity: 0})

	inputEvents := buffer.GetInputEvents(0, 300)
	if len(inputEvents) != 2 {
		t.Errorf("Expected 2 input events, got %d", len(inputEvents))
	}

	// Test output events
	buffer.AddOutputEvent(ControlChangeEvent{BaseEvent: BaseEvent{Offset: 50}, Controller: CCVolume, Value: 100})

	outputEvents := buffer.GetOutputEvents()
	if len(outputEvents) != 1 {
		t.Errorf("Expected 1 output event, got %d", len(outputEvents))
	}

	// Test clear functions
	buffer.ClearInput()
	if len(buffer.GetInputEvents(0, 1000)) != 0 {
		t.Error("Expected input queue to be empty after clear")
	}

	buffer.ClearOutput()
	if len(buffer.GetOutputEvents()) != 0 {
		t.Error("Expected output queue to be empty after clear")
	}
}

type testEventProcessor struct {
	processedEvents []Event
}

func (p *testEventProcessor) ProcessEvent(event Event) {
	p.processedEvents = append(p.processedEvents, event)
}

func TestProcessEvents(t *testing.T) {
	q := NewEventQueue()
	processor := &testEventProcessor{}

	// Add events
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 50}, NoteNumber: 60, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 150}, NoteNumber: 61, Velocity: 100})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 250}, NoteNumber: 62, Velocity: 100})

	// Process events in range [0, 200)
	q.ProcessEvents(processor, 0, 200)

	if len(processor.processedEvents) != 2 {
		t.Fatalf("Expected 2 processed events, got %d", len(processor.processedEvents))
	}

	// Verify correct events were processed
	if processor.processedEvents[0].SampleOffset() != 50 {
		t.Errorf("Expected first event at offset 50, got %d", processor.processedEvents[0].SampleOffset())
	}
	if processor.processedEvents[1].SampleOffset() != 150 {
		t.Errorf("Expected second event at offset 150, got %d", processor.processedEvents[1].SampleOffset())
	}
}

func TestConcurrentAccess(t *testing.T) {
	q := NewEventQueue()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: int32(i)}, NoteNumber: 60, Velocity: 100})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = q.GetEventsInRange(0, 100)
			_ = q.Size()
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	if q.Size() != 100 {
		t.Errorf("Expected 100 events, got %d", q.Size())
	}
}

func TestGetEventsInRangeIsHalfOpen(t *testing.T) {
	q := NewEventQueue()
	for _, off := range []int32{0, 64, 128, 192} {
		q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: off}, NoteNumber: 60, Velocity: 100})
	}

	tests := []struct {
		name    string
		start   int32
		end     int32
		offsets []int32
	}{
		{"start bound is inclusive", 64, 128, []int32{64}},
		{"end bound is exclusive", 0, 128, []int32{0, 64}},
		{"empty range on an event", 64, 64, nil},
		{"single event window", 128, 129, []int32{128}},
		{"past the end", 192, 1000, []int32{192}},
		{"inverted range", 128, 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := q.GetEventsInRange(tt.start, tt.end)
			if len(got) != len(tt.offsets) {
				t.Fatalf("GetEventsInRange(%d, %d) returned %d events, want %d",
					tt.start, tt.end, len(got), len(tt.offsets))
			}
			for i, want := range tt.offsets {
				if got[i].SampleOffset() != want {
					t.Errorf("event %d offset = %d, want %d", i, got[i].SampleOffset(), want)
				}
			}
		})
	}
}

// A chunk boundary event must survive RemoveProcessedEvents: GetEventsInRange is
// half-open, so an event at exactly the boundary has not been delivered yet.
func TestRemoveProcessedEventsBoundaryIsExclusive(t *testing.T) {
	const boundary int32 = 128

	tests := []struct {
		name     string
		upTo     int32
		inputs   []int32
		expected []int32
	}{
		{"keeps event exactly on the boundary", boundary, []int32{0, 127, 128, 129}, []int32{128, 129}},
		{"removes nothing before the first event", 0, []int32{0, 64}, []int32{0, 64}},
		{"removes everything below the bound", 200, []int32{0, 64, 128}, nil},
		{"duplicate offsets on the boundary all survive", boundary, []int32{128, 128, 64}, []int32{128, 128}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewEventQueue()
			for _, off := range tt.inputs {
				q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: off}, NoteNumber: 60, Velocity: 100})
			}

			q.RemoveProcessedEvents(tt.upTo)

			remaining := q.GetAllEvents()
			if len(remaining) != len(tt.expected) {
				t.Fatalf("after RemoveProcessedEvents(%d): %d events remain, want %d",
					tt.upTo, len(remaining), len(tt.expected))
			}
			for i, want := range tt.expected {
				if remaining[i].SampleOffset() != want {
					t.Errorf("remaining event %d offset = %d, want %d", i, remaining[i].SampleOffset(), want)
				}
			}
		})
	}
}

// A block processed as chunks [0,128) then [128,256) must deliver each event
// exactly once, which only holds if both bounds use the same convention.
func TestChunkedDeliveryLosesNoEvent(t *testing.T) {
	q := NewEventQueue()
	offsets := []int32{0, 127, 128, 129, 255}
	for _, off := range offsets {
		q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: off}, NoteNumber: 60, Velocity: 100})
	}

	var delivered []int32
	for start := int32(0); start < 256; start += 128 {
		for _, e := range q.GetEventsInRange(start, start+128) {
			delivered = append(delivered, e.SampleOffset())
		}
		q.RemoveProcessedEvents(start + 128)
	}

	if len(delivered) != len(offsets) {
		t.Fatalf("delivered %v, want %v", delivered, offsets)
	}
	for i, want := range offsets {
		if delivered[i] != want {
			t.Fatalf("delivered %v, want %v", delivered, offsets)
		}
	}
	if !q.IsEmpty() {
		t.Errorf("queue not drained, %d events remain", q.Size())
	}
}

func TestAppendEventsInRange(t *testing.T) {
	q := NewEventQueue()
	for _, off := range []int32{10, 20, 30, 40} {
		q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: off}, NoteNumber: 60, Velocity: 100})
	}

	dst := make([]Event, 0, 8)
	dst = q.AppendEventsInRange(dst, 20, 40)
	if len(dst) != 2 || dst[0].SampleOffset() != 20 || dst[1].SampleOffset() != 30 {
		t.Fatalf("AppendEventsInRange = %v, want events at 20 and 30", dst)
	}

	// Appending again must extend rather than replace.
	dst = q.AppendEventsInRange(dst, 40, 50)
	if len(dst) != 3 || dst[2].SampleOffset() != 40 {
		t.Fatalf("second AppendEventsInRange = %v, want a third event at 40", dst)
	}

	all := q.AppendAllEvents(dst[:0])
	if len(all) != 4 {
		t.Fatalf("AppendAllEvents returned %d events, want 4", len(all))
	}
}

func TestAppendEventsInRangeDoesNotAllocate(t *testing.T) {
	q := NewEventQueue()
	for i := int32(0); i < 32; i++ {
		q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: i * 8}, NoteNumber: 60, Velocity: 100})
	}

	dst := make([]Event, 0, 64)
	dst = q.AppendEventsInRange(dst, 0, 256) // warm up

	allocs := testing.AllocsPerRun(100, func() {
		dst = q.AppendEventsInRange(dst[:0], 0, 256)
	})
	if allocs != 0 {
		t.Fatalf("AppendEventsInRange allocated %f times, want 0", allocs)
	}

	allocs = testing.AllocsPerRun(100, func() {
		dst = q.AppendAllEvents(dst[:0])
	})
	if allocs != 0 {
		t.Fatalf("AppendAllEvents allocated %f times, want 0", allocs)
	}
}

func TestOffsetEventsCoversEveryEventType(t *testing.T) {
	events := []Event{
		NoteOnEvent{BaseEvent: BaseEvent{Offset: 1}},
		NoteOffEvent{BaseEvent: BaseEvent{Offset: 2}},
		ControlChangeEvent{BaseEvent: BaseEvent{Offset: 3}},
		PitchBendEvent{BaseEvent: BaseEvent{Offset: 4}},
		PolyPressureEvent{BaseEvent: BaseEvent{Offset: 5}},
		ChannelPressureEvent{BaseEvent: BaseEvent{Offset: 6}},
		ProgramChangeEvent{BaseEvent: BaseEvent{Offset: 7}},
		SystemExclusiveEvent{BaseEvent: BaseEvent{Offset: 8}},
		ClockEvent{BaseEvent: BaseEvent{Offset: 9}},
		StartEvent{BaseEvent: BaseEvent{Offset: 10}},
		StopEvent{BaseEvent: BaseEvent{Offset: 11}},
		ContinueEvent{BaseEvent: BaseEvent{Offset: 12}},
		ResetEvent{BaseEvent: BaseEvent{Offset: 13}},
		ActiveSensingEvent{BaseEvent: BaseEvent{Offset: 14}},
	}

	q := NewEventQueue()
	q.AddMultiple(events)

	const delta int32 = 1000
	if err := q.OffsetEvents(delta); err != nil {
		t.Fatalf("OffsetEvents() error = %v", err)
	}

	got := q.GetAllEvents()
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	for i, e := range got {
		want := events[i].SampleOffset() + delta
		if e.SampleOffset() != want {
			t.Errorf("%T offset = %d, want %d", e, e.SampleOffset(), want)
		}
		if e.Type() != events[i].Type() {
			t.Errorf("event %d type changed from %v to %v", i, events[i].Type(), e.Type())
		}
	}
}

// foreignEvent is an Event implementation from outside this package; it cannot be
// time-shifted and must be reported rather than silently left in place.
type foreignEvent struct{}

func (foreignEvent) Type() EventType     { return EventTypeNoteOn }
func (foreignEvent) Channel() uint8      { return 0 }
func (foreignEvent) SampleOffset() int32 { return 0 }
func (foreignEvent) String() string      { return "foreign" }

func TestOffsetEventsRejectsUnshiftableEvent(t *testing.T) {
	q := NewEventQueue()
	q.Add(foreignEvent{})

	if err := q.OffsetEvents(10); !errors.Is(err, ErrUnshiftableEvent) {
		t.Fatalf("OffsetEvents() error = %v, want ErrUnshiftableEvent", err)
	}
}

// TestEventQueueRace hammers the queue from a writer and a reader to catch the
// read-lock upgrade that used to drop the lock mid-read. Run with -race.
func TestEventQueueRace(t *testing.T) {
	q := NewEventQueue()

	const iterations = 2000
	var wg sync.WaitGroup
	wg.Add(3)

	// Writer: deliberately interleaves ascending and descending offsets so the
	// ordering invariant is exercised, not just the append fast path.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: int32(i % 512)}, NoteNumber: 60, Velocity: 100})
			q.AddMultiple([]Event{
				ControlChangeEvent{BaseEvent: BaseEvent{Offset: int32(511 - i%512)}, Controller: CCVolume},
			})
		}
	}()

	// Reader: both the allocating and the append-based read paths.
	go func() {
		defer wg.Done()
		buf := make([]Event, 0, 1024)
		for i := 0; i < iterations; i++ {
			prev := int32(-1)
			for _, e := range q.GetEventsInRange(0, 512) {
				if e.SampleOffset() < prev {
					t.Errorf("events out of order: %d after %d", e.SampleOffset(), prev)
					return
				}
				prev = e.SampleOffset()
			}
			buf = q.AppendAllEvents(buf[:0])
			_ = q.Size()
			_ = q.IsEmpty()
		}
	}()

	// Mutator: clears and trims underneath the reader.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			q.RemoveProcessedEvents(int32(i % 512))
			if i%128 == 0 {
				q.Clear()
			}
		}
	}()

	wg.Wait()
}

func TestEventQueueKeepsInsertionOrderForEqualOffsets(t *testing.T) {
	q := NewEventQueue()
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 1})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 50}, NoteNumber: 2})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 3})
	q.Add(NoteOnEvent{BaseEvent: BaseEvent{Offset: 100}, NoteNumber: 4})

	want := []uint8{2, 1, 3, 4}
	for i, e := range q.GetAllEvents() {
		note := e.(NoteOnEvent).NoteNumber
		if note != want[i] {
			t.Errorf("position %d holds note %d, want %d", i, note, want[i])
		}
	}
}
