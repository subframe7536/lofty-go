package vorbis

import (
	"encoding/base64"
	"encoding/binary"
	"io"
	"strings"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// VorbisCommentsTag is a Vorbis comment header (used by FLAC, Vorbis, Opus, Speex).
type VorbisCommentsTag struct {
	Vendor   string
	items    []meta.TagItem
	pictures []meta.PictureInfoPair
}

// NewVorbisCommentsTag creates an empty tag with the default vendor.
func NewVorbisCommentsTag() *VorbisCommentsTag {
	return &VorbisCommentsTag{}
}

// Items returns the tag's items.
func (t *VorbisCommentsTag) Items() []meta.TagItem { return t.items }

// Pictures returns the tag's pictures with their stored information.
func (t *VorbisCommentsTag) Pictures() []meta.PictureInfoPair { return t.pictures }

// SetPictures replaces the tag's stored pictures.
func (t *VorbisCommentsTag) SetPictures(pictures []meta.PictureInfoPair) { t.pictures = pictures }

// GetString gets the first string value for a key.
func (t *VorbisCommentsTag) GetString(key meta.ItemKey) (string, bool) {
	for _, item := range t.items {
		if item.Key() == key {
			if s, ok := item.Value().IntoString(); ok {
				return s, true
			}
		}
	}
	return "", false
}

// GetStrings gets all string values for a key.
func (t *VorbisCommentsTag) GetStrings(key meta.ItemKey) []string {
	var out []string
	for _, item := range t.items {
		if item.Key() == key {
			if s, ok := item.Value().IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// Insert replaces all items with the same key.
func (t *VorbisCommentsTag) Insert(item meta.TagItem) {
	t.Remove(item.Key())
	t.items = append(t.items, item)
}

// Push appends an item.
func (t *VorbisCommentsTag) Push(item meta.TagItem) {
	_, ok := item.Key().MapKey(meta.TagTypeVorbisComments)
	if ok || item.Key() == meta.ItemKeyUnknown {
		t.items = append(t.items, item)
	}
}

// Remove deletes all items with the key.
func (t *VorbisCommentsTag) Remove(key meta.ItemKey) {
	var kept []meta.TagItem
	for _, i := range t.items {
		if i.Key() != key {
			kept = append(kept, i)
		}
	}
	t.items = kept
}

// RemoveKey deletes all items with the format-specific key, returning the
// removed values, mirroring Rust `VorbisComments::remove`.
func (t *VorbisCommentsTag) RemoveKey(key string) []string {
	var kept []meta.TagItem
	var removed []string
	for _, i := range t.items {
		matched := false
		if k, ok := i.Key().MapKey(meta.TagTypeVorbisComments); ok && k == key {
			matched = true
		} else if i.Key() == meta.ItemKeyUnknown && i.Description == key {
			matched = true
		}
		if matched {
			if s, ok2 := i.Value().IntoString(); ok2 {
				removed = append(removed, s)
			}
			continue
		}
		kept = append(kept, i)
	}
	t.items = kept
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

// vorbisItemKey returns the raw field name for an item.
func vorbisItemKey(item meta.TagItem) string {
	if item.Key() == meta.ItemKeyUnknown {
		return item.Description
	}
	k, _ := item.Key().MapKey(meta.TagTypeVorbisComments)
	return k
}

// Get returns the first item with the key (case-insensitive), mirroring
// Rust's `VorbisComments::get`.
func (t *VorbisCommentsTag) Get(key string) (string, bool) {
	if !verifyVorbisKey(key) {
		return "", false
	}
	for _, item := range t.items {
		if k := vorbisItemKey(item); strings.EqualFold(k, key) {
			if s, ok := item.Value().IntoString(); ok {
				return s, true
			}
		}
	}
	return "", false
}

// GetAll returns every item with the key (case-insensitive).
func (t *VorbisCommentsTag) GetAll(key string) []string {
	var out []string
	for _, item := range t.items {
		if k := vorbisItemKey(item); strings.EqualFold(k, key) {
			if s, ok := item.Value().IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// newVorbisItem builds a [meta.TagItem] for a raw field name.
func newVorbisItem(key, value string) meta.TagItem {
	itemKey := meta.ItemKeyOrUnknown(meta.TagTypeVorbisComments, key)
	item := meta.NewTagItem(itemKey, meta.ItemValueText(value))
	if itemKey == meta.ItemKeyUnknown {
		item.Description = key
	}
	return item
}

// InsertKV removes any items with the key (case-insensitive), then appends
// the new item, mirroring Rust's `VorbisComments::insert`. Invalid keys are
// ignored.
func (t *VorbisCommentsTag) InsertKV(key, value string) {
	if !verifyVorbisKey(key) {
		return
	}
	var kept []meta.TagItem
	for _, item := range t.items {
		if !strings.EqualFold(vorbisItemKey(item), key) {
			kept = append(kept, item)
		}
	}
	t.items = append(kept, newVorbisItem(key, value))
}

// PushKV appends an item, mirroring Rust's `VorbisComments::push`. Invalid
// keys are ignored.
func (t *VorbisCommentsTag) PushKV(key, value string) {
	if !verifyVorbisKey(key) {
		return
	}
	t.items = append(t.items, newVorbisItem(key, value))
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
// `OggPictureStorage::insert_picture`.
func (t *VorbisCommentsTag) InsertPicture(p meta.Picture, info meta.Option[meta.PictureInformation]) error {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = meta.PictureInformationFromPicture(&p)
		if err != nil {
			i = meta.PictureInformation{}
		}
	}
	t.pictures = append(t.pictures, meta.PictureInfoPair{Picture: p, Info: i})
	return nil
}

// Clear removes all items and pictures.
func (t *VorbisCommentsTag) Clear() {
	t.items = nil
	t.pictures = nil
}

// IsEmpty reports whether the tag has no items or pictures.
func (t *VorbisCommentsTag) IsEmpty() bool {
	return len(t.items) == 0 && len(t.pictures) == 0
}

// Len returns item + picture count.
func (t *VorbisCommentsTag) Len() int { return len(t.items) + len(t.pictures) }

// Tag converts to a generic [Tag].
func (t *VorbisCommentsTag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeVorbisComments)
	var remainder *VorbisCommentsTag
	for _, item := range t.items {
		if item.Key() == meta.ItemKeyUnknown {
			// Unmappable items stay in the companion (Rust's split_tag)
			if util.GetGlobalOptions().PreserveFormatSpecificItems {
				if remainder == nil {
					remainder = NewVorbisCommentsTag()
				}
				remainder.items = append(remainder.items, item)
			}
			continue
		}
		tag.PushUnchecked(item)
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

// VorbisCommentsTagFromTag converts a generic [Tag] to a [VorbisCommentsTag].
func VorbisCommentsTagFromTag(input *meta.Tag, vendor string) *VorbisCommentsTag {
	input = input.Clone()
	vc := &VorbisCommentsTag{Vendor: vendor}
	// Format-specific items preserved in the companion merge back first
	// (Rust's MergeTag for the VorbisComments remainder).
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*VorbisCommentsTag); ok && comp != nil {
			vc.items = append(vc.items, comp.items...)
			if vc.Vendor == "" {
				vc.Vendor = comp.Vendor
			}
		}
	}
	// The first EncoderSoftware item becomes the vendor string.
	for _, item := range input.Take(meta.ItemKeyEncoderSoftware) {
		if s, ok := item.Value().IntoString(); ok {
			vc.Vendor = s
			break
		}
	}
	for _, item := range input.Items() {
		// Binary items are not allowed in Vorbis comments
		if item.Value().Kind() == meta.ItemValueKindBinary {
			continue
		}
		// Normalize flag items
		if item.Key() == meta.ItemKeyFlagCompilation {
			if s, ok := item.Value().IntoString(); ok {
				if flag := util.FlagItem(s); flag >= 0 {
					item.SetValue(meta.ItemValueText(util.Itoa(flag)))
				}
			}
		}
		vc.Push(item)
	}
	for _, pic := range input.Pictures() {
		vc.PushPicture(pic)
	}
	return vc
}

// --- codec ---

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
	if pos+int(vendorLen) > len(content) {
		return nil, &VorbisCommentsParseError{Source: io.ErrUnexpectedEOF}
	}
	tag.Vendor = string(content[pos : pos+int(vendorLen)])
	pos += int(vendorLen)

	itemCount, err := readU32()
	if err != nil {
		return nil, &VorbisCommentsParseError{Source: err}
	}
	for i := uint32(0); i < itemCount; i++ {
		itemLen, err := readU32()
		if err != nil {
			return nil, &VorbisCommentsParseError{Source: err}
		}
		if pos+int(itemLen) > len(content) {
			return nil, &VorbisCommentsParseError{Source: io.ErrUnexpectedEOF}
		}
		entry := string(content[pos : pos+int(itemLen)])
		pos += int(itemLen)

		eq := strings.IndexByte(entry, '=')
		if eq < 0 {
			continue
		}
		key, value := entry[:eq], entry[eq+1:]

		switch strings.ToUpper(key) {
		case "METADATA_BLOCK_PICTURE":
			if !parseOptions.ReadCoverArt {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				continue
			}
			pic, info, err := pictureFromFlacBytes(data)
			if err == nil {
				tag.pictures = append(tag.pictures, meta.PictureInfoPair{Picture: pic, Info: info})
			}
		case "COVERART", "COVERARTMIME":
			if !parseOptions.ReadCoverArt {
				continue
			}
			if key == "COVERART" {
				data, err := base64.StdEncoding.DecodeString(value)
				if err != nil {
					continue
				}
				pic := meta.PictureUnchecked(data).PicType(meta.PictureTypeCoverFront).Build()
				if len(data) >= 8 {
					if mt, err := meta.MimetypeFromBin(data[:8]); err == nil {
						pic.SetMimeType(mt)
					}
				}
				tag.PushPicture(pic)
			}
		case "BINARY":
			item := meta.NewTagItem(meta.ItemKeyUnknown, meta.ItemValueBinary([]byte(value)))
			item.Description = key
			tag.items = append(tag.items, item)
		case "TRACKNUMBER", "DISCNUMBER":
			upperKey := strings.ToUpper(key)
			isTrack := upperKey == "TRACKNUMBER"
			itemKey := meta.ItemKeyDiscNumber
			if isTrack {
				itemKey = meta.ItemKeyTrackNumber
			}
			raw := func() {
				tag.items = append(tag.items, meta.NewTagItem(itemKey, meta.ItemValueText(value)))
			}
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
			if email, ok := strings.CutPrefix(key, "RATING:"); ok {
				item := meta.NewTagItem(meta.ItemKeyPopularimeter, meta.ItemValueText(value))
				item.Description = email
				tag.items = append(tag.items, item)
				continue
			}
			itemKey := meta.ItemKeyOrUnknown(meta.TagTypeVorbisComments, key)
			item := meta.NewTagItem(itemKey, meta.ItemValueText(value))
			if itemKey == meta.ItemKeyUnknown {
				item.Description = key
			}
			tag.items = append(tag.items, item)
		}
	}
	return tag, nil
}

// EncodeVorbisComments serializes the tag (without signature/framing).
func EncodeVorbisComments(tag *VorbisCommentsTag) ([]byte, error) {
	size := 4 + len(tag.Vendor) + 4
	type kv struct{ k, v string }
	var entries []kv

	for _, item := range tag.items {
		if item.Key() == meta.ItemKeyUnknown {
			if item.Description != "" {
				if item.Value().Kind() == meta.ItemValueKindBinary {
					b, _ := item.Value().IntoBinary()
					entries = append(entries, kv{item.Description, string(b)})
				} else if s, ok := item.Value().IntoString(); ok {
					entries = append(entries, kv{item.Description, s})
				}
			}
			continue
		}
		key, ok := item.Key().MapKey(meta.TagTypeVorbisComments)
		if !ok {
			continue
		}
		s, ok := item.Value().IntoString()
		if !ok {
			continue
		}
		// Ratings with an associated email are stored as RATING:<email>
		// (Rust's merge_tag special case).
		if item.Key() == meta.ItemKeyPopularimeter && item.Description != "" {
			key = key + ":" + item.Description
		}
		entries = append(entries, kv{key, s})
	}

	for _, pic := range tag.pictures {
		data := pic.Picture.AsFlacBytes(pic.Info, false)
		entries = append(entries, kv{"METADATA_BLOCK_PICTURE", base64.StdEncoding.EncodeToString(data)})
	}

	for _, e := range entries {
		size += 4 + len(e.k) + 1 + len(e.v)
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

// pictureFromFlacBytes parses a FLAC METADATA_BLOCK_PICTURE block.
func pictureFromFlacBytes(data []byte) (meta.Picture, meta.PictureInformation, error) {
	return meta.PictureFromFlacBytes(data, false, meta.ParsingModeBestAttempt)
}

// flacBytes serializes a picture to a FLAC picture block body.

// VorbisCommentsTagForWrite builds a [VorbisCommentsTag] for encoding: all tag
// items are kept as comments, and the given vendor is used for the vendor
// field (mirroring Rust's VorbisCommentsRef write path).
func VorbisCommentsTagForWrite(input *meta.Tag, vendor string) *VorbisCommentsTag {
	return VorbisCommentsTagForWritePics(input, vendor, true)
}

// VorbisCommentsTagForWritePics is [VorbisCommentsTagForWrite] with optional
// picture embedding. Pictures are only embedded as comments for Ogg; FLAC
// stores them in dedicated PICTURE metadata blocks.
func VorbisCommentsTagForWritePics(input *meta.Tag, vendor string, withPics bool) *VorbisCommentsTag {
	// The tag's EncoderSoftware is taken as the vendor string
	if s, ok := input.GetString(meta.ItemKeyEncoderSoftware); ok && s != "" {
		vendor = s
	}
	vc := &VorbisCommentsTag{Vendor: vendor}
	for _, item := range input.Items() {
		if item.Value().Kind() == meta.ItemValueKindBinary {
			continue
		}
		if item.Key() == meta.ItemKeyFlagCompilation {
			if s, ok := item.Value().IntoString(); ok {
				if flag := util.FlagItem(s); flag >= 0 {
					item.SetValue(meta.ItemValueText(util.Itoa(flag)))
				}
			}
		}
		vc.Push(item)
	}
	if withPics {
		for _, pic := range input.Pictures() {
			vc.PushPicture(pic)
		}
	}
	return vc
}

// TakeItems removes and returns all items as (key, value) pairs, mirroring
// Rust `VorbisComments::take_items`.
func (t *VorbisCommentsTag) TakeItems() [][2]string {
	out := make([][2]string, 0, len(t.items))
	for _, i := range t.items {
		key := i.Description
		if i.Key() != meta.ItemKeyUnknown {
			if k, ok := i.Key().MapKey(meta.TagTypeVorbisComments); ok {
				key = k
			}
		}
		v, _ := i.Value().IntoString()
		out = append(out, [2]string{key, v})
	}
	t.items = nil
	return out
}

// SetPicture replaces the picture at `index`, mirroring Rust
// `OggPictureStorage::set_picture`.
func (t *VorbisCommentsTag) SetPicture(index int, picture meta.Picture, info meta.PictureInformation) {
	t.pictures[index] = meta.PictureInfoPair{Picture: picture, Info: info}
}

// RemovePicture removes and returns the picture at `index`, mirroring Rust
// `OggPictureStorage::remove_picture`.
func (t *VorbisCommentsTag) RemovePicture(index int) meta.PictureInfoPair {
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
	_, err = writer.Write(data)
	return err
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
