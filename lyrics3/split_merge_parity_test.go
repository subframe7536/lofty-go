package lyrics3

import (
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func TestLyrics3v2SplitMergeRoundTripRetainsFlagsAndImages(t *testing.T) {
	source := &Lyrics3v2Tag{
		Flags:  Lyrics3v2TagFlags{SynchronizedLyrics: true, RandomSelection: true},
		Lyrics: meta.Some("lyrics"),
		Images: []Lyrics3v2Image{{Filename: "cover.png", Description: meta.Some("cover")}},
	}

	remainder, generic := source.SplitTag()
	if remainder.Lyrics.IsSome() {
		t.Fatal("converted lyrics field remained in the split remainder")
	}
	if !remainder.Flags.SynchronizedLyrics || !remainder.Flags.RandomSelection {
		t.Fatalf("remainder flags = %+v, want both flags", remainder.Flags)
	}
	if len(remainder.Images) != 1 || remainder.Images[0].Filename != "cover.png" {
		t.Fatalf("remainder images = %+v, want cover.png", remainder.Images)
	}
	if lyrics, ok := generic.GetString(meta.ItemKeyLyrics); !ok || lyrics != "lyrics" {
		t.Fatalf("generic lyrics = %q, %v; want lyrics", lyrics, ok)
	}

	merged := remainder.MergeTag(generic)
	if lyrics, ok := merged.Lyrics.Get(); !ok || lyrics != "lyrics" {
		t.Fatalf("merged lyrics = %q, %v; want lyrics", lyrics, ok)
	}
	if !merged.Flags.SynchronizedLyrics || !merged.Flags.RandomSelection {
		t.Fatalf("merged flags = %+v, want both flags", merged.Flags)
	}
	if len(merged.Images) != 1 || merged.Images[0].Filename != "cover.png" {
		t.Fatalf("merged images = %+v, want cover.png", merged.Images)
	}
}

func TestLyrics3SplitMergeRoundTrip(t *testing.T) {
	source := &Lyrics3Tag{Lyrics: "lyrics"}

	remainder, generic := source.SplitTag()
	if !remainder.IsEmpty() || !source.IsEmpty() {
		t.Fatalf("Lyrics3 remainder = %+v, source = %+v; want both empty", remainder, source)
	}
	if lyrics, ok := generic.GetString(meta.ItemKeyLyrics); !ok || lyrics != "lyrics" {
		t.Fatalf("generic lyrics = %q, %v; want lyrics", lyrics, ok)
	}

	merged := remainder.MergeTag(generic)
	if merged.Lyrics != "lyrics" {
		t.Fatalf("merged lyrics = %q, want lyrics", merged.Lyrics)
	}
}
