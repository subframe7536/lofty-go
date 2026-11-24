package aac

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
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
	ChannelMask     meta.Option[meta.ChannelMask]
	Copyrighted     bool
	Original        bool
}

// FileProperties converts to generic [FileProperties].
func (p AacProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.None[uint8](),
		meta.Some(p.Channels),
		p.ChannelMask,
	)
}

// AacFile is an ADTS AAC file.
type AacFile struct {
	Properties AacProperties
	ID3v1      meta.Option[*id3v1.Id3v1Tag]
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
}

var aacSampleRates = [16]uint32{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
	16000, 12000, 11025, 8000, 7350, 0, 0, 0,
}

// adtsHeader is a parsed ADTS frame header (Rust `ADTSHeader`).
type adtsHeader struct {
	version    MpegVersion
	objectType AudioObjectType
	sampleRate uint32
	channels   uint8
	copyright  bool
	original   bool
	length     uint16
	bitrate    uint32
	bytes      [7]byte
	hasCRC     bool
}

// readAdtsHeader reads an ADTS header at the reader's position, mirroring
// `ADTSHeader::read`. The reader is left after the header (and CRC, if any).
func readAdtsHeader(reader io.ReadSeeker, parseMode meta.ParsingMode) (*adtsHeader, error) {
	var h adtsHeader
	if _, err := io.ReadFull(reader, h.bytes[:]); err != nil {
		return nil, err
	}
	b := h.bytes
	if (b[1]>>3)&1 == 0 {
		h.version = MpegVersion4
	} else {
		h.version = MpegVersion2
	}
	h.hasCRC = b[1]&1 == 0
	h.objectType = AudioObjectType(((b[2] >> 6) & 0b11) + 1)
	sfIdx := (b[2] >> 2) & 0b1111
	if sfIdx == 15 {
		return nil, errors.New("file contains an invalid sample frequency index")
	}
	h.sampleRate = aacSampleRates[sfIdx]
	h.channels = ((b[2] & 1) << 2) | ((b[3] >> 6) & 0b11)
	h.original = (b[3]>>5)&1 == 1
	h.copyright = (b[3]>>4)&1 == 1
	h.length = uint16(b[3]&0b11)<<11 | uint16(b[4])<<3 | uint16(b[5])>>5
	// The length includes the header, so anything shorter cannot be a frame.
	headerLen := uint16(7)
	if h.hasCRC {
		headerLen = 9
	}
	if h.length < headerLen {
		if parseMode == meta.ParsingModeStrict {
			return nil, errors.New("file contains a frame shorter than its header")
		}
		return nil, nil
	}
	h.bitrate = ((uint32(h.length) * h.sampleRate / 1024) * 8) / 1024
	if h.hasCRC {
		if _, err := reader.Seek(2, io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	return &h, nil
}

// adtsHeaderMask compares headers up to the home bit (Rust `HEADER_MASK`).
const adtsHeaderMask = 0xFFFF_FFE0

// findNextFrame searches for the next ADTS frame, validating it against the
// header that follows it (Rust `find_next_frame` + `cmp_header`). It returns
// the header and the absolute offset where the frame ends; the returned end is
// always strictly greater than the frame's start, so callers that resume
// scanning from it always make progress.
func findNextFrame(reader io.ReadSeeker, parseMode meta.ParsingMode) (*adtsHeader, int64, error) {
	pos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}
	for {
		abs, ok := aacSearchFrameSync(reader, pos)
		if !ok {
			return nil, 0, nil
		}
		if _, err := reader.Seek(abs, io.SeekStart); err != nil {
			return nil, 0, err
		}
		h, err := readAdtsHeader(reader, parseMode)
		if err != nil {
			return nil, 0, err
		}
		if h == nil {
			return nil, 0, nil
		}
		headerLen := int64(7)
		if h.hasCRC {
			headerLen = 9
		}
		// cmp_header seeks `len.saturating_sub(header_len)` past the header.
		advance := int64(h.length)
		if advance < headerLen {
			advance = headerLen
		}
		next := abs + advance
		var nb [4]byte
		if _, err := reader.Seek(next, io.SeekStart); err != nil {
			return nil, 0, nil
		}
		if _, err := io.ReadFull(reader, nb[:]); err != nil {
			// Undetermined: no further valid frames can be read
			return nil, 0, nil
		}
		if binary.BigEndian.Uint32(nb[:])&adtsHeaderMask == binary.BigEndian.Uint32(h.bytes[:4])&adtsHeaderMask {
			return h, next, nil
		}
		pos = next
	}
}

// AacFileReadFrom reads an [AacFile] from a reader.
func AacFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*AacFile, error) {
	var file AacFile

	// Skip any invalid padding
	startPos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	firstNonZero, foundNonZero, err := util.ChunkedScan(reader, 0, -1, func(window []byte) int {
		for i, b := range window {
			if b != 0 {
				return i
			}
		}
		return -1
	})
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if !foundNonZero {
		return nil, &meta.FileParseError{Source: io.ErrUnexpectedEOF}
	}
	pos := startPos + firstNonZero
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd)
	if _, err := reader.Seek(pos, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	var firstHeader *adtsHeader
	var firstFrameEnd int64
	var headerBuf [4]byte
	for {
		if _, err := io.ReadFull(reader, headerBuf[:]); err != nil {
			break
		}
		if bytes.Equal(headerBuf[:3], []byte("ID3")) {
			// Seek back to read the tag in full
			if _, err := reader.Seek(-4, io.SeekCurrent); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			// Rust `Id3v2Header::parse`: errors are fatal in every mode
			header, err := id3v2.ParseId3v2Header(reader, parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
			}
			// Rust quirk: only the declared size is subtracted, not the
			// 10-byte header.
			if streamLen < uint64(header.Size) {
				return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
			}
			streamLen -= uint64(header.Size)

			if parseOptions.ReadTags {
				t, err := id3v2.ParseId3v2Tag(reader, header, parseOptions)
				if err != nil {
					return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
				}
				if existing, ok := file.ID3v2.Get(); ok {
					// Duplicate tags have their frames appended (issue #87)
					for _, fr := range t.Frames.Frames() {
						existing.Frames.Insert(fr)
					}
				} else {
					file.ID3v2 = meta.Some(t)
				}
			} else if _, err := reader.Seek(int64(header.Size-header.ExtendedSize), io.SeekCurrent); err != nil {
				// Rust leaves the content unskipped here and relies on frame
				// validation to step over it; skipping it is equivalent
				// without risking a false sync inside the tag.
				return nil, &meta.FileParseError{Source: err}
			}

			if header.Flags.Footer {
				if streamLen < 10 {
					return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
				}
				streamLen -= 10
				if _, err := reader.Seek(10, io.SeekCurrent); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
			}
			continue
		}

		// Tags might be followed by junk bytes before the first ADTS frame
		if _, err := reader.Seek(-4, io.SeekCurrent); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		h, end, err := findNextFrame(reader, parseOptions.ParsingMode)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if h != nil {
			firstHeader, firstFrameEnd = h, end
		}
		break
	}

	// Rust's aac reader only searches for ID3v1 at the end — there is no
	// APE scan (and no -32 seek) in `aac/read.rs`.
	if t, _, _, foundI, err := id3v1.FindId3v1(reader, parseOptions.ReadTags, parseOptions.ParsingMode); err != nil {
		return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v1, HasTagType: true, Source: err}}
	} else if foundI {
		if streamLen < 128 {
			return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
		}
		streamLen -= 128
		if parseOptions.ReadTags {
			file.ID3v1 = meta.Some(t)
		}
	}

	if parseOptions.ReadProperties {
		if firstHeader == nil {
			// The search for sync bits was unsuccessful
			return nil, &meta.FileParseError{Source: errors.New("file contains an invalid frame")}
		}
		if firstHeader.sampleRate == 0 && parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &meta.FileParseError{Source: errors.New("file specifies a sample rate of 0")}
		}
		if firstHeader.bitrate == 0 {
			return nil, &meta.FileParseError{Source: errors.New("file specifies a bitrate of 0")}
		}

		// Read as many frames as we can to try and find the average bitrate
		if _, err := reader.Seek(firstFrameEnd, io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		bitrateSum := uint64(firstHeader.bitrate)
		frameCount := uint64(1)
		for {
			h, end, err := findNextFrame(reader, parseOptions.ParsingMode)
			if err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			if h == nil {
				break
			}
			bitrateSum += uint64(h.bitrate)
			frameCount++
			if _, err := reader.Seek(end, io.SeekStart); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
		}
		avg := uint32(bitrateSum / frameCount)

		p := &file.Properties
		p.Version = firstHeader.version
		p.AudioObjectType = firstHeader.objectType
		p.SampleRate = firstHeader.sampleRate
		p.Channels = firstHeader.channels
		p.ChannelMask = meta.ChannelMaskFromMp4Channels(p.Channels)
		p.Copyrighted = firstHeader.copyright
		p.Original = firstHeader.original
		if avg > 0 {
			p.AudioBitrate = avg
			p.OverallBitrate = avg
			p.Duration = util.DurationFromMillis(util.SaturatingMul(streamLen, 8) / uint64(avg))
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
	return pos + offset, true
}

// FileType implements AudioFile.
func (f *AacFile) FileType() meta.FileType { return meta.FileTypeAac }

// GetProperties implements AudioFile.
func (f *AacFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *AacFile) ContainsTag() bool { return f.ID3v1.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *AacFile) ContainsTagType(t meta.TagType) bool {
	switch t {
	case meta.TagTypeID3v1:
		return f.ID3v1.IsSome()
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *AacFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeAac, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *AacFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

// SaveFile implements AudioFile.
func (f *AacFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.ID3v2.Get(); ok {
		if err := t.SaveToVerified(meta.NewVerifiedFileOf(meta.FileTypeAac, file), wo); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &meta.FileEncodingError{FileType: meta.FileTypeAac, Source: err}
		}
	}
	if t, ok := f.ID3v1.Get(); ok {
		if err := id3v1.WriteId3v1(file, t, wo); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFromPath implements AudioFile.
func (f *AacFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeAac, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := AacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})

	meta.RegisterFormatWriter(meta.FileTypeAac, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		switch tag.TagType() {
		case meta.TagTypeID3v2:
			return id3v2.WriteId3v2(file.File(), tag, writeOptions)
		case meta.TagTypeID3v1:
			return id3v1.WriteId3v1(file.File(), id3v1.Id3v1TagFromTag(tag), writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
}
