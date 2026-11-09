// Package testutil provides helpers for locating the vendored lofty-rs
// fixtures from any package in the module.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/subframe7536/lofty-go/internal/meta"
)

// root is the module root directory (the parent of internal/).
var root = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}()

// Path returns an absolute path to a file relative to the module root,
// e.g. Path("lofty-rs/lofty/tests/taglib/data/x.mp3").
func Path(parts ...string) string {
	return filepath.Join(append([]string{root}, parts...)...)
}

// Fixture returns an absolute path to a fixture under
// lofty-rs/lofty/tests/files/assets/, mirroring the original `testAsset`.
func Fixture(parts ...string) string {
	return Path(append([]string{"lofty-rs", "lofty", "tests", "files", "assets"}, parts...)...)
}

// TempFile copies the file at assetPath into a temporary file under
// t.TempDir() and returns it, mirroring the original `tempFile`.
func TempFile(t *testing.T, assetPath string) *os.File {
	t.Helper()
	data, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset %s: %v", assetPath, err)
	}
	f, err := os.CreateTemp(t.TempDir(), filepath.Base(assetPath))
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	if _, err := f.Write(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("failed to rewind: %v", err)
	}
	return f
}

// ToolInstalled reports whether a binary is on PATH.
func ToolInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// VerifyTagCommon checks the standard fields created by CreateTagCommon.
func VerifyTagCommon(t *testing.T, tag *meta.Tag, trackNumber, genre bool) {
	t.Helper()
	check := func(key meta.ItemKey, want string) {
		t.Helper()
		if got, _ := tag.GetString(key); got != want {
			t.Fatalf("%v = %q, want %q", key, got, want)
		}
	}
	check(meta.ItemKeyTrackTitle, "Foo title")
	check(meta.ItemKeyTrackArtist, "Bar artist")
	check(meta.ItemKeyAlbumTitle, "Baz album")
	check(meta.ItemKeyComment, "Qux comment")
	if trackNumber {
		check(meta.ItemKeyTrackNumber, "1")
	}
	if genre {
		check(meta.ItemKeyGenre, "Classical")
	}
}

// CreateTagCommon creates a tag with standard test fields.
func CreateTagCommon(ty meta.TagType) *meta.Tag {
	tag := meta.NewTag(ty)
	tag.InsertText(meta.ItemKeyTrackTitle, "Foo title")
	tag.InsertText(meta.ItemKeyTrackArtist, "Bar artist")
	tag.InsertText(meta.ItemKeyAlbumTitle, "Baz album")
	tag.InsertText(meta.ItemKeyComment, "Qux comment")
	tag.InsertText(meta.ItemKeyTrackNumber, "1")
	tag.InsertText(meta.ItemKeyGenre, "Classical")
	return tag
}
