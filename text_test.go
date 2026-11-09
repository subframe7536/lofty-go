package lofty

import (
	"bytes"
	"testing"
)

// Ported from util/text.rs tests

const textTestString = "l\u00f8ft\u00a5"

func TestTextDecode(t *testing.T) {
	// No BOM
	utf16Decode, err := utf16DecodeBytes(
		[]byte{0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00},
		bigEndian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if utf16Decode != textTestString {
		t.Fatalf("got %q", utf16Decode)
	}

	// BOM test
	beDecode, err := decodeText(
		bytes.NewReader([]byte{0xFE, 0xFF, 0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00}),
		textDecodeOptions{Encoding: TextEncodingUTF16},
	)
	if err != nil {
		t.Fatal(err)
	}
	leDecode, err := decodeText(
		bytes.NewReader([]byte{0xFF, 0xFE, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00, 0x00}),
		textDecodeOptions{Encoding: TextEncodingUTF16},
	)
	if err != nil {
		t.Fatal(err)
	}
	if beDecode.Content != leDecode.Content || beDecode.BytesRead != leDecode.BytesRead {
		t.Fatalf("be=%q(%d) le=%q(%d)", beDecode.Content, beDecode.BytesRead, leDecode.Content, leDecode.BytesRead)
	}
	if beDecode.Content != textTestString {
		t.Fatalf("got %q", beDecode.Content)
	}
}

func TestTextEncode(t *testing.T) {
	// No BOM
	utf16EncodeBE := utf16Encode(textTestString, bigEndian, true, false)
	wantBE := []byte{0xFE, 0xFF, 0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5}
	if !bytes.Equal(utf16EncodeBE, wantBE) {
		t.Fatalf("got %x", utf16EncodeBE)
	}

	// BOM test
	beNoBOM := utf16Encode(textTestString, bigEndian, false, false)
	if !bytes.Equal(beNoBOM, wantBE[2:]) {
		t.Fatalf("got %x", beNoBOM)
	}
	leEncode := utf16Encode(textTestString, littleEndian, true, false)
	wantLE := []byte{0xFF, 0xFE, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00}
	if !bytes.Equal(leEncode, wantLE) {
		t.Fatalf("got %x", leEncode)
	}

	utf8Encode, err := TextEncodingUTF8.Encode(textTestString, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(utf8Encode, []byte(textTestString)) {
		t.Fatalf("got %x", utf8Encode)
	}
}

// Ported from id3/v2/header.rs tests

func TestId3v23ExtendedHeader(t *testing.T) {
	tagBytes := []byte("ID3\x03\x00\x40\x00\x00\x00\x0E\x00\x00\x00\x0A\x80\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	header, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts)
	if err != nil {
		t.Fatal(err)
	}
	if header.Version != 3 || header.Size != 14 || header.ExtendedSize != 10 {
		t.Fatalf("header = %+v", header)
	}
	if !header.Flags.Crc {
		t.Fatal("expected crc flag")
	}
}

func TestId3v24ExtendedHeader(t *testing.T) {
	tagBytes := []byte("ID3\x04\x00\x40\x00\x00\x00\x0F\x00\x00\x00\x0F\x01\x70\x00\x05\x00\x00\x00\x00\x00\x01\xF6")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	header, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts)
	if err != nil {
		t.Fatal(err)
	}
	if header.Version != 4 || header.Size != 15 || header.ExtendedSize != 15 {
		t.Fatalf("header = %+v", header)
	}
	if !header.Flags.Crc {
		t.Fatal("expected crc flag")
	}
	r := header.Flags.Restrictions
	if r == nil {
		t.Fatal("expected restrictions")
	}
	if r.Size != TagSizeS32F4K || !r.TextEncoding || !r.ImageEncoding {
		t.Fatalf("restrictions = %+v", r)
	}
	if v, ok := r.TextFieldsSize.Get(); !ok || v != TextSizeC128 {
		t.Fatalf("text_fields_size = %v", r.TextFieldsSize)
	}
	if v, ok := r.ImageSize.Get(); !ok || v != ImageSizeP64 {
		t.Fatalf("image_size = %v", r.ImageSize)
	}
}

func TestId3v23EmptyExtendedHeader(t *testing.T) {
	tagBytes := []byte("ID3\x03\x00\x40\x00\x00\x00\x0A\x00\x00\x00\x06\x00\x00\x00\x00\x00\x00")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	header, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts)
	if err != nil {
		t.Fatal(err)
	}
	if header.Version != 3 || header.Size != 10 || header.ExtendedSize != 6 {
		t.Fatalf("header = %+v", header)
	}
	if header.Flags.Crc || header.Flags.Restrictions != nil {
		t.Fatalf("flags = %+v", header.Flags)
	}
}

func TestId3v24EmptyExtendedHeader(t *testing.T) {
	tagBytes := []byte("ID3\x04\x00\x40\x00\x00\x00\x06\x00\x00\x00\x06\x01\x00")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	header, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts)
	if err != nil {
		t.Fatal(err)
	}
	if header.Version != 4 || header.Size != 6 || header.ExtendedSize != 6 {
		t.Fatalf("header = %+v", header)
	}
}

func TestId3v23ExtendedHeaderBadFlags(t *testing.T) {
	tagBytes := []byte("ID3\x03\x00\x40\x00\x00\x00\x0A\x00\x00\x00\x06\x00\x01\x00\x00\x00\x00")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	if _, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts); err == nil {
		t.Fatal("expected strict-mode error")
	}
	opts.ParsingMode = ParsingModeBestAttempt
	if _, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts); err != nil {
		t.Fatalf("best attempt should succeed: %v", err)
	}
}

func TestId3v24ExtendedHeaderBadFlags(t *testing.T) {
	tagBytes := []byte("ID3\x04\x00\x40\x00\x00\x00\x06\x00\x00\x00\x06\x01\x01")
	opts := NewParseOptions()
	opts.ParsingMode = ParsingModeStrict
	if _, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts); err == nil {
		t.Fatal("expected strict-mode error")
	}
	opts.ParsingMode = ParsingModeBestAttempt
	if _, err := ParseId3v2Header(bytes.NewReader(tagBytes), opts); err != nil {
		t.Fatalf("best attempt should succeed: %v", err)
	}
}
