package lofty

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	bogem "github.com/bogem/id3v2/v2"
)

// --- tests/taglib/test_id3v2.rs ---

// syncsafe28 encodes a 28-bit synchsafe size.
func syncsafe28(n int) []byte {
	return []byte{byte(n >> 21), byte(n >> 14), byte(n >> 7), byte(n)}
}

// rawId3v2Tag builds a minimal ID3v2 tag with the given text frames.
func rawId3v2Tag(version byte, frames ...[2]string) []byte {
	var body []byte
	for _, fr := range frames {
		content := append([]byte{0}, fr[1]...)
		body = append(body, fr[0]...)
		body = append(body, syncsafe28(len(content))...)
		body = append(body, 0, 0)
		body = append(body, content...)
	}
	out := append([]byte("ID3"), version, 0, 0)
	out = append(out, syncsafe28(len(body))...)
	return append(out, body...)
}

func parseRawId3v2(t *testing.T, data []byte) *Tag {
	t.Helper()
	r := bytes.NewReader(data)
	header, err := parseId3v2Header(r)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := parseId3v2(r, header, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func TestTaglibId3v2UnsynchDecode(t *testing.T) {
	f := taglibFile(t, "unsynch.id3")
	defer f.Close()
	po := NewParseOptions()
	po.ReadProperties = false
	mf, err := MpegFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	if s, _ := tag.Title(); s != "My babe just cares for me" {
		t.Fatalf("title = %q", s)
	}
}

// test_itunes_24_frame_size is #[ignore]d upstream — the iTunes non-synchsafe
// v2.4 frame-size bug is not handled by Lofty either.

func TestTaglibId3v2PopmFromFile(t *testing.T) {
	f := taglibTempFile(t, "xing.mp3")
	defer f.Close()
	{
		tag := NewTag(TagTypeID3v2)
		p := PopularimeterCustom("email@example.com", StarRatingFour, 3)
		tag.PushUnchecked(NewTagItem(ItemKeyPopularimeter, ItemValueText(p.String())))
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := tag.SaveTo(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	item := tag.Get(ItemKeyPopularimeter)
	if item == nil {
		t.Fatal("no POPM")
	}
	text, _ := item.Value().IntoString()
	p, err := PopularimeterFromStr(text)
	if err != nil {
		t.Fatal(err)
	}
	if p.Email() != "email@example.com" || p.Rating != StarRatingFour || p.PlayCounter != 3 {
		t.Fatalf("popm = %q", text)
	}
}

func TestTaglibId3v2CompressedFrameBrokenLength(t *testing.T) {
	f := taglibFile(t, "compressed_id3_frame.mp3")
	defer f.Close()
	po := NewParseOptions()
	po.ReadProperties = false
	mf, err := MpegFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	pics := tag.Pictures()
	if len(pics) != 1 {
		t.Fatalf("pictures = %d", len(pics))
	}
	pic := pics[0]
	if mt, _ := pic.MimeType().Get(); mt != MimeTypeBmp {
		t.Fatalf("mime = %v", pic.MimeType())
	}
	if pic.PicType() != PictureTypeOther {
		t.Fatalf("pic_type = %v", pic.PicType())
	}
	if d, ok := pic.Description(); ok && d != "" {
		t.Fatalf("unexpected description %q", d)
	}
	if len(pic.Data()) != 86414 {
		t.Fatalf("data len = %d", len(pic.Data()))
	}
}

func TestTaglibId3v2W000(t *testing.T) {
	f := taglibFile(t, "w000.mp3")
	defer f.Close()
	po := NewParseOptions()
	po.ReadProperties = false
	mf, err := MpegFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	var found bool
	for _, item := range tag.Items() {
		if item.Key() == ItemKeyUnknown && item.Description == "W000" {
			if v, ok := item.Value().IntoString(); !ok || v != "lukas.lalinsky@example.com____" {
				t.Fatalf("W000 = %q", v)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("W000 frame missing")
	}
}

func TestTaglibId3v2SaveUtf16Comment(t *testing.T) {
	f := taglibTempFile(t, "xing.mp3")
	defer f.Close()
	{
		mf, err := MpegFileReadFrom(f, NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tag := NewTag(TagTypeID3v2)
		tag.SetComment("Test comment!")
		mf.ID3v2 = Some(tag)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := mf.SaveFile(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, _ := mf.ID3v2.Get()
	if s, _ := tag.Comment(); s != "Test comment!" {
		t.Fatalf("comment = %q", s)
	}
}

func TestTaglibId3v2UpdateGenre23(t *testing.T) {
	// "Refinement" differs from the ID3v1 genre
	tag := parseRawId3v2(t, rawId3v2Tag(4, [2]string{"TCON", "(4)Eurodisco"}))
	genres := tag.GetStrings(ItemKeyGenre)
	if len(genres) != 2 || genres[0] != "Disco" || genres[1] != "Eurodisco" {
		t.Fatalf("genres = %v", genres)
	}

	// Multiple references and a refinement
	tag = parseRawId3v2(t, rawId3v2Tag(4, [2]string{"TCON", "(9)(138)Viking Metal"}))
	genres = tag.GetStrings(ItemKeyGenre)
	if len(genres) != 3 || genres[0] != "Metal" || genres[1] != "Black Metal" || genres[2] != "Viking Metal" {
		t.Fatalf("genres = %v", genres)
	}
}

func TestTaglibId3v2UpdateGenre24(t *testing.T) {
	tag := parseRawId3v2(t, rawId3v2Tag(4, [2]string{"TCON", "14\x00Eurodisco"}))
	genres := tag.GetStrings(ItemKeyGenre)
	if len(genres) != 2 || genres[0] != "R&B" || genres[1] != "Eurodisco" {
		t.Fatalf("genres = %v", genres)
	}
}

func TestTaglibId3v2UpdateDate22(t *testing.T) {
	f := taglibFile(t, "id3v22-tda.mp3")
	defer f.Close()
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	if d, ok := tag.Date().Get(); !ok || d.Year != 2010 {
		t.Fatalf("date = %v", tag.Date())
	}
}

func TestTaglibId3v2DowngradeTo23(t *testing.T) {
	f := taglibTempFile(t, "xing.mp3")
	defer f.Close()
	{
		tag := NewTag(TagTypeID3v2)
		tag.PushUnchecked(NewTagItem(ItemKeyOriginalReleaseDate, ItemValueText("2011-03-16")))
		tag.PushUnchecked(NewTagItem(ItemKeyRecordingDate, ItemValueText("2012-04-17T12:01")))
		tag.PushUnchecked(NewTagItem(ItemKeyArranger, ItemValueText("Artist 1")))
		tag.PushUnchecked(NewTagItem(ItemKeyEngineer, ItemValueText("Artist 2")))
		tag.PushUnchecked(NewTagItem(ItemKeyProducer, ItemValueText("Artist 3")))
		tag.PushUnchecked(NewTagItem(ItemKeyMixEngineer, ItemValueText("Artist 4")))
		tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText("51")))
		tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText("39")))
		tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText("Power Noise")))
		tag.PushUnchecked(NewTagItem(ItemKeyReleaseDate, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeyTaggingTime, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeyMood, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeyAlbumTitleSortOrder, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeyTrackTitleSortOrder, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeyTrackArtistSortOrder, ItemValueText("")))
		tag.PushUnchecked(NewTagItem(ItemKeySetSubtitle, ItemValueText("")))
		wo := NewWriteOptions()
		wo.UseID3v23 = true
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := tag.SaveTo(NewOSFile(f), wo); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	if tag.OriginalVersion != 3 {
		t.Fatalf("version = %d", tag.OriginalVersion)
	}
	// TORY from TDOR (year only)
	if s, _ := tag.GetString(ItemKeyOriginalReleaseDate); s != "2011" {
		t.Fatalf("original date = %q", s)
	}
	// TYER+TDAT+TIME recombined by the upgrade path
	if d, ok := tag.Date().Get(); !ok || d.Year != 2012 {
		t.Fatalf("date = %v", tag.Date())
	}
	genres := tag.GetStrings(ItemKeyGenre)
	if len(genres) != 3 || genres[0] != "Techno-Industrial" || genres[1] != "Noise" || genres[2] != "Power Noise" {
		t.Fatalf("genres = %v", genres)
	}
	// v2.4-only frames are dropped, not written
	for _, k := range []ItemKey{ItemKeyTaggingTime, ItemKeyMood,
		ItemKeyAlbumTitleSortOrder, ItemKeyTrackTitleSortOrder,
		ItemKeyTrackArtistSortOrder, ItemKeySetSubtitle} {
		if item := tag.Get(k); item != nil {
			t.Fatalf("%v not dropped", k)
		}
	}
	// TIPL/TMCL merged into IPLS — re-reads as involved-people items
	if s, _ := tag.GetString(ItemKeyProducer); s != "Artist 3" {
		t.Fatalf("producer = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyEngineer); s != "Artist 2" {
		t.Fatalf("engineer = %q", s)
	}
	// Byte-level check on the written header
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 10)
	if _, err := io.ReadFull(f, head); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(head[:4], []byte("ID3\x03")) {
		t.Fatalf("header = %x", head)
	}
	// Confirm the on-disk TCON frame is "(51)(39)Power Noise"
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	idx := bytes.Index(content, []byte("TCON"))
	if idx < 0 {
		t.Fatal("TCON missing")
	}
	// Written in UTF-16 (generic-tag writes use UTF-16 for ID3v2.3); decode to
	// verify the parenthesized genre references.
	enc, payload := content[idx+10], content[idx+11:idx+10+int(binary.BigEndian.Uint32(content[idx+4:idx+8]))]
	var tcon string
	if enc == 1 {
		s, err := utf16DecodeBytes(payload[2:], bigEndian)
		if err != nil {
			t.Fatal(err)
		}
		tcon = s
	} else {
		tcon = latin1Decode(payload)
	}
	if tcon != "(51)(39)Power Noise" {
		t.Fatalf("tcon = %q", tcon)
	}
}

func TestTaglibId3v2DeleteFrame(t *testing.T) {
	f := taglibTempFile(t, "rare_frames.mp3")
	defer f.Close()
	{
		mf, err := MpegFileReadFrom(f, NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tag, ok := mf.ID3v2.Get()
		if !ok {
			t.Fatal("no id3v2")
		}
		tag.RemoveKey(ItemKeyGenre)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := mf.SaveFile(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, _ := mf.ID3v2.Get()
	if _, ok := tag.Genre(); ok {
		t.Fatal("TCON not removed")
	}
}

func TestTaglibId3v2SaveAndStripId3v1(t *testing.T) {
	f := taglibTempFile(t, "xing.mp3")
	defer f.Close()
	{
		mf, err := MpegFileReadFrom(f, NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tag := NewTag(TagTypeID3v2)
		tag.SetArtist("Artist")
		mf.ID3v2 = Some(tag)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := mf.SaveFile(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	{
		mf, err := MpegFileReadFrom(f, NewParseOptions())
		if err != nil {
			t.Fatal(err)
		}
		tag, ok := mf.ID3v2.Get()
		if !ok {
			t.Fatal("no id3v2")
		}
		tag.RemoveKey(ItemKeyTrackArtist)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if err := mf.SaveFile(NewOSFile(f), NewWriteOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if mf.ID3v2.IsSome() {
		t.Fatal("id3v2 still present")
	}
}

func TestTaglibId3v2TocManyChildren(t *testing.T) {
	f := taglibFile(t, "toc_many_children.mp3")
	defer f.Close()
	po := NewParseOptions()
	po.ReadProperties = false
	mf, err := MpegFileReadFrom(f, po)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := mf.ID3v2.Get()
	if !ok {
		t.Fatal("no id3v2")
	}
	// Rust sees 130 frames: 1 CTOC + 129 CHAP
	if tag.Len() != 130 {
		t.Fatalf("frames = %d", tag.Len())
	}
}

// rawId3v2TagRaw builds a minimal ID3v2 tag with raw frame bodies.
func rawId3v2TagRaw(version byte, frames ...[2][]byte) []byte {
	var body []byte
	for _, fr := range frames {
		body = append(body, fr[0]...)
		body = append(body, syncsafe28(len(fr[1]))...)
		body = append(body, 0, 0)
		body = append(body, fr[1]...)
	}
	out := append([]byte("ID3"), version, 0, 0)
	out = append(out, syncsafe28(len(body))...)
	return append(out, body...)
}

func TestTaglibId3v2Utf16beDelimiter(t *testing.T) {
	body := []byte{0x02,
		0, 'F', 0, 'o', 0, 'o', 0, 0,
		0, 'B', 0, 'a', 0, 'r'}
	tag := parseRawId3v2(t, rawId3v2TagRaw(4, [2][]byte{[]byte("TIT2"), body}))
	got := tag.GetStrings(ItemKeyTrackTitle)
	if len(got) != 2 || got[0] != "Foo" || got[1] != "Bar" {
		t.Fatalf("titles = %v", got)
	}
}

func TestTaglibId3v2Utf16Delimiter(t *testing.T) {
	// single BOM LE, multi BOM LE, multi BOM BE, single BOM BE
	bodies := [][]byte{
		{0x01, 0xff, 0xfe, 'F', 0, 'o', 0, 'o', 0, 0, 0, 'B', 0, 'a', 0, 'r', 0},
		{0x01, 0xff, 0xfe, 'F', 0, 'o', 0, 'o', 0, 0, 0, 0xff, 0xfe, 'B', 0, 'a', 0, 'r', 0},
		{0x01, 0xfe, 0xff, 0, 'F', 0, 'o', 0, 'o', 0, 0, 0xfe, 0xff, 0, 'B', 0, 'a', 0, 'r'},
		{0x01, 0xfe, 0xff, 0, 'F', 0, 'o', 0, 'o', 0, 0, 0, 'B', 0, 'a', 0, 'r'},
	}
	for i, body := range bodies {
		tag := parseRawId3v2(t, rawId3v2TagRaw(4, [2][]byte{[]byte("TIT2"), body}))
		got := tag.GetStrings(ItemKeyTrackTitle)
		if len(got) != 2 || got[0] != "Foo" || got[1] != "Bar" {
			t.Fatalf("case %d: titles = %v", i, got)
		}
	}
}

func TestTaglibId3v2DowngradeUtf8For23(t *testing.T) {
	f := taglibTempFile(t, "xing.mp3")
	defer f.Close()
	tag := NewTag(TagTypeID3v2)
	tag.SetArtist("Foo")
	wo := NewWriteOptions()
	wo.UseID3v23 = true
	if err := tag.SaveTo(NewOSFile(f), wo); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	// No UTF-8 (encoding 3) may remain in an ID3v2.3 frame; text must be
	// downgraded to UTF-16 (encoding 1).
	idx := bytes.Index(content, []byte("TPE1"))
	if idx < 0 {
		t.Fatal("TPE1 missing")
	}
	if content[idx+10] != 1 {
		t.Fatalf("TPE1 encoding = %d, want 1 (UTF-16)", content[idx+10])
	}
	// USLT likewise
	if idx := bytes.Index(content, []byte("USLT")); idx >= 0 && content[idx+10] == 3 {
		t.Fatal("USLT still UTF-8")
	}
}

func TestTaglibId3v2ParseApicV22(t *testing.T) {
	// ID3v2.2 PIC frame: enc=0, "JPG" format, type=1, desc "d", data \x00
	picBody := []byte{0x00, 'J', 'P', 'G', 0x01, 'd', 0x00, 0x00}
	var frame []byte
	frame = append(frame, 'P', 'I', 'C')
	frame = append(frame, 0, 0, byte(len(picBody)))
	frame = append(frame, picBody...)
	tag := append([]byte("ID3"), 2, 0, 0)
	tag = append(tag, syncsafe28(len(frame))...)
	tag = append(tag, frame...)

	parsed := parseRawId3v2(t, tag)
	pics := parsed.Pictures()
	if len(pics) != 1 {
		t.Fatalf("pictures = %d", len(pics))
	}
	p := pics[0]
	if mt, ok := p.MimeType().Get(); !ok || mt != MimeTypeJpeg {
		t.Fatalf("mime = %v", p.MimeType())
	}
	if p.PicType() != PictureTypeIcon {
		t.Fatalf("pic_type = %v", p.PicType())
	}
	if d, ok := p.Description(); !ok || d != "d" {
		t.Fatalf("desc = %v", d)
	}
}

// --- tests/tags/conversions.rs ---

func TestTaglibTagToId3v2LangFrame(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyUnsyncLyrics, ItemValueText("Test lyrics")))
	tag.PushUnchecked(NewTagItem(ItemKeyComment, ItemValueText("Test comment")))

	btag, err := genericTagToBogem(tag, false)
	if err != nil {
		t.Fatal(err)
	}
	uslt := btag.GetFrames("USLT")
	if len(uslt) != 1 {
		t.Fatalf("USLT frames = %d", len(uslt))
	}
	lf, ok := uslt[0].(bogem.UnsynchronisedLyricsFrame)
	if !ok {
		t.Fatalf("USLT type = %T", uslt[0])
	}
	if lf.Language != "XXX" || lf.ContentDescriptor != "" || lf.Lyrics != "Test lyrics" {
		t.Fatalf("USLT = %+v", lf)
	}
	comm := btag.GetFrames("COMM")
	if len(comm) != 1 {
		t.Fatalf("COMM frames = %d", len(comm))
	}
	cf, ok := comm[0].(bogem.CommentFrame)
	if !ok {
		t.Fatalf("COMM type = %T", comm[0])
	}
	if cf.Language != "XXX" || cf.Description != "" || cf.Text != "Test comment" {
		t.Fatalf("COMM = %+v", cf)
	}
}
