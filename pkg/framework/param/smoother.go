package param

import (
	"math"
	"sync/atomic"
)

// SmoothingType defines different parameter smoothing algorithms.
type SmoothingType int

const (
	// LinearSmoothing uses linear interpolation; rate is the ramp length in samples.
	LinearSmoothing SmoothingType = iota
	// ExponentialSmoothing uses a one-pole filter; rate is the pole coefficient in [0,1).
	ExponentialSmoothing
	// LogarithmicSmoothing interpolates in log space (better for frequency
	// parameters). It is only defined for strictly positive values; a ramp
	// touching zero or negative values falls back to linear interpolation.
	LogarithmicSmoothing
)

// Smoother provides parameter smoothing to prevent zipper noise.
//
// The control thread publishes target/rate/threshold through atomics; the
// audio thread owns the ramp state and picks changes up in Next. This keeps
// the audio path lock-free and allocation-free.
type Smoother struct {
	smoothingType SmoothingType

	// Published across threads.
	target    atomic.Uint64 // float64 bits
	current   atomic.Uint64 // float64 bits
	rate      atomic.Uint64 // float64 bits
	threshold atomic.Uint64 // float64 bits
	smoothing atomic.Bool

	// Ramp state owned by the audio thread (Next). Reset also touches it and is
	// therefore documented as control-thread-only, to be called while stopped.
	ackTarget  float64
	activeMode SmoothingType
	step       float64
	logCurrent float64
	logTarget  float64
	logStep    float64
}

// NewSmoother creates a new parameter smoother.
// rate: smoothing rate (0-0.999 for exponential, ramp length in samples otherwise)
func NewSmoother(smoothingType SmoothingType, rate float64) *Smoother {
	s := &Smoother{smoothingType: smoothingType}
	s.rate.Store(math.Float64bits(rate))
	s.threshold.Store(math.Float64bits(0.0001))
	return s
}

func (s *Smoother) loadTarget() float64    { return math.Float64frombits(s.target.Load()) }
func (s *Smoother) loadCurrent() float64   { return math.Float64frombits(s.current.Load()) }
func (s *Smoother) loadRate() float64      { return math.Float64frombits(s.rate.Load()) }
func (s *Smoother) loadThreshold() float64 { return math.Float64frombits(s.threshold.Load()) }

// SetTarget sets the target value for smoothing. Safe to call from any thread.
func (s *Smoother) SetTarget(target float64) {
	if !isFinite(target) {
		return
	}
	if math.Abs(target-s.loadTarget()) < s.loadThreshold() {
		return // Target hasn't changed significantly
	}

	s.target.Store(math.Float64bits(target))
	s.smoothing.Store(true)
}

// beginRamp recomputes the audio-thread ramp state for a newly observed target.
// Every degenerate configuration snaps to the target instead of ramping, so a
// bad rate can never leave the smoother stuck in an infinite ramp.
func (s *Smoother) beginRamp(target float64) {
	current := s.loadCurrent()
	rate := s.loadRate()

	mode := s.smoothingType
	if mode == LogarithmicSmoothing && (current <= 0 || target <= 0) {
		mode = LinearSmoothing
	}
	s.activeMode = mode

	switch mode {
	case LinearSmoothing:
		if rate <= 0 {
			s.snap(target)
			return
		}
		s.step = (target - current) / rate
		if s.step == 0 {
			s.snap(target)
			return
		}

	case ExponentialSmoothing:
		// Outside [0,1) the one-pole filter diverges or never converges.
		if rate < 0 || rate >= 1 {
			s.snap(target)
			return
		}

	case LogarithmicSmoothing:
		if rate <= 0 {
			s.snap(target)
			return
		}
		s.logCurrent = math.Log(current)
		s.logTarget = math.Log(target)
		s.logStep = (s.logTarget - s.logCurrent) / rate
		if s.logStep == 0 {
			s.snap(target)
			return
		}
	}

	s.smoothing.Store(true)
}

func (s *Smoother) snap(target float64) {
	s.current.Store(math.Float64bits(target))
	s.step = 0
	s.logStep = 0
	s.smoothing.Store(false)
}

// Next advances the ramp by one sample and returns the new smoothed value.
// Call it from the audio thread only; it is lock-free and allocation-free.
func (s *Smoother) Next() float64 {
	target := s.loadTarget()
	if target != s.ackTarget {
		s.ackTarget = target
		s.beginRamp(target)
	}

	if !s.smoothing.Load() {
		return s.loadCurrent()
	}

	current := s.loadCurrent()

	switch s.activeMode {
	case ExponentialSmoothing:
		// One-pole filter: y = y + a * (x - y)
		current += (target - current) * (1.0 - s.loadRate())
		if math.Abs(current-target) < s.loadThreshold() {
			current = target
			s.smoothing.Store(false)
		}

	case LinearSmoothing:
		current += s.step
		if (s.step > 0 && current >= target) || (s.step < 0 && current <= target) {
			current = target
			s.smoothing.Store(false)
		}

	case LogarithmicSmoothing:
		s.logCurrent += s.logStep
		if (s.logStep > 0 && s.logCurrent >= s.logTarget) || (s.logStep < 0 && s.logCurrent <= s.logTarget) {
			current = target
			s.smoothing.Store(false)
		} else {
			current = math.Exp(s.logCurrent)
		}
	}

	s.current.Store(math.Float64bits(current))
	return current
}

// Value returns the current smoothed value without advancing the ramp.
func (s *Smoother) Value() float64 {
	return s.loadCurrent()
}

// Target returns the value the smoother is ramping towards.
func (s *Smoother) Target() float64 {
	return s.loadTarget()
}

// Process processes a buffer with the smoothed parameter.
// The callback receives the current smoothed value for each sample.
func (s *Smoother) Process(buffer []float32, callback func(value float64, sample float32) float32) {
	for i := range buffer {
		value := s.Next()
		buffer[i] = callback(value, buffer[i])
	}
}

// IsSmoothing returns true if the smoother is currently smoothing.
func (s *Smoother) IsSmoothing() bool {
	return s.smoothing.Load()
}

// Reset resets the smoother to a specific value. Control thread only; call it
// while the audio thread is stopped.
func (s *Smoother) Reset(value float64) {
	if !isFinite(value) {
		return
	}
	s.current.Store(math.Float64bits(value))
	s.target.Store(math.Float64bits(value))
	s.ackTarget = value
	s.step = 0
	s.logStep = 0
	s.smoothing.Store(false)
}

// SetRate updates the smoothing rate. It takes effect on the next ramp.
func (s *Smoother) SetRate(rate float64) {
	if !isFinite(rate) {
		return
	}
	s.rate.Store(math.Float64bits(rate))
}

// Rate returns the current smoothing rate.
func (s *Smoother) Rate() float64 {
	return s.loadRate()
}

// SetThreshold sets the threshold for considering smoothing complete.
func (s *Smoother) SetThreshold(threshold float64) {
	if !isFinite(threshold) {
		return
	}
	s.threshold.Store(math.Float64bits(threshold))
}

// SmoothedParameter is a convenience handle over a Parameter with smoothing
// enabled. The smoothing itself lives in the Parameter, so a plain
// *Parameter held by the registry, the state manager or the process context
// sees the same behavior.
type SmoothedParameter struct {
	*Parameter
	smoother *Smoother
}

// NewSmoothedParameter enables smoothing on param and returns a handle to it.
func NewSmoothedParameter(param *Parameter, smoothingType SmoothingType, rate float64) *SmoothedParameter {
	return &SmoothedParameter{
		Parameter: param,
		smoother:  param.EnableSmoothing(smoothingType, rate),
	}
}

// SetSmoothing enables or disables smoothing.
func (sp *SmoothedParameter) SetSmoothing(enabled bool) {
	if !enabled {
		sp.DisableSmoothing()
		return
	}
	sp.smoother.Reset(sp.GetPlain())
	sp.attachSmoother(sp.smoother)
}

// SetSmoothingRate updates the smoothing rate.
func (sp *SmoothedParameter) SetSmoothingRate(rate float64) {
	sp.smoother.SetRate(rate)
}

// UpdateSampleRate recomputes the smoothing rate for a time-based ramp length.
func (sp *SmoothedParameter) UpdateSampleRate(sampleRate, targetTimeMs float64) {
	samples := sampleRate * targetTimeMs / 1000.0

	switch sp.smoother.smoothingType {
	case LinearSmoothing, LogarithmicSmoothing:
		// Both ramp over a fixed number of samples.
		sp.SetSmoothingRate(samples)
	case ExponentialSmoothing:
		// Pole coefficient for -60 dB decay within targetTimeMs.
		sp.SetSmoothingRate(math.Exp(-6.908 / samples))
	}
}

// ParameterSmoother manages smoothing for multiple parameters.
type ParameterSmoother struct {
	smoothers map[uint32]*SmoothedParameter
	// order mirrors smoothers so AdvanceAll can iterate a slice instead of a
	// map on the audio thread.
	order []*SmoothedParameter
}

// NewParameterSmoother creates a new parameter smoother manager.
func NewParameterSmoother() *ParameterSmoother {
	return &ParameterSmoother{
		smoothers: make(map[uint32]*SmoothedParameter),
	}
}

// Add adds a parameter with smoothing.
func (ps *ParameterSmoother) Add(id uint32, param *Parameter, smoothingType SmoothingType, rate float64) {
	sp := NewSmoothedParameter(param, smoothingType, rate)
	if old, exists := ps.smoothers[id]; exists {
		for i, existing := range ps.order {
			if existing == old {
				ps.order[i] = sp
				break
			}
		}
	} else {
		ps.order = append(ps.order, sp)
	}
	ps.smoothers[id] = sp
}

// Value returns the current smoothed value for a parameter without advancing it.
func (ps *ParameterSmoother) Value(id uint32) float64 {
	if sp, ok := ps.smoothers[id]; ok {
		return sp.SmoothedValue()
	}
	return 0
}

// Advance advances a single parameter by one sample and returns its value.
func (ps *ParameterSmoother) Advance(id uint32) float64 {
	if sp, ok := ps.smoothers[id]; ok {
		return sp.Advance()
	}
	return 0
}

// AdvanceAll advances all smoothers by one sample (call once per sample).
func (ps *ParameterSmoother) AdvanceAll() {
	for _, sp := range ps.order {
		sp.Advance()
	}
}

// SetSmoothing enables/disables smoothing for a specific parameter.
func (ps *ParameterSmoother) SetSmoothing(id uint32, enabled bool) {
	if sp, ok := ps.smoothers[id]; ok {
		sp.SetSmoothing(enabled)
	}
}

// SetValue sets the value for a parameter (normalized 0-1).
func (ps *ParameterSmoother) SetValue(id uint32, value float64) {
	if sp, ok := ps.smoothers[id]; ok {
		sp.SetNormalized(value)
	}
}

// Get returns the SmoothedParameter for direct access.
func (ps *ParameterSmoother) Get(id uint32) (*SmoothedParameter, bool) {
	sp, ok := ps.smoothers[id]
	return sp, ok
}
