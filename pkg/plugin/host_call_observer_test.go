package plugin

import (
	"slices"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

type hostCallProbe struct {
	recordingProcessor
	calls []HostCall
}

func (p *hostCallProbe) ObserveHostCall(call HostCall) {
	p.calls = append(p.calls, call)
}

func TestObservedHostCallOrder(t *testing.T) {
	parameters := param.NewRegistry()
	if err := parameters.Add(param.New(7, "Probe").Range(0, 1).Default(.25).Build()); err != nil {
		t.Fatal(err)
	}
	p := &hostCallProbe{recordingProcessor: recordingProcessor{params: parameters, buses: bus.Stereo()}}
	c := newComponent(p)
	if err := c.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	if err := c.SetIOMode(2); err != nil {
		t.Fatal(err)
	}
	if err := c.SetupProcessing(&vst3.ProcessSetup{SampleRate: 96000, MaxSamplesPerBlock: 257}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetActive(true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetProcessing(true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParamNormalized(7, .75); err != nil {
		t.Fatal(err)
	}
	if c.GetParamNormalized(7) != .75 {
		t.Fatal("parameter value was not applied")
	}
	if err := c.SetProcessing(false); err != nil {
		t.Fatal(err)
	}
	if err := c.Terminate(); err != nil {
		t.Fatal(err)
	}
	want := []HostCall{
		{Method: "IComponent.initialize"},
		{Method: "IComponent.setIoMode", Int: [3]int64{2}},
		{Method: "IAudioProcessor.setupProcessing", Int: [3]int64{257}, Float: 96000},
		{Method: "IComponent.setActive", Int: [3]int64{1}},
		{Method: "IAudioProcessor.setProcessing", Int: [3]int64{1}},
		{Method: "IEditController.setParamNormalized", Int: [3]int64{7}, Float: .75},
		{Method: "IEditController.getParamNormalized", Int: [3]int64{7}, Float: .75},
		{Method: "IAudioProcessor.setProcessing"},
		{Method: "IComponent.terminate"},
	}
	if !slices.Equal(p.calls, want) {
		t.Fatalf("host calls = %+v, want %+v", p.calls, want)
	}
}
