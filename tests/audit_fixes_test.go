package lofty_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	. "github.com/subframe7536/lofty-go"
	"github.com/subframe7536/lofty-go/id3v2"
	"github.com/subframe7536/lofty-go/iff"
	"github.com/subframe7536/lofty-go/lyrics3"
	"github.com/subframe7536/lofty-go/mp4"
)

func TestAuditCoreAndMetaFixes(t *testing.T) {
	// Tag.SetPicture with negative index appends instead of panicking.
	tag := NewTag(TagTypeID3v2)
	pic := PictureUnchecked([]byte{1, 2, 3}).PicType(PictureTypeCoverFront).MimeType(MimeTypePng).Build()
	tag.SetPicture(-1, pic)
	if len(tag.Pictures()) != 1 {
		t.Fatalf("SetPicture(-1): len = %d, want 1", len(tag.Pictures()))
	}

	// Zero FileType string is UNKNOWN, not CUSTOM:.
	var zeroFT FileType
	if got := zeroFT.String(); got != "UNKNOWN" {
		t.Fatalf("FileType{}.String() = %q, want UNKNOWN", got)
	}

	// TimestampParseError with nil Source does not panic.
	tpe := &TimestampParseError{Kind: TimestampParseErrorIo}
	if msg := tpe.Error(); msg == "" {
		t.Fatal("TimestampParseError{Io, nil}.Error() returned empty string")
	}
	tpeField := &TimestampParseError{Kind: TimestampParseErrorFieldTooLarge, Field: TimestampFieldYear}
	if got := tpeField.Error(); got != "field 'Year' exceeds its limits" {
		t.Fatalf("TimestampParseError field msg = %q", got)
	}

	// BytesFile.Write(nil) past EOF must not grow buffer.
	bf := NewBytesFile([]byte{1, 2, 3})
	if _, err := bf.Seek(100, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if n, err := bf.Write(nil); err != nil || n != 0 {
		t.Fatalf("Write(nil) = (%d, %v), want (0, nil)", n, err)
	}
	if len(bf.Bytes()) != 3 {
		t.Fatalf("BytesFile grew to %d on empty write", len(bf.Bytes()))
	}

	// DecodeText rejects invalid encoding.
	if _, err := DecodeText(bytes.NewReader([]byte("abc")), TextDecodeOptions{Encoding: TextEncoding(99)}); err == nil {
		t.Fatal("DecodeText with invalid encoding succeeded, want error")
	}

	// SetCustomRatingProvider(nil) returns false; MusicBeeProvider handles invalid StarRating.
	if SetCustomRatingProvider(nil) {
		t.Fatal("SetCustomRatingProvider(nil) returned true")
	}
	if got := (MusicBeeProvider{}).Rate(TagTypeID3v2, StarRating(0)); got != 0 {
		t.Fatalf("MusicBeeProvider.Rate(ID3v2, 0) = %d, want 0", got)
	}
}

func TestAuditId3v2CompanionAndRemoveKey(t *testing.T) {
	// Companion-only ID3v2 tag (tag.IsEmpty() == true) must still serialize on DumpTo.
	v2 := id3v2.NewId3v2Tag()
	v2.Frames.Insert(id3v2.NewPrivateFrame("com.example.test", []byte{0xDE, 0xAD}))
	generic := v2.Tag()
	if !generic.IsEmpty() {
		t.Fatalf("expected generic tag with only PRIV frame to have IsEmpty() == true")
	}
	var dumped bytes.Buffer
	if err := generic.DumpTo(&dumped, NewWriteOptions()); err != nil {
		t.Fatalf("DumpTo failed: %v", err)
	}
	if dumped.Len() == 0 {
		t.Fatal("DumpTo wrote 0 bytes for companion-only ID3v2 tag")
	}

	// Id3v2TagFromTag must not clear or mutate input's CompanionTag.
	_ = id3v2.Id3v2TagFromTag(generic)
	if generic.CompanionTag() == nil {
		t.Fatal("Id3v2TagFromTag cleared input CompanionTag")
	}

	// Id3v2Tag.RemoveKey must remove TXXX, UFID, TIPL, and ParentalAdvisory frames.
	v2b := id3v2.NewId3v2Tag()
	v2b.Frames.Insert(id3v2.NewExtendedTextFrame(TextEncodingUTF8, "REPLAYGAIN_ALBUM_GAIN", "-5.0 dB"))
	v2b.Frames.Insert(id3v2.NewExtendedTextFrame(TextEncodingUTF8, "ITUNESADVISORY", "1"))
	v2b.Frames.Insert(id3v2.NewUniqueFileIdentifierFrame("http://musicbrainz.org", []byte("abc-123")))
	v2b.Frames.Insert(id3v2.NewKeyValueFrame(id3v2.FrameIdValid("TIPL"), TextEncodingUTF8, [][2]string{{"producer", "Alice"}, {"engineer", "Bob"}}))

	v2b.RemoveKey(ItemKeyReplayGainAlbumGain)
	v2b.RemoveKey(ItemKeyParentalAdvisory)
	v2b.RemoveKey(ItemKeyMusicBrainzRecordingId)
	v2b.RemoveKey(ItemKeyProducer)

	if len(v2b.Frames.Frames()) != 1 {
		t.Fatalf("remaining frames = %d, want 1 (TIPL with engineer)", len(v2b.Frames.Frames()))
	}
	kv, ok := v2b.Frames.Frames()[0].(*id3v2.KeyValueFrame)
	if !ok || len(kv.KeyValuePairs) != 1 || kv.KeyValuePairs[0][0] != "engineer" {
		t.Fatalf("unexpected remaining TIPL pairs: %+v", kv)
	}
}

func TestAuditLyrics3NonDestructiveAndEmptyStrip(t *testing.T) {
	tag := NewTag(TagTypeLyrics3v2)
	tag.SetTitle("Hello")
	tag.InsertText(ItemKeyLyrics, "La la la")

	l1 := lyrics3.Lyrics3v2TagFromTag(tag)
	l2 := lyrics3.Lyrics3v2TagFromTag(tag)
	if v, ok := l1.ExtendedTrackTitle.Get(); !ok || v != "Hello" {
		t.Fatalf("first conversion title = (%q, %v)", v, ok)
	}
	if v, ok := l2.ExtendedTrackTitle.Get(); !ok || v != "Hello" {
		t.Fatalf("second conversion title = (%q, %v) — input tag was mutated", v, ok)
	}

	// Lyrics3v2 tag with only Some("") should encode to empty (strip).
	emptyL := &lyrics3.Lyrics3v2Tag{Lyrics: Some("")}
	var buf bytes.Buffer
	if err := emptyL.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatalf("DumpTo failed: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("DumpTo for Some(\"\") wrote %d bytes, want 0", buf.Len())
	}
}

func TestAuditMp4IlstLocatorIgnored(t *testing.T) {
	tag := NewTag(TagTypeMp4Ilst)
	tag.Push(NewTagItem(ItemKeyTrackTitle, ItemValueLocator("https://example.com")))
	ilst := mp4.IlstFromTag(tag)
	if !ilst.IsEmpty() {
		t.Fatalf("IlstFromTag accepted ItemValueLocator, got %d atoms", ilst.Len())
	}
}

func TestAuditFlacReadOnlyId3v2Rejected(t *testing.T) {
	f := &FlacFile{}
	v2 := id3v2.NewId3v2Tag()
	v2.SetTitle("Nope")
	f.ID3v2 = Some(v2)
	bf := NewBytesFile([]byte("fLaC"))
	err := f.SaveFile(bf, NewWriteOptions())
	var unsupported *UnsupportedTagError
	if !errors.As(err, &unsupported) {
		t.Fatalf("FlacFile.SaveFile with non-empty ID3v2 err = %v, want UnsupportedTagError", err)
	}
}

func TestAuditMpegConflictingLyrics3(t *testing.T) {
	f := &MpegFile{
		Lyrics3:   Some(&lyrics3.Lyrics3Tag{Lyrics: "v1"}),
		Lyrics3v2: Some(&lyrics3.Lyrics3v2Tag{Lyrics: Some("v2")}),
	}
	bf := NewBytesFile(nil)
	err := f.SaveFile(bf, NewWriteOptions())
	var conflict *ConflictingTagError
	if !errors.As(err, &conflict) {
		t.Fatalf("MpegFile.SaveFile with Lyrics3+Lyrics3v2 err = %v, want ConflictingTagError", err)
	}
}

func TestAuditIffSaveFilePreservesFormatSpecific(t *testing.T) {
	// AIFF COMT preserved by AiffFile.SaveFile.
	var aiffBuf bytes.Buffer
	aiffBuf.WriteString("FORM")
	aiffBuf.Write([]byte{0, 0, 0, 30}) // size = 4 ("AIFF") + 8 ("COMM"+size) + 18
	aiffBuf.WriteString("AIFF")
	aiffBuf.WriteString("COMM")
	aiffBuf.Write([]byte{0, 0, 0, 18})
	aiffBuf.Write(make([]byte, 18))
	bf := NewBytesFile(aiffBuf.Bytes())

	af := &AiffFile{
		TextChunks: Some(&iff.AiffTextTag{
			Name:     Some("Title"),
			Comments: []iff.AiffComment{{Timestamp: 1, MarkerID: 2, Text: "kept comment"}},
		}),
	}
	if err := af.SaveFile(bf, NewWriteOptions()); err != nil {
		t.Fatalf("AiffFile.SaveFile failed: %v (unwrap: %v)", err, errors.Unwrap(err))
	}
	if !bytes.Contains(bf.Bytes(), []byte("kept comment")) {
		t.Fatal("AiffFile.SaveFile dropped COMT comment")
	}
}
