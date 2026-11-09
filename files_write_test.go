package lofty

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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

// Ported from wav.rs::growing_trailing_id3v2_updates_stream_size — when a
// trailing ID3v2 grows the file, the RIFF chunk size must be updated.
func TestWavGrowingTrailingId3v2UpdatesStreamSize(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "wav_format_pcm.wav"))
	defer f.Close()

	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := removeTag(TagTypeRiffInfo, vf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	tag := btf.Tag(TagTypeID3v2)
	if tag == nil {
		t.Fatal("expected an ID3v2 tag")
	}
	tag.SetArtist(strings.Repeat("A much longer artist name", 100))
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		t.Fatal(err)
	}
	fileLen, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatal(err)
	}
	if got := int64(binary.LittleEndian.Uint32(hdr[4:])) + 8; got != fileLen {
		t.Fatalf("riff size + 8 = %d, want %d", got, fileLen)
	}
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

// Ported from aiff.rs::growing_trailing_id3v2_updates_stream_size — when a
// trailing ID3v2 grows the file, the FORM chunk size must be updated.
func TestAiffGrowingTrailingId3v2UpdatesStreamSize(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.aiff"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("read_bound failed: %v", err)
	}
	tag := btf.Tag(TagTypeID3v2)
	if tag == nil {
		t.Fatal("expected an ID3v2 tag")
	}
	tag.SetArtist(strings.Repeat("A much longer artist name", 100))
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		t.Fatal(err)
	}
	fileLen, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatal(err)
	}
	if got := int64(binary.BigEndian.Uint32(hdr[4:])) + 8; got != fileLen {
		t.Fatalf("form size + 8 = %d, want %d", got, fileLen)
	}
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
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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

	btag := genericTagToBogem(tag, false)
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

// Ported from ogg.rs::flac_remove_id3v2.
func TestFlacRemoveId3v2(t *testing.T) {
	removeTagTest(t, testAsset("flac_with_id3v2.flac"), TagTypeID3v2)
}

// Ported from ogg.rs::flac_try_write_non_empty_id3v2 — ID3v2 is read-only in
// FLAC, so only removal (empty tag writes) is permitted.
func TestFlacTryWriteNonEmptyId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyTrackArtist, "Foo artist")

	data, err := os.ReadFile(testAsset("flac_with_id3v2.flac"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "flac_with_id3v2.flac")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tag.SaveToPath(path, NewWriteOptions()); err == nil {
		t.Fatal("expected an error writing a non-empty ID3v2 tag to a FLAC file")
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

// --- Ports of tests/files/resize.rs FLAC padding tests ---

func flacResizeTag(size int) *Tag {
	tag := NewTag(TagTypeVorbisComments)
	tag.SetTitle(strings.Repeat("Test", size))
	tag.SetArtist(strings.Repeat("Serial-ATA", size))
	tag.SetAlbum(strings.Repeat("Lofty", size))
	tag.SetComment(strings.Repeat("Size test", size))
	return tag
}

// flacMetadataEnd mirrors resize.rs::flac_metadata_end — returns the offset
// after the last metadata block and the sizes of all PADDING blocks.
func flacMetadataEnd(t *testing.T, f *os.File) (int64, []int) {
	t.Helper()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "fLaC" {
		t.Fatal("missing fLaC magic")
	}
	var padding []int
	offset := 4
	for {
		hdr := data[offset : offset+4]
		isLast := hdr[0]&0x80 != 0
		blockType := hdr[0] & 0x7F
		contentLen := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
		if blockType == 1 {
			padding = append(padding, contentLen)
		}
		offset += 4 + contentLen
		if isLast {
			break
		}
	}
	return int64(offset), padding
}

// flacWithLargeTag mirrors resize.rs::flac_with_large_tag — writes a large
// tag with no preferred padding to a copy of stream_info_last.flac.
func flacWithLargeTag(t *testing.T) *os.File {
	t.Helper()
	f := tempFile(t, testAsset("stream_info_last.flac"))
	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := flacResizeTag(1000).SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

// Ported from resize.rs::flac_preferred_padding_is_written_when_metadata_grows
func TestFlacPreferredPaddingWrittenWhenMetadataGrows(t *testing.T) {
	f := tempFile(t, testAsset("stream_info_last.flac"))
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	originalLen := fi.Size()

	wo := NewWriteOptions()
	wo.PreferredPadding = 16
	if err := flacResizeTag(1000).SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	_, padding := flacMetadataEnd(t, f)
	fi, err = f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() <= originalLen {
		t.Fatalf("file len = %d, want > %d", fi.Size(), originalLen)
	}
	sum := 0
	for _, p := range padding {
		sum += p
	}
	if sum < 16 {
		t.Fatalf("padding = %d, want >= 16", sum)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := FlacFileReadFrom(f, NewParseOptions()); err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
}

// Ported from resize.rs::flac_preferred_padding_keeps_a_larger_shrink_gap
func TestFlacPreferredPaddingKeepsLargerShrinkGap(t *testing.T) {
	f := flacWithLargeTag(t)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	originalLen := fi.Size()
	originalMetaEnd, _ := flacMetadataEnd(t, f)
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	wo := NewWriteOptions()
	wo.PreferredPadding = 16
	if err := flacResizeTag(1).SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	metaEnd, padding := flacMetadataEnd(t, f)
	fi, err = f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != originalLen {
		t.Fatalf("file len = %d, want unchanged %d", fi.Size(), originalLen)
	}
	if metaEnd != originalMetaEnd {
		t.Fatalf("metadata end = %d, want %d", metaEnd, originalMetaEnd)
	}
	sum := 0
	for _, p := range padding {
		sum += p
	}
	if sum <= 16 {
		t.Fatalf("padding = %d, want > 16", sum)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := FlacFileReadFrom(f, NewParseOptions()); err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
}

// Ported from resize.rs::flac_preferred_padding_grows_when_shrink_gap_is_smaller
func TestFlacPreferredPaddingGrowsWhenShrinkGapIsSmaller(t *testing.T) {
	f := flacWithLargeTag(t)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	originalLen := fi.Size()

	wo := NewWriteOptions()
	wo.PreferredPadding = 100000
	if err := flacResizeTag(1).SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	_, padding := flacMetadataEnd(t, f)
	fi, err = f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() <= originalLen {
		t.Fatalf("file len = %d, want > %d", fi.Size(), originalLen)
	}
	sum := 0
	for _, p := range padding {
		sum += p
	}
	if sum < 100000 {
		t.Fatalf("padding = %d, want >= 100000", sum)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := FlacFileReadFrom(f, NewParseOptions()); err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
}

// Ported from resize.rs::flac_without_preferred_padding_still_shrinks
func TestFlacWithoutPreferredPaddingStillShrinks(t *testing.T) {
	f := flacWithLargeTag(t)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	originalLen := fi.Size()

	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := flacResizeTag(1).SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	fi, err = f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() >= originalLen {
		t.Fatalf("file len = %d, want < %d", fi.Size(), originalLen)
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

// Ported from ogg.rs::opus_issue_499 — TRACKNUMBER=11/22 (<current>/<total>).
func TestOpusIssue499(t *testing.T) {
	f, err := os.Open(testAsset("issue_499.opus"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	of, err := OggOpusFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	vc, ok := of.VorbisComments.Get()
	if !ok {
		t.Fatal("no vorbis comments")
	}
	if track, ok := vc.Track(); !ok || track != 11 {
		t.Fatalf("track = %v (ok=%v), want 11", track, ok)
	}
	if total, ok := vc.TrackTotal(); !ok || total != 22 {
		t.Fatalf("track_total = %v (ok=%v), want 22", total, ok)
	}
}

// Ported from ogg.rs::opus_issue_499_vinyl_track_number — TRACKNUMBER=a5
// (vinyl format) is left for the caller, not parsed as a number.
func TestOpusIssue499VinylTrackNumber(t *testing.T) {
	f, err := os.Open(testAsset("issue_499_vinyl_track_number.opus"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	of, err := OggOpusFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	vc, ok := of.VorbisComments.Get()
	if !ok {
		t.Fatal("no vorbis comments")
	}
	if track, ok := vc.Track(); ok {
		t.Fatalf("track = %v, want None for vinyl track number", track)
	}
	if s, ok := vc.Get("TRACKNUMBER"); !ok || s != "a5" {
		t.Fatalf("TRACKNUMBER = %q (ok=%v), want a5", s, ok)
	}
}

// Ported from tag/tests.rs::issue_130_huge_picture — an 81KB picture surpasses
// the max Ogg page size; the comment packet must continue across pages and the
// file must stay parseable. Rust also validates with `opusinfo`; we assert the
// round-trip survives instead.
func TestOpusIssue130HugePicture(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.opus"))
	defer f.Close()

	data, err := os.ReadFile(testAsset("issue_37.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	picture, err := PictureFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	picture.SetPicType(PictureTypeCoverFront)

	tag := NewTag(TagTypeVorbisComments)
	tag.PushPicture(*picture)
	if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	of, err := OggOpusFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatalf("file no longer parseable after huge picture write: %v", err)
	}
	vc, ok := of.VorbisComments.Get()
	if !ok {
		t.Fatal("no vorbis comments")
	}
	pics := vc.Pictures()
	if len(pics) != 1 {
		t.Fatalf("picture count = %d, want 1", len(pics))
	}
	if !bytes.Equal(pics[0].Picture.Data(), data) {
		t.Fatal("picture data does not match")
	}
}

// --- Lyrics3v2 write (ported loosely from id3/lyrics3/v2/tests.rs) ---

func TestLyrics3v2Write(t *testing.T) {
	// full_test.mp3 contains an ID3v1 tag, required for Lyrics3v2
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "la la la")
	tag.InsertText(ItemKeyTrackTitle, "title test")
	if err := Lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := f.Seek(-128, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	_, _, foundTag, ok, err := findLyrics3(f, true, NewParseOptions())
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

	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "la la la")
	if err := Lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// Remove by writing an empty tag
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf2, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := Lyrics3v2WriteTo(vf2.File(), &Lyrics3v2Tag{}, NewWriteOptions()); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, err := f.Seek(-128, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	_, _, _, ok, err := findLyrics3(f, false, NewParseOptions())
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
		verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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
		verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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
	packets, _, _, err := OggReadCountPackets(f, 3)
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
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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

	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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
	tag.PushPicture(PictureUnchecked([]byte{}).PicType(PictureTypeCoverFront).Build())

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verified, err := NewVerifiedFile(NewOSFile(f), noProps)
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

// Ported from mpeg.rs::remove_lyrics3 / remove_lyrics3v2.
func TestMpegRemoveLyrics3(t *testing.T) {
	removeTagTest(t, testAsset("lyrics3.mp3"), TagTypeLyrics3)
}

func TestMpegRemoveLyrics3v2(t *testing.T) {
	removeTagTest(t, testAsset("lyrics3v2.mp3"), TagTypeLyrics3v2)
}

// Ported from mpeg.rs::lyrics3_tags_conflict — Lyrics3 and Lyrics3v2 cannot
// co-exist; saving a TaggedFile containing both must fail.
func TestLyrics3TagsConflict(t *testing.T) {
	conflictErr := func(t *testing.T, f *os.File, have, insert TagType) {
		t.Helper()
		btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
		if err != nil {
			t.Fatal(err)
		}
		if !btf.ContainsTag(have) {
			t.Fatalf("expected a %s tag", have)
		}
		btf.InsertTag(NewTag(insert))
		err = btf.Save(NewWriteOptions())
		if err == nil {
			t.Fatalf("expected a conflicting-tag error saving %s + %s", have, insert)
		}
		var cte *ConflictingTagError
		if !errors.As(err, &cte) && !strings.Contains(err.Error(), "conflicts with") {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	f := tempFile(t, testAsset("lyrics3v2.mp3"))
	defer f.Close()
	conflictErr(t, f, TagTypeLyrics3v2, TagTypeLyrics3)

	f2 := tempFile(t, testAsset("lyrics3.mp3"))
	defer f2.Close()
	conflictErr(t, f2, TagTypeLyrics3, TagTypeLyrics3v2)
}

// Ported from mpeg.rs::lyrics3_requires_id3v1 — a Lyrics3 tag can only be
// saved to a file that contains an ID3v1 tag.
func TestLyrics3RequiresId3v1(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()
	vf := NewOSFile(f)
	lyrics := &Lyrics3Tag{Lyrics: "[00:00]Serial-ATA was here"}

	// Has an ID3v1 tag, should work
	if err := Lyrics3v1WriteTo(vf, lyrics, NewWriteOptions()); err != nil {
		t.Fatalf("save with ID3v1 present: %v", err)
	}

	if err := TagTypeID3v1.RemoveFrom(vf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	err := Lyrics3v1WriteTo(vf, lyrics, NewWriteOptions())
	var l3err *Lyrics3ParseError
	if !errors.As(err, &l3err) || l3err.Message != "the destination file must contain an ID3v1 tag" {
		t.Fatalf("expected ID3v1-required error, got %v", err)
	}
}

// Ported from mpeg.rs::save_number_of_track_and_disk_to_id3v2 /
// save_total_of_track_and_disk_to_id3v2 / save_number_pair_of_track_and_disk_to_id3v2.
func TestMpegSaveTrackAndDiskToId3v2(t *testing.T) {
	reread := func(t *testing.T, f *os.File) *Tag {
		t.Helper()
		vf := NewOSFile(f)
		if _, err := vf.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		btf, err := NewProbe(vf).Options(noProps).ReadBound(NewOSFile(f))
		if err != nil {
			t.Fatal(err)
		}
		tag := btf.Tag(TagTypeID3v2)
		if tag == nil {
			t.Fatal("no id3v2 tag")
		}
		return tag
	}
	assertNum := func(get func() (uint32, bool), name string, want uint32, wantOk bool) {
		got, ok := get()
		if ok != wantOk || (ok && got != want) {
			t.Errorf("%s = %v (ok=%v), want %v (ok=%v)", name, got, ok, want, wantOk)
		}
	}

	t.Run("number only", func(t *testing.T) {
		f := tempFile(t, testAsset("minimal", "full_test.mp3"))
		defer f.Close()
		tag := NewTag(TagTypeID3v2)
		tag.SetTrack(1)
		tag.SetDisk(2)
		if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
		tag = reread(t, f)
		assertNum(tag.Track, "track", 1, true)
		assertNum(tag.TrackTotal, "track_total", 0, false)
		assertNum(tag.Disk, "disk", 2, true)
		assertNum(tag.DiskTotal, "disk_total", 0, false)
	})

	t.Run("total only", func(t *testing.T) {
		f := tempFile(t, testAsset("minimal", "full_test.mp3"))
		defer f.Close()
		tag := NewTag(TagTypeID3v2)
		tag.SetTrackTotal(2)
		tag.SetDiskTotal(3)
		if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
		tag = reread(t, f)
		// Rust writes "0/total" when only a total exists
		assertNum(tag.Track, "track", 0, true)
		assertNum(tag.TrackTotal, "track_total", 2, true)
		assertNum(tag.Disk, "disk", 0, true)
		assertNum(tag.DiskTotal, "disk_total", 3, true)
	})

	t.Run("number pair", func(t *testing.T) {
		f := tempFile(t, testAsset("minimal", "full_test.mp3"))
		defer f.Close()
		tag := NewTag(TagTypeID3v2)
		tag.SetTrack(1)
		tag.SetTrackTotal(2)
		tag.SetDisk(3)
		tag.SetDiskTotal(4)
		if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
		tag = reread(t, f)
		assertNum(tag.Track, "track", 1, true)
		assertNum(tag.TrackTotal, "track_total", 2, true)
		assertNum(tag.Disk, "disk", 3, true)
		assertNum(tag.DiskTotal, "disk_total", 4, true)
	})
}

// Ported from mpeg.rs::test_bound_tagged_into_inner — a BoundTaggedFile's
// tags are editable in place, saved, and the underlying FileLike recovered
// with IntoInner.
func TestBoundTaggedIntoInner(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatal(err)
	}
	tag := btf.Tag(TagTypeID3v2)
	if tag == nil {
		t.Fatal("couldn't get ref to tag")
	}
	tag.SetDisk(123)
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	// Reread the file
	inner := btf.IntoInner()
	if _, err := inner.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	btf, err = NewProbe(inner).Options(noProps).ReadBound(inner)
	if err != nil {
		t.Fatal(err)
	}
	tag = btf.Tag(TagTypeID3v2)
	if tag == nil {
		t.Fatal("no id3v2 tag")
	}
	if disk, ok := tag.Disk(); !ok || disk != 123 {
		t.Fatalf("disk = %v (ok=%v), want 123", disk, ok)
	}
}

// Ported from mpeg.rs::remove_id3v2_by_path (issue #750).
func TestMpegRemoveId3v2ByPath(t *testing.T) {
	data, err := os.ReadFile(testAsset("minimal", "full_test.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(t.TempDir(), "named-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatal(err)
	}
	if !btf.ContainsTag(TagTypeID3v2) {
		t.Fatal("expected an id3v2 tag")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	if err := TagTypeID3v2.RemoveFromPath(path, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatal(err)
	}
	if btf.ContainsTag(TagTypeID3v2) {
		t.Fatal("id3v2 tag should be removed")
	}
}

// Ported from mpeg.rs::stripping_id3v1_removes_lyrics3v2 — Lyrics3v2 cannot
// exist without ID3v1.
func TestStrippingId3v1RemovesLyrics3v2(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	// Write a Lyrics3v2 tag (requires the existing ID3v1)
	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "[00:00]Serial-ATA was here")
	if err := Lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("lyrics3 write failed: %v", err)
	}

	// Removing ID3v1 must strip the Lyrics3v2 tag too
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err = NewVerifiedFile(NewOSFile(f), NewParseOptions())
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
	if mf.Lyrics3v2.IsSome() {
		t.Fatal("lyrics3v2 still present after id3v1 removal")
	}
	if mf.ID3v1.IsSome() {
		t.Fatal("id3v1 still present after removal")
	}
}

// Ported from mpeg.rs::stripping_id3v1_removes_lyrics3
func TestStrippingId3v1RemovesLyrics3(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	// Lyrics3 is an extension of ID3v1 and cannot exist alone
	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag := NewTag(TagTypeLyrics3)
	tag.InsertText(ItemKeyLyrics, "[00:00]Serial-ATA was here")
	if err := Lyrics3v1WriteTo(vf.File(), Lyrics3TagFromTag(tag), NewWriteOptions()); err != nil {
		t.Fatalf("lyrics3 write failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err = NewVerifiedFile(NewOSFile(f), NewParseOptions())
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

// Ported from mpeg.rs::test_tag_save_with_parse_options — a tag can be saved
// to a file that only parses with non-default ParseOptions.
func TestMpegTagSaveWithParseOptions(t *testing.T) {
	data, err := os.ReadFile(taglibPath("xing.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(t.TempDir(), "xing-prefixed-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(make([]byte, 2048)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	po := NewParseOptions()
	po.MaxJunkBytes = 3000
	if _, err := MpegFileReadFrom(f, po); err != nil {
		t.Fatal(err)
	}

	tag := NewTag(TagTypeID3v2)
	tag.SetTitle("Title B")

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	wo := NewWriteOptions()
	wo.ParseOptions = po
	if err := tag.SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	id3v2, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2 tag")
	}
	if s, _ := id3v2.Title(); s != "Title B" {
		t.Fatalf("title = %q, want Title B", s)
	}
}

// Ported from mpeg.rs::lyrics3v2_requires_id3v1
func TestLyrics3v2RequiresId3v1(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.mp3"))
	defer f.Close()

	// Strip ID3v1 (which also removes any Lyrics3)
	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := removeTag(TagTypeID3v1, vf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	vf, err = NewVerifiedFile(NewOSFile(f), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag := NewTag(TagTypeLyrics3v2)
	tag.InsertText(ItemKeyLyrics, "[00:00]Serial-ATA was here")
	err = Lyrics3v2WriteTo(vf.File(), Lyrics3v2TagFromTag(tag), NewWriteOptions())
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
	vf, err := NewVerifiedFile(NewOSFile(f), noProps)
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

// mp4TestChunkOffsets reads every entry of all `stco`/`co64` atoms reachable
// through the moov container chain.
func mp4TestChunkOffsets(t *testing.T, f *os.File, fourcc string) []uint64 {
	t.Helper()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	var out []uint64
	var walk func(start, end int)
	walk = func(start, end int) {
		for p := start; p+8 <= end; {
			sz := int(binary.BigEndian.Uint32(content[p:]))
			hdr := 8
			if sz == 1 {
				sz = int(binary.BigEndian.Uint64(content[p+8:]))
				hdr = 16
			}
			if sz <= 0 || p+sz > end {
				break
			}
			name := string(content[p+4 : p+8])
			if name == fourcc {
				switch fourcc {
				case "stco":
					for i := 16; i+4 <= sz; i += 4 {
						out = append(out, uint64(binary.BigEndian.Uint32(content[p+i:])))
					}
				case "co64":
					for i := 16; i+8 <= sz; i += 8 {
						out = append(out, binary.BigEndian.Uint64(content[p+i:]))
					}
				}
			}
			switch name {
			case "moov", "trak", "mdia", "minf", "stbl", "udta", "moof":
				walk(p+hdr, p+sz)
			}
			p += sz
		}
	}
	walk(0, len(content))
	return out
}

// When an `ilst` write changes the file length, media data moves and the
// `stco`/`co64` chunk offsets (and `moof`/`tfhd` base offsets) must follow —
// Rust `AtomWriterCompanion::update_offsets`.
func TestMp4ChunkOffsetUpdate(t *testing.T) {
	writeArtist := func(t *testing.T, f *os.File, artist string) int64 {
		t.Helper()
		start, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		vf, err := NewVerifiedFile(NewOSFile(f), noProps)
		if err != nil {
			t.Fatal(err)
		}
		ilst := NewTag(TagTypeMp4Ilst)
		ilst.InsertText(ItemKeyTrackArtist, artist)
		wo := NewWriteOptions()
		wo.PreferredPadding = 0
		if err := writeTag(ilst, vf, wo); err != nil {
			t.Fatal(err)
		}
		end, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			t.Fatal(err)
		}
		return end - start
	}
	rereadArtist := func(t *testing.T, f *os.File, want string) {
		t.Helper()
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		mf, err := Mp4FileReadFrom(f, noProps)
		if err != nil {
			t.Fatal(err)
		}
		tag := mf.TaggedFile().Tag(TagTypeMp4Ilst)
		if tag == nil {
			t.Fatal("no ilst after write")
		}
		if s, ok := tag.GetString(ItemKeyTrackArtist); !ok || s != want {
			t.Fatalf("artist = %q", s)
		}
	}

	t.Run("co64 shifts with mdat", func(t *testing.T) {
		// moov precedes mdat here; creating udta inside moov moves mdat, so
		// every co64 entry must shift by the file growth.
		f := tempFile(t, testAsset("issue_746_extended_mdat.m4a"))
		defer f.Close()
		orig := mp4TestChunkOffsets(t, f, "co64")
		if len(orig) == 0 {
			t.Fatal("no co64 offsets in fixture")
		}
		delta := writeArtist(t, f, "Foo artist with a much longer value to force growth")
		if delta <= 0 {
			t.Fatalf("expected file growth, got %d", delta)
		}
		got := mp4TestChunkOffsets(t, f, "co64")
		if len(got) != len(orig) {
			t.Fatalf("co64 count = %d, want %d", len(got), len(orig))
		}
		for i := range orig {
			if want := orig[i] + uint64(delta); got[i] != want {
				t.Fatalf("co64[%d] = %d, want %d", i, got[i], want)
			}
		}
		rereadArtist(t, f, "Foo artist with a much longer value to force growth")
	})

	t.Run("stco before moov is untouched", func(t *testing.T) {
		// mdat precedes moov here, so a growing ilst inside meta must leave
		// the stco entries pointing at the unmoved media data alone.
		f := tempFile(t, testAsset("non_full_meta_atom.m4a"))
		defer f.Close()
		orig := mp4TestChunkOffsets(t, f, "stco")
		if len(orig) == 0 {
			t.Fatal("no stco offsets in fixture")
		}
		// The fixture's `ilst` sits next to a ~1KB `free` atom — the tag has
		// to outgrow it before the file actually grows.
		artist := strings.Repeat("Foo artist ", 200)
		delta := writeArtist(t, f, artist)
		if delta <= 0 {
			t.Fatalf("expected file growth, got %d", delta)
		}
		got := mp4TestChunkOffsets(t, f, "stco")
		if len(got) != len(orig) {
			t.Fatalf("stco count = %d, want %d", len(got), len(orig))
		}
		for i := range orig {
			if got[i] != orig[i] {
				t.Fatalf("stco[%d] = %d, want %d", i, got[i], orig[i])
			}
		}
		rereadArtist(t, f, artist)
	})
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
	vf, err := NewVerifiedFile(NewOSFile(f), NewParseOptions())
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
