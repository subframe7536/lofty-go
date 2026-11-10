package aac

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/subframe7536/lofty-go/internal/util"
)

// --- AAC ---

// AacProperties are the audio properties of an ADTS AAC file.
type AacProperties struct {
	Version         MpegVersion
	AudioObjectType AudioObjectType
	Duration        time.Duration
	OverallBitrate  uint32
	AudioBitrate    uint32
	SampleRate      uint32
	BitDepth        uint8
	Channels        uint8
	ChannelMask     Option[ChannelMask]
	Copyrighted     bool
	Original        bool
}

// FileProperties converts to generic [FileProperties].
func (p AacProperties) FileProperties() FileProperties {
	return NewFileProperties(
		p.Duration,
		Some(p.OverallBitrate),
		Some(p.AudioBitrate),
		Some(p.SampleRate),
		None[uint8](),
		Some(p.Channels),
		None[ChannelMask](),
	)
}

// AacFile is an ADTS AAC file.
type AacFile struct {
	Properties AacProperties
	ID3v1      Option[*Id3v1Tag]
	ID3v2      Option[*Id3v2Tag]
}

var aacSampleRates = [16]uint32{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
	16000, 12000, 11025, 8000, 7350, 0, 0, 0,
}

// AacFileReadFrom reads an [AacFile] from a reader.
func AacFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*AacFile, error) {
	var file AacFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	var id3v2Tag *Id3v2Tag
	pos, _ := reader.Seek(0, io.SeekCurrent)
	header, _, found2, err := findId3v2(reader, false, parseOptions)
	if err == nil && found2 {
		// Rust quirk: only the tag content size is subtracted, not the
		// 10-byte header — leaving it counted in stream_len.
		streamLen -= uint64(header.Size)
		if parseOptions.ReadTags {
			if _, err := reader.Seek(pos+int64(header.FullTagSize()-header.Size), io.SeekStart); err != nil {
				return nil, &FileParseError{Source: err}
			}
			if t, err := parseId3v2Tag(reader, header, parseOptions); err == nil {
				id3v2Tag = t
			}
		}
	} else {
		if _, err := reader.Seek(pos, io.SeekStart); err != nil {
			return nil, &FileParseError{Source: err}
		}
	}

	newLen, id3v1Tag, _, err := ReadEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen = newLen

	// Scan ADTS frames from just after any ID3v2 tag.
	// Rust: each frame is validated by comparing the next 4 bytes at
	// frame_start + frame_len against this frame's header (masked by
	// 0xFFFE0C00). Invalid candidates are skipped; scanning resumes at
	// the claimed frame boundary.
	p := &file.Properties
	streamStart := pos
	if found2 {
		streamStart = pos + int64(header.FullTagSize())
	}
	var bitrateSum, frameCount uint64
	if parseOptions.ReadProperties {
		searchPos := streamStart
		for {
			abs, ok := aacSearchFrameSync(reader, searchPos)
			if !ok {
				break
			}
			if _, err := reader.Seek(abs, io.SeekStart); err != nil {
				break
			}
			var h [7]byte
			if _, err := io.ReadFull(reader, h[:]); err != nil {
				break
			}
			sfIdx := (h[2] >> 2) & 0xF
			if sfIdx == 15 {
				return nil, &FileParseError{Source: errors.New("invalid AAC sampling frequency index")}
			}
			rate := aacSampleRates[sfIdx]
			frameLen := int64(h[3]&3)<<11 | int64(h[4])<<3 | int64(h[5])>>5
			// cmp_header: next 4 bytes at the claimed boundary
			var nb [4]byte
			if _, err := reader.Seek(abs+frameLen, io.SeekStart); err != nil {
				break
			}
			if _, err := io.ReadFull(reader, nb[:]); err != nil {
				break
			}
			if binary.BigEndian.Uint32(nb[:])&0xFFFE0C00 == binary.BigEndian.Uint32(h[:4])&0xFFFE0C00 {
				if frameCount == 0 {
					if (h[1]>>3)&1 == 0 {
						p.Version = MpegVersion4
					} else {
						p.Version = MpegVersion2
					}
					p.AudioObjectType = AudioObjectType(((h[2] >> 6) & 0b11) + 1)
					p.SampleRate = rate
					p.Channels = ((h[2] & 1) << 2) | (h[3] >> 6)
					p.ChannelMask = ChannelMaskFromMp4Channels(p.Channels)
					p.Original = (h[3]>>5)&1 == 1
					p.Copyrighted = (h[3]>>4)&1 == 1
				}
				// Rust: bitrate = ((len * sample_rate / 1024) * 8) / 1024, averaged
				bitrateSum += uint64((uint32(frameLen) * rate / 1024) * 8 / 1024)
				frameCount++
			}
			searchPos = abs + frameLen
		}
		if frameCount > 0 {
			bitrate := uint32(bitrateSum / frameCount)
			if bitrate > 0 {
				p.AudioBitrate = bitrate
				p.OverallBitrate = bitrate
				p.Duration = time.Duration((streamLen*8)/uint64(bitrate)) * time.Millisecond
			}
		}
	}

	if parseOptions.ReadTags {
		file.ID3v2 = optionFromPtr(id3v2Tag)
		if id3v1Tag != nil {
			file.ID3v1 = Some(id3v1Tag)
		}
	}
	return &file, nil
}

// aacSearchFrameSync scans for an 11-bit frame sync (0xFF followed by a byte
// whose top 3 bits are set) starting at pos, returning the absolute offset.
func aacSearchFrameSync(reader io.ReadSeeker, pos int64) (int64, bool) {
	if _, err := reader.Seek(pos, io.SeekStart); err != nil {
		return 0, false
	}
	offset, found, err := util.ChunkedScan(reader, 1, -1, func(window []byte) int {
		for i := 0; i+1 < len(window); i++ {
			if window[i] == 0xFF && window[i+1]>>5 == 0b111 {
				return i
			}
		}
		return -1
	})
	if err != nil || !found {
		return 0, false
	}
	abs := pos + offset
	if _, err := reader.Seek(abs+2, io.SeekStart); err != nil {
		return 0, false
	}
	return abs, true
}

// FileType implements AudioFile.
func (f *AacFile) FileType() FileType { return FileTypeAac }

// GetProperties implements AudioFile.
func (f *AacFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *AacFile) ContainsTag() bool { return f.ID3v1.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *AacFile) ContainsTagType(t TagType) bool {
	switch t {
	case TagTypeID3v1:
		return f.ID3v1.IsSome()
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *AacFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return NewTaggedFile(FileTypeAac, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *AacFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeID3v2, wo)
}

// SaveFile implements AudioFile.
func (f *AacFile) SaveFile(file FileLike, wo WriteOptions) error {
	return f.TaggedFile().Save(file, wo)
}

// RemoveFromPath implements AudioFile.
func (f *AacFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

func init() {
	RegisterFormatReader(FileTypeAac, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := AacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	RegisterFormatWriter(FileTypeAac, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeID3v2:
			return WriteId3v2(file.File(), tag, writeOptions)
		case TagTypeID3v1:
			return WriteId3v1(file.File(), Id3v1TagFromTag(tag), writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	})
}
