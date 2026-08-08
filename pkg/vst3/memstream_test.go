package vst3

import (
	"bytes"
	"errors"
	"testing"
)

// newFilledStream returns a seekable memory stream containing content, cursor at 0.
func newFilledStream(t *testing.T, content []byte) *MemoryStream {
	t.Helper()
	m := NewMemoryStreamFromBytes(content)
	if m == nil {
		t.Fatal("NewMemoryStreamFromBytes returned nil")
	}
	t.Cleanup(m.Close)
	return m
}

func TestMemoryStreamEmpty(t *testing.T) {
	m := NewMemoryStream()
	if m == nil {
		t.Fatal("NewMemoryStream returned nil")
	}
	defer m.Close()

	if m.Stream() == nil {
		t.Error("Stream() must expose a non-nil IBStream pointer")
	}
	if got := m.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := m.Bytes(); got != nil {
		t.Errorf("Bytes() = %v, want nil", got)
	}
	if got := m.Pos(); got != 0 {
		t.Errorf("Pos() = %d, want 0", got)
	}
}

func TestMemoryStreamWriteThenBytes(t *testing.T) {
	m := NewMemoryStream()
	defer m.Close()

	payload := []byte("hello vst3")
	n, err := m.WriteAt(payload)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("WriteAt wrote %d bytes, want %d", n, len(payload))
	}
	if !bytes.Equal(m.Bytes(), payload) {
		t.Errorf("Bytes() = %q, want %q", m.Bytes(), payload)
	}
	if m.Len() != int64(len(payload)) {
		t.Errorf("Len() = %d, want %d", m.Len(), len(payload))
	}
}

// TestMemoryStreamGrowth pushes the buffer well past its initial capacity to
// prove the realloc path keeps earlier content intact.
func TestMemoryStreamGrowth(t *testing.T) {
	m := NewMemoryStream()
	defer m.Close()

	var want []byte
	for i := 0; i < 500; i++ {
		chunk := bytes.Repeat([]byte{byte(i)}, 37)
		if _, err := m.WriteAt(chunk); err != nil {
			t.Fatalf("WriteAt iteration %d: %v", i, err)
		}
		want = append(want, chunk...)
	}
	if !bytes.Equal(m.Bytes(), want) {
		t.Fatalf("content mismatch after growth: got %d bytes, want %d", m.Len(), len(want))
	}
}

func TestMemoryStreamSeekModes(t *testing.T) {
	content := []byte("0123456789")

	tests := []struct {
		name    string
		pos     int64
		mode    int
		want    int64
		wantErr bool
	}{
		{name: "set to start", pos: 0, mode: SeekSet, want: 0},
		{name: "set to middle", pos: 4, mode: SeekSet, want: 4},
		{name: "set past end is legal", pos: 100, mode: SeekSet, want: 100},
		{name: "set negative rejected", pos: -1, mode: SeekSet, wantErr: true},
		{name: "cur from start", pos: 3, mode: SeekCur, want: 3},
		{name: "cur negative rejected", pos: -3, mode: SeekCur, wantErr: true},
		{name: "end zero is length", pos: 0, mode: SeekEnd, want: int64(len(content))},
		{name: "end backwards", pos: -4, mode: SeekEnd, want: int64(len(content)) - 4},
		{name: "end before start rejected", pos: -100, mode: SeekEnd, wantErr: true},
		{name: "unknown mode rejected", pos: 0, mode: 99, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFilledStream(t, content)
			w := m.Wrapper()

			got, err := w.Seek(tt.pos, tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Seek(%d, %d) = %d, want error", tt.pos, tt.mode, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Seek(%d, %d): %v", tt.pos, tt.mode, err)
			}
			if got != tt.want {
				t.Errorf("Seek(%d, %d) = %d, want %d", tt.pos, tt.mode, got, tt.want)
			}
			pos, err := w.Tell()
			if err != nil {
				t.Fatalf("Tell: %v", err)
			}
			if pos != tt.want {
				t.Errorf("Tell() = %d, want %d", pos, tt.want)
			}
		})
	}
}

// TestMemoryStreamSeekCurAccumulates checks that SeekCur is relative to the
// running cursor and not silently treated as SeekSet.
func TestMemoryStreamSeekCurAccumulates(t *testing.T) {
	m := newFilledStream(t, []byte("0123456789"))
	w := m.Wrapper()

	for i, want := range []int64{2, 4, 6} {
		got, err := w.Seek(2, SeekCur)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("step %d: pos = %d, want %d", i, got, want)
		}
	}
}

// TestMemoryStreamSeekPastEndWriteZeroFills mirrors real host streams: the gap
// created by seeking beyond the end must read back as zeros, not garbage.
func TestMemoryStreamSeekPastEndWriteZeroFills(t *testing.T) {
	m := newFilledStream(t, []byte("AB"))
	w := m.Wrapper()

	if _, err := w.Seek(5, SeekSet); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if _, err := w.Write([]byte("Z")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := []byte{'A', 'B', 0, 0, 0, 'Z'}
	if !bytes.Equal(m.Bytes(), want) {
		t.Errorf("Bytes() = %v, want %v", m.Bytes(), want)
	}
}

func TestMemoryStreamReadAtEndReturnsZero(t *testing.T) {
	m := newFilledStream(t, []byte("abc"))
	w := m.Wrapper()

	if _, err := w.Seek(0, SeekEnd); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 8)
	n, err := w.Read(buf)
	if err != nil {
		t.Fatalf("Read at EOF returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Read at EOF returned %d bytes, want 0", n)
	}
}

func TestMemoryStreamCloseIsIdempotent(t *testing.T) {
	m := NewMemoryStream()
	if _, err := m.WriteAt([]byte("data")); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	m.Close()
	m.Close()
	m.Close()

	if got := m.Stream(); got != nil {
		t.Error("Stream() must be nil after Close")
	}
	if got := m.Bytes(); got != nil {
		t.Errorf("Bytes() = %v, want nil after Close", got)
	}
	if got := m.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 after Close", got)
	}
	if _, err := m.WriteAt([]byte("x")); !errors.Is(err, ErrNilStream) {
		t.Errorf("WriteAt after Close = %v, want ErrNilStream", err)
	}
	if w := m.Wrapper(); w != nil {
		t.Error("Wrapper() must be nil after Close")
	}
}

func TestUnseekableMemoryStream(t *testing.T) {
	m := NewUnseekableMemoryStream()
	if m == nil {
		t.Fatal("NewUnseekableMemoryStream returned nil")
	}
	defer m.Close()

	if _, err := m.WriteAt([]byte("payload")); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	m.Rewind()

	w := m.Wrapper()
	if _, err := w.Tell(); !errors.Is(err, ErrStreamFailure) {
		t.Errorf("Tell on unseekable stream = %v, want ErrStreamFailure", err)
	}
	if _, err := w.Seek(0, SeekSet); !errors.Is(err, ErrStreamFailure) {
		t.Errorf("Seek on unseekable stream = %v, want ErrStreamFailure", err)
	}

	// Sequential reading must still work.
	buf := make([]byte, 7)
	if err := w.ReadFull(buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != "payload" {
		t.Errorf("read %q, want %q", buf, "payload")
	}
}

// TestMemoryStreamNilReceiver guards the documented "nil is inert" contract, so
// a failed allocation cannot turn into a panic at the C boundary.
func TestMemoryStreamNilReceiver(t *testing.T) {
	var m *MemoryStream

	if m.Stream() != nil {
		t.Error("Stream() on nil should be nil")
	}
	if m.Bytes() != nil {
		t.Error("Bytes() on nil should be nil")
	}
	if m.Len() != 0 || m.Pos() != 0 {
		t.Error("Len()/Pos() on nil should be 0")
	}
	if _, err := m.WriteAt([]byte("x")); !errors.Is(err, ErrNilStream) {
		t.Errorf("WriteAt on nil = %v, want ErrNilStream", err)
	}
	m.Rewind()
	m.Close()
}
