package mpc

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"time"

	"github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

var mpckPreamble = []byte("MPCK")

// --- Musepack ---

// MpcProfile is the SV7 encoding profile.
// http://trac.musepack.net/musepack/wiki/SV7Specification
type MpcProfile uint8

const (
	MpcProfileNone MpcProfile = iota
	MpcProfileUnstable
	MpcProfileUnused
	MpcProfileBelowTelephone0
	MpcProfileBelowTelephone1
	MpcProfileTelephone
	MpcProfileThumb
	MpcProfileRadio
	MpcProfileStandard
	MpcProfileXtreme
	MpcProfileInsane
	MpcProfileBrainDead
	MpcProfileAboveBrainDead9
	MpcProfileAboveBrainDead10
)

// MpcProfileFromU8 maps a raw SV7 profile nibble to a [MpcProfile].
func MpcProfileFromU8(value uint8) MpcProfile {
	switch value {
	case 1:
		return MpcProfileUnstable
	case 2, 3, 4:
		return MpcProfileUnused
	case 5:
		return MpcProfileBelowTelephone0
	case 6:
		return MpcProfileBelowTelephone1
	case 7:
		return MpcProfileTelephone
	case 8:
		return MpcProfileThumb
	case 9:
		return MpcProfileRadio
	case 10:
		return MpcProfileStandard
	case 11:
		return MpcProfileXtreme
	case 12:
		return MpcProfileInsane
	case 13:
		return MpcProfileBrainDead
	case 14:
		return MpcProfileAboveBrainDead9
	case 15:
		return MpcProfileAboveBrainDead10
	default:
		return MpcProfileNone
	}
}

// MpcLink describes the volume profile at a title's start and end.
type MpcLink uint8

const (
	MpcLinkVeryLowStartOrEnd MpcLink = iota
	MpcLinkLoudEnd
	MpcLinkLoudStart
	MpcLinkLoudStartAndEnd
)

// MpcProperties are the audio properties of a Musepack file.
type MpcProperties struct {
	Version        uint8
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8

	// SV7 header fields
	FrameCount      uint32
	IntensityStereo bool
	MidSideStereo   bool
	MaxBand         uint8
	Profile         MpcProfile
	Link            MpcLink
	MaxLevel        uint16
	TitleGain       int16
	TitlePeak       uint16
	AlbumGain       int16
	AlbumPeak       uint16
	TrueGapless     bool
	LastFrameLength uint16
	FastSeekingSafe bool
	EncoderVersion  uint8
}

// FileProperties converts to generic [FileProperties].
func (p MpcProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.None[uint8](),
		meta.Some(p.Channels),
		meta.None[meta.ChannelMask](),
	)
}

// MusepackFile is a Musepack file.
type MusepackFile struct {
	Properties MpcProperties
	ID3v1      meta.Option[*id3v1.Id3v1Tag]
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
	Ape        meta.Option[*ape.ApeTag]
}

var mpcSampleRates = [4]uint32{44100, 48000, 37800, 32000}

// MusepackFileReadFrom reads a [MusepackFile] from a reader.
func MusepackFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*MusepackFile, error) {
	var file MusepackFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	var id3v2Tag *id3v2.Id3v2Tag
	// ID3v2 at the start
	header, _, found2, err := id3v2.FindId3v2(reader, parseOptions.ReadTags, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	var id3v2Size uint64
	if found2 {
		id3v2Size = uint64(header.FullTagSize())
		if newLen := streamLen - id3v2Size; newLen < streamLen {
			streamLen = newLen
		}
		if parseOptions.ReadTags {
			if _, err := reader.Seek(start+int64(header.FullTagSize()-header.Size), io.SeekStart); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			if t, err := id3v2.ParseId3v2Tag(reader, header, parseOptions); err == nil {
				id3v2Tag = t
			}
		}
	}

	// End tags
	newLen, id3v1Tag, apeTag, err := ape.ReadEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen = newLen

	// Read MPC stream info
	if _, err := reader.Seek(start+int64(id3v2Size), io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	p := &file.Properties
	var sig [3]byte
	if _, err := io.ReadFull(reader, sig[:]); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if bytes.Equal(sig[:], mpckPreamble[:3]) {
		// SV8: "MPCK" + SH packet: [2B key "SH"][varint size][data]
		var sig2 [1]byte
		if _, err := io.ReadFull(reader, sig2[:]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if sig2[0] != 'K' {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
		}
		p.Version = 8
		// Scan packets; stream_length accumulates each packet's content
		// length (size field minus key and size bytes) until SE or EOF.
		var sv8StreamLen, sampleCount, beginningSilence uint64
		foundSH, foundSE, foundRG := false, false, false
		for {
			pktStart, err := reader.Seek(0, io.SeekCurrent)
			if err != nil {
				break
			}
			var key [2]byte
			if _, err := io.ReadFull(reader, key[:]); err != nil {
				break
			}
			// Rust: packet keys are two uppercase ASCII bytes; anything else
			// ends the packet scan (erroring in strict mode).
			if key[0] < 'A' || key[0] > 'Z' || key[1] < 'A' || key[1] > 'Z' {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &meta.FileParseError{Source: &MpcParseError{Message: "invalid packet key"}}
				}
				break
			}
			size, sizeBytes, err := mpcReadSizeCount(reader)
			if err != nil {
				break
			}
			var capacity int64
			if int64(size) > int64(2+sizeBytes) {
				capacity = int64(size) - int64(2+sizeBytes)
			}
			sv8StreamLen += uint64(capacity)
			switch string(key[:]) {
			case "SH":
				// SH body: CRC(4), version(1), sample count(varint),
				// beginning silence(varint), then 2 flag bytes
				var crc [4]byte
				if _, err := io.ReadFull(reader, crc[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				var ver [1]byte
				if _, err := io.ReadFull(reader, ver[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				if sampleCount, err = mpcReadSize(reader); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				if beginningSilence, err = mpcReadSize(reader); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				var bits [2]byte
				if _, err := io.ReadFull(reader, bits[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				p.SampleRate = mpcSampleRates[(int(bits[0])>>5)&7]
				p.Channels = uint8((int(bits[1]) >> 4) + 1)
				foundSH = true
			case "SE":
				foundSE = true
			case "RG":
				foundRG = true
			}
			if _, err := reader.Seek(pktStart+int64(size), io.SeekStart); err != nil {
				break
			}
			if foundSE {
				break
			}
		}
		if !foundSH {
			if parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a Stream Header packet"}}
			}
		} else if parseOptions.ReadProperties {
			if beginningSilence > sampleCount {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "beginning silence is greater than the total sample count"}}
			}
			totalSamples := sampleCount - beginningSilence
			if p.SampleRate > 0 && totalSamples > 0 {
				length := util.DivRound(totalSamples*1000, uint64(p.SampleRate))
				p.Duration = time.Duration(length) * time.Millisecond
				bitrate := uint32(sv8StreamLen * 8 * uint64(p.SampleRate) / (totalSamples * 1000))
				p.OverallBitrate = bitrate
				p.AudioBitrate = bitrate
			}
		}
		if sv8StreamLen == 0 && parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing an Audio packet"}}
		}
		if !foundSE && parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a Stream End packet"}}
		}
		if !foundRG && parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a ReplayGain packet"}}
		}
	} else {
		// SV7: signature is "MP+" with the version in the same u32.
		// Anything else falls back to the SV4-6 raw header.
		var rest [29]byte
		if _, err := io.ReadFull(reader, rest[:]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		// First 3 bytes were 'M','P','+' or part of the signature dword.
		// SV7 dword: sig "MP+" in low 24 bits, version in high 8
		fullSig := uint32(sig[0]) | uint32(sig[1])<<8 | uint32(sig[2])<<16 | uint32(rest[0])<<24
		if (fullSig & 0xFFFFFF) == 0x2B504D { // "MP+"
			p.Version = uint8((fullSig >> 24) & 0xFF)
			frameCount := binary.LittleEndian.Uint32(rest[1:])
			p.FrameCount = frameCount
			hdrFlags := binary.LittleEndian.Uint32(rest[5:])
			byte1 := uint8(hdrFlags >> 24)
			byte2 := uint8(hdrFlags >> 16)
			p.IntensityStereo = byte1&0x80 != 0
			p.MidSideStereo = byte1&0x40 != 0
			p.MaxBand = byte1 & 0x3F
			p.Profile = MpcProfileFromU8((byte2 & 0xF0) >> 4)
			p.Link = MpcLink((byte2 & 0x0C) >> 2)
			sfIdx := (hdrFlags >> 16) & 3
			p.SampleRate = mpcSampleRates[sfIdx]
			p.MaxLevel = uint16(hdrFlags & 0xFFFF)
			titlePeak := binary.LittleEndian.Uint16(rest[9:])
			titleGain := binary.LittleEndian.Uint16(rest[11:])
			albumPeak := binary.LittleEndian.Uint16(rest[13:])
			albumGain := binary.LittleEndian.Uint16(rest[15:])
			chunk5 := binary.LittleEndian.Uint32(rest[17:])
			p.TrueGapless = chunk5>>31 == 1
			if p.TrueGapless {
				p.LastFrameLength = uint16((chunk5 >> 20) & 0x7FF)
				if p.LastFrameLength > 1152 {
					return nil, &meta.FileParseError{Source: &meta.TooMuchDataError{}}
				}
			}
			p.FastSeekingSafe = (chunk5>>19)&1 == 1
			p.EncoderVersion = rest[21]
			p.TitleGain = mpcReplayGain(titleGain)
			p.TitlePeak = mpcReplayPeak(titlePeak)
			p.AlbumGain = mpcReplayGain(albumGain)
			p.AlbumPeak = mpcReplayPeak(albumPeak)
			p.Channels = 2
			if parseOptions.ReadProperties && p.SampleRate > 0 && frameCount > 0 {
				const mpcFrameLength = 1152
				const mpcDecoderSynthDelay = 481
				length := float64(frameCount) * (float64(mpcFrameLength) / float64(p.SampleRate)) * 1000.0
				p.Duration = time.Duration(length) * time.Millisecond
				var totalSamples uint64
				if p.TrueGapless {
					totalSamples = uint64(frameCount)*mpcFrameLength - (mpcFrameLength - uint64(p.LastFrameLength))
				} else {
					totalSamples = uint64(frameCount)*mpcFrameLength - mpcDecoderSynthDelay
				}
				if totalSamples > 0 {
					bitrate := uint32(streamLen * 8 * uint64(p.SampleRate) / (totalSamples * 1000))
					p.OverallBitrate = bitrate
					p.AudioBitrate = bitrate
				}
			}
		} else {
			// SV4-6: the signature dword + rest make up the 8 u32 header
			var headerData [8]uint32
			headerData[0] = fullSig
			for i := 1; i < 8; i++ {
				headerData[i] = binary.LittleEndian.Uint32(rest[i*4-3:])
			}
			averageBitrate := (headerData[0] >> 23) & 0x1FF
			p.IntensityStereo = (headerData[0]>>22)&1 == 1
			p.MidSideStereo = (headerData[0]>>21)&1 == 1
			version := uint16((headerData[0] >> 11) & 0x3FF)
			if version < 4 || version > 6 {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "invalid stream version encountered"}}
			}
			p.Version = uint8(version)
			p.MaxBand = uint8((headerData[0] >> 6) & 0x1F)
			blockSize := headerData[0] & 0x3F
			if version >= 5 {
				p.FrameCount = headerData[1]
			} else {
				p.FrameCount = headerData[1] >> 16
			}
			if parseOptions.ParsingMode == meta.ParsingModeStrict {
				if averageBitrate != 0 {
					return nil, &meta.FileParseError{Source: &MpcParseError{Message: "encountered CBR stream"}}
				}
				if p.IntensityStereo {
					return nil, &meta.FileParseError{Source: &MpcParseError{Message: "stream uses intensity stereo coding"}}
				}
				if blockSize != 1 {
					return nil, &meta.FileParseError{Source: &MpcParseError{Message: "stream has an invalid block size (must be 1)"}}
				}
			}
			if version < 6 && p.FrameCount > 0 {
				p.FrameCount--
			}
			p.SampleRate = 44100
			p.Channels = 2
			if parseOptions.ReadProperties && p.FrameCount > 0 {
				const mpcFrameLength = 1152
				const mpcDecoderSynthDelay = 481
				samples := uint64(p.FrameCount)*mpcFrameLength - mpcDecoderSynthDelay
				p.Duration = time.Duration(util.DivRound(samples*1000, uint64(p.SampleRate))) * time.Millisecond
				pcmFrames := uint64(p.FrameCount)*mpcFrameLength - 576
				bitrate := uint32(float64(streamLen) * 8.0 * float64(p.SampleRate) / float64(pcmFrames) / mpcFrameLength)
				p.OverallBitrate = bitrate
				p.AudioBitrate = bitrate
			}
		}
	}

	if parseOptions.ReadTags {
		file.ID3v2 = util.OptionFromPtr(id3v2Tag)
		if apeTag != nil {
			file.Ape = meta.Some(apeTag)
		}
		if id3v1Tag != nil {
			file.ID3v1 = meta.Some(id3v1Tag)
		}
	}
	return &file, nil
}

// mpcReadSize reads an SV8 "magnitude" variable-length integer.
func mpcReadSize(reader io.Reader) (uint64, error) {
	n, _, err := mpcReadSizeCount(reader)
	return n, err
}

// mpcReadSizeCount reads an SV8 magnitude varint, also returning the byte count.
func mpcReadSizeCount(reader io.Reader) (uint64, uint8, error) {
	var n uint64
	var count uint8
	for {
		var b [1]byte
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return 0, count, err
		}
		count++
		if count > 9 {
			return 0, count, &meta.TooMuchDataError{}
		}
		n = (n << 7) | uint64(b[0]&0x7F)
		if b[0]&0x80 == 0 {
			break
		}
	}
	return n, count, nil
}

// FileType implements AudioFile.
func (f *MusepackFile) FileType() meta.FileType { return meta.FileTypeMpc }

// GetProperties implements AudioFile.
func (f *MusepackFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *MusepackFile) ContainsTag() bool {
	return f.ID3v1.IsSome() || f.ID3v2.IsSome() || f.Ape.IsSome()
}

// ContainsTagType implements AudioFile.
func (f *MusepackFile) ContainsTagType(t meta.TagType) bool {
	switch t {
	case meta.TagTypeID3v1:
		return f.ID3v1.IsSome()
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	case meta.TagTypeApe:
		return f.Ape.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v2, ID3v1, Ape.
func (f *MusepackFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeMpc, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *MusepackFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, meta.TagTypeApe, wo)
}

// SaveFile implements AudioFile.
func (f *MusepackFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	var tags []*meta.Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.SaveTagFields(file, meta.FileTypeMpc, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *MusepackFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

type MpcParseError struct {
	Message string
	Source  error
}

func (e *MpcParseError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Source != nil {
		return e.Source.Error()
	}
	return "musepack parse error"
}

func (e *MpcParseError) Unwrap() error { return e.Source }

func mpcReplayGain(gain uint16) int16 {
	if gain == 0 {
		return 0
	}
	v := (64.82-float64(gain)/100.0)*256.0 + 0.5
	// Rust `(x) as i16` saturates out-of-range floats before the range
	// check; !(0..i16::MAX).contains(&gain) => 0. Comparing in f64 avoids
	// Go's wrapping int16 conversion.
	if v < 0 || v >= float64(math.MaxInt16) {
		return 0
	}
	return int16(v)
}

// mpcReplayPeak converts a raw SV7 peak value to a log-scale peak.
func mpcReplayPeak(peak uint16) uint16 {
	if peak == 0 {
		return 0
	}
	return uint16(math.Log10(float64(peak))*20.0*256.0 + 0.5)
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeMpc, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := MusepackFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	meta.RegisterFormatWriter(meta.FileTypeMpc, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		switch tag.TagType() {
		case meta.TagTypeApe:
			return ape.ApeWriteTo(file.File(), ape.ApeTagFromTag(tag), writeOptions)
		case meta.TagTypeID3v1:
			return id3v1.WriteId3v1(file.File(), id3v1.Id3v1TagFromTag(tag), writeOptions)
		case meta.TagTypeID3v2:
			return id3v2.WriteId3v2(file.File(), tag, writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
}
