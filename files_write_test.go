package lofty

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	bogem "github.com/bogem/id3v2/v2"
)

// removeTagTest mirrors Rust util::remove_tag_test: copy the asset, verify the
// tag exists, remove it, then verify it is gone on re-read.
func removeTagTest(t *testing.T, assetPath string, tagType TagType) {
	t.Helper()
	f := tempFile(t, assetPath)
	defer f.Close()

	p, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed: %v", err)
	}
	tagged, err := p.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if tagged.Tag(tagType) == nil {
		t.Fatalf("expected a %s tag before removal", tagType)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := removeTag(tagType, verified, NewWriteOptions()); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	p, err = NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed after remove: %v", err)
	}
	tagged, err = p.Read()
	if err != nil {
		t.Fatalf("read failed after remove: %v", err)
	}
	if tagged.Tag(tagType) != nil {
		t.Fatalf("expected no %s tag after removal", tagType)
	}
}

// writeArtistReread saves a bound file, re-reads it, and verifies the artist.
func writeArtistReread(t *testing.T, f *os.File, tagType TagType, expected, newValue string, count int) {
	t.Helper()
	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	setArtist(t, btf, tagType, expected, newValue, count)
}

// --- ported from lofty-rs lofty/tests/files/wav.rs ---

func TestWavWrite(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "wav_format_pcm.wav"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", btf.FileType())
	}

	// ID3v2
	setArtist(t, btf, TagTypeID3v2, "Foo artist", "Bar artist", 1)
	// RIFF INFO
	setArtist(t, btf, TagTypeRiffInfo, "Bar artist", "Baz artist", 1)

	// Reread
	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	setArtist(t, btf, TagTypeID3v2, "Bar artist", "Foo artist", 1)
	setArtist(t, btf, TagTypeRiffInfo, "Baz artist", "Bar artist", 1)
}

func TestWavRemoveId3v2(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "wav_format_pcm.wav"), TagTypeID3v2)
}

func TestWavRemoveRiffInfo(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "wav_format_pcm.wav"), TagTypeRiffInfo)
}

// --- ported from lofty-rs lofty/tests/files/aiff.rs ---

func TestAiffWrite(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.aiff"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeAiff) {
		t.Fatalf("expected Aiff, got %s", btf.FileType())
	}

	setArtist(t, btf, TagTypeID3v2, "Foo artist", "Bar artist", 1)
	setArtist(t, btf, TagTypeAiffText, "Bar artist", "Baz artist", 1)

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	setArtist(t, btf, TagTypeID3v2, "Bar artist", "Foo artist", 1)
	setArtist(t, btf, TagTypeAiffText, "Baz artist", "Bar artist", 1)
}

func TestAiffRemoveTextChunks(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.aiff"), TagTypeAiffText)
}

func TestAiffRemoveId3v2(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.aiff"), TagTypeID3v2)
}

// --- ported from lofty-rs lofty/tests/files/mp4.rs ---

func TestMp4Write(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "m4a_codec_aac.m4a"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeMp4) {
		t.Fatalf("expected Mp4, got %s", btf.FileType())
	}
	tag := btf.Tag(TagTypeMp4Ilst)
	if tag == nil {
		t.Fatal("expected an ilst tag")
	}
	artist, _ := tag.GetString(ItemKeyTrackArtist)
	tag.InsertUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("Bar artist")))
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Reread
	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	tag = btf.Tag(TagTypeMp4Ilst)
	if tag == nil {
		t.Fatal("expected an ilst tag after write")
	}
	if a, ok := tag.GetString(ItemKeyTrackArtist); !ok || a != "Bar artist" {
		t.Fatalf("expected artist \"Bar artist\" (was %q), got %q", artist, a)
	}
}

func TestMp4RemoveIlst(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "m4a_codec_aac.m4a"), TagTypeMp4Ilst)
}

// --- ported from lofty-rs lofty/tests/files/dsf.rs ---

func TestDsfWrite(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.dsf"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeDsf) {
		t.Fatalf("expected Dsf, got %s", btf.FileType())
	}
	setArtist(t, btf, TagTypeID3v2, "Foo artist", "Bar artist", 1)

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	verifyArtist(t, &btf.TaggedFile, TagTypeID3v2, "Bar artist", 1)
}

func TestDsfRemoveId3v2(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.dsf"), TagTypeID3v2)
}

// --- ported from lofty-rs lofty/tests/files/mpc.rs ---

func mpcWrite(t *testing.T, path string) {
	t.Helper()
	f := tempFile(t, path)
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeMpc) {
		t.Fatalf("expected Mpc, got %s", btf.FileType())
	}
	setArtist(t, btf, TagTypeApe, "Foo artist", "Bar artist", 1)

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	verifyArtist(t, &btf.TaggedFile, TagTypeApe, "Bar artist", 1)
}

func TestMpcWriteSv7(t *testing.T) {
	mpcWrite(t, testAsset("minimal", "mpc_sv7.mpc"))
}

func TestMpcWriteSv8(t *testing.T) {
	mpcWrite(t, testAsset("minimal", "mpc_sv8.mpc"))
}

func TestMpcRemoveId3v2Sv7(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "mpc_sv7.mpc"), TagTypeID3v2)
}

func TestMpcRemoveId3v1Sv7(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "mpc_sv7.mpc"), TagTypeID3v1)
}

func TestMpcRemoveApeSv7(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "mpc_sv7.mpc"), TagTypeApe)
}

// --- ported from lofty-rs lofty/tests/files/wavpack.rs ---

func TestWavPackWrite(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.wv"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	if !btf.FileType().Equal(FileTypeWavPack) {
		t.Fatalf("expected WavPack, got %s", btf.FileType())
	}
	setArtist(t, btf, TagTypeApe, "Foo artist", "Bar artist", 1)

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	verifyArtist(t, &btf.TaggedFile, TagTypeApe, "Bar artist", 1)
}

func TestWavPackRemoveId3v1(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.wv"), TagTypeID3v1)
}

func TestWavPackRemoveApe(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.wv"), TagTypeApe)
}

// --- ported from lofty-rs lofty/tests/files/{flac,ogg}.rs remove tests ---

func TestFlacRemoveVorbisComments(t *testing.T) {
	removeTagTest(t, testAsset("minimal", "full_test.flac"), TagTypeVorbisComments)
}

// For Ogg formats the metadata packet is mandatory: removal leaves only the
// retained vendor string (1 item).
func oggRemoveTest(t *testing.T, assetPath string) {
	t.Helper()
	f := tempFile(t, assetPath)
	defer f.Close()

	p, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed: %v", err)
	}
	tagged, err := p.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if tag := tagged.Tag(TagTypeVorbisComments); tag == nil || tag.ItemCount() != 2 {
		t.Fatalf("expected a 2-item tag before removal")
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := removeTag(TagTypeVorbisComments, verified, NewWriteOptions()); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	p, err = NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed after remove: %v", err)
	}
	tagged, err = p.Read()
	if err != nil {
		t.Fatalf("read failed after remove: %v", err)
	}
	tag := tagged.Tag(TagTypeVorbisComments)
	if tag == nil || tag.ItemCount() != 1 {
		t.Fatalf("expected a vendor-only tag after removal, got %v", tag)
	}
}

func TestOpusRemove(t *testing.T) {
	oggRemoveTest(t, testAsset("minimal", "full_test.opus"))
}

func TestVorbisRemove(t *testing.T) {
	oggRemoveTest(t, testAsset("minimal", "full_test.ogg"))
}

func TestSpeexRemove(t *testing.T) {
	oggRemoveTest(t, testAsset("minimal", "full_test.spx"))
}

// --- ported from lofty-rs lofty/tests/files/zero_sized.rs ---

func zeroAudio(t *testing.T, path string, fTy FileType, readPropsShouldErr bool) {
	t.Helper()
	f := mustOpen(t, path)
	defer f.Close()

	// Reading properties of a zero-size audio stream should error (or produce
	// no meaningful properties); reading without properties must succeed.
	strict := NewParseOptions()
	strict.ParsingMode = ParsingModeStrict
	_, err := readFormat(fTy, f, strict)
	if readPropsShouldErr && err == nil {
		t.Fatalf("%s: expected an error reading properties", path)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	noPropsOpts := ParseOptions{ReadProperties: false, ReadTags: true}
	tf, err := readFormat(fTy, f, noPropsOpts)
	if err != nil {
		t.Fatalf("%s: read without properties failed: %v", path, err)
	}
	_ = tf
}

func TestZeroAudioFlac(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.flac"), FileTypeFlac, false)
}

func TestZeroAudioWav(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.wav"), FileTypeWav, true)
}

func TestZeroAudioAiff(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.aiff"), FileTypeAiff, true)
}

func TestZeroAudioDsf(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.dsf"), FileTypeDsf, true)
}

func TestZeroAudioMp4(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.mp4"), FileTypeMp4, true)
}

func TestZeroAudioMp3(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.mp3"), FileTypeMpeg, true)
}

func TestZeroAudioApe(t *testing.T) {
	zeroAudio(t, testAsset("zero", "zero.ape"), FileTypeApe, true)
}

// Ported from tags/conversions.rs::tag_to_id3v2_lang_frame — UnsyncLyrics and
// Comment land in USLT/COMM frames when converting a generic tag to ID3v2.
func TestTagToId3v2LangFrame(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyUnsyncLyrics, "Test lyrics")
	tag.InsertText(ItemKeyComment, "Test comment")

	btag, err := genericTagToBogem(tag, false)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}
	uslt := btag.GetFrames("USLT")
	if len(uslt) != 1 || uslt[0].(bogem.UnsynchronisedLyricsFrame).Lyrics != "Test lyrics" {
		t.Fatalf("expected one USLT frame with lyrics, got %+v", uslt)
	}
	comm := btag.GetFrames("COMM")
	if len(comm) != 1 || comm[0].(bogem.CommentFrame).Text != "Test comment" {
		t.Fatalf("expected one COMM frame with comment, got %+v", comm)
	}
}

// --- regression tests from non-standard assets ---

func TestMp4IlstTrailingPadding(t *testing.T) {
	file := probeRead(t, testAsset("ilst_trailing_padding.m4a"), noProps)
	if !file.FileType().Equal(FileTypeMp4) {
		t.Fatalf("expected Mp4, got %s", file.FileType())
	}
	if file.Tag(TagTypeMp4Ilst) == nil {
		t.Fatal("expected an ilst tag")
	}
}

func TestMp4NonFullMetaAtom(t *testing.T) {
	// meta atom without the 4-byte fullbox header must still parse
	file := probeRead(t, testAsset("non_full_meta_atom.m4a"), noProps)
	if !file.FileType().Equal(FileTypeMp4) {
		t.Fatalf("expected Mp4, got %s", file.FileType())
	}
	if file.Tag(TagTypeMp4Ilst) == nil {
		t.Fatal("expected an ilst tag")
	}
}

func TestMp4Issue746ExtendedMdat(t *testing.T) {
	// mdat and moov use 64-bit extended sizes; writing a tag must update
	// the largesize fields correctly.
	f := tempFile(t, testAsset("issue_746_extended_mdat.m4a"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	tag := NewTag(TagTypeMp4Ilst)
	tag.InsertText(ItemKeyTrackArtist, "Foo artist")
	btf.InsertTag(tag)
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("reread failed: %v", err)
	}
	verifyArtist(t, &btf.TaggedFile, TagTypeMp4Ilst, "Foo artist", 1)
}

func TestFlacWithId3v2(t *testing.T) {
	file := probeRead(t, testAsset("flac_with_id3v2.flac"), noProps)
	if !file.FileType().Equal(FileTypeFlac) {
		t.Fatalf("expected Flac, got %s", file.FileType())
	}
	if len(file.Tags()) != 2 {
		t.Fatalf("expected 2 tags (vorbis + id3v2), got %d", len(file.Tags()))
	}
}

func TestFlacStreamInfoLast(t *testing.T) {
	// STREAMINFO may appear after other metadata blocks
	file := probeRead(t, testAsset("stream_info_last.flac"), NewParseOptions())
	if !file.FileType().Equal(FileTypeFlac) {
		t.Fatalf("expected Flac, got %s", file.FileType())
	}
	if file.Properties().Duration() <= 0 {
		t.Fatal("expected non-zero duration from trailing STREAMINFO")
	}
}

func TestFlacApplicationBlockLast(t *testing.T) {
	file := probeRead(t, testAsset("application_block_last.flac"), NewParseOptions())
	if !file.FileType().Equal(FileTypeFlac) {
		t.Fatalf("expected Flac, got %s", file.FileType())
	}
}

func TestAacUntagged(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "untagged.aac"), NewParseOptions())
	if !file.FileType().Equal(FileTypeAac) {
		t.Fatalf("expected Aac, got %s", file.FileType())
	}
	if len(file.Tags()) != 0 {
		t.Fatalf("expected no tags, got %d", len(file.Tags()))
	}
}

func TestWavEmptyId3v2(t *testing.T) {
	file := probeRead(t, testAsset("644_empty_id3v2.wav"), noProps)
	if !file.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", file.FileType())
	}
}

func TestOggIssue37(t *testing.T) {
	oggRead(t, testAsset("issue_37.ogg"), FileTypeVorbis)
}

func TestOpusIssue499(t *testing.T) {
	file := probeRead(t, testAsset("issue_499.opus"), noProps)
	if !file.FileType().Equal(FileTypeOpus) {
		t.Fatalf("expected Opus, got %s", file.FileType())
	}
}

// --- Lyrics3v2 write (ported loosely from id3/lyrics3/v2/tests.rs) ---

func TestLyrics3v2Write(t *testing.T) {
	// full_test.mp3 contains an ID3v1 tag, required for Lyrics3v2
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	vf, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "la la la")
	tag.InsertText(ItemKeyTrackTitle, "title test")
	if err := lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := f.Seek(-128, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	_, _, _, foundTag, ok, err := findLyrics3(f, true, NewParseOptions())
	if err != nil || !ok {
		t.Fatalf("lyrics3 not found after write: %v", err)
	}
	g := foundTag.(*Lyrics3v2Tag).Tag()
	if s, ok := g.GetString(ItemKeyLyrics); !ok || s != "la la la" {
		t.Fatalf("expected lyrics \"la la la\", got %q", s)
	}
	if s, ok := g.GetString(ItemKeyTrackTitle); !ok || s != "title test" {
		t.Fatalf("expected title \"title test\", got %q", s)
	}
}

func TestLyrics3v2Remove(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	vf, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "la la la")
	if err := lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// Remove by writing an empty tag
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf2, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := lyrics3v2WriteTo(vf2.File(), &Lyrics3v2Tag{}, NewWriteOptions()); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, err := f.Seek(-128, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, ok, err := findLyrics3(f, false, NewParseOptions())
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if ok {
		t.Fatal("lyrics3 tag still present after removal")
	}
}

// --- resize tests (ported from files/resize.rs, ffmpeg-gated) ---

func resizeTestTag(tagType TagType, size int) *Tag {
	tag := NewTag(tagType)
	tag.InsertText(ItemKeyTrackTitle, strings.Repeat("Test", size))
	tag.InsertText(ItemKeyTrackArtist, strings.Repeat("Serial-ATA", size))
	tag.InsertText(ItemKeyAlbumTitle, strings.Repeat("Lofty", size))
	tag.InsertText(ItemKeyComment, strings.Repeat("Size test", size))
	return tag
}

func ffmpegCheck(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-i", path, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg rejected %s: %v\n%s", path, err, out)
	}
}

func toolInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// tagResizeTest mirrors the Rust helper: remove all tags, write a large tag,
// shrink it, re-grow it — ffmpeg validating each step.
func tagResizeTest(t *testing.T, assetPath string, tagType TagType) {
	t.Helper()
	f := tempFile(t, assetPath)
	defer f.Close()

	// Remove all existing tags (FFmpeg dislikes e.g. ID3v2 on APE files)
	p, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed: %v", err)
	}
	tagged, err := p.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	fTy := tagged.FileType()
	for _, tag := range tagged.Tags() {
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		verified, err := newVerifiedFile(NewOSFile(f), noProps)
		if err != nil {
			t.Fatalf("verify failed: %v", err)
		}
		if err := removeTag(tag.TagType(), verified, NewWriteOptions()); err != nil {
			t.Fatalf("remove %v failed: %v", tag.TagType(), err)
		}
	}

	for _, size := range []int{1000, 1, 1000} {
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		verified, err := newVerifiedFile(NewOSFile(f), noProps)
		if err != nil {
			t.Fatalf("verify failed: %v", err)
		}
		tag := resizeTestTag(tagType, size)
		if err := writeTag(tag, verified, NewWriteOptions()); err != nil {
			t.Fatalf("write %v (size %d) failed: %v", tagType, size, err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		ffmpegCheck(t, f.Name())
	}
	_ = fTy
}

var resizeAssets = map[FileType]string{
	FileTypeAac:     "minimal/full_test.aac",
	FileTypeAiff:    "minimal/full_test.aiff",
	FileTypeApe:     "minimal/full_test.ape",
	FileTypeDsf:     "minimal/full_test.dsf",
	FileTypeFlac:    "minimal/full_test.flac",
	FileTypeMpeg:    "minimal/full_test.mp3",
	FileTypeMp4:     "minimal/m4a_codec_alac.m4a",
	FileTypeMpc:     "minimal/mpc_sv8.mpc",
	FileTypeOpus:    "minimal/full_test.opus",
	FileTypeVorbis:  "minimal/full_test.ogg",
	FileTypeSpeex:   "minimal/full_test.spx",
	FileTypeWav:     "minimal/wav_format_pcm.wav",
	FileTypeWavPack: "minimal/full_test.wv",
}

func TestTagResize(t *testing.T) {
	if !toolInstalled("ffmpeg") {
		t.Skip("ffmpeg not installed")
	}
	for _, tagType := range []TagType{
		TagTypeApe, TagTypeID3v2, TagTypeID3v1,
		TagTypeVorbisComments, TagTypeRiffInfo, TagTypeAiffText,
		TagTypeMp4Ilst,
	} {
		for fTy, asset := range resizeAssets {
			if !fTy.TagSupport(tagType).IsWritable() {
				continue
			}
			t.Run(fTy.String()+"_"+tagType.String(), func(t *testing.T) {
				tagResizeTest(t, testAsset(asset), tagType)
			})
		}
	}
}

// Ported from files/ogg.rs::vorbis_comments_padding_rounds — preferred padding
// rounds the comment packet up to a written-page boundary (8160).
func oggCommentsLen(t *testing.T, f *os.File) int {
	t.Helper()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	packets, _, _, err := oggReadCountPackets(f, 3)
	if err != nil || len(packets) < 2 {
		t.Fatalf("failed to read comment packet: %v", err)
	}
	return len(packets[1])
}

func TestVorbisCommentsPaddingRounds(t *testing.T) {
	src := mustOpen(t, testAsset("minimal", "full_test.ogg"))
	originalLen := oggCommentsLen(t, src)
	src.Close()

	f := tempFile(t, testAsset("minimal", "full_test.ogg"))
	defer f.Close()

	// Read + save with default-ish padding of half a page (4080)
	tagged, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	wo := NewWriteOptions()
	wo.PreferredPadding = 4080
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatal(err)
	}
	tag := tf.Tag(TagTypeVorbisComments)
	if tag == nil {
		t.Fatal("no vorbis tag")
	}
	if err := writeTag(tag, verified, wo); err != nil {
		t.Fatal(err)
	}

	// Delta: the implicit ENCODER comment field ("....ENCODER=Lavf58.76.100")
	const commentDelta = 25
	const actualPadding = 8085
	if got, want := oggCommentsLen(t, f), originalLen+commentDelta+actualPadding; got != want {
		t.Fatalf("comment len = %d, want %d", got, want)
	}
}

func TestVorbisCommentsDisabledPadding(t *testing.T) {
	src := mustOpen(t, testAsset("minimal", "full_test.ogg"))
	originalLen := oggCommentsLen(t, src)
	src.Close()

	f := tempFile(t, testAsset("minimal", "full_test.ogg"))
	defer f.Close()

	tagged, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatal(err)
	}
	tag := tf.Tag(TagTypeVorbisComments)
	if tag == nil {
		t.Fatal("no vorbis tag")
	}
	if err := writeTag(tag, verified, wo); err != nil {
		t.Fatal(err)
	}

	const commentDelta = 25
	if got, want := oggCommentsLen(t, f), originalLen+commentDelta; got != want {
		t.Fatalf("comment len = %d, want %d", got, want)
	}
}

// Ported from wavpack.rs::write_ape_disc_key — "Disc" APE items store "N/T".
func TestWavPackApeDiscKey(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.wv"))
	defer f.Close()

	tag := NewTag(TagTypeApe)
	tag.PushUnchecked(NewTagItem(ItemKeyDiscNumber, ItemValueText("3")))
	tag.PushUnchecked(NewTagItem(ItemKeyDiscTotal, ItemValueText("5")))

	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTag(tag, verified, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	p, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := p.Read()
	if err != nil {
		t.Fatal(err)
	}
	got := tagged.Tag(TagTypeApe)
	if got == nil {
		t.Fatal("no APE tag after write")
	}
	if s, ok := got.GetString(ItemKeyDiscNumber); !ok || s != "3" {
		t.Fatalf("disc number = %q", s)
	}
	if s, ok := got.GetString(ItemKeyDiscTotal); !ok || s != "5" {
		t.Fatalf("disc total = %q", s)
	}
}

// Ported from flac.rs::picture_is_last — a picture can be the final metadata block.
func TestFlacPictureIsLast(t *testing.T) {
	f := tempFile(t, testAsset("stream_info_last.flac"))
	defer f.Close()

	p, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := p.Read()
	if err != nil {
		t.Fatal(err)
	}
	if tagged.Tag(TagTypeVorbisComments) != nil {
		t.Fatal("expected no vorbis comments in stream_info_last.flac")
	}

	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	tag := NewTag(TagTypeVorbisComments)
	tag.PushPicture(Picture{picType: PictureTypeCoverFront, data: []byte{}})

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTag(tag, verified, wo); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	p, err = NewProbe(f).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err = p.Read()
	if err != nil {
		t.Fatal(err)
	}
	if tag := tagged.Tag(TagTypeVorbisComments); tag == nil || len(tag.Pictures()) != 1 {
		t.Fatalf("expected 1 picture, got %v", tag)
	}
}

// Ported from mpeg.rs::stripping_id3v1_removes_lyrics3 — Lyrics3 cannot exist
// without ID3v1.
func TestStrippingId3v1RemovesLyrics3(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	// Write a Lyrics3v2 tag (requires the existing ID3v1)
	vf, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "[00:00]Serial-ATA was here")
	if err := lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("lyrics3 write failed: %v", err)
	}

	// Removing ID3v1 must strip the Lyrics3 tag too
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err = newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := removeTag(TagTypeID3v1, vf, NewWriteOptions()); err != nil {
		t.Fatalf("id3v1 remove failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if mf.Lyrics3.IsSome() {
		t.Fatal("lyrics3 still present after id3v1 removal")
	}
	if mf.ID3v1.IsSome() {
		t.Fatal("id3v1 still present after removal")
	}
}

// Ported from mpeg.rs::lyrics3v2_requires_id3v1
func TestLyrics3v2RequiresId3v1(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	// Strip ID3v1 (which also removes any Lyrics3)
	vf, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := removeTag(TagTypeID3v1, vf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err = newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "[00:00]Serial-ATA was here")
	err = lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions())
	if err == nil {
		t.Fatal("expected an error writing lyrics3v2 without id3v1")
	}
	// The error chain should mention the ID3v1 requirement
	found := false
	for e := err; e != nil; {
		if u, ok := e.(interface{ Unwrap() error }); ok {
			next := u.Unwrap()
			if next == nil {
				break
			}
			if strings.Contains(next.Error(), "ID3v1") {
				found = true
			}
			e = next
		} else {
			break
		}
	}
	if !found && !strings.Contains(err.Error(), "ID3v1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ported from mp4.rs::extended_atoms — moov/mdat keep 64-bit sizes on write.
func TestMp4ExtendedAtoms(t *testing.T) {
	const artist = "Foo artist"
	atomSize := func(f *os.File, fourcc string) uint64 {
		t.Helper()
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		idx := bytes.Index(content, []byte(fourcc))
		if idx < 0 {
			t.Fatalf("atom %s not found", fourcc)
		}
		sz := binary.BigEndian.Uint32(content[idx-4:])
		if sz != 1 {
			t.Fatalf("atom %s should be 64-bit", fourcc)
		}
		return binary.BigEndian.Uint64(content[idx+4:])
	}

	f := tempFile(t, testAsset("issue_746_extended_mdat.m4a"))
	defer f.Close()
	if got := atomSize(f, "mdat"); got != 22508 {
		t.Fatalf("mdat size = %d", got)
	}
	originalMoov := atomSize(f, "moov")

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	tagged, err := NewProbe(f).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := tagged.Read()
	if err != nil {
		t.Fatal(err)
	}
	ilst := NewTag(TagTypeMp4Ilst)
	ilst.InsertText(ItemKeyTrackArtist, artist)
	tf.InsertTag(ilst)

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	vf, err := newVerifiedFile(NewOSFile(f), noProps)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTag(ilst, vf, wo); err != nil {
		t.Fatal(err)
	}

	// udta(8) + meta(12 incl. fullbox) + hdlr(33) + ilst
	ilstSize := uint64(8 + (8 + 16 + len(artist)))
	newMoov := atomSize(f, "moov")
	want := originalMoov + 8 + 12 + 33 + ilstSize
	if newMoov != want {
		t.Fatalf("moov size = %d, want %d", newMoov, want)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	mf, err := Mp4FileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if tag := mf.TaggedFile().Tag(TagTypeMp4Ilst); tag == nil {
		t.Fatal("no ilst after write")
	} else if s, ok := tag.GetString(ItemKeyTrackArtist); !ok || s != artist {
		t.Fatalf("artist = %q", s)
	}
}

// Ported from mpeg.rs::read_and_write_tpil_frame — TIPL role/name pairs
// survive a write round-trip.
func TestMpegTip1Frame(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	var tag *Tag
	if t2, ok := mf.ID3v2.Get(); ok {
		tag = t2
	} else {
		tag = NewTag(TagTypeID3v2)
	}
	tag.InsertText(ItemKeyEngineer, "testperson")
	tag.InsertText(ItemKeyProducer, "testhuman")

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err := newVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTag(tag, vf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	mf, err = MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	rt, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2 after write")
	}
	if s, ok := rt.GetString(ItemKeyEngineer); !ok || s != "testperson" {
		t.Fatalf("engineer = %q", s)
	}
	if s, ok := rt.GetString(ItemKeyProducer); !ok || s != "testhuman" {
		t.Fatalf("vocalist = %q", s)
	}
}
