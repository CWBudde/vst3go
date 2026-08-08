package plugin

// #cgo CFLAGS: -I../../include
// #include "../../include/vst3/vst3_c_api.h"
//
// // Helper to access channelBuffers32 from the union
// static inline float** getChannelBuffers32(struct Steinberg_Vst_AudioBusBuffers* bus) {
//     return bus->Steinberg_Vst_AudioBusBuffers_channelBuffers32;
// }
import "C"

import (
	"unicode/utf16"
	"unsafe"
)

// maxTCharScan bounds how far stringFromTChar will look for a terminator.
// VST3's String128 is the largest TChar buffer the host hands us; a host that
// passes an unterminated buffer must not send us reading off the end of it.
const maxTCharScan = 128

// Aliases for the two host character types. cgo forbids _test.go files from
// naming C types, so the package's tests use these to build destinations for
// the copy helpers.
type (
	tcharAlias = C.Steinberg_Vst_TChar
	char8Alias = C.char
)

// getChannelBuffers32 extracts the 32-bit channel buffers from an audio bus
func getChannelBuffers32(bus *C.struct_Steinberg_Vst_AudioBusBuffers) **C.float {
	return C.getChannelBuffers32(bus)
}

// copyStringToChar8 copies a Go string into a fixed-size host char8 buffer,
// truncating on a rune boundary and zeroing the remainder. maxLen is the full
// capacity of dst including the null terminator.
func copyStringToChar8(dst *C.char, maxLen int, src string) {
	if dst == nil || maxLen <= 0 {
		return
	}

	putUTF8(unsafe.Slice((*byte)(unsafe.Pointer(dst)), maxLen), src)
}

// copyStringToTChar copies a Go string to a VST3 TChar (UTF-16) buffer.
// maxLen is the capacity of dst in TChars, including the null terminator.
func copyStringToTChar(src string, dst *C.Steinberg_Vst_TChar, maxLen int) {
	if dst == nil || maxLen <= 0 {
		return
	}

	putUTF16(unsafe.Slice((*uint16)(unsafe.Pointer(dst)), maxLen), src)
}

// stringFromTChar converts a VST3 TChar (UTF-16) buffer to a Go string.
// The scan is bounded by maxTCharScan so an unterminated host buffer cannot
// cause an out-of-bounds read.
func stringFromTChar(src *C.Steinberg_Vst_TChar) string {
	if src == nil {
		return ""
	}

	return takeUTF16(unsafe.Slice((*uint16)(unsafe.Pointer(src)), maxTCharScan))
}

// putUTF8 writes src into dst as a null-terminated UTF-8 string and zeroes the
// remainder. Split out from copyStringToChar8 so it is reachable from tests,
// which cgo does not allow to name C types.
func putUTF8(dst []byte, src string) {
	// Truncate on a rune boundary so we never leave a partial UTF-8 sequence.
	limit := len(dst) - 1
	if len(src) > limit {
		for limit > 0 && src[limit]&0xC0 == 0x80 {
			limit--
		}
		src = src[:limit]
	}

	n := copy(dst, src)
	clear(dst[n:])
}

// putUTF16 writes src into dst as a null-terminated UTF-16 string.
func putUTF16(dst []uint16, src string) {
	// utf16.Encode handles surrogate pairs; casting runes directly would
	// corrupt anything outside the BMP.
	encoded := utf16.Encode([]rune(src))
	if len(encoded) > len(dst)-1 {
		encoded = encoded[:len(dst)-1]
		// Don't leave a dangling high surrogate at the end.
		if n := len(encoded); n > 0 && encoded[n-1] >= 0xD800 && encoded[n-1] <= 0xDBFF {
			encoded = encoded[:n-1]
		}
	}

	n := copy(dst, encoded)
	dst[n] = 0
}

// takeUTF16 decodes the null-terminated UTF-16 string at the start of src,
// stopping at the end of src if no terminator is present.
func takeUTF16(src []uint16) string {
	length := 0
	for length < len(src) && src[length] != 0 {
		length++
	}

	return string(utf16.Decode(src[:length]))
}
