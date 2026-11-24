package lofty_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"testing"

	. "github.com/subframe7536/lofty-go"
	"github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/flac"
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	"github.com/subframe7536/lofty-go/iff"
	"github.com/subframe7536/lofty-go/lyrics3"
	"github.com/subframe7536/lofty-go/ogg"
	"github.com/subframe7536/lofty-go/vorbis"
)

// A preserved companion tag's items must replay on generic-Tag writes
// (DumpTo and SaveTo), independent of the current global options — Rust's
// TagRef writes mapped items, then the companion.
func TestGenericCompanionWrites(t *testing.T) {
	wo := NewWriteOptions()

	t.Run("VorbisComments", func(t *testing.T) {
		v := vorbis.NewVorbisCommentsTag()
		v.InsertKV("X-AUDIT", "audit-private")
		g := v.Tag()
		g.SetTitle("edited")

		var buf bytes.Buffer
		if err := g.DumpTo(&buf, wo); err != nil {
			t.Fatalf("DumpTo: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("X-AUDIT=audit-private")) {
			t.Fatal("DumpTo dropped the companion item")
		}
		if !bytes.Contains(buf.Bytes(), []byte("edited")) {
			t.Fatal("DumpTo dropped the mapped item")
		}

		data, err := os.ReadFile(testAsset("minimal", "full_test.flac"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(data)
		if err := g.SaveTo(bf, wo); err != nil {
			t.Fatalf("SaveTo: %v", err)
		}
		f, err := flac.FlacFileReadFrom(bytes.NewReader(bf.Bytes()), NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		vc, ok := f.VorbisComments.Get()
		if !ok {
			t.Fatal("expected a VorbisComments tag")
		}
		if got, ok := vc.Get("X-AUDIT"); !ok || got != "audit-private" {
			t.Fatalf("X-AUDIT = (%q, %v), want audit-private", got, ok)
		}
		if got, ok := vc.Get("TITLE"); !ok || got != "edited" {
			t.Fatalf("TITLE = (%q, %v), want edited", got, ok)
		}
	})

	t.Run("RiffInfo", func(t *testing.T) {
		r := &iff.RiffInfoTag{}
		r.InsertKV("IZZZ", "audit-private")
		g := r.Tag()
		g.SetTitle("edited")

		var buf bytes.Buffer
		if err := g.DumpTo(&buf, wo); err != nil {
			t.Fatalf("DumpTo: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("IZZZ")) || !bytes.Contains(buf.Bytes(), []byte("audit-private")) {
			t.Fatal("DumpTo dropped the companion item")
		}
		if !bytes.Contains(buf.Bytes(), []byte("edited")) {
			t.Fatal("DumpTo dropped the mapped item")
		}

		data, err := os.ReadFile(testAsset("minimal", "wav_format_pcm.wav"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(data)
		if err := g.SaveTo(bf, wo); err != nil {
			t.Fatalf("SaveTo: %v", err)
		}
		f, err := iff.WavFileReadFrom(bytes.NewReader(bf.Bytes()), NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		ri, ok := f.RiffInfo.Get()
		if !ok {
			t.Fatal("expected a RiffInfo tag")
		}
		if got, ok := ri.Get("IZZZ"); !ok || got != "audit-private" {
			t.Fatalf("IZZZ = (%q, %v), want audit-private", got, ok)
		}
		if got, ok := ri.Title(); !ok || got != "edited" {
			t.Fatalf("INAM = (%q, %v), want edited", got, ok)
		}
	})

	t.Run("AiffText", func(t *testing.T) {
		want := []iff.AiffComment{{Timestamp: 1, MarkerID: 2, Text: "audit-private"}}
		at := &iff.AiffTextTag{Comments: want}
		g := at.Tag()
		g.SetTitle("edited")

		var buf bytes.Buffer
		if err := g.DumpTo(&buf, wo); err != nil {
			t.Fatalf("DumpTo: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("audit-private")) {
			t.Fatal("DumpTo dropped the companion COMT")
		}
		if !bytes.Contains(buf.Bytes(), []byte("edited")) {
			t.Fatal("DumpTo dropped the mapped item")
		}

		data, err := os.ReadFile(testAsset("minimal", "full_test.aiff"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(data)
		if err := g.SaveTo(bf, wo); err != nil {
			t.Fatalf("SaveTo: %v", err)
		}
		f, err := iff.AiffFileReadFrom(bytes.NewReader(bf.Bytes()), NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tx, ok := f.TextChunks.Get()
		if !ok {
			t.Fatal("expected an AiffText tag")
		}
		if !slices.Equal(tx.Comments, want) {
			t.Fatalf("COMT = %+v, want %+v", tx.Comments, want)
		}
		if got, ok := tx.Name.Get(); !ok || got != "edited" {
			t.Fatalf("NAME = (%q, %v), want edited", got, ok)
		}
	})
}

// Concrete AudioFile.SaveFile must write each tag through the destination
// file's format, not the source's.
func TestConcreteSaveUsesDestination(t *testing.T) {
	wo := NewWriteOptions()

	t.Run("Id3v2ToWav", func(t *testing.T) {
		v2 := id3v2.NewId3v2Tag()
		v2.SetTitle("edited")
		mf := &MpegFile{ID3v2: Some(v2)}

		data, err := os.ReadFile(testAsset("minimal", "wav_format_pcm.wav"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(data)
		if _, err := bf.Seek(int64(len(data)/2), io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := mf.SaveFile(bf, wo); err != nil {
			t.Fatalf("SaveFile: %v", err)
		}
		if !bytes.HasPrefix(bf.Bytes(), []byte("RIFF")) {
			t.Fatal("destination lost its RIFF header")
		}
		wf, err := iff.WavFileReadFrom(bytes.NewReader(bf.Bytes()), NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tag, ok := wf.ID3v2.Get()
		if !ok {
			t.Fatal("expected an ID3v2 chunk in the WAV")
		}
		if got := tag.Title(); got != "edited" {
			t.Fatalf("title = %q, want edited", got)
		}
	})

	t.Run("Id3v1ToWavRejected", func(t *testing.T) {
		mf := &MpegFile{ID3v1: Some(&id3v1.Id3v1Tag{Title: Some("edited")})}

		data, err := os.ReadFile(testAsset("minimal", "wav_format_pcm.wav"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(bytes.Clone(data))
		err = mf.SaveFile(bf, wo)
		var unsupported *UnsupportedTagError
		if !errors.As(err, &unsupported) {
			t.Fatalf("SaveFile err = %v, want UnsupportedTagError", err)
		}
		if !bytes.Equal(bf.Bytes(), data) {
			t.Fatal("SaveFile modified the destination on failure")
		}
	})

	// A concrete FlacFile's VorbisComments must write through the
	// Opus-specific dispatch when saved into an Opus file.
	t.Run("FlacVorbisToOpus", func(t *testing.T) {
		vc := vorbis.NewVorbisCommentsTag()
		vc.InsertKV("TITLE", "edited")
		vc.InsertKV("X-AUDIT", "flac-private")
		src := &FlacFile{VorbisComments: Some(vc)}

		opus, err := os.ReadFile(testAsset("minimal", "full_test.opus"))
		if err != nil {
			t.Fatal(err)
		}
		bf := NewBytesFile(opus)
		if err := src.SaveFile(bf, wo); err != nil {
			t.Fatalf("SaveFile: %v", err)
		}
		of, err := ogg.OggOpusFileReadFrom(bytes.NewReader(bf.Bytes()), NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		ovc, ok := of.VorbisComments.Get()
		if !ok {
			t.Fatal("expected a VorbisComments tag")
		}
		if got, ok := ovc.Get("TITLE"); !ok || got != "edited" {
			t.Fatalf("TITLE = (%q, %v), want edited", got, ok)
		}
		if got, ok := ovc.Get("X-AUDIT"); !ok || got != "flac-private" {
			t.Fatalf("X-AUDIT = (%q, %v), want flac-private", got, ok)
		}
	})
}

// AudioFile.TagTypes lists present tag fields in declaration order;
// AudioFile.Conflicts lists present conflicting pairs.
func TestAudioFileTagQueries(t *testing.T) {
	var full AudioFile = &MpegFile{
		ID3v2:     Some(id3v2.NewId3v2Tag()),
		ID3v1:     Some(&id3v1.Id3v1Tag{}),
		Lyrics3:   Some(&lyrics3.Lyrics3Tag{}),
		Lyrics3v2: Some(&lyrics3.Lyrics3v2Tag{}),
		APE:       Some(ape.NewApeTag()),
	}
	want := []TagType{TagTypeID3v2, TagTypeID3v1, TagTypeLyrics3, TagTypeLyrics3v2, TagTypeApe}
	if got := full.TagTypes(); !slices.Equal(got, want) {
		t.Fatalf("TagTypes = %v, want %v", got, want)
	}
	conflicts := full.Conflicts()
	if len(conflicts) != 1 || conflicts[0] != [2]TagType{TagTypeLyrics3, TagTypeLyrics3v2} {
		t.Fatalf("Conflicts = %v, want [(Lyrics3 Lyrics3v2)]", conflicts)
	}

	var empty AudioFile = &MpegFile{}
	if got := empty.TagTypes(); len(got) != 0 {
		t.Fatalf("empty TagTypes = %v, want none", got)
	}
	if got := empty.Conflicts(); len(got) != 0 {
		t.Fatalf("empty Conflicts = %v, want none", got)
	}

	// An empty tag present by option still reports its type.
	var wav AudioFile = &WavFile{ID3v2: Some(id3v2.NewId3v2Tag())}
	if got := wav.TagTypes(); !slices.Equal(got, []TagType{TagTypeID3v2}) {
		t.Fatalf("TagTypes = %v, want [ID3v2]", got)
	}
	if got := wav.Conflicts(); len(got) != 0 {
		t.Fatalf("Conflicts = %v, want none", got)
	}

	var single AudioFile = &MpegFile{ID3v2: Some(id3v2.NewId3v2Tag())}
	if got := single.Conflicts(); len(got) != 0 {
		t.Fatalf("Conflicts = %v, want none", got)
	}
}
