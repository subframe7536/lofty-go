package util

import (
	"bytes"
	"io"
	"math"
	"testing"
)

// --- src/util/math.rs ---

func TestUtilDivRound(t *testing.T) {
	for _, tc := range []struct{ lhs, rhs, want uint64 }{
		{1, 1, 1},
		{1, 2, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{5, 2, 3},
		{800, 1500, 1},
		{1500, 3000, 1},
		{0, 4000, 0},
		{1500, 4000, 0},
	} {
		if got := DivRound(tc.lhs, tc.rhs); got != tc.want {
			t.Errorf("%d.DivRound(%d) = %d, want %d", tc.lhs, tc.rhs, got, tc.want)
		}
	}
}

func TestUtilF80(t *testing.T) {
	nearlyEq := func(a, b float64) bool {
		if math.IsInf(a, 0) && math.IsInf(b, 0) {
			return a == b
		}
		if math.IsNaN(a) && math.IsNaN(b) {
			return math.Signbit(a) == math.Signbit(b)
		}
		return math.Abs(a-b) < math.SmallestNonzeroFloat64
	}
	for _, tc := range []struct {
		in   [10]byte
		want float64
	}{
		{[10]byte{}, 0.0},
		{[10]byte{0x7F, 0xFF}, math.Inf(1)},
		{[10]byte{0xFF, 0xFF}, math.Inf(-1)},
		{[10]byte{0x7F, 0xFF, 0x80}, math.NaN()},
		{[10]byte{0xFF, 0xFF, 0x80}, math.Copysign(math.NaN(), -1)},
		{[10]byte{0x3F, 0xFC, 0x80}, 0.125},
		{[10]byte{0x3F, 0xFF, 0x80}, 1.0},
		{[10]byte{0x40, 0x00, 0x80}, 2.0},
		{[10]byte{0x40, 0x00, 0xC0}, 3.0},
		{[10]byte{0x40, 0x0E, 0xBB, 0x80}, 48000.0},
	} {
		if got := F80FromBEBytes(tc.in).AsF64(); !nearlyEq(got, tc.want) {
			t.Errorf("F80(%x) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- src/id3/v2/util/synchsafe.rs ---

func TestSynchsafeIntegers(t *testing.T) {
	// synch
	if got := Synchsafe32(0x7F); got != [4]byte{0, 0, 0, 0x7F} {
		t.Errorf("synch u8 = %x", got)
	}
	if got := Synchsafe32(0x3FFF); got != [4]byte{0, 0, 0x7F, 0x7F} {
		t.Errorf("synch u16 = %x", got)
	}
	if got := Synchsafe32(0x0FFFFFFF); got != [4]byte{0x7F, 0x7F, 0x7F, 0x7F} {
		t.Errorf("synch u32 = %x", got)
	}
	// unsynch
	if got := Unsynchsafe32([]byte{0, 0, 0, 0x7F}); got != 0x7F {
		t.Errorf("unsynch u8 = %x", got)
	}
	if got := Unsynchsafe32([]byte{0, 0, 0x7F, 0x7F}); got != 0x3FFF {
		t.Errorf("unsynch u16 = %x", got)
	}
	if got := Unsynchsafe32([]byte{0x7F, 0x7F, 0x7F, 0x7F}); got != 0x0FFFFFFF {
		t.Errorf("unsynch u32 = %x", got)
	}
}

// --- src/id3/v2/util/pairs.rs (set_number) ---

func TestIoRevSearch(t *testing.T) {
	pat := []byte("PATTERN")
	mk := func(extra ...[]byte) *bytes.Reader {
		data := append([]byte(nil), pat...)
		for _, e := range extra {
			data = append(data, e...)
		}
		return bytes.NewReader(data)
	}

	// Basic search
	ok, err := NewRevPatternSearcher(mk(make([]byte, 5000)), pat).Search()
	if err != nil || !ok {
		t.Fatalf("basic search: %v %v", ok, err)
	}

	// Search across boundaries (pattern + 1023 zeros crosses the 1024 buffer)
	ok, err = NewRevPatternSearcher(mk(make([]byte, 1023)), pat).Search()
	if err != nil || !ok {
		t.Fatalf("boundary search: %v %v", ok, err)
	}

	// Multiple occurrences -> finds the last one
	const junkLen = 20
	data3 := append(append(append(append([]byte(nil), pat...), make([]byte, junkLen)...), pat...), make([]byte, junkLen)...)
	want := uint64(len(data3) - junkLen - len(pat))
	stream3 := bytes.NewReader(data3)
	ok, err = NewRevPatternSearcher(stream3, pat).Search()
	if err != nil || !ok {
		t.Fatalf("last-occurrence search: %v %v", ok, err)
	}
	if pos, _ := stream3.Seek(0, io.SeekCurrent); uint64(pos) != want {
		t.Fatalf("pos = %d, want %d", pos, want)
	}

	// Multiple occurrences, search starts within a partial match
	data4 := append(append(append(append(append([]byte(nil), pat...), make([]byte, junkLen)...), pat...), make([]byte, junkLen)...), pat...)
	middleMatchOffset := uint64(len(pat) + junkLen)
	stream4 := bytes.NewReader(data4)
	// Eat partially into the last match
	if _, err := stream4.Seek(-int64(len(pat)-3), io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	ok, err = NewRevPatternSearcher(stream4, pat).
		WithStart(RevSearchFromCurrent).
		WithEnd(RevSearchEndStreamStart).Search()
	if err != nil || !ok {
		t.Fatalf("partial-match search: %v %v", ok, err)
	}
	if pos, _ := stream4.Seek(0, io.SeekCurrent); uint64(pos) != middleMatchOffset {
		t.Fatalf("pos = %d, want %d", pos, middleMatchOffset)
	}
}

func applyRange(t *testing.T, input []byte, start, end uint64, replacement []byte) {
	t.Helper()
	expected := append([]byte(nil), input[:start]...)
	expected = append(expected, replacement...)
	expected = append(expected, input[end:]...)

	bf := NewBytesFile(append([]byte(nil), input...))
	if err := replaceRange(bf, uint64(len(input)), start, end, replacement); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bf.Bytes(), expected) {
		t.Fatalf("got %q, want %q", bf.Bytes(), expected)
	}
}

func TestReplaceRange(t *testing.T) {
	applyRange(t, []byte("0123456789"), 2, 5, []byte("XYZ"))
	applyRange(t, []byte("0123456789"), 2, 5, []byte("abcdef"))
	applyRange(t, []byte("0123456789"), 2, 8, []byte("X"))

	// Across multiple buffers
	const moveBufferSize = 1 << 16 // matches util_io buffer
	big := append([]byte(nil), []byte("prefix")...)
	for i := 0; i < 3*moveBufferSize+17; i++ {
		big = append(big, byte(i%251))
	}
	applyRange(t, big, 1, 4, []byte("a much longer metadata replacement"))
	applyRange(t, big, 1, 4, []byte("x"))
}
