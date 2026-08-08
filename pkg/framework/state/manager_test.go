package state

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/cwbudde/vst3go/pkg/framework/param"
)

func TestManagerRejectsNilRegistry(t *testing.T) {
	m := NewManager(nil)

	if err := m.Save(&bytes.Buffer{}); !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("Save() error = %v, want ErrNilRegistry", err)
	}

	if err := m.Load(bytes.NewReader([]byte("VST3GO"))); !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("Load() error = %v, want ErrNilRegistry", err)
	}
}

func TestManagerSaveLoadRoundTrip(t *testing.T) {
	reg := param.NewRegistry()
	p := &param.Parameter{ID: 1, Name: "Gain", Min: -12, Max: 12}
	p.SetPlain(6)
	if err := reg.Add(p); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	var buf bytes.Buffer
	m := NewManager(reg)
	if err := m.Save(&buf); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	p.SetPlain(-6)
	if err := m.Load(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := p.GetPlain(); got != 6 {
		t.Fatalf("GetPlain() after Load = %f, want 6", got)
	}
}

func newTestRegistry(t *testing.T, ids ...uint32) (*param.Registry, []*param.Parameter) {
	t.Helper()

	reg := param.NewRegistry()
	params := make([]*param.Parameter, 0, len(ids))
	for _, id := range ids {
		p := &param.Parameter{ID: id, Name: "P", Min: 0, Max: 1}
		p.SetNormalized(0.25)
		if err := reg.Add(p); err != nil {
			t.Fatalf("Add(%d) error = %v", id, err)
		}
		params = append(params, p)
	}
	return reg, params
}

func TestSaveLoadRoundTripWithCustomState(t *testing.T) {
	tests := []struct {
		name       string
		custom     []byte
		withLoader bool
		wantErr    error
	}{
		{name: "no custom state", custom: nil, withLoader: false},
		{name: "custom state with loader", custom: []byte("hello preset"), withLoader: true},
		{name: "empty custom state with loader", custom: []byte{}, withLoader: true},
		{name: "custom state without loader", custom: []byte("orphaned"), withLoader: false, wantErr: ErrNoCustomLoader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, params := newTestRegistry(t, 1, 2, 3)
			saver := NewManager(reg)
			if tt.custom != nil {
				saver.SetCustomSaveFunc(func(w io.Writer) error {
					_, err := w.Write(tt.custom)
					return err
				})
			}

			for i, p := range params {
				p.SetNormalized(float64(i+1) / 10.0)
			}

			var buf bytes.Buffer
			if err := saver.Save(&buf); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			// Trailing data must stay reachable, which only works if the custom
			// payload is framed.
			const sentinel = "TRAILER"
			buf.WriteString(sentinel)

			loader := NewManager(reg)
			var loaded []byte
			if tt.withLoader {
				loader.SetCustomLoadFunc(func(r io.Reader) error {
					var err error
					loaded, err = io.ReadAll(r)
					return err
				})
			}

			for _, p := range params {
				p.SetNormalized(0.99)
			}

			reader := bytes.NewReader(buf.Bytes())
			err := loader.Load(reader)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			for i, p := range params {
				want := float64(i+1) / 10.0
				if got := p.GetNormalized(); got != want {
					t.Errorf("parameter %d = %f, want %f", p.ID, got, want)
				}
			}

			if tt.withLoader && !bytes.Equal(loaded, tt.custom) {
				t.Errorf("custom state = %q, want %q", loaded, tt.custom)
			}

			rest, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if string(rest) != sentinel {
				t.Errorf("stream position after Load leaves %q, want %q", rest, sentinel)
			}
		})
	}
}

// A preset that fails to decode must leave every parameter untouched rather than
// applying the prefix that happened to parse.
func TestLoadTruncatedPresetDoesNotMutate(t *testing.T) {
	reg, params := newTestRegistry(t, 1, 2, 3)
	m := NewManager(reg)
	m.SetCustomSaveFunc(func(w io.Writer) error {
		_, err := w.Write([]byte("custom payload"))
		return err
	})

	for i, p := range params {
		p.SetNormalized(float64(i+1) / 10.0)
	}

	var buf bytes.Buffer
	if err := m.Save(&buf); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	full := buf.Bytes()

	// Cut at every length that still carries the header, so truncation lands in
	// the middle of an id, a value, the marker and the payload alike.
	for cut := magicHeaderSize; cut < len(full); cut++ {
		t.Run(fmt.Sprintf("truncated_at_%d", cut), func(t *testing.T) {
			const marker = 0.75
			for _, p := range params {
				p.SetNormalized(marker)
			}

			err := m.Load(bytes.NewReader(full[:cut]))
			if err == nil {
				t.Fatalf("Load() of %d/%d bytes succeeded, want error", cut, len(full))
			}

			for _, p := range params {
				if got := p.GetNormalized(); got != marker {
					t.Errorf("parameter %d mutated to %f by a failed Load", p.ID, got)
				}
			}
		})
	}
}

func TestLoadRejectsBadStreams(t *testing.T) {
	validVersion := func(v uint32) []byte {
		var b bytes.Buffer
		b.WriteString("VST3GO")
		_ = binary.Write(&b, binary.LittleEndian, v)
		_ = binary.Write(&b, binary.LittleEndian, int32(0))
		_ = binary.Write(&b, binary.LittleEndian, uint32(0))
		return b.Bytes()
	}

	negativeCount := func() []byte {
		var b bytes.Buffer
		b.WriteString("VST3GO")
		_ = binary.Write(&b, binary.LittleEndian, FormatVersion)
		_ = binary.Write(&b, binary.LittleEndian, int32(-1))
		return b.Bytes()
	}

	hugeCustom := func() []byte {
		var b bytes.Buffer
		b.WriteString("VST3GO")
		_ = binary.Write(&b, binary.LittleEndian, FormatVersion)
		_ = binary.Write(&b, binary.LittleEndian, int32(0))
		_ = binary.Write(&b, binary.LittleEndian, uint32(1))
		_ = binary.Write(&b, binary.LittleEndian, uint64(1)<<40)
		return b.Bytes()
	}

	tests := []struct {
		name  string
		input []byte
		want  error
	}{
		{"wrong magic", append([]byte("NOTVST"), 0, 0, 0, 0), ErrInvalidFormat},
		{"version zero", validVersion(0), ErrUnsupportedVersion},
		{"future version", validVersion(FormatVersion + 1), ErrUnsupportedVersion},
		{"negative parameter count", negativeCount(), ErrInvalidFormat},
		{"implausible custom length", hugeCustom(), ErrInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, _ := newTestRegistry(t, 1)
			m := NewManager(reg)
			if err := m.Load(bytes.NewReader(tt.input)); !errors.Is(err, tt.want) {
				t.Fatalf("Load() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// Version 1 presets stored the custom payload unframed, running to EOF. They must
// still load.
func TestLoadVersion1Preset(t *testing.T) {
	buildV1 := func(withCustom bool) []byte {
		var b bytes.Buffer
		b.WriteString("VST3GO")
		_ = binary.Write(&b, binary.LittleEndian, uint32(1))
		_ = binary.Write(&b, binary.LittleEndian, int32(1))
		_ = binary.Write(&b, binary.LittleEndian, uint32(1))
		_ = binary.Write(&b, binary.LittleEndian, 0.5)
		if withCustom {
			_ = binary.Write(&b, binary.LittleEndian, uint32(1))
			b.WriteString("legacy custom")
		} else {
			_ = binary.Write(&b, binary.LittleEndian, uint32(0))
		}
		return b.Bytes()
	}

	t.Run("without custom data", func(t *testing.T) {
		reg, params := newTestRegistry(t, 1)
		m := NewManager(reg)
		if err := m.Load(bytes.NewReader(buildV1(false))); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := params[0].GetNormalized(); got != 0.5 {
			t.Errorf("parameter = %f, want 0.5", got)
		}
	})

	t.Run("with custom data and loader", func(t *testing.T) {
		reg, params := newTestRegistry(t, 1)
		m := NewManager(reg)
		var loaded []byte
		m.SetCustomLoadFunc(func(r io.Reader) error {
			var err error
			loaded, err = io.ReadAll(r)
			return err
		})
		if err := m.Load(bytes.NewReader(buildV1(true))); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if string(loaded) != "legacy custom" {
			t.Errorf("custom state = %q, want %q", loaded, "legacy custom")
		}
		if got := params[0].GetNormalized(); got != 0.5 {
			t.Errorf("parameter = %f, want 0.5", got)
		}
	})

	t.Run("with custom data and no loader", func(t *testing.T) {
		reg, params := newTestRegistry(t, 1)
		m := NewManager(reg)
		if err := m.Load(bytes.NewReader(buildV1(true))); !errors.Is(err, ErrNoCustomLoader) {
			t.Fatalf("Load() error = %v, want ErrNoCustomLoader", err)
		}
		if got := params[0].GetNormalized(); got != 0.5 {
			t.Errorf("parameter = %f, want 0.5", got)
		}
	})
}

func TestLoadIgnoresUnknownParameters(t *testing.T) {
	srcReg, srcParams := newTestRegistry(t, 1, 2, 99)
	for i, p := range srcParams {
		p.SetNormalized(float64(i+1) / 10.0)
	}

	var buf bytes.Buffer
	if err := NewManager(srcReg).Save(&buf); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	dstReg, dstParams := newTestRegistry(t, 1, 2)
	if err := NewManager(dstReg).Load(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for i, p := range dstParams {
		want := float64(i+1) / 10.0
		if got := p.GetNormalized(); got != want {
			t.Errorf("parameter %d = %f, want %f", p.ID, got, want)
		}
	}
}

// The header count and the body must come from the same snapshot; otherwise a
// concurrent Add produces a preset that cannot be read back.
func TestSaveHeaderMatchesBodyUnderConcurrentAdd(t *testing.T) {
	reg, _ := newTestRegistry(t, 1, 2, 3)
	m := NewManager(reg)

	const extraParams = 500
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < extraParams; i++ {
			p := &param.Parameter{ID: uint32(100 + i), Name: "extra", Min: 0, Max: 1}
			if err := reg.Add(p); err != nil {
				return
			}
		}
	}()

	for i := 0; i < extraParams; i++ {
		var buf bytes.Buffer
		if err := m.Save(&buf); err != nil {
			t.Errorf("Save() error = %v", err)
			break
		}

		// A self-consistent preset decodes cleanly and consumes the whole stream.
		reader := bytes.NewReader(buf.Bytes())
		if err := m.Load(reader); err != nil {
			t.Errorf("Load() of concurrently written preset: %v", err)
			break
		}
		if reader.Len() != 0 {
			t.Errorf("preset has %d trailing bytes, header and body disagree", reader.Len())
			break
		}
	}

	wg.Wait()
}
