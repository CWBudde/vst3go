// Package bus provides VST3 audio bus configuration and management.
package bus

import (
	"errors"
	"fmt"
)

// Sentinel errors returned while building or querying a bus configuration.
// They are wrapped with context, so match them with errors.Is.
var (
	// ErrBusNotFound is returned when no bus matches the requested media type,
	// direction and index.
	ErrBusNotFound = errors.New("bus not found")
	// ErrNoMainOutputBus is returned when a configuration has no main output
	// bus at all; a VST3 plugin cannot produce anything in that state.
	ErrNoMainOutputBus = errors.New("configuration must have at least one main output bus (audio or event)")
	// ErrInvalidChannelCount is returned for a non-positive channel count.
	ErrInvalidChannelCount = errors.New("invalid channel count")
	// ErrChannelCountTooHigh is returned when a bus exceeds MaxChannelsPerBus.
	ErrChannelCountTooHigh = errors.New("channel count exceeds maximum")
)

// MaxChannelsPerBus is the largest channel count accepted for a single bus.
const MaxChannelsPerBus = 32

// Builder provides a fluent API for building bus configurations
type Builder struct {
	config *Configuration
	errors []error
}

// NewBuilder creates a new bus configuration builder
func NewBuilder() *Builder {
	return &Builder{
		config: &Configuration{
			audioBuses: []Info{},
			eventBuses: []Info{},
		},
		errors: []error{},
	}
}

// WithAudioInput adds an audio input bus
func (b *Builder) WithAudioInput(name string, channels int32) *Builder {
	b.config.audioBuses = append(b.config.audioBuses,
		newInfo(MediaTypeAudio, DirectionInput, channels, name, TypeMain, true))
	return b
}

// WithAudioOutput adds an audio output bus
func (b *Builder) WithAudioOutput(name string, channels int32) *Builder {
	b.config.audioBuses = append(b.config.audioBuses,
		newInfo(MediaTypeAudio, DirectionOutput, channels, name, TypeMain, true))
	return b
}

// WithAuxInput adds an auxiliary audio input bus (e.g., sidechain)
func (b *Builder) WithAuxInput(name string, channels int32) *Builder {
	// Aux buses start inactive; the host opts in to a sidechain explicitly.
	b.config.audioBuses = append(b.config.audioBuses,
		newInfo(MediaTypeAudio, DirectionInput, channels, name, TypeAux, false))
	return b
}

// WithAuxOutput adds an auxiliary audio output bus
func (b *Builder) WithAuxOutput(name string, channels int32) *Builder {
	// Aux buses start inactive; the host opts in to extra sends explicitly.
	b.config.audioBuses = append(b.config.audioBuses,
		newInfo(MediaTypeAudio, DirectionOutput, channels, name, TypeAux, false))
	return b
}

// WithEventInput adds an event (MIDI) input bus
func (b *Builder) WithEventInput(name string) *Builder {
	b.config.eventBuses = append(b.config.eventBuses,
		newInfo(MediaTypeEvent, DirectionInput, 1, name, TypeMain, true))
	return b
}

// WithEventOutput adds an event (MIDI) output bus
func (b *Builder) WithEventOutput(name string) *Builder {
	b.config.eventBuses = append(b.config.eventBuses,
		newInfo(MediaTypeEvent, DirectionOutput, 1, name, TypeMain, true))
	return b
}

// WithStereoInput is a convenience method for adding stereo input
func (b *Builder) WithStereoInput(name string) *Builder {
	return b.WithAudioInput(name, 2)
}

// WithStereoOutput is a convenience method for adding stereo output
func (b *Builder) WithStereoOutput(name string) *Builder {
	return b.WithAudioOutput(name, 2)
}

// WithMonoInput is a convenience method for adding mono input
func (b *Builder) WithMonoInput(name string) *Builder {
	return b.WithAudioInput(name, 1)
}

// WithMonoOutput is a convenience method for adding mono output
func (b *Builder) WithMonoOutput(name string) *Builder {
	return b.WithAudioOutput(name, 1)
}

// WithSidechain adds a sidechain input bus (auxiliary stereo input)
func (b *Builder) WithSidechain(name string) *Builder {
	return b.WithAuxInput(name, 2)
}

// WithQuadInput adds a quadraphonic (4-channel) input
func (b *Builder) WithQuadInput(name string) *Builder {
	return b.WithAudioInput(name, 4)
}

// WithQuadOutput adds a quadraphonic (4-channel) output
func (b *Builder) WithQuadOutput(name string) *Builder {
	return b.WithAudioOutput(name, 4)
}

// With5_1Input adds a 5.1 surround input
func (b *Builder) With5_1Input(name string) *Builder {
	return b.WithAudioInput(name, 6)
}

// With5_1Output adds a 5.1 surround output
func (b *Builder) With5_1Output(name string) *Builder {
	return b.WithAudioOutput(name, 6)
}

// With7_1Input adds a 7.1 surround input
func (b *Builder) With7_1Input(name string) *Builder {
	return b.WithAudioInput(name, 8)
}

// With7_1Output adds a 7.1 surround output
func (b *Builder) With7_1Output(name string) *Builder {
	return b.WithAudioOutput(name, 8)
}

// SetBusActive sets a bus's initial and default activation state.
func (b *Builder) SetBusActive(mediaType MediaType, direction Direction, index int32, active bool) *Builder {
	if err := b.config.SetBusActive(mediaType, direction, index, active); err != nil {
		b.errors = append(b.errors, err)
	} else {
		b.config.findBus(mediaType, direction, index).defaultActive = active
	}
	return b
}

// Validate checks if the configuration is valid
func (b *Builder) Validate() error {
	// Check for errors accumulated during building
	if len(b.errors) > 0 {
		return fmt.Errorf("builder errors: %w", errors.Join(b.errors...))
	}

	// Check that we have at least one main output bus (audio or event)
	hasMainOutput := false
	for _, audioBus := range b.config.audioBuses {
		if audioBus.Direction == DirectionOutput && audioBus.BusType == TypeMain {
			hasMainOutput = true
			break
		}
	}

	// MIDI effects might only have event buses
	if !hasMainOutput && len(b.config.eventBuses) > 0 {
		for _, eventBus := range b.config.eventBuses {
			if eventBus.Direction == DirectionOutput && eventBus.BusType == TypeMain {
				hasMainOutput = true
				break
			}
		}
	}

	if !hasMainOutput {
		return ErrNoMainOutputBus
	}

	// Validate channel counts
	for _, audioBus := range b.config.audioBuses {
		if audioBus.ChannelCount <= 0 {
			return fmt.Errorf("%w %d for bus %q", ErrInvalidChannelCount, audioBus.ChannelCount, audioBus.Name)
		}
		if audioBus.ChannelCount > MaxChannelsPerBus {
			return fmt.Errorf("%w: %d exceeds %d for bus %q", ErrChannelCountTooHigh, audioBus.ChannelCount, MaxChannelsPerBus, audioBus.Name)
		}
	}

	return nil
}

// Build returns the built configuration or an error
func (b *Builder) Build() (*Configuration, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b.config, nil
}

// MustBuild returns the built configuration or panics on error.
//
// Only use it where the configuration is statically known to be valid, such as
// the argument-free templates in this package. Anything that derives a
// configuration from caller input must use Build and return the error.
func (b *Builder) MustBuild() *Configuration {
	config, err := b.Build()
	if err != nil {
		panic(err)
	}
	return config
}
