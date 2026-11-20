package lofty_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

func TestPairsSetNumber(t *testing.T) {
	// Whitespace-padded text is trimmed and parsed.
	tag := NewApeTag()
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText("  12  "))); err != nil {
		t.Fatal(err)
	}
	if n, ok := tag.Track(); !ok || n != 12 {
		n, _ := tag.Track()
		t.Fatalf("track = %v", n)
	}
	// Empty/whitespace-only values insert nothing.
	tag = NewApeTag()
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText(""))); err != nil {
		t.Fatal(err)
	}
	if err := tag.Insert(NewTagItem(ItemKeyTrackNumber, ItemValueText("        "))); err != nil {
		t.Fatal(err)
	}
	if _, ok := tag.Track(); ok {
		t.Fatal("track should be unset")
	}
	if !tag.IsEmpty() {
		t.Fatal("tag should be empty")
	}
}

// --- src/util/io.rs ---

const ioTestAsset = "../lofty-rs/lofty/tests/files/assets/minimal/full_test.mp3"

func ioAlterTag(file *MpegFile) {
	tag := file.ID3v2.MustGet()
	tag.SetArtist("Bar artist")
}

func ioRevertTag(file *MpegFile) {
	tag := file.ID3v2.MustGet()
	tag.SetArtist("Foo artist")
}

func TestIoSaveToFile(t *testing.T) {
	fileContent, err := os.ReadFile(ioTestAsset)
	if err != nil {
		t.Fatal(err)
	}
	file, err := MpegFileReadFrom(bytes.NewReader(fileContent), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioAlterTag(file)

	tmp, err := os.CreateTemp("", "lofty-io-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(fileContent); err != nil {
		t.Fatal(err)
	}

	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := file.SaveFile(NewOSFile(tmp), wo); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	file2, err := MpegFileReadFrom(NewOSFile(tmp), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioRevertTag(file2)
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := file2.SaveFile(NewOSFile(tmp), wo); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, fileContent) {
		t.Fatal("contents differ after revert")
	}
}

func TestIoSaveToVec(t *testing.T) {
	fileContent, err := os.ReadFile(ioTestAsset)
	if err != nil {
		t.Fatal(err)
	}
	file, err := MpegFileReadFrom(bytes.NewReader(fileContent), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioAlterTag(file)

	bf := NewBytesFile(append([]byte(nil), fileContent...))
	wo := NewWriteOptions()
	wo.PreferredPadding = 0
	if err := file.SaveFile(bf, wo); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	file2, err := MpegFileReadFrom(bf, NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	ioRevertTag(file2)
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := file2.SaveFile(bf, wo); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if !bytes.Equal(bf.Bytes(), fileContent) {
		t.Fatal("contents differ after revert")
	}
}

func TestSynchsafeUnsynchronizedStream(t *testing.T) {
	// tests/taglib/test_synchdata.rs::test_decode{1..4}
	for _, tc := range []struct{ in_, want []byte }{
		{[]byte{0xFF, 0x00, 0x00}, []byte{0xFF, 0x00}},
		{[]byte{0xFF, 0x44}, []byte{0xFF, 0x44}},
		{[]byte{0xFF, 0xFF, 0x00}, []byte{0xFF, 0xFF}},
		{[]byte{0xFF, 0xFF, 0xFF}, []byte{0xFF, 0xFF, 0xFF}},
	} {
		if got := Id3v2Deunsync(append([]byte{}, tc.in_...)); !bytes2Equal(got, tc.want) {
			t.Errorf("deunsync(%x) = %x, want %x", tc.in_, got, tc.want)
		}
	}

	unsyncContent := []byte{0xFF, 0x00, 0x00, 0xFF, 0x12, 0xB0, 0x05, 0xFF, 0x00, 0x00}
	expected := []byte{0xFF, 0x00, 0xFF, 0x12, 0xB0, 0x05, 0xFF, 0x00}

	if got := Id3v2Deunsync(append([]byte{}, unsyncContent...)); !bytes2Equal(got, expected) {
		t.Errorf("deunsync = %x", got)
	}
	// Large buffer (repeated content)
	large := append([]byte{}, unsyncContent...)
	for i := 0; i < 999; i++ {
		large = append(large, unsyncContent...)
	}
	largeExp := append([]byte{}, expected...)
	for i := 0; i < 999; i++ {
		largeExp = append(largeExp, expected...)
	}
	if got := Id3v2Deunsync(large); !bytes2Equal(got, largeExp) {
		t.Errorf("deunsync large: len = %d, want %d", len(got), len(largeExp))
	}
	// Unrelated content is untouched
	original := []byte{0xFF, 0x1A, 0xFF, 0xC0, 0x10, 0x01}
	if got := Id3v2Deunsync(append([]byte{}, original...)); !bytes2Equal(got, original) {
		t.Errorf("deunsync unrelated = %x", got)
	}
}

func bytes2Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
