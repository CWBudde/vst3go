package vst3

// #include "../../include/vst3/vst3_c_api.h"
// #include <stdlib.h>
// #include <string.h>
//
// // GoMemoryStream is a self-contained, host-free implementation of
// // Steinberg_IBStream over a malloc'd, growable buffer. The struct starts with
// // the vtable pointer so a GoMemoryStream* is directly usable as a
// // Steinberg_IBStream* -- that is the whole point: plugin code under test can
// // be handed one of these and cannot tell it from a real host stream.
// //
// // The implementation lives in C rather than in Go callbacks because cgo is not
// // permitted in _test.go files, and because a C-side vtable keeps the object
// // valid across the C boundary without any Go pointer pinning concerns.
// typedef struct GoMemoryStream {
//     struct Steinberg_IBStreamVtbl* lpVtbl;
//     unsigned char* data;
//     Steinberg_int64 size;      // bytes of valid content
//     Steinberg_int64 capacity;  // allocated bytes
//     Steinberg_int64 pos;       // read/write cursor
//     Steinberg_uint32 refCount;
//     int seekable;              // 0 => seek/tell report kNotImplemented
// } GoMemoryStream;
//
// // Grows the allocation to hold at least `need` bytes. Newly reserved bytes are
// // zeroed so that a seek past the end followed by a write leaves a defined gap,
// // matching what real host streams do.
// static int goMemStreamGrow(GoMemoryStream* s, Steinberg_int64 need) {
//     Steinberg_int64 newCap;
//     unsigned char* p;
//     if (need <= s->capacity) {
//         return 1;
//     }
//     newCap = s->capacity > 0 ? s->capacity : 64;
//     while (newCap < need) {
//         if (newCap > (Steinberg_int64)1 << 40) {
//             return 0;
//         }
//         newCap *= 2;
//     }
//     p = (unsigned char*)realloc(s->data, (size_t)newCap);
//     if (p == NULL) {
//         return 0;
//     }
//     memset(p + s->size, 0, (size_t)(newCap - s->size));
//     s->data = p;
//     s->capacity = newCap;
//     return 1;
// }
//
// static Steinberg_tresult SMTG_STDMETHODCALLTYPE goMemStreamQueryInterface(
//         void* thisInterface, const Steinberg_TUID iid, void** obj) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     if (s == NULL || obj == NULL) {
//         return Steinberg_kInvalidArgument;
//     }
//     if (memcmp(iid, Steinberg_IBStream_iid, sizeof(Steinberg_TUID)) == 0 ||
//         memcmp(iid, Steinberg_FUnknown_iid, sizeof(Steinberg_TUID)) == 0) {
//         s->refCount++;
//         *obj = s;
//         return Steinberg_kResultOk;
//     }
//     *obj = NULL;
//     return Steinberg_kNoInterface;
// }
//
// static Steinberg_uint32 SMTG_STDMETHODCALLTYPE goMemStreamAddRef(void* thisInterface) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     if (s == NULL) {
//         return 0;
//     }
//     return ++s->refCount;
// }
//
// static Steinberg_uint32 SMTG_STDMETHODCALLTYPE goMemStreamRelease(void* thisInterface) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     if (s == NULL) {
//         return 0;
//     }
//     if (--s->refCount == 0) {
//         free(s->data);
//         free(s);
//         return 0;
//     }
//     return s->refCount;
// }
//
// static Steinberg_tresult SMTG_STDMETHODCALLTYPE goMemStreamRead(
//         void* thisInterface, void* buffer, Steinberg_int32 numBytes, Steinberg_int32* numBytesRead) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     Steinberg_int64 avail;
//     Steinberg_int32 n;
//     if (s == NULL || numBytes < 0 || (buffer == NULL && numBytes > 0)) {
//         return Steinberg_kInvalidArgument;
//     }
//     avail = s->size - s->pos;
//     if (avail < 0) {
//         avail = 0;
//     }
//     n = (avail < (Steinberg_int64)numBytes) ? (Steinberg_int32)avail : numBytes;
//     if (n > 0) {
//         memcpy(buffer, s->data + s->pos, (size_t)n);
//         s->pos += n;
//     }
//     if (numBytesRead != NULL) {
//         *numBytesRead = n;
//     }
//     return Steinberg_kResultOk;
// }
//
// static Steinberg_tresult SMTG_STDMETHODCALLTYPE goMemStreamWrite(
//         void* thisInterface, void* buffer, Steinberg_int32 numBytes, Steinberg_int32* numBytesWritten) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     if (s == NULL || numBytes < 0 || (buffer == NULL && numBytes > 0)) {
//         return Steinberg_kInvalidArgument;
//     }
//     if (numBytes > 0) {
//         if (!goMemStreamGrow(s, s->pos + numBytes)) {
//             return Steinberg_kOutOfMemory;
//         }
//         memcpy(s->data + s->pos, buffer, (size_t)numBytes);
//         s->pos += numBytes;
//         if (s->pos > s->size) {
//             s->size = s->pos;
//         }
//     }
//     if (numBytesWritten != NULL) {
//         *numBytesWritten = numBytes;
//     }
//     return Steinberg_kResultOk;
// }
//
// static Steinberg_tresult SMTG_STDMETHODCALLTYPE goMemStreamSeek(
//         void* thisInterface, Steinberg_int64 pos, Steinberg_int32 mode, Steinberg_int64* result) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     Steinberg_int64 base;
//     Steinberg_int64 target;
//     if (s == NULL) {
//         return Steinberg_kInvalidArgument;
//     }
//     if (!s->seekable) {
//         return Steinberg_kNotImplemented;
//     }
//     switch (mode) {
//         case Steinberg_IBStream_IStreamSeekMode_kIBSeekSet: base = 0;       break;
//         case Steinberg_IBStream_IStreamSeekMode_kIBSeekCur: base = s->pos;  break;
//         case Steinberg_IBStream_IStreamSeekMode_kIBSeekEnd: base = s->size; break;
//         default: return Steinberg_kInvalidArgument;
//     }
//     if ((pos > 0 && base > INT64_MAX - pos) || (pos < 0 && base < INT64_MIN - pos)) {
//         return Steinberg_kInvalidArgument;
//     }
//     target = base + pos;
//     if (target < 0) {
//         return Steinberg_kInvalidArgument;
//     }
//     s->pos = target;
//     if (result != NULL) {
//         *result = target;
//     }
//     return Steinberg_kResultOk;
// }
//
// static Steinberg_tresult SMTG_STDMETHODCALLTYPE goMemStreamTell(void* thisInterface, Steinberg_int64* pos) {
//     GoMemoryStream* s = (GoMemoryStream*)thisInterface;
//     if (s == NULL || pos == NULL) {
//         return Steinberg_kInvalidArgument;
//     }
//     if (!s->seekable) {
//         return Steinberg_kNotImplemented;
//     }
//     *pos = s->pos;
//     return Steinberg_kResultOk;
// }
//
// static struct Steinberg_IBStreamVtbl goMemStreamVtbl = {
//     goMemStreamQueryInterface,
//     goMemStreamAddRef,
//     goMemStreamRelease,
//     goMemStreamRead,
//     goMemStreamWrite,
//     goMemStreamSeek,
//     goMemStreamTell
// };
//
// static GoMemoryStream* goMemStreamNew(int seekable) {
//     GoMemoryStream* s = (GoMemoryStream*)calloc(1, sizeof(GoMemoryStream));
//     if (s == NULL) {
//         return NULL;
//     }
//     s->lpVtbl = &goMemStreamVtbl;
//     s->refCount = 1;
//     s->seekable = seekable;
//     return s;
// }
import "C"

import (
	"unsafe"
)

// Seek origins. The values match both Steinberg_IBStream_IStreamSeekMode and
// io.SeekStart/SeekCurrent/SeekEnd, so StreamWrapper.Seek satisfies io.Seeker.
const (
	// SeekSet positions relative to the start of the stream.
	SeekSet = 0
	// SeekCur positions relative to the current cursor.
	SeekCur = 1
	// SeekEnd positions relative to the end of the stream's content.
	SeekEnd = 2
)

// MemoryStream is an in-memory Steinberg_IBStream backed by a malloc'd buffer.
//
// It exists so that state serialization can be exercised without a host: pass
// Stream() anywhere a host IBStream pointer is expected (NewStreamWrapper, or
// straight into a C API that wants a struct Steinberg_IBStream*) and inspect the
// result with Bytes().
//
// The zero value is not usable; construct with NewMemoryStream or
// NewMemoryStreamFromBytes. A MemoryStream owns C memory and must be released
// with Close. It is not safe for concurrent use.
type MemoryStream struct {
	ptr *C.GoMemoryStream
}

// NewMemoryStream returns an empty, seekable in-memory IBStream positioned at
// offset 0. The caller must Close it.
func NewMemoryStream() *MemoryStream {
	return newMemoryStream(true)
}

// NewMemoryStreamFromBytes returns a seekable in-memory IBStream pre-filled with
// data and positioned at offset 0, ready to be read. The data is copied.
func NewMemoryStreamFromBytes(data []byte) *MemoryStream {
	m := newMemoryStream(true)
	if m == nil {
		return nil
	}
	if len(data) > 0 {
		if _, err := m.WriteAt(data); err != nil {
			m.Close()
			return nil
		}
		m.mustRewind()
	}
	return m
}

// NewUnseekableMemoryStream returns an in-memory IBStream whose seek and tell
// report kNotImplemented. Some hosts genuinely supply forward-only streams, and
// readers must cope; this makes that path reachable in tests.
func NewUnseekableMemoryStream() *MemoryStream {
	return newMemoryStream(false)
}

func newMemoryStream(seekable bool) *MemoryStream {
	var flag C.int
	if seekable {
		flag = 1
	}
	ptr := C.goMemStreamNew(flag)
	if ptr == nil {
		return nil
	}
	return &MemoryStream{ptr: ptr}
}

// Stream returns the raw Steinberg_IBStream pointer. It stays valid until Close.
// Returns nil once closed.
func (m *MemoryStream) Stream() unsafe.Pointer {
	if m == nil || m.ptr == nil {
		return nil
	}
	return unsafe.Pointer(m.ptr)
}

// Wrapper returns a StreamWrapper over this stream, for the common case of
// exercising the typed accessors.
func (m *MemoryStream) Wrapper() *StreamWrapper {
	return NewStreamWrapper(m.Stream())
}

// Bytes returns a copy of the stream's content, independent of the cursor.
func (m *MemoryStream) Bytes() []byte {
	if m == nil || m.ptr == nil || m.ptr.size <= 0 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(m.ptr.data), C.int(m.ptr.size))
}

// Len returns the number of valid bytes in the stream.
func (m *MemoryStream) Len() int64 {
	if m == nil || m.ptr == nil {
		return 0
	}
	return int64(m.ptr.size)
}

// Pos returns the current cursor position, even for unseekable streams.
func (m *MemoryStream) Pos() int64 {
	if m == nil || m.ptr == nil {
		return 0
	}
	return int64(m.ptr.pos)
}

// Rewind moves the cursor back to offset 0. It works even on unseekable streams
// because it bypasses the vtable -- test setup should not be constrained by the
// limitation it is trying to simulate.
func (m *MemoryStream) Rewind() {
	m.mustRewind()
}

func (m *MemoryStream) mustRewind() {
	if m == nil || m.ptr == nil {
		return
	}
	m.ptr.pos = 0
}

// WriteAt appends data at the current cursor, growing the buffer as needed. It
// bypasses the vtable so that an unseekable stream can still be primed.
func (m *MemoryStream) WriteAt(data []byte) (int, error) {
	if m == nil || m.ptr == nil {
		return 0, ErrNilStream
	}
	if len(data) == 0 {
		return 0, nil
	}
	var written C.Steinberg_int32
	res := C.goMemStreamWrite(unsafe.Pointer(m.ptr), unsafe.Pointer(&data[0]),
		C.Steinberg_int32(len(data)), &written)
	if res != C.Steinberg_tresult(ResultOK) {
		return 0, &StreamError{Op: "write", Code: int32(res)}
	}
	return int(written), nil
}

// Close releases the C memory. It is idempotent; further use of the stream
// returns nil/zero rather than touching freed memory.
func (m *MemoryStream) Close() {
	if m == nil || m.ptr == nil {
		return
	}
	C.goMemStreamRelease(unsafe.Pointer(m.ptr))
	m.ptr = nil
}
