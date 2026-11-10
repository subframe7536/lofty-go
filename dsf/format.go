package dsf

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
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
	ID3v2      Option[*Id3v2Tag]
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
				p.OverallBitrate = uint32(divRound(streamLen*8, millis))
			}
		}
	}

	// ID3v2 tag at metadata pointer (usually EOF)
	if parseOptions.ReadTags && metadataPtr > 0 && metadataPtr < streamLen {
		if _, err := reader.Seek(start+int64(metadataPtr), io.SeekStart); err == nil {
			if header, err := ParseId3v2HeaderRaw(reader); err == nil {
				if t, err := parseId3v2Tag(reader, header, parseOptions); err == nil {
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
		tags = append(tags, t.Tag())
	}
	return NewTaggedFile(FileTypeDsf, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *DsfFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeID3v2, wo)
}

// SaveFile implements AudioFile.
func (f *DsfFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return SaveTagFields(file, FileTypeDsf, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *DsfFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

// dsfWriteId3v2 writes an ID3v2 tag at the DSF metadata offset (EOF).
func dsfWriteId3v2(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	var encoded []byte
	if !tag.IsEmpty() {
		var err error
		encoded, err = Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}
	return dsfWriteId3v2Chunk(file, encoded)
}

// dsfWriteId3v2Chunk writes serialized ID3v2 bytes at the DSF metadata
// pointer, mirroring Rust `chunk_file::write_to_dsf` (empty removes the tag).
func dsfWriteId3v2Chunk(file FileLike, encoded []byte) error {
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
func init() {
	RegisterFormatReader(FileTypeDsf, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := DsfFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	RegisterFormatWriter(FileTypeDsf, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
		if tag.TagType() != TagTypeID3v2 {
			return &FileEncodingError{Source: &UnsupportedTagError{}}
		}
		return dsfWriteId3v2(file.File(), tag, writeOptions)
	})
	RegisterConcreteTagWriter(FileTypeDsf, TagTypeID3v2, func(file *VerifiedFile, tag any, writeOptions WriteOptions) error {
		return dsfWriteId3v2Chunk(file.File(), tag.([]byte))
	})
}
