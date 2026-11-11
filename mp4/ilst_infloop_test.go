package mp4

import (
	"os"
	"testing"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
)

// infloop.m4a has a `gnre` atom whose `data` child declares size 0: Rust's
// AtomInfo::read extends it to the end of the ilst reader, permanently
// consuming every subsequent atom, so the scan ends with just
// trkn/disk/tmpo and the upgraded ©gen. Go must not rewind the cursor to
// the parent atom's end.
func TestInfloopSize0DataChildEndsIlstScan(t *testing.T) {
	f, err := os.Open(testutil.Path("lofty-rs", "lofty", "tests", "taglib", "data", "infloop.m4a"))
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	defer f.Close()
	mf, err := Mp4FileReadFrom(f, meta.ParseOptions{
		ReadTags: true, ReadProperties: true,
		ReadCoverArt: true, ImplicitConversions: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	il, ok := mf.Ilst.Get()
	if !ok {
		t.Fatal("no ilst")
	}
	var names []string
	for _, a := range il.atoms {
		names = append(names, mp4AtomName(a.Ident.Fourcc))
	}
	want := []string{"trkn", "disk", "tmpo", "©gen"}
	if len(names) != len(want) {
		t.Fatalf("atoms = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Fatalf("atoms = %v, want %v", names, want)
		}
	}
}
