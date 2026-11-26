package ape

import (
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func TestTagCloneCompanionApeDeepCopy(t *testing.T) {
	source := NewApeTag()
	source.Push(mustApeItem(t, "X-PRIVATE", meta.ItemValueText("secret")))

	generic := source.Tag()
	companion, ok := generic.CompanionTag().(*ApeTag)
	if !ok || companion == nil {
		t.Fatalf("generic companion = %T, want *ApeTag", generic.CompanionTag())
	}
	if _, ok := companion.Item("X-PRIVATE"); !ok {
		t.Fatal("test setup: companion does not contain X-PRIVATE")
	}

	clone := generic.Clone()
	cloneCompanion, ok := clone.CompanionTag().(*ApeTag)
	if !ok || cloneCompanion == nil {
		t.Fatalf("clone companion = %T, want *ApeTag", clone.CompanionTag())
	}
	cloneCompanion.Clear()

	restored := ApeTagFromTag(generic)
	if _, ok := restored.Item("X-PRIVATE"); !ok {
		t.Fatal("clearing the clone's companion destroyed the original tag's X-PRIVATE item")
	}
}

func TestApeTagSplitMergeRoundTripRetainsUnconvertibleItems(t *testing.T) {
	source := NewApeTag()
	source.Push(mustApeItem(t, "Title", meta.ItemValueText("title")))
	source.Push(mustApeItem(t, "X-PRIVATE", meta.ItemValueText("format specific")))

	remainder, generic := source.SplitTag()
	if _, ok := remainder.Item("X-PRIVATE"); !ok {
		t.Fatal("unmapped X-PRIVATE item was not retained in the split remainder")
	}
	if remainder.Contains("Title") {
		t.Fatal("converted Title item remained in the split remainder")
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}

	merged := remainder.MergeTag(generic)
	if value, ok := merged.GetText("X-PRIVATE"); !ok || value != "format specific" {
		t.Fatalf("merged X-PRIVATE = %q, %v; want format specific", value, ok)
	}
	if value, ok := merged.GetText("Title"); !ok || value != "title" {
		t.Fatalf("merged Title = %q, %v; want title", value, ok)
	}
}
