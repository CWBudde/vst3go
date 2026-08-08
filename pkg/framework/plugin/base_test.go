package plugin

import (
	"bytes"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

func TestNewBaseWiresParametersAndState(t *testing.T) {
	b := NewBase(&Info{ID: "com.example.base", Name: "Base"})

	if b.Parameters() == nil {
		t.Fatal("parameter registry must be non-nil")
	}
	if b.State() == nil {
		t.Fatal("state manager must be non-nil")
	}
	if b.Info.ID != "com.example.base" {
		t.Errorf("Info.ID = %q, want %q", b.Info.ID, "com.example.base")
	}
}

func TestNewBaseCopiesInfo(t *testing.T) {
	info := &Info{ID: "com.example.base", Name: "Original"}

	b := NewBase(info)
	info.Name = "Mutated"

	// Base stores Info by value, so a caller reusing the struct must not be
	// able to change the plugin's identity after construction.
	if b.Info.Name != "Original" {
		t.Errorf("Info.Name = %q, want %q — Base must not alias the caller's Info", b.Info.Name, "Original")
	}
}

// TestBaseStateUsesSameRegistry pins the wiring that makes Base.State useful:
// the manager must serialize the very parameters Base.Parameters exposes.
func TestBaseStateUsesSameRegistry(t *testing.T) {
	b := NewBase(&Info{ID: "com.example.base"})

	gain := param.New(0, "Gain").Range(0, 1).Default(0.25).Build()
	if err := b.Parameters().Add(gain); err != nil {
		t.Fatalf("Add: %v", err)
	}

	gain.SetNormalized(0.75)

	var buf bytes.Buffer
	if err := b.State().Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	gain.SetNormalized(0.1)

	if err := b.State().Load(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := gain.GetNormalized(); got != 0.75 {
		t.Errorf("after round-trip GetNormalized = %v, want 0.75", got)
	}
}
