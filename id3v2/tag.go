package id3v2

import (
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
	"strings"

	"github.com/subframe7536/lofty-go/id3v1"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

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

// AsId3v24Byte gets the ID3v2.4 byte representation of the flags.
//
// This does not include the extended header flags.
func (f Id3v2TagFlags) AsId3v24Byte() uint8 {
	var b uint8
	if f.Unsynchronisation {
		b |= 0x80
	}
	if f.Experimental {
		b |= 0x20
	}
	if f.Footer {
		b |= 0x10
	}
	return b
}

// AsId3v23Byte gets the ID3v2.3 byte representation of the flags.
//
// This does not include the extended header flags.
func (f Id3v2TagFlags) AsId3v23Byte() uint8 {
	var b uint8
	if f.Experimental {
		b |= 0x40
	}
	if f.Footer {
		b |= 0x10
	}
	return b
}

func (f Id3v2TagFlags) asId3v24Byte() uint8 { return f.AsId3v24Byte() }
func (f Id3v2TagFlags) asId3v23Byte() uint8 { return f.AsId3v23Byte() }

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
	TextFieldsSize meta.Option[TextSizeRestriction]
	ImageEncoding  bool
	ImageSize      meta.Option[ImageSizeRestriction]
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
		r.TextFieldsSize = meta.Some(TextSizeC1024)
	case 0x10:
		r.TextFieldsSize = meta.Some(TextSizeC128)
	case 0x18:
		r.TextFieldsSize = meta.Some(TextSizeC30)
	}
	if b&0x04 != 0 {
		r.ImageEncoding = true
	}
	switch b & 0x03 {
	case 1:
		r.ImageSize = meta.Some(ImageSizeP256)
	case 2:
		r.ImageSize = meta.Some(ImageSizeP64)
	case 3:
		r.ImageSize = meta.Some(ImageSizeP6464)
	}
	return r
}

// asByte encodes a [TagRestrictions] byte (id3v2.4 §3.2 item d).
func (r TagRestrictions) asByte() uint8 {
	var b uint8
	switch r.Size {
	case TagSizeS64F128K:
		b |= 0x40
	case TagSizeS32F40K:
		b |= 0x80
	case TagSizeS32F4K:
		b |= 0xC0
	}
	if r.TextEncoding {
		b |= 0x20
	}
	if s, ok := r.TextFieldsSize.Get(); ok {
		switch s {
		case TextSizeC1024:
			b |= 0x08
		case TextSizeC128:
			b |= 0x10
		case TextSizeC30:
			b |= 0x18
		}
	}
	if r.ImageEncoding {
		b |= 0x04
	}
	if s, ok := r.ImageSize.Get(); ok {
		switch s {
		case ImageSizeP256:
			b |= 0x01
		case ImageSizeP64:
			b |= 0x02
		case ImageSizeP6464:
			b |= 0x03
		}
	}
	return b
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
//
// Like Rust's `Id3v2Header::full_tag_size`, the extended header size is added
// even though it is already counted in [Id3v2Header.Size] — the resulting
// double-count is mirrored intentionally for parity.
func (h Id3v2Header) FullTagSize() uint32 {
	size := h.Size + 10 + h.ExtendedSize
	if h.Flags.Footer {
		size += 10
	}
	return size
}

// ParseId3v2HeaderRaw parses the 10-byte ID3v2 header from reader.
func ParseId3v2HeaderRaw(reader io.Reader) (Id3v2Header, error) {
	var b [10]byte
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return Id3v2Header{}, err
	}
	if !bytes.Equal(b[:3], []byte("ID3")) {
		return Id3v2Header{}, &Id3v2ParseError{Source: &meta.FakeTagError{}}
	}
	version := b[3]
	if version != 2 && version != 3 && version != 4 {
		return Id3v2Header{}, &Id3v2ParseError{Source: &Id3v2HeaderError{Kind: Id3v2HeaderBadVersion, Major: version, Minor: b[4]}}
	}
	var h Id3v2Header
	h.Version = version
	flags := b[5]
	switch version {
	case 2:
		// Bit 2 is the abandoned v2.2 compression flag — the spec says to ignore the tag.
		if flags&0x40 != 0 {
			return Id3v2Header{}, &Id3v2ParseError{Source: errV2Compression}
		}
	}
	h.Flags.Unsynchronisation = flags&0x80 != 0
	if version >= 3 {
		h.Flags.Experimental = flags&0x20 != 0
		h.Flags.Footer = flags&0x10 != 0
		h.Flags.ExtendedHeader = flags&0x40 != 0
	}
	h.Size = util.Unsynchsafe32(b[6:10])
	return h, nil
}

// ParseId3v2Header parses an ID3v2 header plus its extended header (if any)
// from reader, matching Rust's `Id3v2Header::parse`.
func ParseId3v2Header(reader io.Reader, parseOptions meta.ParseOptions) (Id3v2Header, error) {
	h, err := ParseId3v2HeaderRaw(reader)
	if err != nil {
		return h, err
	}
	if !h.Flags.ExtendedHeader {
		return h, nil
	}
	// In ID3v2.3, the extended header is considered separate from the tag
	// header and is thus subject to unsynchronization; in v2.4 no flags apply.
	extReader := reader
	if h.Version == 3 && h.Flags.Unsynchronisation {
		extReader = &unsyncReader{r: reader}
	}
	var sizeBuf [4]byte
	if _, err := io.ReadFull(extReader, sizeBuf[:]); err != nil {
		return h, &Id3v2ParseError{Source: err}
	}
	var extContent []byte
	if h.Version == 3 {
		h.ExtendedSize = binary.BigEndian.Uint32(sizeBuf[:])
		if h.ExtendedSize != 6 && h.ExtendedSize != 10 {
			return h, &Id3v2ParseError{Source: errBadExtendedHeaderSize}
		}
		extContent = make([]byte, h.ExtendedSize)
	} else {
		// The v2.4 extended header size is synchsafe and includes its own
		// size field.
		h.ExtendedSize = util.Unsynchsafe32(sizeBuf[:])
		if h.ExtendedSize < 6 || h.ExtendedSize > 15 {
			return h, &Id3v2ParseError{Source: errBadExtendedHeaderSize}
		}
		extContent = make([]byte, h.ExtendedSize-4)
	}
	if _, err := io.ReadFull(extReader, extContent); err != nil {
		return h, &Id3v2ParseError{Source: err}
	}
	if err := h.parseExtendedHeader(extContent, parseOptions.ParsingMode); err != nil {
		return h, err
	}
	if h.ExtendedSize > 0 && h.ExtendedSize > h.Size {
		return h, &Id3v2ParseError{Source: errBadExtendedHeaderSize}
	}
	return h, nil
}

// parseExtendedHeader interprets the extended header content (after the size
// field for v2.3; after the synchsafe size field for v2.4).
func (h *Id3v2Header) parseExtendedHeader(content []byte, mode meta.ParsingMode) error {
	if h.Version == 3 {
		// ext content: 2-byte flags + 4-byte padding size + optional 4-byte CRC
		if h.ExtendedSize != 6 && h.ExtendedSize != 10 {
			return &Id3v2ParseError{Source: errBadExtendedHeaderSize}
		}
		if len(content) < int(h.ExtendedSize) {
			return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		extFlags := binary.BigEndian.Uint16(content[:2])
		remaining := content[6:h.ExtendedSize]
		if extFlags&0x8000 != 0 {
			h.Flags.Crc = true
			if len(remaining) < 4 {
				return &Id3v2ParseError{Source: io.ErrUnexpectedEOF}
			}
			remaining = remaining[4:]
		}
		if extFlags&^0x8000 != 0 || len(remaining) != 0 {
			if mode == meta.ParsingModeStrict {
				return &Id3v2ParseError{Source: errBadExtendedHeaderFlags}
			}
		}
		return nil
	}

	// v2.4: ext content (after size field): 1-byte flag count + 1-byte flags +
	// per-flag (len + data). h.ExtendedSize already includes the size field.
	if h.ExtendedSize < 6 || h.ExtendedSize > 15 {
		return &Id3v2ParseError{Source: errBadExtendedHeaderSize}
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
		if mode == meta.ParsingModeStrict {
			return &Id3v2ParseError{Source: errBadExtendedHeaderFlags}
		}
	}
	return nil
}

// --- Id3v2Tag ---

// Id3v2Tag is a native ID3v2 tag containing a list of [Frame]s.
//
// It is both a frame container ([FrameList]) and the conversion target for
// the generic [Tag] via [Id3v2TagToTag] and [Id3v2TagFromTag].
type Id3v2Tag struct {
	flags           Id3v2TagFlags
	originalVersion Id3v2Version
	Frames          *FrameList
}

// NewId3v2Tag creates an empty [Id3v2Tag].
func NewId3v2Tag() *Id3v2Tag {
	return &Id3v2Tag{originalVersion: id3v2Version4, Frames: NewFrameList()}
}

// Flags returns the tag's flags.
func (t *Id3v2Tag) Flags() Id3v2TagFlags { return t.flags }

// SetFlags replaces the tag's flags.
func (t *Id3v2Tag) SetFlags(flags Id3v2TagFlags) { t.flags = flags }

// OriginalVersion returns the ID3v2 version the tag was read as (2, 3, or 4).
func (t *Id3v2Tag) OriginalVersion() Id3v2Version { return t.originalVersion }

// SetOriginalVersion sets the stored original version.
func (t *Id3v2Tag) SetOriginalVersion(v Id3v2Version) { t.originalVersion = v }

// IsEmpty reports whether the tag has no frames.
func (t *Id3v2Tag) IsEmpty() bool { return t.Frames.IsEmpty() }

// UnsyncText returns the text of the only USLT frame, if exactly one exists
// (Rust `Id3v2Tag::unsync_text`).
func (t *Id3v2Tag) UnsyncText() string {
	var content string
	found := false
	for _, f := range t.Frames.Frames() {
		if f.ID() != FrameIdValid("USLT") {
			continue
		}
		if found {
			return ""
		}
		if uf, ok := f.(*UnsynchronizedTextFrame); ok {
			content = uf.Content
			found = true
		}
	}
	return content
}

// Comments returns the comment frames with the empty content descriptor
// (Rust `Id3v2Tag::comments` — custom-description comments are excluded).
func (t *Id3v2Tag) Comments() []*CommentFrame {
	var out []*CommentFrame
	for _, f := range t.Frames.Frames() {
		if cf, ok := f.(*CommentFrame); ok && cf.Description == EmptyContentDescriptor {
			out = append(out, cf)
		}
	}
	return out
}

// Genres translates the numeric ID3v1 genre references in TCON into their
// textual equivalents (Rust `Id3v2Tag::genres`).
func (t *Id3v2Tag) Genres() []string {
	genre, ok := t.Frames.GetText(FrameIdValid("TCON"))
	if !ok {
		return nil
	}
	return id3v2GenresIter(genre, false)
}

// --- Accessor impl ---

func (t *Id3v2Tag) text(id string) string {
	s, _ := t.Frames.GetText(FrameIdValid(id))
	return s
}

func (t *Id3v2Tag) setText(id string, value string) {
	fid := FrameIdValid(id)
	t.Frames.Insert(NewTextInformationFrame(fid, util.TextEncodingUTF8, value))
}

// Title returns the tag's title (TIT2).
func (t *Id3v2Tag) Title() string { return t.text("TIT2") }

// SetTitle sets the tag's title (TIT2).
func (t *Id3v2Tag) SetTitle(title string) { t.setText("TIT2", title) }

// RemoveTitle removes the tag's title (TIT2).
func (t *Id3v2Tag) RemoveTitle() { t.Frames.Remove(FrameIdValid("TIT2")) }

// Artist returns the tag's artist (TPE1).
func (t *Id3v2Tag) Artist() string { return t.text("TPE1") }

// SetArtist sets the tag's artist (TPE1).
func (t *Id3v2Tag) SetArtist(artist string) { t.setText("TPE1", artist) }

// RemoveArtist removes the tag's artist (TPE1).
func (t *Id3v2Tag) RemoveArtist() { t.Frames.Remove(FrameIdValid("TPE1")) }

// Album returns the tag's album title (TALB).
func (t *Id3v2Tag) Album() string { return t.text("TALB") }

// SetAlbum sets the tag's album title (TALB).
func (t *Id3v2Tag) SetAlbum(album string) { t.setText("TALB", album) }

// RemoveAlbum removes the tag's album title (TALB).
func (t *Id3v2Tag) RemoveAlbum() { t.Frames.Remove(FrameIdValid("TALB")) }

// Track returns the tag's track number (TRCK "N/T" first part).
func (t *Id3v2Tag) Track() meta.Option[uint32] {
	return firstPairValue(t.text("TRCK"))
}

// SetTrack sets the tag's track number, preserving the total if present.
func (t *Id3v2Tag) SetTrack(track uint32) { t.setNumberPair("TRCK", meta.Some(track)) }

// RemoveTrack removes the tag's track number.
func (t *Id3v2Tag) RemoveTrack() { t.Frames.Remove(FrameIdValid("TRCK")) }

// TrackTotal returns the tag's total tracks (TRCK "N/T" second part).
func (t *Id3v2Tag) TrackTotal() meta.Option[uint32] {
	return secondPairValue(t.text("TRCK"))
}

// SetTrackTotal sets the tag's total tracks, preserving the number if present.
func (t *Id3v2Tag) SetTrackTotal(total uint32) { t.setNumberPairTotal("TRCK", meta.Some(total)) }

// RemoveTrackTotal removes the tag's total tracks.
func (t *Id3v2Tag) RemoveTrackTotal() { t.setNumberPairTotal("TRCK", meta.None[uint32]()) }

// Disk returns the tag's disc number (TPOS).
func (t *Id3v2Tag) Disk() meta.Option[uint32] { return firstPairValue(t.text("TPOS")) }

// SetDisk sets the tag's disc number, preserving the total if present.
func (t *Id3v2Tag) SetDisk(disk uint32) { t.setNumberPair("TPOS", meta.Some(disk)) }

// RemoveDisk removes the tag's disc number.
func (t *Id3v2Tag) RemoveDisk() { t.Frames.Remove(FrameIdValid("TPOS")) }

// DiskTotal returns the tag's total discs (TPOS).
func (t *Id3v2Tag) DiskTotal() meta.Option[uint32] { return secondPairValue(t.text("TPOS")) }

// SetDiskTotal sets the tag's total discs, preserving the number if present.
func (t *Id3v2Tag) SetDiskTotal(total uint32) { t.setNumberPairTotal("TPOS", meta.Some(total)) }

// RemoveDiskTotal removes the tag's total discs.
func (t *Id3v2Tag) RemoveDiskTotal() { t.setNumberPairTotal("TPOS", meta.None[uint32]()) }

// Date returns the tag's timestamp (TDRC).
func (t *Id3v2Tag) Date() string { return t.text("TDRC") }

// SetDate sets the tag's timestamp (TDRC) — accepts a parseable timestamp
// string or falls back to plain text.
func (t *Id3v2Tag) SetDate(date string) {
	if ts, err := meta.TimestampParseString(date); err == nil {
		t.Frames.Insert(NewTimestampFrame(FrameIdValid("TDRC"), util.TextEncodingUTF8, ts))
		return
	}
	t.setText("TDRC", date)
}

// RemoveDate removes the tag's timestamp (TDRC).
func (t *Id3v2Tag) RemoveDate() { t.Frames.Remove(FrameIdValid("TDRC")) }

// Genre returns the tag's genre (TCON) with numeric references translated.
func (t *Id3v2Tag) Genre() string {
	genres := t.Genres()
	if len(genres) == 0 {
		return ""
	}
	return strings.Join(genres, "/")
}

// SetGenre sets the tag's genre (TCON).
func (t *Id3v2Tag) SetGenre(genre string) { t.setText("TCON", genre) }

// RemoveGenre removes the tag's genre (TCON).
func (t *Id3v2Tag) RemoveGenre() { t.Frames.Remove(FrameIdValid("TCON")) }

// Comment returns the first empty-descriptor comment's text.
func (t *Id3v2Tag) Comment() string {
	comments := t.Comments()
	if len(comments) == 0 {
		return ""
	}
	return comments[0].Content
}

// SetComment sets the tag's comment, mirroring Rust `set_comment`: replace
// the first empty-descriptor frame's content in place, remove any other
// empty-descriptor comments, or append a new frame when none exists.
func (t *Id3v2Tag) SetComment(comment string) {
	value := comment
	var remaining []Frame
	set := false
	for _, f := range t.Frames.Frames() {
		cf, ok := f.(*CommentFrame)
		if !ok || cf.Description != EmptyContentDescriptor {
			remaining = append(remaining, f)
			continue
		}
		if !set {
			cf.Content = value
			value = ""
			set = true
			remaining = append(remaining, f)
		}
	}
	if !set {
		remaining = append(remaining, NewCommentFrame(util.TextEncodingUTF8, meta.LangUnknown, EmptyContentDescriptor, comment))
	}
	t.Frames.SetFrames(remaining)
}

// RemoveComment removes all empty-descriptor COMM frames (Rust `remove_comment`).
func (t *Id3v2Tag) RemoveComment() {
	var remaining []Frame
	for _, f := range t.Frames.Frames() {
		if cf, ok := f.(*CommentFrame); ok && cf.Description == EmptyContentDescriptor {
			continue
		}
		remaining = append(remaining, f)
	}
	t.Frames.SetFrames(remaining)
}

func firstPairValue(text string) meta.Option[uint32] {
	num, _ := splitNumTotal(text)
	if num == "" {
		return meta.None[uint32]()
	}
	if n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 32); err == nil {
		return meta.Some(uint32(n))
	}
	return meta.None[uint32]()
}

func secondPairValue(text string) meta.Option[uint32] {
	_, tot := splitNumTotal(text)
	if tot == "" {
		return meta.None[uint32]()
	}
	if n, err := strconv.ParseUint(strings.TrimSpace(tot), 10, 32); err == nil {
		return meta.Some(uint32(n))
	}
	return meta.None[uint32]()
}

// setNumberPair sets the number of an "N/T" frame, preserving the total.
func (t *Id3v2Tag) setNumberPair(id string, num meta.Option[uint32]) {
	_, tot := splitNumTotal(t.text(id))
	var totOpt meta.Option[uint32]
	if n, err := strconv.ParseUint(strings.TrimSpace(tot), 10, 32); err == nil && tot != "" {
		totOpt = meta.Some(uint32(n))
	}
	if f := newNumberPairFrame(FrameIdValid(id), num, totOpt); f != nil {
		t.Frames.Insert(f)
	} else {
		t.Frames.Remove(FrameIdValid(id))
	}
}

// setNumberPairTotal sets the total of an "N/T" frame, preserving the number.
func (t *Id3v2Tag) setNumberPairTotal(id string, total meta.Option[uint32]) {
	num, _ := splitNumTotal(t.text(id))
	var numOpt meta.Option[uint32]
	if n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 32); err == nil && num != "" {
		numOpt = meta.Some(uint32(n))
	}
	if f := newNumberPairFrame(FrameIdValid(id), numOpt, total); f != nil {
		t.Frames.Insert(f)
	} else {
		t.Frames.Remove(FrameIdValid(id))
	}
}

// DumpTo writes the tag to writer, mirroring Rust `Id3v2Tag::dump_to`.
func (t *Id3v2Tag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	frames := t.Frames.CloneFrames()
	encoded, err := createTag(t.flags, frames, writeOptions)
	if err != nil {
		return &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}
	}
	if _, err := writer.Write(encoded); err != nil {
		return &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}
	}
	return nil
}

// --- File-level helpers ---

// ParseId3v2 reads an ID3v2 tag from reader (header already consumed) and
// returns it as a generic [Tag]. Use [ParseId3v2Tag] for the native model.
func ParseId3v2(reader io.Reader, header Id3v2Header, parseOptions meta.ParseOptions) (*meta.Tag, error) {
	itag, err := ParseId3v2Tag(reader, header, parseOptions)
	if err != nil {
		return nil, err
	}
	return Id3v2TagToTag(itag), nil
}

// Id3v2Serialize serializes a generic [Tag] to ID3v2 bytes, mirroring Rust's
// `dump_to` on a generic Tag (default flags + item/companion/picture frames).
func Id3v2Serialize(tag *meta.Tag, writeOptions meta.WriteOptions) ([]byte, error) {
	frames := tagFrames(tag)
	return createTag(Id3v2TagFlags{}, frames, writeOptions)
}

// Tag converts the tag to a generic [Tag], mirroring Rust's `split_tag`.
func (t *Id3v2Tag) Tag() *meta.Tag { return Id3v2TagToTag(t) }

// SaveTo writes the native tag into file at the ID3v2 position, mirroring
// Rust `TagWriteExt::save_to` — replaces any existing tag, removes when empty.
func (t *Id3v2Tag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if err := t.checkTagSupport(vf.Format()); err != nil {
		return err
	}
	return t.SaveToVerified(vf, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *Id3v2Tag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	if err := t.checkTagSupport(vf.Format()); err != nil {
		return err
	}
	return t.SaveToVerified(vf, writeOptions)
}

// checkTagSupport mirrors Rust's `TagExt::save_to` support gate: unsupported
// formats always fail; read-only formats only accept an empty (stripping) tag.
func (t *Id3v2Tag) checkTagSupport(format meta.FileType) error {
	support := format.TagSupport(meta.TagTypeID3v2)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return nil
}

// SaveToVerified writes the tag into a [meta.VerifiedFile] without the public
// support check — mirrors Rust's `TagWriteExt::save_to`, which AudioFile save
// paths invoke directly (e.g. ID3v2 in FLAC is read-only yet still written).
func (t *Id3v2Tag) SaveToVerified(vf *meta.VerifiedFile, writeOptions meta.WriteOptions) error {
	format := vf.Format()
	var encoded []byte
	if !t.Frames.IsEmpty() {
		var err error
		encoded, err = createTag(t.flags, t.Frames.CloneFrames(), writeOptions)
		if err != nil {
			return &meta.FileEncodingError{FileType: format, Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}
	// Format-specific placement (WAV/AIFF chunk, DSF end-of-stream)
	if handled, err := meta.WriteConcreteTag(vf, meta.TagTypeID3v2, encoded, writeOptions); handled {
		return err
	}
	file := vf.File()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	_, rangeEnd, found, err := FindId3v2(file, false, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var end uint64
	if found {
		end = uint64(rangeEnd)
	}
	return meta.Splice(file, 0, end, encoded)
}

// RemoveFrom removes an ID3v2 tag from a [meta.FileLike].
func (t *Id3v2Tag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeID3v2.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes an ID3v2 tag from a path.
func (t *Id3v2Tag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeID3v2.RemoveFromPath(path, writeOptions)
}

// WriteId3v2 writes an ID3v2 tag at the start of a file.
func WriteId3v2(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	var encoded []byte
	if !tag.IsEmpty() {
		var err error
		encoded, err = Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
	}

	// Find the existing tag range
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	_, rangeEnd, found, err := FindId3v2(file, false, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	var start, end uint64
	if found {
		end = uint64(rangeEnd)
	}
	return meta.Splice(file, start, end, encoded)
}

// Id3v2FindResult describes an ID3v2 tag located by [LocateId3v2], mirroring
// Rust's `ID3FindResults<Id3v2Header, Option<Vec<u8>>>`.
type Id3v2FindResult struct {
	// Header is the parsed tag header, extended header included.
	Header Id3v2Header
	// Content holds the bytes following the header (and extended header);
	// nil unless reading was requested.
	Content []byte
	// Start is the position the search began at, End the position after the
	// tag (footer included). Junk preceding the tag lies inside the range.
	Start, End int64
}

// Parse parses the located tag's content. It requires the result to have been
// located with reading enabled.
func (r *Id3v2FindResult) Parse(parseOptions meta.ParseOptions) (*Id3v2Tag, error) {
	return ParseId3v2Tag(bytes.NewReader(r.Content), r.Header, parseOptions)
}

// LocateId3v2 finds an ID3v2 tag at the current position (possibly preceded by
// junk), mirroring Rust's `find_id3v2`. It returns nil when no tag is found.
// On success the reader is positioned at the end of the tag; otherwise at the
// original position.
func LocateId3v2(reader io.ReadSeeker, read bool, parseOptions meta.ParseOptions) (*Id3v2FindResult, error) {
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	var offset int64
	var found bool
	if parseOptions.MaxJunkBytes > 0 {
		offset, found, err = util.ChunkedScan(reader, 2, int64(parseOptions.MaxJunkBytes),
			func(window []byte) int { return bytes.Index(window, []byte("ID3")) })
		if err != nil {
			return nil, err
		}
	}
	if !found {
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Seek back to start of "ID3"
	if _, err := reader.Seek(start+offset, io.SeekStart); err != nil {
		return nil, err
	}

	header, err := ParseId3v2Header(reader, parseOptions)
	if err != nil {
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Rust reads/skips `header.size` bytes after the header parser has already
	// consumed the extended header, which `size` includes, so its range ends
	// too far (a later splice would eat into the audio). Count only what is
	// left of the tag instead.
	headerEnd, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	remaining := start + offset + 10 + int64(header.Size) - headerEnd
	if remaining < 0 {
		remaining = 0
	}

	res := &Id3v2FindResult{Header: header, Start: start}
	if read {
		content, err := util.TryByteSlice(int(remaining))
		if err != nil {
			return nil, &Id3v2ParseError{Source: err}
		}
		if _, err := io.ReadFull(reader, content); err != nil {
			return nil, err
		}
		res.Content = content
	} else {
		if _, err := reader.Seek(remaining, io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	if header.Flags.Footer {
		if _, err := reader.Seek(10, io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	if res.End, err = reader.Seek(0, io.SeekCurrent); err != nil {
		return nil, err
	}
	return res, nil
}

// FindId3v2 finds an ID3v2 tag at the current position (possibly preceded by junk).
// Returns the header (if found), the end position of the tag, whether found, and an error.
// On success the reader is positioned at the end of the tag; on failure at the original position.
//
// Callers that need the tag's content should use [LocateId3v2].
func FindId3v2(reader io.ReadSeeker, read bool, parseOptions meta.ParseOptions) (Id3v2Header, int64, bool, error) {
	res, err := LocateId3v2(reader, read, parseOptions)
	if err != nil || res == nil {
		return Id3v2Header{}, 0, false, err
	}
	return res.Header, res.End, true, nil
}

// FindId3v2InJunk scans reader for an "ID3" magic; on return the reader is
// positioned right after it. Returns (offset of magic, found).
func FindId3v2InJunk(reader io.Reader) (int64, bool) {
	if rs, ok := reader.(io.ReadSeeker); ok {
		start, err := rs.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, false
		}
		offset, found, err := util.ChunkedScan(rs, 2, -1,
			func(window []byte) int { return bytes.Index(window, []byte("ID3")) })
		if err != nil || !found {
			return 0, false
		}
		if _, err := rs.Seek(start+offset+3, io.SeekStart); err != nil {
			return 0, false
		}
		return offset, true
	}
	var window [3]byte
	idx := int64(0)
	var oneByte [1]byte
	empty := 0
	for {
		n, err := reader.Read(oneByte[:])
		if n > 0 {
			empty = 0
			window[0] = window[1]
			window[1] = window[2]
			window[2] = oneByte[0]
			idx++
			if bytes.Equal(window[:], []byte("ID3")) {
				return idx - 3, true
			}
		}
		if err != nil {
			return 0, false
		}
		if n == 0 {
			// Like bufio, give up on a reader that never makes progress
			if empty++; empty >= 100 {
				return 0, false
			}
		}
	}
}

// Id3v2Deunsync removes 0x00 bytes following 0xFF (unsynchronisation scheme).
func Id3v2Deunsync(data []byte) []byte {
	out := data[:0]
	for i := 0; i < len(data); i++ {
		out = append(out, data[i])
		if data[i] == 0xFF && i+1 < len(data) && data[i+1] == 0x00 {
			i++
		}
	}
	return out
}

// --- genre helpers ---

// parseId3Genre translates a single genre component (Rust `parse_genre`).
func parseId3Genre(genre string, preserveIndexes bool) string {
	if len(genre) > 3 {
		return genre
	}
	if id, err := util.Atoi(genre); err == nil && id >= 0 {
		if id < len(id3v1.Id3v1Genres) && !preserveIndexes {
			return id3v1.Id3v1Genres[id]
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
	n, err := util.Atoi(s)
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
			b.WriteByte('(')
			b.WriteString(genre)
			b.WriteByte(')')
		}
	}
	return b.String()
}

// --- merge maps ---

// id3v23DiscardFrames are v2.4-only frame IDs dropped when saving as v2.3.
var id3v23DiscardFrames = map[string]bool{
	"ASPI": true, "EQU2": true, "RVA2": true, "SEEK": true, "SIGN": true,
	"TDEN": true, "TDRL": true, "TDTG": true, "TMOO": true, "TPRO": true,
	"TSOA": true, "TSOP": true, "TSOT": true, "TSST": true,
}

// id3v2MultiTxxxKeys merge multiple items into a single TXXX frame,
// NUL-separated (Rust `from_tag` multi-valued TXXX arm).
var id3v2MultiTxxxKeys = map[meta.ItemKey]bool{
	meta.ItemKeyTrackArtists: true, meta.ItemKeyAlbumArtists: true,
	meta.ItemKeyDirector: true, meta.ItemKeyAcoustId: true,
	meta.ItemKeyAcoustIdFingerprint: true, meta.ItemKeyCatalogNumber: true,
	meta.ItemKeyMusicBrainzArtistId: true, meta.ItemKeyMusicBrainzReleaseArtistId: true,
	meta.ItemKeyMusicBrainzWorkId: true, meta.ItemKeyMusicBrainzReleaseType: true,
	meta.ItemKeyMusicBrainzTrackId: true, meta.ItemKeyMusicBrainzReleaseId: true,
	meta.ItemKeyMusicBrainzReleaseGroupId: true, meta.ItemKeyReleaseCountry: true,
	meta.ItemKeyBarcode: true, meta.ItemKeyColor: true, meta.ItemKeyWork: true,
}

// id3v2SingleTxxxKeys write a single TXXX frame; duplicates are
// deduplicated keeping the last value (Rust HashMap insert semantics).
var id3v2SingleTxxxKeys = map[meta.ItemKey]bool{
	meta.ItemKeyReplayGainAlbumGain: true, meta.ItemKeyReplayGainAlbumPeak: true,
	meta.ItemKeyReplayGainTrackGain: true, meta.ItemKeyReplayGainTrackPeak: true,
}

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

// id3v2DecodeTextBody decodes text from a frame body where data[0] is the
// encoding byte (0=latin1, 1=UTF-16 w/ BOM, 2=UTF-16BE, 3=UTF-8).
func id3v2DecodeTextBody(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	switch data[0] {
	case 1:
		d := data[1:]
		order := util.LittleEndian
		if len(d) >= 2 && d[0] == 0xFE && d[1] == 0xFF {
			order = util.BigEndian
		}
		s, err := util.Utf16DecodeBytes(d, order)
		if err == nil {
			return s
		}
		return util.Latin1Decode(d)
	case 2:
		s, err := util.Utf16DecodeBytes(data[1:], util.BigEndian)
		if err == nil {
			return s
		}
		return util.Latin1Decode(data[1:])
	case 3:
		return util.TrimEndNulls(string(data[1:]))
	default:
		return util.Latin1Decode(data[1:])
	}
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

// langOrXxx returns "XXX" for an unset language, mirroring Rust's UNKNOWN_LANGUAGE.
func langOrXxx(l meta.Lang) string {
	if l == meta.LangUnknown || l == [3]byte{} {
		return "XXX"
	}
	return string(l[:])
}

// --- upgrade maps ---

// UpgradeV2 upgrades an ID3v2.2 key to its ID3v2.4 equivalent.
func UpgradeV2(id string) (string, bool) {
	v, ok := v2ToV4FrameIds[id]
	return v, ok
}

// UpgradeV3 upgrades an ID3v2.3 key to its ID3v2.4 equivalent.
func UpgradeV3(id string) (string, bool) {
	v, ok := v3ToV4FrameIds[id]
	return v, ok
}

// v2ToV4FrameIds maps ID3v2.2 frame IDs to their ID3v2.4 equivalents.
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

// v3ToV4FrameIds maps ID3v2.3 frame IDs to their ID3v2.4 equivalents.
var v3ToV4FrameIds = map[string]string{
	"TORY": "TDOR", "TYER": "TDRC", "IPLS": "TIPL",
}

func init() {
	meta.RegisterTagDumper(meta.TagTypeID3v2, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		if tag.IsEmpty() {
			return nil
		}
		encoded, err := Id3v2Serialize(tag, writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(encoded)
		return err
	})
}

// RemoveKey removes all frames mapping to `key`, mirroring Rust's
// `TagExt::remove_key` on `Id3v2Tag`.
func (t *Id3v2Tag) RemoveKey(key meta.ItemKey) {
	removed := map[string]bool{}
	if id, ok := key.MapKey(meta.TagTypeID3v2); ok {
		removed[id] = true
	}
	// Keys sharing a frame ID (e.g. TrackNumber/TrackTotal -> TRCK) and
	// multi-role frames (TIPL/TMCL) are dropped when any of their keys is
	// removed, matching Rust item-level retain semantics on frames.
	var fid FrameId
	for id := range removed {
		fid = FrameIdValid(id)
		t.Frames.Remove(fid)
	}
}

// Remove removes all frames with the given [FrameId].
func (t *Id3v2Tag) Remove(id FrameId) { t.Frames.Remove(id) }
