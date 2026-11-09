package lofty

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"time"
)

var dsdPreamble = []byte("DSD ")
var dsdFmtPreamble = []byte("fmt ")
var dsdDataPreamble = []byte("data")
var wvpkPreamble = []byte("wvpk")
var mpckPreamble = []byte("MPCK")

var dsfSampleRateIdx = [16]uint32{2822400, 5644800, 0}

// readEndTags scans the end of a stream for ID3v1, Lyrics3, and APE tags.
// Returns the adjusted stream length and any tags found.
func readEndTags(reader io.ReadSeeker, start int64, streamLen uint64, parseOptions ParseOptions) (uint64, *Id3v1Tag, *ApeTag, error) {
	id3v1, _, _, foundI, err := findId3v1(reader, parseOptions.ReadTags, parseOptions.ParsingMode)
	if err != nil {
		return 0, nil, nil, err
	}
	if foundI {
		streamLen -= 128
	}
	if _, _, _, _, foundL, err := findLyrics3(reader, false, parseOptions); err == nil && foundL {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		if pos-start < int64(streamLen) {
			streamLen = uint64(pos - start)
		}
	}
	var apeTag *ApeTag
	endPos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, nil, nil, err
	}
	if _, err := reader.Seek(endPos-32, io.SeekStart); err == nil {
		tag, apeHeader, err := readApeTag(reader, true, parseOptions)
		if err != nil {
			return 0, nil, nil, &TagParseError{Source: err}
		}
		if apeHeader != nil {
			if parseOptions.ReadTags {
				apeTag = tag
			}
			if newLen := streamLen - uint64(apeHeader.Size); newLen < streamLen {
				streamLen = newLen
			}
		}
	}
	return streamLen, id3v1, apeTag, nil
}

// --- DSF ---

// DsfFormatId is the DSD encoding format identifier.
type DsfFormatId uint32

const (
	DsfFormatIdDsdRaw     DsfFormatId = 0
	DsfFormatIdDsdPhilips DsfFormatId = 1
	DsfFormatIdDsdDst     DsfFormatId = 2
	DsfFormatIdUnknown    DsfFormatId = 0xFFFFFFFF
)

// DsfProperties are the audio properties of a DSF file.
type DsfProperties struct {
	FormatId       DsfFormatId
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	ChannelMask    Option[ChannelMask]
	SampleCount    uint64
	BlockSize      uint32
}

// FileProperties converts to generic [FileProperties].
func (p DsfProperties) FileProperties() FileProperties {
	return NewFileProperties(
		p.Duration,
		Some(p.OverallBitrate),
		Some(p.AudioBitrate),
		Some(p.SampleRate),
		Some(p.BitDepth),
		Some(p.Channels),
		p.ChannelMask,
	)
}

// DsfFile is a DSD Storage Facility file.
type DsfFile struct {
	Properties DsfProperties
	ID3v2      Option[*Tag]
}

// DsfFileReadFrom reads a [DsfFile] from a reader.
func DsfFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*DsfFile, error) {
	var file DsfFile
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

	// DSD chunk: "DSD " + size u64le + file_size u64le + meta_ptr u64le
	var dsd [28]byte
	if _, err := io.ReadFull(reader, dsd[:]); err != nil {
		return nil, &FileParseError{Source: err}
	}
	if !bytes.Equal(dsd[:4], dsdPreamble) {
		return nil, &FileParseError{Source: &FakeTagError{}}
	}
	metadataPtr := binary.LittleEndian.Uint64(dsd[20:])

	var channels, sampleFreq, bitsPerSample, formatId, channelType, blockSize uint32
	var sampleCount uint64
	if parseOptions.ReadProperties {
		// fmt chunk is required for properties
		var fmtHdr [12]byte
		if _, err := io.ReadFull(reader, fmtHdr[:]); err != nil {
			return nil, &FileParseError{Source: err}
		}
		if !bytes.Equal(fmtHdr[:4], dsdFmtPreamble) {
			return nil, &FileParseError{Source: &FakeTagError{}}
		}
		if binary.LittleEndian.Uint64(fmtHdr[4:]) != 52 {
			return nil, &FileParseError{Source: &FakeTagError{}}
		}
		// fmt body: version(4) format_id(4) channel_type(4) channel_num(4)
		// sample_freq(4) bits_per_sample(4) sample_count(8) block_size(4) reserved(4)
		var fb [40]byte
		if _, err := io.ReadFull(reader, fb[:]); err != nil {
			return nil, &FileParseError{Source: err}
		}
		if binary.LittleEndian.Uint32(fb[0:]) != 1 {
			return nil, &FileParseError{Source: &FakeTagError{}}
		}
		channels = binary.LittleEndian.Uint32(fb[12:])
		if channels < 1 || channels > 6 {
			return nil, &FileParseError{Source: &FakeTagError{}}
		}
		sampleFreq = binary.LittleEndian.Uint32(fb[16:])
		bitsPerSample = binary.LittleEndian.Uint32(fb[20:])
		sampleCount = binary.LittleEndian.Uint64(fb[24:])
		formatId = binary.LittleEndian.Uint32(fb[4:])
		channelType = binary.LittleEndian.Uint32(fb[8:])
		blockSize = binary.LittleEndian.Uint32(fb[32:])
	}

	if parseOptions.ReadProperties {
		p := &file.Properties
		p.FormatId = DsfFormatId(formatId)
		p.ChannelMask = channelMaskFromDsfChannelType(channelType)
		p.SampleCount = sampleCount
		p.BlockSize = blockSize
		p.Channels = uint8(channels)
		p.SampleRate = sampleFreq
		p.BitDepth = uint8(bitsPerSample)
		p.AudioBitrate = uint32(divRound(uint64(channels)*uint64(sampleFreq)*uint64(bitsPerSample), 1000))
		if sampleFreq > 0 && sampleCount > 0 {
			millis := divRound(sampleCount*1000, uint64(sampleFreq))
			p.Duration = time.Duration(millis) * time.Millisecond
			if millis > 0 {
				p.OverallBitrate = uint32(divRound(uint64(streamLen)*8, millis))
			}
		}
	}

	// ID3v2 tag at metadata pointer (usually EOF)
	if parseOptions.ReadTags && metadataPtr > 0 && metadataPtr < streamLen {
		if _, err := reader.Seek(start+int64(metadataPtr), io.SeekStart); err == nil {
			if header, err := parseId3v2Header(reader); err == nil {
				if t, err := parseId3v2(reader, header, parseOptions); err == nil {
					file.ID3v2 = Some(t)
				}
			}
		}
	}
	return &file, nil
}

// FileType implements AudioFile.
func (f *DsfFile) FileType() FileType { return FileTypeDsf }

// GetProperties implements AudioFile.
func (f *DsfFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *DsfFile) ContainsTag() bool { return f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *DsfFile) ContainsTagType(t TagType) bool {
	return t == TagTypeID3v2 && f.ID3v2.IsSome()
}

// TaggedFile converts to a [TaggedFile].
func (f *DsfFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeDsf, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *DsfFile) SavePath(path string, wo WriteOptions) error {
	return saveToPath(path, f, TagTypeID3v2, wo)
}

// SaveFile implements AudioFile.
func (f *DsfFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return saveTagFields(file, FileTypeDsf, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *DsfFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, FileTypeDsf, t)
}

// dsfWriteId3v2 writes an ID3v2 tag at the DSF metadata offset (EOF).
func dsfWriteId3v2(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var dsd [28]byte
	if _, err := io.ReadFull(file, dsd[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	if !bytes.Equal(dsd[:4], dsdPreamble) {
		return &FileEncodingError{Source: &FakeTagError{}}
	}
	metadataPtr := binary.LittleEndian.Uint64(dsd[20:])
	var encoded []byte
	if !tag.IsEmpty() {
		btag, err := genericTagToBogem(tag, writeOptions.UseID3v23)
		if err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		var buf bytes.Buffer
		if _, err := btag.WriteTo(&buf); err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		encoded = buf.Bytes()
	}
	if metadataPtr > 0 {
		end, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if err := Splice(file, metadataPtr, uint64(end), encoded); err != nil {
			return &FileEncodingError{Source: err}
		}
		// Update metadata ptr
		newPtr := metadataPtr
		if len(encoded) == 0 {
			newPtr = 0
		}
		if _, err := file.Seek(20, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
		var pb [8]byte
		binary.LittleEndian.PutUint64(pb[:], newPtr)
		if _, err := file.Write(pb[:]); err != nil {
			return &FileEncodingError{Source: err}
		}
	} else if len(encoded) > 0 {
		end, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := file.Write(encoded); err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := file.Seek(20, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
		var pb [8]byte
		binary.LittleEndian.PutUint64(pb[:], uint64(end))
		if _, err := file.Write(pb[:]); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	// Fix file size in DSD chunk
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	if _, err := file.Seek(12, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var fb [8]byte
	binary.LittleEndian.PutUint64(fb[:], uint64(end))
	if _, err := file.Write(fb[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	return nil
}

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
	newLen, id3v1Tag, apeTag, err := readEndTags(reader, start, streamLen, parseOptions)
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
	return saveToPath(path, f, TagTypeApe, wo)
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
	return saveTagFields(file, FileTypeWavPack, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *WavPackFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, FileTypeWavPack, t)
}

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
func (p MpcProperties) FileProperties() FileProperties {
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

// MusepackFile is a Musepack file.
type MusepackFile struct {
	Properties MpcProperties
	ID3v1      Option[*Tag]
	ID3v2      Option[*Tag]
	Ape        Option[*Tag]
}

var mpcSampleRates = [4]uint32{44100, 48000, 37800, 32000}

// MusepackFileReadFrom reads a [MusepackFile] from a reader.
func MusepackFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*MusepackFile, error) {
	var file MusepackFile
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

	var id3v2Tag *Tag
	// ID3v2 at the start
	header, _, found2, err := findId3v2(reader, parseOptions.ReadTags, parseOptions)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	var id3v2Size uint64
	if found2 {
		id3v2Size = uint64(header.FullTagSize())
		if newLen := streamLen - id3v2Size; newLen < streamLen {
			streamLen = newLen
		}
		if parseOptions.ReadTags {
			if _, err := reader.Seek(start+int64(header.FullTagSize()-header.Size), io.SeekStart); err != nil {
				return nil, &FileParseError{Source: err}
			}
			if t, err := parseId3v2(reader, header, parseOptions); err == nil {
				id3v2Tag = t
			}
		}
	}

	// End tags
	newLen, id3v1Tag, apeTag, err := readEndTags(reader, start, streamLen, parseOptions)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamLen = newLen

	// Read MPC stream info
	if _, err := reader.Seek(start+int64(id3v2Size), io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	p := &file.Properties
	var sig [3]byte
	if _, err := io.ReadFull(reader, sig[:]); err != nil {
		return nil, &FileParseError{Source: err}
	}
	if bytes.Equal(sig[:], mpckPreamble[:3]) {
		// SV8: "MPCK" + SH packet: [2B key "SH"][varint size][data]
		var sig2 [1]byte
		if _, err := io.ReadFull(reader, sig2[:]); err != nil {
			return nil, &FileParseError{Source: err}
		}
		if sig2[0] != 'K' {
			return nil, &FileParseError{Source: &FakeTagError{}}
		}
		p.Version = 8
		// Scan packets; stream_length accumulates each packet's content
		// length (size field minus key and size bytes) until SE or EOF.
		var sv8StreamLen, sampleCount, beginningSilence uint64
		foundSH, foundSE := false, false
		for {
			pktStart, err := reader.Seek(0, io.SeekCurrent)
			if err != nil {
				break
			}
			var key [2]byte
			if _, err := io.ReadFull(reader, key[:]); err != nil {
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
					return nil, &FileParseError{Source: err}
				}
				var ver [1]byte
				if _, err := io.ReadFull(reader, ver[:]); err != nil {
					return nil, &FileParseError{Source: err}
				}
				if sampleCount, err = mpcReadSize(reader); err != nil {
					return nil, &FileParseError{Source: err}
				}
				if beginningSilence, err = mpcReadSize(reader); err != nil {
					return nil, &FileParseError{Source: err}
				}
				var bits [2]byte
				if _, err := io.ReadFull(reader, bits[:]); err != nil {
					return nil, &FileParseError{Source: err}
				}
				p.SampleRate = mpcSampleRates[(int(bits[0])>>5)&7]
				p.Channels = uint8((int(bits[1]) >> 4) + 1)
				foundSH = true
			case "SE":
				foundSE = true
			}
			if _, err := reader.Seek(pktStart+int64(size), io.SeekStart); err != nil {
				break
			}
			if foundSE {
				break
			}
		}
		if !foundSH {
			if parseOptions.ParsingMode == ParsingModeStrict {
				return nil, &FileParseError{Source: &MpcParseError{Message: "file is missing a Stream Header packet"}}
			}
		} else if parseOptions.ReadProperties {
			if beginningSilence > sampleCount {
				return nil, &FileParseError{Source: &MpcParseError{Message: "beginning silence is greater than the total sample count"}}
			}
			totalSamples := sampleCount - beginningSilence
			if p.SampleRate > 0 && totalSamples > 0 {
				length := divRound(totalSamples*1000, uint64(p.SampleRate))
				p.Duration = time.Duration(length) * time.Millisecond
				bitrate := uint32(sv8StreamLen * 8 * uint64(p.SampleRate) / (totalSamples * 1000))
				p.OverallBitrate = bitrate
				p.AudioBitrate = bitrate
			}
		}
		if sv8StreamLen == 0 && parseOptions.ParsingMode == ParsingModeStrict {
			return nil, &FileParseError{Source: &MpcParseError{Message: "file is missing an Audio packet"}}
		}
		if !foundSE && parseOptions.ParsingMode == ParsingModeStrict {
			return nil, &FileParseError{Source: &MpcParseError{Message: "file is missing a Stream End packet"}}
		}
	} else {
		// SV7: signature is "MP+" with the version in the same u32.
		// Anything else falls back to the SV4-6 raw header.
		var rest [29]byte
		if _, err := io.ReadFull(reader, rest[:]); err != nil {
			return nil, &FileParseError{Source: err}
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
					return nil, &FileParseError{Source: &TooMuchDataError{}}
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
					bitrate := uint32(uint64(streamLen) * 8 * uint64(p.SampleRate) / (totalSamples * 1000))
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
				return nil, &FileParseError{Source: &MpcParseError{Message: "invalid stream version encountered"}}
			}
			p.Version = uint8(version)
			p.MaxBand = uint8((headerData[0] >> 6) & 0x1F)
			blockSize := headerData[0] & 0x3F
			if version >= 5 {
				p.FrameCount = headerData[1]
			} else {
				p.FrameCount = headerData[1] >> 16
			}
			if parseOptions.ParsingMode == ParsingModeStrict {
				if averageBitrate != 0 {
					return nil, &FileParseError{Source: &MpcParseError{Message: "encountered CBR stream"}}
				}
				if p.IntensityStereo {
					return nil, &FileParseError{Source: &MpcParseError{Message: "stream uses intensity stereo coding"}}
				}
				if blockSize != 1 {
					return nil, &FileParseError{Source: &MpcParseError{Message: "stream has an invalid block size (must be 1)"}}
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
				p.Duration = time.Duration(divRound(samples*1000, uint64(p.SampleRate))) * time.Millisecond
				pcmFrames := uint64(p.FrameCount)*mpcFrameLength - 576
				bitrate := uint32(float64(streamLen) * 8.0 * float64(p.SampleRate) / float64(pcmFrames) / mpcFrameLength)
				p.OverallBitrate = bitrate
				p.AudioBitrate = bitrate
			}
		}
	}

	if parseOptions.ReadTags {
		file.ID3v2 = optionFromPtr(id3v2Tag)
		if apeTag != nil {
			file.Ape = Some(apeTag.Tag())
		}
		if id3v1Tag != nil {
			file.ID3v1 = Some(id3v1Tag.Tag())
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
			return 0, count, &TooMuchDataError{}
		}
		n = (n << 7) | uint64(b[0]&0x7F)
		if b[0]&0x80 == 0 {
			break
		}
	}
	return n, count, nil
}

// FileType implements AudioFile.
func (f *MusepackFile) FileType() FileType { return FileTypeMpc }

// GetProperties implements AudioFile.
func (f *MusepackFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *MusepackFile) ContainsTag() bool {
	return f.ID3v1.IsSome() || f.ID3v2.IsSome() || f.Ape.IsSome()
}

// ContainsTagType implements AudioFile.
func (f *MusepackFile) ContainsTagType(t TagType) bool {
	switch t {
	case TagTypeID3v1:
		return f.ID3v1.IsSome()
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	case TagTypeApe:
		return f.Ape.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *MusepackFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeMpc, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *MusepackFile) SavePath(path string, wo WriteOptions) error {
	return saveToPath(path, f, TagTypeApe, wo)
}

// SaveFile implements AudioFile.
func (f *MusepackFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t)
	}
	return saveTagFields(file, FileTypeMpc, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *MusepackFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, FileTypeMpc, t)
}

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
	ID3v1      Option[*Tag]
	ID3v2      Option[*Tag]
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

	var id3v2Tag *Tag
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
			if t, err := parseId3v2(reader, header, parseOptions); err == nil {
				id3v2Tag = t
			}
		}
	} else {
		reader.Seek(pos, io.SeekStart)
	}

	newLen, id3v1Tag, _, err := readEndTags(reader, start, streamLen, parseOptions)
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
					p.Channels = uint8(((h[2] & 1) << 2) | (h[3] >> 6))
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
			file.ID3v1 = Some(id3v1Tag.Tag())
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
	var buf [1]byte
	prev := byte(0)
	for i := pos; ; i++ {
		if _, err := reader.Read(buf[:]); err != nil {
			return 0, false
		}
		if prev == 0xFF && buf[0]>>5 == 0b111 {
			return i - 1, true
		}
		prev = buf[0]
	}
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
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeAac, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *AacFile) SavePath(path string, wo WriteOptions) error {
	return saveToPath(path, f, TagTypeID3v2, wo)
}

// SaveFile implements AudioFile.
func (f *AacFile) SaveFile(file FileLike, wo WriteOptions) error {
	return f.TaggedFile().Save(file, wo)
}

// RemoveFromPath implements AudioFile.
func (f *AacFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, FileTypeAac, t)
}

func init() {
	formatReaders[fileTypeDsf] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := DsfFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatReaders[fileTypeWavPack] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := WavPackFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatReaders[fileTypeMpc] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := MusepackFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatReaders[fileTypeAac] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := AacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatWriters[fileTypeDsf] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		if tag.TagType() != TagTypeID3v2 {
			return &FileEncodingError{Source: &UnsupportedTagError{}}
		}
		return dsfWriteId3v2(file.File(), tag, writeOptions)
	}
	formatWriters[fileTypeAac] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeID3v2:
			return writeId3v2(file.File(), tag, writeOptions)
		case TagTypeID3v1:
			return writeId3v1(file.File(), Id3v1TagFromTag(tag), writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	formatWriters[fileTypeWavPack] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeApe:
			return apeWriteTo(file.File(), ApeTagFromTag(tag), writeOptions)
		case TagTypeID3v1:
			return writeId3v1(file.File(), Id3v1TagFromTag(tag), writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	formatWriters[fileTypeMpc] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeApe:
			return apeWriteTo(file.File(), ApeTagFromTag(tag), writeOptions)
		case TagTypeID3v1:
			return writeId3v1(file.File(), Id3v1TagFromTag(tag), writeOptions)
		case TagTypeID3v2:
			return writeId3v2(file.File(), tag, writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
}

// WavPack block header flags.
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
func channelMaskFromDsfChannelType(channelType uint32) Option[ChannelMask] {
	switch channelType {
	case 1:
		return Some(ChannelMaskMono)
	case 2:
		return Some(ChannelMaskStereo)
	case 3:
		return Some(ChannelMaskLinearSurround)
	case 4:
		return Some(ChannelMaskFrontLeft | ChannelMaskFrontRight | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 5:
		return Some(ChannelMaskLinearSurround | ChannelMaskLowFrequency)
	case 6:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 7:
		return Some(ChannelMaskLinearSurround | ChannelMaskLowFrequency | ChannelMaskBackLeft | ChannelMaskBackRight)
	}
	return None[ChannelMask]()
}

// mpcReplayGain converts a raw SV7 ReplayGain value (hundredths of dB from
// the old 89dB reference) to the new-scale signed gain, matching Rust.
func mpcReplayGain(gain uint16) int16 {
	if gain == 0 {
		return 0
	}
	g := int16((64.82-float64(gain)/100.0)*256.0 + 0.5)
	if g < 0 || g >= 0x7FFF {
		return 0
	}
	return g
}

// mpcReplayPeak converts a raw SV7 peak value to a log-scale peak.
func mpcReplayPeak(peak uint16) uint16 {
	if peak == 0 {
		return 0
	}
	return uint16(math.Log10(float64(peak))*20.0*256.0 + 0.5)
}
