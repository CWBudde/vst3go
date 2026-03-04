package param

import "testing"

func TestNormalizedAccessors(t *testing.T) {
	p := &Parameter{Min: -12, Max: 12}

	p.SetNormalized(0.75)
	if got := p.GetNormalized(); got != 0.75 {
		t.Fatalf("GetNormalized() = %f, want 0.75", got)
	}

	if got := p.GetValue(); got != 0.75 {
		t.Fatalf("GetValue() = %f, want 0.75", got)
	}
}

func TestPlainAccessors(t *testing.T) {
	p := &Parameter{Min: -12, Max: 12}

	p.SetPlain(6)
	if got := p.GetPlain(); got != 6 {
		t.Fatalf("GetPlain() = %f, want 6", got)
	}

	if got := p.GetPlainValue(); got != 6 {
		t.Fatalf("GetPlainValue() = %f, want 6", got)
	}
}
