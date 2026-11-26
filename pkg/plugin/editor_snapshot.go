package plugin

import (
	"math"
	"sync/atomic"
)

// EditorSnapshot transfers a fixed-size frame of meter or plot values from one
// audio producer to any number of UI readers. Give each processor instance its
// own snapshot and allocate it before starting audio. Reopening a view can
// reuse the same snapshot without synchronizing with the audio thread.
type EditorSnapshot struct {
	sequence atomic.Uint64
	values   []atomic.Uint32
}

// NewEditorSnapshot allocates storage for size float32 values.
func NewEditorSnapshot(size int) *EditorSnapshot {
	return &EditorSnapshot{values: make([]atomic.Uint32, size)}
}

// Publish stores one complete frame without allocating or locking. Only the
// audio producer may call Publish. A slice of the wrong size is ignored.
func (s *EditorSnapshot) Publish(values []float32) {
	if len(values) != len(s.values) {
		return
	}
	s.sequence.Add(1)
	for i, value := range values {
		s.values[i].Store(math.Float32bits(value))
	}
	s.sequence.Add(1)
}

// Copy reads a complete frame into dst. If the writer overlaps the read or
// dst has the wrong length, Copy returns false and dst must not be displayed.
// Call again on a later UI update. Before the first Publish the frame is zero.
func (s *EditorSnapshot) Copy(dst []float32) bool {
	sequence := s.sequence.Load()
	if len(dst) != len(s.values) || sequence&1 != 0 {
		return false
	}
	for i := range dst {
		dst[i] = math.Float32frombits(s.values[i].Load())
	}
	return s.sequence.Load() == sequence
}
