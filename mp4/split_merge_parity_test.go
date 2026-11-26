package mp4

import (
	"bytes"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

func pushMp4Text(tag *meta.Tag, key meta.ItemKey, values ...string) {
	for _, value := range values {
		tag.PushUnchecked(meta.NewTagItem(key, meta.ItemValueText(value)))
	}
}

func countIlstAtoms(ilst *Ilst, ident AtomIdent) int {
	count := 0
	for _, atom := range ilst.Atoms() {
		if atom.Ident == ident {
			count++
		}
	}
	return count
}

func TestIlstFromTagWriteSemanticsParity(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeMp4Ilst)
	pushMp4Text(tag, meta.ItemKeyTrackArtist, "A", "B")
	pushMp4Text(tag, meta.ItemKeyFlagCompilation, "0", "1")
	pushMp4Text(tag, meta.ItemKeyTrackNumber, "7", "N/A")

	ilst := IlstFromTag(tag)

	artist := ilst.Get(FourccIdent("©ART"))
	if artist == nil || len(artist.Data) != 2 {
		t.Fatalf("©ART atom = %+v, want one atom with two data payloads", artist)
	}
	if artist.Data[0].Text != "A" || artist.Data[1].Text != "B" {
		t.Fatalf("©ART values = %q, %q, want A, B", artist.Data[0].Text, artist.Data[1].Text)
	}
	if got := countIlstAtoms(ilst, FourccIdent("©ART")); got != 1 {
		t.Fatalf("©ART atom count = %d, want 1", got)
	}

	compilation := ilst.Get(FourccIdent("cpil"))
	if compilation == nil || len(compilation.Data) != 1 || compilation.Data[0].Kind != AtomDataBool || !compilation.Data[0].Bool {
		t.Fatalf("cpil atom = %+v, want a single true bool payload", compilation)
	}

	track := ilst.Get(FourccIdent("trkn"))
	if track == nil || len(track.Data) != 1 || len(track.Data[0].Data) < 6 {
		t.Fatalf("trkn atom = %+v", track)
	}
	if !bytes.Equal(track.Data[0].Data[:6], []byte{0, 0, 0, 7, 0, 0}) {
		t.Fatalf("trkn data = %v, want track 7 retained", track.Data[0].Data)
	}
}

func TestIlstFromTagDoesNotMergeConvertedAtomsIntoCompanion(t *testing.T) {
	tag := meta.NewTag(meta.TagTypeMp4Ilst)
	tag.SetCompanionTag(&Ilst{atoms: []Atom{{
		Ident: FourccIdent("©ART"),
		Data:  []AtomData{{Kind: AtomDataUTF8, Text: "companion"}},
	}}})
	pushMp4Text(tag, meta.ItemKeyTrackArtist, "converted")

	ilst := IlstFromTag(tag)
	if got := countIlstAtoms(ilst, FourccIdent("©ART")); got != 2 {
		t.Fatalf("©ART atom count = %d, want separate companion and converted atoms", got)
	}
	atoms := ilst.Atoms()
	if len(atoms) != 2 || atoms[0].Data[0].Text != "companion" || atoms[1].Data[0].Text != "converted" {
		t.Fatalf("atoms = %+v, want companion atom followed by converted atom", atoms)
	}
}

func TestIlstSplitMergeRoundTripRetainsUnconvertibleAtoms(t *testing.T) {
	ilst := NewIlst()
	ilst.Insert(Atom{
		Ident: FourccIdent("SMTH"),
		Data:  []AtomData{{Kind: AtomDataUnknown, Code: 0, Data: []byte("format specific")}},
	})
	ilst.Insert(Atom{
		Ident: FourccIdent("©nam"),
		Data:  []AtomData{{Kind: AtomDataUTF8, Text: "title"}},
	})

	remainder, generic := ilst.SplitTag()
	if got := countIlstAtoms(remainder, FourccIdent("SMTH")); got != 1 {
		t.Fatalf("remainder SMTH count = %d, want 1", got)
	}
	if remainder.Contains(FourccIdent("©nam")) {
		t.Fatal("converted ©nam atom remained in the split remainder")
	}
	if title, ok := generic.GetString(meta.ItemKeyTrackTitle); !ok || title != "title" {
		t.Fatalf("generic title = %q, %v; want title", title, ok)
	}

	merged := remainder.MergeTag(generic)
	if got := countIlstAtoms(merged, FourccIdent("SMTH")); got != 1 {
		t.Fatalf("merged SMTH count = %d, want 1", got)
	}
	if title := merged.Get(FourccIdent("©nam")); title == nil || title.Data[0].Text != "title" {
		t.Fatalf("merged ©nam = %+v, want title", title)
	}
}
