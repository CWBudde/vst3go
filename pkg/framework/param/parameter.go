// Package param provides VST3 parameter management with formatting,
// validation, and thread-safe value handling.
//
// A [Parameter] carries both a normalized value in 0..1, which is the domain
// the host automates and the preset format stores, and a plain value in the
// parameter's own units, which is what DSP wants. Prefer the explicit
// [Parameter.GetNormalized]/[Parameter.SetNormalized] and
// [Parameter.GetPlain]/[Parameter.SetPlain] spellings; the older Value forms
// are deprecated aliases that do not say which domain they mean.
//
// # Parameter smoothing
//
// Parameter smoothing is caller-driven. Enable it during processor setup with
// [Parameter.EnableSmoothing]. Host automation and calls to [Parameter.SetPlain]
// or [Parameter.SetNormalized] update the target. [Parameter.GetNormalized] and
// [Parameter.GetPlain] return that target immediately in their respective units.
//
// In the processor's audio loop, call [Parameter.Advance] once per sample frame
// and use the returned plain value for every channel in that frame. To advance
// a group of parameters, call [ParameterSmoother.AdvanceAll] once per frame and
// read each value with [ParameterSmoother.Value]. [Parameter.SmoothedValue] also
// reads the latest smoothed plain value without advancing the ramp.
//
// # Threading
//
// The value of a Parameter is stored atomically and is the supported channel
// between the control thread and the audio thread:
//
//   - Safe on any thread: GetNormalized, SetNormalized, GetPlain, SetPlain,
//     Normalize, Denormalize, SmoothedValue, Smoother. All are
//     lock-free and allocation-free.
//   - Control thread only: EnableSmoothing, DisableSmoothing, SetFormatter,
//     and every [Registry] mutation ([Registry.Add] and the builders).
//   - Audio thread, once per sample frame: [Parameter.Advance]. It owns the ramp
//     state, so exactly one thread may call it.
//   - Formatting and parsing (FormatValue, ParseValue) allocate and are
//     control-thread only.
//
// A [Registry] is guarded by an RWMutex, so concurrent use is memory-safe, but
// that makes its accessors unsuitable for the audio thread twice over: a
// lookup takes a lock the control thread can hold, and [Registry.All]
// allocates. Build the registry during setup, resolve the *[Parameter] values
// you need there, and hold those pointers for the audio path. Reading a
// parameter through its pointer is always safe from ProcessAudio; looking one
// up by ID is not.
//
// A [Smoother] follows the same split: the control thread publishes target and
// rate through atomics, the audio thread owns the ramp and picks changes up in
// [Smoother.Next]. [Smoother.Reset] touches ramp state directly and is
// control-thread only, to be called while stopped.
package param

import (
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
)

// Parameter represents a plugin parameter.
//
// A Parameter must not be copied once it has been used: it carries atomic
// state that the audio thread reads concurrently with the UI thread.
type Parameter struct {
	ID           uint32
	Name         string
	ShortName    string
	Unit         string
	Min          float64
	Max          float64
	DefaultValue float64
	// StepCount counts intervals, not positions: a toggle has StepCount 1 and
	// an N-way list has StepCount N-1 (see the VST3 ParameterInfo docs).
	StepCount int32
	Flags     uint32
	UnitID    int32

	// Normalized value, stored as raw bits for lock-free access from the audio thread.
	value atomic.Uint64

	// Optional smoother. Nil (the zero value) means no smoothing and no cost.
	smoother atomic.Pointer[Smoother]

	// Value formatting
	formatFunc func(float64) string
	parseFunc  func(string) (float64, error)
}

// Flags for parameters
const (
	CanAutomate     uint32 = 1 << 0
	IsReadOnly      uint32 = 1 << 1
	IsWrapAround    uint32 = 1 << 2
	IsList          uint32 = 1 << 3
	IsHidden        uint32 = 1 << 4
	IsProgramChange uint32 = 1 << 15
	IsBypass        uint32 = 1 << 16
)

// GetValue returns the current normalized value (0-1)
//
// Deprecated: use [Parameter.GetNormalized]. The Value spelling does not say
// which of the two value domains it means; the runtime no longer calls it.
func (p *Parameter) GetValue() float64 {
	return p.GetNormalized()
}

// GetNormalized returns the target value in normalized units (0–1).
func (p *Parameter) GetNormalized() float64 {
	return math.Float64frombits(p.value.Load())
}

// SetValue sets the normalized value (0-1)
//
// Deprecated: use [Parameter.SetNormalized]. The Value spelling does not say
// which of the two value domains it means; the runtime no longer calls it.
func (p *Parameter) SetValue(value float64) {
	p.SetNormalized(value)
}

// SetNormalized sets the normalized value (0-1) and, when smoothing is
// enabled, retargets the smoother. Non-finite input is dropped rather than
// stored: preset loading and host automation are untrusted sources, and a
// stored NaN would never clamp back out again.
func (p *Parameter) SetNormalized(value float64) {
	if !isFinite(value) {
		return
	}

	value = clamp01(value)
	p.value.Store(math.Float64bits(value))

	if s := p.smoother.Load(); s != nil {
		s.SetTarget(p.Denormalize(value))
	}
}

// GetPlainValue converts normalized to plain value
//
// Deprecated: use [Parameter.GetPlain]. The runtime no longer calls it.
func (p *Parameter) GetPlainValue() float64 {
	return p.GetPlain()
}

// GetPlain returns the target value in the parameter's plain units.
func (p *Parameter) GetPlain() float64 {
	return p.Denormalize(p.GetNormalized())
}

// SetPlainValue converts plain to normalized value
//
// Deprecated: use [Parameter.SetPlain]. The runtime no longer calls it.
func (p *Parameter) SetPlainValue(plain float64) {
	p.SetPlain(plain)
}

// SetPlain converts a plain value to normalized form and stores it.
func (p *Parameter) SetPlain(plain float64) {
	if !isFinite(plain) {
		return
	}
	p.SetNormalized(p.Normalize(plain))
}

// EnableSmoothing attaches a smoother initialized to the current plain value.
// Call it during processor setup. Subsequent value changes retarget the
// smoother, which the processor advances explicitly with [Parameter.Advance].
func (p *Parameter) EnableSmoothing(smoothingType SmoothingType, rate float64) *Smoother {
	s := NewSmoother(smoothingType, rate)
	s.Reset(p.GetPlain())
	p.smoother.Store(s)
	return s
}

// DisableSmoothing detaches the smoother; Advance then returns the plain value directly.
func (p *Parameter) DisableSmoothing() {
	p.smoother.Store(nil)
}

// Smoother returns the attached smoother, or nil when smoothing is disabled.
func (p *Parameter) Smoother() *Smoother {
	return p.smoother.Load()
}

// attachSmoother re-attaches a previously created smoother (used by SmoothedParameter).
func (p *Parameter) attachSmoother(s *Smoother) {
	p.smoother.Store(s)
}

// Advance moves the smoothing ramp by one sample frame and returns its plain
// value. With smoothing disabled it returns [Parameter.GetPlain]. Call it once
// per frame from the audio thread. It is lock-free and allocation-free.
func (p *Parameter) Advance() float64 {
	if s := p.smoother.Load(); s != nil {
		return s.Next()
	}
	return p.GetPlain()
}

// SmoothedValue returns the current smoothed plain value without advancing the ramp.
func (p *Parameter) SmoothedValue() float64 {
	if s := p.smoother.Load(); s != nil {
		return s.Value()
	}
	return p.GetPlain()
}

// SetFormatter sets custom value formatting
func (p *Parameter) SetFormatter(format func(float64) string, parse func(string) (float64, error)) {
	p.formatFunc = format
	p.parseFunc = parse
}

// FormatValue returns formatted parameter value
func (p *Parameter) FormatValue(normalized float64) string {
	plain := p.Denormalize(normalized)

	if p.formatFunc != nil {
		return p.formatFunc(plain)
	}

	// Default formatting
	if p.StepCount > 0 {
		// For discrete parameters, show integer
		return fmt.Sprintf("%.0f", plain)
	}
	return fmt.Sprintf("%.2f", plain)
}

// ParseValue parses string to normalized value
func (p *Parameter) ParseValue(str string) (float64, error) {
	if p.parseFunc != nil {
		plain, err := p.parseFunc(str)
		if err != nil {
			return 0, err
		}
		return p.Normalize(plain), nil
	}
	// Default parsing
	plain, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}
	return p.Normalize(plain), nil
}

// Normalize converts a plain value to normalized (0-1) form.
//
// For stepped parameters (StepCount > 0) the VST3 mapping is
// normalized = (plain - Min) / StepCount, which is the exact inverse of
// Denormalize on every detent.
func (p *Parameter) Normalize(plain float64) float64 {
	if math.IsNaN(plain) {
		return 0
	}

	if p.StepCount > 0 {
		return clamp01((plain - p.Min) / float64(p.StepCount))
	}

	if p.Max <= p.Min {
		return 0
	}
	return clamp01((plain - p.Min) / (p.Max - p.Min))
}

// Denormalize converts normalized (0-1) to a plain value.
//
// For stepped parameters (StepCount > 0) the VST3 mapping is
// plain = Min + round(normalized * StepCount), so the value snaps to a detent
// instead of degrading into a continuous range.
func (p *Parameter) Denormalize(normalized float64) float64 {
	if math.IsNaN(normalized) {
		normalized = 0
	}
	normalized = clamp01(normalized)

	if p.StepCount > 0 {
		return p.Min + math.Round(normalized*float64(p.StepCount))
	}
	return p.Min + normalized*(p.Max-p.Min)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
