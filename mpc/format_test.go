package mpc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
)

// sv8Packet encodes an SV8 packet: key + varint size (covering key, size
// bytes, and payload) + payload.
func sv8Packet(key string, payload []byte) []byte {
	// Encode size as an MPC magnitude varint (7 bits/byte, MSB continuation).
	// Iterate since the varint's own length is part of the size.
	size := len(payload) + 2
	for {
		n := size
		var enc [9]byte
		i := len(enc)
		for {
			i--
			enc[i] = byte(n & 0x7F)
			n >>= 7
			if n == 0 {
				break
			}
		}
		vb := enc[i:]
		total := len(payload) + 2 + len(vb)
		if total == size {
			pkt := []byte(key)
			for j := 0; j < len(vb)-1; j++ {
				pkt = append(pkt, vb[j]|0x80)
			}
			pkt = append(pkt, vb[len(vb)-1])
			return append(pkt, payload...)
		}
		size = total
	}
}

// sv8Stream builds a minimal SV8 stream: SH + caller-provided packets + SE.
func sv8Stream(middle ...[]byte) []byte {
	// SH body: CRC(4) + version(1) + sample_count varint(44100=0x82 0xD8 0x44)
	// + beginning_silence varint(0) + 2 flag bytes (sr idx 0, 2ch)
	sh := []byte{0, 0, 0, 0, 8, 0x82, 0xD8, 0x44, 0x00, 0x00, 0x10}
	stream := []byte("MPCK")
	stream = append(stream, sv8Packet("SH", sh)...)
	for _, p := range middle {
		stream = append(stream, p...)
	}
	return append(stream, sv8Packet("SE", nil)...)
}

func TestMpcSv8Basic(t *testing.T) {
	rg := make([]byte, 9)
	data := sv8Stream(sv8Packet("RG", rg), sv8Packet("AP", make([]byte, 20000)))
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.SampleRate != 44100 || f.Properties.Channels != 2 || f.Properties.Version != 8 {
		t.Fatalf("props = %+v", f.Properties)
	}
	if f.Properties.Duration == 0 || f.Properties.OverallBitrate == 0 {
		t.Fatalf("props = %+v", f.Properties)
	}
}

// Rust reads packet bodies through the capacity-bounded PacketReader: an RG
// packet whose declared payload is too small errors instead of reading into
// the next packet.
func TestMpcSv8TruncatedRg(t *testing.T) {
	// Declared RG capacity of 4 bytes; body read needs 9.
	pkt := []byte("RG")
	pkt = append(pkt, byte(4+3)) // size = key(2)+size(1)+4
	pkt = append(pkt, 0, 0, 0, 0)
	data := sv8Stream(pkt)
	po := meta.NewParseOptions()
	if _, err := MusepackFileReadFrom(bytes.NewReader(data), po); err == nil {
		t.Fatal("expected bounded-read error for truncated RG packet")
	}
}

// An invalid packet key ends the scan like Rust's `while let Ok(...)` —
// strict mode then errors only because mandatory packets are missing.
func TestMpcSv8BadKeyEndsScan(t *testing.T) {
	// The AP packet is never read (the bad key ends the scan first) — it
	// just pads the stream past 32 bytes so Rust's trailing `seek(-32)`
	// for the APE probe doesn't fail the read outright.
	data := sv8Stream([]byte{'a', 'a'}, sv8Packet("AP", make([]byte, 64)))
	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeBestAttempt
	// SE was never reached, but non-strict mode doesn't require it
	f, err := MusepackFileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.Version != 8 {
		t.Fatalf("version = %d", f.Properties.Version)
	}
}

// mpcReadWithTimeout runs MusepackFileReadFrom in a goroutine and fails the
// test (instead of hanging the suite) if it has not returned within 3s.
func mpcReadWithTimeout(t *testing.T, data []byte, po meta.ParseOptions) error {
	t.Helper()
	ch := make(chan error, 1)
	go func() {
		_, err := MusepackFileReadFrom(bytes.NewReader(data), po)
		ch <- err
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("MusepackFileReadFrom did not return within 3s (infinite loop on SV8 packet scan)")
		return nil
	}
}

// A zero-size SV8 packet with an unknown key must not hang the scan. Rust
// (musepack/sv8/read.rs PacketReader) never moves the cursor backwards.
func TestMpcSv8ZeroSizePacketNoHang(t *testing.T) {
	data := append([]byte("MPCK"), 'A', 'P', 0x00)
	data = append(data, make([]byte, 64)...)
	mpcReadWithTimeout(t, data, meta.NewParseOptions())
}

// SV8 packets whose declared size is smaller than their own header (1 or 2)
// carry no payload; Rust resumes scanning right after the size byte, so the
// following SE packet is still found and strict mode accepts the stream.
func TestMpcSv8SmallPacketSizeAdvances(t *testing.T) {
	rg := make([]byte, 9)
	for _, size := range []byte{0, 1, 2} {
		sh := []byte{0, 0, 0, 0, 8, 0x82, 0xD8, 0x44, 0x00, 0x00, 0x10}
		data := []byte("MPCK")
		data = append(data, sv8Packet("SH", sh)...)
		data = append(data, sv8Packet("RG", rg)...)
		data = append(data, 'A', 'P', size)
		data = append(data, sv8Packet("SE", nil)...)
		po := meta.NewParseOptions()
		po.ParsingMode = meta.ParsingModeStrict
		if err := mpcReadWithTimeout(t, data, po); err != nil {
			t.Fatalf("AP size %d: strict read failed (SE not reached): %v", size, err)
		}
	}
}

func mpcSynchsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

// mpcId3v24 builds an ID3v2.4 tag. extHeader adds a 6-byte v2.4 extended
// header (size field + one flag-count byte + one flag byte), counted in Size.
func mpcId3v24(frames []byte, footer, extHeader bool) []byte {
	var flags byte
	body := frames
	if footer {
		flags |= 0x10
	}
	if extHeader {
		flags |= 0x40
		body = append(mpcSynchsafe(6), 0x01, 0x00)
		body = append(body, frames...)
	}
	b := append([]byte("ID3\x04\x00"), flags)
	b = append(b, mpcSynchsafe(len(body))...)
	b = append(b, body...)
	if footer {
		b = append(b, "3DI\x04\x00\x10"...)
		b = append(b, mpcSynchsafe(len(body))...)
	}
	return b
}

//nolint:unparam // text kept for explicit call sites
func mpcTIT2(text string) []byte {
	body := append([]byte{0x00}, text...)
	f := append([]byte("TIT2"), mpcSynchsafe(len(body))...)
	f = append(f, 0, 0)
	return append(f, body...)
}

// mpcSv7 is a minimal SV7 stream: "MP+", version 7, 100 frames, zeroed rest.
func mpcSv7() []byte {
	b := make([]byte, 40)
	copy(b, "MP+")
	b[3] = 0x07
	b[4] = 100
	return b
}

func mpcTitle(t *testing.T, f *MusepackFile) string {
	t.Helper()
	tag, ok := f.ID3v2.Get()
	if !ok {
		t.Fatal("expected an ID3v2 tag")
	}
	s, _ := tag.Tag().Title()
	return s
}

// Footer tag: the content starts right after the 10-byte header, not after
// the footer-adjusted offset the old code computed.
func TestMpcId3v2FooterTag(t *testing.T) {
	data := append(mpcId3v24(mpcTIT2("Hello"), true, false), mpcSv7()...)
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := mpcTitle(t, f); got != "Hello" {
		t.Fatalf("title = %q, want Hello", got)
	}
	if f.Properties.FrameCount != 100 {
		t.Fatalf("SV7 stream not read after footer tag: frames = %d", f.Properties.FrameCount)
	}
}

// Junk before the ID3v2 tag: the stream begins after the tag end, junk included.
func TestMpcId3v2JunkBeforeTag(t *testing.T) {
	data := append([]byte("JUNK"), mpcId3v24(mpcTIT2("Hello"), false, false)...)
	data = append(data, mpcSv7()...)
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got := mpcTitle(t, f); got != "Hello" {
		t.Fatalf("title = %q, want Hello", got)
	}
	if f.Properties.FrameCount != 100 {
		t.Fatalf("frames = %d", f.Properties.FrameCount)
	}
}

// Extended header in front of the SV7 stream.
func TestMpcId3v2ExtendedHeader(t *testing.T) {
	data := append(mpcId3v24(mpcTIT2("Hello"), false, true), mpcSv7()...)
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got := mpcTitle(t, f); got != "Hello" {
		t.Fatalf("title = %q, want Hello", got)
	}
	if f.Properties.FrameCount != 100 {
		t.Fatalf("frames = %d", f.Properties.FrameCount)
	}
}

// Rust musepack/read.rs propagates the ID3v2 parse error with `?`; a
// zero-length frame is an undersized-frame error in strict mode.
func TestMpcId3v2ParseErrorPropagates(t *testing.T) {
	zero := append([]byte("TIT2"), mpcSynchsafe(0)...)
	zero = append(zero, 0, 0)
	data := append(mpcId3v24(zero, false, false), mpcSv7()...)
	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	if _, err := MusepackFileReadFrom(bytes.NewReader(data), po); err == nil {
		t.Fatal("expected strict ID3v2 parse error to propagate")
	}
}

func mpcFixture(t *testing.T, name string) string {
	t.Helper()
	p := testutil.Fixture("minimal", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	return p
}

func mpcOpenRead(t *testing.T, path string, po meta.ParseOptions) *MusepackFile {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	file, err := MusepackFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// Rust read_no_properties_sv7 / read_no_properties_sv8: properties are empty.
func TestMpcSv7ReadNoProperties(t *testing.T) {
	po := meta.NewParseOptions()
	po.ReadProperties = false
	f := mpcOpenRead(t, mpcFixture(t, "mpc_sv7.mpc"), po)
	if f.Properties != (MpcProperties{}) {
		t.Fatalf("properties not empty: %+v", f.Properties)
	}
}

// Rust read_no_tags_sv7: with tags off, no tag is attached.
func TestMpcSv7ReadNoTags(t *testing.T) {
	po := meta.NewParseOptions()
	po.ReadTags = false
	f := mpcOpenRead(t, mpcFixture(t, "mpc_sv7.mpc"), po)
	if f.ContainsTag() {
		t.Fatal("expected no tags with ReadTags=false")
	}
}

// Rust remove_id3v2_sv8 / remove_id3v1_sv8 / remove_ape_sv8: the tag is
// present, removing it leaves the other tags, and a re-read lacks it.
func TestMpcSv8RemoveTags(t *testing.T) {
	src := mpcFixture(t, "mpc_sv8.mpc")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		ty   meta.TagType
		has  func(*MusepackFile) bool
	}{
		{"id3v2", meta.TagTypeID3v2, func(f *MusepackFile) bool { return f.ID3v2.IsSome() }},
		{"id3v1", meta.TagTypeID3v1, func(f *MusepackFile) bool { return f.ID3v1.IsSome() }},
		{"ape", meta.TagTypeApe, func(f *MusepackFile) bool { return f.Ape.IsSome() }},
	}
	po := meta.NewParseOptions()
	po.ReadProperties = false
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mpc_sv8.mpc")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if !c.has(mpcOpenRead(t, path, po)) {
				t.Fatalf("fixture has no %s tag to remove", c.name)
			}
			if err := c.ty.RemoveFromPath(path, meta.NewWriteOptions()); err != nil {
				t.Fatal(err)
			}
			if c.has(mpcOpenRead(t, path, po)) {
				t.Fatalf("%s tag still present after removal", c.name)
			}
		})
	}
}

// segfault2.mpc is a 6-byte "MPCK"+"SH" stub: Rust's end-of-file APE probe
// (`seek(SeekFrom::Current(-32))`) lands before position 0 and the I/O
// error propagates, failing the whole read. Go must propagate too rather
// than silently succeeding with zeroed properties.
func TestSegfault2ShortFileErrors(t *testing.T) {
	f, err := os.Open(testutil.Path("lofty-rs", "lofty", "tests", "taglib", "data", "segfault2.mpc"))
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	defer f.Close()
	if _, err := MusepackFileReadFrom(f, meta.NewParseOptions()); err == nil {
		t.Fatal("expected error on truncated MPCK file")
	}
}
