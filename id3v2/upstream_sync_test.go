package id3v2

import (
	"bytes"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Ported from id3/v2/tag/tests.rs::wxxx_frame_encodings (upstream issue #795).
func TestWxxxFrameEncodings(t *testing.T) {
	for _, enc := range []TextEncoding{TextEncodingLatin1, TextEncodingUTF16} {
		tag := NewId3v2Tag()
		tag.Frames.Insert(NewExtendedUrlFrame(enc, "Description", "https://example.com"))

		var buf bytes.Buffer
		if err := tag.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
			t.Fatalf("enc=%v DumpTo: %v", enc, err)
		}

		strict := meta.NewParseOptions()
		strict.ParsingMode = meta.ParsingModeStrict
		r := bytes.NewReader(buf.Bytes())
		header, err := ParseId3v2HeaderRaw(r)
		if err != nil {
			t.Fatalf("enc=%v ParseId3v2HeaderRaw: %v", enc, err)
		}
		parsed, err := ParseId3v2Tag(r, header, strict)
		if err != nil {
			t.Fatalf("enc=%v ParseId3v2Tag: %v", enc, err)
		}

		frames := parsed.Frames.Frames()
		if len(frames) != 1 {
			t.Fatalf("enc=%v frame count = %d, want 1", enc, len(frames))
		}
		wxxx, ok := frames[0].(*ExtendedUrlFrame)
		if !ok {
			t.Fatalf("enc=%v frame type = %T, want *ExtendedUrlFrame", enc, frames[0])
		}
		if wxxx.Encoding != enc || wxxx.Description != "Description" || wxxx.Content != "https://example.com" {
			t.Fatalf("enc=%v got (%v, %q, %q)", enc, wxxx.Encoding, wxxx.Description, wxxx.Content)
		}
	}
}

// Ported from id3/v2/tag/tests.rs::tyer (upstream issue #794).
func TestTyerFullDates(t *testing.T) {
	writeV3 := func(frames ...Frame) []byte {
		tag := NewId3v2Tag()
		for _, fr := range frames {
			tag.Frames.Insert(fr)
		}
		var buf bytes.Buffer
		wo := meta.NewWriteOptions()
		wo.UseID3v23 = true
		if err := tag.DumpTo(&buf, wo); err != nil {
			t.Fatalf("DumpTo v2.3: %v", err)
		}
		return buf.Bytes()
	}

	parseTag := func(raw []byte, mode meta.ParsingMode) (*Id3v2Tag, error) {
		r := bytes.NewReader(raw)
		header, err := ParseId3v2HeaderRaw(r)
		if err != nil {
			return nil, err
		}
		po := meta.NewParseOptions()
		po.ParsingMode = mode
		return ParseId3v2Tag(r, header, po)
	}

	getTdrc := func(tag *Id3v2Tag) meta.Timestamp {
		for _, fr := range tag.Frames.Frames() {
			if tf, ok := fr.(*TimestampFrame); ok && tf.ID().String() == "TDRC" {
				return tf.Timestamp
			}
		}
		t.Fatal("missing TDRC TimestampFrame")
		return meta.Timestamp{}
	}

	cases := []struct {
		date string
		want meta.Timestamp
	}{
		{"2026", meta.Timestamp{Year: 2026}},
		{"2026-11-22", meta.Timestamp{Year: 2026, Month: meta.Some[uint8](11), Day: meta.Some[uint8](22)}},
	}
	for _, tc := range cases {
		raw := writeV3(NewTextInformationFrame(FrameIdValid("TYER"), TextEncodingUTF16, tc.date))
		parsed, err := parseTag(raw, meta.ParsingModeBestAttempt)
		if err != nil {
			t.Fatalf("TYER=%q parse: %v", tc.date, err)
		}
		if got := getTdrc(parsed); got != tc.want {
			t.Fatalf("TYER=%q got %+v, want %+v", tc.date, got, tc.want)
		}
	}

	// TDAT and TIME can still overwrite a full date in TYER.
	raw := writeV3(
		NewTextInformationFrame(FrameIdValid("TYER"), TextEncodingUTF16, "2026-11-22"),
		NewTextInformationFrame(FrameIdValid("TDAT"), TextEncodingUTF16, "2110"),
		NewTextInformationFrame(FrameIdValid("TIME"), TextEncodingUTF16, "0908"),
	)
	parsed, err := parseTag(raw, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatalf("TYER+TDAT+TIME parse: %v", err)
	}
	want := meta.Timestamp{
		Year:   2026,
		Month:  meta.Some[uint8](10),
		Day:    meta.Some[uint8](21),
		Hour:   meta.Some[uint8](9),
		Minute: meta.Some[uint8](8),
	}
	if got := getTdrc(parsed); got != want {
		t.Fatalf("TYER+TDAT+TIME got %+v, want %+v", got, want)
	}

	// Strict mode rejects a full date in TYER.
	rawStrict := writeV3(NewTextInformationFrame(FrameIdValid("TYER"), TextEncodingUTF16, "2026-11-22"))
	if _, err := parseTag(rawStrict, meta.ParsingModeStrict); err == nil {
		t.Fatal("expected strict mode error for full date in TYER")
	}
}
