package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// Mp4ParseError is returned when an MP4 file fails to parse.
type Mp4ParseError struct {
	Message string
	Source  error
}

func (e *Mp4ParseError) Error() string {
	if e.Message != "" {
		return "MP4: " + e.Message
	}
	return "failed to parse MP4 file"
}
func (e *Mp4ParseError) Unwrap() error { return e.Source }

// Mp4Codec is the codec of an MP4 audio stream.
type Mp4Codec string

// MP4 codec identifiers.
const (
	Mp4CodecNone Mp4Codec = ""
	Mp4CodecAAC  Mp4Codec = "AAC"
	Mp4CodecALAC Mp4Codec = "ALAC"
	Mp4CodecMP3  Mp4Codec = "MP3"
	Mp4CodecFLAC Mp4Codec = "FLAC"
)

// Mp4Properties are the audio properties of an MP4 file.
type Mp4Properties struct {
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	Codec          Mp4Codec
	// IsDrmProtected reports whether the stream is FairPlay-protected.
	IsDrmProtected bool
	// ExtendedAudioObjectType is the AudioObjectType from the esds descriptor's
	// AudioSpecificConfig, when present.
	ExtendedAudioObjectType meta.Option[AudioObjectType]
	// Ftyp is the file's major brand from the ftyp atom.
	Ftyp string
}

// AudioObjectType is an MPEG-4 Part 3 audio object type.
type AudioObjectType = meta.AudioObjectType

const (
	// AudioObjectTypeNull is the null/unknown object type.
	AudioObjectTypeNull = meta.AudioObjectTypeNull
	// AudioObjectTypeAacMain is AAC Main Profile.
	AudioObjectTypeAacMain = meta.AudioObjectTypeAacMain
	// AudioObjectTypeAacLowComplexity is AAC Low Complexity.
	AudioObjectTypeAacLowComplexity = meta.AudioObjectTypeAacLowComplexity
	// AudioObjectTypeAacScalableSampleRate is AAC Scalable Sample Rate.
	AudioObjectTypeAacScalableSampleRate = meta.AudioObjectTypeAacScalableSampleRate
	// AudioObjectTypeAacLongTermPrediction is AAC Long Term Predictor.
	AudioObjectTypeAacLongTermPrediction = meta.AudioObjectTypeAacLongTermPrediction
	// AudioObjectTypeAudioLosslessCoding is MPEG-4 ALS.
	AudioObjectTypeAudioLosslessCoding = meta.AudioObjectTypeAudioLosslessCoding
)

// FileProperties converts to generic [meta.FileProperties].
func (p Mp4Properties) FileProperties() meta.FileProperties {
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

// Mp4File is an MP4 audio file.
type Mp4File struct {
	Properties Mp4Properties
	Ilst       meta.Option[*Ilst]
}

// mp4BoxHeader is a parsed box header.
type mp4BoxHeader struct {
	name [4]byte
	size uint64 // payload size (excludes header)
	hdr  uint64 // header size
	pos  int64  // payload offset
}

// mp4ReadBox reads a box header at the current position.
func mp4ReadBox(reader io.ReadSeeker) (*mp4BoxHeader, error) {
	return mp4ReadBoxBound(reader, math.MaxUint64, meta.ParsingModeRelaxed)
}

// mp4ReadBoxBound reads a box header with Rust `AtomInfo::read` semantics:
// the ident must contain valid bytes, the size must cover the header, and the
// content must fit within `avail` bytes from the atom's start. Returns
// (nil, nil) for the Rust `Ok(None)` cases — an atom that can't be recovered,
// where parsing should simply stop.
func mp4ReadBoxBound(reader io.ReadSeeker, avail uint64, mode meta.ParsingMode) (*mp4BoxHeader, error) {
	pos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	var hdr [8]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, err
	}
	for _, b := range hdr[4:] {
		if b != 0xA9 && (b < 0x20 || b > 0x7E) {
			// Unrecoverable; seek to end like Rust
			_, _ = reader.Seek(0, io.SeekEnd)
			if mode == meta.ParsingModeStrict {
				return nil, &Mp4ParseError{Message: "atom ident contains invalid characters"}
			}
			return nil, nil
		}
	}
	size := uint64(binary.BigEndian.Uint32(hdr[:4]))
	h := &mp4BoxHeader{hdr: 8, pos: pos + 8}
	copy(h.name[:], hdr[4:])
	var total uint64
	switch size {
	case 0:
		// extends to EOF
		end, err := reader.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		size = uint64(end - h.pos)
		total = size + 8
		if _, err := reader.Seek(h.pos, io.SeekStart); err != nil {
			return nil, err
		}
	case 1:
		var ls [8]byte
		if _, err := io.ReadFull(reader, ls[:]); err != nil {
			return nil, err
		}
		total = binary.BigEndian.Uint64(ls[:])
		h.hdr = 16
		h.pos = pos + 16
		if total < 16 {
			_, _ = reader.Seek(0, io.SeekEnd)
			return nil, &Mp4ParseError{Source: util.NewNotEnoughDataError(8)}
		}
		size = total - 16
	default:
		total = size
		if total < 8 {
			_, _ = reader.Seek(0, io.SeekEnd)
			return nil, &Mp4ParseError{Source: util.NewNotEnoughDataError(8)}
		}
		size = total - 8
	}
	if avail != math.MaxUint64 && total > avail {
		// Content extends past the reader bound.
		skippable := (mode != meta.ParsingModeStrict && h.name == [4]byte{'f', 'r', 'e', 'e'}) || mode == meta.ParsingModeRelaxed
		_, _ = reader.Seek(0, io.SeekEnd)
		if skippable {
			return nil, nil
		}
		return nil, &Mp4ParseError{Source: &meta.SizeMismatchError{}}
	}
	h.size = size
	return h, nil
}

// mp4SkipBox skips past a box.
func mp4SkipBox(reader io.ReadSeeker, h *mp4BoxHeader) error {
	_, err := reader.Seek(h.pos+int64(h.size), io.SeekStart)
	return err
}

// offsetReadSeeker wraps a ReadSeeker so position 0 corresponds to the
// absolute offset base: Seeks and reported positions stay file-absolute.
// It lets a buffered moov subtree be walked with the same code as the file.
type offsetReadSeeker struct {
	r    io.ReadSeeker
	base int64
}

func (o *offsetReadSeeker) Read(p []byte) (int, error) { return o.r.Read(p) }

func (o *offsetReadSeeker) Seek(off int64, whence int) (int64, error) {
	if whence == io.SeekStart {
		off -= o.base
	}
	n, err := o.r.Seek(off, whence)
	return n + o.base, err
}

// mp4MetaIsFull reports whether a `meta` atom carries the 4-byte
// version+flags field before its children (Rust `meta_is_full`): if the
// second four bytes of the payload are a known child ident, the `meta` is
// non-full and children start immediately.
func mp4MetaIsFull(reader io.ReadSeeker, h *mp4BoxHeader) bool {
	var b [8]byte
	if _, err := reader.Seek(h.pos, io.SeekStart); err != nil {
		return true
	}
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return true
	}
	_, _ = reader.Seek(h.pos, io.SeekStart)
	switch string(b[4:8]) {
	case "hdlr", "ilst", "mhdr", "ctry", "lang":
		return false
	}
	return true
}

// mp4FindDescendants walks a container box's children looking for a named box.
// (Removed: no longer used after the meta_is_full read-path change.)

// mp4IlstKeyToItemKey maps an ilst atom name to a freeform key string.
// Returns (key, isIntPair) where key is "" for data-only atoms.
var mp4Genres = []string{
	"", "Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge",
	"Hip-Hop", "Jazz", "Metal", "New Age", "Oldies", "Other", "Pop", "R&B",
	"Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative", "Ska",
	"Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient",
	"Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance", "Classical",
	"Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise",
	"AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative",
	"Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic", "Darkwave",
	"Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream",
	"Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40", "Christian Rap",
	"Pop/Funk", "Jungle", "Native American", "Cabaret", "New Wave",
	"Psychadelic", "Rave", "Showtunes", "Trailer", "Lo-Fi", "Tribal",
	"Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical", "Rock & Roll",
	"Hard Rock",
}

// (mp4BoxPayload removed: superseded by mp4ParseFreeform's ordered parse.)

// Mp4FileReadFrom reads an [Mp4File] from a reader.
func Mp4FileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*Mp4File, error) {
	var file Mp4File
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	fileLen := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	// Top-level walk: the first atom must be `ftyp` (Rust `verify_mp4`).
	first, err := mp4ReadBoxBound(reader, fileLen, parseOptions.ParsingMode)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if first == nil || string(first.name[:]) != "ftyp" {
		return nil, &meta.FileParseError{Source: &Mp4ParseError{Source: &meta.FakeTagError{}}}
	}
	if first.size < 4 {
		return nil, &meta.FileParseError{Source: util.NewNotEnoughDataError(12)}
	}
	var brand [4]byte
	if _, err := io.ReadFull(reader, brand[:]); err != nil {
		return nil, &meta.FileParseError{Source: &Mp4ParseError{Source: err}}
	}
	bstr, err := util.Utf8DecodeStr(brand[:])
	if err != nil {
		return nil, &meta.FileParseError{Source: &Mp4ParseError{Message: "malformed ftyp major brand"}}
	}
	file.Properties.Ftyp = bstr
	if _, err := reader.Seek(first.pos+int64(first.size), io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	// Find moov + mdat
	var moov *mp4BoxHeader
	var mdatSize uint64
	for {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		if pos >= streamEnd {
			break
		}
		h, err := mp4ReadBoxBound(reader, uint64(streamEnd-pos), parseOptions.ParsingMode)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if h == nil {
			break
		}
		switch string(h.name[:]) {
		case "moov":
			moov = h
		case "mdat":
			if mdatSize == 0 {
				mdatSize = h.size
			}
		}
		if err := mp4SkipBox(reader, h); err != nil {
			break
		}
	}
	if moov == nil {
		return nil, &meta.FileParseError{Source: &Mp4ParseError{Source: &meta.FakeTagError{}}}
	}

	// Walk moov children: trak (props), udta/meta/ilst (tag). Buffer the moov
	// body when it fits the allocation limit so the many small box reads and
	// seeks inside it become memory operations instead of syscalls.
	mr := reader
	if int64(moov.size) > 0 {
		if buf, aerr := util.TryByteSlice(int(moov.size)); aerr == nil {
			if _, serr := reader.Seek(moov.pos, io.SeekStart); serr == nil {
				if n, rerr := io.ReadFull(reader, buf); rerr == nil || rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
					mr = &offsetReadSeeker{r: bytes.NewReader(buf[:n]), base: moov.pos}
				}
			}
		}
	}
	if _, err := mr.Seek(moov.pos, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	moovEnd := moov.pos + int64(moov.size)
	var ilstTag *Ilst
	var traks []*mp4BoxHeader

	for {
		pos, _ := mr.Seek(0, io.SeekCurrent)
		if pos >= moovEnd {
			break
		}
		h, err := mp4ReadBox(mr)
		if err != nil || h == nil {
			break
		}
		boxEnd := h.pos + int64(h.size)
		switch string(h.name[:]) {
		case "trak":
			traks = append(traks, h)
		case "udta":
			// udta/meta/ilst
			if t, err := mp4WalkUdta(mr, boxEnd, parseOptions); err == nil && t != nil {
				ilstTag = t
			}
		}
		if _, err := mr.Seek(boxEnd, io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
	}

	if parseOptions.ReadProperties {
		if err := mp4ReadProperties(mr, traks, fileLen, mdatSize, parseOptions, &file.Properties); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
	}
	if parseOptions.ReadTags {
		file.Ilst = util.OptionFromPtr(ilstTag)
	}
	return &file, nil
}

// mp4Children calls fn for each child box of container h; fn returning false stops.
func mp4Children(reader io.ReadSeeker, h *mp4BoxHeader, fn func(c *mp4BoxHeader) bool) {
	end := h.pos + int64(h.size)
	if _, err := reader.Seek(h.pos, io.SeekStart); err != nil {
		return
	}
	for {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		if pos >= end {
			return
		}
		c, err := mp4ReadBox(reader)
		if err != nil || c == nil {
			return
		}
		if !fn(c) {
			return
		}
		if _, err := reader.Seek(c.pos+int64(c.size), io.SeekStart); err != nil {
			return
		}
	}
}

// mp4ReadProperties mirrors Rust's mp4::read_properties: the duration comes
// from the audio track's mdhd, the stream info from its stsd entry, and the
// bitrate from mdat size + stts.
func mp4ReadProperties(reader io.ReadSeeker, traks []*mp4BoxHeader, fileLen, mdatLen uint64, parseOptions meta.ParseOptions, p *Mp4Properties) error {
	var mdhd, minf *mp4BoxHeader
	// Search all traks for the first audio track (hdlr == "soun")
	for _, trak := range traks {
		mdhd, minf = nil, nil
		isAudio := false
		var mdia *mp4BoxHeader
		mp4Children(reader, trak, func(c *mp4BoxHeader) bool {
			if string(c.name[:]) == "mdia" {
				mdia = c
				return false
			}
			return true
		})
		if mdia == nil {
			continue
		}
		mp4Children(reader, mdia, func(c *mp4BoxHeader) bool {
			switch string(c.name[:]) {
			case "mdhd":
				mdhd = c
			case "minf":
				minf = c
			case "hdlr":
				if c.size >= 12 {
					var hb [12]byte
					if _, err := reader.Seek(c.pos, io.SeekStart); err == nil {
						if _, err := io.ReadFull(reader, hb[:]); err == nil && string(hb[8:]) == "soun" {
							isAudio = true
						}
					}
				}
			}
			return true
		})
		if isAudio {
			break
		}
	}
	if mdhd == nil {
		return &Mp4ParseError{Message: "file contains no audio tracks"}
	}

	// mdhd: fullbox, then timescale + duration
	if _, err := reader.Seek(mdhd.pos, io.SeekStart); err != nil {
		return &Mp4ParseError{Source: err}
	}
	var vb [4]byte
	if _, err := io.ReadFull(reader, vb[:]); err != nil {
		return &Mp4ParseError{Source: err}
	}
	var timescale, duration uint64
	if vb[0] == 1 {
		var fb [24]byte
		if _, err := io.ReadFull(reader, fb[:]); err != nil {
			return &Mp4ParseError{Source: err}
		}
		timescale = uint64(binary.BigEndian.Uint32(fb[16:]))
		duration = binary.BigEndian.Uint64(fb[16+4:])
	} else {
		var fb [16]byte
		if _, err := io.ReadFull(reader, fb[:]); err != nil {
			return &Mp4ParseError{Source: err}
		}
		timescale = uint64(binary.BigEndian.Uint32(fb[8:]))
		duration = uint64(binary.BigEndian.Uint32(fb[12:]))
	}
	if timescale > 0 {
		p.Duration = time.Duration(util.DivRound(duration*1000, timescale)) * time.Millisecond
	}
	if minf == nil {
		return nil
	}

	// minf → stbl → stsd + stts
	var stbl *mp4BoxHeader
	mp4Children(reader, minf, func(c *mp4BoxHeader) bool {
		if string(c.name[:]) == "stbl" {
			stbl = c
			return false
		}
		return true
	})
	if stbl == nil {
		return nil
	}
	var stsdData []byte
	sttsPresent, sttsSpecifies := false, false
	mp4Children(reader, stbl, func(c *mp4BoxHeader) bool {
		switch string(c.name[:]) {
		case "stsd":
			var aerr error
			if stsdData, aerr = util.TryByteSlice(int(c.size)); aerr == nil {
				if _, err := reader.Seek(c.pos, io.SeekStart); err != nil {
					stsdData = nil
				} else if _, err := io.ReadFull(reader, stsdData); err != nil {
					stsdData = nil
				}
			}
		case "stts":
			sttsPresent = true
			var fb [8]byte
			if _, err := reader.Seek(c.pos, io.SeekStart); err == nil {
				if _, err := io.ReadFull(reader, fb[:]); err == nil {
					count := binary.BigEndian.Uint32(fb[4:])
					if count == 1 {
						var eb [8]byte
						if _, err := io.ReadFull(reader, eb[:]); err == nil && binary.BigEndian.Uint32(eb[4:]) == 1 {
							sttsSpecifies = false
							return true
						}
					}
					sttsSpecifies = count > 0
				}
			}
		}
		return true
	})
	if stsdData == nil {
		return nil
	}
	if err := mp4ParseStsd(stsdData, p); err != nil {
		return err
	}

	if duration > 0 {
		if mdatLen == 0 {
			// Rust: `mdat_length` errors in strict mode, warns otherwise
			if parseOptions.ParsingMode == meta.ParsingModeStrict {
				return &Mp4ParseError{Message: "no \"mdat\" atom found"}
			}
			return nil
		}
		if sttsPresent && sttsSpecifies {
			// Accurate audio bitrate: mdat*8*timescale/duration (bps) → kbps
			p.AudioBitrate = uint32(mdatLen * 8 * timescale / duration / 1000)
		}
		durationMs := uint64(p.Duration.Milliseconds())
		if durationMs == 0 {
			return nil
		}
		p.OverallBitrate = uint32(fileLen * 8 / durationMs)
		if p.AudioBitrate == 0 {
			p.AudioBitrate = uint32(mdatLen * 8 / durationMs)
		}
	}
	return nil
}

var mp4SampleRates = [15]uint32{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350, 0, 0}

// mp4Descriptor reads an MPEG-4 descriptor tag byte + 1-4 length bytes.
func mp4Descriptor(r *bytes.Reader) (byte, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	for i := 0; i < 4; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b&0x80 == 0 {
			break
		}
	}
	return tag, nil
}

// mp4ParseStsd parses the stsd payload for channels/rate/bit-depth and
// codec-specific properties, mirroring Rust's read_stsd.
func mp4ParseStsd(stsd []byte, p *Mp4Properties) error {
	r := bytes.NewReader(stsd)
	if _, err := r.Seek(4, io.SeekCurrent); err != nil {
		return &Mp4ParseError{Source: err}
	}
	var cb [4]byte
	if _, err := io.ReadFull(r, cb[:]); err != nil {
		return &Mp4ParseError{Source: err}
	}
	count := binary.BigEndian.Uint32(cb[:])
	for i := uint32(0); i < count; i++ {
		entry, err := mp4ReadBox(r)
		if err != nil || entry == nil {
			return &Mp4ParseError{Source: err, Message: "expected sample entry atom"}
		}
		if entry.hdr == 16 {
			return &Mp4ParseError{Message: "extended atoms are not supported in `stsd`"}
		}
		format := string(entry.name[:])
		if _, err := r.Seek(8, io.SeekCurrent); err != nil {
			return &Mp4ParseError{Source: err}
		}
		var vb [2]byte
		if _, err := io.ReadFull(r, vb[:]); err != nil {
			return &Mp4ParseError{Source: err}
		}
		stsdVersion := binary.BigEndian.Uint16(vb[:])
		if stsdVersion > 2 {
			return &Mp4ParseError{Message: "unsupported `stsd` version"}
		}
		if _, err := r.Seek(6, io.SeekCurrent); err != nil {
			return &Mp4ParseError{Source: err}
		}
		var ch [2]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return &Mp4ParseError{Source: err}
		}
		p.Channels = uint8(binary.BigEndian.Uint16(ch[:]))
		if _, err := r.Seek(6, io.SeekCurrent); err != nil {
			return &Mp4ParseError{Source: err}
		}
		var sb [4]byte
		if _, err := io.ReadFull(r, sb[:]); err != nil {
			return &Mp4ParseError{Source: err}
		}
		p.SampleRate = binary.BigEndian.Uint32(sb[:]) >> 16
		switch stsdVersion {
		case 1:
			if _, err := r.Seek(16, io.SeekCurrent); err != nil {
				return &Mp4ParseError{Source: err}
			}
		case 2:
			var eb [4]byte
			if _, err := io.ReadFull(r, eb[:]); err != nil {
				return &Mp4ParseError{Source: err}
			}
			if _, err := r.Seek(32+int64(binary.BigEndian.Uint32(eb[:])), io.SeekCurrent); err != nil {
				return &Mp4ParseError{Source: err}
			}
		}
		entryEnd := entry.pos + int64(entry.size)
		switch format {
		case "mp4a":
			mp4aProperties(r, entryEnd, p)
		case "alac":
			mp4AlacProperties(r, int64(len(stsd)), p)
		case "fLaC":
			mp4FlacProperties(r, p)
		case "drms":
			p.IsDrmProtected = true
			continue
		default:
			continue
		}
		// Only the first stream we can recognize
		break
	}
	return nil
}

// mp4aProperties parses the esds descriptor inside an mp4a sample entry.
func mp4aProperties(r *bytes.Reader, entryEnd int64, p *Mp4Properties) {
	p.Codec = Mp4CodecAAC
	esds, err := mp4ReadBox(r)
	if err != nil || string(esds.name[:]) != "esds" {
		return
	}
	var vb [4]byte
	if _, err := io.ReadFull(r, vb[:]); err != nil || binary.BigEndian.Uint32(vb[:]) != 0 {
		return
	}
	tag, err := mp4Descriptor(r)
	if err != nil || tag != 0x03 {
		return
	}
	// Elementary stream ID (2) + flags (1)
	if _, err := r.Seek(3, io.SeekCurrent); err != nil {
		return
	}
	tag, err = mp4Descriptor(r)
	if err != nil || tag != 0x04 {
		return
	}
	codec, err := r.ReadByte()
	if err != nil {
		return
	}
	switch codec {
	case 0x40, 0x41, 0x66, 0x67, 0x68:
		p.Codec = Mp4CodecAAC
	case 0x69, 0x6B:
		p.Codec = Mp4CodecMP3
	default:
		p.Codec = Mp4CodecNone
	}
	// Stream type (1) + buffer size (3) + max bitrate (4)
	if _, err := r.Seek(8, io.SeekCurrent); err != nil {
		return
	}
	var avg uint32
	if err := binary.Read(r, binary.BigEndian, &avg); err != nil {
		return
	}
	tag, err = mp4Descriptor(r)
	if err == nil && tag == 0x05 {
		var ab [2]byte
		if _, err := io.ReadFull(r, ab[:]); err == nil {
			objectType := ab[0] >> 3
			frequencyIndex := ((ab[0] & 0x07) << 1) | (ab[1] >> 7)
			var channelConf byte
			extended := false
			if objectType == 31 {
				extended = true
				objectType = 32 + ((ab[0] & 7) | (ab[1] >> 5))
				frequencyIndex = (ab[1] >> 1) & 0x0F
			}
			p.ExtendedAudioObjectType = meta.Some(AudioObjectType(objectType))
			switch {
			case frequencyIndex == 0x0F:
				var rate24 [3]byte
				if _, err := io.ReadFull(r, rate24[:]); err == nil {
					explicit := uint32(rate24[0])<<16 | uint32(rate24[1])<<8 | uint32(rate24[2])
					var sampleRate uint32
					if extended {
						sampleRate = explicit >> 1
						channelConf = uint8((explicit >> 4) & 0x0F)
					} else {
						sampleRate = explicit << 1
						var c [1]byte
						if _, err := io.ReadFull(r, c[:]); err == nil {
							channelConf = (uint8(explicit&0x80) | (c[0] >> 1)) & 0x0F
						}
					}
					if sampleRate > 0 {
						p.SampleRate = sampleRate
					}
				}
			case int(frequencyIndex) < len(mp4SampleRates):
				p.SampleRate = mp4SampleRates[frequencyIndex]
				if extended {
					var c [1]byte
					if _, err := io.ReadFull(r, c[:]); err == nil {
						channelConf = (ab[1] & 1) | (c[0] & 0xE0)
					}
				} else {
					channelConf = (ab[1] >> 3) & 0x0F
				}
			}
			if channelConf > 0 {
				p.Channels = channelConf
			}
			if objectType == 36 {
				var ident [5]byte
				if _, err := io.ReadFull(r, ident[:]); err == nil &&
					bytes.Equal(ident[:], []byte{0, 'A', 'L', 'S', 0}) {
					var rate uint32
					if err := binary.Read(r, binary.BigEndian, &rate); err == nil {
						p.SampleRate = rate
					}
					if _, err := r.Seek(4, io.SeekCurrent); err == nil {
						var ch uint16
						if err := binary.Read(r, binary.BigEndian, &ch); err == nil {
							p.Channels = uint8(ch) + 1
						}
					}
				}
			}
		}
	}
	if avg > 0 || p.Duration == 0 {
		p.AudioBitrate = avg / 1000
	}
	_ = entryEnd
}

// mp4AlacProperties parses the inner "alac" box of an alac sample entry.
func mp4AlacProperties(r *bytes.Reader, stsdLen int64, p *Mp4Properties) {
	// The entire stsd buffer must be exactly 80 bytes
	if stsdLen != 80 {
		return
	}
	alac, err := mp4ReadBox(r)
	if err != nil || string(alac.name[:]) != "alac" {
		return
	}
	p.Codec = Mp4CodecALAC
	// Version (4) + samples per frame (4) + compatible version (1)
	if _, err := r.Seek(9, io.SeekCurrent); err != nil {
		return
	}
	sampleSize, err := r.ReadByte()
	if err != nil {
		return
	}
	p.BitDepth = sampleSize
	// Rice history mult + initial history + parameter limit (3)
	if _, err := r.Seek(3, io.SeekCurrent); err != nil {
		return
	}
	ch, err := r.ReadByte()
	if err != nil {
		return
	}
	p.Channels = ch
	// Max run (2) + max frame size (4)
	if _, err := r.Seek(6, io.SeekCurrent); err != nil {
		return
	}
	var br, rate uint32
	if err := binary.Read(r, binary.BigEndian, &br); err != nil {
		return
	}
	p.AudioBitrate = br / 1000
	if err := binary.Read(r, binary.BigEndian, &rate); err != nil {
		return
	}
	p.SampleRate = rate
}

// mp4FlacProperties parses the dfLa STREAMINFO of a fLaC sample entry.
func mp4FlacProperties(r *bytes.Reader, p *Mp4Properties) {
	p.Codec = Mp4CodecFLAC
	dfla, err := mp4ReadBox(r)
	if err != nil || string(dfla.name[:]) != "dfLa" {
		return
	}
	// Version (1) + flags (3)
	if _, err := r.Seek(4, io.SeekCurrent); err != nil {
		return
	}
	if int64(dfla.size)-4 < 22 {
		return
	}
	// 4-byte FLAC metadata block header (last-block flag, type, 24-bit len)
	if _, err := r.Seek(4, io.SeekCurrent); err != nil {
		return
	}
	// STREAMINFO: 10 bytes of block/frame sizes, then packed rate/channels/bps
	var si [18]byte
	if _, err := io.ReadFull(r, si[:]); err != nil {
		return
	}
	packed := binary.BigEndian.Uint64(si[10:])
	p.SampleRate = uint32(packed >> 44)
	p.Channels = uint8((packed>>41)&0x7) + 1
	p.BitDepth = uint8((packed>>36)&0x1F) + 1
}

// mp4WalkUdta descends udta → meta (fullbox) → ilst.
func mp4WalkUdta(reader io.ReadSeeker, end int64, parseOptions meta.ParseOptions) (*Ilst, error) {
	for {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		if pos >= end {
			return nil, nil
		}
		h, err := mp4ReadBox(reader)
		if err != nil || h == nil {
			return nil, err
		}
		boxEnd := h.pos + int64(h.size)
		if string(h.name[:]) == "meta" {
			// meta is normally a fullbox (4-byte version+flags), but some
			// encoders write it as a plain container — Rust `meta_is_full`
			// decides by the first child ident.
			if mp4MetaIsFull(reader, h) {
				if _, err := reader.Seek(4, io.SeekCurrent); err != nil {
					return nil, err
				}
			}
			for {
				pos, _ := reader.Seek(0, io.SeekCurrent)
				if pos >= boxEnd {
					return nil, nil
				}
				mh, err := mp4ReadBox(reader)
				if err != nil || mh == nil {
					return nil, err
				}
				if string(mh.name[:]) == "ilst" {
					return ParseIlst(reader, mh.pos+int64(mh.size), parseOptions)
				}
				if _, err := reader.Seek(mh.pos+int64(mh.size), io.SeekStart); err != nil {
					return nil, err
				}
			}
		}
		if _, err := reader.Seek(boxEnd, io.SeekStart); err != nil {
			return nil, err
		}
	}
}

// FileType implements meta.AudioFile.
func (f *Mp4File) FileType() meta.FileType { return meta.FileTypeMp4 }

// GetProperties implements meta.AudioFile.
func (f *Mp4File) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *Mp4File) ContainsTag() bool { return f.Ilst.IsSome() }

// ContainsTagType implements meta.AudioFile.
func (f *Mp4File) ContainsTagType(t meta.TagType) bool {
	return t == meta.TagTypeMp4Ilst && f.Ilst.IsSome()
}

// TaggedFile converts to a [meta.TaggedFile].
func (f *Mp4File) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.Ilst.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeMp4, tags, *f.GetProperties())
}

// SavePath implements meta.AudioFile.
func (f *Mp4File) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, meta.TagTypeMp4Ilst, wo)
}

// SaveFile implements meta.AudioFile.
func (f *Mp4File) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	ilst, ok := f.Ilst.Get()
	if !ok {
		return nil
	}
	return mp4WriteIlstData(file, ilst, wo)
}

// RemoveFromPath implements meta.AudioFile.
func (f *Mp4File) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

// mp4UpdateBoxSize adjusts a box's size field after its payload changed by
// delta. Handles both normal (32-bit) and largesize (64-bit) boxes. Unlike
// Rust (which promotes to an extended-size box), a normal box that would
// exceed u32 is an error rather than a silent truncation.
func mp4UpdateBoxSize(file meta.FileLike, box *mp4BoxHeader, delta int64) error {
	newTotal := int64(box.size) + delta + int64(box.hdr)
	if box.hdr == 16 {
		if _, err := file.Seek(box.pos-8, io.SeekStart); err != nil {
			return err
		}
		var sz [8]byte
		binary.BigEndian.PutUint64(sz[:], uint64(newTotal))
		_, err := file.Write(sz[:])
		return err
	}
	if newTotal > math.MaxUint32 {
		return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
	}
	if _, err := file.Seek(box.pos-int64(box.hdr), io.SeekStart); err != nil {
		return err
	}
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(newTotal))
	_, err := file.Write(sz[:])
	return err
}

// mp4OffsetUpdateContainers are the container atoms Rust treats as important
// when adjusting chunk offsets after a size shift (write.rs
// `IMPORTANT_CONTAINERS`): only atoms nested inside these are found.
var mp4OffsetUpdateContainers = map[string]bool{
	"moov": true, "udta": true, "moof": true, "trak": true,
	"mdia": true, "minf": true, "stbl": true,
}

// mp4UpdateChunkOffsets shifts every `stco`/`co64` chunk offset inside `moov`
// and every `moof` `tfhd` base data offset at or beyond shiftPos by delta —
// Rust's `AtomWriterCompanion::update_offsets`. The file is walked post-edit,
// so box positions already reflect the shift; only the stored offsets are
// adjusted.
func mp4UpdateChunkOffsets(file meta.FileLike, moov, moof *mp4BoxHeader, shiftPos, delta int64) error {
	if delta == 0 {
		return nil
	}
	if moov != nil {
		// moov's own start always precedes the edit point; its end grew
		moovEnd := moov.pos + int64(moov.size) + delta
		if err := mp4WalkOffsetAtoms(file, moov.pos, moovEnd, false, shiftPos, delta); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	if moof != nil {
		// moof is top-level — its position may have shifted with the edit
		moofStart := moof.pos
		if moofStart >= shiftPos {
			moofStart += delta
		}
		if err := mp4WalkOffsetAtoms(file, moofStart, moofStart+int64(moof.size), true, shiftPos, delta); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	return nil
}

// mp4WalkOffsetAtoms recursively walks container children between start and
// end, adjusting offset atoms it finds.
func mp4WalkOffsetAtoms(file meta.FileLike, start, end int64, tfhdMode bool, shiftPos, delta int64) error {
	for pos := start; pos+8 <= end; {
		if _, err := file.Seek(pos, io.SeekStart); err != nil {
			return err
		}
		h, err := mp4ReadBox(file)
		if err != nil {
			// Rust's atom tree only contains atoms it could parse — an
			// unreadable child just ends the walk, it doesn't fail.
			break
		}
		atomEnd := h.pos + int64(h.size)
		if atomEnd > end || int64(h.size) < 0 {
			break
		}
		name := string(h.name[:])
		var adjErr error
		switch {
		case !tfhdMode && name == "stco":
			adjErr = mp4ShiftStco(file, h, shiftPos, delta)
		case !tfhdMode && name == "co64":
			adjErr = mp4ShiftCo64(file, h, shiftPos, delta)
		case tfhdMode && name == "tfhd":
			adjErr = mp4ShiftTfhd(file, h, shiftPos, delta)
		}
		if adjErr != nil {
			return adjErr
		}
		if mp4OffsetUpdateContainers[name] {
			if err := mp4WalkOffsetAtoms(file, h.pos, atomEnd, tfhdMode, shiftPos, delta); err != nil {
				return err
			}
		}
		pos = atomEnd
	}
	return nil
}

// mp4ShiftStco updates the u32 chunk offsets inside an stco atom.
func mp4ShiftStco(file meta.FileLike, h *mp4BoxHeader, shiftPos, delta int64) error {
	// version (1) + flags (3) + entry count (4) + u32 entries
	count, err := mp4ReadU32At(file, h.pos+4)
	if err != nil {
		return err
	}
	maxEntries := int64(0)
	if h.size > 8 {
		maxEntries = (int64(h.size) - 8) / 4
	}
	if int64(count) < maxEntries {
		maxEntries = int64(count)
	}
	for i := int64(0); i < maxEntries; i++ {
		entryPos := h.pos + 8 + i*4
		off, err := mp4ReadU32At(file, entryPos)
		if err != nil {
			return err
		}
		if int64(off) < shiftPos {
			continue
		}
		if err := mp4WriteU32At(file, entryPos, uint32(int64(off)+delta)); err != nil {
			return err
		}
	}
	return nil
}

// mp4ShiftCo64 updates the u64 chunk offsets inside a co64 atom.
func mp4ShiftCo64(file meta.FileLike, h *mp4BoxHeader, shiftPos, delta int64) error {
	count, err := mp4ReadU32At(file, h.pos+4)
	if err != nil {
		return err
	}
	maxEntries := int64(0)
	if h.size > 8 {
		maxEntries = (int64(h.size) - 8) / 8
	}
	if int64(count) < maxEntries {
		maxEntries = int64(count)
	}
	for i := int64(0); i < maxEntries; i++ {
		entryPos := h.pos + 8 + i*8
		var b [8]byte
		if _, err := file.Seek(entryPos, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.ReadFull(file, b[:]); err != nil {
			return err
		}
		off := binary.BigEndian.Uint64(b[:])
		if int64(off) < shiftPos {
			continue
		}
		binary.BigEndian.PutUint64(b[:], uint64(int64(off)+delta))
		if _, err := file.Seek(entryPos, io.SeekStart); err != nil {
			return err
		}
		if _, err := file.Write(b[:]); err != nil {
			return err
		}
	}
	return nil
}

// mp4ShiftTfhd updates a tfhd atom's base data offset when present.
// Rust reads the u64 immediately after the 3 flag bytes (payload offset 4),
// which overlaps track_ID — mirrored verbatim. (`traf` is not an important
// container, so only a direct moof.tfhd child would ever be found.)
func mp4ShiftTfhd(file meta.FileLike, h *mp4BoxHeader, shiftPos, delta int64) error {
	// version (1) + flags (3) — flag bit 0x1 signals a base_data_offset u64
	if h.size < 12 {
		return nil
	}
	var b [4]byte
	if _, err := file.Seek(h.pos, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.ReadFull(file, b[:]); err != nil {
		return err
	}
	if b[3]&0b1 == 0 {
		return nil
	}
	entryPos := h.pos + 4
	var ob [8]byte
	if _, err := io.ReadFull(file, ob[:]); err != nil {
		return err
	}
	off := binary.BigEndian.Uint64(ob[:])
	if int64(off) < shiftPos {
		return nil
	}
	binary.BigEndian.PutUint64(ob[:], uint64(int64(off)+delta))
	if _, err := file.Seek(entryPos, io.SeekStart); err != nil {
		return err
	}
	_, err := file.Write(ob[:])
	return err
}

func mp4ReadU32At(file meta.FileLike, pos int64) (uint32, error) {
	var b [4]byte
	if _, err := file.Seek(pos, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(file, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

func mp4WriteU32At(file meta.FileLike, pos int64, v uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	if _, err := file.Seek(pos, io.SeekStart); err != nil {
		return err
	}
	_, err := file.Write(b[:])
	return err
}

// mp4FreeAtom builds a `free` padding atom of `size` bytes (header included).
// Rust `write_free_atom` pads with 0x01 bytes.
func mp4FreeAtom(size uint32) []byte {
	b := make([]byte, size)
	binary.BigEndian.PutUint32(b[:4], size)
	copy(b[4:8], "free")
	for i := 8; i < len(b); i++ {
		b[i] = 1
	}
	return b
}

// mp4HdlrAtom is the `hdlr` atom written inside `meta` when creating a new
// udta/meta subtree (moov.udta.meta.hdlr).
var mp4HdlrAtom = []byte{
	0, 0, 0, 33,
	'h', 'd', 'l', 'r',
	0, 0, 0, 0, // version + flags
	0, 0, 0, 0, // predefined
	'm', 'd', 'i', 'r', // component subtype
	'a', 'p', 'p', 'l', // component manufacturer
	0, 0, 0, 0, 0, 0, 0, 0, 0, // flags/flags mask/name reserved
}

// mp4WriteIlst replaces the ilst atom (or adds one under moov/udta/meta).
func mp4WriteIlst(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	return mp4WriteIlstData(file, IlstFromTag(tag), writeOptions)
}

// memFileLike is an in-memory [meta.FileLike] used to stage MP4 writes: the whole
// file is edited in a buffer (like Rust's `AtomWriter` Cursor) and flushed
// back in a single write, so a mid-write error can't corrupt the file.
type memFileLike struct {
	buf []byte
	pos int64
}

func (m *memFileLike) Read(p []byte) (int, error) {
	if m.pos >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[m.pos:])
	m.pos += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memFileLike) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		nb := make([]byte, end)
		copy(nb, m.buf)
		m.buf = nb
	}
	n := copy(m.buf[m.pos:], p)
	m.pos += int64(n)
	return n, nil
}

func (m *memFileLike) Seek(off int64, whence int) (int64, error) {
	var n int64
	switch whence {
	case io.SeekStart:
		n = off
	case io.SeekCurrent:
		n = m.pos + off
	case io.SeekEnd:
		n = int64(len(m.buf)) + off
	}
	if n < 0 {
		return m.pos, fmt.Errorf("mp4: negative seek position %d", n)
	}
	m.pos = n
	return n, nil
}

func (m *memFileLike) Truncate(size int64) error {
	if size < int64(len(m.buf)) {
		m.buf = m.buf[:size]
	} else {
		m.buf = append(m.buf, make([]byte, size-int64(len(m.buf)))...)
	}
	if m.pos > size {
		m.pos = size
	}
	return nil
}

func (m *memFileLike) Len() (int64, error) { return int64(len(m.buf)), nil }

// mp4WriteIlstData is the concrete `Ilst` variant of [mp4WriteIlst].
func mp4WriteIlstData(file meta.FileLike, ilst *Ilst, writeOptions meta.WriteOptions) error {
	// Stage the whole file in memory — Rust performs all fallible work on a
	// Cursor and only writes back once the new layout is complete.
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	buf, err := util.TryByteSlice(int(end))
	if err == nil {
		_, err = io.ReadFull(file, buf)
	}
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	mem := &memFileLike{buf: buf}
	if err := mp4WriteIlstDataInner(mem, ilst, writeOptions); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if _, err := file.Write(mem.buf); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return file.Truncate(int64(len(mem.buf)))
}

func mp4WriteIlstDataInner(file meta.FileLike, ilst *Ilst, writeOptions meta.WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	streamEnd, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	// Find moov (+ moof for tfhd offset updates), then ilst via udta/meta
	var moov, moof *mp4BoxHeader
	for {
		pos, _ := file.Seek(0, io.SeekCurrent)
		if pos >= streamEnd {
			break
		}
		h, err := mp4ReadBox(file)
		if err != nil || h == nil {
			break
		}
		switch string(h.name[:]) {
		case "moov":
			moov = h
		case "moof":
			moof = h
		}
		if err := mp4SkipBox(file, h); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	if moov == nil {
		return &meta.FileEncodingError{Source: &Mp4ParseError{Message: "no moov box"}}
	}

	// Encode the ilst atom
	ilstData, err := encodeIlst(ilst)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeMp4Ilst, HasTagType: true, Source: err}}
	}

	// Find udta/meta/ilst inside moov
	if _, err := file.Seek(moov.pos, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	moovEnd := moov.pos + int64(moov.size)
	var udtaBox *mp4BoxHeader
	for {
		pos, _ := file.Seek(0, io.SeekCurrent)
		if pos >= moovEnd {
			break
		}
		h, err := mp4ReadBox(file)
		if err != nil || h == nil {
			break
		}
		if string(h.name[:]) == "udta" {
			udtaBox = h
		}
		if _, err := file.Seek(h.pos+int64(h.size), io.SeekStart); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}

	if udtaBox == nil {
		// Create udta/meta/ilst at the end of moov
		if len(ilstData) == 0 {
			return nil
		}
		var udta bytes.Buffer
		udta.WriteString("udta")
		var metaBox bytes.Buffer
		metaBox.WriteString("meta")
		metaBox.Write([]byte{0, 0, 0, 0}) // fullbox header
		metaBox.Write(mp4HdlrAtom)
		metaBox.Write(ilstData)
		var metaHdr [4]byte
		binary.BigEndian.PutUint32(metaHdr[:], uint32(metaBox.Len()+4))
		udta.Write(metaHdr[:])
		udta.Write(metaBox.Bytes())
		var udtaHdr [4]byte
		binary.BigEndian.PutUint32(udtaHdr[:], uint32(udta.Len()+4))
		var out bytes.Buffer
		out.Write(udtaHdr[:])
		out.Write(udta.Bytes())
		// Rust puts a created `udta` at the start of `moov` (data_start)
		insertPos := moov.pos
		if err := meta.Splice(file, uint64(insertPos), uint64(insertPos), out.Bytes()); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		// Fix moov size
		if err := mp4UpdateBoxSize(file, moov, int64(out.Len())); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		return mp4UpdateChunkOffsets(file, moov, moof, insertPos, int64(out.Len()))
	}

	// udta exists: find meta inside it
	if _, err := file.Seek(udtaBox.pos, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	udtaEnd := udtaBox.pos + int64(udtaBox.size)
	var metaBox, ilstBox *mp4BoxHeader
	var metaChildren []*mp4BoxHeader
	for {
		pos, _ := file.Seek(0, io.SeekCurrent)
		if pos >= udtaEnd {
			break
		}
		h, err := mp4ReadBox(file)
		if err != nil || h == nil {
			break
		}
		if string(h.name[:]) == "meta" {
			metaBox = h
			// A `meta` atom is usually "full" (version+flags precede its
			// children), but a non-full `meta` nests children immediately
			// (Rust `meta_is_full`).
			if mp4MetaIsFull(file, h) {
				if _, err := file.Seek(4, io.SeekCurrent); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
			}
			metaEnd := h.pos + int64(h.size)
			for {
				pos, _ := file.Seek(0, io.SeekCurrent)
				if pos >= metaEnd {
					break
				}
				mh, err := mp4ReadBox(file)
				if err != nil || mh == nil {
					break
				}
				metaChildren = append(metaChildren, mh)
				if string(mh.name[:]) == "ilst" {
					ilstBox = mh
				}
				if _, err := file.Seek(mh.pos+int64(mh.size), io.SeekStart); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
			}
		}
		if _, err := file.Seek(h.pos+int64(h.size), io.SeekStart); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}

	if ilstBox != nil {
		// The replace range covers the `ilst` plus any `free` atoms
		// directly adjacent to it inside `meta` (Rust `find_ilst`).
		ilstIdx := -1
		for i, ch := range metaChildren {
			if ch == ilstBox {
				ilstIdx = i
				break
			}
		}
		rangeStart := ilstBox.pos - int64(ilstBox.hdr)
		rangeEnd := ilstBox.pos + int64(ilstBox.size)
		// Rust only expands the range over adjacent `free` atoms when
		// writing a tag — removal leaves the padding in place.
		if len(ilstData) > 0 && ilstIdx >= 0 {
			for i := ilstIdx - 1; i >= 0; i-- {
				ch := metaChildren[i]
				if string(ch.name[:]) != "free" || ch.pos+int64(ch.size) != rangeStart {
					break
				}
				rangeStart = ch.pos - int64(ch.hdr)
			}
			for i := ilstIdx + 1; i < len(metaChildren); i++ {
				ch := metaChildren[i]
				if string(ch.name[:]) != "free" || ch.pos-int64(ch.hdr) != rangeEnd {
					break
				}
				rangeEnd = ch.pos + int64(ch.size)
			}
		}

		availableSpace := rangeEnd - rangeStart
		if len(ilstData) == 0 {
			// Removal: with preferred padding the old `ilst` becomes a
			// same-size `free` atom so nothing shifts
			// (Rust `save_to_existing`).
			if writeOptions.PreferredPadding > 0 && availableSpace > 8 {
				ilstData = mp4FreeAtom(uint32(availableSpace))
			}
		} else if diff := availableSpace - int64(len(ilstData)); diff >= 8 {
			// Shrink by >= 8: a `free` atom fills the gap so the ancestor
			// sizes (and chunk offsets) stay put — Rust's `pad_atom`.
			ilstData = append(ilstData, mp4FreeAtom(uint32(diff))...)
		} else if diff > 0 && writeOptions.PreferredPadding > 0 {
			// Shrink by < 8 with padding enabled: a `free` atom of the
			// preferred size is appended and offsets do shift.
			ilstData = append(ilstData, mp4FreeAtom(writeOptions.PreferredPadding)...)
		}

		if err := meta.Splice(file, uint64(rangeStart), uint64(rangeEnd), ilstData); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		// Fix ancestor sizes
		delta := int64(len(ilstData)) - availableSpace
		if delta != 0 {
			for _, box := range []*mp4BoxHeader{metaBox, udtaBox, moov} {
				if box == nil {
					continue
				}
				if err := mp4UpdateBoxSize(file, box, delta); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
			}
		}
		return mp4UpdateChunkOffsets(file, moov, moof, rangeStart, delta)
	}
	if len(ilstData) == 0 {
		return nil
	}
	if metaBox != nil {
		// Insert ilst at end of meta
		insertPos := metaBox.pos + int64(metaBox.size)
		if err := meta.Splice(file, uint64(insertPos), uint64(insertPos), ilstData); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		delta := int64(len(ilstData))
		for _, box := range []*mp4BoxHeader{metaBox, udtaBox, moov} {
			if err := mp4UpdateBoxSize(file, box, delta); err != nil {
				return &meta.FileEncodingError{Source: err}
			}
		}
		return mp4UpdateChunkOffsets(file, moov, moof, insertPos, delta)
	}
	// udta exists without meta: create meta/ilst inside it
	var metaOut bytes.Buffer
	metaOut.WriteString("meta")
	metaOut.Write([]byte{0, 0, 0, 0}) // fullbox header
	metaOut.Write(mp4HdlrAtom)
	metaOut.Write(ilstData)
	var metaHdr [4]byte
	binary.BigEndian.PutUint32(metaHdr[:], uint32(metaOut.Len()+4))
	var out bytes.Buffer
	out.Write(metaHdr[:])
	out.Write(metaOut.Bytes())
	// Rust puts a created `meta` at the start of `udta` (data_start)
	insertPos := udtaBox.pos
	if err := meta.Splice(file, uint64(insertPos), uint64(insertPos), out.Bytes()); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	delta := int64(out.Len())
	for _, box := range []*mp4BoxHeader{udtaBox, moov} {
		if err := mp4UpdateBoxSize(file, box, delta); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	return mp4UpdateChunkOffsets(file, moov, moof, insertPos, delta)
}

// EncodeMp4Ilst encodes a generic tag as an ilst atom (includes the box header).
func EncodeMp4Ilst(tag *meta.Tag) ([]byte, error) {
	return encodeIlst(IlstFromTag(tag))
}

// mp4AtomName converts a raw 4-byte atom name to a lookup key, translating
// the 0xA9 byte to U+00A9 (©).
func mp4AtomName(raw [4]byte) string {
	out := make([]rune, 0, 4)
	for _, b := range raw {
		if b == 0xA9 {
			out = append(out, '©')
		} else {
			out = append(out, rune(b))
		}
	}
	return string(out)
}

// mp4NameFromKey converts a UTF-8 ilst key back to a 4-byte atom name,
// translating U+00A9 to the single byte 0xA9.
func mp4NameFromKey(key string) string {
	out := make([]byte, 0, 4)
	for _, r := range key {
		switch {
		case r == '©':
			out = append(out, 0xA9)
		case r < 0x100:
			out = append(out, byte(r))
		}
	}
	return string(out)
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeMp4, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := Mp4FileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterFormatWriter(meta.FileTypeMp4, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		if tag.TagType() != meta.TagTypeMp4Ilst {
			return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
		}
		return mp4WriteIlst(file.File(), tag, writeOptions)
	})
	meta.RegisterTagDumper(meta.TagTypeMp4Ilst, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		data, err := EncodeMp4Ilst(tag)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}
