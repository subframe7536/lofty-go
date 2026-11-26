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

// MpcStreamVersion is the version of a Musepack stream. The zero value is
// stream version 8, like Rust's default.
type MpcStreamVersion uint8

const (
	// MpcStreamVersionSv8 is stream version 8.
	MpcStreamVersionSv8 MpcStreamVersion = iota
	// MpcStreamVersionSv7 is stream version 7.
	MpcStreamVersionSv7
	// MpcStreamVersionSv4to6 is stream versions 4 to 6.
	MpcStreamVersionSv4to6
)

// MpcSv8StreamHeader is the information from an SV8 Stream Header packet
// (Rust `musepack::sv8::StreamHeader`).
type MpcSv8StreamHeader struct {
	// CRC is the CRC 32 of the stream header packet.
	CRC uint32
	// StreamVersion is the bitstream version.
	StreamVersion uint8
	// SampleCount is the number of samples in the stream. 0 = unknown.
	SampleCount uint64
	// BeginningSilence is the number of samples to skip at the start.
	BeginningSilence uint64
	// SampleRate is the mapped sampling frequency in Hz.
	SampleRate uint32
	// MaxUsedBands is the maximum number of bands used in the file.
	MaxUsedBands uint8
	// Channels is the number of channels in the stream.
	Channels uint8
	// MSUsed reports whether Mid Side Stereo is enabled.
	MSUsed bool
	// AudioBlockFrames is the number of frames per audio packet.
	AudioBlockFrames uint16
}

// MpcSv8ReplayGain is the information from an SV8 ReplayGain packet
// (Rust `musepack::sv8::ReplayGain`). Values are dB in Q8.8 format; 0 means
// the field was not computed.
type MpcSv8ReplayGain struct {
	Version   uint8
	TitleGain uint16
	TitlePeak uint16
	AlbumGain uint16
	AlbumPeak uint16
}

// MpcSv8EncoderInfo is the information from an SV8 Encoder Info packet
// (Rust `musepack::sv8::EncoderInfo`).
type MpcSv8EncoderInfo struct {
	// Profile is the quality in 4.3 format.
	Profile float32
	// PNSTool reports whether the Perceptual Noise Substitution tool is on.
	PNSTool bool
	Major   uint8
	// Minor is even for stable versions, odd when unstable.
	Minor uint8
	Build uint8
}

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

	// SV8 packets: the Stream Header and ReplayGain packets (zero when
	// missing in a relaxed parse) and the optional Encoder Info packet.
	StreamHeader MpcSv8StreamHeader
	ReplayGain   MpcSv8ReplayGain
	EncoderInfo  meta.Option[MpcSv8EncoderInfo]
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
	streamVersion MpcStreamVersion
	Properties    MpcProperties
	ID3v1         meta.Option[*id3v1.Id3v1Tag]
	ID3v2         meta.Option[*id3v2.Id3v2Tag]
	Ape           meta.Option[*ape.ApeTag]
}

// StreamVersion returns the version of the Musepack stream, determined from
// the file's magic signature.
func (f *MusepackFile) StreamVersion() MpcStreamVersion { return f.streamVersion }

var mpcSampleRates = [8]uint32{44100, 48000, 37800, 32000, 0, 0, 0, 0}

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
	streamLen := uint64(streamEnd - start) // #nosec G115 -- stream end position is always >= the current in-stream position
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	var id3v2Tag *id3v2.Id3v2Tag
	// ID3v2 at the start (Rust musepack/read.rs: find_id3v2 with read_tags).
	res, err := id3v2.LocateId3v2(reader, parseOptions.ReadTags, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if res != nil {
		// Rust: stream_length.checked_sub(header.full_tag_size())
		fullTagSize := uint64(res.Header.FullTagSize())
		if fullTagSize > streamLen {
			return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
		}
		streamLen -= fullTagSize
		if parseOptions.ReadTags {
			t, err := res.Parse(parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
			}
			id3v2Tag = t
		}
	}
	// Rust: pos_past_id3v2 = reader.stream_position() after the ID3v2 search.
	posPastId3v2, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	// End tags
	newLen, id3v1Tag, apeTag, err := ape.ReadEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen = newLen

	// Read MPC stream info
	if _, err := reader.Seek(posPastId3v2, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	p := &file.Properties
	var sig [3]byte
	if _, err := io.ReadFull(reader, sig[:]); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	isSV8 := false
	if bytes.Equal(sig[:], mpckPreamble[:3]) {
		var sig2 [1]byte
		if _, err := io.ReadFull(reader, sig2[:]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if sig2[0] == 'K' {
			isSV8 = true
		} else {
			// Rust: a non-"MPCK" 4-byte signature falls through to the
			// SV4-6 raw-header path after seeking back. Rewind to the
			// header start and re-read the first 3 bytes into sig.
			if _, err := reader.Seek(-4, io.SeekCurrent); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			if _, err := io.ReadFull(reader, sig[:]); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
		}
	}
	switch {
	case isSV8:
		file.streamVersion = MpcStreamVersionSv8
	case bytes.Equal(sig[:], []byte("MP+")):
		file.streamVersion = MpcStreamVersionSv7
	default:
		file.streamVersion = MpcStreamVersionSv4to6
	}
	if isSV8 {
		p.Version = 8
		// SV8: "MPCK" + SH packet: [2B key "SH"][varint size][data]
		// Scan packets; stream_length accumulates each packet's content
		// length (size field minus key and size bytes) until SE or EOF.
		// Rust gates the whole property parse on read_properties.
		var sv8StreamLen, sampleCount, beginningSilence uint64
		foundSH, foundSE, foundRG := false, false, false
		// Rust's `while let Ok(...)` swallows all PacketReader::next errors —
		// invalid keys, failed varints, and short reads just end the scan.
		for parseOptions.ReadProperties {
			var key [2]byte
			if _, err := io.ReadFull(reader, key[:]); err != nil {
				break
			}
			if key[0] < 'A' || key[0] > 'Z' || key[1] < 'A' || key[1] > 'Z' {
				break
			}
			size, sizeBytes, err := mpcReadSizeCount(reader)
			if err != nil {
				break
			}
			var capacity int64
			if int64(size) > int64(2+sizeBytes) { // #nosec G115 -- size is a varint of at most 9 bytes x 7 bits = 63 bits, so it always fits in int64
				capacity = int64(size) - int64(2+sizeBytes) // #nosec G115 -- size is a varint of at most 9 bytes x 7 bits = 63 bits, so it always fits in int64
			}
			sv8StreamLen += uint64(capacity)
			// Rust reads packet bodies through a reader bounded by the
			// packet's remaining capacity; overruns are errors.
			bounded := &io.LimitedReader{R: reader, N: capacity}
			switch string(key[:]) {
			case "SH":
				// SH body: CRC(4), version(1), sample count(varint),
				// beginning silence(varint), then 2 flag bytes
				var crc [4]byte
				if _, err := io.ReadFull(bounded, crc[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				sh := &p.StreamHeader
				sh.CRC = binary.BigEndian.Uint32(crc[:])
				var ver [1]byte
				if _, err := io.ReadFull(bounded, ver[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				p.Version = ver[0]
				sh.StreamVersion = ver[0]
				if sampleCount, err = mpcReadSize(bounded); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				if beginningSilence, err = mpcReadSize(bounded); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				var bits [2]byte
				if _, err := io.ReadFull(bounded, bits[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				sh.SampleCount = sampleCount
				sh.BeginningSilence = beginningSilence
				sh.SampleRate = mpcSampleRates[(bits[0]>>5)&7]
				sh.MaxUsedBands = (bits[0] & 0x1F) + 1
				sh.Channels = (bits[1] >> 4) + 1
				sh.MSUsed = bits[1]&0x08 != 0
				sh.AudioBlockFrames = 1 << (2 * (bits[1] & 0x07))
				p.SampleRate = sh.SampleRate
				p.Channels = sh.Channels
				foundSH = true
			case "SE":
				foundSE = true
			case "RG":
				// RG body: version(1) + 4 big-endian u16 gain/peak values
				var rg [9]byte
				if _, err := io.ReadFull(bounded, rg[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				p.ReplayGain = MpcSv8ReplayGain{
					Version:   rg[0],
					TitleGain: binary.BigEndian.Uint16(rg[1:]),
					TitlePeak: binary.BigEndian.Uint16(rg[3:]),
					AlbumGain: binary.BigEndian.Uint16(rg[5:]),
					AlbumPeak: binary.BigEndian.Uint16(rg[7:]),
				}
				foundRG = true
			case "EI":
				// EI body: profile/PNS byte + major/minor/build bytes
				var ei [4]byte
				if _, err := io.ReadFull(bounded, ei[:]); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
				p.EncoderInfo = meta.Some(MpcSv8EncoderInfo{
					Profile: float32((ei[0]&0xFE)>>1) / 8.0,
					PNSTool: ei[0]&0x01 == 1,
					Major:   ei[1],
					Minor:   ei[2],
					Build:   ei[3],
				})
			}
			// Rust's PacketReader discards the packet remainder from the current
			// position on the next `next()`; the cursor only ever moves forward.
			if _, err := reader.Seek(bounded.N, io.SeekCurrent); err != nil {
				break
			}
			if foundSE {
				break
			}
		}
		if parseOptions.ReadProperties {
			if !foundSH && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a Stream Header packet"}}
			}
			if !foundRG && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a ReplayGain packet"}}
			}
			if sv8StreamLen == 0 && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing an Audio packet"}}
			}
			if !foundSE && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "file is missing a Stream End packet"}}
			}
			if foundSH {
				if beginningSilence > sampleCount {
					return nil, &meta.FileParseError{Source: &MpcParseError{Message: "beginning silence is greater than the total sample count"}}
				}
				totalSamples := sampleCount - beginningSilence
				if p.SampleRate > 0 && totalSamples > 0 {
					length := util.MulDivRound(totalSamples, 1000, uint64(p.SampleRate))
					p.Duration = util.DurationFromMillis(length)
					bitrate := uint32(util.MulDiv(util.SaturatingMul(sv8StreamLen, 8), uint64(p.SampleRate), util.SaturatingMul(totalSamples, 1000))) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
					p.OverallBitrate = bitrate
					p.AudioBitrate = bitrate
				}
			}
		}
	} else if parseOptions.ReadProperties {
		// SV7: signature is "MP+" with the version in the same u32.
		// Anything else falls back to the SV4-6 raw header.
		// Rust consumes 22 bytes for SV7; SV4-6 needs the full 32.
		var rest [29]byte
		if _, err := io.ReadFull(reader, rest[:22]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		// First 3 bytes were 'M','P','+' or part of the signature dword.
		// SV7 dword: sig "MP+" in low 24 bits, version in high 8
		fullSig := uint32(sig[0]) | uint32(sig[1])<<8 | uint32(sig[2])<<16 | uint32(rest[0])<<24
		if (fullSig & 0xFFFFFF) == 0x2B504D { // "MP+"
			if rest[0]&0x0F != 7 {
				return nil, &meta.FileParseError{Source: &MpcParseError{Message: "expected stream version 7"}}
			}
			p.Version = rest[0] & 0x0F
			frameCount := binary.LittleEndian.Uint32(rest[1:])
			p.FrameCount = frameCount
			hdrFlags := binary.LittleEndian.Uint32(rest[5:])
			byte1 := uint8(hdrFlags >> 24)
			byte2 := uint8(hdrFlags >> 16) // #nosec G115 -- intentional extraction of bits 16-23 from a 32-bit flags word; the discarded upper bits are handled via byte1
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
				p.Duration = util.DurationFromMillis(uint64(length))
				var totalSamples uint64
				if p.TrueGapless {
					totalSamples = uint64(frameCount)*mpcFrameLength - (mpcFrameLength - uint64(p.LastFrameLength))
				} else {
					totalSamples = uint64(frameCount)*mpcFrameLength - mpcDecoderSynthDelay
				}
				if totalSamples > 0 {
					bitrate := uint32(util.MulDiv(util.SaturatingMul(streamLen, 8), uint64(p.SampleRate), totalSamples*1000)) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
					p.OverallBitrate = bitrate
					p.AudioBitrate = bitrate
				}
			}
		} else {
			// SV4-6: the signature dword + rest make up the 8 u32 header
			if _, err := io.ReadFull(reader, rest[22:]); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
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
			// Rust keeps the raw header bitrate when frame_count is 0
			p.OverallBitrate = averageBitrate
			p.AudioBitrate = averageBitrate
			if parseOptions.ReadProperties && p.FrameCount > 0 {
				const mpcFrameLength = 1152
				const mpcDecoderSynthDelay = 481
				samples := uint64(p.FrameCount)*mpcFrameLength - mpcDecoderSynthDelay
				p.Duration = util.DurationFromMillis(util.MulDivRound(samples, 1000, uint64(p.SampleRate)))
				pcmFrames := uint64(p.FrameCount)*mpcFrameLength - 576
				bitrate := util.F64ToU32(float64(streamLen) * 8.0 * float64(p.SampleRate) / float64(pcmFrames) / mpcFrameLength)
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
	return meta.SaveToPath(path, f, wo)
}

// TagTypes implements AudioFile.
func (f *MusepackFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeID3v2, meta.TagTypeID3v1, meta.TagTypeApe)
}

// Conflicts implements AudioFile.
func (f *MusepackFile) Conflicts() [][2]meta.TagType { return nil }

// SaveFile implements AudioFile.
func (f *MusepackFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.ID3v2.Get(); ok {
		if err := t.SaveTo(file, wo); err != nil {
			return err
		}
	}
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
	// Rust computes in f32: (64.82 - gain/100) * 256 + 0.5
	v := (float32(64.82)-float32(gain)/100.0)*256.0 + 0.5
	// Rust `(x) as i16` saturates out-of-range floats before the range
	// check; !(0..i16::MAX).contains(&gain) => 0. Comparing in f32 avoids
	// Go's wrapping int16 conversion.
	if v < 0 || v >= float32(math.MaxInt16) {
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
