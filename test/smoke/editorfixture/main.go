// Command editorfixture is a VST3 module for the C editor lifecycle smoke test.
package main

import (
	"errors"
	"io"
	"time"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	fwplugin "github.com/cwbudde/vst3go/pkg/framework/plugin"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/plugin"
	_ "github.com/cwbudde/vst3go/pkg/plugin/cbridge"
)

type processor struct {
	parameters *param.Registry
	buses      *bus.Configuration
}

func (p *processor) Initialize(float64, int32) error   { return nil }
func (p *processor) ProcessAudio(ctx *process.Context) { ctx.PassThrough() }
func (p *processor) GetParameters() *param.Registry    { return p.parameters }
func (p *processor) GetBuses() *bus.Configuration      { return p.buses }
func (p *processor) SetActive(bool) error              { return nil }
func (p *processor) GetLatencySamples() int32          { return 0 }
func (p *processor) GetTailSamples() int32             { return 0 }

type editorProcessor struct {
	*processor
	notifyLatencyChanged func()
	compact              bool
}

func (p *editorProcessor) CreateEditor(context plugin.EditorContext) plugin.EditorView {
	return &view{context: context, processor: p}
}

func (p *editorProcessor) SetLatencyChangeNotifier(notify func()) {
	p.notifyLatencyChanged = notify
}

func (p *editorProcessor) SaveEditorState(w io.Writer) error {
	data := [1]byte{}
	if p.compact {
		data[0] = 1
	}
	_, err := w.Write(data[:])
	return err
}

func (p *editorProcessor) LoadEditorState(r io.Reader) error {
	var data [1]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return err
	}
	p.compact = data[0] != 0
	return nil
}

func (p *editorProcessor) GetLatencySamples() int32 {
	value, _ := p.parameters.GetNormalized(0)
	const latencyScale = 128
	return int32((1 - value) * latencyScale)
}

type programProcessor struct{ *processor }

func (*programProcessor) FactoryPrograms() plugin.FactoryProgramList {
	const listID, selectorID = 11, 9
	return plugin.FactoryProgramList{
		ID: listID, Name: "Factory Presets", ParameterID: selectorID,
		Names: []string{"Müller", "Default"}, DefaultIndex: 1,
	}
}

func (p *programProcessor) ApplyFactoryProgram(index int32) {
	const quietGain = 0.25
	values := [...]float64{quietGain, 1}
	p.parameters.Get(0).SetNormalized(values[index])
}

type fixturePlugin struct {
	name     string
	editor   bool
	programs bool
}

func (p fixturePlugin) GetInfo() fwplugin.Info {
	return fwplugin.Info{
		ID: "com.vst3go.editorfixture." + p.name, Name: p.name,
		Version: "1.0.0", Vendor: "vst3go", Category: "Fx",
	}
}

func (p fixturePlugin) CreateProcessor() plugin.Processor {
	parameters := param.NewRegistry()
	if err := parameters.Add(param.New(0, "Gain").Range(0, 1).Default(1).Build()); err != nil {
		panic(err)
	}
	base := &processor{parameters: parameters, buses: bus.Stereo()}
	if p.editor {
		return &editorProcessor{processor: base}
	}
	if p.programs {
		return &programProcessor{processor: base}
	}
	return base
}

type view struct {
	context   plugin.EditorContext
	processor *editorProcessor
	focused   bool
	wheel     float32
	keyDown   bool
	ticked    bool
}

func (v *view) SupportsPlatform(platform plugin.EditorPlatform) bool {
	return platform == plugin.EditorX11EmbedWindowID
}

func (v *view) Attach(platform plugin.EditorPlatform, parent uintptr) error {
	if platform != plugin.EditorX11EmbedWindowID || parent != 123 {
		return errors.New("unexpected native parent")
	}
	if err := v.context.RequestResize(plugin.EditorSize{Width: 320, Height: 240}); err != nil {
		return err
	}
	const tickInterval = 30 * time.Millisecond
	_, err := v.context.StartTimer(tickInterval, func() { v.ticked = true })
	return err
}

func (v *view) Remove() {
	v.focused, v.wheel, v.keyDown = false, 0, false
	v.ticked = false
}

func (v *view) InitialSize() plugin.EditorSize {
	return plugin.EditorSize{Width: 300, Height: 200}
}

func (v *view) Resize(plugin.EditorSize) error { return nil }

func (v *view) ConstrainSize(size plugin.EditorSize) plugin.EditorSize {
	return plugin.EditorSize{Width: max(100, size.Width), Height: max(100, size.Height)}
}

func (v *view) OnFocus(focused bool) { v.focused = focused }

func (v *view) OnWheel(distance float32) bool {
	v.wheel = distance
	return distance == 2.5
}

func (v *view) OnKey(key plugin.EditorKey, down bool) bool {
	if key.Character == 'K' && down {
		return v.ticked
	}
	if key.Character == 'T' && down {
		v.processor.compact = !v.processor.compact
		return v.context.MarkDirty() == nil
	}
	if key.Character == 'C' && down {
		return v.processor.compact
	}
	if key.Character == 'G' && down {
		p := v.context.Parameters
		const first, last = 0.5, 0.75
		if p.BeginEdit(0) != nil || p.PerformEdit(0, first) != nil || p.PerformEdit(0, last) != nil || p.EndEdit(0) != nil {
			return false
		}
		v.processor.notifyLatencyChanged()
		return true
	}
	value, ok := v.context.Parameters.Normalized(0)
	if !ok || value != 0.25 || !v.focused || v.wheel != 2.5 || key != (plugin.EditorKey{Character: 'A', Code: 7, Modifiers: 2}) {
		return false
	}
	if down {
		v.keyDown = true
		return true
	}
	wasDown := v.keyDown
	v.keyDown = false
	return wasDown
}

func (v *view) SetScaleFactor(factor float32) error {
	return v.context.RequestResize(plugin.EditorSize{
		Width: int32(320 * factor), Height: int32(240 * factor),
	})
}

func init() {
	if err := plugin.RegisterClasses(fixturePlugin{name: "Native Editor", editor: true}, fixturePlugin{name: "Generic Editor"},
		fixturePlugin{name: "Factory Programs", programs: true}); err != nil {
		panic(err)
	}
}

func main() {}
