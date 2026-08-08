// Package midi provides MIDI event types and a sample-accurate event queue for
// VST3 plugins.
package midi

import (
	"errors"
	"fmt"
	"math"
)

// EventType identifies the kind of MIDI event carried by an Event.
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

// ErrUnshiftableEvent is returned when an event cannot be moved in time because
// its type does not implement OffsetShifter.
var ErrUnshiftableEvent = errors.New("midi: event type cannot be time-shifted")

// Event is a single MIDI message placed at a sample offset within the current
// processing block.
type Event interface {
	Type() EventType
	Channel() uint8
	SampleOffset() int32
	String() string
}

// OffsetShifter is implemented by every event type in this package. It returns a
// copy of the event moved by delta samples. EventQueue.OffsetEvents relies on it
// so that adding an event type cannot silently skip the rebase.
type OffsetShifter interface {
	ShiftOffset(delta int32) Event
}

// BaseEvent carries the channel and sample offset shared by all event types.
type BaseEvent struct {
	EventChannel uint8
	Offset       int32
}

// Channel returns the MIDI channel the event belongs to.
func (e BaseEvent) Channel() uint8 {
	return e.EventChannel
}

// SampleOffset returns the event position in samples from the start of the block.
func (e BaseEvent) SampleOffset() int32 {
	return e.Offset
}

// NoteOnEvent starts a note.
type NoteOnEvent struct {
	BaseEvent
	NoteNumber uint8
	Velocity   uint8
}

// Type returns EventTypeNoteOn.
func (e NoteOnEvent) Type() EventType {
	return EventTypeNoteOn
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e NoteOnEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e NoteOnEvent) String() string {
	return fmt.Sprintf("NoteOn{ch:%d, note:%d, vel:%d, offset:%d}",
		e.EventChannel, e.NoteNumber, e.Velocity, e.Offset)
}

// NoteOffEvent releases a note.
type NoteOffEvent struct {
	BaseEvent
	NoteNumber uint8
	Velocity   uint8
}

// Type returns EventTypeNoteOff.
func (e NoteOffEvent) Type() EventType {
	return EventTypeNoteOff
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e NoteOffEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e NoteOffEvent) String() string {
	return fmt.Sprintf("NoteOff{ch:%d, note:%d, vel:%d, offset:%d}",
		e.EventChannel, e.NoteNumber, e.Velocity, e.Offset)
}

// ControlChangeEvent carries a continuous controller change.
type ControlChangeEvent struct {
	BaseEvent
	Controller uint8
	Value      uint8
}

// Type returns EventTypeControlChange.
func (e ControlChangeEvent) Type() EventType {
	return EventTypeControlChange
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ControlChangeEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ControlChangeEvent) String() string {
	return fmt.Sprintf("CC{ch:%d, ctrl:%d, val:%d, offset:%d}",
		e.EventChannel, e.Controller, e.Value, e.Offset)
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

// PitchBendEvent carries a channel pitch bend.
type PitchBendEvent struct {
	BaseEvent
	Value int16 // -8192 to 8191, 0 is center
}

// Type returns EventTypePitchBend.
func (e PitchBendEvent) Type() EventType {
	return EventTypePitchBend
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e PitchBendEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e PitchBendEvent) String() string {
	return fmt.Sprintf("PitchBend{ch:%d, val:%d, offset:%d}",
		e.EventChannel, e.Value, e.Offset)
}

// NormalizedValue maps the raw bend value onto [-1, 1).
func (e PitchBendEvent) NormalizedValue() float64 {
	return float64(e.Value) / pitchBendScale
}

// PolyPressureEvent carries per-note aftertouch.
type PolyPressureEvent struct {
	BaseEvent
	NoteNumber uint8
	Pressure   uint8
}

// Type returns EventTypePolyPressure.
func (e PolyPressureEvent) Type() EventType {
	return EventTypePolyPressure
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e PolyPressureEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e PolyPressureEvent) String() string {
	return fmt.Sprintf("PolyPressure{ch:%d, note:%d, pressure:%d, offset:%d}",
		e.EventChannel, e.NoteNumber, e.Pressure, e.Offset)
}

// ChannelPressureEvent carries channel-wide aftertouch.
type ChannelPressureEvent struct {
	BaseEvent
	Pressure uint8
}

// Type returns EventTypeChannelPressure.
func (e ChannelPressureEvent) Type() EventType {
	return EventTypeChannelPressure
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ChannelPressureEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ChannelPressureEvent) String() string {
	return fmt.Sprintf("ChannelPressure{ch:%d, pressure:%d, offset:%d}",
		e.EventChannel, e.Pressure, e.Offset)
}

// ProgramChangeEvent selects a patch on a channel.
type ProgramChangeEvent struct {
	BaseEvent
	Program uint8
}

// Type returns EventTypeProgramChange.
func (e ProgramChangeEvent) Type() EventType {
	return EventTypeProgramChange
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ProgramChangeEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ProgramChangeEvent) String() string {
	return fmt.Sprintf("ProgramChange{ch:%d, prog:%d, offset:%d}",
		e.EventChannel, e.Program, e.Offset)
}

// SystemExclusiveEvent carries a raw SysEx payload. Data is the message body
// without the leading 0xF0 and trailing 0xF7 status bytes and is not copied, so
// the producer must not reuse the buffer.
type SystemExclusiveEvent struct {
	BaseEvent
	Data []byte
}

// Type returns EventTypeSystemExclusive.
func (e SystemExclusiveEvent) Type() EventType {
	return EventTypeSystemExclusive
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e SystemExclusiveEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e SystemExclusiveEvent) String() string {
	return fmt.Sprintf("SysEx{bytes:%d, offset:%d}", len(e.Data), e.Offset)
}

// ClockEvent is a MIDI timing clock tick.
type ClockEvent struct {
	BaseEvent
}

// Type returns EventTypeClock.
func (e ClockEvent) Type() EventType {
	return EventTypeClock
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ClockEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ClockEvent) String() string {
	return fmt.Sprintf("Clock{offset:%d}", e.Offset)
}

// StartEvent starts transport playback from the beginning.
type StartEvent struct {
	BaseEvent
}

// Type returns EventTypeStart.
func (e StartEvent) Type() EventType {
	return EventTypeStart
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e StartEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e StartEvent) String() string {
	return fmt.Sprintf("Start{offset:%d}", e.Offset)
}

// StopEvent stops transport playback.
type StopEvent struct {
	BaseEvent
}

// Type returns EventTypeStop.
func (e StopEvent) Type() EventType {
	return EventTypeStop
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e StopEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e StopEvent) String() string {
	return fmt.Sprintf("Stop{offset:%d}", e.Offset)
}

// ContinueEvent resumes transport playback from the current position.
type ContinueEvent struct {
	BaseEvent
}

// Type returns EventTypeContinue.
func (e ContinueEvent) Type() EventType {
	return EventTypeContinue
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ContinueEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ContinueEvent) String() string {
	return fmt.Sprintf("Continue{offset:%d}", e.Offset)
}

// ResetEvent is the system reset message.
type ResetEvent struct {
	BaseEvent
}

// Type returns EventTypeReset.
func (e ResetEvent) Type() EventType {
	return EventTypeReset
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ResetEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ResetEvent) String() string {
	return fmt.Sprintf("Reset{offset:%d}", e.Offset)
}

// ActiveSensingEvent is the active sensing keep-alive message.
type ActiveSensingEvent struct {
	BaseEvent
}

// Type returns EventTypeActiveSensing.
func (e ActiveSensingEvent) Type() EventType {
	return EventTypeActiveSensing
}

// ShiftOffset returns a copy of the event moved by delta samples.
func (e ActiveSensingEvent) ShiftOffset(delta int32) Event {
	e.Offset += delta
	return e
}

// String renders the event for logging.
func (e ActiveSensingEvent) String() string {
	return fmt.Sprintf("ActiveSensing{offset:%d}", e.Offset)
}

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
