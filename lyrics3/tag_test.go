package lyrics3

import (
	"bytes"
	"strings"
	"testing"
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
	if err := parsed.DumpTo(&buf, NewWriteOptions()); err != nil {
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
	if s, ok := tag.GetString(ItemKeyLyrics); !ok || s != lyrics3v1Expected().Lyrics {
		t.Fatalf("got %q", s)
	}
}

func TestTagToLyrics3(t *testing.T) {
	tag := NewTag(TagTypeLyrics3)
	tag.InsertText(ItemKeyLyrics, lyrics3v1Expected().Lyrics)
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
		wo := NewWriteOptions()
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
		Lyrics:             Some("[00:00]Awesome lyrics\r\n[00:10]Written by Lofty"),
		Info:               Some("This track has lots of interesting info.\r\nSo much in fact, that it spans multiple lines!"),
		Author:             Some("Serial-ATA"),
		ExtendedAlbumName:  Some("Lofty Stuff"),
		ExtendedArtistName: Some("Foo & the Bars"),
		ExtendedTrackTitle: Some("BazLand"),
		Images: []Lyrics3v2Image{
			{
				Filename:    "foo.jpg",
				Description: Some("foo is a great picture"),
				Timestamp:   &Timestamp{Minute: Some(uint8(15)), Second: Some(uint8(1))},
			},
			{
				Filename:  "no_description.jpg",
				Timestamp: &Timestamp{Minute: Some(uint8(15)), Second: Some(uint8(1))},
			},
			{Filename: "filename_only.jpg"},
		},
	}
}

func lyrics3v2ExpectedTag() *Tag {
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "[00:00]Awesome lyrics\r\n[00:10]Written by Lofty")
	tag.InsertText(ItemKeyComment, "This track has lots of interesting info.\r\nSo much in fact, that it spans multiple lines!")
	tag.InsertText(ItemKeyLyricist, "Serial-ATA")
	tag.InsertText(ItemKeyAlbumTitle, "Lofty Stuff")
	tag.InsertText(ItemKeyTrackArtist, "Foo & the Bars")
	tag.InsertText(ItemKeyTrackTitle, "BazLand")
	return tag
}

func optStrEq(t *testing.T, name string, got, want Option[string]) {
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
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(lyrics3v2TagBytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	lyrics3v2Eq(t, parsed, lyrics3v2ExpectedConcrete())
}

func TestLyrics3v2ReRead(t *testing.T) {
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(lyrics3v2TagBytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := parsed.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	re, err := ParseLyrics3v2Tag(buf.Bytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	lyrics3v2Eq(t, re, parsed)
}

func TestLyrics3v2ToTag(t *testing.T) {
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
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
	tag := &Lyrics3v2Tag{Lyrics: Some("fooÿbar")}
	wo := NewWriteOptions()
	wo.LossyTextEncoding = false
	if err := tag.DumpTo(&bytes.Buffer{}, wo); err == nil {
		t.Fatal("expected error")
	}
}

func TestLyrics3v2FieldsTruncate(t *testing.T) {
	tag := &Lyrics3v2Tag{
		Lyrics: Some(strings.Repeat("W", lyrics3v2FieldMaxSize+1)),
		Author: Some(strings.Repeat("X", lyrics3v2BasicMaxSize+1)),
		Images: []Lyrics3v2Image{{
			Filename:    strings.Repeat("Y", lyrics3v2PathMax+1),
			Description: Some(strings.Repeat("Z", lyrics3v2BasicMaxSize+1)),
		}},
	}
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(buf.Bytes(), opts)
	if err != nil {
		t.Fatal(err)
	}
	optStrEq(t, "lyrics", parsed.Lyrics, Some(strings.Repeat("W", lyrics3v2FieldMaxSize)))
	optStrEq(t, "author", parsed.Author, Some(strings.Repeat("X", lyrics3v2BasicMaxSize)))
	if len(parsed.Images) != 1 {
		t.Fatalf("images len = %d", len(parsed.Images))
	}
	img := parsed.Images[0]
	if img.Filename != strings.Repeat("Y", lyrics3v2PathMax) {
		t.Fatalf("filename len = %d", len(img.Filename))
	}
	optStrEq(t, "img desc", img.Description, Some(strings.Repeat("Z", lyrics3v2BasicMaxSize)))
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

	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	parsed, err := ParseLyrics3v2Tag(tag, opts)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := parsed.ExtendedAlbumName.Get(); !ok || v != "Lofty Stuff" {
		t.Fatalf("eal = %q", v)
	}
}
