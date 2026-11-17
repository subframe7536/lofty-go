package id3v2

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

func withAllocationLimit(t *testing.T, limit int) {
	t.Helper()
	old := util.GetGlobalOptions()
	opts := old
	opts.AllocationLimit = limit
	util.ApplyGlobalOptions(opts)
	t.Cleanup(func() { util.ApplyGlobalOptions(old) })
}

func dumpTag(t *testing.T, tag *Id3v2Tag, wo meta.WriteOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, wo); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func noPadding() meta.WriteOptions {
	wo := meta.NewWriteOptions()
	wo.PreferredPadding = 0
	return wo
}

// --- Date ---

func TestDateReadsTimestampFrames(t *testing.T) {
	tag := NewId3v2Tag()
	if got := tag.Date(); got != "" {
		t.Fatalf("empty tag date = %q", got)
	}
	tag.Frames.Push(NewTimestampFrame(FrameIdValid("TDRC"), TextEncodingUTF8,
		meta.Timestamp{Year: 2020, Month: meta.Some(uint8(5)), Day: meta.Some(uint8(1))}))
	if got := tag.Date(); got != "2020-05-01" {
		t.Fatalf("date = %q", got)
	}

	// Plain text frames (the SetDate fallback) are still readable
	other := NewId3v2Tag()
	other.Frames.Push(NewTextInformationFrame(FrameIdValid("TDRC"), TextEncodingUTF8, "sometime"))
	if got := other.Date(); got != "sometime" {
		t.Fatalf("text date = %q", got)
	}
}

func TestDateParsedFromTagBytes(t *testing.T) {
	data := rtTag(4, 0, rtFrame("TDRC", 0, append([]byte{3}, "2019-03-04"...), true))
	tag, err := rtParse(data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if got := tag.Date(); got != "2019-03-04" {
		t.Fatalf("date = %q", got)
	}
}

func TestSetDateReplacesExistingFrame(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetDate("2001-02-03")
	tag.SetDate("1999")
	if got := tag.Date(); got != "1999" {
		t.Fatalf("date = %q", got)
	}
	if n := len(tag.Frames.Remove(FrameIdValid("TDRC"))); n != 1 {
		t.Fatalf("%d TDRC frames after SetDate twice", n)
	}

	// Replaces a text frame too, and unparseable input falls back to text
	tag.Frames.Push(NewTextInformationFrame(FrameIdValid("TDRC"), TextEncodingUTF8, "old"))
	tag.SetDate("2010")
	tag.SetDate("not a date")
	if got := tag.Date(); got != "not a date" {
		t.Fatalf("date = %q", got)
	}
	if n := len(tag.Frames.Remove(FrameIdValid("TDRC"))); n != 1 {
		t.Fatalf("%d TDRC frames", n)
	}
}

// --- nested CHAP/CTOC ---

func nestedChapContent(depth int) []byte {
	head := rtCat([]byte("c\x00"), make([]byte, 16))
	if depth == 1 {
		return rtCat(head, rtFrame("TIT2", 0, append([]byte{3}, "leaf"...), true))
	}
	return rtCat(head, rtFrame("CHAP", 0, nestedChapContent(depth-1), true))
}

// chapChain follows the first CHAP child at each level.
func chapChain(tag *Id3v2Tag) (depth int, leaf string) {
	list := tag.Frames
	for {
		ch, ok := list.Get(FrameIdValid("CHAP")).(*ChapterFrame)
		if !ok {
			return depth, ""
		}
		depth++
		if title, ok := ch.Children.GetText(FrameIdValid("TIT2")); ok {
			return depth, title
		}
		list = ch.Children
	}
}

func TestNestedChapterDepthIsCapped(t *testing.T) {
	// At the limit everything parses
	ok := rtTag(4, 0, rtFrame("CHAP", 0, nestedChapContent(maxFrameNesting), true))
	rtParseAll(t, ok, func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if depth, leaf := chapChain(tag); depth != maxFrameNesting || leaf != "leaf" {
			t.Fatalf("depth=%d leaf=%q", depth, leaf)
		}
	})

	// One level too deep: strict fails, the others drop the offending frame
	for _, n := range []int{maxFrameNesting + 1, 500} {
		deep := rtTag(4, 0, rtFrame("CHAP", 0, nestedChapContent(n), true))
		for _, m := range rtModes {
			tag, err := rtParse(deep, m.mode)
			if m.mode == meta.ParsingModeStrict {
				if err == nil {
					t.Fatalf("n=%d strict: expected an error", n)
				}
				continue
			}
			if err != nil {
				t.Fatalf("n=%d %s: %v", n, m.name, err)
			}
			if depth, leaf := chapChain(tag); depth != maxFrameNesting || leaf != "" {
				t.Fatalf("n=%d %s: depth=%d leaf=%q", n, m.name, depth, leaf)
			}
		}
	}
}

func TestNestedCtocDepthIsCapped(t *testing.T) {
	var build func(depth int) []byte
	build = func(depth int) []byte {
		head := []byte{'t', 0, 0, 0} // id, flags, zero entries
		if depth == 1 {
			return rtCat(head, rtFrame("TIT2", 0, append([]byte{3}, "leaf"...), true))
		}
		return rtCat(head, rtFrame("CTOC", 0, build(depth-1), true))
	}
	for _, n := range []int{maxFrameNesting, maxFrameNesting + 1} {
		data := rtTag(4, 0, rtFrame("CTOC", 0, build(n), true))
		_, err := rtParse(data, meta.ParsingModeStrict)
		if (n > maxFrameNesting) != (err != nil) {
			t.Fatalf("n=%d strict err = %v", n, err)
		}
		if _, err := rtParse(data, meta.ParsingModeRelaxed); err != nil {
			t.Fatalf("n=%d relaxed: %v", n, err)
		}
	}
}

// Children are sub-slices of the parent's content: in-place work on one child
// (deunsynchronisation) must not disturb its siblings.
func TestNestedChildrenAreIndependent(t *testing.T) {
	children := rtCat(
		rtFrame("TIT2", 0x0002, []byte{0, 'a', 0xFF, 0x00, 'b'}, true),
		rtFrame("TIT3", 0, []byte{0, 'n', 'e', 'x', 't'}, true),
		rtFrame("TALB", 0, []byte{0, 'z'}, true),
	)
	chap := rtCat([]byte("c\x00"), make([]byte, 16), children)
	data := rtTag(4, 0, rtCat(rtFrame("CHAP", 0, chap, true), rtFrame("TPE1", 0, []byte{0, 'x'}, true)))
	rtParseAll(t, data, func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		ch := tag.Frames.Get(FrameIdValid("CHAP")).(*ChapterFrame)
		for id, want := range map[string]string{"TIT2": "a\u00ffb", "TIT3": "next", "TALB": "z"} {
			if got, _ := ch.Children.GetText(FrameIdValid(id)); got != want {
				t.Errorf("%s = %q, want %q", id, got, want)
			}
		}
		if got, _ := tag.Frames.GetText(FrameIdValid("TPE1")); got != "x" {
			t.Errorf("TPE1 = %q", got)
		}
	})
}

func TestChapterParsedStandaloneIsBounded(t *testing.T) {
	withAllocationLimit(t, 64)
	data := rtCat([]byte("c\x00"), make([]byte, 16), rtFrame("TIT2", 0, append([]byte{3}, bytes.Repeat([]byte("a"), 200)...), true))
	_, err := parseChapterFrame(bytes.NewReader(data), meta.NewParseOptions(), id3v2Version4, FrameFlags{})
	var ae *util.AllocationError
	if !errors.As(err, &ae) {
		t.Fatalf("got %v, want an AllocationError", err)
	}
}

// --- compressed frames ---

func compressedTitle(raw []byte) []byte {
	return rtTag(4, 0, rtFrame("TIT2", 0x0009, rtCat(rtSynch(len(raw)), rtZlib(raw)), true))
}

func TestCompressedFrameOutputIsBounded(t *testing.T) {
	small := append([]byte{3}, bytes.Repeat([]byte("a"), 100)...)
	big := append([]byte{3}, bytes.Repeat([]byte("a"), 4096)...)

	withAllocationLimit(t, 1024)

	rtParseAll(t, compressedTitle(small), func(t *testing.T, tag *Id3v2Tag, err error) {
		if err != nil {
			t.Fatal(err)
		}
		if got := rtText(t, tag); got != string(small[1:]) {
			t.Fatalf("title = %q", got)
		}
	})

	rtParseAll(t, compressedTitle(big), func(t *testing.T, _ *Id3v2Tag, err error) {
		var ae *util.AllocationError
		if !errors.As(err, &ae) {
			t.Fatalf("got %v, want an AllocationError", err)
		}
	})
}

func TestCompressedFrameExactlyAtLimit(t *testing.T) {
	withAllocationLimit(t, 1024)
	raw := append([]byte{3}, bytes.Repeat([]byte("a"), 1023)...)
	tag, err := rtParse(compressedTitle(raw), meta.ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if got := rtText(t, tag); len(got) != 1023 {
		t.Fatalf("title length = %d", len(got))
	}
}

// --- accessors ---

func TestAccessorsReplaceV4Separators(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetTitle("A\x00B")
	tag.SetArtist("X\x00Y\x00Z")
	tag.SetAlbum("one")
	if got := tag.Title(); got != "A/B" {
		t.Fatalf("title = %q", got)
	}
	if got := tag.Artist(); got != "X/Y/Z" {
		t.Fatalf("artist = %q", got)
	}
	if got := tag.Album(); got != "one" {
		t.Fatalf("album = %q", got)
	}
	// The stored value keeps its separators
	if got, _ := tag.Frames.GetText(FrameIdValid("TIT2")); got != "A\x00B" {
		t.Fatalf("raw title = %q", got)
	}

	// Other versions are returned untouched
	tag.SetOriginalVersion(Id3v2Version3)
	if got := tag.Title(); got != "A\x00B" {
		t.Fatalf("v2.3 title = %q", got)
	}
}

func TestGenreJoinsWithSpacedSlash(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetGenre("17\x0013")
	if got := tag.Genre(); got != "Rock / Pop" {
		t.Fatalf("genre = %q", got)
	}
	tag.SetGenre("Metal")
	if got := tag.Genre(); got != "Metal" {
		t.Fatalf("genre = %q", got)
	}
}

func TestPairAccessorsParseLikeRust(t *testing.T) {
	type pair struct{ num, total meta.Option[uint32] }
	some := func(n uint32) meta.Option[uint32] { return meta.Some(n) }
	none := meta.None[uint32]()
	for text, want := range map[string]pair{
		"1/2":          {some(1), some(2)},
		"1\x002":       {some(1), some(2)},
		"7":            {some(7), none},
		"1/":           {some(1), none},
		"/2":           {some(2), none},
		" 1/2":         {some(2), none}, // not trimmed: " 1" is skipped
		"a/1/2":        {some(1), some(2)},
		"+3/4":         {some(3), some(4)},
		"1/2/3":        {some(1), some(2)},
		"x":            {none, none},
		"":             {none, none},
		"-1/2":         {some(2), none},
		"4294967296/1": {some(1), none},
	} {
		tag := NewId3v2Tag()
		tag.Frames.Insert(NewTextInformationFrame(FrameIdValid("TRCK"), TextEncodingUTF8, text))
		tag.Frames.Insert(NewTextInformationFrame(FrameIdValid("TPOS"), TextEncodingUTF8, text))
		if got := tag.Track(); got != want.num {
			t.Errorf("Track(%q) = %v, want %v", text, got, want.num)
		}
		if got := tag.TrackTotal(); got != want.total {
			t.Errorf("TrackTotal(%q) = %v, want %v", text, got, want.total)
		}
		if got := tag.Disk(); got != want.num {
			t.Errorf("Disk(%q) = %v, want %v", text, got, want.num)
		}
		if got := tag.DiskTotal(); got != want.total {
			t.Errorf("DiskTotal(%q) = %v, want %v", text, got, want.total)
		}
	}
}

func TestSetPairKeepsOtherHalf(t *testing.T) {
	tag := NewId3v2Tag()
	tag.Frames.Insert(NewTextInformationFrame(FrameIdValid("TRCK"), TextEncodingUTF8, "1\x009"))
	tag.SetTrack(4)
	if got := tag.text("TRCK"); got != "4/9" {
		t.Fatalf("TRCK = %q", got)
	}
	tag.SetTrackTotal(12)
	if got := tag.text("TRCK"); got != "4/12" {
		t.Fatalf("TRCK = %q", got)
	}
	tag.RemoveTrackTotal()
	if got := tag.text("TRCK"); got != "4" {
		t.Fatalf("TRCK = %q", got)
	}
}

// Generic conversion uses `splitn(2, [NUL, '/'])` and requires both parts to
// be numbers. Otherwise the frame is not split into number/total (upstream
// then maps the raw text through the key table, which is not checked here).
func TestTrackSplitMatchesRustSplitPair(t *testing.T) {
	convert := func(text string) *meta.Tag {
		tag := NewId3v2Tag()
		tag.Frames.Insert(NewTextInformationFrame(FrameIdValid("TRCK"), TextEncodingUTF8, text))
		return tag.Tag()
	}
	for text, want := range map[string][2]string{
		"1/2":     {"1", "2"},
		"1\x002":  {"1", "2"},
		" 1 / 2 ": {"1", "2"},
		"5":       {"5", ""},
		// The second part is "2/3": not a number, so nothing is converted
		"1\x002/3": {"", "?"},
		"1/2/3":    {"", "?"},
		"1/2\x003": {"", "?"},
		"a/2":      {"", "?"},
		"1/":       {"", "?"},
	} {
		tag := convert(text)
		num, hasNum := tag.GetString(meta.ItemKeyTrackNumber)
		total, hasTotal := tag.GetString(meta.ItemKeyTrackTotal)
		if (want[0] != "") != hasNum || num != want[0] {
			t.Errorf("%q: num=%q,%v, want %q", text, num, hasNum, want[0])
		}
		if want[1] == "?" {
			continue
		}
		if (want[1] != "") != hasTotal || total != want[1] {
			t.Errorf("%q: num=%q,%v total=%q,%v, want %q", text, num, hasNum, total, hasTotal, want)
		}
	}
}

func TestUnsyncTextsReturnsEveryFrame(t *testing.T) {
	tag := NewId3v2Tag()
	if got := tag.UnsyncTexts(); len(got) != 0 {
		t.Fatalf("got %d frames", len(got))
	}
	tag.Frames.Push(NewUnsynchronizedTextFrame(TextEncodingUTF8, meta.LangUnknown, "a", "one"))
	tag.Frames.Push(NewUnsynchronizedTextFrame(TextEncodingUTF8, meta.LangUnknown, "b", "two"))
	tag.Frames.Push(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingUTF8, "x"))
	got := tag.UnsyncTexts()
	if len(got) != 2 || got[0].Content != "one" || got[1].Content != "two" {
		t.Fatalf("got %+v", got)
	}
	// The single-value accessor keeps its "exactly one" semantics
	if tag.UnsyncText() != "" {
		t.Fatal("UnsyncText with two frames must be empty")
	}
}

// --- writing ---

func TestWriteGoOnlySpecialFrames(t *testing.T) {
	frames := map[string]Frame{
		"SYLT": NewSynchronizedTextFrame(TextEncodingLatin1, [3]byte{'e', 'n', 'g'}, TimestampFormatMS, SyncTextLyrics, meta.None[string](),
			[]SyncTextItem{{Timestamp: 1, Text: "a"}}),
		"GEOB": NewGeneralEncapsulatedObject(TextEncodingLatin1, meta.Some("text/plain"), meta.None[string](), meta.None[string](), []byte("data")),
		"ATXT": NewAudioTextFrame(TextEncodingLatin1, "audio/mpeg", AudioTextFrameFlags{}, "text", []byte{1, 2, 3}),
		"OWNE": NewOwnershipFrame(TextEncodingLatin1, "USD1", "20200101", "me"),
		"ETCO": NewEventTimingCodesFrame(TimestampFormatMS, []Event{{EventType: EventTypeIntroStart, Timestamp: 5}}),
	}
	for id, frame := range frames {
		for _, v3 := range []bool{false, true} {
			tag := NewId3v2Tag()
			tag.Frames.Push(frame)
			wo := noPadding()
			wo.UseID3v23 = v3
			out := dumpTag(t, tag, wo)
			if !bytes.Contains(out, []byte(id)) {
				t.Errorf("%s (v3=%v) was not written", id, v3)
			}
		}
	}
}

func TestVerifyFrameStillRejectsMismatchedIds(t *testing.T) {
	geob := NewGeneralEncapsulatedObject(TextEncodingLatin1, meta.None[string](), meta.None[string](), meta.None[string](), nil)
	geob.header.ID = FrameIdValid("TIT2")
	sylt := NewSynchronizedTextFrame(TextEncodingLatin1, [3]byte{'e', 'n', 'g'}, TimestampFormatMS, SyncTextLyrics, meta.None[string](), nil)
	sylt.header.ID = FrameIdValid("APIC")
	for _, f := range []Frame{geob, sylt, NewTextInformationFrame(FrameId{}, TextEncodingUTF8, "x")} {
		if err := verifyFrame(f); err == nil {
			t.Errorf("%T under %q accepted", f, f.ID().String())
		}
	}
}

func TestCtocRootStateDoesNotLeakIntoChildren(t *testing.T) {
	child := NewChapterTableOfContentsFrame("child", CtocFlags{TopLevel: true}, nil, nil)
	children := NewFrameList()
	children.Push(child)
	chap := NewChapterFrame("c", 0, 0, 0, 0, children)
	root := NewChapterTableOfContentsFrame("root", CtocFlags{TopLevel: true}, []string{"c"}, nil)

	tag := NewId3v2Tag()
	tag.Frames.Push(chap)
	tag.Frames.Push(root)
	out := dumpTag(t, tag, noPadding())
	if n := bytes.Count(out, []byte("CTOC")); n != 2 {
		t.Fatalf("wrote %d CTOC frames, want the nested one and the root", n)
	}

	// Two roots at one level: only the first is written
	tag = NewId3v2Tag()
	tag.Frames.Push(NewChapterTableOfContentsFrame("a", CtocFlags{TopLevel: true}, nil, nil))
	tag.Frames.Push(NewChapterTableOfContentsFrame("b", CtocFlags{TopLevel: true}, nil, nil))
	if n := bytes.Count(dumpTag(t, tag, noPadding()), []byte("CTOC")); n != 1 {
		t.Fatalf("wrote %d root CTOC frames", n)
	}
}

func TestV23TdorYearIsZeroPadded(t *testing.T) {
	for year, want := range map[uint16]string{5: "0005", 987: "0987", 2024: "2024"} {
		tag := NewId3v2Tag()
		tag.Frames.Push(NewTimestampFrame(FrameIdValid("TDOR"), TextEncodingLatin1, meta.Timestamp{Year: year}))
		wo := noPadding()
		wo.UseID3v23 = true
		out := dumpTag(t, tag, wo)
		needle := []byte("TORY\x00\x00\x00\x05\x00\x00\x00" + want)
		if !bytes.Contains(out, needle) {
			t.Errorf("year %d: %q not found in %q", year, needle, out)
		}
	}
}

// With a CRC and restrictions Rust hashes from byte 25; a tag whose frames all
// vanish is only 24 bytes long and must not slice out of range.
func TestWriteCrcWithoutContentDoesNotPanic(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetFlags(Id3v2TagFlags{Crc: true, Restrictions: &TagRestrictions{}})
	tag.Frames.Push(NewBinaryFrame(FrameIdOutdated("TT2"), []byte{1}))
	out := dumpTag(t, tag, noPadding())
	if len(out) != 24 {
		t.Fatalf("len = %d", len(out))
	}

	// With content the offsets are unchanged (Rust parity)
	tag.Frames.Push(NewTextInformationFrame(FrameIdValid("TIT2"), TextEncodingLatin1, "x"))
	if out := dumpTag(t, tag, noPadding()); len(out) <= 24 {
		t.Fatalf("len = %d", len(out))
	}
}

func TestVerifyTagSizeRejectsLengthsPastFourGiB(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("needs a 64-bit int")
	}
	shift := uint(33)
	huge := 1 << shift // would wrap to a small value if narrowed to uint32 first
	_, _, err := verifyTagSize(Id3v2TagFlags{}, huge+5, 10, 0, 0)
	var tmd *meta.TooMuchDataError
	if !errors.As(err, &tmd) {
		t.Fatalf("got %v, want TooMuchDataError", err)
	}

	total, padding, err := verifyTagSize(Id3v2TagFlags{}, 110, 10, 0, 50)
	if err != nil || total != 150 || padding != 50 {
		t.Fatalf("total=%d padding=%d err=%v", total, padding, err)
	}
}

func TestPaddingAllocationHonorsLimit(t *testing.T) {
	withAllocationLimit(t, 100)
	tag := NewId3v2Tag()
	tag.SetTitle("x")
	wo := meta.NewWriteOptions()
	wo.PreferredPadding = 1000
	for _, v3 := range []bool{false, true} {
		wo.UseID3v23 = v3
		var buf bytes.Buffer
		err := tag.DumpTo(&buf, wo)
		var ae *util.AllocationError
		if !errors.As(err, &ae) {
			t.Fatalf("v3=%v: got %v, want an AllocationError", v3, err)
		}
	}

	wo.PreferredPadding = 50
	if err := tag.DumpTo(&bytes.Buffer{}, wo); err != nil {
		t.Fatalf("padding within the limit: %v", err)
	}
}
