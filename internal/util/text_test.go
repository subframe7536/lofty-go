package util

import (
	"bytes"
	"testing"
)

// Ported from util/text.rs tests

const textTestString = "l\u00f8ft\u00a5"

func TestTextDecode(t *testing.T) {
	// No BOM
	utf16Decode, err := Utf16DecodeBytes(
		[]byte{0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00},
		BigEndian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if utf16Decode != textTestString {
		t.Fatalf("got %q", utf16Decode)
	}

	// BOM test
	beDecode, err := DecodeText(
		bytes.NewReader([]byte{0xFE, 0xFF, 0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00}),
		TextDecodeOptions{Encoding: TextEncodingUTF16},
	)
	if err != nil {
		t.Fatal(err)
	}
	leDecode, err := DecodeText(
		bytes.NewReader([]byte{0xFF, 0xFE, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5, 0x00, 0x00, 0x00}),
		TextDecodeOptions{Encoding: TextEncodingUTF16},
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
	utf16EncodeBE := Utf16Encode(textTestString, BigEndian, true, false)
	wantBE := []byte{0xFE, 0xFF, 0x00, 0x6C, 0x00, 0xF8, 0x00, 0x66, 0x00, 0x74, 0x00, 0xA5}
	if !bytes.Equal(utf16EncodeBE, wantBE) {
		t.Fatalf("got %x", utf16EncodeBE)
	}

	// BOM test
	beNoBOM := Utf16Encode(textTestString, BigEndian, false, false)
	if !bytes.Equal(beNoBOM, wantBE[2:]) {
		t.Fatalf("got %x", beNoBOM)
	}
	leEncode := Utf16Encode(textTestString, LittleEndian, true, false)
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
