package plugin

import (
	"errors"
	"math"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

func TestEditorParametersReflectAutomationAndRestoredState(t *testing.T) {
	registry := param.NewRegistry()
	if err := registry.Add(param.New(7, "Gain").Range(0, 1).Default(0.25).Build()); err != nil {
		t.Fatal(err)
	}
	component := newComponent(&recordingProcessor{params: registry})
	editor := EditorParameters{wrapper: &componentWrapper{component: component}}
	wantValue := func(want float64) {
		t.Helper()
		if got, ok := editor.Normalized(7); !ok || got != want {
			t.Fatalf("editor value = %v, %v, want %v", got, ok, want)
		}
	}
	wantValue(0.25)
	state, err := component.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if err := component.SetParamNormalized(7, 0.5); err != nil {
		t.Fatal(err)
	}
	wantValue(0.5)
	if err := editor.BeginEdit(7); err != nil {
		t.Fatal(err)
	}
	if err := editor.PerformEdit(7, 2); err != nil {
		t.Fatal(err)
	}
	if err := editor.EndEdit(7); err != nil {
		t.Fatal(err)
	}
	wantValue(1)
	if err := editor.PerformEdit(7, math.NaN()); !errors.Is(err, vst3.ErrInvalidArgument) {
		t.Fatalf("PerformEdit accepted NaN: %v", err)
	}
	wantValue(1)
	if err := component.SetState(state); err != nil {
		t.Fatal(err)
	}
	wantValue(0.25)
	if _, ok := editor.Normalized(8); ok {
		t.Fatal("editor found an unregistered parameter")
	}
}
