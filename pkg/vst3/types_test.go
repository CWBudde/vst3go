package vst3

import (
	"bytes"
	"errors"
	"runtime"
	"testing"
	"unsafe"
)

// tuidBytes reads back the 16 bytes ToTUID hands to C.
func tuidBytes(p unsafe.Pointer) []byte {
	return bytes.Clone(unsafe.Slice((*byte)(p), 16))
}

// TestToTUIDByteOrder pins that ToTUID hands C the bytes in array order, with no
// reordering or padding. A wrong order here silently breaks queryInterface.
func TestToTUIDByteOrder(t *testing.T) {
	iid := [16]byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
	}

	ptr := ToTUID(iid)
	if ptr == nil {
		t.Fatal("ToTUID returned nil")
	}
	got := tuidBytes(ptr)
	if !bytes.Equal(got, iid[:]) {
		t.Errorf("ToTUID produced % X, want % X", got, iid)
	}
}

// TestToTUIDIsStable guards against the pointer aliasing the caller's array: the
// value is copied, so mutating the source afterwards must not be observable.
func TestToTUIDIsStable(t *testing.T) {
	iid := IIDIComponent
	ptr := ToTUID(iid)
	first := tuidBytes(ptr)

	iid[0] ^= 0xFF
	second := tuidBytes(ptr)

	if !bytes.Equal(first, second) {
		t.Errorf("ToTUID result changed after mutating the source: % X -> % X", first, second)
	}
	if !bytes.Equal(first, IIDIComponent[:]) {
		t.Errorf("ToTUID produced % X, want % X", first, IIDIComponent)
	}
}

// TestInterfaceIDsMatchSDK cross-checks the IIDs derived from the C header
// against the published VST3 values. On non-Windows builds the SDK lays a UID
// out big-endian per 32-bit group; Windows uses a COM-compatible permutation,
// which the header handles, so only the layout this platform uses is asserted.
func TestInterfaceIDsMatchSDK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("COM-compatible UID layout differs; the header macro handles it")
	}

	tests := []struct {
		name string
		got  [16]byte
		want [16]byte
	}{
		{
			name: "FUnknown",
			got:  IIDFUnknown,
			want: [16]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
			},
		},
		{
			name: "IPluginFactory",
			got:  IIDIPluginFactory,
			want: [16]byte{
				0x7A, 0x4D, 0x81, 0x1C, 0x52, 0x11, 0x4A, 0x1F,
				0xAE, 0xD9, 0xD2, 0xEE, 0x0B, 0x43, 0xBF, 0x9F,
			},
		},
		{
			name: "IComponent",
			got:  IIDIComponent,
			want: [16]byte{
				0xE8, 0x31, 0xFF, 0x31, 0xF2, 0xD5, 0x43, 0x01,
				0x92, 0x8E, 0xBB, 0xEE, 0x25, 0x69, 0x78, 0x02,
			},
		},
		{
			name: "IAudioProcessor",
			got:  IIDIAudioProcessor,
			want: [16]byte{
				0x42, 0x04, 0x3F, 0x99, 0xB7, 0xDA, 0x45, 0x3C,
				0xA5, 0x69, 0xE7, 0x9D, 0x9A, 0xAE, 0xC3, 0x3D,
			},
		},
		{
			name: "IEditController",
			got:  IIDIEditController,
			want: [16]byte{
				0xDC, 0xD7, 0xBB, 0xE3, 0x77, 0x42, 0x44, 0x8D,
				0xA8, 0x74, 0xAA, 0xCC, 0x97, 0x9C, 0x75, 0x9E,
			},
		},
		{
			name: "IBStream",
			got:  IIDIBStream,
			want: [16]byte{
				0xC3, 0xBF, 0x6E, 0xA2, 0x30, 0x99, 0x47, 0x52,
				0x9B, 0x6B, 0xF9, 0x90, 0x1E, 0xE3, 0x3E, 0x9B,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s IID = % X, want % X", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		err  Error
		want string
	}{
		{err: ErrNotImplemented, want: "not implemented"},
		{err: ErrInvalidArgument, want: "invalid argument"},
		{err: ErrNilStream, want: "nil stream"},
		{err: ErrShortRead, want: "short read"},
		{err: ErrInvalidLength, want: "invalid length"},
		{err: ErrStreamFailure, want: "stream failure"},
		{err: ErrShortWrite, want: "short write"},
		{err: Error(-9999), want: "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorsAreDistinct: the point of the sentinel set is that a caller can tell
// "the host refused" apart from "the preset is corrupt".
func TestErrorsAreDistinct(t *testing.T) {
	all := []Error{
		ErrNotImplemented, ErrInvalidArgument, ErrNilStream,
		ErrShortRead, ErrInvalidLength, ErrStreamFailure, ErrShortWrite,
	}

	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("errors.Is(%v, %v) is true; sentinels must be distinct", a, b)
			}
		}
	}
}

func TestResultConstants(t *testing.T) {
	if ResultOK != 0 {
		t.Errorf("ResultOK = %d, want 0 (kResultOk)", ResultOK)
	}
	if ResultFalse != 1 {
		t.Errorf("ResultFalse = %d, want 1 (kResultFalse)", ResultFalse)
	}
	if ResultOk != ResultOK || ResultTrue != ResultOK {
		t.Error("deprecated aliases must keep matching ResultOK")
	}
}
