package lofty

import "bytes"

// Lang is a three character language code, as specified by ISO-639-2.
type Lang [3]byte

// LangEnglish is the English language code.
var LangEnglish = Lang{'e', 'n', 'g'}

// LangUnknown is the unknown/unspecified language code ("XXX").
var LangUnknown = Lang{'X', 'X', 'X'}

// ItemValue is the value of a [TagItem].
type ItemValue struct {
	kind   itemValueKind
	text   string
	binary []byte
}

type itemValueKind uint8

const (
	itemValueText itemValueKind = iota + 1
	itemValueLocator
	itemValueBinary
)

// ItemValueText is any UTF-8 encoded text.
func ItemValueText(s string) ItemValue { return ItemValue{kind: itemValueText, text: s} }

// ItemValueLocator is any UTF-8 encoded locator of external information.
//
// This only gets special treatment in ID3v2 and APE tags, being written
// as a normal string in other tags.
func ItemValueLocator(s string) ItemValue { return ItemValue{kind: itemValueLocator, text: s} }

// ItemValueBinary is binary information.
func ItemValueBinary(b []byte) ItemValue { return ItemValue{kind: itemValueBinary, binary: b} }

// Text returns the value if it is [ItemValueText].
func (v ItemValue) Text() (string, bool) {
	if v.kind == itemValueText {
		return v.text, true
	}
	return "", false
}

// Locator returns the value if it is [ItemValueLocator].
func (v ItemValue) Locator() (string, bool) {
	if v.kind == itemValueLocator {
		return v.text, true
	}
	return "", false
}

// Binary returns the value if it is [ItemValueBinary].
func (v ItemValue) Binary() ([]byte, bool) {
	if v.kind == itemValueBinary {
		return v.binary, true
	}
	return nil, false
}

// IntoString returns a string if the variant is [ItemValueText] or [ItemValueLocator].
func (v ItemValue) IntoString() (string, bool) {
	if v.kind == itemValueText || v.kind == itemValueLocator {
		return v.text, true
	}
	return "", false
}

// IntoBinary returns a byte slice if the variant is [ItemValueBinary].
func (v ItemValue) IntoBinary() ([]byte, bool) { return v.Binary() }

// Equal reports whether two values are identical (kind and content).
func (v ItemValue) Equal(other ItemValue) bool {
	if v.kind != other.kind {
		return false
	}
	if v.kind == itemValueBinary {
		return bytes.Equal(v.binary, other.binary)
	}
	return v.text == other.text
}

// IsEmpty reports whether the value is empty.
func (v ItemValue) IsEmpty() bool {
	switch v.kind {
	case itemValueBinary:
		return len(v.binary) == 0
	default:
		return v.text == ""
	}
}

// TagItem is a generic tag item.
type TagItem struct {
	Lang        Lang
	Description string
	key         ItemKey
	value       ItemValue
}

// NewTagItem creates a new [TagItem].
func NewTagItem(itemKey ItemKey, itemValue ItemValue) TagItem {
	return TagItem{Lang: LangUnknown, key: itemKey, value: itemValue}
}

// NewTagItemChecked creates a new [TagItem], returning false if the [ItemKey]
// does not map to a key in the target [TagType].
func NewTagItemChecked(tagType TagType, itemKey ItemKey, itemValue ItemValue) (TagItem, bool) {
	if _, ok := itemKey.MapKey(tagType); !ok {
		return TagItem{}, false
	}
	return NewTagItem(itemKey, itemValue), true
}

// Key returns the [ItemKey].
func (t TagItem) Key() ItemKey { return t.key }

// Value returns the [ItemValue].
func (t TagItem) Value() ItemValue { return t.value }

// IntoValue returns the [ItemValue].
func (t TagItem) IntoValue() ItemValue { return t.value }

// reMap reports whether the item's key is valid for `tagType`.
func (t TagItem) reMap(tagType TagType) bool {
	switch tagType {
	case TagTypeID3v1:
		return itemKeyInSlice(id3v1ValidItemKeys, t.key)
	case TagTypeLyrics3:
		return itemKeyInSlice(lyrics3v1ValidItemKeys, t.key)
	case TagTypeLyrics3v2:
		return itemKeyInSlice(lyrics3v2ValidItemKeys, t.key)
	default:
		_, ok := t.key.MapKey(tagType)
		return ok
	}
}

func itemKeyInSlice(s []ItemKey, k ItemKey) bool {
	for _, x := range s {
		if x == k {
			return true
		}
	}
	return false
}

// ID3v1's supported keys.
var id3v1ValidItemKeys = []ItemKey{
	ItemKeyTrackTitle,
	ItemKeyTrackArtist,
	ItemKeyAlbumTitle,
	ItemKeyComment,
	ItemKeyTrackNumber,
	ItemKeyGenre,
	ItemKeyYear,
	ItemKeyRecordingDate,
}

// Lyrics3's supported keys.
var lyrics3v1ValidItemKeys = []ItemKey{ItemKeyLyrics}

// Lyrics3v2's supported keys.
var lyrics3v2ValidItemKeys = []ItemKey{
	ItemKeyLyrics,
	ItemKeyComment,
	ItemKeyLyricist,
	ItemKeyAlbumTitle,
	ItemKeyTrackArtist,
	ItemKeyTrackTitle,
}
