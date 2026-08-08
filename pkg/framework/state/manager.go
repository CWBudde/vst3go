// Package state provides VST3 plugin state management for parameter and custom data persistence.
package state

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

const (
	// magicHeaderSize is the size of the VST3GO magic header
	magicHeaderSize = 6

	// magicHeader marks a stream as a vst3go preset
	magicHeader = "VST3GO"

	// customStateFramedVersion is the first format version that length-prefixes
	// the custom payload, and therefore the first that can skip it.
	customStateFramedVersion uint32 = 2

	// FormatVersion is the preset format version written by Save.
	//
	// Version 1 stored the custom payload unframed, so it could only ever be
	// consumed by a registered loader and never skipped. Version 2 length-prefixes
	// it. Load still reads version 1 streams: there the payload runs to EOF, which
	// is enough to drain it safely.
	FormatVersion uint32 = 2

	// maxCustomStateSize bounds the custom payload accepted by Load so a corrupt
	// length field cannot drive an unbounded allocation or read.
	maxCustomStateSize uint64 = 64 << 20
)

// Sentinel errors returned by Save and Load. All returned errors wrap one of
// these where a caller could reasonably branch on the cause.
var (
	// ErrNilRegistry indicates the manager was constructed without a registry.
	ErrNilRegistry = errors.New("state manager requires a parameter registry")

	// ErrInvalidFormat indicates the stream is not a vst3go preset or is malformed.
	ErrInvalidFormat = errors.New("invalid state format")

	// ErrUnsupportedVersion indicates the preset was written by a newer release.
	ErrUnsupportedVersion = errors.New("unsupported state version")

	// ErrNoCustomLoader indicates the preset carries a custom payload but no
	// custom load function is registered to interpret it.
	ErrNoCustomLoader = errors.New("state contains custom data but no custom load function is registered")
)

// Manager handles plugin state saving and loading
type Manager struct {
	registry   *param.Registry
	customSave CustomSaveFunc
	customLoad CustomLoadFunc
}

// CustomSaveFunc allows plugins to save additional state beyond parameters
type CustomSaveFunc func(w io.Writer) error

// CustomLoadFunc allows plugins to load additional state beyond parameters
type CustomLoadFunc func(r io.Reader) error

// NewManager creates a new state manager
func NewManager(registry *param.Registry) *Manager {
	return &Manager{
		registry: registry,
	}
}

// SetCustomSaveFunc sets a function for saving custom state
func (m *Manager) SetCustomSaveFunc(fn CustomSaveFunc) {
	m.customSave = fn
}

// SetCustomLoadFunc sets a function for loading custom state
func (m *Manager) SetCustomLoadFunc(fn CustomLoadFunc) {
	m.customLoad = fn
}

// Save writes the plugin state to a writer
func (m *Manager) Save(w io.Writer) error {
	if m.registry == nil {
		return ErrNilRegistry
	}

	// One snapshot for both the count and the body: taking them from separate
	// registry calls lets a concurrent Add produce a header that disagrees with
	// the payload, yielding a preset that cannot be read back.
	params := m.registry.All()

	if _, err := w.Write([]byte(magicHeader)); err != nil {
		return fmt.Errorf("write state header: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, FormatVersion); err != nil {
		return fmt.Errorf("write state version: %w", err)
	}

	if len(params) > math.MaxInt32 {
		return fmt.Errorf("%w: %d parameters exceed the format limit", ErrInvalidFormat, len(params))
	}
	// The bound above makes the narrowing conversion safe.
	if err := binary.Write(w, binary.LittleEndian, int32(len(params))); err != nil { //nolint:gosec
		return fmt.Errorf("write parameter count: %w", err)
	}

	for _, p := range params {
		if err := binary.Write(w, binary.LittleEndian, p.ID); err != nil {
			return fmt.Errorf("write parameter %d id: %w", p.ID, err)
		}

		if err := binary.Write(w, binary.LittleEndian, p.GetNormalized()); err != nil {
			return fmt.Errorf("write parameter %d value: %w", p.ID, err)
		}
	}

	return m.saveCustom(w)
}

// saveCustom appends the custom payload marker and, if a save function is
// registered, the length-prefixed payload itself. Buffering is what makes the
// length known up front, which is what lets a reader without a loader skip it.
func (m *Manager) saveCustom(w io.Writer) error {
	if m.customSave == nil {
		if err := binary.Write(w, binary.LittleEndian, uint32(0)); err != nil {
			return fmt.Errorf("write custom state marker: %w", err)
		}
		return nil
	}

	var payload bytes.Buffer
	if err := m.customSave(&payload); err != nil {
		return fmt.Errorf("save custom state: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(1)); err != nil {
		return fmt.Errorf("write custom state marker: %w", err)
	}
	// payload.Len() is a non-negative int, so the widening conversion is exact.
	if err := binary.Write(w, binary.LittleEndian, uint64(payload.Len())); err != nil { //nolint:gosec
		return fmt.Errorf("write custom state length: %w", err)
	}
	if _, err := w.Write(payload.Bytes()); err != nil {
		return fmt.Errorf("write custom state: %w", err)
	}
	return nil
}

// Load reads the plugin state from a reader.
//
// Parameters are decoded in full before any of them is applied, so a truncated or
// corrupt preset leaves the plugin on its previous state instead of a mix of old
// and new values.
func (m *Manager) Load(r io.Reader) error {
	if m.registry == nil {
		return ErrNilRegistry
	}

	header := make([]byte, magicHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return fmt.Errorf("read state header: %w", err)
	}
	if string(header) != magicHeader {
		return fmt.Errorf("%w: bad magic header", ErrInvalidFormat)
	}

	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return fmt.Errorf("read state version: %w", err)
	}
	if version == 0 || version > FormatVersion {
		return fmt.Errorf("%w: %d (supported: 1..%d)", ErrUnsupportedVersion, version, FormatVersion)
	}

	var paramCount int32
	if err := binary.Read(r, binary.LittleEndian, &paramCount); err != nil {
		return fmt.Errorf("read parameter count: %w", err)
	}
	if paramCount < 0 {
		return fmt.Errorf("%w: negative parameter count %d", ErrInvalidFormat, paramCount)
	}

	pending, err := m.decodeParams(r, paramCount)
	if err != nil {
		return err
	}

	customErr := m.loadCustom(r, version)
	if customErr != nil && !errors.Is(customErr, ErrNoCustomLoader) {
		return customErr
	}

	// Commit only once the whole stream decoded cleanly. ErrNoCustomLoader is the
	// one tolerated case: the parameters are intact and were skipped correctly, so
	// they are applied and the dropped custom payload is merely reported.
	for _, pv := range pending {
		pv.param.SetNormalized(pv.value)
	}
	return customErr
}

// pendingValue is a decoded parameter value waiting to be committed.
type pendingValue struct {
	param *param.Parameter
	value float64
}

// decodeParams reads count id/value pairs without touching live parameters.
// Unknown ids are dropped for forward compatibility.
func (m *Manager) decodeParams(r io.Reader, count int32) ([]pendingValue, error) {
	// The slice is not presized from count: a corrupt count must not drive the
	// allocation, so it grows only as pairs actually decode.
	var pending []pendingValue

	for i := int32(0); i < count; i++ {
		var id uint32
		if err := binary.Read(r, binary.LittleEndian, &id); err != nil {
			return nil, fmt.Errorf("read parameter id: %w", err)
		}

		var value float64
		if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
			return nil, fmt.Errorf("read parameter %d value: %w", id, err)
		}

		if p, ok := m.registry.GetOK(id); ok {
			pending = append(pending, pendingValue{param: p, value: value})
		}
	}

	return pending, nil
}

// loadCustom consumes the custom payload, either handing it to the registered
// loader or skipping it so the stream stays positioned correctly.
func (m *Manager) loadCustom(r io.Reader, version uint32) error {
	var hasCustom uint32
	if err := binary.Read(r, binary.LittleEndian, &hasCustom); err != nil {
		return fmt.Errorf("read custom state marker: %w", err)
	}
	if hasCustom == 0 {
		return nil
	}

	// Version 1 has no length prefix, so the payload is whatever remains.
	if version < customStateFramedVersion {
		if m.customLoad == nil {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return fmt.Errorf("skip custom state: %w", err)
			}
			return ErrNoCustomLoader
		}
		if err := m.customLoad(r); err != nil {
			return fmt.Errorf("load custom state: %w", err)
		}
		return nil
	}

	var length uint64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return fmt.Errorf("read custom state length: %w", err)
	}
	if length > maxCustomStateSize {
		return fmt.Errorf("%w: custom state length %d exceeds limit %d", ErrInvalidFormat, length, maxCustomStateSize)
	}

	if m.customLoad == nil {
		// Skip rather than leave the reader mid-payload, then report the loss so
		// the caller knows state was dropped.
		if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
			return fmt.Errorf("skip custom state: %w", err)
		}
		return ErrNoCustomLoader
	}

	limited := io.LimitReader(r, int64(length))
	if err := m.customLoad(limited); err != nil {
		return fmt.Errorf("load custom state: %w", err)
	}
	// Drain whatever the loader left behind so trailing data stays addressable.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return fmt.Errorf("skip trailing custom state: %w", err)
	}
	return nil
}
