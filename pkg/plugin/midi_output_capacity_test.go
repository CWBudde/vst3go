package plugin

import (
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/midi"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

type outputCapacityProcessor struct{ nullProcessor }

func (*outputCapacityProcessor) OutputEventCapacity(maxBlockSize int32) int {
	return 2 * min(int(maxBlockSize), 1023)
}

func TestSetupProcessingOutputEventCapacity(t *testing.T) {
	base := &nullProcessor{recordingProcessor{params: param.NewRegistry(), buses: bus.MIDIEffect()}}
	ordinary := newComponent(base)
	dense := newComponent(&outputCapacityProcessor{*base})
	for _, tt := range []struct {
		name      string
		component *componentImpl
		blockSize int32
		capacity  int
	}{
		{"default", ordinary, 1024, 128},
		{"small", dense, 17, 128},
		{"dense", dense, 1024, 2046},
		{"resized", dense, 257, 514},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.component
			setup := vst3.ProcessSetup{SampleRate: 48000, MaxSamplesPerBlock: tt.blockSize}
			if err := c.SetupProcessing(&setup); err != nil {
				t.Fatal(err)
			}
			if got := cap(c.outputEvents); got != tt.capacity {
				t.Fatalf("output buffer capacity %d, want %d", got, tt.capacity)
			}

			contexts := []*process.Context{c.processCtx.Load()}
			if err := c.SetupProcessing(&setup); err != nil {
				t.Fatal(err)
			}
			contexts = append(contexts, c.processCtx.Load())

			// AllocsPerRun invokes the function once before measuring. Use a
			// fresh queue for the measurement so it tests the first block.
			run := 0
			allocations := testing.AllocsPerRun(1, func() {
				ctx := contexts[run]
				run++
				for range tt.capacity {
					ctx.AddOutputEvent(midi.Event{Type: midi.EventTypeNoteOn})
				}
				if got := len(ctx.AppendOutputEvents(c.outputEvents[:0])); got != tt.capacity {
					t.Fatalf("queued %d events, want %d", got, tt.capacity)
				}
				ctx.ClearOutputEvents()
			})
			if allocations != 0 {
				t.Errorf("first output block made %.0f allocations", allocations)
			}
		})
	}
}
