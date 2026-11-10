package lofty

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Ported from tag/tests.rs::should_preserve_empty_title
func TestTagPreserveEmptyTitle(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetTitle("Foo title")
	if s, ok := tag.Title(); !ok || s != "Foo title" {
		t.Fatalf("title wrong")
	}
	tag.SetTitle("")
	if s, ok := tag.Title(); !ok || s != "" {
		t.Fatal("expected empty title")
	}
	tag.RemoveTitle()
	if _, ok := tag.Title(); ok {
		t.Fatal("expected no title")
	}
}

// Ported from tag/tests.rs::should_not_parse_year_from_less_than_4_digits
func TestTagParseYearMinDigits(t *testing.T) {
	for _, in := range []string{"198", "19", "1"} {
		if tryParseTimestamp(in).IsSome() {
			t.Fatalf("expected no timestamp for %q", in)
		}
	}
}

// setupConsistencyTag builds a tag with every ItemKey variant (ported from
// tag/tests.rs::setup_tag).
func setupConsistencyTag(ty TagType) (*Tag, map[ItemKey]bool) {
	tag := NewTag(ty)
	for k := ItemKey(0); k < ItemKeyUnknown; k++ {
		switch {
		case k.IsTimestamp():
			ts := Timestamp{
				Year: 2026, Month: Some(uint8(8)), Day: Some(uint8(31)),
				Hour: Some(uint8(1)), Minute: Some(uint8(2)), Second: Some(uint8(3)),
			}
			tag.PushUnchecked(NewTagItem(k, ItemValueText(ts.String())))
		case k.IsNumeric() || k.IsFlag():
			tag.PushUnchecked(NewTagItem(k, ItemValueText("1")))
		case k == ItemKeyPopularimeter:
			tag.PushUnchecked(NewTagItem(k, ItemValueText(PopularimeterMusicBee(StarRatingFive, 10).String())))
		case k == ItemKeyGenre:
			tag.PushUnchecked(NewTagItem(k, ItemValueText("Classical")))
		default:
			tag.PushUnchecked(NewTagItem(k, ItemValueText("foo")))
		}
	}
	supported := map[ItemKey]bool{}
	for _, k := range ItemKeySupportedKeys(ty) {
		supported[k] = true
	}
	return tag, supported
}

// verifyRoundTrip ports tag/tests.rs::verify_round_trip: all supported keys
// must survive generic→concrete→generic conversion.
func verifyRoundTrip(
	t *testing.T,
	tagType TagType,
	toConcrete func(*Tag) map[string]bool,
	fromConcrete func() *Tag,
	filterExpected func(map[ItemKey]bool) map[ItemKey]bool,
) {
	t.Helper()
	tag, supported := setupConsistencyTag(tagType)

	actual := toConcrete(tag)
	expected := map[string]bool{}
	for k := range supported {
		if mk, ok := k.MapKey(tagType); ok {
			expected[mk] = true
		}
	}
	for k := range expected {
		if !actual[k] {
			t.Errorf("%s: conversion dropped concrete key %q", tagType, k)
		}
	}

	roundTripped := fromConcrete()
	var expectedItemKeys map[ItemKey]bool
	if filterExpected != nil {
		expectedItemKeys = filterExpected(supported)
	} else {
		expectedItemKeys = supported
	}
	actualItemKeys := map[ItemKey]bool{}
	for _, item := range roundTripped.Items() {
		actualItemKeys[item.Key()] = true
		if !supported[item.Key()] {
			t.Errorf("%s: round-trip produced unsupported key %v", tagType, item.Key())
		}
	}
	for k := range expectedItemKeys {
		if !actualItemKeys[k] {
			t.Errorf("%s: round-trip dropped item key %v", tagType, k)
		}
	}
}

func TestAiffTextSupportedItemsConsistent(t *testing.T) {
	var concrete *AiffTextTag
	verifyRoundTrip(t, TagTypeAiffText,
		func(tag *Tag) map[string]bool {
			concrete = AiffTextTagFromTag(tag)
			keys := map[string]bool{}
			if concrete.Name.IsSome() {
				keys["NAME"] = true
			}
			if concrete.Author.IsSome() {
				keys["AUTH"] = true
			}
			if concrete.Copyright.IsSome() {
				keys["(c) "] = true
			}
			if len(concrete.Annotations) > 0 {
				keys["ANNO"] = true
			}
			return keys
		},
		func() *Tag { return concrete.Tag() },
		nil,
	)
}

func TestApeSupportedItemsConsistent(t *testing.T) {
	var concrete *ApeTag
	verifyRoundTrip(t, TagTypeApe,
		func(tag *Tag) map[string]bool {
			concrete = ApeTagFromTag(tag)
			keys := map[string]bool{}
			for _, item := range concrete.Items() {
				keys[item.Key] = true
			}
			return keys
		},
		func() *Tag { return concrete.Tag() },
		// APE maps both RecordingDate and Year to the same field
		func(keys map[ItemKey]bool) map[ItemKey]bool {
			out := map[ItemKey]bool{}
			for k := range keys {
				if k != ItemKeyYear {
					out[k] = true
				}
			}
			return out
		},
	)
}

func TestId3v1SupportedItemsConsistent(t *testing.T) {
	var concrete *Id3v1Tag
	verifyRoundTrip(t, TagTypeID3v1,
		func(tag *Tag) map[string]bool {
			concrete = Id3v1TagFromTag(tag)
			keys := map[string]bool{}
			if concrete.Title.IsSome() {
				keys["TIT2"] = true
			}
			if concrete.Artist.IsSome() {
				keys["TPE1"] = true
			}
			if concrete.Album.IsSome() {
				keys["TALB"] = true
			}
			if concrete.Year.IsSome() {
				keys["TYER"] = true
			}
			if concrete.Comment.IsSome() {
				keys["COMM"] = true
			}
			if concrete.TrackNumber.IsSome() {
				keys["TRCK"] = true
			}
			if concrete.Genre.IsSome() {
				keys["TCON"] = true
			}
			return keys
		},
		func() *Tag { return concrete.Tag() },
		// ID3v1 maps both RecordingDate and Year to the year field
		func(keys map[ItemKey]bool) map[ItemKey]bool {
			out := map[ItemKey]bool{}
			for k := range keys {
				if k != ItemKeyRecordingDate {
					out[k] = true
				}
			}
			return out
		},
	)
}

func TestRiffInfoSupportedItemsConsistent(t *testing.T) {
	var concrete *RiffInfoTag
	verifyRoundTrip(t, TagTypeRiffInfo,
		func(tag *Tag) map[string]bool {
			concrete = RiffInfoTagFromTag(tag)
			keys := map[string]bool{}
			for _, k := range concrete.Keys() {
				keys[k] = true
			}
			return keys
		},
		func() *Tag { return concrete.Tag() },
		nil,
	)
}

func TestVorbisCommentsSupportedItemsConsistent(t *testing.T) {
	var concrete *VorbisCommentsTag
	verifyRoundTrip(t, TagTypeVorbisComments,
		func(tag *Tag) map[string]bool {
			concrete = VorbisCommentsTagForWrite(tag, "")
			keys := map[string]bool{}
			for _, item := range concrete.Items() {
				if k, ok := item.Key().MapKey(TagTypeVorbisComments); ok {
					keys[k] = true
				}
			}
			return keys
		},
		func() *Tag { return concrete.Tag() },
		// EncoderSoftware lands in the vendor field, not a comment
		func(keys map[ItemKey]bool) map[ItemKey]bool {
			out := map[ItemKey]bool{}
			for k := range keys {
				out[k] = true
			}
			return out
		},
	)
}

// Ported from tag/tests.rs::issue_37 — a picture write to Ogg must not corrupt
// the stream (ffprobe must not report CRC mismatches).
func TestOggIssue37Write(t *testing.T) {
	if !toolInstalled("ffprobe") {
		t.Skip("ffprobe not installed")
	}
	picData, err := os.ReadFile(testAsset("issue_37.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	f := tempFile(t, testAsset("issue_37.ogg"))
	defer f.Close()

	tag := NewTag(TagTypeVorbisComments)
	pic, err := PictureFromReader(bytes.NewReader(picData))
	if err != nil {
		t.Fatalf("picture parse failed: %v", err)
	}
	pic.SetPicType(PictureTypeCoverFront)
	tag.PushPicture(*pic)

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := writeTag(tag, verified, NewWriteOptions()); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	f.Close()

	out, err := exec.Command("ffprobe", f.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "CRC mismatch") || strings.Contains(string(out), "Invalid data") {
		t.Fatalf("ffprobe reported corruption: %s", out)
	}
}

// Ported from tag/tests.rs::id3v2_supported_items_are_consistent — via a
// bogem encode→parse round trip rather than a concrete struct.
func TestId3v2SupportedItemsConsistent(t *testing.T) {
	tag, supported := setupConsistencyTag(TagTypeID3v2)

	roundTripped := id3v2TagToTag(id3v2TagFromTag(tag))

	// Duplicate keys that map to the same frame as another key
	dupes := map[ItemKey]bool{
		ItemKeyWriter:          true, // same as Lyricist
		ItemKeyLabel:           true, // same as Publisher
		ItemKeyEncoderSoftware: true, // same as EncoderSettings
	}
	actual := map[ItemKey]bool{}
	for _, item := range roundTripped.Items() {
		actual[item.Key()] = true
	}
	for k := range supported {
		if dupes[k] {
			continue
		}
		if !actual[k] {
			t.Errorf("id3v2 round-trip dropped %v", k)
		}
	}
	for k := range actual {
		if !supported[k] && k != ItemKeyUnknown {
			t.Errorf("id3v2 round-trip produced unsupported %v", k)
		}
	}
}

// Ported from tag/tests.rs::ilst_supported_items_are_consistent — encode then
// re-parse the ilst data.
func TestIlstSupportedItemsConsistent(t *testing.T) {
	tag, supported := setupConsistencyTag(TagTypeMp4Ilst)

	data, err := EncodeMp4Ilst(tag)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	// The ilst payload is parsed after the atom header
	parsedIlst, err := ParseIlst(bytes.NewReader(data[8:]), int64(len(data)-8), NewParseOptions())
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	parsed := parsedIlst.Tag()
	actual := map[ItemKey]bool{}
	for _, item := range parsed.Items() {
		actual[item.Key()] = true
	}
	// Rust filters UnsyncLyrics — it maps to the same atom as Lyrics
	for k := range supported {
		if k == ItemKeyUnsyncLyrics {
			continue
		}
		if !actual[k] {
			t.Errorf("ilst round-trip dropped %v", k)
		}
	}
	for k := range actual {
		if !supported[k] && k != ItemKeyUnknown {
			t.Errorf("ilst round-trip produced unsupported %v", k)
		}
	}
}

// Ported from tag/item.rs — one-to-many and many-to-one key mappings.
func TestItemKeyMapOneToMany(t *testing.T) {
	if got, ok := ItemKeyPublisher.MapKey(TagTypeID3v2); !ok || got != "TPUB" {
		t.Fatalf("Publisher -> %q", got)
	}
	if got, ok := ItemKeyLabel.MapKey(TagTypeID3v2); !ok || got != "TPUB" {
		t.Fatalf("Label -> %q", got)
	}
}

func TestItemKeyMapManyToOne(t *testing.T) {
	for _, k := range []string{"ALBUMARTIST", "ALBUM ARTIST"} {
		if got, ok := ItemKeyFromKey(TagTypeVorbisComments, k); !ok || got != ItemKeyAlbumArtist {
			t.Fatalf("%q -> %v", k, got)
		}
	}
}

// Ported from tag/items/popularimeter.rs::popm_conversions.
func TestPopmConversions(t *testing.T) {
	expected := PopularimeterMusicBee(StarRatingThree, 5)

	cases := []struct {
		tagType          TagType
		toGeneric        func(*Tag) *Tag
		checkPlayCounter bool
	}{
		{TagTypeID3v2, func(tag *Tag) *Tag {
			return id3v2TagToTag(id3v2TagFromTag(tag))
		}, true},
		{TagTypeMp4Ilst, func(tag *Tag) *Tag {
			data, err := EncodeMp4Ilst(tag)
			if err != nil {
				t.Fatal(err)
			}
			gotIlst, err := ParseIlst(bytes.NewReader(data[8:]), int64(len(data)-8), NewParseOptions())
			if err != nil {
				t.Fatal(err)
			}
			got := gotIlst.Tag()
			return got
		}, false},
		{TagTypeVorbisComments, func(tag *Tag) *Tag {
			return VorbisCommentsTagFromTag(tag, "").Tag()
		}, false},
		{TagTypeRiffInfo, func(tag *Tag) *Tag {
			return RiffInfoTagFromTag(tag).Tag()
		}, false},
	}

	for _, tc := range cases {
		tag := NewTag(tc.tagType)
		tag.InsertText(ItemKeyPopularimeter, expected.String())
		tag = tc.toGeneric(tag)

		ratings := tag.Ratings()
		if len(ratings) != 1 {
			t.Fatalf("%v: ratings = %d, want 1", tc.tagType, len(ratings))
		}
		popm := ratings[0]
		if popm.Email() != expected.Email() {
			t.Errorf("%v: email = %q, want %q", tc.tagType, popm.Email(), expected.Email())
		}
		if popm.Rating != expected.Rating {
			t.Errorf("%v: rating = %d, want %d", tc.tagType, popm.Rating, expected.Rating)
		}
		if tc.checkPlayCounter && popm.PlayCounter != expected.PlayCounter {
			t.Errorf("%v: counter = %d, want %d", tc.tagType, popm.PlayCounter, expected.PlayCounter)
		}
	}
}
