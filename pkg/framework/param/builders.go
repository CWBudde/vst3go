package param

import (
	"fmt"
	"strconv"
	"strings"
)

// ChoiceOption represents a single choice in a list parameter
type ChoiceOption struct {
	Value   float64
	Name    string
	Aliases []string
}

// Choice creates a parameter builder for a multiple choice parameter.
//
// Option values must be consecutive integers (0, 1, 2, ...): VST3 addresses a
// list parameter through StepCount intervals, so the plain range has to span
// exactly len(options)-1. TryBuild reports a violation.
func Choice(id uint32, name string, options []ChoiceOption) *Builder {
	if len(options) == 0 {
		// A choice without options has no valid detent; produce a harmless
		// constant parameter and surface the mistake through TryBuild.
		b := New(id, name).Formatter(func(float64) string { return "" }, nil)
		b.errs = append(b.errs, fmt.Errorf("%q: %w", name, ErrNoOptions))
		return b
	}

	// Create name list for formatter
	names := make([]string, len(options))
	for i, opt := range options {
		names[i] = opt.Name
	}

	// Create formatter
	formatter := func(value float64) string {
		for _, opt := range options {
			if opt.Value == value {
				return opt.Name
			}
		}
		// Fallback to index-based lookup for integer values
		index := int(value)
		if index >= 0 && index < len(names) {
			return names[index]
		}
		return "Unknown"
	}

	// Create parser
	parser := func(str string) (float64, error) {
		normalizedStr := strings.ToLower(strings.TrimSpace(str))

		// Check each option and its aliases
		for _, opt := range options {
			if strings.EqualFold(str, opt.Name) {
				return opt.Value, nil
			}
			for _, alias := range opt.Aliases {
				if strings.EqualFold(normalizedStr, strings.ToLower(alias)) {
					return opt.Value, nil
				}
			}
		}

		return 0, fmt.Errorf("%w: %s", ErrUnknownOption, str)
	}

	// StepCount counts intervals between detents, not detents themselves.
	return New(id, name).
		Range(options[0].Value, options[len(options)-1].Value).
		Steps(int32(len(options)-1)).
		Default(options[0].Value).
		AddFlags(IsList).
		Formatter(formatter, parser)
}

// Common parameter helpers

// GainParameter creates a standard gain parameter (-inf to +12dB)
func GainParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(-80, 12).
		Default(0).
		Unit("dB").
		Formatter(func(v float64) string {
			if v <= -80 {
				return displaySilence
			}
			return fmt.Sprintf("%.1f dB", v)
		}, DecibelParser) // "-∞" yields SilenceDB, which Normalize clamps to Min
}

// MixParameter creates a standard mix/blend parameter (0-100%)
func MixParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 100).
		Default(100).
		Unit("%").
		Formatter(PercentFormatter, PercentParser)
}

// FrequencyParameter creates a frequency parameter with Hz/kHz formatting.
// Use [Builder.Logarithmic] to set a logarithmic normalization curve.
func FrequencyParameter(id uint32, name string, min, max, defaultVal float64) *Builder {
	return New(id, name).
		Range(min, max).
		Default(defaultVal).
		Unit("Hz").
		Formatter(FrequencyFormatter, FrequencyParser)
}

// TimeParameter creates a time parameter (ms or s depending on range)
func TimeParameter(id uint32, name string, minMs, maxMs, defaultMs float64) *Builder {
	return New(id, name).
		Range(minMs, maxMs).
		Default(defaultMs).
		Unit("ms").
		Formatter(func(v float64) string {
			if v >= 1000 {
				return fmt.Sprintf("%.2f s", v/1000.0)
			}
			return fmt.Sprintf("%.1f ms", v)
		}, func(s string) (float64, error) {
			s = strings.TrimSpace(strings.ToLower(s))

			// Check for seconds
			if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ms") {
				s = strings.TrimSuffix(s, "s")
				s = strings.TrimSpace(s)
				val, err := parseFloat(s)
				if err != nil {
					return 0, err
				}
				return val * 1000.0, nil // Convert to ms
			}

			// Check for milliseconds
			s = strings.TrimSuffix(s, "ms")
			s = strings.TrimSpace(s)
			return parseFloat(s)
		})
}

// RatioParameter creates a compression/expansion ratio parameter
func RatioParameter(id uint32, name string, minRatio, maxRatio, defaultRatio float64) *Builder {
	return New(id, name).
		Range(minRatio, maxRatio).
		Default(defaultRatio).
		Formatter(func(v float64) string {
			if v >= 100 {
				return "∞:1"
			}
			return fmt.Sprintf("%.1f:1", v)
		}, func(s string) (float64, error) {
			s = strings.TrimSpace(strings.ToLower(s))

			// Handle infinity
			if strings.Contains(s, "inf") || strings.Contains(s, "∞") {
				return 100, nil // Use 100 as infinity
			}

			// Remove :1 suffix if present
			s = strings.TrimSuffix(s, ":1")
			s = strings.TrimSpace(s)

			return parseFloat(s)
		})
}

// QParameter creates a Q/resonance parameter
func QParameter(id uint32, name string, minQ, maxQ, defaultQ float64) *Builder {
	return New(id, name).
		Range(minQ, maxQ).
		Default(defaultQ).
		Formatter(func(v float64) string {
			return fmt.Sprintf("Q: %.2f", v)
		}, nil)
}

// PanParameter creates a stereo pan parameter on the package-wide pan
// convention: plain values run from -100 (hard left) to +100 (hard right).
func PanParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(-100, 100).
		Default(0).
		Formatter(PanFormatter, PanParser)
}

// PhaseParameter creates a phase parameter (0-360 degrees)
func PhaseParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 360).
		Default(0).
		Unit("°").
		Formatter(func(v float64) string {
			return fmt.Sprintf("%.1f°", v)
		}, func(s string) (float64, error) {
			s = strings.TrimSuffix(s, "°")
			s = strings.TrimSuffix(s, "deg")
			s = strings.TrimSuffix(s, "degrees")
			s = strings.TrimSpace(s)
			return parseFloat(s)
		})
}

// FeedbackParameter creates a standard feedback parameter (0-100%)
func FeedbackParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 100).
		Default(0).
		Unit("%").
		Formatter(PercentFormatter, PercentParser)
}

// ResonanceParameter creates a standard resonance parameter
func ResonanceParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 1).
		Default(0.707).
		Formatter(func(v float64) string {
			return fmt.Sprintf("%.3f", v)
		}, nil)
}

// DriveParameter creates a drive/saturation parameter (0-100%)
func DriveParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 100).
		Default(0).
		Unit("%").
		Formatter(PercentFormatter, PercentParser)
}

// OutputLevelMeter creates a read-only output level meter
func OutputLevelMeter(id uint32, name string) *Builder {
	return New(id, name).
		Range(-60, 0).
		Default(-60).
		Unit("dB").
		Formatter(DecibelFormatter, nil). // No parser for read-only
		ReadOnly()
}

// ThresholdParameter creates a threshold parameter (typically for dynamics)
func ThresholdParameter(id uint32, name string, minDB, maxDB, defaultDB float64) *Builder {
	return New(id, name).
		Range(minDB, maxDB).
		Default(defaultDB).
		Unit("dB").
		Formatter(DecibelFormatter, DecibelParser)
}

// AttackParameter creates an attack time parameter
func AttackParameter(id uint32, name string, maxMs float64) *Builder {
	return TimeParameter(id, name, 0.1, maxMs, 10.0)
}

// ReleaseParameter creates a release time parameter
func ReleaseParameter(id uint32, name string, maxMs float64) *Builder {
	return TimeParameter(id, name, 1.0, maxMs, 100.0)
}

// RateParameter creates a rate parameter (Hz) for LFOs, etc.
func RateParameter(id uint32, name string, minHz, maxHz, defaultHz float64) *Builder {
	return New(id, name).
		Range(minHz, maxHz).
		Default(defaultHz).
		Unit("Hz").
		Formatter(func(v float64) string {
			if v < 1.0 {
				return fmt.Sprintf("%.3f Hz", v)
			}
			return fmt.Sprintf("%.2f Hz", v)
		}, FrequencyParser)
}

// DepthParameter creates a depth/amount parameter (0-100%)
func DepthParameter(id uint32, name string) *Builder {
	return New(id, name).
		Range(0, 100).
		Default(50).
		Unit("%").
		Formatter(PercentFormatter, PercentParser)
}

// BypassParameter creates a bypass on/off switch. The IsBypass flag is what
// makes host-level and offline-render bypass work, so it must be set.
func BypassParameter(id uint32, name string) *Builder {
	return Choice(id, name, []ChoiceOption{
		{Value: 0, Name: "Active"},
		{Value: 1, Name: "Bypassed"},
	}).Bypass()
}

// parseFloat parses a complete numeric string. Unlike fmt.Sscanf it rejects
// trailing garbage such as "12notanumber".
func parseFloat(s string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidNumber, s)
	}
	return value, nil
}
