package lyrics3

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// Ported from id3/lyrics3/v1/tests.rs

func lyrics3v1TagBytes() []byte {
	var tag []byte
	tag = append(tag, []byte("LYRICSBEGIN")...)
	for _, line := range []string{"[00:00]Awesome lyrics\r\n", "[00:10]Written by Lofty"} {
		tag = append(tag, line...)
	}
	tag = append(tag, lyrics3v1Footer...)
	return tag
}

func lyrics3v1Expected() *Lyrics3Tag {
	return &Lyrics3Tag{Lyrics: "[00:00]Awesome lyrics\r\n[00:10]Written by Lofty"}
}

func TestParseLyrics3(t *testing.T) {
	parsed, err := ParseLyrics3Tag(lyrics3v1TagBytes())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Lyrics != lyrics3v1Expected().Lyrics {
		t.Fatalf("got %q", parsed.Lyrics)
	}
}

func TestLyrics3ReRead(t *testing.T) {
	parsed, err := ParseLyrics3Tag(lyrics3v1TagBytes())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := parsed.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	re, err := ParseLyrics3Tag(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if re.Lyrics != parsed.Lyrics {
		t.Fatalf("got %q", re.Lyrics)
	}
}

func TestLyrics3ToTag(t *testing.T) {
	parsed, err := ParseLyrics3Tag(lyrics3v1TagBytes())
	if err != nil {
		t.Fatal(err)
	}
	tag := parsed.Tag()
	if s, ok := tag.GetString(meta.ItemKeyLyrics); !ok || s != lyrics3v1Expected().Lyrics {
		t.Fatalf("got %q", s)
	}
}

func TestTagToLyrics3(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeLyrics3)
	tag.InsertText(meta.ItemKeyLyrics, lyrics3v1Expected().Lyrics)
	l3 := Lyrics3TagFromTag(tag)
	if l3.Lyrics != lyrics3v1Expected().Lyrics {
		t.Fatalf("got %q", l3.Lyrics)
	}
}

func TestForbiddenLyrics3Content(t *testing.T) {
	for _, content := range []string{
		"blahblahblahLYRICSBEGINblahblahblah",
		"blahblahblahLYRICSENDblahblahblah",
		"fooÿbar",
	} {
		tag := &Lyrics3Tag{Lyrics: content}
		wo := meta.NewWriteOptions()
		wo.LossyTextEncoding = false
		if err := tag.DumpTo(&bytes.Buffer{}, wo); err == nil {
			t.Fatalf("expected error for %q", content)
		}
	}
}

// Ported from id3/lyrics3/v2/tests.rs

func lyrics3v2TagBytes() []byte {
	var tag []byte
	tag = append(tag, lyrics3v2Header...)
	tag = append(tag, []byte("IND00003")...)
	tag = append(tag, '1', '1', '0')
	tag = append(tag, []byte("EAL00011")...)
	tag = append(tag, []byte("Lofty Stuff")...)
	tag = append(tag, []byte("EAR00014")...)
	tag = append(tag, []byte("Foo & the Bars")...)
	tag = append(tag, []byte("ETT00007")...)
	tag = append(tag, []byte("BazLand")...)
	tag = append(tag, []byte("INF00088")...)
	tag = append(tag, []byte("This track has lots of interesting info.\r\nSo much in fact, that it spans multiple lines!")...)
	tag = append(tag, []byte("AUT00010")...)
	tag = append(tag, []byte("Serial-ATA")...)
	tag = append(tag, []byte("LYR00046")...)
	tag = append(tag, []byte("[00:00]Awesome lyrics\r\n[00:10]Written by Lofty")...)
	tag = append(tag, []byte("IMG00094")...)
	tag = append(tag, []byte("foo.jpg||foo is a great picture||[15:01]\r\n")...)
	tag = append(tag, []byte("no_description.jpg||||[15:01]\r\n")...)
	tag = append(tag, []byte("filename_only.jpg||||")...)
	tag = append(tag, []byte("000348")...)
	tag = append(tag, lyrics3v2Footer...)
	return tag
}

func lyrics3v2ExpectedConcrete() *Lyrics3v2Tag {
	return &Lyrics3v2Tag{
		Flags:              Lyrics3v2TagFlags{SynchronizedLyrics: true, RandomSelection: false},
		Lyrics:             meta.Some("[00:00]Awesome lyrics\r\n[00:10]Written by Lofty"),
		Info:               meta.Some("This track has lots of interesting info.\r\nSo much in fact, that it spans multiple lines!"),
		Author:             meta.Some("Serial-ATA"),
		ExtendedAlbumName:  meta.Some("Lofty Stuff"),
		ExtendedArtistName: meta.Some("Foo & the Bars"),
		ExtendedTrackTitle: meta.Some("BazLand"),
		Images: []Lyrics3v2Image{
			{
				Filename:    "foo.jpg",
				Description: meta.Some("foo is a great picture"),
				Timestamp:   &meta.Timestamp{Minute: meta.Some(uint8(15)), Second: meta.Some(uint8(1))},
			},
			{
				Filename:  "no_description.jpg",
				Timestamp: &meta.Timestamp{Minute: meta.Some(uint8(15)), Second: meta.Some(uint8(1))},
			},
			{Filename: "filename_only.jpg"},
		},
	}
}

func lyrics3v2ExpectedTag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeLyrics3v2)
	tag.InsertText(meta.ItemKeyLyrics, "[00:00]Awesome lyrics\r\n[00:10]Written by Lofty")
	tag.InsertText(meta.ItemKeyComment, "This track has lots of interesting info.\r\nSo much in fact, that it spans multiple lines!")
	tag.InsertText(meta.ItemKeyLyricist, "Serial-ATA")
	tag.InsertText(meta.ItemKeyAlbumTitle, "Lofty Stuff")
	tag.InsertText(meta.ItemKeyTrackArtist, "Foo & the Bars")
	tag.InsertText(meta.ItemKeyTrackTitle, "BazLand")
	return tag
}

func optStrEq(t *testing.T, name string, got, want meta.Option[string]) {
	t.Helper()
	gv, gok := got.Get()
	wv, wok := want.Get()
	if gok != wok || (gok && gv != wv) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func lyrics3v2Eq(t *testing.T, got, want *Lyrics3v2Tag) {
	t.Helper()
	if got.Flags != want.Flags {
		t.Fatalf("flags = %+v, want %+v", got.Flags, want.Flags)
	}
	optStrEq(t, "lyrics", got.Lyrics, want.Lyrics)
	optStrEq(t, "info", got.Info, want.Info)
	optStrEq(t, "author", got.Author, want.Author)
	optStrEq(t, "eal", got.ExtendedAlbumName, want.ExtendedAlbumName)
	optStrEq(t, "ear", got.ExtendedArtistName, want.ExtendedArtistName)
	optStrEq(t, "ett", got.ExtendedTrackTitle, want.ExtendedTrackTitle)
	if len(got.Images) != len(want.Images) {
		t.Fatalf("images len = %d, want %d", len(got.Images), len(want.Images))
	}
	for i := range got.Images {
		gi, wi := got.Images[i], want.Images[i]
		if gi.Filename != wi.Filename {
			t.Fatalf("image %d filename = %q, want %q", i, gi.Filename, wi.Filename)
		}
		optStrEq(t, "image desc", gi.Description, wi.Description)
		if (gi.Timestamp == nil) != (wi.Timestamp == nil) {
			t.Fatalf("image %d timestamp = %v, want %v", i, gi.Timestamp, wi.Timestamp)
		}
		if gi.Timestamp != nil && *gi.Timestamp != *wi.Timestamp {
			t.Fatalf("image %d timestamp = %+v, want %+v", i, *gi.Timestamp, *wi.Timestamp)
		}
	}
}

func TestParseLyrics3v2(t *testing.T) {
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(lyrics3v2TagBytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	lyrics3v2Eq(t, parsed, lyrics3v2ExpectedConcrete())
}

func TestLyrics3v2ReRead(t *testing.T) {
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(lyrics3v2TagBytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := parsed.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	re, err := ParseLyrics3v2Tag(buf.Bytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	lyrics3v2Eq(t, re, parsed)
}

func TestLyrics3v2ToTag(t *testing.T) {
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(lyrics3v2TagBytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	tag := parsed.Tag()
	want := lyrics3v2ExpectedTag()
	if tag.ItemCount() != want.ItemCount() {
		t.Fatalf("len = %d, want %d", tag.ItemCount(), want.ItemCount())
	}
	for _, item := range want.Items() {
		v, _ := item.Value().Text()
		if s, ok := tag.GetString(item.Key()); !ok || s != v {
			t.Fatalf("missing/mismatched item %v", item.Key())
		}
	}
}

func TestTagToLyrics3v2(t *testing.T) {
	l3 := Lyrics3v2TagFromTag(lyrics3v2ExpectedTag())
	want := lyrics3v2ExpectedConcrete()
	want.Images = nil
	want.Flags = Lyrics3v2TagFlags{}
	lyrics3v2Eq(t, l3, want)
}

func TestLyrics3v2SpecialItemsRoundtrip(t *testing.T) {
	want := lyrics3v2ExpectedConcrete()
	tag := want.Tag()
	roundtripped := Lyrics3v2TagFromTag(tag)
	lyrics3v2Eq(t, roundtripped, want)
}

func TestForbiddenLyrics3v2Content(t *testing.T) {
	tag := &Lyrics3v2Tag{Lyrics: meta.Some("fooÿbar")}
	wo := meta.NewWriteOptions()
	wo.LossyTextEncoding = false
	if err := tag.DumpTo(&bytes.Buffer{}, wo); err == nil {
		t.Fatal("expected error")
	}
}

func TestLyrics3v2FieldsTruncate(t *testing.T) {
	tag := &Lyrics3v2Tag{
		Lyrics: meta.Some(strings.Repeat("W", lyrics3v2FieldMaxSize+1)),
		Author: meta.Some(strings.Repeat("X", lyrics3v2BasicMaxSize+1)),
		Images: []Lyrics3v2Image{{
			Filename:    strings.Repeat("Y", lyrics3v2PathMax+1),
			Description: meta.Some(strings.Repeat("Z", lyrics3v2BasicMaxSize+1)),
		}},
	}
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(buf.Bytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	optStrEq(t, "lyrics", parsed.Lyrics, meta.Some(strings.Repeat("W", lyrics3v2FieldMaxSize)))
	optStrEq(t, "author", parsed.Author, meta.Some(strings.Repeat("X", lyrics3v2BasicMaxSize)))
	if len(parsed.Images) != 1 {
		t.Fatalf("images len = %d", len(parsed.Images))
	}
	img := parsed.Images[0]
	if img.Filename != strings.Repeat("Y", lyrics3v2PathMax) {
		t.Fatalf("filename len = %d", len(img.Filename))
	}
	optStrEq(t, "img desc", img.Description, meta.Some(strings.Repeat("Z", lyrics3v2BasicMaxSize)))
}

func TestIgnoreUnknownLyrics3v2Fields(t *testing.T) {
	var tag []byte
	tag = append(tag, lyrics3v2Header...)
	tag = append(tag, []byte("IND00003")...)
	tag = append(tag, '0', '0', '0')
	tag = append(tag, []byte("EAL00011")...)
	tag = append(tag, []byte("Lofty Stuff")...)
	tag = append(tag, []byte("HUH00020")...)
	tag = append(tag, []byte("Some unknown content")...)
	tag = append(tag, []byte("000069")...)
	tag = append(tag, lyrics3v2Footer...)

	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(tag, opts)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := parsed.ExtendedAlbumName.Get(); !ok || v != "Lofty Stuff" {
		t.Fatalf("eal = %q", v)
	}
}

func lyrics3v2WithImages(images string) []byte {
	body := append([]byte(nil), lyrics3v2Header...)
	body = append(body, "IND00003000"...)
	body = append(body, fmt.Sprintf("IMG%05d%s", len(images), images)...)
	out := append(body, fmt.Sprintf("%06d", len(body))...)
	return append(out, lyrics3v2Footer...)
}

// Rust splits IMG entries with `str::lines`, which keeps blank lines.
func TestLyrics3v2ImgKeepsBlankLines(t *testing.T) {
	data := lyrics3v2WithImages("a||b||[00:01]\r\n\r\nc||d||[00:02]\r\n")

	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeRelaxed
	parsed, err := ParseLyrics3v2Tag(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Images) != 3 {
		t.Fatalf("got %d images, want 3: %+v", len(parsed.Images), parsed.Images)
	}
	if parsed.Images[0].Filename != "a" || parsed.Images[1].Filename != "" || parsed.Images[2].Filename != "c" {
		t.Fatalf("images = %+v", parsed.Images)
	}

	// The blank entry has no description, which strict mode rejects
	opts.ParsingMode = meta.ParsingModeStrict
	if _, err := ParseLyrics3v2Tag(data, opts); err == nil {
		t.Fatal("strict mode accepted a blank image entry")
	}
}

func TestRustLines(t *testing.T) {
	for in, want := range map[string][]string{
		"":          nil,
		"a":         {"a"},
		"a\n":       {"a"},
		"a\r\nb":    {"a", "b"},
		"a\n\nb\n":  {"a", "", "b"},
		"a\rb\r\n":  {"a\rb"},
		"\n":        {""},
		"a\r":       {"a\r"},
		"\r\n\r\nx": {"", "", "x"},
	} {
		got := rustLines(in)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("rustLines(%q) = %q, want %q", in, got, want)
		}
	}
}

// Rust's `is_empty` ignores the IND flags.
func TestLyrics3v2IsEmptyIgnoresFlags(t *testing.T) {
	tag := &Lyrics3v2Tag{Flags: Lyrics3v2TagFlags{SynchronizedLyrics: true, RandomSelection: true}}
	if !tag.IsEmpty() {
		t.Fatal("flags must not make the tag non-empty")
	}
	tag.Images = []Lyrics3v2Image{{Filename: "x"}}
	if tag.IsEmpty() {
		t.Fatal("an image makes the tag non-empty")
	}
}

// Characters past the cut are never encoded (Rust encodes lazily, `take(max)`).
func TestLatin1FieldLazyTruncation(t *testing.T) {
	wo := meta.NewWriteOptions()
	wo.LossyTextEncoding = false
	wo.ImplicitTruncation = true

	enc, err := writeLatin1Field("abc€", 3, wo, errors.New("bad"))
	if err != nil || string(enc) != "abc" {
		t.Fatalf("got %q, %v", enc, err)
	}

	// Within the cut it is a real encoding error, reported like Rust's `b?`
	_, err = writeLatin1Field("a€c", 3, wo, errors.New("bad"))
	var tee *util.TextEncodingError
	if !errors.As(err, &tee) {
		t.Fatalf("got %v, want a TextEncodingError", err)
	}

	// Too long without implicit truncation is TooMuchData
	wo.ImplicitTruncation = false
	_, err = writeLatin1Field("abcd", 3, wo, errors.New("bad"))
	var tmd *meta.TooMuchDataError
	if !errors.As(err, &tmd) {
		t.Fatalf("got %v, want TooMuchDataError", err)
	}

	// 0xFF fails with the caller's error, ordered with the other characters
	bad := errors.New("bad")
	if _, err = writeLatin1Field("a\u00ffb", 5, wo, bad); err != bad {
		t.Fatalf("got %v, want the caller's error", err)
	}
}

func TestFindLyrics3ReadIgnoresReadTags(t *testing.T) {
	data := lyrics3v1TagBytes()
	opts := meta.NewParseOptions()
	opts.ReadTags = false

	r := bytes.NewReader(data)
	if _, err := r.Seek(0, 2); err != nil {
		t.Fatal(err)
	}
	_, _, got, found, err := FindLyrics3(r, true, opts)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if l, ok := got.(*Lyrics3Tag); !ok || l.Lyrics != lyrics3v1Expected().Lyrics {
		t.Fatalf("tag = %#v", got)
	}

	// `read` = false never parses
	if _, err := r.Seek(0, 2); err != nil {
		t.Fatal(err)
	}
	opts.ReadTags = true
	if _, _, got, _, _ = FindLyrics3(r, false, opts); got != nil {
		t.Fatalf("parsed a tag with read=false: %#v", got)
	}
}

func TestFindLyrics3NegativeSeekIsAnError(t *testing.T) {
	r := bytes.NewReader([]byte("12345"))
	if _, err := r.Seek(0, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, _, found, err := FindLyrics3(r, false, meta.NewParseOptions()); err == nil || found {
		t.Fatalf("found=%v err=%v, want an error", found, err)
	}
}

// A footer without a header is corrupt; writing must surface that instead of
// silently inserting a second tag.
func TestLyrics3WritePropagatesFindErrors(t *testing.T) {
	file := append(bytes.Repeat([]byte{'x'}, 64), "LYRICSEND"...)
	id3v1 := make([]byte, 128)
	copy(id3v1, "TAG")
	file = append(file, id3v1...)

	wo := meta.NewWriteOptions()
	v1 := &Lyrics3Tag{Lyrics: "hello"}
	if err := Lyrics3v1WriteTo(meta.NewBytesFile(append([]byte(nil), file...)), v1, wo); err == nil {
		t.Fatal("v1 write ignored a corrupt Lyrics3 footer")
	}

	file = append(bytes.Repeat([]byte{'x'}, 64), "LYRICS200"...)
	file = append(file, id3v1...)
	v2 := &Lyrics3v2Tag{Lyrics: meta.Some("hello")}
	if err := Lyrics3v2WriteTo(meta.NewBytesFile(file), v2, wo); err == nil {
		t.Fatal("v2 write ignored a corrupt Lyrics3 footer")
	}
}
