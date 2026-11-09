package lofty

import (
	"bytes"
	"testing"
)

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
