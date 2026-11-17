package mp4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func testBox(name string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(out, uint32(8+len(body)))
	copy(out[4:], mp4NameFromKey(name))
	return append(out, body...)
}

func testDataBox(code uint32, payload []byte) []byte {
	hdr := []byte{0, byte(code >> 16), byte(code >> 8), byte(code), 0, 0, 0, 0}
	return testBox("data", hdr, payload)
}

func TestIlstRemoveRemovesAllMatching(t *testing.T) {
	ident := FourccIdent("©nam")
	l := &Ilst{atoms: []Atom{
		{ident, []AtomData{{Kind: AtomDataUTF8, Text: "a"}}},
		{FourccIdent("©alb"), []AtomData{{Kind: AtomDataUTF8, Text: "b"}}},
		{ident, []AtomData{{Kind: AtomDataUTF8, Text: "c"}}},
	}}
	l.Remove(ident)
	if l.Len() != 1 || l.Get(ident) != nil || l.Get(FourccIdent("©alb")) == nil {
		t.Fatalf("atoms after Remove = %+v", l.atoms)
	}
}

func TestIlstRemovePicturesRemovesEveryPictureAtom(t *testing.T) {
	pic := AtomData{Kind: AtomDataPicture}
	l := &Ilst{atoms: []Atom{
		{FourccIdent("covr"), []AtomData{pic}},
		{FourccIdent("©nam"), []AtomData{{Kind: AtomDataUTF8, Text: "a"}}},
		{FreeformIdent("m", "n"), []AtomData{pic}},
	}}
	l.RemovePictures()
	if l.Len() != 1 || l.Get(FourccIdent("©nam")) == nil {
		t.Fatalf("atoms after RemovePictures = %+v", l.atoms)
	}
}

func TestIlstTagSplitRemovesAllRtng(t *testing.T) {
	rtng := FourccIdent("rtng")
	l := &Ilst{atoms: []Atom{
		{rtng, []AtomData{{Kind: AtomDataSignedInt, Int: 1}}},
		{rtng, []AtomData{{Kind: AtomDataSignedInt, Int: 2}}},
	}}
	tag := l.Tag() // PreserveFormatSpecificItems defaults to true
	if v, ok := tag.GetString(meta.ItemKeyParentalAdvisory); !ok || v != "1" {
		t.Fatalf("ParentalAdvisory = %q, %v", v, ok)
	}
	if comp, ok := tag.CompanionTag().(*Ilst); ok && comp.Contains(rtng) {
		t.Fatalf("companion still has rtng: %+v", comp.atoms)
	}
}

func TestIlstSetFlagFalseRemoves(t *testing.T) {
	ident := FourccIdent("cpil")
	l := NewIlst()
	l.SetFlag(ident, true)
	if v, ok := l.GetFlag(ident); !ok || !v {
		t.Fatal("flag not set")
	}
	l.SetFlag(ident, false)
	if l.Contains(ident) {
		t.Fatal("false flag should remove the atom")
	}
}

func TestIlstRemoveTotals(t *testing.T) {
	pair := func(fourcc string, num, tot uint16) *Ilst {
		l := NewIlst()
		d := []byte{0, 0, byte(num >> 8), byte(num), byte(tot >> 8), byte(tot)}
		l.atoms = append(l.atoms, Atom{FourccIdent(fourcc), []AtomData{{Kind: AtomDataUnknown, Data: d}}})
		return l
	}

	l := pair("trkn", 3, 10)
	l.RemoveTrackTotal()
	if n, ok := l.Track(); !ok || n != 3 {
		t.Fatalf("track = %d, %v", n, ok)
	}
	if tot, _ := l.TrackTotal(); tot != 0 {
		t.Fatalf("track total = %d", tot)
	}

	l = pair("disk", 2, 4)
	l.RemoveDiskTotal()
	if n, ok := l.Disk(); !ok || n != 2 {
		t.Fatalf("disk = %d, %v", n, ok)
	}
	if tot, _ := l.DiskTotal(); tot != 0 {
		t.Fatalf("disk total = %d", tot)
	}

	// No existing atom: nothing is created.
	empty := NewIlst()
	empty.RemoveTrackTotal()
	empty.RemoveDiskTotal()
	if !empty.IsEmpty() {
		t.Fatalf("atoms = %+v", empty.atoms)
	}

	// An atom whose data isn't a readable pair has no number: it is removed.
	odd := NewIlst()
	odd.atoms = append(odd.atoms, Atom{FourccIdent("trkn"), []AtomData{{Kind: AtomDataUTF8, Text: "x"}}})
	odd.RemoveTrackTotal()
	if !odd.IsEmpty() {
		t.Fatalf("atoms = %+v", odd.atoms)
	}
}

func TestIlstAdvisoryRatingRangeAndFallthrough(t *testing.T) {
	rtng := FourccIdent("rtng")
	mk := func(data ...AtomData) *Ilst {
		return &Ilst{atoms: []Atom{{rtng, data}}}
	}

	// 257 must not wrap to 1.
	if r, ok := mk(AtomData{Kind: AtomDataSignedInt, Int: 257}).AdvisoryRating(); ok {
		t.Fatalf("257 produced %v", r)
	}
	if _, ok := mk(AtomData{Kind: AtomDataSignedInt, Int: -1}).AdvisoryRating(); ok {
		t.Fatal("-1 produced a rating")
	}
	// First value invalid -> try the next.
	r, ok := mk(
		AtomData{Kind: AtomDataSignedInt, Int: 9},
		AtomData{Kind: AtomDataUnknown, Data: []byte{2}},
	).AdvisoryRating()
	if !ok || r != AdvisoryRatingClean {
		t.Fatalf("fallthrough rating = %v, %v", r, ok)
	}
	// UnsignedInt is ignored.
	if _, ok := mk(AtomData{Kind: AtomDataUnsignedInt, Uint: 1}).AdvisoryRating(); ok {
		t.Fatal("UnsignedInt must be ignored")
	}
}

func TestIlstDateRequiresUTF8(t *testing.T) {
	l := NewIlst()
	l.atoms = append(l.atoms, Atom{FourccIdent("©day"), []AtomData{{Kind: AtomDataUTF16, Text: "1984"}}})
	if _, ok := l.Date(); ok {
		t.Fatal("UTF-16 ©day must not be accepted")
	}
	l = NewIlst()
	l.atoms = append(l.atoms, Atom{FourccIdent("©day"), []AtomData{{Kind: AtomDataUTF8, Text: "1984"}}})
	if _, ok := l.Date(); !ok {
		t.Fatal("UTF-8 ©day should parse")
	}
}

func TestIlstFromTagDropsUnknownKeyItems(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeMp4Ilst)
	item := meta.NewTagItem(meta.ItemKeyUnknown, meta.ItemValueText("x"))
	item.Description = "abcd"
	tag.PushUnchecked(item)
	free := meta.NewTagItem(meta.ItemKeyUnknown, meta.ItemValueText("y"))
	free.Description = "----:mean:name"
	tag.PushUnchecked(free)
	if l := IlstFromTag(tag); !l.IsEmpty() {
		t.Fatalf("unknown-key items kept: %+v", l.atoms)
	}
}

func TestIlstAtomSizeOverflow(t *testing.T) {
	if _, err := ilstAtomSize(16, math.MaxUint32-16); err != nil {
		t.Fatalf("exact fit: %v", err)
	}
	_, err := ilstAtomSize(16, math.MaxUint32-16+1)
	var tm *meta.TooMuchDataError
	if !errors.As(err, &tm) {
		t.Fatalf("err = %v, want TooMuchDataError", err)
	}
}

func TestIlstParseDataErrorDropsWholeAtom(t *testing.T) {
	good := testDataBox(1, []byte("ok"))
	bad := testDataBox(1, []byte{0xFF, 0xFE})
	body := bytes.Join([][]byte{
		testBox("©nam", good, bad, good),
		testBox("©alb", good),
	}, nil)

	po := meta.NewParseOptions()
	ilst, err := ParseIlst(bytes.NewReader(body), int64(len(body)), po)
	if err != nil {
		t.Fatal(err)
	}
	if ilst.Contains(FourccIdent("©nam")) {
		t.Fatalf("atom with an invalid data child must be dropped: %+v", ilst.atoms)
	}
	if !ilst.Contains(FourccIdent("©alb")) {
		t.Fatalf("following atom lost: %+v", ilst.atoms)
	}
}

func TestMp4AudioBitrateBps(t *testing.T) {
	cases := [][3]uint64{
		{1000, 8, 3},
		{math.MaxUint64, math.MaxUint32, 1},
		{math.MaxUint64, math.MaxUint32, 7},
		{1 << 40, 44100, 123456789},
		{math.MaxUint64 / 3, 48000, math.MaxUint64},
	}
	for _, c := range cases {
		n := new(big.Int).SetUint64(c[0])
		n.Mul(n, big.NewInt(8))
		n.Mul(n, new(big.Int).SetUint64(c[1]))
		n.Quo(n, new(big.Int).SetUint64(c[2]))
		want := uint32(new(big.Int).And(n, big.NewInt(math.MaxUint32)).Uint64())
		if got := mp4AudioBitrateBps(c[0], c[1], c[2]); got != want {
			t.Errorf("mp4AudioBitrateBps(%d,%d,%d) = %d, want %d", c[0], c[1], c[2], got, want)
		}
	}
}

func testFtyp() []byte {
	return testBox("ftyp", []byte("M4A "), []byte{0, 0, 0, 0}, []byte("M4A "))
}

func TestMp4ReadNonFtypIsUnknownFormat(t *testing.T) {
	data := testBox("free", make([]byte, 16))
	_, err := Mp4FileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	var fpe *meta.FileParseError
	if !errors.As(err, &fpe) || !fpe.IsUnknownFormat() {
		t.Fatalf("err = %v, want unknown format", err)
	}
}

func TestMp4ReadMissingMoov(t *testing.T) {
	_, err := Mp4FileReadFrom(bytes.NewReader(testFtyp()), meta.NewParseOptions())
	var mpe *Mp4ParseError
	if !errors.As(err, &mpe) || mpe.Message != `file does not contain a "moov" atom` {
		t.Fatalf("err = %v, want missing moov Mp4ParseError", err)
	}
}

func testAudioTrak() []byte {
	hdlr := testBox("hdlr", []byte{0, 0, 0, 0}, make([]byte, 4), []byte("soun"), make([]byte, 13))
	mdhd := testBox("mdhd", make([]byte, 12), []byte{0, 0, 0, 1}, []byte{0, 0, 0, 1})
	return testBox("trak", testBox("mdia", mdhd, hdlr))
}

func TestMp4MalformedLaterTrakFailsParse(t *testing.T) {
	// A trak whose child declares more bytes than the trak holds.
	badTrak := testBox("trak", []byte{0, 0, 1, 0, 'm', 'd', 'i', 'a'})
	file := bytes.Join([][]byte{testFtyp(), testBox("moov", testAudioTrak(), badTrak)}, nil)

	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	if _, err := Mp4FileReadFrom(bytes.NewReader(file), po); err == nil {
		t.Fatal("expected the malformed second trak to fail the parse")
	}

	good := bytes.Join([][]byte{testFtyp(), testBox("moov", testAudioTrak())}, nil)
	if _, err := Mp4FileReadFrom(bytes.NewReader(good), po); err != nil {
		t.Fatalf("well-formed file: %v", err)
	}
}

func TestMp4WriteNonFtypIsUnknownFormat(t *testing.T) {
	mem := &memFileLike{buf: testBox("free", make([]byte, 16))}
	ilst := NewIlst()
	ilst.SetTitle("t")
	err := mp4WriteIlstDataInner(mem, ilst, meta.NewWriteOptions())
	var fpe *meta.FileParseError
	if !errors.As(err, &fpe) || !fpe.IsUnknownFormat() {
		t.Fatalf("err = %v, want unknown format", err)
	}
}

func TestMp4WriteMissingMoov(t *testing.T) {
	mem := &memFileLike{buf: testFtyp()}
	ilst := NewIlst()
	ilst.SetTitle("t")
	err := mp4WriteIlstDataInner(mem, ilst, meta.NewWriteOptions())
	var mpe *Mp4ParseError
	if !errors.As(err, &mpe) || mpe.Message != `file does not contain a "moov" atom` {
		t.Fatalf("err = %v, want missing moov Mp4ParseError", err)
	}
}

func TestMp4WriteUsesFirstMoov(t *testing.T) {
	ftyp := testFtyp()
	mem := &memFileLike{buf: bytes.Join([][]byte{ftyp, testBox("moov"), testBox("moov")}, nil)}
	ilst := NewIlst()
	ilst.SetTitle("t")
	if err := mp4WriteIlstDataInner(mem, ilst, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	moov1 := mem.buf[len(ftyp):]
	first := binary.BigEndian.Uint32(moov1)
	if first <= 8 {
		t.Fatalf("first moov was not extended (size %d)", first)
	}
	if second := binary.BigEndian.Uint32(moov1[first:]); second != 8 || string(moov1[first+4:first+8]) != "moov" {
		t.Fatalf("second moov modified (size %d)", second)
	}
}

func TestMp4WriteLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates >16 MiB")
	}
	// Larger than util.TryByteSlice's allocation limit.
	mdat := testBox("mdat", make([]byte, 17<<20))
	buf := bytes.Join([][]byte{testFtyp(), testBox("moov"), mdat}, nil)
	mem := &memFileLike{buf: buf}
	ilst := NewIlst()
	ilst.SetTitle("big")
	if err := mp4WriteIlstData(mem, ilst, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if len(mem.buf) <= len(buf) {
		t.Fatal("file was not rewritten with the new tag")
	}
}
