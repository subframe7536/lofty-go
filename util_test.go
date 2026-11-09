package lofty

import (
	"bytes"
	"io"
	"math"
	"os"
	"testing"
)

// --- src/util/math.rs ---

func TestUtilDivRound(t *testing.T) {
	for _, tc := range []struct{ lhs, rhs, want uint64 }{
		{1, 1, 1}, {1, 2, 1}, {2, 2, 1}, {3, 2, 2}, {4, 2, 2}, {5, 2, 3},
		{800, 1500, 1}, {1500, 3000, 1},
		{0, 4000, 0}, {1500, 4000, 0},
	} {
		if got := divRound(tc.lhs, tc.rhs); got != tc.want {
			t.Errorf("%d.divRound(%d) = %d, want %d", tc.lhs, tc.rhs, got, tc.want)
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
		if got := f80FromBEBytes(tc.in).asF64(); !nearlyEq(got, tc.want) {
			t.Errorf("f80(%x) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- src/id3/v2/util/synchsafe.rs ---

func TestSynchsafeIntegers(t *testing.T) {
	// synch
	if got := synchsafe32(0x7F); got != [4]byte{0, 0, 0, 0x7F} {
		t.Errorf("synch u8 = %x", got)
	}
	if got := synchsafe32(0x3FFF); got != [4]byte{0, 0, 0x7F, 0x7F} {
		t.Errorf("synch u16 = %x", got)
	}
	if got := synchsafe32(0x0FFFFFFF); got != [4]byte{0x7F, 0x7F, 0x7F, 0x7F} {
		t.Errorf("synch u32 = %x", got)
	}
	// unsynch
	if got := unsynchsafe32([]byte{0, 0, 0, 0x7F}); got != 0x7F {
		t.Errorf("unsynch u8 = %x", got)
	}
	if got := unsynchsafe32([]byte{0, 0, 0x7F, 0x7F}); got != 0x3FFF {
		t.Errorf("unsynch u16 = %x", got)
	}
	if got := unsynchsafe32([]byte{0x7F, 0x7F, 0x7F, 0x7F}); got != 0x0FFFFFFF {
		t.Errorf("unsynch u32 = %x", got)
	}
}

func TestSynchsafeUnsynchronizedStream(t *testing.T) {
	unsyncContent := []byte{0xFF, 0x00, 0x00, 0xFF, 0x12, 0xB0, 0x05, 0xFF, 0x00, 0x00}
	expected := []byte{0xFF, 0x00, 0xFF, 0x12, 0xB0, 0x05, 0xFF, 0x00}

	if got := id3v2Deunsync(append([]byte{}, unsyncContent...)); !bytes2Equal(got, expected) {
		t.Errorf("deunsync = %x", got)
	}
	// Large buffer (repeated content)
	large := append([]byte{}, unsyncContent...)
	for i := 0; i < 999; i++ {
		large = append(large, unsyncContent...)
	}
	largeExp := append([]byte{}, expected...)
	for i := 0; i < 999; i++ {
		largeExp = append(largeExp, expected...)
	}
	if got := id3v2Deunsync(large); !bytes2Equal(got, largeExp) {
		t.Errorf("deunsync large: len = %d, want %d", len(got), len(largeExp))
	}
	// Unrelated content is untouched
	original := []byte{0xFF, 0x1A, 0xFF, 0xC0, 0x10, 0x01}
	if got := id3v2Deunsync(append([]byte{}, original...)); !bytes2Equal(got, original) {
		t.Errorf("deunsync unrelated = %x", got)
	}
}

func bytes2Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- src/id3/v2/util/pairs.rs (set_number) ---

func TestPairsSetNumber(t *testing.T) {
	// Whitespace-padded text is trimmed and parsed.
	tag := NewApeTag()
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText("  12  "))); err != nil {
		t.Fatal(err)
	}
	if n, ok := tag.Track(); !ok || n != 12 {
		n, _ := tag.Track()
		t.Fatalf("track = %v", n)
	}
	// Empty/whitespace-only values insert nothing.
	tag = NewApeTag()
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText(""))); err != nil {
		t.Fatal(err)
	}
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText("        "))); err != nil {
		t.Fatal(err)
	}
	if _, ok := tag.Track(); ok {
		t.Fatal("track should be unset")
	}
	if !tag.IsEmpty() {
		t.Fatal("tag should be empty")
	}
}

// --- src/util/io.rs ---

const ioTestAsset = "lofty-rs/lofty/tests/files/assets/minimal/full_test.mp3"

func ioAlterTag(file *MpegFile) {
	tag := file.ID3v2.MustGet()
	tag.SetArtist("Bar artist")
}

func ioRevertTag(file *MpegFile) {
	tag := file.ID3v2.MustGet()
	tag.SetArtist("Foo artist")
}

func TestIoSaveToFile(t *testing.T) {
	fileContent, err := os.ReadFile(ioTestAsset)
	if err != nil {
		t.Fatal(err)
	}
	file, err := MpegFileReadFrom(bytes.NewReader(fileContent), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioAlterTag(file)

	tmp, err := os.CreateTemp("", "lofty-io-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(fileContent); err != nil {
		t.Fatal(err)
	}

	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := file.SaveFile(NewOSFile(tmp), wo); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	file2, err := MpegFileReadFrom(NewOSFile(tmp), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioRevertTag(file2)
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := file2.SaveFile(NewOSFile(tmp), wo); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, fileContent) {
		t.Fatal("contents differ after revert")
	}
}

func TestIoSaveToVec(t *testing.T) {
	fileContent, err := os.ReadFile(ioTestAsset)
	if err != nil {
		t.Fatal(err)
	}
	file, err := MpegFileReadFrom(bytes.NewReader(fileContent), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioAlterTag(file)

	bf := NewBytesFile(append([]byte(nil), fileContent...))
	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := file.SaveFile(bf, wo); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	file2, err := MpegFileReadFrom(bf, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioRevertTag(file2)
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := file2.SaveFile(bf, wo); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if !bytes.Equal(bf.Bytes(), fileContent) {
		t.Fatal("contents differ after revert")
	}
}

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
	ok, err := newRevPatternSearcher(mk(make([]byte, 5000)), pat).search()
	if err != nil || !ok {
		t.Fatalf("basic search: %v %v", ok, err)
	}

	// Search across boundaries (pattern + 1023 zeros crosses the 1024 buffer)
	ok, err = newRevPatternSearcher(mk(make([]byte, 1023)), pat).search()
	if err != nil || !ok {
		t.Fatalf("boundary search: %v %v", ok, err)
	}

	// Multiple occurrences -> finds the last one
	const junkLen = 20
	data3 := append(append(append(append([]byte(nil), pat...), make([]byte, junkLen)...), pat...), make([]byte, junkLen)...)
	want := uint64(len(data3) - junkLen - len(pat))
	stream3 := bytes.NewReader(data3)
	ok, err = newRevPatternSearcher(stream3, pat).search()
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
	ok, err = newRevPatternSearcher(stream4, pat).
		withStart(revSearchFromCurrent).
		withEnd(revSearchEndStreamStart).
		search()
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
