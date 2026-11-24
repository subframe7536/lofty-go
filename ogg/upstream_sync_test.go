package ogg

import (
	"bytes"
	"math"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Regression test for upstream lofty-rs 63ee24b8 (issue #801):
// Extreme Opus granule positions (huge overflow-inducing values or tiny values
// that round down to 0ms) must never panic.
func TestOpusExtremeGranulePositions(t *testing.T) {
	makeOpus := func(lastGranule uint64) []byte {
		var out []byte
		out = append(out, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestOpusIdent()})...)
		out = append(out, oggTestPage(1, 0, 0, [][]byte{append([]byte("OpusTags"), oggTestCommentPayload("v")...)})...)
		out = append(out, oggTestPage(2, lastGranule, oggPageEOS, [][]byte{{0xAA}})...)
		return out
	}

	// Huge granule position: 128-bit MulDivRound must not overflow/panic.
	if _, err := OggOpusFileReadFrom(bytes.NewReader(makeOpus(math.MaxUint64-1)), meta.NewParseOptions()); err != nil {
		t.Fatalf("huge granule OggOpusFileReadFrom: %v", err)
	}

	// Tiny granule position (1 sample at 48kHz rounds to 0ms): must not divide by zero.
	f, err := OggOpusFileReadFrom(bytes.NewReader(makeOpus(1)), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("tiny granule OggOpusFileReadFrom: %v", err)
	}
	if f.Properties.Duration != 0 || f.Properties.OverallBitrate != 0 || f.Properties.AudioBitrate != 0 {
		t.Fatalf("tiny granule props = %+v, want zero duration/bitrates", f.Properties)
	}
}
