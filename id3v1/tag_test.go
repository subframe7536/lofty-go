package id3v1

import (
	"errors"
	"strings"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

const yearOffset = 3 + 30*3

func encodedYear(t *testing.T, year uint16) string {
	t.Helper()
	tag := &Id3v1Tag{Year: meta.Some(year)}
	data, err := id3v1Encode(tag, meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 128 {
		t.Fatalf("encoded %d bytes", len(data))
	}
	return string(data[yearOffset : yearOffset+4])
}

// Rust `write.rs`: the year is zero-padded to four digits and clamped to 9999.
func TestEncodeYearPaddedAndClamped(t *testing.T) {
	for year, want := range map[uint16]string{
		0:     "0000",
		5:     "0005",
		1999:  "1999",
		9999:  "9999",
		10000: "9999",
		65535: "9999",
	} {
		if got := encodedYear(t, year); got != want {
			t.Errorf("year %d encoded as %q, want %q", year, got, want)
		}
	}

	data, err := id3v1Encode(&Id3v1Tag{Title: meta.Some("x")}, meta.NewWriteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data[yearOffset : yearOffset+4]); got != "\x00\x00\x00\x00" {
		t.Fatalf("absent year encoded as %q", got)
	}
}

// Rust encodes lazily and takes `size` characters, so what lies past the cut
// is never checked; too-long values are a TooMuchDataError otherwise.
func TestEncodeFieldTruncation(t *testing.T) {
	long := strings.Repeat("a", 30) + "€"
	tag := &Id3v1Tag{Title: meta.Some(long)}

	wo := meta.NewWriteOptions()
	wo.LossyTextEncoding = false
	wo.ImplicitTruncation = true
	data, err := id3v1Encode(tag, wo)
	if err != nil {
		t.Fatalf("unencodable text past the cut must be ignored: %v", err)
	}
	if got := string(data[3:33]); got != strings.Repeat("a", 30) {
		t.Fatalf("title = %q", got)
	}

	wo.ImplicitTruncation = false
	_, err = id3v1Encode(tag, wo)
	var tmd *meta.TooMuchDataError
	if !errors.As(err, &tmd) {
		t.Fatalf("got %v, want TooMuchDataError", err)
	}
	var ee *Id3v1EncodingError
	if !errors.As(err, &ee) {
		t.Fatalf("got %T, want *Id3v1EncodingError", err)
	}

	// An unencodable character inside the field is still an error
	wo.ImplicitTruncation = true
	if _, err := id3v1Encode(&Id3v1Tag{Title: meta.Some("a€")}, wo); err == nil {
		t.Fatal("expected an encoding error")
	}

	// Multi-byte characters are counted as characters, not bytes
	wo.ImplicitTruncation = false
	if _, err := id3v1Encode(&Id3v1Tag{Title: meta.Some(strings.Repeat("é", 30))}, wo); err != nil {
		t.Fatalf("30 characters fit a 30 character field: %v", err)
	}
}

func fromTagWith(key meta.ItemKey, value string) *Id3v1Tag {
	tag := meta.NewTag(meta.TagTypeID3v1)
	tag.InsertText(key, value)
	return Id3v1TagFromTag(tag)
}

// Rust parses with `parse::<u16>()` / `parse::<u8>()`: out-of-range and signed
// values are rejected instead of wrapping.
func TestFromTagNumericBounds(t *testing.T) {
	for value, want := range map[string]meta.Option[uint16]{
		"1999":  meta.Some(uint16(1999)),
		"65535": meta.Some(uint16(65535)),
		"+1999": meta.Some(uint16(1999)),
		"-1":    meta.None[uint16](),
		"nope":  meta.None[uint16](),
	} {
		if got := fromTagWith(meta.ItemKeyYear, value).Year; got != want {
			t.Errorf("year %q = %v, want %v", value, got, want)
		}
	}
	// Out-of-range years must not wrap to a small number (70000 -> 4464);
	// like Rust they fall through to the tag's date, which reads four digits.
	if got := fromTagWith(meta.ItemKeyYear, "70000").Year; got == meta.Some(uint16(4464)) {
		t.Errorf("year 70000 wrapped to %v", got)
	}
	for in, want := range map[string]bool{"65535": true, "65536": false, "-1": false, "": false, "+": false, "+7": true, " 7": false} {
		if _, ok := parseUint(in, 16); ok != want {
			t.Errorf("parseUint(%q, 16) ok = %v, want %v", in, ok, want)
		}
	}

	for value, want := range map[string]meta.Option[uint8]{
		"7":   meta.Some(uint8(7)),
		"255": meta.Some(uint8(255)),
		"256": meta.None[uint8](),
		"300": meta.None[uint8](),
		"-1":  meta.None[uint8](),
		"1/2": meta.None[uint8](),
	} {
		if got := fromTagWith(meta.ItemKeyTrackNumber, value).TrackNumber; got != want {
			t.Errorf("track %q = %v, want %v", value, got, want)
		}
	}
}

func TestFromTagGenreFallsBackToNumber(t *testing.T) {
	for value, want := range map[string]meta.Option[uint8]{
		"Rock":       meta.Some(uint8(17)),
		"Blues":      meta.Some(uint8(0)),
		"5":          meta.Some(uint8(5)),
		"200":        meta.Some(uint8(200)),
		"256":        meta.None[uint8](),
		"Not a real": meta.None[uint8](),
		// Rust matches names exactly
		"rock": meta.None[uint8](),
	} {
		if got := fromTagWith(meta.ItemKeyGenre, value).Genre; got != want {
			t.Errorf("genre %q = %v, want %v", value, got, want)
		}
	}
}

func TestFromTagYearFallsBackToDate(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v1)
	tag.InsertText(meta.ItemKeyYear, "70000")
	tag.InsertText(meta.ItemKeyRecordingDate, "2001-02-03")
	if got := Id3v1TagFromTag(tag).Year; got != meta.Some(uint16(2001)) {
		t.Fatalf("year = %v", got)
	}
}
