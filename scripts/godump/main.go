// godump prints a file's parsed metadata in the line format used by the
// fixture differential run against lofty-rs' `dump` example:
//
//	FILE <path>
//	TYPE <Rust FileType debug name>
//	PROPS dur=<ms> overall=<kbps> audio=<kbps> rate=<hz> depth=<bits|-> ch=<n|-> mask=<ChannelMask(n)|->
//	TAG <TagType>
//	  ITEM <ItemKey> = T:"<text>" | L:"<locator>" | B:<len>   (sorted by key, then value)
//	  PIC <type> <MimeName|-> <data len>
//	ERR                       (read failure)
//
// Usage: godump <file> [file...]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	meta "github.com/subframe7536/lofty-go/internal/meta"

	_ "github.com/subframe7536/lofty-go/aac"
	_ "github.com/subframe7536/lofty-go/ape"
	_ "github.com/subframe7536/lofty-go/dsf"
	_ "github.com/subframe7536/lofty-go/flac"
	_ "github.com/subframe7536/lofty-go/id3v1"
	_ "github.com/subframe7536/lofty-go/id3v2"
	_ "github.com/subframe7536/lofty-go/iff"
	_ "github.com/subframe7536/lofty-go/lyrics3"
	_ "github.com/subframe7536/lofty-go/mp4"
	_ "github.com/subframe7536/lofty-go/mpc"
	_ "github.com/subframe7536/lofty-go/mpeg"
	_ "github.com/subframe7536/lofty-go/ogg"
	_ "github.com/subframe7536/lofty-go/vorbis"
	_ "github.com/subframe7536/lofty-go/wavpack"
)

// rustFileTypeName prints the Rust `FileType` variant's Debug name (the Go
// String form is all-caps).
func rustFileTypeName(t meta.FileType) string {
	switch {
	case t == meta.FileTypeAac:
		return "Aac"
	case t == meta.FileTypeAiff:
		return "Aiff"
	case t == meta.FileTypeApe:
		return "Ape"
	case t == meta.FileTypeDsf:
		return "Dsf"
	case t == meta.FileTypeFlac:
		return "Flac"
	case t == meta.FileTypeMpeg:
		return "Mpeg"
	case t == meta.FileTypeMp4:
		return "Mp4"
	case t == meta.FileTypeMpc:
		return "Mpc"
	case t == meta.FileTypeOpus:
		return "Opus"
	case t == meta.FileTypeVorbis:
		return "Vorbis"
	case t == meta.FileTypeSpeex:
		return "Speex"
	case t == meta.FileTypeWav:
		return "Wav"
	case t == meta.FileTypeWavPack:
		return "WavPack"
	case t.IsCustom():
		return fmt.Sprintf("Custom(%q)", t.CustomName())
	}
	return "Unknown"
}

func optU32(o meta.Option[uint32]) string {
	if v, ok := o.Get(); ok {
		return strconv.FormatUint(uint64(v), 10)
	}
	return "-"
}

func optU8(o meta.Option[uint8]) string {
	if v, ok := o.Get(); ok {
		return strconv.FormatUint(uint64(v), 10)
	}
	return "-"
}

// mimeName prints the Rust `MimeType` variant name (`-` when unset).
func mimeName(p meta.Picture) string {
	m, ok := p.MimeType().Get()
	if !ok {
		return "-"
	}
	switch m.AsStr() {
	case "image/png":
		return "Png"
	case "image/jpeg":
		return "Jpeg"
	case "image/tiff":
		return "Tiff"
	case "image/bmp":
		return "Bmp"
	case "image/gif":
		return "Gif"
	}
	return fmt.Sprintf("Unknown(%s)", m.AsStr())
}

func itemValueRepr(v meta.ItemValue) string {
	if s, ok := v.Text(); ok {
		return fmt.Sprintf("T:%q", s)
	}
	if s, ok := v.Locator(); ok {
		return fmt.Sprintf("L:%q", s)
	}
	if b, ok := v.Binary(); ok {
		return fmt.Sprintf("B:%d", len(b))
	}
	return "?:"
}

func dump(path string) {
	fmt.Printf("FILE %s\n", path)
	tagged, err := meta.ReadFromPath(path)
	if err != nil {
		fmt.Println("ERR")
		return
	}
	fmt.Printf("TYPE %s\n", rustFileTypeName(tagged.FileType()))
	p := tagged.Properties()
	mask := "-"
	if m, ok := p.ChannelMask().Get(); ok {
		mask = fmt.Sprintf("ChannelMask(%d)", m.Bits())
	}
	fmt.Printf("PROPS dur=%d overall=%s audio=%s rate=%s depth=%s ch=%s mask=%s\n",
		p.Duration().Milliseconds(),
		optU32(p.OverallBitrate()),
		optU32(p.AudioBitrate()),
		optU32(p.SampleRate()),
		optU8(p.BitDepth()),
		optU8(p.Channels()),
		mask,
	)
	for _, tag := range tagged.Tags() {
		fmt.Printf("TAG %s\n", tag.TagType())
		items := tag.Items()
		slices.SortFunc(items, func(a, b meta.TagItem) int {
			if c := strings.Compare(a.Key().String(), b.Key().String()); c != 0 {
				return c
			}
			return strings.Compare(itemValueRepr(a.Value()), itemValueRepr(b.Value()))
		})
		for _, item := range items {
			fmt.Printf("  ITEM %s = %s\n", item.Key(), itemValueRepr(item.Value()))
		}
		for _, pic := range tag.Pictures() {
			fmt.Printf("  PIC %d %s %d\n", pic.PicType(), mimeName(pic), len(pic.Data()))
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <file> [file...]\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		dump(path)
	}
}
