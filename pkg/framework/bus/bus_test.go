package bus

import (
	"errors"
	"testing"
)

// Bus names reused across assertions.
const (
	nameStereoIn    = "Stereo In"
	nameSidechainIn = "Sidechain In"
)

func TestNewStereoConfiguration(t *testing.T) {
	config := NewStereoConfiguration()

	// Check bus counts
	if got := config.GetBusCount(MediaTypeAudio, DirectionInput); got != 1 {
		t.Errorf("Expected 1 audio input bus, got %d", got)
	}
	if got := config.GetBusCount(MediaTypeAudio, DirectionOutput); got != 1 {
		t.Errorf("Expected 1 audio output bus, got %d", got)
	}

	// Check input bus
	inBus := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if inBus == nil {
		t.Fatal("Expected input bus to exist")
	}
	if inBus.ChannelCount != 2 {
		t.Errorf("Expected 2 input channels, got %d", inBus.ChannelCount)
	}
	if inBus.Name != "Stereo In" {
		t.Errorf("Expected input name 'Stereo In', got %s", inBus.Name)
	}

	// Check output bus
	outBus := config.GetBusInfo(MediaTypeAudio, DirectionOutput, 0)
	if outBus == nil {
		t.Fatal("Expected output bus to exist")
	}
	if outBus.ChannelCount != 2 {
		t.Errorf("Expected 2 output channels, got %d", outBus.ChannelCount)
	}
}

func TestStereo(t *testing.T) {
	config := Stereo()

	if got := config.GetBusCount(MediaTypeAudio, DirectionInput); got != 1 {
		t.Errorf("Expected 1 audio input bus, got %d", got)
	}
	if got := config.GetBusCount(MediaTypeAudio, DirectionOutput); got != 1 {
		t.Errorf("Expected 1 audio output bus, got %d", got)
	}
}

func TestNewMonoConfiguration(t *testing.T) {
	config := NewMonoConfiguration()

	inBus := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if inBus.ChannelCount != 1 {
		t.Errorf("Expected 1 input channel, got %d", inBus.ChannelCount)
	}

	outBus := config.GetBusInfo(MediaTypeAudio, DirectionOutput, 0)
	if outBus.ChannelCount != 1 {
		t.Errorf("Expected 1 output channel, got %d", outBus.ChannelCount)
	}
}

func TestMono(t *testing.T) {
	config := Mono()

	inBus := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if inBus.ChannelCount != 1 {
		t.Errorf("Expected 1 input channel, got %d", inBus.ChannelCount)
	}
}

func TestAddEventBus(t *testing.T) {
	config := NewStereoConfiguration()
	config.AddEventBus(DirectionInput, "MIDI In")

	if got := config.GetBusCount(MediaTypeEvent, DirectionInput); got != 1 {
		t.Errorf("Expected 1 event input bus, got %d", got)
	}

	eventBus := config.GetBusInfo(MediaTypeEvent, DirectionInput, 0)
	if eventBus == nil {
		t.Fatal("Expected event bus to exist")
	}
	if eventBus.Name != "MIDI In" {
		t.Errorf("Expected event bus name 'MIDI In', got %s", eventBus.Name)
	}
}

func TestSetBusActive(t *testing.T) {
	config := NewStereoConfiguration()

	// Initially should be active
	bus := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if !bus.IsActive {
		t.Error("Expected bus to be initially active")
	}

	// Deactivate
	err := config.SetBusActive(MediaTypeAudio, DirectionInput, 0, false)
	if err != nil {
		t.Errorf("SetBusActive failed: %v", err)
	}

	bus = config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if bus.IsActive {
		t.Error("Expected bus to be inactive after SetBusActive(false)")
	}

	// Try invalid bus
	err = config.SetBusActive(MediaTypeAudio, DirectionInput, 99, false)
	if err == nil {
		t.Error("Expected error for invalid bus index")
	}
}

func TestGetActiveChannelCounts(t *testing.T) {
	config := NewBuilder().
		WithStereoInput("Main").
		WithMonoInput("Aux").
		WithStereoOutput("Out").
		MustBuild()

	if got := config.GetActiveInputChannelCount(); got != 3 {
		t.Errorf("Expected 3 active input channels, got %d", got)
	}

	if got := config.GetActiveOutputChannelCount(); got != 2 {
		t.Errorf("Expected 2 active output channels, got %d", got)
	}

	// Deactivate aux input
	err := config.SetBusActive(MediaTypeAudio, DirectionInput, 1, false)
	if err != nil {
		t.Fatalf("SetBusActive failed: %v", err)
	}

	if got := config.GetActiveInputChannelCount(); got != 2 {
		t.Errorf("Expected 2 active input channels after deactivation, got %d", got)
	}
}

func TestHasSidechain(t *testing.T) {
	// Without sidechain
	config := NewEffectStereo()
	if config.HasSidechain() {
		t.Error("Expected no sidechain in stereo effect")
	}

	// With sidechain
	config = NewEffectStereoSidechain()
	if !config.HasSidechain() {
		t.Error("Expected sidechain in sidechain effect")
	}

	sidechainBus := config.GetSidechainBus()
	if sidechainBus == nil {
		t.Fatal("Expected sidechain bus to exist")
	}
	if sidechainBus.Name != "Sidechain In" {
		t.Errorf("Expected sidechain name 'Sidechain In', got %s", sidechainBus.Name)
	}
	if sidechainBus.BusType != TypeAux {
		t.Error("Expected sidechain to be auxiliary bus")
	}
}

func TestGetActiveBuses(t *testing.T) {
	config := NewBuilder().
		WithStereoInput("In1").
		WithStereoInput("In2").
		WithStereoOutput("Out").
		MustBuild()

	// Deactivate second input
	err := config.SetBusActive(MediaTypeAudio, DirectionInput, 1, false)
	if err != nil {
		t.Fatalf("SetBusActive failed: %v", err)
	}

	activeBuses := config.GetActiveBuses(MediaTypeAudio, DirectionInput)
	if len(activeBuses) != 1 {
		t.Errorf("Expected 1 active input bus, got %d", len(activeBuses))
	}
	if activeBuses[0].Name != "In1" {
		t.Errorf("Expected active bus name 'In1', got %s", activeBuses[0].Name)
	}
}

// GetBusInfo used to return a pointer into the internal slice, so a later
// AddEventBus could reallocate the backing array and leave the caller reading
// a stale copy.
func TestGetBusInfoIsNotInvalidatedByLaterAppends(t *testing.T) {
	config := Stereo()

	info := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if info == nil {
		t.Fatal("expected input bus")
	}

	for i := 0; i < 8; i++ {
		config.AddEventBus(DirectionInput, "MIDI In")
	}

	if info.Name != nameStereoIn || info.ChannelCount != 2 {
		t.Errorf("Info was invalidated by appends: %+v", info)
	}

	// The copy must be detached: mutating it must not corrupt the config.
	info.ChannelCount = 99
	fresh := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if fresh.ChannelCount != 2 {
		t.Errorf("mutating the returned Info leaked into the configuration: %d", fresh.ChannelCount)
	}
}

func TestGetSidechainBusIsDetached(t *testing.T) {
	config := NewEffectStereoSidechain()

	sc := config.GetSidechainBus()
	if sc == nil {
		t.Fatal("expected sidechain bus")
	}

	sc.Name = "mutated"
	if again := config.GetSidechainBus(); again.Name != nameSidechainIn {
		t.Errorf("GetSidechainBus aliases internal storage: %s", again.Name)
	}
}

func TestSetBusActiveErrors(t *testing.T) {
	config := NewEffectStereo()

	tests := []struct {
		name      string
		mediaType MediaType
		direction Direction
		index     int32
		wantErr   bool
	}{
		{name: "valid audio input", mediaType: MediaTypeAudio, direction: DirectionInput, index: 0},
		{name: "valid audio output", mediaType: MediaTypeAudio, direction: DirectionOutput, index: 0},
		{name: "index out of range", mediaType: MediaTypeAudio, direction: DirectionInput, index: 7, wantErr: true},
		{name: "no event buses", mediaType: MediaTypeEvent, direction: DirectionInput, index: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.SetBusActive(tt.mediaType, tt.direction, tt.index, true)
			if tt.wantErr {
				if !errors.Is(err, ErrBusNotFound) {
					t.Fatalf("expected ErrBusNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetBusInfoOutOfRange(t *testing.T) {
	config := NewEffectStereo()

	if got := config.GetBusInfo(MediaTypeAudio, DirectionInput, 5); got != nil {
		t.Errorf("expected nil for out-of-range index, got %+v", got)
	}
	if got := config.GetBusInfo(MediaTypeEvent, DirectionInput, 0); got != nil {
		t.Errorf("expected nil for missing event bus, got %+v", got)
	}
}
