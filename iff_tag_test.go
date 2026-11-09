package lofty

// Ports of lofty-rs src/iff/wav/tag/mod.rs and src/iff/aiff/tag/mod.rs tests.

import (
	"bytes"
	"os"
	"testing"
)

func readRiffAsset(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.riff")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func riffInfoItemsEqual(t *testing.T, want, got *RiffInfoTag) {
	t.Helper()
	if want.Len() != got.Len() {
		t.Fatalf("len = %d, want %d", got.Len(), want.Len())
	}
	for _, item := range want.items {
		v, ok := got.Get(item.key)
		if !ok || v != item.value {
			t.Fatalf("item %q = %q, want %q", item.key, v, item.value)
		}
	}
}

func TestRiffInfoParse(t *testing.T) {
	expected := &RiffInfoTag{}
	expected.InsertKV("IART", "Bar artist")
	expected.InsertKV("ICMT", "Qux comment")
	expected.InsertKV("ICRD", "1984")
	expected.InsertKV("INAM", "Foo title")
	expected.InsertKV("IPRD", "Baz album")
	expected.InsertKV("IPRT", "1")

	data := readRiffAsset(t)
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	parsed := parseRiffInfoList(bytes.NewReader(data), int64(len(data)), po)
	riffInfoItemsEqual(t, expected, parsed)
}

func TestRiffInfoReRead(t *testing.T) {
	data := readRiffAsset(t)
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	parsed := parseRiffInfoList(bytes.NewReader(data), int64(len(data)), po)
	var buf bytes.Buffer
	if err := parsed.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	// Skip the 12-byte LIST+size+INFO header
	reParsed := parseRiffInfoList(bytes.NewReader(buf.Bytes()[12:]), int64(buf.Len()), po)
	riffInfoItemsEqual(t, parsed, reParsed)
}

func TestRiffInfoToTag(t *testing.T) {
	data := readRiffAsset(t)
	po := NewParseOptions()
	po.ParsingMode = ParsingModeStrict
	info := parseRiffInfoList(bytes.NewReader(data), int64(len(data)), po)
	tag := info.Tag()
	verifyTagCommon(t, tag, true, false)
}

func TestTagToRiffInfo(t *testing.T) {
	info := RiffInfoTagFromTag(createTagCommon(TagTypeRiffInfo))
	check := func(key, want string) {
		t.Helper()
		if got, _ := info.Get(key); got != want {
			t.Fatalf("%q = %q, want %q", key, got, want)
		}
	}
	check("INAM", "Foo title")
	check("IART", "Bar artist")
	check("IPRD", "Baz album")
	check("ICMT", "Qux comment")
	check("IPRT", "1")
}

func TestRiffInfoCompanionTagRoundtrip(t *testing.T) {
	info := &RiffInfoTag{}
	info.SetArtist("Foo artist")
	info.InsertKV("WHAT", "Something format-specific")
	if info.Len() != 2 {
		t.Fatalf("len = %d", info.Len())
	}
	tag := info.Tag()
	if a, _ := tag.Artist(); a != "Foo artist" {
		t.Fatalf("artist = %q", a)
	}
	if tag.Len() != 1 {
		t.Fatalf("tag len = %d, want 1", tag.Len())
	}
	info2 := RiffInfoTagFromTag(tag)
	if info2.Len() != 2 {
		t.Fatalf("len = %d, want 2", info2.Len())
	}
	if v, _ := info2.Get("WHAT"); v != "Something format-specific" {
		t.Fatalf("WHAT = %q", v)
	}
}

// --- AIFF ---

func readAiffTextTag(t *testing.T, data []byte) *AiffTextTag {
	t.Helper()
	po := NewParseOptions()
	po.ReadProperties = false
	f, err := AiffFileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.TextChunks.Get()
	if !ok {
		t.Fatal("no text chunks")
	}
	return tag
}

func TestAiffTextParse(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.aiff_text")
	if err != nil {
		t.Fatal(err)
	}
	tag := readAiffTextTag(t, data)
	if v, _ := tag.Name.Get(); v != "Foo title" {
		t.Fatalf("name = %q", v)
	}
	if v, _ := tag.Author.Get(); v != "Bar artist" {
		t.Fatalf("author = %q", v)
	}
	if v, _ := tag.Copyright.Get(); v != "Baz copyright" {
		t.Fatalf("copyright = %q", v)
	}
	if len(tag.Annotations) != 2 || tag.Annotations[0] != "Qux annotation" || tag.Annotations[1] != "Quux annotation" {
		t.Fatalf("annotations = %v", tag.Annotations)
	}
	want := []AiffComment{
		{Timestamp: 1024, MarkerID: 0, Text: "Quuz comment"},
		{Timestamp: 2048, MarkerID: 40, Text: "Corge comment"},
	}
	if len(tag.Comments) != len(want) {
		t.Fatalf("comments = %v", tag.Comments)
	}
	for i, c := range want {
		if tag.Comments[i] != c {
			t.Fatalf("comment %d = %+v, want %+v", i, tag.Comments[i], c)
		}
	}
}

func TestAiffTextReRead(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.aiff_text")
	if err != nil {
		t.Fatal(err)
	}
	tag := readAiffTextTag(t, data)
	// Fake AIFF signature + chunks
	writer := []byte{'F', 'O', 'R', 'M', 0, 0, 0, 0xC6, 'A', 'I', 'F', 'F'}
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	writer = append(writer, buf.Bytes()...)
	reParsed := readAiffTextTag(t, writer)
	if n, _ := reParsed.Name.Get(); n != tag.Name.OrZero() ||
		len(reParsed.Annotations) != len(tag.Annotations) ||
		len(reParsed.Comments) != len(tag.Comments) {
		t.Fatalf("re-read mismatch: %+v", reParsed)
	}
}

func TestAiffTextReReadOddSizedComments(t *testing.T) {
	tag := &AiffTextTag{
		Comments: []AiffComment{
			{Timestamp: 1024, MarkerID: 40, Text: "Corge comment"},
			{Timestamp: 2048, MarkerID: 0, Text: "Waldo comment"},
			{Timestamp: 4096, MarkerID: 80, Text: "Grault comment"},
		},
	}
	writer := []byte{'F', 'O', 'R', 'M', 0, 0, 0, 0, 'A', 'I', 'F', 'F'}
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	writer = append(writer, buf.Bytes()...)
	parsed := readAiffTextTag(t, writer)
	if len(parsed.Comments) != 3 {
		t.Fatalf("comments = %v", parsed.Comments)
	}
	for i, c := range tag.Comments {
		if parsed.Comments[i] != c {
			t.Fatalf("comment %d = %+v, want %+v", i, parsed.Comments[i], c)
		}
	}
}

func TestAiffParsePaddedComt(t *testing.T) {
	var aiff []byte
	aiff = append(aiff, []byte("FORM\x00\x00\x00\x00AIFF")...)
	aiff = append(aiff, []byte("COMT\x00\x00\x00\x2C\x00\x02")...)
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0DCorge comment\x00")...)
	aiff = append(aiff, []byte("\x00\x00\x08\x00\x00\x28\x00\x0CQuuz comment")...)
	parsed := readAiffTextTag(t, aiff)
	want := []AiffComment{
		{Timestamp: 1024, MarkerID: 0, Text: "Corge comment"},
		{Timestamp: 2048, MarkerID: 40, Text: "Quuz comment"},
	}
	if len(parsed.Comments) != 2 || parsed.Comments[0] != want[0] || parsed.Comments[1] != want[1] {
		t.Fatalf("comments = %v", parsed.Comments)
	}
}

func TestAiffParseUnpaddedFinalComt(t *testing.T) {
	var aiff []byte
	aiff = append(aiff, []byte("FORM\x00\x00\x00\x00AIFF")...)
	aiff = append(aiff, []byte("COMT\x00\x00\x00\x17\x00\x01")...)
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0DCorge comment")...)
	aiff = append(aiff, 0) // chunk padding
	aiff = append(aiff, []byte("NAME\x00\x00\x00\x09Foo title\x00")...)
	parsed := readAiffTextTag(t, aiff)
	if len(parsed.Comments) != 1 || parsed.Comments[0].Text != "Corge comment" {
		t.Fatalf("comments = %v", parsed.Comments)
	}
	if v, _ := parsed.Name.Get(); v != "Foo title" {
		t.Fatalf("name = %q", v)
	}
}

func TestAiffParseComtInvalidPadding(t *testing.T) {
	var aiff []byte
	aiff = append(aiff, []byte("FORM\x00\x00\x00\x00AIFF")...)
	aiff = append(aiff, []byte("COMT\x00\x00\x00\x52\x00\x04")...)
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0BFoo comment\x00")...)
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0BBar comment\x00")...)
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0BBaz comment")...)
	aiff = append(aiff, 0x45) // garbage padding byte
	aiff = append(aiff, []byte("\x00\x00\x04\x00\x00\x00\x00\x0BQux comment\x00")...)
	aiff = append(aiff, []byte("NAME\x00\x00\x00\x09Foo title\x00")...)

	// BestAttempt: keep Foo and Bar, drop the rest
	parsed := readAiffTextTag(t, aiff)
	if len(parsed.Comments) != 2 || parsed.Comments[0].Text != "Foo comment" || parsed.Comments[1].Text != "Bar comment" {
		t.Fatalf("comments = %v", parsed.Comments)
	}
	if v, _ := parsed.Name.Get(); v != "Foo title" {
		t.Fatalf("name = %q", v)
	}

	// Strict mode should fail
	po := NewParseOptions()
	po.ReadProperties = false
	po.ParsingMode = ParsingModeStrict
	if _, err := AiffFileReadFrom(bytes.NewReader(aiff), po); err == nil {
		t.Fatal("expected strict-mode failure")
	}
}

func TestAiffTextToTag(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/test.aiff_text")
	if err != nil {
		t.Fatal(err)
	}
	tag := readAiffTextTag(t, data).Tag()
	if v, _ := tag.GetString(ItemKeyTrackTitle); v != "Foo title" {
		t.Fatalf("title = %q", v)
	}
	if v, _ := tag.GetString(ItemKeyTrackArtist); v != "Bar artist" {
		t.Fatalf("artist = %q", v)
	}
	if v, _ := tag.GetString(ItemKeyCopyrightMessage); v != "Baz copyright" {
		t.Fatalf("copyright = %q", v)
	}
	comments := tag.GetStrings(ItemKeyComment)
	if len(comments) != 2 || comments[0] != "Qux annotation" || comments[1] != "Quux annotation" {
		t.Fatalf("comments = %v", comments)
	}
}

func TestTagToAiffText(t *testing.T) {
	tag := NewTag(TagTypeAiffText)
	tag.InsertText(ItemKeyTrackTitle, "Foo title")
	tag.InsertText(ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(ItemKeyCopyrightMessage, "Baz copyright")
	tag.PushUnchecked(NewTagItem(ItemKeyComment, ItemValueText("Qux annotation")))
	tag.PushUnchecked(NewTagItem(ItemKeyComment, ItemValueText("Quux annotation")))

	aiffText := AiffTextTagFromTag(tag)
	if v, _ := aiffText.Name.Get(); v != "Foo title" {
		t.Fatalf("name = %q", v)
	}
	if v, _ := aiffText.Author.Get(); v != "Bar artist" {
		t.Fatalf("author = %q", v)
	}
	if v, _ := aiffText.Copyright.Get(); v != "Baz copyright" {
		t.Fatalf("copyright = %q", v)
	}
	if len(aiffText.Annotations) != 2 {
		t.Fatalf("annotations = %v", aiffText.Annotations)
	}
	if len(aiffText.Comments) != 0 {
		t.Fatalf("comments = %v", aiffText.Comments)
	}
}

func TestAiffTextSpecialItemsRoundtrip(t *testing.T) {
	comments := []AiffComment{
		{Timestamp: 10, MarkerID: 20, Text: "foo"},
		{Timestamp: 30, MarkerID: 40, Text: "bar"},
	}
	aiffText := &AiffTextTag{Comments: comments}
	if aiffText.Len() != 2 {
		t.Fatalf("len = %d", aiffText.Len())
	}
	tag := aiffText.Tag()
	if tag.Len() != 0 {
		t.Fatalf("tag len = %d, want 0", tag.Len())
	}
	back := AiffTextTagFromTag(tag)
	if len(back.Comments) != 2 || back.Comments[0] != comments[0] || back.Comments[1] != comments[1] {
		t.Fatalf("comments = %v", back.Comments)
	}
}

func TestAiffZeroSizedTextChunks(t *testing.T) {
	data, err := os.ReadFile("lofty-rs/lofty/tests/tags/assets/zero.aiff_text")
	if err != nil {
		t.Fatal(err)
	}
	tag := readAiffTextTag(t, data)
	if v, ok := tag.Name.Get(); !ok || v != "" {
		t.Fatalf("name = %v", tag.Name)
	}
	if v, ok := tag.Author.Get(); !ok || v != "" {
		t.Fatalf("author = %v", tag.Author)
	}
	if len(tag.Annotations) != 1 || tag.Annotations[0] != "" {
		t.Fatalf("annotations = %v", tag.Annotations)
	}
	if len(tag.Comments) != 0 {
		t.Fatalf("comments = %v", tag.Comments)
	}
	if v, ok := tag.Copyright.Get(); !ok || v != "" {
		t.Fatalf("copyright = %v", tag.Copyright)
	}
}
