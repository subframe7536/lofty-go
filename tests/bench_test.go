package lofty_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/subframe7536/lofty-go"
)

func taglibData(parts ...string) string {
	p := append([]string{"..", "lofty-rs", "lofty", "tests", "taglib", "data"}, parts...)
	return filepath.Join(p...)
}

func benchRead(b *testing.B, path string) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		tf, err := ReadFromPath(path, noProps)
		if err != nil {
			b.Fatal(err)
		}
		_ = tf.Tags()
	}
}

func benchReadProps(b *testing.B, path string) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		tf, err := ReadFromPath(path)
		if err != nil {
			b.Fatal(err)
		}
		_ = tf.Properties()
	}
}

func BenchmarkReadMP3(b *testing.B)  { benchRead(b, taglibData("bladeenc.mp3")) }
func BenchmarkReadFLAC(b *testing.B) { benchRead(b, taglibData("sinewave.flac")) }
func BenchmarkReadM4A(b *testing.B)  { benchRead(b, taglibData("has-tags.m4a")) }
func BenchmarkReadAPE(b *testing.B)  { benchRead(b, taglibData("mac-399-tagged.ape")) }
func BenchmarkReadOpus(b *testing.B) {
	benchRead(b, taglibData("correctness_gain_silent_output.opus"))
}
func BenchmarkReadWavPack(b *testing.B) { benchRead(b, taglibData("tagged.wv")) }
func BenchmarkReadWAV(b *testing.B)     { benchRead(b, taglibData("float64.wav")) }

func BenchmarkReadMP3WithProperties(b *testing.B) {
	benchReadProps(b, taglibData("bladeenc.mp3"))
}

func BenchmarkReadFLACWithProperties(b *testing.B) {
	benchReadProps(b, taglibData("sinewave.flac"))
}

func BenchmarkWriteMP3Tag(b *testing.B) {
	src := taglibData("bladeenc.mp3")
	b.ReportAllocs()
	for b.Loop() {
		data, err := os.ReadFile(src)
		if err != nil {
			b.Fatal(err)
		}
		f, err := os.CreateTemp(b.TempDir(), "bench-*.mp3")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			b.Fatal(err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			b.Fatal(err)
		}
		btf, err := NewProbe(f).Options(noProps).ReadBound(NewOSFile(f))
		if err != nil {
			b.Fatal(err)
		}
		tag := btf.PrimaryTagMutOrInsert()
		if tag == nil {
			b.Fatal("no tag")
		}
		tag.InsertText(ItemKeyTrackTitle, "bench title")
		if err := btf.Save(NewWriteOptions()); err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkParseId3v2Tag(b *testing.B) {
	data, err := os.ReadFile(taglibData("005411.id3"))
	if err != nil {
		b.Fatal(err)
	}
	opts := NewParseOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		header, err := ParseId3v2Header(bytes.NewReader(data), opts)
		if err != nil {
			b.Fatal(err)
		}
		tag, err := ParseId3v2Tag(bytes.NewReader(data[10:]), header, opts)
		if err != nil {
			b.Fatal(err)
		}
		_ = tag
	}
}
