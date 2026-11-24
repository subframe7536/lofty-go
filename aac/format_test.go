package aac

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Regression test for upstream lofty-rs ec142091 (issue #783):
// ADTS frames whose declared length is shorter than their header (7 or 9 bytes)
// must never infinite-loop, must be skipped in non-strict mode, and must fail
// in strict mode.
func TestUndersizedADTSFrame(t *testing.T) {
	// Valid ADTS sync (0xFFF1: MPEG-4, no CRC), AAC-LC, 44.1kHz (idx 4),
	// stereo (2ch), frame length = 0 (< 7 byte header), repeated twice so the
	// next-header comparison would match if the undersized guard were absent.
	frame := []byte{0xFF, 0xF1, 0x50, 0x80, 0x00, 0x00, 0xFC}
	data := append(append([]byte(nil), frame...), frame...)

	// Non-strict mode: readAdtsHeader returns (nil, nil) instead of looping.
	po := meta.NewParseOptions()
	po.ReadTags = false
	if _, err := AacFileReadFrom(bytes.NewReader(data), po); err == nil {
		t.Fatal("expected error when no valid ADTS frame exists")
	}

	// Strict mode: rejects the undersized frame explicitly.
	strict := meta.NewParseOptions()
	strict.ReadTags = false
	strict.ParsingMode = meta.ParsingModeStrict
	_, err := AacFileReadFrom(bytes.NewReader(data), strict)
	if err == nil {
		t.Fatal("expected strict-mode error for undersized ADTS frame")
	}
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil || !strings.Contains(unwrapped.Error(), "shorter than its header") {
		t.Fatalf("strict error = %v (unwrapped: %v), want substring %q", err, unwrapped, "shorter than its header")
	}
}
