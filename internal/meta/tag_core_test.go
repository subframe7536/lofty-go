package meta

import (
	"bytes"
	"testing"
)

func textOf(items []TagItem) []string {
	var out []string
	for _, it := range items {
		s, _ := it.Value().Text()
		out = append(out, s)
	}
	return out
}

func vorbisTag3() *Tag {
	tg := NewTag(TagTypeVorbisComments)
	tg.PushUnchecked(NewTagItem(ItemKeyTrackTitle, ItemValueText("a")))
	tg.PushUnchecked(NewTagItem(ItemKeyAlbumTitle, ItemValueText("b")))
	tg.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText("c")))
	return tg
}

// A slice returned by Items() must not be rewritten by later mutations.
func TestItemsSnapshotSurvivesMutation(t *testing.T) {
	check := func(name string, mutate func(*Tag)) {
		tg := vorbisTag3()
		snap := tg.Items()
		mutate(tg)
		got := textOf(snap)
		if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
			t.Errorf("%s: snapshot = %v, want [a b c]", name, got)
		}
	}
	check("RemoveKey", func(tg *Tag) { tg.RemoveKey(ItemKeyTrackTitle) })
	check("Retain", func(tg *Tag) { tg.Retain(func(i *TagItem) bool { return i.Key() != ItemKeyAlbumTitle }) })
	check("RemoveEmpty", func(tg *Tag) {
		tg.PushUnchecked(NewTagItem(ItemKeyMood, ItemValueText("")))
		tg.RemoveEmpty()
	})
	check("Insert", func(tg *Tag) { tg.InsertText(ItemKeyTrackTitle, "new") })

	// And the results are still right.
	tg := vorbisTag3()
	tg.RemoveKey(ItemKeyTrackTitle)
	if got := textOf(tg.Items()); len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("after RemoveKey: %v", got)
	}
	tg.InsertText(ItemKeyAlbumTitle, "B")
	if got := textOf(tg.Items()); len(got) != 2 || got[0] != "c" || got[1] != "B" {
		t.Fatalf("after Insert: %v", got)
	}
	tg.Retain(func(*TagItem) bool { return true })
	if tg.ItemCount() != 2 {
		t.Fatalf("Retain(all) changed count: %d", tg.ItemCount())
	}
}

func TestPicturesSnapshotSurvivesRemoval(t *testing.T) {
	tg := NewTag(TagTypeID3v2)
	tg.PushPicture(PictureUnchecked([]byte{1}).PicType(PictureTypeCoverFront).Build())
	tg.PushPicture(PictureUnchecked([]byte{2}).PicType(PictureTypeCoverBack).Build())
	tg.PushPicture(PictureUnchecked([]byte{3}).PicType(PictureTypeMedia).Build())
	snap := tg.Pictures()
	tg.RemovePictureType(PictureTypeCoverFront)
	if snap[0].Data()[0] != 1 || snap[1].Data()[0] != 2 || snap[2].Data()[0] != 3 {
		t.Fatalf("snapshot rewritten by RemovePictureType")
	}
	snap = tg.Pictures()
	removed := tg.RemovePicture(0)
	if removed.Data()[0] != 2 || snap[0].Data()[0] != 2 || snap[1].Data()[0] != 3 || tg.PictureCount() != 1 {
		t.Fatalf("RemovePicture mangled state")
	}
}

type cloneableCompanion struct{ vals []int }

func (c *cloneableCompanion) CloneCompanion() any {
	return &cloneableCompanion{vals: append([]int(nil), c.vals...)}
}

type plainCompanion struct{ x int }

func TestCloneIsDeep(t *testing.T) {
	tg := NewTag(TagTypeID3v2)
	tg.PushUnchecked(NewTagItem(ItemKeyMusicBrainzRecordingId, ItemValueBinary([]byte{1, 2, 3})))
	tg.PushPicture(PictureUnchecked([]byte{9, 9, 9}).PicType(PictureTypeCoverFront).Description("d").Build())
	comp := &cloneableCompanion{vals: []int{1, 2}}
	tg.SetCompanionTag(comp)
	tg.OriginalVersion = 3

	cl := tg.Clone()
	// mutate the clone's buffers and companion
	b, _ := cl.Items()[0].Value().Binary()
	b[0] = 0xEE
	cl.Pictures()[0].Data()[0] = 0xEE
	cl.CompanionTag().(*cloneableCompanion).vals[0] = 99

	ob, _ := tg.Items()[0].Value().Binary()
	if !bytes.Equal(ob, []byte{1, 2, 3}) {
		t.Errorf("binary item shared with clone: %v", ob)
	}
	if !bytes.Equal(tg.Pictures()[0].Data(), []byte{9, 9, 9}) {
		t.Errorf("picture data shared with clone: %v", tg.Pictures()[0].Data())
	}
	if tg.CompanionTag().(*cloneableCompanion) != comp || comp.vals[0] != 1 {
		t.Errorf("companion shared with clone: %v", comp.vals)
	}
	if cl.CompanionTag() == any(comp) {
		t.Errorf("clone kept the same companion pointer")
	}
	if cl.OriginalVersion != 3 || cl.TagType() != TagTypeID3v2 {
		t.Errorf("metadata lost: %v %v", cl.OriginalVersion, cl.TagType())
	}
	// items added to the clone don't show up in the original
	cl.InsertText(ItemKeyTrackTitle, "x")
	if tg.ItemCount() != 1 {
		t.Errorf("original grew to %d", tg.ItemCount())
	}

	// Companions without a cloner are shared (documented fallback).
	p := &plainCompanion{x: 1}
	tg.SetCompanionTag(p)
	if tg.Clone().CompanionTag() != any(p) {
		t.Errorf("plain companion unexpectedly replaced")
	}
	// nil slices stay nil
	if e := NewTag(TagTypeApe).Clone(); e.Items() != nil || e.Pictures() != nil {
		t.Errorf("empty clone not nil-sliced")
	}
}

// Rust parses numbers with `str::parse::<u32>()`, which accepts one leading '+'.
func TestNumericAccessorsParseLikeRust(t *testing.T) {
	cases := []struct {
		in   string
		want uint32
		ok   bool
	}{
		{"3", 3, true},
		{"+3", 3, true},
		{"007", 7, true},
		{"4294967295", 4294967295, true},
		{"++3", 0, false},
		{"-0", 0, false},
		{"-3", 0, false},
		{"", 0, false},
		{"+", 0, false},
		{" 3", 0, false},
		{"3 ", 0, false},
		{"3/10", 0, false},
		{"4294967296", 0, false},
	}
	for _, c := range cases {
		tg := NewTag(TagTypeID3v2)
		tg.InsertText(ItemKeyTrackNumber, c.in)
		tg.InsertText(ItemKeyTrackTotal, c.in)
		tg.InsertText(ItemKeyDiscNumber, c.in)
		tg.InsertText(ItemKeyDiscTotal, c.in)
		for name, get := range map[string]func() (uint32, bool){
			"Track": tg.Track, "TrackTotal": tg.TrackTotal, "Disk": tg.Disk, "DiskTotal": tg.DiskTotal,
		} {
			if got, ok := get(); ok != c.ok || got != c.want {
				t.Errorf("%s(%q) = %d,%v want %d,%v", name, c.in, got, ok, c.want, c.ok)
			}
		}
	}
}

func TestAccessorDate(t *testing.T) {
	tg := NewTag(TagTypeVorbisComments)
	tg.InsertText(ItemKeyYear, "1999")
	if d := tg.Date(); d.IsNone() {
		t.Fatal("Year not used as fallback")
	}
	// An unparsable RecordingDate shadows Year (Rust `or_else` only on None).
	tg.InsertText(ItemKeyRecordingDate, "garbage")
	if d := tg.Date(); d.IsSome() {
		t.Fatal("garbage RecordingDate fell back to Year")
	}
}

func TestInsertPushSemantics(t *testing.T) {
	tg := NewTag(TagTypeID3v1)
	if tg.InsertText(ItemKeyAlbumArtist, "x") {
		t.Fatal("ID3v1 accepted AlbumArtist")
	}
	tg.InsertText(ItemKeyTrackTitle, "a")
	tg.InsertText(ItemKeyAlbumTitle, "b")
	tg.InsertText(ItemKeyTrackTitle, "c") // replaces and moves to the end
	if got := textOf(tg.Items()); len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("insert order: %v", got)
	}
	tg.Push(NewTagItem(ItemKeyTrackTitle, ItemValueText("d")))
	if n := len(tg.GetStrings(ItemKeyTrackTitle)); n != 2 {
		t.Fatalf("push did not append: %d", n)
	}
	tg.ReMap(TagTypeAiffText)
	if tg.TagType() != TagTypeAiffText || tg.ItemCount() != 2 {
		t.Fatalf("remap: %v %d", tg.TagType(), tg.ItemCount())
	}
}
