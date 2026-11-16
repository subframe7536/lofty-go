package vorbis

import (
	"encoding/binary"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
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
