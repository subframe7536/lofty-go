package mpeg

import (
	"bytes"
	"os"
	"testing"
)

// --- src/mpeg/header.rs ---

func TestSearchForFrameSync(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want int64
	}{
		{[]byte{0xFF, 0xFB, 0x00}, 0},
		{[]byte{0x00, 0x00, 0x01, 0xFF, 0xFB}, 3},
		{[]byte{0x01, 0xFF}, -1},
	} {
		got, err := searchForFrameSync(bytes.NewReader(tc.data))
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%x: got %d, want %d", tc.data, got, tc.want)
		}
	}
}

func TestRevSearchForFrameHeader(t *testing.T) {
	check := func(data []byte, wantPos uint64, wantFound bool) {
		t.Helper()
		r := bytes.NewReader(data)
		end, err := r.Seek(0, 2)
		if err != nil {
			t.Fatal(err)
		}
		_, pos, found, err := revSearchForFrameHeader(r, uint64(end))
		if err != nil {
			t.Fatal(err)
		}
		if found != wantFound {
			t.Fatalf("found = %v, want %v", found, wantFound)
		}
		if wantFound && pos != wantPos {
			t.Fatalf("pos = %d, want %d", pos, wantPos)
		}
	}

	check([]byte{0xFF, 0xFB, 0x52, 0xC4}, 0, true)
	check([]byte{0x00, 0x00, 0x01, 0xFF, 0xFB, 0x52, 0xC4}, 3, true)
	check([]byte{0x01, 0xFF}, 0, false)

	data, err := os.ReadFile("../lofty-rs/lofty/tests/files/assets/rev_frame_sync_search.mp3")
	if err != nil {
		t.Fatal(err)
	}
	check(data, 595, true)
}
