package process

import (
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

func TestProcessSamplesDoesNotAllocateAfterWarmup(t *testing.T) {
	ctx := NewContext(64, param.NewRegistry())
	ctx.Input = [][]float32{
		make([]float32, 64),
		make([]float32, 64),
	}
	ctx.Output = [][]float32{
		make([]float32, 64),
		make([]float32, 64),
	}

	// Warmup initializes scratch buffers once.
	ctx.ProcessSamples(func(sample int, inputs, outputs []float32) {
		outputs[0] = inputs[0]
		outputs[1] = inputs[1]
	})

	allocs := testing.AllocsPerRun(100, func() {
		ctx.ProcessSamples(func(sample int, inputs, outputs []float32) {
			outputs[0] = inputs[0]
			outputs[1] = inputs[1]
		})
	})

	if allocs != 0 {
		t.Fatalf("expected zero allocations after warmup, got %f", allocs)
	}
}

// newChannelContext builds a context with the given channel counts, each sample
// pre-filled with a channel-dependent value so copies are verifiable.
func newChannelContext(numIn, numOut, numSamples int) *Context {
	ctx := NewContext(numSamples, param.NewRegistry())

	ctx.Input = make([][]float32, numIn)
	for ch := range ctx.Input {
		ctx.Input[ch] = make([]float32, numSamples)
		for i := range ctx.Input[ch] {
			ctx.Input[ch][i] = float32(ch*100 + i)
		}
	}

	ctx.Output = make([][]float32, numOut)
	for ch := range ctx.Output {
		ctx.Output[ch] = make([]float32, numSamples)
	}

	return ctx
}

func TestChannelCountHelpers(t *testing.T) {
	tests := []struct {
		name          string
		numIn, numOut int
		wantChannels  int
		wantStereo    int
	}{
		{name: "mono", numIn: 1, numOut: 1, wantChannels: 1, wantStereo: 1},
		{name: "stereo", numIn: 2, numOut: 2, wantChannels: 2, wantStereo: 2},
		{name: "more inputs than outputs", numIn: 4, numOut: 2, wantChannels: 2, wantStereo: 2},
		{name: "surround caps at stereo", numIn: 6, numOut: 6, wantChannels: 6, wantStereo: 2},
		{name: "generator has no input", numIn: 0, numOut: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newChannelContext(tt.numIn, tt.numOut, 8)
			if got := ctx.GetNumChannels(); got != tt.wantChannels {
				t.Errorf("GetNumChannels = %d, want %d", got, tt.wantChannels)
			}
			if got := ctx.GetNumStereoChannels(); got != tt.wantStereo {
				t.Errorf("GetNumStereoChannels = %d, want %d", got, tt.wantStereo)
			}
		})
	}
}

func TestProcessChannelVariants(t *testing.T) {
	tests := []struct {
		name         string
		numIn        int
		numOut       int
		process      func(ctx *Context, seen *[]int)
		wantChannels []int
	}{
		{
			name: "ProcessChannels covers the narrower side",
			process: func(ctx *Context, seen *[]int) {
				ctx.ProcessChannels(func(ch int, input, output []float32) {
					*seen = append(*seen, ch)
					copy(output, input)
				})
			},
			numIn: 4, numOut: 2, wantChannels: []int{0, 1},
		},
		{
			name: "ProcessStereo caps at two channels",
			process: func(ctx *Context, seen *[]int) {
				ctx.ProcessStereo(func(ch int, input, output []float32) {
					*seen = append(*seen, ch)
					copy(output, input)
				})
			},
			numIn: 6, numOut: 6, wantChannels: []int{0, 1},
		},
		{
			name: "ProcessMono only touches channel zero",
			process: func(ctx *Context, seen *[]int) {
				ctx.ProcessMono(func(input, output []float32) {
					*seen = append(*seen, 0)
					copy(output, input)
				})
			},
			numIn: 2, numOut: 2, wantChannels: []int{0},
		},
		{
			name: "ProcessMono skips a missing input bus",
			process: func(ctx *Context, seen *[]int) {
				ctx.ProcessMono(func(_, _ []float32) {
					*seen = append(*seen, 0)
				})
			},
			numIn: 0, numOut: 2,
		},
		{
			name: "CopyInputToOutput copies every shared channel",
			process: func(ctx *Context, seen *[]int) {
				ctx.CopyInputToOutput()
				*seen = append(*seen, 0, 1)
			},
			numIn: 2, numOut: 2, wantChannels: []int{0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newChannelContext(tt.numIn, tt.numOut, 8)

			var seen []int
			tt.process(ctx, &seen)

			if len(seen) != len(tt.wantChannels) {
				t.Fatalf("processed channels = %v, want %v", seen, tt.wantChannels)
			}
			for i, ch := range tt.wantChannels {
				if seen[i] != ch {
					t.Fatalf("processed channels = %v, want %v", seen, tt.wantChannels)
				}
				if ctx.Output[ch][3] != ctx.Input[ch][3] {
					t.Errorf("channel %d was not copied: %v", ch, ctx.Output[ch])
				}
			}
		})
	}
}

func TestProcessChannelsSeparately(t *testing.T) {
	ctx := newChannelContext(3, 3, 4)

	// Fewer functions than channels: the extra channel is left untouched.
	ctx.ProcessChannelsSeparately(
		func(input, output []float32) { copy(output, input) },
		func(input, output []float32) {
			for i := range output {
				output[i] = -input[i]
			}
		},
	)

	if ctx.Output[0][2] != ctx.Input[0][2] {
		t.Errorf("channel 0 not copied: %v", ctx.Output[0])
	}
	if ctx.Output[1][2] != -ctx.Input[1][2] {
		t.Errorf("channel 1 not inverted: %v", ctx.Output[1])
	}
	if ctx.Output[2][2] != 0 {
		t.Errorf("channel 2 should be untouched, got %v", ctx.Output[2])
	}
}

// NewContext reserves scratch for maxScratchChannels; only a wider layout than
// that may allocate, and only once.
func TestEnsureSampleScratchGrowsBeyondReservation(t *testing.T) {
	ctx := NewContext(8, param.NewRegistry())

	if allocs := testing.AllocsPerRun(10, func() {
		ctx.ensureSampleScratch(maxScratchChannels)
	}); allocs != 0 {
		t.Errorf("expected the reserved scratch to cover %d channels, got %f allocations", maxScratchChannels, allocs)
	}

	ctx.ensureSampleScratch(maxScratchChannels + 8)
	if len(ctx.sampleIn) != maxScratchChannels+8 || len(ctx.sampleOut) != maxScratchChannels+8 {
		t.Fatalf("scratch not grown: in=%d out=%d", len(ctx.sampleIn), len(ctx.sampleOut))
	}

	if allocs := testing.AllocsPerRun(10, func() {
		ctx.ensureSampleScratch(maxScratchChannels + 8)
	}); allocs != 0 {
		t.Errorf("expected the grown scratch to be reused, got %f allocations", allocs)
	}
}
