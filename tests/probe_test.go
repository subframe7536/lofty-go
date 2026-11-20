package lofty_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

// --- src/probe.rs ---

var probeAssetsDir = filepath.Join("..", "lofty-rs", "lofty", "tests", "files", "assets")

func testProbeFile(t *testing.T, name string, want FileType) {
	t.Helper()
	path := filepath.Join(probeAssetsDir, name)
	f, err := os.Open(path)
	if err != nil {
		t.Skip("fixture missing:", err)
	}
	defer f.Close()
	p, err := NewProbe(f).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.FileType().Get(); got != want {
		t.Errorf("%s: file_type = %v, want %v", name, got, want)
	}
}

func testProbePath(t *testing.T, name string, want FileType) {
	t.Helper()
	path := filepath.Join(probeAssetsDir, name)
	if _, err := os.Stat(path); err != nil {
		t.Skip("fixture missing:", err)
	}
	if got, ok := FileTypeFromPath(path); !ok || got != want {
		t.Errorf("%s: path file_type = %v, want %v", name, got, want)
	}
}

func testProbe(t *testing.T, name string, want FileType) {
	t.Helper()
	testProbeFile(t, name, want)
	testProbePath(t, name, want)
}

func TestProbeAac(t *testing.T) { testProbe(t, filepath.Join("minimal", "untagged.aac"), FileTypeAac) }

func TestProbeAacWithId3v2(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.aac"), FileTypeAac)
}

func TestProbeAiff(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.aiff"), FileTypeAiff)
}

func TestProbeApeWithId3v2(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.ape"), FileTypeApe)
}

func TestProbeFlac(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.flac"), FileTypeFlac)
}

func TestProbeFlacWithId3v2(t *testing.T) {
	testProbe(t, "flac_with_id3v2.flac", FileTypeFlac)
}

func TestProbeMp3WithId3v2(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.mp3"), FileTypeMpeg)
}

func TestProbeMp3WithLotsOfJunk(t *testing.T) {
	testProbe(t, "junk.mp3", FileTypeMpeg)
}

func TestProbeVorbis(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.ogg"), FileTypeVorbis)
}

func TestProbeOpus(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.opus"), FileTypeOpus)
}

func TestProbeSpeex(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "full_test.spx"), FileTypeSpeex)
}

func TestProbeMp4(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "m4a_codec_aac.m4a"), FileTypeMp4)
}

func TestProbeWav(t *testing.T) {
	testProbe(t, filepath.Join("minimal", "wav_format_pcm.wav"), FileTypeWav)
}

func TestProbeMp3Id3v2TrailingJunk(t *testing.T) {
	// 4 junk bytes between the ID3v2 portion and the first MP3 frame
	data := []byte{0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x23}
	data = append(data,
		0x54, 0x41, 0x4C, 0x42, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x01, 0xFF, 0xFE, 0x61,
		0x00, 0x61, 0x00, 0x61, 0x00, 0x61, 0x00, 0x61, 0x00, 0x61, 0x00, 0x61, 0x00, 0x61,
		0x00, 0x61, 0x00, 0x61, 0x00, 0x61, 0x00)
	data = append(data, 0x20, 0x20, 0x20, 0x20)
	data = append(data,
		0xFF, 0xFB, 0x50, 0xC4, 0x00, 0x03, 0xC0, 0x00, 0x01, 0xA4, 0x00, 0x00, 0x00, 0x20,
		0x00, 0x00, 0x34, 0x80, 0x00, 0x00, 0x04)
	p, err := NewProbe(bytes.NewReader(data)).GuessFileType()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.FileType().Get(); got != FileTypeMpeg {
		t.Fatalf("file_type = %v", got)
	}
}

func TestProbeAllocationLimit(t *testing.T) {
	createEncryptedFrame := func(size int) []byte {
		flagData := make([]byte, 5)
		payload := make([]byte, size)
		frameLen := synchsafe32(uint32(len(payload) + len(flagData)))
		header := append([]byte{'S', 'M', 'T', 'H'},
			frameLen[0], frameLen[1], frameLen[2], frameLen[3],
			0x00, 0b00000101)
		return append(append(header, flagData...), payload...)
	}
	createFakeMp3 := func(frameSize uint32) []byte {
		tagLen := synchsafe32(frameSize + 5 + 10)
		out := append([]byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x00},
			tagLen[0], tagLen[1], tagLen[2], tagLen[3])
		out = append(out, createEncryptedFrame(int(frameSize))...)
		return append(out,
			0xFF, 0xFB, 0x50, 0xC4, 0x00, 0x03, 0xC0, 0x00, 0x01, 0xA4, 0x00, 0x00, 0x00,
			0x20, 0x00, 0x00, 0x34, 0x80, 0x00, 0x00, 0x04)
	}

	po := NewParseOptions()
	po.ReadProperties = false

	global := NewGlobalOptions()
	global.AllocationLimit = 50
	ApplyGlobalOptions(global)
	defer ApplyGlobalOptions(NewGlobalOptions())

	if _, err := ProbeWithFileType(bytes.NewReader(createFakeMp3(40)), FileTypeMpeg).Options(po).Read(); err != nil {
		t.Fatalf("within limits: %v", err)
	}
	if _, err := ProbeWithFileType(bytes.NewReader(createFakeMp3(60)), FileTypeMpeg).Options(po).Read(); err == nil {
		t.Fatal("over limit: expected error")
	}
	global.AllocationLimit = DefaultAllocationLimit
	ApplyGlobalOptions(global)
	if _, err := ProbeWithFileType(bytes.NewReader(createFakeMp3(60)), FileTypeMpeg).Options(po).Read(); err != nil {
		t.Fatalf("default limit: %v", err)
	}
}

var _ = io.EOF // silence unused import if helpers change
