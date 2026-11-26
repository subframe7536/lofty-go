package iff

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strings"
	"time"

	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
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

// TagType returns [meta.TagTypeRiffInfo].
func (t *RiffInfoTag) TagType() meta.TagType { return meta.TagTypeRiffInfo }

// Contains reports whether an item with the key exists (case-insensitive).
func (t *RiffInfoTag) Contains(key string) bool {
	for _, item := range t.items {
		if strings.EqualFold(item.key, key) {
			return true
		}
	}
	return false
}

// Clear removes all items from the tag.
func (t *RiffInfoTag) Clear() { t.items = nil }

// IsEmpty reports whether the tag has no items.
func (t *RiffInfoTag) IsEmpty() bool { return len(t.items) == 0 }

// Keys returns the tag's item keys in order.
func (t *RiffInfoTag) Keys() []string {
	keys := make([]string, 0, len(t.items))
	for _, item := range t.items {
		keys = append(keys, item.key)
	}
	return keys
}

// Items returns all key/value pairs in order, mirroring Rust's `RiffInfoList::into_iter`.
func (t *RiffInfoTag) Items() [][2]string {
	out := make([][2]string, len(t.items))
	for i, item := range t.items {
		out[i] = [2]string{item.key, item.value}
	}
	return out
}

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
	return util.U32FromString(s)
}

// SetTrack sets the track number.
func (t *RiffInfoTag) SetTrack(value uint32) { t.InsertKV("IPRT", util.Itoa(int(value))) }

// RemoveTrack removes the track number.
func (t *RiffInfoTag) RemoveTrack() { t.RemoveKey("IPRT") }

// TrackTotal returns the total tracks ("IFRM").
func (t *RiffInfoTag) TrackTotal() (uint32, bool) {
	s, ok := t.Get("IFRM")
	if !ok {
		return 0, false
	}
	return util.U32FromString(s)
}

// SetTrackTotal sets the total tracks.
func (t *RiffInfoTag) SetTrackTotal(value uint32) { t.InsertKV("IFRM", util.Itoa(int(value))) }

// RemoveTrackTotal removes the total tracks.
func (t *RiffInfoTag) RemoveTrackTotal() { t.RemoveKey("IFRM") }

// Date returns the recording date ("ICRD").
func (t *RiffInfoTag) Date() meta.Option[meta.Timestamp] {
	if s, ok := t.Get("ICRD"); ok {
		return util.TryParseTimestamp(s)
	}
	return meta.None[meta.Timestamp]()
}

// SetDate sets the recording date.
func (t *RiffInfoTag) SetDate(value meta.Timestamp) { t.InsertKV("ICRD", value.String()) }

// RemoveDate removes the recording date.
func (t *RiffInfoTag) RemoveDate() { t.RemoveKey("ICRD") }

func (t *RiffInfoTag) clone() *RiffInfoTag {
	if t == nil {
		return nil
	}
	return &RiffInfoTag{items: append([]riffInfoItem(nil), t.items...)}
}

// CloneCompanion implements [meta.CompanionCloner].
func (t *RiffInfoTag) CloneCompanion() any { return t.clone() }

// Tag converts to a generic [Tag]. It is non-destructive; use
// [RiffInfoTag.SplitTag] for the destructive conversion.
func (t *RiffInfoTag) Tag() *meta.Tag {
	remainder, tag := t.clone().SplitTag()
	if util.GetGlobalOptions().PreserveFormatSpecificItems && !remainder.IsEmpty() {
		tag.SetCompanionTag(remainder)
	}
	return tag
}

// SplitTag destructively moves representable INFO items into a generic [Tag],
// leaving unmapped FourCCs in the receiver, mirroring Rust's
// `SplitTag::split_tag`. The receiver is returned as the remainder.
func (t *RiffInfoTag) SplitTag() (*RiffInfoTag, *meta.Tag) {
	tag := meta.NewTag(meta.TagTypeRiffInfo)
	original := t.items
	kept := make([]riffInfoItem, 0, len(original))
	for _, i := range original {
		ik := meta.ItemKeyOrUnknown(meta.TagTypeRiffInfo, i.key)
		if ik == meta.ItemKeyUnknown {
			kept = append(kept, i)
			continue
		}
		// Rust trims NUL bytes from INFO values (v.trim_matches('\0'))
		tag.PushUnchecked(meta.NewTagItem(ik, meta.ItemValueText(strings.Trim(i.value, "\x00"))))
	}
	t.items = kept
	return t, tag
}

// MergeTag merges a generic [Tag] into this remainder, mirroring Rust's
// `MergeTag::merge_tag`. The receiver is mutated and returned.
func (t *RiffInfoTag) MergeTag(tag *meta.Tag) *RiffInfoTag {
	for _, item := range tag.Items() {
		// Rust's merge_tag only uses item_key.map_key — Unknown-keyed
		// items are dropped.
		if item.Key() == meta.ItemKeyUnknown {
			continue
		}
		key, ok := item.Key().MapKey(meta.TagTypeRiffInfo)
		if !ok || len(key) != 4 {
			continue
		}
		if s, ok := item.Value().IntoString(); ok {
			t.items = append(t.items, riffInfoItem{key, s})
		}
	}
	return t
}

// RiffInfoTagFromTag converts a generic [Tag] to a [RiffInfoTag].
func RiffInfoTagFromTag(input *meta.Tag) *RiffInfoTag {
	base := &RiffInfoTag{}
	// Format-specific items preserved in the companion merge back first
	// (Rust's MergeTag for the RiffInfoList remainder).
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*RiffInfoTag); ok && comp != nil {
			base = comp.clone()
		}
	}
	return base.MergeTag(input)
}

// tagitemsIntoRiff mirrors Rust's tagitems_into_riff: text/locator items are
// mapped to their RIFF INFO fourcc, and popularimeters are re-encoded as the
// RIFF rating number (mapped u8). Used by generic-Tag write/dump paths only —
// merge_tag (RiffInfoTagFromTag) stores the raw value instead.
func tagitemsIntoRiff(tag *meta.Tag) []riffInfoItem {
	var items []riffInfoItem
	for _, item := range tag.Items() {
		key, ok := item.Key().MapKey(meta.TagTypeRiffInfo)
		if !ok || len(key) != 4 {
			continue
		}
		s, ok := item.Value().IntoString()
		if !ok {
			continue
		}
		if item.Key() == meta.ItemKeyPopularimeter {
			popm, err := meta.PopularimeterFromStr(s)
			if err != nil {
				continue
			}
			s = util.Itoa(int(popm.MappedValue(meta.TagTypeRiffInfo)))
		}
		items = append(items, riffInfoItem{key, s})
	}
	// An existing companion's items replay unconditionally (Rust's TagRef).
	if comp, ok := tag.CompanionTag().(*RiffInfoTag); ok && comp != nil {
		items = append(items, comp.items...)
	}
	return items
}

// AiffComment represents an AIFF `COMT` chunk comment.
type AiffComment struct {
	Timestamp uint32
	MarkerID  uint16
	Text      string
}

// AiffTextTag is a collection of AIFF text chunks.
type AiffTextTag struct {
	Name        meta.Option[string]
	Author      meta.Option[string]
	Copyright   meta.Option[string]
	Annotations []string
	Comments    []AiffComment
}

// TagType returns [meta.TagTypeAiffText].
func (t *AiffTextTag) TagType() meta.TagType { return meta.TagTypeAiffText }

// Clear removes all text chunks from the tag.
func (t *AiffTextTag) Clear() {
	t.Name = meta.None[string]()
	t.Author = meta.None[string]()
	t.Copyright = meta.None[string]()
	t.Annotations = nil
	t.Comments = nil
}

// Artist returns the author chunk (`AUTH`).
func (t *AiffTextTag) Artist() (string, bool) { return t.Author.Get() }

// SetArtist sets the author chunk (`AUTH`).
func (t *AiffTextTag) SetArtist(value string) { t.Author = meta.Some(value) }

// RemoveArtist clears the author chunk (`AUTH`).
func (t *AiffTextTag) RemoveArtist() { t.Author = meta.None[string]() }

// Title returns the name chunk (`NAME`).
func (t *AiffTextTag) Title() (string, bool) { return t.Name.Get() }

// SetTitle sets the name chunk (`NAME`).
func (t *AiffTextTag) SetTitle(value string) { t.Name = meta.Some(value) }

// RemoveTitle clears the name chunk (`NAME`).
func (t *AiffTextTag) RemoveTitle() { t.Name = meta.None[string]() }

// Comment returns the first annotation (`ANNO`), or the first `COMT` comment text
// if no annotations are present (matching Rust `AiffTextChunks::comment`).
func (t *AiffTextTag) Comment() (string, bool) {
	if len(t.Annotations) > 0 {
		return t.Annotations[0], true
	}
	if len(t.Comments) > 0 {
		return t.Comments[0].Text, true
	}
	return "", false
}

// SetComment replaces annotations with a single value (matching Rust `AiffTextChunks::set_comment`).
func (t *AiffTextTag) SetComment(value string) { t.Annotations = []string{value} }

// RemoveComment clears all annotations and comments.
func (t *AiffTextTag) RemoveComment() {
	t.Annotations = nil
	t.Comments = nil
}

func (t *AiffTextTag) clone() *AiffTextTag {
	if t == nil {
		return nil
	}
	clone := *t
	clone.Annotations = append([]string(nil), t.Annotations...)
	clone.Comments = append([]AiffComment(nil), t.Comments...)
	return &clone
}

// CloneCompanion implements [meta.CompanionCloner].
func (t *AiffTextTag) CloneCompanion() any { return t.clone() }

// Tag converts to a generic [Tag]. It is non-destructive; use
// [AiffTextTag.SplitTag] for the destructive conversion.
func (t *AiffTextTag) Tag() *meta.Tag {
	remainder, tag := t.clone().SplitTag()
	if len(remainder.Comments) > 0 && util.GetGlobalOptions().PreserveFormatSpecificItems {
		tag.SetCompanionTag(remainder)
	}
	return tag
}

// SplitTag destructively moves representable text chunks into a generic
// [Tag], leaving COMT comments in the receiver, mirroring Rust's
// `SplitTag::split_tag`. The receiver is returned as the remainder.
func (t *AiffTextTag) SplitTag() (*AiffTextTag, *meta.Tag) {
	tag := meta.NewTag(meta.TagTypeAiffText)
	pushItem := func(field *meta.Option[string], itemKey meta.ItemKey) {
		if text, ok := field.Get(); ok {
			tag.PushUnchecked(meta.NewTagItem(itemKey, meta.ItemValueText(text)))
			*field = meta.None[string]()
		}
	}
	pushItem(&t.Name, meta.ItemKeyTrackTitle)
	pushItem(&t.Author, meta.ItemKeyTrackArtist)
	pushItem(&t.Copyright, meta.ItemKeyCopyrightMessage)
	for _, annotation := range t.Annotations {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyComment, meta.ItemValueText(annotation)))
	}
	t.Annotations = nil
	return t, tag
}

// MergeTag merges a generic [Tag] into this remainder, mirroring Rust's
// `MergeTag::merge_tag`. Pre-existing remainder fields win over generic
// values. The receiver is mutated and returned.
func (t *AiffTextTag) MergeTag(tag *meta.Tag) *AiffTextTag {
	if s, ok := tag.GetString(meta.ItemKeyTrackTitle); t.Name.IsNone() && ok {
		t.Name = meta.Some(s)
	}
	if s, ok := tag.GetString(meta.ItemKeyTrackArtist); t.Author.IsNone() && ok {
		t.Author = meta.Some(s)
	}
	if s, ok := tag.GetString(meta.ItemKeyCopyrightMessage); t.Copyright.IsNone() && ok {
		t.Copyright = meta.Some(s)
	}
	if len(t.Annotations) == 0 {
		t.Annotations = tag.GetStrings(meta.ItemKeyComment)
	}
	return t
}

// AiffTextTagFromTag converts a generic [Tag] to an [AiffTextTag].
func AiffTextTagFromTag(input *meta.Tag) *AiffTextTag {
	base := &AiffTextTag{}
	// Format-specific fields preserved in the companion merge back first
	// (Rust's MergeTag for the AiffTextChunks remainder).
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*AiffTextTag); ok && comp != nil {
			base = comp.clone()
		}
	}
	return base.MergeTag(input)
}

// aiffParseComt parses a COMT chunk body of `size` bytes, appending comments
// to the tag. Odd-sized comment texts carry a pad byte not included in the
// size; the final comment's pad is left to the chunk skipper.
func aiffParseComt(reader io.ReadSeeker, size int64, text *AiffTextTag, parseOptions meta.ParseOptions) error {
	chunkEnd, _ := reader.Seek(0, io.SeekCurrent)
	chunkEnd += size
	// Rust reads through the bounded Chunk reader: overruns are errors.
	bounded := &io.LimitedReader{R: reader, N: size}
	var numComments uint16
	if err := binary.Read(bounded, binary.BigEndian, &numComments); err != nil {
		return &AiffTextChunksParseError{Source: err}
	}
	for i := uint16(0); i < numComments; i++ {
		var hdr [8]byte
		if _, err := io.ReadFull(bounded, hdr[:]); err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		timestamp := binary.BigEndian.Uint32(hdr[0:])
		markerID := binary.BigEndian.Uint16(hdr[4:])
		count := binary.BigEndian.Uint16(hdr[6:])
		data := make([]byte, count)
		if _, err := io.ReadFull(bounded, data); err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		s, err := util.Utf8Decode(data)
		if err != nil {
			return &AiffTextChunksParseError{Source: err}
		}
		if count%2 != 0 && i+1 < numComments {
			var pad [1]byte
			if _, err := io.ReadFull(bounded, pad[:]); err != nil {
				return &AiffTextChunksParseError{Source: err}
			}
			if pad[0] != 0 {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
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

// WavFormat is the WAV format tag, mirroring Rust's `WavFormat`.
type WavFormat uint16

const (
	// WavFormatPCM is pulse-code modulation (uncompressed).
	WavFormatPCM WavFormat = 0x0001
	// WavFormatIeeeFloat is IEEE 754 floating-point.
	WavFormatIeeeFloat WavFormat = 0x0003
)

// WavFormatOther wraps a non-PCM format tag, mirroring `WavFormat::Other`.
func WavFormatOther(tag uint16) WavFormat { return WavFormat(tag) }

// IsOther reports whether the format is neither PCM nor IEEE float.
func (f WavFormat) IsOther() bool { return f != WavFormatPCM && f != WavFormatIeeeFloat }

// WavProperties are the audio properties of a WAV file.
type WavProperties struct {
	Format         WavFormat
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	ChannelMask    meta.Option[meta.ChannelMask]
}

// FileProperties converts to generic [meta.FileProperties].
func (p WavProperties) FileProperties() meta.FileProperties {
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

// WavFile is a RIFF/WAVE file.
type WavFile struct {
	Properties WavProperties
	RiffInfo   meta.Option[*RiffInfoTag]
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
}

// WavFileReadFrom reads a [WavFile] from a reader.
func WavFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*WavFile, error) {
	var file WavFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - start) // #nosec G115 -- streamEnd and start are non-negative int64 stream positions with streamEnd >= start
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	var hdr [12]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
	}
	if !bytes.Equal(hdr[:4], riffPreamble) || !bytes.Equal(hdr[8:12], wavePreamble) {
		return nil, &meta.FileParseError{Source: &IffParseError{Source: &meta.UnknownFormatError{}}}
	}

	var info *RiffInfoTag
	var id3v2Tag *id3v2.Id3v2Tag
	var fmtData []byte
	var totalSamples, streamLen uint32

	// Mirrors Rust's `Chunks` walker: bounded by the file size after the
	// 12-byte RIFF header, with FourCC and size validation honored per
	// meta.ParsingMode.
	remaining := int64(fileLength) - 12 // #nosec G115 -- fileLength derives from a difference of non-negative int64 stream positions, so the int64 round-trip is lossless
	for remaining >= 8 {
		fcc, size, ok, herr := iffReadChunkHeader(reader, binary.LittleEndian, remaining, parseOptions.ParsingMode)
		if herr != nil {
			return nil, &meta.FileParseError{Source: herr}
		}
		if !ok {
			break
		}
		remaining -= 8
		pos, _ := reader.Seek(0, io.SeekCurrent)
		switch {
		case bytes.Equal(fcc[:], fmtPreamble) && parseOptions.ReadProperties && len(fmtData) == 0:
			var aerr error
			if fmtData, aerr = util.TryByteSlice(int(size)); aerr != nil {
				return nil, &meta.FileParseError{Source: aerr}
			}
			if _, err := io.ReadFull(reader, fmtData); err != nil {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
			}
		case bytes.Equal(fcc[:], []byte("fact")) && parseOptions.ReadProperties && totalSamples == 0:
			// Rust reads the u32 inside a chunk-bounded reader — a short
			// fact chunk is a parse error.
			if size < 4 {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: io.ErrUnexpectedEOF}}
			}
			var fb [4]byte
			if _, err := io.ReadFull(reader, fb[:]); err != nil {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
			}
			totalSamples = binary.LittleEndian.Uint32(fb[:])
		case bytes.Equal(fcc[:], dataPreamble) && parseOptions.ReadProperties && streamLen == 0:
			streamLen += size
		case bytes.Equal(fcc[:], listPreamble):
			if size < 4 {
				return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
			}
			var sub [4]byte
			if _, err := io.ReadFull(reader, sub[:]); err != nil {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
			}
			if bytes.Equal(sub[:], infoPreamble) && parseOptions.ReadTags {
				// `end` check from Rust: the INFO list must not extend past
				// the file.
				if pos-start+int64(size) > int64(fileLength) { // #nosec G115 -- int64(size) widens a u32 losslessly; fileLength derives from a difference of non-negative int64 stream positions
					return nil, &meta.FileParseError{Source: &meta.SizeMismatchError{}}
				}
				t, err := parseRiffInfoList(reader, int64(size)-4, parseOptions)
				if err != nil {
					return nil, err
				}
				// Rust parses every LIST/INFO into one list, appending items
				if info == nil {
					info = t
				} else {
					info.items = append(info.items, t.items...)
				}
			}
		case (bytes.Equal(fcc[:], id3ChunkName) || bytes.Equal(fcc[:], id3ChunkName2)) && parseOptions.ReadTags:
			tag, err := iffReadId3Chunk(reader, size, parseOptions)
			if err != nil {
				return nil, err
			}
			if tag != nil {
				if id3v2Tag != nil {
					// https://github.com/Serial-ATA/lofty-rs/issues/87 —
					// duplicate tags have their frames appended
					for _, fr := range tag.Frames.Frames() {
						id3v2Tag.Frames.Insert(fr)
					}
				} else {
					id3v2Tag = tag
				}
			}
		}
		if _, err := reader.Seek(pos+int64(size), io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
		}
		remaining -= int64(size)
		remaining -= int64(iffCorrectPosition(reader, size, streamEnd))
	}

	if parseOptions.ReadProperties {
		// Rust uses the position where the chunk walk ended, not the full
		// file length, for the overall bitrate (walkEnd).
		walkEnd, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if err := wavReadProperties(fmtData, totalSamples, streamLen, uint64(walkEnd-start), &file.Properties); err != nil { // #nosec G115 -- walkEnd and start are non-negative stream positions with walkEnd >= start, so the difference fits int64
			return nil, &meta.FileParseError{Source: err}
		}
	}

	if parseOptions.ReadTags {
		// Rust: `(!riff_info.items.is_empty()).then_some(...)`
		if info != nil && len(info.items) == 0 {
			info = nil
		}
		file.RiffInfo = util.OptionFromPtr(info)
		file.ID3v2 = util.OptionFromPtr(id3v2Tag)
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
// Returns the number of padding bytes consumed (0 or 1).
func iffCorrectPosition(reader io.ReadSeeker, chunkSize uint32, streamEnd int64) int {
	if chunkSize%2 == 0 {
		return 0
	}
	pos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0
	}
	return iffCorrectPositionRemaining(reader, chunkSize, streamEnd-pos)
}

// iffCorrectPositionRemaining is [iffCorrectPosition] with the Rust
// `remaining_size` given directly (the unread bytes of the enclosing stream,
// measured before the padding byte).
func iffCorrectPositionRemaining(reader io.ReadSeeker, chunkSize uint32, remaining int64) int {
	if chunkSize%2 == 0 || remaining < 1 {
		return 0
	}
	var pad [1]byte
	if _, err := io.ReadFull(reader, pad[:]); err != nil {
		return 0
	}
	if pad[0] != 0 && remaining > 4 {
		var nb [3]byte
		if _, err := io.ReadFull(reader, nb[:]); err != nil {
			return 1
		}
		fc := [4]byte{pad[0], nb[0], nb[1], nb[2]}
		if validFourCC(fc) {
			// Eating into the next chunk; the "padding" was already consumed
			_, _ = reader.Seek(-4, io.SeekCurrent)
			return 0
		}
		_, _ = reader.Seek(-3, io.SeekCurrent)
	}
	return 1
}

// iffReadChunkHeader mirrors the header handling of Rust's `Chunks::next`:
// the FourCC is read and validated before the size, so a stop leaves the
// reader 4 bytes (invalid FourCC) or 8 bytes (oversized chunk) past the chunk
// start. `remaining` is the unread byte count of the enclosing stream. When
// `ok` is false the walk should stop; `err` is non-nil for strict-mode
// mismatches and I/O failures.
func iffReadChunkHeader(reader io.Reader, order binary.ByteOrder, remaining int64, mode meta.ParsingMode) (fcc [4]byte, size uint32, ok bool, err error) {
	if _, rerr := io.ReadFull(reader, fcc[:]); rerr != nil {
		return fcc, 0, false, &IffParseError{Source: rerr}
	}
	if !validFourCC(fcc) {
		if mode == meta.ParsingModeStrict {
			return fcc, 0, false, &meta.SizeMismatchError{}
		}
		return fcc, 0, false, nil
	}
	var sb [4]byte
	if _, rerr := io.ReadFull(reader, sb[:]); rerr != nil {
		return fcc, 0, false, &IffParseError{Source: rerr}
	}
	size = order.Uint32(sb[:])
	if int64(size) > remaining {
		if mode == meta.ParsingModeStrict {
			return fcc, size, false, &meta.SizeMismatchError{}
		}
		return fcc, size, false, nil
	}
	return fcc, size, true, nil
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
	var channelMask meta.Option[meta.ChannelMask]
	if formatTag == uint16(waveFormatExt) { // #nosec G115 -- compile-time constant 0xFFFE always fits uint16 (Go rejects non-fitting constant conversions)
		if len(fmt) < 40 {
			return &IffParseError{Message: "extensible format identified, invalid \"fmt \" chunk size found (< 40)"}
		}
		// cbSize (2)
		validBits = binary.LittleEndian.Uint16(fmt[18:])
		channelMask = meta.Some(meta.ChannelMask(binary.LittleEndian.Uint32(fmt[20:])))
		formatTag = binary.LittleEndian.Uint16(fmt[24:])
	}

	// Rust truncates channels to u8 before the zero check
	channelsU8 := uint8(channels) // #nosec G115 -- mirrors Rust `channels as u8` cast in read_fmt_chunk; truncation before the zero check is intentional parity
	if channelsU8 == 0 {
		return &IffParseError{Message: "file contains 0 channels"}
	}
	if bitsPerSample%8 != 0 {
		return &IffParseError{Message: "bits per sample is not a multiple of 8"}
	}

	var bitDepth uint8
	if validBits > 0 {
		bitDepth = uint8(validBits)
	} else if bitsPerSample > 0 {
		bitDepth = uint8(bitsPerSample) // #nosec G115 -- mirrors Rust `bits_per_sample as u8` cast; truncation is intentional parity
	} else {
		// Rust: bytes_per_sample.saturating_mul(8) as u8
		bps := uint32(blockAlign/uint16(channelsU8)) * 8
		if bps > 0xFFFF {
			bps = 0xFFFF
		}
		bitDepth = uint8(bps) // #nosec G115 -- mirrors Rust `bytes_per_sample.saturating_mul(8) as u8` cast; truncation is intentional parity
	}

	pcm := formatTag == 1 || formatTag == 3
	if !pcm && totalSamples == 0 {
		return &IffParseError{Message: "non-PCM format identified, no \"fact\" chunk found"}
	}
	if bitsPerSample > 0 && (totalSamples == 0 || pcm) {
		totalSamples = streamLen / (uint32(channelsU8) * uint32(bitsPerSample/8))
	}

	p.Format = WavFormat(formatTag)
	p.Channels = uint8(channels) // #nosec G115 -- mirrors Rust truncating channels to u8 in read_fmt_chunk (`channels as u8`); truncation is intentional parity
	p.SampleRate = sampleRate
	p.BitDepth = bitDepth
	p.ChannelMask = channelMask
	if bytesPerSecond > 0 {
		p.AudioBitrate = uint32(util.DivRound(uint64(bytesPerSecond)*8, 1000)) // #nosec G115 -- mirrors Rust `(u64::from(bytes_per_second) * 8).div_round(1000) as u32` cast; truncation is intentional parity
	}
	if sampleRate > 0 && totalSamples > 0 {
		length := util.DivRound(uint64(totalSamples)*1000, uint64(sampleRate))
		p.Duration = time.Duration(length) * time.Millisecond // #nosec G115 -- length is bounded by u32 inputs (max ~(2^32-1)*1000 ms), which fits int64 even after scaling to nanoseconds
		if length > 0 {
			p.OverallBitrate = uint32(util.DivRound(fileLength*8, length)) // #nosec G115 -- mirrors Rust `(file_length * 8).div_round(length) as u32` cast; truncation is intentional parity
			if p.AudioBitrate == 0 {
				p.AudioBitrate = uint32(util.DivRound(uint64(streamLen)*8, length)) // #nosec G115 -- mirrors Rust `(u64::from(stream_len) * 8).div_round(length) as u32` cast; truncation is intentional parity
			}
		}
	} else if streamLen > 0 && bytesPerSecond > 0 {
		length := util.DivRound(uint64(streamLen)*1000, uint64(bytesPerSecond))
		p.Duration = time.Duration(length) * time.Millisecond // #nosec G115 -- length is bounded by u32 inputs (max ~(2^32-1)*1000 ms), which fits int64 even after scaling to nanoseconds
		if length > 0 {
			p.OverallBitrate = uint32(util.DivRound(fileLength*8, length)) // #nosec G115 -- mirrors Rust `(file_length * 8).div_round(length) as u32` cast; truncation is intentional parity
		}
	}
	return nil
}

// iffReadId3Chunk reads an ID3 chunk's content and parses it as an ID3v2 tag.
// Mirrors Rust's `Chunk::id3_chunk`: a chunk under 10 bytes is skipped
// (erroring in strict mode); parse errors propagate.
func iffReadId3Chunk(reader io.Reader, size uint32, parseOptions meta.ParseOptions) (*id3v2.Id3v2Tag, error) {
	content, err := util.TryByteSlice(int(size))
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
	}
	if len(content) < 10 {
		if parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &meta.FileParseError{Source: &meta.FakeTagError{}}
		}
		return nil, nil
	}
	r := bytes.NewReader(content)
	// Rust's Id3v2Header::parse also consumes the extended header, which
	// ParseId3v2Tag then excludes from the content size.
	header, err := id3v2.ParseId3v2Header(r, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
	}
	tag, err := id3v2.ParseId3v2Tag(r, header, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
	}
	return tag, nil
}

// parseRiffInfoList parses the contents of a LIST/INFO chunk, mirroring Rust's
// `parse_riff_info`: values are UTF-8 (undecodable items are skipped unless
// strict), and chunks exceeding the list bounds stop or error per meta.ParsingMode.
func parseRiffInfoList(reader io.ReadSeeker, size int64, parseOptions meta.ParseOptions) (*RiffInfoTag, error) {
	tag := &RiffInfoTag{}
	listStart, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	listEnd := listStart + size
	for {
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		// Unread bytes of the list, like the locked Rust `Chunks` reader.
		remaining := listEnd - pos
		if remaining < 8 {
			break
		}
		fcc, szU32, ok, herr := iffReadChunkHeader(reader, binary.LittleEndian, remaining, parseOptions.ParsingMode)
		if herr != nil {
			return nil, &meta.FileParseError{Source: herr}
		}
		if !ok {
			break
		}
		sz := int64(szU32)
		data, err := util.TryByteSlice(int(sz))
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
		}
		value, err := util.Utf8Decode(data)
		switch {
		case err == nil:
			tag.items = append(tag.items, riffInfoItem{string(fcc[:]), value})
		case parseOptions.ParsingMode == meta.ParsingModeStrict:
			return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeRiffInfo, HasTagType: true, Source: err}}
		default:
			// RIFF INFO tags have no standard text encoding, so they will
			// occasionally default to the system encoding — keep trying.
		}
		// Rust's `Chunks::skip` handles the pad byte for every chunk, decoded
		// or not, without eating into the next FourCC.
		after, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		iffCorrectPositionRemaining(reader, szU32, listEnd-after)
	}
	return tag, nil
}

// FileType implements meta.AudioFile.
func (f *WavFile) FileType() meta.FileType { return meta.FileTypeWav }

// GetProperties implements meta.AudioFile.
func (f *WavFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *WavFile) ContainsTag() bool { return f.RiffInfo.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements meta.AudioFile.
func (f *WavFile) ContainsTagType(t meta.TagType) bool {
	switch t {
	case meta.TagTypeRiffInfo:
		return f.RiffInfo.IsSome()
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [meta.TaggedFile].
func (f *WavFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.RiffInfo.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeWav, tags, *f.GetProperties())
}

// SavePath implements meta.AudioFile.
func (f *WavFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

// TagTypes implements meta.AudioFile.
func (f *WavFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeRiffInfo, meta.TagTypeID3v2)
}

// Conflicts implements meta.AudioFile.
func (f *WavFile) Conflicts() [][2]meta.TagType { return nil }

// SaveFile implements meta.AudioFile.
func (f *WavFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.RiffInfo.Get(); ok {
		if err := t.SaveTo(file, wo); err != nil {
			return err
		}
	}
	if t, ok := f.ID3v2.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

// RemoveFromPath implements meta.AudioFile.
func (f *WavFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

// wavWriteRiffInfo replaces or adds a LIST/INFO chunk in a WAV file.
func wavWriteTag(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	if tag.TagType() == meta.TagTypeID3v2 {
		return wavWriteId3v2(file, tag, writeOptions)
	}
	return wavWriteRiffInfo(file, tag, writeOptions)
}

func wavWriteRiffInfo(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	// Rust writes generic RiffInfo tags via tagitems_into_riff, which
	// special-cases popularimeters (merge_tag would store the raw text).
	return wavWriteRiffInfoTag(file, &RiffInfoTag{items: tagitemsIntoRiff(tag)}, writeOptions)
}

// wavWriteRiffInfoTag replaces or adds a LIST/INFO chunk from a concrete tag.
func wavWriteRiffInfoTag(file meta.FileLike, tag *RiffInfoTag, writeOptions meta.WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if !bytes.Equal(hdr[:4], riffPreamble) || !bytes.Equal(hdr[8:12], wavePreamble) {
		return &meta.FileEncodingError{Source: &meta.FakeTagError{}}
	}
	// Rust bounds the scan by the declared RIFF size and errors when the
	// file is shorter than declared.
	declaredSize := binary.LittleEndian.Uint32(hdr[4:])
	streamEnd, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if streamEnd < int64(declaredSize)+8 {
		return &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
	}
	if _, err := file.Seek(12, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	// Find an existing LIST/INFO chunk, walking within the declared RIFF
	// bounds like Rust's `find_info_list`.
	scanBound := int64(declaredSize) + 8
	remaining := int64(declaredSize) - 4
	var listStart, listEnd int64
	for remaining >= 8 {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if pos >= scanBound {
			break
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			return &meta.FileEncodingError{Source: &IffParseError{Source: err}}
		}
		var fcc [4]byte
		copy(fcc[:], ch[:4])
		if !validFourCC(fcc) {
			if writeOptions.ParseOptions.ParsingMode == meta.ParsingModeStrict {
				return &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
			}
			break
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		if int64(size) > remaining {
			if writeOptions.ParseOptions.ParsingMode == meta.ParsingModeStrict {
				return &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
			}
			break
		}
		remaining -= 8
		if bytes.Equal(ch[:4], listPreamble) {
			if size < 4 {
				return &meta.FileEncodingError{Source: &IffParseError{Source: io.ErrUnexpectedEOF}}
			}
			var sub [4]byte
			if _, err := io.ReadFull(file, sub[:]); err != nil {
				return &meta.FileEncodingError{Source: &IffParseError{Source: err}}
			}
			if bytes.Equal(sub[:], infoPreamble) {
				listStart = pos
				// Rust replaces exactly 8+size; a pad byte is NOT consumed
				listEnd = pos + 8 + int64(size)
				break
			}
		}
		if _, err := file.Seek(pos+8+int64(size), io.SeekStart); err != nil {
			break
		}
		// iffCorrectPosition peeks at the pad byte like Rust's Chunks::skip
		remaining -= int64(size) + int64(iffCorrectPosition(file, size, scanBound))
	}

	// Encode the INFO list
	var encoded []byte
	if !tag.IsEmpty() {
		if encoded, err = riffInfoListBytes(tag); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}

	if err := wavInsertChunk(file, listStart, listEnd, encoded); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return nil
}

// riffInfoListBytes encodes the tag as a `LIST<size>INFO` chunk (empty if the
// tag has no items), matching Rust's `create_riff_info`. Empty values are
// skipped like Rust; a payload exceeding u32::MAX is a TooMuchDataError.
func riffInfoListBytes(info *RiffInfoTag) ([]byte, error) {
	if len(info.items) == 0 {
		return nil, nil
	}
	var payload bytes.Buffer
	payload.WriteString("INFO")
	for _, item := range info.items {
		val := item.value
		if val == "" {
			continue
		}
		size := len(val) + 1
		var ch [8]byte
		copy(ch[:4], item.key)
		binary.LittleEndian.PutUint32(ch[4:], uint32(size)) // #nosec G115 -- mirrors Rust `(len as u32)` cast in create_riff_info; truncation is intentional parity
		payload.Write(ch[:])
		payload.WriteString(val)
		payload.WriteByte(0)
		if size&1 == 1 {
			payload.WriteByte(0)
		}
	}
	listSize := payload.Len()
	if listSize > math.MaxUint32 {
		return nil, &meta.TooMuchDataError{}
	}
	var encoded bytes.Buffer
	encoded.WriteString("LIST")
	var szb [4]byte
	binary.LittleEndian.PutUint32(szb[:], uint32(listSize)) // #nosec G115 -- guarded by the explicit `listSize > math.MaxUint32` TooMuchDataError check above
	encoded.Write(szb[:])
	encoded.Write(payload.Bytes())
	return encoded.Bytes(), nil
}

// DumpTo writes the tag as a `LIST<size>INFO` chunk to a writer.
func (t *RiffInfoTag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := riffInfoListBytes(t)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// wavInsertChunk splices an encoded chunk into the file inside the RIFF
// bounds and adjusts the RIFF size by the byte delta. When
// `chunkStart <= 0` the chunk is inserted at the end of the RIFF (before any
// trailing non-RIFF data).
func wavInsertChunk(file meta.FileLike, chunkStart, chunkEnd int64, encoded []byte) error {
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
	} else if len(encoded) > 0 {
		delta = int64(len(encoded))
	}
	// Rust errors TooMuchDataError when the new stream length exceeds u32::MAX;
	// check before mutating the file.
	if riffSize+delta > math.MaxUint32 {
		return &meta.TooMuchDataError{}
	}
	if chunkStart > 0 {
		if err := meta.Splice(file, uint64(chunkStart), uint64(chunkEnd), encoded); err != nil { // #nosec G115 -- chunkStart/chunkEnd are non-negative file positions; this branch is guarded by `chunkStart > 0`
			return err
		}
	} else if len(encoded) > 0 {
		insertAt := 8 + riffSize
		if err := meta.Splice(file, uint64(insertAt), uint64(insertAt), encoded); err != nil {
			return err
		}
	}
	if delta == 0 {
		return nil
	}
	if _, err := file.Seek(4, io.SeekStart); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(sz[:], uint32(riffSize+delta)) // #nosec G115 -- guarded against MaxUint32 above; non-negative because the replaced chunk range lies within the RIFF stream
	_, err = file.Write(sz[:])
	return err
}

// wavWriteId3v2 replaces or adds an "ID3 "/id3 chunk.
func wavWriteId3v2(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	var body []byte
	if !tag.IsEmpty() {
		var err error
		body, err = id3v2.Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}
	return wavWriteId3v2Chunk(file, body, writeOptions)
}

// wavWriteId3v2Chunk replaces or adds an "ID3 "/id3 chunk from serialized
// ID3v2 bytes (empty removes the chunk).
func wavWriteId3v2Chunk(file meta.FileLike, body []byte, writeOptions meta.WriteOptions) error {
	return iffWriteId3v2Chunk(file, body, writeOptions, true, binary.LittleEndian)
}

var junkChunkName = []byte("JUNK")

// iffByteOrder is the chunk size byte order (LE for RIFF, BE for FORM).
type iffByteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

// iffEncodeId3v2Chunk encodes the `ID3 `/`id3 ` chunk (header, content and
// pad byte), mirroring Rust's `write_id3v2_chunk`.
func iffEncodeId3v2Chunk(tag []byte, order iffByteOrder, writeOptions meta.WriteOptions) []byte {
	out := make([]byte, 0, len(tag)+9)
	if writeOptions.UppercaseId3v2Chunk {
		out = append(out, id3ChunkName...)
	} else {
		out = append(out, id3ChunkName2...)
	}
	out = order.AppendUint32(out, uint32(len(tag))) // #nosec G115 -- mirrors Rust `tag.len() as u32` cast in chunk_file.rs; truncation is intentional parity
	out = append(out, tag...)
	if len(tag)%2 != 0 {
		out = append(out, 0)
	}
	return out
}

// iffWriteId3v2Chunk ports Rust's `write_to_chunk_file` (id3/v2/write/
// chunk_file.rs) for WAV and AIFF. An empty `tag` removes the chunk.
func iffWriteId3v2Chunk(file meta.FileLike, tag []byte, writeOptions meta.WriteOptions, isWav bool, order iffByteOrder) error {
	var tagChunkSize int64
	if len(tag) > 0 {
		tagChunkSize = int64(len(tag)) + 8
		if len(tag)%2 != 0 {
			tagChunkSize++
		}
	}

	// Only the first chunk is bounded by the file size; the stream length
	// comes from the first chunk's size, since a file can have trailing junk.
	fileLen, err := file.Len()
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	ctx, err := iffFindExistingId3v2Chunk(file, fileLen, order, writeOptions.ParseOptions.ParsingMode)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	setStreamSize := func(streamLength int64) error {
		if streamLength-8 > math.MaxUint32 {
			return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
		}
		if _, err := file.Seek(4, io.SeekStart); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		var sz [4]byte
		order.PutUint32(sz[:], uint32(streamLength-8)) // #nosec G115 -- guarded against MaxUint32 above; stream lengths include the 8-byte chunk header by construction, so the value is non-negative
		if _, err := file.Write(sz[:]); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		return nil
	}

	if tagChunkSize > math.MaxUint32 {
		return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
	}

	if ctx.found {
		existingLen := ctx.end - ctx.start

		// We can just overwrite tags at the end of the file
		if ctx.end == ctx.streamLength && fileLen == ctx.streamLength {
			updated := ctx.streamLength
			if len(tag) == 0 {
				updated -= existingLen
				if updated-8 > math.MaxUint32 {
					return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
				}
				if err := file.Truncate(ctx.start); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
			} else {
				if existingLen > tagChunkSize {
					updated -= existingLen - tagChunkSize
				} else {
					updated += tagChunkSize - existingLen
				}
				if updated-8 > math.MaxUint32 {
					return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
				}
				if _, err := file.Seek(ctx.start, io.SeekStart); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				if _, err := file.Write(iffEncodeId3v2Chunk(tag, order, writeOptions)); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				if existingLen > tagChunkSize {
					remainder := existingLen - tagChunkSize
					if err := file.Truncate(ctx.end - remainder); err != nil {
						return &meta.FileEncodingError{Source: err}
					}
				}
			}
			return setStreamSize(updated)
		}

		// WAV has a `JUNK` chunk we can use to avoid rewriting the whole
		// file. AIFF has no equivalent, so it always rewrites.
		if isWav && writeOptions.PreferredPadding > 0 {
			if len(tag) == 0 {
				// No need to overwrite the tag: a JUNK FourCC turns it into
				// filler.
				if _, err := file.Seek(ctx.start, io.SeekStart); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				if _, err := file.Write(junkChunkName); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				return nil
			}

			// A JUNK chunk needs at least a chunk header's worth of room
			if existingLen-8 >= tagChunkSize {
				remainder := existingLen - tagChunkSize
				if _, err := file.Seek(ctx.start, io.SeekStart); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				out := iffEncodeId3v2Chunk(tag, order, writeOptions)
				out = append(out, junkChunkName...)
				out = order.AppendUint32(out, uint32(remainder-8)) // #nosec G115 -- mirrors Rust `(remainder - u64::from(IFF_CHUNK_HEADER_SIZE)) as u32` cast; the branch condition keeps remainder-8 non-negative
				if _, err := file.Write(out); err != nil {
					return &meta.FileEncodingError{Source: err}
				}
				return nil
			}
		}
		// Existing tag too small to overwrite: rewrite
	} else if len(tag) == 0 {
		return nil
	}

	// Rust rebuilds the file with the new chunk inserted at the end of the
	// stream and the old chunk drained; Splice yields the same bytes without
	// loading the audio into memory. Validate the resulting stream length first.
	finalStreamLength := ctx.streamLength + tagChunkSize
	if ctx.found {
		finalStreamLength -= ctx.end - ctx.start
	}
	if finalStreamLength-8 > math.MaxUint32 {
		return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
	}
	if len(tag) > 0 {
		at := uint64(ctx.streamLength) // #nosec G115 -- ctx.streamLength is int64(u32 first-chunk size)+8, always positive and far below uint64 overflow
		if err := meta.Splice(file, at, at, iffEncodeId3v2Chunk(tag, order, writeOptions)); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	if ctx.found {
		if err := meta.Splice(file, uint64(ctx.start), uint64(ctx.end), nil); err != nil { // #nosec G115 -- ctx.start/end are non-negative chunk positions within the file, derived from Seek positions and u32 chunk sizes
			return &meta.FileEncodingError{Source: err}
		}
	}
	return setStreamSize(finalStreamLength)
}

type iffId3v2Context struct {
	streamLength int64
	found        bool
	start, end   int64
}

// iffFindExistingId3v2Chunk ports Rust's `find_existing_id3v2_tag`: the first
// chunk (RIFF/FORM) defines the stream length, then its children are walked
// like `Chunks` does (including the 4-byte-larger bound Rust uses).
func iffFindExistingId3v2Chunk(file meta.FileLike, fileLen int64, order iffByteOrder, mode meta.ParsingMode) (iffId3v2Context, error) {
	var ctx iffId3v2Context
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ctx, err
	}

	// `Chunks::next` returning `None` for the first chunk is an unknown format
	if fileLen < 8 {
		return ctx, &meta.UnknownFormatError{}
	}
	_, firstSize, ok, err := iffReadChunkHeader(file, order, fileLen, mode)
	if err != nil {
		return ctx, err
	}
	if !ok {
		return ctx, &meta.UnknownFormatError{}
	}

	actualStreamSize := int64(firstSize) + 8
	// File is lying about its length
	if fileLen < actualStreamSize {
		return ctx, &meta.SizeMismatchError{}
	}
	ctx.streamLength = actualStreamSize

	// RIFF....WAVE
	const firstChunkLen = 12
	if _, err := file.Seek(firstChunkLen, io.SeekStart); err != nil {
		return ctx, err
	}
	total := ctx.streamLength - 8
	remaining := total
	for remaining >= 8 {
		chunkStart := firstChunkLen + (total - remaining)
		fcc, size, ok, err := iffReadChunkHeader(file, order, remaining, mode)
		if err != nil {
			return ctx, err
		}
		if !ok {
			break
		}
		remaining -= 8

		// Skip the content and handle padding like `Chunks::skip`. We can't
		// trust the written chunk size, since some encoders don't handle
		// padding correctly, so the walk position decides where the chunk ends.
		if size > 0 {
			if _, err := file.Seek(int64(size), io.SeekCurrent); err != nil {
				return ctx, err
			}
			remaining -= int64(size)
			if remaining < 0 {
				remaining = 0
			}
		}
		remaining -= int64(iffCorrectPositionRemaining(file, size, remaining))

		if bytes.Equal(fcc[:], id3ChunkName) || bytes.Equal(fcc[:], id3ChunkName2) {
			ctx.found = true
			ctx.start = chunkStart
			ctx.end = firstChunkLen + (total - remaining)
			break
		}
	}
	return ctx, nil
}

// --- AIFF ---

// AiffProperties are the audio properties of an AIFF file.
// AiffCompressionType identifies an AIFC compression algorithm.
type AiffCompressionType struct {
	id   [4]byte
	name string
}

var (
	AiffCompressionNone = AiffCompressionType{id: [4]byte{'N', 'O', 'N', 'E'}, name: "not compressed"}
	AiffCompressionACE2 = aiffCompressionTypeFromId("ACE2")
	AiffCompressionACE8 = aiffCompressionTypeFromId("ACE8")
	AiffCompressionMAC3 = aiffCompressionTypeFromId("MAC3")
	AiffCompressionMAC6 = aiffCompressionTypeFromId("MAC6")
	AiffCompressionSowt = aiffCompressionTypeFromId("sowt")
	AiffCompressionFl32 = aiffCompressionTypeFromId("fl32")
	AiffCompressionFl64 = aiffCompressionTypeFromId("fl64")
	AiffCompressionAlaw = aiffCompressionTypeFromId("alaw")
	AiffCompressionULaw = aiffCompressionTypeFromId("ulaw")
	AiffCompressionALAW = aiffCompressionTypeFromId("ALAW")
	AiffCompressionULAW = aiffCompressionTypeFromId("ULAW")
	AiffCompressionFL32 = aiffCompressionTypeFromId("FL32")
)

// aiffCompressionNames is the set of compression IDs Rust recognizes; other
// IDs map to `Other { compression_name }` read from the chunk.
var aiffCompressionNames = map[[4]byte]string{
	{'N', 'O', 'N', 'E'}: "not compressed",
	{'A', 'C', 'E', '2'}: "ACE 2-to-1",
	{'A', 'C', 'E', '8'}: "ACE 8-to-3",
	{'M', 'A', 'C', '3'}: "MACE 3-to-1",
	{'M', 'A', 'C', '6'}: "MACE 6-to-1",
	{'s', 'o', 'w', 't'}: "",
	{'f', 'l', '3', '2'}: "32-bit floating point",
	{'f', 'l', '6', '4'}: "64-bit floating point",
	{'a', 'l', 'a', 'w'}: "ALaw 2:1",
	{'u', 'l', 'a', 'w'}: "µLaw 2:1",
	{'U', 'L', 'A', 'W'}: "CCITT G.711 u-law",
	{'A', 'L', 'A', 'W'}: "CCITT G.711 A-law",
	{'F', 'L', '3', '2'}: "Float 32",
}

func aiffCompressionTypeFromId(id string) AiffCompressionType {
	var t AiffCompressionType
	copy(t.id[:], id)
	switch id {
	case "NONE":
		t.name = "not compressed"
	case "ACE2":
		t.name = "ACE 2-to-1"
	case "ACE8":
		t.name = "ACE 8-to-3"
	case "MAC3":
		t.name = "MACE 3-to-1"
	case "MAC6":
		t.name = "MACE 6-to-1"
	case "sowt":
		t.name = "" // Has no compression name
	case "fl32":
		t.name = "32-bit floating point"
	case "fl64":
		t.name = "64-bit floating point"
	case "alaw":
		t.name = "ALaw 2:1"
	case "ulaw":
		t.name = "µLaw 2:1"
	case "ULAW":
		t.name = "CCITT G.711 u-law"
	case "ALAW":
		t.name = "CCITT G.711 A-law"
	case "FL32":
		t.name = "Float 32"
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
	BitDepth        uint16
	Channels        uint16
	CompressionType meta.Option[AiffCompressionType]
}

// FileProperties converts to generic [meta.FileProperties].
func (p AiffProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(uint8(p.BitDepth)), // #nosec G115 -- mirrors Rust `value.sample_size as u8` cast; truncation is intentional parity
		meta.Some(uint8(p.Channels)), // #nosec G115 -- mirrors Rust `value.channels as u8` cast; truncation is intentional parity
		meta.None[meta.ChannelMask](),
	)
}

// AiffFile is an AIFF file.
type AiffFile struct {
	Properties AiffProperties
	TextChunks meta.Option[*AiffTextTag]
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
}

// AiffFileReadFrom reads an [AiffFile] from a reader.
func AiffFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*AiffFile, error) {
	var file AiffFile
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	fileLength := uint64(streamEnd - start) // #nosec G115 -- streamEnd and start are non-negative int64 stream positions with streamEnd >= start
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}

	var hdr [12]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
	}
	isAifc := false
	if bytes.Equal(hdr[:4], formPreamble) && bytes.Equal(hdr[8:12], aiffPreamble) {
		isAifc = false
	} else if bytes.Equal(hdr[:4], formPreamble) && bytes.Equal(hdr[8:12], aifcPreamble) {
		isAifc = true
	} else {
		return nil, &meta.FileParseError{Source: &IffParseError{Source: &meta.UnknownFormatError{}}}
	}

	var text *AiffTextTag
	var id3v2Tag *id3v2.Id3v2Tag
	var comm []byte
	var ssndSize uint32

	// Same `Chunks` walker semantics as the WAV reader (big-endian sizes).
	remaining := int64(fileLength) - 12 // #nosec G115 -- fileLength derives from a difference of non-negative int64 stream positions, so the int64 round-trip is lossless
	for remaining >= 8 {
		fcc, size, ok, herr := iffReadChunkHeader(reader, binary.BigEndian, remaining, parseOptions.ParsingMode)
		if herr != nil {
			return nil, &meta.FileParseError{Source: herr}
		}
		if !ok {
			break
		}
		remaining -= 8
		pos, _ := reader.Seek(0, io.SeekCurrent)
		switch {
		case bytes.Equal(fcc[:], commPreamble) && parseOptions.ReadProperties && comm == nil:
			if size < 18 {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: &meta.NotEnoughDataError{Expected: 18, HasExpected: true}}}
			}
			var aerr error
			if comm, aerr = util.TryByteSlice(int(size)); aerr != nil {
				return nil, &meta.FileParseError{Source: aerr}
			}
			if _, err := io.ReadFull(reader, comm); err != nil {
				return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
			}
		case bytes.Equal(fcc[:], aiffComtChunk) && parseOptions.ReadTags && (text == nil || len(text.Comments) == 0):
			if size >= 2 {
				if text == nil {
					text = &AiffTextTag{}
				}
				if err := aiffParseComt(reader, int64(size), text, parseOptions); err != nil {
					return nil, &meta.FileParseError{Source: err}
				}
			}
		case bytes.Equal(fcc[:], aiffNameChunk) && parseOptions.ReadTags && (text == nil || text.Name.IsNone()):
			v, err := iffReadChunkString(reader, size)
			if err != nil {
				return nil, err
			}
			if text == nil {
				text = &AiffTextTag{}
			}
			text.Name = meta.Some(v)
		case bytes.Equal(fcc[:], aiffAuthChunk) && parseOptions.ReadTags && (text == nil || text.Author.IsNone()):
			v, err := iffReadChunkString(reader, size)
			if err != nil {
				return nil, err
			}
			if text == nil {
				text = &AiffTextTag{}
			}
			text.Author = meta.Some(v)
		case bytes.Equal(fcc[:], aiffCpyChunk) && parseOptions.ReadTags && (text == nil || text.Copyright.IsNone()):
			v, err := iffReadChunkString(reader, size)
			if err != nil {
				return nil, err
			}
			if text == nil {
				text = &AiffTextTag{}
			}
			text.Copyright = meta.Some(v)
		case bytes.Equal(fcc[:], aiffAnnoChunk) && parseOptions.ReadTags:
			v, err := iffReadChunkString(reader, size)
			if err != nil {
				return nil, err
			}
			if text == nil {
				text = &AiffTextTag{}
			}
			text.Annotations = append(text.Annotations, v)
		case (bytes.Equal(fcc[:], id3ChunkName) || bytes.Equal(fcc[:], id3ChunkName2)) && parseOptions.ReadTags:
			tag, err := iffReadId3Chunk(reader, size, parseOptions)
			if err != nil {
				return nil, err
			}
			if tag != nil {
				if id3v2Tag != nil {
					for _, fr := range tag.Frames.Frames() {
						id3v2Tag.Frames.Insert(fr)
					}
				} else {
					id3v2Tag = tag
				}
			}
		case bytes.Equal(fcc[:], ssndPreamble) && parseOptions.ReadProperties:
			ssndSize = size
		}
		if _, err := reader.Seek(pos+int64(size), io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: &IffParseError{Source: err}}
		}
		remaining -= int64(size)
		remaining -= int64(iffCorrectPosition(reader, size, streamEnd))
	}

	if parseOptions.ReadProperties {
		if comm == nil {
			return nil, &meta.FileParseError{Source: &IffParseError{Message: "AIFF file is missing a COMM chunk"}}
		}
		if ssndSize == 0 {
			return nil, &meta.FileParseError{Source: &IffParseError{Message: "file does not contain an \"SSND\" chunk"}}
		}
		// Rust passes `data.stream_position()` — the position where the chunk
		// walk ended, not necessarily EOF.
		walkEnd, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if err := aiffReadProperties(comm, isAifc, ssndSize, uint64(walkEnd-start), &file.Properties); err != nil { // #nosec G115 -- walkEnd and start are non-negative stream positions with walkEnd >= start, so the difference fits int64
			return nil, &meta.FileParseError{Source: err}
		}
	}

	if parseOptions.ReadTags {
		// Rust returns None when all text chunks are empty
		if text != nil && text.Name.IsNone() && text.Author.IsNone() && text.Copyright.IsNone() &&
			len(text.Annotations) == 0 && len(text.Comments) == 0 {
			text = nil
		}
		file.TextChunks = util.OptionFromPtr(text)
		file.ID3v2 = util.OptionFromPtr(id3v2Tag)
	}
	return &file, nil
}

// iffReadChunkString reads a chunk's content as a UTF-8 string, mirroring
// Rust's `Chunk::read_string` (decode errors propagate).
func iffReadChunkString(reader io.Reader, size uint32) (string, error) {
	data, err := util.TryByteSlice(int(size))
	if err != nil {
		return "", &meta.FileParseError{Source: err}
	}
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", &meta.FileParseError{Source: &IffParseError{Source: err}}
	}
	v, err := util.Utf8Decode(data)
	if err != nil {
		return "", &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeAiffText, HasTagType: true, Source: err}}
	}
	return v, nil
}

// aiffReadProperties computes AIFF properties from the COMM chunk content,
// mirroring Rust's aiff::properties::read_properties.
func aiffReadProperties(comm []byte, isAifc bool, streamLen uint32, fileLength uint64, p *AiffProperties) error {
	channels := binary.BigEndian.Uint16(comm[0:])
	if channels == 0 {
		return &IffParseError{Message: "file specifies 0 audio channels"}
	}
	sampleFrames := binary.BigEndian.Uint32(comm[2:])
	sampleSize := binary.BigEndian.Uint16(comm[6:])
	var rate10 [10]byte
	copy(rate10[:], comm[8:18])
	rateF := util.F80FromBEBytes(rate10).AsF64()
	if math.IsNaN(rateF) || math.IsInf(rateF, 0) || math.Signbit(rateF) {
		return &IffParseError{Message: "invalid sample rate"}
	}
	sampleRate := util.F64ToU32(math.Round(rateF))

	if sampleRate > 0 && sampleFrames > 0 {
		length := float64(sampleFrames) * 1000 / float64(sampleRate)
		p.Duration = util.DurationFromMillis(uint64(length))
		p.OverallBitrate = util.F64ToU32(float64(fileLength)*8/length + 0.5)
		p.AudioBitrate = util.F64ToU32(float64(streamLen)*8/length + 0.5)
	}
	p.SampleRate = sampleRate
	p.BitDepth = sampleSize
	p.Channels = channels

	// Only AIFC files can carry a compression type.
	if len(comm) >= 5+18 && isAifc {
		var id [4]byte
		copy(id[:], comm[18:22])
		ct := AiffCompressionTypeFromId(id)
		if _, known := aiffCompressionNames[ct.id]; !known {
			// Unknown compression: read the Pascal-string name
			// (Rust: read_u8 / read_exact / utf8_decode, all with `?`)
			var name string
			if len(comm) < 23 {
				return &IffParseError{Source: io.ErrUnexpectedEOF}
			}
			if nameLen := int(comm[22]); nameLen > 0 {
				if len(comm) < 23+nameLen {
					return &IffParseError{Source: io.ErrUnexpectedEOF}
				}
				var derr error
				if name, derr = util.Utf8Decode(comm[23 : 23+nameLen]); derr != nil {
					return &IffParseError{Source: derr}
				}
			}
			ct.name = name
		}
		p.CompressionType = meta.Some(ct)
	}
	return nil
}

// FileType implements meta.AudioFile.
func (f *AiffFile) FileType() meta.FileType { return meta.FileTypeAiff }

// GetProperties implements meta.AudioFile.
func (f *AiffFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *AiffFile) ContainsTag() bool { return f.TextChunks.IsSome() || f.ID3v2.IsSome() }

// ContainsTagType implements meta.AudioFile.
func (f *AiffFile) ContainsTagType(t meta.TagType) bool {
	switch t {
	case meta.TagTypeAiffText:
		return f.TextChunks.IsSome()
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [meta.TaggedFile].
func (f *AiffFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.TextChunks.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeAiff, tags, *f.GetProperties())
}

// SavePath implements meta.AudioFile.
func (f *AiffFile) SavePath(path string, wo meta.WriteOptions) error {
	return meta.SaveToPath(path, f, wo)
}

// TagTypes implements meta.AudioFile.
func (f *AiffFile) TagTypes() []meta.TagType {
	return meta.PresentTagTypes(f.ContainsTagType, meta.TagTypeAiffText, meta.TagTypeID3v2)
}

// Conflicts implements meta.AudioFile.
func (f *AiffFile) Conflicts() [][2]meta.TagType { return nil }

// SaveFile implements meta.AudioFile.
func (f *AiffFile) SaveFile(file meta.FileLike, wo meta.WriteOptions) error {
	if t, ok := f.TextChunks.Get(); ok {
		if err := t.SaveTo(file, wo); err != nil {
			return err
		}
	}
	if t, ok := f.ID3v2.Get(); ok {
		return t.SaveTo(file, wo)
	}
	return nil
}

// RemoveFromPath implements meta.AudioFile.
func (f *AiffFile) RemoveFromPath(path string, t meta.TagType) error {
	return meta.RemoveTagFromPath(path, t)
}

// aiffWriteTag writes NAME/AUTH/(c) /ANNO chunks in an AIFF file.
func aiffWriteTag(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	if tag.TagType() == meta.TagTypeID3v2 {
		return aiffWriteId3v2(file, tag, writeOptions)
	}
	return aiffWriteText(file, tag, writeOptions)
}

// aiffTextChunkBytes encodes the tag's text chunks (COMT first, then
// NAME/AUTH/(c) /ANNO), matching Rust's `AiffTextChunks::dump_to`.
func aiffTextChunkBytes(text *AiffTextTag) ([]byte, error) {
	var encoded bytes.Buffer
	writeChunk := func(name [4]byte, value string) {
		if uint64(len(value)) > math.MaxUint32 {
			return
		}
		size := uint32(len(value)) // #nosec G115 -- guarded by the MaxUint32 check directly above, mirroring Rust's u32::try_from skip
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
			return nil, &meta.TooMuchDataError{}
		}
		encoded.Write(aiffComtChunk)
		comtSizePos := encoded.Len()
		encoded.Write([]byte{0, 0, 0, 0})
		var nb [8]byte
		binary.BigEndian.PutUint16(nb[:], uint16(len(text.Comments))) // #nosec G115 -- guarded by the `len(text.Comments) > 0xFFFF` TooMuchDataError check above
		encoded.Write(nb[:2])
		for _, c := range text.Comments {
			if len(c.Text) > 0xFFFF {
				return nil, &meta.TooMuchDataError{}
			}
			binary.BigEndian.PutUint32(nb[0:], c.Timestamp)
			binary.BigEndian.PutUint16(nb[4:], c.MarkerID)
			binary.BigEndian.PutUint16(nb[6:], uint16(len(c.Text))) // #nosec G115 -- guarded by the `len(c.Text) > 0xFFFF` TooMuchDataError check above
			encoded.Write(nb[:8])
			encoded.WriteString(c.Text)
			if len(c.Text)%2 != 0 {
				encoded.WriteByte(0)
			}
		}
		comtLen64 := uint64(encoded.Len() - comtSizePos - 4) // #nosec G115 -- subtraction is non-negative by construction: 4 placeholder bytes plus the 2-byte comment count were written after comtSizePos
		if comtLen64 > math.MaxUint32 {
			return nil, &meta.TooMuchDataError{}
		}
		comtLen := uint32(comtLen64)
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
func (t *AiffTextTag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := aiffTextChunkBytes(t)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	_, err = writer.Write(data)
	return err
}

func aiffWriteText(file meta.FileLike, tag *meta.Tag, wo meta.WriteOptions) error {
	return aiffWriteTextChunks(file, aiffTextFromGeneric(tag), wo)
}

// aiffTextFromGeneric builds an AiffTextTag from a generic Tag's items,
// matching Rust's AiffTextChunksRef conversion: name/author/copyright from
// the mapped strings, annotations from Comment items, and COMT comments
// replayed from the preserved companion tag.
func aiffTextFromGeneric(tag *meta.Tag) *AiffTextTag {
	t := &AiffTextTag{}
	if s, ok := tag.GetString(meta.ItemKeyTrackTitle); ok {
		t.Name = meta.Some(s)
	}
	if s, ok := tag.GetString(meta.ItemKeyTrackArtist); ok {
		t.Author = meta.Some(s)
	}
	if s, ok := tag.GetString(meta.ItemKeyCopyrightMessage); ok {
		t.Copyright = meta.Some(s)
	}
	t.Annotations = tag.GetStrings(meta.ItemKeyComment)
	// Only COMT comments come from the companion (Rust's TagRef appends it).
	if comp, ok := tag.CompanionTag().(*AiffTextTag); ok && comp != nil {
		t.Comments = comp.Comments
	}
	return t
}

// aiffWriteTextChunks replaces the AIFF text chunks from a concrete tag.
func aiffWriteTextChunks(file meta.FileLike, tag *AiffTextTag, writeOptions meta.WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if !bytes.Equal(hdr[:4], formPreamble) || (!bytes.Equal(hdr[8:12], aiffPreamble) && !bytes.Equal(hdr[8:12], aifcPreamble)) {
		return &meta.FileEncodingError{Source: &meta.FakeTagError{}}
	}

	streamEnd, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if _, err := file.Seek(12, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}

	// Collect ranges of existing text chunks to remove, tracking the COMM
	// chunk end (Rust's find_text_chunks, bounded by the actual file length).
	type rng struct{ start, end int64 }
	var ranges []rng
	var commEnd int64
	remaining := streamEnd - 12
	for remaining >= 8 {
		var ch [8]byte
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return &meta.FileEncodingError{Source: err}
		}
		if _, err := io.ReadFull(file, ch[:]); err != nil {
			break
		}
		var fcc [4]byte
		copy(fcc[:], ch[:4])
		if !validFourCC(fcc) {
			if writeOptions.ParseOptions.ParsingMode == meta.ParsingModeStrict {
				return &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
			}
			break
		}
		size := binary.BigEndian.Uint32(ch[4:])
		if int64(size) > remaining {
			if writeOptions.ParseOptions.ParsingMode == meta.ParsingModeStrict {
				return &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
			}
			break
		}
		isText := bytes.Equal(ch[:4], aiffNameChunk) || bytes.Equal(ch[:4], aiffAuthChunk) ||
			bytes.Equal(ch[:4], aiffCpyChunk) || bytes.Equal(ch[:4], aiffAnnoChunk) ||
			bytes.Equal(ch[:4], aiffComtChunk)
		end := pos + 8 + int64(size)
		if _, err := file.Seek(end, io.SeekStart); err != nil {
			break
		}
		end += int64(iffCorrectPosition(file, size, streamEnd))
		remaining = streamEnd - end
		switch {
		case isText:
			ranges = append(ranges, rng{pos, end})
		case bytes.Equal(ch[:4], commPreamble):
			commEnd = end
		}
	}
	if commEnd == 0 {
		return &meta.FileEncodingError{Source: &IffParseError{Message: `file is missing a "COMM" chunk`}}
	}
	encoded, err := aiffTextChunkBytes(tag)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if len(ranges) == 0 {
		// No existing text chunks: insert right after COMM (Rust)
		if err := meta.Splice(file, uint64(commEnd), uint64(commEnd), encoded); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	} else {
		// Remove all but the first range (back to front), then splice the
		// new chunks at the first range's position (Rust).
		for i := len(ranges) - 1; i >= 1; i-- {
			if err := meta.Splice(file, uint64(ranges[i].start), uint64(ranges[i].end), nil); err != nil { // #nosec G115 -- range start/end are non-negative chunk positions within the file, derived from Seek positions and u32 chunk sizes
				return &meta.FileEncodingError{Source: err}
			}
		}
		if err := meta.Splice(file, uint64(ranges[0].start), uint64(ranges[0].end), encoded); err != nil { // #nosec G115 -- range start/end are non-negative chunk positions within the file, derived from Seek positions and u32 chunk sizes
			return &meta.FileEncodingError{Source: err}
		}
	}
	return iffFixSize(file)
}

func aiffWriteId3v2(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	var body []byte
	if !tag.IsEmpty() {
		var err error
		body, err = id3v2.Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}
	return aiffWriteId3v2Chunk(file, body, writeOptions)
}

// aiffWriteId3v2Chunk replaces or adds an "ID3 " chunk from serialized ID3v2
// bytes (empty removes the chunk).
func aiffWriteId3v2Chunk(file meta.FileLike, body []byte, writeOptions meta.WriteOptions) error {
	return iffWriteId3v2Chunk(file, body, writeOptions, false, binary.BigEndian)
}

// iffFixSize updates the FORM chunk size to fileLen - 8 (big-endian).
func iffFixSize(file meta.FileLike) error {
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if end-8 > math.MaxUint32 {
		return &meta.TooMuchDataError{}
	}
	if _, err := file.Seek(4, io.SeekStart); err != nil {
		return err
	}
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(end-8)) // #nosec G115 -- end is the file length (>= 12 for the verified FORM header) and end-8 is guarded against MaxUint32 above
	_, err = file.Write(sz[:])
	return err
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeWav, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := WavFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterFormatReader(meta.FileTypeAiff, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := AiffFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterConcreteTagWriter(meta.FileTypeWav, meta.TagTypeID3v2, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
		return wavWriteId3v2Chunk(file.File(), tag.([]byte), writeOptions)
	})
	meta.RegisterConcreteTagWriter(meta.FileTypeAiff, meta.TagTypeID3v2, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
		return aiffWriteId3v2Chunk(file.File(), tag.([]byte), writeOptions)
	})
	meta.RegisterFormatWriter(meta.FileTypeWav, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		switch tag.TagType() {
		case meta.TagTypeRiffInfo, meta.TagTypeID3v2:
			return wavWriteTag(file.File(), tag, writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
	meta.RegisterFormatWriter(meta.FileTypeAiff, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		switch tag.TagType() {
		case meta.TagTypeAiffText, meta.TagTypeID3v2:
			return aiffWriteTag(file.File(), tag, writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
	meta.RegisterTagDumper(meta.TagTypeRiffInfo, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		// Rust dumps via RIFFInfoListRef{tagitems_into_riff(tag.items())}
		data, err := riffInfoListBytes(&RiffInfoTag{items: tagitemsIntoRiff(tag)})
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	meta.RegisterTagDumper(meta.TagTypeAiffText, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		// Rust's dump path uses AiffTextChunksRef (comments: None)
		data, err := aiffTextChunkBytes(aiffTextFromGeneric(tag))
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}

// SaveTo writes the tag into a WAV file, mirroring Rust `TagExt::save_to`.
func (t *RiffInfoTag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeRiffInfo)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return wavWriteRiffInfoTag(vf.File(), t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *RiffInfoTag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeRiffInfo)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return wavWriteRiffInfoTag(vf.File(), t, writeOptions)
}

// RemoveFrom removes a RIFF INFO list from a [meta.FileLike].
func (t *RiffInfoTag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeRiffInfo.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes a RIFF INFO list from a path.
func (t *RiffInfoTag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeRiffInfo.RemoveFromPath(path, writeOptions)
}

// SaveTo writes the tag into an AIFF file, mirroring Rust `TagExt::save_to`.
func (t *AiffTextTag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeAiffText)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return aiffWriteTextChunks(vf.File(), t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *AiffTextTag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeAiffText)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return aiffWriteTextChunks(vf.File(), t, writeOptions)
}

// RemoveFrom removes the AIFF text chunks from a [meta.FileLike].
func (t *AiffTextTag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeAiffText.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes the AIFF text chunks from a path.
func (t *AiffTextTag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeAiffText.RemoveFromPath(path, writeOptions)
}
