package midi

import (
	"sort"
	"sync"
)

// EventQueue is a time-ordered collection of MIDI events for a single processing
// block. The queue keeps its events sorted by sample offset at insertion time, so
// the read paths never have to sort and therefore never have to upgrade a read
// lock to a write lock (which would drop the lock and expose the slice to
// concurrent mutation).
//
// Add takes the write lock once per event. An uncontended mutex is a pair of
// atomic operations and allocates nothing, so this remains safe for the audio
// thread as long as producer and consumer do not hammer the queue simultaneously.
type EventQueue struct {
	events []Event
	mu     sync.RWMutex
}

// defaultQueueCapacity preallocates for a typical block's worth of events so the
// audio thread does not grow the slice in the common case.
const defaultQueueCapacity = 128

// NewEventQueue creates an empty queue preallocated for a typical block of events.
func NewEventQueue() *EventQueue {
	return &EventQueue{
		events: make([]Event, 0, defaultQueueCapacity),
	}
}

// Add inserts a single event, keeping the queue ordered by sample offset.
func (q *EventQueue) Add(event Event) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.insert(event)
}

// AddMultiple inserts a batch of events, keeping the queue ordered by sample offset.
func (q *EventQueue) AddMultiple(events []Event) {
	if len(events) == 0 {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for _, event := range events {
		q.insert(event)
	}
}

// insert places event at its ordered position. Events normally arrive in
// ascending time order, so the common case is a plain append; out-of-order
// events shift the (short) tail. Insertion happens after events with an equal
// offset so arrival order is preserved for simultaneous events.
//
// The caller must hold the write lock.
func (q *EventQueue) insert(event Event) {
	offset := event.Offset

	i := len(q.events)
	for i > 0 && q.events[i-1].Offset > offset {
		i--
	}

	if i == len(q.events) {
		q.events = append(q.events, event)
		return
	}

	q.events = append(q.events, Event{})
	copy(q.events[i+1:], q.events[i:])
	q.events[i] = event
}

// GetEventsInRange returns a copy of the events in the half-open range
// [startSample, endSample). It allocates; prefer AppendEventsInRange on the
// audio thread.
func (q *EventQueue) GetEventsInRange(startSample, endSample int32) []Event {
	q.mu.RLock()
	defer q.mu.RUnlock()

	startIdx, endIdx := q.rangeBounds(startSample, endSample)
	if startIdx == endIdx {
		return nil
	}

	result := make([]Event, endIdx-startIdx)
	copy(result, q.events[startIdx:endIdx])
	return result
}

// AppendEventsInRange appends the events in the half-open range
// [startSample, endSample) to dst and returns the extended slice. It performs no
// allocation as long as dst has spare capacity, which makes it the audio-thread
// friendly counterpart of GetEventsInRange.
func (q *EventQueue) AppendEventsInRange(dst []Event, startSample, endSample int32) []Event {
	q.mu.RLock()
	defer q.mu.RUnlock()

	startIdx, endIdx := q.rangeBounds(startSample, endSample)
	return append(dst, q.events[startIdx:endIdx]...)
}

// rangeBounds locates the half-open index range covering [startSample, endSample).
// The caller must hold at least the read lock.
func (q *EventQueue) rangeBounds(startSample, endSample int32) (start, end int) {
	start = sort.Search(len(q.events), func(i int) bool {
		return q.events[i].Offset >= startSample
	})
	end = sort.Search(len(q.events), func(i int) bool {
		return q.events[i].Offset >= endSample
	})
	if end < start {
		end = start
	}
	return start, end
}

// GetAllEvents returns a copy of every queued event in time order. It allocates;
// prefer AppendAllEvents on the audio thread.
func (q *EventQueue) GetAllEvents() []Event {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]Event, len(q.events))
	copy(result, q.events)
	return result
}

// AppendAllEvents appends every queued event in time order to dst and returns the
// extended slice, without allocating when dst has spare capacity.
func (q *EventQueue) AppendAllEvents(dst []Event) []Event {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return append(dst, q.events...)
}

// Clear removes all events, retaining the backing array.
func (q *EventQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Zero the retired entries so any SysEx payload they reference can be
	// collected rather than pinned by the retained backing array.
	clear(q.events)
	q.events = q.events[:0]
}

// RemoveProcessedEvents drops every event before upToSample. The bound is
// exclusive to match the half-open ranges used by GetEventsInRange, so an event
// landing exactly on upToSample survives to be delivered by the next block.
func (q *EventQueue) RemoveProcessedEvents(upToSample int32) {
	q.mu.Lock()
	defer q.mu.Unlock()

	keepIdx := sort.Search(len(q.events), func(i int) bool {
		return q.events[i].Offset >= upToSample
	})
	if keepIdx == 0 {
		return
	}

	n := copy(q.events, q.events[keepIdx:])
	clear(q.events[n:])
	q.events = q.events[:n]
}

// Size returns the number of queued events.
func (q *EventQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.events)
}

// IsEmpty reports whether the queue holds no events.
func (q *EventQueue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.events) == 0
}

// OffsetEvents shifts every queued event in time by offset samples, which is how
// a block-relative queue is rebased onto a sub-chunk boundary. Shifting all
// events by the same amount preserves the ordering invariant.
func (q *EventQueue) OffsetEvents(offset int32) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.events {
		q.events[i].Offset += offset
	}
}

// EventProcessor consumes MIDI events one at a time.
type EventProcessor interface {
	ProcessEvent(event Event)
}

// ProcessEvents hands every event in the half-open range [startSample, endSample)
// to processor in time order.
//
// It snapshots the range first so processor may mutate the queue, which costs an
// allocation. On the audio thread prefer AppendEventsInRange into a retained
// scratch slice; process.Context.ProcessEvents does exactly that.
func (q *EventQueue) ProcessEvents(processor EventProcessor, startSample, endSample int32) {
	events := q.GetEventsInRange(startSample, endSample)
	for _, event := range events {
		processor.ProcessEvent(event)
	}
}

// EventBuffer pairs an incoming and an outgoing MIDI event queue for one plugin
// instance.
type EventBuffer struct {
	inputQueue  *EventQueue
	outputQueue *EventQueue
}

// NewEventBuffer creates an empty input/output event buffer pair.
func NewEventBuffer() *EventBuffer {
	return &EventBuffer{
		inputQueue:  NewEventQueue(),
		outputQueue: NewEventQueue(),
	}
}

// AddInputEvent queues an event received from the host.
func (b *EventBuffer) AddInputEvent(event Event) {
	b.inputQueue.Add(event)
}

// AddOutputEvent queues an event to be sent back to the host.
func (b *EventBuffer) AddOutputEvent(event Event) {
	b.outputQueue.Add(event)
}

// GetInputEvents returns the input events in the half-open range
// [startSample, endSample).
func (b *EventBuffer) GetInputEvents(startSample, endSample int32) []Event {
	return b.inputQueue.GetEventsInRange(startSample, endSample)
}

// AppendInputEvents appends the input events in [startSample, endSample) to dst
// without allocating when dst has spare capacity.
func (b *EventBuffer) AppendInputEvents(dst []Event, startSample, endSample int32) []Event {
	return b.inputQueue.AppendEventsInRange(dst, startSample, endSample)
}

// GetOutputEvents returns all queued output events in time order.
func (b *EventBuffer) GetOutputEvents() []Event {
	return b.outputQueue.GetAllEvents()
}

// AppendOutputEvents appends all queued output events to dst without allocating
// when dst has spare capacity.
func (b *EventBuffer) AppendOutputEvents(dst []Event) []Event {
	return b.outputQueue.AppendAllEvents(dst)
}

// ClearInput drops all queued input events.
func (b *EventBuffer) ClearInput() {
	b.inputQueue.Clear()
}

// ClearOutput drops all queued output events.
func (b *EventBuffer) ClearOutput() {
	b.outputQueue.Clear()
}

// ClearAll drops all queued input and output events.
func (b *EventBuffer) ClearAll() {
	b.inputQueue.Clear()
	b.outputQueue.Clear()
}
