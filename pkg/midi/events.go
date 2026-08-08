// Package midi provides MIDI event types and a sample-accurate event queue for
// VST3 plugins.
//
// [Event] is one concrete tagged struct rather than an interface hierarchy, so
// events never box and the audio path stays allocation-free. Build one with a
// constructor ([NewNoteOn], [NewControlChange], …), discriminate on
// [Event.Type], and read the payload through the type-guarded accessors
// ([Event.Note], [Event.Velocity], [Event.Controller], …), which return zero
// for an event of the wrong kind.
//
// SampleOffset positions an event inside the current block. The runtime rebases
// offsets so they are relative to the buffer the plugin actually sees, which
// matters when a block is split into chunks around parameter automation.
//
// # Threading
//
// [EventQueue] is guarded by an RWMutex and is safe for concurrent use. The
// lock is uncontended in the normal case — one producer, one consumer — where
// it costs a pair of atomics and allocates nothing, which is what makes it
// usable from the audio thread. It is not lock-free, so a design in which the
// control thread hammers the same queue during processing will make the audio
// thread wait.
//
// Events themselves are values, so copying one is safe. The single exception
// is [Event.SysEx]: [NewSysEx] retains the caller's slice rather than copying
// it, so every copy of that event shares one backing array.
//
// [EventQueue.GetEventsInRange] and [EventQueue.GetAllEvents] return freshly
// allocated copies and are therefore control-thread calls. Their audio-thread
// counterparts, [EventQueue.AppendEventsInRange] and
// [EventQueue.AppendAllEvents], append into caller-owned scratch and allocate
// nothing as long as it has spare capacity; the result is valid only until the
// queue is next mutated, so it must not outlive the process call that read it.
package midi

import (
	"fmt"
	"math"
	"strings"
)

// EventType identifies the kind of MIDI message carried by an Event.
type EventType uint8

// The MIDI event kinds understood by this package.
const (
	EventTypeNoteOff EventType = iota
	EventTypeNoteOn
	EventTypePolyPressure
	EventTypeControlChange
	EventTypeProgramChange
	EventTypeChannelPressure
	EventTypePitchBend
	EventTypeSystemExclusive
	EventTypeClock
	EventTypeStart
	EventTypeStop
	EventTypeContinue
	EventTypeReset
	EventTypeActiveSensing
)

const (
	// semitonesPerOctave is the equal-temperament division used by all note and
	// frequency conversions.
	semitonesPerOctave = 12

	// referenceNote is the MIDI note number of A4, the tuning reference.
	referenceNote = 69

	// maxNoteNumber is the highest valid MIDI note number.
	maxNoteNumber = 127

	// defaultTuningA4 is the frequency of A4 in Hz when no tuning is given.
	defaultTuningA4 = 440.0

	// pitchBendScale is the magnitude of a full pitch bend deflection.
	pitchBendScale = 8192.0
)

// Event is a single MIDI message placed at a sample offset within the current
// processing block.
//
// Event is a concrete tagged struct rather than an interface on purpose. Events
// are produced on the audio thread, once per message per block; storing them
// behind an interface would heap-allocate every one of them, which is exactly
// what a realtime callback must not do. The cost is that the payload fields are
// a union discriminated by Type instead of separate types per message.
//
// Use the constructors (NewNoteOn, NewControlChange, …) to build events and the
// accessors (Note, Velocity, Controller, …) to read them; both document which
// field means what for a given Type.
type Event struct {
	// SysEx is the payload of an EventTypeSystemExclusive message, without the
	// leading 0xF0 and trailing 0xF7 status bytes. It is not copied, so the
	// producer must not reuse the buffer. It is nil for every other Type.
	SysEx []byte

	// Offset is the event position in samples from the start of the block.
	Offset int32

	// Bend is the raw pitch bend deflection for EventTypePitchBend, in the range
	// -8192..8191 with 0 at center. It is 0 for every other Type.
	Bend int16

	// Type selects which of the payload fields below carry meaning.
	Type EventType

	// Channel is the zero-based MIDI channel. System messages (clock, start,
	// stop, continue, reset, active sensing, sysex) have no channel and leave
	// it at 0.
	Channel uint8

	// Data1 and Data2 are the two MIDI data bytes. Their meaning depends on Type:
	//
	//	NoteOff, NoteOn      Data1 = note number, Data2 = velocity
	//	PolyPressure         Data1 = note number, Data2 = pressure
	//	ControlChange        Data1 = controller,  Data2 = value
	//	ChannelPressure      Data1 = pressure
	//	ProgramChange        Data1 = program number
	//	everything else      unused
	Data1 uint8
	Data2 uint8
}

// NewNoteOn builds a note-on event.
func NewNoteOn(channel, note, velocity uint8, offset int32) Event {
	return Event{Type: EventTypeNoteOn, Channel: channel, Data1: note, Data2: velocity, Offset: offset}
}

// NewNoteOff builds a note-off event.
func NewNoteOff(channel, note, velocity uint8, offset int32) Event {
	return Event{Type: EventTypeNoteOff, Channel: channel, Data1: note, Data2: velocity, Offset: offset}
}

// NewPolyPressure builds a per-note aftertouch event.
func NewPolyPressure(channel, note, pressure uint8, offset int32) Event {
	return Event{Type: EventTypePolyPressure, Channel: channel, Data1: note, Data2: pressure, Offset: offset}
}

// NewControlChange builds a continuous controller event.
func NewControlChange(channel, controller, value uint8, offset int32) Event {
	return Event{Type: EventTypeControlChange, Channel: channel, Data1: controller, Data2: value, Offset: offset}
}

// NewChannelPressure builds a channel-wide aftertouch event.
func NewChannelPressure(channel, pressure uint8, offset int32) Event {
	return Event{Type: EventTypeChannelPressure, Channel: channel, Data1: pressure, Offset: offset}
}

// NewProgramChange builds a patch-select event.
func NewProgramChange(channel, program uint8, offset int32) Event {
	return Event{Type: EventTypeProgramChange, Channel: channel, Data1: program, Offset: offset}
}

// NewPitchBend builds a pitch bend event from a raw deflection in -8192..8191.
func NewPitchBend(channel uint8, bend int16, offset int32) Event {
	return Event{Type: EventTypePitchBend, Channel: channel, Bend: bend, Offset: offset}
}

// NewSysEx builds a system-exclusive event. data is retained, not copied.
func NewSysEx(data []byte, offset int32) Event {
	return Event{Type: EventTypeSystemExclusive, SysEx: data, Offset: offset}
}

// NewSystemMessage builds a payload-free system message such as
// EventTypeClock or EventTypeStart.
func NewSystemMessage(eventType EventType, offset int32) Event {
	return Event{Type: eventType, Offset: offset}
}

// Note returns the note number of a note-on, note-off or poly-pressure event,
// and 0 for any other Type.
func (e Event) Note() uint8 {
	switch e.Type {
	case EventTypeNoteOn, EventTypeNoteOff, EventTypePolyPressure:
		return e.Data1
	default:
		return 0
	}
}

// Velocity returns the velocity of a note-on or note-off event, and 0 for any
// other Type.
func (e Event) Velocity() uint8 {
	switch e.Type {
	case EventTypeNoteOn, EventTypeNoteOff:
		return e.Data2
	default:
		return 0
	}
}

// Controller returns the controller number of a control-change event, and 0 for
// any other Type.
func (e Event) Controller() uint8 {
	if e.Type == EventTypeControlChange {
		return e.Data1
	}
	return 0
}

// Value returns the controller value of a control-change event, and 0 for any
// other Type.
func (e Event) Value() uint8 {
	if e.Type == EventTypeControlChange {
		return e.Data2
	}
	return 0
}

// Pressure returns the aftertouch amount of a poly-pressure or channel-pressure
// event, and 0 for any other Type.
func (e Event) Pressure() uint8 {
	switch e.Type {
	case EventTypePolyPressure:
		return e.Data2
	case EventTypeChannelPressure:
		return e.Data1
	default:
		return 0
	}
}

// Program returns the patch number of a program-change event, and 0 for any
// other Type.
func (e Event) Program() uint8 {
	if e.Type == EventTypeProgramChange {
		return e.Data1
	}
	return 0
}

// NormalizedBend maps the raw pitch bend deflection onto [-1, 1).
func (e Event) NormalizedBend() float64 {
	return float64(e.Bend) / pitchBendScale
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e Event) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e Event) String() string {
	var b strings.Builder

	b.WriteString(e.Type.String())
	b.WriteByte('{')

	switch e.Type {
	case EventTypeNoteOn, EventTypeNoteOff:
		fmt.Fprintf(&b, "ch:%d, note:%d, vel:%d, ", e.Channel, e.Data1, e.Data2)
	case EventTypePolyPressure:
		fmt.Fprintf(&b, "ch:%d, note:%d, pressure:%d, ", e.Channel, e.Data1, e.Data2)
	case EventTypeControlChange:
		fmt.Fprintf(&b, "ch:%d, ctrl:%d, val:%d, ", e.Channel, e.Data1, e.Data2)
	case EventTypeChannelPressure:
		fmt.Fprintf(&b, "ch:%d, pressure:%d, ", e.Channel, e.Data1)
	case EventTypeProgramChange:
		fmt.Fprintf(&b, "ch:%d, prog:%d, ", e.Channel, e.Data1)
	case EventTypePitchBend:
		fmt.Fprintf(&b, "ch:%d, val:%d, ", e.Channel, e.Bend)
	case EventTypeSystemExclusive:
		fmt.Fprintf(&b, "bytes:%d, ", len(e.SysEx))
	default:
		// Clock, Start, Stop, Continue, Reset and ActiveSensing carry no payload.
	}

	fmt.Fprintf(&b, "offset:%d}", e.Offset)
	return b.String()
}

// String returns the event kind's name.
func (t EventType) String() string {
	switch t {
	case EventTypeNoteOff:
		return "NoteOff"
	case EventTypeNoteOn:
		return "NoteOn"
	case EventTypePolyPressure:
		return "PolyPressure"
	case EventTypeControlChange:
		return "ControlChange"
	case EventTypeProgramChange:
		return "ProgramChange"
	case EventTypeChannelPressure:
		return "ChannelPressure"
	case EventTypePitchBend:
		return "PitchBend"
	case EventTypeSystemExclusive:
		return "SysEx"
	case EventTypeClock:
		return "Clock"
	case EventTypeStart:
		return "Start"
	case EventTypeStop:
		return "Stop"
	case EventTypeContinue:
		return "Continue"
	case EventTypeReset:
		return "Reset"
	case EventTypeActiveSensing:
		return "ActiveSensing"
	default:
		return fmt.Sprintf("EventType(%d)", uint8(t))
	}
}

// Standard MIDI continuous controller numbers.
const (
	CCModWheel       uint8 = 1
	CCBreath         uint8 = 2
	CCFoot           uint8 = 4
	CCPortamentoTime uint8 = 5
	CCVolume         uint8 = 7
	CCBalance        uint8 = 8
	CCPan            uint8 = 10
	CCExpression     uint8 = 11
	CCSustain        uint8 = 64
	CCPortamento     uint8 = 65
	CCSostenuto      uint8 = 66
	CCSoft           uint8 = 67
	CCLegato         uint8 = 68
	CCHold2          uint8 = 69
	CCAllSoundOff    uint8 = 120
	CCResetAll       uint8 = 121
	CCLocalControl   uint8 = 122
	CCAllNotesOff    uint8 = 123
)

// NoteToFrequency converts a MIDI note number to its frequency in Hz using equal
// temperament. A tuningA4 of 0 selects the 440 Hz standard.
//
// math.Exp2 compiles to a single intrinsic and allocates nothing, so there is no
// reason to approximate it: an earlier hand-rolled cubic was both slower and off
// by up to 7 cents.
func NoteToFrequency(note uint8, tuningA4 float64) float64 {
	if tuningA4 == 0 {
		tuningA4 = defaultTuningA4
	}
	return tuningA4 * math.Exp2((float64(note)-referenceNote)/semitonesPerOctave)
}

// FrequencyToNote converts a frequency in Hz to the nearest MIDI note number,
// clamped to the valid 0..127 range. A tuningA4 of 0 selects the 440 Hz standard.
func FrequencyToNote(freq, tuningA4 float64) uint8 {
	if tuningA4 == 0 {
		tuningA4 = defaultTuningA4
	}
	note := referenceNote + semitonesPerOctave*math.Log2(freq/tuningA4)
	// A non-positive freq yields -Inf or NaN, both of which clamp to note 0.
	if math.IsNaN(note) || note < 0 {
		return 0
	}
	if note > maxNoteNumber {
		return maxNoteNumber
	}
	return uint8(math.Round(note))
}

// NoteNumberToName renders a MIDI note number as a scientific pitch name, e.g. C4
// for middle C.
func NoteNumberToName(note uint8) string {
	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	octave := int(note)/semitonesPerOctave - 1
	noteName := noteNames[int(note)%semitonesPerOctave]
	return fmt.Sprintf("%s%d", noteName, octave)
}
