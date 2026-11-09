package lofty

// Ports of lofty-rs src/id3/v1/tag.rs unit tests.

import (
	"bytes"
	"os"
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

func readId3v1Asset(t *testing.T) *Id3v1Tag {
	t.Helper()
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.id3v1")
	if err != nil {
		t.Fatal(err)
	}
	var buf [128]byte
	copy(buf[:], data)
	tag, err := Id3v1TagParse(buf, ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

// Ported from id3/v1/tag.rs::parse_id3v1
func TestId3v1Parse(t *testing.T) {
	tag := readId3v1Asset(t)
	if v, _ := tag.Title.Get(); v != "Foo title" {
		t.Fatalf("title = %q", v)
	}
	if v, _ := tag.Artist.Get(); v != "Bar artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := tag.Album.Get(); v != "Baz album" {
		t.Fatalf("album = %q", v)
	}
	if v, _ := tag.Year.Get(); v != 1984 {
		t.Fatalf("year = %d", v)
	}
	if v, _ := tag.Comment.Get(); v != "Qux comment" {
		t.Fatalf("comment = %q", v)
	}
	if v, _ := tag.TrackNumber.Get(); v != 1 {
		t.Fatalf("track = %d", v)
	}
	if v, _ := tag.Genre.Get(); v != 32 {
		t.Fatalf("genre = %d", v)
	}
}

// Ported from id3/v1/tag.rs::id3v1_re_read
func TestId3v1ReRead(t *testing.T) {
	tag := readId3v1Asset(t)
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	var raw [128]byte
	copy(raw[:], buf.Bytes())
	reparsed, err := Id3v1TagParse(raw, ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if *reparsed != *tag {
		t.Fatalf("re-read mismatch: %+v vs %+v", reparsed, tag)
	}
}

// Ported from id3/v1/tag.rs::id3v1_to_tag
func TestId3v1ToTag(t *testing.T) {
	tag := readId3v1Asset(t).Tag()
	if v, _ := tag.Title(); v != "Foo title" {
		t.Fatalf("title = %q", v)
	}
	if v, _ := tag.Artist(); v != "Bar artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := tag.Album(); v != "Baz album" {
		t.Fatalf("album = %q", v)
	}
	if v, _ := tag.Comment(); v != "Qux comment" {
		t.Fatalf("comment = %q", v)
	}
	if v, _ := tag.Track(); v != 1 {
		t.Fatalf("track = %d", v)
	}
	if v, _ := tag.Genre(); v != "Classical" {
		t.Fatalf("genre = %q", v)
	}
}

// Ported from id3/v1/tag.rs::tag_to_id3v1
func TestTagToId3v1(t *testing.T) {
	tag := NewTag(TagTypeID3v1)
	tag.SetTitle("Foo title")
	tag.SetArtist("Bar artist")
	tag.SetAlbum("Baz album")
	tag.SetComment("Qux comment")
	tag.SetTrack(1)
	tag.SetGenre("Classical")

	id3v1Tag := Id3v1TagFromTag(tag)
	if v, _ := id3v1Tag.Title.Get(); v != "Foo title" {
		t.Fatalf("title = %q", v)
	}
	if v, _ := id3v1Tag.Artist.Get(); v != "Bar artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := id3v1Tag.Album.Get(); v != "Baz album" {
		t.Fatalf("album = %q", v)
	}
	if v, _ := id3v1Tag.Comment.Get(); v != "Qux comment" {
		t.Fatalf("comment = %q", v)
	}
	if v, _ := id3v1Tag.TrackNumber.Get(); v != 1 {
		t.Fatalf("track = %d", v)
	}
	if v, _ := id3v1Tag.Genre.Get(); v != 32 {
		t.Fatalf("genre = %d", v)
	}
}
