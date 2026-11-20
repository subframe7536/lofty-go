package meta_test

import (
	"io"
	"os"
	"testing"

	. "github.com/subframe7536/lofty-go/internal/meta"
)

// --- src/resolve.rs ---

type myFileResolver struct{}

func (myFileResolver) Extension() string       { return "myfile" }
func (myFileResolver) PrimaryTagType() TagType { return TagTypeID3v2 }
func (myFileResolver) TagSupport(t TagType) TagSupport {
	if t == TagTypeID3v2 {
		return TagSupportReadWrite
	}
	return TagSupportUnsupported
}
func (myFileResolver) Guess(buf []byte) bool { return len(buf) >= 6 && string(buf[:6]) == "myfile" }
func (myFileResolver) ReadFrom(reader io.ReadSeeker, options ParseOptions) (*TaggedFile, error) {
	tag := NewTag(TagTypeID3v2)
	tag.SetArtist("All is well!")
	return NewTaggedFile(FileTypeCustom("MyFile"), []*Tag{tag}, FileProperties{}), nil
}

func TestCustomResolver(t *testing.T) {
	RegisterCustomResolver("MyFile", myFileResolver{})
	defer UnregisterCustomResolver("MyFile")

	path := "../../lofty-rs/examples/custom_resolver/test_asset.myfile"
	read, err := ReadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if ft := read.FileType(); !ft.IsCustom() || ft.CustomName() != "MyFile" {
		t.Fatalf("file_type = %v", ft)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	read2, err := ReadFrom(f)
	if err != nil {
		t.Fatal(err)
	}
	if ft := read2.FileType(); !ft.IsCustom() || ft.CustomName() != "MyFile" {
		t.Fatalf("file_type = %v", ft)
	}

	// Double-registering the same name must panic (Rust's contract)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on double register")
		}
	}()
	RegisterCustomResolver("MyFile", myFileResolver{})
}
