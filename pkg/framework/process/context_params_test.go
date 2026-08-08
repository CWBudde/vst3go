package process

import (
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

func newTestRegistry(t *testing.T, ids ...uint32) *param.Registry {
	t.Helper()

	registry := param.NewRegistry()
	for _, id := range ids {
		p := param.New(id, "p").Range(0, 1).Default(0).Build()
		if err := registry.Add(p); err != nil {
			t.Fatalf("registry.Add(%d) failed: %v", id, err)
		}
	}
	return registry
}

func TestParameterChangeCapacityScalesWithBlockSize(t *testing.T) {
	tests := []struct {
		name         string
		maxBlockSize int
		wantCapacity int
	}{
		{name: "negative clamps to floor", maxBlockSize: -1, wantCapacity: minParameterChanges},
		{name: "tiny block gets the floor", maxBlockSize: 32, wantCapacity: minParameterChanges},
		{name: "floor boundary", maxBlockSize: minParameterChanges, wantCapacity: minParameterChanges},
		{name: "large block scales", maxBlockSize: 8192, wantCapacity: 8192},
		{name: "absurd block hits the cap", maxBlockSize: 1 << 20, wantCapacity: maxParameterChanges},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(tt.maxBlockSize, param.NewRegistry())
			if got := len(ctx.paramChanges); got != tt.wantCapacity {
				t.Fatalf("automation buffer = %d, want %d", got, tt.wantCapacity)
			}
		})
	}
}

// A dense automation lane used to be truncated at 128 points with no signal at
// all, leaving the parameter stuck mid-block.
func TestAddParameterChangeKeepsLastValueOnOverflow(t *testing.T) {
	ctx := NewContext(0, newTestRegistry(t, 1))
	capacity := len(ctx.paramChanges)

	total := capacity + 50
	for i := 0; i < total; i++ {
		ctx.AddParameterChange(1, float64(i)/float64(total), i)
	}

	if got := len(ctx.GetParameterChanges()); got != capacity {
		t.Fatalf("stored changes = %d, want %d", got, capacity)
	}
	if got := ctx.CoalescedParameterChanges(); got != 50 {
		t.Errorf("coalesced = %d, want 50", got)
	}
	if got := ctx.DroppedParameterChanges(); got != 0 {
		t.Errorf("dropped = %d, want 0", got)
	}

	ctx.SortParameterChanges()
	changes := ctx.GetParameterChanges()
	last := changes[len(changes)-1]

	wantValue := float64(total-1) / float64(total)
	if last.Value != wantValue {
		t.Errorf("last change value = %v, want %v (the block must end on the host's final value)", last.Value, wantValue)
	}
	if last.SampleOffset != total-1 {
		t.Errorf("last change offset = %d, want %d", last.SampleOffset, total-1)
	}
}

// A parameter that never got an entry cannot be folded into one; it is counted
// so the loss is at least visible.
func TestAddParameterChangeCountsUnfoldableDrops(t *testing.T) {
	ctx := NewContext(0, newTestRegistry(t, 1, 2))
	capacity := len(ctx.paramChanges)

	for i := 0; i < capacity; i++ {
		ctx.AddParameterChange(1, 0.5, i)
	}

	ctx.AddParameterChange(2, 0.25, capacity)

	if got := ctx.DroppedParameterChanges(); got != 1 {
		t.Errorf("dropped = %d, want 1", got)
	}
	if got := ctx.CoalescedParameterChanges(); got != 0 {
		t.Errorf("coalesced = %d, want 0", got)
	}
}

func TestResetParameterChangesClearsCounters(t *testing.T) {
	ctx := NewContext(0, newTestRegistry(t, 1))
	for i := 0; i < len(ctx.paramChanges)+3; i++ {
		ctx.AddParameterChange(1, 0.1, i)
	}

	ctx.ResetParameterChanges()

	if ctx.HasParameterChanges() {
		t.Error("expected no parameter changes after reset")
	}
	if ctx.CoalescedParameterChanges() != 0 || ctx.DroppedParameterChanges() != 0 {
		t.Error("expected overflow counters to reset")
	}
}

func TestSortParameterChangesOrdersByOffset(t *testing.T) {
	ctx := NewContext(256, newTestRegistry(t, 1))

	offsets := []int{200, 0, 128, 17, 128}
	for i, offset := range offsets {
		ctx.AddParameterChange(1, float64(i), offset)
	}

	ctx.SortParameterChanges()

	changes := ctx.GetParameterChanges()
	for i := 1; i < len(changes); i++ {
		if changes[i-1].SampleOffset > changes[i].SampleOffset {
			t.Fatalf("changes not sorted: %v", changes)
		}
	}

	// Stable: the two entries at offset 128 keep their insertion order, so the
	// later change still wins when applied in sequence.
	if changes[2].Value != 2 || changes[3].Value != 4 {
		t.Errorf("expected stable order at equal offsets, got %v and %v", changes[2].Value, changes[3].Value)
	}
}

func TestSortParameterChangesHandlesSmallCounts(t *testing.T) {
	ctx := NewContext(64, newTestRegistry(t, 1))

	ctx.SortParameterChanges() // empty
	if ctx.HasParameterChanges() {
		t.Error("expected no changes")
	}

	ctx.AddParameterChange(1, 0.5, 10)
	ctx.SortParameterChanges() // single entry
	if got := ctx.GetParameterChanges(); len(got) != 1 || got[0].SampleOffset != 10 {
		t.Errorf("unexpected changes: %v", got)
	}
}

func TestApplyParameterChange(t *testing.T) {
	registry := newTestRegistry(t, 7)
	ctx := NewContext(64, registry)

	ctx.ApplyParameterChange(ParameterChange{ParamID: 7, Value: 0.75, SampleOffset: 0})
	if got := ctx.Param(7); got != 0.75 {
		t.Errorf("Param(7) = %v, want 0.75", got)
	}

	// Unknown parameter and nil registry must be no-ops, not panics.
	ctx.ApplyParameterChange(ParameterChange{ParamID: 999, Value: 1})
	(&Context{}).ApplyParameterChange(ParameterChange{ParamID: 1, Value: 1})
}

func TestSetParameterAtOffsetAppliesImmediately(t *testing.T) {
	registry := newTestRegistry(t, 3)
	ctx := NewContext(64, registry)

	ctx.SetParameterAtOffset(3, 0.5, 12)
	if got := ctx.Param(3); got != 0.5 {
		t.Errorf("Param(3) = %v, want 0.5", got)
	}

	// An unknown id must be a no-op, as must a context without a registry.
	ctx.SetParameterAtOffset(999, 0.5, 0)
	(&Context{}).SetParameterAtOffset(3, 0.5, 0)
}

// The guard that keeps the whole audio path honest: a realistic block must not
// allocate anywhere in the Context API.
func TestContextAudioPathDoesNotAllocate(t *testing.T) {
	registry := newTestRegistry(t, 1, 2)
	ctx := NewContext(512, registry)
	ctx.Input = [][]float32{make([]float32, 512), make([]float32, 512)}
	ctx.Output = [][]float32{make([]float32, 512), make([]float32, 512)}

	block := func() {
		ctx.ResetParameterChanges()
		ctx.AddParameterChange(1, 0.25, 300)
		ctx.AddParameterChange(2, 0.5, 10)
		ctx.AddParameterChange(1, 0.75, 128)
		ctx.AddParameterChange(2, 0.9, 0)
		ctx.SortParameterChanges()

		for _, change := range ctx.GetParameterChanges() {
			ctx.ApplyParameterChange(change)
		}

		work := ctx.WorkBuffer()
		temp := ctx.TempBuffer()
		work[0] = 1
		temp[0] = 1

		ctx.PassThrough()
	}

	block() // warm up any lazily-built state before measuring

	if allocs := testing.AllocsPerRun(100, block); allocs != 0 {
		t.Fatalf("expected zero allocations on the audio path, got %f", allocs)
	}
}

// The host is not supposed to exceed the block size it declared, but some do;
// the buffers must stay total instead of panicking into the C++ host.
func TestWorkAndTempBufferSurviveOversizedBlocks(t *testing.T) {
	ctx := NewContext(64, param.NewRegistry())
	ctx.Input = [][]float32{make([]float32, 256)}
	ctx.Output = [][]float32{make([]float32, 256)}

	if got := len(ctx.WorkBuffer()); got != 256 {
		t.Errorf("WorkBuffer length = %d, want 256", got)
	}
	if got := len(ctx.TempBuffer()); got != 256 {
		t.Errorf("TempBuffer length = %d, want 256", got)
	}

	// The grown buffers are retained, so a repeat block costs nothing.
	if allocs := testing.AllocsPerRun(50, func() {
		_ = ctx.WorkBuffer()
		_ = ctx.TempBuffer()
	}); allocs != 0 {
		t.Errorf("expected the grown buffers to be reused, got %f allocations", allocs)
	}
}

func TestNumSamplesInt32Saturates(t *testing.T) {
	ctx := NewContext(0, param.NewRegistry())

	if got := ctx.numSamplesInt32(); got != 0 {
		t.Errorf("numSamplesInt32 with no buffers = %d, want 0", got)
	}

	ctx.Input = [][]float32{make([]float32, 32)}
	if got := ctx.numSamplesInt32(); got != 32 {
		t.Errorf("numSamplesInt32 = %d, want 32", got)
	}
}

func TestNumSamplesPrefersOutputWhenInputIsEmpty(t *testing.T) {
	ctx := NewContext(64, param.NewRegistry())

	if got := ctx.NumSamples(); got != 0 {
		t.Errorf("NumSamples with no buffers = %d, want 0", got)
	}

	ctx.Output = [][]float32{make([]float32, 48)}
	if got := ctx.NumSamples(); got != 48 {
		t.Errorf("NumSamples = %d, want 48 (generators have no input bus)", got)
	}

	ctx.Input = [][]float32{make([]float32, 16)}
	if got := ctx.NumSamples(); got != 16 {
		t.Errorf("NumSamples = %d, want 16", got)
	}
}

func TestClearAndPassThrough(t *testing.T) {
	ctx := NewContext(8, param.NewRegistry())
	ctx.Input = [][]float32{{1, 2, 3, 4}, {5, 6, 7, 8}}
	ctx.Output = [][]float32{make([]float32, 4)}

	if got := ctx.NumInputChannels(); got != 2 {
		t.Errorf("NumInputChannels = %d, want 2", got)
	}
	if got := ctx.NumOutputChannels(); got != 1 {
		t.Errorf("NumOutputChannels = %d, want 1", got)
	}

	// PassThrough is bounded by the narrower side, here the single output.
	ctx.PassThrough()
	if ctx.Output[0][3] != 4 {
		t.Errorf("PassThrough did not copy input: %v", ctx.Output[0])
	}

	ctx.Clear()
	for _, v := range ctx.Output[0] {
		if v != 0 {
			t.Fatalf("Clear left non-zero output: %v", ctx.Output[0])
		}
	}
}
