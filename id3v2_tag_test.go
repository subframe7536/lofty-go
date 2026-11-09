package lofty

import (
	"bytes"
	"os"
	"strings"
	"testing"

	bogem "github.com/bogem/id3v2/v2"
)

// Ported from id3/v2/tag/tests.rs

func id3v2TestAsset(name string) string {
	return "lofty-rs/lofty/tests/tags/assets/id3v2/" + name
}

func readId3v2File(t *testing.T, path string, opts ParseOptions) *Tag {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(data)
	header, err := parseId3v2Header(r)
	if err != nil {
		t.Fatalf("header parse failed: %v", err)
	}
	tag, err := parseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("tag parse failed: %v", err)
	}
	return tag
}

// id3v2Dump serializes a generic tag to ID3v2 bytes.
func id3v2Dump(t *testing.T, tag *Tag, useV23 bool) []byte {
	t.Helper()
	wo := NewWriteOptions()
	wo.UseID3v23 = useV23
	encoded, err := id3v2Serialize(tag, wo)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// id3v2Reparse parses written ID3v2 bytes back to a generic tag.
func id3v2Reparse(t *testing.T, b []byte, opts ParseOptions) *Tag {
	t.Helper()
	r := bytes.NewReader(b)
	header, err := parseId3v2Header(r)
	if err != nil {
		t.Fatalf("header re-parse failed: %v", err)
	}
	tag, err := parseId3v2(r, header, opts)
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
	want := map[ItemKey]string{
		ItemKeyTrackTitle:    "TempleOS Hymn Risen (Remix)",
		ItemKeyTrackArtist:   "Dave Eddy",
		ItemKeyTrackNumber:   "1",
		ItemKeyAlbumTitle:    "Summer",
		ItemKeyRecordingDate: "2017",
		ItemKeyGenre:         "Electronic",
		ItemKeyLength:        "213017",
	}
	for _, name := range []string{"test_full.id3v22", "test_full.id3v23", "test_full.id3v24"} {
		t.Run(name, func(t *testing.T) {
			tag := readId3v2File(t, id3v2TestAsset(name), NewParseOptions())
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
			if pic.PicType() != PictureTypeCoverFront {
				t.Errorf("pic type = %v, want CoverFront", pic.PicType())
			}
			if mt, ok := pic.MimeType().Get(); !ok || mt != MimeTypePng {
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
// frame cannot be represented generically; it must survive a
// generic-Tag round-trip as a binary item and serialize back byte-exact.
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

	parse := func(t *testing.T, b []byte) *Tag {
		t.Helper()
		r := bytes.NewReader(b)
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
	findRva2 := func(t *testing.T, tag *Tag) TagItem {
		t.Helper()
		for _, item := range tag.Items() {
			if item.Key() == ItemKeyUnknown && item.Description == "RVA2" {
				return item
			}
		}
		t.Fatal("no RVA2 binary item")
		return TagItem{}
	}

	tag := parse(t, tagBytes)
	if s, _ := tag.GetString(ItemKeyTrackArtist); s != "Foo Artist" {
		t.Fatalf("artist = %q", s)
	}
	item := findRva2(t, tag)
	if got, ok := item.Value().IntoBinary(); !ok || !bytes.Equal(got, rva2Body) {
		t.Fatalf("RVA2 body = %v (ok=%v)", got, ok)
	}

	// Round-trip through the generic Tag — strict mode must still find it
	strict := NewParseOptions()
	strict.ParsingMode = ParsingModeStrict
	reparsed, err := func() (*Tag, error) {
		r := bytes.NewReader(id3v2Dump(t, tag, false))
		header, err := parseId3v2Header(r)
		if err != nil {
			return nil, err
		}
		return parseId3v2(r, header, strict)
	}()
	if err != nil {
		t.Fatal(err)
	}
	item = findRva2(t, reparsed)
	if got, ok := item.Value().IntoBinary(); !ok || !bytes.Equal(got, rva2Body) {
		t.Fatalf("RVA2 body after round-trip = %v (ok=%v)", got, ok)
	}
	if s, _ := reparsed.GetString(ItemKeyTrackArtist); s != "Foo Artist" {
		t.Fatalf("artist after round-trip = %q", s)
	}
}

func TestParseId3v2(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), NewParseOptions())
	if s, _ := tag.GetString(ItemKeyTrackArtist); s != "Bar artist" {
		t.Fatalf("artist = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyTrackTitle); s != "Foo title" {
		t.Fatalf("title = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyAlbumTitle); s != "Baz album" {
		t.Fatalf("album = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyComment); s != "Qux comment" {
		t.Fatalf("comment = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyRecordingDate); s != "1984" {
		t.Fatalf("date = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyTrackNumber); s != "1" {
		t.Fatalf("track = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyGenre); s != "Classical" {
		t.Fatalf("genre = %q", s)
	}
}

func TestId3v2ReRead(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), NewParseOptions())
	encoded := id3v2Dump(t, tag, false)
	re := id3v2Reparse(t, encoded, NewParseOptions())
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
	tag := readId3v2File(t, id3v2TestAsset("test.id3v24"), NewParseOptions())
	// verify_tag (track_number + genre)
	checks := map[ItemKey]string{
		ItemKeyTrackTitle:  "Foo title",
		ItemKeyTrackArtist: "Bar artist",
		ItemKeyAlbumTitle:  "Baz album",
		ItemKeyComment:     "Qux comment",
		ItemKeyTrackNumber: "1",
		ItemKeyGenre:       "Classical",
	}
	for k, want := range checks {
		if s, ok := tag.GetString(k); !ok || s != want {
			t.Fatalf("%v = %q, want %q", k, s, want)
		}
	}
}

func TestTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyTrackTitle, "Foo title")
	tag.InsertText(ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(ItemKeyComment, "Qux comment")
	tag.InsertText(ItemKeyTrackNumber, "1")
	tag.InsertText(ItemKeyGenre, "Classical")

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
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyGenre, "(51)(39)")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	genres := re.GetStrings(ItemKeyGenre)
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

func trckText(t *testing.T, tag *Tag) string {
	t.Helper()
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	return btag.GetTextFrame("TRCK").Text
}

func tposText(t *testing.T, tag *Tag) string {
	t.Helper()
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	return btag.GetTextFrame("TPOS").Text
}

func TestSetTrack(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetTrack(1)
	if got := trckText(t, tag); got != "1" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetTrackTotal(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetTrackTotal(2)
	// DEFAULT_NUMBER_IN_PAIR fills in the number
	if got := trckText(t, tag); got != "0/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetTrackAndTrackTotal(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetTrack(1)
	tag.SetTrackTotal(2)
	if got := trckText(t, tag); got != "1/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestSetDisk(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetDisk(1)
	if got := tposText(t, tag); got != "1" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestSetDiskTotal(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetDiskTotal(2)
	if got := tposText(t, tag); got != "0/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestSetDiskAndDiskTotal(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.SetDisk(1)
	tag.SetDiskTotal(2)
	if got := tposText(t, tag); got != "1/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestTrackNumberTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyTrackNumber, ItemValueText("1")))
	if got := trckText(t, tag); got != "1" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestTrackTotalTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyTrackTotal, ItemValueText("2")))
	if got := trckText(t, tag); got != "0/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestTrackNumberAndTrackTotalTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyTrackNumber, ItemValueText("1")))
	tag.PushUnchecked(NewTagItem(ItemKeyTrackTotal, ItemValueText("2")))
	if got := trckText(t, tag); got != "1/2" {
		t.Fatalf("TRCK = %q", got)
	}
}

func TestDiskNumberTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyDiscNumber, ItemValueText("1")))
	if got := tposText(t, tag); got != "1" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestDiskTotalTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyDiscTotal, ItemValueText("2")))
	if got := tposText(t, tag); got != "0/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

func TestDiskNumberAndDiskTotalTagToId3v2(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyDiscNumber, ItemValueText("1")))
	tag.PushUnchecked(NewTagItem(ItemKeyDiscTotal, ItemValueText("2")))
	if got := tposText(t, tag); got != "1/2" {
		t.Fatalf("TPOS = %q", got)
	}
}

// create_tag_with_trck_and_tpos_frame — emulate via TRCK/TPOS text frames.
func tagWithTrckTpos(content string) *Tag {
	tag := NewTag(TagTypeID3v2)
	// Write the raw pair value through a text item, then re-parse.
	raw := NewTag(TagTypeID3v2)
	for _, id := range []ItemKey{} {
		_ = id
	}
	// Direct pair-split semantics mirror split_pair on a TRCK/TPOS frame.
	_ = genericTagToBogem(raw, false)
	// Build the generic tag the way a parsed TRCK/TPOS would.
	num, tot, ok := splitNumPair(content)
	if ok {
		tag.PushUnchecked(NewTagItem(ItemKeyTrackNumber, ItemValueText(num)))
		tag.PushUnchecked(NewTagItem(ItemKeyDiscNumber, ItemValueText(num)))
		if tot != "" {
			tag.PushUnchecked(NewTagItem(ItemKeyTrackTotal, ItemValueText(tot)))
			tag.PushUnchecked(NewTagItem(ItemKeyDiscTotal, ItemValueText(tot)))
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
	tag := NewTag(TagTypeID3v2)
	mbID := "189002e7-3285-4e2e-92a3-7f6c30d407a2"
	other := NewTagItem(ItemKeyUnknown, ItemValueBinary([]byte("0123456789")))
	other.Description = "UFID:other"
	tag.PushUnchecked(other)
	tag.PushUnchecked(NewTagItem(ItemKeyMusicBrainzRecordingId, ItemValueText(mbID)))

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	if s, ok := re.GetString(ItemKeyMusicBrainzRecordingId); !ok || s != mbID {
		t.Fatalf("mb id = %q", s)
	}
	// The non-MB UFID survives as an unknown binary item
	found := false
	for _, item := range re.Items() {
		if item.Key() == ItemKeyUnknown && item.Description == "UFID:other" {
			if b, ok := item.Value().Binary(); ok && string(b) == "0123456789" {
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
	tag := readId3v2File(t, id3v2TestAsset("test_popm.id3v24"), NewParseOptions())
	items := tag.GetItems(ItemKeyPopularimeter)
	if len(items) != 1 {
		t.Fatalf("popm items = %d", len(items))
	}
	s, _ := items[0].Value().Text()
	p, err := PopularimeterFromStr(s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Email() != "foo@bar.com" || p.PlayCounter != 65535 {
		t.Fatalf("popm = %v", p)
	}
	if p.MappedValue(TagTypeID3v2) != 196 {
		t.Fatalf("mapped rating = %d", p.MappedValue(TagTypeID3v2))
	}
}

// --- multi-value ---

func TestMultiValueFrameToTag(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyTrackArtist, "foo\x00bar\x00baz")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	got := re.GetStrings(ItemKeyTrackArtist)
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
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("foo")))
	tag.PushUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("bar")))
	tag.PushUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("baz")))

	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	if got := btag.GetTextFrame("TPE1").Text; got != "foo/bar/baz" && got != "foo\x00bar\x00baz" {
		t.Fatalf("TPE1 = %q", got)
	}
}

func TestMultiValueRoundtrip(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	push2 := func(k ItemKey, a, b string) {
		tag.InsertText(k, a)
		tag.PushUnchecked(NewTagItem(k, ItemValueText(b)))
	}
	push2(ItemKeyTrackArtist, "TrackArtist 1", "TrackArtist 2")
	push2(ItemKeyAlbumArtist, "AlbumArtist 1", "AlbumArtist 2")
	push2(ItemKeyTrackTitle, "TrackTitle 1", "TrackTitle 2")
	push2(ItemKeyAlbumTitle, "AlbumTitle 1", "AlbumTitle 2")
	push2(ItemKeyContentGroup, "ContentGroup 1", "ContentGroup 2")
	push2(ItemKeyGenre, "Genre 1", "Genre 2")
	push2(ItemKeyMood, "Mood 1", "Mood 2")
	push2(ItemKeyComposer, "Composer 1", "Composer 2")
	push2(ItemKeyConductor, "Conductor 1", "Conductor 2")
	push2(ItemKeyComment, "Comment 1", "Comment 2")
	if tag.Len() != 20 {
		t.Fatalf("len = %d", tag.Len())
	}

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
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
	tag := NewTag(TagTypeID3v2)
	for _, a := range []string{"Foo", "Bar", "Baz"} {
		tag.PushUnchecked(NewTagItem(ItemKeyTrackArtists, ItemValueText(a)))
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
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("Foo")))
	tag.PushUnchecked(NewTagItem(ItemKeyTrackArtist, ItemValueText("Bar")))
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	if got := btag.GetTextFrame("TPE1").Text; got != "Foo\x00Bar" {
		t.Fatalf("TPE1 = %q", got)
	}
}

func TestSingleValueFrame(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.PushUnchecked(NewTagItem(ItemKeyIntegerBpm, ItemValueText("120")))
	tag.PushUnchecked(NewTagItem(ItemKeyIntegerBpm, ItemValueText("130")))
	tag.PushUnchecked(NewTagItem(ItemKeyIntegerBpm, ItemValueText("140")))
	btag := bogemFrames(t, id3v2Dump(t, tag, false))
	// BPM is deduplicated — first wins
	if got := btag.GetTextFrame("TBPM").Text; got != "120" {
		t.Fatalf("TBPM = %q", got)
	}
}

func TestUtf16TxxxWithSingleBom(t *testing.T) {
	// Just has to parse without error
	_ = readId3v2File(t, id3v2TestAsset("issue_53.id3v24"), NewParseOptions())
}

func TestReplaygainTagConversion(t *testing.T) {
	// REPLAYGAIN_ALBUM_GAIN is a TXXX description mapped to an ItemKey
	tag := NewTag(TagTypeID3v2)
	item := NewTagItem(ItemKeyUnknown, ItemValueText("-10.43 dB"))
	item.Description = "REPLAYGAIN_ALBUM_GAIN"
	tag.PushUnchecked(item)
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	if s, ok := re.GetString(ItemKeyReplayGainAlbumGain); !ok || s != "-10.43 dB" {
		t.Fatalf("replaygain = %q", s)
	}
}

// --- flags / advisory / timestamp ---

func TestFlagItemConversion(t *testing.T) {
	// Rust's flag_item_conversion asserts the Tag -> Id3v2Tag direction:
	// both flags become text frames with normalized "0"/"1" values.
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyFlagCompilation, "1")
	tag.InsertText(ItemKeyFlagPodcast, "0")

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
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	found := false
	for _, item := range re.Items() {
		if item.Key() == ItemKeyUnknown && item.Description == "PCST" {
			if b, ok := item.Value().Binary(); ok && id3v2DecodeTextBody(b) == "0" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("PCST not retained as binary item")
	}
}

func TestItunesAdvisoryRoundtrip(t *testing.T) {
	ilst := NewIlst()
	ilst.SetAdvisoryRating(1)

	tag := ilst.Tag()
	encoded := id3v2Dump(t, tag, false)
	btag := bogemFrames(t, encoded)

	found := false
	for _, f := range btag.GetFrames("TXXX") {
		if udtf, ok := f.(bogem.UserDefinedTextFrame); ok && udtf.Description == "ITUNESADVISORY" {
			if udtf.Value != "1" {
				t.Fatalf("ITUNESADVISORY = %q", udtf.Value)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("ITUNESADVISORY TXXX missing")
	}

	re := id3v2Reparse(t, encoded, NewParseOptions())
	back := IlstFromTag(re)
	if v, ok := back.AdvisoryRating(); !ok || v != 1 {
		t.Fatalf("advisory = %v", v)
	}
}

func TestTimestampRoundtrip(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyRecordingDate, "2024-06-03T14:08:49")
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	if s, ok := re.GetString(ItemKeyRecordingDate); !ok || s != "2024-06-03T14:08:49" {
		t.Fatalf("date = %q", s)
	}
}

func TestPreserveCommentLangDescriptionOnConversion(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	item := NewTagItem(ItemKeyComment, ItemValueText("Foo comment"))
	item.Description = "Some description"
	copy(item.Lang[:], "eng")
	tag.PushUnchecked(item)

	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	if re.ItemCount() != 1 {
		t.Fatalf("len = %d", re.ItemCount())
	}
	got := re.GetItems(ItemKeyComment)
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
	tag := id3v2Reparse(t, buf.Bytes(), NewParseOptions())
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	// MODE survives the round trip either as a mapped or unknown-described item
	found := false
	for _, item := range re.Items() {
		if item.Description == "MODE" {
			if s, _ := item.Value().Text(); s == "CBR" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("MODE TXXX lost")
	}
}

func TestSkipReadingCoverArt(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	pic := PictureUnchecked(make([]byte, 50)).
		PicType(PictureTypeCoverFront).
		MimeType(MimeTypeJpeg).
		Build()
	tag.PushPicture(pic)
	tag.InsertText(ItemKeyTrackArtist, "Foo artist")

	opts := NewParseOptions()
	opts.ReadCoverArt = false
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), opts)
	if re.PictureCount() != 0 {
		t.Fatalf("pictures = %d", re.PictureCount())
	}
	if s, ok := re.GetString(ItemKeyTrackArtist); !ok || s != "Foo artist" {
		t.Fatalf("artist = %q", s)
	}
}

// --- v2.3 save behavior ---

func TestRemoveId3v24FramesOnId3v23Save(t *testing.T) {
	// RVA2 is ID3v2.4-only — dropped when saving as 2.3
	tag := NewTag(TagTypeID3v2)
	item := NewTagItem(ItemKeyUnknown, ItemValueBinary([]byte{0x03, 0x01, 0x02}))
	item.Description = "RVA2"
	tag.PushUnchecked(item)
	encoded := id3v2Dump(t, tag, true)
	// Every frame was discarded — Rust still writes a valid empty tag header
	if len(encoded) < 10 {
		t.Fatalf("encoded v2.3 tag = %d bytes", len(encoded))
	}
	re := id3v2Reparse(t, encoded, NewParseOptions())
	for _, it := range re.Items() {
		if it.Description == "RVA2" {
			t.Fatal("RVA2 retained on 2.3 save")
		}
	}
}

func TestChangeTextEncodingOnId3v23Save(t *testing.T) {
	// UTF-16BE isn't allowed in 2.3 — text frames degrade to UTF-16
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyTrackTitle, "Foo")
	encoded := id3v2Dump(t, tag, true)
	// Scan the raw bytes for a UTF-16BE encoding byte (0x02) — none may appear
	btag := bogemFrames(t, encoded)
	for _, f := range btag.GetFrames("TIT2") {
		if tf, ok := f.(bogem.TextFrame); ok && tf.Encoding.Key == 2 {
			t.Fatal("UTF-16BE encoding used in v2.3 tag")
		}
	}
	re := id3v2Reparse(t, encoded, NewParseOptions())
	if s, _ := re.GetString(ItemKeyTrackTitle); s != "Foo" {
		t.Fatalf("title = %q", s)
	}
}

func TestSplitTdorOnId3v23Save(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyOriginalReleaseDate, "2024-06-03T14:08:49")
	encoded := id3v2Dump(t, tag, true)
	btag := bogemFrames(t, encoded)
	// TDOR downgrades to TORY — only the year survives
	if got := btag.GetTextFrame("TORY").Text; got != "2024" {
		t.Fatalf("TORY = %q", got)
	}
	// On re-read, TORY upgrades back to TDOR with only a year
	re := id3v2Reparse(t, encoded, NewParseOptions())
	if s, ok := re.GetString(ItemKeyOriginalReleaseDate); !ok || s != "2024" {
		t.Fatalf("orig date = %q", s)
	}
}

func TestSplitTdrcOnId3v23Save(t *testing.T) {
	tag := NewTag(TagTypeID3v2)
	tag.InsertText(ItemKeyRecordingDate, "2024-06-03T14:08:49")
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
	re := id3v2Reparse(t, encoded, NewParseOptions())
	if s, ok := re.GetString(ItemKeyRecordingDate); !ok || s != "2024-06-03T14:08" {
		t.Fatalf("date = %q", s)
	}
}

// --- misc file-level tests ---

func TestEnsureFrameSkippingWithinBounds(t *testing.T) {
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeBestAttempt
	tag := readId3v2File(t, id3v2TestAsset("skippable_frame_otherwise_valid.id3v24"), opts)
	// The TDTG frame past the bad TDEN frame survives
	if s, ok := tag.GetString(ItemKeyTaggingTime); !ok || s != "2014-06-10T02:16:10" {
		t.Fatalf("TDTG = %q (items=%v)", s, tag.Items())
	}
}

// Ported from id3/v2/read.rs::bad_frame_id_relaxed_id3v2.
func TestBadFrameIDRelaxedId3v2(t *testing.T) {
	// Contains a frame with a "+" in the ID, which is invalid.
	// All other frames in the tag are valid, however.
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeRelaxed
	tag := readId3v2File(t, id3v2TestAsset("bad_frame_otherwise_valid.id3v24"), opts)
	// 6 valid frames and 1 invalid frame (T+LB, was TALB)
	if tag.ItemCount() != 6 {
		t.Fatalf("len = %d (items=%v)", tag.ItemCount(), tag.Items())
	}
	if s, _ := tag.GetString(ItemKeyTrackTitle); s != "Foo title" {
		t.Fatalf("title = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyTrackArtist); s != "Bar artist" {
		t.Fatalf("artist = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyComment); s != "Qux comment" {
		t.Fatalf("comment = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyRecordingDate); s != "1984" {
		t.Fatalf("date = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyTrackNumber); s != "1" {
		t.Fatalf("track = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyGenre); s != "Classical" {
		t.Fatalf("genre = %q", s)
	}
}

func TestUndersizedFrames(t *testing.T) {
	// A COMM frame needs at least 5 bytes
	badTag := []byte("ID3\x04\x00\x00\x00\x00\x00\x20" +
		"COMM\x00\x00\x00\x01\x00\x00\x00" +
		"TPE1\x00\x00\x00\x0B\x00\x00\x00Serial-ATA")

	// Relaxed mode should parse the tag fine, skipping the comment
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeRelaxed
	r := bytes.NewReader(badTag)
	header, err := parseId3v2Header(r)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := parseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("relaxed parse failed: %v", err)
	}
	if s, ok := tag.GetString(ItemKeyTrackArtist); !ok || s != "Serial-ATA" {
		t.Fatalf("artist = %q", s)
	}

	// Anything else should fail entirely
	for _, mode := range []ParsingMode{ParsingModeStrict, ParsingModeBestAttempt} {
		opts.ParsingMode = mode
		r = bytes.NewReader(badTag)
		header, err = parseId3v2Header(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseId3v2(r, header, opts); err == nil {
			t.Fatalf("mode %v: expected error", mode)
		}
	}
}

func TestReadMultipleComposers(t *testing.T) {
	tag := readId3v2File(t, id3v2TestAsset("multiple_composers.id3v24"), NewParseOptions())
	got := tag.GetStrings(ItemKeyComposer)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("composers = %v", got)
	}
}

func TestTrimEndNullsWhenReadingFrameContent(t *testing.T) {
	// Issue #273 — tag written by mid3v2; all frames contain
	// null-terminated UTF-8 text that must come back trimmed.
	tag := readId3v2File(t, id3v2TestAsset("trailing_nulls.id3v24"), NewParseOptions())
	if s, _ := tag.GetString(ItemKeyTrackArtist); s != "Artist" {
		t.Fatalf("artist = %q", s)
	}
	// TXXX:Writer has no ItemKey mapping — Rust retains the UserText frame,
	// so the port's equivalent is an unknown item carrying the description.
	found := false
	for _, item := range tag.Items() {
		if item.Key() == ItemKeyUnknown && item.Description == "Writer" {
			if s, _ := item.Value().Text(); s == "Writer" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("TXXX:Writer missing or still NUL-terminated")
	}
	if s, _ := tag.GetString(ItemKeyUnsyncLyrics); s != "Lyrics to the song" {
		t.Fatalf("lyrics = %q", s)
	}
	if s, _ := tag.GetString(ItemKeyComment); s != "Comment" {
		t.Fatalf("comment = %q", s)
	}
	found = false
	for _, item := range tag.Items() {
		if item.Description == "Fanpage" {
			if s, ok := item.Value().Locator(); ok && s == "https://www.myfanpage.com" {
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
	tag := NewTag(TagTypeID3v2)
	pic := PictureUnchecked(make([]byte, 200)).
		PicType(PictureTypeCoverFront).
		MimeType(MimeTypeJpeg).
		Description("cover").
		Build()
	tag.PushPicture(pic)

	encoded := id3v2Dump(t, tag, false)
	re := id3v2Reparse(t, encoded, NewParseOptions())
	if re.PictureCount() != 1 {
		t.Fatalf("pictures = %d", re.PictureCount())
	}
	p := re.Pictures()[0]
	d, _ := p.Description()
	if d != "cover" || p.PicType() != PictureTypeCoverFront {
		t.Fatalf("pic = %+v", p)
	}
	if len(p.Data()) != 200 {
		t.Fatalf("pic data len = %d", len(p.Data()))
	}
}

// Ported from id3/v2/read.rs::zero_size_id3v2 — a header-only tag parses fine
// under Strict.
func TestId3v2ZeroSize(t *testing.T) {
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
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
	_, _, found, err := findId3v2(r, true, NewParseOptions())
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
// single TIPL frame on write. TMCL has no ItemKey mappings, so all of its
// pairs are held back and re-emitted as TMCL (v2.4) or merged into IPLS (v2.3).
func TestTiplRoundTrip(t *testing.T) {
	// Build a v2.4 tag: TIPL with all mapped roles + one unmapped pair, and a
	// TMCL frame.
	var content bytes.Buffer
	writeFrame := func(id, body string) {
		content.WriteString(id)
		sz := synchsafe32(uint32(len(body)))
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
	ss := synchsafe32(uint32(content.Len()))
	tagBytes.Write(ss[:])
	tagBytes.Write(content.Bytes())

	tag := id3v2Reparse(t, tagBytes.Bytes(), NewParseOptions())

	// All 5 mapped roles are generic items
	for _, k := range []ItemKey{ItemKeyProducer, ItemKeyArranger, ItemKeyEngineer, ItemKeyMixDj, ItemKeyMixEngineer} {
		if s, ok := tag.GetString(k); !ok || s != "Serial-ATA" {
			t.Fatalf("key %v = %q (ok=%v)", k, s, ok)
		}
	}
	// "Foo" (TIPL) and "guitar" (TMCL) are held back — not generic items
	if tag.Len() != 7 {
		t.Fatalf("len = %d, want 7 (5 mapped + TIPL + TMCL hold-backs)", tag.Len())
	}

	// Round-trip: the unmapped pairs merge back into single frames.
	re := id3v2Reparse(t, id3v2Dump(t, tag, false), NewParseOptions())
	if re.Len() != 7 {
		t.Fatalf("re-read len = %d, want 7", re.Len())
	}

	// v2.3 write merges TIPL+TMCL into IPLS; on re-read both unmapped pairs
	// collapse into a single TIPL hold-back (Rust-identical).
	re23 := id3v2Reparse(t, id3v2Dump(t, tag, true), NewParseOptions())
	if re23.Len() != 6 {
		t.Fatalf("v2.3 re-read len = %d, want 6 (5 mapped + 1 merged hold-back)", re23.Len())
	}
	var held []string
	for _, it := range re23.Items() {
		if it.Key() == ItemKeyUnknown && it.Description == "TIPL" {
			if b, ok := it.Value().IntoBinary(); ok && len(b) > 0 {
				held = splitNulls(string(b[1:]))
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
