package ogg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

var oggPreamble = []byte("OggS")

// OggParseError is returned when an Ogg file fails to parse.
type OggParseError struct {
	Message string
	Source  error
}

func (e *OggParseError) Error() string {
	if e.Message != "" {
		return "Ogg: " + e.Message
	}
	return "failed to parse Ogg file"
}
func (e *OggParseError) Unwrap() error { return e.Source }

// OggPageHeader is the header of an [OggPage], mirroring `ogg_pager::PageHeader`.
type OggPageHeader struct {
	Start          int64
	HeaderType     byte
	Abgp           uint64
	StreamSerial   uint32
	SequenceNumber uint32
	Segments       []byte
	Checksum       uint32
}

// OggPage is a single Ogg page.
type OggPage struct {
	headerType byte
	granulePos uint64
	serial     uint32
	sequence   uint32
	crc        uint32
	segments   []byte // lacing values
	data       []byte
	start      int64 // file offset of page start
	dataStart  int64 // file offset of segment data
	end        int64 // file offset after page
}

// Header returns the page's header fields, mirroring `ogg_pager::Page::header`.
func (p *OggPage) Header() OggPageHeader {
	return OggPageHeader{
		Start:          p.start,
		HeaderType:     p.headerType,
		Abgp:           p.granulePos,
		StreamSerial:   p.serial,
		SequenceNumber: p.sequence,
		Segments:       p.segments,
		Checksum:       p.crc,
	}
}

// Content returns the page's payload bytes, mirroring `ogg_pager::Page::content`.
func (p *OggPage) Content() []byte { return p.data }

// Start returns the byte offset where the page begins.
func (p *OggPage) Start() int64 { return p.start }

// End returns the byte offset immediately after the page.
func (p *OggPage) End() int64 { return p.end }

// GenCrc computes and stores the Ogg CRC-32 checksum on the page, mirroring
// `ogg_pager::Page::gen_crc`.
func (p *OggPage) GenCrc() {
	raw := oggSerializePage(p)
	p.crc = binary.LittleEndian.Uint32(raw[22:26])
}

// AsBytes serializes the page (computing its CRC-32), mirroring
// `ogg_pager::Page::as_bytes`.
func (p *OggPage) AsBytes() []byte { return oggSerializePage(p) }

const (
	// OggPageFlagContinuation indicates a continued packet (0x01).
	OggPageFlagContinuation byte = 1
	// OggPageFlagBOS indicates the beginning of a logical bitstream (0x02).
	OggPageFlagBOS byte = 2
	// OggPageFlagEOS indicates the end of a logical bitstream (0x04).
	OggPageFlagEOS byte = 4

	oggPageContinuation = OggPageFlagContinuation
	oggPageBOS          = OggPageFlagBOS
	oggPageEOS          = OggPageFlagEOS
)

var (
	// Sentinel page errors matching the `ogg_pager::PageError` variants that
	// `find_last_page` retries when a reverse search lands inside page content.
	errOggMissingMagic    = &OggParseError{Source: &meta.FakeTagError{}}
	errOggInvalidVersion  = &OggParseError{Message: "unsupported Ogg page version"}
	errOggBadSegmentCount = &OggParseError{Message: "invalid Ogg page segment count"}
)

// oggHeaderError marks a failure while reading or validating a page header.
// ogg_pager's `Packets::read_count` loops with `while let Ok(header)`, so a
// header failure ends the packet scan, while body read errors propagate.
type oggHeaderError struct{ err error }

func (e *oggHeaderError) Error() string { return e.err.Error() }
func (e *oggHeaderError) Unwrap() error { return e.err }

// ReadOggPage reads a single [OggPage] at the reader's current position,
// mirroring `ogg_pager::Page::read`.
func ReadOggPage(reader io.ReadSeeker) (*OggPage, error) {
	return readOggPage(reader)
}

// readOggPage reads one page at the current position, mirroring
// `ogg_pager::Page::read` (`PageHeader::read` followed by the page body).
func readOggPage(reader io.ReadSeeker) (*OggPage, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	var hdr [27]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, &oggHeaderError{err: err}
	}
	if !bytes.Equal(hdr[:4], oggPreamble) {
		return nil, &oggHeaderError{err: errOggMissingMagic}
	}
	if hdr[4] != 0 {
		return nil, &oggHeaderError{err: errOggInvalidVersion}
	}
	nsegs := int(hdr[26])
	if nsegs < 1 {
		return nil, &oggHeaderError{err: errOggBadSegmentCount}
	}
	p := &OggPage{
		headerType: hdr[5],
		granulePos: binary.LittleEndian.Uint64(hdr[6:]),
		serial:     binary.LittleEndian.Uint32(hdr[14:]),
		sequence:   binary.LittleEndian.Uint32(hdr[18:]),
		crc:        binary.LittleEndian.Uint32(hdr[22:]),
		start:      start,
	}
	segs := make([]byte, nsegs)
	if _, err := io.ReadFull(reader, segs); err != nil {
		return nil, &oggHeaderError{err: err}
	}
	p.segments = segs
	total := 0
	for _, s := range segs {
		total += int(s)
	}
	p.data = make([]byte, total)
	if _, err := io.ReadFull(reader, p.data); err != nil {
		return nil, err
	}
	p.dataStart = start + 27 + int64(nsegs)
	p.end = p.dataStart + int64(total)
	return p, nil
}

// oggFirstPage returns the serial and granule position of the first page.
func oggFirstPage(reader io.ReadSeeker) (serial uint32, granule uint64, err error) {
	page, err := readOggPage(reader)
	if err != nil {
		return 0, 0, err
	}
	return page.serial, page.granulePos, nil
}

// oggLastGranulePos finds the last page's granule position for duration,
// mirroring Rust's `find_last_page`: the first reverse search runs from EOF
// all the way to the stream start; only the retries after a false positive
// (`MissingMagic`/`InvalidVersion`/`BadSegmentCount`) are bounded by
// `headerEnd`. Any other error aborts the search and the caller falls back to
// the 1000ms default length.
func oggLastGranulePos(reader io.ReadSeeker, headerEnd int64) (uint64, error) {
	found, err := util.NewRevPatternSearcher(reader, oggPreamble).Search()
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, errOggMissingMagic
	}
	for {
		current, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		page, err := readOggPage(reader)
		if err == nil {
			return page.granulePos, nil
		}
		var he *oggHeaderError
		if !errors.As(err, &he) {
			return 0, err
		}
		if !errors.Is(he.err, errOggMissingMagic) &&
			!errors.Is(he.err, errOggInvalidVersion) &&
			!errors.Is(he.err, errOggBadSegmentCount) {
			return 0, err
		}
		// False positive, keep searching
		if _, err := reader.Seek(current, io.SeekStart); err != nil {
			return 0, err
		}
		found, err := util.NewRevPatternSearcher(reader, oggPreamble).
			WithStart(util.RevSearchFromCurrent).
			WithEnd(util.RevSearchEndAtPos(uint64(max(headerEnd, 0)))).
			Search()
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, errOggMissingMagic
		}
	}
}

// OggProperties are the audio properties of an Ogg file.
type OggProperties struct {
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	Channels       uint8

	// Vorbis: codec version + declared bitrates
	Version        uint32
	BitrateMax     int32
	BitrateNominal int32
	BitrateMin     int32

	// Opus: input rate (48kHz output is constant) + output gain
	InputSampleRate uint32
	OutputGainDb    float32

	// Speex: mode + VBR flag + declared nominal bitrate
	Mode uint32
	Vbr  bool

	// Opus: channel mapping derived from the mapping family + channel count
	ChannelMask meta.Option[meta.ChannelMask]
}

// FileProperties converts to generic [meta.FileProperties].
func (p OggProperties) FileProperties() meta.FileProperties {
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

// OggVorbisFile is an Ogg Vorbis file.
type OggVorbisFile struct {
	Properties     OggProperties
	VorbisComments meta.Option[*vorbis.VorbisCommentsTag]
}

// OggOpusFile is an Ogg Opus file.
type OggOpusFile struct {
	Properties     OggProperties
	VorbisComments meta.Option[*vorbis.VorbisCommentsTag]
}

// OggSpeexFile is an Ogg Speex file.
type OggSpeexFile struct {
	Properties     OggProperties
	VorbisComments meta.Option[*vorbis.VorbisCommentsTag]
}

// oggReadTagged sniffs the codec from the identification packet and reads an
// Ogg file: returns (properties, vorbis comments tag, file type). The format
// readers use [oggReadTaggedAs] with the requested type instead.
func oggReadTagged(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*OggProperties, *vorbis.VorbisCommentsTag, meta.FileType, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, meta.FileType{}, &meta.FileParseError{Source: err}
	}
	sniffed, _, _, err := OggReadCountPackets(reader, 1)
	if err != nil {
		return nil, nil, meta.FileType{}, &meta.FileParseError{Source: &OggParseError{Source: err}}
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, nil, meta.FileType{}, &meta.FileParseError{Source: err}
	}
	ident := sniffed[0]

	var fType meta.FileType
	switch {
	case len(ident) >= 7 && bytes.Equal(ident[1:7], []byte("vorbis")):
		fType = meta.FileTypeVorbis
	case len(ident) >= 8 && bytes.Equal(ident[:8], []byte("OpusHead")):
		fType = meta.FileTypeOpus
	case len(ident) >= 8 && bytes.Equal(ident[:8], []byte("Speex   ")):
		fType = meta.FileTypeSpeex
	default:
		return nil, nil, meta.FileType{}, &meta.FileParseError{Source: &OggParseError{Message: "unsupported Ogg codec"}}
	}
	props, vc, err := oggReadTaggedAs(reader, parseOptions, fType)
	if err != nil {
		return nil, nil, meta.FileType{}, err
	}
	return props, vc, fType, nil
}

// oggReadTaggedAs reads an Ogg file as the requested codec, mirroring Rust's
// per-type `read_from`: each type knows its signatures and header packet
// count (3 for Vorbis, 2 for Opus/Speex), and a stream that does not carry the
// requested identification signature fails with "file missing magic signature".
func oggReadTaggedAs(reader io.ReadSeeker, parseOptions meta.ParseOptions, fType meta.FileType) (*OggProperties, *vorbis.VorbisCommentsTag, error) {
	// Errors raised before the properties step carry the file type (Rust's
	// `with_format` on the shared `read_from`).
	headerErr := func(src error) error { return &meta.FileParseError{FileType: fType, Source: src} }

	var identSig, commentSig []byte
	packetCount := 2
	switch {
	case fType.Equal(meta.FileTypeVorbis):
		identSig = []byte{1, 'v', 'o', 'r', 'b', 'i', 's'}
		commentSig = []byte{3, 'v', 'o', 'r', 'b', 'i', 's'}
		packetCount = 3
	case fType.Equal(meta.FileTypeOpus):
		identSig = []byte("OpusHead")
		commentSig = []byte("OpusTags")
	case fType.Equal(meta.FileTypeSpeex):
		identSig = []byte("Speex   ")
	default:
		return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "unsupported Ogg codec"}}
	}

	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, headerErr(err)
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, headerErr(err)
	}
	streamLen := uint64(streamEnd - start) // #nosec G115 -- stream end position is always >= the current in-stream position
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, nil, headerErr(err)
	}

	_, firstGranule, err := oggFirstPage(reader)
	if err != nil {
		return nil, nil, headerErr(&OggParseError{Source: err})
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, nil, headerErr(err)
	}

	packets, _, _, err := OggReadCountPackets(reader, packetCount)
	if err != nil {
		return nil, nil, headerErr(&OggParseError{Source: err})
	}
	if len(packets) < 1 || (parseOptions.ReadTags && len(packets) < 2) {
		return nil, nil, headerErr(&OggParseError{Message: "missing Ogg header packets"})
	}
	ident := packets[0]
	// Rust `verify_signature` — a hard error in every parsing mode
	if len(ident) < len(identSig) || !bytes.Equal(ident[:len(identSig)], identSig) {
		return nil, nil, headerErr(&OggParseError{Message: "file missing magic signature"})
	}
	// Rust's find_last_page searches back to where the header packets ended.
	headerEnd, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, headerErr(err)
	}

	props := &OggProperties{}
	var vc *vorbis.VorbisCommentsTag
	var preSkip uint64

	if parseOptions.ReadTags {
		comment := packets[1]
		if len(commentSig) > 0 &&
			(len(comment) < len(commentSig) || !bytes.Equal(comment[:len(commentSig)], commentSig)) {
			return nil, nil, headerErr(&OggParseError{Message: "file missing magic signature"})
		}
		t, err := vorbis.ParseVorbisComments(comment[len(commentSig):], parseOptions)
		if err != nil {
			return nil, nil, headerErr(&vorbis.VorbisCommentsParseError{Source: err})
		}
		vc = t
	}

	if parseOptions.ReadProperties {
		switch fType {
		case meta.FileTypeVorbis:
			// Identification header: vorbis_version(4) channels(1) rate(4)
			// bitrate_max(4) nominal(4) min(4) — 28 bytes minimum.
			if len(ident) < 28 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Source: io.ErrUnexpectedEOF}}
			}
			props.Version = binary.LittleEndian.Uint32(ident[7:])
			props.Channels = ident[11]
			props.SampleRate = binary.LittleEndian.Uint32(ident[12:])
			props.BitrateMax = int32(binary.LittleEndian.Uint32(ident[16:]))     // #nosec G115 -- bit-preserving conversion equivalent to Rust `read_i32`; mirrors upstream
			props.BitrateNominal = int32(binary.LittleEndian.Uint32(ident[20:])) // #nosec G115 -- bit-preserving conversion equivalent to Rust `read_i32`; mirrors upstream
			props.BitrateMin = int32(binary.LittleEndian.Uint32(ident[24:]))     // #nosec G115 -- bit-preserving conversion equivalent to Rust `read_i32`; mirrors upstream
		case meta.FileTypeOpus:
			// OpusHead: version(1) channels(1) preskip(2) rate(4) gain(2) mapping(1)
			// Rust reads these fields through a byte reader — a truncated
			// OpusHead errors.
			if len(ident) < 19 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "truncated OpusHead"}}
			}
			props.Version = uint32(ident[8])
			props.Channels = ident[9]
			preSkip = uint64(binary.LittleEndian.Uint16(ident[10:]))
			props.InputSampleRate = binary.LittleEndian.Uint32(ident[12:])
			props.OutputGainDb = float32(int16(binary.LittleEndian.Uint16(ident[16:]))) / 256 // #nosec G115 -- bit-preserving conversion equivalent to Rust `read_i16`; mirrors upstream
			family := ident[18]
			if family > 1 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "invalid channel mapping family"}}
			}
			if (family == 0 && props.Channels > 2) || (family == 1 && props.Channels > 8) {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "invalid channel count for mapping family"}}
			}
			mask, ok := meta.ChannelMaskFromOpusChannels(props.Channels).Get()
			if !ok {
				// Rust's `expect` cannot fail after the family checks; a
				// channel count of 0 lands here instead.
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "invalid channel count"}}
			}
			props.ChannelMask = meta.Some(mask)
			// Rust reports `input_sample_rate`; duration still uses 48kHz.
			props.SampleRate = props.InputSampleRate
		case meta.FileTypeSpeex:
			// The Speex header is a fixed 80-byte structure.
			if len(ident) < 80 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "header packet too small"}}
			}
			props.Version = binary.LittleEndian.Uint32(ident[28:])
			if props.Version > 1 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "unknown speex stream version"}}
			}
			props.SampleRate = binary.LittleEndian.Uint32(ident[36:])
			props.Mode = binary.LittleEndian.Uint32(ident[40:])
			if channels := binary.LittleEndian.Uint32(ident[48:]); channels != 1 && channels != 2 {
				return nil, nil, &meta.FileParseError{Source: &OggParseError{Message: "found invalid channel count, must be mono or stereo"}}
			} else {
				props.Channels = uint8(channels)
			}
			props.BitrateNominal = int32(binary.LittleEndian.Uint32(ident[52:])) // #nosec G115 -- bit-preserving conversion equivalent to Rust `read_i32`; mirrors upstream
			props.Vbr = binary.LittleEndian.Uint32(ident[60:]) == 1
		}
	}

	if parseOptions.ReadProperties {
		// `length` defaults to 1000ms when the last page or the PCM
		// values can't be determined (Rust `read_properties`).
		lengthMs := uint64(1000)
		gotTotal := false
		// Opus duration is always computed at 48kHz; `props.SampleRate`
		// reports the declared input rate.
		rate := props.SampleRate
		if fType == meta.FileTypeOpus {
			rate = 48000
		}
		if granule, err := oggLastGranulePos(reader, headerEnd); err == nil && rate > 0 {
			// Duration = (last_abgp - first_abgp [- preskip]) / rate, in whole ms
			var total uint64
			if granule > firstGranule {
				total = granule - firstGranule
			}
			if total > preSkip {
				total -= preSkip
			} else {
				total = 0
			}
			if total > 0 {
				// Rust's Vorbis math is u128; Opus/Speex multiply in u64
				// (saturated here instead of wrapping).
				lengthMs = util.MulDivRound(total, 1000, uint64(rate))
				props.Duration = util.DurationFromMillis(lengthMs)
				gotTotal = true
			}
		}

		// Bitrate semantics are codec-specific (mirroring Rust):
		// overall is always fileLen*8/len; audio is the stream length
		// minus the two header packets, or the declared nominal rate.
		// Vorbis and Speex use the 1000ms fallback unconditionally;
		// Opus computes bitrates only when a valid total exists.
		var headerLen uint64
		for _, p := range packets[:2] {
			headerLen += uint64(len(p))
		}
		var dataLen uint64
		if streamLen > headerLen {
			dataLen = streamLen - headerLen
		}
		switch fType {
		case meta.FileTypeVorbis:
			if lengthMs > 0 { // Rust: checked_div
				props.OverallBitrate = uint32(util.SaturatingMul(streamLen, 8) / lengthMs) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
			}
			if props.BitrateNominal > 0 {
				props.AudioBitrate = uint32(uint64(props.BitrateNominal) / 1000) // #nosec G115 -- mirrors Rust `(nominal_bitrate as u64 / 1000) as u32`; truncation is intentional parity
			}
		case meta.FileTypeOpus:
			if gotTotal && lengthMs > 0 {
				props.OverallBitrate = uint32(util.SaturatingMul(streamLen, 8) / lengthMs) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
				props.AudioBitrate = uint32(util.SaturatingMul(dataLen, 8) / lengthMs)     // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
			}
		case meta.FileTypeSpeex:
			if props.BitrateNominal > 0 {
				if lengthMs > 0 {
					props.OverallBitrate = uint32(util.SaturatingMul(streamLen, 8) / lengthMs) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
				}
				props.AudioBitrate = uint32(uint64(props.BitrateNominal) / 1000) // #nosec G115 -- mirrors Rust `(nominal_bitrate as u64 / 1000) as u32`; truncation is intentional parity
			} else if lengthMs > 0 {
				// `DivRound(SaturatingMul(x, 8), n)` would wrap at the
				// saturation point; the 128-bit form saturates cleanly.
				props.OverallBitrate = uint32(util.MulDivRound(streamLen, 8, lengthMs)) // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
				props.AudioBitrate = uint32(util.MulDivRound(dataLen, 8, lengthMs))     // #nosec G115 -- mirrors Rust `as u32` cast; truncation is intentional parity
			}
		}
	}
	return props, vc, nil
}

// --- shared Ogg tag write ---

// oggCrcLookup is the Ogg checksum table (MSB-first, polynomial 0x04C11DB7).
var oggCrcLookup = func() [256]uint32 {
	var t [256]uint32
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
		t[i] = crc
	}
	return t
}()

func OggCrc32(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ oggCrcLookup[byte(crc>>24)^b]
	}
	return crc
}

// oggSerializePage marshals a page, generating the CRC over the whole page.
func oggSerializePage(p *OggPage) []byte {
	out := make([]byte, 0, 27+len(p.segments)+len(p.data))
	var hdr [27]byte
	copy(hdr[:4], oggPreamble)
	hdr[5] = p.headerType
	binary.LittleEndian.PutUint64(hdr[6:], p.granulePos)
	binary.LittleEndian.PutUint32(hdr[14:], p.serial)
	binary.LittleEndian.PutUint32(hdr[18:], p.sequence)
	hdr[26] = byte(len(p.segments)) // #nosec G115 -- segment count is at most 255 (parsed from a single count byte) or 32 (pagination cap)
	out = append(out, hdr[:]...)
	out = append(out, p.segments...)
	out = append(out, p.data...)
	crc := OggCrc32(out)
	binary.LittleEndian.PutUint32(out[22:], crc)
	return out
}

const oggMaxWrittenSegmentCount = 32

// OggReadCountPackets reads `count` complete packets for the stream, returning
// the packets and a synthetic remainder page when the last packet ends mid-page
// (mirroring ogg_pager::Packets::read_count).
func OggReadCountPackets(reader io.ReadSeeker, count int) ([][]byte, *OggPage, uint32, error) {
	// ogg_pager: zero and values below -1 are an empty successful read.
	if count == 0 || count < -1 {
		return nil, nil, 0, nil
	}
	unlimited := count == -1

	var packets [][]byte
	var current []byte
	var remainder *OggPage
	var serial uint32
	for unlimited || len(packets) < count {
		page, err := readOggPage(reader)
		if err != nil {
			// ogg_pager's `Packets::read_count` loops `while let Ok(header)`:
			// a header failure ends the scan, body errors propagate.
			var he *oggHeaderError
			if errors.As(err, &he) {
				break
			}
			return nil, nil, 0, err
		}
		if len(packets) == 0 && current == nil {
			serial = page.serial
		}
		pos := 0
		for segIdx, s := range page.segments {
			current = append(current, page.data[pos:pos+int(s)]...)
			pos += int(s)
			if s == 255 {
				continue
			}
			packets = append(packets, current)
			current = nil
			if !unlimited && len(packets) == count {
				remSegs := page.segments[segIdx+1:]
				if len(remSegs) > 0 {
					granule := ^uint64(0) // no packet finishes on this page
					for _, rs := range remSegs {
						if rs < 255 {
							granule = page.granulePos
							break
						}
					}
					remainder = &OggPage{
						headerType: page.headerType &^ oggPageContinuation,
						granulePos: granule,
						serial:     page.serial,
						segments:   append([]byte{}, remSegs...),
						data:       append([]byte{}, page.data[pos:]...),
					}
				}
				return packets, remainder, serial, nil
			}
		}
	}
	if !unlimited && len(packets) != count {
		return nil, nil, 0, &OggParseError{Source: util.ErrNotEnoughData}
	}
	return packets, remainder, serial, nil
}

// oggPaginate turns packets into pages of at most 32 segments
// (mirroring ogg_pager::paginate).
//
//nolint:unparam // abgp mirrors ogg_pager::paginate
func oggPaginate(packets [][]byte, serial uint32, abgp uint64, flags byte) []*OggPage {
	const maxContent = 255 * oggMaxWrittenSegmentCount
	var pages []*OggPage
	firstPage, freshPacket, packetFinished := true, true, false
	var lastSegSize byte
	var pageContent []byte
	remaining := maxContent

	flush := func() {
		var htype byte
		switch {
		case firstPage && flags&oggPageBOS != 0:
			htype = oggPageBOS
		case !freshPacket:
			htype = oggPageContinuation
		}
		var granule uint64
		if packetFinished {
			granule = abgp
		} else {
			granule = ^uint64(0)
		}
		var segs []byte
		fullSegs := len(pageContent) / 255
		for i := 0; i < fullSegs; i++ {
			segs = append(segs, 255)
		}
		if fullSegs != oggMaxWrittenSegmentCount {
			segs = append(segs, lastSegSize)
		}
		pages = append(pages, &OggPage{
			headerType: htype,
			granulePos: granule,
			serial:     serial,
			segments:   segs,
			data:       pageContent,
		})
		pageContent = nil
		packetFinished = false
		remaining = maxContent
	}

	for _, packet := range packets {
		freshPacket = true
		lastSegSize = byte(len(packet) % 255)
		if len(packet) == 0 {
			packetFinished = true
			flush()
			firstPage = false
			continue
		}
		pkt := packet
		for len(pkt) > 0 {
			n := remaining
			if len(pkt) < n {
				n = len(pkt)
			}
			pageContent = append(pageContent, pkt[:n]...)
			remaining -= n
			pkt = pkt[n:]
			if len(pkt) == 0 {
				packetFinished = true
			}
			if remaining == 0 || len(pkt) == 0 {
				flush()
			}
			firstPage = false
			freshPacket = false
		}
		if len(pageContent) > 0 {
			flush()
		}
		// A packet exactly filling a page needs a nil page to mark its end
		if len(packet)%(255*oggMaxWrittenSegmentCount) == 0 {
			packetFinished = true
			flush()
		}
	}
	if len(pages) > 0 && flags&oggPageEOS != 0 {
		pages[len(pages)-1].headerType |= oggPageEOS
	}
	return pages
}

// OggPaginate turns packets into Ogg pages of at most 32 segments, mirroring
// `ogg_pager::paginate`.
func OggPaginate(packets [][]byte, serial uint32, abgp uint64, flags byte) []*OggPage {
	return oggPaginate(packets, serial, abgp, flags)
}

// oggWriteVorbisComments replaces the comment packet in an Ogg file.
func oggWriteVorbisComments(file meta.FileLike, ft meta.FileType, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	pics := make([]meta.PictureInfoPair, 0, len(tag.Pictures()))
	for _, p := range tag.Pictures() {
		info, err := meta.PictureInformationFromPicture(&p)
		if err != nil {
			info = meta.PictureInformation{}
		}
		pics = append(pics, meta.PictureInfoPair{Picture: p, Info: info})
	}
	return oggWriteVorbisCommentsPics(file, ft, tag, pics, writeOptions)
}

// oggWriteVorbisCommentsPics is [oggWriteVorbisComments] with explicit
// [meta.PictureInformation] for each picture comment.
func oggWriteVorbisCommentsPics(file meta.FileLike, ft meta.FileType, tag *meta.Tag, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	return oggWriteVorbisCommentsImpl(file, ft, writeOptions, func(vendor string) *vorbis.VorbisCommentsTag {
		vc := vorbis.VorbisCommentsTagForWritePics(tag, vendor, false)
		vc.SetPictures(pics)
		return vc
	})
}

// oggWriteVorbisCommentsImpl replaces the comment packet in an Ogg file. The
// `build` callback receives the file's retained vendor string, and the written
// packet always carries that vendor (Rust's `write` sets `tag.vendor` before
// serializing).
func oggWriteVorbisCommentsImpl(file meta.FileLike, ft meta.FileType, writeOptions meta.WriteOptions, build func(vendor string) *vorbis.VorbisCommentsTag) error {
	var headerPacketCount int
	var sig []byte
	switch ft {
	case meta.FileTypeVorbis:
		headerPacketCount, sig = 3, []byte{3, 'v', 'o', 'r', 'b', 'i', 's'}
	case meta.FileTypeOpus:
		headerPacketCount, sig = 2, []byte("OpusTags")
	case meta.FileTypeSpeex:
		headerPacketCount = 2
	default:
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	packets, remainder, serial, err := OggReadCountPackets(file, headerPacketCount)
	if err != nil {
		return &meta.FileEncodingError{Source: &OggParseError{Source: err}}
	}
	// Everything after the consumed page(s) is copied verbatim
	rest, err := io.ReadAll(file)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	if len(packets) < 2 {
		return &meta.FileEncodingError{Source: &OggParseError{Message: "missing Ogg header packets"}}
	}
	comment := packets[1]
	if sig != nil && (len(comment) < len(sig) || !bytes.Equal(comment[:len(sig)], sig)) {
		return &meta.FileEncodingError{Source: &meta.FileParseError{Source: &OggParseError{Message: "file missing magic signature"}}}
	}

	// Retain the vendor string. Rust reads `vendor_len` bytes unconditionally:
	// a truncated vendor is a hard error, and an invalid UTF-8 vendor
	// (`String::from_utf8` failing) becomes "".
	off := len(sig)
	if len(comment) < off+4 {
		return &meta.FileEncodingError{Source: &OggParseError{Source: io.ErrUnexpectedEOF}}
	}
	vlen := binary.LittleEndian.Uint32(comment[off:])
	if uint64(vlen) > uint64(len(comment)-off-4) { // #nosec G115 -- len(comment) >= off+4 is guaranteed by the truncation check above, so the subtraction is non-negative
		return &meta.FileEncodingError{Source: &OggParseError{Source: io.ErrUnexpectedEOF}}
	}
	var vendor string
	if vb := comment[off+4 : off+4+int(vlen)]; utf8.Valid(vb) {
		vendor = string(vb)
	}

	// Items are serialized verbatim (incl. EncoderSoftware → ENCODER); the
	// file's retained vendor is offered to the builder.
	vc := build(vendor)
	// The file's vendor always wins over the tag's own (Rust's
	// `tag.vendor = vendor_str`). WithVendor copies, so the concrete SaveFile
	// path doesn't mutate the caller's tag.
	vc = vc.WithVendor(vendor)
	vcData, err := vorbis.EncodeVorbisComments(vc)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeVorbisComments, HasTagType: true, Source: err}}
	}
	newComment := append(append([]byte{}, sig...), vcData...)
	if ft == meta.FileTypeVorbis {
		newComment = append(newComment, 1) // framing bit
	}
	// Pad the comment packet, aligned to written-page boundaries
	if pp := writeOptions.PreferredPadding; pp > 0 {
		const boundary = 255 * oggMaxWrittenSegmentCount
		padded := ((len(newComment) + int(pp) + boundary - 1) / boundary) * boundary
		newComment = append(newComment, make([]byte, padded-len(newComment))...)
	}
	packets[1] = newComment

	var out bytes.Buffer
	newPages := oggPaginate(packets, serial, 0, oggPageBOS)
	var seq uint32
	for _, p := range newPages {
		p.sequence = seq
		out.Write(oggSerializePage(p))
		seq++
	}
	if remainder != nil {
		remainder.sequence = seq
		out.Write(oggSerializePage(remainder))
		seq++
	}
	// Replay the remaining pages verbatim, renumbering + regen CRC
	restSeeker := io.NewSectionReader(bytes.NewReader(rest), 0, int64(len(rest)))
	for {
		page, err := readOggPage(restSeeker)
		if err != nil {
			break
		}
		page.sequence = seq
		out.Write(oggSerializePage(page))
		seq++
	}

	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return meta.Splice(file, 0, uint64(end), out.Bytes()) // #nosec G115 -- end is a Seek end-of-stream position, always non-negative
}

func (f *OggVorbisFile) FileType() meta.FileType { return meta.FileTypeVorbis }
func (f *OggVorbisFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggVorbisFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggVorbisFile) ContainsTagType(t meta.TagType) bool {
	return t == meta.TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggVorbisFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeVorbis, tags, *f.GetProperties())
}

func (f *OggVorbisFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

func (f *OggVorbisFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeVorbisComments)
}

func (f *OggVorbisFile) Conflicts() [][2]meta.TagType { return nil }

func (f *OggVorbisFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

func (f *OggVorbisFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

func (f *OggOpusFile) FileType() meta.FileType { return meta.FileTypeOpus }
func (f *OggOpusFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggOpusFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggOpusFile) ContainsTagType(t meta.TagType) bool {
	return t == meta.TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggOpusFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeOpus, tags, *f.GetProperties())
}

func (f *OggOpusFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

func (f *OggOpusFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeVorbisComments)
}

func (f *OggOpusFile) Conflicts() [][2]meta.TagType { return nil }

func (f *OggOpusFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

func (f *OggOpusFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

func (f *OggSpeexFile) FileType() meta.FileType { return meta.FileTypeSpeex }
func (f *OggSpeexFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggSpeexFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggSpeexFile) ContainsTagType(t meta.TagType) bool {
	return t == meta.TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggSpeexFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeSpeex, tags, *f.GetProperties())
}

func (f *OggSpeexFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

func (f *OggSpeexFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeVorbisComments)
}

func (f *OggSpeexFile) Conflicts() [][2]meta.TagType { return nil }

func (f *OggSpeexFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

func (f *OggSpeexFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

// OggVorbisFileReadFrom reads an [OggVorbisFile].
func OggVorbisFileReadFrom(reader io.ReadSeeker, po meta.ParseOptions) (*OggVorbisFile, error) {
	props, vc, err := oggReadTaggedAs(reader, po, meta.FileTypeVorbis)
	if err != nil {
		return nil, err
	}
	return &OggVorbisFile{Properties: *props, VorbisComments: util.OptionFromPtr(vc)}, nil
}

// OggOpusFileReadFrom reads an [OggOpusFile].
func OggOpusFileReadFrom(reader io.ReadSeeker, po meta.ParseOptions) (*OggOpusFile, error) {
	props, vc, err := oggReadTaggedAs(reader, po, meta.FileTypeOpus)
	if err != nil {
		return nil, err
	}
	return &OggOpusFile{Properties: *props, VorbisComments: util.OptionFromPtr(vc)}, nil
}

// OggSpeexFileReadFrom reads an [OggSpeexFile].
func OggSpeexFileReadFrom(reader io.ReadSeeker, po meta.ParseOptions) (*OggSpeexFile, error) {
	props, vc, err := oggReadTaggedAs(reader, po, meta.FileTypeSpeex)
	if err != nil {
		return nil, err
	}
	return &OggSpeexFile{Properties: *props, VorbisComments: util.OptionFromPtr(vc)}, nil
}

func init() {
	for _, ft := range []meta.FileType{meta.FileTypeVorbis, meta.FileTypeOpus, meta.FileTypeSpeex} {
		meta.RegisterFormatReader(ft, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
			props, vc, err := oggReadTaggedAs(rs, opts, ft)
			if err != nil {
				return nil, err
			}
			var tags []*meta.Tag
			if vc != nil {
				tags = append(tags, vc.Tag())
			}
			return meta.NewTaggedFile(ft, tags, props.FileProperties()), nil
		})
		meta.RegisterFormatWriter(ft, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
			if tag.TagType() != meta.TagTypeVorbisComments {
				return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
			}
			return oggWriteVorbisComments(file.File(), file.Format(), tag, writeOptions)
		})
		meta.RegisterConcreteTagWriter(ft, meta.TagTypeVorbisComments, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
			vc := tag.(*vorbis.VorbisCommentsTag)
			return oggWriteVorbisCommentsImpl(file.File(), file.Format(), writeOptions, func(string) *vorbis.VorbisCommentsTag {
				return vc
			})
		})
	}
}
