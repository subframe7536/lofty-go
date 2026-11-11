package dsf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

var (
	dsdFmtPreamble = []byte("fmt ")
	dsdPreamble    = []byte("DSD ")
)

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
	ChannelMask    meta.Option[meta.ChannelMask]
	SampleCount    uint64
	BlockSize      uint32
}

// FileProperties converts to generic [FileProperties].
func (p DsfProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(p.BitDepth),
		meta.Some(p.Channels),
		p.ChannelMask,
	)
}

// DsfFile is a DSD Storage Facility file.
type DsfFile struct {
	Properties DsfProperties
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
}

// DsfFileReadFrom reads a [DsfFile] from a reader.
func DsfFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*DsfFile, error) {
	var file DsfFile
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

	// DSD chunk: "DSD " + size u64le + file_size u64le + meta_ptr u64le
	var dsd [28]byte
	if _, err := io.ReadFull(reader, dsd[:]); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if !bytes.Equal(dsd[:4], dsdPreamble) {
		return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
	}
	// Rust `verify_dsf` requires the declared chunk size to be 28
	if binary.LittleEndian.Uint64(dsd[4:]) != 28 {
		return nil, &meta.FileParseError{Source: errors.New(`file has invalid "DSD " chunk size`)}
	}
	totalFileSize := binary.LittleEndian.Uint64(dsd[12:])
	metadataPtr := binary.LittleEndian.Uint64(dsd[20:])

	var channels, sampleFreq, bitsPerSample, formatId, channelType, blockSize uint32
	var sampleCount uint64
	if parseOptions.ReadProperties {
		// fmt chunk is required for properties
		var fmtHdr [12]byte
		if _, err := io.ReadFull(reader, fmtHdr[:]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if !bytes.Equal(fmtHdr[:4], dsdFmtPreamble) {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
		}
		if binary.LittleEndian.Uint64(fmtHdr[4:]) != 52 {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
		}
		// fmt body: version(4) format_id(4) channel_type(4) channel_num(4)
		// sample_freq(4) bits_per_sample(4) sample_count(8) block_size(4) reserved(4)
		var fb [40]byte
		if _, err := io.ReadFull(reader, fb[:]); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if binary.LittleEndian.Uint32(fb[0:]) != 1 {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
		}
		channels = binary.LittleEndian.Uint32(fb[12:])
		if channels < 1 || channels > 6 {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
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
		// Rust: u8::try_from(channel_type) + ChannelMask::from_dsf_channel_type,
		// erroring on >255 or unmapped values
		if channelType > 255 {
			return nil, &meta.FileParseError{Source: errors.New("unsupported channel type")}
		}
		p.ChannelMask = channelMaskFromDsfChannelType(channelType)
		if p.ChannelMask.IsNone() {
			return nil, &meta.FileParseError{Source: errors.New("unsupported channel type")}
		}
		p.SampleCount = sampleCount
		p.BlockSize = blockSize
		p.Channels = uint8(channels)
		p.SampleRate = sampleFreq
		p.BitDepth = uint8(bitsPerSample)
		// Rust truncates bit_depth to u8 before the bitrate calculation
		p.AudioBitrate = uint32(util.DivRound(uint64(channels)*uint64(sampleFreq)*uint64(p.BitDepth), 1000))
		if sampleFreq > 0 && sampleCount > 0 {
			millis := util.DivRound(sampleCount*1000, uint64(sampleFreq))
			p.Duration = time.Duration(millis) * time.Millisecond
			if millis > 0 {
				// Rust uses the declared total_file_size, not the actual
				// stream length
				p.OverallBitrate = uint32(util.DivRound(totalFileSize*8, millis))
			}
		}
	}

	// ID3v2 tag at metadata pointer (usually EOF) — Rust seeks there
	// unconditionally and runs find_id3v2 (which scans for the "ID3" magic).
	if parseOptions.ReadTags && metadataPtr > 0 && metadataPtr < streamLen {
		if _, err := reader.Seek(start+int64(metadataPtr), io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		header, end, found, err := id3v2.FindId3v2(reader, true, parseOptions)
		if err != nil {
			return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
		if found {
			contentStart := end - int64(header.Size)
			if header.Flags.Footer {
				contentStart -= 10
			}
			if _, err := reader.Seek(contentStart, io.SeekStart); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			tag, err := id3v2.ParseId3v2Tag(reader, header, parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
			}
			file.ID3v2 = meta.Some(tag)
		}
	}
	return &file, nil
}

// FileType implements AudioFile.
func (f *DsfFile) FileType() meta.FileType { return meta.FileTypeDsf }

// GetProperties implements AudioFile.
func (f *DsfFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *DsfFile) ContainsTag() bool { return f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *DsfFile) ContainsTagType(t meta.TagType) bool {
	return t == meta.TagTypeID3v2 && f.ID3v2.IsSome()
}

// TaggedFile converts to a [TaggedFile].
func (f *DsfFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeDsf, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *DsfFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, meta.TagTypeID3v2, wo)
}

// SaveFile implements AudioFile.
func (f *DsfFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.SaveTagFields(file, meta.FileTypeDsf, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *DsfFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

// dsfWriteId3v2 writes an ID3v2 tag at the DSF metadata offset (EOF).
func dsfWriteId3v2(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	var encoded []byte
	if !tag.IsEmpty() {
		var err error
		encoded, err = id3v2.Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}
	return dsfWriteId3v2Chunk(file, encoded)
}

// dsfWriteId3v2Chunk writes serialized ID3v2 bytes at the DSF metadata
// pointer, mirroring Rust `chunk_file::write_to_dsf` (empty removes the tag).
func dsfWriteId3v2Chunk(file meta.FileLike, encoded []byte) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var dsd [28]byte
	if _, err := io.ReadFull(file, dsd[:]); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if !bytes.Equal(dsd[:4], dsdPreamble) {
		return &meta.FileEncodingError{Source: &meta.FakeTagError{}}
	}
	if binary.LittleEndian.Uint64(dsd[4:]) != 28 {
		return &meta.FileEncodingError{Source: errors.New(`file has invalid "DSD " chunk size`)}
	}
	totalFileSize := binary.LittleEndian.Uint64(dsd[12:])
	metadataPtr := binary.LittleEndian.Uint64(dsd[20:])

	var newTotalFileSize, newMetadataPtr uint64
	if metadataPtr > 0 {
		if metadataPtr > totalFileSize {
			return &meta.FileEncodingError{Source: errors.New("existing metadata ptr is invalid")}
		}
		if len(encoded) == 0 {
			// Drop the pointer and shrink the size
			newTotalFileSize = metadataPtr
			newMetadataPtr = 0
		} else {
			newTotalFileSize = metadataPtr + uint64(len(encoded))
			newMetadataPtr = metadataPtr
		}
	} else {
		if len(encoded) == 0 {
			// No tag to overwrite, nothing to do
			return nil
		}
		newTotalFileSize = totalFileSize + uint64(len(encoded))
		newMetadataPtr = totalFileSize
	}

	if _, err := file.Seek(12, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var fb [16]byte
	binary.LittleEndian.PutUint64(fb[:], newTotalFileSize)
	binary.LittleEndian.PutUint64(fb[8:], newMetadataPtr)
	if _, err := file.Write(fb[:]); err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	if newMetadataPtr == 0 {
		return file.Truncate(int64(newTotalFileSize))
	}
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return meta.Splice(file, newMetadataPtr, uint64(end), encoded)
}

func channelMaskFromDsfChannelType(channelType uint32) meta.Option[meta.ChannelMask] {
	switch channelType {
	case 1:
		return meta.Some(meta.ChannelMaskMono)
	case 2:
		return meta.Some(meta.ChannelMaskStereo)
	case 3:
		return meta.Some(meta.ChannelMaskLinearSurround)
	case 4:
		return meta.Some(meta.ChannelMaskFrontLeft | meta.ChannelMaskFrontRight | meta.ChannelMaskBackLeft | meta.ChannelMaskBackRight)
	case 5:
		return meta.Some(meta.ChannelMaskLinearSurround | meta.ChannelMaskLowFrequency)
	case 6:
		return meta.Some(meta.ChannelMaskLinearSurround | meta.ChannelMaskBackLeft | meta.ChannelMaskBackRight)
	case 7:
		return meta.Some(meta.ChannelMaskLinearSurround | meta.ChannelMaskLowFrequency | meta.ChannelMaskBackLeft | meta.ChannelMaskBackRight)
	}
	return meta.None[meta.ChannelMask]()
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeDsf, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := DsfFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	meta.RegisterFormatWriter(meta.FileTypeDsf, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		if tag.TagType() != meta.TagTypeID3v2 {
			return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
		}
		return dsfWriteId3v2(file.File(), tag, writeOptions)
	})
	meta.RegisterConcreteTagWriter(meta.FileTypeDsf, meta.TagTypeID3v2, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
		return dsfWriteId3v2Chunk(file.File(), tag.([]byte))
	})
}
