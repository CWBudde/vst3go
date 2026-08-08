package midi

import (
	"math"
	"testing"
)

func TestNoteOnEvent(t *testing.T) {
	event := NewNoteOn(0, 60, 64, 100) // Middle C

	if event.Type != EventTypeNoteOn {
		t.Errorf("Expected type %v, got %v", EventTypeNoteOn, event.Type)
	}

	if event.Channel != 0 {
		t.Errorf("Expected channel 0, got %d", event.Channel)
	}

	if event.Offset != 100 {
		t.Errorf("Expected offset 100, got %d", event.Offset)
	}

	if event.Note() != 60 {
		t.Errorf("Expected note 60, got %d", event.Note())
	}

	if event.Velocity() != 64 {
		t.Errorf("Expected velocity 64, got %d", event.Velocity())
	}

	expected := "NoteOn{ch:0, note:60, vel:64, offset:100}"
	if event.String() != expected {
		t.Errorf("Expected string %s, got %s", expected, event.String())
	}
}

func TestNoteOffEvent(t *testing.T) {
	event := NewNoteOff(1, 72, 0, 200) // C5

	if event.Type != EventTypeNoteOff {
		t.Errorf("Expected type %v, got %v", EventTypeNoteOff, event.Type)
	}

	if event.Channel != 1 {
		t.Errorf("Expected channel 1, got %d", event.Channel)
	}
}

func TestControlChangeEvent(t *testing.T) {
	event := NewControlChange(0, CCModWheel, 100, 50)

	if event.Type != EventTypeControlChange {
		t.Errorf("Expected type %v, got %v", EventTypeControlChange, event.Type)
	}

	if event.Controller() != CCModWheel {
		t.Errorf("Expected controller %d, got %d", CCModWheel, event.Controller())
	}

	if event.Value() != 100 {
		t.Errorf("Expected value 100, got %d", event.Value())
	}

	expected := "ControlChange{ch:0, ctrl:1, val:100, offset:50}"
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
		event := NewPitchBend(0, tt.value, 0)

		normalized := event.NormalizedBend()
		if diff := math.Abs(normalized - tt.normalized); diff > 0.01 {
			t.Errorf("For value %d, expected normalized %f, got %f", tt.value, tt.normalized, normalized)
		}
	}
}

// TestEventAccessorsAreTypeGuarded pins the one hazard of a union payload: a
// field only means what its name says for the matching Type. The accessors must
// report zero rather than reinterpreting another message's bytes.
func TestEventAccessorsAreTypeGuarded(t *testing.T) {
	cc := NewControlChange(3, CCVolume, 90, 7)

	if got := cc.Note(); got != 0 {
		t.Errorf("Note() on a control change = %d, want 0", got)
	}
	if got := cc.Velocity(); got != 0 {
		t.Errorf("Velocity() on a control change = %d, want 0", got)
	}
	if got := cc.Program(); got != 0 {
		t.Errorf("Program() on a control change = %d, want 0", got)
	}
	if got := cc.Pressure(); got != 0 {
		t.Errorf("Pressure() on a control change = %d, want 0", got)
	}

	noteOn := NewNoteOn(0, 64, 100, 0)
	if got := noteOn.Controller(); got != 0 {
		t.Errorf("Controller() on a note on = %d, want 0", got)
	}
	if got := noteOn.Value(); got != 0 {
		t.Errorf("Value() on a note on = %d, want 0", got)
	}
}

func TestEventConstructors(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		wantType EventType
		wantStr  string
		check    func(*testing.T, Event)
	}{
		{
			name:     "poly pressure",
			event:    NewPolyPressure(2, 60, 80, 16),
			wantType: EventTypePolyPressure,
			wantStr:  "PolyPressure{ch:2, note:60, pressure:80, offset:16}",
			check: func(t *testing.T, e Event) {
				if e.Note() != 60 || e.Pressure() != 80 {
					t.Errorf("note/pressure = %d/%d, want 60/80", e.Note(), e.Pressure())
				}
			},
		},
		{
			name:     "channel pressure",
			event:    NewChannelPressure(1, 45, 32),
			wantType: EventTypeChannelPressure,
			wantStr:  "ChannelPressure{ch:1, pressure:45, offset:32}",
			check: func(t *testing.T, e Event) {
				if e.Pressure() != 45 {
					t.Errorf("pressure = %d, want 45", e.Pressure())
				}
			},
		},
		{
			name:     "program change",
			event:    NewProgramChange(0, 12, 64),
			wantType: EventTypeProgramChange,
			wantStr:  "ProgramChange{ch:0, prog:12, offset:64}",
			check: func(t *testing.T, e Event) {
				if e.Program() != 12 {
					t.Errorf("program = %d, want 12", e.Program())
				}
			},
		},
		{
			name:     "sysex retains its payload",
			event:    NewSysEx([]byte{0x7e, 0x00, 0x06}, 8),
			wantType: EventTypeSystemExclusive,
			wantStr:  "SysEx{bytes:3, offset:8}",
			check: func(t *testing.T, e Event) {
				if len(e.SysEx) != 3 || e.SysEx[0] != 0x7e {
					t.Errorf("SysEx = %v, want [126 0 6]", e.SysEx)
				}
			},
		},
		{
			name:     "clock carries no payload",
			event:    NewSystemMessage(EventTypeClock, 4),
			wantType: EventTypeClock,
			wantStr:  "Clock{offset:4}",
			check:    func(*testing.T, Event) {},
		},
		{
			name:     "reset carries no payload",
			event:    NewSystemMessage(EventTypeReset, 0),
			wantType: EventTypeReset,
			wantStr:  "Reset{offset:0}",
			check:    func(*testing.T, Event) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", tt.event.Type, tt.wantType)
			}
			if got := tt.event.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
			tt.check(t, tt.event)
		})
	}
}

func TestEventTypeString(t *testing.T) {
	if got := EventTypeActiveSensing.String(); got != "ActiveSensing" {
		t.Errorf("EventTypeActiveSensing.String() = %q, want %q", got, "ActiveSensing")
	}
	if got := EventType(200).String(); got != "EventType(200)" {
		t.Errorf("EventType(200).String() = %q, want %q", got, "EventType(200)")
	}
}

func TestShiftOffsetDoesNotMutateTheOriginal(t *testing.T) {
	original := NewNoteOn(0, 60, 100, 128)

	shifted := original.ShiftOffset(-128)

	if original.Offset != 128 {
		t.Errorf("original offset = %d, want 128 — ShiftOffset must copy", original.Offset)
	}
	if shifted.Offset != 0 {
		t.Errorf("shifted offset = %d, want 0", shifted.Offset)
	}
	if shifted.Note() != 60 || shifted.Velocity() != 100 {
		t.Error("ShiftOffset must preserve the payload")
	}
}

// TestEventsDoNotAllocate is the reason Event is a struct and not an interface.
// Events are built on the audio thread once per message per block; boxing them
// would allocate on every one.
func TestEventsDoNotAllocate(t *testing.T) {
	var sink Event

	allocs := testing.AllocsPerRun(100, func() {
		sink = NewNoteOn(0, 60, 100, 0)
		sink = sink.ShiftOffset(8)
	})
	if allocs != 0 {
		t.Errorf("building and shifting an event allocated %v times, want 0", allocs)
	}

	queue := NewEventQueue()
	scratch := make([]Event, 0, 64)

	allocs = testing.AllocsPerRun(100, func() {
		queue.Clear()
		for i := range int32(32) {
			queue.Add(NewNoteOn(0, uint8(i), 100, i*4))
		}
		scratch = queue.AppendEventsInRange(scratch[:0], 0, 128)
	})
	if allocs != 0 {
		t.Errorf("filling and draining a queue allocated %v times, want 0", allocs)
	}
	if len(scratch) != 32 {
		t.Errorf("drained %d events, want 32", len(scratch))
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

func TestMixedEventsShareOneSliceElement(t *testing.T) {
	events := []Event{
		NewNoteOn(0, 60, 100, 0),
		NewNoteOff(0, 60, 0, 100),
		NewControlChange(0, CCSustain, 127, 200),
		NewPitchBend(0, 0, 300),
	}

	wantOffsets := []int32{0, 100, 200, 300}
	for i, event := range events {
		if event.Offset != wantOffsets[i] {
			t.Errorf("event %d offset = %d, want %d", i, event.Offset, wantOffsets[i])
		}
		if event.Channel != 0 {
			t.Errorf("event %d channel = %d, want 0", i, event.Channel)
		}
		if event.String() == "" {
			t.Errorf("event %d has an empty String()", i)
		}
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
