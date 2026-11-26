package plugin

import (
	"errors"
	"slices"
	"testing"
)

type nestedProcessingProbe struct {
	*recordingProcessor
	transitions []bool
	err         error
}

func (p *nestedProcessingProbe) SetProcessing(active bool) error {
	p.transitions = append(p.transitions, active)
	return p.err
}

func TestProcessingTransitionsReachNestedProcessor(t *testing.T) {
	probe := &nestedProcessingProbe{recordingProcessor: &recordingProcessor{}}
	component := newComponent(probe)
	for _, active := range []bool{true, false, true} {
		if err := component.SetProcessing(active); err != nil {
			t.Fatal(err)
		}
		if component.processing.Load() != active {
			t.Fatalf("processing state after transition is %v, want %v", component.processing.Load(), active)
		}
	}
	if !slices.Equal(probe.transitions, []bool{true, false, true}) {
		t.Fatalf("nested transitions = %v", probe.transitions)
	}
	probe.err = errors.New("nested stop failed")
	if err := component.SetProcessing(false); !errors.Is(err, probe.err) || !component.processing.Load() {
		t.Fatalf("failed stop changed state or lost error: processing=%v, error=%v", component.processing.Load(), err)
	}
}
