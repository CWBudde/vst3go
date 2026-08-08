package plugin

// Note: cgo is not permitted in _test.go files, so these tests exercise the
// Go-typed cores that the cgo-facing helpers delegate to.

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// String128 is the largest TChar buffer a host hands us, and stringFromTChar
// always scans that far, so tests size their buffers to match.
const testTCharLen = 128

func TestPutUTF16RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"ascii", "Cutoff"},
		{"latin1", "Größe"},
		{"cjk", "音量パラメータ"},
		{"astral", "gain \U0001F50A level"},
		{"accents", "égaliseur"},
		{"exact fit", strings.Repeat("a", testTCharLen-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]uint16, testTCharLen)
			putUTF16(buf, tt.src)

			if got := takeUTF16(buf); got != tt.src {
				t.Errorf("round trip = %q, want %q", got, tt.src)
			}
		})
	}
}

func TestPutUTF16Truncates(t *testing.T) {
	buf := make([]uint16, testTCharLen)

	// 200 BMP runes encode to 200 UTF-16 units; only 127 plus a terminator fit.
	putUTF16(buf, strings.Repeat("x", 200))

	got := takeUTF16(buf)
	if len(got) != testTCharLen-1 {
		t.Errorf("truncated length = %d, want %d", len(got), testTCharLen-1)
	}
	if buf[testTCharLen-1] != 0 {
		t.Errorf("buffer not terminated: last unit = %d", buf[testTCharLen-1])
	}
}

func TestPutUTF16DoesNotSplitSurrogatePair(t *testing.T) {
	buf := make([]uint16, testTCharLen)

	// 126 filler units plus a 2-unit astral rune is 128 units, one too many.
	// The pair must be dropped whole rather than halved into a lone surrogate.
	putUTF16(buf, strings.Repeat("x", 126)+"\U0001F50A")

	got := takeUTF16(buf)
	if got != strings.Repeat("x", 126) {
		t.Errorf("got %q, want 126 filler runes with the astral rune dropped", got)
	}
	for _, unit := range utf16.Encode([]rune(got)) {
		if unit >= 0xD800 && unit <= 0xDFFF {
			t.Fatalf("result contains a lone surrogate: %04x", unit)
		}
	}
}

func TestPutUTF16AstralAtExactFit(t *testing.T) {
	buf := make([]uint16, testTCharLen)

	// 125 filler plus a 2-unit astral rune is 127 units, which fits exactly.
	src := strings.Repeat("x", 125) + "\U0001F50A"
	putUTF16(buf, src)

	if got := takeUTF16(buf); got != src {
		t.Errorf("got %q, want %q", got, src)
	}
}

func TestPutUTF16SingleUnitBuffer(t *testing.T) {
	buf := []uint16{0xFFFF}
	putUTF16(buf, "anything")

	if buf[0] != 0 {
		t.Errorf("one-TChar buffer = %d, want a bare terminator", buf[0])
	}
}

func TestCopyStringToTCharRejectsBadDestination(t *testing.T) {
	// A nil or non-positive destination must be a no-op, not a crash.
	copyStringToTChar("value", nil, testTCharLen)

	buf := make([]uint16, testTCharLen)
	buf[0] = 0x41
	dst := tcharPtr(buf)

	copyStringToTChar("value", dst, 0)
	copyStringToTChar("value", dst, -1)
	if buf[0] != 0x41 {
		t.Errorf("non-positive length wrote to the buffer: buf[0] = %d", buf[0])
	}

	// Sanity check that the same pointer does work with a valid length.
	copyStringToTChar("ok", dst, len(buf))
	if got := stringFromTChar(dst); got != "ok" {
		t.Errorf("valid copy = %q, want %q", got, "ok")
	}
}

func TestStringFromTCharNil(t *testing.T) {
	if got := stringFromTChar(nil); got != "" {
		t.Errorf("stringFromTChar(nil) = %q, want empty", got)
	}
}

func TestTakeUTF16Unterminated(t *testing.T) {
	buf := make([]uint16, maxTCharScan)
	for i := range buf {
		buf[i] = 'z'
	}

	// No terminator anywhere: the scan must stop at the buffer bound.
	if got := takeUTF16(buf); len(got) != maxTCharScan {
		t.Errorf("length = %d, want %d", len(got), maxTCharScan)
	}
	if got := stringFromTChar(tcharPtr(buf)); len(got) != maxTCharScan {
		t.Errorf("stringFromTChar length = %d, want %d", len(got), maxTCharScan)
	}
}

func TestTakeUTF16Empty(t *testing.T) {
	if got := takeUTF16(nil); got != "" {
		t.Errorf("takeUTF16(nil) = %q, want empty", got)
	}
}

func TestPutUTF8(t *testing.T) {
	const size = 16

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"empty", "", ""},
		{"fits", "vendor", "vendor"},
		{"exact fit", "123456789012345", "123456789012345"},
		// The 15th and 16th bytes are the two halves of "ä"; cutting at the
		// terminator position would split it, so the whole rune has to go.
		{"rune boundary", "12345678901234ä", "12345678901234"},
		// Here the trailing rune ends exactly at the last writable byte.
		{"multibyte exact fit", "1234567890123ä", "1234567890123ä"},
		{"multibyte overflow", strings.Repeat("ä", 16), strings.Repeat("ä", 7)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, size)
			// Pre-fill with garbage to prove the remainder gets zeroed.
			for i := range buf {
				buf[i] = 0xFF
			}

			putUTF8(buf, tt.src)

			if got := string(buf[:len(tt.want)]); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			for i := len(tt.want); i < size; i++ {
				if buf[i] != 0 {
					t.Errorf("byte %d not zeroed: %#x", i, buf[i])
				}
			}
		})
	}
}

func TestCopyStringToChar8RejectsBadDestination(t *testing.T) {
	copyStringToChar8(nil, 64, "value")

	buf := make([]byte, 8)
	buf[0] = 0xFF
	copyStringToChar8(char8Ptr(buf), 0, "value")
	if buf[0] != 0xFF {
		t.Errorf("zero-length copy wrote to the buffer")
	}
}

// synthBuses builds a channel pointer table per bus over Go-allocated audio,
// mirroring the layout AudioBusBuffers hands us.
type synthBuses struct {
	tables  [][]*float32
	storage [][][]float32
}

func newSynthBuses(numSamples int, channelCounts ...int) *synthBuses {
	s := &synthBuses{}

	next := float32(1)
	for _, numChannels := range channelCounts {
		table := make([]*float32, numChannels)
		bus := make([][]float32, numChannels)

		for ch := 0; ch < numChannels; ch++ {
			samples := make([]float32, numSamples)
			for i := range samples {
				samples[i] = next
			}
			next++
			bus[ch] = samples
			if numSamples > 0 {
				table[ch] = &samples[0]
			}
		}

		s.tables = append(s.tables, table)
		s.storage = append(s.storage, bus)
	}

	return s
}

func (s *synthBuses) tablePtr(bus int) **float32 {
	if len(s.tables[bus]) == 0 {
		return nil
	}
	return &s.tables[bus][0]
}

func TestAppendBusChannelsMultipleBuses(t *testing.T) {
	const numSamples = 64

	// More than 16 channels in a single bus: this is the shape that used to
	// panic when the channel pointer table was read as a fixed-size array.
	buses := newSynthBuses(numSamples, 20, 1, 8)

	var dst [][]float32
	for i := range buses.tables {
		dst = appendBusChannels(dst, buses.tablePtr(i), len(buses.tables[i]), numSamples)
	}

	if len(dst) != 29 {
		t.Fatalf("channel count = %d, want 29", len(dst))
	}

	for i, channel := range dst {
		if len(channel) != numSamples {
			t.Fatalf("channel %d length = %d, want %d", i, len(channel), numSamples)
		}
		want := float32(i + 1)
		if channel[0] != want || channel[numSamples-1] != want {
			t.Errorf("channel %d = %v/%v, want %v", i, channel[0], channel[numSamples-1], want)
		}
	}

	// The slices must alias the host's memory rather than copy it, otherwise
	// the plugin's output would never reach the host. Channel 25 is bus 2's
	// channel 4 (20 + 1 preceding channels).
	dst[25][3] = -1
	if buses.storage[2][4][3] != -1 {
		t.Errorf("mapped buffer is a copy: source sample = %v, want -1", buses.storage[2][4][3])
	}
}

func TestAppendBusChannelsSkipsEmptyBuses(t *testing.T) {
	const numSamples = 32

	buses := newSynthBuses(numSamples, 2)

	var dst [][]float32
	// A bus the host reports as unused: nil table and/or zero channels.
	dst = appendBusChannels(dst, nil, 4, numSamples)
	dst = appendBusChannels(dst, buses.tablePtr(0), 0, numSamples)
	dst = appendBusChannels(dst, buses.tablePtr(0), -1, numSamples)
	dst = appendBusChannels(dst, buses.tablePtr(0), 2, numSamples)

	if len(dst) != 2 {
		t.Fatalf("channel count = %d, want 2", len(dst))
	}
}

func TestAppendBusChannelsSkipsNilChannels(t *testing.T) {
	const numSamples = 32

	buses := newSynthBuses(numSamples, 3)
	// A host may leave an individual channel pointer null.
	buses.tables[0][1] = nil

	dst := appendBusChannels(nil, buses.tablePtr(0), 3, numSamples)
	if len(dst) != 2 {
		t.Fatalf("channel count = %d, want 2", len(dst))
	}
	if dst[0][0] != 1 || dst[1][0] != 3 {
		t.Errorf("wrong channels kept: %v, %v", dst[0][0], dst[1][0])
	}
}

func tcharPtr(buf []uint16) *tcharAlias {
	return (*tcharAlias)(unsafe.Pointer(&buf[0]))
}

func char8Ptr(buf []byte) *char8Alias {
	return (*char8Alias)(unsafe.Pointer(&buf[0]))
}
