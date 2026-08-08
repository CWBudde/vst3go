package plugin

import (
	"errors"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/bus"
	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
)

func TestNewBaseProcessorDefaultsToStereo(t *testing.T) {
	p := NewBaseProcessor(nil)

	if p.GetBuses() == nil {
		t.Fatal("nil bus configuration should fall back to a default, got nil")
	}
	if p.GetParameters() == nil {
		t.Fatal("parameter registry must be non-nil")
	}
	if p.Parameters() != p.GetParameters() {
		t.Error("Parameters and GetParameters must return the same registry")
	}
}

func TestBaseProcessorKeepsSuppliedBuses(t *testing.T) {
	buses := bus.Mono()

	p := NewBaseProcessor(buses)

	if p.GetBuses() != buses {
		t.Error("a supplied bus configuration must not be replaced by the default")
	}
}

func TestBaseProcessorLifecycleDefaults(t *testing.T) {
	p := NewBaseProcessor(nil)

	// With no callbacks registered every lifecycle hook must succeed quietly.
	if err := p.Initialize(48000, 512); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := p.SampleRate(); got != 48000 {
		t.Errorf("SampleRate = %v, want 48000", got)
	}
	if err := p.SetActive(true); err != nil {
		t.Fatalf("SetActive(true): %v", err)
	}
	if err := p.SetActive(false); err != nil {
		t.Fatalf("SetActive(false): %v", err)
	}
	if got := p.GetLatencySamples(); got != 0 {
		t.Errorf("GetLatencySamples = %d, want 0", got)
	}
	if got := p.GetTailSamples(); got != 0 {
		t.Errorf("GetTailSamples = %d, want 0", got)
	}
}

func TestBaseProcessorCallbacks(t *testing.T) {
	errInit := errors.New("init failed")
	errActive := errors.New("activate failed")

	t.Run("initialize receives its arguments and propagates errors", func(t *testing.T) {
		p := NewBaseProcessor(nil)
		var gotRate float64
		var gotBlock int32

		p.OnInitialize(func(sampleRate float64, maxBlockSize int32) error {
			gotRate, gotBlock = sampleRate, maxBlockSize
			return errInit
		})

		if err := p.Initialize(96000, 1024); !errors.Is(err, errInit) {
			t.Fatalf("Initialize error = %v, want %v", err, errInit)
		}
		if gotRate != 96000 || gotBlock != 1024 {
			t.Errorf("callback got (%v, %d), want (96000, 1024)", gotRate, gotBlock)
		}
		// The sample rate is recorded before the callback runs, so a failing
		// callback must not leave it unset.
		if p.SampleRate() != 96000 {
			t.Errorf("SampleRate = %v, want 96000 even after a failed callback", p.SampleRate())
		}
	})

	t.Run("setActive propagates errors", func(t *testing.T) {
		p := NewBaseProcessor(nil)
		p.OnSetActive(func(bool) error { return errActive })

		if err := p.SetActive(true); !errors.Is(err, errActive) {
			t.Fatalf("SetActive error = %v, want %v", err, errActive)
		}
	})

	t.Run("reset fires only on deactivation", func(t *testing.T) {
		p := NewBaseProcessor(nil)
		resets := 0
		p.OnReset(func() { resets++ })

		if err := p.SetActive(true); err != nil {
			t.Fatalf("SetActive(true): %v", err)
		}
		if resets != 0 {
			t.Fatalf("reset fired %d times on activation, want 0", resets)
		}

		if err := p.SetActive(false); err != nil {
			t.Fatalf("SetActive(false): %v", err)
		}
		if resets != 1 {
			t.Errorf("reset fired %d times on deactivation, want 1", resets)
		}
	})

	t.Run("reset runs before the setActive callback", func(t *testing.T) {
		p := NewBaseProcessor(nil)
		var order []string
		p.OnReset(func() { order = append(order, "reset") })
		p.OnSetActive(func(bool) error { order = append(order, "setActive"); return nil })

		if err := p.SetActive(false); err != nil {
			t.Fatalf("SetActive(false): %v", err)
		}
		if len(order) != 2 || order[0] != "reset" || order[1] != "setActive" {
			t.Errorf("callback order = %v, want [reset setActive]", order)
		}
	})
}

func TestSimpleProcessor(t *testing.T) {
	t.Run("delegates to the process function", func(t *testing.T) {
		called := 0
		p := NewSimpleProcessor(bus.Stereo(), func(*process.Context) { called++ })

		p.ProcessAudio(process.NewContext(64, param.NewRegistry()))

		if called != 1 {
			t.Errorf("process function called %d times, want 1", called)
		}
	})

	t.Run("a nil process function is a no-op rather than a panic", func(t *testing.T) {
		p := NewSimpleProcessor(bus.Stereo(), nil)

		p.ProcessAudio(process.NewContext(64, param.NewRegistry()))
	})

	t.Run("inherits the BaseProcessor contract", func(t *testing.T) {
		p := NewSimpleProcessor(nil, nil)

		if p.GetBuses() == nil || p.GetParameters() == nil {
			t.Error("embedded BaseProcessor must still be wired up")
		}
	})
}
