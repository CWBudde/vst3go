package midi

import (
	"math"
	"testing"
)

func TestNoteOnEvent(t *testing.T) {
	event := NoteOnEvent{
		BaseEvent: BaseEvent{
			EventChannel: 0,
			Offset:       100,
		},
		NoteNumber: 60, // Middle C
		Velocity:   64,
	}

	if event.Type() != EventTypeNoteOn {
		t.Errorf("Expected type %v, got %v", EventTypeNoteOn, event.Type())
	}

	if event.Channel() != 0 {
		t.Errorf("Expected channel 0, got %d", event.Channel())
	}

	if event.SampleOffset() != 100 {
		t.Errorf("Expected offset 100, got %d", event.SampleOffset())
	}

	expected := "NoteOn{ch:0, note:60, vel:64, offset:100}"
	if event.String() != expected {
		t.Errorf("Expected string %s, got %s", expected, event.String())
	}
}

func TestNoteOffEvent(t *testing.T) {
	event := NoteOffEvent{
		BaseEvent: BaseEvent{
			EventChannel: 1,
			Offset:       200,
		},
		NoteNumber: 72, // C5
		Velocity:   0,
	}

	if event.Type() != EventTypeNoteOff {
		t.Errorf("Expected type %v, got %v", EventTypeNoteOff, event.Type())
	}

	if event.Channel() != 1 {
		t.Errorf("Expected channel 1, got %d", event.Channel())
	}
}

func TestControlChangeEvent(t *testing.T) {
	event := ControlChangeEvent{
		BaseEvent: BaseEvent{
			EventChannel: 0,
			Offset:       50,
		},
		Controller: CCModWheel,
		Value:      100,
	}

	if event.Type() != EventTypeControlChange {
		t.Errorf("Expected type %v, got %v", EventTypeControlChange, event.Type())
	}

	expected := "CC{ch:0, ctrl:1, val:100, offset:50}"
	if event.String() != expected {
		t.Errorf("Expected string %s, got %s", expected, event.String())
	}
}

func TestPitchBendEvent(t *testing.T) {
	tests := []struct {
		value      int16
		normalized float64
	}{
		{0, 0.0},
		{8191, 0.999878}, // Close to 1.0
		{-8192, -1.0},
		{4096, 0.5},
		{-4096, -0.5},
	}

	for _, tt := range tests {
		event := PitchBendEvent{
			BaseEvent: BaseEvent{
				EventChannel: 0,
				Offset:       0,
			},
			Value: tt.value,
		}

		normalized := event.NormalizedValue()
		if diff := normalized - tt.normalized; diff > 0.01 && diff < -0.01 {
			t.Errorf("For value %d, expected normalized %f, got %f", tt.value, tt.normalized, normalized)
		}
	}
}

func TestNoteToFrequency(t *testing.T) {
	tests := []struct {
		note uint8
		freq float64
	}{
		{69, 440.0},  // A4
		{60, 261.63}, // Middle C (C4)
		{57, 220.0},  // A3
		{81, 880.0},  // A5
	}

	for _, tt := range tests {
		freq := NoteToFrequency(tt.note, 440.0)
		if diff := freq - tt.freq; diff > 0.1 && diff < -0.1 {
			t.Errorf("For note %d, expected frequency %f, got %f", tt.note, tt.freq, freq)
		}
	}
}

func TestNoteNumberToName(t *testing.T) {
	tests := []struct {
		note uint8
		name string
	}{
		{60, "C4"},  // Middle C
		{69, "A4"},  // A440
		{0, "C-1"},  // Lowest MIDI note
		{127, "G9"}, // Highest MIDI note
		{61, "C#4"}, // C# above middle C
		{70, "A#4"}, // A# above A4
	}

	for _, tt := range tests {
		name := NoteNumberToName(tt.note)
		if name != tt.name {
			t.Errorf("For note %d, expected name %s, got %s", tt.note, tt.name, name)
		}
	}
}

func TestEventInterface(t *testing.T) {
	events := []Event{
		NoteOnEvent{BaseEvent: BaseEvent{EventChannel: 0, Offset: 0}, NoteNumber: 60, Velocity: 100},
		NoteOffEvent{BaseEvent: BaseEvent{EventChannel: 0, Offset: 100}, NoteNumber: 60, Velocity: 0},
		ControlChangeEvent{BaseEvent: BaseEvent{EventChannel: 0, Offset: 200}, Controller: CCSustain, Value: 127},
		PitchBendEvent{BaseEvent: BaseEvent{EventChannel: 0, Offset: 300}, Value: 0},
	}

	wantOffsets := []int32{0, 100, 200, 300}
	for i, event := range events {
		// Ensure all events implement the interface and report their own data
		if event.SampleOffset() != wantOffsets[i] {
			t.Errorf("event %d offset = %d, want %d", i, event.SampleOffset(), wantOffsets[i])
		}
		if event.Channel() != 0 {
			t.Errorf("event %d channel = %d, want 0", i, event.Channel())
		}
		if event.String() == "" {
			t.Errorf("event %d has an empty String()", i)
		}
		_ = event.Type()
	}
}

func TestNoteToFrequencyTuning(t *testing.T) {
	tests := []struct {
		name     string
		note     uint8
		tuningA4 float64
		want     float64
		tol      float64
	}{
		// A4 is the tuning reference and must come out bit-exact.
		{"A4 is exactly the reference", 69, 440.0, 440.0, 0},
		{"A4 with zero tuning defaults to 440", 69, 0, 440.0, 0},
		{"A4 with 432 tuning", 69, 432.0, 432.0, 0},
		{"middle C", 60, 440.0, 261.6255653005986, 1e-9},
		{"A3", 57, 440.0, 220.0, 1e-9},
		{"A5", 81, 440.0, 880.0, 1e-9},
		{"lowest note", 0, 440.0, 8.175798915643707, 1e-9},
		{"highest note", 127, 440.0, 12543.853951415975, 1e-9},
		// The old cubic approximation was worst here, off by -7.13 cents.
		{"note 104", 104, 440.0, 3322.437580639561, 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NoteToFrequency(tt.note, tt.tuningA4)
			if diff := math.Abs(got - tt.want); diff > tt.tol {
				t.Errorf("NoteToFrequency(%d, %v) = %.10f, want %.10f (diff %.3g)",
					tt.note, tt.tuningA4, got, tt.want, diff)
			}
		})
	}
}

// Every note must survive note -> frequency -> note. The old approximations
// drifted by several cents, which is audible and could round to a wrong note.
func TestNoteFrequencyRoundTrip(t *testing.T) {
	for _, tuning := range []float64{440.0, 432.0, 415.0} {
		for note := 0; note <= 127; note++ {
			freq := NoteToFrequency(uint8(note), tuning)
			if got := FrequencyToNote(freq, tuning); got != uint8(note) {
				t.Errorf("tuning %.0f: note %d -> %.6f Hz -> note %d", tuning, note, freq, got)
			}

			// Detuning by a quarter tone either way must still round to the same note.
			const quarterTone = 1.0072464122237039 // 2^(1/24)
			if got := FrequencyToNote(freq*quarterTone*0.999, tuning); note < 127 && got != uint8(note) {
				t.Errorf("tuning %.0f: note %d sharpened by ~49 cents resolved to %d", tuning, note, got)
			}
			if got := FrequencyToNote(freq/quarterTone*1.001, tuning); note > 0 && got != uint8(note) {
				t.Errorf("tuning %.0f: note %d flattened by ~49 cents resolved to %d", tuning, note, got)
			}
		}
	}
}

func TestFrequencyToNoteClamps(t *testing.T) {
	tests := []struct {
		name string
		freq float64
		want uint8
	}{
		{"zero frequency", 0, 0},
		{"negative frequency", -100, 0},
		{"NaN", math.NaN(), 0},
		{"far below range", 0.001, 0},
		{"far above range", 1e9, 127},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FrequencyToNote(tt.freq, 440.0); got != tt.want {
				t.Errorf("FrequencyToNote(%v) = %d, want %d", tt.freq, got, tt.want)
			}
		})
	}
}

func TestNoteFrequencyConversionDoesNotAllocate(t *testing.T) {
	var sink float64
	var noteSink uint8

	sink = NoteToFrequency(69, 440.0) // warm up
	noteSink = FrequencyToNote(sink, 440.0)

	allocs := testing.AllocsPerRun(100, func() {
		sink = NoteToFrequency(noteSink, 440.0)
	})
	if allocs != 0 {
		t.Fatalf("NoteToFrequency allocated %f times, want 0", allocs)
	}

	allocs = testing.AllocsPerRun(100, func() {
		noteSink = FrequencyToNote(sink, 440.0)
	})
	if allocs != 0 {
		t.Fatalf("FrequencyToNote allocated %f times, want 0", allocs)
	}
}
