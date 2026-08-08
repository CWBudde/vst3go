// Package bus provides VST3 audio bus configuration and management.
//
// A [Configuration] declares a plugin's audio and event buses. The templates
// ([Stereo], [Mono], [EffectStereo], [Generator], [MIDIEffect]) cover the usual
// topologies; the builder covers the rest.
//
// # Threading
//
// A Configuration is built once, during setup, and treated as immutable
// afterwards: adding a bus reallocates the underlying slices, which is not
// safe against a concurrent reader. The host, however, keeps toggling bus
// activation for the lifetime of the plugin, so that one flag is separated out
// and shared through an atomic.
//
//   - Control thread only: everything that adds or replaces a bus.
//   - Safe on any thread, including the audio thread: [Info.Active] and
//     [Configuration.SetBusActive].
//   - Descriptive reads ([Configuration.GetBusInfo], [Configuration.GetBusCount])
//     are safe once the configuration is built. GetBusInfo returns a detached
//     copy whose IsActive field is a snapshot; call [Info.Active] for the live
//     value.
package bus

import (
	"fmt"
	"sync/atomic"
)

// MediaType represents the type of bus
type MediaType int32

const (
	// MediaTypeAudio represents audio bus type
	MediaTypeAudio MediaType = 0
	// MediaTypeEvent represents event/MIDI bus type
	MediaTypeEvent MediaType = 1
)

// Direction represents the bus direction
type Direction int32

const (
	// DirectionInput represents input bus
	DirectionInput Direction = 0
	// DirectionOutput represents output bus
	DirectionOutput Direction = 1
)

// Type represents the bus type
type Type int32

const (
	// TypeMain represents main bus
	TypeMain Type = 0
	// TypeAux represents auxiliary bus
	TypeAux Type = 1
)

// Info contains bus configuration.
//
// The host activates and deactivates buses from its UI/main thread while the
// audio thread reads the activation state, so the flag lives in a shared atomic
// that survives copying an Info by value. Everything else in an Info is
// immutable once the Configuration has been built.
type Info struct {
	MediaType    MediaType
	Direction    Direction
	ChannelCount int32
	Name         string
	BusType      Type

	// IsActive is a snapshot of the activation state taken when this Info was
	// handed out. It stays valid for descriptive use (filling in a VST3
	// BusInfo, tests, logging); call Active for the live value.
	IsActive bool

	// active is the authoritative activation flag, shared by every copy of this
	// Info. It is nil for Info values constructed outside this package, in
	// which case Active falls back to IsActive.
	active *atomic.Bool
}

// newInfo builds an Info with its shared activation flag initialized. Every
// Info stored in a Configuration must go through here, otherwise Active has no
// live value to read.
func newInfo(mediaType MediaType, direction Direction, channelCount int32, name string, busType Type, active bool) Info {
	info := Info{
		MediaType:    mediaType,
		Direction:    direction,
		ChannelCount: channelCount,
		Name:         name,
		BusType:      busType,
		IsActive:     active,
		active:       new(atomic.Bool),
	}
	info.active.Store(active)
	return info
}

// Active reports the current activation state. It is safe to call from the
// audio thread while the host toggles the bus from another thread.
func (i *Info) Active() bool {
	if i.active != nil {
		return i.active.Load()
	}
	return i.IsActive
}

// setActive publishes a new activation state to every copy of this Info.
func (i *Info) setActive(active bool) {
	if i.active != nil {
		i.active.Store(active)
		return
	}
	i.IsActive = active
}

// snapshot returns a detached copy with IsActive refreshed from the shared
// flag. Callers get a value that cannot be invalidated by a later append to the
// Configuration's bus slices, while still sharing the activation atomic.
func (i *Info) snapshot() Info {
	out := *i
	out.IsActive = i.Active()
	return out
}

// Configuration manages audio and event buses
type Configuration struct {
	audioBuses []Info
	eventBuses []Info
}

// Stereo creates a standard stereo I/O configuration.
func Stereo() *Configuration {
	return &Configuration{
		audioBuses: []Info{
			newInfo(MediaTypeAudio, DirectionInput, 2, "Stereo In", TypeMain, true),
			newInfo(MediaTypeAudio, DirectionOutput, 2, "Stereo Out", TypeMain, true),
		},
	}
}

// Mono creates a mono I/O configuration.
func Mono() *Configuration {
	return &Configuration{
		audioBuses: []Info{
			newInfo(MediaTypeAudio, DirectionInput, 1, "Mono In", TypeMain, true),
			newInfo(MediaTypeAudio, DirectionOutput, 1, "Mono Out", TypeMain, true),
		},
	}
}

// NewStereoConfiguration creates a standard stereo I/O configuration.
//
// Deprecated: use Stereo.
func NewStereoConfiguration() *Configuration {
	return Stereo()
}

// NewMonoConfiguration creates a mono I/O configuration.
//
// Deprecated: use Mono.
func NewMonoConfiguration() *Configuration {
	return Mono()
}

// busesFor returns the slice backing the given media type.
func (c *Configuration) busesFor(mediaType MediaType) []Info {
	if mediaType == MediaTypeEvent {
		return c.eventBuses
	}
	return c.audioBuses
}

// GetBusCount returns the number of buses for a given type and direction
func (c *Configuration) GetBusCount(mediaType MediaType, direction Direction) int32 {
	count := int32(0)
	buses := c.busesFor(mediaType)

	for i := range buses {
		if buses[i].Direction == direction {
			count++
		}
	}

	return count
}

// GetBusInfo returns information about a specific bus, or nil if it does not
// exist. The returned Info is a copy: adding buses later reallocates the
// internal slices, so a pointer into them would go stale. Mutating the copy
// does not affect the Configuration.
func (c *Configuration) GetBusInfo(mediaType MediaType, direction Direction, index int32) *Info {
	buses := c.busesFor(mediaType)

	busIndex := int32(0)
	for i := range buses {
		if buses[i].Direction == direction {
			if busIndex == index {
				info := buses[i].snapshot()
				return &info
			}
			busIndex++
		}
	}

	return nil
}

// AddEventBus adds an event bus (for MIDI input)
func (c *Configuration) AddEventBus(direction Direction, name string) {
	c.eventBuses = append(c.eventBuses, newInfo(MediaTypeEvent, direction, 1, name, TypeMain, true))
}

// SetBusActive activates or deactivates a specific bus. It is safe to call
// while the audio thread reads the activation state through Info.Active.
func (c *Configuration) SetBusActive(mediaType MediaType, direction Direction, index int32, active bool) error {
	buses := c.busesFor(mediaType)

	busIndex := int32(0)
	for i := range buses {
		if buses[i].Direction == direction {
			if busIndex == index {
				buses[i].setActive(active)
				return nil
			}
			busIndex++
		}
	}

	return fmt.Errorf("%w: mediaType=%d, direction=%d, index=%d", ErrBusNotFound, mediaType, direction, index)
}

// GetActiveInputChannelCount returns the total number of active input channels
func (c *Configuration) GetActiveInputChannelCount() int32 {
	count := int32(0)
	for i := range c.audioBuses {
		if c.audioBuses[i].Direction == DirectionInput && c.audioBuses[i].Active() {
			count += c.audioBuses[i].ChannelCount
		}
	}
	return count
}

// GetActiveOutputChannelCount returns the total number of active output channels
func (c *Configuration) GetActiveOutputChannelCount() int32 {
	count := int32(0)
	for i := range c.audioBuses {
		if c.audioBuses[i].Direction == DirectionOutput && c.audioBuses[i].Active() {
			count += c.audioBuses[i].ChannelCount
		}
	}
	return count
}

// GetActiveBuses returns copies of all active buses of a given type and direction
func (c *Configuration) GetActiveBuses(mediaType MediaType, direction Direction) []Info {
	buses := c.busesFor(mediaType)

	var active []Info
	for i := range buses {
		if buses[i].Direction == direction && buses[i].Active() {
			active = append(active, buses[i].snapshot())
		}
	}
	return active
}

// HasSidechain returns true if the configuration has a sidechain input
func (c *Configuration) HasSidechain() bool {
	for i := range c.audioBuses {
		if c.audioBuses[i].Direction == DirectionInput && c.audioBuses[i].BusType == TypeAux {
			return true
		}
	}
	return false
}

// GetSidechainBus returns a copy of the first sidechain (aux input) bus if it
// exists. As with GetBusInfo the copy is detached from the internal slice.
func (c *Configuration) GetSidechainBus() *Info {
	for i := range c.audioBuses {
		if c.audioBuses[i].Direction == DirectionInput && c.audioBuses[i].BusType == TypeAux {
			info := c.audioBuses[i].snapshot()
			return &info
		}
	}
	return nil
}
