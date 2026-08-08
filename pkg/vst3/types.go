package vst3

// #include "../../include/vst3/vst3_c_api.h"
import "C"
import "unsafe"

// Result codes - we need to define these as values, not C constants
const (
	ResultOK    = 0
	ResultFalse = 1

	// Deprecated aliases kept for compatibility with older callers.
	ResultOk   = ResultOK
	ResultTrue = ResultOK
)

// Basic type aliases
type (
	Result     = C.Steinberg_tresult
	TUID       = C.Steinberg_TUID
	FUnknown   = C.struct_Steinberg_FUnknown
	ParamValue = C.Steinberg_Vst_ParamValue
	ParamID    = C.Steinberg_Vst_ParamID
	Sample32   = C.Steinberg_Vst_Sample32
	Sample64   = C.Steinberg_Vst_Sample64
)

// Class categories
const (
	CategoryAudioEffect = "Audio Module Class"
)

// Error is a sentinel error class for the binding layer. Values are distinct so
// that callers can tell a host refusal apart from a corrupt payload; previously
// every failure collapsed onto ErrNotImplemented.
type Error int

const (
	// ErrNotImplemented means the host does not provide the requested operation.
	ErrNotImplemented Error = -1
	// ErrInvalidArgument means the caller supplied an unusable argument.
	ErrInvalidArgument Error = -2
	// ErrNilStream means the stream (or its wrapper) is nil.
	ErrNilStream Error = -3
	// ErrShortRead means the stream delivered fewer bytes than the format requires.
	ErrShortRead Error = -4
	// ErrInvalidLength means a length prefix read from the stream is negative or
	// exceeds the documented cap. Typically a corrupt or hostile preset.
	ErrInvalidLength Error = -5
	// ErrStreamFailure means the host stream returned a non-OK tresult. Wrapped by
	// StreamError, which carries the actual code.
	ErrStreamFailure Error = -6
	// ErrShortWrite means the stream accepted fewer bytes than were offered.
	ErrShortWrite Error = -7
)

func (e Error) Error() string {
	switch e {
	case ErrNotImplemented:
		return "not implemented"
	case ErrInvalidArgument:
		return "invalid argument"
	case ErrNilStream:
		return "nil stream"
	case ErrShortRead:
		return "short read"
	case ErrInvalidLength:
		return "invalid length"
	case ErrStreamFailure:
		return "stream failure"
	case ErrShortWrite:
		return "short write"
	default:
		return "unknown error"
	}
}

// ToTUID exposes an interface ID as a pointer to 16 contiguous bytes, the layout
// the C API's Steinberg_TUID expects.
//
// The argument is copied, so the returned pointer refers to that copy and not to
// the caller's array; it stays valid as long as the returned pointer is
// reachable. Per the cgo pointer rules it may be handed to C for the duration of
// a call, but C must not retain it.
func ToTUID(iid [16]byte) unsafe.Pointer {
	return unsafe.Pointer(&iid[0])
}
