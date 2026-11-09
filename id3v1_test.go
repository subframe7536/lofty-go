package lofty

// Ports of lofty-rs src/id3/v1/tag.rs unit tests.

import (
	"bytes"
	"strings"
	"testing"
)

func TestId3v1YearRoundtrip(t *testing.T) {
	// via set_date(), which uses ItemKey::RecordingDate
	tag := NewTag(TagTypeID3v1)
	tag.SetDate(Timestamp{
		Year:   1999,
		Month:  Some(uint8(10)),
		Day:    Some(uint8(11)),
		Hour:   Some(uint8(12)),
		Minute: Some(uint8(13)),
		Second: Some(uint8(14)),
	})

	id3v1Tag := Id3v1TagFromTag(tag)
	if y, _ := id3v1Tag.Year.Get(); y != 1999 {
		t.Fatalf("year = %d", y)
	}
	d, ok := id3v1Tag.Date().Get()
	if !ok || d.Year != 1999 || d.Month.IsSome() {
		t.Fatalf("date = %+v", d)
	}

	// via ItemKey::Year
	tag = NewTag(TagTypeID3v1)
	tag.InsertText(ItemKeyYear, "1999")
	id3v1Tag = Id3v1TagFromTag(tag)
	if y, _ := id3v1Tag.Year.Get(); y != 1999 {
		t.Fatalf("year = %d", y)
	}
}

func TestId3v1LossyEncodings(t *testing.T) {
	tag := NewTag(TagTypeID3v1)
	tag.SetArtist("lфfty")

	// Lossy encoding should pass
	wo := NewWriteOptions()
	wo.LossyTextEncoding = true
	id3v1Tag := Id3v1TagFromTag(tag)
	var buf bytes.Buffer
	if err := id3v1Tag.DumpTo(&buf, wo); err != nil {
		t.Fatal(err)
	}
	var raw [128]byte
	copy(raw[:], buf.Bytes())
	parsed, err := Id3v1TagParse(raw, ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := parsed.Artist.Get(); a != "l?fty" {
		t.Fatalf("artist = %q, want %q", a, "l?fty")
	}

	// And should fail when disabled
	wo.LossyTextEncoding = false
	if err := id3v1Tag.DumpTo(&buf, wo); err == nil {
		t.Fatal("expected encoding error with lossy disabled")
	}
}

func TestId3v1ImplicitTruncation(t *testing.T) {
	id3v1Tag := &Id3v1Tag{}
	id3v1Tag.Comment = Some(strings.Repeat("X", 29))

	// By default, we'll just truncate the string
	var buf bytes.Buffer
	if err := id3v1Tag.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	// But it should fail with implicit_truncation disabled
	wo := NewWriteOptions()
	wo.ImplicitTruncation = false
	if err := id3v1Tag.DumpTo(&buf, wo); err == nil {
		t.Fatal("expected error with implicit_truncation disabled")
	}
}
