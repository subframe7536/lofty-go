package lofty

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// FuzzReadFrom replays arbitrary bytes through every format reader, seeded
// with the Rust fuzz corpus. The fuzz engine treats panics as crashes.
func FuzzReadFrom(f *testing.F) {
	base := filepath.Join("lofty-rs", "lofty", "tests", "fuzz", "assets")
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
		return nil
	})
	f.Fuzz(func(t *testing.T, data []byte) {
		po := NewParseOptions()
		if _, err := ReadFrom(bytes.NewReader(data), po); err != nil {
			return
		}
		// Exercise the strict path too — behavior must diverge only via
		// error returns, never panics.
		po.ParsingMode = ParsingModeStrict
		_, _ = ReadFrom(bytes.NewReader(data), po)
	})
}

// FuzzId3v2Parse fuzzes the ID3v2 tag parser directly, mirroring the Rust
// `tests/fuzz/id3v2.rs` targets.
func FuzzId3v2Parse(f *testing.F) {
	f.Add([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"))
	f.Add([]byte("ID3\x03\x00\xC0\x00\x00\x00\x0A\x00\x06\x80\x00\x00\x00\x00"))
	f.Add([]byte{1, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		po := NewParseOptions()
		r := bytes.NewReader(data)
		header, err := ParseId3v2Header(r, po)
		if err != nil {
			return
		}
		if _, err := ParseId3v2Tag(r, header, po); err != nil {
			return
		}
	})
}
