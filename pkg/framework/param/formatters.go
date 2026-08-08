package param

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Common parameter formatters and parsers

// Display strings shared by the formatters in this package and by the
// pre-built parameter sets in builders.go and registry_auto.go.
const (
	displaySilence   = "-∞ dB"
	displayPanCenter = "Center"
	displayOff       = "Off"
	displayOn        = "On"
)

// FrequencyFormatter formats frequency values with Hz/kHz
func FrequencyFormatter(hz float64) string {
	if hz >= 1000 {
		return fmt.Sprintf("%.2f kHz", hz/1000)
	}
	return fmt.Sprintf("%.1f Hz", hz)
}

// FrequencyParser parses frequency strings
func FrequencyParser(str string) (float64, error) {
	str = strings.TrimSpace(str)

	// Handle kHz
	if strings.HasSuffix(str, "kHz") || strings.HasSuffix(str, "khz") {
		numStr := strings.TrimSuffix(strings.TrimSuffix(str, "kHz"), "khz")
		numStr = strings.TrimSpace(numStr)
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, err
		}
		return val * 1000, nil
	}

	// Handle Hz
	str = strings.TrimSuffix(strings.TrimSuffix(str, "Hz"), "hz")
	str = strings.TrimSpace(str)
	return strconv.ParseFloat(str, 64)
}

// SilenceDB is the plain value this package uses for "-∞ dB". It is below any
// realistic parameter minimum, so Parameter.Normalize clamps it to the
// parameter's own floor.
const SilenceDB = -96.0

// DecibelDisplayFloor is the level at or below which DecibelFormatter prints
// "-∞ dB". Parameters with a lower minimum supply their own formatter.
const DecibelDisplayFloor = -60.0

// DecibelFormatter formats dB values
func DecibelFormatter(db float64) string {
	if db <= DecibelDisplayFloor {
		return displaySilence
	}
	return fmt.Sprintf("%.1f dB", db)
}

// DecibelParser parses dB strings
func DecibelParser(str string) (float64, error) {
	if strings.Contains(str, "∞") || strings.Contains(strings.ToLower(str), "inf") {
		return SilenceDB, nil
	}
	str = strings.TrimSuffix(strings.TrimSpace(str), "dB")
	str = strings.TrimSuffix(strings.TrimSpace(str), "db")
	return strconv.ParseFloat(strings.TrimSpace(str), 64)
}

// PercentFormatter formats percentage values
func PercentFormatter(value float64) string {
	return fmt.Sprintf("%.0f%%", value)
}

// PercentParser parses percentage strings
func PercentParser(str string) (float64, error) {
	str = strings.TrimSuffix(strings.TrimSpace(str), "%")
	return strconv.ParseFloat(str, 64)
}

// TimeFormatter formats time values with appropriate units
func TimeFormatter(ms float64) string {
	if ms < 1 {
		return fmt.Sprintf("%.2f µs", ms*1000)
	} else if ms < 1000 {
		return fmt.Sprintf("%.1f ms", ms)
	}
	return fmt.Sprintf("%.2f s", ms/1000)
}

// TimeParser parses time strings
func TimeParser(str string) (float64, error) {
	str = strings.TrimSpace(str)

	// Handle microseconds
	if strings.HasSuffix(str, "µs") || strings.HasSuffix(str, "us") {
		numStr := strings.TrimSuffix(strings.TrimSuffix(str, "µs"), "us")
		val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return val / 1000, nil // Convert to ms
	}

	// Handle seconds
	if strings.HasSuffix(str, "s") && !strings.HasSuffix(str, "ms") {
		numStr := strings.TrimSuffix(str, "s")
		val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return val * 1000, nil // Convert to ms
	}

	// Handle milliseconds (default)
	str = strings.TrimSuffix(str, "ms")
	return strconv.ParseFloat(strings.TrimSpace(str), 64)
}

// RatioFormatter formats ratio values
func RatioFormatter(value float64) string {
	return fmt.Sprintf("%.1f:1", value)
}

// RatioParser parses ratio strings
func RatioParser(str string) (float64, error) {
	str = strings.TrimSpace(str)
	str = strings.TrimSuffix(str, ":1")
	return strconv.ParseFloat(strings.TrimSpace(str), 64)
}

// PanFormatter formats a pan position on the package-wide pan convention:
// plain values run from -100 (hard left) through 0 (center) to +100 (hard right).
func PanFormatter(pan float64) string {
	switch {
	case math.Abs(pan) < 0.5:
		return displayPanCenter
	case pan < 0:
		return fmt.Sprintf("%.0f%% L", -pan)
	default:
		return fmt.Sprintf("%.0f%% R", pan)
	}
}

// PanParser parses pan position strings into the -100..100 pan convention.
// It accepts "Center"/"C", "50L"/"50% left" and plain numbers.
func PanParser(str string) (float64, error) {
	str = strings.ToLower(strings.TrimSpace(str))

	if str == "c" || str == "center" {
		return 0, nil
	}

	sign := 1.0
	switch {
	case strings.HasSuffix(str, "left"):
		sign, str = -1, strings.TrimSuffix(str, "left")
	case strings.HasSuffix(str, "l"):
		sign, str = -1, strings.TrimSuffix(str, "l")
	case strings.HasSuffix(str, "right"):
		str = strings.TrimSuffix(str, "right")
	case strings.HasSuffix(str, "r"):
		str = strings.TrimSuffix(str, "r")
	}

	str = strings.TrimSuffix(strings.TrimSpace(str), "%")
	val, err := parseFloat(str)
	if err != nil {
		return 0, err
	}
	return sign * val, nil
}

// noteNames indexes the twelve pitch classes starting at C.
var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// NoteFormatter formats MIDI note numbers. The host may denormalize to any
// value, including negatives, so the pitch class uses a floored modulo.
func NoteFormatter(noteNumber float64) string {
	if math.IsNaN(noteNumber) || math.IsInf(noteNumber, 0) {
		return unknownValue
	}

	n := int(math.Floor(noteNumber))
	pitchClass := ((n % 12) + 12) % 12
	octave := int(math.Floor(float64(n)/12)) - 1
	return fmt.Sprintf("%s%d", noteNames[pitchClass], octave)
}

// NoteParser parses note names to MIDI numbers
func NoteParser(str string) (float64, error) {
	str = strings.ToUpper(strings.TrimSpace(str))

	noteMap := map[string]int{
		"C": 0, "B#": 0,
		"C#": 1, "DB": 1,
		"D":  2,
		"D#": 3, "EB": 3,
		"E": 4, "FB": 4,
		"F": 5, "E#": 5,
		"F#": 6, "GB": 6,
		"G":  7,
		"G#": 8, "AB": 8,
		"A":  9,
		"A#": 10, "BB": 10,
		"B": 11, "CB": 11,
	}

	// Find where the octave number starts
	octaveStart := -1
	for i, ch := range str {
		if ch >= '0' && ch <= '9' || ch == '-' {
			octaveStart = i
			break
		}
	}

	if octaveStart == -1 {
		return 0, fmt.Errorf("%w: no octave number in %q", ErrInvalidNote, str)
	}

	noteName := str[:octaveStart]
	octaveStr := str[octaveStart:]

	noteOffset, ok := noteMap[noteName]
	if !ok {
		return 0, fmt.Errorf("%w: unknown note name %q", ErrInvalidNote, noteName)
	}

	octave, err := strconv.Atoi(octaveStr)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid octave %q", ErrInvalidNote, octaveStr)
	}

	return float64((octave+1)*12 + noteOffset), nil
}

// OnOffFormatter formats boolean as On/Off
func OnOffFormatter(value float64) string {
	if value > 0.5 {
		return displayOn
	}
	return displayOff
}

// OnOffParser parses On/Off strings
func OnOffParser(str string) (float64, error) {
	str = strings.ToLower(strings.TrimSpace(str))
	switch str {
	case "on", "yes", "true", "1":
		return 1, nil
	case "off", "no", "false", "0":
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidOnOff, str)
	}
}
