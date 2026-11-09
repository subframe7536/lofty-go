package flac

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	flac "github.com/go-flac/go-flac/v2"
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
	OverallBitrate Option[uint32]
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	// Signature is the MD5 of the unencoded audio data from STREAMINFO.
	Signature [16]byte
}

// FileProperties converts to generic [FileProperties].
func (p FlacProperties) FileProperties() FileProperties {
	return NewFileProperties(
		p.Duration,
		p.OverallBitrate,
		Some(p.AudioBitrate),
		Some(p.SampleRate),
		Some(p.BitDepth),
		Some(p.Channels),
		None[ChannelMask](),
	)
}

// FlacFile is a FLAC file.
type FlacFile struct {
	Properties     FlacProperties
	VorbisComments Option[*VorbisCommentsTag]
	ID3v2          Option[*Tag]
	pictures       []PictureInfoPair
}

// Pictures returns the file's pictures.
func (f *FlacFile) Pictures() []PictureInfoPair { return f.pictures }

// SetPictures replaces the file's pictures.
func (f *FlacFile) SetPictures(pictures []PictureInfoPair) { f.pictures = pictures }

// InsertPicture appends a picture to the file. When `info` is unset, the
// information is extracted from the picture data (PNG/JPEG only).
func (f *FlacFile) InsertPicture(p Picture, info Option[PictureInformation]) error {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = PictureInformationFromPicture(&p)
		if err != nil {
			i = PictureInformation{}
		}
	}
	f.pictures = append(f.pictures, PictureInfoPair{Picture: p, Info: i})
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
func FlacFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*FlacFile, error) {
	var file FlacFile
	fileStart, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - fileStart)

	// Optional leading ID3v2
	if _, err := reader.Seek(fileStart, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}
	var id3v2Tag *Tag
	header, tagEnd, found, err := findId3v2(reader, false, parseOptions)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	if found {
		if parseOptions.ReadTags {
			if _, err := reader.Seek(tagEnd-int64(header.Size), io.SeekStart); err != nil {
				return nil, &FileParseError{Source: err}
			}
			t, err := parseId3v2(reader, header, parseOptions)
			if err != nil {
				return nil, &FileParseError{Source: &TagParseError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
			}
			id3v2Tag = t
		}
	}

	// flac.ParseMetadata consumes the "fLaC" marker itself
	ffile, err := flac.ParseMetadata(reader)
	if err != nil {
		return nil, &FileParseError{Source: &FlacParseError{Source: err}}
	}
	metaEnd, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - metaEnd)

	var vc *VorbisCommentsTag
	var pictures []PictureInfoPair

	for _, block := range ffile.Meta {
		switch block.Type {
		case flac.VorbisComment:
			if parseOptions.ReadTags {
				if vc != nil && parseOptions.ParsingMode == ParsingModeStrict {
					return nil, &FileParseError{Source: &FlacParseError{Message: "FLAC file contains multiple VORBIS_COMMENT blocks"}}
				}
				t, err := ParseVorbisComments(block.Data, parseOptions)
				if err != nil {
					return nil, &FileParseError{Source: &VorbisCommentsParseError{Source: err}}
				}
				vc = t
			}
		case flac.Picture:
			if parseOptions.ReadTags && parseOptions.ReadCoverArt {
				pic, info, err := PictureFromFlacBytes(block.Data, false, parseOptions.ParsingMode)
				if err == nil {
					pictures = append(pictures, PictureInfoPair{Picture: pic, Info: info})
				}
			}
		}
	}

	if parseOptions.ReadProperties {
		si, err := ffile.GetStreamInfo()
		if err != nil {
			return nil, &FileParseError{Source: &FlacParseError{Source: err}}
		}
		p := &file.Properties
		p.SampleRate = uint32(si.SampleRate)
		p.BitDepth = uint8(si.BitDepth)
		p.Channels = uint8(si.ChannelCount)
		copy(p.Signature[:], si.AudioMD5)
		if si.SampleRate > 0 {
			p.Duration = durationFromSeconds(float64(si.SampleCount) / float64(si.SampleRate))
			if secs := float64(si.SampleCount) / float64(si.SampleRate); secs > 0 {
				p.AudioBitrate = uint32(float64(streamLen) * 8 / secs / 1000)
				p.OverallBitrate = Some(uint32(float64(fileLength) * 8 / secs / 1000))
			}
		}
	}

	file.VorbisComments = optionFromPtr(vc)
	file.ID3v2 = optionFromPtr(id3v2Tag)
	file.pictures = pictures
	return &file, nil
}

// FileType implements AudioFile.
func (f *FlacFile) FileType() FileType { return FileTypeFlac }

// GetProperties implements AudioFile.
func (f *FlacFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *FlacFile) ContainsTag() bool {
	return f.VorbisComments.IsSome() || f.ID3v2.IsSome() || len(f.pictures) > 0
}

// ContainsTagType implements AudioFile.
func (f *FlacFile) ContainsTagType(tagType TagType) bool {
	switch tagType {
	case TagTypeVorbisComments:
		return f.VorbisComments.IsSome() || len(f.pictures) > 0
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v2, VorbisComments.
func (f *FlacFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.VorbisComments.Get(); ok {
		g := t.Tag()
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	} else if len(f.pictures) > 0 {
		g := NewTag(TagTypeVorbisComments)
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	}
	return NewTaggedFile(FileTypeFlac, tags, *f.GetProperties())
}

// SavePath saves the file's tags back to the path.
func (f *FlacFile) SavePath(path string, writeOptions WriteOptions) error {
	return SaveToPath(path, f, TagTypeVorbisComments, writeOptions)
}

// SaveFile writes tags to an open file.
func (f *FlacFile) SaveFile(file FileLike, writeOptions WriteOptions) error {
	if t, ok := f.ID3v2.Get(); ok {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
		if err := WriteId3v2(file, t, writeOptions); err != nil {
			return err
		}
	}
	var g *Tag
	if t, ok := f.VorbisComments.Get(); ok {
		g = t.Tag()
	} else if len(f.pictures) == 0 {
		// Rust's write_to is a no-op when the tag is absent and there are no
		// pictures to store; it does not strip the existing VC block.
		return nil
	} else {
		g = NewTag(TagTypeVorbisComments)
	}
	// The Vorbis comments' own pictures chain with the file's pictures,
	// mirroring Rust's `FlacFile::write_to`.
	pics := make([]PictureInfoPair, 0, len(f.pictures)+len(g.Pictures()))
	for _, p := range g.Pictures() {
		info, err := PictureInformationFromPicture(&p)
		if err != nil {
			info = PictureInformation{}
		}
		pics = append(pics, PictureInfoPair{Picture: p, Info: info})
	}
	pics = append(pics, f.pictures...)
	return flacWriteTagPics(file, g, pics, writeOptions)
}

// RemoveFromPath removes a tag type.
func (f *FlacFile) RemoveFromPath(path string, tagType TagType) error {
	return RemoveTagFromPath(path, tagType)
}

// flacWriteTag replaces the VORBIS_COMMENT + PICTURE blocks in a FLAC file,
// preserving all other metadata blocks and the audio frames.
func flacWriteTag(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	pics := make([]PictureInfoPair, 0, len(tag.Pictures()))
	for _, p := range tag.Pictures() {
		info, err := PictureInformationFromPicture(&p)
		if err != nil {
			info = PictureInformation{}
		}
		pics = append(pics, PictureInfoPair{Picture: p, Info: info})
	}
	return flacWriteTagPics(file, tag, pics, writeOptions)
}

// flacWriteTagPics is [flacWriteTag] with explicit [PictureInformation] for
// each picture block.
func flacWriteTagPics(file FileLike, tag *Tag, pics []PictureInfoPair, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	// Skip a leading ID3v2 tag if present
	if _, _, found, err := findId3v2(file, false, writeOptions.ParseOptions); err != nil {
		return &FileEncodingError{Source: err}
	} else if !found {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
	}

	flacStart, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	var marker [4]byte
	if _, err := io.ReadFull(file, marker[:]); err != nil || !bytes.Equal(marker[:], flacPreamble) {
		return &FileEncodingError{Source: &FlacParseError{Source: &FakeTagError{}}}
	}

	// Read the metadata blocks manually so we know their extent
	metaStart, _ := file.Seek(0, io.SeekCurrent)
	var kept [][]byte // raw marshaled blocks to preserve (without last flag)
	hasPadding := false
	var vendor string
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(file, hdr[:]); err != nil {
			return &FileEncodingError{Source: err}
		}
		last := hdr[0]&0x80 != 0
		blockType := hdr[0] & 0x7F
		size := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
		data, err := tryByteSlice(size)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, data); err != nil {
			return &FileEncodingError{Source: err}
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
		return &FileEncodingError{Source: err}
	}

	// Build new tag blocks
	vc := VorbisCommentsTagForWritePics(tag, vendor, false)
	vcData, err := EncodeVorbisComments(vc)
	if err != nil {
		return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeVorbisComments, HasTagType: true, Source: err}}
	}
	if !tag.IsEmpty() {
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
	var meta bytes.Buffer
	for i, block := range kept {
		if i == len(kept)-1 {
			block[0] |= 0x80 // last flag
		}
		meta.Write(block)
	}

	_ = flacStart
	return Splice(file, uint64(metaStart), uint64(metaEnd), meta.Bytes())
}

// readVorbisCommentBlock parses a raw VORBIS_COMMENT block into a tag.
// ensure binary import is used
var _ = binary.LittleEndian

func init() {
	RegisterFormatReader(FileTypeFlac, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := FlacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	RegisterFormatWriter(FileTypeFlac, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
		f := file.File()
		switch tag.TagType() {
		case TagTypeVorbisComments:
			return flacWriteTag(f, tag, writeOptions)
		case TagTypeID3v2:
			return WriteId3v2(f, tag, writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	})
	RegisterTagDumper(TagTypeVorbisComments, func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		vc := VorbisCommentsTagFromTag(tag, "")
		data, err := EncodeVorbisComments(vc)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}
