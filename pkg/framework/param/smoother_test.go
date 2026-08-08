package param

import (
	"math"
	"testing"
)

func TestSmoother(t *testing.T) {
	t.Run("LinearSmoothing", func(t *testing.T) {
		smoother := NewSmoother(LinearSmoothing, 10) // 10 samples
		smoother.Reset(0.0)
		smoother.SetTarget(1.0)

		// Should take 10 samples to reach target
		for i := 0; i < 10; i++ {
			value := smoother.Next()
			expected := float64(i+1) * 0.1
			if math.Abs(value-expected) > 0.001 {
				t.Errorf("Sample %d: expected %f, got %f", i, expected, value)
			}
		}

		// Should stay at target
		if smoother.Next() != 1.0 {
			t.Error("Should stay at target after reaching it")
		}
		if smoother.IsSmoothing() {
			t.Error("Should not be smoothing after reaching target")
		}
	})

	t.Run("ExponentialSmoothing", func(t *testing.T) {
		smoother := NewSmoother(ExponentialSmoothing, 0.9) // High = slow
		smoother.Reset(0.0)
		smoother.SetTarget(1.0)

		// Should approach target exponentially
		prev := 0.0
		for i := 0; i < 50; i++ {
			value := smoother.Next()
			if value <= prev {
				t.Error("Value should be increasing")
			}
			if value >= 1.0 {
				t.Error("Should not exceed target")
			}
			prev = value
		}

		// Should eventually reach target (within threshold)
		for i := 0; i < 200; i++ {
			smoother.Next()
		}
		if smoother.IsSmoothing() {
			t.Error("Should have reached target by now")
		}
	})

	t.Run("LogarithmicSmoothing", func(t *testing.T) {
		smoother := NewSmoother(LogarithmicSmoothing, 10)
		smoother.Reset(100.0)      // Start at 100 Hz
		smoother.SetTarget(1000.0) // Target 1000 Hz

		// Should interpolate in log space
		values := []float64{}
		for i := 0; i < 10; i++ {
			values = append(values, smoother.Next())
		}

		// Check that we're interpolating logarithmically
		// The ratio between consecutive values should be constant
		ratio := values[1] / values[0]
		for i := 2; i < len(values); i++ {
			currentRatio := values[i] / values[i-1]
			if math.Abs(currentRatio-ratio) > 0.01 {
				t.Error("Logarithmic interpolation not maintaining constant ratio")
			}
		}
	})

	t.Run("Threshold", func(t *testing.T) {
		smoother := NewSmoother(ExponentialSmoothing, 0.9)
		smoother.SetThreshold(0.1)
		smoother.Reset(0.0)
		smoother.SetTarget(0.05) // Less than threshold

		// Should not start smoothing
		if smoother.IsSmoothing() {
			t.Error("Should not smooth when change is below threshold")
		}
	})

	t.Run("Process", func(t *testing.T) {
		smoother := NewSmoother(LinearSmoothing, 5)
		smoother.Reset(0.0)
		smoother.SetTarget(1.0)

		buffer := []float32{1.0, 1.0, 1.0, 1.0, 1.0}
		smoother.Process(buffer, func(value float64, sample float32) float32 {
			return sample * float32(value)
		})

		// Check that smoothing was applied
		expected := []float32{0.2, 0.4, 0.6, 0.8, 1.0}
		for i, v := range buffer {
			if math.Abs(float64(v-expected[i])) > 0.001 {
				t.Errorf("Sample %d: expected %f, got %f", i, expected[i], v)
			}
		}
	})
}

func TestSmoothedParameter(t *testing.T) {
	t.Run("BasicOperation", func(t *testing.T) {
		// Create a test parameter
		param := &Parameter{
			ID:           1,
			Name:         "Test",
			ShortName:    "Test",
			Unit:         "",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.5,
			StepCount:    0,
			Flags:        CanAutomate,
			UnitID:       0,
		}
		param.SetNormalized(0.5)

		// Wrap with smoothing
		smoothed := NewSmoothedParameter(param, ExponentialSmoothing, 0.9)

		// Set a new value
		smoothed.SetNormalized(1.0)

		// Should smooth towards the new value
		prev := 0.5
		for i := 0; i < 10; i++ {
			value := smoothed.Advance()
			if value <= prev {
				t.Error("Value should be increasing")
			}
			prev = value
		}
	})

	t.Run("DisableSmoothing", func(t *testing.T) {
		param := &Parameter{
			ID:           1,
			Name:         "Test",
			ShortName:    "Test",
			Unit:         "",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.5,
			StepCount:    0,
			Flags:        CanAutomate,
			UnitID:       0,
		}
		param.SetNormalized(0.5)

		smoothed := NewSmoothedParameter(param, LinearSmoothing, 10)
		smoothed.SetSmoothing(false)
		smoothed.SetNormalized(1.0)

		// Should jump immediately
		if smoothed.Advance() != 1.0 {
			t.Error("Should not smooth when disabled")
		}
	})

	t.Run("UpdateSampleRate", func(t *testing.T) {
		param := &Parameter{
			ID:           1,
			Name:         "Test",
			ShortName:    "Test",
			Unit:         "",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.5,
			StepCount:    0,
			Flags:        CanAutomate,
			UnitID:       0,
		}
		param.SetNormalized(0.5)

		// Linear smoothing with time-based rate
		smoothed := NewSmoothedParameter(param, LinearSmoothing, 10)
		smoothed.UpdateSampleRate(48000, 20) // 20ms at 48kHz

		// Should have updated the rate
		if smoothed.smoother.Rate() != 960 { // 48000 * 20 / 1000
			t.Errorf("Expected rate 960, got %f", smoothed.smoother.Rate())
		}
	})
}

func TestParameterSmoother(t *testing.T) {
	t.Run("ManageMultiple", func(t *testing.T) {
		ps := NewParameterSmoother()

		// Create test parameters
		param1 := &Parameter{
			ID:           1,
			Name:         "Param1",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.0,
		}
		param1.SetNormalized(0.0)

		param2 := &Parameter{
			ID:           2,
			Name:         "Param2",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.5,
		}
		param2.SetNormalized(0.5)

		// Add with different smoothing types
		ps.Add(1, param1, LinearSmoothing, 5)
		ps.Add(2, param2, ExponentialSmoothing, 0.9)

		// Set new values through the parameter smoother
		ps.SetValue(1, 1.0)
		ps.SetValue(2, 1.0)

		// Get smoothed values
		val1 := ps.Advance(1)
		val2 := ps.Advance(2)

		// For linear smoothing with 5 steps, first value should be 0.2
		if val1 <= 0.0 || val1 >= 0.3 {
			t.Errorf("Parameter 1 should be smoothing (expected ~0.2), got %f", val1)
		}
		// For exponential smoothing, should be between old and new value
		if val2 <= 0.5 || val2 >= 1.0 {
			t.Errorf("Parameter 2 should be smoothing (between 0.5 and 1.0), got %f", val2)
		}
	})

	t.Run("DisableSpecific", func(t *testing.T) {
		ps := NewParameterSmoother()

		param := &Parameter{
			ID:           1,
			Name:         "Test",
			Min:          0.0,
			Max:          1.0,
			DefaultValue: 0.0,
		}
		param.SetNormalized(0.0)

		ps.Add(1, param, LinearSmoothing, 10)
		ps.SetSmoothing(1, false)

		ps.SetValue(1, 1.0)
		if ps.Advance(1) != 1.0 {
			t.Error("Should not smooth when disabled")
		}
	})
}

func BenchmarkSmoother(b *testing.B) {
	b.Run("LinearNext", func(b *testing.B) {
		smoother := NewSmoother(LinearSmoothing, 100)
		smoother.SetTarget(1.0)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = smoother.Next()
		}
	})

	b.Run("ExponentialNext", func(b *testing.B) {
		smoother := NewSmoother(ExponentialSmoothing, 0.99)
		smoother.SetTarget(1.0)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = smoother.Next()
		}
	})

	b.Run("LogarithmicNext", func(b *testing.B) {
		smoother := NewSmoother(LogarithmicSmoothing, 100)
		smoother.SetTarget(1000.0)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = smoother.Next()
		}
	})

	b.Run("ProcessBuffer", func(b *testing.B) {
		smoother := NewSmoother(ExponentialSmoothing, 0.99)
		smoother.SetTarget(1.0)
		buffer := make([]float32, 512)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			smoother.Process(buffer, func(value float64, sample float32) float32 {
				return sample * float32(value)
			})
		}
	})
}

func TestSmootherDegenerateRateDoesNotHang(t *testing.T) {
	tests := []struct {
		name          string
		smoothingType SmoothingType
		rate          float64
		start, target float64
	}{
		{"linear zero rate", LinearSmoothing, 0, 0, 1},
		{"linear negative rate", LinearSmoothing, -5, 0, 1},
		{"logarithmic zero rate", LogarithmicSmoothing, 0, 100, 1000},
		{"exponential pole one", ExponentialSmoothing, 1, 0, 1},
		{"exponential negative pole", ExponentialSmoothing, -1, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSmoother(tt.smoothingType, tt.rate)
			s.Reset(tt.start)
			s.SetTarget(tt.target)

			// A degenerate rate must snap instead of ramping forever.
			if got := s.Next(); got != tt.target {
				t.Errorf("Next() = %v, want %v", got, tt.target)
			}
			if s.IsSmoothing() {
				t.Error("smoother stuck in an infinite ramp")
			}
		})
	}
}

func TestSmootherUpdateSampleRateZeroTime(t *testing.T) {
	p := &Parameter{Min: 0, Max: 1}
	p.SetNormalized(0)

	sp := NewSmoothedParameter(p, LinearSmoothing, 10)
	sp.UpdateSampleRate(48000, 0) // rate becomes 0
	sp.SetNormalized(1.0)

	if got := sp.Advance(); got != 1.0 {
		t.Errorf("Advance() = %v, want 1.0", got)
	}
	if sp.Smoother().IsSmoothing() {
		t.Error("smoother stuck after a zero-length ramp")
	}
}

func TestSmootherUpdateSampleRateLogarithmic(t *testing.T) {
	p := &Parameter{Min: 20, Max: 20000}
	p.SetPlain(100)

	sp := NewSmoothedParameter(p, LogarithmicSmoothing, 10)
	sp.UpdateSampleRate(48000, 20)

	if got := sp.Smoother().Rate(); got != 960 {
		t.Errorf("Rate() = %v, want 960", got)
	}
}

func TestSmootherLogarithmicNonPositiveRange(t *testing.T) {
	// dB gain and pan ranges include zero and negatives, where log
	// interpolation is undefined; the ramp must fall back to linear.
	s := NewSmoother(LogarithmicSmoothing, 4)
	s.Reset(-1.0)
	s.SetTarget(1.0)

	want := []float64{-0.5, 0, 0.5, 1.0}
	for i, w := range want {
		if got := s.Next(); math.Abs(got-w) > 1e-9 {
			t.Fatalf("sample %d: Next() = %v, want %v", i, got, w)
		}
	}
	if s.IsSmoothing() {
		t.Error("ramp should be complete")
	}
}

func TestSmootherRejectsNonFiniteTarget(t *testing.T) {
	s := NewSmoother(LinearSmoothing, 10)
	s.Reset(0.5)

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		s.SetTarget(v)
		if got := s.Next(); got != 0.5 {
			t.Errorf("SetTarget(%v) poisoned the smoother: %v", v, got)
		}
	}
}

func TestSmootherConcurrentRetarget(t *testing.T) {
	// The audio thread must be able to advance while the UI thread retargets.
	s := NewSmoother(LinearSmoothing, 32)
	s.Reset(0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10000; i++ {
			s.SetTarget(float64(i % 2))
		}
	}()

	for i := 0; i < 10000; i++ {
		if v := s.Next(); v < -0.001 || v > 1.001 {
			t.Errorf("smoothed value out of range: %v", v)
			break
		}
	}
	<-done
}

func TestSmootherAdvanceDoesNotAllocate(t *testing.T) {
	p := &Parameter{Min: 0, Max: 1}
	p.SetNormalized(0)
	p.EnableSmoothing(LinearSmoothing, 1024)
	p.SetNormalized(1.0)

	allocs := testing.AllocsPerRun(1000, func() {
		p.Advance()
	})

	if allocs != 0 {
		t.Fatalf("expected zero allocations per sample, got %f", allocs)
	}
}

func TestParameterSmootherAdvanceIsExplicit(t *testing.T) {
	ps := NewParameterSmoother()

	p := &Parameter{Min: 0, Max: 1}
	p.SetNormalized(0)
	ps.Add(1, p, LinearSmoothing, 4)
	ps.SetValue(1, 1.0)

	// Value must be a pure read, no matter how often it is called.
	if got := ps.Value(1); got != 0 {
		t.Errorf("Value() = %v before any advance, want 0", got)
	}
	if got := ps.Value(1); got != 0 {
		t.Errorf("Value() advanced the ramp: %v", got)
	}

	ps.AdvanceAll()
	if got := ps.Value(1); math.Abs(got-0.25) > 1e-9 {
		t.Errorf("Value() = %v after one advance, want 0.25", got)
	}

	ps.AdvanceAll()
	if got := ps.Value(1); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("Value() = %v after two advances, want 0.5", got)
	}
}

func TestParameterSmootherAdvanceAllDoesNotAllocate(t *testing.T) {
	ps := NewParameterSmoother()
	for id := uint32(1); id <= 8; id++ {
		p := &Parameter{Min: 0, Max: 1}
		p.SetNormalized(0)
		ps.Add(id, p, LinearSmoothing, 4096)
		ps.SetValue(id, 1.0)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		ps.AdvanceAll()
	})

	if allocs != 0 {
		t.Fatalf("expected zero allocations per sample, got %f", allocs)
	}
}
