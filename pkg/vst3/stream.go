package vst3

// #include "../../include/vst3/vst3_c_api.h"
// #include <stdlib.h>
//
// // Helper functions to work with IBStream
// static inline Steinberg_tresult stream_read(struct Steinberg_IBStream* stream, void* buffer, Steinberg_int32 numBytes, Steinberg_int32* numBytesRead) {
//     return stream->lpVtbl->read(stream, buffer, numBytes, numBytesRead);
// }
//
// static inline Steinberg_tresult stream_write(struct Steinberg_IBStream* stream, void* buffer, Steinberg_int32 numBytes, Steinberg_int32* numBytesWritten) {
//     return stream->lpVtbl->write(stream, buffer, numBytes, numBytesWritten);
// }
//
// static inline Steinberg_tresult stream_seek(struct Steinberg_IBStream* stream, Steinberg_int64 pos, Steinberg_int32 mode, Steinberg_int64* result) {
//     return stream->lpVtbl->seek(stream, pos, mode, result);
// }
//
// static inline Steinberg_tresult stream_tell(struct Steinberg_IBStream* stream, Steinberg_int64* pos) {
//     return stream->lpVtbl->tell(stream, pos);
// }
import "C"

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"
)

// Caps on allocations driven by data read from a stream. A preset comes from
// disk or from a host and is therefore untrusted input: a corrupt length prefix
// must not turn into an unbounded allocation.
const (
	// MaxStringLength is the largest length prefix ReadString will honor (16 MiB).
	// Parameter titles, unit names and preset labels are orders of magnitude below.
	MaxStringLength = 16 << 20
	// MaxStreamSize is the largest payload ReadAll will accumulate (256 MiB).
	MaxStreamSize = 256 << 20

	// readChunkSize is the buffer size used when the stream cannot be seeked and
	// its length is therefore unknown up front.
	readChunkSize = 4096
)

// StreamError reports a non-OK tresult from the host stream, preserving the code
// so callers can log or forward it. It unwraps to ErrStreamFailure.
type StreamError struct {
	Op   string
	Code int32
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("vst3: stream %s failed with tresult 0x%08X", e.Op, uint32(e.Code))
}

// Unwrap lets errors.Is(err, ErrStreamFailure) match any host stream failure.
func (e *StreamError) Unwrap() error { return ErrStreamFailure }

// StreamWrapper wraps VST3 IBStream for Go usage.
//
// Every method tolerates a nil receiver and a nil underlying stream, returning
// ErrNilStream. This matters because these methods run inside cgo callbacks: a
// panic there unwinds into C++ and takes the host down.
type StreamWrapper struct {
	stream *C.struct_Steinberg_IBStream
}

// NewStreamWrapper creates a wrapper for an IBStream. Returns nil for a nil
// pointer; the resulting nil *StreamWrapper is still safe to call.
func NewStreamWrapper(streamPtr unsafe.Pointer) *StreamWrapper {
	if streamPtr == nil {
		return nil
	}
	return &StreamWrapper{
		stream: (*C.struct_Steinberg_IBStream)(streamPtr),
	}
}

// ok reports whether the wrapper can be used at all.
func (s *StreamWrapper) ok() bool {
	return s != nil && s.stream != nil
}

// Read reads data from the stream. A short read is not an error here; callers
// that require exact counts check the returned length.
func (s *StreamWrapper) Read(buffer []byte) (int32, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	if len(buffer) == 0 {
		return 0, nil
	}

	n := len(buffer)
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}

	var numBytesRead C.Steinberg_int32
	result := C.stream_read(s.stream, unsafe.Pointer(&buffer[0]), C.Steinberg_int32(n), &numBytesRead)
	if result != C.Steinberg_tresult(ResultOK) {
		return 0, &StreamError{Op: "read", Code: int32(result)}
	}
	if numBytesRead < 0 {
		return 0, ErrInvalidLength
	}
	return int32(numBytesRead), nil
}

// ReadFull reads exactly len(buffer) bytes, looping over short reads.
func (s *StreamWrapper) ReadFull(buffer []byte) error {
	if !s.ok() {
		return ErrNilStream
	}
	for read := 0; read < len(buffer); {
		n, err := s.Read(buffer[read:])
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrShortRead
		}
		read += int(n)
	}
	return nil
}

// Write writes data to the stream.
func (s *StreamWrapper) Write(buffer []byte) (int32, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	if len(buffer) == 0 {
		return 0, nil
	}

	n := len(buffer)
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}

	var numBytesWritten C.Steinberg_int32
	result := C.stream_write(s.stream, unsafe.Pointer(&buffer[0]), C.Steinberg_int32(n), &numBytesWritten)
	if result != C.Steinberg_tresult(ResultOK) {
		return 0, &StreamError{Op: "write", Code: int32(result)}
	}
	if int(numBytesWritten) != n {
		return int32(numBytesWritten), ErrShortWrite
	}
	return int32(numBytesWritten), nil
}

// Seek moves the cursor and returns the resulting absolute position. Hosts are
// allowed to not support seeking, in which case the error unwraps to
// ErrStreamFailure with the host's tresult.
func (s *StreamWrapper) Seek(pos int64, mode int) (int64, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	var result C.Steinberg_int64
	res := C.stream_seek(s.stream, C.Steinberg_int64(pos), C.Steinberg_int32(mode), &result)
	if res != C.Steinberg_tresult(ResultOK) {
		return 0, &StreamError{Op: "seek", Code: int32(res)}
	}
	return int64(result), nil
}

// Tell returns the current cursor position.
func (s *StreamWrapper) Tell() (int64, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	var pos C.Steinberg_int64
	res := C.stream_tell(s.stream, &pos)
	if res != C.Steinberg_tresult(ResultOK) {
		return 0, &StreamError{Op: "tell", Code: int32(res)}
	}
	return int64(pos), nil
}

// WriteInt32 writes a little-endian int32 to the stream.
func (s *StreamWrapper) WriteInt32(value int32) error {
	if !s.ok() {
		return ErrNilStream
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(value))
	_, err := s.Write(buf)
	return err
}

// ReadInt32 reads a little-endian int32 from the stream.
func (s *StreamWrapper) ReadInt32() (int32, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	buf := make([]byte, 4)
	if err := s.ReadFull(buf); err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(buf)), nil
}

// WriteFloat64 writes a little-endian IEEE-754 float64 to the stream.
func (s *StreamWrapper) WriteFloat64(value float64) error {
	if !s.ok() {
		return ErrNilStream
	}
	buf := make([]byte, 8)
	// math.Float64bits is a compiler intrinsic; the equivalent unsafe.Pointer
	// cast is neither cheaper nor safe under the unsafe rules.
	binary.LittleEndian.PutUint64(buf, math.Float64bits(value))
	_, err := s.Write(buf)
	return err
}

// ReadFloat64 reads a little-endian IEEE-754 float64 from the stream. NaN,
// infinities and negative zero round-trip bit-exactly.
func (s *StreamWrapper) ReadFloat64() (float64, error) {
	if !s.ok() {
		return 0, ErrNilStream
	}
	buf := make([]byte, 8)
	if err := s.ReadFull(buf); err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(buf)), nil
}

// WriteString writes a string to the stream with an int32 length prefix.
func (s *StreamWrapper) WriteString(str string) error {
	if !s.ok() {
		return ErrNilStream
	}
	if len(str) > MaxStringLength {
		return ErrInvalidLength
	}
	if err := s.WriteInt32(int32(len(str))); err != nil {
		return err
	}
	if str == "" {
		return nil
	}
	_, err := s.Write([]byte(str))
	return err
}

// ReadString reads a length-prefixed string. The length comes from untrusted
// data, so negative values and anything beyond MaxStringLength are rejected
// before allocating.
func (s *StreamWrapper) ReadString() (string, error) {
	if !s.ok() {
		return "", ErrNilStream
	}
	length, err := s.ReadInt32()
	if err != nil {
		return "", err
	}
	if length < 0 || length > MaxStringLength {
		return "", ErrInvalidLength
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if err := s.ReadFull(buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// ReadAll reads all remaining data from the stream.
func (s *StreamWrapper) ReadAll() ([]byte, error) {
	if !s.ok() {
		return nil, ErrNilStream
	}

	currentPos, err := s.Tell()
	if err != nil {
		// No tell support: fall back to reading until the stream runs dry.
		return s.readAllChunked()
	}

	endPos, err := s.Seek(0, SeekEnd)
	if err != nil {
		return s.readAllChunked()
	}

	// int64 throughout: truncating to int32 here used to turn a large stream into
	// a negative or wrapped length.
	remaining := endPos - currentPos
	if remaining <= 0 {
		// Still restore the cursor -- seeking to the end must not be observable.
		if _, err := s.Seek(currentPos, SeekSet); err != nil {
			return nil, err
		}
		return []byte{}, nil
	}
	if remaining > MaxStreamSize {
		return nil, ErrInvalidLength
	}

	if _, err := s.Seek(currentPos, SeekSet); err != nil {
		return s.readAllChunked()
	}

	buffer := make([]byte, remaining)
	total := 0
	for total < len(buffer) {
		n, err := s.Read(buffer[total:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break // Stream ended earlier than its reported size.
		}
		total += int(n)
	}

	return buffer[:total], nil
}

// readAllChunked reads data in chunks when seeking is not supported.
func (s *StreamWrapper) readAllChunked() ([]byte, error) {
	var result []byte
	chunk := make([]byte, readChunkSize)

	for {
		n, err := s.Read(chunk)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		if len(result)+int(n) > MaxStreamSize {
			return nil, ErrInvalidLength
		}
		result = append(result, chunk[:n]...)
		if int(n) < len(chunk) {
			break // End of stream
		}
	}

	return result, nil
}
