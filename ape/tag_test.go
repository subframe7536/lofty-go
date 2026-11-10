package ape

// Ports of lofty-rs src/ape/tag/mod.rs unit tests.

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func mustApeItem(t *testing.T, key string, value meta.ItemValue) *ApeItem {
	t.Helper()
	item, err := NewApeItem(key, value)
	if err != nil {
		t.Fatalf("NewApeItem(%q): %v", key, err)
	}
	return item
}

func readApev2Asset(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../lofty-rs/lofty/tests/tags/assets/test.apev2")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestApeParseApe(t *testing.T) {
	expected := NewApeTag()
	expected.Push(mustApeItem(t, "TITLE", meta.ItemValueText("Foo title")))
	expected.Push(mustApeItem(t, "ARTIST", meta.ItemValueText("Bar artist")))
	expected.Push(mustApeItem(t, "ALBUM", meta.ItemValueText("Baz album")))
	expected.Push(mustApeItem(t, "COMMENT", meta.ItemValueText("Qux comment")))
	expected.Push(mustApeItem(t, "YEAR", meta.ItemValueText("1984")))
	expected.Push(mustApeItem(t, "TRACK", meta.ItemValueText("1")))
	expected.Push(mustApeItem(t, "GENRE", meta.ItemValueText("Classical")))

	reader := bytes.NewReader(readApev2Asset(t))
	parsed, header, err := ReadApeTag(reader, false, meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if header == nil || parsed == nil {
		t.Fatal("expected a tag to be parsed")
	}
	if expected.Len() != parsed.Len() {
		t.Fatalf("len = %d, want %d", parsed.Len(), expected.Len())
	}
	for _, item := range expected.Items() {
		got, ok := parsed.Item(item.Key)
		if !ok {
			t.Fatalf("missing item %q", item.Key)
		}
		if !got.Value.Equal(item.Value) {
			t.Fatalf("item %q = %v, want %v", item.Key, got.Value, item.Value)
		}
	}
}

// Ported from ape/tag/read.rs::ape_without_header — a footer-only tag parses
// when the header block is absent.
func TestApeWithoutHeader(t *testing.T) {
	data := readApev2Asset(t)

	// Drain the 32-byte header, then mark the footer as footer-only
	tagBytes := data[32:]
	flagsPos := len(tagBytes) - 12 // preamble+version+size+count = 20 bytes into the footer
	binary.LittleEndian.PutUint32(tagBytes[flagsPos:], apeHeaderContainsFooter)

	reader := bytes.NewReader(tagBytes)
	if _, err := reader.Seek(-32, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	parsed, _, err := ReadApeTag(reader, true, po)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Len() != 7 {
		t.Fatalf("len = %d, want 7", parsed.Len())
	}
}

// Ported from ape/tag/read.rs::ape_without_footer — a header-only tag parses
// when the footer block is absent.
func TestApeWithoutFooter(t *testing.T) {
	data := readApev2Asset(t)

	// Trim the 32-byte footer, then mark the header as header-only
	tagBytes := data[:len(data)-32]
	binary.LittleEndian.PutUint32(tagBytes[20:], apeHeaderContainsHeader|apeHeaderIsHeader)

	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	parsed, _, err := ReadApeTag(bytes.NewReader(tagBytes), false, po)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Len() != 7 {
		t.Fatalf("len = %d, want 7", parsed.Len())
	}
}

func TestApeReRead(t *testing.T) {
	reader := bytes.NewReader(readApev2Asset(t))
	parsed, header, err := ReadApeTag(reader, false, meta.NewParseOptions())
	if err != nil || header == nil || parsed == nil {
		t.Fatalf("read: %v", err)
	}
	var buf bytes.Buffer
	if err := meta.DumpTag(parsed.Tag(), &buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	reParsed, header2, err := ReadApeTag(bytes.NewReader(buf.Bytes()), false, meta.NewParseOptions())
	if err != nil || header2 == nil || reParsed == nil {
		t.Fatalf("re-read: %v", err)
	}
	if parsed.Len() != reParsed.Len() {
		t.Fatalf("re-read len = %d, want %d", reParsed.Len(), parsed.Len())
	}
	for _, item := range parsed.Items() {
		got, ok := reParsed.Item(item.Key)
		if !ok || !got.Value.Equal(item.Value) {
			t.Fatalf("item %q mismatch", item.Key)
		}
	}
}

func verifyTagCommon(t *testing.T, tag *meta.Tag, trackNumber, genre bool) {
	t.Helper()
	check := func(key meta.ItemKey, want string) {
		t.Helper()
		if got, _ := tag.GetString(key); got != want {
			t.Fatalf("%v = %q, want %q", key, got, want)
		}
	}
	check(meta.ItemKeyTrackTitle, "Foo title")
	check(meta.ItemKeyTrackArtist, "Bar artist")
	check(meta.ItemKeyAlbumTitle, "Baz album")
	check(meta.ItemKeyComment, "Qux comment")
	if trackNumber {
		check(meta.ItemKeyTrackNumber, "1")
	}
	if genre {
		check(meta.ItemKeyGenre, "Classical")
	}
}

func createTagCommon(ty meta.TagType) *meta.Tag {
	tag := meta.NewTag(ty)
	tag.InsertText(meta.ItemKeyTrackTitle, "Foo title")
	tag.InsertText(meta.ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(meta.ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(meta.ItemKeyComment, "Qux comment")
	tag.InsertText(meta.ItemKeyTrackNumber, "1")
	tag.InsertText(meta.ItemKeyGenre, "Classical")
	return tag
}

func TestApeToTag(t *testing.T) {
	reader := bytes.NewReader(readApev2Asset(t))
	ape, header, err := ReadApeTag(reader, false, meta.NewParseOptions())
	if err != nil || header == nil || ape == nil {
		t.Fatalf("read: %v", err)
	}
	tag := ape.Tag()
	verifyTagCommon(t, tag, true, true)
}

func TestTagToApe(t *testing.T) {
	apeTag := ApeTagFromTag(createTagCommon(meta.TagTypeApe))
	verifyKey := func(key, expected string) {
		t.Helper()
		got, ok := apeTag.Item(key)
		if !ok || !got.Value.Equal(meta.ItemValueText(expected)) {
			t.Fatalf("%q = %v, want %q", key, got.Value, expected)
		}
	}
	verifyKey("Title", "Foo title")
	verifyKey("Artist", "Bar artist")
	verifyKey("Album", "Baz album")
	verifyKey("Comment", "Qux comment")
	verifyKey("Track", "1")
	verifyKey("Genre", "Classical")
}

func TestApeSetTrack(t *testing.T) {
	ape := NewApeTag()
	ape.SetTrack(1)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if _, ok := ape.TrackTotal(); ok {
		t.Fatal("expected no track total")
	}
}

func TestApeSetTrackTotal(t *testing.T) {
	ape := NewApeTag()
	ape.SetTrackTotal(2)
	if n, _ := ape.Track(); n != 0 {
		t.Fatalf("track = %d, want 0", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeSetTrackAndTrackTotal(t *testing.T) {
	ape := NewApeTag()
	ape.SetTrack(1)
	ape.SetTrackTotal(2)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeSetTrackTotalAndTrack(t *testing.T) {
	ape := NewApeTag()
	ape.SetTrackTotal(2)
	ape.SetTrack(1)
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
}

func TestApeSetDisk(t *testing.T) {
	ape := NewApeTag()
	ape.SetDisk(1)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if _, ok := ape.DiskTotal(); ok {
		t.Fatal("expected no disk total")
	}
}

func TestApeSetDiskTotal(t *testing.T) {
	ape := NewApeTag()
	ape.SetDiskTotal(2)
	if n, _ := ape.Disk(); n != 0 {
		t.Fatalf("disk = %d, want 0", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

func TestApeSetDiskAndDiskTotal(t *testing.T) {
	ape := NewApeTag()
	ape.SetDisk(1)
	ape.SetDiskTotal(2)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

func TestApeSetDiskTotalAndDisk(t *testing.T) {
	ape := NewApeTag()
	ape.SetDiskTotal(2)
	ape.SetDisk(1)
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
}

func TestApeTrackNumberTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("1")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if _, ok := ape.TrackTotal(); ok {
		t.Fatal("expected no track total")
	}
}

func TestApeTrackTotalTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyTrackTotal, meta.ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 0 {
		t.Fatalf("track = %d, want 0", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeTrackNumberAndTrackTotalTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("1")))
	tag.Push(meta.NewTagItem(meta.ItemKeyTrackTotal, meta.ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeDiskNumberTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyDiscNumber, meta.ItemValueText("1")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if _, ok := ape.DiskTotal(); ok {
		t.Fatal("expected no disk total")
	}
}

func TestApeDiskTotalTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyDiscTotal, meta.ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 0 {
		t.Fatalf("disk = %d, want 0", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

func TestApeDiskNumberAndDiskTotalTagToApe(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyDiscNumber, meta.ItemValueText("1")))
	tag.Push(meta.NewTagItem(meta.ItemKeyDiscTotal, meta.ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

// Ported from id3/v2/util/pairs.rs::whitespace_in_number (reached through the
// ApeTag generic conversion — Rust `set_number` trims before parsing).
func TestApeWhitespaceInNumber(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.Push(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("  12  ")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 12 {
		t.Fatalf("track = %d, want 12", n)
	}
}

// Ported from id3/v2/util/pairs.rs::empty_number_string — empty or
// whitespace-only number text is skipped entirely.
func TestApeEmptyNumberString(t *testing.T) {
	for _, text := range []string{"", "        "} {
		tag := meta.NewTag(meta.TagTypeApe)
		tag.Push(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText(text)))
		ape := ApeTagFromTag(tag)
		if item, ok := ape.Item("Track"); ok {
			t.Fatalf("Track item %v written for %q", item, text)
		}
	}
}

// Ported from ape/tag/mod.rs::split_text_values — same-key text items merge
// with NUL on push, and the generic conversion splits them back out.
func TestApeSplitTextValues(t *testing.T) {
	ape := NewApeTag()
	ape.Push(mustApeItem(t, "Artists", meta.ItemValueText("Serial-ATA")))
	ape.Push(mustApeItem(t, "Artists", meta.ItemValueText("Lofty")))

	artists, ok := ape.Item("Artists")
	if !ok {
		t.Fatal("no Artists item")
	}
	text, _ := artists.Value.IntoString()
	if text != "Serial-ATA\x00Lofty" {
		t.Fatalf("Artists = %q, want merged multi-value", text)
	}

	tag := ape.Tag()
	var values []string
	for _, item := range tag.GetItems(meta.ItemKeyTrackArtists) {
		s, _ := item.Value().IntoString()
		values = append(values, s)
	}
	if len(values) != 2 || values[0] != "Serial-ATA" || values[1] != "Lofty" {
		t.Fatalf("TrackArtists = %v, want [Serial-ATA Lofty]", values)
	}
}

// Ported from ape/tag/mod.rs::merge_text_values — multiple same-key generic
// items merge into one NUL-separated APE item.
func TestApeMergeTextValues(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeApe)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtists, meta.ItemValueText("Serial-ATA")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtists, meta.ItemValueText("Lofty")))

	ape := ApeTagFromTag(tag)
	if ape.Len() != 1 {
		t.Fatalf("len = %d, want 1", ape.Len())
	}
	artists, ok := ape.Item("Artists")
	if !ok {
		t.Fatal("no Artists item")
	}
	text, _ := artists.Value.IntoString()
	if text != "Serial-ATA\x00Lofty" {
		t.Fatalf("Artists = %q, want merged multi-value", text)
	}
}

func TestApeSkipReadingCoverArt(t *testing.T) {
	p := meta.PictureUnchecked(make([]byte, 50)).
		PicType(meta.PictureTypeCoverFront).
		MimeType(meta.MimeTypeJpeg).
		Build()
	tag := meta.NewTag(meta.TagTypeApe)
	tag.PushPicture(p)
	tag.SetArtist("Foo artist")

	var buf bytes.Buffer
	if err := meta.DumpTag(tag, &buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	po := meta.NewParseOptions()
	po.ReadCoverArt = false
	ape, header, err := ReadApeTag(bytes.NewReader(buf.Bytes()), false, po)
	if err != nil || header == nil || ape == nil {
		t.Fatalf("read: %v", err)
	}
	if ape.Len() != 1 {
		t.Fatalf("len = %d, want 1", ape.Len())
	}
}

func TestApeRemoveDiskTotalPreservesDiskNumber(t *testing.T) {
	ape := NewApeTag()
	ape.SetTrack(2)
	ape.SetDisk(3)
	ape.SetDiskTotal(5)
	ape.RemoveDiskTotal()
	if n, _ := ape.Disk(); n != 3 {
		t.Fatalf("disk = %d, want 3", n)
	}
	if _, ok := ape.DiskTotal(); ok {
		t.Fatal("expected no disk total")
	}
	if n, _ := ape.Track(); n != 2 {
		t.Fatalf("track = %d, want 2", n)
	}
}
