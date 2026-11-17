package vorbis

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// vcEntry is a single stored comment: the mapped item plus the field name
// exactly as it was read (Rust stores `(String, String)` pairs verbatim).
type vcEntry struct {
	item meta.TagItem
	key  string
}

// VorbisCommentsTag is a Vorbis comment header (used by FLAC, Vorbis, Opus, Speex).
type VorbisCommentsTag struct {
	Vendor   string
	entries  []vcEntry
	pictures []meta.PictureInfoPair
}

// NewVorbisCommentsTag creates an empty tag with the default vendor.
func NewVorbisCommentsTag() *VorbisCommentsTag {
	return &VorbisCommentsTag{}
}

// asciiEqualFold is Rust's `eq_ignore_ascii_case` (no Unicode folding).
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// asciiUpper is Rust's `to_ascii_uppercase`.
func asciiUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// entryForItem builds an entry for a generic item. Mapped items are stored
// under their canonical field name, unknown items under their description,
// which must be a valid field name.
func entryForItem(item meta.TagItem) (vcEntry, bool) {
	if item.Key() == meta.ItemKeyUnknown {
		if !verifyVorbisKey(item.Description) {
			return vcEntry{}, false
		}
		return vcEntry{item: item, key: item.Description}, true
	}
	k, ok := item.Key().MapKey(meta.TagTypeVorbisComments)
	if !ok {
		return vcEntry{}, false
	}
	return vcEntry{item: item, key: k}, true
}

// Items returns the tag's items.
func (t *VorbisCommentsTag) Items() []meta.TagItem {
	if len(t.entries) == 0 {
		return nil
	}
	out := make([]meta.TagItem, len(t.entries))
	for i, e := range t.entries {
		out[i] = e.item
	}
	return out
}

// Pairs returns the stored comments as raw (field name, value) pairs in file
// order, with the field names spelled as they were read — Rust's `items()`.
func (t *VorbisCommentsTag) Pairs() [][2]string {
	out := make([][2]string, 0, len(t.entries))
	for _, e := range t.entries {
		v, _ := entryValue(e)
		out = append(out, [2]string{e.key, v})
	}
	return out
}

// entryValue returns the textual value of an entry (binary unknown items are
// written verbatim).
func entryValue(e vcEntry) (string, bool) {
	if s, ok := e.item.Value().IntoString(); ok {
		return s, true
	}
	if e.item.Key() == meta.ItemKeyUnknown {
		if b, ok := e.item.Value().Binary(); ok {
			return string(b), true
		}
	}
	return "", false
}

// Pictures returns the tag's pictures with their stored information.
func (t *VorbisCommentsTag) Pictures() []meta.PictureInfoPair { return t.pictures }

// SetPictures replaces the tag's stored pictures.
func (t *VorbisCommentsTag) SetPictures(pictures []meta.PictureInfoPair) { t.pictures = pictures }

// GetString gets the first string value for a key.
func (t *VorbisCommentsTag) GetString(key meta.ItemKey) (string, bool) {
	for _, e := range t.entries {
		if e.item.Key() == key {
			if s, ok := e.item.Value().IntoString(); ok {
				return s, true
			}
		}
	}
	return "", false
}

// GetStrings gets all string values for a key.
func (t *VorbisCommentsTag) GetStrings(key meta.ItemKey) []string {
	var out []string
	for _, e := range t.entries {
		if e.item.Key() == key {
			if s, ok := e.item.Value().IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// Insert replaces all items with the same key. Items that cannot be stored in
// Vorbis comments (no field mapping, or an invalid unknown field name) are
// ignored.
func (t *VorbisCommentsTag) Insert(item meta.TagItem) {
	e, ok := entryForItem(item)
	if !ok {
		return
	}
	t.Remove(item.Key())
	t.entries = append(t.entries, e)
}

// Push appends an item. Items that cannot be stored in Vorbis comments (no
// field mapping, or an invalid unknown field name) are ignored.
func (t *VorbisCommentsTag) Push(item meta.TagItem) {
	if e, ok := entryForItem(item); ok {
		t.entries = append(t.entries, e)
	}
}

// pushMapped appends an item only when its key maps to a Vorbis comment field
// name — mirroring Rust's `map_key(...) -> None => continue` in `merge_tag`
// and `create_vorbis_comments_ref`. Unlike [Push], unknown items are dropped
// rather than stored under their description.
func (t *VorbisCommentsTag) pushMapped(item meta.TagItem) {
	k, ok := item.Key().MapKey(meta.TagTypeVorbisComments)
	if !ok {
		return
	}
	t.entries = append(t.entries, vcEntry{item: item, key: k})
}

// Remove deletes all items with the key.
func (t *VorbisCommentsTag) Remove(key meta.ItemKey) {
	var kept []vcEntry
	for _, e := range t.entries {
		if e.item.Key() != key {
			kept = append(kept, e)
		}
	}
	t.entries = kept
}

// RemoveKey deletes all items with the field name (an exact, case-sensitive
// match like Rust's `VorbisComments::remove`), returning the removed values.
func (t *VorbisCommentsTag) RemoveKey(key string) []string {
	var kept []vcEntry
	var removed []string
	for _, e := range t.entries {
		if e.key == key {
			if s, ok := e.item.Value().IntoString(); ok {
				removed = append(removed, s)
			}
			continue
		}
		kept = append(kept, e)
	}
	t.entries = kept
	return removed
}

// verifyVorbisKey validates a field name: ASCII 0x20 through 0x7D, '='
// excluded, mirroring Rust's `verify_key`.
func verifyVorbisKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] > 0x7D || key[i] == '=' {
			return false
		}
	}
	return true
}

// Get returns the first item with the key (ASCII case-insensitive), mirroring
// Rust's `VorbisComments::get`.
func (t *VorbisCommentsTag) Get(key string) (string, bool) {
	if !verifyVorbisKey(key) {
		return "", false
	}
	for _, e := range t.entries {
		if asciiEqualFold(e.key, key) {
			if s, ok := e.item.Value().IntoString(); ok {
				return s, true
			}
		}
	}
	return "", false
}

// GetAll returns every item with the key (ASCII case-insensitive).
func (t *VorbisCommentsTag) GetAll(key string) []string {
	var out []string
	for _, e := range t.entries {
		if asciiEqualFold(e.key, key) {
			if s, ok := e.item.Value().IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// newVorbisEntry builds an entry for a raw field name.
func newVorbisEntry(key, value string) vcEntry {
	itemKey := meta.ItemKeyOrUnknown(meta.TagTypeVorbisComments, key)
	item := meta.NewTagItem(itemKey, meta.ItemValueText(value))
	if itemKey == meta.ItemKeyUnknown {
		item.Description = key
	}
	return vcEntry{item: item, key: key}
}

// InsertKV removes any items with the key (ASCII case-insensitive), then
// appends the new item, mirroring Rust's `VorbisComments::insert`. Invalid
// keys are ignored.
func (t *VorbisCommentsTag) InsertKV(key, value string) {
	if !verifyVorbisKey(key) {
		return
	}
	var kept []vcEntry
	for _, e := range t.entries {
		if !asciiEqualFold(e.key, key) {
			kept = append(kept, e)
		}
	}
	t.entries = append(kept, newVorbisEntry(key, value))
}

// PushKV appends an item, mirroring Rust's `VorbisComments::push`. Invalid
// keys are ignored.
func (t *VorbisCommentsTag) PushKV(key, value string) {
	if !verifyVorbisKey(key) {
		return
	}
	t.entries = append(t.entries, newVorbisEntry(key, value))
}

// Title returns the track title.
func (t *VorbisCommentsTag) Title() (string, bool) { return t.Get("TITLE") }

// SetTitle sets the track title.
func (t *VorbisCommentsTag) SetTitle(value string) { t.InsertKV("TITLE", value) }

// RemoveTitle removes the track title.
func (t *VorbisCommentsTag) RemoveTitle() { t.RemoveKey("TITLE") }

// Artist returns the artist.
func (t *VorbisCommentsTag) Artist() (string, bool) { return t.Get("ARTIST") }

// SetArtist sets the artist.
func (t *VorbisCommentsTag) SetArtist(value string) { t.InsertKV("ARTIST", value) }

// RemoveArtist removes the artist.
func (t *VorbisCommentsTag) RemoveArtist() { t.RemoveKey("ARTIST") }

// Album returns the album.
func (t *VorbisCommentsTag) Album() (string, bool) { return t.Get("ALBUM") }

// SetAlbum sets the album.
func (t *VorbisCommentsTag) SetAlbum(value string) { t.InsertKV("ALBUM", value) }

// RemoveAlbum removes the album.
func (t *VorbisCommentsTag) RemoveAlbum() { t.RemoveKey("ALBUM") }

// Genre returns the genre.
func (t *VorbisCommentsTag) Genre() (string, bool) { return t.Get("GENRE") }

// SetGenre sets the genre.
func (t *VorbisCommentsTag) SetGenre(value string) { t.InsertKV("GENRE", value) }

// RemoveGenre removes the genre.
func (t *VorbisCommentsTag) RemoveGenre() { t.RemoveKey("GENRE") }

// Comment returns the comment.
func (t *VorbisCommentsTag) Comment() (string, bool) { return t.Get("COMMENT") }

// SetComment sets the comment.
func (t *VorbisCommentsTag) SetComment(value string) { t.InsertKV("COMMENT", value) }

// RemoveComment removes the comment.
func (t *VorbisCommentsTag) RemoveComment() { t.RemoveKey("COMMENT") }

// Track returns the track number, checking TRACKNUMBER then TRACKNUM.
func (t *VorbisCommentsTag) Track() (uint32, bool) {
	if s, ok := t.Get("TRACKNUMBER"); !ok {
		if s, ok = t.Get("TRACKNUM"); !ok {
			return 0, false
		} else {
			return util.U32FromString(s)
		}
	} else {
		return util.U32FromString(s)
	}
}

// SetTrack sets the track number, normalizing to TRACKNUMBER.
func (t *VorbisCommentsTag) SetTrack(value uint32) {
	t.RemoveKey("TRACKNUMBER")
	t.RemoveKey("TRACKNUM")
	t.InsertKV("TRACKNUMBER", util.Itoa(int(value)))
}

// RemoveTrack removes the track number.
func (t *VorbisCommentsTag) RemoveTrack() {
	t.RemoveKey("TRACKNUMBER")
	t.RemoveKey("TRACKNUM")
}

// TrackTotal returns the total tracks, checking TRACKTOTAL then TOTALTRACKS.
func (t *VorbisCommentsTag) TrackTotal() (uint32, bool) {
	if s, ok := t.Get("TRACKTOTAL"); !ok {
		if s, ok = t.Get("TOTALTRACKS"); !ok {
			return 0, false
		} else {
			return util.U32FromString(s)
		}
	} else {
		return util.U32FromString(s)
	}
}

// SetTrackTotal sets the total tracks, normalizing to TRACKTOTAL.
func (t *VorbisCommentsTag) SetTrackTotal(value uint32) {
	t.InsertKV("TRACKTOTAL", util.Itoa(int(value)))
	t.RemoveKey("TOTALTRACKS")
}

// RemoveTrackTotal removes the total tracks.
func (t *VorbisCommentsTag) RemoveTrackTotal() {
	t.RemoveKey("TRACKTOTAL")
	t.RemoveKey("TOTALTRACKS")
}

// Disk returns the disc number.
func (t *VorbisCommentsTag) Disk() (uint32, bool) {
	s, ok := t.Get("DISCNUMBER")
	if !ok {
		return 0, false
	}
	return util.U32FromString(s)
}

// SetDisk sets the disc number.
func (t *VorbisCommentsTag) SetDisk(value uint32) { t.InsertKV("DISCNUMBER", util.Itoa(int(value))) }

// RemoveDisk removes the disc number.
func (t *VorbisCommentsTag) RemoveDisk() { t.RemoveKey("DISCNUMBER") }

// DiskTotal returns the total discs, checking DISCTOTAL then TOTALDISCS.
func (t *VorbisCommentsTag) DiskTotal() (uint32, bool) {
	if s, ok := t.Get("DISCTOTAL"); !ok {
		if s, ok = t.Get("TOTALDISCS"); !ok {
			return 0, false
		} else {
			return util.U32FromString(s)
		}
	} else {
		return util.U32FromString(s)
	}
}

// SetDiskTotal sets the total discs, normalizing to DISCTOTAL.
func (t *VorbisCommentsTag) SetDiskTotal(value uint32) {
	t.InsertKV("DISCTOTAL", util.Itoa(int(value)))
	t.RemoveKey("TOTALDISCS")
}

// RemoveDiskTotal removes the total discs.
func (t *VorbisCommentsTag) RemoveDiskTotal() {
	t.RemoveKey("DISCTOTAL")
	t.RemoveKey("TOTALDISCS")
}

// Date returns the recording date, checking DATE then YEAR.
func (t *VorbisCommentsTag) Date() meta.Option[meta.Timestamp] {
	if s, ok := t.Get("DATE"); ok {
		return util.TryParseTimestamp(s)
	}
	if s, ok := t.Get("YEAR"); ok {
		return util.TryParseTimestamp(s)
	}
	return meta.None[meta.Timestamp]()
}

// SetDate sets the recording date, normalizing to DATE.
func (t *VorbisCommentsTag) SetDate(value meta.Timestamp) {
	t.InsertKV("DATE", value.String())
	t.RemoveKey("YEAR")
}

// RemoveDate removes the recording date.
func (t *VorbisCommentsTag) RemoveDate() {
	t.RemoveKey("DATE")
	t.RemoveKey("YEAR")
}

// PushPicture adds a picture, extracting the information from its data.
func (t *VorbisCommentsTag) PushPicture(p meta.Picture) {
	info, err := meta.PictureInformationFromPicture(&p)
	if err != nil {
		info = meta.PictureInformation{}
	}
	t.pictures = append(t.pictures, meta.PictureInfoPair{Picture: p, Info: info})
}

// InsertPicture adds a picture with explicit information, mirroring Rust's
// `OggPictureStorage::insert_picture`. According to the spec there can only be
// one picture of type Icon and one of type OtherIcon, so an existing picture of
// the same type is removed first.
//
// Rust removes the existing icon before extracting information, so a failed
// extraction still drops it. Here the extraction runs first, so a failure
// leaves the tag unchanged — an intentional deviation.
func (t *VorbisCommentsTag) InsertPicture(p meta.Picture, info meta.Option[meta.PictureInformation]) error {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = meta.PictureInformationFromPicture(&p)
		if err != nil {
			// Rust's `insert_picture` propagates the extraction error
			return err
		}
	}
	if ty := p.PicType(); ty == meta.PictureTypeIcon || ty == meta.PictureTypeOtherIcon {
		for i, existing := range t.pictures {
			if existing.Picture.PicType() == ty {
				t.RemovePicture(i)
				break
			}
		}
	}
	t.pictures = append(t.pictures, meta.PictureInfoPair{Picture: p, Info: i})
	return nil
}

// Clear removes all items and pictures.
func (t *VorbisCommentsTag) Clear() {
	t.entries = nil
	t.pictures = nil
}

// IsEmpty reports whether the tag has no items or pictures.
func (t *VorbisCommentsTag) IsEmpty() bool {
	return len(t.entries) == 0 && len(t.pictures) == 0
}

// Len returns item + picture count.
func (t *VorbisCommentsTag) Len() int { return len(t.entries) + len(t.pictures) }

// parseU8 is Rust's `str::parse::<u8>()` (one leading '+' accepted).
func parseU8(s string) (uint8, bool) {
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	n, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0, false
	}
	return uint8(n), true
}

// splitRating converts a `RATING:<email>` comment into a generic popularimeter,
// like the special case in Rust's `SplitTag for VorbisComments`. Ratings with
// an unparseable value or an email no provider handles stay in the remainder.
func splitRating(e vcEntry) (meta.Popularimeter, bool) {
	email, ok := strings.CutPrefix(e.key, "RATING:")
	if !ok {
		return meta.Popularimeter{}, false
	}
	v, ok := e.item.Value().IntoString()
	if !ok {
		return meta.Popularimeter{}, false
	}
	rate, ok := parseU8(v)
	if !ok {
		return meta.Popularimeter{}, false
	}
	// There is no play counter in Vorbis Comments
	return meta.PopularimeterMapped(email, meta.TagTypeVorbisComments, rate, 0)
}

// Tag converts to a generic [Tag].
func (t *VorbisCommentsTag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeVorbisComments)
	var remainder *VorbisCommentsTag
	for _, e := range t.entries {
		if e.item.Key() == meta.ItemKeyUnknown {
			if popm, ok := splitRating(e); ok {
				tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyPopularimeter, meta.ItemValueText(popm.String())))
				continue
			}
			// Unmappable items stay in the companion (Rust's split_tag)
			if util.GetGlobalOptions().PreserveFormatSpecificItems {
				if remainder == nil {
					remainder = NewVorbisCommentsTag()
				}
				remainder.entries = append(remainder.entries, e)
			}
			continue
		}
		tag.PushUnchecked(e.item)
	}
	if remainder != nil {
		remainder.Vendor = t.Vendor
		tag.SetCompanionTag(remainder)
	}
	// Preserve the vendor string as an EncoderSoftware item if one does
	// not already exist.
	if t.Vendor != "" && !tag.Contains(meta.ItemKeyEncoderSoftware) {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyEncoderSoftware, meta.ItemValueText(t.Vendor)))
	}
	for _, pic := range t.pictures {
		tag.PushPicture(pic.Picture)
	}
	return tag
}

// VorbisCommentsTagFromTag converts a generic [Tag] to a [VorbisCommentsTag],
// mirroring Rust's `MergeTag for SplitTagRemainder`.
func VorbisCommentsTagFromTag(input *meta.Tag, vendor string) *VorbisCommentsTag {
	input = input.Clone()
	vc := &VorbisCommentsTag{Vendor: vendor}
	// Format-specific items preserved in the companion merge back first
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*VorbisCommentsTag); ok && comp != nil {
			vc.entries = append(vc.entries, comp.entries...)
			if vc.Vendor == "" {
				vc.Vendor = comp.Vendor
			}
		}
	}
	// The first EncoderSoftware item becomes the vendor string.
	if taken := input.Take(meta.ItemKeyEncoderSoftware); len(taken) > 0 {
		if s, ok := taken[0].Value().Text(); ok {
			vc.Vendor = s
		}
	}
	for _, item := range input.Items() {
		// Binary items are not allowed in Vorbis comments
		val, ok := item.Value().IntoString()
		if !ok {
			continue
		}
		// Normalize flag items
		if item.Key() == meta.ItemKeyFlagCompilation {
			flag := util.FlagItem(val)
			if flag < 0 {
				continue
			}
			val = util.Itoa(flag)
			item.SetValue(meta.ItemValueText(val))
		}
		// Special case for generic popularimeters, since emails are part of
		// the field name
		if item.Key() == meta.ItemKeyPopularimeter {
			popm, err := meta.PopularimeterFromStr(val)
			if err != nil {
				continue
			}
			key := "RATING"
			if email := popm.Email(); email != "" {
				if !validVorbisCommentsKey(email) {
					continue
				}
				key += ":" + email
			}
			ritem := newVorbisEntry(key, util.Itoa(int(popm.MappedValue(meta.TagTypeVorbisComments))))
			vc.entries = append(vc.entries, ritem)
			continue
		}
		vc.pushMapped(item)
	}
	for _, pic := range input.Pictures() {
		info, err := meta.PictureInformationFromPicture(&pic)
		if err != nil {
			continue
		}
		vc.pictures = append(vc.pictures, meta.PictureInfoPair{Picture: pic, Info: info})
	}
	return vc
}

// --- codec ---

var errInvalidUtf8 = errors.New("invalid UTF-8")

// validVorbisCommentsKey mirrors Rust `valid_vorbis_comments_key`: the valid
// range is 0x20..=0x7D excluding 0x3D ('=').
func validVorbisCommentsKey(key string) bool {
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c < ' ' || c > '}' || c == '=' {
			return false
		}
	}
	return true
}

// asciiEscape renders a string like Rust's `escape_ascii`.
func asciiEscape(s string) string {
	q := strconv.QuoteToASCII(s)
	return q[1 : len(q)-1]
}

// strictBase64Decode decodes like `data_encoding::BASE64`: padding is
// required, trailing bits must be zero, and no characters (such as line
// breaks, which Go's decoder silently skips) are ignored.
func strictBase64Decode(s string) ([]byte, error) {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return nil, base64.CorruptInputError(i)
	}
	return base64.StdEncoding.Strict().DecodeString(s)
}

func allocCheck(n uint64) error {
	if n > uint64(util.GetGlobalOptions().AllocationLimit) {
		return &util.AllocationError{Kind: util.AllocationLimitExceeded}
	}
	return nil
}

// ParseVorbisComments parses the vendor+comments content (no "vorbis"/"OpusTags"
// signature, no framing bit — callers strip those).
func ParseVorbisComments(content []byte, parseOptions meta.ParseOptions) (*VorbisCommentsTag, error) {
	tag := NewVorbisCommentsTag()
	pos := 0
	readU32 := func() (uint32, error) {
		if pos+4 > len(content) {
			return 0, io.ErrUnexpectedEOF
		}
		v := binary.LittleEndian.Uint32(content[pos:])
		pos += 4
		return v, nil
	}
	vendorLen, err := readU32()
	if err != nil {
		return nil, &VorbisCommentsParseError{Source: err}
	}
	if uint64(vendorLen) > uint64(len(content)-pos) {
		return nil, &VorbisCommentsParseError{Source: &util.SizeMismatchError{}}
	}
	if err := allocCheck(uint64(vendorLen)); err != nil {
		return nil, &VorbisCommentsParseError{Source: err}
	}
	vendorBytes := content[pos : pos+int(vendorLen)]
	pos += int(vendorLen)
	if v, err := util.Utf8Decode(vendorBytes); err == nil {
		tag.Vendor = v
	} else if parseOptions.ParsingMode == meta.ParsingModeStrict {
		return nil, &VorbisCommentsParseError{Source: err}
	} else {
		// Rust recovery: decode each byte as a u16 word (UTF-16)
		words := make([]uint16, len(vendorBytes))
		for i, b := range vendorBytes {
			words[i] = uint16(b)
		}
		tag.Vendor = util.TrimEndNulls(string(utf16.Decode(words)))
	}

	itemCount, err := readU32()
	if err != nil {
		return nil, &VorbisCommentsParseError{Source: err}
	}
	if uint64(itemCount) > uint64(len(content)-pos)>>2 {
		return nil, &VorbisCommentsParseError{Source: &util.SizeMismatchError{}}
	}
	for i := uint32(0); i < itemCount; i++ {
		itemLen, err := readU32()
		if err != nil {
			return nil, &VorbisCommentsParseError{Source: err}
		}
		if uint64(itemLen) > uint64(len(content)-pos) {
			return nil, &VorbisCommentsParseError{Source: &util.SizeMismatchError{}}
		}
		if err := allocCheck(uint64(itemLen)); err != nil {
			return nil, &VorbisCommentsParseError{Source: err}
		}
		entry := string(content[pos : pos+int(itemLen)])
		pos += int(itemLen)

		eq := strings.IndexByte(entry, '=')
		if eq < 0 {
			continue
		}
		key, value := entry[:eq], entry[eq+1:]

		switch upper := asciiUpper(key); upper {
		case "METADATA_BLOCK_PICTURE":
			if !parseOptions.ReadCoverArt {
				continue
			}
			data, err := strictBase64Decode(value)
			if err != nil {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Source: err}
				}
				continue
			}
			pic, info, err := meta.PictureFromFlacBytes(data, false, parseOptions.ParsingMode)
			if err != nil {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Source: err}
				}
				continue
			}
			tag.pictures = append(tag.pictures, meta.PictureInfoPair{Picture: pic, Info: info})
		case "COVERART":
			if !parseOptions.ReadCoverArt {
				continue
			}
			// Deprecated format; converted to a picture with type Other
			// and a best-effort MIME guess (Rust's mimetype_from_bin
			// fallback is "image/").
			data, err := strictBase64Decode(value)
			if err != nil {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Source: err}
				}
				continue
			}
			pic := meta.PictureUnchecked(data).PicType(meta.PictureTypeOther).Build()
			if mt, err := meta.MimetypeFromBin(data); err == nil {
				pic.SetMimeType(mt)
			} else {
				pic.SetMimeType(meta.MimeTypeUnknown("image/"))
			}
			// Rust pushes PictureInformation::default() for COVERART
			tag.pictures = append(tag.pictures, meta.PictureInfoPair{Picture: pic, Info: meta.PictureInformation{}})
		case "TRACKNUMBER", "DISCNUMBER":
			if !utf8.ValidString(value) {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Source: errInvalidUtf8}
				}
				continue
			}
			// Rust's `utf8_decode_str` trims trailing NULs
			value = util.TrimEndNulls(value)
			isTrack := upper == "TRACKNUMBER"
			raw := func() { tag.entries = append(tag.entries, newVorbisEntry(upper, value)) }
			if !parseOptions.ImplicitConversions {
				raw()
				continue
			}
			// Support the case of TRACKNUMBER/DISCNUMBER being current/total
			parts := strings.SplitN(value, "/", 2)
			current, hasCurrent := util.U32FromString(parts[0])
			var total uint32
			var hasTotal bool
			if len(parts) == 2 {
				total, hasTotal = util.U32FromString(parts[1])
			}
			if isTrack {
				if hasTotal {
					tag.SetTrackTotal(total)
				}
				if hasCurrent {
					tag.SetTrack(current)
				} else {
					raw()
				}
			} else {
				if hasTotal {
					tag.SetDiskTotal(total)
				}
				if hasCurrent {
					tag.SetDisk(current)
				} else {
					raw()
				}
			}
		default:
			if !validVorbisCommentsKey(key) {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Message: "invalid item key: " + asciiEscape(key)}
				}
				continue
			}
			if !utf8.ValidString(value) {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &VorbisCommentsParseError{Source: errInvalidUtf8}
				}
				continue
			}
			// Rust's `utf8_decode_str` trims trailing NULs
			tag.entries = append(tag.entries, newVorbisEntry(key, util.TrimEndNulls(value)))
		}
	}
	return tag, nil
}

// EncodeVorbisComments serializes the tag (without signature/framing). Pictures
// are included as `METADATA_BLOCK_PICTURE` comments.
func EncodeVorbisComments(tag *VorbisCommentsTag) ([]byte, error) {
	return encodeVorbisComments(tag, true)
}

// EncodeVorbisCommentsItems serializes the tag's vendor and items only —
// pictures are left out. This is the FLAC `VORBIS_COMMENT` block body, where
// pictures live in their own `PICTURE` blocks (Rust `Block::new_comments`).
func EncodeVorbisCommentsItems(tag *VorbisCommentsTag) ([]byte, error) {
	return encodeVorbisComments(tag, false)
}

func encodeVorbisComments(tag *VorbisCommentsTag, withPictures bool) ([]byte, error) {
	size := 4 + len(tag.Vendor) + 4
	type kv struct{ k, v string }
	var entries []kv

	for _, e := range tag.entries {
		s, ok := entryValue(e)
		if !ok {
			continue
		}
		entries = append(entries, kv{e.key, s})
	}

	if withPictures {
		for _, pic := range tag.pictures {
			data := pic.Picture.AsFlacBytes(pic.Info, false)
			entries = append(entries, kv{"METADATA_BLOCK_PICTURE", base64.StdEncoding.EncodeToString(data)})
		}
	}

	// Rust `create_comments` drops empty values
	filtered := entries[:0]
	for _, e := range entries {
		if e.v != "" {
			filtered = append(filtered, e)
		}
	}
	entries = filtered

	for _, e := range entries {
		n := uint64(len(e.k)) + 1 + uint64(len(e.v))
		if n > math.MaxUint32 {
			return nil, &VorbisCommentsEncodingError{Source: &util.TooMuchDataError{}}
		}
		size += 4 + int(n)
	}
	buf, err := util.TryCapSlice[byte](size)
	if err != nil {
		return nil, &VorbisCommentsEncodingError{Source: err}
	}
	out := buf[:0]
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(tag.Vendor)))
	out = append(out, tmp[:]...)
	out = append(out, tag.Vendor...)
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(entries)))
	out = append(out, tmp[:]...)
	for _, e := range entries {
		binary.LittleEndian.PutUint32(tmp[:], uint32(len(e.k)+1+len(e.v)))
		out = append(out, tmp[:]...)
		out = append(out, e.k...)
		out = append(out, '=')
		out = append(out, e.v...)
	}
	return out, nil
}

// PadMetadataPacket pads a comment packet with zeroes so that the padding
// ends on a written-page boundary (8160 bytes), like Rust's
// `create_metadata_packet`. A `preferredPadding` of 0 disables padding.
func PadMetadataPacket(packet []byte, preferredPadding uint32) []byte {
	if preferredPadding == 0 {
		return packet
	}
	const boundary = 255 * 32
	cur := int64(len(packet))
	padded := (cur + int64(preferredPadding) + boundary - 1) / boundary * boundary
	return append(packet, make([]byte, padded-cur)...)
}

// VorbisCommentsParseError is returned when Vorbis comments fail to parse.
type VorbisCommentsParseError struct {
	Message string
	Source  error
}

func (e *VorbisCommentsParseError) Error() string {
	if e.Message != "" {
		return "VorbisComments: " + e.Message
	}
	return "failed to parse Vorbis comments"
}
func (e *VorbisCommentsParseError) Unwrap() error { return e.Source }

// VorbisCommentsEncodingError is returned when Vorbis comments fail to encode.
type VorbisCommentsEncodingError struct {
	Message string
	Source  error
}

func (e *VorbisCommentsEncodingError) Error() string {
	if e.Message != "" {
		return "VorbisComments: " + e.Message
	}
	return "failed to encode Vorbis comments"
}
func (e *VorbisCommentsEncodingError) Unwrap() error { return e.Source }

// VorbisCommentsTagForWrite builds a [VorbisCommentsTag] for encoding: all tag
// items are kept as comments, and the given vendor is used for the vendor
// field unless the tag carries an EncoderSoftware item (mirroring Rust's
// `create_vorbis_comments_ref` write path).
func VorbisCommentsTagForWrite(input *meta.Tag, vendor string) *VorbisCommentsTag {
	return VorbisCommentsTagForWritePics(input, vendor, true)
}

// VorbisCommentsTagForWritePics is [VorbisCommentsTagForWrite] with optional
// picture embedding. Pictures are only embedded as comments for Ogg; FLAC
// stores them in dedicated PICTURE metadata blocks.
func VorbisCommentsTagForWritePics(input *meta.Tag, vendor string, withPics bool) *VorbisCommentsTag {
	vc := VorbisCommentsTagForWriteRef(input, withPics)
	if vc.Vendor == "" {
		vc.Vendor = vendor
	}
	return vc
}

// VorbisCommentsTagForWriteRef is Rust's `create_vorbis_comments_ref`: the
// vendor is the tag's EncoderSoftware (or empty), text and locator items with
// a field mapping are kept verbatim (no flag or popularimeter conversion —
// that only happens when merging into a concrete tag), and pictures carry
// their extracted information. Writers replace the vendor with the file's own.
func VorbisCommentsTagForWriteRef(input *meta.Tag, withPics bool) *VorbisCommentsTag {
	vc := &VorbisCommentsTag{}
	if s, ok := input.GetString(meta.ItemKeyEncoderSoftware); ok {
		vc.Vendor = s
	}
	for _, item := range input.Items() {
		if _, ok := item.Value().IntoString(); !ok {
			continue
		}
		vc.pushMapped(item)
	}
	if withPics {
		for _, pic := range input.Pictures() {
			vc.PushPicture(pic)
		}
	}
	return vc
}

// WithVendor returns a shallow copy of the tag with the vendor replaced. The
// original is left untouched.
func (t *VorbisCommentsTag) WithVendor(vendor string) *VorbisCommentsTag {
	c := *t
	c.Vendor = vendor
	return &c
}

// TakeItems removes and returns all items as (key, value) pairs, mirroring
// Rust `VorbisComments::take_items`.
func (t *VorbisCommentsTag) TakeItems() [][2]string {
	out := t.Pairs()
	t.entries = nil
	return out
}

// SetPicture replaces the picture at `index`, mirroring Rust
// `OggPictureStorage::set_picture`: an out-of-range index appends the picture
// instead.
func (t *VorbisCommentsTag) SetPicture(index int, picture meta.Picture, info meta.PictureInformation) {
	if index < 0 || index >= len(t.pictures) {
		_ = t.InsertPicture(picture, meta.Some(info))
		return
	}
	t.pictures[index] = meta.PictureInfoPair{Picture: picture, Info: info}
}

// RemovePicture removes and returns the picture at `index`, mirroring Rust
// `OggPictureStorage::remove_picture`.
//
// Unlike Rust, an out-of-range index does not panic; it removes nothing and
// returns a zero value.
func (t *VorbisCommentsTag) RemovePicture(index int) meta.PictureInfoPair {
	if index < 0 || index >= len(t.pictures) {
		return meta.PictureInfoPair{}
	}
	pic := t.pictures[index]
	t.pictures = append(t.pictures[:index], t.pictures[index+1:]...)
	return pic
}

// RemovePictureType removes all pictures of `pictureType`, mirroring Rust
// `OggPictureStorage::remove_picture_type`.
func (t *VorbisCommentsTag) RemovePictureType(pictureType meta.PictureType) {
	var kept []meta.PictureInfoPair
	for _, p := range t.pictures {
		if p.Picture.PicType() != pictureType {
			kept = append(kept, p)
		}
	}
	t.pictures = kept
}

// RemovePictures removes and returns all pictures, mirroring Rust
// `OggPictureStorage::remove_pictures`.
func (t *VorbisCommentsTag) RemovePictures() []meta.PictureInfoPair {
	pics := t.pictures
	t.pictures = nil
	return pics
}

// DumpTo writes the tag's metadata packet to a writer, mirroring Rust
// `VorbisComments::dump_to` — this is only the tag data, not a usable file.
func (t *VorbisCommentsTag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := EncodeVorbisComments(t)
	if err != nil {
		return &meta.TagEncodingError{TagType: meta.TagTypeVorbisComments, HasTagType: true, Source: err}
	}
	data = PadMetadataPacket(data, writeOptions.PreferredPadding)
	_, err = writer.Write(data)
	return err
}

func init() {
	// Rust `dump_tag` (tag/utils.rs) builds a `VorbisCommentsRef` through
	// `create_vorbis_comments_ref` — the ENCODER item stays a comment, items
	// are written verbatim, pictures become METADATA_BLOCK_PICTURE comments —
	// and calls `VorbisCommentsRef::dump_to`, which applies preferred_padding.
	meta.RegisterTagDumper(meta.TagTypeVorbisComments, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		return VorbisCommentsTagForWriteRef(tag, true).DumpTo(writer, writeOptions)
	})
}

// SaveTo writes the tag into a Vorbis/Opus/Speex/FLAC file, mirroring Rust
// `VorbisComments::save_to`.
func (t *VorbisCommentsTag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return t.saveToVerified(vf, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *VorbisCommentsTag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	return t.saveToVerified(vf, writeOptions)
}

func (t *VorbisCommentsTag) saveToVerified(vf *meta.VerifiedFile, writeOptions meta.WriteOptions) error {
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeVorbisComments)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	if handled, err := meta.WriteConcreteTag(vf, meta.TagTypeVorbisComments, t, writeOptions); handled {
		return err
	}
	return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
}

// RemoveFrom removes a Vorbis comment block from a [meta.FileLike].
func (t *VorbisCommentsTag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeVorbisComments.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes a Vorbis comment block from a path.
func (t *VorbisCommentsTag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeVorbisComments.RemoveFromPath(path, writeOptions)
}
