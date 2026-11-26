package plugin

import (
	"fmt"
	"math"
	"slices"
	"sync/atomic"

	"github.com/cwbudde/vst3go/pkg/framework/param"
	"github.com/cwbudde/vst3go/pkg/framework/process"
	"github.com/cwbudde/vst3go/pkg/vst3"
)

// FactoryProgramList describes the root unit's factory programs. ID and
// ParameterID are stable host-visible IDs. ParameterID must be distinct from
// the processor's own parameter IDs. Names are in program selection order.
type FactoryProgramList struct {
	ID           int32
	Name         string
	ParameterID  uint32
	Names        []string
	DefaultIndex int32
}

// FactoryProgramProvider exposes factory programs through IUnitInfo and a
// program selector parameter. A provider supplies its parameter registry and
// default program values when it is constructed. The runtime adds the program
// selector before initializing the processor.
//
// ApplyFactoryProgram runs on the audio thread while processing, so it must
// use preallocated state and avoid locks, blocking and host calls. The runtime
// can also call it on the control thread before processing starts. On return,
// the runtime updates the selector. Loading saved state restores parameters
// and the selector directly, preserving any saved edits to the program.
type FactoryProgramProvider interface {
	FactoryPrograms() FactoryProgramList
	ApplyFactoryProgram(index int32)
}

type factoryPrograms struct {
	list      FactoryProgramList
	parameter *param.Parameter
	apply     func(int32)
	// The high bit is the processing state. The remaining bits hold a
	// one-based program index, with zero meaning no pending UI selection.
	selection atomic.Int32
}

const programsRunning int32 = -1 << 31

func newFactoryPrograms(provider FactoryProgramProvider, parameters *param.Registry) (*factoryPrograms, error) {
	list := provider.FactoryPrograms()
	if parameters == nil || list.ID < 0 || list.Name == "" || len(list.Names) > math.MaxInt32 ||
		list.DefaultIndex < 0 || int(list.DefaultIndex) >= len(list.Names) {
		return nil, fmt.Errorf("invalid factory program list: %w", vst3.ErrInvalidArgument)
	}
	list.Names = slices.Clone(list.Names)
	options := make([]param.ChoiceOption, len(list.Names))
	for i, name := range list.Names {
		if name == "" {
			return nil, fmt.Errorf("empty name for factory program %d: %w", i, vst3.ErrInvalidArgument)
		}
		options[i] = param.ChoiceOption{Value: float64(i), Name: name}
	}
	// A one-entry program list has exactly one value and no intervals.
	parameter := param.Choice(list.ParameterID, "Program", options).
		Default(float64(list.DefaultIndex)).
		AddFlags(param.IsProgramChange).Build()
	if err := parameters.Add(parameter); err != nil {
		return nil, fmt.Errorf("register factory program parameter: %w", err)
	}
	return &factoryPrograms{list: list, parameter: parameter, apply: provider.ApplyFactoryProgram}, nil
}

func (p *factoryPrograms) index(normalized float64) (int32, bool) {
	if math.IsNaN(normalized) || math.IsInf(normalized, 0) {
		return 0, false
	}
	return int32(p.parameter.Denormalize(normalized)), true
}

func (p *factoryPrograms) selectFromController(normalized float64) error {
	index, ok := p.index(normalized)
	if !ok {
		return vst3.ErrInvalidArgument
	}
	for {
		previous := p.selection.Load()
		if previous&programsRunning != 0 {
			if p.selection.CompareAndSwap(previous, programsRunning|(index+1)) {
				return nil
			}
		} else if p.selection.CompareAndSwap(previous, 0) {
			p.applyIndex(index)
			return nil
		}
	}
}

func (p *factoryPrograms) applyIndex(index int32) {
	if int32(p.parameter.GetPlain()) != index {
		p.apply(index)
		p.parameter.SetPlain(float64(index))
	}
}

func (p *factoryPrograms) applyPending(changes []process.ParameterChange) {
	selection := p.selection.Load()
	pending := selection &^ programsRunning
	if pending == 0 {
		return
	}
	index := pending - 1
	hostSelection := false
	for _, change := range changes {
		if change.ParamID != p.list.ParameterID {
			continue
		}
		if queued, ok := p.index(change.Value); ok {
			hostSelection = true
			if queued == index {
				p.selection.CompareAndSwap(selection, selection&programsRunning)
				return // Apply the duplicate only at the host's offset.
			}
		}
	}
	if !hostSelection && p.selection.CompareAndSwap(selection, selection&programsRunning) {
		p.applyIndex(index)
	}
}
