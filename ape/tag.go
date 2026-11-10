package ape

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
)

var apePreamble = []byte("APETAGEX")

const (
	apeHeaderContainsHeader uint32 = 1 << 31
	apeHeaderContainsFooter uint32 = 1 << 30
	apeHeaderIsHeader       uint32 = 1 << 29
)

var apeInvalidKeys = []string{"ID3", "TAG", "OGGS", "MP+"}

// ApeTagItemParseError occurs when a single item in an APE tag fails to parse.
type ApeTagItemParseError struct {
	Key     string
	Message string
	Source  error
}

func (e *ApeTagItemParseError) Error() string {
	if e.Key != "" {
		return "APE: item " + e.Key + ": " + e.Message
	}
	return "APE: " + e.Message
}
func (e *ApeTagItemParseError) Unwrap() error { return e.Source }

// ApeTagParseError is returned when an APE tag fails to parse.
type ApeTagParseError struct {
	Message string
	Source  error
}

func (e *ApeTagParseError) Error() string {
	if e.Message != "" {
		return "APE: " + e.Message
	}
	return "failed to parse APE tag"
}
func (e *ApeTagParseError) Unwrap() error { return e.Source }

// ApeTagEncodingError is returned when an APE tag fails to encode.
type ApeTagEncodingError struct {
	Message string
	Source  error
}

func (e *ApeTagEncodingError) Error() string {
	if e.Message != "" {
		return "APE: " + e.Message
	}
	return "failed to encode APE tag"
}
func (e *ApeTagEncodingError) Unwrap() error { return e.Source }

// ApeParseError is returned when APE audio properties fail to parse.
type ApeParseError struct {
	Message string
	Source  error
}

func (e *ApeParseError) Error() string {
	if e.Message != "" {
		return "APE: " + e.Message
	}
	return "failed to parse APE file"
}
func (e *ApeParseError) Unwrap() error { return e.Source }

// ApeHeader is an APE tag header/footer.
type ApeHeader struct {
	Version   uint32
	Size      uint32
	ItemCount uint32
	Flags     uint32
}

// ReadApeHeader reads an APE header/footer's fields after the preamble.
// If `footer` is set, the just-read 32 bytes were a footer and the reader is
// seeked back to the start of the tag's items (or header).
func ReadApeHeader(reader io.ReadSeeker, footer bool) (*ApeHeader, error) {
	var data [24]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return nil, err
	}
	version := binary.LittleEndian.Uint32(data[0:])
	size := binary.LittleEndian.Uint32(data[4:])
	if size < 32 {
		return nil, &ApeTagParseError{Source: &meta.SizeMismatchError{}}
	}
	h := &ApeHeader{
		Version:   version,
		Size:      size,
		ItemCount: binary.LittleEndian.Uint32(data[8:]),
		Flags:     binary.LittleEndian.Uint32(data[12:]),
	}
	if footer {
		// Seek back to the end of the header
		if _, err := reader.Seek(-int64(size), io.SeekCurrent); err != nil {
			return nil, err
		}
		// Version 1 doesn't include a header
		if version == 2000 && h.Flags&apeHeaderContainsHeader != 0 {
			h.Size += 32
		}
	} else if version == 2000 && h.Flags&apeHeaderContainsFooter != 0 {
		h.Size += 32
	}
	if streamLength, err := util.StreamLen(reader); err == nil {
		if uint64(h.Size) > streamLength {
			return nil, &ApeTagParseError{Source: &meta.SizeMismatchError{}}
		}
	}
	return h, nil
}

// ApeItem is a single item in an [ApeTag].
type ApeItem struct {
	ReadOnly bool
	Key      string
	Value    meta.ItemValue
}

// NewApeItem creates an [ApeItem].
func NewApeItem(key string, value meta.ItemValue) (*ApeItem, error) {
	for _, k := range apeInvalidKeys {
		if strings.EqualFold(k, key) {
			return nil, &ApeTagItemParseError{Key: key, Message: "invalid key"}
		}
	}
	if len(key) < 2 || len(key) > 255 {
		return nil, &ApeTagItemParseError{Key: key, Message: "invalid key length"}
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] > 0x7E {
			return nil, &ApeTagItemParseError{Key: key, Message: "invalid characters in key"}
		}
	}
	return &ApeItem{Key: key, Value: value}, nil
}

// ApeTag is an APEv1/APEv2 tag.
type ApeTag struct {
	ReadOnly bool
	items    []*ApeItem
}

// NewApeTag creates an empty [ApeTag].
func NewApeTag() *ApeTag { return &ApeTag{} }

// Items returns the tag's items.
func (t *ApeTag) Items() []*ApeItem { return t.items }

// Item gets an item by key.
func (t *ApeTag) Item(key string) (*ApeItem, bool) {
	for _, i := range t.items {
		if strings.EqualFold(i.Key, key) {
			return i, true
		}
	}
	return nil, false
}

// Push appends an item to the tag. If an item with the same key exists and
// both values are the same text-based variant (text or locator), the value is
// merged with a NUL separator; otherwise the existing item is replaced
// (Rust `ApeTag::push`).
func (t *ApeTag) Push(item *ApeItem) {
	for _, existing := range t.items {
		if !strings.EqualFold(existing.Key, item.Key) {
			continue
		}
		eText, eOk := existing.Value.IntoString()
		nText, nOk := item.Value.IntoString()
		if eOk && nOk && existing.Value.Kind() == item.Value.Kind() {
			existing.Value = itemValueFromKind(item.Value.Kind(), eText+"\x00"+nText)
			return
		}
		*existing = *item
		return
	}
	t.items = append(t.items, item)
}

// insert removes any item with the same key, then appends (Rust
// `ApeTag::insert`).
func (t *ApeTag) insert(item *ApeItem) {
	t.Remove(item.Key)
	t.items = append(t.items, item)
}

func itemValueFromKind(kind meta.ItemValueKind, text string) meta.ItemValue {
	if kind == meta.ItemValueKindLocator {
		return meta.ItemValueLocator(text)
	}
	return meta.ItemValueText(text)
}

// Insert inserts a generic [meta.TagItem]. Pictures are stored for APE_PICTURE_TYPES keys.
func (t *ApeTag) Insert(item meta.TagItem) error {
	if item.Key() == meta.ItemKeyUnknown {
		if len(item.Description) < 2 {
			return nil
		}
		apeItem, err := NewApeItem(item.Description, item.Value())
		if err != nil {
			return err
		}
		t.Push(apeItem)
		return nil
	}
	// Number-pair keys are normalized through the pair setters — whitespace is
	// trimmed and the value merges into the "N/T" field (Rust `set_number`).
	switch item.Key() {
	case meta.ItemKeyTrackNumber, meta.ItemKeyTrackTotal, meta.ItemKeyDiscNumber, meta.ItemKeyDiscTotal:
		if text, ok := item.Value().IntoString(); ok {
			if n, err := strconv.ParseUint(strings.TrimSpace(text), 10, 32); err == nil {
				switch item.Key() {
				case meta.ItemKeyTrackNumber:
					t.SetTrack(uint32(n))
				case meta.ItemKeyTrackTotal:
					t.SetTrackTotal(uint32(n))
				case meta.ItemKeyDiscNumber:
					t.SetDisk(uint32(n))
				case meta.ItemKeyDiscTotal:
					t.SetDiskTotal(uint32(n))
				}
			}
		}
		return nil
	}
	key, ok := item.Key().MapKey(meta.TagTypeApe)
	if !ok {
		return nil
	}
	if key == "Cover Art (Front)" || key == "Cover Art (Back)" {
		return nil
	}
	var value meta.ItemValue
	if item.Value().Kind() == meta.ItemValueKindBinary {
		value = item.Value()
	} else {
		text, ok := item.Value().IntoString()
		if !ok {
			return nil
		}
		if item.Value().Kind() == meta.ItemValueKindLocator {
			value = meta.ItemValueLocator(text)
		} else {
			value = meta.ItemValueText(text)
		}
	}
	apeItem, err := NewApeItem(key, value)
	if err != nil {
		return err
	}
	t.Push(apeItem)
	return nil
}

// Remove deletes all items with the key (case-insensitive), mirroring Rust's
// `ApeTag::remove`.
func (t *ApeTag) Remove(key string) {
	var kept []*ApeItem
	for _, item := range t.items {
		if !strings.EqualFold(item.Key, key) {
			kept = append(kept, item)
		}
	}
	t.items = kept
}

// IsEmpty reports whether the tag has no items.
func (t *ApeTag) IsEmpty() bool { return len(t.items) == 0 }

// Clear removes all items.
func (t *ApeTag) Clear() { t.items = nil }

// Len returns the number of items.
func (t *ApeTag) Len() int { return len(t.items) }

// GetText returns the text value for a key (case-insensitive).
func (t *ApeTag) GetText(key string) (string, bool) {
	if item, ok := t.Item(key); ok {
		return item.Value.IntoString()
	}
	return "", false
}

// SetText replaces an item's text value (invalid keys are ignored).
func (t *ApeTag) SetText(key, value string) {
	if item, err := NewApeItem(key, meta.ItemValueText(value)); err == nil {
		t.insert(item)
	}
}

// Artist returns the artist.
func (t *ApeTag) Artist() (string, bool) { return t.GetText("Artist") }

// SetArtist sets the artist.
func (t *ApeTag) SetArtist(value string) { t.SetText("Artist", value) }

// RemoveArtist removes the artist.
func (t *ApeTag) RemoveArtist() { t.Remove("Artist") }

// Title returns the title.
func (t *ApeTag) Title() (string, bool) { return t.GetText("Title") }

// SetTitle sets the title.
func (t *ApeTag) SetTitle(value string) { t.SetText("Title", value) }

// RemoveTitle removes the title.
func (t *ApeTag) RemoveTitle() { t.Remove("Title") }

// Album returns the album.
func (t *ApeTag) Album() (string, bool) { return t.GetText("Album") }

// SetAlbum sets the album.
func (t *ApeTag) SetAlbum(value string) { t.SetText("Album", value) }

// RemoveAlbum removes the album.
func (t *ApeTag) RemoveAlbum() { t.Remove("Album") }

// Genre returns the genre.
func (t *ApeTag) Genre() (string, bool) { return t.GetText("GENRE") }

// SetGenre sets the genre.
func (t *ApeTag) SetGenre(value string) { t.SetText("GENRE", value) }

// RemoveGenre removes the genre.
func (t *ApeTag) RemoveGenre() { t.Remove("GENRE") }

// Comment returns the comment.
func (t *ApeTag) Comment() (string, bool) { return t.GetText("Comment") }

// SetComment sets the comment.
func (t *ApeTag) SetComment(value string) { t.SetText("Comment", value) }

// RemoveComment removes the comment.
func (t *ApeTag) RemoveComment() { t.Remove("Comment") }

// splitNumPair splits a "num/total" text item, mirroring Rust's
// `ApeTag::split_num_pair`.
func (t *ApeTag) splitNumPair(key string) (meta.Option[uint32], meta.Option[uint32]) {
	text, ok := t.GetText(key)
	if !ok {
		return meta.None[uint32](), meta.None[uint32]()
	}
	var num, total meta.Option[uint32]
	parts := strings.SplitN(text, "/", 3)
	if n, ok := util.U32FromString(parts[0]); ok {
		num = meta.Some(n)
	}
	if len(parts) > 1 {
		if n, ok := util.U32FromString(parts[1]); ok {
			total = meta.Some(n)
		}
	}
	return num, total
}

// insertNumberPair formats and inserts a "num/total" pair (Rust's
// `insert_number_pair` + `format_number_pair`).
func (t *ApeTag) insertNumberPair(key string, number, total meta.Option[uint32]) {
	var value string
	n, hasN := number.Get()
	tot, hasT := total.Get()
	switch {
	case hasN && hasT:
		value = util.Itoa(int(n)) + "/" + util.Itoa(int(tot))
	case hasN:
		value = util.Itoa(int(n))
	case hasT:
		value = "0/" + util.Itoa(int(tot))
	default:
		return
	}
	t.SetText(key, value)
}

// Track returns the track number from the "Track" pair.
func (t *ApeTag) Track() (uint32, bool) {
	n, _ := t.splitNumPair("Track")
	return n.Get()
}

// SetTrack sets the track number, keeping the total.
func (t *ApeTag) SetTrack(value uint32) {
	_, total := t.splitNumPair("Track")
	t.insertNumberPair("Track", meta.Some(value), total)
}

// RemoveTrack removes the track.
func (t *ApeTag) RemoveTrack() { t.Remove("Track") }

// TrackTotal returns the track total from the "Track" pair.
func (t *ApeTag) TrackTotal() (uint32, bool) {
	_, total := t.splitNumPair("Track")
	return total.Get()
}

// SetTrackTotal sets the track total, keeping the number.
func (t *ApeTag) SetTrackTotal(value uint32) {
	num, _ := t.splitNumPair("Track")
	t.insertNumberPair("Track", num, meta.Some(value))
}

// RemoveTrackTotal removes the total, keeping the bare track number.
func (t *ApeTag) RemoveTrackTotal() {
	num, _ := t.splitNumPair("Track")
	t.Remove("Track")
	if n, ok := num.Get(); ok {
		t.SetText("Track", util.Itoa(int(n)))
	}
}

// Disk returns the disc number from the "Disc" pair.
func (t *ApeTag) Disk() (uint32, bool) {
	n, _ := t.splitNumPair("Disc")
	return n.Get()
}

// SetDisk sets the disc number, keeping the total.
func (t *ApeTag) SetDisk(value uint32) {
	_, total := t.splitNumPair("Disc")
	t.insertNumberPair("Disc", meta.Some(value), total)
}

// RemoveDisk removes the disc.
func (t *ApeTag) RemoveDisk() { t.Remove("Disc") }

// DiskTotal returns the disc total from the "Disc" pair.
func (t *ApeTag) DiskTotal() (uint32, bool) {
	_, total := t.splitNumPair("Disc")
	return total.Get()
}

// SetDiskTotal sets the disc total, keeping the number.
func (t *ApeTag) SetDiskTotal(value uint32) {
	num, _ := t.splitNumPair("Disc")
	t.insertNumberPair("Disc", num, meta.Some(value))
}

// RemoveDiskTotal removes the total, keeping the bare disc number.
func (t *ApeTag) RemoveDiskTotal() {
	num, _ := t.splitNumPair("Disc")
	t.Remove("Disc")
	if n, ok := num.Get(); ok {
		t.SetText("Disc", util.Itoa(int(n)))
	}
}

// Date returns the recording date from the "Year" item.
func (t *ApeTag) Date() meta.Option[meta.Timestamp] {
	if s, ok := t.GetText("Year"); ok {
		return util.TryParseTimestamp(s)
	}
	return meta.None[meta.Timestamp]()
}

// SetDate sets the recording date ("Year").
func (t *ApeTag) SetDate(value meta.Timestamp) { t.SetText("Year", value.String()) }

// RemoveDate removes the recording date.
func (t *ApeTag) RemoveDate() { t.Remove("Year") }

// Tag converts the [ApeTag] to a generic [Tag].
func (t *ApeTag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeApe)
	for _, item := range t.items {
		isPic := false
		var picType meta.PictureType
		if idx := util.IndexOfStr(meta.ApePictureTypes[:], item.Key); idx >= 0 {
			picType = meta.PictureType(idx)
			isPic = true
		}
		if isPic {
			if binary, ok := item.Value.IntoBinary(); ok {
				if pic, err := meta.PictureFromApeBytes(item.Key, binary); err == nil {
					pic.SetPicType(picType)
					tag.PushPicture(pic)
				}
			}
			continue
		}
		ik := meta.ItemKeyOrUnknown(meta.TagTypeApe, item.Key)
		if ik != meta.ItemKeyUnknown {
			if text, ok := item.Value.IntoString(); ok {
				var curKey, totKey meta.ItemKey
				switch ik {
				case meta.ItemKeyTrackNumber, meta.ItemKeyTrackTotal:
					curKey, totKey = meta.ItemKeyTrackNumber, meta.ItemKeyTrackTotal
				case meta.ItemKeyDiscNumber, meta.ItemKeyDiscTotal:
					curKey, totKey = meta.ItemKeyDiscNumber, meta.ItemKeyDiscTotal
				}
				// "Track"/"Disc" items may hold "N/T" pairs; split them
				if totKey != 0 {
					parts := strings.SplitN(text, "/", 2)
					tag.PushUnchecked(meta.NewTagItem(curKey, meta.ItemValueText(parts[0])))
					if len(parts) > 1 {
						tag.PushUnchecked(meta.NewTagItem(totKey, meta.ItemValueText(parts[1])))
					}
					continue
				}
				// Multi-value text/locator items split on NUL (Rust `split_tag`)
				if strings.Contains(text, "\x00") {
					for _, value := range strings.Split(text, "\x00") {
						tag.PushUnchecked(meta.NewTagItem(ik, itemValueFromKind(item.Value.Kind(), value)))
					}
					continue
				}
			}
		}
		tagItem := meta.NewTagItem(ik, item.Value)
		if ik == meta.ItemKeyUnknown {
			tagItem.Description = item.Key
		}
		tag.PushUnchecked(tagItem)
	}
	return tag
}

// SaveTo writes the tag to a [meta.FileLike].
//
// Unlike going through [ApeTag.Tag] and [Tag.SaveTo], this writes items
// verbatim — keys are preserved exactly rather than canonicalized.
func (t *ApeTag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeApe)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if err := ApeWriteTo(file, t, writeOptions); err != nil {
		return &meta.FileEncodingError{FileType: format, Source: err}
	}
	return nil
}

// SaveToPath writes the tag to a path.
func (t *ApeTag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return err
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeApe)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	if err := ApeWriteTo(meta.NewOSFile(f), t, writeOptions); err != nil {
		return &meta.FileEncodingError{FileType: format, Source: err}
	}
	return nil
}

// numberPairKeys are the meta.ItemKeys stored in "N/T" APE fields.
var apeNumberPairKeys = map[meta.ItemKey]bool{
	meta.ItemKeyTrackNumber: true, meta.ItemKeyTrackTotal: true,
	meta.ItemKeyDiscNumber: true, meta.ItemKeyDiscTotal: true,
}

// TagFromApe converts a generic [Tag] to an [ApeTag].
func ApeTagFromTag(input *meta.Tag) *ApeTag {
	apeTag := NewApeTag()
	for _, item := range input.Items() {
		// Number pairs are merged below
		if apeNumberPairKeys[item.Key()] {
			continue
		}
		_ = apeTag.Insert(item)
	}
	for _, pair := range [][2]meta.ItemKey{
		{meta.ItemKeyTrackNumber, meta.ItemKeyTrackTotal},
		{meta.ItemKeyDiscNumber, meta.ItemKeyDiscTotal},
	} {
		parseNum := func(key meta.ItemKey) meta.Option[uint32] {
			// Rust `set_number` runs per item; each parseable value
			// overwrites via the setter, so the last parseable wins.
			var n meta.Option[uint32]
			for _, item := range input.GetItems(key) {
				text, ok := item.Value().IntoString()
				if !ok {
					continue
				}
				if parsed, err := strconv.ParseUint(strings.TrimSpace(text), 10, 32); err == nil {
					n = meta.Some(uint32(parsed))
				}
			}
			return n
		}
		n, tot := parseNum(pair[0]), parseNum(pair[1])
		if n.IsNone() && tot.IsNone() {
			continue
		}
		key, _ := pair[0].MapKey(meta.TagTypeApe)
		apeTag.insertNumberPair(key, n, tot)
	}
	for _, pic := range input.Pictures() {
		if key, ok := pic.PicType().AsApeKey(); ok {
			picCopy := pic
			apeTag.Push(&ApeItem{Key: key, Value: meta.ItemValueBinary(picCopy.AsApeBytes())})
		}
	}
	return apeTag
}

// --- read ---

// ReadApeTagWithHeader reads an APE tag after the preamble+header was consumed.
func ReadApeTagWithHeader(reader io.ReadSeeker, header *ApeHeader, parseOptions meta.ParseOptions) (*ApeTag, error) {
	tag := NewApeTag()
	remainingSize := header.Size

	parseItemWithKey := func(key string, flags, valueSize uint32) (*ApeItem, error) {
		readOnly := flags&1 == 1
		itemType := (flags >> 1) & 3
		value, err := util.TryByteSlice(int(valueSize))
		if err != nil {
			return nil, &ApeTagItemParseError{Key: key, Source: err}
		}
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, &ApeTagItemParseError{Key: key, Source: err}
		}
		var parsed meta.ItemValue
		switch itemType {
		case 0:
			v, err := util.Utf8Decode(value)
			if err != nil {
				return nil, &ApeTagItemParseError{Key: key, Source: err}
			}
			parsed = meta.ItemValueText(v)
		case 1:
			parsed = meta.ItemValueBinary(value)
		case 2:
			v, err := util.Utf8Decode(value)
			if err != nil {
				return nil, &ApeTagItemParseError{Key: key, Source: err}
			}
			parsed = meta.ItemValueLocator(v)
		default:
			return nil, &ApeTagItemParseError{Key: key, Message: "illegal item type"}
		}
		item, err := NewApeItem(key, parsed)
		if err != nil {
			return nil, err
		}
		item.ReadOnly = readOnly
		return item, nil
	}

	for i := uint32(0); i < header.ItemCount; i++ {
		if remainingSize < 11 {
			break
		}
		var valueSizeBuf [4]byte
		if _, err := io.ReadFull(reader, valueSizeBuf[:]); err != nil {
			return nil, &ApeTagParseError{Source: err}
		}
		valueSize := binary.LittleEndian.Uint32(valueSizeBuf[:])
		if valueSize > remainingSize {
			return nil, &ApeTagParseError{Source: &meta.SizeMismatchError{}}
		}
		remainingSize -= 4
		var flagsBuf [4]byte
		if _, err := io.ReadFull(reader, flagsBuf[:]); err != nil {
			return nil, &ApeTagParseError{Source: err}
		}
		flags := binary.LittleEndian.Uint32(flagsBuf[:])
		remainingSize -= 4

		var key []byte
		var kbuf [64]byte
		for {
			n, err := io.ReadFull(reader, kbuf[:])
			if err != nil && err != io.ErrUnexpectedEOF {
				return nil, &ApeTagParseError{Source: err}
			}
			if z := bytes.IndexByte(kbuf[:n], 0); z >= 0 {
				key = append(key, kbuf[:z]...)
				// Push back bytes read past the terminator.
				if extra := n - (z + 1); extra > 0 {
					if _, serr := reader.Seek(-int64(extra), io.SeekCurrent); serr != nil {
						return nil, &ApeTagParseError{Source: serr}
					}
				}
				remainingSize -= uint32(z + 1)
				break
			}
			key = append(key, kbuf[:n]...)
			remainingSize -= uint32(n)
			if err == io.ErrUnexpectedEOF || n == 0 {
				return nil, &ApeTagParseError{Source: io.ErrUnexpectedEOF}
			}
		}
		keyStr, err := util.Utf8Decode(key)
		if err != nil {
			return nil, &ApeTagItemParseError{Source: err}
		}

		if util.IndexOfStr(meta.ApePictureTypes[:], keyStr) >= 0 && !parseOptions.ReadCoverArt {
			if _, err := reader.Seek(int64(valueSize), io.SeekCurrent); err != nil {
				return nil, &ApeTagParseError{Source: err}
			}
			remainingSize -= valueSize
			continue
		}
		item, err := parseItemWithKey(keyStr, flags, valueSize)
		if err != nil {
			return nil, err
		}
		remainingSize -= valueSize
		tag.insert(item)
	}
	if _, err := reader.Seek(32, io.SeekCurrent); err != nil {
		return nil, &ApeTagParseError{Source: err}
	}
	return tag, nil
}

// ReadApeTag reads an APE tag from the current position.
// Returns (tag, header, found).
func ReadApeTag(reader io.ReadSeeker, footer bool, parseOptions meta.ParseOptions) (*ApeTag, *ApeHeader, error) {
	var preamble [8]byte
	if _, err := io.ReadFull(reader, preamble[:]); err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(preamble[:], apePreamble) {
		return nil, nil, nil
	}
	header, err := ReadApeHeader(reader, footer)
	if err != nil {
		return nil, nil, err
	}
	var tag *ApeTag
	if parseOptions.ReadTags {
		tag, err = ReadApeTagWithHeader(reader, header, parseOptions)
		if err != nil {
			return nil, nil, err
		}
	}
	return tag, header, nil
}

// --- write ---

func ApeWriteTo(file meta.FileLike, tag *ApeTag, writeOptions meta.WriteOptions) error {
	type apeTags struct {
		readOnly      *ApeTag
		header        [2]int64
		headerFound   bool
		footer        [2]int64
		footerFound   bool
		writePosition int64
	}

	findExistingApeTags := func() (*apeTags, error) {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		// Skip ID3v2
		if _, _, found, err := id3v2.FindId3v2(file, false, writeOptions.ParseOptions); err != nil {
			return nil, err
		} else if !found {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
		}
		tags := &apeTags{}
		var preamble [8]byte
		if _, err := io.ReadFull(file, preamble[:]); err != nil {
			return nil, err
		}
		_ = preamble
		start, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		start -= 8
		if tag, header, err := ReadApeTag(file, false, writeOptions.ParseOptions); err == nil && header != nil {
			if writeOptions.RespectReadOnly && tag != nil {
				var ro []*ApeItem
				for _, i := range tag.items {
					if i.ReadOnly {
						ro = append(ro, i)
					}
				}
				if len(ro) > 0 {
					tags.readOnly = &ApeTag{items: ro}
				}
			}
			tags.header = [2]int64{start, start + int64(header.Size)}
			tags.headerFound = true
		} else {
			if _, err := file.Seek(start+8, io.SeekStart); err != nil {
				return nil, err
			}
		}

		// Skip ID3v1/Lyrics3. When ID3v1 is found the reader sits at its
		// start, which is where the APE tag ends; otherwise the APE tag
		// (if any) ends at EOF. Only seek to EOF when nothing was found.
		id3v1Found := false
		if _, _, _, found, err := id3v1.FindId3v1(file, false, writeOptions.ParseOptions.ParsingMode); err != nil {
			return nil, err
		} else if found {
			id3v1Found = true
		} else {
			if _, err := file.Seek(0, io.SeekEnd); err != nil {
				return nil, err
			}
		}
		if _, _, _, found, err := lyrics3.FindLyrics3(file, false, writeOptions.ParseOptions); err != nil {
			return nil, err
		} else if !found && !id3v1Found {
			if _, err := file.Seek(0, io.SeekEnd); err != nil {
				return nil, err
			}
		}

		apePos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		tags.writePosition = apePos

		if _, err := file.Seek(-32, io.SeekCurrent); err != nil {
			return tags, nil
		}
		start, err = file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		if tag, header, err := ReadApeTag(file, true, writeOptions.ParseOptions); err == nil && header != nil {
			if writeOptions.RespectReadOnly && tag != nil {
				var ro []*ApeItem
				for _, i := range tag.items {
					if i.ReadOnly {
						ro = append(ro, i)
					}
				}
				if len(ro) > 0 {
					tags.readOnly = &ApeTag{items: ro}
				}
			}
			// The tag region ends at start+32 and spans header.Size total
			// (header.Size includes a 32-byte header when CONTAINS_HEADER).
			tags.footer = [2]int64{start + 32 - int64(header.Size), start + 32}
			tags.footerFound = true
		} else {
			if _, err := file.Seek(apePos, io.SeekStart); err != nil {
				return nil, err
			}
		}
		return tags, nil
	}

	existing, err := findExistingApeTags()
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagParseError{TagType: meta.TagTypeApe, HasTagType: true, Source: err}}
	}

	// Build the item list: new tag items + preserved read-only ones
	var items []*ApeItem
	if existing.readOnly != nil {
		items = append(items, existing.readOnly.items...)
	}
	for _, item := range tag.items {
		if item.ReadOnly {
			continue // already preserved
		}
		items = append(items, item)
	}

	// Encode
	var content bytes.Buffer
	var keys []string
	itemMap := map[string]*ApeItem{}
	for _, i := range items {
		itemMap[i.Key] = i
		keys = append(keys, i.Key)
	}
	sort.Strings(keys)
	size := uint64(0)
	for _, key := range keys {
		item := itemMap[key]
		var valueLen uint32
		var itemFlags uint32
		switch item.Value.Kind() {
		case meta.ItemValueKindText:
			s, _ := item.Value.IntoString()
			// multi-value text uses null separators
			parts := util.SplitNulls(s)
			enc := []byte(util.JoinNulls(parts))
			valueLen = uint32(len(enc))
		case meta.ItemValueKindLocator:
			s, _ := item.Value.IntoString()
			valueLen = uint32(len(s))
			itemFlags = 2 << 1
		case meta.ItemValueKindBinary:
			b, _ := item.Value.IntoBinary()
			valueLen = uint32(len(b))
			itemFlags = 1 << 1
		}
		if item.ReadOnly {
			itemFlags |= 1
		}
		_ = binary.Write(&content, binary.LittleEndian, valueLen)
		_ = binary.Write(&content, binary.LittleEndian, itemFlags)
		content.WriteString(item.Key)
		content.WriteByte(0)
		switch item.Value.Kind() {
		case meta.ItemValueKindBinary:
			b, _ := item.Value.IntoBinary()
			content.Write(b)
		default:
			s, _ := item.Value.IntoString()
			content.WriteString(s)
		}
		size += 8 + uint64(len(item.Key)) + 1 + uint64(valueLen)
	}

	// Remove existing tags. An APE tag at the start of the file is removed
	// with a splice, not a truncate (audio data may follow it).
	insertPos := existing.writePosition
	if existing.headerFound {
		if err := meta.Splice(file, uint64(existing.header[0]), uint64(existing.header[1]), nil); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}
	if existing.footerFound {
		insertPos = existing.footer[0]
		if err := meta.Splice(file, uint64(existing.footer[0]), uint64(existing.footer[1]), nil); err != nil {
			return &meta.FileEncodingError{Source: err}
		}
	}

	if len(items) == 0 {
		return nil
	}

	// Encode the full tag (header + content + footer) and insert it at the
	// previous tag's position (or before any trailing ID3v1/Lyrics3 tags).
	totalSize := uint32(size + 32) // footer included
	var encoded bytes.Buffer
	var hdr [32]byte
	copy(hdr[:8], apePreamble)
	binary.LittleEndian.PutUint32(hdr[8:], 2000)
	binary.LittleEndian.PutUint32(hdr[12:], totalSize)
	binary.LittleEndian.PutUint32(hdr[16:], uint32(len(items)))
	binary.LittleEndian.PutUint32(hdr[20:], apeHeaderIsHeader|apeHeaderContainsFooter|apeHeaderContainsHeader)
	encoded.Write(hdr[:])
	encoded.Write(content.Bytes())
	var ftr [32]byte
	copy(ftr[:8], apePreamble)
	binary.LittleEndian.PutUint32(ftr[8:], 2000)
	binary.LittleEndian.PutUint32(ftr[12:], totalSize)
	binary.LittleEndian.PutUint32(ftr[16:], uint32(len(items)))
	binary.LittleEndian.PutUint32(ftr[20:], apeHeaderContainsFooter|apeHeaderContainsHeader)
	encoded.Write(ftr[:])

	if err := meta.Splice(file, uint64(insertPos), uint64(insertPos), encoded.Bytes()); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return nil
}

func init() {
	meta.RegisterTagDumper(meta.TagTypeApe, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		at := ApeTagFromTag(tag)
		if len(at.items) == 0 {
			return nil
		}
		// Reuse the encoder: write into a buffer via ApeWriteTo on a memory meta.FileLike
		var buf bytes.Buffer
		var hdr [32]byte
		copy(hdr[:8], apePreamble)
		binary.LittleEndian.PutUint32(hdr[8:], 2000)
		// compute content
		var content bytes.Buffer
		var keys []string
		itemMap := map[string]*ApeItem{}
		for _, i := range at.items {
			itemMap[i.Key] = i
			keys = append(keys, i.Key)
		}
		sort.Strings(keys)
		size := uint64(0)
		for _, key := range keys {
			item := itemMap[key]
			var valueLen uint32
			var itemFlags uint32
			switch item.Value.Kind() {
			case meta.ItemValueKindLocator:
				s, _ := item.Value.IntoString()
				valueLen = uint32(len(s))
				itemFlags = 2 << 1
			case meta.ItemValueKindBinary:
				b, _ := item.Value.IntoBinary()
				valueLen = uint32(len(b))
				itemFlags = 1 << 1
			default:
				s, _ := item.Value.IntoString()
				valueLen = uint32(len(s))
			}
			if item.ReadOnly {
				itemFlags |= 1
			}
			_ = binary.Write(&content, binary.LittleEndian, valueLen)
			_ = binary.Write(&content, binary.LittleEndian, itemFlags)
			content.WriteString(item.Key)
			content.WriteByte(0)
			switch item.Value.Kind() {
			case meta.ItemValueKindBinary:
				b, _ := item.Value.IntoBinary()
				content.Write(b)
			default:
				s, _ := item.Value.IntoString()
				content.WriteString(s)
			}
			size += 8 + uint64(len(item.Key)) + 1 + uint64(valueLen)
		}
		totalSize := uint32(size + 32)
		binary.LittleEndian.PutUint32(hdr[12:], totalSize)
		binary.LittleEndian.PutUint32(hdr[16:], uint32(len(at.items)))
		binary.LittleEndian.PutUint32(hdr[20:], apeHeaderIsHeader|apeHeaderContainsFooter|apeHeaderContainsHeader)
		buf.Write(hdr[:])
		buf.Write(content.Bytes())
		var ftr [32]byte
		copy(ftr[:8], apePreamble)
		binary.LittleEndian.PutUint32(ftr[8:], 2000)
		binary.LittleEndian.PutUint32(ftr[12:], totalSize)
		binary.LittleEndian.PutUint32(ftr[16:], uint32(len(at.items)))
		binary.LittleEndian.PutUint32(ftr[20:], apeHeaderContainsFooter|apeHeaderContainsHeader)
		buf.Write(ftr[:])
		_, err := writer.Write(buf.Bytes())
		return err
	})
}

// --- APE file ---

// ApeProperties are the audio properties of an APE (Monkey's Audio) file.
type ApeProperties struct {
	Version        uint16
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
}

// FileProperties converts to generic [meta.FileProperties].
func (p ApeProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(p.BitDepth),
		meta.Some(p.Channels),
		meta.None[meta.ChannelMask](),
	)
}

// ApeFile is an APE (Monkey's Audio) file.
type ApeFile struct {
	Properties ApeProperties
	ID3v1      meta.Option[*id3v1.Id3v1Tag]
	ID3v2      meta.Option[*id3v2.Id3v2Tag]
	Ape        meta.Option[*ApeTag]
}

// ApeFileReadFrom reads an [ApeFile] from a reader.
func ApeFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*ApeFile, error) {
	data := reader
	start, err := data.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if _, err := data.Seek(0, io.SeekEnd); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	end, err := data.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	streamLen := uint64(end - start)
	fileLength := streamLen

	var id3v2Tag *id3v2.Id3v2Tag
	var id3v1Tag *id3v1.Id3v1Tag
	var apev1, apev2 *ApeTag

	if _, err := data.Seek(start, io.SeekStart); err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	header, _, found, err := id3v2.FindId3v2(data, parseOptions.ReadTags, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if found {
		if newLen := streamLen - uint64(header.FullTagSize()); newLen > 0 {
			streamLen = newLen
		}
		if parseOptions.ReadTags {
			// re-parse the tag
			if _, err := data.Seek(start+int64(header.FullTagSize()-header.Size), io.SeekStart); err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			t, err := id3v2.ParseId3v2Tag(data, header, parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: err}
			}
			id3v2Tag = t
		}
	}

	// Look for an APE tag at the start
	tag, apeHeader, err := ReadApeTag(data, false, parseOptions)
	if err == nil && apeHeader != nil {
		if parseOptions.ReadTags {
			if apeHeader.Version == 2 {
				apev2 = tag
			} else {
				apev1 = tag
			}
		}
		if newLen := streamLen - uint64(apeHeader.Size); newLen > 0 {
			streamLen = newLen
		}
	} else if err != nil {
		// The failed APE preamble read consumed bytes. Rewind to start and
		// re-skip a possible ID3v2 tag: id3v2.FindId3v2 leaves the reader after a
		// found tag, or back at start when absent.
		if _, err := data.Seek(start, io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		if _, _, _, err := id3v2.FindId3v2(data, false, parseOptions); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
	}

	// Find ID3v1 at the end
	id3v1, _, _, foundI, err := id3v1.FindId3v1(data, parseOptions.ReadTags, parseOptions.ParsingMode)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if foundI {
		id3v1Tag = id3v1
		streamLen -= 128
	}

	// Find Lyrics3
	if _, _, _, foundL, err := lyrics3.FindLyrics3(data, false, parseOptions); err == nil && foundL {
		// Lyrics3 consumed: the reader sits at its start
		pos, _ := data.Seek(0, io.SeekCurrent)
		if pos-start < int64(streamLen) {
			streamLen = uint64(pos - start)
		}
	}

	// Find APE tag at the end. The reader is positioned at the start of any
	// trailing ID3v1/Lyrics3 tags, which is where the APE tag ends.
	endPos, err := data.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if _, err := data.Seek(endPos-32, io.SeekStart); err == nil {
		if tag, apeHeader, err := ReadApeTag(data, true, parseOptions); err == nil && apeHeader != nil {
			if parseOptions.ReadTags {
				if apeHeader.Version >= 2000 {
					apev2 = tag
				} else {
					apev1 = tag
				}
			}
			if newLen := streamLen - uint64(apeHeader.Size); newLen > 0 {
				streamLen = newLen
			}
		}
	}
	// Rust quirk: file_length is the stream position after the trailing tag
	// read attempt, not the actual file length.
	if fileLenPos, err := data.Seek(0, io.SeekCurrent); err == nil && fileLenPos > 0 {
		fileLength = uint64(fileLenPos)
	}

	// Read properties
	var properties *ApeProperties
	if parseOptions.ReadProperties {
		if _, err := data.Seek(start, io.SeekStart); err != nil {
			return nil, &meta.FileParseError{Source: err}
		}
		macStart, ferr := apeFindMacHeader(data)
		if ferr != nil {
			return nil, &meta.FileParseError{Source: ferr}
		}
		if _, ferr := data.Seek(macStart, io.SeekStart); ferr == nil {
			properties, ferr = apeReadProperties(data, streamLen, fileLength, parseOptions.ParsingMode)
			if ferr != nil && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, &meta.FileParseError{Source: ferr}
			}
		}
	}
	if properties == nil {
		properties = &ApeProperties{}
	}

	return &ApeFile{
		Properties: *properties,
		ID3v1:      util.OptionFromPtr(id3v1Tag),
		ID3v2:      util.OptionFromPtr(id3v2Tag),
		Ape:        firstApe(apev2, apev1),
	}, nil
}

// firstApe picks the primary APE tag (v2 preferred), mirroring Rust's single
// `ape_tag` slot.
func firstApe(v2, v1 *ApeTag) meta.Option[*ApeTag] {
	if v2 != nil {
		return meta.Some(v2)
	}
	return util.OptionFromPtr(v1)
}

// FileType implements meta.AudioFile.
func (f *ApeFile) FileType() meta.FileType { return meta.FileTypeApe }

// Properties implements meta.AudioFile.
func (f *ApeFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *ApeFile) ContainsTag() bool {
	return f.ID3v1.IsSome() || f.ID3v2.IsSome() || f.Ape.IsSome()
}

// ContainsTagType implements meta.AudioFile.
func (f *ApeFile) ContainsTagType(tagType meta.TagType) bool {
	switch tagType {
	case meta.TagTypeID3v1:
		return f.ID3v1.IsSome()
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	case meta.TagTypeApe:
		return f.Ape.IsSome()
	}
	return false
}

// Tags implements TaggedFileAccessor-style conversion via TaggedFileFromFile.
// TaggedFile converts to a [meta.TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v1, ID3v2, Ape (both APE variants share one slot).
func (f *ApeFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.NewTaggedFile(meta.FileTypeApe, tags, *f.GetProperties())
}

// SavePath saves the tags back to the file.
func (f *ApeFile) SavePath(path string, writeOptions meta.WriteOptions) error {
	if f.Ape.IsSome() {
		return meta.SaveToPath(path, f, meta.TagTypeApe, writeOptions)
	}
	return meta.SaveToPath(path, f, f.primaryTagType(), writeOptions)
}

func (f *ApeFile) primaryTagType() meta.TagType {
	if f.ID3v2.IsSome() {
		return meta.TagTypeID3v2
	}
	if f.Ape.IsSome() {
		return meta.TagTypeApe
	}
	return meta.TagTypeID3v1
}

// SaveFile writes the tags back to an open file.
func (f *ApeFile) SaveFile(file meta.FileLike, writeOptions meta.WriteOptions) error {
	var tags []*meta.Tag
	if t, ok := f.ID3v1.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.Ape.Get(); ok {
		tags = append(tags, t.Tag())
	}
	return meta.SaveTagFields(file, meta.FileTypeApe, tags, writeOptions)
}

// RemoveFromPath removes a tag type from the file at `path`.
func (f *ApeFile) RemoveFromPath(path string, tagType meta.TagType) error {
	return meta.RemoveTagFromPath(path, tagType)
}

// apeFindMacHeader scans for the "MAC " magic with a strict 4-byte window,
// skipping any ID3v2 or APE tags encountered along the way. Returns the
// position just after the magic.
func apeFindMacHeader(data io.ReadSeeker) (int64, error) {
	for {
		var window [4]byte
		if _, err := io.ReadFull(data, window[:]); err != nil {
			return -1, &ApeParseError{Message: `Invalid data found while reading header, expected any of ["MAC ", "APETAGEX", "ID3"]`}
		}
		switch {
		case bytes.Equal(window[:], []byte("MAC ")):
			return data.Seek(0, io.SeekCurrent)
		case bytes.Equal(window[:3], []byte("ID3")):
			// Skip the ID3v2 tag: 10-byte header, size is synchsafe at [6:10]
			var rest [6]byte
			if _, err := io.ReadFull(data, rest[:]); err != nil {
				return -1, &ApeParseError{Message: "unable to read ID3v2 header"}
			}
			size := int64(rest[2]&0x7F)<<21 | int64(rest[3]&0x7F)<<14 | int64(rest[4]&0x7F)<<7 | int64(rest[5]&0x7F)
			if rest[1]&0x10 != 0 { // v2.4 footer flag
				size += 10
			}
			if _, err := data.Seek(size, io.SeekCurrent); err != nil {
				return -1, err
			}
		case bytes.Equal(window[:], []byte("APET")):
			// Possible APE tag: verify the full preamble
			var rest [4]byte
			if _, err := io.ReadFull(data, rest[:]); err != nil {
				return -1, &ApeParseError{Message: "unable to read APE preamble"}
			}
			if !bytes.Equal(rest[:], []byte("AGEX")) {
				return -1, &ApeParseError{Message: "expected ape preamble"}
			}
			apeHeader, err := ReadApeHeader(data, false)
			if err != nil {
				return -1, err
			}
			if _, err := data.Seek(int64(apeHeader.Size)-32, io.SeekCurrent); err != nil {
				return -1, err
			}
		default:
			return -1, &ApeParseError{Message: `Invalid data found while reading header, expected any of ["MAC ", "APETAGEX", "ID3"]`}
		}
	}
}

// apeReadProperties reads the APE audio properties.
func apeReadProperties(data io.ReadSeeker, streamLen, fileLength uint64, parseMode meta.ParsingMode) (*ApeProperties, error) {
	var versionBuf [2]byte
	if _, err := io.ReadFull(data, versionBuf[:]); err != nil {
		return nil, &ApeParseError{Message: "unable to read APE tag version"}
	}
	version := binary.LittleEndian.Uint16(versionBuf[:])

	p := &ApeProperties{Version: version}
	var blocksPerFrame, finalFrameBlocks, totalFrames uint32

	if version >= 3980 {
		var descriptor [46]byte
		if _, err := io.ReadFull(data, descriptor[:]); err != nil {
			return nil, &ApeParseError{Message: "not enough data left in reader to finish file descriptor"}
		}
		descriptorLen := binary.LittleEndian.Uint32(descriptor[2:6])
		if descriptorLen > 52 {
			if _, err := data.Seek(int64(descriptorLen-52), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
		var header [24]byte
		if _, err := io.ReadFull(data, header[:]); err != nil {
			return nil, &ApeParseError{Message: "not enough data left in reader to finish MAC header"}
		}
		blocksPerFrame = binary.LittleEndian.Uint32(header[4:])
		finalFrameBlocks = binary.LittleEndian.Uint32(header[8:])
		totalFrames = binary.LittleEndian.Uint32(header[12:])
		p.BitDepth = uint8(binary.LittleEndian.Uint16(header[16:]))
		p.Channels = uint8(binary.LittleEndian.Uint16(header[18:]))
		p.SampleRate = binary.LittleEndian.Uint32(header[20:])
	} else {
		var header [26]byte
		if _, err := io.ReadFull(data, header[:]); err != nil {
			return nil, &ApeParseError{Message: "not enough data left in reader to finish MAC header"}
		}
		compressionLevel := binary.LittleEndian.Uint16(header[0:])
		formatFlags := binary.LittleEndian.Uint16(header[2:])
		switch {
		case formatFlags&0b1 == 1:
			p.BitDepth = 8
		case formatFlags&0b1000 == 8:
			p.BitDepth = 24
		default:
			p.BitDepth = 16
		}
		switch {
		case version >= 3950:
			blocksPerFrame = 73728 * 4
		case version >= 3900 || (version >= 3800 && compressionLevel >= 4000):
			blocksPerFrame = 73728
		default:
			blocksPerFrame = 9216
		}
		p.Channels = uint8(binary.LittleEndian.Uint16(header[4:]))
		p.SampleRate = binary.LittleEndian.Uint32(header[6:])
		totalFrames = binary.LittleEndian.Uint32(header[18:])
		finalFrameBlocks = binary.LittleEndian.Uint32(header[22:])
	}

	if p.Channels < 1 || p.Channels > 32 || totalFrames == 0 {
		if parseMode == meta.ParsingModeStrict {
			return nil, &ApeParseError{Message: "file contains no frames or has an invalid channel count"}
		}
		return p, nil
	}

	totalSamples := uint64(finalFrameBlocks)
	if totalSamples > 1 {
		totalSamples += uint64(blocksPerFrame) * uint64(totalFrames-1)
	}
	if p.SampleRate > 0 {
		length := float64(totalSamples) * 1000.0 / float64(p.SampleRate)
		p.Duration = time.Duration(length+0.5) * time.Millisecond
		p.AudioBitrate = uint32(float64(streamLen)*8.0/length + 0.5)
		p.OverallBitrate = uint32(float64(fileLength)*8.0/length + 0.5)
	}
	return p, nil
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeApe, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := ApeFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterFormatWriter(meta.FileTypeApe, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		f := file.File()
		switch tag.TagType() {
		case meta.TagTypeApe:
			return ApeWriteTo(f, ApeTagFromTag(tag), writeOptions)
		case meta.TagTypeID3v2:
			return id3v2.WriteId3v2(f, tag, writeOptions)
		case meta.TagTypeID3v1:
			return id3v1.WriteId3v1(f, id3v1.Id3v1TagFromTag(tag), writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
}

// ReadEndTags locates trailing ID3v1/Lyrics3/APEv2 tags.
func ReadEndTags(reader io.ReadSeeker, start int64, streamLen uint64, parseOptions meta.ParseOptions) (uint64, *id3v1.Id3v1Tag, *ApeTag, error) {
	id3v1, _, _, foundI, err := id3v1.FindId3v1(reader, parseOptions.ReadTags, parseOptions.ParsingMode)
	if err != nil {
		return 0, nil, nil, err
	}
	if foundI {
		streamLen -= 128
	}
	if _, _, _, foundL, err := lyrics3.FindLyrics3(reader, false, parseOptions); err == nil && foundL {
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
		tag, apeHeader, err := ReadApeTag(reader, true, parseOptions)
		if err != nil {
			return 0, nil, nil, &meta.TagParseError{Source: err}
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

// DumpTo writes the tag to a writer, mirroring Rust `ApeTag::dump_to` — this
// is only the tag data, not a usable file.
func (t *ApeTag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	_, err := writer.Write(encodeApeTag(t))
	return err
}

// RemoveFrom removes an APE tag from a [meta.FileLike].
func (t *ApeTag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeApe.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes an APE tag from a path.
func (t *ApeTag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeApe.RemoveFromPath(path, writeOptions)
}

// encodeApeTag serializes the tag as APEv2 (header + items + footer),
// mirroring Rust `ApeTag::dump_to`.
func encodeApeTag(t *ApeTag) []byte {
	var content bytes.Buffer
	keys := make([]string, 0, len(t.items))
	itemMap := map[string]*ApeItem{}
	for _, i := range t.items {
		itemMap[i.Key] = i
		keys = append(keys, i.Key)
	}
	sort.Strings(keys)
	count := uint32(0)
	for _, key := range keys {
		item := itemMap[key]
		var valueLen uint32
		var itemFlags uint32
		var val []byte
		switch item.Value.Kind() {
		case meta.ItemValueKindLocator:
			s, _ := item.Value.IntoString()
			val = []byte(s)
			itemFlags = 2 << 1
		case meta.ItemValueKindBinary:
			b, _ := item.Value.IntoBinary()
			val = b
			itemFlags = 1 << 1
		default:
			s, _ := item.Value.IntoString()
			val = []byte(s)
		}
		valueLen = uint32(len(val))
		if item.ReadOnly {
			itemFlags |= 1
		}
		_ = binary.Write(&content, binary.LittleEndian, valueLen)
		_ = binary.Write(&content, binary.LittleEndian, itemFlags)
		content.WriteString(item.Key)
		content.WriteByte(0)
		content.Write(val)
		count++
	}
	if count == 0 {
		return nil
	}
	totalSize := uint32(content.Len() + 32)
	var out bytes.Buffer
	var hdr [32]byte
	copy(hdr[:8], apePreamble)
	binary.LittleEndian.PutUint32(hdr[8:], 2000)
	binary.LittleEndian.PutUint32(hdr[12:], totalSize)
	binary.LittleEndian.PutUint32(hdr[16:], count)
	binary.LittleEndian.PutUint32(hdr[20:], apeHeaderIsHeader|apeHeaderContainsFooter|apeHeaderContainsHeader)
	out.Write(hdr[:])
	out.Write(content.Bytes())
	var ftr [32]byte
	copy(ftr[:8], apePreamble)
	binary.LittleEndian.PutUint32(ftr[8:], 2000)
	binary.LittleEndian.PutUint32(ftr[12:], totalSize)
	binary.LittleEndian.PutUint32(ftr[16:], count)
	binary.LittleEndian.PutUint32(ftr[20:], apeHeaderContainsFooter|apeHeaderContainsHeader)
	out.Write(ftr[:])
	return out.Bytes()
}
