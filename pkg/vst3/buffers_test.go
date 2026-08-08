package vst3

import "testing"

// AudioBuffer and ProcessDataWrapper are normally built from host-owned C
// structs, which a _test.go file cannot construct (no cgo in tests). The
// accessors are still worth pinning: they are called from cgo callbacks, where
// an out-of-range index or a nil receiver would panic into C++.

func TestAudioBufferAccessors(t *testing.T) {
	buf := &AudioBuffer{
		channelBuffers: [][]float32{{1, 2, 3}, {4, 5, 6}},
		numSamples:     3,
	}

	if got := buf.NumChannels(); got != 2 {
		t.Errorf("NumChannels() = %d, want 2", got)
	}
	if got := buf.NumSamples(); got != 3 {
		t.Errorf("NumSamples() = %d, want 3", got)
	}
	if got := buf.GetChannel(1); len(got) != 3 || got[0] != 4 {
		t.Errorf("GetChannel(1) = %v, want [4 5 6]", got)
	}

	for _, idx := range []int{-1, 2, 1000} {
		if got := buf.GetChannel(idx); got != nil {
			t.Errorf("GetChannel(%d) = %v, want nil", idx, got)
		}
	}
}

func TestAudioBufferNilReceiver(t *testing.T) {
	var buf *AudioBuffer

	if got := buf.NumChannels(); got != 0 {
		t.Errorf("NumChannels() on nil = %d, want 0", got)
	}
	if got := buf.NumSamples(); got != 0 {
		t.Errorf("NumSamples() on nil = %d, want 0", got)
	}
	if got := buf.GetChannel(0); got != nil {
		t.Errorf("GetChannel(0) on nil = %v, want nil", got)
	}
}

func TestProcessDataWrapperAccessors(t *testing.T) {
	in := &AudioBuffer{channelBuffers: [][]float32{{1}}, numSamples: 1}
	out := &AudioBuffer{channelBuffers: [][]float32{{2}}, numSamples: 1}
	w := &ProcessDataWrapper{
		inputBuffers:  []*AudioBuffer{in},
		outputBuffers: []*AudioBuffer{out},
		context:       &ProcessContext{Tempo: 120},
	}

	if w.GetInput(0) != in {
		t.Error("GetInput(0) did not return the input buffer")
	}
	if w.GetOutput(0) != out {
		t.Error("GetOutput(0) did not return the output buffer")
	}
	if ctx := w.GetContext(); ctx == nil || ctx.Tempo != 120 {
		t.Errorf("GetContext() = %v, want tempo 120", ctx)
	}
	for _, idx := range []int{-1, 1, 99} {
		if w.GetInput(idx) != nil {
			t.Errorf("GetInput(%d) should be nil", idx)
		}
		if w.GetOutput(idx) != nil {
			t.Errorf("GetOutput(%d) should be nil", idx)
		}
	}
	// A wrapper without a C pointer must report 0 rather than dereferencing it.
	if got := w.NumSamples(); got != 0 {
		t.Errorf("NumSamples() with nil ptr = %d, want 0", got)
	}
}

func TestProcessDataWrapperNilReceiver(t *testing.T) {
	var w *ProcessDataWrapper

	if w.GetInput(0) != nil || w.GetOutput(0) != nil {
		t.Error("GetInput/GetOutput on nil wrapper must return nil")
	}
	if w.GetContext() != nil {
		t.Error("GetContext on nil wrapper must return nil")
	}
	if got := w.NumSamples(); got != 0 {
		t.Errorf("NumSamples() on nil wrapper = %d, want 0", got)
	}
	if got := NewProcessDataWrapper(nil); got != nil {
		t.Error("NewProcessDataWrapper(nil) must return nil")
	}
}

func TestProcessContextPredicates(t *testing.T) {
	tests := []struct {
		name  string
		flag  ProcessContextFlags
		check func(*ProcessContext) bool
	}{
		{name: "IsPlaying", flag: ProcessContextFlagPlaying, check: (*ProcessContext).IsPlaying},
		{name: "IsRecording", flag: ProcessContextFlagRecording, check: (*ProcessContext).IsRecording},
		{name: "IsCycleActive", flag: ProcessContextFlagCycleActive, check: (*ProcessContext).IsCycleActive},
		{name: "HasTempo", flag: ProcessContextFlagTempoValid, check: (*ProcessContext).HasTempo},
		{name: "HasTimeSignature", flag: ProcessContextFlagTimeSigValid, check: (*ProcessContext).HasTimeSignature},
		{name: "HasBarPosition", flag: ProcessContextFlagBarPositionValid, check: (*ProcessContext).HasBarPosition},
		{name: "HasProjectTimeMusic", flag: ProcessContextFlagProjectTimeValid, check: (*ProcessContext).HasProjectTimeMusic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check(&ProcessContext{State: tt.flag}) != true {
				t.Errorf("%s should be true when its flag is set", tt.name)
			}
			// Every other flag set, but not this one.
			if tt.check(&ProcessContext{State: ^tt.flag}) != false {
				t.Errorf("%s should be false when only other flags are set", tt.name)
			}
			if tt.check(nil) != false {
				t.Errorf("%s on a nil context should be false, not a panic", tt.name)
			}
		})
	}
}
