package param

import (
	"errors"
	"fmt"
)

// firstAutoID is the first automatically assigned ID. ID 0 doubles as "unset"
// on an incoming Parameter, so auto-assignment must not hand it out.
const firstAutoID uint32 = 1

// AutoRegistry wraps a Registry with automatic ID management and name lookup.
//
// The Registry is deliberately a plain field rather than an embedded one: a
// promoted Add would insert parameters behind the name index and break the
// name/ID invariant.
type AutoRegistry struct {
	registry    *Registry
	nextID      uint32 // guarded by registry.mu
	nameToID    map[string]uint32
	autoEnabled bool
}

// NewAutoRegistry creates a registry with automatic ID management
func NewAutoRegistry() *AutoRegistry {
	return &AutoRegistry{
		registry:    NewRegistry(),
		nextID:      firstAutoID,
		nameToID:    make(map[string]uint32),
		autoEnabled: true,
	}
}

// Registry returns the underlying registry for APIs that take a *Registry.
func (r *AutoRegistry) Registry() *Registry {
	return r.registry
}

// EnableAutoID enables or disables automatic ID assignment
func (r *AutoRegistry) EnableAutoID(enabled bool) {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	r.autoEnabled = enabled
}

// Register adds parameters, assigning IDs automatically when their ID is 0.
// Registering a name twice is an error; use Replace to overwrite deliberately.
func (r *AutoRegistry) Register(params ...*Parameter) error {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()

	for _, p := range params {
		if _, exists := r.nameToID[p.Name]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateName, p.Name)
		}

		if r.autoEnabled && p.ID == 0 {
			p.ID = r.nextID
			r.nextID++
		}

		if _, exists := r.registry.params[p.ID]; exists {
			return fmt.Errorf("%w: %d", ErrDuplicateID, p.ID)
		}

		r.insertLocked(p.ID, p)
	}

	return nil
}

// Replace registers a parameter, overwriting any parameter with the same name.
func (r *AutoRegistry) Replace(p *Parameter) error {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()

	if existingID, exists := r.nameToID[p.Name]; exists {
		p.ID = existingID
		r.registry.params[existingID] = p
		return nil
	}

	if r.autoEnabled && p.ID == 0 {
		p.ID = r.nextID
		r.nextID++
	}
	if _, exists := r.registry.params[p.ID]; exists {
		return fmt.Errorf("%w: %d", ErrDuplicateID, p.ID)
	}

	r.insertLocked(p.ID, p)
	return nil
}

// RegisterWithID adds a parameter with a specific ID
func (r *AutoRegistry) RegisterWithID(id uint32, param *Parameter) error {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()

	if existing, exists := r.registry.params[id]; exists {
		return fmt.Errorf("%w: %d already used by %q", ErrDuplicateID, id, existing.Name)
	}
	if _, exists := r.nameToID[param.Name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateName, param.Name)
	}

	param.ID = id

	// Keep auto-assignment clear of manually placed IDs.
	if id >= r.nextID {
		r.nextID = id + 1
	}

	r.insertLocked(id, param)
	return nil
}

// insertLocked adds a parameter to all three indices. Caller holds the lock.
func (r *AutoRegistry) insertLocked(id uint32, p *Parameter) {
	r.registry.params[id] = p
	r.registry.order = append(r.registry.order, id)
	r.nameToID[p.Name] = id
}

// GetByName retrieves a parameter by name
func (r *AutoRegistry) GetByName(name string) *Parameter {
	r.registry.mu.RLock()
	defer r.registry.mu.RUnlock()

	id, exists := r.nameToID[name]
	if !exists {
		return nil
	}

	return r.registry.params[id]
}

// GetID returns the ID for a parameter name
func (r *AutoRegistry) GetID(name string) (uint32, bool) {
	r.registry.mu.RLock()
	defer r.registry.mu.RUnlock()

	id, exists := r.nameToID[name]
	return id, exists
}

// Get retrieves a parameter by ID.
func (r *AutoRegistry) Get(id uint32) *Parameter { return r.registry.Get(id) }

// GetOK retrieves a parameter by ID and reports whether it exists.
func (r *AutoRegistry) GetOK(id uint32) (*Parameter, bool) { return r.registry.GetOK(id) }

// Has reports whether a parameter with the given ID exists.
func (r *AutoRegistry) Has(id uint32) bool { return r.registry.Has(id) }

// GetNormalized returns the normalized value of a parameter if it exists.
func (r *AutoRegistry) GetNormalized(id uint32) (float64, bool) { return r.registry.GetNormalized(id) }

// GetPlain returns the plain value of a parameter if it exists.
func (r *AutoRegistry) GetPlain(id uint32) (float64, bool) { return r.registry.GetPlain(id) }

// GetByIndex retrieves a parameter by index.
func (r *AutoRegistry) GetByIndex(index int32) *Parameter { return r.registry.GetByIndex(index) }

// Count returns the number of parameters.
func (r *AutoRegistry) Count() int32 { return r.registry.Count() }

// All returns all parameters in order.
func (r *AutoRegistry) All() []*Parameter { return r.registry.All() }

// Clear removes all parameters and resets the ID counter
func (r *AutoRegistry) Clear() {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()

	r.registry.params = make(map[uint32]*Parameter)
	r.registry.order = make([]uint32, 0)
	r.nameToID = make(map[string]uint32)
	r.nextID = firstAutoID
}

// Reserve reserves a range of IDs for manual assignment and returns the first.
func (r *AutoRegistry) Reserve(count uint32) uint32 {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()

	first := r.nextID
	r.nextID += count
	return first
}

// RegistryBuilder provides fluent parameter registration.
type RegistryBuilder struct {
	registry *AutoRegistry
	errors   []error
}

// NewRegistryBuilder creates a builder for fluent registration
func NewRegistryBuilder(registry *AutoRegistry) *RegistryBuilder {
	return &RegistryBuilder{
		registry: registry,
		errors:   make([]error, 0),
	}
}

// Add registers a parameter
func (b *RegistryBuilder) Add(param *Parameter) *RegistryBuilder {
	if err := b.registry.Register(param); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

// AddWithID registers a parameter with a specific ID
func (b *RegistryBuilder) AddWithID(id uint32, param *Parameter) *RegistryBuilder {
	if err := b.registry.RegisterWithID(id, param); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

// Build finalizes the registration and returns any errors
func (b *RegistryBuilder) Build() error {
	return errors.Join(b.errors...)
}

// Helper functions for common parameter groups

// RegisterStandardControls registers common audio effect controls
func (r *AutoRegistry) RegisterStandardControls() error {
	return NewRegistryBuilder(r).
		Add(BypassParameter(0, "Bypass").Build()).
		Add(MixParameter(0, "Mix").Build()).
		Add(GainParameter(0, "Input Gain").Build()).
		Add(GainParameter(0, "Output Gain").Build()).
		Build()
}

// RegisterCompressorControls registers standard compressor parameters
func (r *AutoRegistry) RegisterCompressorControls() error {
	return NewRegistryBuilder(r).
		Add(ThresholdParameter(0, "Threshold", -60, 0, -20).Build()).
		Add(RatioParameter(0, "Ratio", 1, 20, 4).Build()).
		Add(AttackParameter(0, "Attack", 100).Build()).
		Add(ReleaseParameter(0, "Release", 1000).Build()).
		Add(New(0, "Knee").
			Range(0, 10).
			Default(2).
			Formatter(func(v float64) string { return fmt.Sprintf("%.1f dB", v) }, nil).
			Build()).
		Add(Choice(0, "Auto Gain", []ChoiceOption{
			{Value: 0, Name: displayOff},
			{Value: 1, Name: displayOn},
		}).Build()).
		Build()
}

// RegisterEQBand registers a parametric EQ band
func (r *AutoRegistry) RegisterEQBand(bandNumber int) error {
	prefix := fmt.Sprintf("Band %d", bandNumber)

	filterTypes := []ChoiceOption{
		{Value: 0, Name: "Bell"},
		{Value: 1, Name: filterNameLowShelf},
		{Value: 2, Name: filterNameHighShelf},
		{Value: 3, Name: "Low Pass"},
		{Value: 4, Name: "High Pass"},
		{Value: 5, Name: filterNameNotch},
	}

	return NewRegistryBuilder(r).
		Add(Choice(0, prefix+" Enable", []ChoiceOption{
			{Value: 0, Name: displayOff},
			{Value: 1, Name: displayOn},
		}).Build()).
		Add(FrequencyParameter(0, prefix+" Frequency", 20, 20000, 1000).Build()).
		Add(GainParameter(0, prefix+" Gain").Build()).
		Add(QParameter(0, prefix+" Q", 0.1, 10, 0.7).Build()).
		Add(Choice(0, prefix+" Type", filterTypes).Build()).
		Build()
}
