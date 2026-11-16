package ogg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

// oggTestPage serializes one page holding the given packets on serial 1.
// Each packet is appended whole; packets >= 255 bytes are split across
// continuation segments like a real lacing table.
func oggTestPage(seq uint32, granule uint64, headerType byte, pkts [][]byte) []byte {
	p := &OggPage{
		headerType: headerType,
		granulePos: granule,
		serial:     1,
		sequence:   seq,
	}
	for _, pkt := range pkts {
		for len(pkt) >= 255 {
			p.segments = append(p.segments, 255)
			p.data = append(p.data, pkt[:255]...)
			pkt = pkt[255:]
		}
		p.segments = append(p.segments, byte(len(pkt)))
		p.data = append(p.data, pkt...)
	}
	return oggSerializePage(p)
}

// oggTestVorbisIdent builds a stereo 44.1kHz Vorbis identification packet
// (30 bytes).
func oggTestVorbisIdent() []byte {
	pkt := []byte{1, 'v', 'o', 'r', 'b', 'i', 's'}
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], 0) // vorbis_version
	pkt = append(pkt, u32[:]...)
	pkt = append(pkt, 2) // channels
	binary.LittleEndian.PutUint32(u32[:], 44100)
	pkt = append(pkt, u32[:]...)
	pkt = append(pkt, u32[:]...)   // bitrate_maximum
	pkt = append(pkt, u32[:]...)   // bitrate_nominal
	pkt = append(pkt, u32[:]...)   // bitrate_minimum
	return append(pkt, 0xB0, 0x01) // blocksizes + framing
}

// oggTestSpeexIdent builds a fixed 80-byte Speex header packet.
func oggTestSpeexIdent(version, channels uint32) []byte {
	pkt := make([]byte, 80)
	copy(pkt[:8], "Speex   ")
	binary.LittleEndian.PutUint32(pkt[28:], version)
	binary.LittleEndian.PutUint32(pkt[36:], 16000) // sample_rate
	binary.LittleEndian.PutUint32(pkt[48:], channels)
	return pkt
}

// oggTestCommentPayload builds the vendor+items region of a comment packet
// (no signature, no framing bit).
func oggTestCommentPayload(vendor string, kvs ...string) []byte {
	var out []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(vendor)))
	out = append(out, u32[:]...)
	out = append(out, vendor...)
	binary.LittleEndian.PutUint32(u32[:], uint32(len(kvs)))
	out = append(out, u32[:]...)
	for _, kv := range kvs {
		binary.LittleEndian.PutUint32(u32[:], uint32(len(kv)))
		out = append(out, u32[:]...)
		out = append(out, kv...)
	}
	return out
}

func oggTestVorbisComment(vendor string, kvs ...string) []byte {
	pkt := append([]byte{3, 'v', 'o', 'r', 'b', 'i', 's'}, oggTestCommentPayload(vendor, kvs...)...)
	return append(pkt, 1) // framing bit
}

func oggTestSetup() []byte {
	return append([]byte{5, 'v', 'o', 'r', 'b', 'i', 's'}, make([]byte, 8)...)
}

// TestOggVorbisSharedHeaderPage feeds a stream whose comment and setup
// packets share one page. A two-call header read leaves the reader mid-page
// and fails; the single `Packets::read_count`-style read handles it.
func TestOggVorbisSharedHeaderPage(t *testing.T) {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestVorbisIdent()})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{
		oggTestVorbisComment("test-vendor", "TITLE=Shared"),
		oggTestSetup(),
	})...)
	file = append(file, oggTestPage(2, 88200, 0, [][]byte{{0xAA}})...)

	props, vc, ft, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("oggReadTagged: %v", err)
	}
	if ft != meta.FileTypeVorbis {
		t.Fatalf("file type = %v, want Vorbis", ft)
	}
	if vc == nil {
		t.Fatal("no vorbis comments parsed")
	}
	if vc.Vendor != "test-vendor" {
		t.Fatalf("vendor = %q, want test-vendor", vc.Vendor)
	}
	if props.Channels != 2 || props.SampleRate != 44100 {
		t.Fatalf("properties = %+v", props)
	}
	if props.Duration != 2*time.Second {
		t.Fatalf("duration = %v, want 2s", props.Duration)
	}
}

// TestOggTruncatedLastPage verifies Rust's `find_last_page` error semantics:
// a last page whose body is truncated aborts the reverse search, so the
// 1000ms length fallback is used (duration stays unset).
func TestOggTruncatedLastPage(t *testing.T) {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestVorbisIdent()})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{oggTestVorbisComment("v"), oggTestSetup()})...)
	// Header parses (complete segment table) but the body is truncated.
	last := oggTestPage(2, 88200, 0, [][]byte{make([]byte, 10)})
	file = append(file, last[:len(last)-5]...)

	props, _, _, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("oggReadTagged: %v", err)
	}
	if props.Duration != 0 {
		t.Fatalf("duration = %v, want 0 (1000ms fallback is bitrate-only)", props.Duration)
	}
}

// TestOggBogusTrailingPage verifies that a false "OggS" candidate with an
// invalid version is skipped and the preceding valid page is used.
func TestOggBogusTrailingPage(t *testing.T) {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestVorbisIdent()})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{oggTestVorbisComment("v"), oggTestSetup()})...)
	file = append(file, oggTestPage(2, 44100, 0, [][]byte{{0xAA}})...)
	bogus := oggTestPage(3, 999999, 0, [][]byte{{0xCC}})
	bogus[4] = 1 // invalid page version
	file = append(file, bogus...)

	props, _, _, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("oggReadTagged: %v", err)
	}
	if props.Duration != time.Second {
		t.Fatalf("duration = %v, want 1s", props.Duration)
	}
}

// TestOggSpeexValidation covers the fixed 80-byte header checks.
func TestOggSpeexValidation(t *testing.T) {
	comment := oggTestCommentPayload("v")
	build := func(ident []byte) []byte {
		var file []byte
		file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{ident})...)
		file = append(file, oggTestPage(1, 0, 0, [][]byte{comment})...)
		return append(file, oggTestPage(2, 16000, 0, [][]byte{{0xAA}})...)
	}

	oggErr := func(t *testing.T, err error) string {
		t.Helper()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var pe *OggParseError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v, want *OggParseError", err)
		}
		return pe.Message
	}

	t.Run("version above 1", func(t *testing.T) {
		_, _, _, err := oggReadTagged(bytes.NewReader(build(oggTestSpeexIdent(2, 2))), meta.NewParseOptions())
		if msg := oggErr(t, err); !strings.Contains(msg, "speex stream version") {
			t.Fatalf("msg = %q", msg)
		}
	})
	t.Run("invalid channel count", func(t *testing.T) {
		_, _, _, err := oggReadTagged(bytes.NewReader(build(oggTestSpeexIdent(1, 3))), meta.NewParseOptions())
		if msg := oggErr(t, err); !strings.Contains(msg, "channel count") {
			t.Fatalf("msg = %q", msg)
		}
	})
	t.Run("header packet too small", func(t *testing.T) {
		_, _, _, err := oggReadTagged(bytes.NewReader(build(oggTestSpeexIdent(1, 2)[:79])), meta.NewParseOptions())
		if msg := oggErr(t, err); !strings.Contains(msg, "too small") {
			t.Fatalf("msg = %q", msg)
		}
	})
	t.Run("valid", func(t *testing.T) {
		props, _, ft, err := oggReadTagged(bytes.NewReader(build(oggTestSpeexIdent(1, 1))), meta.NewParseOptions())
		if err != nil {
			t.Fatalf("oggReadTagged: %v", err)
		}
		if ft != meta.FileTypeSpeex || props.Channels != 1 || props.SampleRate != 16000 {
			t.Fatalf("ft=%v props=%+v", ft, props)
		}
	})
}

// TestOggShortVorbisIdent: an identification packet under 28 bytes errors
// only when properties are requested (Rust reads them lazily).
func TestOggShortVorbisIdent(t *testing.T) {
	ident := append([]byte{1, 'v', 'o', 'r', 'b', 'i', 's'}, make([]byte, 10)...)
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{ident})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{oggTestVorbisComment("v"), oggTestSetup()})...)
	file = append(file, oggTestPage(2, 44100, 0, [][]byte{{0xAA}})...)

	if _, _, _, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions()); err == nil {
		t.Fatal("expected error for short ident with ReadProperties")
	}
	opts := meta.NewParseOptions()
	opts.ReadProperties = false
	if _, vc, _, err := oggReadTagged(bytes.NewReader(file), opts); err != nil || vc == nil {
		t.Fatalf("ReadProperties=false: vc=%v err=%v", vc, err)
	}
}

// oggTestWrite builds a minimal Vorbis stream with the given raw comment
// packet payload (signature included).
func oggTestWrite(comment []byte) *util.BytesFile {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestVorbisIdent()})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{comment, oggTestSetup()})...)
	file = append(file, oggTestPage(2, 44100, 0, [][]byte{{0xAA}})...)
	return util.NewBytesFile(file)
}

func oggTestWriteOptions() meta.WriteOptions {
	return meta.WriteOptions{} // no padding
}

// TestOggWriteTruncatedVendor: Rust reads vendor_len unconditionally, so a
// comment packet too short to hold it is a hard error.
func TestOggWriteTruncatedVendor(t *testing.T) {
	f := oggTestWrite([]byte{3, 'v', 'o', 'r', 'b', 'i', 's', 0xFF, 0xFF})
	err := oggWriteVorbisCommentsImpl(f, meta.FileTypeVorbis, oggTestWriteOptions(), func(string) *vorbis.VorbisCommentsTag {
		return &vorbis.VorbisCommentsTag{}
	})
	if err == nil {
		t.Fatal("expected error for truncated vendor")
	}
}

// TestOggWriteInvalidUtf8Vendor: an invalid UTF-8 vendor is written back as
// empty (Rust's `String::from_utf8` fallback).
func TestOggWriteInvalidUtf8Vendor(t *testing.T) {
	f := oggTestWrite(oggTestVorbisComment("\xFF\xFE", "TITLE=X"))
	if err := oggWriteVorbisCommentsImpl(f, meta.FileTypeVorbis, oggTestWriteOptions(), func(string) *vorbis.VorbisCommentsTag {
		return &vorbis.VorbisCommentsTag{Vendor: "ignored"}
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, vc, _, err := oggReadTagged(bytes.NewReader(f.Bytes()), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if vc == nil {
		t.Fatal("no vorbis comments parsed")
	}
	if vc.Vendor != "" {
		t.Fatalf("vendor = %q, want empty", vc.Vendor)
	}
}

// TestOggWriteFileVendorWins: the file's retained vendor overrides the
// tag's own (Rust `tag.vendor = vendor_str`), and the caller's tag is not
// mutated.
func TestOggWriteFileVendorWins(t *testing.T) {
	f := oggTestWrite(oggTestVorbisComment("file-vendor", "TITLE=X"))
	tag := &vorbis.VorbisCommentsTag{Vendor: "tag-vendor"}
	if err := oggWriteVorbisCommentsImpl(f, meta.FileTypeVorbis, oggTestWriteOptions(), func(string) *vorbis.VorbisCommentsTag {
		return tag
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if tag.Vendor != "tag-vendor" {
		t.Fatalf("caller tag mutated: vendor = %q", tag.Vendor)
	}
	_, vc, _, err := oggReadTagged(bytes.NewReader(f.Bytes()), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if vc == nil {
		t.Fatal("no vorbis comments parsed")
	}
	if vc.Vendor != "file-vendor" {
		t.Fatalf("vendor = %q, want file-vendor", vc.Vendor)
	}
}
