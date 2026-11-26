package param

import (
	"errors"
	"fmt"
	"math"
)

// Builder provides a fluent API for creating parameters.
//
// The default value is remembered in plain form and only converted to
// normalized form in Build, so Default and Range may be called in any order.
type Builder struct {
	param      *Parameter
	rawDefault float64
	errs       []error
}

// New creates a new parameter builder
func New(id uint32, name string) *Builder {
	return &Builder{
		param: &Parameter{
			ID:        id,
			Name:      name,
			ShortName: name,
			Min:       0,
			Max:       1,
			Flags:     CanAutomate,
		},
	}
}

// ShortName sets the short name
func (b *Builder) ShortName(name string) *Builder {
	b.param.ShortName = name
	return b
}

// Range sets the min and max values
func (b *Builder) Range(minVal, maxVal float64) *Builder {
	b.param.Min = minVal
	b.param.Max = maxVal
	return b
}

// Logarithmic sets a continuous parameter's normalization to
// log1p(factor * unit) / log1p(factor), with unit the plain value scaled to
// 0–1. The factor and plain range span must be positive and finite.
func (b *Builder) Logarithmic(factor float64) *Builder {
	if !isFinite(factor) || factor <= 0 {
		b.errs = append(b.errs, fmt.Errorf("factor %v: %w", factor, ErrInvalidCurveFactor))
		return b
	}
	b.param.logarithmicFactor = factor
	b.param.logarithmicScale = math.Log1p(factor)
	return b
}

// Default sets the default value (in plain range, not normalized)
func (b *Builder) Default(value float64) *Builder {
	if !isFinite(value) {
		b.errs = append(b.errs, fmt.Errorf("default value %v: %w", value, ErrInvalidNumber))
		return b
	}
	b.rawDefault = value
	return b
}

// Unit sets the unit string
func (b *Builder) Unit(unit string) *Builder {
	b.param.Unit = unit
	return b
}

// Steps sets the number of discrete steps (intervals, so an N-way list uses N-1)
func (b *Builder) Steps(count int32) *Builder {
	if count < 0 {
		b.errs = append(b.errs, fmt.Errorf("step count %d: %w", count, ErrInvalidStepCount))
		return b
	}
	b.param.StepCount = count
	return b
}

// Flags replaces the parameter flags. Use AddFlags to combine with the
// flags already set by ReadOnly, Bypass or Hidden.
func (b *Builder) Flags(flags uint32) *Builder {
	b.param.Flags = flags
	return b
}

// AddFlags adds flags to the existing set.
func (b *Builder) AddFlags(flags uint32) *Builder {
	b.param.Flags |= flags
	return b
}

// RemoveFlags clears flags from the existing set.
func (b *Builder) RemoveFlags(flags uint32) *Builder {
	b.param.Flags &^= flags
	return b
}

// Toggle turns the parameter into an on/off switch. It keeps a default set
// by an earlier or later Default call.
func (b *Builder) Toggle() *Builder {
	b.param.Min = 0
	b.param.Max = 1
	b.param.StepCount = 1
	return b
}

// ReadOnly marks the parameter as read-only
func (b *Builder) ReadOnly() *Builder {
	return b.AddFlags(IsReadOnly).RemoveFlags(CanAutomate)
}

// Hidden marks the parameter as hidden
func (b *Builder) Hidden() *Builder {
	return b.AddFlags(IsHidden)
}

// Bypass marks this as the bypass parameter. VST3 requires the bypass
// parameter to be a stepped list, so IsList and a single step are implied.
func (b *Builder) Bypass() *Builder {
	if b.param.StepCount == 0 {
		b.param.Min, b.param.Max, b.param.StepCount = 0, 1, 1
	}
	return b.AddFlags(IsBypass | IsList)
}

// Formatter sets custom value formatting and parsing
func (b *Builder) Formatter(format func(float64) string, parse func(string) (float64, error)) *Builder {
	b.param.formatFunc = format
	b.param.parseFunc = parse
	return b
}

// Err reports the first configuration problem detected so far, if any.
func (b *Builder) Err() error {
	return errors.Join(b.errs...)
}

// validate checks the assembled parameter for internal consistency.
func (b *Builder) validate() error {
	errs := append([]error(nil), b.errs...)
	p := b.param

	if p.Name == "" {
		errs = append(errs, ErrMissingName)
	}
	if p.Max <= p.Min {
		errs = append(errs, fmt.Errorf("%q: min %v, max %v: %w", p.Name, p.Min, p.Max, ErrInvalidRange))
	}
	// VST3 stepped parameters map plain = Min + round(normalized*StepCount),
	// which only covers the full range when the range spans exactly StepCount.
	if p.StepCount > 0 && p.Max-p.Min != float64(p.StepCount) {
		errs = append(errs, fmt.Errorf("%q: range %v..%v does not span %d steps: %w",
			p.Name, p.Min, p.Max, p.StepCount, ErrInvalidStepCount))
	}
	if p.logarithmicFactor > 0 {
		if p.StepCount > 0 {
			errs = append(errs, fmt.Errorf("%q: logarithmic normalization requires a continuous parameter: %w",
				p.Name, ErrInvalidStepCount))
		}
		if !isFinite(p.Max - p.Min) {
			errs = append(errs, fmt.Errorf("%q: logarithmic normalization requires a finite range span: %w",
				p.Name, ErrInvalidRange))
		}
	}

	return errors.Join(errs...)
}

// Build returns the configured parameter, ignoring validation problems.
// Use TryBuild when the configuration is not statically known to be valid.
func (b *Builder) Build() *Parameter {
	p, _ := b.TryBuild()
	return p
}

// TryBuild returns the configured parameter together with any validation
// error. The parameter is returned even when invalid so callers can report
// and continue.
func (b *Builder) TryBuild() (*Parameter, error) {
	b.param.DefaultValue = b.param.Normalize(b.rawDefault)
	b.param.SetNormalized(b.param.DefaultValue)
	return b.param, b.validate()
}
