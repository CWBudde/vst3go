package param

import (
	"fmt"
	"sync"
)

// Registry manages plugin parameters
type Registry struct {
	params map[uint32]*Parameter
	order  []uint32 // Maintain order for indexed access
	mu     sync.RWMutex
}

// NewRegistry creates a new parameter registry
func NewRegistry() *Registry {
	return &Registry{
		params: make(map[uint32]*Parameter),
		order:  make([]uint32, 0),
	}
}

// Add registers a new parameter
func (r *Registry) Add(params ...*Parameter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range params {
		if _, exists := r.params[p.ID]; exists {
			return fmt.Errorf("parameter ID %d already exists", p.ID)
		}
		r.params[p.ID] = p
		r.order = append(r.order, p.ID)
	}

	return nil
}

// Get retrieves a parameter by ID
func (r *Registry) Get(id uint32) *Parameter {
	param, _ := r.GetOK(id)
	return param
}

// GetOK retrieves a parameter by ID and reports whether it exists.
func (r *Registry) GetOK(id uint32) (*Parameter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	param, ok := r.params[id]
	return param, ok
}

// Has reports whether a parameter with the given ID exists.
func (r *Registry) Has(id uint32) bool {
	_, ok := r.GetOK(id)
	return ok
}

// GetNormalized returns the normalized value of a parameter if it exists.
func (r *Registry) GetNormalized(id uint32) (float64, bool) {
	param, ok := r.GetOK(id)
	if !ok {
		return 0, false
	}
	return param.GetNormalized(), true
}

// GetPlain returns the plain value of a parameter if it exists.
func (r *Registry) GetPlain(id uint32) (float64, bool) {
	param, ok := r.GetOK(id)
	if !ok {
		return 0, false
	}
	return param.GetPlain(), true
}

// GetByIndex retrieves a parameter by index
func (r *Registry) GetByIndex(index int32) *Parameter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if index < 0 || index >= int32(len(r.order)) {
		return nil
	}

	id := r.order[index]
	return r.params[id]
}

// Count returns the number of parameters
func (r *Registry) Count() int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int32(len(r.order))
}

// All returns all parameters in order
func (r *Registry) All() []*Parameter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Parameter, len(r.order))
	for i, id := range r.order {
		result[i] = r.params[id]
	}

	return result
}
