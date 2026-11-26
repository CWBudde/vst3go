package param

import (
	"errors"
	"math"
	"testing"
)

func TestLogarithmicDefaultsAndMidpoint(t *testing.T) {
	// DAV frequency and bandwidth mappings evaluated independently.
	tests := []struct {
		name          string
		minimum       float64
		maximum       float64
		factor        float64
		defaultPlain  float64
		defaultNorm   float64
		midpointPlain float64
	}{
		{"frequency", 20, 20000, 1100, 1000, 0.572031757333367, 604.5297950186917},
		{"bandwidth", 0.01, 20, 2000, 1, 0.6058967948667972, 0.4471067222344374},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(12, tc.name).Default(tc.defaultPlain).Logarithmic(tc.factor).
				Range(tc.minimum, tc.maximum).TryBuild()
			if err != nil {
				t.Fatal(err)
			}

			if got := p.DefaultValue; math.Abs(got-tc.defaultNorm) > 1e-14 {
				t.Errorf("normalized default = %.16g, want %.16g", got, tc.defaultNorm)
			}
			if got := p.GetPlain(); math.Abs(got-tc.defaultPlain) > 1e-10 {
				t.Errorf("current plain value = %.16g, want %.16g", got, tc.defaultPlain)
			}
			if got := p.Denormalize(0.5); math.Abs(got-tc.midpointPlain) > 1e-10 {
				t.Errorf("plain midpoint = %.16g, want %.16g", got, tc.midpointPlain)
			}

			p.SetNormalized(0.5)
			if got := p.GetPlain(); math.Abs(got-tc.midpointPlain) > 1e-10 {
				t.Errorf("host midpoint = %.16g, want %.16g", got, tc.midpointPlain)
			}
			p.SetPlain(tc.defaultPlain)
			if got := p.GetNormalized(); math.Abs(got-tc.defaultNorm) > 1e-14 {
				t.Errorf("plain setter encoded %.16g, want %.16g", got, tc.defaultNorm)
			}
		})
	}
}

func TestLogarithmicEndpointsAndRoundTrip(t *testing.T) {
	p := New(0, "Curve").Range(-3.25, 24.875).Logarithmic(1100).Build()

	if p.Normalize(p.Min) != 0 || p.Normalize(p.Max) != 1 ||
		p.Denormalize(0) != p.Min || p.Denormalize(1) != p.Max {
		t.Fatal("curve did not preserve exact endpoints")
	}
	if p.Normalize(math.Inf(-1)) != 0 || p.Normalize(math.Inf(1)) != 1 ||
		p.Denormalize(-1) != p.Min || p.Denormalize(2) != p.Max ||
		p.Denormalize(math.NaN()) != p.Min {
		t.Fatal("curve did not clamp out-of-range conversions")
	}

	for _, normalized := range []float64{0.125, 0.25, 0.5, 0.875, 0.999} {
		if got := p.Normalize(p.Denormalize(normalized)); math.Abs(got-normalized) > 2e-14 {
			t.Errorf("normalized round-trip = %.16g, want %.16g", got, normalized)
		}
	}
	for _, plain := range []float64{-3, -1.25, 0, 0.125, 10, 24.5} {
		if got := p.Denormalize(p.Normalize(plain)); math.Abs(got-plain) > 2e-12 {
			t.Errorf("plain round-trip = %.16g, want %.16g", got, plain)
		}
	}
}

func TestLogarithmicUsesPlainFormatterAndParser(t *testing.T) {
	p := FrequencyParameter(3, "Frequency", 20, 20000, 1000).Logarithmic(1100).Build()
	if got := p.FormatValue(0.5); got != "604.5 Hz" {
		t.Errorf("formatted midpoint = %q, want 604.5 Hz", got)
	}

	parsed, err := p.ParseValue("1.00 kHz")
	if err != nil || math.Abs(parsed-0.572031757333367) > 1e-14 {
		t.Errorf("parsed frequency = %.16g, err=%v", parsed, err)
	}
}

func TestLogarithmicRejectsInvalidFactorAndSteppedParameters(t *testing.T) {
	for _, factor := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		p, err := New(1, "Invalid").Logarithmic(factor).TryBuild()
		if !errors.Is(err, ErrInvalidCurveFactor) || p.Denormalize(0.5) != 0.5 {
			t.Errorf("factor %v returned err=%v and midpoint=%g", factor, err, p.Denormalize(0.5))
		}
	}

	p, err := New(2, "Steps").Range(0, 4).Logarithmic(1100).Steps(4).TryBuild()
	if !errors.Is(err, ErrInvalidStepCount) {
		t.Fatalf("stepped curve error = %v", err)
	}
	if p.Denormalize(0.4) != 2 || p.Normalize(3) != 0.75 {
		t.Fatal("stepped mapping changed despite the invalid curve")
	}

	bypass, err := New(3, "Bypass").Bypass().Logarithmic(3).TryBuild()
	if !errors.Is(err, ErrInvalidStepCount) || bypass.Denormalize(0.5) != 1 {
		t.Fatalf("bypass curve returned err=%v and midpoint=%g", err, bypass.Denormalize(0.5))
	}

	_, err = New(4, "Unbounded").Range(0, math.Inf(1)).Logarithmic(3).TryBuild()
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("non-finite curve range error = %v", err)
	}
}

func TestLogarithmicExtremeFactors(t *testing.T) {
	tiny, err := New(3, "Tiny").Logarithmic(math.SmallestNonzeroFloat64).TryBuild()
	if err != nil || tiny.Normalize(0.25) != 0.25 || tiny.Denormalize(0.25) != 0.25 {
		t.Fatalf("subnormal factor returned err=%v, normalized=%g, plain=%g",
			err, tiny.Normalize(0.25), tiny.Denormalize(0.25))
	}

	large, err := New(4, "Large").Logarithmic(math.MaxFloat64).TryBuild()
	if err != nil {
		t.Fatal(err)
	}
	if got := large.Denormalize(0.5); !(got > 0 && got < 1e-100) {
		t.Fatalf("max factor midpoint = %g", got)
	}
	nearEnd := math.Nextafter(1, 0)
	if got := large.Denormalize(nearEnd); !(got < 1 && got > 0.99) {
		t.Fatalf("max factor value at %.17g = %.17g", nearEnd, got)
	}
	if large.Denormalize(1) != 1 || large.Normalize(1) != 1 {
		t.Fatal("max factor did not preserve the last endpoint")
	}
}
