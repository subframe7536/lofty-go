package dsf

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

func dsfBytes(totalFileSize uint64, sampleFreq uint32, sampleCount uint64) []byte {
	var b []byte
	b = append(b, "DSD "...)
	b = binary.LittleEndian.AppendUint64(b, 28)
	b = binary.LittleEndian.AppendUint64(b, totalFileSize)
	b = binary.LittleEndian.AppendUint64(b, 0)
	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint64(b, 52)
	b = binary.LittleEndian.AppendUint32(b, 1) // version
	b = binary.LittleEndian.AppendUint32(b, 0) // format id
	b = binary.LittleEndian.AppendUint32(b, 2) // channel type (stereo)
	b = binary.LittleEndian.AppendUint32(b, 2) // channels
	b = binary.LittleEndian.AppendUint32(b, sampleFreq)
	b = binary.LittleEndian.AppendUint32(b, 1) // bits per sample
	b = binary.LittleEndian.AppendUint64(b, sampleCount)
	b = binary.LittleEndian.AppendUint32(b, 4096) // block size
	b = binary.LittleEndian.AppendUint32(b, 0)    // reserved
	return b
}

func TestDsfPropertiesBasic(t *testing.T) {
	// 2822400 Hz * 10 s -> 10000 ms; 8*1000000 bits over 10000 ms = 800 kbps
	f, err := DsfFileReadFrom(bytes.NewReader(dsfBytes(1_000_000, 2822400, 28224000)), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Properties.Duration.Milliseconds(); got != 10000 {
		t.Fatalf("duration = %dms, want 10000", got)
	}
	if got := f.Properties.OverallBitrate; got != 800 {
		t.Fatalf("overall bitrate = %d, want 800", got)
	}
}

// Rust computes the duration with u128 math (`sample_count * 1000` cannot
// overflow) and `Duration::from_millis(u64)`, which never overflows; Go must
// saturate rather than wrap.
func TestDsfPropertiesHugeSampleCountNoOverflow(t *testing.T) {
	const sampleCount = uint64(1) << 60
	const sampleFreq = 44100
	f, err := DsfFileReadFrom(bytes.NewReader(dsfBytes(1000, sampleFreq, sampleCount)), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	// sampleCount*1000 overflows u64; the u128 result still fits in u64 millis.
	millis := util.MulDivRound(sampleCount, 1000, sampleFreq)
	if millis == 0 || millis == math.MaxUint64 {
		t.Fatalf("unexpected millis %d", millis)
	}
	if got, want := f.Properties.Duration, util.DurationFromMillis(millis); got != want {
		t.Fatalf("duration = %v, want %v", got, want)
	}
	if f.Properties.Duration <= 0 {
		t.Fatalf("duration wrapped negative: %v", f.Properties.Duration)
	}
	// 1000 bytes * 8 bits over ~2.6e16 ms rounds to 0
	if f.Properties.OverallBitrate != 0 {
		t.Fatalf("overall bitrate = %d, want 0", f.Properties.OverallBitrate)
	}
}

// An ID3v2 tag carrying an extended header and a footer parses through
// LocateId3v2's content capture (Rust find_id3v2), landing at the right
// content start despite the extra headers.
func TestDsfID3ExtendedHeader(t *testing.T) {
	b := dsfBytes(0, 2822400, 28224000)
	binary.LittleEndian.PutUint64(b[20:28], uint64(len(b)))
	id3 := "ID3\x04\x00\x50" + // header: v2.4, extended header + footer flags
		"\x00\x00\x00\x1c" + // content size 28 (syncsafe)
		"\x00\x00\x00\x06\x01\x00" + // extended header: size 6, 1 flag byte
		"TIT2\x00\x00\x00\x0c\x00\x00" + // frame: TIT2, size 12, no flags
		"\x03" + "Audit title" + // UTF-8 text
		"3DI\x04\x00\x50\x00\x00\x00\x1c" // footer
	b = append(b, id3...)
	binary.LittleEndian.PutUint64(b[12:20], uint64(len(b)))
	f, err := DsfFileReadFrom(bytes.NewReader(b), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.ID3v2.Get()
	if !ok {
		t.Fatal("expected an ID3v2 tag")
	}
	if got := tag.Title(); got != "Audit title" {
		t.Fatalf("title = %q, want %q", got, "Audit title")
	}
}
