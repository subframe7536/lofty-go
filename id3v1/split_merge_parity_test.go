package id3v1

import (
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func TestId3v1SplitMergeRoundTrip(t *testing.T) {
	source := &Id3v1Tag{
		Title:       meta.Some("title"),
		Artist:      meta.Some("artist"),
		TrackNumber: meta.Some(uint8(7)),
	}

	remainder, generic := source.SplitTag()
	if !remainder.IsEmpty() || !source.IsEmpty() {
		t.Fatalf("ID3v1 remainder = %+v, source = %+v; want both empty", remainder, source)
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}
	if track, ok := generic.GetString(meta.ItemKeyTrackNumber); !ok || track != "7" {
		t.Fatalf("generic track = %q, %v; want 7", track, ok)
	}

	merged := remainder.MergeTag(generic)
	if title, ok := merged.Title.Get(); !ok || title != "title" {
		t.Fatalf("merged title = %q, %v; want title", title, ok)
	}
	if artist, ok := merged.Artist.Get(); !ok || artist != "artist" {
		t.Fatalf("merged artist = %q, %v; want artist", artist, ok)
	}
	if track, ok := merged.TrackNumber.Get(); !ok || track != 7 {
		t.Fatalf("merged track = %d, %v; want 7", track, ok)
	}
}
