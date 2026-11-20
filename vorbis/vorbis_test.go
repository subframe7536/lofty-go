package vorbis_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

// --- src/ogg/tag.rs ---

var tagAssetsDir = filepath.Join("..", "lofty-rs", "lofty", "tests", "tags", "assets")

func readVCTest(t *testing.T, name string, po ParseOptions) *VorbisCommentsTag {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tagAssetsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	tag, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func vcItemMap(t *VorbisCommentsTag) map[string][]string {
	out := map[string][]string{}
	for _, item := range t.Items() {
		key := item.Description
		if item.Key() != ItemKeyUnknown {
			if mk, ok := item.Key().MapKey(TagTypeVorbisComments); ok {
				key = mk
			}
		}
		if s, ok := item.Value().IntoString(); ok {
			out[key] = append(out[key], s)
		}
	}
	return out
}

func TestVorbisCommentsParse(t *testing.T) {
	tag := readVCTest(t, "test.vorbis", NewParseOptions())
	if tag.Vendor != "Lavf58.76.100" {
		t.Fatalf("vendor = %q", tag.Vendor)
	}
	want := map[string]string{
		"ALBUM":       "Baz album",
		"ARTIST":      "Bar artist",
		"COMMENT":     "Qux comment",
		"DATE":        "1984",
		"GENRE":       "Classical",
		"TITLE":       "Foo title",
		"TRACKNUMBER": "1",
	}
	for k, v := range want {
		if got, ok := tag.Get(k); !ok || got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestVorbisCommentsReRead(t *testing.T) {
	tag := readVCTest(t, "test.vorbis", NewParseOptions())
	// Create a zero-size vendor for comparison
	tag.Vendor = ""
	data, err := EncodeVorbisComments(tag)
	if err != nil {
		t.Fatal(err)
	}
	re, err := ParseVorbisComments(data, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if re.Vendor != tag.Vendor {
		t.Fatalf("vendor = %q, want %q", re.Vendor, tag.Vendor)
	}
	if len(re.Items()) != len(tag.Items()) {
		t.Fatalf("items = %d, want %d", len(re.Items()), len(tag.Items()))
	}
	for k, vals := range vcItemMap(tag) {
		got := vcItemMap(re)[k]
		if len(got) != len(vals) {
			t.Fatalf("%s: %v, want %v", k, got, vals)
		}
		for i, v := range vals {
			if got[i] != v {
				t.Fatalf("%s[%d] = %q, want %q", k, i, got[i], v)
			}
		}
	}
}

func vcCreateTag(ty TagType) *Tag {
	tag := NewTag(ty)
	tag.InsertText(ItemKeyTrackTitle, "Foo title")
	tag.InsertText(ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(ItemKeyComment, "Qux comment")
	tag.InsertText(ItemKeyTrackNumber, "1")
	tag.InsertText(ItemKeyGenre, "Classical")
	return tag
}

func vcVerifyTag(t *testing.T, tag *Tag, trackNumber, genre bool) {
	t.Helper()
	for k, v := range map[ItemKey]string{
		ItemKeyTrackTitle:  "Foo title",
		ItemKeyTrackArtist: "Bar artist",
		ItemKeyAlbumTitle:  "Baz album",
		ItemKeyComment:     "Qux comment",
	} {
		if got, ok := tag.GetString(k); !ok || got != v {
			t.Errorf("%v = %q, want %q", k, got, v)
		}
	}
	if trackNumber {
		if got, ok := tag.GetString(ItemKeyTrackNumber); !ok || got != "1" {
			t.Errorf("track = %q", got)
		}
	}
	if genre {
		if got, ok := tag.GetString(ItemKeyGenre); !ok || got != "Classical" {
			t.Errorf("genre = %q", got)
		}
	}
}

func TestVorbisCommentsToTag(t *testing.T) {
	tag := readVCTest(t, "test.vorbis", NewParseOptions())
	generic := tag.Tag()
	vcVerifyTag(t, generic, true, true)
}

func TestTagToVorbisComments(t *testing.T) {
	tag := vcCreateTag(TagTypeVorbisComments)
	vc := VorbisCommentsTagFromTag(tag, "")
	for k, v := range map[string]string{
		"TITLE":       "Foo title",
		"ARTIST":      "Bar artist",
		"ALBUM":       "Baz album",
		"COMMENT":     "Qux comment",
		"TRACKNUMBER": "1",
		"GENRE":       "Classical",
	} {
		if got, ok := vc.Get(k); !ok || got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestVorbisCommentsMultiValueRoundTrip(t *testing.T) {
	tag := NewTag(TagTypeVorbisComments)
	push2 := func(k ItemKey, v1, v2 string) {
		tag.InsertText(k, v1)
		tag.PushUnchecked(NewTagItem(k, ItemValueText(v2)))
	}
	push2(ItemKeyTrackArtist, "TrackArtist 1", "TrackArtist 2")
	push2(ItemKeyAlbumArtist, "AlbumArtist 1", "AlbumArtist 2")
	push2(ItemKeyTrackTitle, "TrackTitle 1", "TrackTitle 2")
	push2(ItemKeyAlbumTitle, "AlbumTitle 1", "AlbumTitle 2")
	push2(ItemKeyComment, "Comment 1", "Comment 2")
	push2(ItemKeyContentGroup, "ContentGroup 1", "ContentGroup 2")
	push2(ItemKeyGenre, "Genre 1", "Genre 2")
	push2(ItemKeyMood, "Mood 1", "Mood 2")
	push2(ItemKeyComposer, "Composer 1", "Composer 2")
	push2(ItemKeyConductor, "Conductor 1", "Conductor 2")
	// Otherwise the following item would be inserted implicitly
	// during the conversion.
	tag.InsertText(ItemKeyEncoderSoftware, "EncoderSoftware")
	if tag.Len() != 20+1 {
		t.Fatalf("len = %d", tag.Len())
	}

	vc1 := VorbisCommentsTagFromTag(tag, "")
	// split: concrete -> generic
	splitTag := vc1.Tag()
	if splitTag.Len() != tag.Len() {
		t.Fatalf("split len = %d, want %d", splitTag.Len(), tag.Len())
	}
	// merge back
	vc2 := VorbisCommentsTagFromTag(splitTag, vc1.Vendor)

	m1, m2 := vcItemMap(vc1), vcItemMap(vc2)
	if len(m1) != len(m2) {
		t.Fatalf("keys %v vs %v", m1, m2)
	}
	for k, v1 := range m1 {
		v2 := m2[k]
		if len(v1) != len(v2) {
			t.Fatalf("%s: %v vs %v", k, v1, v2)
		}
		for i := range v1 {
			if v1[i] != v2[i] {
				t.Fatalf("%s[%d] = %q, want %q", k, i, v2[i], v1[i])
			}
		}
	}
}

func TestVorbisCommentsZeroSized(t *testing.T) {
	readVCTest(t, "zero.vorbis", NewParseOptions())
}

func TestVorbisCommentsIssue60(t *testing.T) {
	tag := readVCTest(t, "issue_60.vorbis", NewParseOptions())
	if len(tag.Pictures()) != 1 {
		t.Fatalf("pictures = %d", len(tag.Pictures()))
	}
	if len(tag.Items()) != 0 {
		t.Fatalf("items = %d", len(tag.Items()))
	}
}

func TestVorbisCommentsInitialKeyRoundTrip(t *testing.T) {
	// Both the primary and alternate key should map back to the primary key.
	for _, key := range []string{"INITIALKEY", "KEY"} {
		vc := NewVorbisCommentsTag()
		vc.PushKV(key, "Cmaj")
		tag := vc.Tag()
		if got, ok := tag.GetString(ItemKeyInitialKey); !ok || got != "Cmaj" {
			t.Fatalf("%s: initialkey = %q", key, got)
		}
		tag.InsertText(ItemKeyInitialKey, "Cmin")
		vc2 := VorbisCommentsTagFromTag(tag, vc.Vendor)
		if got, ok := vc2.Get("INITIALKEY"); !ok || got != "Cmin" {
			t.Fatalf("%s: INITIALKEY = %q", key, got)
		}
	}
}

func TestVorbisCommentsSkipCoverArt(t *testing.T) {
	pic := PictureUnchecked(bytes.Repeat([]byte{0}, 50)).
		PicType(PictureTypeCoverFront).
		MimeType(MimeTypeJpeg).
		Build()
	tag := NewTag(TagTypeVorbisComments)
	tag.PushPicture(pic)
	tag.SetArtist("Foo artist")

	data, err := EncodeVorbisComments(VorbisCommentsTagFromTag(tag, ""))
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	po.ReadCoverArt = false
	got, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pictures()) != 0 {
		t.Fatalf("pictures = %d", len(got.Pictures()))
	}
	if _, ok := got.Artist(); !ok {
		t.Fatal("artist missing")
	}
}

func TestVorbisCommentsIssue540ImplicitOff(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("TRACKNUMBER", "01/05")
	data, err := EncodeVorbisComments(vc)
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	po.ImplicitConversions = false
	po.ReadCoverArt = false
	tag, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tag.Track(); ok {
		t.Fatal("track should be None")
	}
	if _, ok := tag.TrackTotal(); ok {
		t.Fatal("track_total should be None")
	}
	if got, ok := tag.Get("TRACKNUMBER"); !ok || got != "01/05" {
		t.Fatalf("TRACKNUMBER = %q", got)
	}
}

func TestVorbisCommentsIssue540Leading0(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("TRACKNUMBER", "01")
	vc.InsertKV("TRACKTOTAL", "05")
	data, err := EncodeVorbisComments(vc)
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	po.ImplicitConversions = false
	po.ReadCoverArt = false
	tag, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := tag.Get("TRACKNUMBER"); !ok || got != "01" {
		t.Fatalf("TRACKNUMBER = %q", got)
	}
	if got, ok := tag.Get("TRACKTOTAL"); !ok || got != "05" {
		t.Fatalf("TRACKTOTAL = %q", got)
	}
}

func TestVorbisCommentsIssue543(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("DISCNUMBER", "01/05")
	data, err := EncodeVorbisComments(vc)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := ParseVorbisComments(data, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := tag.Disk(); !ok || n != 1 {
		t.Fatalf("disk = %v", n)
	}
	if n, ok := tag.DiskTotal(); !ok || n != 5 {
		t.Fatalf("disk_total = %v", n)
	}
}

func TestVorbisCommentsDiscImplicitOff(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("DISCNUMBER", "01/05")
	data, err := EncodeVorbisComments(vc)
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	po.ImplicitConversions = false
	po.ReadCoverArt = false
	tag, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tag.Disk(); ok {
		t.Fatal("disk should be None")
	}
	if _, ok := tag.DiskTotal(); ok {
		t.Fatal("disk_total should be None")
	}
	if got, ok := tag.Get("DISCNUMBER"); !ok || got != "01/05" {
		t.Fatalf("DISCNUMBER = %q", got)
	}
}

func TestVorbisCommentsDiscLeading0(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("DISCNUMBER", "01")
	vc.InsertKV("DISCTOTAL", "05")
	data, err := EncodeVorbisComments(vc)
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	po.ImplicitConversions = false
	po.ReadCoverArt = false
	tag, err := ParseVorbisComments(data, po)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := tag.Get("DISCNUMBER"); !ok || got != "01" {
		t.Fatalf("DISCNUMBER = %q", got)
	}
	if got, ok := tag.Get("DISCTOTAL"); !ok || got != "05" {
		t.Fatalf("DISCTOTAL = %q", got)
	}
}

func TestVorbisCommentsCompanionTagRoundTrip(t *testing.T) {
	vc := NewVorbisCommentsTag()
	vc.InsertKV("ARTIST", "Foo artist")
	vc.InsertKV("WHAT", "Something format-specific")
	if vc.Len() != 2 {
		t.Fatalf("len = %d", vc.Len())
	}
	tag := vc.Tag()
	if s, _ := tag.Artist(); s != "Foo artist" {
		t.Fatalf("artist = %q", s)
	}
	// The format-specific item hides in the companion
	if tag.Len() != 1 {
		t.Fatalf("tag len = %d, want 1", tag.Len())
	}
	vc2 := VorbisCommentsTagFromTag(tag, vc.Vendor)
	if vc2.Len() != 2 {
		t.Fatalf("merged len = %d, want 2", vc2.Len())
	}
	if got, ok := vc2.Get("WHAT"); !ok || got != "Something format-specific" {
		t.Fatalf("WHAT = %q", got)
	}
}
