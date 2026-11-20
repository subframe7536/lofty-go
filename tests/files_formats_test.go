package lofty_test

import (
	"os"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

// probeRead reads a TaggedFile via Probe+GuessFileType.
func probeRead(t *testing.T, path string, options ParseOptions) *TaggedFile {
	t.Helper()
	p, err := NewProbe(mustOpen(t, path)).Options(options).GuessFileType()
	if err != nil {
		t.Fatalf("failed to guess %s: %v", path, err)
	}
	file, err := p.Read()
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return file
}

// --- ported from lofty-rs lofty/tests/files/flac.rs ---

func TestFlacMultipleVorbisComments(t *testing.T) {
	// Reading a file with multiple VORBIS_COMMENT blocks should error in
	// Strict mode.
	f := mustOpen(t, testAsset("two_vorbis_comments.flac"))
	_, err := FlacFileReadFrom(f, ParseOptions{
		ReadProperties: false,
		ReadTags:       true,
		ParsingMode:    ParsingModeStrict,
	})
	if err == nil {
		t.Fatal("expected an error reading multiple vorbis comments in strict mode")
	}
	f.Close()

	// By default, the last tag in the stream is used.
	f = mustOpen(t, testAsset("two_vorbis_comments.flac"))
	file, err := FlacFileReadFrom(f, ParseOptions{ReadProperties: false, ReadTags: true, ParsingMode: ParsingModeBestAttempt})
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	f.Close()

	tag, ok := file.VorbisComments.Get()
	if !ok {
		t.Fatal("expected a vorbis comment tag")
	}
	if a, ok := tag.Tag().GetString(ItemKeyTrackArtist); !ok || a != "Artist 2" {
		t.Fatalf("expected artist \"Artist 2\", got %q", a)
	}
}

func TestFlacReadNoProperties(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.flac"), ParseOptions{ReadProperties: false})
	if !file.Properties().IsEmpty() {
		t.Fatal("expected empty properties")
	}
}

func TestFlacReadNoTags(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.flac"), ParseOptions{ReadTags: false, ReadProperties: false})
	if len(file.Tags()) != 0 {
		t.Fatalf("expected no tags, got %d", len(file.Tags()))
	}
}

func TestFlacRead(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.flac"), noProps)
	if !file.FileType().Equal(FileTypeFlac) {
		t.Fatalf("expected Flac, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeVorbisComments, "Foo artist", 2)
}

func TestFlacRetainVendorString(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.flac"))
	defer f.Close()
	file, err := FlacFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	tag, ok := file.VorbisComments.Get()
	if !ok {
		t.Fatal("expected a vorbis comment tag")
	}
	if tag.Vendor != "Lavf58.76.100" {
		t.Fatalf("expected vendor \"Lavf58.76.100\", got %q", tag.Vendor)
	}
}

func TestFlacWrite(t *testing.T) {
	f := tempFile(t, testAsset("minimal", "full_test.flac"))
	defer f.Close()
	file, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	setArtist(t, file, TagTypeVorbisComments, "Foo artist", "Bar artist", 2)

	// Reread
	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("failed to reread: %v", err)
	}
	verifyArtist(t, &btf.TaggedFile, TagTypeVorbisComments, "Bar artist", 2)
}

// --- ported from lofty-rs lofty/tests/files/ogg.rs ---

func oggRead(t *testing.T, path string, expected FileType) {
	file := probeRead(t, path, noProps)
	if !file.FileType().Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, file.FileType())
	}
	tag := file.Tag(TagTypeVorbisComments)
	if tag == nil {
		t.Fatal("expected a vorbis comments tag")
	}
	// Expecting 2 items: vendor string (EncoderSoftware) and artist
	verifyArtist(t, file, TagTypeVorbisComments, "Foo artist", 2)
}

func TestOpusRead(t *testing.T) {
	oggRead(t, testAsset("minimal", "full_test.opus"), FileTypeOpus)
}

func TestVorbisRead(t *testing.T) {
	oggRead(t, testAsset("minimal", "full_test.ogg"), FileTypeVorbis)
}

func TestSpeexRead(t *testing.T) {
	oggRead(t, testAsset("minimal", "full_test.spx"), FileTypeSpeex)
}

func oggWrite(t *testing.T, path string) {
	f := tempFile(t, path)
	defer f.Close()
	btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	tag := btf.Tag(TagTypeVorbisComments)
	if tag == nil {
		t.Fatal("expected a vorbis comments tag")
	}
	tag.InsertUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("Foo artist")))
	if err := btf.Save(NewWriteOptions()); err != nil {
		t.Fatalf("failed to save: %v", err)
	}
	// Reread
	btf, err = NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
	if err != nil {
		t.Fatalf("failed to reread: %v", err)
	}
	tag = btf.Tag(TagTypeVorbisComments)
	if tag == nil {
		t.Fatal("expected a vorbis comments tag after write")
	}
	if a, ok := tag.GetString(ItemKeyTrackArtist); !ok || a != "Foo artist" {
		t.Fatalf("expected artist \"Foo artist\", got %q", a)
	}
}

func TestOpusWrite(t *testing.T) {
	oggWrite(t, testAsset("minimal", "full_test.opus"))
}

func TestVorbisWrite(t *testing.T) {
	oggWrite(t, testAsset("minimal", "full_test.ogg"))
}

func TestSpeexWrite(t *testing.T) {
	oggWrite(t, testAsset("minimal", "full_test.spx"))
}

// --- ported from lofty-rs lofty/tests/files/wav.rs ---

func TestWavRead(t *testing.T) {
	file := probeRead(t, testAsset("hound", "pcmwaveformat-16bit-44100Hz-mono.wav"), noProps)
	if !file.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", file.FileType())
	}
}

func TestWavReadTagged(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "wav_format_pcm.wav"), noProps)
	if !file.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", file.FileType())
	}
}

// Ported from mpc.rs::read_sv5 — an MPC file with ID3v2, ID3v1, and APEv2
// tags; each reports its own artist.
func TestMpcReadSv5(t *testing.T) {
	f, err := os.Open(testAsset("minimal", "mpc_sv5.mpc"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	mf, err := MusepackFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tf := mf.TaggedFile()
	if !tf.FileType().Equal(FileTypeMpc) {
		t.Fatalf("expected Mpc, got %s", tf.FileType())
	}
	verifyArtist(t, tf, TagTypeApe, "Foo artist", 1)
	verifyArtist(t, tf, TagTypeID3v1, "Bar artist", 1)
	verifyArtist(t, tf, TagTypeID3v2, "Baz artist", 1)
}

// Ported from wav.rs::empty_id3_chunk_skipped_644 — an `ID3 ` chunk with no
// payload is skipped rather than producing an empty tag.
func TestWavEmptyId3ChunkSkipped(t *testing.T) {
	file := probeRead(t, testAsset("644_empty_id3v2.wav"), NewParseOptions())
	if !file.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", file.FileType())
	}
	if file.Tag(TagTypeID3v2) != nil {
		t.Fatal("expected no ID3v2 tag")
	}
}

// Ported from wav.rs::issue_174_divide_by_zero — a WAVE_FORMAT_EXTENSIBLE
// file with a byte rate of 0 must not panic computing durations.
func TestWavIssue174DivideByZero(t *testing.T) {
	file := probeRead(t, testAsset("issue_174_waveformatextensible-ieeefloat-44100Hz-mono95060.wav"), NewParseOptions())
	if !file.FileType().Equal(FileTypeWav) {
		t.Fatalf("expected Wav, got %s", file.FileType())
	}
}

// --- ported from lofty-rs lofty/tests/files/aiff.rs ---

func TestAiffRead(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.aiff"), noProps)
	if !file.FileType().Equal(FileTypeAiff) {
		t.Fatalf("expected Aiff, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeID3v2, "Foo artist", 1)
	verifyArtist(t, file, TagTypeAiffText, "Bar artist", 1)
}

// Ported from aiff.rs::empty_id3_chunk_skipped_644 — an `ID3 ` chunk with no
// payload is skipped rather than producing an empty tag.
func TestAiffEmptyId3ChunkSkipped(t *testing.T) {
	file := probeRead(t, testAsset("644_empty_id3v2.aiff"), NewParseOptions())
	if !file.FileType().Equal(FileTypeAiff) {
		t.Fatalf("expected Aiff, got %s", file.FileType())
	}
	if file.Tag(TagTypeID3v2) != nil {
		t.Fatal("expected no ID3v2 tag")
	}
}

// --- ported from lofty-rs lofty/tests/files/mp4.rs ---

func TestMp4Read(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "m4a_codec_aac.m4a"), noProps)
	if !file.FileType().Equal(FileTypeMp4) {
		t.Fatalf("expected Mp4, got %s", file.FileType())
	}
	tag := file.Tag(TagTypeMp4Ilst)
	if tag == nil {
		t.Fatal("expected an ilst tag")
	}
}

func TestMp4ReadAlac(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "m4a_codec_alac.m4a"), noProps)
	if !file.FileType().Equal(FileTypeMp4) {
		t.Fatalf("expected Mp4, got %s", file.FileType())
	}
}

// --- ported from lofty-rs lofty/tests/files/aac.rs ---

func TestAacRead(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.aac"), noProps)
	if !file.FileType().Equal(FileTypeAac) {
		t.Fatalf("expected Aac, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeID3v2, "Foo artist", 1)
	verifyArtist(t, file, TagTypeID3v1, "Bar artist", 1)
}

func TestAacReadJunkBetweenFrames(t *testing.T) {
	file := probeRead(t, testAsset("junk_between_id3_and_adts.aac"), NewParseOptions())
	if !file.FileType().Equal(FileTypeAac) {
		t.Fatalf("expected Aac, got %s", file.FileType())
	}
	tags := file.Tags()
	if len(tags) < 2 {
		t.Fatalf("expected at least 2 tags, got %d", len(tags))
	}
	id3v2 := tags[0]
	if a, ok := id3v2.GetString(ItemKeyTrackArtist); !ok || a != "artist test" {
		t.Fatalf("expected artist \"artist test\", got %q", a)
	}
	if a, ok := id3v2.GetString(ItemKeyAlbumTitle); !ok || a != "album test" {
		t.Fatalf("expected album \"album test\", got %q", a)
	}
	if a, ok := id3v2.GetString(ItemKeyTrackTitle); !ok || a != "title test" {
		t.Fatalf("expected title \"title test\", got %q", a)
	}
	if s, _ := id3v2.GetString(ItemKeyEncoderSettings); s != "Lavf58.62.100" {
		t.Fatalf("expected encoder settings \"Lavf58.62.100\", got %q", s)
	}
	id3v1 := tags[1]
	if a, ok := id3v1.GetString(ItemKeyTrackArtist); !ok || a != "artist test" {
		t.Fatalf("expected ID3v1 artist \"artist test\", got %q", a)
	}
	if a, ok := id3v1.GetString(ItemKeyAlbumTitle); !ok || a != "album test" {
		t.Fatalf("expected ID3v1 album \"album test\", got %q", a)
	}
	if a, ok := id3v1.GetString(ItemKeyTrackTitle); !ok || a != "title test" {
		t.Fatalf("expected ID3v1 title \"title test\", got %q", a)
	}
}

// --- ported from lofty-rs lofty/tests/files/dsf.rs ---

func TestDsfRead(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.dsf"), noProps)
	if !file.FileType().Equal(FileTypeDsf) {
		t.Fatalf("expected Dsf, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeID3v2, "Foo artist", 1)
}

// --- ported from lofty-rs lofty/tests/files/mpc.rs ---

func mpcRead(t *testing.T, path string) {
	file := probeRead(t, path, noProps)
	if !file.FileType().Equal(FileTypeMpc) {
		t.Fatalf("expected Mpc, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeApe, "Foo artist", 1)
	verifyArtist(t, file, TagTypeID3v1, "Bar artist", 1)
	verifyArtist(t, file, TagTypeID3v2, "Baz artist", 1)
}

func TestMpcReadSv7(t *testing.T) {
	mpcRead(t, testAsset("minimal", "mpc_sv7.mpc"))
}

func TestMpcReadSv8(t *testing.T) {
	mpcRead(t, testAsset("minimal", "mpc_sv8.mpc"))
}

// --- ported from lofty-rs lofty/tests/files/wavpack.rs ---

func TestWavPackRead(t *testing.T) {
	file := probeRead(t, testAsset("minimal", "full_test.wv"), noProps)
	if !file.FileType().Equal(FileTypeWavPack) {
		t.Fatalf("expected WavPack, got %s", file.FileType())
	}
	verifyArtist(t, file, TagTypeApe, "Foo artist", 1)
	verifyArtist(t, file, TagTypeID3v1, "Bar artist", 1)
}

// --- read_no_properties / read_no_tags variants (files/*.rs) ---

func readNoProps(t *testing.T, asset string) *TaggedFile {
	t.Helper()
	p, err := NewProbe(mustOpen(t, asset)).Options(noProps).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed: %v", err)
	}
	tf, err := p.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if d := tf.Properties().Duration(); d != 0 {
		t.Fatalf("expected zero duration without properties, got %v", d)
	}
	return tf
}

func readNoTags(t *testing.T, asset string) *TaggedFile {
	t.Helper()
	opts := NewParseOptions()
	opts.ReadTags = false
	p, err := NewProbe(mustOpen(t, asset)).Options(opts).GuessFileType()
	if err != nil {
		t.Fatalf("guess failed: %v", err)
	}
	tf, err := p.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(tf.Tags()) != 0 {
		t.Fatalf("expected no tags, got %d", len(tf.Tags()))
	}
	return tf
}

func TestReadNoPropertiesVariants(t *testing.T) {
	for _, asset := range []string{
		"minimal/full_test.aac", "minimal/full_test.aiff", "minimal/full_test.ape",
		"minimal/full_test.dsf", "minimal/full_test.flac", "minimal/full_test.mp3",
		"minimal/m4a_codec_aac.m4a", "minimal/mpc_sv8.mpc", "minimal/full_test.opus",
		"minimal/full_test.ogg", "minimal/full_test.spx", "minimal/wav_format_pcm.wav",
		"minimal/full_test.wv",
	} {
		t.Run(asset, func(t *testing.T) {
			readNoProps(t, testAsset(asset))
		})
	}
}

func TestReadNoTagsVariants(t *testing.T) {
	for _, asset := range []string{
		"minimal/full_test.aac", "minimal/full_test.aiff", "minimal/full_test.ape",
		"minimal/full_test.dsf", "minimal/full_test.flac", "minimal/full_test.mp3",
		"minimal/m4a_codec_aac.m4a", "minimal/mpc_sv8.mpc", "minimal/full_test.opus",
		"minimal/full_test.ogg", "minimal/full_test.spx", "minimal/wav_format_pcm.wav",
		"minimal/full_test.wv",
	} {
		t.Run(asset, func(t *testing.T) {
			readNoTags(t, testAsset(asset))
		})
	}
}
