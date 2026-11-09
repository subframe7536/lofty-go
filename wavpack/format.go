package wavpack

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

var wvpkPreamble = []byte("wvpk")

// --- WavPack ---

// WavPackProperties are the audio properties of a WavPack file.
type WavPackProperties struct {
	Version        uint16
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	ChannelMask    Option[ChannelMask]
	IsLossless     bool
}

// FileProperties converts to generic [FileProperties].
func (p WavPackProperties) FileProperties() FileProperties {
	return NewFileProperties(
		p.Duration,
		Some(p.OverallBitrate),
		Some(p.AudioBitrate),
		Some(p.SampleRate),
		Some(p.BitDepth),
		Some(p.Channels),
		None[ChannelMask](),
	)
}

var wvSampleRates = [16]uint32{
	6000, 8000, 9600, 11025, 12000, 16000, 22050, 24000,
	32000, 44100, 48000, 64000, 88200, 96000, 192000, 0,
}

// WavPackFile is a WavPack file.
type WavPackFile struct {
	Properties WavPackProperties
	ID3v1      Option[*Tag]
	Ape        Option[*Tag]
}

// WavPackFileReadFrom reads a [WavPackFile] from a reader.
func WavPackFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*WavPackFile, error) {
	var file WavPackFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - start)

	// Check for end tags (ID3v1, Lyrics3, APE)
	newLen, id3v1Tag, apeTag, err := ReadEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen = newLen

	// Check wvpk signature up front (required regardless of options)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return nil, &FileParseError{Source: err}
	}
	if !bytes.Equal(magic[:], wvpkPreamble) {
		return nil, &FileParseError{Source: &FakeTagError{}}
	}

	p := &file.Properties
	if parseOptions.ReadProperties {
		if err := wvReadProperties(reader, streamLen, uint64(streamEnd-start), parseOptions.ParsingMode, p); err != nil {
			return nil, &FileParseError{Source: err}
		}
	}

	if parseOptions.ReadTags {
		if apeTag != nil {
			file.Ape = Some(apeTag.Tag())
		}
		if id3v1Tag != nil {
			file.ID3v1 = Some(id3v1Tag.Tag())
		}
	}
	return &file, nil
}

// FileType implements AudioFile.
func (f *WavPackFile) FileType() FileType { return FileTypeWavPack }

// GetProperties implements AudioFile.
func (f *WavPackFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *WavPackFile) ContainsTag() bool { return f.ID3v1.IsSome() || f.Ape.IsSome() }

// ContainsTagType implements AudioFile.
func (f *WavPackFile) ContainsTagType(t TagType) bool {
	switch t {
	case TagTypeID3v1:
		return f.ID3v1.IsSome()
	case TagTypeApe:
		return f.Ape.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *WavPackFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeWavPack, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *WavPackFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeApe, wo)
}

// SaveFile implements AudioFile.
func (f *WavPackFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t)
	}
	return SaveTagFields(file, FileTypeWavPack, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *WavPackFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

const (
	wvFlagMono              = 0x0004
	wvFlagHybridCompression = 0x0008
	wvFlagInitialBlock      = 0x0800
	wvFlagFinalBlock        = 0x1000
	wvFlagDSD               = 0x80000000
)

// WavPack metadata block IDs.
const (
	wvIDMultichannel          = 0x0D
	wvIDDSD                   = 0x0E
	wvIDNonStandardSampleRate = 0x27
	wvIDFlagOddSize           = 0x40
	wvIDFlagLargeSize         = 0x80
	wvMetadataMask            = 0x3F
)

// WavPackParseError is returned when a WavPack file fails to parse.
type WavPackParseError struct {
	Message string
	Source  error
}

func (e *WavPackParseError) Error() string {
	if e.Message != "" {
		return "wavpack: " + e.Message
	}
	return "failed to parse WavPack file"
}

func (e *WavPackParseError) Unwrap() error { return e.Source }

// wvExtendedMetaInfo parses WavPack metadata blocks for the real sample rate
// (non-standard and DSD rates) and channel information.
// wvReadProperties iterates WavPack blocks, mirroring Rust's
// wavpack::properties::read_properties.
func wvReadProperties(reader io.ReadSeeker, streamLen, fileLen uint64, mode ParsingMode, p *WavPackProperties) error {
	strict := mode == ParsingModeStrict
	var offset uint64
	var totalSamples uint32
	for {
		if _, err := reader.Seek(int64(offset), io.SeekStart); err != nil {
			return err
		}
		var hdr [32]byte
		if _, err := io.ReadFull(reader, hdr[:]); err != nil {
			if strict {
				return &WavPackParseError{Source: err}
			}
			break
		}
		if !bytes.Equal(hdr[:4], wvpkPreamble) {
			if strict {
				return &WavPackParseError{Message: "missing wvpk preamble"}
			}
			break
		}
		blockSize := binary.LittleEndian.Uint32(hdr[4:])
		if blockSize < 24 || blockSize > 1048576 {
			if strict {
				return &WavPackParseError{Message: "invalid block size"}
			}
			break
		}
		version := binary.LittleEndian.Uint16(hdr[8:])
		total := binary.LittleEndian.Uint32(hdr[12:])
		samples := binary.LittleEndian.Uint32(hdr[20:])
		flags := binary.LittleEndian.Uint32(hdr[24:])

		rateIdx := (flags >> 23) & 0xF
		p.SampleRate = wvSampleRates[rateIdx]

		// Non-standard rates and DSD audio store the real rate in metadata blocks
		if rateIdx == 15 || flags&wvFlagDSD != 0 {
			contents, err := tryByteSlice(int(blockSize) - 24)
			if err != nil {
				return err
			}
			if _, err := io.ReadFull(reader, contents); err != nil {
				if strict {
					return &WavPackParseError{Source: err}
				}
				break
			}
			if err := wvExtendedMetaInfo(contents, p, mode); err != nil {
				if strict {
					return err
				}
				break
			}
			if rateIdx == 15 && p.SampleRate == 0 {
				if strict {
					return &WavPackParseError{Message: "expected custom sample rate"}
				}
				break
			}
		}

		if flags&wvFlagInitialBlock != 0 {
			if version < 0x402 || version > 0x410 {
				if strict {
					return &WavPackParseError{Message: "unsupported stream version encountered"}
				}
				break
			}
			totalSamples = total
			p.BitDepth = uint8(((flags&3)+1)*8 - ((flags >> 13) & 0x1F))
			p.Version = version
			p.IsLossless = flags&wvFlagHybridCompression == 0
			if flags&wvFlagFinalBlock != 0 {
				if flags&wvFlagMono != 0 {
					p.Channels = 1
					p.ChannelMask = Some(ChannelMaskMono)
				} else {
					p.Channels = 2
					p.ChannelMask = Some(ChannelMaskStereo)
				}
			}
		}

		// Skip blocks with no samples
		if samples == 0 {
			offset += uint64(blockSize) + 8
			continue
		}
		if flags&wvFlagFinalBlock != 0 {
			break
		}
		offset += uint64(blockSize) + 8
	}

	// Unknown sample counts are unsupported
	if totalSamples == 0xFFFFFFFF {
		return nil
	}
	if totalSamples == 0 || p.SampleRate == 0 {
		if strict {
			return &WavPackParseError{Message: "unable to calculate duration (sample count == 0 || sample rate == 0)"}
		}
		return nil
	}

	length := float64(totalSamples) * 1000 / float64(p.SampleRate)
	p.Duration = time.Duration(uint64(length+0.5)) * time.Millisecond
	p.AudioBitrate = uint32(float64(streamLen)*8/length + 0.5)
	p.OverallBitrate = uint32(float64(fileLen)*8/length + 0.5)
	return nil
}

func wvExtendedMetaInfo(content []byte, p *WavPackProperties, mode ParsingMode) error {
	for len(content) >= 2 {
		id := content[0]
		size := int(content[1]) << 1
		content = content[2:]
		if id&wvIDFlagLargeSize != 0 {
			if len(content) < 2 {
				break
			}
			size += int(content[0]) << 9
			size += int(content[1]) << 17
			content = content[2:]
		}
		if size == 0 {
			continue
		}
		if size > len(content) {
			return &WavPackParseError{Message: "metadata block size exceeds remaining content"}
		}
		if id&wvIDFlagOddSize != 0 {
			size--
		}
		switch id & wvMetadataMask {
		case wvIDNonStandardSampleRate:
			if size < 3 {
				return &WavPackParseError{Message: "invalid block size for non-standard sample rate"}
			}
			p.SampleRate = uint32(content[0]) | uint32(content[1])<<8 | uint32(content[2])<<16
		case wvIDDSD:
			if size <= 1 {
				return &WavPackParseError{Message: "invalid DSD block size"}
			}
			multiplier := content[0]
			if multiplier > 30 {
				if mode == ParsingModeStrict {
					return &WavPackParseError{Message: "invalid sample rate multiplier"}
				}
				return nil
			}
			p.SampleRate *= 1 << multiplier
		case wvIDMultichannel:
			if size <= 1 {
				return &WavPackParseError{Message: "unable to extract channel information"}
			}
			channels := uint32(content[0])
			maskBytes := content[1 : 1+min(size-1, 5)]
			var mask uint32
			for i, b := range maskBytes {
				if i >= 4 {
					break
				}
				mask |= uint32(b) << (8 * i)
			}
			// High channel counts encode extra bits after the mask
			if size >= 5 {
				channels |= uint32(content[size-1]&0xF) << 8
				channels++
			}
			if channels > 0xFF {
				p.Channels = 0xFF
			} else {
				p.Channels = uint8(channels)
			}
			if mask != 0 {
				p.ChannelMask = Some(ChannelMask(mask))
			}
		}
		if size > len(content) {
			return &WavPackParseError{Message: "metadata block size exceeds remaining content"}
		}
		content = content[size:]
		if id&wvIDFlagOddSize != 0 && len(content) > 0 {
			content = content[1:]
		}
	}
	return nil
}

// channelMaskFromDsfChannelType maps a DSF channel type to a ChannelMask.
func init() {
	RegisterFormatReader(FileTypeWavPack, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := WavPackFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	RegisterFormatWriter(FileTypeWavPack, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeApe:
			return ApeWriteTo(file.File(), ApeTagFromTag(tag), writeOptions)
		case TagTypeID3v1:
			return WriteId3v1(file.File(), Id3v1TagFromTag(tag), writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	})
}
