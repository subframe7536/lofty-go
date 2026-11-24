package lofty_test

import (
	"io"
	"strings"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

// Ported from tests/files/mpeg.rs::issue_762_ape_header_mistaken_for_id3v1
// (upstream lofty-rs commit 700bf53d).
func TestIssue762ApeHeaderNotMistakenForId3v1(t *testing.T) {
	// Ten silent MPEG-1 Layer III frames.
	frame := make([]byte, 417)
	copy(frame[:4], []byte{0xFF, 0xFB, 0x90, 0x64})
	var raw []byte
	for i := 0; i < 10; i++ {
		raw = append(raw, frame...)
	}
	bf := NewBytesFile(raw)

	// An APE tag of 131 bytes puts the "TAG" in its header's "APETAGEX" 128 bytes from EOF.
	title := strings.Repeat("x", 53)
	apeTag := NewTag(TagTypeApe)
	apeTag.SetTitle(title)
	if err := apeTag.SaveTo(bf, NewWriteOptions()); err != nil {
		t.Fatalf("SaveTo APE: %v", err)
	}
	b := bf.Bytes()
	if len(b) < 131 || string(b[len(b)-131:len(b)-123]) != "APETAGEX" {
		t.Fatalf("expected APETAGEX at len-131, got %q", b[len(b)-131:len(b)-123])
	}

	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf, err := MpegFileReadFrom(bf, NewParseOptions())
	if err != nil {
		t.Fatalf("MpegFileReadFrom: %v", err)
	}
	if mf.ID3v1.IsSome() {
		t.Fatal("expected ID3v1 to be None, but APE header was mistaken for ID3v1")
	}
	gotApe := mf.TaggedFile().Tag(TagTypeApe)
	if gotApe == nil {
		t.Fatal("expected APE tag to be present")
	}
	if got, _ := gotApe.Title(); got != title {
		t.Fatalf("APE title = %q, want %q", got, title)
	}

	// Removing an ID3v1 tag must not truncate the APE tag.
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := TagTypeID3v1.RemoveFrom(bf, NewWriteOptions()); err != nil {
		t.Fatalf("TagTypeID3v1.RemoveFrom: %v", err)
	}
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	mf2, err := MpegFileReadFrom(bf, NewParseOptions())
	if err != nil {
		t.Fatalf("MpegFileReadFrom after ID3v1 remove: %v", err)
	}
	gotApe2 := mf2.TaggedFile().Tag(TagTypeApe)
	if gotApe2 == nil {
		t.Fatal("APE tag was truncated by ID3v1 removal")
	}
	if got, _ := gotApe2.Title(); got != title {
		t.Fatalf("APE title after ID3v1 remove = %q, want %q", got, title)
	}
}
