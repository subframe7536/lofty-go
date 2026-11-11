package mpeg

import (
	"os"
	"testing"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
)

// invalid-frames3.mp3 contains a valid-looking header at 0x3c9 whose first
// two bytes are swallowed by an *invalid* sync candidate at 0x3c7. Rust's
// `find_next_frame` searches the raw sync pattern and resumes 4 bytes past
// an invalid candidate, so the first accepted frame is at 0x7dd, giving a
// 157ms duration. Scanning for valid headers byte-by-byte accepts 0x3c9
// instead and yields 183ms.
func TestInvalidFrames3FirstFrameSkipsOverlappedSync(t *testing.T) {
	f, err := os.Open(testutil.Path("lofty-rs", "lofty", "tests", "taglib", "data", "invalid-frames3.mp3"))
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	defer f.Close()
	mf, err := MpegFileReadFrom(f, meta.ParseOptions{ReadTags: true, ReadProperties: true})
	if err != nil {
		t.Fatal(err)
	}
	p := mf.Properties
	if p.Duration.Milliseconds() != 157 {
		t.Fatalf("duration = %dms, want 157", p.Duration.Milliseconds())
	}
	if p.OverallBitrate != 417 || p.AudioBitrate != 320 {
		t.Fatalf("bitrates = %d/%d, want 417/320", p.OverallBitrate, p.AudioBitrate)
	}
}
