package lofty

import (
	"encoding/base64"
	"encoding/binary"
	"io"
	"strconv"
	"strings"
)

// VorbisCommentsTag is a Vorbis comment header (used by FLAC, Vorbis, Opus, Speex).
type VorbisCommentsTag struct {
	Vendor   string
	items    []TagItem
	pictures []PictureInfoPair
}

// NewVorbisCommentsTag creates an empty tag with the default vendor.
func NewVorbisCommentsTag() *VorbisCommentsTag {
	return &VorbisCommentsTag{}
}

// Items returns the tag's items.
func (t *VorbisCommentsTag) Items() []TagItem { return t.items }

// Pictures returns the tag's pictures with their stored information.
func (t *VorbisCommentsTag) Pictures() []PictureInfoPair { return t.pictures }

// GetString gets the first string value for a key.
func (t *VorbisCommentsTag) GetString(key ItemKey) (string, bool) {
	for _, item := range t.items {
		if item.key == key {
			if s, ok := item.value.IntoString(); ok {
				return s, true
			}
		}
	}
	return "", false
}

// GetStrings gets all string values for a key.
func (t *VorbisCommentsTag) GetStrings(key ItemKey) []string {
	var out []string
	for _, item := range t.items {
		if item.key == key {
			if s, ok := item.value.IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// Insert replaces all items with the same key.
func (t *VorbisCommentsTag) Insert(item TagItem) {
	t.Remove(item.key)
	t.items = append(t.items, item)
}

// Push appends an item.
func (t *VorbisCommentsTag) Push(item TagItem) {
	_, ok := item.key.MapKey(TagTypeVorbisComments)
	if ok || item.key == ItemKeyUnknown {
		t.items = append(t.items, item)
	}
}

// Remove deletes all items with the key.
func (t *VorbisCommentsTag) Remove(key ItemKey) {
	var kept []TagItem
	for _, i := range t.items {
		if i.key != key {
			kept = append(kept, i)
		}
	}
	t.items = kept
}

// RemoveKey deletes all items with the format-specific key.
func (t *VorbisCommentsTag) RemoveKey(key string) {
	var kept []TagItem
	for _, i := range t.items {
		if k, ok := i.key.MapKey(TagTypeVorbisComments); !ok || k != key {
			if i.key != ItemKeyUnknown || i.Description != key {
				kept = append(kept, i)
			}
		}
	}
	t.items = kept
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
func vorbisItemKey(item TagItem) string {
	if item.key == ItemKeyUnknown {
		return item.Description
	}
	k, _ := item.key.MapKey(TagTypeVorbisComments)
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
			if s, ok := item.value.IntoString(); ok {
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
			if s, ok := item.value.IntoString(); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// newVorbisItem builds a [TagItem] for a raw field name.
func newVorbisItem(key, value string) TagItem {
	itemKey := itemKeyOrUnknown(TagTypeVorbisComments, key)
	item := NewTagItem(itemKey, ItemValueText(value))
	if itemKey == ItemKeyUnknown {
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
	var kept []TagItem
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

// u32FromString parses a decimal string, mirroring `str::parse::<u32>`.
func u32FromString(s string) (uint32, bool) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// Track returns the track number, checking TRACKNUMBER then TRACKNUM.
func (t *VorbisCommentsTag) Track() (uint32, bool) {
	if s, ok := t.Get("TRACKNUMBER"); !ok {
		if s, ok = t.Get("TRACKNUM"); !ok {
			return 0, false
		} else {
			return u32FromString(s)
		}
	} else {
		return u32FromString(s)
	}
}

// SetTrack sets the track number, normalizing to TRACKNUMBER.
func (t *VorbisCommentsTag) SetTrack(value uint32) {
	t.RemoveKey("TRACKNUMBER")
	t.RemoveKey("TRACKNUM")
	t.InsertKV("TRACKNUMBER", itoa(int(value)))
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
			return u32FromString(s)
		}
	} else {
		return u32FromString(s)
	}
}

// SetTrackTotal sets the total tracks, normalizing to TRACKTOTAL.
func (t *VorbisCommentsTag) SetTrackTotal(value uint32) {
	t.InsertKV("TRACKTOTAL", itoa(int(value)))
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
	return u32FromString(s)
}

// SetDisk sets the disc number.
func (t *VorbisCommentsTag) SetDisk(value uint32) { t.InsertKV("DISCNUMBER", itoa(int(value))) }

// RemoveDisk removes the disc number.
func (t *VorbisCommentsTag) RemoveDisk() { t.RemoveKey("DISCNUMBER") }

// DiskTotal returns the total discs, checking DISCTOTAL then TOTALDISCS.
func (t *VorbisCommentsTag) DiskTotal() (uint32, bool) {
	if s, ok := t.Get("DISCTOTAL"); !ok {
		if s, ok = t.Get("TOTALDISCS"); !ok {
			return 0, false
		} else {
			return u32FromString(s)
		}
	} else {
		return u32FromString(s)
	}
}

// SetDiskTotal sets the total discs, normalizing to DISCTOTAL.
func (t *VorbisCommentsTag) SetDiskTotal(value uint32) {
	t.InsertKV("DISCTOTAL", itoa(int(value)))
	t.RemoveKey("TOTALDISCS")
}

// RemoveDiskTotal removes the total discs.
func (t *VorbisCommentsTag) RemoveDiskTotal() {
	t.RemoveKey("DISCTOTAL")
	t.RemoveKey("TOTALDISCS")
}

// Date returns the recording date, checking DATE then YEAR.
func (t *VorbisCommentsTag) Date() Option[Timestamp] {
	if s, ok := t.Get("DATE"); ok {
		return tryParseTimestamp(s)
	}
	if s, ok := t.Get("YEAR"); ok {
		return tryParseTimestamp(s)
	}
	return None[Timestamp]()
}

// SetDate sets the recording date, normalizing to DATE.
func (t *VorbisCommentsTag) SetDate(value Timestamp) {
	t.InsertKV("DATE", value.String())
	t.RemoveKey("YEAR")
}

// RemoveDate removes the recording date.
func (t *VorbisCommentsTag) RemoveDate() {
	t.RemoveKey("DATE")
	t.RemoveKey("YEAR")
}

// PushPicture adds a picture, extracting the information from its data.
func (t *VorbisCommentsTag) PushPicture(p Picture) {
	info, err := PictureInformationFromPicture(&p)
	if err != nil {
		info = PictureInformation{}
	}
	t.pictures = append(t.pictures, PictureInfoPair{Picture: p, Info: info})
}

// InsertPicture adds a picture with explicit information, mirroring Rust's
// `OggPictureStorage::insert_picture`.
func (t *VorbisCommentsTag) InsertPicture(p Picture, info Option[PictureInformation]) error {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = PictureInformationFromPicture(&p)
		if err != nil {
			i = PictureInformation{}
		}
	}
	t.pictures = append(t.pictures, PictureInfoPair{Picture: p, Info: i})
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
func (t *VorbisCommentsTag) Tag() *Tag {
	tag := NewTag(TagTypeVorbisComments)
	var remainder *VorbisCommentsTag
	for _, item := range t.items {
		if item.key == ItemKeyUnknown {
			// Unmappable items stay in the companion (Rust's split_tag)
			if getGlobalOptions().PreserveFormatSpecificItems {
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
		tag.companionTag = remainder
	}
	// Preserve the vendor string as an EncoderSoftware item if one does
	// not already exist.
	if t.Vendor != "" && !tag.Contains(ItemKeyEncoderSoftware) {
		tag.PushUnchecked(NewTagItem(ItemKeyEncoderSoftware, ItemValueText(t.Vendor)))
	}
	for _, pic := range t.pictures {
		tag.PushPicture(pic.Picture)
	}
	return tag
}

// VorbisCommentsTagFromTag converts a generic [Tag] to a [VorbisCommentsTag].
func VorbisCommentsTagFromTag(input *Tag, vendor string) *VorbisCommentsTag {
	input = input.Clone()
	vc := &VorbisCommentsTag{Vendor: vendor}
	// Format-specific items preserved in the companion merge back first
	// (Rust's MergeTag for the VorbisComments remainder).
	if getGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.companionTag.(*VorbisCommentsTag); ok && comp != nil {
			vc.items = append(vc.items, comp.items...)
			if vc.Vendor == "" {
				vc.Vendor = comp.Vendor
			}
		}
	}
	// The first EncoderSoftware item becomes the vendor string.
	for _, item := range input.Take(ItemKeyEncoderSoftware) {
		if s, ok := item.value.IntoString(); ok {
			vc.Vendor = s
			break
		}
	}
	for _, item := range input.Items() {
		// Binary items are not allowed in Vorbis comments
		if item.value.kind == itemValueBinary {
			continue
		}
		// Normalize flag items
		if item.key == ItemKeyFlagCompilation {
			if s, ok := item.value.IntoString(); ok {
				if flag := flagItem(s); flag >= 0 {
					item.value = ItemValueText(itoa(flag))
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

// vorbisCommentSetupKeys are keys that store setup data, not metadata.
var vorbisCommentSetupKeys = []string{
	"METADATA_BLOCK_PICTURE", "COVERART", "COVERARTMIME", "BINARY",
}

// parseVorbisComments parses the vendor+comments content (no "vorbis"/"OpusTags"
// signature, no framing bit — callers strip those).
func parseVorbisComments(content []byte, parseOptions ParseOptions) (*VorbisCommentsTag, error) {
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
				tag.pictures = append(tag.pictures, PictureInfoPair{Picture: pic, Info: info})
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
				pic := PictureUnchecked(data).PicType(PictureTypeCoverFront).Build()
				if len(data) >= 8 {
					if mt, err := mimetypeFromBin(data[:8]); err == nil {
						pic.mimeType = Some(mt)
					}
				}
				tag.PushPicture(pic)
			}
		case "BINARY":
			item := NewTagItem(ItemKeyUnknown, ItemValueBinary([]byte(value)))
			item.Description = key
			tag.items = append(tag.items, item)
		case "TRACKNUMBER", "DISCNUMBER":
			upperKey := strings.ToUpper(key)
			isTrack := upperKey == "TRACKNUMBER"
			itemKey := ItemKeyDiscNumber
			if isTrack {
				itemKey = ItemKeyTrackNumber
			}
			raw := func() {
				tag.items = append(tag.items, NewTagItem(itemKey, ItemValueText(value)))
			}
			if !parseOptions.ImplicitConversions {
				raw()
				continue
			}
			// Support the case of TRACKNUMBER/DISCNUMBER being current/total
			parts := strings.SplitN(value, "/", 2)
			current, hasCurrent := u32FromString(parts[0])
			var total uint32
			var hasTotal bool
			if len(parts) == 2 {
				total, hasTotal = u32FromString(parts[1])
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
				item := NewTagItem(ItemKeyPopularimeter, ItemValueText(value))
				item.Description = email
				tag.items = append(tag.items, item)
				continue
			}
			itemKey := itemKeyOrUnknown(TagTypeVorbisComments, key)
			item := NewTagItem(itemKey, ItemValueText(value))
			if itemKey == ItemKeyUnknown {
				item.Description = key
			}
			tag.items = append(tag.items, item)
		}
	}
	return tag, nil
}

// encodeVorbisComments serializes the tag (without signature/framing).
func encodeVorbisComments(tag *VorbisCommentsTag) ([]byte, error) {
	size := 4 + len(tag.Vendor) + 4
	type kv struct{ k, v string }
	var entries []kv

	for _, item := range tag.items {
		if item.key == ItemKeyUnknown {
			if item.Description != "" {
				if item.value.kind == itemValueBinary {
					b, _ := item.value.IntoBinary()
					entries = append(entries, kv{item.Description, string(b)})
				} else if s, ok := item.value.IntoString(); ok {
					entries = append(entries, kv{item.Description, s})
				}
			}
			continue
		}
		key, ok := item.key.MapKey(TagTypeVorbisComments)
		if !ok {
			continue
		}
		s, ok := item.value.IntoString()
		if !ok {
			continue
		}
		// Ratings with an associated email are stored as RATING:<email>
		// (Rust's merge_tag special case).
		if item.key == ItemKeyPopularimeter && item.Description != "" {
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
	buf, err := tryCapSlice[byte](size)
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
func pictureFromFlacBytes(data []byte) (Picture, PictureInformation, error) {
	return PictureFromFlacBytes(data, false, ParsingModeBestAttempt)
}

// flacBytes serializes a picture to a FLAC picture block body.
func (p *Picture) flacBytes() ([]byte, error) {
	info, err := PictureInformationFromPicture(p)
	if err != nil {
		info = PictureInformation{}
	}
	return p.AsFlacBytes(info, false), nil
}

// flagItem normalizes a flag item value ("0"/"1"/"true"/"false"), -1 if invalid.
func flagItem(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true":
		return 1
	case "0", "false":
		return 0
	}
	return -1
}

// vorbisCommentsTagForWrite builds a [VorbisCommentsTag] for encoding: all tag
// items are kept as comments, and the given vendor is used for the vendor
// field (mirroring Rust's VorbisCommentsRef write path).
func vorbisCommentsTagForWrite(input *Tag, vendor string) *VorbisCommentsTag {
	return vorbisCommentsTagForWritePics(input, vendor, true)
}

// vorbisCommentsTagForWritePics is [vorbisCommentsTagForWrite] with optional
// picture embedding. Pictures are only embedded as comments for Ogg; FLAC
// stores them in dedicated PICTURE metadata blocks.
func vorbisCommentsTagForWritePics(input *Tag, vendor string, withPics bool) *VorbisCommentsTag {
	// The tag's EncoderSoftware is taken as the vendor string
	if s, ok := input.GetString(ItemKeyEncoderSoftware); ok && s != "" {
		vendor = s
	}
	vc := &VorbisCommentsTag{Vendor: vendor}
	for _, item := range input.Items() {
		if item.value.kind == itemValueBinary {
			continue
		}
		if item.key == ItemKeyFlagCompilation {
			if s, ok := item.value.IntoString(); ok {
				if flag := flagItem(s); flag >= 0 {
					item.value = ItemValueText(itoa(flag))
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
