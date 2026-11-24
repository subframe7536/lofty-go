package vorbis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// vcTestPayload builds the vendor+items region of a comment packet.
func vcTestPayload(vendor string, kvs ...string) []byte {
	var out []byte
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(vendor)))
	out = append(out, u32[:]...)
	out = append(out, vendor...)
	binary.LittleEndian.PutUint32(u32[:], uint32(len(kvs)))
	out = append(out, u32[:]...)
	for _, kv := range kvs {
		binary.LittleEndian.PutUint32(u32[:], uint32(len(kv)))
		out = append(out, u32[:]...)
		out = append(out, kv...)
	}
	return out
}

// TestParseTrimsTrailingNulls: Rust's `utf8_decode_str` strips trailing NULs
// from comment values — before the current/total split for number fields.
func TestParseTrimsTrailingNulls(t *testing.T) {
	tag, err := ParseVorbisComments(
		vcTestPayload("v", "TITLE=Foo\x00\x00", "TRACKNUMBER=3\x00"),
		meta.NewParseOptions(),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, _ := tag.Get("TITLE"); v != "Foo" {
		t.Fatalf("TITLE = %q, want Foo", v)
	}
	if n, ok := tag.Track(); !ok || n != 3 {
		t.Fatalf("track = %v,%v, want 3,true", n, ok)
	}
}

// TestInsertPictureInfoError: `insert_picture` propagates the
// PictureInformation extraction error and leaves the tag unchanged.
func TestInsertPictureInfoError(t *testing.T) {
	tag := NewVorbisCommentsTag()
	// <8 bytes: PictureInformation::from_picture fails
	pic := meta.PictureUnchecked([]byte{1, 2, 3}).Build()
	if err := tag.InsertPicture(pic, meta.None[meta.PictureInformation]()); err == nil {
		t.Fatal("expected error for undecodable picture")
	}
	if len(tag.Pictures()) != 0 {
		t.Fatalf("tag mutated on error: %d pictures", len(tag.Pictures()))
	}
}

// TestUnknownItemDropped: generic→concrete conversion drops items whose key
// has no Vorbis field mapping (Rust `map_key -> None => continue`), rather
// than storing them under their description.
func TestUnknownItemDropped(t *testing.T) {
	input := meta.NewTag(meta.TagTypeVorbisComments)
	input.PushUnchecked(meta.NewTagItem(meta.ItemKeyUnknown, meta.ItemValueText("custom")))
	input.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTitle, meta.ItemValueText("Song")))

	if vc := VorbisCommentsTagFromTag(input, ""); len(vc.Pairs()) != 1 {
		t.Fatalf("FromTag pairs = %v, want 1 item", vc.Pairs())
	}
	if vc := VorbisCommentsTagForWriteRef(input, false); len(vc.Pairs()) != 1 {
		t.Fatalf("ForWriteRef pairs = %v, want 1 item", vc.Pairs())
	}
}

// TestParseItemLengthBeyondRemaining: an item longer than the remaining data
// is a SizeMismatchError, like Rust's `read_comments`.
func TestParseItemLengthBeyondRemaining(t *testing.T) {
	payload := vcTestPayload("v", "TITLE=Foo")
	// Inflate the item length past the end of the payload
	itemLenPos := 4 + 1 + 4
	binary.LittleEndian.PutUint32(payload[itemLenPos:], 1000)

	_, err := ParseVorbisComments(payload, meta.NewParseOptions())
	var sm *util.SizeMismatchError
	if !errors.As(err, &sm) {
		t.Fatalf("err = %v, want SizeMismatchError", err)
	}
}

// TestDumpTagKeepsEncoderAndPads: the registered Vorbis comments dumper
// follows Rust's `VorbisComments::as_tag_ref` + `VorbisCommentsRef::dump_to`:
// ENCODER stays a comment (vendor is empty), and preferred_padding applies.
func TestDumpTagKeepsEncoderAndPads(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeVorbisComments)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyEncoderSoftware, meta.ItemValueText("enc")))
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTitle, meta.ItemValueText("T")))

	var buf bytes.Buffer
	if err := tag.DumpTo(&buf, meta.WriteOptions{}); err != nil {
		t.Fatalf("dump: %v", err)
	}
	vc, err := ParseVorbisComments(buf.Bytes(), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("parse dump: %v", err)
	}
	if vc.Vendor != "" {
		t.Fatalf("vendor = %q, want empty", vc.Vendor)
	}
	if v, ok := vc.Get("ENCODER"); !ok || v != "enc" {
		t.Fatalf("ENCODER comment = %q,%v", v, ok)
	}
	if v, _ := vc.Get("TITLE"); v != "T" {
		t.Fatalf("TITLE = %q", v)
	}
	if buf.Len()%8160 == 0 {
		t.Fatal("unpadded dump is unexpectedly page-aligned")
	}

	buf.Reset()
	if err := tag.DumpTo(&buf, meta.WriteOptions{PreferredPadding: 100}); err != nil {
		t.Fatalf("padded dump: %v", err)
	}
	if buf.Len() != 8160 {
		t.Fatalf("padded dump length = %d, want 8160", buf.Len())
	}
}
