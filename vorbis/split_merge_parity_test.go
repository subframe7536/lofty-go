package vorbis

import (
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func TestVorbisCommentsSplitMergeRoundTripRetainsUnconvertibleItems(t *testing.T) {
	source := NewVorbisCommentsTag()
	source.Vendor = "test vendor"
	source.PushKV("TITLE", "title")
	source.PushKV("X-UNKNOWN", "format specific")

	remainder, generic := source.SplitTag()
	if value, ok := remainder.Get("X-UNKNOWN"); !ok || value != "format specific" {
		t.Fatalf("remainder X-UNKNOWN = %q, %v; want format specific", value, ok)
	}
	if remainder.Contains("TITLE") {
		t.Fatal("converted TITLE item remained in the split remainder")
	}
	if remainder.Vendor != "test vendor" {
		t.Fatalf("remainder vendor = %q, want test vendor", remainder.Vendor)
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}

	merged := remainder.MergeTag(generic)
	if value, ok := merged.Get("X-UNKNOWN"); !ok || value != "format specific" {
		t.Fatalf("merged X-UNKNOWN = %q, %v; want format specific", value, ok)
	}
	if value, ok := merged.Get("TITLE"); !ok || value != "title" {
		t.Fatalf("merged TITLE = %q, %v; want title", value, ok)
	}
	if merged.Vendor != "test vendor" {
		t.Fatalf("merged vendor = %q, want test vendor", merged.Vendor)
	}
}
