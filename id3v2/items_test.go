package id3v2

// Ported from the `mod tests` blocks in lofty-rs/lofty/src/id3/v2/items/*.rs
// and util/pairs.rs — fixture-based decode/encode tests.

import (
	"bytes"
	"os"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func id3v2ItemAsset(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(id3v2TestAsset(name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fullTestMp3(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../lofty-rs/lofty/tests/files/assets/minimal/full_test.mp3")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// audio_text_frame.rs::atxt_decode / atxt_encode
func TestAtxtDecodeEncode(t *testing.T) {
	want := NewAudioTextFrame(
		TextEncodingLatin1,
		"audio/mpeg",
		AudioTextFrameFlags{Scrambling: false},
		"foo bar baz",
		fullTestMp3(t),
	)
	parsed, err := ParseAudioTextFrame(id3v2ItemAsset(t, "test.atxt"), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Encoding != want.Encoding || parsed.MimeType != want.MimeType ||
		parsed.AudioFlags != want.AudioFlags || parsed.EquivalentText != want.EquivalentText ||
		!bytes.Equal(parsed.AudioData, want.AudioData) {
		t.Fatalf("parsed = %+v", parsed)
	}
	encoded, err := want.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.atxt")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// chapter_frame.rs::chap_decode / chap_encode
func TestChapDecodeEncode(t *testing.T) {
	children := NewFrameList()
	children.Push(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingLatin1, "Chapter 1 - Lofty"))
	want := NewChapterFrame("CH1", 1000, 4000, 80, 256, children)

	parsed, err := parseChapterFrame(bytes.NewReader(id3v2ItemAsset(t, "test.chap")), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Id != want.Id || parsed.StartTime != want.StartTime || parsed.EndTime != want.EndTime ||
		parsed.StartOffset != want.StartOffset || parsed.EndOffset != want.EndOffset {
		t.Fatalf("parsed = %+v", parsed)
	}
	if v, ok := parsed.Children.GetText(FrameIdValid("TIT2")); !ok || v != "Chapter 1 - Lofty" {
		t.Fatalf("TIT2 = %q (ok=%v)", v, ok)
	}
	encoded, err := want.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.chap")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// table_of_contents_frame.rs::ctoc_decode / ctoc_encode / ctoc_many_roots
func TestCtocDecodeEncode(t *testing.T) {
	children := NewFrameList()
	children.Push(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingLatin1, "TOC 1 - The Beginning"))
	want := NewChapterTableOfContentsFrame(
		"TOC1",
		CtocFlags{TopLevel: true, Ordered: true},
		[]string{"CH1", "CH2", "CH3"},
		children,
	)

	parsed, err := parseChapterTableOfContentsFrame(bytes.NewReader(id3v2ItemAsset(t, "test.ctoc")), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Id != want.Id || parsed.TocFlags != want.TocFlags ||
		len(parsed.Entries) != 3 || parsed.Entries[0] != "CH1" || parsed.Entries[1] != "CH2" || parsed.Entries[2] != "CH3" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if v, ok := parsed.Children.GetText(FrameIdValid("TIT2")); !ok || v != "TOC 1 - The Beginning" {
		t.Fatalf("TIT2 = %q (ok=%v)", v, ok)
	}
	encoded, err := want.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.ctoc")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// ctoc_many_roots — only one top-level CTOC may be written; extras are
// discarded during encode.
func TestCtocManyRoots(t *testing.T) {
	newRoot := func(id string) *ChapterTableOfContentsFrame {
		return NewChapterTableOfContentsFrame(id, CtocFlags{TopLevel: true, Ordered: true}, []string{"CH1"}, NewFrameList())
	}
	tag := NewId3v2Tag()
	tag.Frames.Insert(newRoot("TOC1"))
	for _, id := range []string{"TOC2", "TOC3", "TOC4"} {
		tag.Frames.Insert(newRoot(id))
	}
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseId3v2Tag(r, header, meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	var tocs []*ChapterTableOfContentsFrame
	for _, f := range parsed.Frames.Frames() {
		if tc, ok := f.(*ChapterTableOfContentsFrame); ok {
			tocs = append(tocs, tc)
		}
	}
	if len(tocs) != 1 || tocs[0].Id != "TOC1" {
		t.Fatalf("toc frames = %d", len(tocs))
	}
}

// encapsulated_object.rs::geob_decode / geob_encode
func TestGeobDecodeEncode(t *testing.T) {
	want := NewGeneralEncapsulatedObject(
		TextEncodingLatin1,
		meta.Some("audio/mpeg"),
		meta.Some("a.mp3"),
		meta.Some("Test Asset"),
		fullTestMp3(t),
	)
	parsed, err := ParseGeneralEncapsulatedObject(id3v2ItemAsset(t, "test.geob"), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if mt, _ := parsed.MimeType.Get(); mt != "audio/mpeg" {
		t.Fatalf("mime = %q", mt)
	}
	if fn, _ := parsed.FileName.Get(); fn != "a.mp3" {
		t.Fatalf("file_name = %q", fn)
	}
	if d, _ := parsed.Descriptor.Get(); d != "Test Asset" {
		t.Fatalf("descriptor = %q", d)
	}
	if !bytes.Equal(parsed.Data, want.Data) {
		t.Fatal("data mismatch")
	}
	encoded, err := want.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.geob")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// event_timing_codes_frame.rs::etco_decode / etco_encode
func TestEtcoDecodeEncode(t *testing.T) {
	want := NewEventTimingCodesFrame(TimestampFormatMS, []Event{
		{EventType: EventTypeIntroStart, Timestamp: 1500},
		{EventType: EventTypeIntroEnd, Timestamp: 5000},
		{EventType: EventTypeMainPartStart, Timestamp: 7500},
		{EventType: EventTypeMainPartEnd, Timestamp: 900_000},
		{EventType: EventTypeAudioFileEnds, Timestamp: 750_000_000},
	})
	parsed, err := parseEventTimingCodesFrame(bytes.NewReader(id3v2ItemAsset(t, "test.etco")), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 5 || parsed.Events[0].EventType != EventTypeIntroStart ||
		parsed.Events[4].Timestamp != 750_000_000 {
		t.Fatalf("parsed = %+v", parsed.Events)
	}
	encoded, err := want.AsBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.etco")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// key_value_frame.rs::tipl_decode / tipl_encode (+ utf16 variants)
func TestTiplDecodeEncode(t *testing.T) {
	want := func(encoding TextEncoding) *KeyValueFrame {
		f := NewKeyValueFrame(FrameIdValid("TIPL"), encoding, [][2]string{
			{"producer", "Serial-ATA"},
			{"mixer", "lofty-rs"},
		})
		return f
	}

	parsed, err := parseKeyValueFrame(bytes.NewReader(id3v2ItemAsset(t, "test.tipl")), FrameIdValid("TIPL"), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.KeyValuePairs) != 2 ||
		parsed.KeyValuePairs[0] != [2]string{"producer", "Serial-ATA"} ||
		parsed.KeyValuePairs[1] != [2]string{"mixer", "lofty-rs"} {
		t.Fatalf("pairs = %v", parsed.KeyValuePairs)
	}
	encoded, err := want(TextEncodingLatin1).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.tipl")) {
		t.Fatal("latin1 encoded bytes mismatch")
	}

	parsed16, err := parseKeyValueFrame(bytes.NewReader(id3v2ItemAsset(t, "test_utf16.tipl")), FrameIdValid("TIPL"), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed16.KeyValuePairs) != 2 ||
		parsed16.KeyValuePairs[0] != [2]string{"producer", "Serial-ATA"} ||
		parsed16.KeyValuePairs[1] != [2]string{"mixer", "lofty-rs"} {
		t.Fatalf("utf16 pairs = %v", parsed16.KeyValuePairs)
	}
	encoded16, err := want(TextEncodingUTF16).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded16, id3v2ItemAsset(t, "test_utf16.tipl")) {
		t.Fatal("utf16 encoded bytes mismatch")
	}
}

// ownership_frame.rs::owne_decode / owne_encode
func TestOwneDecodeEncode(t *testing.T) {
	parsed, err := parseOwnershipFrame(bytes.NewReader(id3v2ItemAsset(t, "test.owne")), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PricePaid != "USD1000" || parsed.DateOfPurchase != "19840407" || parsed.Seller != "FooBar" {
		t.Fatalf("parsed = %+v", parsed)
	}
	encoded, err := NewOwnershipFrame(TextEncodingLatin1, "USD1000", "19840407", "FooBar").AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.owne")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// popularimeter.rs::write_popm — counter serialization width.
func TestWritePopm(t *testing.T) {
	testPopm := func(t *testing.T, email string, rating uint8, counter uint64) {
		t.Helper()
		b, err := NewPopularimeterFrame(email, rating, counter).AsBytes(meta.NewWriteOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b[:len(email)], []byte(email)) {
			t.Fatalf("email = %q", b[:len(email)])
		}
		if b[len(email)] != 0 {
			t.Fatal("email not NUL-terminated")
		}
		if b[len(email)+1] != rating {
			t.Fatalf("rating = %d", b[len(email)+1])
		}
		var counterLen int
		if counter <= 0xFFFFFFFF {
			counterLen = 4
		} else {
			var cb [8]byte
			for i := 7; i >= 0; i-- {
				cb[i] = byte(counter)
				counter >>= 8
			}
			first := 0
			for first < len(cb) && cb[first] == 0 {
				first++
			}
			counterLen = 8 - first
			if counterLen < 4 {
				counterLen = 4
			}
		}
		if len(b[len(email)+2:]) != counterLen {
			t.Fatalf("counter len = %d, want %d", len(b[len(email)+2:]), counterLen)
		}
	}
	testPopm(t, "foo@bar.com", 255, 0xFFFFFFFF)
	testPopm(t, "baz@qux.com", 196, 0x100000000)
}

// private_frame.rs::priv_decode / priv_encode
func TestPrivDecodeEncode(t *testing.T) {
	parsed, err := parsePrivateFrame(bytes.NewReader(id3v2ItemAsset(t, "test.priv")), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Owner != "foo@bar.com" || string(parsed.PrivateData) != "some data" {
		t.Fatalf("parsed = %+v", parsed)
	}
	encoded, err := NewPrivateFrame("foo@bar.com", []byte("some data")).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.priv")) {
		t.Fatal("encoded bytes mismatch")
	}
}

// relative_volume_adjustment_frame.rs::rva2_decode / rva2_encode
func TestRva2DecodeEncode(t *testing.T) {
	want := NewRelativeVolumeAdjustmentFrame("Surround sound", map[ChannelType]ChannelInformation{
		// NOTE: the fixture stores 0x00f0 (240) for master volume while Rust's
		// expected() says 15 — Rust's PartialEq only compares identification,
		// so upstream never checks channel values. We assert the real decode.
		ChannelTypeMasterVolume: {ChannelType: ChannelTypeMasterVolume, VolumeAdjustment: 240, BitsRepresentingPeak: 4, PeakVolume: meta.Some([]byte{4})},
		ChannelTypeFrontLeft:    {ChannelType: ChannelTypeFrontLeft, VolumeAdjustment: 21, BitsRepresentingPeak: 0, PeakVolume: meta.None[[]byte]()},
		ChannelTypeSubwoofer:    {ChannelType: ChannelTypeSubwoofer, VolumeAdjustment: 30, BitsRepresentingPeak: 11, PeakVolume: meta.Some([]byte{0xFF, 0x07})},
	})
	parsed, err := parseRelativeVolumeAdjustmentFrame(bytes.NewReader(id3v2ItemAsset(t, "test.rva2")), FrameFlags{}, meta.ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Identification != "Surround sound" || len(parsed.Channels) != 3 {
		t.Fatalf("parsed = %+v", parsed)
	}
	for ct, want := range want.Channels {
		got, ok := parsed.Channels[ct]
		if !ok || got.VolumeAdjustment != want.VolumeAdjustment ||
			got.BitsRepresentingPeak != want.BitsRepresentingPeak {
			t.Fatalf("channel %v = %+v, want %+v", ct, got, want)
		}
		gp, gok := got.PeakVolume.Get()
		wp, wok := want.PeakVolume.Get()
		if gok != wok || (gok && !bytes.Equal(gp, wp)) {
			t.Fatalf("channel %v peak = %v, want %v", ct, gp, wp)
		}
	}

	encoded, err := want.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	// Channel order is map-dependent — verify every expected channel blob is present.
	needles := [][]byte{
		{0x01, 0x00, 0xF0, 0x04, 0x04},       // master: adj 240, 4 bits peak, peak=4
		{0x03, 0x00, 0x15, 0x00},             // front-left: adj 21, 0 bits, no peak
		{0x08, 0x00, 0x1E, 0x0B, 0xFF, 0x07}, // subwoofer: adj 30, 11 bits, peak 0xFF07
	}
	body := encoded[len("Surround sound")+1:]
	for _, n := range needles {
		if !bytes.Contains(body, n) {
			t.Fatalf("encoded %x missing channel blob %x", body, n)
		}
	}
}

// sync_text.rs::sylt_decode / sylt_encode (+ utf16 variants)
func TestSyltDecodeEncode(t *testing.T) {
	wantItems := []SyncTextItem{
		{0, "\nLofty"},
		{10000, "\nIs"},
		{15000, "\nReading"},
		{30000, "\nThis"},
		{1938000, "\nCorrectly"},
	}
	want := func(enc TextEncoding) *SynchronizedTextFrame {
		return NewSynchronizedTextFrame(enc, [3]byte{'e', 'n', 'g'}, TimestampFormatMS, SyncTextLyrics, meta.Some("Test Sync Text"), wantItems)
	}
	check := func(f *SynchronizedTextFrame, enc TextEncoding) {
		t.Helper()
		if f.Encoding != enc || f.Language != [3]byte{'e', 'n', 'g'} ||
			f.TimestampFormat != TimestampFormatMS || f.ContentType != SyncTextLyrics {
			t.Fatalf("frame = %+v", f)
		}
		if d, _ := f.Description.Get(); d != "Test Sync Text" {
			t.Fatalf("description = %q", d)
		}
		items := f.Items()
		if len(items) != len(wantItems) {
			t.Fatalf("items = %d", len(items))
		}
		for i, it := range items {
			if it != wantItems[i] {
				t.Fatalf("item[%d] = %+v, want %+v", i, it, wantItems[i])
			}
		}
	}

	parsed, err := ParseSynchronizedTextFrame(id3v2ItemAsset(t, "test.sylt"), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	check(parsed, TextEncodingLatin1)
	encoded, err := want(TextEncodingLatin1).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, id3v2ItemAsset(t, "test.sylt")) {
		t.Fatal("latin1 encoded bytes mismatch")
	}

	parsed16, err := ParseSynchronizedTextFrame(id3v2ItemAsset(t, "test_utf16.sylt"), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	check(parsed16, TextEncodingUTF16)
	encoded16, err := want(TextEncodingUTF16).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded16, id3v2ItemAsset(t, "test_utf16.sylt")) {
		t.Fatal("utf16 encoded bytes mismatch")
	}
}

// unique_file_identifier.rs::issue_204_invalid_ufid_parsing_mode_best_attempt
func TestIssue204InvalidUfid(t *testing.T) {
	b, err := NewUniqueFileIdentifierFrame("", []byte{0}).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseUniqueFileIdentifierFrame(bytes.NewReader(b), FrameFlags{}, meta.ParsingModeStrict); err == nil {
		t.Fatal("strict should fail on empty owner")
	}
	if _, err := parseUniqueFileIdentifierFrame(bytes.NewReader(b), FrameFlags{}, meta.ParsingModeBestAttempt); err != nil {
		t.Fatalf("best-attempt should tolerate empty owner: %v", err)
	}
}

// pairs.rs::whitespace_in_number / empty_number_string — Rust's trimming
// `set_number` is used by the APE conversion; id3v2 `from_tag` uses a
// non-trimming `parse_number`, so whitespace-padded text produces no frame.
func TestNumberPairWhitespace(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetTrack(12)
	if v, ok := tag.Track().Get(); !ok || v != 12 {
		t.Fatalf("track = %v (ok=%v)", v, ok)
	}

	gtag := meta.NewTag(meta.TagTypeID3v2)
	gtag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("  12  ")))
	itag := Id3v2TagFromTag(gtag)
	if _, ok := itag.Frames.GetText(FrameIdValid("TRCK")); ok {
		t.Fatal("whitespace-padded TRCK should produce no frame (Rust parse_number does not trim)")
	}

	gtag2 := meta.NewTag(meta.TagTypeID3v2)
	gtag2.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("        ")))
	itag2 := Id3v2TagFromTag(gtag2)
	if _, ok := itag2.Frames.GetText(FrameIdValid("TRCK")); ok {
		t.Fatal("whitespace-only TRCK should produce no frame")
	}
}

// Ported from lofty-rs/lofty/tests/fuzz/id3v2.rs (issue #295).
func TestId3v2FuzzRegressionUnreachable1(t *testing.T) {
	data := []byte{1, 0, 0, 0}
	ver := NewId3v2Tag().OriginalVersion()
	_, _ = parseExtendedTextFrame(bytes.NewReader(data), FrameFlags{}, ver)
}

func TestId3v2FuzzRegressionOverflow1(t *testing.T) {
	data := []byte{
		57, 25, 25, 0, 4, 1, 54, 0, 51, 6, 6, 6, 25, 25, 25, 129, 6, 151, 28, 25, 25, 0, 51, 51,
		50, 5, 5, 5, 26, 5, 5, 25, 6, 6, 25, 26, 246, 25, 25, 129, 6, 151, 3, 252, 56, 0, 53, 56,
		55, 52,
	}
	_, _ = parseRelativeVolumeAdjustmentFrame(bytes.NewReader(data), FrameFlags{}, meta.ParsingModeBestAttempt)
}
