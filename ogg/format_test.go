package ogg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
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

func oggTestOpusIdent() []byte {
	pkt := []byte("OpusHead")
	pkt = append(pkt, 1, 2, 0, 0) // version, channels, pre-skip
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], 48000)
	pkt = append(pkt, u32[:]...)
	return append(pkt, 0, 0, 0) // gain, mapping family
}

func oggTestVorbisStream(ident []byte, lastGranule uint64) []byte {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{ident})...)
	file = append(file, oggTestPage(1, 0, 0, [][]byte{oggTestVorbisComment("v"), oggTestSetup()})...)
	return append(file, oggTestPage(2, lastGranule, 0, [][]byte{{0xAA}})...)
}

// TestOggReadersHonourRequestedType: like Rust's per-type `read_from`, a
// reader fails with "file missing magic signature" when the stream carries a
// different codec, while the sniffing entry point still detects the type.
func TestOggReadersHonourRequestedType(t *testing.T) {
	vorbisFile := oggTestVorbisStream(oggTestVorbisIdent(), 44100)

	var opusFile []byte
	opusFile = append(opusFile, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestOpusIdent()})...)
	opusFile = append(opusFile, oggTestPage(1, 0, 0, [][]byte{append([]byte("OpusTags"), oggTestCommentPayload("v")...)})...)
	opusFile = append(opusFile, oggTestPage(2, 48000, 0, [][]byte{{0xAA}})...)

	wantMagicErr := func(t *testing.T, err error) {
		t.Helper()
		var pe *OggParseError
		if !errors.As(err, &pe) || pe.Message != "file missing magic signature" {
			t.Fatalf("err = %v, want OggParseError(file missing magic signature)", err)
		}
	}

	if _, err := OggOpusFileReadFrom(bytes.NewReader(vorbisFile), meta.NewParseOptions()); err == nil {
		t.Fatal("Opus reader accepted a Vorbis stream")
	} else {
		wantMagicErr(t, err)
	}
	if _, err := OggSpeexFileReadFrom(bytes.NewReader(vorbisFile), meta.NewParseOptions()); err == nil {
		t.Fatal("Speex reader accepted a Vorbis stream")
	} else {
		wantMagicErr(t, err)
	}
	if _, err := OggVorbisFileReadFrom(bytes.NewReader(opusFile), meta.NewParseOptions()); err == nil {
		t.Fatal("Vorbis reader accepted an Opus stream")
	} else {
		wantMagicErr(t, err)
	}

	// Mismatches are hard errors even without tags/properties
	bare := meta.NewParseOptions()
	bare.ReadTags, bare.ReadProperties = false, false
	if _, err := OggOpusFileReadFrom(bytes.NewReader(vorbisFile), bare); err == nil {
		t.Fatal("Opus reader accepted a Vorbis stream with ReadTags=false")
	}

	if f, err := OggVorbisFileReadFrom(bytes.NewReader(vorbisFile), meta.NewParseOptions()); err != nil || f.Properties.Duration != time.Second {
		t.Fatalf("Vorbis reader: f=%+v err=%v", f, err)
	}
	if f, err := OggOpusFileReadFrom(bytes.NewReader(opusFile), meta.NewParseOptions()); err != nil || f.Properties.Duration != time.Second {
		t.Fatalf("Opus reader: f=%+v err=%v", f, err)
	}

	// The sniffing entry point (used when the type is not known up front)
	if _, _, ft, err := oggReadTagged(bytes.NewReader(opusFile), meta.NewParseOptions()); err != nil || ft != meta.FileTypeOpus {
		t.Fatalf("sniff opus: ft=%v err=%v", ft, err)
	}
	if _, _, ft, err := oggReadTagged(bytes.NewReader(vorbisFile), meta.NewParseOptions()); err != nil || ft != meta.FileTypeVorbis {
		t.Fatalf("sniff vorbis: ft=%v err=%v", ft, err)
	}
}

// TestOggLastPageSearchReachesStreamStart: Rust's first reverse search in
// `find_last_page` runs to the stream start; only retries stop at the header
// end. A stream whose last page is a header page therefore still gets a
// duration from that page.
func TestOggLastPageSearchReachesStreamStart(t *testing.T) {
	var file []byte
	file = append(file, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestVorbisIdent()})...)
	file = append(file, oggTestPage(1, 88200, 0, [][]byte{oggTestVorbisComment("v"), oggTestSetup()})...)

	props, _, _, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("oggReadTagged: %v", err)
	}
	if props.Duration != 2*time.Second {
		t.Fatalf("duration = %v, want 2s", props.Duration)
	}
}

// TestOggVorbisDurationUses128Bit: `total_samples * 1000` overflows u64 here;
// Rust's Vorbis math is u128, so the duration must not wrap.
func TestOggVorbisDurationUses128Bit(t *testing.T) {
	ident := oggTestVorbisIdent()
	binary.LittleEndian.PutUint32(ident[12:], 4_000_000_000) // sample rate
	const total = uint64(2e16)                               // 2e19 > MaxUint64 once multiplied by 1000

	props, _, _, err := oggReadTagged(bytes.NewReader(oggTestVorbisStream(ident, total)), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("oggReadTagged: %v", err)
	}
	if want := util.DurationFromMillis(5_000_000_000); props.Duration != want {
		t.Fatalf("duration = %v, want %v", props.Duration, want)
	}
}

// TestOggDurationSaturates: an absurd granule position saturates the Duration
// instead of overflowing it (Rust `Duration` holds u64 seconds).
func TestOggDurationSaturates(t *testing.T) {
	huge := uint64(1) << 62
	for name, file := range map[string][]byte{
		"vorbis": oggTestVorbisStream(oggTestVorbisIdent(), huge),
		"speex": func() []byte {
			var f []byte
			f = append(f, oggTestPage(0, 0, oggPageBOS, [][]byte{oggTestSpeexIdent(1, 2)})...)
			f = append(f, oggTestPage(1, 0, 0, [][]byte{oggTestCommentPayload("v")})...)
			return append(f, oggTestPage(2, huge, 0, [][]byte{{0xAA}})...)
		}(),
	} {
		props, _, _, err := oggReadTagged(bytes.NewReader(file), meta.NewParseOptions())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if props.Duration != time.Duration(1<<63-1) {
			t.Fatalf("%s: duration = %v, want saturated", name, props.Duration)
		}
	}
}

// TestOggWriteBadCommentSignature: a comment packet without the codec's
// signature fails with the same "file missing magic signature" parse error as
// the read path (Rust `verify_signature`).
func TestOggWriteBadCommentSignature(t *testing.T) {
	f := oggTestWrite([]byte{9, 'n', 'o', 't', 'v', 'o', 'r', 0, 0, 0, 0, 0, 0, 0, 0})
	err := oggWriteVorbisCommentsImpl(f, meta.FileTypeVorbis, oggTestWriteOptions(), func(string) *vorbis.VorbisCommentsTag {
		return &vorbis.VorbisCommentsTag{}
	})
	var fpe *meta.FileParseError
	var pe *OggParseError
	if !errors.As(err, &fpe) || !errors.As(err, &pe) || pe.Message != "file missing magic signature" {
		t.Fatalf("err = %v, want FileParseError(OggParseError(file missing magic signature))", err)
	}
}

func oggPagerAsset(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "lofty-rs", "ogg_pager", "test_assets", name)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("ogg_pager test asset missing:", p)
		}
		t.Fatal(err)
	}
	return b
}

func TestOggPagerOpusIdentHeader(t *testing.T) {
	content := oggPagerAsset(t, "opus_ident_header.page")
	page, err := readOggPage(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("readOggPage: %v", err)
	}
	wantData := []byte{
		0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 0x01, 0x02, 0x38, 0x01, 0x80, 0xBB,
		0, 0, 0, 0, 0,
	}
	if !bytes.Equal(page.data, wantData) {
		t.Fatalf("data = %x, want %x", page.data, wantData)
	}
	if page.start != 0 || page.end != 47 {
		t.Fatalf("bounds = [%d, %d), want [0, 47)", page.start, page.end)
	}
	if page.headerType != 2 || page.granulePos != 0 || page.serial != 1759377061 || page.sequence != 0 || page.crc != 3579522525 {
		t.Fatalf("header = %+v", page)
	}
	if !bytes.Equal(page.segments, []byte{19}) {
		t.Fatalf("segments = %v, want [19]", page.segments)
	}
}

func TestOggPagerPaginateLarge(t *testing.T) {
	packet := oggPagerAsset(t, "large_comment_packet.page")
	pages := oggPaginate([][]byte{packet}, 1234, 0, 0)
	if len(pages) != 130 {
		t.Fatalf("len(pages) = %d, want 130", len(pages))
	}
	last := pages[len(pages)-1]
	if got := len(last.data) % 255; got != 171 {
		t.Fatalf("last page len %% 255 = %d, want 171", got)
	}
	for i, page := range pages {
		if page.serial != 1234 {
			t.Fatalf("page %d serial = %d, want 1234", i, page.serial)
		}
		wantGranule := uint64(math.MaxUint64)
		if i+1 == len(pages) {
			wantGranule = 0
		}
		if page.granulePos != wantGranule {
			t.Fatalf("page %d granulePos = %d, want %d", i, page.granulePos, wantGranule)
		}
		wantType := oggPageContinuation
		if i == 0 {
			wantType = 0
		}
		if page.headerType != wantType {
			t.Fatalf("page %d headerType = %d, want %d", i, page.headerType, wantType)
		}
		if i+1 == len(pages) {
			for j := 0; j < len(page.segments)-1; j++ {
				if page.segments[j] != 255 {
					t.Fatalf("last page segment %d = %d, want 255", j, page.segments[j])
				}
			}
			if page.segments[len(page.segments)-1] != 171 {
				t.Fatalf("last page final segment = %d, want 171", page.segments[len(page.segments)-1])
			}
		} else {
			if len(page.segments) != oggMaxWrittenSegmentCount {
				t.Fatalf("page %d segments len = %d, want %d", i, len(page.segments), oggMaxWrittenSegmentCount)
			}
			for j, s := range page.segments {
				if s != 255 {
					t.Fatalf("page %d segment %d = %d, want 255", i, j, s)
				}
			}
		}
	}
}

func TestOggPagerPaginateLargePerfectlyDivisible(t *testing.T) {
	const pagesToWrite = 20
	const packetSize = (oggMaxWrittenSegmentCount * 255) * pagesToWrite
	packet := make([]byte, packetSize)
	pages := oggPaginate([][]byte{packet}, 1234, 0, 0)
	if len(pages) != pagesToWrite+1 {
		t.Fatalf("len(pages) = %d, want %d", len(pages), pagesToWrite+1)
	}
	for i := 0; i < len(pages)-1; i++ {
		for j, s := range pages[i].segments {
			if s != 255 {
				t.Fatalf("page %d segment %d = %d, want 255", i, j, s)
			}
		}
	}
	last := pages[len(pages)-1]
	if !bytes.Equal(last.segments, []byte{0}) {
		t.Fatalf("last segments = %v, want [0]", last.segments)
	}
	totalSize := 0
	for _, p := range pages {
		for _, s := range p.segments {
			totalSize += int(s)
		}
	}
	if totalSize != packetSize {
		t.Fatalf("totalSize = %d, want %d", totalSize, packetSize)
	}
}

func TestOggPagerPaginatePerfectlyDivisibleTerminate(t *testing.T) {
	const segments = 17
	const packetSize = segments * 255
	packet := make([]byte, packetSize)
	pages := oggPaginate([][]byte{packet}, 1234, 0, 0)
	if len(pages) != 1 {
		t.Fatalf("len(pages) = %d, want 1", len(pages))
	}
	page := pages[0]
	if len(page.segments) != segments+1 {
		t.Fatalf("len(segments) = %d, want %d", len(page.segments), segments+1)
	}
	for i := 0; i < segments; i++ {
		if page.segments[i] != 255 {
			t.Fatalf("segment %d = %d, want 255", i, page.segments[i])
		}
	}
	if page.segments[segments] != 0 {
		t.Fatalf("terminating segment = %d, want 0", page.segments[segments])
	}
}

func TestOggPagerPacketRemainder(t *testing.T) {
	packetsData := [][]byte{[]byte("123"), []byte("456")}
	pages := oggPaginate(packetsData, 1234, 0, 0)
	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}
	encoded := oggSerializePage(pages[0])
	encoded[26] = 2
	// Insert second packet segment length at index 27
	encoded = append(encoded[:27], append([]byte{byte(len(pages[1].data))}, encoded[27:]...)...)
	encoded = append(encoded, pages[1].data...)

	packets, remainder, _, err := OggReadCountPackets(bytes.NewReader(encoded), 1)
	if err != nil {
		t.Fatalf("OggReadCountPackets: %v", err)
	}
	if remainder == nil {
		t.Fatal("expected non-nil remainder page")
	}
	if len(packets) != 1 || !bytes.Equal(packets[0], packetsData[0]) {
		t.Fatalf("packets = %q, want [%q]", packets, packetsData[0])
	}
	if !bytes.Equal(remainder.data, packetsData[1]) {
		t.Fatalf("remainder.data = %q, want %q", remainder.data, packetsData[1])
	}
}
