package plugin

import (
	"slices"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
)

type programTestProcessor struct {
	*recordingProcessor
	gain    *param.Parameter
	applied int
}

func (*programTestProcessor) FactoryPrograms() FactoryProgramList {
	return FactoryProgramList{ID: 5, Name: "Programs", ParameterID: 9, Names: []string{"Original", "Quiet"}}
}

func (p *programTestProcessor) ApplyFactoryProgram(index int32) {
	p.applied++
	p.gain.SetNormalized([]float64{1, 0.25}[index])
}

func (p *programTestProcessor) ProcessAudio(ctx *process.Context) {
	for i := range ctx.Output[0] {
		ctx.Output[0][i] = float32(p.gain.GetNormalized())
	}
}

func TestFactoryProgramQueueUsesHostOffset(t *testing.T) {
	const programID, frames, switchFrame = 9, 4, 2
	registry := param.NewRegistry()
	gain := param.New(1, "Gain").Default(1).Build()
	if err := registry.Add(gain); err != nil {
		t.Fatal(err)
	}
	processor := &programTestProcessor{recordingProcessor: &recordingProcessor{params: registry}, gain: gain}
	component := newComponent(processor)
	programs, err := newFactoryPrograms(processor, registry)
	if err != nil {
		t.Fatal(err)
	}
	component.programs = programs
	if err := component.SetProcessing(true); err != nil {
		t.Fatal(err)
	}
	if err := component.SetParamNormalized(programID, 1); err != nil {
		t.Fatal(err)
	}
	if processor.applied != 0 {
		t.Fatal("controller applied a program before process() delivered the host's offset")
	}
	ctx := process.NewContext(frames, registry)
	output := make([]float32, frames)
	ctx.Output = [][]float32{output}
	ctx.AddParameterChange(1, 0.75, switchFrame)
	ctx.AddParameterChange(programID, 1, switchFrame)
	programs.applyPending(ctx.GetParameterChanges())
	component.processSampleAccurate(ctx)
	if want := []float32{1, 1, 0.75, 0.75}; !slices.Equal(output, want) {
		t.Fatalf("output = %v, want %v", output, want)
	}
	if processor.applied != 1 || component.GetParamNormalized(programID) != 1 {
		t.Fatalf("program selected %d times, selector value %v", processor.applied, component.GetParamNormalized(programID))
	}

	if err := component.SetParamNormalized(programID, 0); err != nil {
		t.Fatal(err)
	}
	programs.applyPending(ctx.GetParameterChanges())
	if processor.applied != 1 {
		t.Fatal("controller selection disrupted an unrelated host selection in the same block")
	}
	programs.applyPending(nil)
	if processor.applied != 2 || gain.GetNormalized() != 1 {
		t.Fatal("controller selection without a host queue did not apply on the next block")
	}
}
