package lofty

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	bogem "github.com/bogem/id3v2/v2"
)

// Id3v2Version is the ID3v2 version.
type Id3v2Version = byte

const (
	// Id3v2Version3 is ID3v2.3.
	Id3v2Version3 Id3v2Version = 3
	// Id3v2Version4 is ID3v2.4.
	Id3v2Version4 Id3v2Version = 4
)

// Id3v2ParseError is returned when an ID3v2 tag fails to parse.
type Id3v2ParseError struct {
	Message string
	Source  error
}

func (e *Id3v2ParseError) Error() string {
	if e.Message != "" {
		return "ID3v2: " + e.Message
	}
	return "failed to parse ID3v2 tag"
}
func (e *Id3v2ParseError) Unwrap() error { return e.Source }

// Id3v2EncodingError is returned when an ID3v2 tag fails to encode.
type Id3v2EncodingError struct {
	Message string
	Source  error
}

func (e *Id3v2EncodingError) Error() string {
	if e.Message != "" {
		return "ID3v2: " + e.Message
	}
	return "failed to write ID3v2 tag"
}
func (e *Id3v2EncodingError) Unwrap() error { return e.Source }

// Id3v2TagFlags are the ID3v2 header's flags.
type Id3v2TagFlags struct {
	Unsynchronisation bool
	ExtendedHeader    bool
	Experimental      bool
	Footer            bool
	// Whether the extended header includes a CRC-32.
	Crc bool
	// Restrictions declared in the extended header.
	Restrictions *TagRestrictions
}

// TagSizeRestriction restricts the total size of an ID3v2.4 tag.
type TagSizeRestriction int

const (
	// TagSizeS128F1M — no more than 128 frames and 1 MB total tag size.
	TagSizeS128F1M TagSizeRestriction = iota
	// TagSizeS64F128K — no more than 64 frames and 128 KB total tag size.
	TagSizeS64F128K
	// TagSizeS32F40K — no more than 32 frames and 40 KB total tag size.
	TagSizeS32F40K
	// TagSizeS32F4K — no more than 32 frames and 4 KB total tag size.
	TagSizeS32F4K
)

// TextSizeRestriction restricts text field sizes.
type TextSizeRestriction int

const (
	// TextSizeC1024 — no longer than 1024 characters.
	TextSizeC1024 TextSizeRestriction = iota
	// TextSizeC128 — no longer than 128 characters.
	TextSizeC128
	// TextSizeC30 — no longer than 30 characters.
	TextSizeC30
)

// ImageSizeRestriction restricts image sizes.
type ImageSizeRestriction int

const (
	// ImageSizeP256 — all images are 256x256 or smaller.
	ImageSizeP256 ImageSizeRestriction = iota + 1
	// ImageSizeP64 — all images are 64x64 or smaller.
	ImageSizeP64
	// ImageSizeP6464 — all images are exactly 64x64.
	ImageSizeP6464
)

// TagRestrictions are the restrictions declared in an ID3v2.4 extended header.
type TagRestrictions struct {
	Size           TagSizeRestriction
	TextEncoding   bool
	TextFieldsSize Option[TextSizeRestriction]
	ImageEncoding  bool
	ImageSize      Option[ImageSizeRestriction]
}

// tagRestrictionsFromByte decodes a restrictions byte (id3v2.4 §3.2 item d).
func tagRestrictionsFromByte(b byte) TagRestrictions {
	var r TagRestrictions
	switch b & 0xC0 {
	case 0x40:
		r.Size = TagSizeS64F128K
	case 0x80:
		r.Size = TagSizeS32F40K
	case 0xC0:
		r.Size = TagSizeS32F4K
	}
	if b&0x20 != 0 {
		r.TextEncoding = true
	}
	switch b & 0x18 {
	case 0x08:
		r.TextFieldsSize = Some(TextSizeC1024)
	case 0x10:
		r.TextFieldsSize = Some(TextSizeC128)
	case 0x18:
		r.TextFieldsSize = Some(TextSizeC30)
	}
	if b&0x04 != 0 {
		r.ImageEncoding = true
	}
	switch b & 0x03 {
	case 1:
		r.ImageSize = Some(ImageSizeP256)
	case 2:
		r.ImageSize = Some(ImageSizeP64)
	case 3:
		r.ImageSize = Some(ImageSizeP6464)
	}
	return r
}

// Id3v2Header is the ID3v2 tag's header.
type Id3v2Header struct {
	Version Id3v2Version
	Flags   Id3v2TagFlags
	// Size is the size of the tag, excluding the 10-byte header.
	Size uint32
	// ExtendedSize is the size of the extended header (excludes its own size
	// field for v2.3; includes it for v2.4).
	ExtendedSize uint32
}

// FullTagSize returns the size of the entire tag, including header and footer.
func (h Id3v2Header) FullTagSize() uint32 {
	size := h.Size + 10
	if h.Flags.Footer {
		size += 10
	}
	return size
}

func parseId3v2Header(reader io.Reader) (Id3v2Header, error) {
	var b [10]byte
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return Id3v2Header{}, err
	}
	if !bytes.Equal(b[:3], []byte("ID3")) {
		return Id3v2Header{}, &Id3v2ParseError{Source: &FakeTagError{}}
	}
	version := b[3]
	if version != 2 && version != 3 && version != 4 {
		return Id3v2Header{}, &Id3v2ParseError{Message: "unsupported ID3v2 version"}
	}
	var h Id3v2Header
	h.Version = version
	flags := b[5]
	if version == 2 {
		// Bit 2 is the abandoned v2.2 compression flag — the spec says to ignore the tag.
		if flags&0x40 != 0 {
			return Id3v2Header{}, &Id3v2ParseError{Message: "ID3v2.2 compression is not supported"}
		}
		h.Flags.Unsynchronisation = flags&0x80 != 0
	} else if version == 3 {
		h.Flags.Unsynchronisation = flags&0x80 != 0
		h.Flags.ExtendedHeader = flags&0x40 != 0
		h.Flags.Experimental = flags&0x20 != 0
	} else {
		h.Flags.Unsynchronisation = flags&0x80 != 0
		h.Flags.ExtendedHeader = flags&0x40 != 0
		h.Flags.Experimental = flags&0x20 != 0
		h.Flags.Footer = flags&0x10 != 0
	}
	h.Size = unsynchsafe32(b[6:10])
	return h, nil
}

// ParseId3v2Header parses an ID3v2 header plus its extended header (if any)
// from reader, matching Rust's `Id3v2Header::parse`.
func ParseId3v2Header(reader io.Reader, parseOptions ParseOptions) (Id3v2Header, error) {
	h, err := parseId3v2Header(reader)
	if err != nil {
		return h, err
	}
	if !h.Flags.ExtendedHeader {
		return h, nil
	}
	var sizeBuf [4]byte
	if _, err := io.ReadFull(reader, sizeBuf[:]); err != nil {
		return h, &Id3v2ParseError{Source: err}
	}
	var extContent []byte
	if h.Version == 3 {
		h.ExtendedSize = binary.BigEndian.Uint32(sizeBuf[:])
		extContent = make([]byte, h.ExtendedSize)
	} else {
		h.ExtendedSize = unsynchsafe32(sizeBuf[:])
		// v2.4's extended size includes its own size field
		if h.ExtendedSize < 4 {
			return h, &Id3v2ParseError{Message: "bad extended header size"}
		}
		extContent = make([]byte, h.ExtendedSize-4)
	}
	if _, err := io.ReadFull(reader, extContent); err != nil {
		return h, &Id3v2ParseError{Source: err}
	}
	if err := h.parseExtendedHeader(extContent, parseOptions.ParsingMode); err != nil {
		return h, err
	}
	if h.ExtendedSize > 0 && h.ExtendedSize > h.Size {
		return h, &Id3v2ParseError{Message: "bad extended header size"}
	}
	return h, nil
}

// parseExtendedHeader interprets the extended header content (after the size
// field for v2.3; after the synchsafe size field for v2.4).
func (h *Id3v2Header) parseExtendedHeader(content []byte, mode ParsingMode) error {
	if h.Version == 3 {
		// ext content: 2-byte flags + 4-byte padding size + optional 4-byte CRC
		if h.ExtendedSize != 6 && h.ExtendedSize != 10 {
			return &Id3v2ParseError{Message: "bad extended header size"}
		}
		if len(content) < int(h.ExtendedSize) {
			return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		extFlags := binary.BigEndian.Uint16(content[:2])
		remaining := content[6:h.ExtendedSize]
		if extFlags&0x8000 != 0 {
			h.Flags.Crc = true
			remaining = remaining[4:]
		}
		if extFlags&^0x8000 != 0 || len(remaining) != 0 {
			if mode == ParsingModeStrict {
				return &Id3v2ParseError{Message: "bad extended header flags"}
			}
		}
		return nil
	}

	// v2.4: ext content (after size field): 1-byte flag count + 1-byte flags +
	// per-flag (len + data). h.ExtendedSize already includes the size field.
	if h.ExtendedSize < 6 || h.ExtendedSize > 15 {
		return &Id3v2ParseError{Message: "bad extended header size"}
	}
	if len(content) < 2 {
		return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
	}
	// flag byte count (unused — only one flag byte is defined)
	content = content[1:]
	extFlags := content[0]
	content = content[1:]
	if extFlags&0x40 != 0 { // tag is an update
		if len(content) < 1 {
			return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		content = content[1:]
	}
	if extFlags&0x20 != 0 { // CRC data present
		h.Flags.Crc = true
		if len(content) < 6 {
			return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		content = content[6:]
	}
	if extFlags&0x10 != 0 { // tag restrictions
		if len(content) < 2 {
			return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		r := tagRestrictionsFromByte(content[1])
		h.Flags.Restrictions = &r
		content = content[2:]
	}
	if extFlags&^0x70 != 0 || len(content) != 0 {
		if mode == ParsingModeStrict {
			return &Id3v2ParseError{Message: "bad extended header flags"}
		}
	}
	return nil
}

// --- Tag <-> bogem conversion ---

// The MusicBrainz UFID owner identifier.
const id3v2MusicBrainzUfidOwner = "http://musicbrainz.org"

// tiplKeyRoles maps involved-people roles to ItemKeys.
var tiplKeyRoles = map[string]ItemKey{
	"producer":  ItemKeyProducer,
	"arranger":  ItemKeyArranger,
	"engineer":  ItemKeyEngineer,
	"mix":       ItemKeyMixEngineer,
	"dj-mix":    ItemKeyMixDj,
	"DJ-mix":    ItemKeyMixDj,
	"mixer":     ItemKeyMixEngineer,
	"director":  ItemKeyDirector,
	"conductor": ItemKeyConductor,
	"lyricist":  ItemKeyLyricist,
	"writer":    ItemKeyWriter,
	"composer":  ItemKeyComposer,
	"performer": ItemKeyPerformer,
	"remixer":   ItemKeyRemixer,
	"publisher": ItemKeyPublisher,
}

var tiplItemKeyRoles = map[ItemKey]string{
	ItemKeyProducer:    "producer",
	ItemKeyArranger:    "arranger",
	ItemKeyEngineer:    "engineer",
	ItemKeyMixEngineer: "mix",
	ItemKeyMixDj:       "DJ-mix",
	ItemKeyDirector:    "director",
	ItemKeyConductor:   "conductor",
	ItemKeyLyricist:    "lyricist",
	ItemKeyWriter:      "writer",
	ItemKeyComposer:    "composer",
	ItemKeyPerformer:   "performer",
	ItemKeyRemixer:     "remixer",
	ItemKeyPublisher:   "publisher",
}

// id3v2TagToGeneric converts a parsed bogem tag to a generic [Tag].
func id3v2TagToGeneric(btag *bogem.Tag, parseOptions ParseOptions) *Tag {
	tag := NewTag(TagTypeID3v2)

	addText := func(id string, tf bogem.TextFrame) {
		if id == "TCON" {
			// Genre references ("(4)", "RX", ...) get translated
			for _, g := range id3v2GenresIter(tf.Text, false) {
				tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText(g)))
			}
			return
		}
		// Multi-value text is separated by null bytes
		text := tf.Text
		if tf.Encoding.Key == 1 || tf.Encoding.Key == 2 {
			text = stripUtf16BomRunes(text)
		}
		parts := splitNulls(text)
		if len(parts) == 0 {
			parts = []string{""}
		}
		// Pair frames: "num/total" in a single frame
		var numKey, totKey ItemKey
		switch id {
		case "TRCK":
			numKey, totKey = ItemKeyTrackNumber, ItemKeyTrackTotal
		case "TPOS":
			numKey, totKey = ItemKeyDiscNumber, ItemKeyDiscTotal
		case "MVIN":
			numKey, totKey = ItemKeyMovementNumber, ItemKeyMovementTotal
		}
		if id == "TRCK" || id == "TPOS" || id == "MVIN" {
			num, tot, ok := splitNumPair(text)
			if !ok {
				// The whole frame is unconvertible — retain it as an
				// unknown binary item (Rust keeps it in the remainder).
				var body bytes.Buffer
				body.WriteByte(tf.Encoding.Key)
				body.WriteString(text)
				item := NewTagItem(ItemKeyUnknown, ItemValueBinary(body.Bytes()))
				item.Description = id
				tag.PushUnchecked(item)
				return
			}
			tag.PushUnchecked(NewTagItem(numKey, ItemValueText(num)))
			if tot != "" {
				tag.PushUnchecked(NewTagItem(totKey, ItemValueText(tot)))
			}
			return
		}
		if id == "TIPL" || id == "IPLS" || id == "TMCL" {
			// Involved people: pairs of role \x00 name
			for i := 0; i+1 < len(parts); i += 2 {
				role := parts[i]
				name := parts[i+1]
				if ik, ok := tiplKeyRoles[role]; ok {
					tag.PushUnchecked(NewTagItem(ik, ItemValueText(name)))
				}
			}
			return
		}
		itemKey, ok := ItemKeyFromKey(TagTypeID3v2, id)
		if !ok {
			itemKey = ItemKeyUnknown
		}
		// Timestamp frames whose text fails verification are held back
		// (Rust retains the TimestampFrame in the remainder).
		if itemKey.IsTimestamp() {
			if _, err := TimestampParseString(text); err != nil {
				var body bytes.Buffer
				body.WriteByte(tf.Encoding.Key)
				body.WriteString(text)
				item := NewTagItem(ItemKeyUnknown, ItemValueBinary(body.Bytes()))
				item.Description = id
				tag.PushUnchecked(item)
				return
			}
		}
		for _, p := range parts {
			item := NewTagItem(itemKey, ItemValueText(p))
			if itemKey == ItemKeyUnknown {
				item.Description = id
			}
			tag.PushUnchecked(item)
		}
	}

	for id, frames := range btag.AllFrames() {
		for _, f := range frames {
			switch fr := f.(type) {
			case bogem.TextFrame:
				addText(id, fr)
			case id3v2TextFrameNoTerm:
				addText(id, bogem.TextFrame{Encoding: fr.enc, Text: fr.text})
			case bogem.UserDefinedTextFrame:
				// TXXX frames with an empty description are held back (Rust
				// requires a non-empty description to consume a TXXX).
				if fr.Description == "" {
					body := append([]byte{fr.Encoding.Key, 0}, []byte(fr.Value)...)
					item := NewTagItem(ItemKeyUnknown, ItemValueBinary(body))
					item.Description = "TXXX"
					tag.PushUnchecked(item)
					continue
				}
				itemKey, ok := ItemKeyFromKey(TagTypeID3v2, fr.Description)
				if !ok {
					itemKey = ItemKeyUnknown
				}
				for _, p := range splitNulls(stripBomForEncoding(fr.Value, fr.Encoding.Key)) {
					item := NewTagItem(itemKey, ItemValueText(p))
					if itemKey == ItemKeyUnknown {
						item.Description = fr.Description
					}
					tag.PushUnchecked(item)
				}
			case bogem.CommentFrame:
				for _, c := range splitNulls(stripBomForEncoding(fr.Text, fr.Encoding.Key)) {
					item := NewTagItem(ItemKeyComment, ItemValueText(c))
					item.Description = fr.Description
					if len(fr.Language) == 3 {
						copy(item.Lang[:], fr.Language)
					}
					tag.PushUnchecked(item)
				}
			case bogem.UnsynchronisedLyricsFrame:
				for _, c := range splitNulls(stripBomForEncoding(fr.Lyrics, fr.Encoding.Key)) {
					item := NewTagItem(ItemKeyUnsyncLyrics, ItemValueText(c))
					item.Description = fr.ContentDescriptor
					if len(fr.Language) == 3 {
						copy(item.Lang[:], fr.Language)
					}
					tag.PushUnchecked(item)
				}
			case bogem.PictureFrame:
				if !parseOptions.ReadCoverArt {
					continue
				}
				pic := PictureUnchecked(fr.Picture).
					PicType(PictureType(fr.PictureType)).
					MimeType(MimeTypeFromStr(fr.MimeType)).
					Description(fr.Description).
					Build()
				tag.PushPicture(pic)
			case bogem.PopularimeterFrame:
				counter := uint64(0)
				if fr.Counter != nil {
					counter = fr.Counter.Uint64()
				}
				if p, ok := popularimeterMapped(fr.Email, TagTypeID3v2, fr.Rating, counter); ok {
					tag.PushUnchecked(NewTagItem(ItemKeyPopularimeter, ItemValueText(p.String())))
				}
			case bogem.UFIDFrame:
				if fr.OwnerIdentifier == id3v2MusicBrainzUfidOwner {
					tag.PushUnchecked(NewTagItem(ItemKeyMusicBrainzRecordingId, ItemValueText(string(fr.Identifier))))
				} else {
					item := NewTagItem(ItemKeyUnknown, ItemValueBinary(fr.Identifier))
					item.Description = "UFID:" + fr.OwnerIdentifier
					tag.PushUnchecked(item)
				}
			case bogem.UnknownFrame:
				// IPLS (ID3v2.3 involved-people list) is not a text frame to bogem.
				if id == "IPLS" && len(fr.Body) > 0 {
					parts := splitNulls(id3v2DecodeTextBody(fr.Body))
					for i := 0; i+1 < len(parts); i += 2 {
						if ik, ok := tiplKeyRoles[parts[i]]; ok {
							tag.PushUnchecked(NewTagItem(ik, ItemValueText(parts[i+1])))
						}
					}
					continue
				}
				// Bogem doesn't know every textual frame (TCMP, PCST, TS*, ...).
				// All 'T'-prefixed frames decode as text (Rust treats them as
				// TextInformationFrames).
				if len(id) == 4 && id[0] == 'T' && len(fr.Body) > 0 {
					addText(id, bogem.TextFrame{
						Encoding: bogem.Encoding{Key: fr.Body[0]},
						Text:     id3v2DecodeTextBody(fr.Body),
					})
					continue
				}
				// URL frames have a 'W' prefix; WXXX has an encoding byte.
				if len(id) == 4 && id[0] == 'W' {
					handleUrlFrame(tag, id, fr.Body)
				} else {
					item := NewTagItem(ItemKeyUnknown, ItemValueBinary(fr.Body))
					item.Description = id
					tag.PushUnchecked(item)
				}
			case popmFramer:
				if p, ok := popularimeterMapped(fr.email, TagTypeID3v2, fr.rating, fr.counter); ok {
					tag.PushUnchecked(NewTagItem(ItemKeyPopularimeter, ItemValueText(p.String())))
				}
			case urlFramer:
				ik, ok := ItemKeyFromKey(TagTypeID3v2, id)
				if !ok {
					ik = ItemKeyUnknown
				}
				item := NewTagItem(ik, ItemValueLocator(fr.url))
				if ik == ItemKeyUnknown {
					item.Description = id
				}
				tag.PushUnchecked(item)
			case wxxxFramer:
				ik, ok := ItemKeyFromKey(TagTypeID3v2, "WXXX:"+fr.desc)
				if !ok {
					ik = ItemKeyUnknown
				}
				item := NewTagItem(ik, ItemValueLocator(fr.url))
				item.Description = fr.desc
				tag.PushUnchecked(item)
			case bogem.ChapterFrame:
				// Chapters are preserved as binary items for round-tripping.
				var buf bytes.Buffer
				if _, err := fr.WriteTo(&buf); err == nil {
					item := NewTagItem(ItemKeyUnknown, ItemValueBinary(buf.Bytes()))
					item.Description = id
					tag.PushUnchecked(item)
				}
			}
		}
	}
	return tag
}

// handleUrlFrame decodes W* (and WXXX) frames from a raw body.
func handleUrlFrame(tag *Tag, id string, body []byte) {
	if id == "WXXX" {
		if len(body) == 0 {
			return
		}
		enc := TextEncoding(body[0])
		rest := body[1:]
		// description is encoding-terminated; URL is latin-1 terminated
		descLen := 0
		var desc string
		if enc == TextEncodingUTF16 || enc == TextEncodingUTF16BE {
			for descLen+1 < len(rest) && !(rest[descLen] == 0 && rest[descLen+1] == 0) {
				descLen += 2
			}
			d, _ := utf16DecodeBytes(rest[:descLen], byteOrderForEncoding(enc))
			desc = d
			descLen += 2
		} else {
			for descLen < len(rest) && rest[descLen] != 0 {
				descLen++
			}
			desc = string(rest[:descLen])
			descLen++
		}
		if descLen > len(rest) {
			return
		}
		url := latin1Decode(bytes.TrimRight(rest[descLen:], "\x00"))
		item := NewTagItem(ItemKeyUnknown, ItemValueLocator(url))
		item.Description = desc
		tag.PushUnchecked(item)
		return
	}
	itemKey, ok := ItemKeyFromKey(TagTypeID3v2, id)
	if !ok {
		itemKey = ItemKeyUnknown
	}
	url := latin1Decode(bytes.TrimRight(body, "\x00"))
	item := NewTagItem(itemKey, ItemValueLocator(url))
	if itemKey == ItemKeyUnknown {
		item.Description = id
	}
	tag.PushUnchecked(item)
}

// stripBomForEncoding removes UTF-16 BOM runes (U+FEFF/U+FFFE) from text
// decoded by bogem when the source encoding is UTF-16 or UTF-16BE — matching
// Rust's utf16_decode_bytes which filters BOM word pairs at every offset.
func stripBomForEncoding(s string, encKey byte) string {
	if encKey == 1 || encKey == 2 {
		return stripUtf16BomRunes(s)
	}
	return s
}

func stripUtf16BomRunes(s string) string {
	if !strings.ContainsAny(s, "\uFEFF\uFFFE") {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\uFEFF' || r == '\uFFFE' {
			return -1
		}
		return r
	}, s)
}

func splitNulls(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// byteOrderForEncoding returns the byte order for a UTF-16 encoding.
func byteOrderForEncoding(enc TextEncoding) byteOrder {
	if enc == TextEncodingUTF16BE {
		return bigEndian
	}
	return littleEndian
}

// id3v23DiscardFrames are the ID3v2.4-only frames dropped when saving as ID3v2.3.
var id3v23DiscardFrames = map[string]bool{
	"ASPI": true, "EQU2": true, "RVA2": true, "SEEK": true, "SIGN": true,
	"TDEN": true, "TDRL": true, "TDTG": true, "TMOO": true, "TPRO": true,
	"TSOA": true, "TSOP": true, "TSOT": true, "TSST": true,
}

// parseId3Genre translates a single genre component (Rust `parse_genre`).
func parseId3Genre(genre string, preserveIndexes bool) string {
	if len(genre) > 3 {
		return genre
	}
	if id, err := atoi(genre); err == nil && id >= 0 {
		if id < len(id3v1Genres) && !preserveIndexes {
			return id3v1Genres[id]
		}
		return genre
	}
	switch genre {
	case "RX":
		return "Remix"
	case "CR":
		return "Cover"
	}
	return genre
}

// id3v2GenresIter splits a TCON value on null separators and "(N)" references
// (Rust `GenresIter`).
func id3v2GenresIter(value string, preserveIndexes bool) []string {
	var out []string
	pos := 0
	for pos < len(value) {
		remainder := value[pos:]
		if idx := strings.IndexByte(remainder, 0); idx >= 0 {
			out = append(out, parseId3Genre(value[pos:pos+idx], preserveIndexes))
			pos += idx + 1
			continue
		}
		if strings.HasPrefix(remainder, "(") && strings.Contains(remainder, ")") {
			start := pos + 1
			end := pos + strings.Index(remainder, ")")
			pos = end + 1
			// handle bracketed refinement e.g. "(55)((I think...)"
			if strings.HasPrefix(remainder, "((") {
				end++
			}
			out = append(out, parseId3Genre(value[start:end], preserveIndexes))
			continue
		}
		pos = len(value)
		out = append(out, parseId3Genre(remainder, preserveIndexes))
	}
	return out
}

// isU8Num reports whether s parses as a u8.
func isU8Num(s string) bool {
	n, err := atoi(s)
	return err == nil && n >= 0 && n <= 255
}

// id3v23GenreValue wraps TCON components in parentheses for ID3v2.3, which
// cannot use null separators (Rust `create_items_v3` TCON handling).
func id3v23GenreValue(value string) string {
	genres := id3v2GenresIter(value, true)
	var b strings.Builder
	for i, genre := range genres {
		switch {
		case genre == "Remix":
			b.WriteString("(RX)")
		case genre == "Cover":
			b.WriteString("(CR)")
		case i == len(genres)-1 && !isU8Num(genre):
			b.WriteString(genre)
		default:
			b.WriteString("(" + genre + ")")
		}
	}
	return b.String()
}

// genericTagToBogem converts a generic [Tag] to a bogem tag.
func genericTagToBogem(tag *Tag, useV23 bool) (*bogem.Tag, error) {
	btag := bogem.NewEmptyTag()
	if useV23 {
		btag.SetVersion(3)
	} else {
		btag.SetVersion(4)
	}
	utf8Enc := bogem.EncodingUTF8
	if useV23 {
		utf8Enc = bogem.EncodingUTF16
	}

	// Collect TIPL pair items.
	var tiplPairs []string
	tiplKeys := map[ItemKey]bool{
		ItemKeyProducer: true, ItemKeyArranger: true, ItemKeyEngineer: true,
		ItemKeyMixDj: true, ItemKeyMixEngineer: true,
	}
	var consumedTiplItems []*TagItem

	addTextFrame := func(id string, text string) {
		if id == "TIPL" || id == "IPLS" || id == "TMCL" {
			return // handled via tiplPairs
		}
		btag.AddFrame(id, id3v2TextFrameNoTerm{enc: utf8Enc, text: text})
	}

	// Rust `from_tag` merge accumulators (GenericConversionContext):
	// multi-valued TXXX frames merged with '\0' by description; single-valued
	// TXXX (ReplayGain) last-wins; COMM/USLT merged by (lang, desc); generic
	// text frames merged with '\0' by frame ID; single-valued frames
	// deduplicated by ID keeping the first.
	txxxMerge := map[string]string{}
	var txxxOrder []string
	txxxLast := map[string]string{}
	var txxxLastOrder []string
	type langDescKey struct {
		lyrics     bool
		lang, desc string
	}
	langMerge := map[langDescKey]string{}
	var langOrder []langDescKey
	textMerge := map[string]string{}
	var textOrder []string
	seenFrame := map[string]bool{}
	seenPopm := map[string]bool{}
	seenUfid := map[string]bool{}
	seenUnknownTxxx := map[string]bool{}

	addTxxxMerge := func(desc, value string) {
		if _, ok := txxxMerge[desc]; !ok {
			txxxOrder = append(txxxOrder, desc)
			txxxMerge[desc] = value
		} else {
			txxxMerge[desc] += "\x00" + value
		}
	}
	addTextMerge := func(id, value string) {
		if _, ok := textMerge[id]; !ok {
			textOrder = append(textOrder, id)
			textMerge[id] = value
		} else {
			textMerge[id] += "\x00" + value
		}
	}

	consumed := map[int]bool{}
	for i := range tag.items {
		if consumed[i] {
			continue
		}
		item := &tag.items[i]
		if tiplKeys[item.key] {
			consumedTiplItems = append(consumedTiplItems, item)
			continue
		}
		if item.value.kind == itemValueBinary {
			// Unknown/binary frames (incl. UFID).
			switch {
			case useV23 && id3v23DiscardFrames[item.Description]:
				// ID3v2.4-only frame — dropped when saving as ID3v2.3
			case len(item.Description) >= 5 && item.Description[:5] == "UFID:":
				owner := item.Description[5:]
				if seenUfid[owner] {
					break
				}
				seenUfid[owner] = true
				btag.AddUFIDFrame(bogem.UFIDFrame{
					OwnerIdentifier: owner,
					Identifier:      item.value.binary,
				})
			case len(item.Description) == 4:
				btag.AddFrame(item.Description, bogem.UnknownFrame{Body: item.value.binary})
			default:
				// Unmappable binary item — drop
			}
			continue
		}
		text, _ := item.value.IntoString()

		// Multi-valued TXXX key-to-frame mappings — merge with '\0'
		if id3v2MultiTxxxKeys[item.key] {
			if desc, ok := item.key.MapKey(TagTypeID3v2); ok {
				addTxxxMerge(desc, text)
			}
			continue
		}
		// Single-valued TXXX mappings (ReplayGain) — last wins
		if id3v2SingleTxxxKeys[item.key] {
			if desc, ok := item.key.MapKey(TagTypeID3v2); ok {
				if _, dup := txxxLast[desc]; !dup {
					txxxLastOrder = append(txxxLastOrder, desc)
				}
				txxxLast[desc] = text
			}
			continue
		}
		// Comment/Unsync lyrics — merged by (lang, description)
		if item.key == ItemKeyComment || item.key == ItemKeyUnsyncLyrics {
			ldk := langDescKey{item.key == ItemKeyUnsyncLyrics, langOrXxx(item.Lang), item.Description}
			if _, ok := langMerge[ldk]; !ok {
				langOrder = append(langOrder, ldk)
				langMerge[ldk] = text
			} else {
				langMerge[ldk] += "\x00" + text
			}
			continue
		}
		// Flag items — normalized to "0"/"1", deduplicated by frame ID
		if item.key.IsFlag() {
			if flag := flagItem(text); flag >= 0 {
				if id, ok := item.key.MapKey(TagTypeID3v2); ok && !seenFrame[id] {
					seenFrame[id] = true
					btag.AddTextFrame(id, utf8Enc, itoa(flag))
				}
			}
			continue
		}
		// iTunes advisory rating — validated u8 → AdvisoryRating normalization
		if item.key == ItemKeyParentalAdvisory {
			if n, err := strconv.ParseUint(text, 10, 8); err == nil {
				if rating, ok := advisoryRatingFromU8(uint8(n)); ok && !seenFrame["ITUNESADVISORY"] {
					seenFrame["ITUNESADVISORY"] = true
					btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
						Encoding:    utf8Enc,
						Description: "ITUNESADVISORY",
						Value:       itoa(int(rating)),
					})
				}
			}
			continue
		}
		if item.key == ItemKeyPopularimeter {
			p, err := PopularimeterFromStr(text)
			if err != nil {
				continue
			}
			email := p.Email()
			if seenPopm[email] {
				continue
			}
			seenPopm[email] = true
			btag.AddFrame("POPM", popmFramer{
				email:   email,
				rating:  p.MappedValue(TagTypeID3v2),
				counter: p.PlayCounter,
			})
			continue
		}
		if item.key == ItemKeyMusicBrainzRecordingId {
			if !isASCII(text) || seenUfid[id3v2MusicBrainzUfidOwner] {
				continue
			}
			seenUfid[id3v2MusicBrainzUfidOwner] = true
			btag.AddUFIDFrame(bogem.UFIDFrame{
				OwnerIdentifier: id3v2MusicBrainzUfidOwner,
				Identifier:      []byte(text),
			})
			continue
		}
		if item.key == ItemKeyUnknown {
			if item.value.kind == itemValueLocator {
				btag.AddFrame("WXXX", wxxxFramer{desc: item.Description, url: text})
			} else if len(item.Description) > 0 && !seenUnknownTxxx[item.Description] {
				// Unknown TXXX-style item: write back with its description
				seenUnknownTxxx[item.Description] = true
				btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
					Encoding:    utf8Enc,
					Description: item.Description,
					Value:       text,
				})
			}
			continue
		}

		// Number/total pairs are merged into "N/T" frames below
		if _, ok := id3v2PairKeys[item.key]; ok {
			continue
		}

		frameID, ok := item.key.MapKey(TagTypeID3v2)
		if !ok {
			continue
		}
		// Keys whose id3v2 representation is a TXXX description rather than a
		// real frame ID
		if len(frameID) != 4 || frameID == "WORK" {
			if !seenUnknownTxxx[frameID] {
				seenUnknownTxxx[frameID] = true
				btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
					Encoding:    utf8Enc,
					Description: frameID,
					Value:       text,
				})
			}
			continue
		}
		// URL frames (W*) — deduplicated by frame ID, first wins
		if frameID[0] == 'W' && frameID != "WXXX" {
			if !seenFrame[frameID] {
				seenFrame[frameID] = true
				btag.AddFrame(frameID, urlFramer{url: text})
			}
			continue
		}
		// Timestamp keys — written per item (Rust emits distinct TimestampFrames)
		if item.key.IsTimestamp() {
			if useV23 {
				switch frameID {
				case "TDOR":
					if ts, err := TimestampParseString(text); err == nil {
						btag.AddTextFrame("TORY", utf8Enc, fmt.Sprintf("%04d", ts.Year))
					}
					continue
				case "TDRC":
					if ts, err := TimestampParseString(text); err == nil {
						btag.AddTextFrame("TYER", utf8Enc, itoa(int(ts.Year)))
						if m, ok := ts.Month.Get(); ok {
							if d, ok := ts.Day.Get(); ok {
								btag.AddTextFrame("TDAT", utf8Enc, fmt.Sprintf("%02d%02d", d, m))
							}
						}
						if h, ok := ts.Hour.Get(); ok {
							if mi, ok := ts.Minute.Get(); ok {
								btag.AddTextFrame("TIME", utf8Enc, fmt.Sprintf("%02d%02d", h, mi))
							}
						}
					}
					continue
				}
				if id3v23DiscardFrames[frameID] {
					continue
				}
			}
			addTextFrame(frameID, text)
			continue
		}
		// Single-valued text frame (IntegerBpm → TBPM): deduplicated, first wins
		if id3v2SingleTextKeys[item.key] {
			if !seenFrame[frameID] {
				seenFrame[frameID] = true
				btag.AddTextFrame(frameID, utf8Enc, text)
			}
			continue
		}
		// Multi-valued text frames — merge values with '\0' by frame ID
		strs := tag.GetStrings(item.key)
		if item.value.kind == itemValueLocator {
			strs = tag.GetLocators(item.key)
		}
		joined := joinNulls(strs)
		if useV23 {
			switch {
			case id3v23DiscardFrames[frameID]:
			case frameID == "TCON":
				// ID3v2.3 cannot use null separators — wrap refs in parens
				btag.AddTextFrame("TCON", utf8Enc, id3v23GenreValue(joined))
			default:
				btag.AddTextFrame(frameID, utf8Enc, joined)
			}
		} else {
			addTextMerge(frameID, joined)
		}
		// remaining duplicates of this key were merged into `joined`; mark consumed
		for j := range tag.items {
			if j > i && tag.items[j].key == item.key {
				consumed[j] = true
			}
		}
	}

	// Emit merged frames
	for _, id := range textOrder {
		addTextFrame(id, textMerge[id])
	}
	for _, desc := range txxxOrder {
		btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
			Encoding:    utf8Enc,
			Description: desc,
			Value:       txxxMerge[desc],
		})
	}
	for _, desc := range txxxLastOrder {
		btag.AddUserDefinedTextFrame(bogem.UserDefinedTextFrame{
			Encoding:    utf8Enc,
			Description: desc,
			Value:       txxxLast[desc],
		})
	}
	for _, ldk := range langOrder {
		if ldk.lyrics {
			btag.AddUnsynchronisedLyricsFrame(bogem.UnsynchronisedLyricsFrame{
				Encoding:          utf8Enc,
				Language:          ldk.lang,
				ContentDescriptor: ldk.desc,
				Lyrics:            langMerge[ldk],
			})
		} else {
			btag.AddCommentFrame(bogem.CommentFrame{
				Encoding:    utf8Enc,
				Language:    ldk.lang,
				Description: ldk.desc,
				Text:        langMerge[ldk],
			})
		}
	}

	// Pair frames: "num/total" in TRCK/TPOS
	for _, p := range []struct {
		frameID    string
		numK, totK ItemKey
	}{
		{"TRCK", ItemKeyTrackNumber, ItemKeyTrackTotal},
		{"TPOS", ItemKeyDiscNumber, ItemKeyDiscTotal},
	} {
		num, hasN := tag.GetString(p.numK)
		tot, hasT := tag.GetString(p.totK)
		if !hasN && !hasT {
			continue
		}
		var val string
		switch {
		case hasN && hasT:
			val = num + "/" + tot
		case hasT:
			val = "0/" + tot
		default:
			val = num
		}
		addTextFrame(p.frameID, val)
	}

	// TIPL: role/name pairs
	for _, item := range consumedTiplItems {
		if role, ok := tiplItemKeyRoles[item.key]; ok {
			if s, sok := item.value.IntoString(); sok {
				tiplPairs = append(tiplPairs, role, s)
			}
		}
	}
	if len(tiplPairs) > 0 {
		// ID3v2.3 merges the involved-people and musician-credits lists into IPLS
		id := "TIPL"
		if useV23 {
			id = "IPLS"
		}
		btag.AddFrame(id, id3v2TextFrameNoTerm{enc: utf8Enc, text: joinNulls(tiplPairs)})
	}

	for _, pic := range tag.pictures {
		mime := "application/octet-stream"
		if mt, ok := pic.mimeType.Get(); ok {
			mime = mt.AsStr()
		}
		desc, _ := pic.description.Get()
		btag.AddAttachedPicture(bogem.PictureFrame{
			Encoding:    utf8Enc,
			MimeType:    mime,
			PictureType: pic.picType.AsU8(),
			Description: desc,
			Picture:     pic.data,
		})
	}

	return btag, nil
}

// langOrXxx returns the 3-char language or "XXX".
func langOrXxx(l Lang) string {
	if l == LangUnknown || l == ([3]byte{}) {
		return "XXX"
	}
	return string(l[:])
}

func joinNulls(parts []string) string {
	var b bytes.Buffer
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(p)
	}
	return b.String()
}

// popmFramer writes a POPM frame body.
type popmFramer struct {
	email   string
	rating  uint8
	counter uint64
}

func (p popmFramer) Size() int {
	counterLen := 0
	for c := p.counter; c > 0; c >>= 8 {
		counterLen++
	}
	return len(p.email) + 1 + 1 + counterLen
}

func (p popmFramer) UniqueIdentifier() string { return p.email }

func (p popmFramer) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.WriteString(p.email)
	buf.WriteByte(0)
	buf.WriteByte(p.rating)
	var cb [8]byte
	binary.BigEndian.PutUint64(cb[:], p.counter)
	i := 0
	for i < 8 && cb[i] == 0 {
		i++
	}
	buf.Write(cb[i:])
	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

// urlFramer writes a W* frame body (raw latin-1 URL).
type urlFramer struct{ url string }

func (u urlFramer) Size() int                { return len(u.url) }
func (u urlFramer) UniqueIdentifier() string { return "" }
func (u urlFramer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(latin1EncodeMust(u.url))
	return int64(n), err
}

// wxxxFramer writes a WXXX frame body.
type wxxxFramer struct {
	desc string
	url  string
}

func (x wxxxFramer) Size() int {
	return 1 + len(latin1EncodeMust(x.desc)) + 1 + len(latin1EncodeMust(x.url))
}
func (x wxxxFramer) UniqueIdentifier() string { return x.desc }
func (x wxxxFramer) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.WriteByte(0) // latin-1
	buf.Write(latin1EncodeMust(x.desc))
	buf.WriteByte(0)
	buf.Write(latin1EncodeMust(x.url))
	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

// --- read/write at the file level ---

// parseId3v2 reads the tag content (after the header was consumed).
func parseId3v2(reader io.Reader, header Id3v2Header, parseOptions ParseOptions) (*Tag, error) {
	if header.Version == 2 {
		return parseId3v22(reader, header, parseOptions)
	}
	// Re-feed the header back through bogem: read the remaining `size` bytes.
	// The declared tag size may exceed the remaining file — LimitReader
	// tolerates less, and grows the buffer incrementally rather than
	// trusting the declared size (the allocation limit applies to individual
	// items, not the tag body — matching Rust's take()+read_to_end).
	content, err := io.ReadAll(io.LimitReader(reader, int64(header.Size)))
	if err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	// Whole-tag unsynchronisation must be removed before parsing (bogem does
	// not handle it).
	if header.Flags.Unsynchronisation {
		content = id3v2Deunsync(content)
	}
	// Rust's BestAttempt parser stops at the first malformed frame and keeps
	// what it has. Truncate trailing frames that overflow the tag so a strict
	// parse (bogem) doesn't fail the whole tag.
	content, err = id3v2TruncateBadFrames(content, header)
	if err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	// Decompress zlib-compressed frames — bogem requires a correct declared
	// decompressed size, while Rust inflates regardless of it.
	if header.Version == 3 {
		content = id3v2DecompressFrames(content)
	}
	// bogem expects a stream starting at "ID3"; reconstruct it.
	full := make([]byte, 0, len(content)+10)
	full = append(full, []byte("ID3")...)
	full = append(full, header.Version, 0)
	var flagByte byte
	// Unsynchronisation was already removed above — do not set the flag.
	if header.Flags.ExtendedHeader {
		flagByte |= 0x40
	}
	if header.Flags.Experimental {
		flagByte |= 0x20
	}
	if header.Version == 4 && header.Flags.Footer {
		flagByte |= 0x10
	}
	full = append(full, flagByte)
	ss := synchsafe32(uint32(len(content)))
	full = append(full, ss[:]...)
	full = append(full, content...)

	btag, err := bogem.ParseReader(bytes.NewReader(full), bogem.Options{Parse: true})
	if err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	tag := id3v2TagToGeneric(btag, parseOptions)
	tag.OriginalVersion = header.Version
	if header.Version == 3 && parseOptions.ImplicitConversions {
		upgradeV3Items(tag)
	}
	return tag, nil
}

// takeByDesc removes and returns the unknown items with the given frame id.
func takeByDesc(tag *Tag, id string) []TagItem {
	return tag.TakeFilter(ItemKeyUnknown, func(it *TagItem) bool {
		return it.Description == id
	})
}

func unknownText(item TagItem) string {
	s, _ := item.Value().IntoString()
	return s
}

// upgradeV3Items handles ID3v2.3 -> 2.4 implicit conversions
// (Rust `upgrade_v3` + `construct_tdrc_from_v3`).
func upgradeV3Items(tag *Tag) {
	// TORY -> TDOR (year only, timestamp-formatted)
	for _, item := range takeByDesc(tag, "TORY") {
		if ts, err := TimestampParseString(unknownText(item)); err == nil {
			tag.PushUnchecked(NewTagItem(ItemKeyOriginalReleaseDate,
				ItemValueText(fmt.Sprintf("%04d", ts.Year))))
			continue
		}
		tag.PushUnchecked(item)
	}

	// TYER + TDAT + TIME -> TDRC
	tyers := takeByDesc(tag, "TYER")
	if len(tyers) == 0 {
		return
	}
	text := unknownText(tyers[0])
	ts, err := TimestampParseString(text)
	if err != nil || ts.Month.IsSome() {
		// Not a timestamp, or a real TDRC (already has a month) — retain as-is
		tag.PushUnchecked(NewTagItem(ItemKeyRecordingDate, ItemValueText(text)))
		return
	}
	tdrc := Timestamp{Year: ts.Year}
	dateUsed, timeUsed := false, false
	tdats := takeByDesc(tag, "TDAT")
	times := takeByDesc(tag, "TIME")
	if len(tdats) > 0 {
		date := unknownText(tdats[0])
		if day, month, ok := parseDDMM(date); ok {
			tdrc.Day = Some(day)
			tdrc.Month = Some(month)
			dateUsed = true
			if len(times) > 0 {
				if hour, minute, ok := parseDDMM(unknownText(times[0])); ok {
					tdrc.Hour = Some(hour)
					tdrc.Minute = Some(minute)
					timeUsed = true
				}
			}
		}
	}
	tag.PushUnchecked(NewTagItem(ItemKeyRecordingDate, ItemValueText(tdrc.String())))
	if !dateUsed {
		for _, item := range tdats {
			tag.PushUnchecked(item)
		}
	}
	if !timeUsed {
		for _, item := range times {
			tag.PushUnchecked(item)
		}
	}
}

// parseDDMM parses a 4-ASCII-digit "DDMM"/"HHMM" string into two numbers.
func parseDDMM(s string) (uint8, uint8, bool) {
	if len(s) != 4 {
		return 0, 0, false
	}
	for i := 0; i < 4; i++ {
		if s[i] > 127 {
			return 0, 0, false
		}
	}
	a, err1 := atoi(s[:2])
	b, err2 := atoi(s[2:])
	if err1 != nil || err2 != nil || a < 0 || b < 0 || a > 255 || b > 255 {
		return 0, 0, false
	}
	return uint8(a), uint8(b), true
}

// writeId3v2 writes an ID3v2 tag at the start of a file.
func writeId3v2(file FileLike, tag *Tag, writeOptions WriteOptions) error {
	var encoded []byte
	if !tag.IsEmpty() {
		btag, err := genericTagToBogem(tag, writeOptions.UseID3v23)
		if err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		var buf bytes.Buffer
		if _, err := btag.WriteTo(&buf); err != nil {
			return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v2, HasTagType: true, Source: err}}
		}
		encoded = buf.Bytes()
	}

	// Find the existing tag range
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	header, rangeEnd, found, err := findId3v2(file, false, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	var start uint64
	var end uint64
	if found {
		end = uint64(rangeEnd)
		_ = header
	}
	return Splice(file, start, end, encoded)
}

// findId3v2 finds an ID3v2 tag at the current position (possibly preceded by junk).
// Returns the header (if found), the end position of the tag, whether found, and an error.
// On success the reader is positioned at the end of the tag; on failure at the original position.
func findId3v2(reader io.ReadSeeker, read bool, parseOptions ParseOptions) (Id3v2Header, int64, bool, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return Id3v2Header{}, 0, false, err
	}

	var header Id3v2Header
	search := io.LimitReader(reader, int64(parseOptions.MaxJunkBytes))
	offset, found := findId3v2InJunk(search)
	if !found {
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return Id3v2Header{}, 0, false, err
		}
		return Id3v2Header{}, 0, false, nil
	}
	_ = offset

	// Seek back to start of "ID3"
	if _, err := reader.Seek(-3, io.SeekCurrent); err != nil {
		return Id3v2Header{}, 0, false, err
	}

	header, err = parseId3v2Header(reader)
	if err != nil {
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return Id3v2Header{}, 0, false, err
		}
		return Id3v2Header{}, 0, false, nil
	}

	if read {
		content, err := tryByteSlice(int(header.Size))
		if err != nil {
			return header, 0, false, &Id3v2ParseError{Source: err}
		}
		if _, err := io.ReadFull(reader, content); err != nil {
			return header, 0, false, err
		}
	} else {
		if _, err := reader.Seek(int64(header.Size), io.SeekCurrent); err != nil {
			return header, 0, false, err
		}
	}
	if header.Flags.Footer {
		if _, err := reader.Seek(10, io.SeekCurrent); err != nil {
			return header, 0, false, err
		}
	}
	end, err := reader.Seek(0, io.SeekCurrent)
	return header, end, true, err
}

func findId3v2InJunk(reader io.Reader) (int64, bool) {
	var window [3]byte
	idx := int64(0)
	var oneByte [1]byte
	for {
		_, err := reader.Read(oneByte[:])
		if err != nil {
			return 0, false
		}
		window[0] = window[1]
		window[1] = window[2]
		window[2] = oneByte[0]
		idx++
		if bytes.Equal(window[:], []byte("ID3")) {
			return idx - 3, true
		}
	}
}

func init() {
	tagDumpers[TagTypeID3v2] = func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		if tag.IsEmpty() {
			return nil
		}
		btag, err := genericTagToBogem(tag, writeOptions.UseID3v23)
		if err != nil {
			return err
		}
		_, err = btag.WriteTo(writer)
		return err
	}
}

// splitNumTotal splits an "N/T" pair string into its components.
func splitNumTotal(s string) (num, total string) {
	num = s
	if i := strings.IndexByte(s, '/'); i >= 0 {
		num = s[:i]
		total = s[i+1:]
	}
	return num, total
}

// splitNumPair splits an ID3v2 pair value on the first NUL or '/',
// trims each part, and requires all present parts to parse as u32 —
// matching Rust's `split_pair`. Returns ok=false when unconvertible.
func splitNumPair(s string) (num, total string, ok bool) {
	numPart := s
	totPart := ""
	hasTot := false
	if i := strings.IndexAny(s, "\x00/"); i >= 0 {
		numPart, totPart, hasTot = s[:i], s[i+1:], true
	}
	parse := func(p string) (string, bool) {
		p = strings.TrimSpace(p)
		if p == "" {
			return "", false
		}
		if _, err := strconv.ParseUint(p, 10, 32); err != nil {
			return "", false
		}
		return p, true
	}
	var nok bool
	if num, nok = parse(numPart); !nok {
		return "", "", false
	}
	if hasTot {
		if total, nok = parse(totPart); !nok {
			return "", "", false
		}
	}
	return num, total, true
}

// id3v2PairKeys are stored as "N/T" inside a single frame.
var id3v2PairKeys = map[ItemKey]bool{
	ItemKeyTrackNumber: true, ItemKeyTrackTotal: true,
	ItemKeyDiscNumber: true, ItemKeyDiscTotal: true,
}

// id3v2MultiTxxxKeys merge multiple items into a single TXXX frame,
// NUL-separated (Rust `from_tag` multi-valued TXXX arm).
var id3v2MultiTxxxKeys = map[ItemKey]bool{
	ItemKeyTrackArtists: true, ItemKeyAlbumArtists: true,
	ItemKeyDirector: true, ItemKeyAcoustId: true,
	ItemKeyAcoustIdFingerprint: true, ItemKeyCatalogNumber: true,
	ItemKeyMusicBrainzArtistId: true, ItemKeyMusicBrainzReleaseArtistId: true,
	ItemKeyMusicBrainzWorkId: true, ItemKeyMusicBrainzReleaseType: true,
	ItemKeyMusicBrainzTrackId: true, ItemKeyMusicBrainzReleaseId: true,
	ItemKeyMusicBrainzReleaseGroupId: true, ItemKeyReleaseCountry: true,
	ItemKeyBarcode: true, ItemKeyColor: true, ItemKeyWork: true,
}

// id3v2SingleTxxxKeys write a single TXXX frame; duplicates are
// deduplicated keeping the last value (Rust HashMap insert semantics).
var id3v2SingleTxxxKeys = map[ItemKey]bool{
	ItemKeyReplayGainAlbumGain: true, ItemKeyReplayGainAlbumPeak: true,
	ItemKeyReplayGainTrackGain: true, ItemKeyReplayGainTrackPeak: true,
}

// id3v2SingleTextKeys write single-valued text frames; duplicates are
// deduplicated by frame ID keeping the first value.
var id3v2SingleTextKeys = map[ItemKey]bool{
	ItemKeyIntegerBpm: true,
}

// isASCII reports whether s contains only ASCII bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7F {
			return false
		}
	}
	return true
}

// --- ID3v2.2 support ---

// id3v2Deunsync removes 0x00 bytes following 0xFF (unsynchronisation scheme).
func id3v2Deunsync(data []byte) []byte {
	out := data[:0]
	for i := 0; i < len(data); i++ {
		out = append(out, data[i])
		if data[i] == 0xFF && i+1 < len(data) && data[i+1] == 0x00 {
			i++
		}
	}
	return out
}

// v2ToV4FrameIds maps ID3v2.2 frame ids to their ID3v2.4 equivalents.
var v2ToV4FrameIds = map[string]string{
	"BUF": "RBUF", "CNT": "PCNT", "COM": "COMM", "CRA": "AENC", "ETC": "ETCO",
	"GEO": "GEOB", "IPL": "TIPL", "MCI": "MCDI", "MLL": "MLLT", "PIC": "APIC",
	"POP": "POPM", "REV": "RVRB", "SLT": "SYLT", "STC": "SYTC", "TAL": "TALB",
	"TBP": "TBPM", "TCM": "TCOM", "TCO": "TCON", "TCP": "TCMP", "TCR": "TCOP",
	"TDY": "TDLY", "TEN": "TENC", "TFT": "TFLT", "TKE": "TKEY", "TLA": "TLAN",
	"TLE": "TLEN", "TMT": "TMED", "TOA": "TOPE", "TOF": "TOFN", "TOL": "TOLY",
	"TOR": "TDOR", "TOT": "TOAL", "TP1": "TPE1", "TP2": "TPE2", "TP3": "TPE3",
	"TP4": "TPE4", "TPA": "TPOS", "TPB": "TPUB", "TRC": "TSRC", "TRD": "TDRC",
	"TRK": "TRCK", "TS2": "TSO2", "TSA": "TSOA", "TSC": "TSOC", "TSP": "TSOP",
	"TSS": "TSSE", "TST": "TSOT", "TT1": "TIT1", "TT2": "TIT2", "TT3": "TIT3",
	"TXT": "TEXT", "TXX": "TXXX", "TYE": "TDRC", "UFI": "UFID", "ULT": "USLT",
	"WAF": "WOAF", "WAR": "WOAR", "WAS": "WOAS", "WCM": "WCOM", "WCP": "WCOP",
	"WPB": "WPUB", "WXX": "WXXX",
	// iTunes non-standard frames
	"PCS": "PCST", "TCT": "TCAT", "TDS": "TDES", "TID": "TGID", "WFD": "WFED",
	// Identifiers
	"MVI": "MVIN", "MVN": "MVNM", "GP1": "GRP1", "TDR": "TDRL",
}

// id3v2DecodeTextBody decodes text from a frame body where data[0] is the
// encoding byte (0=latin1, 1=UTF-16 w/ BOM, 2=UTF-16BE, 3=UTF-8).
func id3v2DecodeTextBody(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	switch data[0] {
	case 1:
		d := data[1:]
		order := littleEndian
		if len(d) >= 2 && d[0] == 0xFE && d[1] == 0xFF {
			order = bigEndian
		}
		s, err := utf16DecodeBytes(d, order)
		if err == nil {
			return s
		}
		return latin1Decode(d)
	case 2:
		s, err := utf16DecodeBytes(data[1:], bigEndian)
		if err == nil {
			return s
		}
		return latin1Decode(data[1:])
	case 3:
		return trimEndNulls(string(data[1:]))
	default:
		return latin1Decode(data[1:])
	}
}

// id3v22DecodeText decodes v2.2 text (0=latin1, 1=UTF-16 w/ BOMs stripped).
func id3v22DecodeText(data []byte) string {
	return id3v2DecodeTextBody(data)
}

// id3v22SplitDesc splits a description (encoding-terminated) from the rest.
func id3v22SplitDesc(data []byte, enc byte) (desc string, rest []byte) {
	if enc == 1 {
		for i := 0; i+1 < len(data); i += 2 {
			if data[i] == 0 && data[i+1] == 0 {
				s, _ := utf16DecodeBytes(data[:i], littleEndian)
				return s, data[i+2:]
			}
		}
		s, _ := utf16DecodeBytes(data, littleEndian)
		return s, nil
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return latin1Decode(data[:i]), data[i+1:]
	}
	return latin1Decode(data), nil
}

// parseId3v22 reads an ID3v2.2 tag (3-char ids, 3-byte sizes, no flags).
func parseId3v22(reader io.Reader, header Id3v2Header, parseOptions ParseOptions) (*Tag, error) {
	content, err := tryByteSlice(int(header.Size))
	if err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	if header.Flags.Unsynchronisation {
		content = id3v2Deunsync(content)
	}

	tag := NewTag(TagTypeID3v2)
	tag.OriginalVersion = 2

	pushTimestamp := func(id string, text string) {
		ik, ok := ItemKeyFromKey(TagTypeID3v2, id)
		if !ok {
			ik = ItemKeyUnknown
		}
		if ts, err := TimestampParseString(text); err == nil && ts.Verify() == nil {
			item := NewTagItem(ik, ItemValueText(ts.String()))
			if ik == ItemKeyUnknown {
				item.Description = id
			}
			tag.PushUnchecked(item)
			return
		}
		item := NewTagItem(ik, ItemValueText(text))
		if ik == ItemKeyUnknown {
			item.Description = id
		}
		tag.PushUnchecked(item)
	}

	pushText := func(id string, text string) {
		switch id {
		case "TCON":
			for _, g := range id3v2GenresIter(text, false) {
				tag.PushUnchecked(NewTagItem(ItemKeyGenre, ItemValueText(g)))
			}
		case "TRCK", "TPOS", "MVIN":
			var numKey, totKey ItemKey
			switch id {
			case "TRCK":
				numKey, totKey = ItemKeyTrackNumber, ItemKeyTrackTotal
			case "TPOS":
				numKey, totKey = ItemKeyDiscNumber, ItemKeyDiscTotal
			default:
				numKey, totKey = ItemKeyMovementNumber, ItemKeyMovementTotal
			}
			num, tot := splitNumTotal(text)
			if num != "" {
				tag.PushUnchecked(NewTagItem(numKey, ItemValueText(num)))
			}
			if tot != "" {
				tag.PushUnchecked(NewTagItem(totKey, ItemValueText(tot)))
			}
		case "TIPL", "TMCL":
			parts := splitNulls(text)
			for i := 0; i+1 < len(parts); i += 2 {
				if ik, ok := tiplKeyRoles[parts[i]]; ok {
					tag.PushUnchecked(NewTagItem(ik, ItemValueText(parts[i+1])))
				}
			}
		default:
			ik, ok := ItemKeyFromKey(TagTypeID3v2, id)
			if !ok {
				ik = ItemKeyUnknown
			}
			item := NewTagItem(ik, ItemValueText(text))
			if ik == ItemKeyUnknown {
				item.Description = id
			}
			tag.PushUnchecked(item)
		}
	}

	for pos := 0; pos+6 <= len(content); {
		id := string(content[pos : pos+3])
		if id == "\x00\x00\x00" {
			break
		}
		size := int(content[pos+3])<<16 | int(content[pos+4])<<8 | int(content[pos+5])
		pos += 6
		if size <= 0 || pos+size > len(content) {
			break
		}
		body := content[pos : pos+size]
		pos += size

		v4id, ok := v2ToV4FrameIds[id]
		if !ok {
			v4id = id
		}

		switch v4id {
		case "APIC":
			// v2.2 PIC: enc(1) + format(3) + type(1) + desc + data
			if len(body) < 5 {
				continue
			}
			var mime *MimeType
			switch string(body[1:4]) {
			case "PNG":
				mime = &MimeTypePng
			case "JPG":
				mime = &MimeTypeJpeg
			default:
				continue
			}
			enc := body[0]
			picType := body[4]
			desc, data := id3v22SplitDesc(body[5:], enc)
			if !parseOptions.ReadCoverArt {
				continue
			}
			pb := PictureUnchecked(data).PicType(PictureType(picType)).MimeType(*mime)
			if desc != "" {
				pb = pb.Description(desc)
			}
			tag.PushPicture(pb.Build())
		case "TXXX":
			if len(body) < 1 {
				continue
			}
			desc, value := id3v22SplitDesc(body[1:], body[0])
			ik, ok := ItemKeyFromKey(TagTypeID3v2, desc)
			if !ok {
				ik = ItemKeyUnknown
			}
			item := NewTagItem(ik, ItemValueText(id3v22DecodeText(append([]byte{body[0]}, value...))))
			if ik == ItemKeyUnknown {
				item.Description = desc
			}
			tag.PushUnchecked(item)
		case "COMM", "USLT":
			if len(body) < 4 {
				continue
			}
			desc, text := id3v22SplitDesc(body[4:], body[0])
			ik := ItemKeyComment
			if v4id == "USLT" {
				ik = ItemKeyUnsyncLyrics
			}
			for _, c := range splitNulls(id3v22DecodeText(append([]byte{body[0]}, text...))) {
				item := NewTagItem(ik, ItemValueText(c))
				item.Description = desc
				copy(item.Lang[:], body[1:4])
				tag.PushUnchecked(item)
			}
		case "UFID":
			if i := bytes.IndexByte(body, 0); i >= 0 {
				owner := string(body[:i])
				identifier := body[i+1:]
				if owner == id3v2MusicBrainzUfidOwner {
					tag.PushUnchecked(NewTagItem(ItemKeyMusicBrainzRecordingId, ItemValueText(string(identifier))))
				} else {
					item := NewTagItem(ItemKeyUnknown, ItemValueBinary(identifier))
					item.Description = "UFID:" + owner
					tag.PushUnchecked(item)
				}
			}
		case "POPM":
			if i := bytes.IndexByte(body, 0); i >= 0 && i+1 < len(body) {
				email := string(body[:i])
				rating := body[i+1]
				var counter uint64
				for _, b := range body[i+2:] {
					counter = counter<<8 | uint64(b)
				}
				if p, ok := popularimeterMapped(email, TagTypeID3v2, rating, counter); ok {
					tag.PushUnchecked(NewTagItem(ItemKeyPopularimeter, ItemValueText(p.String())))
				}
			}
		case "WXXX":
			if len(body) < 1 {
				continue
			}
			desc, url := id3v22SplitDesc(body[1:], body[0])
			ik, ok := ItemKeyFromKey(TagTypeID3v2, "WXXX:"+desc)
			if !ok {
				ik = ItemKeyUnknown
			}
			item := NewTagItem(ik, ItemValueLocator(latin1Decode(url)))
			if ik == ItemKeyUnknown {
				item.Description = desc
			}
			tag.PushUnchecked(item)
		default:
			switch {
			case v4id[0] == 'T':
				text := id3v22DecodeText(body)
				if v4id == "TDRC" || v4id == "TDOR" || v4id == "TDRL" || v4id == "TDTG" || v4id == "TDEN" {
					pushTimestamp(v4id, text)
				} else {
					pushText(v4id, text)
				}
			case v4id[0] == 'W':
				ik, ok := ItemKeyFromKey(TagTypeID3v2, v4id)
				if !ok {
					ik = ItemKeyUnknown
				}
				item := NewTagItem(ik, ItemValueLocator(latin1Decode(body)))
				if ik == ItemKeyUnknown {
					item.Description = v4id
				}
				tag.PushUnchecked(item)
			default:
				ik, ok := ItemKeyFromKey(TagTypeID3v2, v4id)
				if !ok {
					ik = ItemKeyUnknown
				}
				item := NewTagItem(ik, ItemValueBinary(body))
				if ik == ItemKeyUnknown {
					item.Description = v4id
				}
				tag.PushUnchecked(item)
			}
		}
	}
	return tag, nil
}

// id3v2TruncateBadFrames trims tag content to the end of the last frame that
// fits inside it, so a trailing malformed frame doesn't fail the whole tag
// (Rust's BestAttempt parse keeps already-parsed frames).
func id3v2TruncateBadFrames(content []byte, header Id3v2Header) ([]byte, error) {
	pos := 0
	last := 0
	for pos+10 <= len(content) {
		id := content[pos : pos+4]
		if id[0] == 0 && id[1] == 0 && id[2] == 0 && id[3] == 0 {
			break // padding
		}
		for _, c := range id {
			if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				return content[:last], nil
			}
		}
		var size uint32
		if header.Version == 4 {
			size = unsynchsafe32(content[pos+4 : pos+8])
		} else {
			size = binary.BigEndian.Uint32(content[pos+4 : pos+8])
		}
		// Encrypted frames are read into memory eagerly in Rust via try_vec —
		// enforce the allocation limit on their declared size.
		encrypted := header.Version == 4 && content[pos+9]&0x04 != 0 ||
			header.Version == 3 && content[pos+9]&0x40 != 0
		if encrypted {
			if _, err := tryByteSlice(int(size)); err != nil {
				return nil, err
			}
		}
		end := pos + 10 + int(size)
		if end > len(content) {
			break
		}
		pos = end
		last = pos
	}
	return content[:last], nil
}

// id3v2DecompressFrames rewrites ID3v2.3 zlib-compressed frames (flag 0x80)
// into uncompressed frames, inflating regardless of the declared decompressed
// size. Uninflatable frames are dropped (Rust's BestAttempt skips them).
func id3v2DecompressFrames(content []byte) []byte {
	var out []byte
	pos := 0
	for pos+10 <= len(content) {
		id := content[pos : pos+4]
		if id[0] == 0 && id[1] == 0 && id[2] == 0 && id[3] == 0 {
			break
		}
		size := int(binary.BigEndian.Uint32(content[pos+4 : pos+8]))
		flags := content[pos+8 : pos+10]
		end := pos + 10 + size
		if end > len(content) {
			break
		}
		body := content[pos+10 : end]
		pos = end
		if flags[1]&0x80 == 0 {
			out = append(out, content[pos-size-10:pos]...)
			continue
		}
		// Compressed: body = 4-byte decompressed size + zlib stream
		if len(body) < 5 {
			continue
		}
		zr, err := zlib.NewReader(bytes.NewReader(body[4:]))
		if err != nil {
			continue
		}
		inflated, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			continue
		}
		out = append(out, id...)
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(len(inflated)))
		out = append(out, sz[:]...)
		out = append(out, flags[0], flags[1]&^0x80)
		out = append(out, inflated...)
	}
	return out
}

// id3v2TextFrameNoTerm is a bogem [bogem.Framer] that writes a text frame body
// without a trailing terminator, matching Rust's write behavior (bogem's
// own TextFrame appends Encoding.TerminationBytes).
type id3v2TextFrameNoTerm struct {
	enc  bogem.Encoding
	text string
}

func (f id3v2TextFrameNoTerm) encoded() []byte {
	switch {
	case f.enc.Equals(bogem.EncodingUTF8):
		return []byte(f.text)
	case f.enc.Equals(bogem.EncodingUTF16):
		return utf16Encode(f.text, littleEndian, true, false)
	case f.enc.Equals(bogem.EncodingUTF16BE):
		return utf16Encode(f.text, bigEndian, false, false)
	case f.enc.Equals(bogem.EncodingISO):
		b, _ := latin1Encode(f.text, true)
		return b
	default:
		return []byte(f.text)
	}
}

func (f id3v2TextFrameNoTerm) Size() int { return 1 + len(f.encoded()) }

func (f id3v2TextFrameNoTerm) UniqueIdentifier() string { return "ID" }

func (f id3v2TextFrameNoTerm) WriteTo(w io.Writer) (int64, error) {
	body := make([]byte, 0, f.Size())
	body = append(body, f.enc.Key)
	body = append(body, f.encoded()...)
	n, err := w.Write(body)
	return int64(n), err
}
