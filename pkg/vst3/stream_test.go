package vst3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
)

// newRoundTripStream returns an empty seekable stream plus its wrapper.
func newRoundTripStream(t *testing.T) (*MemoryStream, *StreamWrapper) {
	t.Helper()
	m := NewMemoryStream()
	if m == nil {
		t.Fatal("NewMemoryStream returned nil")
	}
	t.Cleanup(m.Close)
	return m, m.Wrapper()
}

func TestStreamWrapperInt32RoundTrip(t *testing.T) {
	values := []int32{0, 1, -1, 42, -42, math.MaxInt32, math.MinInt32}

	m, w := newRoundTripStream(t)
	for _, v := range values {
		if err := w.WriteInt32(v); err != nil {
			t.Fatalf("WriteInt32(%d): %v", v, err)
		}
	}
	if got := m.Len(); got != int64(4*len(values)) {
		t.Fatalf("stream length = %d, want %d", got, 4*len(values))
	}

	m.Rewind()
	for _, want := range values {
		got, err := w.ReadInt32()
		if err != nil {
			t.Fatalf("ReadInt32: %v", err)
		}
		if got != want {
			t.Errorf("ReadInt32() = %d, want %d", got, want)
		}
	}
}

// TestStreamWrapperInt32Encoding pins the wire format: little-endian, 4 bytes.
func TestStreamWrapperInt32Encoding(t *testing.T) {
	m, w := newRoundTripStream(t)
	if err := w.WriteInt32(0x01020304); err != nil {
		t.Fatalf("WriteInt32: %v", err)
	}
	want := []byte{0x04, 0x03, 0x02, 0x01}
	if !bytes.Equal(m.Bytes(), want) {
		t.Errorf("encoded as %v, want %v", m.Bytes(), want)
	}
}

// TestStreamWrapperFloat64RoundTrip covers the values where a bit-preserving
// encoding differs in intent from an arithmetic one: NaN, the infinities and
// negative zero must survive unchanged.
func TestStreamWrapperFloat64RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value float64
	}{
		{name: "zero", value: 0},
		{name: "negative zero", value: math.Copysign(0, -1)},
		{name: "one", value: 1},
		{name: "negative", value: -123.456},
		{name: "tiny", value: math.SmallestNonzeroFloat64},
		{name: "huge", value: math.MaxFloat64},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "NaN", value: math.NaN()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, w := newRoundTripStream(t)
			if err := w.WriteFloat64(tt.value); err != nil {
				t.Fatalf("WriteFloat64: %v", err)
			}
			m.Rewind()
			got, err := w.ReadFloat64()
			if err != nil {
				t.Fatalf("ReadFloat64: %v", err)
			}
			// Compare bit patterns so NaN and -0 are checked exactly.
			if math.Float64bits(got) != math.Float64bits(tt.value) {
				t.Errorf("round-tripped %x, want %x (%v vs %v)",
					math.Float64bits(got), math.Float64bits(tt.value), got, tt.value)
			}
		})
	}
}

func TestStreamWrapperFloat64Encoding(t *testing.T) {
	m, w := newRoundTripStream(t)
	if err := w.WriteFloat64(1.0); err != nil {
		t.Fatalf("WriteFloat64: %v", err)
	}
	want := make([]byte, 8)
	binary.LittleEndian.PutUint64(want, math.Float64bits(1.0))
	if !bytes.Equal(m.Bytes(), want) {
		t.Errorf("encoded as %v, want %v", m.Bytes(), want)
	}
}

func TestStreamWrapperStringRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "ascii", value: "Cutoff"},
		{name: "utf8 multi-byte", value: "Frequenz Ã¼ber 1 kHz âˆ† ðŸŽ›"},
		{name: "embedded nul", value: "a\x00b"},
		{name: "long", value: strings.Repeat("x", 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, w := newRoundTripStream(t)
			if err := w.WriteString(tt.value); err != nil {
				t.Fatalf("WriteString: %v", err)
			}
			// The payload must occupy exactly the length prefix plus the bytes:
			// nothing padded, nothing truncated.
			if want := int64(4 + len(tt.value)); m.Len() != want {
				t.Fatalf("stream length = %d, want %d", m.Len(), want)
			}

			m.Rewind()
			got, err := w.ReadString()
			if err != nil {
				t.Fatalf("ReadString: %v", err)
			}
			if got != tt.value {
				t.Errorf("ReadString() = %q, want %q", got, tt.value)
			}
			// Cursor must land exactly at the end, so a following read sees EOF.
			pos, err := w.Tell()
			if err != nil {
				t.Fatalf("Tell: %v", err)
			}
			if pos != m.Len() {
				t.Errorf("cursor at %d after ReadString, want %d", pos, m.Len())
			}
		})
	}
}

func TestStreamWrapperMixedRoundTrip(t *testing.T) {
	m, w := newRoundTripStream(t)

	if err := w.WriteInt32(7); err != nil {
		t.Fatalf("WriteInt32: %v", err)
	}
	if err := w.WriteString("gain"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := w.WriteFloat64(0.75); err != nil {
		t.Fatalf("WriteFloat64: %v", err)
	}

	m.Rewind()
	if got, err := w.ReadInt32(); err != nil || got != 7 {
		t.Fatalf("ReadInt32() = %d, %v; want 7, nil", got, err)
	}
	if got, err := w.ReadString(); err != nil || got != "gain" {
		t.Fatalf("ReadString() = %q, %v; want \"gain\", nil", got, err)
	}
	if got, err := w.ReadFloat64(); err != nil || got != 0.75 {
		t.Fatalf("ReadFloat64() = %v, %v; want 0.75, nil", got, err)
	}
}

// corruptStringPayload builds a length-prefixed string blob with an arbitrary
// (possibly lying) prefix, as a corrupt preset on disk would.
func corruptStringPayload(length int32, payload []byte) []byte {
	buf := make([]byte, 4, 4+len(payload))
	binary.LittleEndian.PutUint32(buf, uint32(length))
	return append(buf, payload...)
}

func TestReadStringRejectsCorruptInput(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "negative length",
			data:    corruptStringPayload(-1, []byte("abc")),
			wantErr: ErrInvalidLength,
		},
		{
			name:    "most negative length",
			data:    corruptStringPayload(math.MinInt32, nil),
			wantErr: ErrInvalidLength,
		},
		{
			name:    "length beyond cap",
			data:    corruptStringPayload(MaxStringLength+1, []byte("abc")),
			wantErr: ErrInvalidLength,
		},
		{
			name:    "truncated payload",
			data:    corruptStringPayload(10, []byte("abc")),
			wantErr: ErrShortRead,
		},
		{
			name:    "length exceeds remaining stream",
			data:    corruptStringPayload(1<<20, []byte("abc")),
			wantErr: ErrShortRead,
		},
		{
			name:    "missing payload entirely",
			data:    corruptStringPayload(4, nil),
			wantErr: ErrShortRead,
		},
		{
			name:    "truncated length prefix",
			data:    []byte{0x01, 0x02},
			wantErr: ErrShortRead,
		},
		{
			name:    "empty stream",
			data:    nil,
			wantErr: ErrShortRead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemoryStreamFromBytes(tt.data)
			if m == nil {
				m = NewMemoryStream()
			}
			defer m.Close()

			got, err := m.Wrapper().ReadString()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadString() error = %v, want %v", err, tt.wantErr)
			}
			if got != "" {
				t.Errorf("ReadString() = %q on failure, want empty string", got)
			}
		})
	}
}

func TestReadInt32AndFloat64RejectShortInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		read func(*StreamWrapper) error
	}{
		{
			name: "int32 with 3 bytes",
			data: []byte{1, 2, 3},
			read: func(w *StreamWrapper) error { _, err := w.ReadInt32(); return err },
		},
		{
			name: "float64 with 7 bytes",
			data: []byte{1, 2, 3, 4, 5, 6, 7},
			read: func(w *StreamWrapper) error { _, err := w.ReadFloat64(); return err },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemoryStreamFromBytes(tt.data)
			defer m.Close()

			if err := tt.read(m.Wrapper()); !errors.Is(err, ErrShortRead) {
				t.Errorf("error = %v, want ErrShortRead", err)
			}
		})
	}
}

func TestWriteStringRejectsOversizedInput(t *testing.T) {
	m, w := newRoundTripStream(t)
	// Not actually allocating 16 MiB+1 of content: a string header with a bogus
	// length is enough to reach the guard, so build it honestly but cheaply.
	huge := strings.Repeat("a", MaxStringLength+1)
	if err := w.WriteString(huge); !errors.Is(err, ErrInvalidLength) {
		t.Errorf("WriteString(oversized) = %v, want ErrInvalidLength", err)
	}
	if m.Len() != 0 {
		t.Errorf("stream length = %d, want 0 (nothing should have been written)", m.Len())
	}
}

func TestReadAllSeekable(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{name: "empty", size: 0},
		{name: "small", size: 10},
		{name: "exactly one chunk", size: readChunkSize},
		{name: "several chunks", size: readChunkSize*3 + 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := make([]byte, tt.size)
			for i := range content {
				content[i] = byte(i % 251)
			}
			m := NewMemoryStreamFromBytes(content)
			if m == nil {
				m = NewMemoryStream()
			}
			defer m.Close()

			got, err := m.Wrapper().ReadAll()
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, content) && !(len(got) == 0 && len(content) == 0) {
				t.Errorf("ReadAll returned %d bytes, want %d", len(got), len(content))
			}
		})
	}
}

// TestReadAllRespectsCursor pins that ReadAll means "all remaining", not "all".
func TestReadAllRespectsCursor(t *testing.T) {
	m := NewMemoryStreamFromBytes([]byte("HEADERBODY"))
	defer m.Close()
	w := m.Wrapper()

	if _, err := w.Seek(6, SeekSet); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	got, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "BODY" {
		t.Errorf("ReadAll() = %q, want %q", got, "BODY")
	}
}

// TestReadAllAtEndRestoresPosition: probing the length must not move the cursor
// when there is nothing left to read.
func TestReadAllAtEndRestoresPosition(t *testing.T) {
	m := NewMemoryStreamFromBytes([]byte("abcdef"))
	defer m.Close()
	w := m.Wrapper()

	if _, err := w.Seek(0, SeekEnd); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	got, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ReadAll() = %q, want empty", got)
	}
	pos, err := w.Tell()
	if err != nil {
		t.Fatalf("Tell: %v", err)
	}
	if pos != 6 {
		t.Errorf("cursor moved to %d, want 6", pos)
	}
}

func TestReadAllUnseekable(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{name: "empty", size: 0},
		{name: "below chunk", size: 100},
		{name: "exactly one chunk", size: readChunkSize},
		{name: "chunk boundary plus one", size: readChunkSize + 1},
		{name: "several chunks", size: readChunkSize*2 + 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := make([]byte, tt.size)
			for i := range content {
				content[i] = byte(i % 253)
			}

			m := NewUnseekableMemoryStream()
			defer m.Close()
			if _, err := m.WriteAt(content); err != nil {
				t.Fatalf("WriteAt: %v", err)
			}
			m.Rewind()

			got, err := m.Wrapper().ReadAll()
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if len(got) != len(content) || !bytes.Equal(got, content) {
				t.Errorf("ReadAll returned %d bytes, want %d", len(got), len(content))
			}
		})
	}
}

func TestReadFullAndPartialRead(t *testing.T) {
	m := NewMemoryStreamFromBytes([]byte("abcdefgh"))
	defer m.Close()
	w := m.Wrapper()

	buf := make([]byte, 3)
	if err := w.ReadFull(buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != "abc" {
		t.Errorf("ReadFull got %q, want %q", buf, "abc")
	}

	// Zero-length reads and writes are no-ops, not errors.
	if n, err := w.Read(nil); n != 0 || err != nil {
		t.Errorf("Read(nil) = %d, %v; want 0, nil", n, err)
	}
	if n, err := w.Write(nil); n != 0 || err != nil {
		t.Errorf("Write(nil) = %d, %v; want 0, nil", n, err)
	}

	rest := make([]byte, 100)
	if err := w.ReadFull(rest); !errors.Is(err, ErrShortRead) {
		t.Errorf("ReadFull past end = %v, want ErrShortRead", err)
	}
}

// TestNilStreamWrapperNeverPanics is the important one: these methods run inside
// cgo callbacks, where a panic unwinds into C++ and kills the host.
func TestNilStreamWrapperNeverPanics(t *testing.T) {
	if w := NewStreamWrapper(nil); w != nil {
		t.Fatal("NewStreamWrapper(nil) must return nil")
	}

	// Both a nil wrapper and a non-nil wrapper around a nil stream.
	wrappers := map[string]*StreamWrapper{
		"nil wrapper":      nil,
		"nil inner stream": {},
	}

	tests := []struct {
		name string
		call func(*StreamWrapper) error
	}{
		{"Read", func(w *StreamWrapper) error { _, err := w.Read(make([]byte, 4)); return err }},
		{"ReadFull", func(w *StreamWrapper) error { return w.ReadFull(make([]byte, 4)) }},
		{"Write", func(w *StreamWrapper) error { _, err := w.Write([]byte("x")); return err }},
		{"Seek", func(w *StreamWrapper) error { _, err := w.Seek(0, SeekSet); return err }},
		{"Tell", func(w *StreamWrapper) error { _, err := w.Tell(); return err }},
		{"WriteInt32", func(w *StreamWrapper) error { return w.WriteInt32(1) }},
		{"ReadInt32", func(w *StreamWrapper) error { _, err := w.ReadInt32(); return err }},
		{"WriteFloat64", func(w *StreamWrapper) error { return w.WriteFloat64(1) }},
		{"ReadFloat64", func(w *StreamWrapper) error { _, err := w.ReadFloat64(); return err }},
		{"WriteString", func(w *StreamWrapper) error { return w.WriteString("x") }},
		{"ReadString", func(w *StreamWrapper) error { _, err := w.ReadString(); return err }},
		{"ReadAll", func(w *StreamWrapper) error { _, err := w.ReadAll(); return err }},
		{"readAllChunked", func(w *StreamWrapper) error { _, err := w.readAllChunked(); return err }},
	}

	for kind, w := range wrappers {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s panicked: %v", tt.name, r)
					}
				}()
				if err := tt.call(w); !errors.Is(err, ErrNilStream) {
					t.Errorf("%s = %v, want ErrNilStream", tt.name, err)
				}
			})
		}
	}
}

func TestStreamErrorCarriesTresult(t *testing.T) {
	m := NewUnseekableMemoryStream()
	defer m.Close()

	_, err := m.Wrapper().Tell()
	if err == nil {
		t.Fatal("Tell on unseekable stream should fail")
	}
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StreamError", err)
	}
	if se.Op != "tell" {
		t.Errorf("Op = %q, want %q", se.Op, "tell")
	}
	if se.Code == 0 {
		t.Error("Code should carry the non-OK tresult, got 0")
	}
	if !strings.Contains(se.Error(), "tell") {
		t.Errorf("Error() = %q, should mention the operation", se.Error())
	}
	if !errors.Is(err, ErrStreamFailure) {
		t.Error("StreamError should unwrap to ErrStreamFailure")
	}
	// A host refusal must be distinguishable from a corrupt preset.
	if errors.Is(err, ErrInvalidLength) || errors.Is(err, ErrShortRead) {
		t.Error("StreamError must not be confusable with payload errors")
	}
}
