package param

import (
	"errors"
	"math"
	"testing"
)

func TestChoice(t *testing.T) {
	options := []ChoiceOption{
		{Value: 0, Name: "Off", Aliases: []string{"disabled", "none"}},
		{Value: 1, Name: "Low", Aliases: []string{"lo", "minimum"}},
		{Value: 2, Name: "Medium", Aliases: []string{"med", "mid", "normal"}},
		{Value: 3, Name: "High", Aliases: []string{"hi", "maximum"}},
	}

	param := Choice(100, "Mode", options).Build()

	t.Run("Formatter", func(t *testing.T) {
		tests := []struct {
			value    float64
			expected string
		}{
			{0, "Off"},
			{1, "Low"},
			{2, "Medium"},
			{3, "High"},
		}

		for _, test := range tests {
			// Need to normalize the value first
			normalized := test.value / 3.0 // 0-3 range
			result := param.FormatValue(normalized)
			if result != test.expected {
				t.Errorf("FormatValue(%f) = %s, want %s", test.value, result, test.expected)
			}
		}
	})

	t.Run("Parser", func(t *testing.T) {
		tests := []struct {
			input         string
			expectedPlain float64
		}{
			{"Off", 0},
			{"disabled", 0},
			{"Low", 1},
			{"lo", 1},
			{"Medium", 2},
			{"med", 2},
			{"High", 3},
			{"hi", 3},
		}

		for _, test := range tests {
			normalized, err := param.ParseValue(test.input)
			if err != nil {
				t.Errorf("ParseValue(%s) error: %v", test.input, err)
				continue
			}
			plain := param.Denormalize(normalized)
			if math.Abs(plain-test.expectedPlain) > 0.001 {
				t.Errorf("ParseValue(%s) = %f (plain), want %f", test.input, plain, test.expectedPlain)
			}
		}
	})
}

func TestGainParameter(t *testing.T) {
	param := GainParameter(200, "Output Gain").Build()

	t.Run("Formatter", func(t *testing.T) {
		tests := []struct {
			plainValue float64
			expected   string
		}{
			{-80, "-∞ dB"},
			{0, "0.0 dB"},
			{6, "6.0 dB"},
			{-6, "-6.0 dB"},
		}

		for _, test := range tests {
			// Format using normalized value
			normalized := param.Normalize(test.plainValue)
			result := param.FormatValue(normalized)
			if result != test.expected {
				t.Errorf("FormatValue(%f dB) = %s, want %s", test.plainValue, result, test.expected)
			}
		}
	})

	t.Run("Parser", func(t *testing.T) {
		normalized, err := param.ParseValue("-inf dB")
		if err != nil {
			t.Errorf("ParseValue error: %v", err)
		}
		plainValue := param.Denormalize(normalized)
		if plainValue != -80 {
			t.Errorf("ParseValue(-inf dB) = %f, want -80", plainValue)
		}
	})
}

func TestMixParameter(t *testing.T) {
	param := MixParameter(300, "Dry/Wet Mix").Build()

	if param.Min != 0 || param.Max != 100 {
		t.Errorf("Mix parameter range should be 0-100, got %f-%f", param.Min, param.Max)
	}

	if param.DefaultValue != 1.0 {
		t.Errorf("Mix parameter default should be 100%% (normalized 1.0), got %f", param.DefaultValue)
	}
}

func TestFrequencyParameter(t *testing.T) {
	param := FrequencyParameter(400, "Cutoff", 20, 20000, 1000).Build()

	// Format using normalized value
	normalized := param.Normalize(1000)
	result := param.FormatValue(normalized)
	if result != "1.00 kHz" {
		t.Errorf("FormatValue(1000 Hz) = %s, want 1.00 kHz", result)
	}
}

func TestTimeParameter(t *testing.T) {
	param := TimeParameter(500, "Attack", 0.1, 5000, 10).Build()

	t.Run("Formatter", func(t *testing.T) {
		tests := []struct {
			plainValue float64
			expected   string
		}{
			{10, "10.0 ms"},
			{100, "100.0 ms"},
			{1000, "1.00 s"},
			{2500, "2.50 s"},
		}

		for _, test := range tests {
			normalized := param.Normalize(test.plainValue)
			result := param.FormatValue(normalized)
			if result != test.expected {
				t.Errorf("FormatValue(%f ms) = %s, want %s (min=%f, max=%f)", test.plainValue, result, test.expected, param.Min, param.Max)
			}
		}
	})

	t.Run("Parser", func(t *testing.T) {
		tests := []struct {
			input         string
			expectedPlain float64
		}{
			{"10 ms", 10},
			{"10ms", 10},
			{"1 s", 1000},
			{"1s", 1000},
			{"2.5 s", 2500},
		}

		for _, test := range tests {
			normalized, err := param.ParseValue(test.input)
			if err != nil {
				t.Errorf("ParseValue(%s) error: %v", test.input, err)
				continue
			}
			plain := param.Denormalize(normalized)
			if math.Abs(plain-test.expectedPlain) > 0.1 {
				t.Errorf("ParseValue(%s) = %f ms (plain), want %f ms", test.input, plain, test.expectedPlain)
			}
		}
	})
}

func TestRatioParameter(t *testing.T) {
	param := RatioParameter(600, "Ratio", 1, 100, 4).Build()

	t.Run("Formatter", func(t *testing.T) {
		tests := []struct {
			plainValue float64
			expected   string
		}{
			{1, "1.0:1"},
			{4, "4.0:1"},
			{100, "∞:1"},
		}

		for _, test := range tests {
			normalized := param.Normalize(test.plainValue)
			result := param.FormatValue(normalized)
			if result != test.expected {
				t.Errorf("FormatValue(%f) = %s, want %s", test.plainValue, result, test.expected)
			}
		}
	})

	t.Run("Parser", func(t *testing.T) {
		tests := []struct {
			input         string
			expectedPlain float64
		}{
			{"4:1", 4},
			{"4", 4},
			{"inf:1", 100},
			{"∞:1", 100},
		}

		for _, test := range tests {
			normalized, err := param.ParseValue(test.input)
			if err != nil {
				t.Errorf("ParseValue(%s) error: %v", test.input, err)
				continue
			}
			plain := param.Denormalize(normalized)
			if math.Abs(plain-test.expectedPlain) > 0.1 {
				t.Errorf("ParseValue(%s) = %f, want %f", test.input, plain, test.expectedPlain)
			}
		}
	})
}

func TestPanParameter(t *testing.T) {
	param := PanParameter(700, "Pan").Build()

	t.Run("Formatter", func(t *testing.T) {
		tests := []struct {
			plainValue float64
			expected   string
		}{
			{0, "Center"},
			{-50, "50% L"},
			{50, "50% R"},
			{-100, "100% L"},
			{100, "100% R"},
		}

		for _, test := range tests {
			normalized := param.Normalize(test.plainValue)
			result := param.FormatValue(normalized)
			if result != test.expected {
				t.Errorf("FormatValue(%f) = %s, want %s", test.plainValue, result, test.expected)
			}
		}
	})

	t.Run("Parser", func(t *testing.T) {
		tests := []struct {
			input         string
			expectedPlain float64
		}{
			{"center", 0},
			{"c", 0},
			{"50 l", -50},
			{"50% left", -50},
			{"50 r", 50},
			{"50% right", 50},
			{"75", 75},
		}

		for _, test := range tests {
			normalized, err := param.ParseValue(test.input)
			if err != nil {
				t.Errorf("ParseValue(%s) error: %v", test.input, err)
				continue
			}
			plain := param.Denormalize(normalized)
			if math.Abs(plain-test.expectedPlain) > 0.1 {
				t.Errorf("ParseValue(%s) = %f, want %f", test.input, plain, test.expectedPlain)
			}
		}
	})
}

func TestChoiceStepCount(t *testing.T) {
	tests := []struct {
		name    string
		options []ChoiceOption
	}{
		{"two", []ChoiceOption{{Value: 0, Name: "A"}, {Value: 1, Name: "B"}}},
		{"three", []ChoiceOption{{Value: 0, Name: "A"}, {Value: 1, Name: "B"}, {Value: 2, Name: "C"}}},
		{"six", []ChoiceOption{
			{Value: 0, Name: "Bell"}, {Value: 1, Name: "Low Shelf"}, {Value: 2, Name: "High Shelf"},
			{Value: 3, Name: "Low Pass"}, {Value: 4, Name: "High Pass"}, {Value: 5, Name: "Notch"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Choice(1, "Mode", tt.options).TryBuild()
			if err != nil {
				t.Fatalf("TryBuild: %v", err)
			}

			// StepCount counts intervals, so N options means N-1 steps.
			if want := int32(len(tt.options) - 1); p.StepCount != want {
				t.Errorf("StepCount = %d, want %d", p.StepCount, want)
			}
			if p.Flags&IsList == 0 {
				t.Error("choice parameter must carry the IsList flag")
			}

			// Every detent must resolve to its own label.
			for i, opt := range tt.options {
				normalized := float64(i) / float64(len(tt.options)-1)
				if got := p.FormatValue(normalized); got != opt.Name {
					t.Errorf("detent %d: FormatValue(%v) = %q, want %q", i, normalized, got, opt.Name)
				}
			}
		})
	}
}

func TestChoiceWithoutOptions(t *testing.T) {
	// Must not panic; the mistake surfaces as an error instead.
	b := Choice(1, "Empty", nil)
	if _, err := b.TryBuild(); !errors.Is(err, ErrNoOptions) {
		t.Errorf("TryBuild() error = %v, want ErrNoOptions", err)
	}
	if p := b.Build(); p == nil {
		t.Error("Build() should still return a parameter")
	}
}

func TestBypassParameter(t *testing.T) {
	p, err := BypassParameter(1, "Bypass").TryBuild()
	if err != nil {
		t.Fatalf("TryBuild: %v", err)
	}

	if p.StepCount != 1 {
		t.Errorf("StepCount = %d, want 1", p.StepCount)
	}
	if p.Flags&IsBypass == 0 {
		t.Error("bypass parameter must carry the IsBypass flag")
	}
	if p.Flags&IsList == 0 {
		t.Error("bypass parameter must carry the IsList flag")
	}
	if p.Flags&CanAutomate == 0 {
		t.Error("bypass parameter must stay automatable")
	}

	// Both detents, and nothing in between.
	if got := p.FormatValue(0); got != "Active" {
		t.Errorf("FormatValue(0) = %q, want Active", got)
	}
	if got := p.FormatValue(1); got != "Bypassed" {
		t.Errorf("FormatValue(1) = %q, want Bypassed", got)
	}
	if got := p.Denormalize(0.9); got != 1 {
		t.Errorf("Denormalize(0.9) = %v, want 1", got)
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"12", 12, false},
		{" 12.5 ", 12.5, false},
		{"-3e2", -300, false},
		{"12notanumber", 0, true},
		{"12 34", 0, true},
		{"", 0, true},
		{"abc", 0, true},
		{"1.2.3", 0, true},
	}

	for _, tt := range tests {
		got, err := parseFloat(tt.input)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidNumber) {
				t.Errorf("parseFloat(%q) error = %v, want ErrInvalidNumber", tt.input, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseFloat(%q) unexpected error: %v", tt.input, err)
		} else if got != tt.want {
			t.Errorf("parseFloat(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNoteFormatterIsTotal(t *testing.T) {
	tests := []struct {
		note float64
		want string
	}{
		{0, "C-1"},
		{60, "C4"},
		{69, "A4"},
		{-1, "B-2"},
		{-12, "C-2"},
		{-13, "B-3"},
		{127, "G9"},
		{math.NaN(), "Unknown"},
		{math.Inf(-1), "Unknown"},
	}

	for _, tt := range tests {
		if got := NoteFormatter(tt.note); got != tt.want {
			t.Errorf("NoteFormatter(%v) = %q, want %q", tt.note, got, tt.want)
		}
	}
}

func TestPanConvention(t *testing.T) {
	// PanFormatter/PanParser and PanParameter share the -100..100 convention.
	p := PanParameter(1, "Pan").Build()

	tests := []struct {
		plain float64
		text  string
	}{
		{0, "Center"},
		{-50, "50% L"},
		{50, "50% R"},
		{-100, "100% L"},
		{100, "100% R"},
	}

	for _, tt := range tests {
		if got := PanFormatter(tt.plain); got != tt.text {
			t.Errorf("PanFormatter(%v) = %q, want %q", tt.plain, got, tt.text)
		}
		if got := p.FormatValue(p.Normalize(tt.plain)); got != tt.text {
			t.Errorf("PanParameter format(%v) = %q, want %q", tt.plain, got, tt.text)
		}

		got, err := PanParser(tt.text)
		if err != nil {
			t.Errorf("PanParser(%q): %v", tt.text, err)
			continue
		}
		if got != tt.plain {
			t.Errorf("PanParser(%q) = %v, want %v", tt.text, got, tt.plain)
		}
	}
}

func TestDecibelSilenceConvention(t *testing.T) {
	// One "-∞" convention: DecibelParser yields SilenceDB, which each
	// parameter clamps to its own minimum.
	got, err := DecibelParser("-∞ dB")
	if err != nil {
		t.Fatalf("DecibelParser: %v", err)
	}
	if got != SilenceDB {
		t.Errorf("DecibelParser(-∞) = %v, want %v", got, SilenceDB)
	}

	gain := GainParameter(1, "Gain").Build()
	normalized, err := gain.ParseValue("-inf dB")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if plain := gain.Denormalize(normalized); plain != gain.Min {
		t.Errorf("gain -inf = %v, want %v", plain, gain.Min)
	}
}
