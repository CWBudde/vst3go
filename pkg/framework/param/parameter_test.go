package param

import (
	"math"
	"testing"
)

func TestNormalizedAccessors(t *testing.T) {
	p := &Parameter{Min: -12, Max: 12}

	p.SetNormalized(0.75)
	if got := p.GetNormalized(); got != 0.75 {
		t.Fatalf("GetNormalized() = %f, want 0.75", got)
	}
}

// TestDeprecatedAliases pins the compatibility spellings to their canonical
// counterparts. They are deprecated, not removed, so downstream code that has
// not migrated yet keeps working.
func TestDeprecatedAliases(t *testing.T) {
	p := &Parameter{Min: -12, Max: 12}

	p.SetValue(0.75)
	if got := p.GetValue(); got != 0.75 {
		t.Fatalf("GetValue() = %f, want 0.75", got)
	}
	if got := p.GetNormalized(); got != 0.75 {
		t.Fatalf("SetValue did not reach SetNormalized: %f", got)
	}

	p.SetPlainValue(6)
	if got := p.GetPlainValue(); got != 6 {
		t.Fatalf("GetPlainValue() = %f, want 6", got)
	}
	if got := p.GetPlain(); got != 6 {
		t.Fatalf("SetPlainValue did not reach SetPlain: %f", got)
	}
}

func TestPlainAccessors(t *testing.T) {
	p := &Parameter{Min: -12, Max: 12}

	p.SetPlain(6)
	if got := p.GetPlain(); got != 6 {
		t.Fatalf("GetPlain() = %f, want 6", got)
	}
}

func TestSteppedRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		min       float64
		stepCount int32
	}{
		{"toggle", 0, 1},
		{"bypass", 0, 1},
		{"four way list", 0, 3},
		{"offset list", 2, 5},
		{"many detents", -3, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parameter{
				Min:       tt.min,
				Max:       tt.min + float64(tt.stepCount),
				StepCount: tt.stepCount,
			}

			for i := int32(0); i <= tt.stepCount; i++ {
				plain := tt.min + float64(i)

				normalized := p.Normalize(plain)
				if got := p.Denormalize(normalized); got != plain {
					t.Errorf("detent %d: Denormalize(Normalize(%v)) = %v", i, plain, got)
				}

				// Detents must be exactly reachable from their normalized position.
				want := float64(i) / float64(tt.stepCount)
				if math.Abs(normalized-want) > 1e-12 {
					t.Errorf("detent %d: Normalize(%v) = %v, want %v", i, plain, normalized, want)
				}
			}
		})
	}
}

func TestSteppedQuantization(t *testing.T) {
	// A one-step (bypass style) parameter must collapse to two plain values.
	p := &Parameter{Min: 0, Max: 1, StepCount: 1}

	tests := []struct {
		normalized float64
		want       float64
	}{
		{0.0, 0},
		{0.1, 0},
		{0.49, 0},
		{0.5, 1}, // round-half-away-from-zero
		{0.9, 1},
		{1.0, 1},
	}

	for _, tt := range tests {
		if got := p.Denormalize(tt.normalized); got != tt.want {
			t.Errorf("Denormalize(%v) = %v, want %v", tt.normalized, got, tt.want)
		}
	}
}

func TestNonFiniteValuesRejected(t *testing.T) {
	nonFinite := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}

	t.Run("SetNormalized", func(t *testing.T) {
		for _, v := range nonFinite {
			p := &Parameter{Min: -12, Max: 12}
			p.SetNormalized(0.25)
			p.SetNormalized(v)

			if got := p.GetNormalized(); got != 0.25 {
				t.Errorf("SetNormalized(%v) poisoned the store: got %v", v, got)
			}
		}
	})

	t.Run("SetPlain", func(t *testing.T) {
		for _, v := range nonFinite {
			p := &Parameter{Min: -12, Max: 12}
			p.SetPlain(6)
			p.SetPlain(v)

			if got := p.GetPlain(); got != 6 {
				t.Errorf("SetPlain(%v) poisoned the store: got %v", v, got)
			}
		}
	})

	t.Run("Conversions", func(t *testing.T) {
		p := &Parameter{Min: -12, Max: 12}
		if got := p.Normalize(math.NaN()); got != 0 {
			t.Errorf("Normalize(NaN) = %v, want 0", got)
		}
		if got := p.Denormalize(math.NaN()); got != p.Min {
			t.Errorf("Denormalize(NaN) = %v, want %v", got, p.Min)
		}
		if got := p.Normalize(math.Inf(1)); got != 1 {
			t.Errorf("Normalize(+Inf) = %v, want 1", got)
		}
		if got := p.Normalize(math.Inf(-1)); got != 0 {
			t.Errorf("Normalize(-Inf) = %v, want 0", got)
		}
	})
}

func TestSetNormalizedFeedsSmoother(t *testing.T) {
	// Regression guard for smoothing being bypassed: a plain *Parameter, as
	// held by the registry and the state manager, must drive the smoother.
	p := &Parameter{Min: 0, Max: 1}
	p.SetNormalized(0)
	p.EnableSmoothing(LinearSmoothing, 4)

	p.SetNormalized(1.0)

	want := []float64{0.25, 0.5, 0.75, 1.0}
	for i, w := range want {
		if got := p.Advance(); math.Abs(got-w) > 1e-9 {
			t.Fatalf("sample %d: Advance() = %v, want %v", i, got, w)
		}
	}

	// Value must not advance the ramp.
	before := p.SmoothedValue()
	if after := p.SmoothedValue(); after != before {
		t.Errorf("SmoothedValue advanced the ramp: %v then %v", before, after)
	}
}
