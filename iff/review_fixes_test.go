package iff

// Regression tests for the RIFF/AIFF review findings (chunk walking, INFO
// padding, ID3v2 chunk writing, AIFC properties).

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

func riffChunk(id string, data []byte, pad bool) []byte {
	out := []byte(id)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, data...)
	if pad {
		out = append(out, 0)
	}
	return out
}

//nolint:unparam // pad kept for explicit call sites
func formChunk(id string, data []byte, pad bool) []byte {
	out := []byte(id)
	out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, data...)
	if pad {
		out = append(out, 0)
	}
	return out
}

func wavFmtChunk() []byte {
	var f []byte
	f = binary.LittleEndian.AppendUint16(f, 1) // PCM
	f = binary.LittleEndian.AppendUint16(f, 1) // channels
	f = binary.LittleEndian.AppendUint32(f, 8000)
	f = binary.LittleEndian.AppendUint32(f, 8000)
	f = binary.LittleEndian.AppendUint16(f, 1)
	f = binary.LittleEndian.AppendUint16(f, 8)
	return riffChunk("fmt ", f, false)
}

func wavWith(chunks ...[]byte) []byte {
	body := []byte("WAVE")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := []byte("RIFF")
	out = binary.LittleEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

func aiffWith(kind string, chunks ...[]byte) []byte {
	body := []byte(kind)
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := []byte("FORM")
	out = binary.BigEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

func tagOnlyOptions() meta.ParseOptions {
	po := meta.NewParseOptions()
	po.ReadProperties = false
	return po
}

// --- RIFF INFO padding ---

func TestRiffInfoInvalidUtf8KeepsPadding(t *testing.T) {
	list := []byte("INFO")
	// IART: 3 invalid UTF-8 bytes + pad byte
	list = append(list, riffChunk("IART", []byte{0xFF, 0xFE, 0xFD}, true)...)
	// INAM: odd-sized, padded
	list = append(list, riffChunk("INAM", []byte("Hello"), true)...)
	list = append(list, riffChunk("IGNR", []byte("Rock"), false)...)
	data := wavWith(riffChunk("LIST", list, false))

	f, err := WavFileReadFrom(bytes.NewReader(data), tagOnlyOptions())
	if err != nil {
		t.Fatal(err)
	}
	info, ok := f.RiffInfo.Get()
	if !ok {
		t.Fatal("no RIFF INFO")
	}
	if _, ok := info.Get("IART"); ok {
		t.Error("IART with invalid UTF-8 should be skipped")
	}
	if v, ok := info.Get("INAM"); !ok || v != "Hello" {
		t.Errorf("INAM = %q, %v; want Hello", v, ok)
	}
	if v, ok := info.Get("IGNR"); !ok || v != "Rock" {
		t.Errorf("IGNR = %q, %v; want Rock", v, ok)
	}

	strict := tagOnlyOptions()
	strict.ParsingMode = meta.ParsingModeStrict
	if _, err := WavFileReadFrom(bytes.NewReader(data), strict); err == nil {
		t.Error("strict mode should fail on invalid UTF-8")
	}
}

// An odd-sized item whose pad byte was not written must not swallow the first
// byte of the next FourCC.
func TestRiffInfoOddItemMissingPad(t *testing.T) {
	list := []byte("INFO")
	list = append(list, riffChunk("INAM", []byte("Hello"), false)...)
	list = append(list, riffChunk("IGNR", []byte("Rock"), false)...)
	data := wavWith(riffChunk("LIST", list, false))

	f, err := WavFileReadFrom(bytes.NewReader(data), tagOnlyOptions())
	if err != nil {
		t.Fatal(err)
	}
	info, _ := f.RiffInfo.Get()
	if info == nil {
		t.Fatal("no RIFF INFO")
	}
	if v, ok := info.Get("INAM"); !ok || v != "Hello" {
		t.Errorf("INAM = %q, %v", v, ok)
	}
	if v, ok := info.Get("IGNR"); !ok || v != "Rock" {
		t.Errorf("IGNR = %q, %v", v, ok)
	}
}

// --- ID3v2 chunk with extended header ---

func TestWavId3ChunkExtendedHeader(t *testing.T) {
	ext := []byte{0, 0, 0, 6, 1, 0} // v2.4: synchsafe size, 1 flag byte, no flags
	frame := []byte("TIT2")
	frame = append(frame, 0, 0, 0, 4, 0, 0) // size 4 (synchsafe), no flags
	frame = append(frame, 3)                // UTF-8
	frame = append(frame, "Foo"...)
	content := append(append([]byte{}, ext...), frame...)
	id3 := []byte{'I', 'D', '3', 4, 0, 0x40, 0, 0, 0, byte(len(content))}
	id3 = append(id3, content...)

	data := wavWith(riffChunk("ID3 ", id3, len(id3)%2 == 1))
	f, err := WavFileReadFrom(bytes.NewReader(data), tagOnlyOptions())
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := f.ID3v2.Get()
	if !ok {
		t.Fatal("no ID3v2 tag")
	}
	title, ok := tag.Tag().GetString(meta.ItemKeyTrackTitle)
	if !ok || title != "Foo" {
		t.Fatalf("title = %q, %v; want Foo", title, ok)
	}
}

// --- Wrong magic ---

func TestWrongMagicIsUnknownFormat(t *testing.T) {
	var uf *meta.UnknownFormatError
	_, err := WavFileReadFrom(bytes.NewReader([]byte("RIFF\x00\x00\x00\x00AVI ")), meta.NewParseOptions())
	if !errors.As(err, &uf) {
		t.Errorf("WAV: err = %v, want UnknownFormatError", err)
	}
	_, err = AiffFileReadFrom(bytes.NewReader([]byte("FORM\x00\x00\x00\x00XXXX")), meta.NewParseOptions())
	if !errors.As(err, &uf) {
		t.Errorf("AIFF: err = %v, want UnknownFormatError", err)
	}
}

// --- Chunk walk end position (overall bitrate) ---

func TestWavOverallBitrateWalkEndInvalidFourCC(t *testing.T) {
	audio := make([]byte, 80) // 10 ms at 8000 Hz, 8-bit mono
	data := wavWith(wavFmtChunk(), riffChunk("data", audio, false))
	data = append(data, make([]byte, 16)...) // trailing junk: invalid FourCC

	f, err := WavFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	// Rust stops 4 bytes into the junk chunk (FourCC read, then rejected).
	walkEnd := uint64(12 + 24 + 8 + 80 + 4)
	if want := uint32(util.DivRound(walkEnd*8, 10)); f.Properties.OverallBitrate != want {
		t.Fatalf("overall bitrate = %d, want %d", f.Properties.OverallBitrate, want)
	}
}

func TestWavOverallBitrateWalkEndOversizedChunk(t *testing.T) {
	audio := make([]byte, 80)
	data := wavWith(wavFmtChunk(), riffChunk("data", audio, false))
	data = append(data, "junk"...)
	data = binary.LittleEndian.AppendUint32(data, 0x7FFFFFFF)
	data = append(data, make([]byte, 8)...)

	f, err := WavFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	// Valid FourCC + oversized: the 8-byte header was consumed.
	walkEnd := uint64(12 + 24 + 8 + 80 + 8)
	if want := uint32(util.DivRound(walkEnd*8, 10)); f.Properties.OverallBitrate != want {
		t.Fatalf("overall bitrate = %d, want %d", f.Properties.OverallBitrate, want)
	}
}

// 8000 Hz as an IEEE 754 extended-precision number.
var f80Rate8000 = []byte{0x40, 0x0B, 0xFA, 0, 0, 0, 0, 0, 0, 0}

func aiffComm(frames uint32, rate []byte, extra ...byte) []byte {
	var c []byte
	c = binary.BigEndian.AppendUint16(c, 1)
	c = binary.BigEndian.AppendUint32(c, frames)
	c = binary.BigEndian.AppendUint16(c, 8)
	c = append(c, rate...)
	return append(c, extra...)
}

func TestAiffOverallBitrateWalkEndInvalidFourCC(t *testing.T) {
	ssnd := make([]byte, 8+80)
	data := aiffWith("AIFF", formChunk("COMM", aiffComm(80, f80Rate8000), false), formChunk("SSND", ssnd, false))
	data = append(data, make([]byte, 16)...)

	f, err := AiffFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	walkEnd := float64(12 + 26 + 8 + 88 + 4)
	if want := util.F64ToU32(walkEnd*8/10 + 0.5); f.Properties.OverallBitrate != want {
		t.Fatalf("overall bitrate = %d, want %d", f.Properties.OverallBitrate, want)
	}
}

// --- AIFC properties ---

func TestAiffCompressionNoneName(t *testing.T) {
	if got := AiffCompressionTypeFromId([4]byte{'N', 'O', 'N', 'E'}).CompressionName(); got != "not compressed" {
		t.Fatalf("NONE name = %q", got)
	}
	var p AiffProperties
	if err := aiffReadProperties(aiffComm(80, f80Rate8000, append([]byte("NONE"), 0)...), true, 88, 138, &p); err != nil {
		t.Fatal(err)
	}
	ct, ok := p.CompressionType.Get()
	if !ok || ct != AiffCompressionNone || ct.CompressionName() != "not compressed" {
		t.Fatalf("compression = %+v, %v", ct, ok)
	}
}

func TestAiffUnknownCompressionName(t *testing.T) {
	var p AiffProperties
	comm := aiffComm(80, f80Rate8000, append([]byte("xxxx"), 3, 'a', 'b', 'c')...)
	if err := aiffReadProperties(comm, true, 88, 138, &p); err != nil {
		t.Fatal(err)
	}
	if ct, _ := p.CompressionType.Get(); ct.CompressionName() != "abc" || ct.ID() != [4]byte{'x', 'x', 'x', 'x'} {
		t.Fatalf("compression = %+v", ct)
	}

	// Invalid UTF-8 name propagates
	comm = aiffComm(80, f80Rate8000, append([]byte("xxxx"), 3, 0xFF, 0xFE, 0xFD)...)
	if err := aiffReadProperties(comm, true, 88, 138, &AiffProperties{}); err == nil {
		t.Error("invalid UTF-8 compression name should error")
	}

	// Short read of the name propagates
	comm = aiffComm(80, f80Rate8000, append([]byte("xxxx"), 5, 'a', 'b')...)
	if err := aiffReadProperties(comm, true, 88, 138, &AiffProperties{}); err == nil {
		t.Error("truncated compression name should error")
	}
}

func TestAiffPropertiesSaturatingCasts(t *testing.T) {
	// 2^40 Hz does not fit in u32; Rust's `as u32` saturates.
	rate := []byte{0x40, 0x27, 0x80, 0, 0, 0, 0, 0, 0, 0}
	var p AiffProperties
	if err := aiffReadProperties(aiffComm(1, rate), false, 1000, 1000, &p); err != nil {
		t.Fatal(err)
	}
	if p.SampleRate != math.MaxUint32 {
		t.Fatalf("sample rate = %d, want saturated", p.SampleRate)
	}
	if p.OverallBitrate != math.MaxUint32 || p.AudioBitrate != math.MaxUint32 {
		t.Fatalf("bitrates = %d/%d, want saturated", p.OverallBitrate, p.AudioBitrate)
	}
}

// --- ID3v2 chunk writing (Rust write_to_chunk_file) ---

func opts(padding uint32) meta.WriteOptions {
	wo := meta.NewWriteOptions()
	wo.PreferredPadding = padding
	return wo
}

func fillBytes(n int, b byte) []byte { return bytes.Repeat([]byte{b}, n) }

func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

func TestWavWriteId3JunkShrinkInPlace(t *testing.T) {
	wo := opts(1024)
	existing := iffEncodeId3v2Chunk(fillBytes(40, 'a'), binary.LittleEndian, wo)
	wav := wavWith(wavFmtChunk(), existing, riffChunk("data", fillBytes(8, 1), false))
	start := 12 + 24
	file := util.NewBytesFile(append([]byte{}, wav...))

	if err := wavWriteId3v2Chunk(file, fillBytes(20, 'b'), wo); err != nil {
		t.Fatal(err)
	}
	got := file.Bytes()
	if len(got) != len(wav) {
		t.Fatalf("length changed: %d -> %d", len(wav), len(got))
	}
	want := iffEncodeId3v2Chunk(fillBytes(20, 'b'), binary.LittleEndian, wo)
	if !bytes.Equal(got[start:start+len(want)], want) {
		t.Fatal("new ID3 chunk not written in place")
	}
	j := start + len(want)
	if string(got[j:j+4]) != "JUNK" || le32(got, j+4) != uint32(len(existing)-len(want)-8) {
		t.Fatalf("expected JUNK of size %d, got %q/%d", len(existing)-len(want)-8, got[j:j+4], le32(got, j+4))
	}
	if !bytes.Equal(got[start+len(existing):], wav[start+len(existing):]) {
		t.Fatal("trailing chunks modified")
	}
	if le32(got, 4) != le32(wav, 4) {
		t.Fatal("RIFF size changed")
	}
}

func TestWavWriteId3RemoveBecomesJunk(t *testing.T) {
	wo := opts(1024)
	existing := iffEncodeId3v2Chunk(fillBytes(40, 'a'), binary.LittleEndian, wo)
	wav := wavWith(wavFmtChunk(), existing, riffChunk("data", fillBytes(8, 1), false))
	start := 12 + 24
	file := util.NewBytesFile(append([]byte{}, wav...))

	if err := wavWriteId3v2Chunk(file, nil, wo); err != nil {
		t.Fatal(err)
	}
	got := file.Bytes()
	if len(got) != len(wav) || string(got[start:start+4]) != "JUNK" {
		t.Fatalf("expected in-place JUNK rename, got len %d, id %q", len(got), got[start:start+4])
	}
	if !bytes.Equal(got[start+4:], wav[start+4:]) {
		t.Fatal("only the FourCC should change")
	}
}

func TestWavWriteId3RemoveWithoutPaddingDropsChunk(t *testing.T) {
	wo := opts(0)
	existing := iffEncodeId3v2Chunk(fillBytes(40, 'a'), binary.LittleEndian, wo)
	data := riffChunk("data", fillBytes(8, 1), false)
	wav := wavWith(wavFmtChunk(), existing, data)
	file := util.NewBytesFile(append([]byte{}, wav...))

	if err := wavWriteId3v2Chunk(file, nil, wo); err != nil {
		t.Fatal(err)
	}
	if want := wavWith(wavFmtChunk(), data); !bytes.Equal(file.Bytes(), want) {
		t.Fatalf("got % x\nwant % x", file.Bytes(), want)
	}
}

func TestWavWriteId3GrowMovesChunkToEnd(t *testing.T) {
	wo := opts(1024)
	existing := iffEncodeId3v2Chunk(fillBytes(40, 'a'), binary.LittleEndian, wo)
	data := riffChunk("data", fillBytes(8, 1), false)
	wav := wavWith(wavFmtChunk(), existing, data)
	file := util.NewBytesFile(append([]byte{}, wav...))

	if err := wavWriteId3v2Chunk(file, fillBytes(101, 'c'), wo); err != nil {
		t.Fatal(err)
	}
	// Rust drains the old chunk and appends the new one at the stream end.
	want := wavWith(wavFmtChunk(), data, iffEncodeId3v2Chunk(fillBytes(101, 'c'), binary.LittleEndian, wo))
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatalf("got % x\nwant % x", file.Bytes(), want)
	}
}

func TestWavWriteId3AtEndOfFile(t *testing.T) {
	wo := opts(1024)
	existing := iffEncodeId3v2Chunk(fillBytes(40, 'a'), binary.LittleEndian, wo)
	wav := wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false), existing)

	// grow
	file := util.NewBytesFile(append([]byte{}, wav...))
	if err := wavWriteId3v2Chunk(file, fillBytes(60, 'b'), wo); err != nil {
		t.Fatal(err)
	}
	want := wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false), iffEncodeId3v2Chunk(fillBytes(60, 'b'), binary.LittleEndian, wo))
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatal("grow at end mismatch")
	}

	// shrink: truncated, no JUNK
	file = util.NewBytesFile(append([]byte{}, wav...))
	if err := wavWriteId3v2Chunk(file, fillBytes(10, 'b'), wo); err != nil {
		t.Fatal(err)
	}
	want = wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false), iffEncodeId3v2Chunk(fillBytes(10, 'b'), binary.LittleEndian, wo))
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatal("shrink at end mismatch")
	}

	// remove
	file = util.NewBytesFile(append([]byte{}, wav...))
	if err := wavWriteId3v2Chunk(file, nil, wo); err != nil {
		t.Fatal(err)
	}
	want = wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false))
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatal("remove at end mismatch")
	}
}

func TestWavWriteId3AppendBeforeTrailingJunk(t *testing.T) {
	wo := opts(1024)
	base := wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false))
	file := util.NewBytesFile(append(append([]byte{}, base...), "TRAILING"...))

	if err := wavWriteId3v2Chunk(file, fillBytes(11, 'z'), wo); err != nil {
		t.Fatal(err)
	}
	want := append(wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false), iffEncodeId3v2Chunk(fillBytes(11, 'z'), binary.LittleEndian, wo)), "TRAILING"...)
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatalf("got % x\nwant % x", file.Bytes(), want)
	}

	// Nothing to remove: untouched
	file = util.NewBytesFile(append([]byte{}, base...))
	if err := wavWriteId3v2Chunk(file, nil, wo); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file.Bytes(), base) {
		t.Fatal("file modified when there was nothing to remove")
	}
}

func TestWavWriteId3ShortFile(t *testing.T) {
	wav := wavWith(wavFmtChunk(), riffChunk("data", fillBytes(8, 1), false))
	var sm *meta.SizeMismatchError

	// Declared size exceeds the file by 2 bytes (fits within the file length)
	truncated := wav[:len(wav)-2]
	err := wavWriteId3v2Chunk(util.NewBytesFile(append([]byte{}, truncated...)), fillBytes(10, 'a'), opts(1024))
	var fErr *meta.FileEncodingError
	if !errors.As(err, &fErr) || !errors.As(err, &sm) {
		t.Fatalf("err = %v, want FileEncodingError(SizeMismatchError)", err)
	}

	// Far shorter than declared: strict mode reports the mismatch from the
	// chunk reader, best-attempt treats the missing stream as unknown format.
	short := []byte("RIFF\x64\x00\x00\x00WAVE")
	strict := opts(1024)
	strict.ParseOptions.ParsingMode = meta.ParsingModeStrict
	err = wavWriteId3v2Chunk(util.NewBytesFile(append([]byte{}, short...)), fillBytes(10, 'a'), strict)
	if !errors.As(err, &sm) {
		t.Fatalf("strict: err = %v, want SizeMismatchError", err)
	}
	var uf *meta.UnknownFormatError
	err = wavWriteId3v2Chunk(util.NewBytesFile(append([]byte{}, short...)), fillBytes(10, 'a'), opts(1024))
	if !errors.As(err, &uf) {
		t.Fatalf("lenient: err = %v, want UnknownFormatError", err)
	}
}

func TestAiffWriteId3ReplaceMovesChunkToEnd(t *testing.T) {
	wo := opts(1024)
	comm := formChunk("COMM", aiffComm(80, f80Rate8000), false)
	ssnd := formChunk("SSND", fillBytes(88, 0), false)
	existing := iffEncodeId3v2Chunk(fillBytes(20, 'a'), binary.BigEndian, wo)
	aiff := aiffWith("AIFF", comm, existing, ssnd)
	file := util.NewBytesFile(append([]byte{}, aiff...))

	// AIFF has no JUNK chunk: even a smaller tag rewrites the file.
	if err := aiffWriteId3v2Chunk(file, fillBytes(9, 'b'), wo); err != nil {
		t.Fatal(err)
	}
	want := aiffWith("AIFF", comm, ssnd, iffEncodeId3v2Chunk(fillBytes(9, 'b'), binary.BigEndian, wo))
	if !bytes.Equal(file.Bytes(), want) {
		t.Fatalf("got % x\nwant % x", file.Bytes(), want)
	}
}
