package lofty_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/subframe7536/lofty-go"
)

// noProps is ParseOptions with ReadProperties disabled.
var noProps = ParseOptions{ReadProperties: false, ReadTags: true, ParsingMode: ParsingModeBestAttempt, MaxJunkBytes: 1024, ReadCoverArt: true, ImplicitConversions: true}

// testAsset returns the path to a lofty-rs test asset, reusing the Rust repo's files.
func testAsset(parts ...string) string {
	p := append([]string{"..", "lofty-rs", "lofty", "tests", "files", "assets"}, parts...)
	return filepath.Join(p...)
}

// tempFile copies an asset to a temporary file for write tests.
func tempFile(t *testing.T, assetPath string) *os.File {
	t.Helper()
	data, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset %s: %v", assetPath, err)
	}
	f, err := os.CreateTemp(t.TempDir(), filepath.Base(assetPath))
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	if _, err := f.Write(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("failed to rewind: %v", err)
	}
	return f
}

// readBound reads a tagged file from a path for write tests.
func readBound(t *testing.T, assetPath string) *BoundTaggedFile {
	t.Helper()
	f := tempFile(t, assetPath)
	btf, err := NewProbe(f).
		Options(noProps).
		ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("failed to read %s: %v", assetPath, err)
	}
	return btf
}

// verifyArtist verifies a tag of `tagType` has TrackArtist == expected and
// exactly expectedItemCount items.
func verifyArtist(t *testing.T, file *TaggedFile, tagType TagType, expected string, expectedItemCount int) {
	t.Helper()
	tag := file.Tag(tagType)
	if tag == nil {
		t.Fatalf("expected a %s tag", tagType)
	}
	if tag.ItemCount() != expectedItemCount {
		t.Fatalf("%s: expected %d items, got %d", tagType, expectedItemCount, tag.ItemCount())
	}
	item := tag.Get(ItemKeyTrackArtist)
	if item == nil {
		t.Fatalf("%s: missing TrackArtist", tagType)
	}
	s, ok := item.Value().Text()
	if !ok || s != expected {
		t.Fatalf("%s: expected artist %q, got %q", tagType, expected, s)
	}
}

// setArtist verifies the artist, sets a new value, and saves.
func setArtist(t *testing.T, file *BoundTaggedFile, tagType TagType, expected, newValue string, expectedItemCount int) {
	t.Helper()
	verifyArtist(t, &file.TaggedFile, tagType, expected, expectedItemCount)
	tag := file.Tag(tagType)
	if tag == nil {
		t.Fatalf("expected a %s tag", tagType)
	}
	tag.InsertUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText(newValue)))
	if err := file.Save(NewWriteOptions()); err != nil {
		t.Fatalf("failed to save %s: %v", tagType, err)
	}
}

// --- ported from lofty-rs lofty/tests/files/mpeg.rs ---

func TestMpegRead(t *testing.T) {
	// MP3 file with an ID3v2, ID3v1, and an APEv2 tag
	file, err := NewProbe(mustOpen(t, testAsset("minimal", "full_test.mp3"))).
		Options(noProps).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := file.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tagged.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", tagged.FileType())
	}
	verifyArtist(t, tagged, TagTypeID3v2, "Foo artist", 1)
	verifyArtist(t, tagged, TagTypeID3v1, "Bar artist", 1)
	verifyArtist(t, tagged, TagTypeApe, "Baz artist", 1)
}

func TestMpegReadJunkBetweenFrames(t *testing.T) {
	// ID3v2.3 block followed by four junk bytes (0x20)
	tagged, err := NewProbe(mustOpen(t, testAsset("junk_between_id3_and_mp3.mp3"))).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tf.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", tf.FileType())
	}
	id3v2 := tf.Tags()[0]
	if s, _ := id3v2.GetString(ItemKeyTrackArtist); s != "artist test" {
		t.Fatalf("unexpected artist %q", s)
	}
	if s, _ := id3v2.GetString(ItemKeyAlbumTitle); s != "album test" {
		t.Fatalf("unexpected album %q", s)
	}
	if s, _ := id3v2.GetString(ItemKeyTrackTitle); s != "title test" {
		t.Fatalf("unexpected title %q", s)
	}
	if s, _ := id3v2.GetString(ItemKeyEncoderSettings); s != "Lavf58.62.100" {
		t.Fatalf("unexpected encoder settings %q", s)
	}
	id3v1 := tf.Tags()[1]
	if s, _ := id3v1.GetString(ItemKeyTrackArtist); s != "artist test" {
		t.Fatalf("unexpected id3v1 artist %q", s)
	}
	if s, _ := id3v1.GetString(ItemKeyAlbumTitle); s != "album test" {
		t.Fatalf("unexpected id3v1 album %q", s)
	}
	if s, _ := id3v1.GetString(ItemKeyTrackTitle); s != "title test" {
		t.Fatalf("unexpected id3v1 title %q", s)
	}
}

func TestMpegIssue82SolidusInTag(t *testing.T) {
	tf, err := NewProbe(mustOpen(t, testAsset("issue_82_solidus_in_tag.mp3"))).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := tf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tagged.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", tagged.FileType())
	}
	if s, _ := tagged.Tags()[0].GetString(ItemKeyTrackTitle); s != "Foo / title" {
		t.Fatalf("unexpected title %q", s)
	}
}

func TestMpegIssue87DuplicateId3v2(t *testing.T) {
	tf, err := NewProbe(mustOpen(t, testAsset("issue_87_duplicate_id3v2.mp3"))).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := tf.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tagged.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", tagged.FileType())
	}
	id3v2 := tagged.Tags()[0]
	if s, _ := id3v2.GetString(ItemKeyAlbumTitle); s != "album test" {
		t.Fatalf("unexpected album %q", s)
	}
	// Second tag's artist replaced the first
	if s, _ := id3v2.GetString(ItemKeyTrackArtist); s != "Foo artist" {
		t.Fatalf("unexpected artist %q", s)
	}
	if s, _ := id3v2.GetString(ItemKeyEncoderSettings); s != "Lavf58.62.100" {
		t.Fatalf("unexpected encoder settings %q", s)
	}
	if s, _ := id3v2.GetString(ItemKeyTrackTitle); s != "title test" {
		t.Fatalf("unexpected title %q", s)
	}
}

func TestMpegWrite(t *testing.T) {
	btf := readBound(t, testAsset("minimal", "full_test.mp3"))
	if !btf.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", btf.FileType())
	}
	setArtist(t, btf, TagTypeID3v2, "Foo artist", "Bar artist", 1)
	setArtist(t, btf, TagTypeID3v1, "Bar artist", "Baz artist", 1)
	setArtist(t, btf, TagTypeApe, "Baz artist", "Qux artist", 1)

	// Reread the file
	f := btf.IntoInner()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	btf2, err := BoundTaggedFileFrom(f, noProps)
	if err != nil {
		t.Fatal(err)
	}
	setArtist(t, btf2, TagTypeID3v2, "Bar artist", "Foo artist", 1)
	setArtist(t, btf2, TagTypeID3v1, "Baz artist", "Bar artist", 1)
	setArtist(t, btf2, TagTypeApe, "Qux artist", "Baz artist", 1)
}

func TestMpegSaveToId3v2(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	tagged, err := NewProbe(f).
		Options(noProps).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tf.FileType().Equal(FileTypeMpeg) {
		t.Fatalf("expected Mpeg, got %s", tf.FileType())
	}
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyTrackTitle, "title")

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	tf2, err := NewProbe(f).
		Options(noProps).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged2, err := tf2.Read()
	if err != nil {
		t.Fatal(err)
	}
	tag2 := tagged2.Tag(TagTypeID3v2)
	if tag2 == nil {
		t.Fatal("expected ID3v2 tag")
	}
	if _, ok := tag2.GetString(ItemKeyTrackNumber); ok {
		t.Fatal("expected no track number")
	}
	if _, ok := tag2.GetString(ItemKeyTrackTotal); ok {
		t.Fatal("expected no track total")
	}
}

// --- ported from lofty-rs lofty/tests/files/ape.rs ---

func TestApeRead(t *testing.T) {
	// APE file with an ID3v2, ID3v1, and an APEv2 tag
	tagged, err := NewProbe(mustOpen(t, testAsset("minimal", "full_test.ape"))).
		Options(noProps).
		GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !tf.FileType().Equal(FileTypeApe) {
		t.Fatalf("expected Ape, got %s", tf.FileType())
	}
	verifyArtist(t, tf, TagTypeApe, "Foo artist", 1)
	verifyArtist(t, tf, TagTypeID3v1, "Bar artist", 1)
	verifyArtist(t, tf, TagTypeID3v2, "Baz artist", 1)
}

func TestApeWrite(t *testing.T) {
	btf := readBound(t, testAsset("minimal", "full_test.ape"))
	if !btf.FileType().Equal(FileTypeApe) {
		t.Fatalf("expected Ape, got %s", btf.FileType())
	}
	setArtist(t, btf, TagTypeApe, "Foo artist", "Bar artist", 1)
	setArtist(t, btf, TagTypeID3v1, "Bar artist", "Baz artist", 1)

	f := btf.IntoInner()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	btf2, err := BoundTaggedFileFrom(f, noProps)
	if err != nil {
		t.Fatal(err)
	}
	setArtist(t, btf2, TagTypeApe, "Bar artist", "Foo artist", 1)
	setArtist(t, btf2, TagTypeID3v1, "Baz artist", "Bar artist", 1)
}

// mustOpen opens a test asset for reading.
func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	return f
}

// tests/fuzz/*_read_from.rs — every corpus file must be readable without a
// panic or an unbounded hang (Rust `oom_test` gives each read 3 seconds).
func TestFuzzCorpusReadFrom(t *testing.T) {
	base := filepath.Join("..", "lofty-rs", "lofty", "tests", "fuzz", "assets")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skip("fuzz corpus missing:", err)
	}
	readers := map[string]func(*os.File, ParseOptions) error{
		"aacfile_read_from":     func(f *os.File, po ParseOptions) error { _, e := AacFileReadFrom(f, po); return e },
		"aifffile_read_from":    func(f *os.File, po ParseOptions) error { _, e := AiffFileReadFrom(f, po); return e },
		"apefile_read_from":     func(f *os.File, po ParseOptions) error { _, e := ApeFileReadFrom(f, po); return e },
		"flacfile_read_from":    func(f *os.File, po ParseOptions) error { _, e := FlacFileReadFrom(f, po); return e },
		"mp4file_read_from":     func(f *os.File, po ParseOptions) error { _, e := Mp4FileReadFrom(f, po); return e },
		"mpcfile_read_from":     func(f *os.File, po ParseOptions) error { _, e := MusepackFileReadFrom(f, po); return e },
		"mpegfile_read_from":    func(f *os.File, po ParseOptions) error { _, e := MpegFileReadFrom(f, po); return e },
		"opusfile_read_from":    func(f *os.File, po ParseOptions) error { _, e := OggOpusFileReadFrom(f, po); return e },
		"speexfile_read_from":   func(f *os.File, po ParseOptions) error { _, e := OggSpeexFileReadFrom(f, po); return e },
		"vorbisfile_read_from":  func(f *os.File, po ParseOptions) error { _, e := OggVorbisFileReadFrom(f, po); return e },
		"wavfile_read_from":     func(f *os.File, po ParseOptions) error { _, e := WavFileReadFrom(f, po); return e },
		"wavpackfile_read_from": func(f *os.File, po ParseOptions) error { _, e := WavPackFileReadFrom(f, po); return e },
		"pictureinformation_from_png": func(f *os.File, _ ParseOptions) error {
			data, e := io.ReadAll(f)
			if e != nil {
				return e
			}
			_, e = PictureInformationFromPNG(data)
			return e
		},
		"pictureinformation_from_jpeg": func(f *os.File, _ ParseOptions) error {
			data, e := io.ReadAll(f)
			if e != nil {
				return e
			}
			_, e = PictureInformationFromJPEG(data)
			return e
		},
	}
	count := 0
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		read, ok := readers[dir.Name()]
		if !ok {
			continue // id3v2 corpus: concrete frame parse, covered by bogem
		}
		files, err := os.ReadDir(filepath.Join(base, dir.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, fe := range files {
			path := filepath.Join(base, dir.Name(), fe.Name())
			count++
			done := make(chan any, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- r
						return
					}
					done <- nil
				}()
				f, err := os.Open(path)
				if err != nil {
					done <- err
					return
				}
				defer f.Close()
				_ = read(f, NewParseOptions()) // errors are fine; only panic/hang fail
			}()
			select {
			case r := <-done:
				if r != nil {
					if err, ok := r.(error); ok {
						t.Errorf("%s: %v", path, err)
					} else {
						t.Errorf("%s: panic %v", path, r)
					}
				}
			case <-time.After(3 * time.Second):
				t.Errorf("%s: read did not finish within 3s (oom_test parity)", path)
			}
		}
	}
	if count == 0 {
		t.Skip("fuzz corpus empty")
	}
}
