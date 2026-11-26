package plugin

import (
	"runtime"
	"slices"
	"testing"
	"unsafe"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

const (
	testMonoArrangement   = 1 << 19
	testStereoArrangement = 0x03
	testQuadArrangement   = 0x33
	test51Arrangement     = 0x3f
)

func TestComponentBusArrangements(t *testing.T) {
	tests := []struct {
		name    string
		buses   *bus.Configuration
		inputs  []int64
		outputs []int64
	}{
		{"mono", bus.Mono(), []int64{testMonoArrangement}, []int64{testMonoArrangement}},
		{"stereo", bus.Stereo(), []int64{testStereoArrangement}, []int64{testStereoArrangement}},
		{"quad", bus.NewBuilder().WithQuadInput("In").WithQuadOutput("Out").MustBuild(), []int64{testQuadArrangement}, []int64{testQuadArrangement}},
		{"5.1", bus.NewSurround5_1Effect(), []int64{test51Arrangement}, []int64{test51Arrangement}},
		{"sidechain", bus.EffectStereoSidechain(), []int64{testStereoArrangement, testStereoArrangement}, []int64{testStereoArrangement}},
		{"generator", bus.Generator(), nil, []int64{testStereoArrangement}},
		{"MIDI effect", bus.MIDIEffect(), nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newComponent(&nullProcessor{recordingProcessor{params: param.NewRegistry(), buses: tt.buses}})
			if err := c.Initialize(nil); err != nil {
				t.Fatal(err)
			}
			if err := c.SetBusArrangements(tt.inputs, tt.outputs); err != nil {
				t.Fatalf("declared arrangement rejected: %v", err)
			}
			for dir, arrangements := range [][]int64{tt.inputs, tt.outputs} {
				for i, want := range arrangements {
					got, err := c.GetBusArrangement(int32(dir), int32(i))
					if err != nil || got != want {
						t.Errorf("GetBusArrangement(%d, %d) = %#x, %v, want %#x", dir, i, got, err, want)
					}
				}
				if _, err := c.GetBusArrangement(int32(dir), int32(len(arrangements))); err == nil {
					t.Errorf("GetBusArrangement returned an arrangement for missing %d bus", dir)
				}
			}
		})
	}
}

func TestComponentRejectsUnsupportedBusArrangements(t *testing.T) {
	c := newComponent(&nullProcessor{recordingProcessor{params: param.NewRegistry(), buses: bus.EffectStereoSidechain()}})
	if err := c.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	if err := c.SetBusArrangements([]int64{testStereoArrangement}, []int64{testStereoArrangement}); err == nil {
		t.Error("accepted a request with the wrong number of buses")
	}
	if err := c.SetBusArrangements([]int64{testStereoArrangement, testStereoArrangement}, []int64{test51Arrangement}); err == nil {
		t.Error("accepted a 5.1 arrangement for a stereo output")
	}
	if got, err := c.GetBusArrangement(int32(bus.DirectionOutput), 0); err != nil || got != testStereoArrangement {
		t.Errorf("GetBusArrangement after rejection = %#x, %v, want stereo", got, err)
	}
}

func TestComponentActivatesSidechain(t *testing.T) {
	buses := bus.EffectStereoSidechain()
	c := newComponent(&nullProcessor{recordingProcessor{params: param.NewRegistry(), buses: buses}})
	if err := c.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	const media, direction = int32(bus.MediaTypeAudio), int32(bus.DirectionInput)
	for _, active := range []bool{true, false, true} {
		if err := c.ActivateBus(media, direction, 1, active); err != nil {
			t.Fatal(err)
		}
		if got := buses.GetSidechainBus().Active(); got != active {
			t.Errorf("sidechain active = %v, want %v", got, active)
		}
		info, err := c.GetBusInfo(media, direction, 1)
		if err != nil || info.Flags != 0 {
			t.Errorf("sidechain default activation flags changed: %+v, %v", info, err)
		}
	}
	if err := c.ActivateBus(media, direction, 2, true); err == nil {
		t.Error("activated a missing bus")
	}
}

// Type inference lets the Go tests use the native structs without importing C.
func nativeProcessData[T any](_ func(*process.Context, *T)) *T { return new(T) }
func nativeBuses[T any](_ *T, count int) []T                   { return make([]T, count) }
func setNativeCount[T ~int32](dst *T, count int)               { *dst = T(count) }

func hostAudioBlock(t *testing.T, c *componentImpl, numSamples int, input, output *synthBuses) unsafe.Pointer {
	t.Helper()
	data := nativeProcessData(c.mapAudioBuffers)
	setNativeCount(&data.numSamples, numSamples)

	// Channel pointer tables are read by C. Pin their storage and the sample
	// buffers so the test fixture follows the same lifetime rules as a host.
	var pinned runtime.Pinner
	t.Cleanup(pinned.Unpin)
	setChannelTable := func(dst unsafe.Pointer, buses *synthBuses, i int) {
		if len(buses.tables[i]) == 0 {
			return
		}
		for _, buffer := range buses.tables[i] {
			if buffer != nil {
				pinned.Pin(buffer)
			}
		}
		table := buses.tablePtr(i)
		pinned.Pin(table)
		*(*unsafe.Pointer)(dst) = unsafe.Pointer(table)
	}

	inputs := nativeBuses(data.inputs, len(input.tables))
	for i := range inputs {
		setNativeCount(&inputs[i].numChannels, len(input.tables[i]))
		setChannelTable(unsafe.Pointer(&inputs[i].anon0), input, i)
	}
	outputs := nativeBuses(data.outputs, len(output.tables))
	for i := range outputs {
		setNativeCount(&outputs[i].numChannels, len(output.tables[i]))
		setChannelTable(unsafe.Pointer(&outputs[i].anon0), output, i)
	}
	data.inputs, data.outputs = unsafe.SliceData(inputs), unsafe.SliceData(outputs)
	setNativeCount(&data.numInputs, len(inputs))
	setNativeCount(&data.numOutputs, len(outputs))
	return unsafe.Pointer(data)
}

type busTestProcessor struct {
	nullProcessor
	processBlock func(*process.Context)
}

func (p *busTestProcessor) ProcessAudio(ctx *process.Context) { p.processBlock(ctx) }

func newBusTestComponent(t *testing.T, buses *bus.Configuration, numSamples int32, processBlock func(*process.Context)) *componentImpl {
	t.Helper()
	c := newComponent(&busTestProcessor{
		nullProcessor: nullProcessor{recordingProcessor{params: param.NewRegistry(), buses: buses}},
		processBlock:  processBlock,
	})
	if err := c.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	if err := c.SetupProcessing(&vst3.ProcessSetup{SampleRate: 48000, MaxSamplesPerBlock: numSamples}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetProcessing(true); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestComponentInactiveAudioBuffers(t *testing.T) {
	const numSamples = 8
	buses := bus.NewBuilder().WithStereoInput("In").WithSidechain("Sidechain").WithStereoOutput("Out").WithAuxOutput("Aux", 2).MustBuild()
	input, output := newSynthBuses(numSamples, 2, 2), newSynthBuses(numSamples, 2, 2)
	var expectedChannels int
	c := newBusTestComponent(t, buses, numSamples, func(ctx *process.Context) {
		if len(ctx.Input) != expectedChannels || len(ctx.Output) != expectedChannels {
			t.Errorf("flat input/output channels = %d/%d, want %d/%d", len(ctx.Input), len(ctx.Output), expectedChannels, expectedChannels)
		}
		if got, want := len(ctx.Buses.GetSidechainInput()), expectedChannels-2; got != want {
			t.Errorf("sidechain channels = %d, want %d", got, want)
		}
		ctx.Clear()
	})
	fullBlock := hostAudioBlock(t, c, numSamples, input, output)
	shortBlock := hostAudioBlock(t, c, numSamples, newSynthBuses(numSamples, 2), newSynthBuses(numSamples, 2))
	const audio = int32(bus.MediaTypeAudio)
	for _, tt := range []struct {
		name   string
		active bool
		block  unsafe.Pointer
	}{
		{"initially inactive with buffers", false, fullBlock},
		{"connected", true, fullBlock},
		{"disconnected with omitted buffers", false, shortBlock},
		{"disconnected with buffers", false, fullBlock},
		{"reconnected", true, fullBlock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expectedChannels = 2
			if tt.active {
				expectedChannels = 4
			}
			for _, dir := range []bus.Direction{bus.DirectionInput, bus.DirectionOutput} {
				if err := c.ActivateBus(audio, int32(dir), 1, tt.active); err != nil {
					t.Fatal(err)
				}
			}
			output.storage[1][0][0] = 19
			if err := c.Process(tt.block); err != nil {
				t.Fatal(err)
			}
			var want float32 = 19
			if tt.active {
				want = 0
			}
			if got := output.storage[1][0][0]; got != want {
				t.Errorf("auxiliary output sample = %v, want %v", got, want)
			}
		})
	}
}

func TestComponentRoutesChannelIsolatedImpulses(t *testing.T) {
	tests := []struct {
		name        string
		buses       *bus.Configuration
		channels    int
		arrangement int64
	}{
		{"mono", bus.Mono(), 1, testMonoArrangement},
		{"stereo", bus.Stereo(), 2, testStereoArrangement},
		{"quad", bus.NewBuilder().WithQuadInput("In").WithQuadOutput("Out").MustBuild(), 4, testQuadArrangement},
		{"5.1", bus.NewSurround5_1Effect(), 6, test51Arrangement},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numSamples := tt.channels + 2
			c := newBusTestComponent(t, tt.buses, int32(numSamples), func(ctx *process.Context) {
				ctx.Buses.PassThroughAll()
			})
			if err := c.SetBusArrangements([]int64{tt.arrangement}, []int64{tt.arrangement}); err != nil {
				t.Fatal(err)
			}
			input, output := newSynthBuses(numSamples, tt.channels), newSynthBuses(numSamples, tt.channels)
			for ch, samples := range input.storage[0] {
				clear(samples)
				samples[ch+1] = float32(ch + 1)
			}
			block := hostAudioBlock(t, c, numSamples, input, output)
			if err := c.Process(block); err != nil {
				t.Fatal(err)
			}
			for ch, samples := range output.storage[0] {
				if want := input.storage[0][ch]; !slices.Equal(samples, want) {
					t.Errorf("output channel %d = %v, want %v", ch, samples, want)
				}
			}
		})
	}
}

func TestComponentKeepsBusIndicesInAutomationChunks(t *testing.T) {
	const numSamples, split = 8, 3
	buses := bus.NewBuilder().
		WithStereoInput("Main In").WithSidechain("First Sidechain").WithSidechain("Second Sidechain").
		WithStereoOutput("Main Out").WithAuxOutput("First Aux", 2).WithAuxOutput("Second Aux", 2).MustBuild()
	input, output := newSynthBuses(numSamples, 2, 2, 2, 2), newSynthBuses(numSamples, 2, 2, 2, 2)
	for i, channels := range input.storage {
		for ch, samples := range channels {
			clear(samples)
			samples[1+ch] = float32(i + 1)
			samples[split+1+ch] = float32(i + 4)
		}
	}

	var chunks []int
	c := newBusTestComponent(t, buses, numSamples, func(ctx *process.Context) {
		if ctx.Buses.GetInputBus(1) != nil || ctx.Buses.GetOutputBus(1) != nil {
			t.Error("inactive buses were exposed to DSP")
		}
		if ctx.Buses.GetInputBus(3) != nil || ctx.Buses.GetOutputBus(3) != nil {
			t.Error("undeclared buses were exposed to DSP")
		}
		for _, i := range []int{0, 2} {
			in, out := ctx.Buses.GetInputBus(i), ctx.Buses.GetOutputBus(i)
			if len(in) != 2 || len(out) != 2 {
				t.Fatalf("bus %d input/output channel counts = %d/%d, want 2/2", i, len(in), len(out))
			}
			if len(in[0]) != ctx.NumSamples() || len(out[0]) != ctx.NumSamples() {
				t.Fatalf("bus %d input/output lengths = %d/%d, want %d", i, len(in[0]), len(out[0]), ctx.NumSamples())
			}
		}
		chunks = append(chunks, ctx.NumSamples())
		ctx.Buses.PassThroughAll()
	})
	for _, direction := range []bus.Direction{bus.DirectionInput, bus.DirectionOutput} {
		if err := c.ActivateBus(int32(bus.MediaTypeAudio), int32(direction), 2, true); err != nil {
			t.Fatal(err)
		}
	}
	block := hostAudioBlock(t, c, numSamples, input, output)
	if err := c.Process(block); err != nil {
		t.Fatal(err)
	}
	// Split the mapped block at a parameter update. Both indexed and flat
	// channel views must now refer to the same chunk-relative window.
	for _, i := range []int{0, 2} {
		for _, samples := range output.storage[i] {
			clear(samples)
		}
	}
	ctx := c.processCtx.Load()
	ctx.AddParameterChange(1, 0.5, split)
	c.processSampleAccurate(ctx, nil)
	if want := []int{numSamples, split, numSamples - split}; !slices.Equal(chunks, want) {
		t.Errorf("DSP block lengths = %v, want %v", chunks, want)
	}
	for i, channels := range output.storage {
		for ch, samples := range channels {
			if i == 0 || i == 2 {
				if want := input.storage[i][ch]; !slices.Equal(samples, want) {
					t.Errorf("output bus %d channel %d = %v, want %v", i, ch, samples, want)
				}
			} else {
				for _, sample := range samples {
					if want := float32(2*i + ch + 1); sample != want {
						t.Errorf("output bus %d channel %d was overwritten: %v, want %v", i, ch, samples, want)
						break
					}
				}
			}
		}
	}
}
