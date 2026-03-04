package plugin

import (
	"testing"

	frameworkbus "github.com/cwbudde/vst3go/pkg/framework/bus"
	frameworkparam "github.com/cwbudde/vst3go/pkg/framework/param"
	frameworkprocess "github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/midi"
)

type testProcessor struct {
	params      *frameworkparam.Registry
	buses       *frameworkbus.Configuration
	activeCalls []bool
}

func newTestProcessor() *testProcessor {
	return &testProcessor{
		params: frameworkparam.NewRegistry(),
		buses:  frameworkbus.Stereo(),
	}
}

func (p *testProcessor) Initialize(sampleRate float64, maxBlockSize int32) error { return nil }
func (p *testProcessor) ProcessAudio(ctx *frameworkprocess.Context)               {}
func (p *testProcessor) GetParameters() *frameworkparam.Registry                  { return p.params }
func (p *testProcessor) GetBuses() *frameworkbus.Configuration                    { return p.buses }
func (p *testProcessor) SetActive(active bool) error {
	p.activeCalls = append(p.activeCalls, active)
	return nil
}
func (p *testProcessor) GetLatencySamples() int32 { return 0 }
func (p *testProcessor) GetTailSamples() int32    { return 0 }

func TestComponentPrepareProcessContextClearsBlockState(t *testing.T) {
	processor := newTestProcessor()
	component := newComponent(processor)

	component.sampleRate = 48000
	component.processCtx.AddInputEvent(midi.NoteOnEvent{})
	component.processCtx.AddOutputEvent(midi.NoteOffEvent{})
	component.processCtx.AddParameterChange(1, 0.5, 32)
	component.processCtx.Input = append(component.processCtx.Input, []float32{1})
	component.processCtx.Output = append(component.processCtx.Output, []float32{1})

	component.prepareProcessContext()

	if component.processCtx.SampleRate != 48000 {
		t.Fatalf("SampleRate = %f, want 48000", component.processCtx.SampleRate)
	}
	if component.processCtx.HasParameterChanges() {
		t.Fatal("parameter changes should be cleared for the next block")
	}
	if component.processCtx.HasInputEvents() {
		t.Fatal("input events should be cleared for the next block")
	}
	if len(component.processCtx.GetOutputEvents()) != 0 {
		t.Fatal("output events should be cleared for the next block")
	}
	if len(component.processCtx.Input) != 0 || len(component.processCtx.Output) != 0 {
		t.Fatal("audio buffer slices should be reset for the next block")
	}
}

func TestComponentSetActiveFalseClearsProcessContextState(t *testing.T) {
	processor := newTestProcessor()
	component := newComponent(processor)

	component.processCtx.AddInputEvent(midi.NoteOnEvent{})
	component.processCtx.AddOutputEvent(midi.NoteOffEvent{})
	component.processCtx.AddParameterChange(1, 0.5, 16)

	if err := component.SetActive(false); err != nil {
		t.Fatalf("SetActive(false) error = %v", err)
	}

	if component.processCtx.HasParameterChanges() {
		t.Fatal("parameter changes should be cleared on deactivate")
	}
	if component.processCtx.HasInputEvents() {
		t.Fatal("input events should be cleared on deactivate")
	}
	if len(component.processCtx.GetOutputEvents()) != 0 {
		t.Fatal("output events should be cleared on deactivate")
	}
	if len(processor.activeCalls) != 1 || processor.activeCalls[0] {
		t.Fatal("processor SetActive(false) should still be forwarded")
	}
}
