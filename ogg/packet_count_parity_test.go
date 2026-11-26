package ogg

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/subframe7536/lofty-go/internal/util"
)

func openFullTestOgg(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open("../lofty-rs/lofty/tests/files/assets/minimal/full_test.ogg")
	if err != nil {
		t.Skip("fixture missing:", err)
	}
	return f
}

func rewindToStart(t *testing.T, f *os.File) {
	t.Helper()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
}

func TestOggReadCountPacketsCountSemantics(t *testing.T) {
	f := openFullTestOgg(t)
	defer f.Close()

	packets, remainder, _, err := OggReadCountPackets(f, -1)
	if err != nil {
		t.Fatalf("count=-1 returned error: %v", err)
	}
	if remainder != nil {
		t.Fatalf("count=-1 remainder = %+v, want nil", remainder)
	}
	if len(packets) != 73 {
		t.Fatalf("count=-1 packet count = %d, want 73", len(packets))
	}

	rewindToStart(t, f)
	packets, _, _, err = OggReadCountPackets(f, 1000000)
	if err == nil {
		t.Fatalf("oversized positive count returned %d packets and no error", len(packets))
	}
	var oggErr *OggParseError
	if !errors.As(err, &oggErr) {
		t.Fatalf("oversized count error = %T (%v), want *OggParseError", err, err)
	}
	if !errors.Is(err, util.ErrNotEnoughData) {
		t.Fatalf("oversized count error = %v, want wrapped ErrNotEnoughData", err)
	}

	for _, count := range []int{0, -2} {
		rewindToStart(t, f)
		packets, remainder, _, err = OggReadCountPackets(f, count)
		if err != nil {
			t.Fatalf("count=%d returned error: %v", count, err)
		}
		if len(packets) != 0 || remainder != nil {
			t.Fatalf("count=%d got %d packets, remainder %v; want empty/nil", count, len(packets), remainder)
		}
	}
}
