package util

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBytesFileWriteAmortisedGrowth(t *testing.T) {
	f := NewBytesFile(nil)
	caps := map[int]struct{}{}
	for i := 0; i < 20000; i++ {
		if _, err := f.Write([]byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
		caps[cap(f.Bytes())] = struct{}{}
	}
	// Geometric growth reallocates O(log n) times; linear growth would be ~20000.
	if len(caps) > 64 {
		t.Fatalf("buffer reallocated %d times for 20000 appends", len(caps))
	}
	for i, b := range f.Bytes() {
		if b != byte(i) {
			t.Fatalf("byte %d = %d", i, b)
		}
	}
}

func TestBytesFileWriteGapIsZeroed(t *testing.T) {
	// Leave stale bytes in the spare capacity, then write past the end.
	f := NewBytesFile([]byte("0123456789"))
	if err := f.Truncate(2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("XY")); err != nil {
		t.Fatal(err)
	}
	if want := []byte("01\x00\x00\x00\x00XY"); !bytes.Equal(f.Bytes(), want) {
		t.Fatalf("got %q, want %q", f.Bytes(), want)
	}

	// Overwrite in the middle must not disturb surrounding bytes.
	g := NewBytesFile([]byte("abcdef"))
	if _, err := g.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Write([]byte("XYZWV")); err != nil {
		t.Fatal(err)
	}
	if want := []byte("abXYZWV"); !bytes.Equal(g.Bytes(), want) {
		t.Fatalf("got %q, want %q", g.Bytes(), want)
	}
}

func TestReplaceRangeGrowSmallAndLarge(t *testing.T) {
	// Tiny tail with a huge growth, and a huge tail with tiny growth: the
	// scratch buffer is sized to the work, not a fixed 64KiB.
	big := make([]byte, 200_000)
	for i := range big {
		big[i] = byte(i % 251)
	}
	applyRange(t, []byte("abcdefgh"), 2, 3, big)
	applyRange(t, big, 10, 11, []byte("XX"))
	applyRange(t, big, 10, 12, []byte("X"))
	applyRange(t, big, 0, 0, []byte("prefix"))
}

func TestRevSearchEndFromCurrent(t *testing.T) {
	pat := []byte("PAT")
	data := append(append(append([]byte("PAT"), make([]byte, 10)...), pat...), make([]byte, 4)...)

	// Searching from the end, stopping at the starting position (offset 8): the
	// second pattern (offset 13) is found...
	r := bytes.NewReader(data)
	if _, err := r.Seek(8, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	ok, err := NewRevPatternSearcher(r, pat).WithEnd(RevSearchEndFromCurrent).Search()
	if err != nil || !ok {
		t.Fatalf("search = %v, %v", ok, err)
	}
	if pos, _ := r.Seek(0, io.SeekCurrent); pos != 13 {
		t.Fatalf("pos = %d, want 13", pos)
	}

	// ...but the first pattern (offset 0) lies before the end bound.
	r = bytes.NewReader(data[:12])
	if _, err := r.Seek(8, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	ok, err = NewRevPatternSearcher(r, pat).WithEnd(RevSearchEndFromCurrent).Search()
	if err != nil || ok {
		t.Fatalf("search = %v, %v; want not found", ok, err)
	}
}

func TestTextDecodeOptionsConstructor(t *testing.T) {
	o := NewTextDecodeOptions()
	if o.Encoding != TextEncodingUTF8 || o.Terminated || o.BOM != [2]byte{} {
		t.Fatalf("defaults = %+v", o)
	}
	res, err := DecodeText(strings.NewReader("héllo"), o)
	if err != nil || res.Content != "héllo" {
		t.Fatalf("DecodeText = %+v, %v", res, err)
	}

	b := o.WithEncoding(TextEncodingUTF16).WithTerminated(true).WithBOM([2]byte{0xFF, 0xFE})
	if b.Encoding != TextEncodingUTF16 || !b.Terminated || b.BOM != [2]byte{0xFF, 0xFE} {
		t.Fatalf("builder = %+v", b)
	}
	// Builders return copies.
	if o.Encoding != TextEncodingUTF8 {
		t.Fatal("WithEncoding mutated the receiver")
	}
}
