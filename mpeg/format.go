package mpeg

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

// MpegParseError is returned when an MPEG file fails to parse.
type MpegParseError struct {
	Message string
	Source  error
}

func (e *MpegParseError) Error() string {
	if e.Message != "" {
		return "MPEG: " + e.Message
	}
	return "failed to parse MPEG file"
}
func (e *MpegParseError) Unwrap() error { return e.Source }

// MpegVersion is the MPEG audio version.
type MpegVersion int

const (
	// MpegVersion1 is MPEG-1.
	MpegVersion1 MpegVersion = iota
	// MpegVersion2 is MPEG-2.
	MpegVersion2
	// MpegVersion25 is MPEG-2.5.
	MpegVersion25
	// MpegVersion4 is MPEG-4 (AAC container framing).
	MpegVersion4
)

// MpegLayer is the MPEG audio layer.
type MpegLayer int

const (
	MpegLayerNone MpegLayer = iota
	// MpegLayer1 is MPEG Layer I.
	MpegLayer1
	// MpegLayer2 is MPEG Layer II.
	MpegLayer2
	// MpegLayer3 is MPEG Layer III.
	MpegLayer3
)

// ChannelMode is the MPEG channel mode.
type ChannelMode int

const (
	ChannelModeStereo ChannelMode = iota
	ChannelModeJointStereo
	ChannelModeDualChannel
	ChannelModeMono
)

// MpegChannelMode is an exported alias for [ChannelMode].
type MpegChannelMode = ChannelMode

// mpegHeader is a parsed 4-byte MPEG frame header.
type mpegHeader struct {
	version     MpegVersion
	layer       MpegLayer
	bitrate     uint32 // kbps
	sampleRate  uint32 // Hz
	channels    uint8
	channelMode ChannelMode
	frameLen    uint32
	samples     uint32
	copyrighted bool
	original    bool
	crc         bool
	emphasis    uint8
}

// Bitrate lookup tables [version!=1][layer][index].
// Layer index 0=L1, 1=L2, 2=L3.
var mpegBitrateL1 = [2][16]uint32{
	{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
	{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
}

var mpegBitrateL2 = [2][16]uint32{
	{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
	{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
}

var mpegBitrateL3 = [2][16]uint32{
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
}

var mpegSampleRates = [3][4]uint32{
	{44100, 48000, 32000, 0}, // V1
	{22050, 24000, 16000, 0}, // V2
	{11025, 12000, 8000, 0},  // V2.5
}

var errBadHeader = &MpegParseError{Message: "invalid MPEG header"}

// parseMpegHeader parses a 4-byte MPEG frame header.
func parseMpegHeader(b []byte, firstHeader *mpegHeader) (*mpegHeader, error) {
	if len(b) < 4 {
		return nil, errBadHeader
	}
	h := binary.BigEndian.Uint32(b[:4])
	if (h>>21)&0x7FF != 0x7FF {
		return nil, errBadHeader
	}
	h2 := &mpegHeader{emphasis: uint8(h & 3)}
	ver := (h >> 19) & 3
	switch ver {
	case 0:
		h2.version = MpegVersion25
	case 2:
		h2.version = MpegVersion2
	case 3:
		h2.version = MpegVersion1
	default:
		return nil, errBadHeader
	}
	layer := (h >> 17) & 3
	switch layer {
	case 1:
		h2.layer = MpegLayer3
	case 2:
		h2.layer = MpegLayer2
	case 3:
		h2.layer = MpegLayer1
	default:
		return nil, errBadHeader
	}
	h2.crc = h&(1<<16) == 0

	bitrateIdx := int((h >> 12) & 0xF)
	vIdx := 0
	if h2.version != MpegVersion1 {
		vIdx = 1
	}
	switch h2.layer {
	case MpegLayer1:
		h2.bitrate = mpegBitrateL1[vIdx][bitrateIdx]
	case MpegLayer2:
		h2.bitrate = mpegBitrateL2[vIdx][bitrateIdx]
	default:
		h2.bitrate = mpegBitrateL3[vIdx][bitrateIdx]
	}
	if h2.bitrate == 0 {
		return nil, errBadHeader
	}

	srIdx := int((h >> 10) & 3)
	srVerIdx := 0
	switch h2.version {
	case MpegVersion2:
		srVerIdx = 1
	case MpegVersion25:
		srVerIdx = 2
	}
	h2.sampleRate = mpegSampleRates[srVerIdx][srIdx]
	if h2.sampleRate == 0 {
		return nil, errBadHeader
	}

	padding := (h >> 9) & 1
	h2.channelMode = ChannelMode((h >> 6) & 3)
	if h2.channelMode == ChannelModeMono {
		h2.channels = 1
	} else {
		h2.channels = 2
	}
	h2.copyrighted = h&(1<<3) != 0
	h2.original = h&(1<<2) != 0

	// frame length and samples per frame
	switch h2.layer {
	case MpegLayer1:
		h2.frameLen = (12*h2.bitrate*1000/h2.sampleRate + padding) * 4
		h2.samples = 384
	case MpegLayer2:
		h2.frameLen = 144*h2.bitrate*1000/h2.sampleRate + padding
		h2.samples = 1152
	default:
		var coeff uint32 = 144
		if h2.version != MpegVersion1 {
			coeff = 72
		}
		h2.frameLen = coeff*h2.bitrate*1000/h2.sampleRate + padding
		if h2.version == MpegVersion1 {
			h2.samples = 1152
		} else {
			h2.samples = 576
		}
	}
	if h2.frameLen == 0 {
		return nil, errBadHeader
	}

	// Consecutive headers must match in version/layer/sample rate
	if firstHeader != nil {
		if h2.version != firstHeader.version || h2.layer != firstHeader.layer ||
			h2.sampleRate != firstHeader.sampleRate {
			return nil, errBadHeader
		}
	}
	return h2, nil
}

// xingOffset returns the offset of the Xing/Info header from the start of the frame.
func (h *mpegHeader) xingOffset() int64 {
	off := int64(4)
	if h.crc {
		off += 2
	}
	if h.version == MpegVersion1 {
		if h.channelMode == ChannelModeMono {
			off += 17
		} else {
			off += 32
		}
	} else {
		if h.channelMode == ChannelModeMono {
			off += 9
		} else {
			off += 17
		}
	}
	return off
}

// vbriOffset is fixed at 32 bytes after the 4-byte header.
const vbriOffset = 36

type vbrHeaderType uint8

const (
	vbrTypeXing vbrHeaderType = iota
	vbrTypeInfo
	vbrTypeVbri
)

type vbrHeader struct {
	ty     vbrHeaderType
	frames uint64
	bytes  uint64
}

func (v *vbrHeader) isValid() bool { return v.frames > 0 && v.bytes > 0 }

func parseXing(data []byte) (*vbrHeader, error) {
	if len(data) < 8 || (!bytes.Equal(data[:4], []byte("Xing")) && !bytes.Equal(data[:4], []byte("Info"))) {
		return nil, errBadHeader
	}
	flags := binary.BigEndian.Uint32(data[4:8])
	off := 8
	h := &vbrHeader{ty: vbrTypeXing}
	if bytes.Equal(data[:4], []byte("Info")) {
		h.ty = vbrTypeInfo
	}
	if flags&1 != 0 { // frames
		if len(data) < off+4 {
			return nil, errBadHeader
		}
		h.frames = uint64(binary.BigEndian.Uint32(data[off:]))
		off += 4
	}
	if flags&2 != 0 { // bytes
		if len(data) < off+4 {
			return nil, errBadHeader
		}
		h.bytes = uint64(binary.BigEndian.Uint32(data[off:]))
	}
	return h, nil
}

func parseVbri(data []byte) (*vbrHeader, error) {
	if len(data) < 26 || !bytes.Equal(data[:4], []byte("VBRI")) {
		return nil, errBadHeader
	}
	return &vbrHeader{
		ty:     vbrTypeVbri,
		bytes:  uint64(binary.BigEndian.Uint32(data[10:14])),
		frames: uint64(binary.BigEndian.Uint32(data[14:18])),
	}, nil
}

// findNextFrame searches for the next valid MPEG frame header.
// Returns (header, offset of the frame start relative to the reader's start when called).
func findNextFrame(reader io.ReadSeeker) (*mpegHeader, int64, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}
	var window [4]byte
	var b [1]byte
	read := 0
	for {
		_, err := reader.Read(b[:])
		if err != nil {
			if err == io.EOF {
				return nil, 0, nil
			}
			return nil, 0, err
		}
		window[0] = window[1]
		window[1] = window[2]
		window[2] = window[3]
		window[3] = b[0]
		read++
		if read >= 4 {
			if h, err := parseMpegHeader(window[:], nil); err == nil {
				offset := start + int64(read) - 4
				return h, offset, nil
			}
		}
	}
}

// MpegProperties are the audio properties of an MPEG file.
type MpegProperties struct {
	Version         MpegVersion
	Layer           MpegLayer
	Duration        time.Duration
	OverallBitrate  uint32
	AudioBitrate    uint32
	SampleRate      uint32
	Channels        uint8
	ChannelMode     ChannelMode
	Copyrighted     bool
	Original        bool
	CRC             bool
	Emphasis        uint8
	VBR             bool
	Encoder         Option[string]
	EncoderSettings Option[string]
}

// FileProperties converts to generic [FileProperties].
func (p MpegProperties) FileProperties() FileProperties {
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

// mpegReadProperties reads the properties given a first frame header and its position.
func mpegReadProperties(
	data io.ReadSeeker,
	firstHeader *mpegHeader,
	firstFrameOffset uint64,
	streamLen uint64,
	fileLength uint64,
) *MpegProperties {
	p := &MpegProperties{
		Version:     firstHeader.version,
		Layer:       firstHeader.layer,
		SampleRate:  firstHeader.sampleRate,
		Channels:    firstHeader.channels,
		ChannelMode: firstHeader.channelMode,
		Copyrighted: firstHeader.copyrighted,
		Original:    firstHeader.original,
		CRC:         firstHeader.crc,
		Emphasis:    firstHeader.emphasis,
	}

	// Check for VBR headers
	var vbr *vbrHeader
	// Xing: firstFrameOffset + xingOffset
	if _, err := data.Seek(int64(firstFrameOffset)+firstHeader.xingOffset(), io.SeekStart); err == nil {
		var buf [32]byte
		if n, err := data.Read(buf[:]); err == nil && n >= 8 {
			if v, err := parseXing(buf[:n]); err == nil {
				vbr = v
			}
		}
	}
	if vbr == nil {
		if _, err := data.Seek(int64(firstFrameOffset)+vbriOffset, io.SeekStart); err == nil {
			var buf [26]byte
			if n, err := data.Read(buf[:]); err == nil && n >= 26 {
				if v, err := parseVbri(buf[:n]); err == nil {
					vbr = v
				}
			}
		}
	}

	if vbr != nil && firstHeader.sampleRate > 0 && vbr.isValid() {
		p.VBR = vbr.ty != vbrTypeInfo
		length := divRound(uint64(firstHeader.samples)*1000*vbr.frames, uint64(firstHeader.sampleRate))
		p.Duration = time.Duration(length) * time.Millisecond
		if length > 0 {
			p.OverallBitrate = uint32(fileLength * 8 / length)
			p.AudioBitrate = uint32(vbr.bytes * 8 / length)
		}
		return p
	}

	// Nothing more we can do
	if firstHeader.bitrate == 0 {
		return p
	}

	// CBR (or a VBR stream treated as CBR): estimate duration from the
	// stream length and the first frame's bitrate.
	isCBR := vbr != nil && vbr.ty == vbrTypeInfo
	_ = isCBR // both branches assign the same bitrate in Rust

	// Search for the last frame, starting at the end of the frames
	lastFrameOffset := firstFrameOffset + streamLen
	var lastFrame *mpegHeader
	pos := lastFrameOffset
	for pos > 0 {
		h, newPos, found, err := revSearchForFrameHeader(data, pos)
		if err != nil {
			break
		}
		pos = newPos
		if !found {
			continue
		}
		if h.cmp(firstHeader) {
			lastFrame = h
			lastFrameOffset = pos
			break
		}
	}

	if lastFrame == nil {
		return p
	}

	streamEnd := lastFrameOffset + uint64(lastFrame.frameLen)
	if streamEnd < firstFrameOffset {
		return p
	}

	p.AudioBitrate = firstHeader.bitrate
	length := divRound((streamEnd-firstFrameOffset)*8, uint64(p.AudioBitrate))
	p.Duration = time.Duration(length) * time.Millisecond
	if length > 0 {
		p.OverallBitrate = uint32(fileLength * 8 / length)
	}
	return p
}

// revSearchForFrameHeader scans backwards from pos (bounded by
// revFrameSearchBounds) for a valid MPEG frame sync. On success it returns the
// header and the absolute frame offset; the reader is left positioned there.
const revFrameSearchBounds = 0x400

func revSearchForFrameHeader(data io.ReadSeeker, pos uint64) (*mpegHeader, uint64, bool, error) {
	bounds := pos
	if bounds > revFrameSearchBounds {
		bounds = revFrameSearchBounds
	}
	start := pos - bounds
	if _, err := data.Seek(int64(start), io.SeekStart); err != nil {
		return nil, 0, false, err
	}
	buf := make([]byte, bounds)
	if _, err := io.ReadFull(data, buf); err != nil {
		return nil, 0, false, err
	}
	for i := len(buf) - 1; i >= 1; i-- {
		if buf[i-1] != 0xFF || buf[i]&0xE0 != 0xE0 {
			continue
		}
		if i+3 > len(buf) {
			continue
		}
		h, err := parseMpegHeader(buf[i-1:i+3], nil)
		if err != nil {
			continue
		}
		abs := start + uint64(i-1)
		if _, err := data.Seek(int64(abs), io.SeekStart); err != nil {
			return nil, 0, false, err
		}
		return h, abs, true, nil
	}
	return nil, start, false, nil
}

func (h *mpegHeader) cmp(other *mpegHeader) bool {
	return h.version == other.version && h.layer == other.layer && h.sampleRate == other.sampleRate
}

// MpegFile is an MPEG audio file (MP1/MP2/MP3).
type MpegFile struct {
	Properties MpegProperties
	ID3v1      Option[*Id3v1Tag]
	ID3v2      Option[*Tag]
	APE        Option[*ApeTag]
	Lyrics3    Option[*Lyrics3Tag]
	Lyrics3v2  Option[*Lyrics3v2Tag]
}

// MpegFileReadFrom reads an [MpegFile] from a reader.
func MpegFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*MpegFile, error) {
	var file MpegFile
	var firstFrameOffset uint64
	var firstHeader *mpegHeader

	// Skip zero padding at the start
	var one [1]byte
	for {
		if _, err := reader.Read(one[:]); err != nil {
			return nil, &FileParseError{Source: &MpegParseError{Source: err}}
		}
		if one[0] != 0 {
			break
		}
	}
	if _, err := reader.Seek(-1, io.SeekCurrent); err != nil {
		return nil, &FileParseError{Source: err}
	}

	var id3v2End uint64
	var headerBuf [4]byte
	for {
		if _, err := io.ReadFull(reader, headerBuf[:]); err != nil {
			break
		}
		if bytes.Equal(headerBuf[:3], []byte("ID3")) {
			if _, err := reader.Seek(-4, io.SeekCurrent); err != nil {
				return nil, &FileParseError{Source: err}
			}
			header, err := ParseId3v2HeaderRaw(reader)
			if err != nil {
				return nil, &FileParseError{Source: &TagParseError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
			}
			if parseOptions.ReadTags {
				tag, err := parseId3v2(reader, header, parseOptions)
				if err != nil {
					return nil, &FileParseError{Source: &TagParseError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
				}
				if existing, ok := file.ID3v2.Get(); ok {
					// Merge duplicate tags; frames in later tags replace earlier ones
					for _, item := range tag.Items() {
						existing.InsertUnchecked(item)
					}
					for _, pic := range tag.Pictures() {
						existing.PushPicture(pic)
					}
					continue
				}
				file.ID3v2 = Some(tag)
			} else {
				if _, err := reader.Seek(int64(header.Size), io.SeekCurrent); err != nil {
					return nil, &FileParseError{Source: err}
				}
			}
			if header.Flags.Footer {
				if _, err := reader.Seek(10, io.SeekCurrent); err != nil {
					return nil, &FileParseError{Source: err}
				}
			}
			continue
		}
		if bytes.Equal(headerBuf[:], []byte("APET")) {
			var rest [4]byte
			if _, err := io.ReadFull(reader, rest[:]); err != nil {
				return nil, &FileParseError{Source: err}
			}
			if !bytes.Equal(rest[:], []byte("AGEX")) {
				return nil, &FileParseError{Source: &FakeTagError{}}
			}
			apeHeader, err := ReadApeHeader(reader, false)
			if err != nil {
				return nil, &FileParseError{Source: &TagParseError{TagType: TagTypeApe, HasTagType: true, Source: err}}
			}
			if parseOptions.ReadTags {
				tag, err := ReadApeTagWithHeader(reader, apeHeader, parseOptions)
				if err != nil {
					return nil, &FileParseError{Source: &TagParseError{TagType: TagTypeApe, HasTagType: true, Source: err}}
				}
				file.APE = Some(tag)
			} else {
				if _, err := reader.Seek(int64(apeHeader.Size), io.SeekCurrent); err != nil {
					return nil, &FileParseError{Source: err}
				}
			}
			continue
		}
		// Not a tag: search for the frame sync
		if _, err := reader.Seek(-4, io.SeekCurrent); err != nil {
			return nil, &FileParseError{Source: err}
		}
		h, offset, err := findNextFrame(reader)
		if err != nil {
			return nil, &FileParseError{Source: err}
		}
		if h == nil {
			break
		}
		firstFrameOffset = uint64(offset)
		firstHeader = h

		// Check for a buried ID3v2 tag
		if file.ID3v2.IsNone() && parseOptions.ParsingMode != ParsingModeStrict && offset > 0 {
			if _, err := reader.Seek(0, io.SeekStart); err == nil {
				if _, junkEnd, found, err := findId3v2(reader, parseOptions.ReadTags, parseOptions); err == nil && found {
					if parseOptions.ReadTags {
						var hdr Id3v2Header
						_ = hdr
					}
					id3v2End = uint64(junkEnd)
				}
			}
			if _, err := reader.Seek(int64(firstFrameOffset), io.SeekStart); err != nil {
				return nil, &FileParseError{Source: err}
			}
		}
		break
	}

	if firstHeader == nil {
		if parseOptions.ReadProperties {
			return nil, &FileParseError{Source: &MpegParseError{Message: "no MPEG frames found"}}
		}
		// No frames and no properties: still collect trailing tags below.
	}

	// Re-parse buried ID3v2 if found in junk
	if id3v2End > 0 && parseOptions.ReadTags && file.ID3v2.IsNone() {
		if _, err := reader.Seek(0, io.SeekStart); err == nil {
			header, end, found, err := findId3v2(reader, false, parseOptions)
			if err == nil && found {
				contentStart := end - int64(header.Size)
				if header.Flags.Footer {
					contentStart -= 10
				}
				if _, err := reader.Seek(contentStart, io.SeekStart); err == nil {
					tag, err := parseId3v2(reader, header, parseOptions)
					if err == nil {
						file.ID3v2 = Some(tag)
					}
				}
			}
		}
		if _, err := reader.Seek(int64(firstFrameOffset), io.SeekStart); err != nil {
			return nil, &FileParseError{Source: err}
		}
	}

	// Find the end of the stream: skip ID3v1/Lyrics3/APE at the end
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	endOffset := uint64(streamEnd)

	// ID3v1
	var id3v1Tag *Id3v1Tag
	if t, start, _, found, err := findId3v1(reader, parseOptions.ReadTags, parseOptions.ParsingMode); err == nil && found {
		id3v1Tag = t
		if uint64(start) < endOffset {
			endOffset = uint64(start)
		}
	}
	if _, err := reader.Seek(int64(endOffset), io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	// Lyrics3
	if start, _, t, found, err := findLyrics3(reader, parseOptions.ReadTags, parseOptions); err == nil && found {
		switch v := t.(type) {
		case *Lyrics3Tag:
			file.Lyrics3 = Some(v)
		case *Lyrics3v2Tag:
			file.Lyrics3v2 = Some(v)
		}

		if uint64(start) < endOffset {
			endOffset = uint64(start)
		}
	}
	if _, err := reader.Seek(int64(endOffset), io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	// APE at the end
	if endOffset >= 32 {
		if _, err := reader.Seek(int64(endOffset)-32, io.SeekStart); err == nil {
			if tag, apeHeader, err := ReadApeTag(reader, true, parseOptions); err == nil && apeHeader != nil {
				if parseOptions.ReadTags {
					file.APE = Some(tag)
				}
				if int64(endOffset)-32-int64(apeHeader.Size)+32 > 0 {
					endOffset = uint64(int64(endOffset) - int64(apeHeader.Size))
				}
			}
		}
	}

	var properties *MpegProperties
	if parseOptions.ReadProperties {
		streamLen := endOffset - firstFrameOffset
		if _, err := reader.Seek(int64(firstFrameOffset), io.SeekStart); err != nil {
			return nil, &FileParseError{Source: err}
		}
		properties = mpegReadProperties(reader, firstHeader, firstFrameOffset, streamLen, uint64(streamEnd))
	} else {
		properties = &MpegProperties{}
	}
	file.Properties = *properties
	file.ID3v1 = optionFromPtr(id3v1Tag)
	return &file, nil
}

// FileType implements AudioFile.
func (f *MpegFile) FileType() FileType { return FileTypeMpeg }

// GetProperties implements AudioFile.
func (f *MpegFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *MpegFile) ContainsTag() bool {
	return f.ID3v1.IsSome() || f.ID3v2.IsSome() || f.APE.IsSome() || f.Lyrics3.IsSome() || f.Lyrics3v2.IsSome()
}

// ContainsTagType implements AudioFile.
func (f *MpegFile) ContainsTagType(tagType TagType) bool {
	switch tagType {
	case TagTypeID3v1:
		return f.ID3v1.IsSome()
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	case TagTypeApe:
		return f.APE.IsSome()
	case TagTypeLyrics3:
		return f.Lyrics3.IsSome()
	case TagTypeLyrics3v2:
		return f.Lyrics3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v2, ID3v1, Lyrics3, Lyrics3v2, Ape.
func (f *MpegFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Lyrics3.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Lyrics3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.APE.Get(); ok {
		tags = append(tags, t.Tag())
	}

	return NewTaggedFile(FileTypeMpeg, tags, *f.GetProperties())
}

// SavePath saves the file's tags back to the path.
func (f *MpegFile) SavePath(path string, writeOptions WriteOptions) error {
	return SaveToPath(path, f, f.primaryTagType(), writeOptions)
}

func (f *MpegFile) primaryTagType() TagType {
	if f.ID3v2.IsSome() {
		return TagTypeID3v2
	}
	if f.APE.IsSome() {
		return TagTypeApe
	}
	if f.ID3v1.IsSome() {
		return TagTypeID3v1
	}
	return TagTypeID3v2
}

// SaveFile writes tags to an open file.
func (f *MpegFile) SaveFile(file FileLike, writeOptions WriteOptions) error {
	// Write ID3v2 at the start, ID3v1 at the end
	if t, ok := f.ID3v2.Get(); ok {
		if err := WriteId3v2(file, t, writeOptions); err != nil {
			return err
		}
	}
	if t, ok := f.ID3v1.Get(); ok {
		if err := WriteId3v1(file, t, writeOptions); err != nil {
			return err
		}
	}
	if t, ok := f.APE.Get(); ok {
		if err := ApeWriteTo(file, t, writeOptions); err != nil {
			return err
		}
	}
	if t, ok := f.Lyrics3v2.Get(); ok {
		if err := Lyrics3v2WriteTo(file, t, writeOptions); err != nil {
			return err
		}
	}
	if t, ok := f.Lyrics3.Get(); ok {
		if err := Lyrics3v1WriteTo(file, t, writeOptions); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFromPath removes a tag type.
func (f *MpegFile) RemoveFromPath(path string, tagType TagType) error {
	return RemoveTagFromPath(path, tagType)
}

func init() {
	RegisterFormatReader(FileTypeMpeg, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := MpegFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	RegisterFormatWriter(FileTypeMpeg, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
		f := file.File()
		switch tag.TagType() {
		case TagTypeID3v2:
			return WriteId3v2(f, tag, writeOptions)
		case TagTypeID3v1:
			return WriteId3v1(f, Id3v1TagFromTag(tag), writeOptions)
		case TagTypeApe:
			return ApeWriteTo(f, ApeTagFromTag(tag), writeOptions)
		case TagTypeLyrics3v2:
			return Lyrics3v2WriteTo(f, Lyrics3v2TagFromTag(tag), writeOptions)
		case TagTypeLyrics3:
			return Lyrics3v1WriteTo(f, Lyrics3TagFromTag(tag), writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	})
}
