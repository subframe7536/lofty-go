package id3v2

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/subframe7536/lofty-go/internal/util"
)

const v4MultiValueSeparator = "\x00"

// tiplMappings mirrors Rust's `TIPL_MAPPINGS`.
var tiplMappings = [][2]any{
	{ItemKeyProducer, "producer"},
	{ItemKeyArranger, "arranger"},
	{ItemKeyEngineer, "engineer"},
	{ItemKeyMixDj, "DJ-mix"},
	{ItemKeyMixEngineer, "mix"},
}

func tiplItemKeyForRole(role string) (ItemKey, bool) {
	for _, m := range tiplMappings {
		if m[1].(string) == role {
			return m[0].(ItemKey), true
		}
	}
	return 0, false
}

func tiplRoleForItemKey(key ItemKey) (string, bool) {
	for _, m := range tiplMappings {
		if m[0].(ItemKey) == key {
			return m[1].(string), true
		}
	}
	return "", false
}

// Id3v2TagToTag converts an [Id3v2Tag] to a generic [Tag] (Rust `From<Id3v2Tag>
// for Tag`). Unconvertible frames remain in the tag's companion when
// [GlobalOptions.PreserveFormatSpecificItems] is enabled.
func Id3v2TagToTag(input *Id3v2Tag) *Tag {
	// SplitTag consumes convertible frames from the list it runs on; clone so
	// that Tag() is non-destructive for callers holding the Id3v2Tag.
	clone := &Id3v2Tag{
		flags:           input.flags,
		originalVersion: input.originalVersion,
		Frames:          input.Frames.Clone(),
	}
	remainder, tag := clone.SplitTag()
	if util.GetGlobalOptions().PreserveFormatSpecificItems && !remainder.IsEmpty() {
		tag.SetCompanionTag(remainder)
	}
	return tag
}

// Id3v2TagFromTag converts a generic [Tag] to an [Id3v2Tag] (Rust `From<Tag>
// for Id3v2Tag`), merging any preserved companion frames.
func Id3v2TagFromTag(input *Tag) *Id3v2Tag {
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if companion, ok := input.CompanionTag().(*Id3v2Tag); ok {
			input.SetCompanionTag(nil)
			return companion.MergeTag(input)
		}
	}
	return NewId3v2Tag().MergeTag(input)
}

// SplitTag mirrors Rust `SplitTag::split_tag` — returns (remainder, generic tag).
func (t *Id3v2Tag) SplitTag() (*Id3v2Tag, *Tag) {
	tag := NewTag(TagTypeID3v2)
	tag.OriginalVersion = t.OriginalVersion()
	t.Frames.RetainMut(func(f Frame) bool {
		return handleTagSplit(tag, f)
	})
	return t, tag
}

// handleTagSplit returns true when the frame must be RETAINED (kept in the
// remainder), false when it was consumed into the generic tag.
func handleTagSplit(tag *Tag, frame Frame) bool {
	const consumed, retained = false, true

	splitPair := func(content string, numKey, totKey ItemKey) bool {
		parts := strings.SplitN(content, "/", 2)
		var numPart, totPart string
		hasTot := false
		if idx := strings.IndexAny(parts[0], "\x00"); idx >= 0 {
			numPart, totPart, hasTot = parts[0][:idx], parts[0][idx+1:], true
		} else if len(parts) == 2 {
			numPart, totPart, hasTot = parts[0], parts[1], true
		} else {
			numPart = parts[0]
		}
		parse := func(s string) (string, bool) {
			s = strings.TrimSpace(s)
			if s == "" {
				return "", false
			}
			if _, err := strconvParseU32(s); err != nil {
				return "", false
			}
			return s, true
		}
		num, ok := parse(numPart)
		if !ok {
			return false
		}
		tag.PushUnchecked(NewTagItem(numKey, ItemValueText(num)))
		if hasTot {
			tot, ok := parse(totPart)
			if !ok {
				return false
			}
			tag.PushUnchecked(NewTagItem(totKey, ItemValueText(tot)))
		}
		return true
	}

	switch f := frame.(type) {
	case *TextInformationFrame:
		id := f.ID().String()
		switch id {
		case "TRCK":
			if splitPair(f.Value, ItemKeyTrackNumber, ItemKeyTrackTotal) {
				return consumed
			}
		case "TPOS":
			if splitPair(f.Value, ItemKeyDiscNumber, ItemKeyDiscTotal) {
				return consumed
			}
		case "MVIN":
			if splitPair(f.Value, ItemKeyMovementNumber, ItemKeyMovementTotal) {
				return consumed
			}
		case "TCON":
			for _, g := range id3v2GenresIter(f.Value, false) {
				tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText(g)))
			}
			return consumed
		}
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, id)
		if !ok {
			return retained
		}
		for _, c := range strings.Split(f.Value, v4MultiValueSeparator) {
			tag.PushUnchecked(NewTagItem(itemKey, ItemValueText(c)))
		}
		return consumed

	case *KeyValueFrame:
		if f.ID().String() != "TIPL" {
			return retained
		}
		var kept [][2]string
		for _, kv := range f.KeyValuePairs {
			if ik, ok := tiplItemKeyForRole(kv[0]); ok {
				tag.PushUnchecked(NewTagItem(ik, ItemValueText(kv[1])))
			} else {
				kept = append(kept, kv)
			}
		}
		f.KeyValuePairs = kept
		return len(kept) != 0 // consumed only if all pairs were consumed

	case *ExtendedTextFrame:
		if f.Description == "" {
			return retained
		}
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, f.Description)
		if !ok {
			return retained
		}
		for _, c := range strings.Split(f.Content, v4MultiValueSeparator) {
			tag.PushUnchecked(NewTagItem(itemKey, ItemValueText(c)))
		}
		return consumed

	case *ExtendedUrlFrame:
		if f.Description == "" || len(f.Description) == 4 {
			return retained
		}
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, f.Description)
		if !ok {
			return retained
		}
		for _, c := range strings.Split(f.Content, v4MultiValueSeparator) {
			tag.PushUnchecked(NewTagItem(itemKey, ItemValueLocator(c)))
		}
		return consumed

	case *UniqueFileIdentifierFrame:
		if f.Owner != musicBrainzUfidOwner {
			return retained
		}
		res, err := decodeText(bytes.NewReader(f.Identifier), textDecodeOpts(util.TextEncodingLatin1, false))
		if err != nil {
			return retained
		}
		tag.PushUnchecked(NewTagItem(ItemKeyMusicBrainzRecordingId, ItemValueText(res.Content)))
		return consumed

	case *CommentFrame:
		pushLangItems(tag, ItemKeyComment, f.Language, f.Description, f.Content)
		return consumed

	case *UnsynchronizedTextFrame:
		pushLangItems(tag, ItemKeyUnsyncLyrics, f.Language, f.Description, f.Content)
		return consumed

	case *AttachedPictureFrame:
		tag.PushPicture(f.Picture)
		return consumed

	case *TimestampFrame:
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, f.ID().String())
		if !ok || f.Timestamp.Verify() != nil {
			return retained
		}
		tag.PushUnchecked(NewTagItem(itemKey, ItemValueText(f.Timestamp.String())))
		return consumed

	case *PopularimeterFrame:
		generic, ok := popularimeterMapped(f.Email, TagTypeID3v2, f.Rating, f.Counter)
		if !ok {
			return retained
		}
		tag.PushUnchecked(NewTagItem(ItemKeyPopularimeter, ItemValueText(generic.String())))
		return consumed

	case *UrlLinkFrame:
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, f.ID().String())
		if !ok {
			return retained
		}
		tag.PushUnchecked(NewTagItem(itemKey, ItemValueLocator(f.Content)))
		return consumed
	}
	return retained
}

func pushLangItems(tag *Tag, itemKey ItemKey, lang Lang, desc, content string) {
	for _, c := range strings.Split(content, v4MultiValueSeparator) {
		item := NewTagItem(itemKey, ItemValueText(c))
		item.Lang = lang
		if desc != EmptyContentDescriptor {
			item.Description = desc
		}
		tag.PushUnchecked(item)
	}
}

// MergeTag mirrors Rust `MergeTag for SplitTagRemainder` — merges generic tag
// items into this Id3v2Tag.
func (t *Id3v2Tag) MergeTag(tag *Tag) *Id3v2Tag {
	for _, frame := range fromTagItems(tag.Items()) {
		if frame.ID().String() == "TIPL" {
			// Extend an existing TIPL frame to retain its encoding/flags
			if existing, ok := t.Frames.TakeFirst(FrameIdValid("TIPL")).(*KeyValueFrame); ok {
				if kv, ok := frame.(*KeyValueFrame); ok {
					existing.KeyValuePairs = append(existing.KeyValuePairs, kv.KeyValuePairs...)
				}
				t.Frames.Push(existing)
				continue
			}
		}
		t.Frames.Push(frame)
	}
	for _, pic := range tag.Pictures() {
		if replaced := t.Frames.Insert(NewAttachedPictureFrame(util.TextEncodingUTF8, pic)); replaced != nil {
			// deduplicated
		}
	}
	return t
}

// tagFrames mirrors Rust `tag_frames` — item frames + companion frames + pictures.
func tagFrames(tag *Tag) []Frame {
	frames := fromTagItems(tag.Items())
	if companion, ok := tag.CompanionTag().(*Id3v2Tag); ok {
		for _, f := range companion.Frames.CloneFrames() {
			frames = append(frames, f)
		}
	}
	for _, pic := range tag.Pictures() {
		frames = append(frames, NewAttachedPictureFrame(util.TextEncodingUTF8, pic))
	}
	return frames
}

// fromTagItems mirrors Rust `conversion::from_tag` — converts TagItems to Frames.
func fromTagItems(items []TagItem) []Frame {
	type langDescKey struct {
		lyrics     bool
		lang, desc string
	}
	type kvPairs = [][2]string

	var tipl KeyValueFrame
	tiplInit := false
	textFrames := map[string]string{}
	var textOrder []string
	txxxFrames := map[string]string{}
	var txxxOrder []string
	comments := map[langDescKey]string{}
	var commentOrder []langDescKey
	unsync := map[langDescKey]string{}
	var unsyncOrder []langDescKey
	var trackNumber, trackTotal, discNumber, discTotal Option[uint32]

	extendText := func(m map[string]string, k, v string, order *[]string) {
		if cur, ok := m[k]; ok {
			m[k] = cur + v4MultiValueSeparator + v
		} else {
			*order = append(*order, k)
			m[k] = v
		}
	}
	parseNumber := func(item *TagItem) Option[uint32] {
		text, ok := item.Value().Text()
		if !ok {
			return None[uint32]()
		}
		n, err := atoi(strings.TrimSpace(text))
		if err != nil || n < 0 {
			return None[uint32]()
		}
		return Some(uint32(n))
	}

	var frames []Frame
	appendUnique := func(f Frame) {
		for _, ex := range frames {
			if ex.frameEqual(f) {
				return
			}
		}
		frames = append(frames, f)
	}

	for i := range items {
		item := &items[i]
		itemKey := item.Key()
		textOf := func() (string, bool) { return item.Value().IntoString() }

		switch {
		// Multi-valued TXXX key-to-frame mappings
		case id3v2MultiTxxxKeys[itemKey]:
			if desc, ok := itemKey.MapKey(TagTypeID3v2); ok {
				if v, ok := textOf(); ok {
					extendText(txxxFrames, desc, v, &txxxOrder)
				}
			}
		// Single-valued text key-to-frame mappings — dedup by ID, first wins
		case itemKey == ItemKeyIntegerBpm:
			if mapped, ok := itemKey.MapKey(TagTypeID3v2); ok {
				if v, ok := textOf(); ok {
					if fid, err := ParseFrameId(mapped); err == nil {
						appendUnique(NewTextInformationFrame(fid, util.TextEncodingUTF8, v))
					}
				}
			}
		// Single-valued TXXX mappings (ReplayGain) — last wins
		case id3v2SingleTxxxKeys[itemKey]:
			if desc, ok := itemKey.MapKey(TagTypeID3v2); ok {
				if v, ok := textOf(); ok {
					if _, exists := txxxFrames[desc]; !exists {
						txxxOrder = append(txxxOrder, desc)
					}
					txxxFrames[desc] = v
				}
			}
		// Comment/Unsync text — merged by (lang, description)
		case itemKey == ItemKeyComment || itemKey == ItemKeyUnsyncLyrics:
			if v, ok := textOf(); ok {
				ldk := langDescKey{itemKey == ItemKeyUnsyncLyrics, langOrXxx(item.Lang), item.Description}
				m := comments
				order := &commentOrder
				if ldk.lyrics {
					m = unsync
					order = &unsyncOrder
				}
				if cur, ok := m[ldk]; ok {
					m[ldk] = cur + v4MultiValueSeparator + v
				} else {
					*order = append(*order, ldk)
					m[ldk] = v
				}
			}
		// Flag items
		case itemKey.IsFlag():
			if frameID, ok := itemKey.MapKey(TagTypeID3v2); ok {
				if text, ok := item.Value().Text(); ok {
					if flag := flagItem(text); flag >= 0 {
						fid, err := ParseFrameId(frameID)
						if err == nil {
							appendUnique(NewTextInformationFrame(fid, util.TextEncodingUTF8, itoa(flag)))
						}
					}
				}
			}
		// iTunes advisory rating
		case itemKey == ItemKeyParentalAdvisory:
			if text, ok := item.Value().Text(); ok {
				if rating, ok := advisoryRatingFromStr(text); ok {
					appendUnique(NewExtendedTextFrame(util.TextEncodingUTF8, "ITUNESADVISORY", itoa(int(rating))))
				}
			}
		// Timestamps
		case itemKey.IsTimestamp():
			if frameID, ok := itemKey.MapKey(TagTypeID3v2); ok {
				if text, ok := textOf(); ok {
					fid, err := ParseFrameId(frameID)
					if err != nil {
						break
					}
					if ts, err := TimestampParseString(text); err == nil {
						appendUnique(NewTimestampFrame(fid, util.TextEncodingUTF8, ts))
					} else {
						appendUnique(NewTextInformationFrame(fid, util.TextEncodingUTF8, text))
					}
				}
			}
		case itemKey == ItemKeyTrackNumber:
			trackNumber = parseNumber(item)
		case itemKey == ItemKeyTrackTotal:
			trackTotal = parseNumber(item)
		case itemKey == ItemKeyDiscNumber:
			discNumber = parseNumber(item)
		case itemKey == ItemKeyDiscTotal:
			discTotal = parseNumber(item)
		case itemKey == ItemKeyMusicBrainzRecordingId:
			if v, ok := textOf(); ok && isASCII(v) {
				appendUnique(NewUniqueFileIdentifierFrame(musicBrainzUfidOwner, []byte(v)))
			}
		case itemKey == ItemKeyPopularimeter:
			if v, ok := textOf(); ok {
				if p, err := PopularimeterFromStr(v); err == nil {
					appendUnique(PopularimeterFrameFromPopularimeter(p))
				}
			}
		// TIPL key-value mappings
		case isTiplItemKey(itemKey):
			if role, ok := tiplRoleForItemKey(itemKey); ok {
				if v, ok := textOf(); ok {
					if !tiplInit {
						tipl = *NewKeyValueFrame(FrameIdValid("TIPL"), util.TextEncodingUTF8, nil)
						tiplInit = true
					}
					tipl.KeyValuePairs = append(tipl.KeyValuePairs, [2]string{role, v})
				}
			}
		default:
			if mapped, ok := itemKey.MapKey(TagTypeID3v2); ok && len(mapped) == 4 {
				if mapped[0] == 'T' || isNonStandardTextKey(itemKey) {
					if v, ok := textOf(); ok {
						extendText(textFrames, mapped, v, &textOrder)
					}
					break
				}
				if mapped[0] == 'W' {
					if fid, err := ParseFrameId(mapped); err == nil {
						if v, ok := textOf(); ok {
							appendUnique(NewUrlLinkFrame(fid, v))
						}
					}
					break
				}
			}
		}
	}

	if tiplInit && len(tipl.KeyValuePairs) > 0 {
		appendUnique(&tipl)
	}
	for _, id := range textOrder {
		fid, err := ParseFrameId(id)
		if err == nil {
			appendUnique(NewTextInformationFrame(fid, util.TextEncodingUTF8, textFrames[id]))
		}
	}
	for _, desc := range txxxOrder {
		appendUnique(NewExtendedTextFrame(util.TextEncodingUTF8, desc, txxxFrames[desc]))
	}
	for _, ldk := range commentOrder {
		lang := langFromXxx(ldk.lang)
		appendUnique(NewCommentFrame(util.TextEncodingUTF8, lang, ldk.desc, comments[ldk]))
	}
	for _, ldk := range unsyncOrder {
		lang := langFromXxx(ldk.lang)
		appendUnique(NewUnsynchronizedTextFrame(util.TextEncodingUTF8, lang, ldk.desc, unsync[ldk]))
	}
	if f := newNumberPairFrame(FrameIdValid("TRCK"), trackNumber, trackTotal); f != nil {
		appendUnique(f)
	}
	if f := newNumberPairFrame(FrameIdValid("TPOS"), discNumber, discTotal); f != nil {
		appendUnique(f)
	}
	return frames
}

func isTiplItemKey(k ItemKey) bool {
	_, ok := tiplRoleForItemKey(k)
	return ok
}

var nonStandardTextKeys = map[ItemKey]bool{
	ItemKeyMovement: true, ItemKeyMovementNumber: true, ItemKeyMovementTotal: true,
	ItemKeyAppleId3v2ContentGroup: true,
}

func isNonStandardTextKey(k ItemKey) bool { return nonStandardTextKeys[k] }

// newNumberPairFrame mirrors `new_number_pair_frame`.
func newNumberPairFrame(id FrameId, number, total Option[uint32]) Frame {
	var content string
	n, hasN := number.Get()
	t, hasT := total.Get()
	switch {
	case hasN && hasT:
		content = itoa(int(n)) + "/" + itoa(int(t))
	case hasN:
		content = itoa(int(n))
	case hasT:
		content = "0/" + itoa(int(t))
	default:
		return nil
	}
	return NewTextInformationFrame(id, util.TextEncodingUTF8, content)
}

func langFromXxx(s string) Lang {
	var l Lang
	if len(s) == 3 {
		copy(l[:], s)
	}
	return l
}

func advisoryRatingFromStr(s string) (uint8, bool) {
	n, err := atoi(s)
	if err != nil || n < 0 || n > 255 {
		return 0, false
	}
	return advisoryRatingFromU8(uint8(n))
}

func strconvParseU32(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	return uint32(n), err
}
