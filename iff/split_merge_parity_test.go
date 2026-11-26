package iff

import (
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func TestRiffInfoTagSplitMergeRoundTripRetainsUnconvertibleItems(t *testing.T) {
	source := &RiffInfoTag{}
	source.InsertKV("INAM", "title")
	source.InsertKV("XABC", "format specific")

	remainder, generic := source.SplitTag()
	if value, ok := remainder.Get("XABC"); !ok || value != "format specific" {
		t.Fatalf("remainder XABC = %q, %v; want format specific", value, ok)
	}
	if remainder.Contains("INAM") {
		t.Fatal("converted INAM item remained in the split remainder")
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}

	merged := remainder.MergeTag(generic)
	if value, ok := merged.Get("XABC"); !ok || value != "format specific" {
		t.Fatalf("merged XABC = %q, %v; want format specific", value, ok)
	}
	if value, ok := merged.Get("INAM"); !ok || value != "title" {
		t.Fatalf("merged INAM = %q, %v; want title", value, ok)
	}
}

func TestAiffTextTagSplitMergeRoundTripRetainsComments(t *testing.T) {
	source := &AiffTextTag{
		Name:        meta.Some("title"),
		Annotations: []string{"annotation"},
		Comments:    []AiffComment{{Timestamp: 42, MarkerID: 7, Text: "chunk comment"}},
	}

	remainder, generic := source.SplitTag()
	if remainder.Name.IsSome() || len(remainder.Annotations) != 0 {
		t.Fatalf("converted fields remained in remainder: %+v", remainder)
	}
	if len(remainder.Comments) != 1 || remainder.Comments[0].Text != "chunk comment" {
		t.Fatalf("remainder comments = %+v, want the COMT comment", remainder.Comments)
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}

	merged := remainder.MergeTag(generic)
	if title, ok := merged.Name.Get(); !ok || title != "title" {
		t.Fatalf("merged title = %q, %v; want title", title, ok)
	}
	if len(merged.Annotations) != 1 || merged.Annotations[0] != "annotation" {
		t.Fatalf("merged annotations = %v, want [annotation]", merged.Annotations)
	}
	if len(merged.Comments) != 1 || merged.Comments[0].Text != "chunk comment" {
		t.Fatalf("merged comments = %+v, want the COMT comment", merged.Comments)
	}
}
