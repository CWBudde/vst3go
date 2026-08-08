package bus

import (
	"sync"
	"testing"
)

// The host toggles buses from its UI thread while the audio thread reads the
// activation state every block. Run with -race; this fails on a plain bool.
func TestSetBusActiveIsRaceFreeWithConcurrentReaders(t *testing.T) {
	config := NewBuilder().
		WithStereoInput("In").
		WithStereoOutput("Out").
		WithSidechain("SC").
		MustBuild()

	// The audio thread typically caches the Info it got at activation time.
	cached := config.GetBusInfo(MediaTypeAudio, DirectionInput, 1)
	if cached == nil {
		t.Fatal("expected sidechain input bus")
	}

	const iterations = 2000
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // audio thread
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = cached.Active()
				_ = config.GetActiveInputChannelCount()
			}
		}
	}()

	wg.Add(1)
	go func() { // UI thread
		defer wg.Done()
		defer close(stop)
		for i := 0; i < iterations; i++ {
			if err := config.SetBusActive(MediaTypeAudio, DirectionInput, 1, i%2 == 0); err != nil {
				t.Errorf("SetBusActive failed: %v", err)
				return
			}
		}
	}()

	wg.Wait()

	// Final state comes from the last write (i == iterations-1, odd -> false).
	if cached.Active() {
		t.Error("expected sidechain to end deactivated")
	}
}

// A cached Info must observe activation changes made after it was handed out,
// otherwise the audio thread keeps processing a bus the host has disabled.
func TestInfoActiveTracksLaterChanges(t *testing.T) {
	config := Stereo()

	cached := config.GetBusInfo(MediaTypeAudio, DirectionInput, 0)
	if !cached.Active() {
		t.Fatal("expected input bus to start active")
	}

	if err := config.SetBusActive(MediaTypeAudio, DirectionInput, 0, false); err != nil {
		t.Fatalf("SetBusActive failed: %v", err)
	}

	if cached.Active() {
		t.Error("cached Info did not observe deactivation")
	}
}

// An Info built by hand outside this package has no shared flag; Active must
// still report something sensible instead of panicking.
func TestInfoActiveFallsBackToIsActiveField(t *testing.T) {
	info := Info{MediaType: MediaTypeAudio, Direction: DirectionInput, ChannelCount: 2, IsActive: true}
	if !info.Active() {
		t.Error("expected Active to fall back to IsActive")
	}

	info.setActive(false)
	if info.Active() {
		t.Error("expected fallback setActive to update IsActive")
	}
}
