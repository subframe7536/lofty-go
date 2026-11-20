package meta_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/subframe7536/lofty-go/internal/meta"
)

var pictureAssetDir = filepath.Join("..", "..", "lofty-rs", "lofty", "tests", "picture", "assets")

func pictureAsset(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(pictureAssetDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func createOriginalPicture(t *testing.T) Picture {
	t.Helper()
	pic, err := PictureFromReader(bytes.NewReader(pictureAsset(t, "png_640x628.png")))
	if err != nil {
		t.Fatal(err)
	}
	pic.SetDescription("png_640x628.png")
	pic.SetPicType(PictureTypeCoverFront)
	return *pic
}

// --- tests/picture/from_reader.rs ---

func TestPictureFromReader(t *testing.T) {
	for _, tc := range []struct {
		file string
		mime MimeType
	}{
		{"png_640x628.png", MimeTypePng},
		{"jpeg_640x628.jpg", MimeTypeJpeg},
		{"bmp_640x628.bmp", MimeTypeBmp},
		{"gif_640x628.gif", MimeTypeGif},
		{"tiff_640x628.tiff", MimeTypeTiff},
	} {
		pic, err := PictureFromReader(bytes.NewReader(pictureAsset(t, tc.file)))
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		m, ok := pic.MimeType().Get()
		if !ok || m != tc.mime {
			t.Fatalf("%s: mime = %v", tc.file, pic.MimeType())
		}
	}
}

// --- tests/picture/information.rs ---

func TestPictureInfoPng(t *testing.T) {
	info, err := PictureInformationFromPNG(pictureAsset(t, "png_640x628.png"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 628 || info.ColorDepth != 32 || info.NumColors != 0 {
		t.Fatalf("%+v", info)
	}
}

func TestPictureInfoPngPlte(t *testing.T) {
	info, err := PictureInformationFromPNG(pictureAsset(t, "png_640x628_plte.png"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 628 || info.ColorDepth != 8 || info.NumColors != 118 {
		t.Fatalf("%+v", info)
	}
}

func TestPictureInfoJpeg(t *testing.T) {
	info, err := PictureInformationFromJPEG(pictureAsset(t, "jpeg_640x628.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 628 || info.ColorDepth != 24 || info.NumColors != 0 {
		t.Fatalf("%+v", info)
	}
}

// --- tests/picture/format_parsers.rs ---

func TestApeBinaryItem(t *testing.T) {
	pic, err := PictureFromApeBytes("Cover Art (Front)", pictureAsset(t, "png_640x628.apev2"))
	if err != nil {
		t.Fatal(err)
	}
	orig := createOriginalPicture(t)
	if pic.PicType() != orig.PicType() ||
		pic.MimeType() != orig.MimeType() ||
		!bytes.Equal(pic.Data(), orig.Data()) {
		t.Fatalf("picture mismatch: %+v", pic)
	}
	if d, _ := pic.Description(); d != "png_640x628.png" {
		t.Fatalf("desc = %q", d)
	}
}

func TestAsApeBytes(t *testing.T) {
	orig := createOriginalPicture(t)
	got := orig.AsApeBytes()
	want := pictureAsset(t, "png_640x628.apev2")
	if !bytes.Equal(got, want) {
		t.Fatalf("ape bytes differ: got %d want %d", len(got), len(want))
	}
}

func TestFlacMetadataBlockPicture(t *testing.T) {
	pic, _, err := PictureFromFlacBytes(pictureAsset(t, "png_640x628.vorbis"), true, ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	orig := createOriginalPicture(t)
	if pic.PicType() != orig.PicType() ||
		pic.MimeType() != orig.MimeType() ||
		!bytes.Equal(pic.Data(), orig.Data()) {
		t.Fatalf("picture mismatch")
	}
}

func TestAsFlacBytes(t *testing.T) {
	orig := createOriginalPicture(t)
	info, err := PictureInformationFromPNG(orig.Data())
	if err != nil {
		t.Fatal(err)
	}
	got := orig.AsFlacBytes(info, true)
	want := pictureAsset(t, "png_640x628.vorbis")
	if !bytes.Equal(got, want) {
		t.Fatalf("flac bytes differ: got %d want %d", len(got), len(want))
	}
}
