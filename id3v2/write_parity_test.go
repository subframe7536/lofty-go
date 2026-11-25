package id3v2

import (
	"bytes"
	"encoding/binary"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Ported from lofty-rs/lofty/src/id3/v2/write/mod.rs::id3v2_write_crc32_v4.
func TestId3v2WriteCrc32V4(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetArtist("Foo artist")
	flags := Id3v2TagFlags{Crc: true}
	tag.SetFlags(flags)

	opts := meta.NewWriteOptions()
	opts.PreferredPadding = 0
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, opts); err != nil {
		t.Fatalf("DumpTo: %v", err)
	}
	writer := buf.Bytes()
	wantCRC := []byte{5, 0x06, 0x35, 0x69, 0x7D, 0x14}
	if len(writer) < 22 || !bytes.Equal(writer[16:22], wantCRC) {
		t.Fatalf("crc_content = %x, want %x", writer[16:min(len(writer), 22)], wantCRC)
	}

	crcContent := writer[17:22]
	var unsynchCRC uint32
	for i := 0; i < 5; i++ {
		b := crcContent[i] & 0x7F
		unsynchCRC |= uint32(b) << ((4 - i) * 7)
	}
	if unsynchCRC != 0x66BA7E94 {
		t.Fatalf("unsynch_crc = 0x%08x, want 0x66BA7E94", unsynchCRC)
	}
}

// Ported from lofty-rs/lofty/src/id3/v2/write/mod.rs::id3v2_write_crc32_v3.
func TestId3v2WriteCrc32V3(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetArtist("Foo artist")
	tag.SetFlags(Id3v2TagFlags{Crc: true})

	opts := meta.NewWriteOptions()
	opts.UseID3v23 = true
	opts.PreferredPadding = 0
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, opts); err != nil {
		t.Fatalf("DumpTo: %v", err)
	}
	writer := buf.Bytes()
	wantCRC := []byte{0xA0, 0x6A, 0x3E, 0x1F}
	if len(writer) < 24 || !bytes.Equal(writer[20:24], wantCRC) {
		t.Fatalf("crc_content = %x, want %x", writer[20:min(len(writer), 24)], wantCRC)
	}
}

// Ported from lofty-rs/lofty/src/id3/v2/write/mod.rs::id3v2_write_extended_padding_size_v3.
func TestId3v2WriteExtendedPaddingSizeV3(t *testing.T) {
	tag := NewId3v2Tag()
	tag.SetArtist("Foo artist")
	tag.SetFlags(Id3v2TagFlags{Crc: true})

	opts := meta.NewWriteOptions()
	opts.UseID3v23 = true
	opts.PreferredPadding = 500
	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, opts); err != nil {
		t.Fatalf("DumpTo: %v", err)
	}
	writer := buf.Bytes()
	if len(writer) < 20 {
		t.Fatalf("short tag: %d bytes", len(writer))
	}
	if got := binary.BigEndian.Uint32(writer[16:20]); got != 500 {
		t.Fatalf("padding size = %d, want 500", got)
	}
}

func TestId3v2TagRestrictionsCodecAndTagExt(t *testing.T) {
	for b := 0; b <= 0xFF; b++ {
		r := TagRestrictionsFromByte(byte(b))
		if got := r.AsByte(); got != byte(b) {
			t.Fatalf("roundtrip 0x%02x -> 0x%02x", b, got)
		}
	}

	tag := NewId3v2Tag()
	if tag.TagType() != meta.TagTypeID3v2 || tag.Len() != 0 || tag.Contains(FrameIdValid("TPE1")) {
		t.Fatalf("unexpected empty tag state")
	}
	tag.SetArtist("Artist")
	if tag.Len() != 1 || !tag.Contains(FrameIdValid("TPE1")) {
		t.Fatalf("expected TPE1 frame")
	}
	tag.Clear()
	if !tag.IsEmpty() || tag.Len() != 0 {
		t.Fatalf("expected empty tag after Clear")
	}
}
