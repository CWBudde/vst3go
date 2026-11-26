// Command plugin is a minimal VST3 plugin built only to prove that the runtime
// links into a loadable module and answers a host correctly.
//
// It is built with -buildmode=c-shared by the smoke test in the parent
// directory and driven from C by testdata/driver.c. Keep it deliberately
// boring: one parameter and one stereo bus pair per class, with a sidechain
// enabled in the C host fixture build.
package main

import (
	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	fwplugin "github.com/cwbudde/vst3go/pkg/framework/plugin"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/plugin"
	// Pulls in the C bridge and the exported GetPluginFactory/ModuleEntry symbols.
	_ "github.com/cwbudde/vst3go/pkg/plugin/cbridge"
)

type processor struct {
	params *param.Registry
	buses  *bus.Configuration
}

func (p *processor) Initialize(sampleRate float64, maxBlockSize int32) error { return nil }
func (p *processor) ProcessAudio(ctx *process.Context) {
	ctx.PassThrough()
	if runtimeSmoke == "" || ctx.Buses == nil {
		return
	}
	outputs := ctx.Buses.GetMainOutput()
	sidechain := ctx.Buses.GetSidechainInput()
	for ch := 0; ch < min(len(outputs), len(sidechain)); ch++ {
		for n := 0; n < min(len(outputs[ch]), len(sidechain[ch])); n++ {
			outputs[ch][n] += sidechain[ch][n]
		}
	}
}
func (p *processor) GetParameters() *param.Registry { return p.params }
func (p *processor) GetBuses() *bus.Configuration   { return p.buses }
func (p *processor) SetActive(active bool) error    { return nil }
func (p *processor) GetLatencySamples() int32       { return 0 }
func (p *processor) GetTailSamples() int32          { return 0 }

type smokePlugin struct {
	suffix       string
	defaultValue float64
}

func (p smokePlugin) GetInfo() fwplugin.Info {
	return fwplugin.Info{
		ID:       "com.vst3go.smoketest" + p.suffix,
		Name:     "VST3Go Smoke Test" + p.suffix,
		Version:  "1.0.0",
		Vendor:   "vst3go",
		Category: "Fx",
	}
}

func (p smokePlugin) CreateProcessor() plugin.Processor {
	registry := param.NewRegistry()
	_ = registry.Add(param.New(0, "Gain").Range(0, 1).Default(p.defaultValue).Build())
	buses := bus.Stereo()
	if runtimeSmoke != "" {
		buses = bus.EffectStereoSidechain()
	}
	return &processor{params: registry, buses: buses}
}

var (
	multiClass   string // Set by the multi-class builds.
	runtimeSmoke string // Set for the C host processing fixture.
)

func init() {
	first := smokePlugin{defaultValue: 1}
	if multiClass == "" {
		plugin.Register(first)
		return
	}
	second := smokePlugin{suffix: " Auxiliary", defaultValue: 0.25}
	if err := plugin.RegisterClasses(first, second); err != nil {
		panic(err)
	}
}

// Required for -buildmode=c-shared; never called.
func main() {}
