package ape

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// apeTestMac builds a minimal Monkey's Audio stream (version >= 3980):
// "MAC ", the version, a 52-byte file descriptor, the 24-byte MAC header and
// some audio payload so the file is longer than the 32-byte footer probe.
//
//nolint:unparam // version is a parameter for readability at call sites
func apeTestMac(version, channels uint16, totalFrames, finalFrameBlocks uint32) []byte {
	b := []byte("MAC ")
	b = binary.LittleEndian.AppendUint16(b, version)
	desc := make([]byte, 46)
	binary.LittleEndian.PutUint32(desc[2:], 52)
	b = append(b, desc...)
	hdr := make([]byte, 24)
	binary.LittleEndian.PutUint16(hdr[0:], 2000)
	binary.LittleEndian.PutUint32(hdr[4:], 73728)
	binary.LittleEndian.PutUint32(hdr[8:], finalFrameBlocks)
	binary.LittleEndian.PutUint32(hdr[12:], totalFrames)
	binary.LittleEndian.PutUint16(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[18:], channels)
	binary.LittleEndian.PutUint32(hdr[20:], 44100)
	b = append(b, hdr...)
	return append(b, make([]byte, 64)...)
}

func apeTestSynchsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

// apeTestId3v24 builds an ID3v2.4 tag holding a single TIT2 frame.
func apeTestId3v24(title string, footer bool) []byte {
	text := append([]byte{0x00}, title...)
	frame := append([]byte("TIT2"), apeTestSynchsafe(len(text))...)
	frame = append(frame, 0, 0)
	frame = append(frame, text...)
	var flags byte
	if footer {
		flags = 0x10
	}
	b := append([]byte("ID3\x04\x00"), flags)
	b = append(b, apeTestSynchsafe(len(frame))...)
	b = append(b, frame...)
	if footer {
		b = append(b, "3DI\x04\x00\x10"...)
		b = append(b, apeTestSynchsafe(len(frame))...)
	}
	return b
}

func apeTestTag(t *testing.T, kv ...string) []byte {
	t.Helper()
	tag := NewApeTag()
	for i := 0; i < len(kv); i += 2 {
		tag.Push(mustApeItem(t, kv[i], meta.ItemValueText(kv[i+1])))
	}
	data, err := encodeApeTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func apeTestRead(t *testing.T, data []byte, mode meta.ParsingMode) (*ApeFile, error) {
	t.Helper()
	po := meta.NewParseOptions()
	po.ParsingMode = mode
	return ApeFileReadFrom(bytes.NewReader(data), po)
}

func apeTestTitle(t *testing.T, f *ApeFile) string {
	t.Helper()
	tag, ok := f.ID3v2.Get()
	if !ok {
		t.Fatal("expected an ID3v2 tag")
	}
	s, _ := tag.Tag().Title()
	return s
}

// A leading ID3v2.4 tag with a footer: the content starts right after the
// 10-byte header and the MAC header is found after the footer.
func TestApeFileLeadingId3v2Footer(t *testing.T) {
	data := append(apeTestId3v24("Hello", true), apeTestMac(3990, 2, 10, 100)...)
	f, err := apeTestRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if got := apeTestTitle(t, f); got != "Hello" {
		t.Fatalf("title = %q, want Hello", got)
	}
	if f.Properties.SampleRate != 44100 || f.Properties.Duration == 0 {
		t.Fatalf("properties = %+v", f.Properties)
	}
}

// Junk before the ID3v2 tag lies inside the tag's range; the MAC header
// follows the tag end.
func TestApeFileLeadingId3v2Junk(t *testing.T) {
	data := append([]byte("JUNK"), apeTestId3v24("Hello", false)...)
	data = append(data, apeTestMac(3990, 2, 10, 100)...)
	f, err := apeTestRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if got := apeTestTitle(t, f); got != "Hello" {
		t.Fatalf("title = %q, want Hello", got)
	}
	if f.Properties.SampleRate != 44100 {
		t.Fatalf("properties = %+v", f.Properties)
	}
}

// Rust finds the MAC magic regardless of read_properties, so a stream
// without it fails even when properties are skipped.
func TestApeFileMissingMacErrorsWithoutProperties(t *testing.T) {
	po := meta.NewParseOptions()
	po.ReadProperties = false
	if _, err := ApeFileReadFrom(bytes.NewReader(bytes.Repeat([]byte("x"), 128)), po); err == nil {
		t.Fatal("expected an error for a stream without a MAC header")
	}
}

// An APE tag at the start of the file is read and the MAC header after it is
// still found.
func TestApeFileLeadingApeTag(t *testing.T) {
	data := append(apeTestTag(t, "Title", "Leading"), apeTestMac(3990, 2, 10, 100)...)
	f, err := apeTestRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.Ape.Get()
	if !ok {
		t.Fatal("expected the leading APE tag")
	}
	if got, _ := tag.Title(); got != "Leading" {
		t.Fatalf("title = %q, want Leading", got)
	}
	if f.Properties.SampleRate != 44100 || f.Properties.Duration == 0 {
		t.Fatalf("properties = %+v", f.Properties)
	}
}

// Rust keeps a single APE slot: the trailing tag overwrites the leading one.
func TestApeFileTrailingTagReplacesLeading(t *testing.T) {
	data := append(apeTestTag(t, "Title", "Leading"), apeTestMac(3990, 2, 10, 100)...)
	data = append(data, apeTestTag(t, "Title", "Trailing")...)
	f, err := apeTestRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.Ape.Get()
	if !ok {
		t.Fatal("expected an APE tag")
	}
	if got, _ := tag.Title(); got != "Trailing" {
		t.Fatalf("title = %q, want Trailing", got)
	}
	if tag.Len() != 1 {
		t.Fatalf("items = %d, want 1", tag.Len())
	}
}

// Rust propagates the trailing read_ape_tag error with `?` in every mode.
func TestApeFileTrailingTagErrorPropagates(t *testing.T) {
	footer := make([]byte, 32)
	copy(footer, "APETAGEX")
	binary.LittleEndian.PutUint32(footer[8:], 2000)
	binary.LittleEndian.PutUint32(footer[12:], 16) // size < 32
	data := append(apeTestMac(3990, 2, 10, 100), footer...)
	for _, mode := range []meta.ParsingMode{meta.ParsingModeStrict, meta.ParsingModeBestAttempt, meta.ParsingModeRelaxed} {
		_, err := apeTestRead(t, data, mode)
		var tpe *meta.TagParseError
		if !errors.As(err, &tpe) {
			t.Fatalf("mode %v: err = %v, want a TagParseError", mode, err)
		}
	}
}

// Rust's footer probe (`seek(Current(-32))?`) fails the read for streams
// shorter than 32 bytes past the MAC magic.
func TestApeFileShortStreamSeekErrorPropagates(t *testing.T) {
	if _, err := apeTestRead(t, []byte("MAC \x00\x00\x00\x00"), meta.ParsingModeRelaxed); err == nil {
		t.Fatal("expected the failed footer seek to propagate")
	}
}

// Only the verify() failures (channels, frames) are mode dependent; a
// truncated header is a hard error in every mode.
func TestApePropertiesErrorsAreHard(t *testing.T) {
	truncated := []byte("MAC \x96\x0f") // version 3990
	truncated = append(truncated, make([]byte, 46+10)...)
	for _, mode := range []meta.ParsingMode{meta.ParsingModeStrict, meta.ParsingModeBestAttempt, meta.ParsingModeRelaxed} {
		if _, err := apeTestRead(t, truncated, mode); err == nil {
			t.Fatalf("mode %v: expected an error for a truncated MAC header", mode)
		}
	}
}

func TestApePropertiesVerifyFailureIsModeDependent(t *testing.T) {
	zeroChannels := apeTestMac(3990, 0, 1, 100)
	if _, err := apeTestRead(t, zeroChannels, meta.ParsingModeStrict); err == nil {
		t.Fatal("strict: expected an invalid channel count error")
	}
	f, err := apeTestRead(t, zeroChannels, meta.ParsingModeRelaxed)
	if err != nil {
		t.Fatalf("relaxed: %v", err)
	}
	if f.Properties.SampleRate != 44100 || f.Properties.Duration != 0 {
		t.Fatalf("relaxed properties = %+v", f.Properties)
	}
	zeroFrames := apeTestMac(3990, 2, 0, 100)
	if _, err := apeTestRead(t, zeroFrames, meta.ParsingModeStrict); err == nil {
		t.Fatal("strict: expected a no-frames error")
	}
	if _, err := apeTestRead(t, zeroFrames, meta.ParsingModeBestAttempt); err != nil {
		t.Fatalf("best attempt: %v", err)
	}
}

// A zero-length stream divides to +Inf; Rust's saturating `as u32` yields
// u32::MAX, which must not depend on the platform's float conversion.
func TestApePropertiesZeroLengthBitrateSaturates(t *testing.T) {
	f, err := apeTestRead(t, apeTestMac(3990, 2, 1, 0), meta.ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.Duration != 0 {
		t.Fatalf("duration = %v, want 0", f.Properties.Duration)
	}
	if f.Properties.AudioBitrate != math.MaxUint32 || f.Properties.OverallBitrate != math.MaxUint32 {
		t.Fatalf("bitrates = %d/%d, want saturated", f.Properties.AudioBitrate, f.Properties.OverallBitrate)
	}
}

// Rust uses checked_sub on the stream length: a tag larger than what is left
// is a size mismatch rather than an unsigned wrap-around.
func TestReadEndTagsSizeMismatch(t *testing.T) {
	data := append(make([]byte, 100), apeTestTag(t, "Title", "Hello")...)
	if _, _, _, err := ReadEndTags(bytes.NewReader(data), 0, 10, meta.NewParseOptions()); err == nil {
		t.Fatal("expected an error")
	} else {
		var sm *meta.SizeMismatchError
		if !errors.As(err, &sm) {
			t.Fatalf("err = %v, want SizeMismatchError", err)
		}
	}

	streamLen, _, tag, err := ReadEndTags(bytes.NewReader(data), 0, uint64(len(data)), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if tag == nil || tag.Len() != 1 {
		t.Fatalf("tag = %v", tag)
	}
	if streamLen != 100 {
		t.Fatalf("stream length = %d, want 100", streamLen)
	}
}

// Rust `create_ape_tag`: items are written in order with their own
// read-only flags, and the tag's read-only bit lands in both the header and
// footer flags.
func TestEncodeApeTagFlagsAndOrder(t *testing.T) {
	tag := NewApeTag()
	tag.ReadOnly = true
	tag.Push(mustApeItem(t, "Zeta", meta.ItemValueText("a")))
	locked := mustApeItem(t, "Alpha", meta.ItemValueLocator("b"))
	locked.ReadOnly = true
	tag.Push(locked)

	data, err := encodeApeTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	flags := func(off int) uint32 { return binary.LittleEndian.Uint32(data[off+20:]) }
	if got, want := flags(0), apeHeaderIsHeader|apeHeaderContainsFooter|apeHeaderContainsHeader|1; got != want {
		t.Fatalf("header flags = %#x, want %#x", got, want)
	}
	if got, want := flags(len(data)-32), apeHeaderContainsFooter|apeHeaderContainsHeader|1; got != want {
		t.Fatalf("footer flags = %#x, want %#x", got, want)
	}
	if got := binary.LittleEndian.Uint32(data[16:]); got != 2 {
		t.Fatalf("item count = %d, want 2", got)
	}
	if got, want := binary.LittleEndian.Uint32(data[12:]), uint32(len(data)-32); got != want {
		t.Fatalf("size = %d, want %d", got, want)
	}
	// First item: Zeta, plain text
	if got := binary.LittleEndian.Uint32(data[32+4:]); got != 0 {
		t.Fatalf("first item flags = %#x, want 0", got)
	}
	if !bytes.HasPrefix(data[32+8:], []byte("Zeta\x00a")) {
		t.Fatalf("first item = %q", data[32+8:48])
	}
	// Second item: Alpha, read-only locator
	second := 32 + 8 + len("Zeta\x00a")
	if got, want := binary.LittleEndian.Uint32(data[second+4:]), uint32(2<<1|1); got != want {
		t.Fatalf("second item flags = %#x, want %#x", got, want)
	}
	if !bytes.HasPrefix(data[second+8:], []byte("Alpha\x00b")) {
		t.Fatalf("second item = %q", data[second+8:second+20])
	}

	// A writable tag carries no read-only bit at all.
	tag.ReadOnly = false
	data, err = encodeApeTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	if flags(0)&1 != 0 || flags(len(data)-32)&1 != 0 {
		t.Fatal("read-only bit set on a writable tag")
	}
}

func TestEncodeApeTagEmpty(t *testing.T) {
	data, err := encodeApeTag(NewApeTag())
	if err != nil || len(data) != 0 {
		t.Fatalf("empty tag = %d bytes, err %v", len(data), err)
	}
}

func TestEncodeApeTagTooMuchData(t *testing.T) {
	if strconv.IntSize < 64 || testing.Short() {
		t.Skip("allocates a 4 GiB value")
	}
	tag := NewApeTag()
	tag.Push(mustApeItem(t, "Huge", meta.ItemValueBinary(make([]byte, math.MaxUint32))))
	_, err := encodeApeTag(tag)
	var tooMuch *meta.TooMuchDataError
	if !errors.As(err, &tooMuch) {
		t.Fatalf("err = %v, want TooMuchDataError", err)
	}
}

// Rust never retains items that are read-only on disk, and writes the tag's
// own read-only items (and the tag-level flag) as given.
func TestApeWriteToDoesNotRetainDiskReadOnly(t *testing.T) {
	existing := NewApeTag()
	keep := mustApeItem(t, "Keep", meta.ItemValueText("on disk"))
	keep.ReadOnly = true
	existing.Push(keep)
	onDisk, err := encodeApeTag(existing)
	if err != nil {
		t.Fatal(err)
	}
	file := meta.NewBytesFile(append(apeTestMac(3990, 2, 10, 100), onDisk...))

	next := NewApeTag()
	next.ReadOnly = true
	next.Push(mustApeItem(t, "Title", meta.ItemValueText("Hello")))
	locked := mustApeItem(t, "Locked", meta.ItemValueText("new"))
	locked.ReadOnly = true
	next.Push(locked)
	if err := ApeWriteTo(file, next, meta.NewWriteOptions()); err != nil {
		t.Fatal(err)
	}

	out := file.Bytes()
	f, err := ApeFileReadFrom(bytes.NewReader(out), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.Ape.Get()
	if !ok {
		t.Fatal("expected an APE tag after the write")
	}
	if _, ok := tag.Item("Keep"); ok {
		t.Fatal("on-disk read-only item was retained")
	}
	if got, _ := tag.Title(); got != "Hello" {
		t.Fatalf("title = %q", got)
	}
	item, ok := tag.Item("Locked")
	if !ok || !item.ReadOnly {
		t.Fatalf("Locked = %+v (found=%v), want a read-only item", item, ok)
	}
	if tag.Len() != 2 {
		t.Fatalf("items = %d, want 2", tag.Len())
	}
	if out[len(out)-32+20]&1 != 1 {
		t.Fatal("tag-level read-only flag missing from the footer")
	}
}

// A malformed existing header or footer tag must abort the write with the
// real error (Rust propagates it) instead of silently writing anyway.
func TestApeWriteRejectsMalformedExistingTag(t *testing.T) {
	mkMarker := func(size, flags uint32) []byte {
		b := []byte("APETAGEX")
		b = binary.LittleEndian.AppendUint32(b, 2000)
		b = binary.LittleEndian.AppendUint32(b, size)
		b = binary.LittleEndian.AppendUint32(b, 0) // item count
		b = binary.LittleEndian.AppendUint32(b, flags)
		return append(b, make([]byte, 8)...)
	}
	audio := apeTestMac(3990, 2, 10, 100)
	cases := []struct {
		name   string
		before []byte
	}{
		// CONTAINS_FOOTER | CONTAINS_HEADER with an impossible size of 0
		{"footer", append(bytes.Clone(audio), mkMarker(0, 0xC0000000)...)},
		// IS_HEADER | CONTAINS_FOOTER | CONTAINS_HEADER with size 0
		{"header", append(mkMarker(0, 0xE0000000), audio...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tag := NewApeTag()
			tag.Push(mustApeItem(t, "Title", meta.ItemValueText("x")))
			file := meta.NewBytesFile(bytes.Clone(tc.before))
			err := ApeWriteTo(file, tag, meta.NewWriteOptions())
			var mismatch *meta.SizeMismatchError
			if !errors.As(err, &mismatch) {
				t.Fatalf("ApeWriteTo err = %v, want SizeMismatchError", err)
			}
			if !bytes.Equal(file.Bytes(), tc.before) {
				t.Fatal("ApeWriteTo modified the file on failure")
			}
		})
	}
}
