package midi

import (
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
	q.Add(NewNoteOn(0, 60, 100, 100))
	q.Add(NewNoteOff(0, 60, 0, 200))
	q.Add(NewControlChange(0, CCSustain, 127, 50))

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
	q.Add(NewNoteOn(0, 62, 100, 300))
	q.Add(NewNoteOn(0, 60, 100, 100))
	q.Add(NewNoteOn(0, 61, 100, 200))

	events := q.GetAllEvents()
	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	// Check that events are sorted by offset
	offsets := []int32{100, 200, 300}
	for i, event := range events {
		if event.Offset != offsets[i] {
			t.Errorf("Event %d: expected offset %d, got %d", i, offsets[i], event.Offset)
		}
	}
}

func TestGetEventsInRange(t *testing.T) {
	q := NewEventQueue()

	// Add events
	q.Add(NewNoteOn(0, 60, 100, 0))
	q.Add(NewNoteOn(0, 61, 100, 50))
	q.Add(NewNoteOn(0, 62, 100, 100))
	q.Add(NewNoteOn(0, 63, 100, 150))
	q.Add(NewNoteOn(0, 64, 100, 200))

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
		q.Add(NewNoteOn(0, 60+uint8(i), 100, i*50))
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
	if remaining[0].Offset != 150 {
		t.Errorf("Expected first remaining event at offset 150, got %d", remaining[0].Offset)
	}
	if remaining[1].Offset != 200 {
		t.Errorf("Expected second remaining event at offset 200, got %d", remaining[1].Offset)
	}
}

func TestOffsetEvents(t *testing.T) {
	q := NewEventQueue()

	// Add various event types
	q.Add(NewNoteOn(0, 60, 100, 100))
	q.Add(NewNoteOff(0, 60, 0, 200))
	q.Add(NewControlChange(0, CCSustain, 127, 50))
	q.Add(NewPitchBend(0, 1000, 150))

	// Offset all events by 100
	q.OffsetEvents(100)

	events := q.GetAllEvents()
	expectedOffsets := []int32{150, 200, 250, 300} // Sorted order

	for i, event := range events {
		if event.Offset != expectedOffsets[i] {
			t.Errorf("Event %d: expected offset %d, got %d",
				i, expectedOffsets[i], event.Offset)
		}
	}
}

func TestEventBuffer(t *testing.T) {
	buffer := NewEventBuffer()

	// Test input events
	buffer.AddInputEvent(NewNoteOn(0, 60, 100, 100))
	buffer.AddInputEvent(NewNoteOff(0, 60, 0, 200))

	inputEvents := buffer.GetInputEvents(0, 300)
	if len(inputEvents) != 2 {
		t.Errorf("Expected 2 input events, got %d", len(inputEvents))
	}

	// Test output events
	buffer.AddOutputEvent(NewControlChange(0, CCVolume, 100, 50))

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

// The append-based EventBuffer API is what the audio thread actually uses;
// GetInputEvents/GetOutputEvents allocate and exist for convenience.
func TestEventBufferAppendAndClearAll(t *testing.T) {
	buffer := NewEventBuffer()

	buffer.AddInputEvent(NewNoteOn(0, 60, 100, 10))
	buffer.AddInputEvent(NewNoteOn(0, 61, 100, 200))
	buffer.AddOutputEvent(NewControlChange(0, CCVolume, 64, 5))

	dst := make([]Event, 0, 8)

	dst = buffer.AppendInputEvents(dst, 0, 100)
	if len(dst) != 1 || dst[0].Note() != 60 {
		t.Fatalf("AppendInputEvents([0,100)) = %v, want the note at offset 10", dst)
	}

	// A second append must extend rather than replace.
	dst = buffer.AppendOutputEvents(dst)
	if len(dst) != 2 || dst[1].Controller() != CCVolume {
		t.Fatalf("AppendOutputEvents = %v, want the volume CC appended", dst)
	}

	buffer.ClearAll()

	if got := buffer.AppendInputEvents(dst[:0], 0, 1000); len(got) != 0 {
		t.Errorf("ClearAll left %d input events", len(got))
	}
	if got := buffer.AppendOutputEvents(dst[:0]); len(got) != 0 {
		t.Errorf("ClearAll left %d output events", len(got))
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
	q.Add(NewNoteOn(0, 60, 100, 50))
	q.Add(NewNoteOn(0, 61, 100, 150))
	q.Add(NewNoteOn(0, 62, 100, 250))

	// Process events in range [0, 200)
	q.ProcessEvents(processor, 0, 200)

	if len(processor.processedEvents) != 2 {
		t.Fatalf("Expected 2 processed events, got %d", len(processor.processedEvents))
	}

	// Verify correct events were processed
	if processor.processedEvents[0].Offset != 50 {
		t.Errorf("Expected first event at offset 50, got %d", processor.processedEvents[0].Offset)
	}
	if processor.processedEvents[1].Offset != 150 {
		t.Errorf("Expected second event at offset 150, got %d", processor.processedEvents[1].Offset)
	}
}

func TestConcurrentAccess(t *testing.T) {
	q := NewEventQueue()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			q.Add(NewNoteOn(0, 60, 100, int32(i)))
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
		q.Add(NewNoteOn(0, 60, 100, off))
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
				if got[i].Offset != want {
					t.Errorf("event %d offset = %d, want %d", i, got[i].Offset, want)
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
				q.Add(NewNoteOn(0, 60, 100, off))
			}

			q.RemoveProcessedEvents(tt.upTo)

			remaining := q.GetAllEvents()
			if len(remaining) != len(tt.expected) {
				t.Fatalf("after RemoveProcessedEvents(%d): %d events remain, want %d",
					tt.upTo, len(remaining), len(tt.expected))
			}
			for i, want := range tt.expected {
				if remaining[i].Offset != want {
					t.Errorf("remaining event %d offset = %d, want %d", i, remaining[i].Offset, want)
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
		q.Add(NewNoteOn(0, 60, 100, off))
	}

	var delivered []int32
	for start := int32(0); start < 256; start += 128 {
		for _, e := range q.GetEventsInRange(start, start+128) {
			delivered = append(delivered, e.Offset)
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
		q.Add(NewNoteOn(0, 60, 100, off))
	}

	dst := make([]Event, 0, 8)
	dst = q.AppendEventsInRange(dst, 20, 40)
	if len(dst) != 2 || dst[0].Offset != 20 || dst[1].Offset != 30 {
		t.Fatalf("AppendEventsInRange = %v, want events at 20 and 30", dst)
	}

	// Appending again must extend rather than replace.
	dst = q.AppendEventsInRange(dst, 40, 50)
	if len(dst) != 3 || dst[2].Offset != 40 {
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
		q.Add(NewNoteOn(0, 60, 100, i*8))
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
		NewNoteOn(0, 0, 0, 1),
		NewNoteOff(0, 0, 0, 2),
		NewControlChange(0, 0, 0, 3),
		NewPitchBend(0, 0, 4),
		NewPolyPressure(0, 0, 0, 5),
		NewChannelPressure(0, 0, 6),
		NewProgramChange(0, 0, 7),
		NewSysEx(nil, 8),
		NewSystemMessage(EventTypeClock, 9),
		NewSystemMessage(EventTypeStart, 10),
		NewSystemMessage(EventTypeStop, 11),
		NewSystemMessage(EventTypeContinue, 12),
		NewSystemMessage(EventTypeReset, 13),
		NewSystemMessage(EventTypeActiveSensing, 14),
	}

	q := NewEventQueue()
	q.AddMultiple(events)

	const delta int32 = 1000
	q.OffsetEvents(delta)

	got := q.GetAllEvents()
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	for i, e := range got {
		want := events[i].Offset + delta
		if e.Offset != want {
			t.Errorf("%v offset = %d, want %d", e.Type, e.Offset, want)
		}
		if e.Type != events[i].Type {
			t.Errorf("event %d type changed from %v to %v", i, events[i].Type, e.Type)
		}
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
			q.Add(NewNoteOn(0, 60, 100, int32(i%512)))
			q.AddMultiple([]Event{
				NewControlChange(0, CCVolume, 0, int32(511-i%512)),
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
				if e.Offset < prev {
					t.Errorf("events out of order: %d after %d", e.Offset, prev)
					return
				}
				prev = e.Offset
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
	q.Add(NewNoteOn(0, 1, 0, 100))
	q.Add(NewNoteOn(0, 2, 0, 50))
	q.Add(NewNoteOn(0, 3, 0, 100))
	q.Add(NewNoteOn(0, 4, 0, 100))

	want := []uint8{2, 1, 3, 4}
	for i, e := range q.GetAllEvents() {
		note := e.Note()
		if note != want[i] {
			t.Errorf("position %d holds note %d, want %d", i, note, want[i])
		}
	}
}
