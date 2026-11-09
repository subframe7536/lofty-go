package lofty

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"time"
)

var (
	riffPreamble  = []byte("RIFF")
	wavePreamble  = []byte("WAVE")
	formPreamble  = []byte("FORM")
	aiffPreamble  = []byte("AIFF")
	aifcPreamble  = []byte("AIFC")
	listPreamble  = []byte("LIST")
	infoPreamble  = []byte("INFO")
	fmtPreamble   = []byte("fmt ")
	dataPreamble  = []byte("data")
	commPreamble  = []byte("COMM")
	ssndPreamble  = []byte("SSND")
	id3ChunkName  = []byte("ID3 ")
	id3ChunkName2 = []byte("id3 ")
	aiffNameChunk = []byte("NAME")
	aiffAuthChunk = []byte("AUTH")
	aiffCpyChunk  = []byte("(c) ")
	aiffAnnoChunk = []byte("ANNO")
	aiffComtChunk = []byte("COMT")
	waveFormatExt = 0xFFFE
)

// IffParseError is returned when an IFF file fails to parse.
type IffParseError struct {
	Message string
	Source  error
}

func (e *IffParseError) Error() string {
	if e.Message != "" {
		return "IFF: " + e.Message
	}
	return "failed to parse IFF file"
}
func (e *IffParseError) Unwrap() error { return e.Source }

// AiffTextChunksParseError is returned when an AIFF text chunk fails to parse.
type AiffTextChunksParseError struct {
	Message string
	Source  error
}

func (e *AiffTextChunksParseError) Error() string {
	if e.Message != "" {
		return "AIFF text chunks: " + e.Message
	}
	return "failed to parse AIFF text chunks"
}
func (e *AiffTextChunksParseError) Unwrap() error { return e.Source }

// RiffInfoTag is a RIFF INFO LIST tag.
type RiffInfoTag struct {
	items []riffInfoItem
}

type riffInfoItem struct {
	key   string
	value string
}

// Get returns an item's value by key (exact match), mirroring Rust's
// `RiffInfoList::get`.
func (t *RiffInfoTag) Get(key string) (string, bool) {
	for _, i := range t.items {
		if i.key == key {
			return i.value, true
		}
	}
	return "", false
}

// InsertKV inserts an item, case-insensitively replacing any existing item
// with the key. Keys must be valid 4-byte FourCCs, mirroring Rust's
// `RiffInfoList::insert`.
func (t *RiffInfoTag) InsertKV(key, value string) {
	if len(key) != 4 {
		return
	}
	var fourcc [4]byte
	copy(fourcc[:], key)
	if !validFourCC(fourcc) {
		return
	}
	for i, item := range t.items {
		if strings.EqualFold(item.key, key) {
			t.items = append(t.items[:i], t.items[i+1:]...)
			break
		}
	}
	t.items = append(t.items, riffInfoItem{key: key, value: value})
}

// RemoveKey removes the first item with the key (case-insensitive),
// mirroring Rust's `RiffInfoList::remove`.
func (t *RiffInfoTag) RemoveKey(key string) (string, bool) {
	for i, item := range t.items {
		if strings.EqualFold(item.key, key) {
			t.items = append(t.items[:i], t.items[i+1:]...)
			return item.value, true
		}
	}
	return "", false
}

// IsEmpty reports whether the tag has no items.
func (t *RiffInfoTag) IsEmpty() bool { return len(t.items) == 0 }

// Len returns the number of items.
func (t *RiffInfoTag) Len() int { return len(t.items) }

// Artist returns the artist ("IART").
func (t *RiffInfoTag) Artist() (string, bool) { return t.Get("IART") }

// SetArtist sets the artist.
func (t *RiffInfoTag) SetArtist(value string) { t.InsertKV("IART", value) }

// RemoveArtist removes the artist.
func (t *RiffInfoTag) RemoveArtist() { t.RemoveKey("IART") }

// Title returns the title ("INAM").
func (t *RiffInfoTag) Title() (string, bool) { return t.Get("INAM") }

// SetTitle sets the title.
func (t *RiffInfoTag) SetTitle(value string) { t.InsertKV("INAM", value) }

// RemoveTitle removes the title.
func (t *RiffInfoTag) RemoveTitle() { t.RemoveKey("INAM") }

// Album returns the album ("IPRD").
func (t *RiffInfoTag) Album() (string, bool) { return t.Get("IPRD") }

// SetAlbum sets the album.
func (t *RiffInfoTag) SetAlbum(value string) { t.InsertKV("IPRD", value) }

// RemoveAlbum removes the album.
func (t *RiffInfoTag) RemoveAlbum() { t.RemoveKey("IPRD") }

// Genre returns the genre ("IGNR").
func (t *RiffInfoTag) Genre() (string, bool) { return t.Get("IGNR") }

// SetGenre sets the genre.
func (t *RiffInfoTag) SetGenre(value string) { t.InsertKV("IGNR", value) }

// RemoveGenre removes the genre.
func (t *RiffInfoTag) RemoveGenre() { t.RemoveKey("IGNR") }

// Comment returns the comment ("ICMT").
func (t *RiffInfoTag) Comment() (string, bool) { return t.Get("ICMT") }

// SetComment sets the comment.
func (t *RiffInfoTag) SetComment(value string) { t.InsertKV("ICMT", value) }

// RemoveComment removes the comment.
func (t *RiffInfoTag) RemoveComment() { t.RemoveKey("ICMT") }

// Track returns the track number ("IPRT").
func (t *RiffInfoTag) Track() (uint32, bool) {
	s, ok := t.Get("IPRT")
	if !ok {
		return 0, false
	}
	return u32FromString(s)
}

// SetTrack sets the track number.
func (t *RiffInfoTag) SetTrack(value uint32) { t.InsertKV("IPRT", itoa(int(value))) }

// RemoveTrack removes the track number.
func (t *RiffInfoTag) RemoveTrack() { t.RemoveKey("IPRT") }

// TrackTotal returns the total tracks ("IFRM").
func (t *RiffInfoTag) TrackTotal() (uint32, bool) {
	s, ok := t.Get("IFRM")
	if !ok {
		return 0, false
	}
	return u32FromString(s)
}

// SetTrackTotal sets the total tracks.
func (t *RiffInfoTag) SetTrackTotal(value uint32) { t.InsertKV("IFRM", itoa(int(value))) }

// RemoveTrackTotal removes the total tracks.
func (t *RiffInfoTag) RemoveTrackTotal() { t.RemoveKey("IFRM") }

// Date returns the recording date ("ICRD").
func (t *RiffInfoTag) Date() Option[Timestamp] {
	if s, ok := t.Get("ICRD"); ok {
		return tryParseTimestamp(s)
	}
	return None[Timestamp]()
}

// SetDate sets the recording date.
func (t *RiffInfoTag) SetDate(value Timestamp) { t.InsertKV("ICRD", value.String()) }

// RemoveDate removes the recording date.
func (t *RiffInfoTag) RemoveDate() { t.RemoveKey("ICRD") }

// Tag converts to a generic [Tag].
func (t *RiffInfoTag) Tag() *Tag {
	tag := NewTag(TagTypeRiffInfo)
	var remainder *RiffInfoTag
	for _, i := range t.items {
		ik := itemKeyOrUnknown(TagTypeRiffInfo, i.key)
		if ik == ItemKeyUnknown {
			// Unmappable keys stay in the companion (Rust's split_tag)
			if getGlobalOptions().PreserveFormatSpecificItems {
				if remainder == nil {
					remainder = &RiffInfoTag{}
				}
				remainder.items = append(remainder.items, i)
			}
			continue
		}
		tag.PushUnchecked(NewTagItem(ik, ItemValueText(i.value)))
	}
	if remainder != nil {
		tag.companionTag = remainder
	}
	return tag
}

// RiffInfoTagFromTag converts a generic [Tag] to a [RiffInfoTag].
func RiffInfoTagFromTag(input *Tag) *RiffInfoTag {
	t := &RiffInfoTag{}
	// Format-specific items preserved in the companion merge back first
	// (Rust's MergeTag for the RiffInfoList remainder).
	if getGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.companionTag.(*RiffInfoTag); ok && comp != nil {
			t.items = append(t.items, comp.items...)
		}
	}
	for _, item := range input.Items() {
		var key string
		if item.key == ItemKeyUnknown {
			key = item.Description
		} else if k, ok := item.key.MapKey(TagTypeRiffInfo); ok {
			key = k
		} else {
			continue
		}
		if len(key) != 4 {
			continue
		}
		if s, ok := item.value.IntoString(); ok {
			t.items = append(t.items, riffInfoItem{key, s})
		}
	}
	return t
}

// AiffComment represents an AIFF `COMT` chunk comment.
type AiffComment struct {
	Timestamp uint32
	MarkerID  uint16
	Text      string
}

// AiffTextTag is a collection of AIFF text chunks.
type AiffTextTag struct {
	Name        Option[string]
	Author      Option[string]
	Copyright   Option[string]
	Annotations []string
	Comments    []AiffComment
}

// Tag converts to a generic [Tag].
func (t *AiffTextTag) Tag() *Tag {
	tag := NewTag(TagTypeAiffText)
	if v, ok := t.Name.Get(); ok {
		tag.InsertText(ItemKeyTrackTitle, v)
	}
	if v, ok := t.Author.Get(); ok {
		tag.InsertText(ItemKeyTrackArtist, v)
	}
	if v, ok := t.Copyright.Get(); ok {
		tag.InsertText(ItemKeyCopyrightMessage, v)
	}
	for _, v := range t.Annotations {
		tag.PushUnchecked(NewTagItem(ItemKeyComment, ItemValueText(v)))
	}
	// Comments are not representable as items; they ride the companion tag
	// (Rust's split_tag remainder).
	if len(t.Comments) > 0 && getGlobalOptions().PreserveFormatSpecificItems {
		tag.companionTag = &AiffTextTag{Comments: t.Comments}
	}
	return tag
}

// AiffTextTagFromTag converts a generic [Tag] to an [AiffTextTag].
func AiffTextTagFromTag(input *Tag) *AiffTextTag {
	t := &AiffTextTag{}
	// Format-specific fields preserved in the companion merge back first
	// (Rust's MergeTag for the AiffTextChunks remainder).
	if getGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.companionTag.(*AiffTextTag); ok && comp != nil {
			*t = *comp
		}
	}
	if s, ok := input.GetString(ItemKeyTrackTitle); t.Name.IsNone() && ok {
		t.Name = Some(s)
	}
	if s, ok := input.GetString(ItemKeyTrackArtist); t.Author.IsNone() && ok {
		t.Author = Some(s)
	}
	if s, ok := input.GetString(ItemKeyCopyrightMessage); t.Copyright.IsNone() && ok {
		t.Copyright = Some(s)
	}
	if len(t.Annotations) == 0 {
		t.Annotations = input.GetStrings(ItemKeyComment)
	}
	return t
}

// aiffParseComt parses a COMT chunk body of `size` bytes, appending comments
// to the tag. Odd-sized comment texts carry a pad byte not included in the
// size; the final comment's pad is left to the chunk skipper.
func aiffParseComt(reader io.ReadSeeker, size int64, text *AiffTextTag, parseOptions ParseOptions) error {
	chunkEnd, _ := reader.Seek(0, io.SeekCurrent)
	chunkEnd += size
	var numComments uint16
	if err := binary.Read(reader, binary.BigEndian, &numComments); err != nil {
		return &AiffTextChunksParseError{Source: err}
	}
	for i := uint16(0); i < numComments; i++ {
		var hdr [8]byte
		if _, err := io.ReadFull(reader, hdr[:]); err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		timestamp := binary.BigEndian.Uint32(hdr[0:])
		markerID := binary.BigEndian.Uint16(hdr[4:])
		count := binary.BigEndian.Uint16(hdr[6:])
		data := make([]byte, count)
		if _, err := io.ReadFull(reader, data); err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		s, err := utf8Decode(data)
		if err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		if count%2 != 0 && i+1 < numComments {
			var pad [1]byte
			if _, err := io.ReadFull(reader, pad[:]); err != nil {
				return &AiffTextChunksParseError{Source: err}
			}
			if pad[0] != 0 {
				if parseOptions.ParsingMode == ParsingModeStrict {
					return &AiffTextChunksParseError{Message: "`COMT` chunk has invalid padding"}
				}
				// Give up on the rest of the comments (Rust behavior)
				_, _ = reader.Seek(chunkEnd, io.SeekStart)
				return nil
			}
		}
		text.Comments = append(text.Comments, AiffComment{
			Timestamp: timestamp,
			MarkerID:  markerID,
			Text:      s,
		})
	}
	return nil
}

// --- WAV ---

// WavProperties are the audio properties of a WAV file.
type WavProperties struct {
	Format         uint16
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	ChannelMask    Option[ChannelMask]
}

// FileProperties converts to generic [FileProperties].
func (p WavProperties) FileProperties() FileProperties {
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

// WavFile is a RIFF/WAVE file.
type WavFile struct {
	Properties WavProperties
	RiffInfo   Option[*RiffInfoTag]
	ID3v2      Option[*Tag]
}

// WavFileReadFrom reads a [WavFile] from a reader.
func WavFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*WavFile, error) {
	var file WavFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	var hdr [12]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, &FileParseError{Source: &IffParseError{Source: err}}
	}
	if !bytes.Equal(hdr[:4], riffPreamble) || !bytes.Equal(hdr[8:12], wavePreamble) {
		return nil, &FileParseError{Source: &IffParseError{Source: &FakeTagError{}}}
	}

	var info *RiffInfoTag
	var id3v2Tag *Tag
	var fmtData []byte
	var totalSamples, streamLen uint32

	for {
		var ch [8]byte
		if _, err := io.ReadFull(reader, ch[:]); err != nil {
			break
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		pos, _ := reader.Seek(0, io.SeekCurrent)
		switch {
		case bytes.Equal(ch[:4], fmtPreamble):
			fmtData = make([]byte, size)
			if _, err := io.ReadFull(reader, fmtData); err != nil {
				return nil, &FileParseError{Source: err}
			}
		case bytes.Equal(ch[:4], []byte("fact")):
			var fb [4]byte
			if _, err := io.ReadFull(reader, fb[:]); err == nil {
				totalSamples = binary.LittleEndian.Uint32(fb[:])
			}
		case bytes.Equal(ch[:4], listPreamble):
			if size >= 4 {
				var sub [4]byte
				if _, err := io.ReadFull(reader, sub[:]); err != nil {
					return nil, &FileParseError{Source: err}
				}
				if bytes.Equal(sub[:], infoPreamble) {
					if t := parseRiffInfoList(reader, int64(size)-4, parseOptions); t != nil {
						info = t
					}
				}
			}
		case bytes.Equal(ch[:4], id3ChunkName) || bytes.Equal(ch[:4], id3ChunkName2):
			if parseOptions.ReadTags {
				header, err := parseId3v2Header(reader)
				if err == nil {
					t, err := parseId3v2(reader, header, parseOptions)
					if err == nil {
						id3v2Tag = t
					}
				}
			}
		case bytes.Equal(ch[:4], dataPreamble):
			streamLen = size
		}
		if _, err := reader.Seek(pos+int64(size), io.SeekStart); err != nil {
			break
		}
		iffCorrectPosition(reader, size, streamEnd)
	}

	if parseOptions.ReadProperties {
		if err := wavReadProperties(fmtData, totalSamples, streamLen, fileLength, &file.Properties); err != nil {
			return nil, &FileParseError{Source: err}
		}
	}

	if parseOptions.ReadTags {
		file.RiffInfo = optionFromPtr(info)
		file.ID3v2 = optionFromPtr(id3v2Tag)
	}
	return &file, nil
}

// validFourCC reports whether the FourCC is valid (printable chars, any
// spaces only trailing), mirroring Rust's iff::chunk::valid_fourcc.
func validFourCC(fourcc [4]byte) bool {
	hasSpaces := false
	for _, b := range fourcc {
		if b == 0x20 {
			hasSpaces = true
		}
		if b < 0x20 || b > 0x7E || (hasSpaces && b != 0x20) {
			return false
		}
	}
	return true
}

// iffCorrectPosition consumes the padding byte after an odd-sized chunk,
// peeking at the next FourCC to avoid eating into the next chunk — mirrors
// Rust's Chunks::correct_position. The reader must be at the chunk end.
func iffCorrectPosition(reader io.ReadSeeker, chunkSize uint32, streamEnd int64) {
	if chunkSize%2 == 0 {
		return
	}
	pos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil || streamEnd-pos < 1 {
		return
	}
	var pad [1]byte
	if _, err := reader.Read(pad[:]); err != nil {
		return
	}
	if pad[0] == 0 {
		return
	}
	if streamEnd-pos-1 > 4 {
		var nb [3]byte
		if _, err := io.ReadFull(reader, nb[:]); err != nil {
			return
		}
		fc := [4]byte{pad[0], nb[0], nb[1], nb[2]}
		if validFourCC(fc) {
			// Eating into the next chunk; the "padding" was already consumed
			_, _ = reader.Seek(-4, io.SeekCurrent)
		} else {
			_, _ = reader.Seek(-3, io.SeekCurrent)
		}
	}
}

// wavReadProperties computes WAV properties from the fmt chunk, total samples
// (fact chunk), data stream length, and file length — mirroring Rust.
func wavReadProperties(fmt []byte, totalSamples, streamLen uint32, fileLength uint64, p *WavProperties) error {
	if len(fmt) < 16 {
		return &IffParseError{Message: "file does not contain a valid \"fmt \" chunk"}
	}
	if streamLen == 0 {
		return &IffParseError{Message: "file does not contain a \"data\" chunk"}
	}

	formatTag := binary.LittleEndian.Uint16(fmt[0:])
	channels := binary.LittleEndian.Uint16(fmt[2:])
	sampleRate := binary.LittleEndian.Uint32(fmt[4:])
	bytesPerSecond := binary.LittleEndian.Uint32(fmt[8:])
	blockAlign := binary.LittleEndian.Uint16(fmt[12:])
	bitsPerSample := binary.LittleEndian.Uint16(fmt[14:])

	var validBits uint16
	var channelMask Option[ChannelMask]
	if formatTag == uint16(waveFormatExt) {
		if len(fmt) < 40 {
			return &IffParseError{Message: "extensible format identified, invalid \"fmt \" chunk size found (< 40)"}
		}
		// cbSize (2)
		validBits = binary.LittleEndian.Uint16(fmt[18:])
		channelMask = Some(ChannelMask(binary.LittleEndian.Uint32(fmt[20:])))
		formatTag = binary.LittleEndian.Uint16(fmt[24:])
	}

	if channels == 0 {
		return &IffParseError{Message: "file contains 0 channels"}
	}
	if bitsPerSample%8 != 0 {
		return &IffParseError{Message: "bits per sample is not a multiple of 8"}
	}

	var bitDepth uint8
	if validBits > 0 {
		bitDepth = uint8(validBits)
	} else if bitsPerSample > 0 {
		bitDepth = uint8(bitsPerSample)
	} else {
		bitDepth = uint8(blockAlign/channels) * 8
	}

	pcm := formatTag == 1 || formatTag == 3
	if !pcm && totalSamples == 0 {
		return &IffParseError{Message: "non-PCM format identified, no \"fact\" chunk found"}
	}
	if bitsPerSample > 0 && (totalSamples == 0 || pcm) {
		totalSamples = streamLen / (uint32(channels) * uint32(bitsPerSample/8))
	}

	p.Format = formatTag
	p.Channels = uint8(channels)
	p.SampleRate = sampleRate
	p.BitDepth = bitDepth
	p.ChannelMask = channelMask
	if bytesPerSecond > 0 {
		p.AudioBitrate = uint32(divRound(uint64(bytesPerSecond)*8, 1000))
	}
	if sampleRate > 0 && totalSamples > 0 {
		length := divRound(uint64(totalSamples)*1000, uint64(sampleRate))
		p.Duration = time.Duration(length) * time.Millisecond
		if length > 0 {
			p.OverallBitrate = uint32(divRound(fileLength*8, length))
			if p.AudioBitrate == 0 {
				p.AudioBitrate = uint32(divRound(uint64(streamLen)*8, length))
			}
		}
	} else if streamLen > 0 && bytesPerSecond > 0 {
		length := divRound(uint64(streamLen)*1000, uint64(bytesPerSecond))
		p.Duration = time.Duration(length) * time.Millisecond
		if length > 0 {
			p.OverallBitrate = uint32(divRound(fileLength*8, length))
		}
	}
	return nil
}

// parseRiffInfoList parses the contents of a LIST/INFO chunk.
// Chunks with sizes exceeding the list bounds stop parsing, like Rust.
func parseRiffInfoList(reader io.Reader, size int64, _ ParseOptions) *RiffInfoTag {
	tag := &RiffInfoTag{}
	remaining := size
	for remaining >= 8 {
		var ch [8]byte
		if _, err := io.ReadFull(reader, ch[:]); err != nil {
			break
		}
		remaining -= 8
		key := string(ch[:4])
		sz := int64(binary.LittleEndian.Uint32(ch[4:]))
		if sz > remaining {
			// Invalid chunk size, stop reading items
			break
		}
		data := make([]byte, sz)
		if _, err := io.ReadFull(reader, data); err != nil {
			break
		}
		remaining -= sz
		tag.items = append(tag.items, riffInfoItem{key, trimEndNulls(latin1Decode(data))})
		if sz&1 == 1 && remaining > 0 {
			var pad [1]byte
			if _, err := reader.Read(pad[:]); err == nil {
				remaining--
			}
		}
	}
	return tag
}

// FileType implements AudioFile.
func (f *WavFile) FileType() FileType { return FileTypeWav }

// GetProperties implements AudioFile.
func (f *WavFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *WavFile) ContainsTag() bool { return f.RiffInfo.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *WavFile) ContainsTagType(t TagType) bool {
	switch t {
	case TagTypeRiffInfo:
		return f.RiffInfo.IsSome()
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *WavFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.RiffInfo.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeWav, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *WavFile) SavePath(path string, wo WriteOptions) error {
	return saveToPath(path, f, FileTypeWav.PrimaryTagType(), wo)
}

// SaveFile implements AudioFile.
func (f *WavFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.RiffInfo.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return saveTagFields(file, FileTypeWav, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *WavFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, t)
}

// wavWriteRiffInfo replaces or adds a LIST/INFO chunk in a WAV file.
func wavWriteTag(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	if tag.TagType() == TagTypeID3v2 {
		return wavWriteId3v2(file, tag, writeOptions)
	}
	return wavWriteRiffInfo(file, tag, writeOptions)
}

func wavWriteRiffInfo(file FileLike, tag *Tag, _ WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	if !bytes.Equal(hdr[:4], riffPreamble) || !bytes.Equal(hdr[8:12], wavePreamble) {
		return &FileEncodingError{Source: &FakeTagError{}}
	}

	// Find an existing LIST/INFO chunk
	var listStart, listEnd int64
	for {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			break
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		if bytes.Equal(ch[:4], listPreamble) && size >= 4 {
			var sub [4]byte
			if _, err := io.ReadFull(file, sub[:]); err != nil {
				return &FileEncodingError{Source: err}
			}
			if bytes.Equal(sub[:], infoPreamble) {
				listStart = pos
				listEnd = pos + 8 + int64(size) + int64(size&1)
				break
			}
		}
		if _, err := file.Seek(pos+8+int64(size)+int64(size&1), io.SeekStart); err != nil {
			break
		}
	}

	// Encode the INFO list
	var encoded []byte
	if !tag.IsEmpty() {
		info := RiffInfoTagFromTag(tag)
		encoded = riffInfoListBytes(info)
	}

	if err := wavInsertChunk(file, listStart, listEnd, encoded); err != nil {
		return &FileEncodingError{Source: err}
	}
	return nil
}

// riffInfoListBytes encodes the tag as a `LIST<size>INFO` chunk (empty if the
// tag has no items), matching Rust's `RiffInfoList::dump_to`.
func riffInfoListBytes(info *RiffInfoTag) []byte {
	var payload bytes.Buffer
	payload.WriteString("INFO")
	for _, item := range info.items {
		val := item.value
		size := len(val) + 1
		var ch [8]byte
		copy(ch[:4], item.key)
		binary.LittleEndian.PutUint32(ch[4:], uint32(size))
		payload.Write(ch[:])
		payload.WriteString(val)
		payload.WriteByte(0)
		if size&1 == 1 {
			payload.WriteByte(0)
		}
	}
	if payload.Len() <= 4 {
		return nil
	}
	var encoded bytes.Buffer
	size := uint32(payload.Len())
	encoded.WriteString("LIST")
	var szb [4]byte
	binary.LittleEndian.PutUint32(szb[:], size)
	encoded.Write(szb[:])
	encoded.Write(payload.Bytes())
	if size&1 == 1 {
		encoded.WriteByte(0)
	}
	return encoded.Bytes()
}

// DumpTo writes the tag as a `LIST<size>INFO` chunk to a writer.
func (t *RiffInfoTag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data := riffInfoListBytes(t)
	_, err := writer.Write(data)
	return err
}

// wavInsertChunk splices an encoded chunk into the file inside the RIFF
// bounds and adjusts the RIFF size by the byte delta. When
// `chunkStart <= 0` the chunk is inserted at the end of the RIFF (before any
// trailing non-RIFF data).
func wavInsertChunk(file FileLike, chunkStart, chunkEnd int64, encoded []byte) error {
	sizePos, err := file.Seek(4, io.SeekStart)
	if err != nil {
		return err
	}
	_ = sizePos
	var sz [4]byte
	if _, err := io.ReadFull(file, sz[:]); err != nil {
		return err
	}
	riffSize := int64(binary.LittleEndian.Uint32(sz[:]))

	var delta int64
	if chunkStart > 0 {
		delta = int64(len(encoded)) - (chunkEnd - chunkStart)
		if err := Splice(file, uint64(chunkStart), uint64(chunkEnd), encoded); err != nil {
			return err
		}
	} else if len(encoded) > 0 {
		delta = int64(len(encoded))
		insertAt := 8 + riffSize
		if err := Splice(file, uint64(insertAt), uint64(insertAt), encoded); err != nil {
			return err
		}
	}
	if delta == 0 {
		return nil
	}
	if _, err := file.Seek(4, io.SeekStart); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(sz[:], uint32(riffSize+delta))
	_, err = file.Write(sz[:])
	return err
}

// wavWriteId3v2 replaces or adds an "ID3 "/id3 chunk.
func wavWriteId3v2(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	var chunkStart, chunkEnd int64
	for {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			break
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		if bytes.Equal(ch[:4], id3ChunkName) || bytes.Equal(ch[:4], id3ChunkName2) {
			chunkStart = pos
			chunkEnd = pos + 8 + int64(size) + int64(size&1)
			break
		}
		if _, err := file.Seek(pos+8+int64(size)+int64(size&1), io.SeekStart); err != nil {
			break
		}
	}
	var encoded bytes.Buffer
	if !tag.IsEmpty() {
		body, err := id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		size := uint32(len(body))
		encoded.WriteString("id3 ")
		var szb [4]byte
		binary.LittleEndian.PutUint32(szb[:], size)
		encoded.Write(szb[:])
		encoded.Write(body)
		if size&1 == 1 {
			encoded.WriteByte(0)
		}
	}
	if err := wavInsertChunk(file, chunkStart, chunkEnd, encoded.Bytes()); err != nil {
		return &FileEncodingError{Source: err}
	}
	return nil
}

// --- AIFF ---

// AiffProperties are the audio properties of an AIFF file.
// AiffCompressionType identifies an AIFC compression algorithm.
type AiffCompressionType struct {
	id   [4]byte
	name string
}

var (
	AiffCompressionNone = AiffCompressionType{name: "not compressed"}
	AiffCompressionSowt = aiffCompressionTypeFromId("sowt")
	AiffCompressionFl32 = aiffCompressionTypeFromId("fl32")
	AiffCompressionFl64 = aiffCompressionTypeFromId("fl64")
	AiffCompressionAlaw = aiffCompressionTypeFromId("alaw")
	AiffCompressionULaw = aiffCompressionTypeFromId("ulaw")
	AiffCompressionALAW = aiffCompressionTypeFromId("ALAW")
	AiffCompressionULAW = aiffCompressionTypeFromId("ULAW")
	AiffCompressionFL32 = aiffCompressionTypeFromId("FL32")
)

func aiffCompressionTypeFromId(id string) AiffCompressionType {
	var t AiffCompressionType
	copy(t.id[:], id)
	switch id {
	case "sowt":
		t.name = "" // Has no compression name
	case "fl32":
		t.name = "32-bit floating point"
	case "fl64":
		t.name = "64-bit floating point"
	case "alaw":
		t.name = "ALaw 2:1"
	case "ulaw":
		t.name = "ULaw 2:1"
	case "ALAW":
		t.name = "CCITT G.711 A-law"
	case "ULAW":
		t.name = "CCITT G.711 u-law"
	case "FL32":
		t.name = "32-bit floating point (SoundHack & Csound)"
	default:
		t.name = ""
	}
	return t
}

// AiffCompressionTypeFromId returns the compression type for a 4-byte ID.
func AiffCompressionTypeFromId(id [4]byte) AiffCompressionType {
	return aiffCompressionTypeFromId(string(id[:]))
}

// CompressionName returns a human-readable name for the compression type.
func (t AiffCompressionType) CompressionName() string { return t.name }

// ID returns the 4-byte identifier.
func (t AiffCompressionType) ID() [4]byte { return t.id }

type AiffProperties struct {
	Duration        time.Duration
	OverallBitrate  uint32
	AudioBitrate    uint32
	SampleRate      uint32
	BitDepth        uint8
	Channels        uint8
	CompressionType Option[AiffCompressionType]
}

// FileProperties converts to generic [FileProperties].
func (p AiffProperties) FileProperties() FileProperties {
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

// AiffFile is an AIFF file.
type AiffFile struct {
	Properties AiffProperties
	TextChunks Option[*AiffTextTag]
	ID3v2      Option[*Tag]
}

// AiffFileReadFrom reads an [AiffFile] from a reader.
func AiffFileReadFrom(reader io.ReadSeeker, parseOptions ParseOptions) (*AiffFile, error) {
	var file AiffFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - start)
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}

	var hdr [12]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, &FileParseError{Source: &IffParseError{Source: err}}
	}
	isAifc := false
	if bytes.Equal(hdr[:4], formPreamble) && bytes.Equal(hdr[8:12], aiffPreamble) {
		isAifc = false
	} else if bytes.Equal(hdr[:4], formPreamble) && bytes.Equal(hdr[8:12], aifcPreamble) {
		isAifc = true
	} else {
		return nil, &FileParseError{Source: &IffParseError{Source: &FakeTagError{}}}
	}
	_ = isAifc

	var text *AiffTextTag
	var id3v2Tag *Tag
	var sawComm bool
	var comm struct {
		frames      uint32
		sampleRate  uint32
		totalFrames uint32
	}
	var ssndSize uint32

	for {
		var ch [8]byte
		if _, err := io.ReadFull(reader, ch[:]); err != nil {
			break
		}
		size := binary.BigEndian.Uint32(ch[4:])
		pos, _ := reader.Seek(0, io.SeekCurrent)
		switch {
		case bytes.Equal(ch[:4], commPreamble):
			sawComm = true
			var cb [18]byte
			if _, err := io.ReadFull(reader, cb[:]); err != nil {
				return nil, &FileParseError{Source: err}
			}
			file.Properties.Channels = uint8(binary.BigEndian.Uint16(cb[0:]))
			comm.totalFrames = binary.BigEndian.Uint32(cb[2:])
			comm.frames = comm.totalFrames
			file.Properties.BitDepth = uint8(binary.BigEndian.Uint16(cb[6:]))
			var rate10 [10]byte
			copy(rate10[:], cb[8:])
			rate := f80FromBEBytes(rate10).asF64()
			comm.sampleRate = uint32(rate)
			file.Properties.SampleRate = comm.sampleRate
			// AIFC: compression type (4) + Pascal string name
			if size > 18 {
				var comp [4]byte
				if _, err := io.ReadFull(reader, comp[:]); err != nil {
					return nil, &FileParseError{Source: err}
				}
				ct := AiffCompressionTypeFromId(comp)
				file.Properties.CompressionType = Some(ct)
			}
		case bytes.Equal(ch[:4], aiffComtChunk) && parseOptions.ReadTags && (text == nil || len(text.Comments) == 0):
			if size >= 2 {
				if text == nil {
					text = &AiffTextTag{}
				}
				if err := aiffParseComt(reader, int64(size), text, parseOptions); err != nil {
					return nil, &FileParseError{Source: err}
				}
			}
		case bytes.Equal(ch[:4], aiffNameChunk) || bytes.Equal(ch[:4], aiffAuthChunk) ||
			bytes.Equal(ch[:4], aiffCpyChunk) || bytes.Equal(ch[:4], aiffAnnoChunk):
			if parseOptions.ReadTags {
				data, err := tryByteSlice(int(size))
				if err != nil {
					return nil, &FileParseError{Source: err}
				}
				if _, err := io.ReadFull(reader, data); err != nil {
					return nil, &FileParseError{Source: err}
				}
				if text == nil {
					text = &AiffTextTag{}
				}
				v := latin1Decode(data)
				switch {
				case bytes.Equal(ch[:4], aiffNameChunk):
					text.Name = Some(v)
				case bytes.Equal(ch[:4], aiffAuthChunk):
					text.Author = Some(v)
				case bytes.Equal(ch[:4], aiffCpyChunk):
					text.Copyright = Some(v)
				case bytes.Equal(ch[:4], aiffAnnoChunk):
					text.Annotations = append(text.Annotations, v)
				}
			}
		case bytes.Equal(ch[:4], []byte("ID3 ")) || bytes.Equal(ch[:4], []byte("ID3\x00")):
			if parseOptions.ReadTags {
				header, err := parseId3v2Header(reader)
				if err == nil {
					t, err := parseId3v2(reader, header, parseOptions)
					if err == nil {
						id3v2Tag = t
					}
				}
			}
		case bytes.Equal(ch[:4], ssndPreamble):
			ssndSize = size
		}
		if _, err := reader.Seek(pos+int64(size), io.SeekStart); err != nil {
			break
		}
		iffCorrectPosition(reader, size, streamEnd)
	}

	if parseOptions.ReadProperties && (!sawComm || ssndSize == 0) {
		return nil, &FileParseError{Source: &IffParseError{Message: "AIFF file is missing a COMM or SSND chunk"}}
	}
	if parseOptions.ReadProperties {
		p := &file.Properties
		if comm.sampleRate > 0 && comm.totalFrames > 0 {
			// Rust: duration = trunc(frames*1000/rate); bitrates = len*8/ms + 0.5
			length := float64(comm.totalFrames) * 1000 / float64(comm.sampleRate)
			p.Duration = time.Duration(uint64(length)) * time.Millisecond
			p.OverallBitrate = uint32(float64(fileLength)*8/length + 0.5)
			p.AudioBitrate = uint32(float64(ssndSize)*8/length + 0.5)
		}
	}

	if parseOptions.ReadTags {
		file.TextChunks = optionFromPtr(text)
		file.ID3v2 = optionFromPtr(id3v2Tag)
	}
	return &file, nil
}

// FileType implements AudioFile.
func (f *AiffFile) FileType() FileType { return FileTypeAiff }

// GetProperties implements AudioFile.
func (f *AiffFile) GetProperties() *FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements AudioFile.
func (f *AiffFile) ContainsTag() bool { return f.TextChunks.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements AudioFile.
func (f *AiffFile) ContainsTagType(t TagType) bool {
	switch t {
	case TagTypeAiffText:
		return f.TextChunks.IsSome()
	case TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [TaggedFile].
func (f *AiffFile) TaggedFile() *TaggedFile {
	var tags []*Tag
	if t, ok := f.TextChunks.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return NewTaggedFile(FileTypeAiff, tags, *f.GetProperties())
}

// SavePath implements AudioFile.
func (f *AiffFile) SavePath(path string, wo WriteOptions) error {
	return saveToPath(path, f, FileTypeAiff.PrimaryTagType(), wo)
}

// SaveFile implements AudioFile.
func (f *AiffFile) SaveFile(file FileLike, wo WriteOptions) error {
	var tags []*Tag
	if t, ok := f.TextChunks.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t)
	}
	return saveTagFields(file, FileTypeAiff, tags, wo)
}

// RemoveFromPath implements AudioFile.
func (f *AiffFile) RemoveFromPath(path string, t TagType) error {
	return removeTagFromPath(path, t)
}

// aiffWriteTag writes NAME/AUTH/(c) /ANNO chunks in an AIFF file.
func aiffWriteTag(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	if tag.TagType() == TagTypeID3v2 {
		return aiffWriteId3v2(file, tag, writeOptions)
	}
	return aiffWriteText(file, tag, writeOptions)
}

// aiffTextChunkBytes encodes the tag's text chunks (COMT first, then
// NAME/AUTH/(c) /ANNO), matching Rust's `AiffTextChunks::dump_to`.
func aiffTextChunkBytes(text *AiffTextTag) ([]byte, error) {
	var encoded bytes.Buffer
	writeChunk := func(name [4]byte, value string) {
		size := uint32(len(value))
		encoded.Write(name[:])
		var szb [4]byte
		binary.BigEndian.PutUint32(szb[:], size)
		encoded.Write(szb[:])
		encoded.WriteString(value)
		if size&1 == 1 {
			encoded.WriteByte(0)
		}
	}
	// Rust writes COMT before the other text chunks.
	if len(text.Comments) > 0 {
		if len(text.Comments) > 0xFFFF {
			return nil, &TooMuchDataError{}
		}
		encoded.Write(aiffComtChunk)
		comtSizePos := encoded.Len()
		encoded.Write([]byte{0, 0, 0, 0})
		var nb [8]byte
		binary.BigEndian.PutUint16(nb[:], uint16(len(text.Comments)))
		encoded.Write(nb[:2])
		for _, c := range text.Comments {
			if len(c.Text) > 0xFFFF {
				return nil, &TooMuchDataError{}
			}
			binary.BigEndian.PutUint32(nb[0:], c.Timestamp)
			binary.BigEndian.PutUint16(nb[4:], c.MarkerID)
			binary.BigEndian.PutUint16(nb[6:], uint16(len(c.Text)))
			encoded.Write(nb[:8])
			encoded.WriteString(c.Text)
			if len(c.Text)%2 != 0 {
				encoded.WriteByte(0)
			}
		}
		comtLen := uint32(encoded.Len() - comtSizePos - 4)
		binary.BigEndian.PutUint32(encoded.Bytes()[comtSizePos:], comtLen)
		if comtLen%2 != 0 {
			encoded.WriteByte(0)
		}
	}
	if v, ok := text.Name.Get(); ok {
		writeChunk([4]byte{'N', 'A', 'M', 'E'}, v)
	}
	if v, ok := text.Author.Get(); ok {
		writeChunk([4]byte{'A', 'U', 'T', 'H'}, v)
	}
	if v, ok := text.Copyright.Get(); ok {
		writeChunk([4]byte{'(', 'c', ')', ' '}, v)
	}
	for _, v := range text.Annotations {
		writeChunk([4]byte{'A', 'N', 'N', 'O'}, v)
	}
	return encoded.Bytes(), nil
}

// Len returns the number of populated fields in the tag.
func (t *AiffTextTag) Len() int {
	n := len(t.Annotations) + len(t.Comments)
	if t.Name.IsSome() {
		n++
	}
	if t.Author.IsSome() {
		n++
	}
	if t.Copyright.IsSome() {
		n++
	}
	return n
}

// IsEmpty reports whether the tag has no fields.
func (t *AiffTextTag) IsEmpty() bool { return t.Len() == 0 }

// DumpTo writes the tag's chunks to a writer.
func (t *AiffTextTag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data, err := aiffTextChunkBytes(t)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	_, err = writer.Write(data)
	return err
}

func aiffWriteText(file FileLike, tag *Tag, _ WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	if !bytes.Equal(hdr[:4], formPreamble) {
		return &FileEncodingError{Source: &FakeTagError{}}
	}

	// Collect ranges of existing text chunks to remove
	type rng struct{ start, end int64 }
	var ranges []rng
	for {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			break
		}
		size := binary.BigEndian.Uint32(ch[4:])
		isText := bytes.Equal(ch[:4], aiffNameChunk) || bytes.Equal(ch[:4], aiffAuthChunk) ||
			bytes.Equal(ch[:4], aiffCpyChunk) || bytes.Equal(ch[:4], aiffAnnoChunk) ||
			bytes.Equal(ch[:4], aiffComtChunk)
		end := pos + 8 + int64(size) + int64(size&1)
		if isText {
			ranges = append(ranges, rng{pos, end})
		}
		if _, err := file.Seek(end, io.SeekStart); err != nil {
			break
		}
	}
	// Remove old text chunks (back to front)
	for i := len(ranges) - 1; i >= 0; i-- {
		if err := Splice(file, uint64(ranges[i].start), uint64(ranges[i].end), nil); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	// Append new chunks
	text := AiffTextTagFromTag(tag)
	encoded, err := aiffTextChunkBytes(text)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	if len(encoded) > 0 {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := file.Write(encoded); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	return iffFixSize(file)
}

func aiffWriteId3v2(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &FileEncodingError{Source: err}
	}
	var chunkStart, chunkEnd int64
	for {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			break
		}
		size := binary.BigEndian.Uint32(ch[4:])
		if bytes.Equal(ch[:4], []byte("ID3 ")) {
			chunkStart = pos
			chunkEnd = pos + 8 + int64(size) + int64(size&1)
			break
		}
		if _, err := file.Seek(pos+8+int64(size)+int64(size&1), io.SeekStart); err != nil {
			break
		}
	}
	var encoded bytes.Buffer
	if !tag.IsEmpty() {
		body, err := id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		size := uint32(len(body))
		encoded.WriteString("ID3 ")
		var szb [4]byte
		binary.BigEndian.PutUint32(szb[:], size)
		encoded.Write(szb[:])
		encoded.Write(body)
		if size&1 == 1 {
			encoded.WriteByte(0)
		}
	}
	if chunkStart > 0 {
		if err := Splice(file, uint64(chunkStart), uint64(chunkEnd), encoded.Bytes()); err != nil {
			return &FileEncodingError{Source: err}
		}
	} else if encoded.Len() > 0 {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			return &FileEncodingError{Source: err}
		}
		if _, err := file.Write(encoded.Bytes()); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	return iffFixSize(file)
}

// iffFixSize updates the FORM chunk size to fileLen - 8 (big-endian).
func iffFixSize(file FileLike) error {
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := file.Seek(4, io.SeekStart); err != nil {
		return err
	}
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(end-8))
	_, err = file.Write(sz[:])
	return err
}

func init() {
	formatReaders[fileTypeWav] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := WavFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatReaders[fileTypeAiff] = func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
		f, err := AiffFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	}
	formatWriters[fileTypeWav] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeRiffInfo, TagTypeID3v2:
			return wavWriteTag(file.File(), tag, writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	formatWriters[fileTypeAiff] = func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error {
		switch tag.TagType() {
		case TagTypeAiffText, TagTypeID3v2:
			return aiffWriteTag(file.File(), tag, writeOptions)
		}
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	tagDumpers[TagTypeRiffInfo] = func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		info := RiffInfoTagFromTag(tag)
		if len(info.items) == 0 {
			return nil
		}
		var payload bytes.Buffer
		payload.WriteString("INFO")
		for _, item := range info.items {
			val := item.value
			size := len(val) + 1
			var ch [8]byte
			copy(ch[:4], item.key)
			binary.LittleEndian.PutUint32(ch[4:], uint32(size))
			payload.Write(ch[:])
			payload.WriteString(val)
			payload.WriteByte(0)
			if size&1 == 1 {
				payload.WriteByte(0)
			}
		}
		var sz [4]byte
		binary.LittleEndian.PutUint32(sz[:], uint32(payload.Len()))
		if _, err := writer.Write([]byte("LIST")); err != nil {
			return err
		}
		if _, err := writer.Write(sz[:]); err != nil {
			return err
		}
		_, err := writer.Write(payload.Bytes())
		return err
	}
	tagDumpers[TagTypeAiffText] = func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		text := AiffTextTagFromTag(tag)
		writeChunk := func(name [4]byte, value string) error {
			var sz [4]byte
			binary.BigEndian.PutUint32(sz[:], uint32(len(value)))
			if _, err := writer.Write(name[:]); err != nil {
				return err
			}
			if _, err := writer.Write(sz[:]); err != nil {
				return err
			}
			if _, err := writer.Write([]byte(value)); err != nil {
				return err
			}
			if len(value)&1 == 1 {
				_, err := writer.Write([]byte{0})
				return err
			}
			return nil
		}
		if v, ok := text.Name.Get(); ok {
			if err := writeChunk([4]byte{'N', 'A', 'M', 'E'}, v); err != nil {
				return err
			}
		}
		if v, ok := text.Author.Get(); ok {
			if err := writeChunk([4]byte{'A', 'U', 'T', 'H'}, v); err != nil {
				return err
			}
		}
		if v, ok := text.Copyright.Get(); ok {
			if err := writeChunk([4]byte{'(', 'c', ')', ' '}, v); err != nil {
				return err
			}
		}
		for _, v := range text.Annotations {
			if err := writeChunk([4]byte{'A', 'N', 'N', 'O'}, v); err != nil {
				return err
			}
		}
		return nil
	}
}
