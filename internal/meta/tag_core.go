package meta

import (
	"bytes"
	"io"
	"strconv"

	"github.com/subframe7536/lofty-go/internal/util"
)

// Id3v2Version is the ID3v2 version.
type Id3v2Version = byte

const (
	// Id3v2Version2 is ID3v2.2.
	Id3v2Version2 Id3v2Version = 2
	// Id3v2Version3 is ID3v2.3.
	Id3v2Version3 Id3v2Version = 3
	// Id3v2Version4 is ID3v2.4.
	Id3v2Version4 Id3v2Version = 4
)

// Tag is a generic tag.
//
// Unlike the format-specific tags, this can hold every tag type in a normalized
// representation.
type Tag struct {
	tagType TagType
	// Pictures are stored independently of items.
	pictures []Picture
	items    []TagItem
	// companionTag stores format-specific metadata for a tag type,
	// when [GlobalOptions.PreserveFormatSpecificItems] is enabled and the tag
	// was created through a conversion. It holds a *T where T is the
	// concrete tag type (e.g. *Id3v2Tag, *ApeTag, ...).
	companionTag any
	// OriginalVersion records the ID3v2 version read from disk (3 or 4),
	// mirroring Rust's `Id3v2Tag::original_version`. Zero for other tag
	// types or tags created in memory.
	OriginalVersion Id3v2Version
}

// NewTag initializes a new tag with a certain [TagType].
func NewTag(tagType TagType) *Tag {
	return &Tag{tagType: tagType}
}

// CompanionCloner may be implemented by the concrete companion tag types
// (e.g. *Id3v2Tag, *ApeTag, ...) stored with [Tag.SetCompanionTag], so that
// [Tag.Clone] can deep-copy them. CloneCompanion must return a value of the
// same dynamic type that shares no mutable state with the receiver.
type CompanionCloner interface {
	CloneCompanion() any
}

// Clone returns a deep copy of the tag.
//
// Items (including binary values), pictures (including their data) are copied.
// The companion tag is deep-copied if it implements [CompanionCloner];
// otherwise it is shared with the clone.
func (t *Tag) Clone() *Tag {
	var items []TagItem
	if t.items != nil {
		items = make([]TagItem, len(t.items))
		for i, it := range t.items {
			it.value.binary = bytes.Clone(it.value.binary)
			items[i] = it
		}
	}
	var pics []Picture
	if t.pictures != nil {
		pics = make([]Picture, len(t.pictures))
		for i, p := range t.pictures {
			p.data = bytes.Clone(p.data)
			pics[i] = p
		}
	}
	companion := t.companionTag
	if c, ok := companion.(CompanionCloner); ok {
		companion = c.CloneCompanion()
	}
	return &Tag{
		tagType:         t.tagType,
		items:           items,
		pictures:        pics,
		companionTag:    companion,
		OriginalVersion: t.OriginalVersion,
	}
}

// ReMap changes the [TagType], remapping all items.
//
// NOTE: If any format-specific items are present, they will be removed.
// See [GlobalOptions.PreserveFormatSpecificItems].
func (t *Tag) ReMap(tagType TagType) {
	if t.companionTag != nil {
		t.companionTag = nil
	}
	t.Retain(func(i *TagItem) bool { return i.reMap(tagType) })
	t.tagType = tagType
}

// HasFormatSpecificItems reports whether the tag contains any format-specific items.
//
// See [GlobalOptions.PreserveFormatSpecificItems].
func (t *Tag) HasFormatSpecificItems() bool { return t.companionTag != nil }

// CompanionTag returns the format-specific companion tag preserved from a
// previous read, or nil. Format implementations use it to replay
// unconvertible frames on write.
func (t *Tag) CompanionTag() any { return t.companionTag }

// SetCompanionTag sets the format-specific companion tag.
func (t *Tag) SetCompanionTag(v any) { t.companionTag = v }

// TagType returns the [TagType].
func (t *Tag) TagType() TagType { return t.tagType }

// SetTagType forcefully changes the [TagType].
//
// WARNING: This will **not** remap the items, it is up to the caller to ensure
// the tag is compatible. See [Tag.ReMap] for the safe version.
func (t *Tag) SetTagType(tagType TagType) { t.tagType = tagType }

// ItemCount returns the number of [TagItem]s.
func (t *Tag) ItemCount() int { return len(t.items) }

// PictureCount returns the number of [Picture]s.
func (t *Tag) PictureCount() int { return len(t.pictures) }

// Items returns the stored [TagItem]s as a slice.
//
// Note: This returns the backing slice for zero-allocation iteration. Callers must
// not mutate elements directly. To modify items, use Tag methods such as [Tag.Insert],
// [Tag.SetText], [Tag.Remove], or [Tag.Retain].
func (t *Tag) Items() []TagItem { return t.items }

// Ratings returns all [Popularimeter] ratings.
func (t *Tag) Ratings() []Popularimeter {
	var ret []Popularimeter
	for _, s := range t.GetStrings(ItemKeyPopularimeter) {
		if p, err := PopularimeterFromStr(s); err == nil {
			ret = append(ret, p)
		}
	}
	return ret
}

// Get returns a reference to a [TagItem] matching an [ItemKey].
func (t *Tag) Get(itemKey ItemKey) *TagItem {
	for i := range t.items {
		if t.items[i].key == itemKey {
			return &t.items[i]
		}
	}
	return nil
}

// GetString gets a string value from an [ItemKey].
func (t *Tag) GetString(itemKey ItemKey) (string, bool) {
	if item := t.Get(itemKey); item != nil {
		return item.value.Text()
	}
	return "", false
}

func (t *Tag) getU32FromString(key ItemKey) (uint32, bool) {
	if s, ok := t.GetString(key); ok {
		// Rust: `s.parse::<u32>()` (accepts a single leading '+')
		return util.U32FromString(s)
	}
	return 0, false
}

// GetBinary gets a byte slice from an [ItemKey].
//
// Use `convert` to convert [ItemValueText] and [ItemValueLocator] to byte slices.
func (t *Tag) GetBinary(itemKey ItemKey, convert bool) ([]byte, bool) {
	if item := t.Get(itemKey); item != nil {
		switch item.value.kind {
		case ItemValueKindText, ItemValueKindLocator:
			if convert {
				return []byte(item.value.text), true
			}
		case ItemValueKindBinary:
			return item.value.binary, true
		}
	}
	return nil, false
}

// Insert inserts a [TagItem], replacing any existing one of the same [ItemKey].
//
// NOTE: This **will** verify an [ItemKey] mapping exists for the target [TagType].
// It returns true if the item was inserted.
func (t *Tag) Insert(item TagItem) bool {
	if item.reMap(t.tagType) {
		t.InsertUnchecked(item)
		return true
	}
	return false
}

// InsertUnchecked inserts a [TagItem], replacing any existing one of the same [ItemKey].
//
// Notes:
//   - This **will not** verify an [ItemKey] mapping exists.
//   - This **will not** allow writing item keys that are out of spec
//     (keys are verified before writing).
func (t *Tag) InsertUnchecked(item TagItem) {
	t.Retain(func(i *TagItem) bool { return i.key != item.key })
	t.items = append(t.items, item)
}

// Push appends a [TagItem] to the tag.
//
// This will not remove any items of the same [ItemKey], unlike [Tag.Insert].
//
// NOTE: This **will** verify an [ItemKey] mapping exists for the target [TagType].
// It returns true if the item was pushed.
func (t *Tag) Push(item TagItem) bool {
	if item.reMap(t.tagType) {
		t.items = append(t.items, item)
		return true
	}
	return false
}

// PushUnchecked appends a [TagItem] to the tag without verifying its key mapping.
func (t *Tag) PushUnchecked(item TagItem) { t.items = append(t.items, item) }

// InsertText is an alias for [Tag.Insert] that doesn't require creating a [TagItem].
func (t *Tag) InsertText(itemKey ItemKey, text string) bool {
	return t.Insert(NewTagItem(itemKey, ItemValueText(text)))
}

// Take removes all items with the specified [ItemKey], and returns them.
func (t *Tag) Take(key ItemKey) []TagItem {
	return t.TakeFilter(key, func(*TagItem) bool { return true })
}

// TakeFilter removes selected items with the specified [ItemKey], and returns them.
//
// Only takes items for which `filter` returns true. All other items are retained.
// Returns the selected items in order and preserves the ordering of the remaining items.
func (t *Tag) TakeFilter(key ItemKey, filter func(*TagItem) bool) []TagItem {
	var taken []TagItem
	var kept []TagItem
	for i := range t.items {
		item := t.items[i]
		if item.key == key && filter(&item) {
			taken = append(taken, item)
		} else {
			kept = append(kept, item)
		}
	}
	t.items = kept
	return taken
}

// TakeStrings removes all items with the specified [ItemKey], and returns their
// text values.
func (t *Tag) TakeStrings(key ItemKey) []string {
	var ret []string
	for _, item := range t.Take(key) {
		if s, ok := item.value.IntoString(); ok {
			ret = append(ret, s)
		}
	}
	return ret
}

// GetItems returns references to all [TagItem]s with the specified key.
func (t *Tag) GetItems(key ItemKey) []TagItem {
	var ret []TagItem
	for i := range t.items {
		if t.items[i].key == key {
			ret = append(ret, t.items[i])
		}
	}
	return ret
}

// GetStrings returns references to the text values of all [TagItem]s with the key.
func (t *Tag) GetStrings(key ItemKey) []string {
	var ret []string
	for i := range t.items {
		if t.items[i].key == key {
			if s, ok := t.items[i].value.Text(); ok {
				ret = append(ret, s)
			}
		}
	}
	return ret
}

// GetLocators returns references to all locators of [TagItem]s with the key.
func (t *Tag) GetLocators(key ItemKey) []string {
	var ret []string
	for i := range t.items {
		if t.items[i].key == key {
			if s, ok := t.items[i].value.Locator(); ok {
				ret = append(ret, s)
			}
		}
	}
	return ret
}

// GetBytes returns references to all bytes of [TagItem]s with the key.
func (t *Tag) GetBytes(key ItemKey) [][]byte {
	var ret [][]byte
	for i := range t.items {
		if t.items[i].key == key {
			if b, ok := t.items[i].value.Binary(); ok {
				ret = append(ret, b)
			}
		}
	}
	return ret
}

// RemoveKey removes all items with this key.
func (t *Tag) RemoveKey(key ItemKey) {
	t.Retain(func(i *TagItem) bool { return i.key != key })
}

// Retain keeps tag items based on the predicate.
//
// The tag's items are never modified in place: a slice previously returned by
// [Tag.Items] keeps its contents.
func (t *Tag) Retain(f func(*TagItem) bool) {
	var kept []TagItem
	removed := false
	for i := range t.items {
		if f(&t.items[i]) {
			if removed {
				kept = append(kept, t.items[i])
			}
			continue
		}
		if !removed {
			removed = true
			kept = make([]TagItem, i, len(t.items))
			copy(kept, t.items[:i])
		}
	}
	if removed {
		t.items = kept
	}
}

// RemoveEmpty removes all items with empty values.
func (t *Tag) RemoveEmpty() {
	t.Retain(func(item *TagItem) bool { return !item.value.IsEmpty() })
}

// Pictures returns the stored [Picture]s as a slice.
//
// Note: This returns the backing slice for zero-allocation iteration. Callers must
// not mutate elements directly. To modify pictures, use Tag methods such as
// [Tag.PushPicture], [Tag.SetPicture], or [Tag.RemovePictureType].
func (t *Tag) Pictures() []Picture { return t.pictures }

// GetPictureType returns the first occurrence of the [PictureType].
func (t *Tag) GetPictureType(pictureType PictureType) *Picture {
	for i := range t.pictures {
		if t.pictures[i].picType == pictureType {
			return &t.pictures[i]
		}
	}
	return nil
}

// PushPicture pushes a [Picture] to the tag.
func (t *Tag) PushPicture(picture Picture) { t.pictures = append(t.pictures, picture) }

// RemovePictureType removes all [Picture]s of a [PictureType].
func (t *Tag) RemovePictureType(pictureType PictureType) {
	kept := make([]Picture, 0, len(t.pictures))
	for _, p := range t.pictures {
		if p.picType != pictureType {
			kept = append(kept, p)
		}
	}
	t.pictures = kept
}

// SetPicture replaces the picture at the given `index`. If `index` is out of
// bounds, `picture` is appended to the list.
func (t *Tag) SetPicture(index int, picture Picture) {
	if index >= len(t.pictures) {
		t.PushPicture(picture)
	} else {
		t.pictures[index] = picture
	}
}

// RemovePicture removes and returns the picture at the given `index`.
func (t *Tag) RemovePicture(index int) Picture {
	pic := t.pictures[index]
	kept := make([]Picture, 0, len(t.pictures)-1)
	kept = append(kept, t.pictures[:index]...)
	kept = append(kept, t.pictures[index+1:]...)
	t.pictures = kept
	return pic
}

// Len returns the number of items and pictures in the tag.
func (t *Tag) Len() int { return len(t.items) + len(t.pictures) }

// Contains reports whether the tag contains an item with the key.
func (t *Tag) Contains(key ItemKey) bool { return t.Get(key) != nil }

// IsEmpty reports whether the tag has no items or pictures.
func (t *Tag) IsEmpty() bool { return len(t.items) == 0 && len(t.pictures) == 0 }

// Clear removes all items and pictures.
func (t *Tag) Clear() {
	t.items = nil
	t.pictures = nil
}

// DumpTo writes the tag to a writer. This is only the tag data itself,
// and is meant for use on a file by itself.
func (t *Tag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	return DumpTag(t, writer, writeOptions)
}

// RemoveFrom removes a tag of this type from a [FileLike].
func (t *Tag) RemoveFrom(file FileLike, writeOptions WriteOptions) error {
	return t.tagType.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes a tag of this type from a path.
func (t *Tag) RemoveFromPath(path string, writeOptions WriteOptions) error {
	return t.tagType.RemoveFromPath(path, writeOptions)
}

// SaveTo writes the tag to a [FileLike].
//
// This will first attempt to find a tag of the same type, and replace it,
// and if not, append the tag to the end of the file.
func (t *Tag) SaveTo(file FileLike, writeOptions WriteOptions) error {
	vf, err := NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return WriteTag(t, vf, writeOptions)
}

// SaveToPath writes the tag to a path.
//
// # Errors
//
// - The file type could not be determined
// - The file is an unsupported format
// - Attempting to write the tag to a format that does not support it
// - The file could not be opened
func (t *Tag) SaveToPath(path string, writeOptions WriteOptions) error {
	vf, f, err := VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer f.Close()
	return WriteTag(t, vf, writeOptions)
}

// SplitTagRemainder is the remainder of a [Tag] after calling [Tag.SplitTag].
type SplitTagRemainder struct{}

// SplitTag returns the remainder and the tag itself.
func (t *Tag) SplitTag() (SplitTagRemainder, *Tag) { return SplitTagRemainder{}, t }

// MergeTag merges the remainder and the tag.
func (SplitTagRemainder) MergeTag(tag *Tag) *Tag { return tag }

// --- Accessor-like common field methods ---

func accessorGet(t *Tag, key ItemKey) (string, bool) { return t.GetString(key) }

func accessorSet(t *Tag, key ItemKey, value string) { t.InsertText(key, value) }

// Artist returns the track artist.
func (t *Tag) Artist() (string, bool) { return accessorGet(t, ItemKeyTrackArtist) }

// SetArtist sets the track artist.
func (t *Tag) SetArtist(value string) { accessorSet(t, ItemKeyTrackArtist, value) }

// RemoveArtist removes the track artist.
func (t *Tag) RemoveArtist() { t.RemoveKey(ItemKeyTrackArtist) }

// Title returns the track title.
func (t *Tag) Title() (string, bool) { return accessorGet(t, ItemKeyTrackTitle) }

// SetTitle sets the track title.
func (t *Tag) SetTitle(value string) { accessorSet(t, ItemKeyTrackTitle, value) }

// RemoveTitle removes the track title.
func (t *Tag) RemoveTitle() { t.RemoveKey(ItemKeyTrackTitle) }

// Album returns the album title.
func (t *Tag) Album() (string, bool) { return accessorGet(t, ItemKeyAlbumTitle) }

// SetAlbum sets the album title.
func (t *Tag) SetAlbum(value string) { accessorSet(t, ItemKeyAlbumTitle, value) }

// RemoveAlbum removes the album title.
func (t *Tag) RemoveAlbum() { t.RemoveKey(ItemKeyAlbumTitle) }

// Genre returns the genre.
func (t *Tag) Genre() (string, bool) { return accessorGet(t, ItemKeyGenre) }

// SetGenre sets the genre.
func (t *Tag) SetGenre(value string) { accessorSet(t, ItemKeyGenre, value) }

// RemoveGenre removes the genre.
func (t *Tag) RemoveGenre() { t.RemoveKey(ItemKeyGenre) }

// Comment returns the comment.
func (t *Tag) Comment() (string, bool) { return accessorGet(t, ItemKeyComment) }

// SetComment sets the comment.
func (t *Tag) SetComment(value string) { accessorSet(t, ItemKeyComment, value) }

// RemoveComment removes the comment.
func (t *Tag) RemoveComment() { t.RemoveKey(ItemKeyComment) }

// Track returns the track number.
func (t *Tag) Track() (uint32, bool) { return t.getU32FromString(ItemKeyTrackNumber) }

// SetTrack sets the track number.
func (t *Tag) SetTrack(value uint32) {
	t.InsertText(ItemKeyTrackNumber, strconv.FormatUint(uint64(value), 10))
}

// RemoveTrack removes the track number.
func (t *Tag) RemoveTrack() { t.RemoveKey(ItemKeyTrackNumber) }

// TrackTotal returns the total number of tracks.
func (t *Tag) TrackTotal() (uint32, bool) { return t.getU32FromString(ItemKeyTrackTotal) }

// SetTrackTotal sets the total number of tracks.
func (t *Tag) SetTrackTotal(value uint32) {
	t.InsertText(ItemKeyTrackTotal, strconv.FormatUint(uint64(value), 10))
}

// RemoveTrackTotal removes the total number of tracks.
func (t *Tag) RemoveTrackTotal() { t.RemoveKey(ItemKeyTrackTotal) }

// Disk returns the disc number.
func (t *Tag) Disk() (uint32, bool) { return t.getU32FromString(ItemKeyDiscNumber) }

// SetDisk sets the disc number.
func (t *Tag) SetDisk(value uint32) {
	t.InsertText(ItemKeyDiscNumber, strconv.FormatUint(uint64(value), 10))
}

// RemoveDisk removes the disc number.
func (t *Tag) RemoveDisk() { t.RemoveKey(ItemKeyDiscNumber) }

// DiskTotal returns the total number of discs.
func (t *Tag) DiskTotal() (uint32, bool) { return t.getU32FromString(ItemKeyDiscTotal) }

// SetDiskTotal sets the total number of discs.
func (t *Tag) SetDiskTotal(value uint32) {
	t.InsertText(ItemKeyDiscTotal, strconv.FormatUint(uint64(value), 10))
}

// RemoveDiskTotal removes the total number of discs.
func (t *Tag) RemoveDiskTotal() { t.RemoveKey(ItemKeyDiscTotal) }

// Date returns the recording date.
func (t *Tag) Date() Option[Timestamp] {
	if s, ok := t.GetString(ItemKeyRecordingDate); ok {
		return tryParseTimestamp(s)
	}
	if s, ok := t.GetString(ItemKeyYear); ok {
		return tryParseTimestamp(s)
	}
	return None[Timestamp]()
}

// SetDate sets the recording date.
func (t *Tag) SetDate(value Timestamp) {
	t.RemoveKey(ItemKeyYear)
	t.InsertText(ItemKeyRecordingDate, value.String())
}

// RemoveDate removes the recording date.
func (t *Tag) RemoveDate() {
	t.RemoveKey(ItemKeyYear)
	t.RemoveKey(ItemKeyRecordingDate)
}
