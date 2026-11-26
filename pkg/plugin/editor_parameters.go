package plugin

import (
	"math"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

// EditorParameters connects a view to its processor instance. Use it on the
// host UI thread, for the lifetime of the view. Refresh controls from Normalized
// during the view's redraw or update cycle to pick up host automation and
// restored presets. Reading a value does not send an edit back to the host.
type EditorParameters struct {
	wrapper *componentWrapper
}

func (e EditorParameters) parameter(id uint32) *param.Parameter {
	if e.wrapper == nil {
		return nil
	}
	component, ok := e.wrapper.component.(*componentImpl)
	if !ok {
		return nil
	}
	params := component.processor.GetParameters()
	if params == nil {
		return nil
	}
	return params.Get(id)
}

// Normalized reads the instance's current target value, including host edits.
func (e EditorParameters) Normalized(id uint32) (float64, bool) {
	if p := e.parameter(id); p != nil {
		return p.GetNormalized(), true
	}
	return 0, false
}

// BeginEdit starts one gesture for the parameter on the host UI thread.
func (e EditorParameters) BeginEdit(id uint32) error {
	if e.parameter(id) == nil {
		return vst3.ErrInvalidArgument
	}
	e.wrapper.notifyParamBeginEdit(id)
	return nil
}

// PerformEdit updates the parameter and reports its clamped normalized value
// to the host. Call it for each change between BeginEdit and EndEdit.
func (e EditorParameters) PerformEdit(id uint32, normalized float64) error {
	p := e.parameter(id)
	if p == nil || math.IsNaN(normalized) || math.IsInf(normalized, 0) {
		return vst3.ErrInvalidArgument
	}
	normalized = min(max(normalized, 0), 1)
	if err := e.wrapper.component.SetParamNormalized(id, normalized); err != nil {
		return err
	}
	e.wrapper.notifyParamPerformEdit(id, normalized)
	return nil
}

// EndEdit closes a gesture. End outstanding gestures before removing a view.
func (e EditorParameters) EndEdit(id uint32) error {
	if e.parameter(id) == nil {
		return vst3.ErrInvalidArgument
	}
	e.wrapper.notifyParamEndEdit(id)
	return nil
}
