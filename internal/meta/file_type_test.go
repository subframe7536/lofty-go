package meta

import (
	"bytes"
	"io"
	"math/rand"
	"path/filepath"
	"testing"
)

type testResolver struct{ ext string }

func (r testResolver) Extension() string           { return r.ext }
func (testResolver) Guess([]byte) bool             { return false }
func (testResolver) PrimaryTagType() TagType       { return TagTypeID3v2 }
func (testResolver) TagSupport(TagType) TagSupport { return TagSupportReadWrite }
func (testResolver) ReadFrom(io.ReadSeeker, ParseOptions) (*TaggedFile, error) {
	return NewTaggedFile(FileTypeCustom("rtest"), nil, FileProperties{}), nil
}

func TestFileTypeFromPath(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		path string
		want FileType
		ok   bool
	}{
		{"song.mp3", FileTypeMpeg, true},
		{"a" + sep + "b.WAV", FileTypeWav, true},
		{"x.tar.mp3", FileTypeMpeg, true},
		{".mp3", FileType{}, false}, // dot-file: no extension (Path::extension)
		{"dir" + sep + ".flac", FileType{}, false},
		{"..mp3", FileTypeMpeg, true}, // stem "." + extension "mp3"
		{"name.", FileType{}, false},
		{"README", FileType{}, false},
		{"", FileType{}, false},
		{"dir.mp3" + sep + "file", FileType{}, false},
		{"dir.mp3" + sep + "file" + sep, FileType{}, false},
		{"song.mp3" + sep, FileTypeMpeg, true}, // trailing separator ignored
		{"..", FileType{}, false},
	}
	for _, c := range cases {
		got, ok := FileTypeFromPath(c.path)
		if ok != c.ok || (ok && !got.Equal(c.want)) {
			t.Errorf("FileTypeFromPath(%q) = %v,%v want %v,%v", c.path, got, ok, c.want, c.ok)
		}
	}
}

func TestResolverWithEmptyExtensionNeverMatchesPaths(t *testing.T) {
	RegisterCustomResolver("rtest", testResolver{ext: ""})
	defer UnregisterCustomResolver("rtest")
	if ft, ok := FileTypeFromPath("README"); ok {
		t.Fatalf("extension-less path matched %v", ft)
	}
	if ft, ok := FileTypeFromExt(""); ok {
		t.Fatalf("empty ext matched %v", ft)
	}
	UnregisterCustomResolver("rtest")
	RegisterCustomResolver("rtest", testResolver{ext: "rtst"})
	if ft, ok := FileTypeFromPath("a.RTST"); !ok || !ft.IsCustom() {
		t.Fatalf("custom ext not matched: %v %v", ft, ok)
	}
}

func TestFileTypeFromExtASCIIFoldOnly(t *testing.T) {
	if ft, ok := FileTypeFromExt("AIFF"); !ok || !ft.Equal(FileTypeAiff) {
		t.Fatalf("AIFF: %v %v", ft, ok)
	}
	// U+0130 lowercases to 'i' under Unicode folding but not in Rust.
	if ft, ok := FileTypeFromExt("a\u0130ff"); ok {
		t.Fatalf("U+0130 matched %v", ft)
	}
}

func TestFileTypeExtensionsAllResolve(t *testing.T) {
	for _, e := range Extensions() {
		if _, ok := FileTypeFromExt(e); !ok {
			t.Errorf("extension %q does not resolve", e)
		}
	}
}

func TestFileTypeFromBuffer(t *testing.T) {
	ogg := func(codec string, off int) []byte {
		b := make([]byte, 36)
		copy(b, "OggS")
		copy(b[off:], codec)
		return b
	}
	cases := []struct {
		name string
		buf  []byte
		want FileType
		ok   bool
	}{
		{"MAC", []byte("MAC "), FileTypeApe, true},
		{"MAC3", []byte("MAC"), FileTypeApe, true},
		{"DSD", []byte("DSD "), FileTypeDsf, true},
		{"DSD short", []byte("DSD"), FileType{}, false},
		{"fLaC", []byte("fLaC"), FileTypeFlac, true},
		{"wvpk", []byte("wvpk"), FileTypeWavPack, true},
		{"MPCK", []byte("MPCK"), FileTypeMpc, true},
		{"MP+", []byte("MP+\x07"), FileTypeMpc, true},
		{"ftyp", []byte("\x00\x00\x00\x20ftypM4A "), FileTypeMp4, true},
		{"AIFF", []byte("FORM\x00\x00\x00\x00AIFF"), FileTypeAiff, true},
		{"AIFC", []byte("FORM\x00\x00\x00\x00AIFC"), FileTypeAiff, true},
		{"FORM other (no ftyp fallthrough)", []byte("FORM\x00\x00\x00\x00XXXXftyp"), FileType{}, false},
		{"WAVE", []byte("RIFF\x00\x00\x00\x00WAVE"), FileTypeWav, true},
		{"RIFF short", []byte("RIFF"), FileType{}, false},
		{"ADTS", []byte{0xFF, 0xF1}, FileTypeAac, true},
		{"MP3", []byte{0xFF, 0xFB}, FileTypeMpeg, true},
		{"MPEG 2.5", []byte{0xFF, 0xE2}, FileTypeMpeg, true},
		{"FF only", []byte{0xFF}, FileType{}, false},
		{"FF no sync", []byte{0xFF, 0x1F}, FileType{}, false},
		{"vorbis", ogg("vorbis", 29), FileTypeVorbis, true},
		{"opus", ogg("OpusHead", 28), FileTypeOpus, true},
		{"speex", ogg("Speex   ", 28), FileTypeSpeex, true},
		{"ogg too short", ogg("OpusHead", 28)[:35], FileType{}, false},
		{"empty", nil, FileType{}, false},
	}
	for _, c := range cases {
		got, ok := FileTypeFromBuffer(c.buf)
		if ok != c.ok || (ok && !got.Equal(c.want)) {
			t.Errorf("%s: got %v,%v want %v,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// refGuess is a reference model of Rust's Probe::guess_inner (probe.rs:330-433)
// for the stage after the first-36-bytes guess: ID3v2 skipping, the APE/FLAC/MPC
// idents, and the max_junk_bytes-limited frame sync search.
func refGuess(data []byte, maxJunk int) (FileType, bool) {
	n := len(data)
	if n > 36 {
		n = 36
	}
	res, ok := fileTypeFromBufferInner(data[:n])
	if !ok {
		return FileType{}, false
	}
	if res.determined {
		return res.fileType, true
	}
	pos := 0
	if !res.maybeJunk {
		pos = 10 + int(res.id3Size)
		var ident [4]byte
		if pos < len(data) {
			copy(ident[:], data[pos:])
		}
		switch {
		case ident[0] == 'M' && ident[1] == 'A' && ident[2] == 'C':
			return FileTypeApe, true
		case string(ident[:]) == "fLaC":
			return FileTypeFlac, true
		case string(ident[:]) == "MPCK" || (ident[0] == 'M' && ident[1] == 'P' && ident[2] == '+'):
			return FileTypeMpc, true
		}
	}
	if pos >= len(data) {
		return FileType{}, false
	}
	win := data[pos:]
	if len(win) > maxJunk {
		win = win[:maxJunk]
	}
	for i := 0; i+1 < len(win); i++ {
		if win[i] == 0xFF && win[i+1]&0xE0 == 0xE0 {
			if win[i+1]&0x10 > 0 && win[i+1]&0x06 == 0 {
				return FileTypeAac, true
			}
			return FileTypeMpeg, true
		}
	}
	return FileType{}, false
}

func TestGuessFileTypeFuzzAgainstModel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alpha := []byte{0xFF, 0xE0, 0xFB, 0xF1, 0x00, 'I', 'D', '3', 0x20, 'M', 'P', 'C', 'K', '+', 'A', 'f', 'L', 'a'}
	for iter := 0; iter < 20000; iter++ {
		n := rng.Intn(80)
		data := make([]byte, n)
		for i := range data {
			data[i] = alpha[rng.Intn(len(alpha))]
		}
		if rng.Intn(3) == 0 && n >= 10 {
			copy(data, "ID3\x04\x00\x00")
			for i := 6; i < 10; i++ {
				data[i] = byte(rng.Intn(3)) // small synchsafe sizes
			}
		}
		maxJunk := rng.Intn(50)
		start := 0
		if rng.Intn(4) == 0 && n > 5 {
			start = rng.Intn(5)
		}
		wantT, wantOk := refGuess(data[start:], maxJunk)
		r := bytes.NewReader(data)
		if _, err := r.Seek(int64(start), io.SeekStart); err != nil {
			t.Fatal(err)
		}
		opts := NewParseOptions()
		opts.MaxJunkBytes = maxJunk
		p, err := NewProbe(r).Options(opts).GuessFileType()
		if err != nil {
			t.Fatalf("err %v data=%x", err, data)
		}
		gotT, gotOk := p.FileType().Get()
		if pos, _ := r.Seek(0, io.SeekCurrent); int(pos) != start {
			t.Fatalf("position not restored: %d want %d (data=%x)", pos, start, data)
		}
		if gotOk != wantOk || (gotOk && !gotT.Equal(wantT)) {
			t.Fatalf("data=%x start=%d maxJunk=%d got %v,%v want %v,%v", data, start, maxJunk, gotT, gotOk, wantT, wantOk)
		}
	}
}

// Rust's max_junk_bytes is unsigned; a negative Go value must behave as 0
// rather than meaning "unbounded".
func TestGuessFileTypeNegativeMaxJunk(t *testing.T) {
	data := append([]byte("zz"), 0xFF, 0xFB, 0x90, 0x00)
	opts := NewParseOptions()
	opts.MaxJunkBytes = -1
	p, err := NewProbe(bytes.NewReader(data)).Options(opts).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	if ft, ok := p.FileType().Get(); ok {
		t.Fatalf("negative MaxJunkBytes found %v", ft)
	}
	opts.MaxJunkBytes = 16
	p, _ = NewProbe(bytes.NewReader(data)).Options(opts).GuessFileType()
	if ft, ok := p.FileType().Get(); !ok || !ft.Equal(FileTypeMpeg) {
		t.Fatalf("junk-prefixed MPEG not found: %v %v", ft, ok)
	}
}

// The ID3v2 size in the header is synchsafe: Rust masks every byte to 7 bits
// (`unsynch`). A set high bit must not push the skip distance out of the file.
func TestProbeID3SizeMasksHighBits(t *testing.T) {
	data := append([]byte("ID3\x03\x00\x00\x80\x00\x00\x00"), 0xFF, 0xFB, 0x90, 0x00, 0, 0, 0, 0)
	res, ok := fileTypeFromBufferInner(data[:10])
	if !ok || res.id3Size != 0 {
		t.Fatalf("id3Size = %d, want 0 (Rust unsynch masks the high bit)", res.id3Size)
	}
	p, err := NewProbe(bytes.NewReader(data)).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	if ft, ok := p.FileType().Get(); !ok || !ft.Equal(FileTypeMpeg) {
		t.Fatalf("got %v,%v want MPEG", ft, ok)
	}
}
