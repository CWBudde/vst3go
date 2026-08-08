package process

import (
	"sync"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
)

// newSidechainMultiBus builds a main + sidechain input layout with a single
// main output, mirroring what a compressor sees.
func newSidechainMultiBus(t *testing.T, numSamples int) (*MultiBusContext, *bus.Configuration) {
	t.Helper()

	config := bus.EffectStereoSidechain()
	mb := NewMultiBusContext(NewContext(numSamples, param.NewRegistry()), config)

	mainIn := config.GetBusInfo(bus.MediaTypeAudio, bus.DirectionInput, 0)
	sideIn := config.GetBusInfo(bus.MediaTypeAudio, bus.DirectionInput, 1)
	mainOut := config.GetBusInfo(bus.MediaTypeAudio, bus.DirectionOutput, 0)
	if mainIn == nil || sideIn == nil || mainOut == nil {
		t.Fatal("expected main in/out and sidechain buses")
	}

	channels := func(n int) [][]float32 {
		out := make([][]float32, n)
		for i := range out {
			out[i] = make([]float32, numSamples)
		}
		return out
	}

	mb.InputBuses = []BusBuffers{
		{Channels: channels(2), BusInfo: mainIn},
		{Channels: channels(2), BusInfo: sideIn},
	}
	mb.OutputBuses = []BusBuffers{
		{Channels: channels(2), BusInfo: mainOut},
	}

	return mb, config
}

func TestMultiBusAccessors(t *testing.T) {
	mb, _ := newSidechainMultiBus(t, 16)

	if got := mb.NumInputBuses(); got != 2 {
		t.Errorf("NumInputBuses = %d, want 2", got)
	}
	if got := mb.NumOutputBuses(); got != 1 {
		t.Errorf("NumOutputBuses = %d, want 1", got)
	}
	if mb.GetMainInput() == nil || mb.GetMainOutput() == nil {
		t.Error("expected main input and output to resolve")
	}
	if mb.GetSidechainInput() == nil {
		t.Error("expected sidechain input to resolve")
	}
	if mb.GetInputBus(0) == nil || mb.GetOutputBus(0) == nil {
		t.Error("expected bus 0 to resolve")
	}
	if mb.GetInputBus(-1) != nil || mb.GetInputBus(9) != nil {
		t.Error("expected out-of-range input bus to be nil")
	}
	if mb.GetOutputBus(-1) != nil || mb.GetOutputBus(9) != nil {
		t.Error("expected out-of-range output bus to be nil")
	}
	if mb.GetInputBusInfo(0) == nil || mb.GetOutputBusInfo(0) == nil {
		t.Error("expected bus info to resolve")
	}
	if mb.GetInputBusInfo(9) != nil || mb.GetOutputBusInfo(9) != nil {
		t.Error("expected out-of-range bus info to be nil")
	}
}

func TestMultiBusProcessBusesSkipsInactive(t *testing.T) {
	mb, config := newSidechainMultiBus(t, 16)

	// The sidechain starts inactive, so only the main input is visited.
	var visited []int
	mb.ProcessInputBuses(func(busIndex int, _ [][]float32, _ *bus.Info) {
		visited = append(visited, busIndex)
	})
	if len(visited) != 1 || visited[0] != 0 {
		t.Errorf("visited input buses = %v, want [0]", visited)
	}

	if err := config.SetBusActive(bus.MediaTypeAudio, bus.DirectionInput, 1, true); err != nil {
		t.Fatalf("SetBusActive failed: %v", err)
	}

	visited = nil
	mb.ProcessInputBuses(func(busIndex int, _ [][]float32, _ *bus.Info) {
		visited = append(visited, busIndex)
	})
	if len(visited) != 2 {
		t.Errorf("visited input buses = %v, want both after activation", visited)
	}

	visited = nil
	mb.ProcessOutputBuses(func(busIndex int, _ [][]float32, _ *bus.Info) {
		visited = append(visited, busIndex)
	})
	if len(visited) != 1 {
		t.Errorf("visited output buses = %v, want [0]", visited)
	}
}

func TestMultiBusProcessMainAndSidechain(t *testing.T) {
	mb, _ := newSidechainMultiBus(t, 4)
	mb.InputBuses[0].Channels[0][0] = 0.5

	called := false
	mb.ProcessMainBuses(func(input, output [][]float32) {
		called = true
		copy(output[0], input[0])
	})
	if !called || mb.OutputBuses[0].Channels[0][0] != 0.5 {
		t.Error("ProcessMainBuses did not run over the main buses")
	}

	var sawSidechain bool
	mb.ProcessWithSidechain(func(_, sidechain, _ [][]float32) {
		sawSidechain = sidechain != nil
	})
	if !sawSidechain {
		t.Error("expected the sidechain buffers to be passed through")
	}

	// Without a main output there is nothing to process.
	mb.OutputBuses = nil
	mb.ProcessMainBuses(func(_, _ [][]float32) {
		t.Error("ProcessMainBuses ran without a main output bus")
	})
	mb.ProcessWithSidechain(func(_, _, _ [][]float32) {
		t.Error("ProcessWithSidechain ran without a main output bus")
	})
}

func TestMultiBusPassThroughAllAndClear(t *testing.T) {
	mb, _ := newSidechainMultiBus(t, 4)
	for ch := range mb.InputBuses[0].Channels {
		for i := range mb.InputBuses[0].Channels[ch] {
			mb.InputBuses[0].Channels[ch][i] = float32(i + 1)
		}
	}

	// Only one output bus exists, so only the first input bus is copied.
	mb.PassThroughAll()
	if mb.OutputBuses[0].Channels[1][3] != 4 {
		t.Errorf("PassThroughAll did not copy: %v", mb.OutputBuses[0].Channels[1])
	}

	mb.ClearAllOutputs()
	for ch := range mb.OutputBuses[0].Channels {
		for _, v := range mb.OutputBuses[0].Channels[ch] {
			if v != 0 {
				t.Fatalf("ClearAllOutputs left non-zero data: %v", mb.OutputBuses[0].Channels[ch])
			}
		}
	}
}

// The host activates buses from its UI thread while the audio thread walks them
// every block. Run with -race.
func TestProcessBusesRaceWithSetBusActive(t *testing.T) {
	mb, config := newSidechainMultiBus(t, 32)

	const iterations = 2000
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // audio thread
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				mb.ProcessInputBuses(func(_ int, _ [][]float32, _ *bus.Info) {})
				mb.ProcessOutputBuses(func(_ int, _ [][]float32, _ *bus.Info) {})
			}
		}
	}()

	wg.Add(1)
	go func() { // UI thread
		defer wg.Done()
		defer close(stop)
		for i := 0; i < iterations; i++ {
			if err := config.SetBusActive(bus.MediaTypeAudio, bus.DirectionInput, 1, i%2 == 0); err != nil {
				t.Errorf("SetBusActive failed: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
