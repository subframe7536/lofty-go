package lofty

// Ports of lofty-rs src/mp4/ilst/tests.rs.

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func readIlstAsset(t *testing.T, name string, po ParseOptions) *Ilst {
	t.Helper()
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/ilst/" + name)
	if err != nil {
		t.Fatal(err)
	}
	ilst, err := parseIlst(bytes.NewReader(data), int64(len(data)), po)
	if err != nil {
		t.Fatal(err)
	}
	return ilst
}

func readIlstAssetStrict(t *testing.T, name string) *Ilst {
	t.Helper()
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	return readIlstAsset(t, name, po)
}

func ilstAtomText(t *testing.T, ilst *Ilst, fourcc, want string) {
	t.Helper()
	a := ilst.Get(FourccIdent(fourcc))
	if a == nil || len(a.Data) == 0 {
		t.Fatalf("no atom %q", fourcc)
	}
	d := a.Data[0]
	if d.Kind != AtomDataUTF8 && d.Kind != AtomDataUTF16 {
		t.Fatalf("%q kind = %v, want text", fourcc, d.Kind)
	}
	if d.Text != want {
		t.Fatalf("%q = %q, want %q", fourcc, d.Text, want)
	}
}

func TestIlstParse(t *testing.T) {
	ilst := readIlstAssetStrict(t, "test.ilst")
	ilstAtomText(t, ilst, "©ART", "Bar artist")
	ilstAtomText(t, ilst, "©alb", "Baz album")
	ilstAtomText(t, ilst, "©cmt", "Qux comment")
	ilstAtomText(t, ilst, "©day", "1984")
	ilstAtomText(t, ilst, "©gen", "Classical")
	ilstAtomText(t, ilst, "©nam", "Foo title")

	trkn := ilst.Get(FourccIdent("trkn"))
	if trkn == nil || len(trkn.Data) == 0 {
		t.Fatal("no trkn")
	}
	if d := trkn.Data[0]; d.Kind != AtomDataUnknown || !bytes.Equal(d.Data, []byte{0, 0, 0, 1, 0, 0, 0, 0}) {
		t.Fatalf("trkn data = %+v", d)
	}
	disk := ilst.Get(FourccIdent("disk"))
	if disk == nil || len(disk.Data) == 0 {
		t.Fatal("no disk")
	}
	if d := disk.Data[0]; d.Kind != AtomDataUnknown || !bytes.Equal(d.Data, []byte{0, 0, 0, 1, 0, 2}) {
		t.Fatalf("disk data = %+v", d)
	}
}

func TestIlstReRead(t *testing.T) {
	ilst := readIlstAssetStrict(t, "test.ilst")
	var buf bytes.Buffer
	if err := ilst.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	reParsed, err := parseIlst(bytes.NewReader(buf.Bytes()[8:]), int64(buf.Len()-8), po)
	if err != nil {
		t.Fatal(err)
	}
	if reParsed.Len() != ilst.Len() {
		t.Fatalf("len = %d, want %d", reParsed.Len(), ilst.Len())
	}
	for _, a := range ilst.Atoms() {
		ra := reParsed.Get(a.Ident)
		if ra == nil || len(ra.Data) != len(a.Data) {
			t.Fatalf("atom %+v missing after re-read", a.Ident)
		}
	}
}

func TestIlstToTag(t *testing.T) {
	ilst := readIlstAssetStrict(t, "test.ilst")
	tag := ilst.Tag()
	verifyTagCommon(t, tag, true, true)
	if v, _ := tag.GetString(ItemKeyDiscNumber); v != "1" {
		t.Fatalf("disc = %q", v)
	}
	if v, _ := tag.GetString(ItemKeyDiscTotal); v != "2" {
		t.Fatalf("disc total = %q", v)
	}
}

func TestTagToIlst(t *testing.T) {
	tag := createTagCommon(TagTypeMp4Ilst)
	tag.InsertText(ItemKeyDiscNumber, "1")
	tag.InsertText(ItemKeyDiscTotal, "2")
	ilst := IlstFromTag(tag)
	ilstAtomText(t, ilst, "©nam", "Foo title")
	ilstAtomText(t, ilst, "©ART", "Bar artist")
	ilstAtomText(t, ilst, "©alb", "Baz album")
	ilstAtomText(t, ilst, "©cmt", "Qux comment")
	ilstAtomText(t, ilst, "©gen", "Classical")

	trkn := ilst.Get(FourccIdent("trkn"))
	if trkn == nil || !bytes.Equal(trkn.Data[0].Data, []byte{0, 0, 0, 1, 0, 0, 0, 0}) {
		t.Fatalf("trkn = %+v", trkn)
	}
	disk := ilst.Get(FourccIdent("disk"))
	if disk == nil || !bytes.Equal(disk.Data[0].Data, []byte{0, 0, 0, 1, 0, 2, 0, 0}) {
		t.Fatalf("disk = %+v", disk)
	}
}

func TestIlstIssue34(t *testing.T) {
	ilst := readIlstAssetStrict(t, "issue_34.ilst")
	ilstAtomText(t, ilst, "©ART", "Foo artist")
	a := ilst.Get(FourccIdent("plID"))
	if a == nil || len(a.Data) == 0 {
		t.Fatal("no plID")
	}
	var want [8]byte
	binary.BigEndian.PutUint64(want[:], 88888)
	d := a.Data[0]
	if d.Kind != AtomDataUnknown || d.Code != 21 || !bytes.Equal(d.Data, want[:]) {
		t.Fatalf("plID = %+v", d)
	}
}

func TestIlstAdvisoryRating(t *testing.T) {
	ilst := readIlstAssetStrict(t, "advisory_rating.ilst")
	ilstAtomText(t, ilst, "©ART", "Foo artist")
	if r, ok := ilst.AdvisoryRating(); !ok || r != 1 {
		t.Fatalf("rating = %v, want Explicit (1)", r)
	}
}

func TestIlstMultiValueAtom(t *testing.T) {
	ilst := readIlstAssetStrict(t, "multi_value_atom.ilst")
	a := ilst.Get(FourccIdent("©ART"))
	if a == nil || len(a.Data) != 2 {
		t.Fatalf("©ART = %+v", a)
	}
	if a.Data[0].Text != "Foo artist" || a.Data[1].Text != "Bar artist" {
		t.Fatalf("©ART = %v", a.Data)
	}
	ilstAtomText(t, ilst, "©gen", "Classical")
}

func TestIlstMultiValueRoundtrip(t *testing.T) {
	tag := NewTag(TagTypeMp4Ilst)
	push2 := func(k ItemKey, v1, v2 string) {
		tag.InsertText(k, v1)
		tag.PushUnchecked(NewTagItem(k, ItemValueText(v2)))
	}
	push2(ItemKeyTrackArtist, "TrackArtist 1", "TrackArtist 2")
	push2(ItemKeyAlbumArtist, "AlbumArtist 1", "AlbumArtist 2")
	push2(ItemKeyTrackTitle, "TrackTitle 1", "TrackTitle 2")
	push2(ItemKeyAlbumTitle, "AlbumTitle 1", "AlbumTitle 2")
	push2(ItemKeyComment, "Comment 1", "Comment 2")
	push2(ItemKeyContentGroup, "ContentGroup 1", "ContentGroup 2")
	push2(ItemKeyGenre, "Genre 1", "Genre 2")
	push2(ItemKeyMood, "Mood 1", "Mood 2")
	push2(ItemKeyComposer, "Composer 1", "Composer 2")
	push2(ItemKeyConductor, "Conductor 1", "Conductor 2")
	if tag.Len() != 20 {
		t.Fatalf("len = %d", tag.Len())
	}
	ilst := IlstFromTag(tag)
	back := ilst.Tag()
	if back.Len() != tag.Len() {
		t.Fatalf("len = %d, want %d", back.Len(), tag.Len())
	}
	if back.HasFormatSpecificItems() {
		t.Fatal("unexpected remainder")
	}
}

func TestIlstMergeInsert(t *testing.T) {
	ilst := NewIlst()
	ilst.SetTitle("Foo")
	ilst.Insert(Atom{FourccIdent("©nam"), []AtomData{{Kind: AtomDataUTF8, Text: "Bar"}}})
	if v, _ := ilst.Title(); v != "Foo" {
		t.Fatalf("title = %q", v)
	}
	if a := ilst.Get(FourccIdent("©nam")); a == nil || len(a.Data) != 2 {
		t.Fatalf("©nam data = %+v", a)
	}
	if ilst.Len() != 1 {
		t.Fatalf("len = %d", ilst.Len())
	}
}

func TestIlstInvalidAtomType(t *testing.T) {
	ilst := readIlstAssetStrict(t, "invalid_atom_type.ilst")
	if ilst.Len() != 2 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if v, _ := ilst.Track(); v != 1 {
		t.Fatalf("track = %d", v)
	}
	if v, _ := ilst.TrackTotal(); v != 0 {
		t.Fatalf("track total = %d", v)
	}
	if v, _ := ilst.Disk(); v != 1 {
		t.Fatalf("disk = %d", v)
	}
	if v, _ := ilst.DiskTotal(); v != 2 {
		t.Fatalf("disk total = %d", v)
	}
}

func TestIlstInvalidStringEncoding(t *testing.T) {
	po := NewParseOptions()
	po.ParsingMode = ParsingModeBestAttempt
	ilst := readIlstAsset(t, "invalid_string_encoding.ilst", po)
	if ilst.Len() != 3 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if v, _ := ilst.Artist(); v != "Foo artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := ilst.Title(); v != "Bar title" {
		t.Fatalf("title = %q", v)
	}
	if v, _ := ilst.Comment(); v != "Baz comment" {
		t.Fatalf("comment = %q", v)
	}
	if _, ok := ilst.Album(); ok {
		t.Fatal("album present")
	}
}

func TestIlstFlagItemConversion(t *testing.T) {
	tag := NewTag(TagTypeMp4Ilst)
	tag.InsertText(ItemKeyFlagCompilation, "1")
	tag.InsertText(ItemKeyFlagPodcast, "0")
	ilst := IlstFromTag(tag)
	if b, ok := ilst.GetFlag(FourccIdent("cpil")); !ok || !b {
		t.Fatalf("cpil = %v", b)
	}
	if b, ok := ilst.GetFlag(FourccIdent("pcst")); !ok || b {
		t.Fatalf("pcst = %v", b)
	}
}

func TestIlstSpecialItemsRoundtrip(t *testing.T) {
	ilst := NewIlst()
	atom := Atom{FourccIdent("SMTH"), []AtomData{{Kind: AtomDataUnknown, Code: 0, Data: []byte("Meaningless Data")}}}
	ilst.Insert(atom)
	ilst.SetArtist("Foo Artist")

	tag := ilst.Tag()
	if tag.Len() != 1 {
		t.Fatalf("tag len = %d", tag.Len())
	}
	if v, _ := tag.Artist(); v != "Foo Artist" {
		t.Fatalf("artist = %q", v)
	}

	back := IlstFromTag(tag)
	if back.Len() != 2 {
		t.Fatalf("ilst len = %d", back.Len())
	}
	if v, _ := back.Artist(); v != "Foo Artist" {
		t.Fatalf("artist = %q", v)
	}
	if a := back.Get(FourccIdent("SMTH")); a == nil || !bytes.Equal(a.Data[0].Data, []byte("Meaningless Data")) {
		t.Fatalf("SMTH = %+v", a)
	}

	var buf bytes.Buffer
	if err := back.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	reRead, err := parseIlst(bytes.NewReader(buf.Bytes()[8:]), int64(buf.Len()-8), po)
	if err != nil {
		t.Fatal(err)
	}
	if reRead.Len() != back.Len() {
		t.Fatalf("re-read len = %d", reRead.Len())
	}
}

func TestIlstSkipReadingCoverArt(t *testing.T) {
	tag := NewTag(TagTypeMp4Ilst)
	tag.PushPicture(PictureUnchecked(make([]byte, 50)).MimeType(MimeTypeJpeg).Build())
	tag.SetArtist("Foo artist")
	data, err := encodeMp4Ilst(tag)
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ReadCoverArt = false
	ilst, err := parseIlst(bytes.NewReader(data[8:]), int64(len(data)-8), po)
	if err != nil {
		t.Fatal(err)
	}
	if ilst.Len() != 1 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if _, ok := ilst.Artist(); !ok {
		t.Fatal("no artist")
	}
}

func TestIlstGnreConversionCase1(t *testing.T) {
	po := NewParseOptions()
	po.ParsingMode = ParsingModeBestAttempt
	ilst := readIlstAsset(t, "gnre_conversion_case_1.ilst", po)
	if ilst.Len() != 2 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if v, _ := ilst.Artist(); v != "Foo artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := ilst.Genre(); v != "Funk" {
		t.Fatalf("genre = %q", v)
	}
}

func TestIlstGnreConversionCase2(t *testing.T) {
	po := NewParseOptions()
	po.ParsingMode = ParsingModeBestAttempt
	ilst := readIlstAsset(t, "gnre_conversion_case_2.ilst", po)
	if ilst.Len() != 2 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if v, _ := ilst.Genre(); v != "My Custom Genre" {
		t.Fatalf("genre = %q", v)
	}
}

func TestIlstGnreConversionCase3(t *testing.T) {
	po := NewParseOptions()
	po.ImplicitConversions = false
	ilst := readIlstAsset(t, "gnre_conversion_case_2.ilst", po)
	if ilst.Len() != 3 {
		t.Fatalf("len = %d", ilst.Len())
	}
	if v, _ := ilst.Genre(); v != "My Custom Genre" {
		t.Fatalf("genre = %q", v)
	}
	a := ilst.Get(FourccIdent("gnre"))
	if a == nil || len(a.Data) == 0 {
		t.Fatal("no gnre")
	}
	d := a.Data[0]
	if d.Kind != AtomDataUnknown || d.Code != 21 || !bytes.Equal(d.Data, []byte{0, 6}) {
		t.Fatalf("gnre = %+v", d)
	}
}

func TestIlstRetainUnknownTypes(t *testing.T) {
	ilst := NewIlst()
	ilst.Insert(Atom{
		FreeformIdent("com.apple.iTunes", "ARTISTS"),
		[]AtomData{
			{Kind: AtomDataUTF8, Text: "Serial-ATA"},
			{Kind: AtomDataUTF8, Text: "Lofty"},
			{Kind: AtomDataUnsignedInt, Uint: 42},
		},
	})
	tag := ilst.Tag()
	strings := tag.GetStrings(ItemKeyTrackArtists)
	if len(strings) != 2 || strings[0] != "Serial-ATA" || strings[1] != "Lofty" {
		t.Fatalf("artists = %v", strings)
	}
	if !tag.HasFormatSpecificItems() {
		t.Fatal("expected remainder")
	}
}

func TestIlstPictureRoundtrip(t *testing.T) {
	ilst := NewIlst()
	ilst.InsertPicture(PictureUnchecked([]byte{1, 2, 3}).Build())
	ilst.InsertPicture(PictureUnchecked([]byte{4, 5, 6}).Build())
	if a := ilst.Get(FourccIdent("covr")); a == nil || len(a.Data) != 2 {
		t.Fatalf("covr = %+v", a)
	}
	tag := ilst.Tag()
	if len(tag.Pictures()) != 2 {
		t.Fatalf("pictures = %d", len(tag.Pictures()))
	}
	back := IlstFromTag(tag)
	if a := back.Get(FourccIdent("covr")); a == nil || len(a.Data) != 2 {
		t.Fatalf("covr = %+v", a)
	}
}

func TestIlstZeroSized(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/files/assets/zero/zero.ilst")
	if err != nil {
		t.Skip("fixture missing")
	}
	po := NewParseOptions()
	po.ReadProperties = false
	f, err := Mp4FileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	ilst, ok := f.Ilst.Get()
	if !ok {
		t.Fatal("no ilst")
	}
	if !ilst.IsEmpty() {
		t.Fatalf("len = %d", ilst.Len())
	}
}

func TestIlstTrailingPadding(t *testing.T) {
	const ilstStart, ilstEnd, paddingSize = 97, 131, 990
	fileBytes, err := os.ReadFile("lofty-rs/lofty/tests/files/assets/ilst_trailing_padding.m4a")
	if err != nil {
		t.Fatal(err)
	}
	po := NewParseOptions()
	po.ReadProperties = false
	if _, err := Mp4FileReadFrom(bytes.NewReader(fileBytes), po); err != nil {
		t.Fatal(err)
	}

	oldFreeSize := binary.BigEndian.Uint32(fileBytes[ilstEnd:])
	if oldFreeSize != paddingSize {
		t.Fatalf("free size = %d", oldFreeSize)
	}
	ilst, err := parseIlst(bytes.NewReader(fileBytes[ilstStart:ilstEnd]), int64(ilstEnd-ilstStart), po)
	if err != nil {
		t.Fatal(err)
	}

	tmp := filepath.Join(t.TempDir(), "test.m4a")
	if err := os.WriteFile(tmp, fileBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ilst.SetTitle("Exactly 21 Characters")
	if err := mp4WriteIlstData(NewOSFile(f), ilst, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	newBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	// 24 (atom + data) + title string (21)
	newDataSize := uint32(24 + 21)
	newIlstEnd := ilstEnd + int(newDataSize)
	fileAtom := newBytes[newIlstEnd : newIlstEnd+8]
	if !bytes.Equal(fileAtom[4:], []byte("free")) {
		t.Fatalf("expected free, got %q", fileAtom)
	}
	if got := binary.BigEndian.Uint32(fileAtom); got != oldFreeSize-newDataSize {
		t.Fatalf("free size = %d, want %d", got, oldFreeSize-newDataSize)
	}
	if _, err := Mp4FileReadFrom(bytes.NewReader(newBytes), po); err != nil {
		t.Fatal(err)
	}
}

func TestIlstReadNonFullMetaAtom(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/files/assets/non_full_meta_atom.m4a")
	if err != nil {
		t.Skip("fixture missing")
	}
	po := NewParseOptions()
	po.ReadProperties = false
	f, err := Mp4FileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	if f.Ilst.IsNone() {
		t.Fatal("no ilst")
	}
}

func TestIlstWriteNonFullMetaAtom(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/files/assets/non_full_meta_atom.m4a")
	if err != nil {
		t.Skip("fixture missing")
	}
	tmp := filepath.Join(t.TempDir(), "test.m4a")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ilst := NewIlst()
	ilst.Insert(Atom{FourccIdent("©ART"), []AtomData{{Kind: AtomDataUTF8, Text: "Foo artist"}}})
	if err := mp4WriteIlstData(NewOSFile(f), ilst, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := Mp4FileReadFrom(f, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	back, ok := mf.Ilst.Get()
	if !ok {
		t.Fatal("no ilst")
	}
	ilstAtomText(t, back, "©ART", "Foo artist")
}
