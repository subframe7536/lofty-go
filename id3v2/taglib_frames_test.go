package id3v2

// Ported from lofty-rs/lofty/tests/taglib/test_id3v2.rs — frame-level
// parse/render tests.

import (
	"bytes"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// test_parse_apic
func TestTaglibParseApic(t *testing.T) {
	data := []byte{
		0x00,
		'm', 0x00,
		0x01,
		'd', 0x00,
		0x00,
	}
	f, err := parseAttachedPictureFrame(bytes.NewReader(data), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if mt, ok := f.Picture.MimeType().Get(); !ok || mt.String() != "m" {
		t.Fatalf("mime = %v (ok=%v)", mt, ok)
	}
	if f.Picture.PicType() != meta.PictureTypeIcon {
		t.Fatalf("pic_type = %v", f.Picture.PicType())
	}
	if d, ok := f.Picture.Description(); !ok || d != "d" {
		t.Fatalf("desc = %q (ok=%v)", d, ok)
	}
}

// test_render_apic
func TestTaglibRenderApic(t *testing.T) {
	pic := meta.PictureUnchecked([]byte("PNG data")).
		PicType(meta.PictureTypeCoverBack).
		MimeType(meta.MimeTypePng).
		Description("Description").
		Build()
	f := NewAttachedPictureFrame(TextEncodingUTF8, pic)

	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x03,
		'i', 'm', 'a', 'g', 'e', '/', 'p', 'n', 'g', 0x00,
		0x04,
		'D', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', 0x00,
		'P', 'N', 'G', ' ', 'd', 'a', 't', 'a',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_geob
func TestTaglibParseGeob(t *testing.T) {
	data := []byte{
		0x00,
		'm', 0x00,
		'f', 0x00,
		'd', 0x00,
		0x00,
	}
	f, err := ParseGeneralEncapsulatedObject(data, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if mt, ok := f.MimeType.Get(); !ok || mt != "m" {
		t.Fatalf("mime = %q (ok=%v)", mt, ok)
	}
	if fn, ok := f.FileName.Get(); !ok || fn != "f" {
		t.Fatalf("file_name = %q (ok=%v)", fn, ok)
	}
	if d, ok := f.Descriptor.Get(); !ok || d != "d" {
		t.Fatalf("descriptor = %q (ok=%v)", d, ok)
	}
}

// test_render_geob
func TestTaglibRenderGeob(t *testing.T) {
	f := NewGeneralEncapsulatedObject(
		TextEncodingLatin1,
		meta.Some("application/octet-stream"),
		meta.Some("test.bin"),
		meta.Some("Description"),
		[]byte{0x01, 0x01, 0x01},
	)
	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00,
		'a', 'p', 'p', 'l', 'i', 'c', 'a', 't', 'i', 'o', 'n', '/', 'o', 'c', 't', 'e', 't', '-', 's', 't', 'r', 'e', 'a', 'm', 0x00,
		't', 'e', 's', 't', '.', 'b', 'i', 'n', 0x00,
		'D', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', 0x00,
		0x01, 0x01, 0x01,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_popm / test_parse_popm_without_counter
func TestTaglibParsePopm(t *testing.T) {
	f, err := parsePopularimeterFrame(bytes.NewReader([]byte{
		'e', 'm', 'a', 'i', 'l', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00,
		0x02,
		0x00, 0x00, 0x00, 0x03,
	}), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Email != "email@example.com" {
		t.Fatalf("email = %q", f.Email)
	}
	if f.Rating != 2 {
		t.Fatalf("rating = %d", f.Rating)
	}
	if f.Counter != 3 {
		t.Fatalf("counter = %d", f.Counter)
	}

	f, err = parsePopularimeterFrame(bytes.NewReader([]byte{
		'e', 'm', 'a', 'i', 'l', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00,
		0x02,
	}), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Email != "email@example.com" {
		t.Fatalf("email = %q", f.Email)
	}
	if f.Rating != 2 {
		t.Fatalf("rating = %d", f.Rating)
	}
	if f.Counter != 0 {
		t.Fatalf("counter = %d", f.Counter)
	}
}

// test_render_popm
func TestTaglibRenderPopm(t *testing.T) {
	f := NewPopularimeterFrame("email@example.com", 2, 3)
	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		'e', 'm', 'a', 'i', 'l', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00,
		0x02,
		0x00, 0x00, 0x00, 0x03,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_relative_volume_frame
func TestTaglibParseRelativeVolumeFrame(t *testing.T) {
	data := []byte{
		'i', 'd', 'e', 'n', 't', 0x00,
		0x02,
		0x00, 0x0F,
		0x08,
		0x45,
	}
	f, err := parseRelativeVolumeAdjustmentFrame(bytes.NewReader(data), FrameFlags{}, meta.ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Identification != "ident" {
		t.Fatalf("identification = %q", f.Identification)
	}
	if len(f.Channels) != 1 {
		t.Fatalf("channels = %d", len(f.Channels))
	}
	fr, ok := f.Channels[ChannelTypeFrontRight]
	if !ok {
		t.Fatal("no FrontRight channel")
	}
	if fr.VolumeAdjustment != 15 {
		t.Fatalf("volume_adjustment = %d", fr.VolumeAdjustment)
	}
	if fr.BitsRepresentingPeak != 8 {
		t.Fatalf("bits_representing_peak = %d", fr.BitsRepresentingPeak)
	}
	if pv, ok := fr.PeakVolume.Get(); !ok || !bytes.Equal(pv, []byte{0x45}) {
		t.Fatalf("peak_volume = %v (ok=%v)", pv, ok)
	}
}

// test_render_relative_volume_frame
func TestTaglibRenderRelativeVolumeFrame(t *testing.T) {
	f := NewRelativeVolumeAdjustmentFrame("ident", map[ChannelType]ChannelInformation{
		ChannelTypeFrontRight: {
			ChannelType:          ChannelTypeFrontRight,
			VolumeAdjustment:     15,
			BitsRepresentingPeak: 8,
			PeakVolume:           meta.Some([]byte{0x45}),
		},
	})
	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		'i', 'd', 'e', 'n', 't', 0x00,
		0x02,
		0x00, 0x0F,
		0x08,
		0x45,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_unique_file_identifier_frame / empty / render
func TestTaglibUniqueFileIdentifierFrame(t *testing.T) {
	f, err := parseUniqueFileIdentifierFrame(bytes.NewReader([]byte{
		'o', 'w', 'n', 'e', 'r', 0x00,
		0x00, 0x01, 0x02,
	}), FrameFlags{}, meta.ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Owner != "owner" {
		t.Fatalf("owner = %q", f.Owner)
	}
	if !bytes.Equal(f.Identifier, []byte{0x00, 0x01, 0x02}) {
		t.Fatalf("identifier = %v", f.Identifier)
	}

	// Empty UFID — TagLib treats a missing owner as valid; Lofty does not.
	if _, err := parseUniqueFileIdentifierFrame(bytes.NewReader([]byte{0x00}), FrameFlags{}, meta.ParsingModeStrict); err == nil {
		t.Fatal("expected error for empty UFID")
	}

	got, err := NewUniqueFileIdentifierFrame("owner", []byte{0x01, 0x02, 0x03}).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{'o', 'w', 'n', 'e', 'r', 0x00, 0x01, 0x02, 0x03}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_url_link_frame / test_render_url_link_frame
func TestTaglibUrlLinkFrame(t *testing.T) {
	data := []byte("http://example.com")
	f, err := parseUrlLinkFrame(bytes.NewReader(data), FrameIdValid("WPUB"), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Url() != "http://example.com" {
		t.Fatalf("url = %q", f.Url())
	}
	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
}

// test_parse_user_url_link_frame / test_render_user_url_link_frame
func TestTaglibUserUrlLinkFrame(t *testing.T) {
	f, err := parseExtendedUrlFrame(bytes.NewReader([]byte{
		0x00,
		'f', 'o', 'o', 0x00,
		'h', 't', 't', 'p', ':', '/', '/', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm',
	}), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Description != "foo" {
		t.Fatalf("description = %q", f.Description)
	}
	if f.Content != "http://example.com" {
		t.Fatalf("content = %q", f.Content)
	}

	got, err := NewExtendedUrlFrame(TextEncodingLatin1, "foo", "http://example.com").AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00,
		'f', 'o', 'o', 0x00,
		'h', 't', 't', 'p', ':', '/', '/', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_ownership_frame / test_render_ownership_frame
func TestTaglibOwnershipFrame(t *testing.T) {
	data := []byte{
		0x00,
		'G', 'B', 'P', '1', '.', '9', '9', 0x00,
		'2', '0', '1', '2', '0', '9', '0', '5',
		'B', 'e', 'a', 't', 'p', 'o', 'r', 't',
	}
	f, err := parseOwnershipFrame(bytes.NewReader(data), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.PricePaid != "GBP1.99" {
		t.Fatalf("price = %q", f.PricePaid)
	}
	if f.DateOfPurchase != "20120905" {
		t.Fatalf("date = %q", f.DateOfPurchase)
	}
	if f.Seller != "Beatport" {
		t.Fatalf("seller = %q", f.Seller)
	}

	got, err := NewOwnershipFrame(TextEncodingLatin1, "GBP1.99", "20120905", "Beatport").AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
}

// test_parse_synchronized_lyrics_frame (+ empty description variant) / render
func TestTaglibSynchronizedLyricsFrame(t *testing.T) {
	parseData := func(desc []byte) []byte {
		out := []byte{0x00}
		out = append(out, 'e', 'n', 'g', 0x02, 0x01)
		out = append(out, desc...)
		out = append(out,
			'E', 'x', 'a', 'm', 'p', 'l', 'e', 0x00,
			0x00, 0x00, 0x04, 0xd2,
			'L', 'y', 'r', 'i', 'c', 's', 0x00,
			0x00, 0x00, 0x11, 0xd7)
		return out
	}

	f, err := ParseSynchronizedTextFrame(parseData([]byte{'f', 'o', 'o', 0x00}), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Encoding != TextEncodingLatin1 {
		t.Fatalf("encoding = %v", f.Encoding)
	}
	if f.Language != [3]byte{'e', 'n', 'g'} {
		t.Fatalf("language = %v", f.Language)
	}
	if f.TimestampFormat != TimestampFormatMS {
		t.Fatalf("timestamp_format = %v", f.TimestampFormat)
	}
	if f.ContentType != SyncTextLyrics {
		t.Fatalf("content_type = %v", f.ContentType)
	}
	if d, ok := f.Description.Get(); !ok || d != "foo" {
		t.Fatalf("description = %q (ok=%v)", d, ok)
	}
	items := f.Items()
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Timestamp != 1234 || items[0].Text != "Example" {
		t.Fatalf("item[0] = %+v", items[0])
	}
	if items[1].Timestamp != 4567 || items[1].Text != "Lyrics" {
		t.Fatalf("item[1] = %+v", items[1])
	}

	// Empty description — description should be None.
	f, err = ParseSynchronizedTextFrame(parseData([]byte{0x00}), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := f.Description.Get(); ok {
		t.Fatalf("description = %q, want none", d)
	}
	items = f.Items()
	if len(items) != 2 || items[0].Text != "Example" || items[1].Text != "Lyrics" {
		t.Fatalf("items = %+v", items)
	}

	// Render
	f = NewSynchronizedTextFrame(
		TextEncodingLatin1,
		[3]byte{'e', 'n', 'g'},
		TimestampFormatMS,
		SyncTextLyrics,
		meta.Some("foo"),
		[]SyncTextItem{{1234, "Example"}, {4567, "Lyrics"}},
	)
	got, err := f.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, parseData([]byte{'f', 'o', 'o', 0x00})) {
		t.Fatalf("bytes = %q", got)
	}
}

// test_parse_event_timing_codes_frame / test_render_event_timing_codes_frame
func TestTaglibEventTimingCodesFrame(t *testing.T) {
	data := []byte{
		0x02,
		0x02,
		0x00, 0x00, 0xf3, 0x5c,
		0xfe,
		0x00, 0x36, 0xee, 0x80,
	}
	f, err := parseEventTimingCodesFrame(bytes.NewReader(data), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.TimestampFormat != TimestampFormatMS {
		t.Fatalf("timestamp_format = %v", f.TimestampFormat)
	}
	if len(f.Events) != 2 {
		t.Fatalf("events = %d", len(f.Events))
	}
	if f.Events[0].EventType != EventTypeIntroStart || f.Events[0].Timestamp != 62300 {
		t.Fatalf("event[0] = %+v", f.Events[0])
	}
	if f.Events[1].EventType != EventTypeAudioFileEnds || f.Events[1].Timestamp != 3600000 {
		t.Fatalf("event[1] = %+v", f.Events[1])
	}

	got, err := NewEventTimingCodesFrame(TimestampFormatMS, []Event{
		{EventType: EventTypeIntroStart, Timestamp: 62300},
		{EventType: EventTypeAudioFileEnds, Timestamp: 3600000},
	}).AsBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
}

// test_parse_comments_frame / test_render_comments_frame
func TestTaglibCommentsFrame(t *testing.T) {
	data := []byte{
		0x03,
		'd', 'e', 'u',
		'D', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', 0x00,
		'T', 'e', 'x', 't',
	}
	f, err := parseCommentFrame(bytes.NewReader(data), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Encoding != TextEncodingUTF8 {
		t.Fatalf("encoding = %v", f.Encoding)
	}
	if f.Language != [3]byte{'d', 'e', 'u'} {
		t.Fatalf("language = %v", f.Language)
	}
	if f.Description != "Description" {
		t.Fatalf("description = %q", f.Description)
	}
	if f.Content != "Text" {
		t.Fatalf("content = %q", f.Content)
	}

	got, err := NewCommentFrame(TextEncodingUTF16, [3]byte{'e', 'n', 'g'}, "Description", "Text").AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x01,
		'e', 'n', 'g',
		0xff, 0xfe, 'D', 0, 'e', 0, 's', 0, 'c', 0, 'r', 0, 'i', 0, 'p', 0, 't', 0, 'i', 0, 'o', 0, 'n', 0, 0x00, 0x00,
		0xff, 0xfe, 'T', 0, 'e', 0, 'x', 0, 't', 0,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_private_frame / test_render_private_frame
func TestTaglibPrivateFrame(t *testing.T) {
	data := []byte{
		'W', 'M', '/', 'P', 'r', 'o', 'v', 'i', 'd', 'e', 'r', 0x00,
		'T', 'L',
	}
	f, err := parsePrivateFrame(bytes.NewReader(data), FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("frame is nil")
	}
	if f.Owner != "WM/Provider" {
		t.Fatalf("owner = %q", f.Owner)
	}
	if !bytes.Equal(f.PrivateData, []byte{'T', 'L'}) {
		t.Fatalf("private_data = %v", f.PrivateData)
	}

	got, err := NewPrivateFrame("WM/Provider", []byte{'T', 'L'}).AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
}

// test_parse_user_text_identification_frame / test_render_user_text_identification_frame
func TestTaglibUserTextIdentificationFrame(t *testing.T) {
	// Rust parses the same layout through ExtendedUrlFrame — description is
	// an empty string when absent.
	f, err := parseExtendedUrlFrame(bytes.NewReader([]byte{0x00, 0x00, 'T', 'e', 'x', 't'}), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if f.Description != "" {
		t.Fatalf("description = %q", f.Description)
	}
	if f.Content != "Text" {
		t.Fatalf("content = %q", f.Content)
	}

	f, err = parseExtendedUrlFrame(bytes.NewReader([]byte{
		0x00,
		'D', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', 0x00,
		'T', 'e', 'x', 't',
	}), FrameFlags{}, id3v2Version4)
	if err != nil {
		t.Fatal(err)
	}
	if f.Description != "Description" {
		t.Fatalf("description = %q", f.Description)
	}
	if f.Content != "Text" {
		t.Fatalf("content = %q", f.Content)
	}

	f2 := NewExtendedTextFrame(TextEncodingLatin1, "", "Text")
	got, err := f2.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x00, 0x00, 'T', 'e', 'x', 't'}) {
		t.Fatalf("bytes = %q", got)
	}
	f2.Description = "Description"
	got, err = f2.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00,
		'D', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', 0x00,
		'T', 'e', 'x', 't',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_chapter_frame / test_render_chapter_frame
func TestTaglibChapterFrame(t *testing.T) {
	chapterData := []byte{
		0x43, 0x00,
		0x00, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x05,
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x03,
	}
	embeddedFrameData := []byte{
		'T', 'I', 'T', '2',
		0x00, 0x00, 0x00, 0x04,
		0x00, 0x00,
		0x00,
		'C', 'H', '1',
	}

	f1, err := parseChapterFrame(bytes.NewReader(chapterData), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f1.Id != "C" {
		t.Fatalf("id = %q", f1.Id)
	}
	if f1.StartTime != 3 || f1.EndTime != 5 || f1.StartOffset != 2 || f1.EndOffset != 3 {
		t.Fatalf("times = %d..%d, offsets = %d..%d", f1.StartTime, f1.EndTime, f1.StartOffset, f1.EndOffset)
	}
	if f1.Children.Len() != 0 {
		t.Fatalf("children = %d", f1.Children.Len())
	}

	withTIT2 := append(append([]byte{}, chapterData...), embeddedFrameData...)
	f2, err := parseChapterFrame(bytes.NewReader(withTIT2), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f2.Children.Len() != 1 {
		t.Fatalf("children = %d", f2.Children.Len())
	}
	if v, ok := f2.Children.GetText(FrameIdValid("TIT2")); !ok || v != "CH1" {
		t.Fatalf("TIT2 = %q (ok=%v)", v, ok)
	}

	// Render
	children := NewFrameList()
	children.Insert(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingLatin1, "CH1"))
	fr := NewChapterFrame("\x43", 3, 5, 2, 3, children)
	got, err := fr.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte{}, chapterData...), embeddedFrameData...)
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

// test_parse_table_of_contents_frame / test_render_table_of_contents_frame
func TestTaglibTableOfContentsFrame(t *testing.T) {
	data := []byte{
		0x54, 0x00,
		0x01,
		0x02,
		0x43, 0x00,
		0x44, 0x00,
		'T', 'I', 'T', '2',
		0x00, 0x00, 0x00, 0x04,
		0x00, 0x00,
		0x00,
		'T', 'C', '1',
	}

	f, err := parseChapterTableOfContentsFrame(bytes.NewReader(data), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Id != "T" {
		t.Fatalf("id = %q", f.Id)
	}
	if f.TocFlags.TopLevel {
		t.Fatal("top_level set")
	}
	if !f.TocFlags.Ordered {
		t.Fatal("ordered not set")
	}
	if len(f.Entries) != 2 || f.Entries[0] != "C" || f.Entries[1] != "D" {
		t.Fatalf("entries = %v", f.Entries)
	}
	if f.Children.Len() != 1 {
		t.Fatalf("children = %d", f.Children.Len())
	}
	if v, ok := f.Children.GetText(FrameIdValid("TIT2")); !ok || v != "TC1" {
		t.Fatalf("TIT2 = %q (ok=%v)", v, ok)
	}

	children := NewFrameList()
	children.Insert(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingLatin1, "TC1"))
	fr := NewChapterTableOfContentsFrame("T", CtocFlags{TopLevel: false, Ordered: true}, []string{"C", "D"}, children)
	got, err := fr.AsBytes(meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
}
