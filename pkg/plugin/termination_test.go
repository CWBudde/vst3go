package plugin

import (
	"errors"
	"testing"
)

type terminationProbe struct {
	*recordingProcessor
	calls int
	err   error
}

func (p *terminationProbe) Terminate() error {
	p.calls++
	return p.err
}

func TestTerminateReleasesProcessorResources(t *testing.T) {
	stopErr := errors.New("nested detector could not stop")
	probe := &terminationProbe{recordingProcessor: &recordingProcessor{}, err: stopErr}
	component := newComponent(probe)
	if err := component.Terminate(); !errors.Is(err, stopErr) {
		t.Fatalf("first termination error = %v, want %v", err, stopErr)
	}

	probe.err = nil
	for range 2 {
		if err := component.Terminate(); err != nil {
			t.Fatal(err)
		}
	}
	if probe.calls != 2 {
		t.Fatalf("processor termination calls = %d, want 2", probe.calls)
	}
}
