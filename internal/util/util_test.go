package util

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/rand"
	"testing"
	"testing/iotest"
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

// --- ChunkedScan / frame sync (src/mpeg/header.rs search_for_frame_sync) ---

// naiveFrameSync is the byte-by-byte reference: a 0xFF followed by a byte with
// its top three bits set, entirely within the first max bytes (max < 0: all).
func naiveFrameSync(data []byte, max int64) int64 {
	lim := int64(len(data))
	if max >= 0 && max < lim {
		lim = max
	}
	for i := int64(0); i+1 < lim; i++ {
		if data[i] == 0xFF && data[i+1]&0xE0 == 0xE0 {
			return i
		}
	}
	return -1
}

// naiveID3 is the byte-by-byte reference for the "ID3" magic within max bytes.
func naiveID3(data []byte, max int64) int64 {
	lim := int64(len(data))
	if max >= 0 && max < lim {
		lim = max
	}
	for i := int64(0); i+3 <= lim; i++ {
		if string(data[i:i+3]) == "ID3" {
			return i
		}
	}
	return -1
}

// scanRS adapts an arbitrary reader into an io.ReadSeeker. ChunkedScan only
// calls Read, so Seek is never used.
type scanRS struct {
	io.Reader
	io.Seeker
}

// dataWithErrReader returns all of data in the same Read call as err.
type dataWithErrReader struct {
	data []byte
	err  error
	done bool
}

func (r *dataWithErrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), r.err
}

func randomScanBytes(rng *rand.Rand, n int, alphabet []byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return b
}

// plantScanPattern writes pat at a boundary-sensitive or random offset.
func plantScanPattern(rng *rand.Rand, data, pat []byte) {
	n := len(data)
	if n < len(pat) {
		return
	}
	cands := []int{0, 8190, 8191, 8192, 8193, 16382, 16383, 16384, 16385, n - len(pat), n - len(pat) - 1, rng.Intn(n - len(pat) + 1)}
	if p := cands[rng.Intn(len(cands))]; p >= 0 && p+len(pat) <= n {
		copy(data[p:], pat)
	}
}

func pickScanMax(rng *rand.Rand, n int) int64 {
	choices := []int64{-1, 0, 1, 2, 3, 8191, 8192, 8193, 8194, int64(n), int64(n) + 1, int64(rng.Intn(n + 10))}
	return choices[rng.Intn(len(choices))]
}

// TestChunkedScanDifferential compares ChunkedScan (and the seeking frame sync
// wrapper) against the naive scan over sizes straddling the 8 KiB chunk
// boundary, matches at the start/end/boundary, and assorted max values.
func TestChunkedScanDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5eed))
	frameAlpha := []byte{0x00, 0x7F, 0xE0, 0xFF, 0x1F, 0x20}
	id3Alpha := []byte{'I', 'D', '3', 'x', 0x00}
	sizes := []int{0, 1, 2, 3, 4, 8190, 8191, 8192, 8193, 8194, 16382, 16383, 16384, 16385, 16386, 24576}
	syncPats := [][]byte{{0xFF, 0xE0}, {0xFF, 0xFB}, {0xFF, 0xFF}}

	for iter := 0; iter < 1200; iter++ {
		n := sizes[rng.Intn(len(sizes))]
		if iter%3 == 0 {
			n = rng.Intn(40000)
		}
		max := pickScanMax(rng, n)

		data := randomScanBytes(rng, n, frameAlpha)
		if rng.Intn(10) < 7 {
			plantScanPattern(rng, data, syncPats[rng.Intn(len(syncPats))])
		}
		want := naiveFrameSync(data, max)
		got, found, err := ChunkedScan(scanRS{bytes.NewReader(data), bytes.NewReader(data)}, 1, max, frameSyncMatch)
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		if (want >= 0) != found || (found && got != want) {
			t.Fatalf("frame n=%d max=%d: naive=%d, ChunkedScan=(%d,%v)", n, max, want, got, found)
		}

		// The seeking wrapper reports the same offset and leaves the reader at match+2.
		rs := bytes.NewReader(data)
		off, serr := SearchForFrameSyncSeeking(rs, max)
		if serr != nil {
			t.Fatalf("iter %d: %v", iter, serr)
		}
		if off != want {
			t.Fatalf("SearchForFrameSyncSeeking n=%d max=%d: got %d, want %d", n, max, off, want)
		}
		if want >= 0 {
			if pos, _ := rs.Seek(0, io.SeekCurrent); pos != want+2 {
				t.Fatalf("SearchForFrameSyncSeeking position: got %d, want %d", pos, want+2)
			}
		}

		// Offsets are relative to the position the scan started from.
		if n > 60 {
			k := rng.Intn(50)
			rk := bytes.NewReader(data)
			if _, err := rk.Seek(int64(k), io.SeekStart); err != nil {
				t.Fatal(err)
			}
			wantK := naiveFrameSync(data[k:], max)
			gotK, foundK, err := ChunkedScan(rk, 1, max, frameSyncMatch)
			if err != nil {
				t.Fatal(err)
			}
			if (wantK >= 0) != foundK || (foundK && gotK != wantK) {
				t.Fatalf("prefix k=%d n=%d max=%d: naive=%d, ChunkedScan=(%d,%v)", k, n, max, wantK, gotK, foundK)
			}
		}
	}

	for iter := 0; iter < 800; iter++ {
		n := sizes[rng.Intn(len(sizes))]
		if iter%2 == 0 {
			n = rng.Intn(40000)
		}
		data := randomScanBytes(rng, n, id3Alpha)
		if rng.Intn(10) < 6 {
			plantScanPattern(rng, data, []byte("ID3"))
		}
		max := pickScanMax(rng, n)
		want := naiveID3(data, max)
		got, found, err := ChunkedScan(scanRS{bytes.NewReader(data), bytes.NewReader(data)}, 2, max, func(w []byte) int {
			return bytes.Index(w, []byte("ID3"))
		})
		if err != nil {
			t.Fatal(err)
		}
		if (want >= 0) != found || (found && got != want) {
			t.Fatalf("ID3 n=%d max=%d: naive=%d, ChunkedScan=(%d,%v)", n, max, want, got, found)
		}
	}
}

// TestChunkedScanShortReads: a legal io.Reader may return fewer bytes than
// requested. The scan must keep filling the window and find the match.
func TestChunkedScanShortReads(t *testing.T) {
	data := make([]byte, 20000)
	data[8191], data[8192] = 0xFF, 0xE0
	data[100] = 0xFF
	data[101] = 0xE0

	for _, tc := range []struct {
		name string
		wrap func(io.Reader) io.Reader
	}{
		{"one-byte", iotest.OneByteReader},
		{"half", iotest.HalfReader},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := func() io.ReadSeeker {
				return scanRS{tc.wrap(bytes.NewReader(data)), bytes.NewReader(data)}
			}
			off, found, err := ChunkedScan(r(), 1, -1, frameSyncMatch)
			if err != nil || !found || off != 100 {
				t.Fatalf("found=%v off=%d err=%v, want 100", found, off, err)
			}
			// The first sync is at 100, so start past it and find the boundary one.
			// Seek the wrapped reader forward by reading 101 bytes first.
			rs := scanRS{tc.wrap(bytes.NewReader(data[101:])), bytes.NewReader(data[101:])}
			off, found, err = ChunkedScan(rs, 1, -1, frameSyncMatch)
			if err != nil || !found || off != 8191-101 {
				t.Fatalf("boundary: found=%v off=%d err=%v, want %d", found, off, err, 8191-101)
			}
			// A sync whose second byte sits at index 8192 needs max >= 8193.
			only := make([]byte, 16384)
			only[8191], only[8192] = 0xFF, 0xE0
			mk := func() io.ReadSeeker {
				return scanRS{tc.wrap(bytes.NewReader(only)), bytes.NewReader(only)}
			}
			off, found, err = ChunkedScan(mk(), 1, 8192, frameSyncMatch)
			if err != nil || found {
				t.Fatalf("max=8192: found=%v off=%d err=%v, want not found", found, off, err)
			}
			off, found, err = ChunkedScan(mk(), 1, 8193, frameSyncMatch)
			if err != nil || !found || off != 8191 {
				t.Fatalf("max=8193: found=%v off=%d err=%v, want 8191", found, off, err)
			}
		})
	}
}

// TestChunkedScanDataWithEOF: a reader may return its final bytes together
// with io.EOF. Those bytes must still be searched.
func TestChunkedScanDataWithEOF(t *testing.T) {
	data := []byte{0, 0, 0xFF, 0xE0}
	rd := iotest.DataErrReader(bytes.NewReader(data))
	off, found, err := ChunkedScan(scanRS{rd, bytes.NewReader(data)}, 1, -1, frameSyncMatch)
	if err != nil || !found || off != 2 {
		t.Fatalf("found=%v off=%d err=%v, want 2", found, off, err)
	}
}

// TestChunkedScanReadErrors: a non-EOF error is returned unless a match is
// found in the bytes that arrived with it.
func TestChunkedScanReadErrors(t *testing.T) {
	boom := errors.New("boom")

	off, found, err := ChunkedScan(scanRS{&dataWithErrReader{data: []byte{0, 0, 0}, err: boom}, bytes.NewReader(nil)}, 1, -1, frameSyncMatch)
	if !errors.Is(err, boom) || found {
		t.Fatalf("no match: found=%v off=%d err=%v, want boom", found, off, err)
	}

	off, found, err = ChunkedScan(scanRS{&dataWithErrReader{data: []byte{0, 0xFF, 0xE0}, err: boom}, bytes.NewReader(nil)}, 1, -1, frameSyncMatch)
	if err != nil || !found || off != 1 {
		t.Fatalf("match before error: found=%v off=%d err=%v, want 1", found, off, err)
	}
}

// TestSearchForFrameSyncNonSeekable covers the io.Reader path of
// SearchForFrameSync with short reads and data returned together with io.EOF.
func TestSearchForFrameSyncNonSeekable(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    io.Reader
		want int64
	}{
		{"data with EOF", iotest.DataErrReader(bytes.NewReader([]byte{0xFF, 0xFB})), 0},
		{"data with EOF, late sync", iotest.DataErrReader(bytes.NewReader([]byte{0, 0, 0xFF, 0xE0})), 2},
		{"one-byte reads", iotest.OneByteReader(bytes.NewReader(append(make([]byte, 100), 0xFF, 0xE0))), 100},
		{"plain reader", struct{ io.Reader }{bytes.NewReader([]byte{0x00, 0xFF, 0xE0})}, 1},
		{"no sync", bytes.NewReader([]byte{0xFF, 0x1F}), -1},
	} {
		got, err := SearchForFrameSync(tc.r)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %d, %v; want %d", tc.name, got, err, tc.want)
		}
	}

	rng := rand.New(rand.NewSource(7))
	alpha := []byte{0x00, 0x7F, 0xE0, 0xFF, 0x1F}
	for iter := 0; iter < 300; iter++ {
		data := randomScanBytes(rng, rng.Intn(3000), alpha)
		want := naiveFrameSync(data, -1)
		got, err := SearchForFrameSync(struct{ io.Reader }{iotest.DataErrReader(bytes.NewReader(data))})
		if err != nil || got != want {
			t.Fatalf("iter %d: got %d, %v; want %d", iter, got, err, want)
		}
	}
}

// --- synchsafe (src/id3/v2/util/synchsafe.rs) ---

func TestUnsynchsafe32MasksHighBits(t *testing.T) {
	// Rust's unsynch() keeps only the low 7 bits of each byte.
	for _, tc := range []struct {
		in   []byte
		want uint32
	}{
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF}, 0x0FFFFFFF},
		{[]byte{0x80, 0x00, 0x00, 0x00}, 0},
		{[]byte{0x80, 0x80, 0x80, 0x80}, 0},
		{[]byte{0xFF, 0x00, 0x00, 0x00}, 0x0FE00000},
		{[]byte{0x00, 0x00, 0x00, 0x80}, 0},
		{[]byte{0x01, 0x00, 0x00, 0x00}, 0x200000},
	} {
		if got := Unsynchsafe32(tc.in); got != tc.want {
			t.Errorf("Unsynchsafe32(% x) = %#x, want %#x", tc.in, got, tc.want)
		}
	}
}

func TestSynchsafeRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	values := []uint32{0, 1, 0x7F, 0x80, 0x3FFF, 0x4000, 0x0FFFFFFF}
	for range 500 {
		values = append(values, rng.Uint32()&0x0FFFFFFF)
	}
	for _, v := range values {
		b := Synchsafe32(v)
		if got := Unsynchsafe32(b[:]); got != v {
			t.Fatalf("round trip %#x: got %#x (bytes % x)", v, got, b)
		}
	}
}

// --- FlagItem / U32FromString (src/util/mod.rs, str::parse::<u32>) ---

func TestFlagItemExact(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"1", 1},
		{"true", 1},
		{"0", 0},
		{"false", 0},
		{"True", -1},
		{"TRUE", -1},
		{" 1 ", -1},
		{"1\n", -1},
		{" true", -1},
		{"yes", -1},
		{"", -1},
		{"2", -1},
	} {
		if got := FlagItem(tc.in); got != tc.want {
			t.Errorf("FlagItem(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestU32FromStringRustParse(t *testing.T) {
	// Each row is what Rust's "<s>".parse::<u32>() gives.
	for _, tc := range []struct {
		in   string
		want uint32
		ok   bool
	}{
		{"5", 5, true},
		{"+5", 5, true},
		{"+0", 0, true},
		{"00005", 5, true},
		{"4294967295", math.MaxUint32, true},
		{"+4294967295", math.MaxUint32, true},
		{"4294967296", 0, false},
		{"+4294967296", 0, false},
		{"+", 0, false},
		{"+-5", 0, false},
		{"++5", 0, false},
		{"-5", 0, false},
		{"-0", 0, false},
		{" 5", 0, false},
		{"5 ", 0, false},
		{"", 0, false},
		{"0x5", 0, false},
	} {
		got, ok := U32FromString(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("U32FromString(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// --- allocation (src/util/alloc.rs) ---

// Port of Rust `util/alloc.rs` test `vec_fallible_repeat`.
func TestVecFallibleRepeat(t *testing.T) {
	u8Vec, err := TryMake[uint8](20)
	if err != nil || len(u8Vec) != 20 {
		t.Fatalf("u8 len 20: len=%d err=%v", len(u8Vec), err)
	}
	for _, b := range u8Vec {
		if b != 0 {
			t.Fatal("u8 vec not zeroed")
		}
	}
	u64Vec, err := TryMake[uint64](89)
	if err != nil || len(u64Vec) != 89 {
		t.Fatalf("u64 len 89: len=%d err=%v", len(u64Vec), err)
	}
	for _, v := range u64Vec {
		if v != 0 {
			t.Fatal("u64 vec not zeroed")
		}
	}
	if _, err := TryMake[uint8](math.MaxUint32); err == nil {
		t.Fatal("u32::MAX bytes: expected an allocation error")
	}
}

func withAllocationLimit(t *testing.T, limit int) {
	t.Helper()
	old := GetGlobalOptions()
	o := old
	o.AllocationLimit = limit
	ApplyGlobalOptions(o)
	t.Cleanup(func() { ApplyGlobalOptions(old) })
}

func TestAllocationLimitBoundary(t *testing.T) {
	withAllocationLimit(t, 100)

	if b, err := TryByteSlice(100); err != nil || len(b) != 100 {
		t.Errorf("TryByteSlice(limit): len=%d err=%v, want ok", len(b), err)
	}
	if _, err := TryByteSlice(101); !isLimitError(err) {
		t.Errorf("TryByteSlice(limit+1): err=%v, want limit error", err)
	}
	if _, err := TryCapSlice[byte](100); err != nil {
		t.Errorf("TryCapSlice(limit): %v", err)
	}
	if _, err := TryCapSlice[byte](101); !isLimitError(err) {
		t.Errorf("TryCapSlice(limit+1): err=%v, want limit error", err)
	}
}

func isLimitError(err error) bool {
	var ae *AllocationError
	return errors.As(err, &ae) && ae.Kind == AllocationLimitExceeded
}

// A negative size must return the same error as an oversized request, not panic.
func TestAllocationNegativeSize(t *testing.T) {
	for _, size := range []int{-1, math.MinInt} {
		if _, err := TryByteSlice(size); !isLimitError(err) {
			t.Errorf("TryByteSlice(%d): err=%v, want limit error", size, err)
		}
		if _, err := TryCapSlice[uint64](size); !isLimitError(err) {
			t.Errorf("TryCapSlice(%d): err=%v, want limit error", size, err)
		}
	}
}

func TestAllocationErrorWithoutCause(t *testing.T) {
	e := &AllocationError{Kind: AllocationReserveError}
	if msg := e.Error(); msg == "" {
		t.Fatal("empty message for reserve error without cause")
	}
	if e.Unwrap() != nil {
		t.Fatal("Unwrap should be nil without a cause")
	}
}

// --- BytesFile (Cursor<Vec<u8>> semantics) ---

func TestBytesFileTruncateLargerIsNoop(t *testing.T) {
	f := NewBytesFile([]byte{1, 2, 3})
	if _, err := f.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(10); err != nil {
		t.Fatalf("Truncate(10): %v, want nil (Vec::truncate is a no-op)", err)
	}
	if !bytes.Equal(f.Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("contents %v, want [1 2 3]", f.Bytes())
	}
	if f.Pos() != 2 {
		t.Fatalf("pos %d, want 2", f.Pos())
	}
}

func TestBytesFileTruncateKeepsPosition(t *testing.T) {
	f := NewBytesFile([]byte{1, 2, 3, 4, 5})
	if _, err := f.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(2); err != nil {
		t.Fatal(err)
	}
	if f.Pos() != 5 {
		t.Fatalf("pos %d, want 5 (Cursor position is not clamped)", f.Pos())
	}
	// Writing past the end zero-fills the gap, as Cursor<Vec<u8>> does.
	if _, err := f.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if want := []byte{1, 2, 0, 0, 0, 'a', 'b'}; !bytes.Equal(f.Bytes(), want) {
		t.Fatalf("contents %v, want %v", f.Bytes(), want)
	}
}

func TestBytesFileTruncateNegative(t *testing.T) {
	f := NewBytesFile([]byte{1, 2, 3})
	if err := f.Truncate(-1); err == nil {
		t.Fatal("Truncate(-1): expected an error")
	}
	if !bytes.Equal(f.Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("contents changed to %v", f.Bytes())
	}
}

func TestBytesFileWriteAfterStaleCapacityZeroFills(t *testing.T) {
	// Truncate leaves stale bytes in the capacity; a write that reslices over them
	// must still read zeros in the gap.
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = 0xAA
	}
	f := NewBytesFile(buf)
	if err := f.Truncate(2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{'z'}); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0xAA, 0xAA, 0, 0, 0, 0, 'z'}; !bytes.Equal(f.Bytes(), want) {
		t.Fatalf("contents %x, want %x", f.Bytes(), want)
	}
}
