// Command midifixture is built as a VST3 shared library for the C MIDI smoke test.
package main

import (
	"math"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	fwplugin "github.com/cwbudde/vst3go/pkg/framework/plugin"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/midi"
	"github.com/cwbudde/vst3go/pkg/plugin"
	_ "github.com/cwbudde/vst3go/pkg/plugin/cbridge"
)

const (
	gainID = iota
	bypassID
)

const (
	defaultGain            = 0.25
	bypassThreshold        = 0.5
	invalidMusicalPosition = -2
)

type processor struct {
	*fwplugin.BaseProcessor
	gain, bypass *param.Parameter
	// The C host uses eight frames. Store one gate change per frame so the
	// test can observe exact note timing without allocating in ProcessAudio.
	notes          [8]midi.EventType
	hasNote        [8]bool
	frames         int
	level          float32
	unexpectedNote bool
}

func (p *processor) ProcessEvent(event midi.Event) {
	if event.Offset < 0 || int(event.Offset) >= p.frames || event.Channel != 3 || event.Note() != 60 ||
		(event.Type == midi.EventTypeNoteOn && event.Velocity() != 64) ||
		(event.Type == midi.EventTypeNoteOff && event.Velocity() != 0) {
		p.unexpectedNote = true
		return
	}
	p.notes[event.Offset] = event.Type
	p.hasNote[event.Offset] = true
}

func (p *processor) ProcessAudio(ctx *process.Context) {
	p.frames = ctx.NumSamples()
	clear(p.hasNote[:])
	p.unexpectedNote = false
	ctx.ProcessEvents(p, 0, math.MaxInt32)
	transport := ctx.Transport

	for i := range p.frames {
		if p.hasNote[i] {
			p.level = 0
			if p.notes[i] == midi.EventTypeNoteOn {
				p.level = 1
				ctx.AddOutputEvent(midi.NewNoteOn(3, 60, 64, int32(i)))
			} else {
				ctx.AddOutputEvent(midi.NewNoteOff(3, 60, 0, int32(i)))
			}
		}
		if len(ctx.Output) == 0 {
			continue
		}
		ctx.Output[0][i] = ctx.Input[0][i]
		if p.bypass.GetNormalized() < bypassThreshold {
			ctx.Output[0][i] = ctx.Input[0][i]*float32(p.gain.GetNormalized()) + p.level
		}

		// The second output channel encodes the transport fields for the host
		// assertion. Calculate the distance from each callback's start.
		project, continuous := transport.ProjectTimeSamples, transport.ContinuousTimeSamples
		if transport.IsPlaying {
			project += int64(i)
			if transport.HasContinuousTime {
				continuous += int64(i)
			}
		}
		ctx.Output[1][i] = float32(project + 16*continuous)
		if transport.IsPlaying {
			ctx.Output[1][i] += 4096
		}
		if transport.HasTempo && transport.Tempo == 120 {
			ctx.Output[1][i] += 2048
		}
		if transport.HasMusicalTime {
			ctx.Output[1][i] += 8192
			musicalSample := int64(math.Round(transport.ProjectTimeMusic * transport.GetSamplesPerBeat(ctx.SampleRate)))
			if project != musicalSample+int64(i) {
				ctx.Output[1][i] = invalidMusicalPosition
			}
		}
		if p.unexpectedNote {
			ctx.Output[1][i] = -1
		}
	}
}

type fixture struct{}

func (fixture) GetInfo() fwplugin.Info {
	return fwplugin.Info{ID: "com.vst3go.midifixture", Name: "MIDI Host Fixture", Version: "1.0.0", Vendor: "vst3go", Category: "Fx"}
}

func (fixture) CreateProcessor() plugin.Processor {
	buses := bus.NewBuilder().WithStereoInput("In").WithStereoOutput("Out").
		WithEventInput("Notes").WithEventOutput("Echo").MustBuild()
	base := fwplugin.NewBaseProcessor(buses)
	gain := param.New(gainID, "Gain").Range(0, 1).Default(defaultGain).Build()
	bypass := param.BypassParameter(bypassID, "Bypass").Build()
	for _, p := range []*param.Parameter{gain, bypass} {
		if err := base.GetParameters().Add(p); err != nil {
			panic(err)
		}
	}
	return &processor{BaseProcessor: base, gain: gain, bypass: bypass}
}

//nolint:gochecknoinits // The C host loads the shared library without calling main.
func init() { plugin.Register(fixture{}) }
func main() {}
