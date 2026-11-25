package lofty_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/subframe7536/lofty-go/id3v2"

	. "github.com/subframe7536/lofty-go"
)

func loftyAsset(rel string) string {
	return filepath.Join("..", "lofty-rs", "lofty", filepath.FromSlash(rel))
}

// --- tests/files/aac.rs: write, remove_id3v2, remove_id3v1 ---

func TestAacWrite(t *testing.T) {
	f := tempFile(t, loftyAsset("tests/files/assets/minimal/full_test.aac"))
	defer f.Close()

	osFile := NewOSFile(f)
	p, err := NewProbe(osFile).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := p.ReadBound(osFile)
	if err != nil {
		t.Fatal(err)
	}
	if tagged.FileType() != FileTypeAac {
		t.Fatalf("file type = %v, want Aac", tagged.FileType())
	}
	setArtist(t, tagged, TagTypeID3v2, "Foo artist", "Bar artist", 1)
	setArtist(t, tagged, TagTypeID3v1, "Bar artist", "Baz artist", 1)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	p2, err := NewProbe(osFile).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged2, err := p2.ReadBound(osFile)
	if err != nil {
		t.Fatal(err)
	}
	setArtist(t, tagged2, TagTypeID3v2, "Bar artist", "Foo artist", 1)
	setArtist(t, tagged2, TagTypeID3v1, "Baz artist", "Bar artist", 1)
}

func TestAacRemoveId3v2(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.aac"), TagTypeID3v2)
}

func TestAacRemoveId3v1(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.aac"), TagTypeID3v1)
}

// --- tests/files/ape.rs: remove_ape, remove_id3v1, remove_id3v2 ---

func TestApeRemoveApe(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.ape"), TagTypeApe)
}

func TestApeRemoveId3v1(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.ape"), TagTypeID3v1)
}

func TestApeRemoveId3v2(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.ape"), TagTypeID3v2)
}

// --- tests/files/mpeg.rs: remove_id3v2, remove_id3v1, remove_ape ---

func TestMpegRemoveId3v2FileLike(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.mp3"), TagTypeID3v2)
}

func TestMpegRemoveId3v1FileLike(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.mp3"), TagTypeID3v1)
}

func TestMpegRemoveApeFileLike(t *testing.T) {
	removeTagTest(t, loftyAsset("tests/files/assets/minimal/full_test.mp3"), TagTypeApe)
}

// --- tests/files/flac.rs: stream_info_is_last_no_padding ---

func TestFlacStreamInfoIsLastNoPadding(t *testing.T) {
	f := tempFile(t, loftyAsset("tests/files/assets/stream_info_last.flac"))
	defer f.Close()

	flacFile, err := FlacFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if flacFile.VorbisComments.IsSome() {
		t.Fatal("expected no vorbis comments initially")
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	opts := NewWriteOptions()
	opts.PreferredPadding = 0
	tag := NewVorbisCommentsTag()
	tag.SetArtist("Foo Artist")
	if err := tag.SaveTo(NewOSFile(f), opts); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	flacFile, err = FlacFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	vc, ok := flacFile.VorbisComments.Get()
	if !ok {
		t.Fatal("expected vorbis comments after write")
	}
	if a, _ := vc.Artist(); a != "Foo Artist" {
		t.Fatalf("artist = %q, want Foo Artist", a)
	}

	// Stripping the Vorbis Comments should set STREAMINFO as the last metadata block.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	emptyTag := NewVorbisCommentsTag()
	if err := emptyTag.SaveTo(NewOSFile(f), opts); err != nil {
		t.Fatalf("strip SaveTo: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	flacFile, err = FlacFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if flacFile.VorbisComments.IsSome() {
		t.Fatal("expected vorbis comments to be stripped")
	}
}

// --- tests/picture/format_parsers.rs: id3v24_apic, as_apic_bytes, id3v22_pic ---

func createOriginalPictureForFormatParsers(t *testing.T) Picture {
	t.Helper()
	raw, err := os.ReadFile(loftyAsset("tests/picture/assets/png_640x628.png"))
	if err != nil {
		t.Fatal(err)
	}
	pic, err := PictureFromReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	pic.SetDescription("png_640x628.png")
	pic.SetPicType(PictureTypeCoverFront)
	return *pic
}

func TestPictureFormatId3v24Apic(t *testing.T) {
	buf, err := os.ReadFile(loftyAsset("tests/picture/assets/png_640x628.apic"))
	if err != nil {
		t.Fatal(err)
	}
	apic, err := id3v2.ParseAttachedPictureFrame(bytes.NewReader(buf), id3v2.FrameFlags{}, Id3v2Version4)
	if err != nil {
		t.Fatalf("ParseAttachedPictureFrame v4: %v", err)
	}
	want := createOriginalPictureForFormatParsers(t)
	if !apic.Picture.Equal(want) {
		t.Fatal("parsed ID3v2.4 APIC does not equal original picture")
	}
}

func TestPictureFormatAsApicBytes(t *testing.T) {
	want, err := os.ReadFile(loftyAsset("tests/picture/assets/png_640x628.apic"))
	if err != nil {
		t.Fatal(err)
	}
	apic := id3v2.NewAttachedPictureFrame(TextEncodingLatin1, createOriginalPictureForFormatParsers(t))
	got, err := apic.AsBytes(NewWriteOptions())
	if err != nil {
		t.Fatalf("AsBytes: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("APIC bytes mismatch: got %d bytes, want %d bytes", len(got), len(want))
	}
}

func TestPictureFormatId3v22Pic(t *testing.T) {
	buf, err := os.ReadFile(loftyAsset("tests/picture/assets/png_640x628.pic"))
	if err != nil {
		t.Fatal(err)
	}
	pic, err := id3v2.ParseAttachedPictureFrame(bytes.NewReader(buf), id3v2.FrameFlags{}, Id3v2Version2)
	if err != nil {
		t.Fatalf("ParseAttachedPictureFrame v2: %v", err)
	}
	want := createOriginalPictureForFormatParsers(t)
	if !pic.Picture.Equal(want) {
		t.Fatal("parsed ID3v2.2 PIC does not equal original picture")
	}
}

// --- tests/taglib/test_fileref.rs::test_file_resolver ---

type dummyMP3Resolver struct{}

func (dummyMP3Resolver) Extension() string             { return "mp3" }
func (dummyMP3Resolver) Guess(_ []byte) bool           { return false }
func (dummyMP3Resolver) PrimaryTagType() TagType       { return TagTypeVorbisComments }
func (dummyMP3Resolver) TagSupport(TagType) TagSupport { return TagSupportUnsupported }
func (dummyMP3Resolver) ReadFrom(_ io.ReadSeeker, _ ParseOptions) (*TaggedFile, error) {
	return NewTaggedFile(FileTypeVorbis, nil, FileProperties{}), nil
}

func TestTaglibFileRefCustomResolver(t *testing.T) {
	prev := GetGlobalOptions()
	t.Cleanup(func() {
		UnregisterCustomResolver("DummyMP3Resolver")
		ApplyGlobalOptions(prev)
	})

	opts := NewGlobalOptions()
	opts.UseCustomResolvers = true
	ApplyGlobalOptions(opts)

	mp3Path := loftyAsset("tests/taglib/data/xing.mp3")
	f, err := ReadFromPath(mp3Path)
	if err != nil {
		t.Fatal(err)
	}
	if f.FileType() != FileTypeMpeg {
		t.Fatalf("before register: FileType = %v, want Mpeg", f.FileType())
	}

	RegisterCustomResolver("DummyMP3Resolver", dummyMP3Resolver{})
	f2, err := ReadFromPath(mp3Path)
	if err != nil {
		t.Fatal(err)
	}
	if f2.FileType() != FileTypeVorbis {
		t.Fatalf("after register: FileType = %v, want Vorbis", f2.FileType())
	}
}

// --- tests/taglib/test_flac.rs::test_properties ---

func TestTaglibFlacPropertiesRoundtrip(t *testing.T) {
	tag := NewVorbisCommentsTag()
	pairs := [][2]string{
		{"ALBUM", "Album"},
		{"ALBUMARTIST", "Album Artist"},
		{"ALBUMARTISTSORT", "Album Artist Sort"},
		{"ALBUMSORT", "Album Sort"},
		{"ARTIST", "Artist"},
		{"ARTISTS", "Artists"},
		{"ARTISTSORT", "Artist Sort"},
		{"ASIN", "ASIN"},
		{"BARCODE", "Barcode"},
		{"CATALOGNUMBER", "Catalog Number 1"},
		{"CATALOGNUMBER", "Catalog Number 2"},
		{"COMMENT", "Comment"},
		{"DATE", "2021-01-10"},
		{"DISCNUMBER", "3/5"},
		{"GENRE", "Genre"},
		{"ISRC", "UKAAA0500001"},
		{"LABEL", "Label 1"},
		{"LABEL", "Label 2"},
		{"MEDIA", "Media"},
		{"MUSICBRAINZ_ALBUMARTISTID", "MusicBrainz_AlbumartistID"},
		{"MUSICBRAINZ_ALBUMID", "MusicBrainz_AlbumID"},
		{"MUSICBRAINZ_ARTISTID", "MusicBrainz_ArtistID"},
		{"MUSICBRAINZ_RELEASEGROUPID", "MusicBrainz_ReleasegroupID"},
		{"MUSICBRAINZ_RELEASETRACKID", "MusicBrainz_ReleasetrackID"},
		{"MUSICBRAINZ_TRACKID", "MusicBrainz_TrackID"},
		{"ORIGINALDATE", "2021-01-09"},
		{"RELEASECOUNTRY", "Release Country"},
		{"RELEASESTATUS", "Release Status"},
		{"RELEASETYPE", "Release Type"},
		{"SCRIPT", "Script"},
		{"TITLE", "Title"},
		{"TRACKNUMBER", "2"},
		{"TRACKTOTAL", "4"},
	}
	for _, kv := range pairs {
		tag.PushKV(kv[0], kv[1])
	}

	f := tempFile(t, loftyAsset("tests/taglib/data/no-tags.flac"))
	defer f.Close()

	flacFile, err := FlacFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	flacFile.VorbisComments = Some(tag)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := flacFile.VorbisComments.MustGet().SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ImplicitConversions = false
	flacFile2, err := FlacFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := flacFile2.VorbisComments.Get()
	if !ok {
		t.Fatal("missing vorbis comments on reread")
	}
	if !reflect.DeepEqual(got.Pairs(), tag.Pairs()) {
		t.Fatalf("vorbis pairs mismatch:\ngot:  %v\nwant: %v", got.Pairs(), tag.Pairs())
	}
}

// --- Coverage for newly added public API surface ---

func TestPortCompletenessNewAPIs(t *testing.T) {
	// FileEncodingError.IsUnsupportedTag
	encErr := &FileEncodingError{FileType: FileTypeFlac, Source: &UnsupportedTagError{}}
	if !encErr.IsUnsupportedTag() {
		t.Fatal("expected IsUnsupportedTag() == true")
	}
	otherErr := &FileEncodingError{FileType: FileTypeFlac, Source: io.EOF}
	if otherErr.IsUnsupportedTag() {
		t.Fatal("expected IsUnsupportedTag() == false")
	}

	// TaggedFile.PrimaryTagType / TagSupport & interface satisfaction
	tf := NewTaggedFile(FileTypeMpeg, nil, FileProperties{})
	var _ TaggedFileExt = tf
	if tf.PrimaryTagType() != TagTypeID3v2 {
		t.Fatalf("PrimaryTagType = %v", tf.PrimaryTagType())
	}
	if tf.TagSupport(TagTypeID3v2) != TagSupportReadWrite {
		t.Fatalf("TagSupport(ID3v2) = %v", tf.TagSupport(TagTypeID3v2))
	}

	// Accessor & TagExt interface satisfaction on *Tag
	genericTag := NewTag(TagTypeID3v2)
	var _ Accessor = genericTag
	var _ TagExt = genericTag
	var _ TagExt = NewId3v1Tag()
	var _ TagExt = NewVorbisCommentsTag()
	var _ TagExt = NewApeTag()
	var _ TagExt = NewLyrics3Tag()
	var _ TagExt = NewLyrics3v2Tag()

	// Id3v1Tag accessors & TagExt
	v1 := NewId3v1Tag()
	if !v1.IsEmpty() || v1.Len() != 0 || v1.TagType() != TagTypeID3v1 {
		t.Fatal("unexpected empty Id3v1Tag")
	}
	v1.SetTitle("T")
	v1.SetArtist("A")
	v1.SetAlbum("Al")
	v1.SetComment("C")
	v1.SetTrack(7)
	v1.SetGenre("rock")
	v1.SetDate(Timestamp{Year: 2024})
	if v1.Len() != 7 || !v1.Contains(ItemKeyGenre) {
		t.Fatalf("Id3v1Tag Len=%d", v1.Len())
	}
	if tr, ok := v1.Track(); !ok || tr != 7 {
		t.Fatalf("Id3v1Tag Track = %d, %v", tr, ok)
	}
	v1.RemoveGenre()
	if v1.Genre.IsSome() {
		t.Fatal("expected Genre removed")
	}
	v1.Clear()
	if !v1.IsEmpty() {
		t.Fatal("expected empty Id3v1Tag after Clear")
	}

	// RiffInfoTag & AiffTextTag
	riff := &RiffInfoTag{}
	riff.InsertKV("INAM", "Song")
	riff.InsertKV("IART", "Band")
	if !riff.Contains("inam") || len(riff.Items()) != 2 || riff.TagType() != TagTypeRiffInfo {
		t.Fatalf("RiffInfoTag state: %v", riff.Items())
	}
	riff.Clear()
	if !riff.IsEmpty() {
		t.Fatal("expected empty RiffInfoTag")
	}

	aiff := &AiffTextTag{}
	aiff.SetTitle("N")
	aiff.SetArtist("Au")
	aiff.SetComment("An")
	if v, ok := aiff.Title(); !ok || v != "N" {
		t.Fatalf("AiffTextTag.Title = %q, %v", v, ok)
	}
	if v, ok := aiff.Artist(); !ok || v != "Au" {
		t.Fatalf("AiffTextTag.Artist = %q, %v", v, ok)
	}
	if v, ok := aiff.Comment(); !ok || v != "An" {
		t.Fatalf("AiffTextTag.Comment = %q, %v", v, ok)
	}
	aiff.Clear()
	if aiff.TagType() != TagTypeAiffText || aiff.Name.IsSome() {
		t.Fatal("unexpected AiffTextTag after Clear")
	}

	// VorbisCommentsTag.InsertPictureReplacing
	vc := NewVorbisCommentsTag()
	icon1 := PictureUnchecked([]byte{1}).PicType(PictureTypeIcon).Build()
	icon2 := PictureUnchecked([]byte{2}).PicType(PictureTypeIcon).Build()
	info := Some(PictureInformation{Width: 32, Height: 32})
	if rep, err := vc.InsertPictureReplacing(icon1, info); err != nil || rep.IsSome() {
		t.Fatalf("first icon insert: rep=%v err=%v", rep, err)
	}
	rep, err := vc.InsertPictureReplacing(icon2, info)
	if err != nil || !rep.IsSome() {
		t.Fatalf("second icon insert should replace first: rep=%v err=%v", rep, err)
	}

	// Lyrics3 helpers
	l1 := NewLyrics3Tag()
	l1.Lyrics = "hello"
	if l1.TagType() != TagTypeLyrics3 || !l1.Contains(ItemKeyLyrics) || Lyrics3v1MaxLyricsLen != 5100 {
		t.Fatal("unexpected Lyrics3Tag state")
	}
	l1.Clear()
	if !l1.IsEmpty() {
		t.Fatal("expected empty Lyrics3Tag")
	}

	// OggPage / OggPaginate / ReadOggPage
	pages := OggPaginate([][]byte{[]byte("hello ogg")}, 0x12345678, 0, OggPageFlagBOS|OggPageFlagEOS)
	if len(pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(pages))
	}
	pages[0].GenCrc()
	hdr := pages[0].Header()
	if hdr.HeaderType&(OggPageFlagBOS|OggPageFlagEOS) != (OggPageFlagBOS | OggPageFlagEOS) {
		t.Fatalf("HeaderType = 0x%x", hdr.HeaderType)
	}
	raw := pages[0].AsBytes()
	parsed, err := ReadOggPage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadOggPage: %v", err)
	}
	if !bytes.Equal(parsed.Content(), []byte("hello ogg")) || parsed.Header().StreamSerial != 0x12345678 {
		t.Fatalf("parsed OggPage mismatch: content=%q serial=0x%x", parsed.Content(), parsed.Header().StreamSerial)
	}

	// Mp4File.Ftyp
	mp4f, err := Mp4FileReadFrom(mustOpenAsset(t, "tests/files/assets/minimal/m4a_codec_aac.m4a"), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if mp4f.Ftyp() == "" {
		t.Fatal("expected non-empty Mp4File.Ftyp()")
	}
}

func mustOpenAsset(t *testing.T, rel string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Clean(loftyAsset(rel)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
