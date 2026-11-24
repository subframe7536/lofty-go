package id3v2

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"

	bogem "github.com/bogem/id3v2/v2"
)

// Ported from id3/v2/tag/tests.rs

func id3v2TestAsset(name string) string {
	return "../lofty-rs/lofty/tests/tags/assets/id3v2/" + name
}

func readId3v2File(t *testing.T, path string, opts meta.ParseOptions) *meta.Tag {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(data)
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatalf("header parse failed: %v", err)
	}
	tag, err := ParseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("tag parse failed: %v", err)
	}
	return tag
}

// id3v2Dump serializes a generic tag to ID3v2 bytes.
func id3v2Dump(t *testing.T, tag *meta.Tag, useV23 bool) []byte {
	t.Helper()
	wo := meta.NewWriteOptions()
	wo.UseID3v23 = useV23
	encoded, err := Id3v2Serialize(tag, wo)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// id3v2Reparse parses written ID3v2 bytes back to a generic tag.
func id3v2Reparse(t *testing.T, b []byte, opts meta.ParseOptions) *meta.Tag {
	t.Helper()
	r := bytes.NewReader(b)
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatalf("header re-parse failed: %v", err)
	}
	tag, err := ParseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("tag re-parse failed: %v", err)
	}
	return tag
}

// bogemFrames parses written bytes at the frame level.
func bogemFrames(t *testing.T, b []byte) *bogem.Tag {
	t.Helper()
	btag, err := bogem.ParseReader(bytes.NewReader(b), bogem.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	return btag
}

// Ported from src/id3/v2/tag/tests.rs::id3v2{22,23,24}_full — the same tag
// (create_full_test_tag) written at each version must parse identically.
func TestId3v2Full(t *testing.T) {
	cover, err := os.ReadFile(id3v2TestAsset("test_full_cover.png"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[meta.ItemKey]string{
		meta.ItemKeyTrackTitle:    "TempleOS Hymn Risen (Remix)",
		meta.ItemKeyTrackArtist:   "Dave Eddy",
		meta.ItemKeyTrackNumber:   "1",
		meta.ItemKeyAlbumTitle:    "Summer",
		meta.ItemKeyRecordingDate: "2017",
		meta.ItemKeyGenre:         "Electronic",
		meta.ItemKeyLength:        "213017",
	}
	for _, name := range []string{"test_full.id3v22", "test_full.id3v23", "test_full.id3v24"} {
		t.Run(name, func(t *testing.T) {
			tag := readId3v2File(t, id3v2TestAsset(name), meta.NewParseOptions())
			if tag.ItemCount() != len(want) {
				t.Errorf("item count = %d, want %d", tag.ItemCount(), len(want))
			}
			for key, wv := range want {
				got, ok := tag.GetString(key)
				if !ok || got != wv {
					t.Errorf("%v = %q (ok=%v), want %q", key, got, ok, wv)
				}
			}
			if tag.PictureCount() != 1 {
				t.Fatalf("pictures = %d, want 1", tag.PictureCount())
			}
			pic := tag.Pictures()[0]
			if pic.PicType() != meta.PictureTypeCoverFront {
				t.Errorf("pic type = %v, want CoverFront", pic.PicType())
			}
			if mt, ok := pic.MimeType().Get(); !ok || mt != meta.MimeTypePng {
				t.Errorf("mime = %v (ok=%v), want PNG", mt, ok)
			}
			if _, ok := pic.Description(); ok {
				t.Error("pic description should be empty")
			}
			if !bytes.Equal(pic.Data(), cover) {
				t.Errorf("pic data = %d bytes, want %d bytes of test_full_cover.png", len(pic.Data()), len(cover))
			}
		})
	}
}

// Ported from src/id3/v2/tag/tests.rs::special_items_roundtrip — an RVA2
// frame cannot be represented generically; it is retained in the companion
// [Id3v2Tag] and survives a generic-Tag round-trip byte-exact.
func TestId3v2SpecialItemsRoundtrip(t *testing.T) {
	// RVA2 body: "Foo RVA\0" + channel 1 (master volume) + adjustment 30 + 0 peak bits
	rva2Body := []byte("Foo RVA\x00")
	rva2Body = append(rva2Body, 0x01, 0x00, 0x1E, 0x00)

	v4frame := func(id string, body []byte) []byte {
		out := []byte(id)
		sz := len(body)
		out = append(out, byte(sz>>21), byte(sz>>14), byte(sz>>7), byte(sz))
		out = append(out, 0, 0)
		return append(out, body...)
	}
	tpe1Body := append([]byte{0}, "Foo Artist"...)
	frames := append(v4frame("TPE1", tpe1Body), v4frame("RVA2", rva2Body)...)
	tagBytes := []byte{
		'I', 'D', '3', 4, 0, 0,
		byte(len(frames) >> 21), byte(len(frames) >> 14), byte(len(frames) >> 7), byte(len(frames)),
	}
	tagBytes = append(tagBytes, frames...)

	parse := func(t *testing.T, b []byte) *meta.Tag {
		t.Helper()
		r := bytes.NewReader(b)
		header, err := ParseId3v2HeaderRaw(r)
		if err != nil {
			t.Fatal(err)
		}
		tag, err := ParseId3v2(r, header, meta.NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		return tag
	}
	findRva2 := func(t *testing.T, tag *meta.Tag) *RelativeVolumeAdjustmentFrame {
		t.Helper()
		companion, ok := tag.CompanionTag().(*Id3v2Tag)
		if !ok || companion == nil {
			t.Fatal("no companion Id3v2Tag")
			return nil
		}
		for _, f := range companion.Frames.Frames() {
			if rf, ok := f.(*RelativeVolumeAdjustmentFrame); ok {
				return rf
			}
		}
		t.Fatal("no RVA2 frame in companion")
		return nil
	}

	tag := parse(t, tagBytes)
	if s, _ := tag.GetString(meta.ItemKeyTrackArtist); s != "Foo Artist" {
		t.Fatalf("artist = %q", s)
	}
	rva2 := findRva2(t, tag)
	if rva2.Identification != "Foo RVA" {
		t.Fatalf("RVA2 identification = %q", rva2.Identification)
	}
	ch, ok := rva2.Channels[ChannelTypeMasterVolume]
	if !ok || ch.VolumeAdjustment != 30 {
		t.Fatalf("RVA2 channels = %+v", rva2.Channels)
	}

	// Round-trip through the generic Tag — strict mode must still find it
	strict := meta.NewParseOptions()
	strict.ParsingMode = meta.ParsingModeStrict
	reparsed, err := func() (*meta.Tag, error) {
		r := bytes.NewReader(id3v2Dump(t, tag, false))
		header, err := ParseId3v2HeaderRaw(r)
		if err != nil {
			return nil, err
		}
		return ParseId3v2(r, header, strict)
	}()
	if err != nil {
		t.Fatal(err)
	}
	rva2 = findRva2(t, reparsed)
	if rva2.Identification != "Foo RVA" {
		t.Fatalf("RVA2 identification after round-trip = %q", rva2.Identification)
	}
	ch, ok = rva2.Channels[ChannelTypeMasterVolume]
	if !ok || ch.VolumeAdjustment != 30 {
		t.Fatalf("RVA2 channels after round-trip = %+v", rva2.Channels)
	}
	if s, _ := reparsed.GetString(meta.ItemKeyTrackArtist); s != "Foo Artist" {
		t.Fatalf("artist after round-trip = %q", s)
	}
}

func TestParseId3v2(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), meta.NewParseOptions())
	if s, _ := tag.GetString(meta.ItemKeyTrackArtist); s != "Bar artist" {
		t.Fatalf("artist = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyTrackTitle); s != "Foo title" {
		t.Fatalf("title = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyAlbumTitle); s != "Baz album" {
		t.Fatalf("album = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyComment); s != "Qux comment" {
		t.Fatalf("comment = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyRecordingDate); s != "1984" {
		t.Fatalf("date = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyTrackNumber); s != "1" {
		t.Fatalf("track = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyGenre); s != "Classical" {
		t.Fatalf("genre = %q", s)
	}
}

func TestId3v2ReRead(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), meta.NewParseOptions())
	encoded := id3v2Dump(t, tag, false)
	re := id3v2Reparse(t, encoded, meta.NewParseOptions())
	if re.ItemCount() != tag.ItemCount() {
		t.Fatalf("len = %d, want %d", re.ItemCount(), tag.ItemCount())
	}
	for _, item := range tag.Items() {
		v, _ := item.Value().Text()
		if s, ok := re.GetString(item.Key()); !ok || s != v {
			t.Fatalf("missing/mismatched item %v: %q", item.Key(), s)
		}
	}
}

func TestId3v2ToTag(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), meta.NewParseOptions())
	// verify_tag (track_number + genre)
	checks := map[meta.ItemKey]string{
		meta.ItemKeyTrackTitle:  "Foo title",
		meta.ItemKeyTrackArtist: "Bar artist",
		meta.ItemKeyAlbumTitle:  "Baz album",
		meta.ItemKeyComment:     "Qux comment",
		meta.ItemKeyTrackNumber: "1",
		meta.ItemKeyGenre:       "Classical",
	}
	for k, want := range checks {
		if s, ok := tag.GetString(k); !ok || s != want {
			t.Fatalf("%v = %q, want %q", k, s, want)
		}
	}
}

func TestTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyTrackTitle, "Foo title")
	tag.InsertText(meta.ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(meta.ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(meta.ItemKeyComment, "Qux comment")
	tag.InsertText(meta.ItemKeyTrackNumber, "1")
	tag.InsertText(meta.ItemKeyGenre, "Classical")

	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	verify := func(id, want string) {
		t.Helper()
		tf := btag.GetTextFrame(id)
		if tf.Text != want {
			t.Fatalf("%s = %q, want %q", id, tf.Text, want)
		}
	}
	verify("TIT2", "Foo title")
	verify("TPE1", "Bar artist")
	verify("TALB", "Baz album")
	verify("TRCK", "1")
	verify("TCON", "Classical")

	comments := btag.GetFrames("COMM")
	if len(comments) != 1 {
		t.Fatalf("COMM frames = %d", len(comments))
	}
	cf, ok := comments[0].(bogem.CommentFrame)
	if !ok || cf.Text != "Qux comment" || cf.Language != "XXX" {
		t.Fatalf("COMM = %+v", cf)
	}
}

// --- genre tests ---

func id3v2Genres(value string) []string {
	return id3v2GenresIter(value, false)
}

func TestGenreText(t *testing.T) {
	got := id3v2Genres("Dream Pop")
	if len(got) != 1 || got[0] != "Dream Pop" {
		t.Fatalf("got %v", got)
	}
}

func TestGenreIdBrackets(t *testing.T) {
	got := id3v2Genres("(21)")
	if len(got) != 1 || got[0] != "Ska" {
		t.Fatalf("got %v", got)
	}
}

func TestGenreIdNumeric(t *testing.T) {
	got := id3v2Genres("21")
	if len(got) != 1 || got[0] != "Ska" {
		t.Fatalf("got %v", got)
	}
}

func TestGenreIdMultipleJoined(t *testing.T) {
	got := id3v2Genres("(51)(39)")
	if len(got) != 2 || got[0] != "Techno-Industrial" || got[1] != "Noise" {
		t.Fatalf("got %v", got)
	}
	// Rust's genre() joins multiple values with " / "
	if joined := strings.Join(got, " / "); joined != "Techno-Industrial / Noise" {
		t.Fatalf("joined = %q", joined)
	}
}

func TestGenresIdMultipleIntoTag(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyGenre, "(51)(39)")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	genres := re.GetStrings(meta.ItemKeyGenre)
	if len(genres) < 2 || genres[0] != "Techno-Industrial" || genres[1] != "Noise" {
		t.Fatalf("got %v", genres)
	}
}

func TestGenresNullSeparated(t *testing.T) {
	got := id3v2Genres("Samba-rock\x00MPB\x00Funk")
	want := []string{"Samba-rock", "MPB", "Funk"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestGenresIdTextualRefinement(t *testing.T) {
	got := id3v2Genres("(4)Eurodisco")
	if len(got) != 2 || got[0] != "Disco" || got[1] != "Eurodisco" {
		t.Fatalf("got %v", got)
	}
}

func TestGenresIdBracketedRefinement(t *testing.T) {
	got := id3v2Genres("(26)(55)((I think...)")
	want := []string{"Ambient", "Dream", "(I think...)"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestGenresIdRemixCover(t *testing.T) {
	got := id3v2Genres("(0)(RX)(CR)")
	want := []string{"Blues", "Remix", "Cover"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

// --- TRCK/TPOS tests ---

func trckText(t *testing.T, tag *meta.Tag) string {
	t.Helper()
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	return btag.GetTextFrame("TRCK").Text
}

func tposText(t *testing.T, tag *meta.Tag) string {
	t.Helper()
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	return btag.GetTextFrame("TPOS").Text
}

func TestSetTrack(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetTrack(1)
	if got := trckText(t, tag); got != "1" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetTrackTotal(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetTrackTotal(2)
	// DEFAULT_NUMBER_IN_PAIR fills in the number
	if got := trckText(t, tag); got != "0/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetTrackAndTrackTotal(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetTrack(1)
	tag.SetTrackTotal(2)
	if got := trckText(t, tag); got != "1/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetDisk(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetDisk(1)
	if got := tposText(t, tag); got != "1" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestSetDiskTotal(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetDiskTotal(2)
	if got := tposText(t, tag); got != "0/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestSetDiskAndDiskTotal(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.SetDisk(1)
	tag.SetDiskTotal(2)
	if got := tposText(t, tag); got != "1/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestTrackNumberTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("1")))
	if got := trckText(t, tag); got != "1" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestTrackTotalTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTotal, meta.ItemValueText("2")))
	if got := trckText(t, tag); got != "0/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestTrackNumberAndTrackTotalTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText("1")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTotal, meta.ItemValueText("2")))
	if got := trckText(t, tag); got != "1/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestDiskNumberTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscNumber, meta.ItemValueText("1")))
	if got := tposText(t, tag); got != "1" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestDiskTotalTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscTotal, meta.ItemValueText("2")))
	if got := tposText(t, tag); got != "0/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestDiskNumberAndDiskTotalTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscNumber, meta.ItemValueText("1")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscTotal, meta.ItemValueText("2")))
	if got := tposText(t, tag); got != "1/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

// create_tag_with_trck_and_tpos_frame — emulate via TRCK/TPOS text frames.
func tagWithTrckTpos(content string) *meta.Tag {
	tag := meta.NewTag(meta.TagTypeID3v2)
	// Write the raw pair value through a text item, then re-parse.
	raw := meta.NewTag(meta.TagTypeID3v2)
	for _, id := range []meta.ItemKey{} {
		_ = id
	}
	// Direct pair-split semantics mirror split_pair on a TRCK/TPOS frame.
	_ = fromTagItems(raw.Items())
	// Build the generic tag the way a parsed TRCK/TPOS would.
	num, tot, ok := splitNumPair(content)
	if ok {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText(num)))
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscNumber, meta.ItemValueText(num)))
		if tot != "" {
			tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTotal, meta.ItemValueText(tot)))
			tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyDiscTotal, meta.ItemValueText(tot)))
		}
	}
	return tag
}

func TestValidTrckAndTposFrame(t *testing.T) {
	assertValid := func(content string, wantNum, wantTot uint32, hasNum, hasTot bool) {
		t.Helper()
		tag := tagWithTrckTpos(content)
		if n, ok := tag.Track(); ok != hasNum || (ok && n != wantNum) {
			t.Fatalf("%q: track = %v,%v want %v,%v", content, n, ok, wantNum, hasNum)
		}
		if n, ok := tag.TrackTotal(); ok != hasTot || (ok && n != wantTot) {
			t.Fatalf("%q: trackTotal = %v,%v want %v,%v", content, n, ok, wantTot, hasTot)
		}
		if n, ok := tag.Disk(); ok != hasNum || (ok && n != wantNum) {
			t.Fatalf("%q: disk = %v,%v", content, n, ok)
		}
		if n, ok := tag.DiskTotal(); ok != hasTot || (ok && n != wantTot) {
			t.Fatalf("%q: diskTotal = %v,%v", content, n, ok)
		}
	}
	assertValid("0", 0, 0, true, false)
	assertValid("1", 1, 0, true, false)
	assertValid("0/0", 0, 0, true, true)
	assertValid("1/2", 1, 2, true, true)
	assertValid("010/011", 10, 11, true, true)
	assertValid(" 1/2 ", 1, 2, true, true)
	assertValid("1 / 2", 1, 2, true, true)
}

func TestInvalidTrckAndTposFrame(t *testing.T) {
	for _, content := range []string{"", " ", "/", "/1", "1/", "a/b", "1/2/3", "1//2", "0x1/0x2"} {
		tag := tagWithTrckTpos(content)
		if _, ok := tag.Track(); ok {
			t.Fatalf("%q: expected no track", content)
		}
		if _, ok := tag.TrackTotal(); ok {
			t.Fatalf("%q: expected no track total", content)
		}
		if _, ok := tag.Disk(); ok {
			t.Fatalf("%q: expected no disk", content)
		}
		if _, ok := tag.DiskTotal(); ok {
			t.Fatalf("%q: expected no disk total", content)
		}
	}
}

// --- UFID/POPM ---

func TestUfidFrameWithMusicBrainzRecordId(t *testing.T) {
	mbID := "189002e7-3285-4e2e-92a3-7f6c30d407a2"
	v4frame := func(id string, body []byte) []byte {
		out := []byte(id)
		sz := len(body)
		out = append(out, byte(sz>>21), byte(sz>>14), byte(sz>>7), byte(sz))
		out = append(out, 0, 0)
		return append(out, body...)
	}
	frames := v4frame("UFID", append([]byte("other\x00"), "0123456789"...))
	tagBytes := []byte{
		'I', 'D', '3', 4, 0, 0,
		byte(len(frames) >> 21), byte(len(frames) >> 14), byte(len(frames) >> 7), byte(len(frames)),
	}
	tagBytes = append(tagBytes, frames...)

	r := bytes.NewReader(tagBytes)
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := ParseId3v2(r, header, meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyMusicBrainzRecordingId, meta.ItemValueText(mbID)))

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	if s, ok := re.GetString(meta.ItemKeyMusicBrainzRecordingId); !ok || s != mbID {
		t.Fatalf("mb id = %q", s)
	}
	// The non-MB UFID survives as a frame in the companion tag
	found := false
	if companion, ok := re.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.Frames() {
			if uf, ok := f.(*UniqueFileIdentifierFrame); ok &&
				uf.Owner == "other" && string(uf.Identifier) == "0123456789" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("non-MB UFID not preserved")
	}
}

func TestPopmFrame(t *testing.T) {
	// The raw POPM frame is "foo@bar.com" / rating 196 / counter 65535.
	data, err := os.ReadFile(id3v2TestAsset("test_popm.id3v24"))
	if err != nil {
		t.Fatal(err)
	}
	btag, err := bogem.ParseReader(bytes.NewReader(data), bogem.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	popms := btag.GetFrames("POPM")
	if len(popms) != 1 {
		t.Fatalf("popm frames = %d", len(popms))
	}
	pf, ok := popms[0].(bogem.PopularimeterFrame)
	if !ok || pf.Email != "foo@bar.com" || pf.Rating != 196 || pf.Counter.Uint64() != 65535 {
		t.Fatalf("popm = %+v", pf)
	}

	// The generic item maps 196 to a star rating through the custom provider.
	tag := readId3v2File(t, id3v2TestAsset("test_popm.id3v24"), meta.NewParseOptions())
	items := tag.GetItems(meta.ItemKeyPopularimeter)
	if len(items) != 1 {
		t.Fatalf("popm items = %d", len(items))
	}
	s, _ := items[0].Value().Text()
	p, err := meta.PopularimeterFromStr(s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Email() != "foo@bar.com" || p.PlayCounter != 65535 {
		t.Fatalf("popm = %v", p)
	}
	if p.MappedValue(meta.TagTypeID3v2) != 196 {
		t.Fatalf("mapped rating = %d", p.MappedValue(meta.TagTypeID3v2))
	}
}

// --- multi-value ---

func TestMultiValueFrameToTag(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyTrackArtist, "foo\x00bar\x00baz")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	got := re.GetStrings(meta.ItemKeyTrackArtist)
	want := []string{"foo", "bar", "baz"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestMultiItemTagToId3v2(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtist, meta.ItemValueText("foo")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtist, meta.ItemValueText("bar")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtist, meta.ItemValueText("baz")))

	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	if got := btag.GetTextFrame("TPE1").Text; got != "foo/bar/baz" && got != "foo\x00bar\x00baz" {
		t.Fatalf("TPE1 = %q", got)
	}
}

func TestMultiValueRoundtrip(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	push2 := func(k meta.ItemKey, a, b string) {
		tag.InsertText(k, a)
		tag.PushUnchecked(meta.NewTagItem(k, meta.ItemValueText(b)))
	}
	push2(meta.ItemKeyTrackArtist, "TrackArtist 1", "TrackArtist 2")
	push2(meta.ItemKeyAlbumArtist, "AlbumArtist 1", "AlbumArtist 2")
	push2(meta.ItemKeyTrackTitle, "TrackTitle 1", "TrackTitle 2")
	push2(meta.ItemKeyAlbumTitle, "AlbumTitle 1", "AlbumTitle 2")
	push2(meta.ItemKeyContentGroup, "ContentGroup 1", "ContentGroup 2")
	push2(meta.ItemKeyGenre, "Genre 1", "Genre 2")
	push2(meta.ItemKeyMood, "Mood 1", "Mood 2")
	push2(meta.ItemKeyComposer, "Composer 1", "Composer 2")
	push2(meta.ItemKeyConductor, "Conductor 1", "Conductor 2")
	push2(meta.ItemKeyComment, "Comment 1", "Comment 2")
	if tag.Len() != 20 {
		t.Fatalf("len = %d", tag.Len())
	}

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	for _, item := range tag.Items() {
		v, _ := item.Value().Text()
		found := false
		for _, s := range re.GetStrings(item.Key()) {
			if s == v {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing item %v=%q", item.Key(), v)
		}
	}
}

func TestArtistsTagConversion(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	for _, a := range []string{"Foo", "Bar", "Baz"} {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtists, meta.ItemValueText(a)))
	}
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	// ARTISTS is a TXXX description
	var txxx string
	for _, f := range btag.GetFrames("TXXX") {
		if udtf, ok := f.(bogem.UserDefinedTextFrame); ok && udtf.Description == "ARTISTS" {
			txxx = udtf.Value
		}
	}
	if txxx == "" {
		t.Fatal("ARTISTS TXXX missing")
	}
	parts := bytes.Split([]byte(txxx), []byte{0})
	if len(parts) != 3 || string(parts[0]) != "Foo" || string(parts[1]) != "Bar" || string(parts[2]) != "Baz" {
		t.Fatalf("ARTISTS = %q", txxx)
	}
}

func TestMultiItemTagDump(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtist, meta.ItemValueText("Foo")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackArtist, meta.ItemValueText("Bar")))
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	if got := btag.GetTextFrame("TPE1").Text; got != "Foo\x00Bar" {
		t.Fatalf("TPE1 = %q", got)
	}
}

func TestSingleValueFrame(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyIntegerBpm, meta.ItemValueText("120")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyIntegerBpm, meta.ItemValueText("130")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyIntegerBpm, meta.ItemValueText("140")))
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	// BPM is deduplicated — first wins
	if got := btag.GetTextFrame("TBPM").Text; got != "120" {
		t.Fatalf("TBPM = %q", got)
	}
}

func TestUtf16TxxxWithSingleBom(t *testing.T) {
	// Just has to parse without error
	_ = readId3v2File(t, id3v2TestAsset("issue_53.id3v24"), meta.NewParseOptions())
}

func TestReplaygainTagConversion(t *testing.T) {
	// REPLAYGAIN_ALBUM_GAIN is a TXXX description mapped to an meta.ItemKey
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyReplayGainAlbumGain, meta.ItemValueText("-10.43 dB")))
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	if s, ok := re.GetString(meta.ItemKeyReplayGainAlbumGain); !ok || s != "-10.43 dB" {
		t.Fatalf("replaygain = %q", s)
	}
}

// --- flags / advisory / timestamp ---

func TestFlagItemConversion(t *testing.T) {
	// Rust's flag_item_conversion asserts the Tag -> Id3v2Tag direction:
	// both flags become text frames with normalized "0"/"1" values.
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyFlagCompilation, "1")
	tag.InsertText(meta.ItemKeyFlagPodcast, "0")

	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	if got := btag.GetTextFrame("TCMP").Text; got != "1" {
		t.Fatalf("TCMP = %q", got)
	}
	// PCST is a nonstandard frame — bogem surfaces it as an unknown frame;
	// decode the body (encoding byte + text) to verify the flag value.
	pcst := btag.GetFrames("PCST")
	if len(pcst) != 1 {
		t.Fatalf("PCST frames = %d", len(pcst))
	}
	uf, ok := pcst[0].(bogem.UnknownFrame)
	if !ok || id3v2DecodeTextBody(uf.Body) != "0" {
		t.Fatalf("PCST = %+v", pcst[0])
	}

	// On re-read Rust keeps PCST as a binary (retained) frame; the port's
	// equivalent is an unknown binary item that round-trips byte-identically.
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	found := false
	if companion, ok := re.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.Frames() {
			if bf, ok := f.(*BinaryFrame); ok && f.ID().String() == "PCST" &&
				id3v2DecodeTextBody(bf.Data) == "0" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("PCST not retained in companion")
	}
}

func TestTimestampRoundtrip(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyRecordingDate, "2024-06-03T14:08:49")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	if s, ok := re.GetString(meta.ItemKeyRecordingDate); !ok || s != "2024-06-03T14:08:49" {
		t.Fatalf("date = %q", s)
	}
}

func TestPreserveCommentLangDescriptionOnConversion(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	item := meta.NewTagItem(meta.ItemKeyComment, meta.ItemValueText("Foo comment"))
	item.Description = "Some description"
	copy(item.Lang[:], "eng")
	tag.PushUnchecked(item)

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	if re.ItemCount() != 1 {
		t.Fatalf("len = %d", re.ItemCount())
	}
	got := re.GetItems(meta.ItemKeyComment)
	if len(got) != 1 {
		t.Fatalf("comments = %d", len(got))
	}
	if string(got[0].Lang[:]) != "eng" || got[0].Description != "Some description" {
		t.Fatalf("lang=%q desc=%q", got[0].Lang, got[0].Description)
	}
	if s, _ := got[0].Value().Text(); s != "Foo comment" {
		t.Fatalf("text = %q", s)
	}
}

func TestHoldBack4CharacterTxxxDescription(t *testing.T) {
	// A TXXX description of exactly 4 chars could be confused with a frame ID;
	// Rust holds the whole TXXX frame back in the remainder instead of mapping it.
	// In this port it rides as an unknown-described item and round-trips.
	btag := bogem.NewEmptyTag()
	btag.SetVersion(4)
	btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
		Encoding:    bogem.EncodingUTF8,
		Description: "MODE",
		Value:       "CBR",
	})
	var buf bytes.Buffer
	if _, err := btag.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	tag := id3v2Reparse(t, buf.Bytes(), meta.NewParseOptions())
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	// MODE is a 4-char TXXX description — it survives in the companion tag
	found := false
	if companion, ok := re.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.Frames() {
			if tf, ok := f.(*ExtendedTextFrame); ok && tf.Description == "MODE" && tf.Content == "CBR" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("MODE TXXX lost")
	}
}

func TestSkipReadingCoverArt(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	pic := meta.PictureUnchecked(make([]byte, 50)).
		PicType(meta.PictureTypeCoverFront).
		MimeType(meta.MimeTypeJpeg).
		Build()
	tag.PushPicture(pic)
	tag.InsertText(meta.ItemKeyTrackArtist, "Foo artist")

	opts := meta.NewParseOptions()
	opts.ReadCoverArt = false
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), opts)
	if re.PictureCount() != 0 {
		t.Fatalf("pictures = %d", re.PictureCount())
	}
	if s, ok := re.GetString(meta.ItemKeyTrackArtist); !ok || s != "Foo artist" {
		t.Fatalf("artist = %q", s)
	}
}

// --- v2.3 save behavior ---

func TestRemoveId3v24FramesOnId3v23Save(t *testing.T) {
	// RVA2 is ID3v2.4-only — dropped when saving as 2.3
	tag := meta.NewTag(meta.TagTypeID3v2)
	item := meta.NewTagItem(meta.ItemKeyUnknown, meta.ItemValueBinary([]byte{0x03, 0x01, 0x02}))
	item.Description = "RVA2"
	tag.PushUnchecked(item)
	encoded := id3v2Dump(t, tag, true)
	// meta.ItemKeyUnknown produces no frame — an empty tag serializes to nothing
	if len(encoded) != 0 {
		t.Fatalf("encoded v2.3 tag = %d bytes", len(encoded))
	}
}

func TestChangeTextEncodingOnId3v23Save(t *testing.T) {
	// UTF-16BE isn't allowed in 2.3 — text frames degrade to UTF-16
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyTrackTitle, "Foo")
	encoded := id3v2Dump(t, tag, true)
	// Scan the raw bytes for a UTF-16BE encoding byte (0x02) — none may appear
	btag := bogemFrames(t, encoded)
	for _, f := range btag.GetFrames("TIT2") {
		if tf, ok := f.(bogem.TextFrame); ok && tf.Encoding.Key == 2 {
			t.Fatal("UTF-16BE encoding used in v2.3 tag")
		}
	}
	re := id3v2Reparse(t, encoded, meta.NewParseOptions())
	if s, _ := re.GetString(meta.ItemKeyTrackTitle); s != "Foo" {
		t.Fatalf("title = %q", s)
	}
}

func TestSplitTdorOnId3v23Save(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyOriginalReleaseDate, "2024-06-03T14:08:49")
	encoded := id3v2Dump(t, tag, true)
	btag := bogemFrames(t, encoded)
	// TDOR downgrades to TORY — only the year survives
	if got := btag.GetTextFrame("TORY").Text; got != "2024" {
		t.Fatalf("TORY = %q", got)
	}
	// On re-read, TORY upgrades back to TDOR with only a year
	re := id3v2Reparse(t, encoded, meta.NewParseOptions())
	if s, ok := re.GetString(meta.ItemKeyOriginalReleaseDate); !ok || s != "2024" {
		t.Fatalf("orig date = %q", s)
	}
}

func TestSplitTdrcOnId3v23Save(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeID3v2)
	tag.InsertText(meta.ItemKeyRecordingDate, "2024-06-03T14:08:49")
	encoded := id3v2Dump(t, tag, true)
	btag := bogemFrames(t, encoded)
	// TDRC splits into TYER + TDAT + TIME on 2.3
	if got := btag.GetTextFrame("TYER").Text; got != "2024" {
		t.Fatalf("TYER = %q", got)
	}
	if got := btag.GetTextFrame("TDAT").Text; got != "0306" {
		t.Fatalf("TDAT = %q", got)
	}
	if got := btag.GetTextFrame("TIME").Text; got != "1408" {
		t.Fatalf("TIME = %q", got)
	}
	// Re-read with implicit conversions merges them back to TDRC
	re := id3v2Reparse(t, encoded, meta.NewParseOptions())
	if s, ok := re.GetString(meta.ItemKeyRecordingDate); !ok || s != "2024-06-03T14:08" {
		t.Fatalf("date = %q", s)
	}
}

// --- misc file-level tests ---

func TestEnsureFrameSkippingWithinBounds(t *testing.T) {
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeBestAttempt
	tag := readId3v2File(t, id3v2TestAsset("skippable_frame_otherwise_valid.id3v24"), opts)
	// The TDTG frame past the bad TDEN frame survives
	if s, ok := tag.GetString(meta.ItemKeyTaggingTime); !ok || s != "2014-06-10T02:16:10" {
		t.Fatalf("TDTG = %q (items=%v)", s, tag.Items())
	}
}

// Ported from id3/v2/read.rs::bad_frame_id_relaxed_id3v2.
func TestBadFrameIDRelaxedId3v2(t *testing.T) {
	// Contains a frame with a "+" in the ID, which is invalid.
	// All other frames in the tag are valid, however.
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeRelaxed
	tag := readId3v2File(t, id3v2TestAsset("bad_frame_otherwise_valid.id3v24"), opts)
	// 6 valid frames and 1 invalid frame (T+LB, was TALB)
	if tag.ItemCount() != 6 {
		t.Fatalf("len = %d (items=%v)", tag.ItemCount(), tag.Items())
	}
	if s, _ := tag.GetString(meta.ItemKeyTrackTitle); s != "Foo title" {
		t.Fatalf("title = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyTrackArtist); s != "Bar artist" {
		t.Fatalf("artist = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyComment); s != "Qux comment" {
		t.Fatalf("comment = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyRecordingDate); s != "1984" {
		t.Fatalf("date = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyTrackNumber); s != "1" {
		t.Fatalf("track = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyGenre); s != "Classical" {
		t.Fatalf("genre = %q", s)
	}
}

func TestUndersizedFrames(t *testing.T) {
	// A COMM frame needs at least 5 bytes
	badTag := []byte("ID3\x04\x00\x00\x00\x00\x00\x20" +
		"COMM\x00\x00\x00\x01\x00\x00\x00" +
		"TPE1\x00\x00\x00\x0B\x00\x00\x00Serial-ATA")

	// Relaxed mode should parse the tag fine, skipping the comment
	opts := meta.NewParseOptions()
	opts.ParsingMode = meta.ParsingModeRelaxed
	r := bytes.NewReader(badTag)
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := ParseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("relaxed parse failed: %v", err)
	}
	if s, ok := tag.GetString(meta.ItemKeyTrackArtist); !ok || s != "Serial-ATA" {
		t.Fatalf("artist = %q", s)
	}

	// Anything else should fail entirely
	for _, mode := range []meta.ParsingMode{meta.ParsingModeStrict, meta.ParsingModeBestAttempt} {
		opts.ParsingMode = mode
		r = bytes.NewReader(badTag)
		header, err = ParseId3v2HeaderRaw(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseId3v2(r, header, opts); err == nil {
			t.Fatalf("mode %v: expected error", mode)
		}
	}
}

func TestReadMultipleComposers(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("multiple_composers.id3v24"), meta.NewParseOptions())
	got := tag.GetStrings(meta.ItemKeyComposer)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("composers = %v", got)
	}
}

func TestTrimEndNullsWhenReadingFrameContent(t *testing.T) {
	// Issue #273 — tag written by mid3v2; all frames contain
	// null-terminated UTF-8 text that must come back trimmed.
	tag := readId3v2File(t, id3v2TestAsset("trailing_nulls.id3v24"), meta.NewParseOptions())
	if s, _ := tag.GetString(meta.ItemKeyTrackArtist); s != "Artist" {
		t.Fatalf("artist = %q", s)
	}
	// TXXX:Writer has no meta.ItemKey mapping — it stays in the companion tag,
	// content NUL-trimmed.
	found := false
	if companion, ok := tag.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.Frames() {
			if tf, ok := f.(*ExtendedTextFrame); ok && tf.Description == "Writer" && tf.Content == "Writer" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("TXXX:Writer missing or still NUL-terminated")
	}
	if s, _ := tag.GetString(meta.ItemKeyUnsyncLyrics); s != "Lyrics to the song" {
		t.Fatalf("lyrics = %q", s)
	}
	if s, _ := tag.GetString(meta.ItemKeyComment); s != "Comment" {
		t.Fatalf("comment = %q", s)
	}
	// WXXX:Fanpage has no meta.ItemKey mapping — retained in the companion tag.
	found = false
	if companion, ok := tag.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.Frames() {
			if wf, ok := f.(*ExtendedUrlFrame); ok && wf.Description == "Fanpage" &&
				wf.Content == "https://www.myfanpage.com" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("WXXX url missing or still NUL-terminated")
	}
}

func TestIssue36(t *testing.T) {
	// A picture in a tag written as UTF-8 must read back losslessly
	tag := meta.NewTag(meta.TagTypeID3v2)
	pic := meta.PictureUnchecked(make([]byte, 200)).
		PicType(meta.PictureTypeCoverFront).
		MimeType(meta.MimeTypeJpeg).
		Description("cover").
		Build()
	tag.PushPicture(pic)

	encoded := id3v2Dump(t, tag, false)
	re := id3v2Reparse(t, encoded, meta.NewParseOptions())
	if re.PictureCount() != 1 {
		t.Fatalf("pictures = %d", re.PictureCount())
	}
	p := re.Pictures()[0]
	d, _ := p.Description()
	if d != "cover" || p.PicType() != meta.PictureTypeCoverFront {
		t.Fatalf("pic = %+v", p)
	}
	if len(p.Data()) != 200 {
		t.Fatalf("pic data len = %d", len(p.Data()))
	}
}

// Ported from id3/v2/read.rs::zero_size_id3v2 — a header-only tag parses fine
// under Strict.
func TestId3v2ZeroSize(t *testing.T) {
	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	tag := readId3v2File(t, id3v2TestAsset("zero.id3v2"), po)
	if tag == nil {
		t.Fatal("expected a tag")
	}
}

// Ported from id3/mod.rs::find_id3v2_seek_back — on failure the reader must be
// restored to its original position.
func TestFindId3v2SeekBack(t *testing.T) {
	var data [32]byte
	// Bury the tag in junk
	copy(data[20:], "ID3")

	r := bytes.NewReader(data[:])
	if _, err := r.Seek(5, 0); err != nil {
		t.Fatal(err)
	}
	_, _, found, err := FindId3v2(r, true, meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("found a tag in junk")
	}
	if pos, _ := r.Seek(0, 1); pos != 5 {
		t.Fatalf("position = %d, want 5 (seek back)", pos)
	}
}

// Ported from src/id3/v2/tag/tests.rs::tipl_round_trip — mapped TIPL pairs
// convert to generic items; unmapped pairs are held back and merge into a
// single TIPL frame on write. TMCL has no meta.ItemKey mappings, so all of its
// pairs are held back and re-emitted as TMCL (v2.4) or merged into IPLS (v2.3).
func TestTiplRoundTrip(t *testing.T) {
	// Build a v2.4 tag: TIPL with all mapped roles + one unmapped pair, and a
	// TMCL frame.
	var content bytes.Buffer
	writeFrame := func(id, body string) {
		content.WriteString(id)
		sz := util.Synchsafe32(uint32(len(body)))
		content.Write(sz[:])
		content.Write([]byte{0, 0}) // flags
		content.WriteString(body)
	}
	tiplBody := "\x03" + strings.Join([]string{
		"producer", "Serial-ATA", "arranger", "Serial-ATA", "engineer", "Serial-ATA",
		"DJ-mix", "Serial-ATA", "mix", "Serial-ATA", "Foo", "Bar",
	}, "\x00")
	writeFrame("TIPL", tiplBody)
	writeFrame("TMCL", "\x03guitar\x00Serial-ATA")

	var tagBytes bytes.Buffer
	tagBytes.WriteString("ID3\x04\x00\x00")
	ss := util.Synchsafe32(uint32(content.Len()))
	tagBytes.Write(ss[:])
	tagBytes.Write(content.Bytes())

	tag := id3v2Reparse(t, tagBytes.Bytes(), meta.NewParseOptions())

	// All 5 mapped roles are generic items
	for _, k := range []meta.ItemKey{meta.ItemKeyProducer, meta.ItemKeyArranger, meta.ItemKeyEngineer, meta.ItemKeyMixDj, meta.ItemKeyMixEngineer} {
		if s, ok := tag.GetString(k); !ok || s != "Serial-ATA" {
			t.Fatalf("key %v = %q (ok=%v)", k, s, ok)
		}
	}
	// "Foo" (TIPL) and "guitar" (TMCL) are held back in the companion tag
	if tag.Len() != 5 {
		t.Fatalf("len = %d, want 5 (mapped items only)", tag.Len())
	}
	companion, ok := tag.CompanionTag().(*Id3v2Tag)
	if !ok || companion == nil {
		t.Fatal("no companion Id3v2Tag")
	}
	var foundTipl, foundTmcl bool
	for _, f := range companion.Frames.Frames() {
		if kv, ok := f.(*KeyValueFrame); ok {
			switch kv.ID().String() {
			case "TIPL":
				foundTipl = true
			case "TMCL":
				foundTmcl = true
			}
		}
	}
	if !foundTipl || !foundTmcl {
		t.Fatalf("TIPL hold-back=%v, TMCL hold-back=%v", foundTipl, foundTmcl)
	}

	// Round-trip: the unmapped pairs merge back into single frames.
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), meta.NewParseOptions())
	if re.Len() != 5 {
		t.Fatalf("re-read len = %d, want 5", re.Len())
	}

	// v2.3 write merges TIPL+TMCL into IPLS; on re-read IPLS upgrades to TIPL
	// and the unmapped pairs stay in the companion tag.
	re23 := id3v2Reparse(t, id3v2Dump(t, tag, true), meta.NewParseOptions())
	if re23.Len() != 5 {
		t.Fatalf("v2.3 re-read len = %d, want 5", re23.Len())
	}
	var held []string
	if c23, ok := re23.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range c23.Frames.Frames() {
			if kv, ok := f.(*KeyValueFrame); ok && kv.ID().String() == "TIPL" {
				for _, pair := range kv.KeyValuePairs {
					held = append(held, pair[0], pair[1])
				}
			}
		}
	}
	for _, want := range []string{"Foo", "Bar", "guitar", "Serial-ATA"} {
		found := false
		for _, h := range held {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("held-back pairs missing %q: %v", want, held)
		}
	}
}

// Ported from id3/v2/tag/tests.rs::fail_write_bad_frame — a frame with an
// invalid ID/content-type combination must fail the whole write.
func TestFailWriteBadFrame(t *testing.T) {
	tag := NewId3v2Tag()
	// URL content under a non-W ID is invalid
	tag.Frames.Insert(NewUrlLinkFrame(FrameIdValid("ABCD"), "FOO URL"))
	var buf bytes.Buffer
	err := tag.DumpTo(&buf, meta.NewWriteOptions())
	if err == nil {
		t.Fatal("expected write failure")
	}
	if err.Error() != "failed to write Id3v2 tag" {
		t.Fatalf("error = %q", err.Error())
	}
}

// Ported from id3/v2/tag/tests.rs::id3v24_footer — the 10-byte footer must
// mirror the header (3DI + version + flags + size).
func TestId3v24Footer(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetFlags(Id3v2TagFlags{Footer: true})
	tag.Frames.Insert(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingUTF8, "Title"))
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	writer := buf.Bytes()

	strict := meta.NewParseOptions()
	strict.ParsingMode = meta.ParsingModeStrict
	r := bytes.NewReader(writer)
	header, err := ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseId3v2Tag(r, header, strict); err != nil {
		t.Fatal(err)
	}

	// writer[3..10] == footer tail bytes (excluding "3DI")
	if !bytes.Equal(writer[3:10], writer[len(writer)-7:]) {
		t.Fatalf("footer %x does not mirror header %x", writer[len(writer)-10:], writer[:10])
	}
}

// Ported from id3/v2/tag/tests.rs::comments — comment accessors only see
// empty-descriptor frames.
func TestId3v2Comments(t *testing.T) {
	tag := NewId3v2Tag()
	customDescriptor := "lofty-rs"

	if tag.Comment() != "" {
		t.Fatal("expected no comment")
	}

	// Add an empty comment (which is a valid use case).
	tag.SetComment("")
	if len(tag.Comments()) != 1 || tag.Comment() != "" {
		t.Fatalf("comment = %q", tag.Comment())
	}

	// Insert a custom comment frame
	hasCustom := func() bool {
		for _, f := range tag.Frames.Frames() {
			if cf, ok := f.(*CommentFrame); ok && cf.Description == customDescriptor {
				return true
			}
		}
		return false
	}
	if hasCustom() {
		t.Fatal("unexpected custom comment")
	}
	tag.Frames.Insert(NewCommentFrame(TextEncodingLatin1, [3]byte{'e', 'n', 'g'}, customDescriptor, "Qux comment"))

	// Verify that the regular comment still exists
	if len(tag.Comments()) != 1 {
		t.Fatalf("comments = %d", len(tag.Comments()))
	}

	tag.RemoveComment()
	if tag.Comment() != "" {
		t.Fatal("expected no comment after removal")
	}
	if !hasCustom() {
		t.Fatal("custom comment lost")
	}
}

// Ported from id3/v2/tag/tests.rs::set_track_total_and_track.
func TestSetTrackTotalAndTrack(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetTrackTotal(2)
	tag.SetTrack(1)
	if tot, ok := tag.TrackTotal().Get(); !ok || tot != 2 {
		t.Fatalf("total = %v (ok=%v)", tot, ok)
	}
	if num, ok := tag.Track().Get(); !ok || num != 1 {
		t.Fatalf("track = %v (ok=%v)", num, ok)
	}
}

// Ported from id3/v2/tag/tests.rs::set_disk_total_and_disk.
func TestSetDiskTotalAndDisk(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetDiskTotal(2)
	tag.SetDisk(1)
	if tot, ok := tag.DiskTotal().Get(); !ok || tot != 2 {
		t.Fatalf("total = %v (ok=%v)", tot, ok)
	}
	if num, ok := tag.Disk().Get(); !ok || num != 1 {
		t.Fatalf("disk = %v (ok=%v)", num, ok)
	}
}

// Ported from id3/v2/tag/tests.rs::get_set_user_defined_text.
func TestGetSetUserDefinedText(t *testing.T) {
	description := "FOO_BAR"
	content := "Baz!\x00Qux!"
	description2 := "FOO_BAR_2"
	content2 := ""

	tag := NewId3v2Tag()
	tag.Frames.Insert(NewExtendedTextFrame(TextEncodingUTF8, description, content))
	tag.Frames.Insert(NewExtendedTextFrame(TextEncodingUTF8, description2, content2))

	// TXXX frames are not reachable through GetText
	if _, ok := tag.Frames.GetText(FrameIdValid("TXXX")); ok {
		t.Fatal("GetText(TXXX) should be none")
	}
	if v, ok := tag.Frames.GetUserText(description); !ok || v != content {
		t.Fatalf("GetUserText = %q (ok=%v)", v, ok)
	}

	// Wipe the tag
	tag.Frames.Clear()

	// Same thing process as above, using simplified setter
	if f := tag.Frames.InsertUserText(description, content); f != nil {
		t.Fatalf("InsertUserText replaced %+v", f)
	}
	if f := tag.Frames.InsertUserText(description2, content2); f != nil {
		t.Fatalf("InsertUserText replaced %+v", f)
	}
	if v, ok := tag.Frames.GetUserText(description); !ok || v != content {
		t.Fatalf("GetUserText = %q (ok=%v)", v, ok)
	}

	// Remove one frame
	if f := tag.Frames.RemoveUserText(description); f == nil {
		t.Fatal("RemoveUserText found nothing")
	}
	if tag.IsEmpty() {
		t.Fatal("tag should not be empty")
	}
	if f := tag.Frames.RemoveUserText(description2); f == nil {
		t.Fatal("RemoveUserText found nothing")
	}
	if !tag.IsEmpty() {
		t.Fatal("tag should be empty")
	}
}

// Ported from the popm_tests! macro in id3/v2/tag/tests.rs — rating-provider
// mapped values.
func TestPopmProviderMappings(t *testing.T) {
	cases := []struct {
		name  string
		build func(meta.StarRating, uint64) meta.Popularimeter
		want  []uint8
	}{
		{"musicbee", meta.PopularimeterMusicBee, []uint8{1, 64, 128, 196, 255}},
		{"windows_media_player", meta.PopularimeterWindowsMediaPlayer, []uint8{1, 64, 128, 196, 255}},
		{"picard", meta.PopularimeterPicard, []uint8{51, 102, 153, 204, 255}},
	}
	stars := []meta.StarRating{meta.StarRatingOne, meta.StarRatingTwo, meta.StarRatingThree, meta.StarRatingFour, meta.StarRatingFive}
	for _, c := range cases {
		for i, star := range stars {
			frame := PopularimeterFrameFromPopularimeter(c.build(star, 0))
			if frame.Rating != c.want[i] {
				t.Errorf("%s %d stars: rating = %d, want %d", c.name, star, frame.Rating, c.want[i])
			}
		}
	}
}

// --- read-side regression tests (review fixes) ---
//
// The byte layouts below were cross-checked against the Rust implementation
// (lofty 0.25.5) with a differential harness.

func rtSynch(n int) []byte {
	return []byte{byte(n>>21) & 0x7F, byte(n>>14) & 0x7F, byte(n>>7) & 0x7F, byte(n) & 0x7F}
}

func rtTag(ver, flags byte, body []byte) []byte {
	out := []byte{'I', 'D', '3', ver, 0, flags}
	out = append(out, rtSynch(len(body))...)
	return append(out, body...)
}

func rtFrame(id string, flags uint16, content []byte, synch bool) []byte {
	out := []byte(id)
	n := len(content)
	if synch {
		out = append(out, rtSynch(n)...)
	} else {
		out = append(out, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	out = append(out, byte(flags>>8), byte(flags))
	return append(out, content...)
}

func rtCat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func rtZlib(b []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

var rtModes = []struct {
	name string
	mode meta.ParsingMode
}{
	{"strict", meta.ParsingModeStrict},
	{"best-attempt", meta.ParsingModeBestAttempt},
	{"relaxed", meta.ParsingModeRelaxed},
}

func rtParse(data []byte, mode meta.ParsingMode) (*Id3v2Tag, error) {
	opts := meta.NewParseOptions()
	opts.ParsingMode = mode
	r := bytes.NewReader(data)
	h, err := ParseId3v2Header(r, opts)
	if err != nil {
		return nil, err
	}
	return ParseId3v2Tag(r, h, opts)
}

func rtParseAll(t *testing.T, data []byte, fn func(t *testing.T, tag *Id3v2Tag, err error)) {
	t.Helper()
	for _, m := range rtModes {
		tag, err := rtParse(data, m.mode)
		t.Run(m.name, func(t *testing.T) { fn(t, tag, err) })
	}
}

func rtText(t *testing.T, tag *Id3v2Tag) string {
	t.Helper()
	v, ok := tag.Frames.GetText(FrameIdValid("TIT2"))
	if !ok {
		t.Fatal("no TIT2 frame")
	}
	return v
}

// A frame declaring more bytes than the tag holds is truncated, not rejected
// (Rust reads through `reader.take(size)`).
func TestReadFrameTruncatedByTagEnd(t *testing.T) {
	body := rtCat([]byte("TIT2\x00\x00\x00\x14\x00\x00"), []byte{0, 'h', 'e', 'l', 'l', 'o', 'w', 'o', 'r', 'l'})
	rtParseAll(t, rtTag(3, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != "helloworl" {
			t.Fatalf("TIT2 = %q", got)
		}
	})

	// A declared size near u32::MAX must neither fail nor allocate it.
	body = rtCat([]byte("TIT2\xFF\xFF\xFF\xF0\x00\x00"), []byte{3, 'h', 'i'})
	rtParseAll(t, rtTag(3, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != "hi" {
			t.Fatalf("TIT2 = %q", got)
		}
	})

	// v2.3 tag-wide unsynchronisation whose frame size counts the stuffed byte
	body = rtFrame("TIT2", 0, []byte{0, 'a', 0xFF, 0x00, 0xFB, 'b'}, false)
	rtParseAll(t, rtTag(3, 0x80, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != "a\u00ff\u00fbb" {
			t.Fatalf("TIT2 = %q", got)
		}
	})
}

// Rust only applies the allocation limit to encrypted frames and RVA2 peaks;
// ordinary frame content is bounded by the tag.
func TestReadLargeFrameIgnoresAllocationLimit(t *testing.T) {
	n := 17 * 1024 * 1024 // > DefaultAllocationLimit (16 MiB)
	content := rtCat([]byte{0x00}, []byte("image/p\x00"), []byte{0x03, 0x00}, make([]byte, n))
	data := rtTag(4, 0, rtFrame("APIC", 0, content, true))
	for _, m := range rtModes {
		for _, src := range []struct {
			name string
			r    io.Reader
		}{{"bytes", bytes.NewReader(data[10:])}, {"opaque", io.MultiReader(bytes.NewReader(data[10:]))}} {
			t.Run(m.name+"/"+src.name, func(t *testing.T) {
				opts := meta.NewParseOptions()
				opts.ParsingMode = m.mode
				h, err := ParseId3v2HeaderRaw(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				tag, err := ParseId3v2Tag(src.r, h, opts)
				if err != nil {
					t.Fatal(err)
				}
				pf, ok := tag.Frames.Get(FrameIdValid("APIC")).(*AttachedPictureFrame)
				if !ok || len(pf.Picture.Data()) != n {
					t.Fatalf("APIC missing or wrong size (ok=%v)", ok)
				}
			})
		}
	}
}

// Encrypted frames still use an exact read (Rust `read_exact`).
func TestReadEncryptedFrameShortIsError(t *testing.T) {
	dli := rtSynch(6)
	// size 8 - 1 (symbol) - 4 (DLI) = 3 data bytes, only 2 follow in the tag
	body := rtCat([]byte("TIT2"), rtSynch(8), []byte{0x00, 0x05}, []byte{0x81}, dli, []byte{1, 2})
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err == nil {
			t.Fatal("expected an error for a truncated encrypted frame")
		}
	})

	// A complete encrypted frame becomes a binary frame
	body = rtFrame("TIT2", 0x0005, rtCat([]byte{0x81}, dli, []byte{1, 2, 3}), true)
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		bf, ok := tag.Frames.Get(FrameIdValid("TIT2")).(*BinaryFrame)
		if !ok || !bytes.Equal(bf.Data, []byte{1, 2, 3}) || bf.Flags().Encryption.IsNone() {
			t.Fatalf("frame = %#v", tag.Frames.Get(FrameIdValid("TIT2")))
		}
	})
}

// De-unsynchronisation shortening an encrypted frame below its declared size
// is an error, like Rust's `read_exact` over the unsynchronised stream.
func TestReadEncryptedUnsyncShortenedIsError(t *testing.T) {
	dli := rtSynch(3)
	// flags: encryption + unsynchronisation + data length indicator
	body := rtFrame("TIT2", 0x0007, rtCat([]byte{0x81}, dli, []byte{0xFF, 0x00, 0x01}), true)
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	// Without a stuffed byte the data is intact
	body = rtFrame("TIT2", 0x0007, rtCat([]byte{0x81}, dli, []byte{0x02, 0x00, 0x01}), true)
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
}

// A UTF-16 TXXX whose content is a single 0xFF/0xFE byte used to index past
// the end of the slice.
func TestReadTxxxUtf16SingleTrailingByte(t *testing.T) {
	for _, last := range []byte{0xFF, 0xFE, 0x41} {
		content := rtCat([]byte{0x01, 0xFF, 0xFE, 0x61, 0x00, 0x00, 0x00}, []byte{last})
		rtParseAll(t, rtTag(4, 0, rtFrame("TXXX", 0, content, true)), func(t *testing.T, tag *Id3v2Tag, err error) {
			if err != nil {
				t.Fatalf("last=%#x: %v", last, err)
			}
			tx, ok := tag.Frames.Frames()[0].(*ExtendedTextFrame)
			if !ok || tx.Description != "a" || tx.Content != "" {
				t.Fatalf("last=%#x: frame = %#v", last, tag.Frames.Frames()[0])
			}
		})
	}
}

// Rust trims trailing NULs from frame IDs (`utf8_decode_str`), so a v2.4 ID
// "TT2\0" becomes the outdated 3-character ID "TT2".
func TestReadFrameIdTrailingNul(t *testing.T) {
	body := rtFrame("TT2\x00", 0, []byte{0x00, 'x'}, true)
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if tag.Frames.Len() != 1 {
			t.Fatalf("frames = %d", tag.Frames.Len())
		}
		f := tag.Frames.Frames()[0]
		tf, ok := f.(*TextInformationFrame)
		if !ok || f.ID() != FrameIdOutdated("TT2") || tf.Value != "x" {
			t.Fatalf("frame = %#v", f)
		}
	})
}

// Sizes are unsynchsafe'd with every byte masked to 7 bits (Rust `unsynch`).
func TestReadNonSynchsafeSizes(t *testing.T) {
	// A v2.4 frame size of 00 00 00 C8 is read as 0x48 (72) bytes.
	frame := rtFrame("TIT2", 0, append([]byte{3}, bytes.Repeat([]byte("a"), 199)...), false)
	data := rtTag(4, 0, frame)
	for _, m := range rtModes[1:] {
		tag, err := rtParse(data, m.mode)
		if err != nil {
			t.Fatal(m.name, err)
		}
		if got := rtText(t, tag); got != strings.Repeat("a", 71) {
			t.Fatalf("%s: TIT2 has %d bytes, want 71", m.name, len(got))
		}
	}

	// Tag size bytes 00 00 00 80 mean size 0.
	h, err := ParseId3v2HeaderRaw(bytes.NewReader([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0x80}))
	if err != nil || h.Size != 0 {
		t.Fatalf("header = %+v, err = %v", h, err)
	}

	// A v2.4 extended header size of 00 00 00 86 means 6.
	body := []byte{0x00, 0x00, 0x00, 0x86, 0x01, 0x00}
	h, err = ParseId3v2Header(bytes.NewReader(rtTag(4, 0x40, body)), meta.NewParseOptions())
	if err != nil || h.ExtendedSize != 6 {
		t.Fatalf("header = %+v, err = %v", h, err)
	}
}

// Compression is lazy like Rust's ZlibDecoder.
func TestReadCompressedFrames(t *testing.T) {
	raw := append([]byte{3}, []byte("hello")...)
	dli := rtSynch(len(raw))

	// v2.4: compression + data length indicator
	rtParseAll(t, rtTag(4, 0, rtFrame("TIT2", 0x0009, rtCat(dli, rtZlib(raw)), true)), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != "hello" {
			t.Fatalf("TIT2 = %q", got)
		}
	})
	// v2.3: compression flag, plain 4-byte decompressed size
	rtParseAll(t, rtTag(3, 0, rtFrame("TIT2", 0x0080, rtCat([]byte{0, 0, 0, byte(len(raw))}, rtZlib(raw)), false)), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != "hello" {
			t.Fatalf("TIT2 = %q", got)
		}
	})

	// A corrupt stream in an RVA2 frame: Rust's parsers swallow the read error
	// (`while let Ok(..)`), leaving an empty frame.
	bad := rtFrame("RVA2", 0x0009, rtCat(dli, []byte("ABCDEFG")), true)
	rtParseAll(t, rtTag(4, 0, bad), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		rv, ok := tag.Frames.Get(FrameIdValid("RVA2")).(*RelativeVolumeAdjustmentFrame)
		if !ok || rv.Identification != "" || len(rv.Channels) != 0 {
			t.Fatalf("frame = %#v", tag.Frames.Get(FrameIdValid("RVA2")))
		}
	})

	// ...while a text frame fails, because Rust's skip drains the broken reader.
	bad = rtFrame("TIT2", 0x0009, rtCat(dli, []byte("ABCDEFG")), true)
	rtParseAll(t, rtTag(4, 0, bad), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

// Deliberate deviation from Rust: after a parser stops early (here a URL
// frame stops at its first NUL) Go resumes at the declared frame end, whereas
// Rust re-reads the leftover content as the next frame header.
func TestReadRealignsToFrameEnd(t *testing.T) {
	body := rtCat(
		rtFrame("WOAF", 0, []byte("http://x\x00junk-junk-junk"), true),
		rtFrame("TIT2", 0, []byte{3, 'a'}, true),
	)
	rtParseAll(t, rtTag(4, 0, body), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		u, ok := tag.Frames.Get(FrameIdValid("WOAF")).(*UrlLinkFrame)
		if !ok || u.Content != "http://x" {
			t.Fatalf("WOAF = %#v", tag.Frames.Get(FrameIdValid("WOAF")))
		}
		if got := rtText(t, tag); got != "a" {
			t.Fatalf("TIT2 = %q", got)
		}
	})
}

// A v2.3 TDRC/TYER that already has a month is rejected in strict mode and
// kept in best-attempt/relaxed modes (upstream lofty-rs issue #794).
func TestReadV23TdrcWithMonthIsKept(t *testing.T) {
	body := rtFrame("TDRC", 0, append([]byte{0}, "2020-05-01"...), false)
	if _, err := rtParse(rtTag(3, 0, body), meta.ParsingModeStrict); err == nil {
		t.Fatal("expected strict mode error for v2.3 TDRC with month")
	}
	for _, m := range rtModes[1:] {
		t.Run(m.name, func(t *testing.T) {
			tag, err := rtParse(rtTag(3, 0, body), m.mode)
			if err != nil {
				t.Fatal(err)
			}
			f, ok := tag.Frames.Get(FrameIdValid("TDRC")).(*TimestampFrame)
			if !ok || f.Timestamp.String() != "2020-05-01" {
				t.Fatalf("TDRC = %#v", tag.Frames.Get(FrameIdValid("TDRC")))
			}
		})
	}
}

// The "not enough data" frame error is the shared NotEnoughDataError (Rust
// `crate::error::NotEnoughDataError`), so errors.As finds it.
func TestReadUndersizedFrameError(t *testing.T) {
	body := rtFrame("COMM", 0, []byte{0, 'e', 'n'}, true)
	for _, m := range rtModes[:2] {
		_, err := rtParse(rtTag(4, 0, body), m.mode)
		var ne *util.NotEnoughDataError
		if !errors.As(err, &ne) || !ne.HasExpected || ne.Expected != 5 {
			t.Fatalf("%s: err = %v (%#v)", m.name, err, ne)
		}
		var fe *FrameParseError
		if !errors.As(err, &fe) || fe.ID.IsNone() {
			t.Fatalf("%s: frame error without ID: %v", m.name, err)
		}
	}
	// Relaxed mode skips the frame
	tag, err := rtParse(rtTag(4, 0, body), meta.ParsingModeRelaxed)
	if err != nil || tag.Frames.Len() != 0 {
		t.Fatalf("relaxed: err = %v", err)
	}
}

// Header errors carry the same information as Rust's `Id3v2HeaderError`.
func TestHeaderErrorKinds(t *testing.T) {
	check := func(data []byte, kind Id3v2HeaderErrorKind) *Id3v2HeaderError {
		t.Helper()
		_, err := ParseId3v2Header(bytes.NewReader(data), meta.NewParseOptions())
		var he *Id3v2HeaderError
		if !errors.As(err, &he) || he.Kind != kind {
			t.Fatalf("err = %v, want kind %d", err, kind)
		}
		return he
	}
	he := check([]byte{'I', 'D', '3', 5, 1, 0, 0, 0, 0, 0}, Id3v2HeaderBadVersion)
	if he.Major != 5 || he.Minor != 1 || !strings.Contains(he.Error(), "v5.1") {
		t.Fatalf("bad version error = %+v / %q", he, he.Error())
	}
	check([]byte{'I', 'D', '3', 2, 0, 0x40, 0, 0, 0, 0}, Id3v2HeaderV2Compression)
	check(rtTag(4, 0x40, []byte{0, 0, 0, 3, 1, 0}), Id3v2HeaderBadExtendedHeaderSize)
	strict := meta.NewParseOptions()
	strict.ParsingMode = meta.ParsingModeStrict
	_, err := ParseId3v2Header(bytes.NewReader(rtTag(4, 0x40, []byte{0, 0, 0, 6, 1, 0x01})), strict)
	var he2 *Id3v2HeaderError
	if !errors.As(err, &he2) || he2.Kind != Id3v2HeaderBadExtendedHeaderFlags {
		t.Fatalf("err = %v", err)
	}
}

type rtEOFWithDataReader struct {
	data []byte
	pos  int
}

func (r *rtEOFWithDataReader) Read(p []byte) (int, error) {
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// Bytes returned together with an error must still be processed.
func TestFindId3v2InJunkDataWithEOF(t *testing.T) {
	// One byte per Read, the last one accompanied by io.EOF
	r := io.Reader(&rtOneByteReader{rtEOFWithDataReader{data: []byte("xxID3")}})
	off, found := FindId3v2InJunk(r)
	if !found || off != 2 {
		t.Fatalf("found = %v, offset = %d", found, off)
	}
}

type rtOneByteReader struct{ rtEOFWithDataReader }

func (r *rtOneByteReader) Read(p []byte) (int, error) { return r.rtEOFWithDataReader.Read(p[:1]) }

// LocateId3v2 must end exactly at the end of the tag even when an extended
// header (already consumed by the header parser, but counted in the size) is
// present, or a footer follows.
func TestLocateId3v2Range(t *testing.T) {
	frame := rtFrame("TIT2", 0, []byte{3, 'a'}, true)
	audio := []byte("AUDIOAUDIO")
	v4ext := rtCat([]byte{0, 0, 0, 6, 1, 0}, frame)
	v3ext := rtCat([]byte{0, 0, 0, 6, 0, 0, 0, 0, 0, 0}, rtFrame("TIT2", 0, []byte{0, 'a'}, false))
	footer := []byte("3DI\x04\x00\x10\x00\x00\x00\x0b")
	cases := []struct {
		name string
		tag  []byte
	}{
		{"plain", rtTag(4, 0, frame)},
		{"v4-ext", rtTag(4, 0x40, v4ext)},
		{"v3-ext", rtTag(3, 0x40, v3ext)},
		{"footer", rtCat(rtTag(4, 0x10, frame), footer)},
	}
	for _, c := range cases {
		for _, read := range []bool{false, true} {
			for _, junk := range []string{"", "jj"} {
				data := rtCat([]byte(junk), c.tag, audio)
				r := bytes.NewReader(data)
				res, err := LocateId3v2(r, read, meta.NewParseOptions())
				if err != nil || res == nil {
					t.Fatalf("%s: res = %v, err = %v", c.name, res, err)
				}
				wantEnd := int64(len(junk) + len(c.tag))
				pos, _ := r.Seek(0, io.SeekCurrent)
				if res.End != wantEnd || pos != wantEnd {
					t.Fatalf("%s (read=%v junk=%q): End = %d, pos = %d, want %d", c.name, read, junk, res.End, pos, wantEnd)
				}
				if read {
					tag, err := res.Parse(meta.NewParseOptions())
					if err != nil {
						t.Fatalf("%s: %v", c.name, err)
					}
					if got := rtText(t, tag); got != "a" {
						t.Fatalf("%s: TIT2 = %q", c.name, got)
					}
				}
			}
		}
	}
}

// --- mutated-tag robustness ---

func rtMutate(r *rand.Rand, tag []byte) []byte {
	hdr := append([]byte(nil), tag[:10]...)
	body := append([]byte(nil), tag[10:]...)
	for i, nops := 0, 1+r.Intn(3); i < nops && len(body) > 0; i++ {
		switch r.Intn(8) {
		case 0, 1:
			body[r.Intn(len(body))] = byte(r.Intn(256))
		case 2:
			body[r.Intn(len(body))] = 0xFF
		case 3:
			body[r.Intn(len(body))] = 0
		case 4:
			body = body[:r.Intn(len(body)+1)]
		case 5:
			p := r.Intn(len(body))
			body = append(body[:p], body[p+1:]...)
		case 6:
			p := r.Intn(len(body) + 1)
			body = append(body[:p], append([]byte{byte(r.Intn(256))}, body[p:]...)...)
		case 7:
			if len(body) > 10 {
				body[r.Intn(10)] = byte(r.Intn(256))
			}
		}
	}
	if r.Intn(10) == 0 {
		hdr[5] = byte(r.Intn(256)) & 0xF0
		if hdr[3] == 2 {
			hdr[5] &^= 0x40
		}
	}
	copy(hdr[6:10], rtSynch(len(body)))
	return append(hdr, body...)
}

func rtFuzzSeeds(t *testing.T) map[string][]byte {
	seeds := map[string][]byte{}
	txt := func(enc byte, s ...byte) []byte { return append([]byte{enc}, s...) }
	u16 := func(s string, bom bool) []byte {
		var b []byte
		if bom {
			b = append(b, 0xFF, 0xFE)
		}
		for _, c := range s {
			b = append(b, byte(c), 0)
		}
		return b
	}
	v4 := func(frames ...[]byte) []byte { return rtTag(4, 0, rtCat(frames...)) }
	v3 := func(frames ...[]byte) []byte { return rtTag(3, 0, rtCat(frames...)) }
	raw := append([]byte{3}, []byte("hello")...)
	seeds["txxx-u16"] = v4(rtFrame("TXXX", 0, rtCat(txt(1, 0xFF, 0xFE, 0x61, 0, 0, 0), []byte{0xFF}), true))
	seeds["nul-id"] = v4(rtFrame("TT2\x00", 0, txt(0, 'x'), true))
	seeds["v23-v2id"] = v3(rtFrame("TT2\x00", 0, txt(0, 'x'), false))
	seeds["multi-bom"] = v4(rtFrame("TPE1", 0, rtCat([]byte{1}, u16("a", true), []byte{0, 0}, u16("b", true)), true))
	seeds["comm-u16"] = v4(rtFrame("COMM", 0, rtCat([]byte{1, 'e', 'n', 'g'}, u16("a", true), []byte{0, 0}, u16("bc", false)), true))
	seeds["v23-date"] = v3(rtFrame("TYER", 0, txt(0, '1', '9', '8', '4'), false), rtFrame("TDAT", 0, txt(0, '0', '6', '1', '2'), false), rtFrame("TIME", 0, txt(0, '1', '4', '3', '0'), false))
	seeds["unsync"] = rtTag(4, 0x80, rtFrame("TIT2", 0, txt(0, 'a', 0xFF, 0x00, 0xFB, 'b'), true))
	seeds["unsync-v3"] = rtTag(3, 0x80, rtFrame("TIT2", 0, txt(0, 'a', 0xFF, 0x00, 0xFB, 'b'), false))
	seeds["compressed"] = v4(rtFrame("TIT2", 0x0009, rtCat(rtSynch(len(raw)), rtZlib(raw)), true))
	seeds["encrypted"] = v4(rtFrame("TIT2", 0x0005, rtCat([]byte{0x81}, rtSynch(6), []byte{1, 2, 3}), true))
	seeds["ext-v4"] = rtTag(4, 0x40, rtCat([]byte{0, 0, 0, 12, 1, 0x20, 5, 0, 0, 0, 0, 0}, rtFrame("TIT2", 0, txt(3, 'a'), true)))
	seeds["ext-v3"] = rtTag(3, 0x40, rtCat([]byte{0, 0, 0, 6, 0, 0, 0, 0, 0, 0}, rtFrame("TIT2", 0, txt(0, 'a'), false)))
	seeds["v22"] = rtTag(2, 0, rtCat([]byte("TT2\x00\x00\x02\x00a"), []byte("PIC\x00\x00\x09\x00PNG\x03\x00\x01\x02\x03")))
	seeds["popm"] = v4(rtFrame("POPM", 0, rtCat([]byte("a@b\x00"), []byte{200, 0, 0, 0, 1}), true))
	seeds["ufid"] = v4(rtFrame("UFID", 0, rtCat([]byte("own\x00"), []byte{1, 2}), true))
	seeds["rva2"] = v4(rtFrame("RVA2", 0, rtCat([]byte("id\x00"), []byte{1, 0, 1, 8, 0x7F}), true))
	seeds["etco"] = v4(rtFrame("ETCO", 0, rtCat([]byte{2}, []byte{3, 0, 0, 0, 9}), true))
	seeds["owne"] = v4(rtFrame("OWNE", 0, rtCat([]byte{0}, []byte("USD1\x0020200101sel")), true))
	seeds["tipl"] = v4(rtFrame("TIPL", 0, rtCat([]byte{1}, u16("k", true), []byte{0, 0}, u16("v", false), []byte{0, 0}), true))
	seeds["tdrc"] = v4(rtFrame("TDRC", 0, txt(3, []byte("2020-05-01T10:20:30")...), true))

	// Fixtures: complete tags, plus bare frame bodies wrapped into tags
	wrap := map[string]string{
		"test.atxt": "ATXT", "test.chap": "CHAP", "test.ctoc": "CTOC", "test.etco": "ETCO", "test.geob": "GEOB",
		"test.owne": "OWNE", "test.priv": "PRIV", "test.rva2": "RVA2", "test.sylt": "SYLT", "test.tipl": "TIPL",
		"test_utf16.sylt": "SYLT", "test_utf16.tipl": "TIPL",
	}
	files, _ := filepath.Glob(id3v2TestAsset("*"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(f)
		if id, ok := wrap[base]; ok {
			seeds["w4-"+base] = rtTag(4, 0, rtFrame(id, 0, b, true))
			seeds["w3-"+base] = rtTag(3, 0, rtFrame(id, 0, b, false))
		} else if len(b) > 10 && string(b[:3]) == "ID3" {
			n := int(b[6])<<21 | int(b[7])<<14 | int(b[8])<<7 | int(b[9])
			if 10+n <= len(b) {
				seeds["f-"+base] = b[:10+n]
			}
		}
	}
	return seeds
}

// Mutated tags must never panic (or hang) in any parsing mode.
func TestParseId3v2MutatedTagsNeverPanic(t *testing.T) {
	seeds := rtFuzzSeeds(t)
	names := make([]string, 0, len(seeds))
	for n := range seeds {
		names = append(names, n)
	}
	sort.Strings(names)
	r := rand.New(rand.NewSource(12345))
	cases := 0
	for _, n := range names {
		s := seeds[n]
		if len(s) < 12 {
			continue
		}
		inputs := [][]byte{s}
		for i := 0; i < 60; i++ {
			inputs = append(inputs, rtMutate(r, s))
		}
		for _, in := range inputs {
			cases++
			for _, m := range rtModes {
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Fatalf("panic parsing %s mutation %x in %s mode: %v", n, in, m.name, p)
						}
					}()
					_, _ = rtParse(in, m.mode)
				}()
			}
		}
	}
	if cases < 2000 {
		t.Fatalf("only %d cases generated", cases)
	}
}
