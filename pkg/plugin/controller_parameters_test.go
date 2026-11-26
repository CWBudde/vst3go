package plugin

import (
	"math"
	"slices"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
)

type controllerParameterWrite struct {
	id           uint32
	value, saved float64
}

type controllerParameterProbe struct {
	recordingProcessor
	writes []controllerParameterWrite
}

func (p *controllerParameterProbe) ControllerParameterChanged(id uint32, value float64) {
	p.writes = append(p.writes, controllerParameterWrite{id, value, p.params.Get(id).GetNormalized()})
}

func TestControllerParameterWritesNotifyTheProcessor(t *testing.T) {
	registry := param.NewRegistry()
	if err := registry.Add(param.New(7, "Cutoff").Range(0, 1).Default(0.25).Build()); err != nil {
		t.Fatal(err)
	}
	probe := &controllerParameterProbe{recordingProcessor: recordingProcessor{params: registry}}
	component := newComponent(probe)

	// Even a write equal to the current value can initialize legacy DSP state.
	for _, value := range []float64{0.25, 0.75, math.NaN(), math.Inf(1)} {
		if err := component.SetParamNormalized(7, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := component.SetParamNormalized(8, 0.5); err == nil {
		t.Fatal("accepted unknown controller parameter")
	}

	editor := EditorParameters{wrapper: &componentWrapper{component: component}}
	if err := editor.PerformEdit(7, 2); err != nil {
		t.Fatal(err)
	}
	ctx := process.NewContext(64, registry)
	component.applyParameterChanges(ctx, []process.ParameterChange{{ParamID: 7, Value: 0.125}})

	expected := []controllerParameterWrite{{7, .25, .25}, {7, .75, .75}, {7, 1, 1}}
	if !slices.Equal(probe.writes, expected) {
		t.Fatalf("controller writes = %+v, want %+v", probe.writes, expected)
	}
	if got := registry.Get(7).GetNormalized(); got != .125 {
		t.Fatalf("audio queue wrote %g, want .125", got)
	}
}
