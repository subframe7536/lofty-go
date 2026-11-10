package flac

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	flac "github.com/go-flac/go-flac/v2"

	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

var flacPreamble = []byte("fLaC")

// FlacParseError is returned when a FLAC file fails to parse.
type FlacParseError struct {
	Message string
	Source  error
}

func (e *FlacParseError) Error() string {
	if e.Message != "" {
		return "FLAC: " + e.Message
	}
	return "failed to parse FLAC file"
}
func (e *FlacParseError) Unwrap() error { return e.Source }

// FlacProperties are the audio properties of a FLAC file.
type FlacProperties struct {
	Duration       time.Duration
	OverallBitrate meta.Option[uint32]
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	// Signature is the MD5 of the unencoded audio data from STREAMINFO.
	Signature [16]byte
}

// FileProperties converts to generic [meta.FileProperties].
func (p FlacProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		p.OverallBitrate,
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(p.BitDepth),
		meta.Some(p.Channels),
		meta.None[meta.ChannelMask](),
	)
}

// FlacFile is a FLAC file.
type FlacFile struct {
	Properties     FlacProperties
	VorbisComments meta.Option[*vorbis.VorbisCommentsTag]
	ID3v2          meta.Option[*id3v2.Id3v2Tag]
	pictures       []meta.PictureInfoPair
}

// Pictures returns the file's pictures.
func (f *FlacFile) Pictures() []meta.PictureInfoPair { return f.pictures }

// SetPictures replaces the file's pictures.
func (f *FlacFile) SetPictures(pictures []meta.PictureInfoPair) { f.pictures = pictures }

// InsertPicture appends a picture to the file. When `info` is unset, the
// information is extracted from the picture data (PNG/JPEG only).
func (f *FlacFile) InsertPicture(p meta.Picture, info meta.Option[meta.PictureInformation]) error {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = meta.PictureInformationFromPicture(&p)
		if err != nil {
			i = meta.PictureInformation{}
		}
	}
	f.pictures = append(f.pictures, meta.PictureInfoPair{Picture: p, Info: i})
	return nil
}

// RemovePicture removes the picture at `index`.
func (f *FlacFile) RemovePicture(index int) {
	if index >= 0 && index < len(f.pictures) {
		f.pictures = append(f.pictures[:index], f.pictures[index+1:]...)
	}
}

// RemovePictures removes all of the file's pictures.
func (f *FlacFile) RemovePictures() { f.pictures = nil }

// FlacFileReadFrom reads a [FlacFile] from a reader.
func FlacFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*FlacFile, error) {
	var file FlacFile
	fileStart, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - fileStart)

	// Optional leading ID3v2
	if _, err := reader.Seek(fileStart, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	var id3v2Tag *id3v2.Id3v2Tag
	header, tagEnd, found, err := id3v2.FindId3v2(reader, false, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if found {
		if parseOptions.ReadTags {
			if _, err := reader.Seek(tagEnd-int64(header.Size), io.SeekStart); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			t, err := id3v2.ParseId3v2Tag(reader, header, parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
			}
			id3v2Tag = t
		}
	}

	// flac.ParseMetadata consumes the "fLaC" marker itself
	ffile, err := flac.ParseMetadata(reader)
	if err != nil {
		return nil, &meta.FileParseError{Source: &FlacParseError{Source: err}}
	}
	metaEnd, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - metaEnd)

	var vc *vorbis.VorbisCommentsTag
	var pictures []meta.PictureInfoPair

	for _, block := range ffile.Meta {
		switch block.Type {
		case flac.VorbisComment:
			if parseOptions.ReadTags {
				if vc != nil && parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &meta.FileParseError{Source: &FlacParseError{Message: "FLAC file contains multiple VORBIS_COMMENT blocks"}}
				}
				t, err := vorbis.ParseVorbisComments(block.Data, parseOptions)
				if err != nil {
					return nil, &meta.FileParseError{Source: &vorbis.VorbisCommentsParseError{Source: err}}
				}
				vc = t
			}
		case flac.Picture:
			if parseOptions.ReadTags && parseOptions.ReadCoverArt {
				pic, info, err := meta.PictureFromFlacBytes(block.Data, false, parseOptions.ParsingMode)
				if err == nil {
					pictures = append(pictures, meta.PictureInfoPair{Picture: pic, Info: info})
				}
			}
		}
	}

	if parseOptions.ReadProperties {
		si, err := ffile.GetStreamInfo()
		if err != nil {
			return nil, &meta.FileParseError{Source: &FlacParseError{Source: err}}
		}
		p := &file.Properties
		p.SampleRate = uint32(si.SampleRate)
		p.BitDepth = uint8(si.BitDepth)
		p.Channels = uint8(si.ChannelCount)
		copy(p.Signature[:], si.AudioMD5)
		if si.SampleRate > 0 {
			p.Duration = util.DurationFromSeconds(float64(si.SampleCount) / float64(si.SampleRate))
			if secs := float64(si.SampleCount) / float64(si.SampleRate); secs > 0 {
				p.AudioBitrate = uint32(float64(streamLen) * 8 / secs / 1000)
				p.OverallBitrate = meta.Some(uint32(float64(fileLength) * 8 / secs / 1000))
			}
		}
	}

	file.VorbisComments = util.OptionFromPtr(vc)
	file.ID3v2 = util.OptionFromPtr(id3v2Tag)
	file.pictures = pictures
	return &file, nil
}

// FileType implements meta.AudioFile.
func (f *FlacFile) FileType() meta.FileType { return meta.FileTypeFlac }

// GetProperties implements meta.AudioFile.
func (f *FlacFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *FlacFile) ContainsTag() bool {
	return f.VorbisComments.IsSome() || f.ID3v2.IsSome() || len(f.pictures) > 0
}

// ContainsTagType implements meta.AudioFile.
func (f *FlacFile) ContainsTagType(tagType meta.TagType) bool {
	switch tagType {
	case meta.TagTypeVorbisComments:
		return f.VorbisComments.IsSome() || len(f.pictures) > 0
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [meta.TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v2, VorbisComments.
func (f *FlacFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.VorbisComments.Get(); ok {
		g := t.Tag()
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	} else if len(f.pictures) > 0 {
		g := meta.NewTag(meta.TagTypeVorbisComments)
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	}
	return meta.NewTaggedFile(meta.FileTypeFlac, tags, *f.GetProperties())
}

// SavePath saves the file's tags back to the path.
func (f *FlacFile) SavePath(path string, writeOptions meta.WriteOptions) error {
	return meta.SaveToPath(path, f, meta.TagTypeVorbisComments, writeOptions)
}

// SaveFile writes tags to an open file.
func (f *FlacFile) SaveFile(file meta.FileLike, writeOptions meta.WriteOptions) error {
	if t, ok := f.ID3v2.Get(); ok {
		if err := t.SaveToVerified(meta.NewVerifiedFileOf(meta.FileTypeFlac, file), writeOptions); err != nil {
			return err
		}
	}
	var g *meta.Tag
	if t, ok := f.VorbisComments.Get(); ok {
		g = t.Tag()
	} else if len(f.pictures) == 0 {
		// Rust's write_to is a no-op when the tag is absent and there are no
		// pictures to store; it does not strip the existing VC block.
		return nil
	} else {
		g = meta.NewTag(meta.TagTypeVorbisComments)
	}
	// The Vorbis comments' own pictures chain with the file's pictures,
	// mirroring Rust's `FlacFile::write_to`.
	pics := make([]meta.PictureInfoPair, 0, len(f.pictures)+len(g.Pictures()))
	for _, p := range g.Pictures() {
		info, err := meta.PictureInformationFromPicture(&p)
		if err != nil {
			info = meta.PictureInformation{}
		}
		pics = append(pics, meta.PictureInfoPair{Picture: p, Info: info})
	}
	pics = append(pics, f.pictures...)
	return flacWriteTagPics(file, g, pics, writeOptions)
}

// RemoveFromPath removes a tag type.
func (f *FlacFile) RemoveFromPath(path string, tagType meta.TagType) error {
	return meta.RemoveTagFromPath(path, tagType)
}

// flacWriteTag replaces the VORBIS_COMMENT + PICTURE blocks in a FLAC file,
// preserving all other metadata blocks and the audio frames.
func flacWriteTag(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	pics := make([]meta.PictureInfoPair, 0, len(tag.Pictures()))
	for _, p := range tag.Pictures() {
		info, err := meta.PictureInformationFromPicture(&p)
		if err != nil {
			info = meta.PictureInformation{}
		}
		pics = append(pics, meta.PictureInfoPair{Picture: p, Info: info})
	}
	return flacWriteTagPics(file, tag, pics, writeOptions)
}

// flacWriteTagPics is [flacWriteTag] with explicit [meta.PictureInformation] for
// each picture block.
func flacWriteTagPics(file meta.FileLike, tag *meta.Tag, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	// Skip a leading ID3v2 tag if present
	if _, _, found, err := id3v2.FindId3v2(file, false, writeOptions.ParseOptions); err != nil {
		return &meta.FileEncodingError{Source: err}
	} else if !found {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}

	var marker [4]byte
	if _, err := io.ReadFull(file, marker[:]); err != nil || !bytes.Equal(marker[:], flacPreamble) {
		return &meta.FileEncodingError{Source: &FlacParseError{Source: &meta.FakeTagError{}}}
	}

	// Read the metadata blocks manually so we know their extent
	metaStart, _ := file.Seek(0, io.SeekCurrent)
	var kept [][]byte // raw marshaled blocks to preserve (without last flag)
	hasPadding := false
	var vendor string
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(file, hdr[:]); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		last := hdr[0]&0x80 != 0
		blockType := hdr[0] & 0x7F
		size := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
		data, err := util.TryByteSlice(size)
		if err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, data); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if blockType == byte(flac.Padding) {
			hasPadding = true
		}
		// Retain the original vendor string
		if blockType == byte(flac.VorbisComment) && len(data) >= 4 {
			vlen := int(binary.LittleEndian.Uint32(data))
			if vlen <= len(data)-4 {
				vendor = string(data[4 : 4+vlen])
			}
		}
		// Keep non-tag blocks verbatim (STREAMINFO, SEEKTABLE, PADDING, etc.)
		if blockType != byte(flac.VorbisComment) && blockType != byte(flac.Picture) {
			var hdrB [4]byte
			hdrB[0] = blockType
			hdrB[1] = byte(size >> 16)
			hdrB[2] = byte(size >> 8)
			hdrB[3] = byte(size)
			kept = append(kept, append(hdrB[:], data...))
		}
		if last {
			break
		}
	}
	metaEnd, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	// Build new tag blocks
	vc := vorbis.VorbisCommentsTagForWritePics(tag, vendor, false)
	return flacWriteBlocksAt(file, metaStart, metaEnd, kept, hasPadding, vc, !tag.IsEmpty(), pics, writeOptions)
}

// flacWriteVorbisComments writes a concrete [vorbis.VorbisCommentsTag] into a FLAC
// file, mirroring Rust `flac::write::write_to_inner`.
func flacWriteVorbisComments(file meta.FileLike, vc *vorbis.VorbisCommentsTag, writeOptions meta.WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if _, _, found, err := id3v2.FindId3v2(file, false, writeOptions.ParseOptions); err != nil {
		return &meta.FileEncodingError{Source: err}
	} else if !found {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	var marker [4]byte
	if _, err := io.ReadFull(file, marker[:]); err != nil || !bytes.Equal(marker[:], flacPreamble) {
		return &meta.FileEncodingError{Source: &FlacParseError{Source: &meta.FakeTagError{}}}
	}
	metaStart, _ := file.Seek(0, io.SeekCurrent)
	var kept [][]byte
	hasPadding := false
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(file, hdr[:]); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		last := hdr[0]&0x80 != 0
		blockType := hdr[0] & 0x7F
		size := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
		data, err := util.TryByteSlice(size)
		if err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, data); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if blockType == byte(flac.Padding) {
			hasPadding = true
		}
		if blockType != byte(flac.VorbisComment) && blockType != byte(flac.Picture) {
			var hdrB [4]byte
			hdrB[0] = blockType
			hdrB[1] = byte(size >> 16)
			hdrB[2] = byte(size >> 8)
			hdrB[3] = byte(size)
			kept = append(kept, append(hdrB[:], data...))
		}
		if last {
			break
		}
	}
	metaEnd, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return flacWriteBlocksAt(file, metaStart, metaEnd, kept, hasPadding, vc, !vc.IsEmpty(), vc.Pictures(), writeOptions)
}

func flacWriteBlocksAt(file meta.FileLike, metaStart, metaEnd int64, kept [][]byte, hasPadding bool, vc *vorbis.VorbisCommentsTag, writeVC bool, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	vcData, err := vorbis.EncodeVorbisComments(vc)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeVorbisComments, HasTagType: true, Source: err}}
	}
	if writeVC {
		var hdrB [4]byte
		hdrB[0] = byte(flac.VorbisComment)
		hdrB[1] = byte(len(vcData) >> 16)
		hdrB[2] = byte(len(vcData) >> 8)
		hdrB[3] = byte(len(vcData))
		kept = append(kept, append(hdrB[:], vcData...))
	}
	for _, pic := range pics {
		data := pic.Picture.AsFlacBytes(pic.Info, false)
		var hdrB [4]byte
		hdrB[0] = byte(flac.Picture)
		hdrB[1] = byte(len(data) >> 16)
		hdrB[2] = byte(len(data) >> 8)
		hdrB[3] = byte(len(data))
		kept = append(kept, append(hdrB[:], data...))
	}

	// If the file lacks a PADDING block and padding is preferred, add one last
	if !hasPadding && writeOptions.PreferredPadding > 0 {
		var blocksLen uint64
		for _, b := range kept {
			blocksLen += uint64(len(b))
		}
		oldMetaLen := uint64(metaEnd - metaStart)
		var available uint64
		if oldMetaLen > blocksLen+4 {
			available = oldMetaLen - blocksLen - 4
		}
		padLen := available
		if uint64(writeOptions.PreferredPadding) > padLen {
			padLen = uint64(writeOptions.PreferredPadding)
		}
		if padLen > 0xFFFFFF {
			padLen = 0xFFFFFF
		}
		var hdrB [4]byte
		hdrB[0] = byte(flac.Padding)
		hdrB[1] = byte(padLen >> 16)
		hdrB[2] = byte(padLen >> 8)
		hdrB[3] = byte(padLen)
		kept = append(kept, append(hdrB[:], make([]byte, padLen)...))
	}

	// Assemble the new metadata region
	var metaBuf bytes.Buffer
	for i, block := range kept {
		if i == len(kept)-1 {
			block[0] |= 0x80 // last flag
		}
		metaBuf.Write(block)
	}

	return meta.Splice(file, uint64(metaStart), uint64(metaEnd), metaBuf.Bytes())
}

// readVorbisCommentBlock parses a raw VORBIS_COMMENT block into a tag.
// ensure binary import is used
var _ = binary.LittleEndian

func init() {
	meta.RegisterFormatReader(meta.FileTypeFlac, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := FlacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterFormatWriter(meta.FileTypeFlac, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		f := file.File()
		switch tag.TagType() {
		case meta.TagTypeVorbisComments:
			return flacWriteTag(f, tag, writeOptions)
		case meta.TagTypeID3v2:
			return id3v2.WriteId3v2(f, tag, writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
	meta.RegisterConcreteTagWriter(meta.FileTypeFlac, meta.TagTypeVorbisComments, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
		return flacWriteVorbisComments(file.File(), tag.(*vorbis.VorbisCommentsTag), writeOptions)
	})
	meta.RegisterTagDumper(meta.TagTypeVorbisComments, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		vc := vorbis.VorbisCommentsTagFromTag(tag, "")
		data, err := vorbis.EncodeVorbisComments(vc)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}
