package wavpack

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	"github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/id3v1"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
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
	// Channels is a u16 since WavPack supports "unlimited" streams;
	// [WavPackProperties.FileProperties] truncates it to u8 like Rust.
	Channels    uint16
	ChannelMask meta.Option[meta.ChannelMask]
	IsLossless  bool
}

// FileProperties converts to generic [FileProperties].
func (p WavPackProperties) FileProperties() meta.FileProperties {
	// Rust's `From<WavPackProperties> for FileProperties` maps a zero mask to
	// `None`, so a later zero-mask multichannel block clears the earlier one.
	channelMask := p.ChannelMask
	if m, ok := channelMask.Get(); ok && m == 0 {
		channelMask = meta.None[meta.ChannelMask]()
	}
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(p.BitDepth),
		meta.Some(uint8(p.Channels)),
		channelMask,
	)
}

var wvSampleRates = [16]uint32{
	6000, 8000, 9600, 11025, 12000, 16000, 22050, 24000,
	32000, 44100, 48000, 64000, 88200, 96000, 192000, 0,
}

// WavPackFile is a WavPack file.
type WavPackFile struct {
	Properties WavPackProperties
	ID3v1      meta.Option[*id3v1.Id3v1Tag]
	Ape        meta.Option[*ape.ApeTag]
}

// WavPackFileReadFrom reads a [WavPackFile] from a reader.
func WavPackFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*WavPackFile, error) {
	var file WavPackFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - start)

	// Check for end tags (ID3v1, APE)
	newLen, id3v1Tag, apeTag, err := ape.ReadEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen = newLen

	p := &file.Properties
	if parseOptions.ReadProperties {
		if err := wvReadProperties(reader, streamLen, uint64(streamEnd-start), parseOptions.ParsingMode, p); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
	}

	if parseOptions.ReadTags {
		if apeTag != nil {
			file.Ape = meta.Some(apeTag)
		}
		if id3v1Tag != nil {
			file.ID3v1 = meta.Some(id3v1Tag)
		}
	}
	return &file, nil
}

// FileType implements AudioFile.
func (f *WavPackFile) FileType() meta.FileType { return meta.FileTypeWavPack }

// GetProperties implements AudioFile.
func (f *WavPackFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *WavPackFile) ContainsTag() bool { return f.ID3v1.IsSome() || f.Ape.IsSome() }

// ContainsTagType implements AudioFile.
func (f *WavPackFile) ContainsTagType(t meta.TagType) bool {
	switch t {
	case meta.TagTypeID3v1:
		return f.ID3v1.IsSome()
	case meta.TagTypeApe:
		return f.Ape.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *WavPackFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeWavPack, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *WavPackFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

// TagTypes implements AudioFile.
func (f *WavPackFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeID3v1, meta.TagTypeApe)
}

// Conflicts implements AudioFile.
func (f *WavPackFile) Conflicts() [][2]meta.TagType { return nil }

// SaveFile implements AudioFile.
func (f *WavPackFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.ID3v1.Get(); ok {
		if err := t.SaveTo(file, wo); err != nil {
			return err
		}
	}
	if t, ok := f.Ape.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

// RemoveFromPath implements AudioFile.
func (f *WavPackFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
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
func wvReadProperties(reader io.ReadSeeker, streamLen, fileLen uint64, mode meta.ParsingMode, p *WavPackProperties) error {
	strict := mode == meta.ParsingModeStrict
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
			contents, err := util.TryByteSlice(int(blockSize) - 24)
			if err != nil {
				return err
			}
			// Rust propagates the block_contents read error unconditionally
			if _, err := io.ReadFull(reader, contents); err != nil {
				return &WavPackParseError{Source: err}
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
			// Rust uses saturating_sub for the bit-depth shift
			bitDepth := ((flags & 3) + 1) * 8
			if shift := (flags >> 13) & 0x1F; shift < bitDepth {
				bitDepth -= shift
			} else {
				bitDepth = 0
			}
			p.BitDepth = uint8(bitDepth)
			p.Version = version
			p.IsLossless = flags&wvFlagHybridCompression == 0
			if flags&wvFlagFinalBlock != 0 {
				if flags&wvFlagMono != 0 {
					p.Channels = 1
					p.ChannelMask = meta.Some(meta.ChannelMaskMono)
				} else {
					p.Channels = 2
					p.ChannelMask = meta.Some(meta.ChannelMaskStereo)
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
	p.Duration = util.DurationFromMillis(uint64(length + 0.5))
	// Rust's saturating float-to-int `as` casts.
	p.AudioBitrate = util.F64ToU32(float64(streamLen)*8/length + 0.5)
	p.OverallBitrate = util.F64ToU32(float64(fileLen)*8/length + 0.5)
	return nil
}

func wvExtendedMetaInfo(content []byte, p *WavPackProperties, mode meta.ParsingMode) error {
	pos := 0
	for len(content)-pos >= 2 {
		id := content[pos]
		size := int(content[pos+1]) << 1
		pos += 2
		if id&wvIDFlagLargeSize != 0 {
			// Rust's `read_u8()?` propagates the short-read error
			if len(content)-pos < 2 {
				return &WavPackParseError{Source: io.ErrUnexpectedEOF}
			}
			size += int(content[pos]) << 9
			size += int(content[pos+1]) << 17
			pos += 2
		}
		if size == 0 {
			continue
		}
		if size > len(content)-pos {
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
			p.SampleRate = uint32(content[pos]) | uint32(content[pos+1])<<8 | uint32(content[pos+2])<<16
			pos += 3
			size -= 3
		case wvIDDSD:
			if size <= 1 {
				return &WavPackParseError{Message: "invalid DSD block size"}
			}
			multiplier := content[pos]
			pos++
			size--
			if multiplier > 30 {
				if mode == meta.ParsingModeStrict {
					return &WavPackParseError{Message: "invalid sample rate multiplier"}
				}
				return nil
			}
			p.SampleRate *= 1 << multiplier
		case wvIDMultichannel:
			if size <= 1 {
				return &WavPackParseError{Message: "unable to extract channel information"}
			}
			// Rust keys off s = size - 2 (block size minus id and channel
			// bytes); the mask layout depends on s.
			channels := uint16(content[pos])
			pos++
			var mask uint32
			s := size - 2
			switch s {
			case 0:
				mask = uint32(content[pos])
				pos++
				size -= 1
			case 1:
				mask = uint32(binary.LittleEndian.Uint16(content[pos:]))
				pos += 2
				size -= 2
			case 2:
				mask = uint32(content[pos]) | uint32(content[pos+1])<<8 | uint32(content[pos+2])<<16
				pos += 3
				size -= 3
			case 3:
				mask = binary.LittleEndian.Uint32(content[pos:])
				pos += 4
				size -= 4
			case 4:
				channels |= uint16(content[pos]&0xF) << 8
				channels++
				pos++
				mask = uint32(content[pos]) | uint32(content[pos+1])<<8 | uint32(content[pos+2])<<16
				pos += 3
				size -= 4
			case 5:
				channels |= uint16(content[pos]&0xF) << 8
				channels++
				pos++
				mask = binary.LittleEndian.Uint32(content[pos:])
				pos += 4
				size -= 5
			default:
				return &WavPackParseError{Message: "encountered invalid channel info size"}
			}
			// Rust stores channels as u16; FileProperties truncates to u8, and
			// assigns the mask unconditionally (a zero mask clears it).
			p.Channels = channels
			p.ChannelMask = meta.Some(meta.ChannelMask(mask))
		}
		if size > len(content)-pos {
			return &WavPackParseError{Message: "metadata block size exceeds remaining content"}
		}
		pos += size
		if id&wvIDFlagOddSize != 0 {
			// Rust's `read_u8()?` propagates the missing-pad error
			if len(content)-pos < 1 {
				return &WavPackParseError{Source: io.ErrUnexpectedEOF}
			}
			pos++
		}
	}
	return nil
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeWavPack, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := WavPackFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	meta.RegisterFormatWriter(meta.FileTypeWavPack, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		switch tag.TagType() {
		case meta.TagTypeApe:
			return ape.ApeWriteTo(file.File(), ape.ApeTagFromTag(tag), writeOptions)
		case meta.TagTypeID3v1:
			return id3v1.WriteId3v1(file.File(), id3v1.Id3v1TagFromTag(tag), writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
}
