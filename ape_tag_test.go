package lofty

// Ports of lofty-rs src/ape/tag/mod.rs unit tests.

import (
	"bytes"
	"os"
	"testing"
)

func mustApeItem(t *testing.T, key string, value ItemValue) *ApeItem {
	t.Helper()
	item, err := NewApeItem(key, value)
	if err != nil {
		t.Fatalf("NewApeItem(%q): %v", key, err)
	}
	return item
}

func readApev2Asset(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.apev2")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestApeParseApe(t *testing.T) {
	expected := NewApeTag()
	expected.Push(mustApeItem(t, "TITLE", ItemValueText("Foo title")))
	expected.Push(mustApeItem(t, "ARTIST", ItemValueText("Bar artist")))
	expected.Push(mustApeItem(t, "ALBUM", ItemValueText("Baz album")))
	expected.Push(mustApeItem(t, "COMMENT", ItemValueText("Qux comment")))
	expected.Push(mustApeItem(t, "YEAR", ItemValueText("1984")))
	expected.Push(mustApeItem(t, "TRACK", ItemValueText("1")))
	expected.Push(mustApeItem(t, "GENRE", ItemValueText("Classical")))

	reader := bytes.NewReader(readApev2Asset(t))
	parsed, header, err := readApeTag(reader, false, NewParseOptions())
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

func TestApeReRead(t *testing.T) {
	reader := bytes.NewReader(readApev2Asset(t))
	parsed, header, err := readApeTag(reader, false, NewParseOptions())
	if err != nil || header == nil || parsed == nil {
		t.Fatalf("read: %v", err)
	}
	var buf bytes.Buffer
	if err := dumpTag(parsed.Tag(), &buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	reParsed, header2, err := readApeTag(bytes.NewReader(buf.Bytes()), false, NewParseOptions())
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

func verifyTagCommon(t *testing.T, tag *Tag, trackNumber, genre bool) {
	t.Helper()
	check := func(key ItemKey, want string) {
		t.Helper()
		if got, _ := tag.GetString(key); got != want {
			t.Fatalf("%v = %q, want %q", key, got, want)
		}
	}
	check(ItemKeyTrackTitle, "Foo title")
	check(ItemKeyTrackArtist, "Bar artist")
	check(ItemKeyAlbumTitle, "Baz album")
	check(ItemKeyComment, "Qux comment")
	if trackNumber {
		check(ItemKeyTrackNumber, "1")
	}
	if genre {
		check(ItemKeyGenre, "Classical")
	}
}

func createTagCommon(ty TagType) *Tag {
	tag := NewTag(ty)
	tag.InsertText(ItemKeyTrackTitle, "Foo title")
	tag.InsertText(ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(ItemKeyComment, "Qux comment")
	tag.InsertText(ItemKeyTrackNumber, "1")
	tag.InsertText(ItemKeyGenre, "Classical")
	return tag
}

func TestApeToTag(t *testing.T) {
	reader := bytes.NewReader(readApev2Asset(t))
	ape, header, err := readApeTag(reader, false, NewParseOptions())
	if err != nil || header == nil || ape == nil {
		t.Fatalf("read: %v", err)
	}
	tag := ape.Tag()
	verifyTagCommon(t, tag, true, true)
}

func TestTagToApe(t *testing.T) {
	apeTag := ApeTagFromTag(createTagCommon(TagTypeApe))
	verifyKey := func(key, expected string) {
		t.Helper()
		got, ok := apeTag.Item(key)
		if !ok || !got.Value.Equal(ItemValueText(expected)) {
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
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyTrackNumber, ItemValueText("1")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if _, ok := ape.TrackTotal(); ok {
		t.Fatal("expected no track total")
	}
}

func TestApeTrackTotalTagToApe(t *testing.T) {
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyTrackTotal, ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 0 {
		t.Fatalf("track = %d, want 0", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeTrackNumberAndTrackTotalTagToApe(t *testing.T) {
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyTrackNumber, ItemValueText("1")))
	tag.Push(NewTagItem(ItemKeyTrackTotal, ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Track(); n != 1 {
		t.Fatalf("track = %d", n)
	}
	if n, _ := ape.TrackTotal(); n != 2 {
		t.Fatalf("track_total = %d", n)
	}
}

func TestApeDiskNumberTagToApe(t *testing.T) {
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyDiscNumber, ItemValueText("1")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if _, ok := ape.DiskTotal(); ok {
		t.Fatal("expected no disk total")
	}
}

func TestApeDiskTotalTagToApe(t *testing.T) {
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyDiscTotal, ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 0 {
		t.Fatalf("disk = %d, want 0", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

func TestApeDiskNumberAndDiskTotalTagToApe(t *testing.T) {
	tag := NewTag(TagTypeApe)
	tag.Push(NewTagItem(ItemKeyDiscNumber, ItemValueText("1")))
	tag.Push(NewTagItem(ItemKeyDiscTotal, ItemValueText("2")))
	ape := ApeTagFromTag(tag)
	if n, _ := ape.Disk(); n != 1 {
		t.Fatalf("disk = %d", n)
	}
	if n, _ := ape.DiskTotal(); n != 2 {
		t.Fatalf("disk_total = %d", n)
	}
}

func TestApeSkipReadingCoverArt(t *testing.T) {
	p := PictureUnchecked(make([]byte, 50)).
		PicType(PictureTypeCoverFront).
		MimeType(MimeTypeJpeg).
		Build()
	tag := NewTag(TagTypeApe)
	tag.PushPicture(p)
	tag.SetArtist("Foo artist")

	var buf bytes.Buffer
	if err := dumpTag(tag, &buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ReadCoverArt = false
	ape, header, err := readApeTag(bytes.NewReader(buf.Bytes()), false, po)
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
