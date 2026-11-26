package plugin

import (
	"slices"
	"testing"
)

func TestEditorSnapshot(t *testing.T) {
	snapshot := NewEditorSnapshot(3)
	samples := []float32{0.25, -0.5, 1}
	result := make([]float32, len(samples))
	if !snapshot.Copy(result) || !slices.Equal(result, []float32{0, 0, 0}) {
		t.Fatalf("initial frame = %v, want zeros", result)
	}
	if allocs := testing.AllocsPerRun(100, func() { snapshot.Publish(samples) }); allocs != 0 {
		t.Errorf("Publish allocated %g times per frame", allocs)
	}
	snapshot.Publish(samples[:2])
	if snapshot.Copy(result[:2]) {
		t.Error("Copy succeeded with a destination of the wrong size")
	}
	if !snapshot.Copy(result) || !slices.Equal(result, samples) {
		t.Fatalf("frame = %v, want %v", result, samples)
	}
}

func TestEditorSnapshotConcurrentFrames(t *testing.T) {
	const frames = 1000
	snapshot := NewEditorSnapshot(32)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var frame [32]float32
		for n := 1; n <= frames; n++ {
			for i := range frame {
				frame[i] = float32(n)
			}
			snapshot.Publish(frame[:])
		}
	}()
	var frame [32]float32
	for range frames {
		if snapshot.Copy(frame[:]) {
			for _, point := range frame[1:] {
				if point != frame[0] {
					t.Errorf("mixed frame: %v", frame)
					break
				}
			}
		}
	}
	<-finished
	if !snapshot.Copy(frame[:]) || frame[0] != frames || frame[len(frame)-1] != frames {
		t.Fatalf("last frame = %v, want %d in each slot", frame, frames)
	}
}
