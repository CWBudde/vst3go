package state

import (
	"bytes"
	"errors"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

func TestManagerRejectsNilRegistry(t *testing.T) {
	m := NewManager(nil)

	if err := m.Save(&bytes.Buffer{}); !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("Save() error = %v, want ErrNilRegistry", err)
	}

	if err := m.Load(bytes.NewReader([]byte("VST3GO"))); !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("Load() error = %v, want ErrNilRegistry", err)
	}
}

func TestManagerSaveLoadRoundTrip(t *testing.T) {
	reg := param.NewRegistry()
	p := &param.Parameter{ID: 1, Name: "Gain", Min: -12, Max: 12}
	p.SetPlain(6)
	if err := reg.Add(p); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	var buf bytes.Buffer
	m := NewManager(reg)
	if err := m.Save(&buf); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	p.SetPlain(-6)
	if err := m.Load(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := p.GetPlain(); got != 6 {
		t.Fatalf("GetPlain() after Load = %f, want 6", got)
	}
}
