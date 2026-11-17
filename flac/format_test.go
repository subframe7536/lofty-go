package flac

import (
	"bytes"
	"errors"
	"io"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

// flacTestStreamInfo builds a 34-byte STREAMINFO payload (stereo, 16-bit).
func flacTestStreamInfo(sampleRate uint32, totalSamples uint64) []byte {
	out := make([]byte, 34)
	packed := uint64(sampleRate)<<44 | uint64(1)<<41 | uint64(15)<<36 | totalSamples
	for i := 0; i < 8; i++ {
		out[10+i] = byte(packed >> (56 - 8*i))
	}
	for i := 18; i < 34; i++ {
		out[i] = byte(i)
	}
	return out
}

func flacTestBlock(ty byte, last bool, content []byte) []byte {
	return (&flacBlock{ty: ty, content: content}).marshal(last)
}

// flacTestFile concatenates the marker, the given blocks and some audio bytes.
func flacTestFile(blocks ...[]byte) []byte {
	out := append([]byte{}, flacPreamble...)
	for _, b := range blocks {
		out = append(out, b...)
	}
	return append(out, bytes.Repeat([]byte{0xAA}, 64)...)
}

func flacTestVorbisComment(vendor string, kvs ...string) []byte {
	var out []byte
	put := func(n int) { out = append(out, byte(n), byte(n>>8), byte(n>>16), byte(n>>24)) }
	put(len(vendor))
	out = append(out, vendor...)
	put(len(kvs))
	for _, kv := range kvs {
		put(len(kv))
		out = append(out, kv...)
	}
	return out
}

func flacTestPictureBlock(t *testing.T, pic meta.Picture) []byte {
	t.Helper()
	return flacTestBlock(blockIDPicture, false, pic.AsFlacBytes(meta.PictureInformation{}, false))
}

func flacTestParseErr(t *testing.T, err error) *FlacParseError {
	t.Helper()
	var pe *FlacParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *FlacParseError", err)
	}
	return pe
}

func withAllocationLimit(t *testing.T, limit int) {
	t.Helper()
	old := util.GetGlobalOptions()
	opts := old
	opts.AllocationLimit = limit
	util.ApplyGlobalOptions(opts)
	t.Cleanup(func() { util.ApplyGlobalOptions(old) })
}

// TestReadLeadingID3v2Footer: the reader must end exactly at the end of a
// leading ID3v2 tag, including its footer and extended header.
func TestReadLeadingID3v2Footer(t *testing.T) {
	flacBody := flacTestFile(flacTestBlock(blockIDStreamInfo, true, flacTestStreamInfo(44100, 88200)))

	cases := map[string][]byte{
		"footer": func() []byte {
			out := []byte{'I', 'D', '3', 4, 0, 0x10, 0, 0, 0, 10}
			out = append(out, make([]byte, 10)...)
			out = append(out, '3', 'D', 'I', 4, 0, 0x10, 0, 0, 0, 10)
			return out
		}(),
		"extended header": func() []byte {
			out := []byte{'I', 'D', '3', 4, 0, 0x40, 0, 0, 0, 10}
			out = append(out, 0, 0, 0, 6, 1, 0) // extended header
			out = append(out, make([]byte, 4)...)
			return out
		}(),
	}
	for name, id3 := range cases {
		t.Run(name, func(t *testing.T) {
			data := append(append([]byte{}, id3...), flacBody...)
			f, err := FlacFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if f.ID3v2.IsNone() {
				t.Fatal("ID3v2 tag not read")
			}
			if f.Properties.SampleRate != 44100 || f.Properties.Duration.Seconds() != 2 {
				t.Fatalf("properties = %+v", f.Properties)
			}
		})
	}
}

// TestStreamInfoSizeChecks mirrors Rust's `stream_info_len < 18` (measured
// with the 4-byte header) and the lazy property parse.
func TestStreamInfoSizeChecks(t *testing.T) {
	read := func(si []byte, opts meta.ParseOptions) error {
		_, err := FlacFileReadFrom(bytes.NewReader(flacTestFile(flacTestBlock(blockIDStreamInfo, true, si))), opts)
		return err
	}

	var sm *meta.SizeMismatchError
	if err := read(make([]byte, 13), meta.NewParseOptions()); !errors.As(err, &sm) {
		t.Fatalf("13-byte STREAMINFO: err = %v, want SizeMismatchError", err)
	}

	// 14..33 bytes passes the size check but the properties parse runs dry
	if err := read(make([]byte, 20), meta.NewParseOptions()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("20-byte STREAMINFO: err = %v, want unexpected EOF", err)
	}
	noProps := meta.NewParseOptions()
	noProps.ReadProperties = false
	if err := read(make([]byte, 20), noProps); err != nil {
		t.Fatalf("20-byte STREAMINFO without properties: %v", err)
	}
}

func TestReadMissingStreamInfo(t *testing.T) {
	data := flacTestFile(flacTestBlock(blockIDPadding, true, make([]byte, 40)))
	_, err := FlacFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if pe := flacTestParseErr(t, err); pe.Message != "file missing mandatory STREAMINFO block" {
		t.Fatalf("message = %q", pe.Message)
	}

	_, err = FlacFileReadFrom(bytes.NewReader([]byte("fLaX____")), meta.NewParseOptions())
	if pe := flacTestParseErr(t, err); pe.Message != `file missing "fLaC" stream marker` {
		t.Fatalf("message = %q", pe.Message)
	}
}

// TestReadSkipsUnneededBlocks: payloads of blocks the options don't ask for
// are seeked over, never buffered (so they can't hit the allocation limit).
func TestReadSkipsUnneededBlocks(t *testing.T) {
	bigPic := meta.PictureUnchecked(make([]byte, 5000)).PicType(meta.PictureTypeCoverFront).Build()
	data := flacTestFile(
		flacTestBlock(blockIDStreamInfo, false, flacTestStreamInfo(44100, 44100)),
		flacTestBlock(blockIDVorbisComment, false, flacTestVorbisComment("v", "TITLE=T")),
		flacTestPictureBlock(t, bigPic),
		flacTestBlock(blockIDPadding, true, make([]byte, 16)),
	)
	withAllocationLimit(t, 1000)

	opts := meta.NewParseOptions()
	opts.ReadCoverArt = false
	f, err := FlacFileReadFrom(bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("read without cover art: %v", err)
	}
	if len(f.Pictures()) != 0 || f.VorbisComments.IsNone() {
		t.Fatalf("pictures=%d vc=%v", len(f.Pictures()), f.VorbisComments)
	}
	if f.Properties.Duration.Seconds() != 1 {
		t.Fatalf("properties = %+v", f.Properties)
	}

	opts = meta.NewParseOptions()
	opts.ReadTags = false
	opts.ReadCoverArt = false
	f, err = FlacFileReadFrom(bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("read without tags: %v", err)
	}
	if f.VorbisComments.IsSome() {
		t.Fatal("vorbis comments read despite ReadTags=false")
	}

	// Requested pictures are buffered through the allocation limit
	var ae *util.AllocationError
	if _, err := FlacFileReadFrom(bytes.NewReader(data), meta.NewParseOptions()); !errors.As(err, &ae) {
		t.Fatalf("err = %v, want AllocationError", err)
	}
}

// TestWriteRequiresStreamInfoFirst mirrors Rust's `verify_flac` on the write path.
func TestWriteRequiresStreamInfoFirst(t *testing.T) {
	data := flacTestFile(flacTestBlock(blockIDPadding, true, make([]byte, 40)))
	_, err := flacWriteScan(util.NewBytesFile(data), meta.NewParseOptions())
	if pe := flacTestParseErr(t, err); pe.Message != "file missing mandatory STREAMINFO block" {
		t.Fatalf("message = %q", pe.Message)
	}
	var fe *meta.FileEncodingError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FileEncodingError", err)
	}
}

// TestWriteSkipsPictureBuffers: PICTURE blocks are removed without being read,
// so a picture bigger than the allocation limit doesn't block a write.
func TestWriteSkipsPictureBuffers(t *testing.T) {
	bigPic := meta.PictureUnchecked(make([]byte, 5000)).PicType(meta.PictureTypeCoverFront).Build()
	seek := flacTestBlock(blockIDSeekTable, false, make([]byte, 18))
	data := flacTestFile(
		flacTestBlock(blockIDStreamInfo, false, flacTestStreamInfo(44100, 44100)),
		seek,
		flacTestPictureBlock(t, bigPic),
		flacTestBlock(blockIDVorbisComment, true, flacTestVorbisComment("file-vendor", "TITLE=Old")),
	)
	withAllocationLimit(t, 1000)

	f := util.NewBytesFile(data)
	tag := meta.NewTag(meta.TagTypeVorbisComments)
	tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackTitle, meta.ItemValueText("New")))
	if err := flacWriteTag(f, tag, meta.WriteOptions{}); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := FlacFileReadFrom(bytes.NewReader(f.Bytes()), meta.NewParseOptions())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(out.Pictures()) != 0 {
		t.Fatalf("pictures survived the write: %d", len(out.Pictures()))
	}
	vc, ok := out.VorbisComments.Get()
	if !ok {
		t.Fatal("no vorbis comments after write")
	}
	if title, _ := vc.Title(); title != "New" || vc.Vendor != "file-vendor" {
		t.Fatalf("title=%q vendor=%q", title, vc.Vendor)
	}
	if !bytes.Contains(f.Bytes(), seek) {
		t.Fatal("SEEKTABLE block was not preserved verbatim")
	}
}

func TestFlacFilePictureStorage(t *testing.T) {
	mk := func(ty meta.PictureType, b byte) meta.Picture {
		return meta.PictureUnchecked([]byte{b, b, b, b, b, b, b, b}).PicType(ty).Build()
	}
	info := meta.Some(meta.PictureInformation{})

	var f FlacFile
	if err := f.InsertPicture(mk(meta.PictureTypeCoverFront, 1), info); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InsertPictureReplacing(mk(meta.PictureTypeIcon, 2), info); err != nil {
		t.Fatal(err)
	}
	removed, err := f.InsertPictureReplacing(mk(meta.PictureTypeIcon, 3), info)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := removed.Get(); !ok || r.Picture.Data()[0] != 2 {
		t.Fatalf("removed = %+v, want the first icon", removed)
	}
	if len(f.Pictures()) != 2 || f.Pictures()[1].Picture.Data()[0] != 3 {
		t.Fatalf("pictures = %+v", f.Pictures())
	}

	// Non-icon types are never replaced
	removed, err = f.InsertPictureReplacing(mk(meta.PictureTypeCoverFront, 4), info)
	if err != nil || removed.IsSome() || len(f.Pictures()) != 3 {
		t.Fatalf("removed=%v err=%v pictures=%d", removed, err, len(f.Pictures()))
	}

	// Extraction failure is returned and leaves the file untouched
	short := meta.PictureUnchecked([]byte{1, 2, 3}).PicType(meta.PictureTypeIcon).Build()
	if err := f.InsertPicture(short, meta.None[meta.PictureInformation]()); err == nil {
		t.Fatal("expected PictureInformation extraction error")
	}
	if len(f.Pictures()) != 3 {
		t.Fatalf("file mutated on error: %d pictures", len(f.Pictures()))
	}

	f.RemovePictureType(meta.PictureTypeCoverFront)
	if len(f.Pictures()) != 1 || f.Pictures()[0].Picture.PicType() != meta.PictureTypeIcon {
		t.Fatalf("after RemovePictureType: %+v", f.Pictures())
	}

	f.SetPicture(0, mk(meta.PictureTypeBand, 5), meta.PictureInformation{})
	f.SetPicture(100, mk(meta.PictureTypeBand, 6), meta.PictureInformation{})
	if len(f.Pictures()) != 2 || f.Pictures()[0].Picture.Data()[0] != 5 || f.Pictures()[1].Picture.Data()[0] != 6 {
		t.Fatalf("after SetPicture: %+v", f.Pictures())
	}

	if got := f.RemovePicture(0); got.Picture.Data()[0] != 5 || len(f.Pictures()) != 1 {
		t.Fatalf("RemovePicture returned %+v", got)
	}
	if all := f.RemovePictures(); len(all) != 1 || len(f.Pictures()) != 0 {
		t.Fatalf("RemovePictures returned %d", len(all))
	}
}

// Keep the vorbis import honest: FlacFile's tag type is the vorbis tag.
var _ = (*vorbis.VorbisCommentsTag)(nil)
