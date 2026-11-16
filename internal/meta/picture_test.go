package meta

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestPictureTypeUndefined(t *testing.T) {
	u := PictureTypeFromApeKey("Cover Art (Whatever)")
	if u.AsU8() != 0 {
		t.Errorf("unmapped APE key AsU8 = %d, want 0 (Rust Undefined(0))", u.AsU8())
	}
	if u == PictureTypeOther {
		t.Errorf("Undefined(0) must differ from Other")
	}
	if _, ok := u.AsApeKey(); ok {
		t.Errorf("Undefined(0) must have no APE key")
	}
	if u != PictureTypeUndefined(0) {
		t.Errorf("from_ape_key(unmapped) != Undefined(0)")
	}
	// named types are unchanged
	for i, key := range ApePictureTypes {
		if PictureTypeFromApeKey(key) != PictureType(i) {
			t.Errorf("from_ape_key(%q) = %v", key, PictureTypeFromApeKey(key))
		}
		if k, ok := PictureType(i).AsApeKey(); !ok || k != key {
			t.Errorf("as_ape_key(%d) = %q,%v", i, k, ok)
		}
	}
	// Undefined(n) for n > 20 is just the byte; Undefined(n<=20) is distinct but keeps its byte.
	if PictureTypeUndefined(25) != PictureTypeFromU8(25) || PictureTypeUndefined(25).AsU8() != 25 {
		t.Errorf("Undefined(25) mismatch")
	}
	if PictureTypeUndefined(3) == PictureTypeCoverFront || PictureTypeUndefined(3).AsU8() != 3 {
		t.Errorf("Undefined(3) mismatch")
	}
	if _, ok := PictureTypeFromU8(21).AsApeKey(); ok {
		t.Errorf("type 21 has no APE key")
	}
	// the byte written to a FLAC block is 0 for the unmapped APE type
	p := PictureUnchecked([]byte{1}).PicType(u).Build()
	if raw := p.AsFlacBytes(PictureInformation{}, false); raw[3] != 0 {
		t.Errorf("flac type byte = %d", raw[3])
	}
}

func flacPicture(extra int) []byte {
	data := bytes.Repeat([]byte{0xAB}, 10+extra)
	p := PictureUnchecked(data).PicType(PictureTypeCoverFront).MimeType(MimeTypePng).Description("d").Build()
	return p.AsFlacBytes(PictureInformation{Width: 1, Height: 2, ColorDepth: 3, NumColors: 4}, false)
}

func TestPictureFromFlacBytesRoundTrip(t *testing.T) {
	raw := flacPicture(0)
	for _, encoded := range []bool{false, true} {
		in := raw
		if encoded {
			in = []byte(base64.StdEncoding.EncodeToString(raw))
		}
		pic, info, err := PictureFromFlacBytes(in, encoded, ParsingModeStrict)
		if err != nil {
			t.Fatalf("encoded=%v: %v", encoded, err)
		}
		if pic.PicType() != PictureTypeCoverFront || pic.mimeStr() != "image/png" || info.NumColors != 4 || len(pic.Data()) != 10 {
			t.Fatalf("bad decode: %+v %+v", pic, info)
		}
	}
}

// data_encoding::BASE64 is strict: no newlines, no non-zero trailing bits.
func TestPictureFromFlacBytesStrictBase64(t *testing.T) {
	var raw []byte
	var enc string
	for extra := 0; ; extra++ {
		raw = flacPicture(extra)
		enc = base64.StdEncoding.EncodeToString(raw)
		if enc[len(enc)-1] == '=' {
			break
		}
	}
	if _, _, err := PictureFromFlacBytes([]byte(enc), true, ParsingModeStrict); err != nil {
		t.Fatalf("canonical input rejected: %v", err)
	}

	for _, nl := range []string{"\n", "\r\n", "\r"} {
		bad := enc[:8] + nl + enc[8:]
		if _, _, err := PictureFromFlacBytes([]byte(bad), true, ParsingModeStrict); err == nil {
			t.Errorf("newline %q accepted", nl)
		}
	}
	if _, _, err := PictureFromFlacBytes([]byte(enc+"\n"), true, ParsingModeStrict); err == nil {
		t.Errorf("trailing newline accepted")
	}

	// flip a trailing (padding) bit of the last data character
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	i := len(enc) - 1
	for enc[i] == '=' {
		i--
	}
	idx := bytes.IndexByte([]byte(alphabet), enc[i])
	nc := alphabet[idx|1]
	bad := enc[:i] + string(nc) + enc[i+1:]
	if _, _, err := PictureFromFlacBytes([]byte(bad), true, ParsingModeStrict); err == nil {
		t.Errorf("non-canonical trailing bits accepted")
	}
	if _, _, err := PictureFromFlacBytes([]byte("!!!!"), true, ParsingModeStrict); err == nil {
		t.Errorf("non-alphabet bytes accepted")
	}
}

func TestPictureFromFlacBytesErrors(t *testing.T) {
	if _, _, err := PictureFromFlacBytes(make([]byte, 31), false, ParsingModeBestAttempt); err == nil {
		t.Error("short input accepted")
	}
	raw := flacPicture(0)
	raw[3] = 1 // picture type > 255
	raw[2] = 1
	if _, _, err := PictureFromFlacBytes(raw, false, ParsingModeStrict); err == nil {
		t.Error("strict mode accepted picture type 257")
	}
	if p, _, err := PictureFromFlacBytes(raw, false, ParsingModeBestAttempt); err != nil || p.PicType().AsU8() != 1 {
		t.Errorf("best-attempt: %v %v", p.PicType(), err)
	}
	var pe *PictureParseError
	_, _, err := PictureFromFlacBytes(make([]byte, 5), false, ParsingModeStrict)
	if !errors.As(err, &pe) {
		t.Errorf("error type %T", err)
	}
}

func TestMimetypeFromBin(t *testing.T) {
	cases := map[string]struct {
		b    []byte
		want MimeType
	}{
		"png":     {[]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, MimeTypePng},
		"jpeg":    {[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0}, MimeTypeJpeg},
		"gif87":   {[]byte("GIF87a\x00\x00"), MimeTypeGif},
		"gif89":   {[]byte("GIF89a\x00\x00"), MimeTypeGif},
		"bmp":     {[]byte("BM\x00\x00\x00\x00\x00\x00"), MimeTypeBmp},
		"tiff-le": {[]byte("II*\x00\x00\x00\x00\x00"), MimeTypeTiff},
		"tiff-be": {[]byte("MM\x00*\x00\x00\x00\x00"), MimeTypeTiff},
	}
	for n, c := range cases {
		got, err := MimetypeFromBin(c.b)
		if err != nil || got != c.want {
			t.Errorf("%s: %v %v", n, got, err)
		}
	}
	if _, err := MimetypeFromBin([]byte("GIF88a\x00\x00")); err == nil {
		t.Error("GIF88a accepted")
	}
}
