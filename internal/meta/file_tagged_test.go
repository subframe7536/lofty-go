package meta

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func someProps() FileProperties {
	return NewFileProperties(5*time.Second, Some(uint32(128)), Some(uint32(128)), Some(uint32(44100)),
		None[uint8](), Some(uint8(2)), None[ChannelMask]())
}

func tagTypesOf(tf *TaggedFile) []TagType { return tf.TagTypes() }

func TestChangeFileTypeDropsUnsupportedAndResetsProperties(t *testing.T) {
	tf := NewTaggedFile(FileTypeMpeg, []*Tag{NewTag(TagTypeID3v2), NewTag(TagTypeID3v1)}, someProps())
	tf.ChangeFileType(FileTypeWavPack) // WavPack: ID3v1 + APE, no ID3v2
	got := tagTypesOf(tf)
	if len(got) != 1 || got[0] != TagTypeID3v1 {
		t.Fatalf("tags after ChangeFileType = %v, want [Id3v1]", got)
	}
	if tf.Properties().Duration() != 0 || !tf.Properties().IsEmpty() {
		t.Fatalf("properties not reset: %+v", tf.Properties())
	}
	if !tf.FileType().Equal(FileTypeWavPack) {
		t.Fatalf("file type = %v", tf.FileType())
	}
	// Read-only support is kept (ID3v2 is read-only in FLAC).
	tf2 := NewTaggedFile(FileTypeMpeg, []*Tag{NewTag(TagTypeID3v2)}, someProps())
	tf2.ChangeFileType(FileTypeFlac)
	if len(tf2.Tags()) != 1 {
		t.Fatalf("read-only tag dropped: %v", tagTypesOf(tf2))
	}
}

func TestInsertTagChecksSupportAndReturnsReplaced(t *testing.T) {
	fl := NewTaggedFile(FileTypeFlac, nil, FileProperties{})
	if r := fl.InsertTag(NewTag(TagTypeMp4Ilst)); r != nil {
		t.Fatalf("unsupported insert returned %v", r)
	}
	if len(fl.Tags()) != 0 {
		t.Fatalf("unsupported tag inserted: %v", tagTypesOf(fl))
	}

	first := textTag(TagTypeVorbisComments, "one")
	second := textTag(TagTypeVorbisComments, "two")
	other := NewTag(TagTypeID3v2) // read-only but readable in FLAC
	fl.InsertTag(first)
	fl.InsertTag(other)
	if r := fl.InsertTag(second); r != first {
		t.Fatalf("replaced tag = %p want %p", r, first)
	}
	// replaced tag is removed, the new one is appended last
	tags := fl.Tags()
	if len(tags) != 2 || tags[0] != other || tags[1] != second {
		t.Fatalf("tags after replace = %v", tagTypesOf(fl))
	}
}

func TestConflictsReportedOnce(t *testing.T) {
	c := NewTaggedFile(FileTypeMpeg, []*Tag{NewTag(TagTypeLyrics3), NewTag(TagTypeID3v2), NewTag(TagTypeLyrics3v2)}, FileProperties{})
	got := c.Conflicts()
	if len(got) != 1 || got[0] != [2]TagType{TagTypeLyrics3, TagTypeLyrics3v2} {
		t.Fatalf("Conflicts() = %v", got)
	}
	c2 := NewTaggedFile(FileTypeMpeg, []*Tag{NewTag(TagTypeLyrics3v2), NewTag(TagTypeLyrics3)}, FileProperties{})
	got = c2.Conflicts()
	if len(got) != 1 || got[0] != [2]TagType{TagTypeLyrics3v2, TagTypeLyrics3} {
		t.Fatalf("Conflicts() = %v", got)
	}
	err := c.Save(NewBytesFile(mp3Bytes()), NewWriteOptions())
	var ce *ConflictingTagError
	if !errors.As(err, &ce) || ce.First != TagTypeLyrics3 || ce.Second != TagTypeLyrics3v2 {
		t.Fatalf("Save err = %v", err)
	}
}

// Rust's TaggedFile::save_to re-probes the destination for every tag, so an
// MPEG TaggedFile saved into a FLAC file must be rejected (FLAC ID3v2 is
// read-only) instead of being written with the MPEG writer.
func TestSaveReprobesDestination(t *testing.T) {
	resetFakeWrites()
	tf := NewTaggedFile(FileTypeMpeg, []*Tag{textTag(TagTypeID3v2, "x")}, FileProperties{})
	dst := NewBytesFile(flacBytes())
	err := tf.Save(dst, NewWriteOptions())
	var ut *UnsupportedTagError
	if !errors.As(err, &ut) {
		t.Fatalf("Save into FLAC: err = %v, want UnsupportedTagError", err)
	}
	if len(fakeWrites) != 0 {
		t.Fatalf("a writer ran: %v", fakeWrites)
	}

	// Same-format destination still goes through the (destination's) writer.
	if err := tf.Save(NewBytesFile(mp3Bytes()), NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if len(fakeWrites) != 1 || fakeWrites[0] != "MPEG:Id3v2" {
		t.Fatalf("writes = %v", fakeWrites)
	}

	// Destination of unknown format
	err = tf.Save(NewBytesFile([]byte("this is not audio at all, not even close")), NewWriteOptions())
	if err == nil {
		t.Fatal("expected error for unknown destination format")
	}
}

func TestSaveSkipsReadOnlyTags(t *testing.T) {
	resetFakeWrites()
	tf := NewTaggedFile(FileTypeFlac, []*Tag{
		textTag(TagTypeID3v2, "ro"),
		textTag(TagTypeVorbisComments, "rw"),
	}, FileProperties{})
	if err := tf.Save(NewBytesFile(flacBytes()), NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if len(fakeWrites) != 1 || fakeWrites[0] != "FLAC:VorbisComments" {
		t.Fatalf("writes = %v", fakeWrites)
	}
}

// Rust's Probe::read_bound uses the probe's own file type; nothing is sniffed.
func TestReadBoundHonoursProbeFileType(t *testing.T) {
	data := make([]byte, 300)
	copy(data, "zzzz") // content sniffing finds nothing
	bf := NewBytesFile(data)
	if _, err := NewProbe(bf).ReadBound(bf); err == nil {
		t.Fatal("sniffing probe should fail on junk")
	}
	bf = NewBytesFile(data)
	b, err := ProbeWithFileType(bf, FileTypeMpeg).ReadBound(bf)
	if err != nil {
		t.Fatalf("ReadBound with explicit type: %v", err)
	}
	if !b.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("file type = %v", b.FileType())
	}
	if bf.Pos() != 0 {
		t.Fatalf("file not rewound: pos %d", bf.Pos())
	}
	// SetFileType works the same, and a nil file falls back to the probe reader.
	bf = NewBytesFile(data)
	b, err = NewProbe(bf).SetFileType(FileTypeFlac).ReadBound(nil)
	if err != nil || !b.FileType().Equal(FileTypeFlac) {
		t.Fatalf("ReadBound(nil) = %v, %v", b, err)
	}
}

func TestBoundTaggedFileFromRewinds(t *testing.T) {
	bf := NewBytesFile(mp3Bytes())
	if _, err := BoundTaggedFileFrom(bf, NewParseOptions()); err != nil {
		t.Fatal(err)
	}
	if bf.Pos() != 0 {
		t.Fatalf("pos after BoundTaggedFileFrom = %d, want 0", bf.Pos())
	}
	// the fake reader consumed bytes, so this would be non-zero without the rewind
	bf = NewBytesFile(mp3Bytes())
	if _, err := NewProbe(bf).SetFileType(FileTypeMpeg).ReadBound(bf); err != nil {
		t.Fatal(err)
	}
	if bf.Pos() != 0 {
		t.Fatalf("pos after ReadBound = %d, want 0", bf.Pos())
	}
}

// TagType::remove_from_path takes the format from the extension only (Rust's
// VerifiedFile::new_from_path), while Tag::save_to_path sniffs content.
func TestRemoveFromPathUsesExtensionOnly(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	wo := NewWriteOptions()

	// >1024 junk bytes before the first frame: content sniffing fails, the
	// ".mp3" extension still selects MPEG.
	junky := append(make([]byte, 3000), mp3Bytes()...)
	p := write("junky.mp3", junky)
	resetFakeWrites()
	if err := TagTypeID3v2.RemoveFromPath(p, wo); err != nil {
		t.Fatalf("RemoveFromPath(junky.mp3): %v", err)
	}
	if len(fakeWrites) != 1 || fakeWrites[0] != "MPEG:Id3v2" {
		t.Fatalf("writes = %v", fakeWrites)
	}

	// FLAC content with an .mp3 extension: the extension wins, MPEG does not
	// support Vorbis comments.
	p = write("flac_named.mp3", flacBytes())
	resetFakeWrites()
	err := TagTypeVorbisComments.RemoveFromPath(p, wo)
	var ut *UnsupportedTagError
	if !errors.As(err, &ut) {
		t.Fatalf("err = %v, want UnsupportedTagError", err)
	}
	if len(fakeWrites) != 0 {
		t.Fatalf("writer ran: %v", fakeWrites)
	}

	// MPEG content without an extension: nothing is sniffed on this path.
	p = write("noext", mp3Bytes())
	err = TagTypeID3v2.RemoveFromPath(p, wo)
	var uf *UnknownFormatError
	if !errors.As(err, &uf) {
		t.Fatalf("err = %v, want UnknownFormatError", err)
	}
	if err := RemoveTagFromPath(p, TagTypeID3v2); !errors.As(err, &uf) {
		t.Fatalf("RemoveTagFromPath err = %v, want UnknownFormatError", err)
	}

	// Tag.SaveToPath is Rust's save_to (content sniffing), so it still works.
	resetFakeWrites()
	if err := textTag(TagTypeID3v2, "t").SaveToPath(p, wo); err != nil {
		t.Fatalf("SaveToPath(noext): %v", err)
	}
	if len(fakeWrites) != 1 || fakeWrites[0] != "MPEG:Id3v2" {
		t.Fatalf("writes = %v", fakeWrites)
	}
}
