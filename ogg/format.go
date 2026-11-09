package ogg

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
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

const (
	oggPageContinuation byte = 1
	oggPageBOS          byte = 2
	oggPageEOS          byte = 4
)

// readOggPage reads one page at the current position.
func readOggPage(reader io.ReadSeeker) (*OggPage, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	var hdr [27]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, err
	}
	if !bytes.Equal(hdr[:4], oggPreamble) {
		return nil, &OggParseError{Source: &FakeTagError{}}
	}
	if hdr[4] != 0 {
		return nil, &OggParseError{Message: "unsupported Ogg page version"}
	}
	p := &OggPage{
		headerType: hdr[5],
		granulePos: binary.LittleEndian.Uint64(hdr[6:]),
		serial:     binary.LittleEndian.Uint32(hdr[14:]),
		sequence:   binary.LittleEndian.Uint32(hdr[18:]),
		crc:        binary.LittleEndian.Uint32(hdr[22:]),
		start:      start,
	}
	nsegs := int(hdr[26])
	segs := make([]byte, nsegs)
	if _, err := io.ReadFull(reader, segs); err != nil {
		return nil, err
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

// oggPackets collects full packets from a reader for a given serial.
// Returns packets and the file offset of the end of the header packets region.
func oggPackets(reader io.ReadSeeker, serial uint32, count int) ([][]byte, int64, error) {
	var packets [][]byte
	var current []byte
	endPos := int64(0)
	for len(packets) < count {
		page, err := readOggPage(reader)
		if err != nil {
			return nil, 0, err
		}
		if page.serial != serial {
			continue
		}
		pos := 0
		continued := page.headerType&oggPageContinuation != 0
		if !continued && len(current) > 0 {
			packets = append(packets, current)
			current = nil
		}
		for _, s := range page.segments {
			current = append(current, page.data[pos:pos+int(s)]...)
			pos += int(s)
			if s < 255 {
				packets = append(packets, current)
				current = nil
				if len(packets) >= count {
					endPos = page.end
					return packets, endPos, nil
				}
			}
		}
	}
	return packets, endPos, nil
}

// oggFirstSerial returns the serial number of the first page's stream.
// oggFirstPage returns the serial and granule position of the first page.
func oggFirstPage(reader io.ReadSeeker) (serial uint32, granule uint64, err error) {
	page, err := readOggPage(reader)
	if err != nil {
		return 0, 0, err
	}
	return page.serial, page.granulePos, nil
}

// oggLastGranulePos finds the last page's granule position for duration.
func oggLastGranulePos(reader io.ReadSeeker, wantSerial uint32) (uint64, error) {
	end, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	// Scan the last 64KB for an OggS page
	scan := int64(65536)
	if scan > end {
		scan = end
	}
	if _, err := reader.Seek(end-scan, io.SeekStart); err != nil {
		return 0, err
	}
	buf, err := io.ReadAll(io.LimitReader(reader, scan))
	if err != nil {
		return 0, err
	}
	idx := bytes.LastIndex(buf, oggPreamble)
	for idx >= 0 {
		if idx+27 > len(buf) {
			break
		}
		hdr := buf[idx:]
		nsegs := int(hdr[26])
		if idx+27+nsegs > len(buf) {
			break
		}
		pageSerial := binary.LittleEndian.Uint32(hdr[14:])
		if pageSerial == wantSerial {
			return binary.LittleEndian.Uint64(hdr[6:]), nil
		}
		idx = bytes.LastIndex(buf[:idx], oggPreamble)
	}
	return 0, &OggParseError{Message: "no valid Ogg pages found"}
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
	ChannelMask Option[ChannelMask]
}

// FileProperties converts to generic [FileProperties].
func (p OggProperties) FileProperties() FileProperties {
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

// OggVorbisFile is an Ogg Vorbis file.
type OggVorbisFile struct {
	Properties     OggProperties
	VorbisComments Option[*VorbisCommentsTag]
}

// OggOpusFile is an Ogg Opus file.
type OggOpusFile struct {
	Properties     OggProperties
	VorbisComments Option[*VorbisCommentsTag]
}

// OggSpeexFile is an Ogg Speex file.
type OggSpeexFile struct {
	Properties     OggProperties
	VorbisComments Option[*VorbisCommentsTag]
}

// oggReadTagged reads an Ogg file: returns (properties, vorbis comments tag, file type).
func oggReadTagged(reader io.ReadSeeker, parseOptions ParseOptions) (*OggProperties, *VorbisCommentsTag, FileType, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: err}
	}
	streamLen := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: err}
	}

	serial, firstGranule, err := oggFirstPage(reader)
	if err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: &OggParseError{Source: err}}
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: err}
	}

	packets, _, err := oggPackets(reader, serial, 3)
	if err != nil {
		return nil, nil, FileType{}, &FileParseError{Source: &OggParseError{Source: err}}
	}
	if len(packets) < 2 {
		return nil, nil, FileType{}, &FileParseError{Source: &OggParseError{Message: "missing Ogg header packets"}}
	}

	props := &OggProperties{}
	var vc *VorbisCommentsTag
	var fType FileType
	var preSkip uint64

	ident := packets[0]
	switch {
	case len(ident) >= 7 && bytes.Equal(ident[1:7], []byte("vorbis")) && ident[0] == 1:
		fType = FileTypeVorbis
		// Identification header: vorbis_version(4) channels(1) rate(4) bitrate_max(4) bitrate_nom(4) bitrate_min(4)
		if len(ident) >= 30 {
			props.Version = binary.LittleEndian.Uint32(ident[7:])
			props.Channels = ident[11]
			props.SampleRate = binary.LittleEndian.Uint32(ident[12:])
			props.BitrateMax = int32(binary.LittleEndian.Uint32(ident[16:]))
			props.BitrateNominal = int32(binary.LittleEndian.Uint32(ident[20:]))
			props.BitrateMin = int32(binary.LittleEndian.Uint32(ident[24:]))
			if props.BitrateNominal > 0 {
				props.AudioBitrate = uint32(props.BitrateNominal) / 1000
			}
		}
		// Comment packet: 0x03 + "vorbis" + comments + framing
		comment := packets[1]
		if len(comment) >= 7 && comment[0] == 3 && bytes.Equal(comment[1:7], []byte("vorbis")) {
			if parseOptions.ReadTags {
				t, err := ParseVorbisComments(comment[7:], parseOptions)
				if err != nil {
					return nil, nil, FileType{}, &FileParseError{Source: &VorbisCommentsParseError{Source: err}}
				}
				vc = t
			}
		}
	case len(ident) >= 8 && bytes.Equal(ident[:8], []byte("OpusHead")):
		fType = FileTypeOpus
		// OpusHead: version(1) channels(1) preskip(2) rate(4) gain(2) mapping(1)
		if len(ident) >= 19 {
			props.Version = uint32(ident[8])
			props.Channels = ident[9]
			preSkip = uint64(binary.LittleEndian.Uint16(ident[10:]))
			props.InputSampleRate = binary.LittleEndian.Uint32(ident[12:])
			props.OutputGainDb = float32(int16(binary.LittleEndian.Uint16(ident[16:]))) / 256
			props.SampleRate = 48000 // Opus is always decoded at 48kHz
			props.ChannelMask = ChannelMaskFromOpusChannels(props.Channels)
		}
		comment := packets[1]
		if len(comment) >= 8 && bytes.Equal(comment[:8], []byte("OpusTags")) {
			if parseOptions.ReadTags {
				t, err := ParseVorbisComments(comment[8:], parseOptions)
				if err != nil {
					return nil, nil, FileType{}, &FileParseError{Source: &VorbisCommentsParseError{Source: err}}
				}
				vc = t
			}
		}
	case len(ident) >= 8 && bytes.Equal(ident[:8], []byte("Speex   ")):
		fType = FileTypeSpeex
		// Speex header: rate(4) at offset 36, channels(4) at 48, bitrate(4) at 52
		if len(ident) >= 80 {
			props.Version = binary.LittleEndian.Uint32(ident[28:])
			props.SampleRate = binary.LittleEndian.Uint32(ident[36:])
			props.Mode = binary.LittleEndian.Uint32(ident[40:])
			props.Channels = uint8(binary.LittleEndian.Uint32(ident[48:]))
			props.BitrateNominal = int32(binary.LittleEndian.Uint32(ident[52:]))
			props.Vbr = binary.LittleEndian.Uint32(ident[60:]) != 0
			if props.BitrateNominal > 0 {
				props.AudioBitrate = uint32(props.BitrateNominal) / 1000
			}
		}
		comment := packets[1]
		if parseOptions.ReadTags {
			t, err := ParseVorbisComments(comment, parseOptions)
			if err != nil {
				return nil, nil, FileType{}, &FileParseError{Source: &VorbisCommentsParseError{Source: err}}
			}
			vc = t
		}
	default:
		return nil, nil, FileType{}, &FileParseError{Source: &OggParseError{Message: "unsupported Ogg codec"}}
	}

	if parseOptions.ReadProperties {
		if granule, err := oggLastGranulePos(reader, serial); err == nil && props.SampleRate > 0 {
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
				lengthMs := divRound(total*1000, uint64(props.SampleRate))
				props.Duration = time.Duration(lengthMs) * time.Millisecond

				// Bitrate semantics are codec-specific (mirroring Rust):
				// overall is always fileLen*8/len; audio is the stream length
				// minus the two header packets, or the declared nominal rate.
				var headerLen uint64
				for _, p := range packets[:2] {
					headerLen += uint64(len(p))
				}
				var dataLen uint64
				if streamLen > headerLen {
					dataLen = streamLen - headerLen
				}
				switch fType {
				case FileTypeVorbis:
					props.OverallBitrate = uint32(streamLen * 8 / lengthMs)
					if props.BitrateNominal > 0 {
						props.AudioBitrate = uint32(uint64(props.BitrateNominal) / 1000)
					}
				case FileTypeOpus:
					props.OverallBitrate = uint32(streamLen * 8 / lengthMs)
					props.AudioBitrate = uint32(dataLen * 8 / lengthMs)
				case FileTypeSpeex:
					if props.BitrateNominal > 0 {
						props.OverallBitrate = uint32(streamLen * 8 / lengthMs)
						props.AudioBitrate = uint32(uint64(props.BitrateNominal) / 1000)
					} else {
						props.OverallBitrate = uint32(divRound(streamLen*8, lengthMs))
						props.AudioBitrate = uint32(divRound(dataLen*8, lengthMs))
					}
				}
			}
		}
	}
	return props, vc, fType, nil
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
	hdr[26] = byte(len(p.segments))
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
	var packets [][]byte
	var current []byte
	var remainder *OggPage
	var serial uint32
	for len(packets) < count {
		page, err := readOggPage(reader)
		if err != nil {
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
			if len(packets) == count {
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
	return packets, remainder, serial, nil
}

// oggPaginate turns packets into pages of at most 32 segments
// (mirroring ogg_pager::paginate).
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
		if len(packet) != 0 && len(packet)%(255*oggMaxWrittenSegmentCount) == 0 {
			packetFinished = true
			flush()
		}
	}
	return pages
}

// oggWriteVorbisComments replaces the comment packet in an Ogg file.
func oggWriteVorbisComments(file FileLike, ft FileType, tag *Tag, writeOptions WriteOptions) error {
	pics := make([]PictureInfoPair, 0, len(tag.Pictures()))
	for _, p := range tag.Pictures() {
		info, err := PictureInformationFromPicture(&p)
		if err != nil {
			info = PictureInformation{}
		}
		pics = append(pics, PictureInfoPair{Picture: p, Info: info})
	}
	return oggWriteVorbisCommentsPics(file, ft, tag, pics, writeOptions)
}

// oggWriteVorbisCommentsPics is [oggWriteVorbisComments] with explicit
// [PictureInformation] for each picture comment.
func oggWriteVorbisCommentsPics(file FileLike, ft FileType, tag *Tag, pics []PictureInfoPair, writeOptions WriteOptions) error {
	return oggWriteVorbisCommentsImpl(file, ft, writeOptions, func(vendor string) *VorbisCommentsTag {
		vc := VorbisCommentsTagForWritePics(tag, vendor, false)
		vc.SetPictures(pics)
		return vc
	})
}

// oggWriteVorbisCommentsImpl replaces the comment packet in an Ogg file. The
// `build` callback receives the file's retained vendor string.
func oggWriteVorbisCommentsImpl(file FileLike, ft FileType, writeOptions WriteOptions, build func(vendor string) *VorbisCommentsTag) error {
	var headerPacketCount int
	var sig []byte
	switch ft {
	case FileTypeVorbis:
		headerPacketCount, sig = 3, []byte{3, 'v', 'o', 'r', 'b', 'i', 's'}
	case FileTypeOpus:
		headerPacketCount, sig = 2, []byte("OpusTags")
	case FileTypeSpeex:
		headerPacketCount = 2
	default:
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	packets, remainder, serial, err := OggReadCountPackets(file, headerPacketCount)
	if err != nil {
		return &FileEncodingError{Source: &OggParseError{Source: err}}
	}
	restPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	// Everything after the consumed page(s) is copied verbatim
	rest, err := io.ReadAll(file)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	_ = restPos

	if len(packets) < 2 {
		return &FileEncodingError{Source: &OggParseError{Message: "missing Ogg header packets"}}
	}
	comment := packets[1]
	if sig != nil && (len(comment) < len(sig) || !bytes.Equal(comment[:len(sig)], sig)) {
		return &FileEncodingError{Source: &OggParseError{Source: &FakeTagError{}}}
	}

	// Retain the vendor string
	var vendor string
	if off := len(sig); len(comment) >= off+4 {
		vlen := int(binary.LittleEndian.Uint32(comment[off:]))
		if vlen <= len(comment)-off-4 {
			vendor = string(comment[off+4 : off+4+vlen])
		}
	}

	// Items are serialized verbatim (incl. EncoderSoftware → ENCODER); the
	// file's retained vendor is offered to the builder.
	vc := build(vendor)
	vcData, err := EncodeVorbisComments(vc)
	if err != nil {
		return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeVorbisComments, HasTagType: true, Source: err}}
	}
	newComment := append(append([]byte{}, sig...), vcData...)
	if ft == FileTypeVorbis {
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
		return &FileEncodingError{Source: err}
	}
	return Splice(file, 0, uint64(end), out.Bytes())
}

func (f *OggVorbisFile) FileType() FileType { return FileTypeVorbis }
func (f *OggVorbisFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggVorbisFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggVorbisFile) ContainsTagType(t TagType) bool {
	return t == TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggVorbisFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return NewTaggedFile(FileTypeVorbis, tags, *f.GetProperties())
}

func (f *OggVorbisFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeVorbisComments, wo)
}

func (f *OggVorbisFile) SaveFile(file FileLike, wo WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return oggWriteVorbisCommentsImpl(file, f.FileType(), wo, func(string) *VorbisCommentsTag { return t })
	}
	return nil
}

func (f *OggVorbisFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

func (f *OggOpusFile) FileType() FileType { return FileTypeOpus }
func (f *OggOpusFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggOpusFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggOpusFile) ContainsTagType(t TagType) bool {
	return t == TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggOpusFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return NewTaggedFile(FileTypeOpus, tags, *f.GetProperties())
}

func (f *OggOpusFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeVorbisComments, wo)
}

func (f *OggOpusFile) SaveFile(file FileLike, wo WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return oggWriteVorbisCommentsImpl(file, f.FileType(), wo, func(string) *VorbisCommentsTag { return t })
	}
	return nil
}

func (f *OggOpusFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

func (f *OggSpeexFile) FileType() FileType { return FileTypeSpeex }
func (f *OggSpeexFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}
func (f *OggSpeexFile) ContainsTag() bool { return f.VorbisComments.IsSome() }
func (f *OggSpeexFile) ContainsTagType(t TagType) bool {
	return t == TagTypeVorbisComments && f.VorbisComments.IsSome()
}

func (f *OggSpeexFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.VorbisComments.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return NewTaggedFile(FileTypeSpeex, tags, *f.GetProperties())
}

func (f *OggSpeexFile) SavePath(path string, wo WriteOptions) error {
	return SaveToPath(path, f, TagTypeVorbisComments, wo)
}

func (f *OggSpeexFile) SaveFile(file FileLike, wo WriteOptions) error {
	if t, ok := f.VorbisComments.Get(); ok {
		return oggWriteVorbisCommentsImpl(file, f.FileType(), wo, func(string) *VorbisCommentsTag { return t })
	}
	return nil
}

func (f *OggSpeexFile) RemoveFromPath(path string, t TagType) error {
	return RemoveTagFromPath(path, t)
}

// OggVorbisFileReadFrom reads an [OggVorbisFile].
func OggVorbisFileReadFrom(reader io.ReadSeeker, po ParseOptions) (*OggVorbisFile, error) {
	props, vc, ft, err := oggReadTagged(reader, po)
	if err != nil {
		return nil, err
	}
	if !ft.Equal(FileTypeVorbis) {
		return nil, &FileParseError{Source: &OggParseError{Message: "not a Vorbis stream"}}
	}
	return &OggVorbisFile{Properties: *props, VorbisComments: optionFromPtr(vc)}, nil
}

// OggOpusFileReadFrom reads an [OggOpusFile].
func OggOpusFileReadFrom(reader io.ReadSeeker, po ParseOptions) (*OggOpusFile, error) {
	props, vc, ft, err := oggReadTagged(reader, po)
	if err != nil {
		return nil, err
	}
	if !ft.Equal(FileTypeOpus) {
		return nil, &FileParseError{Source: &OggParseError{Message: "not an Opus stream"}}
	}
	return &OggOpusFile{Properties: *props, VorbisComments: optionFromPtr(vc)}, nil
}

// OggSpeexFileReadFrom reads an [OggSpeexFile].
func OggSpeexFileReadFrom(reader io.ReadSeeker, po ParseOptions) (*OggSpeexFile, error) {
	props, vc, ft, err := oggReadTagged(reader, po)
	if err != nil {
		return nil, err
	}
	if !ft.Equal(FileTypeSpeex) {
		return nil, &FileParseError{Source: &OggParseError{Message: "not a Speex stream"}}
	}
	return &OggSpeexFile{Properties: *props, VorbisComments: optionFromPtr(vc)}, nil
}

func init() {
	for _, ft := range []FileType{FileTypeVorbis, FileTypeOpus, FileTypeSpeex} {
		RegisterFormatReader(ft, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
			props, vc, ft, err := oggReadTagged(rs, opts)
			if err != nil {
				return nil, err
			}
			var tags []*Tag
			if vc != nil {
				tags = append(tags, vc.Tag())
			}
			return NewTaggedFile(ft, tags, props.FileProperties()), nil
		})
		RegisterFormatWriter(ft, func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error {
			if tag.TagType() != TagTypeVorbisComments {
				return &FileEncodingError{Source: &UnsupportedTagError{}}
			}
			return oggWriteVorbisComments(file.File(), file.Format(), tag, writeOptions)
		})
	}
}
