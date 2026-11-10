package id3v2

import (
	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// The MusicBrainz UFID owner identifier.
const musicBrainzUfidOwner = "http://musicbrainz.org"

// EmptyContentDescriptor is the empty content descriptor used by comment and
// unsynchronized text frames that carry no description.
const EmptyContentDescriptor = ""

// FrameId is an ID3v2 frame ID.
//
// A frame ID is either a valid ID3v2.3/4 ID (4 characters) or an outdated
// ID3v2.2 ID (3 characters) that failed to upgrade. Outdated IDs are never
// written; they must be upgraded first.
type FrameId struct {
	id       string
	outdated bool
}

// FrameIdValid creates a valid (4-character) [FrameId] without verification.
func FrameIdValid(id string) FrameId { return FrameId{id: id} }

// FrameIdOutdated creates an outdated (3-character) [FrameId] without verification.
func FrameIdOutdated(id string) FrameId { return FrameId{id: id, outdated: true} }

// ParseFrameId attempts to create a [FrameId] from an ID string, verifying it
// contains only 'A'-'Z' and '0'-'9' and is 3 or 4 characters long. This does
// not upgrade IDs.
func ParseFrameId(id string) (FrameId, error) {
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return FrameId{}, &FrameIdParseError{IdBytes: []byte(id)}
		}
	}
	switch len(id) {
	case 3:
		return FrameIdOutdated(id), nil
	case 4:
		return FrameIdValid(id), nil
	default:
		return FrameId{}, &FrameIdParseError{IdBytes: []byte(id)}
	}
}

// IsOutdated reports whether this is an outdated (ID3v2.2) ID.
func (i FrameId) IsOutdated() bool { return i.outdated }

// IsValid reports whether this is a valid (ID3v2.3 or ID3v2.4) ID.
func (i FrameId) IsValid() bool { return !i.outdated }

// String returns the ID string.
func (i FrameId) String() string { return i.id }

// AsStr returns the ID string.
func (i FrameId) AsStr() string { return i.id }

// FrameFlags are various flags that describe the content of a frame.
type FrameFlags struct {
	// TagAlterPreservation — preserve the frame on tag edit.
	TagAlterPreservation bool
	// FileAlterPreservation — preserve the frame on file edit.
	FileAlterPreservation bool
	// ReadOnly — the frame cannot be written to.
	ReadOnly bool
	// GroupingIdentity is the group identifier the frame belongs to.
	GroupingIdentity Option[uint8]
	// Compression — the frame is zlib compressed.
	Compression bool
	// Encryption is the frame's encryption method symbol (> 0x80).
	Encryption Option[uint8]
	// Unsynchronisation — the frame content is unsynchronised.
	Unsynchronisation bool
	// DataLengthIndicator is the size of the frame if the flags were all zeroed out.
	DataLengthIndicator Option[uint32]
}

// ParseFrameFlagsId3v24 parses the flags from an ID3v2.4 frame.
func ParseFrameFlagsId3v24(flags uint16) FrameFlags {
	f := FrameFlags{
		TagAlterPreservation:  flags&0x4000 == 0x4000,
		FileAlterPreservation: flags&0x2000 == 0x2000,
		ReadOnly:              flags&0x1000 == 0x1000,
		Compression:           flags&0x0008 == 0x0008,
		Unsynchronisation:     flags&0x0002 == 0x0002,
	}
	if flags&0x0040 == 0x0040 {
		f.GroupingIdentity = Some(uint8(0))
	}
	if flags&0x0004 == 0x0004 {
		f.Encryption = Some(uint8(0))
	}
	if flags&0x0001 == 0x0001 {
		f.DataLengthIndicator = Some(uint32(0))
	}
	return f
}

// ParseFrameFlagsId3v23 parses the flags from an ID3v2.3 frame.
func ParseFrameFlagsId3v23(flags uint16) FrameFlags {
	f := FrameFlags{
		TagAlterPreservation:  flags&0x8000 == 0x8000,
		FileAlterPreservation: flags&0x4000 == 0x4000,
		ReadOnly:              flags&0x2000 == 0x2000,
		Compression:           flags&0x0080 == 0x0080,
	}
	if flags&0x0020 == 0x0020 {
		f.GroupingIdentity = Some(uint8(0))
	}
	if flags&0x0040 == 0x0040 {
		f.Encryption = Some(uint8(0))
	}
	return f
}

// IsDefault reports whether no flags are set.
func (f FrameFlags) IsDefault() bool { return f == FrameFlags{} }

// AsId3v24Bytes gets the ID3v2.4 byte representation of the flags.
func (f FrameFlags) AsId3v24Bytes() uint16 {
	if f.IsDefault() {
		return 0
	}
	var flags uint16
	if f.TagAlterPreservation {
		flags |= 0x4000
	}
	if f.FileAlterPreservation {
		flags |= 0x2000
	}
	if f.ReadOnly {
		flags |= 0x1000
	}
	if f.GroupingIdentity.IsSome() {
		flags |= 0x0040
	}
	if f.Compression {
		flags |= 0x0008
	}
	if f.Encryption.IsSome() {
		flags |= 0x0004
	}
	if f.Unsynchronisation {
		flags |= 0x0002
	}
	if f.DataLengthIndicator.IsSome() {
		flags |= 0x0001
	}
	return flags
}

// AsId3v23Bytes gets the ID3v2.3 byte representation of the flags.
func (f FrameFlags) AsId3v23Bytes() uint16 {
	if f.IsDefault() {
		return 0
	}
	var flags uint16
	if f.TagAlterPreservation {
		flags |= 0x8000
	}
	if f.FileAlterPreservation {
		flags |= 0x4000
	}
	if f.ReadOnly {
		flags |= 0x2000
	}
	if f.GroupingIdentity.IsSome() {
		flags |= 0x0020
	}
	if f.Compression {
		flags |= 0x0080
	}
	if f.Encryption.IsSome() {
		flags |= 0x0040
	}
	return flags
}

// FrameHeader is an ID3v2 frame header.
type FrameHeader struct {
	ID    FrameId
	Flags FrameFlags
}

// NewFrameHeader creates a new [FrameHeader].
func NewFrameHeader(id FrameId, flags FrameFlags) FrameHeader {
	return FrameHeader{ID: id, Flags: flags}
}

// Frame is an ID3v2 frame.
//
// This is a sealed interface implemented by the concrete frame types in this
// package: [CommentFrame], [UnsynchronizedTextFrame], [TextInformationFrame],
// [ExtendedTextFrame], [UrlLinkFrame], [ExtendedUrlFrame],
// [AttachedPictureFrame], [PopularimeterFrame], [KeyValueFrame],
// [RelativeVolumeAdjustmentFrame], [UniqueFileIdentifierFrame],
// [OwnershipFrame], [EventTimingCodesFrame], [PrivateFrame],
// [TimestampFrame], [ChapterFrame], [ChapterTableOfContentsFrame], and
// [BinaryFrame].
type Frame interface {
	// ID returns the frame's ID.
	ID() FrameId
	// Flags returns the frame's flags.
	Flags() FrameFlags
	// SetFlags replaces the frame's flags.
	SetFlags(flags FrameFlags)

	// frameEqual mirrors the Rust PartialEq semantics used for deduplication:
	// identifying fields only for some types (e.g. description for TXXX/WXXX,
	// email for POPM, owner for UFID), full content equality for the rest.
	frameEqual(other Frame) bool
	// frameIsEmpty reports (isEmpty, known) — known is false for frame types
	// where emptiness is undefined.
	frameIsEmpty() (isEmpty, known bool)
	// frameBytes renders the frame content (no header). May return
	// (nil, nil) when the frame should be skipped (e.g. a duplicate rooted CTOC).
	frameBytes(ctx *frameEncodingContext) ([]byte, error)
	// frameName returns the variant name for error messages.
	frameName() string
	// cloneFrame returns a copy safe to mutate during writing.
	cloneFrame() Frame
}

// frameEncodingContext carries state needed while encoding frames.
type frameEncodingContext struct {
	version             Id3v2Version
	writeOptions        WriteOptions
	ctocRootEncountered bool
}

// frameAsBytes mirrors Rust's `Frame::as_bytes` dispatch.
func frameAsBytes(f Frame, ctx *frameEncodingContext) ([]byte, error) {
	if ctoc, ok := f.(*ChapterTableOfContentsFrame); ok && ctoc.TocFlags.TopLevel {
		if ctx.ctocRootEncountered {
			return nil, nil
		}
		ctx.ctocRootEncountered = true
	}
	ret, err := f.frameBytes(ctx)
	if err != nil {
		if fe, ok := err.(*FrameEncodingError); ok && fe.ID.IsNone() {
			fe.ID = Some(f.ID())
		}
		return nil, err
	}
	return ret, nil
}

// verifyFrame mirrors Rust's `write::frame::verify_frame` — each frame ID may
// only carry compatible content types.
func verifyFrame(f Frame) error {
	id := f.ID().String()
	// Flag frames are written as text but their content doesn't matter to
	// apps that rely on them — any variant is accepted under these IDs.
	if id == "TCMP" || id == "PCST" {
		return nil
	}
	bad := func() error {
		return frameEncodingErrorMsg(Some(f.ID()),
			"a value of type '"+f.frameName()+"' cannot be written under this ID")
	}
	switch f.(type) {
	case *AttachedPictureFrame:
		if id != "APIC" {
			return bad()
		}
	case *UnsynchronizedTextFrame:
		if id != "USLT" {
			return bad()
		}
	case *CommentFrame:
		if id != "COMM" {
			return bad()
		}
	case *ExtendedTextFrame:
		if id != "TXXX" {
			return bad()
		}
	case *ExtendedUrlFrame:
		if id != "WXXX" {
			return bad()
		}
	case *BinaryFrame:
		// any ID
	case *UniqueFileIdentifierFrame:
		if id != "UFID" {
			return bad()
		}
	case *PopularimeterFrame:
		if id != "POPM" {
			return bad()
		}
	case *KeyValueFrame:
		if id != "TIPL" && id != "TMCL" {
			return bad()
		}
	case *TimestampFrame:
		switch id {
		case "TDEN", "TDOR", "TDRC", "TDRL", "TDTG":
		default:
			return bad()
		}
	case *RelativeVolumeAdjustmentFrame:
		if id != "RVA2" {
			return bad()
		}
	case *PrivateFrame:
		if id != "PRIV" {
			return bad()
		}
	case *ChapterFrame:
		if id != "CHAP" {
			return bad()
		}
	case *ChapterTableOfContentsFrame:
		if id != "CTOC" {
			return bad()
		}
	case *TextInformationFrame:
		switch id {
		case "WFED", "GRP1", "MVNM", "MVIN", "TCMP", "PCST":
			// allowed non-'T' IDs
		default:
			if id[0] != 'T' {
				return bad()
			}
		}
	case *UrlLinkFrame:
		if id != "TCMP" && id != "PCST" && id[0] != 'W' {
			return bad()
		}
	default:
		return bad()
	}
	return nil
}

// decodeText is a shorthand for util.DecodeText.
var decodeText = util.DecodeText

// textDecodingError wraps a decode failure for frame parse errors.
func textDecodeOpts(enc TextEncoding, terminated bool) util.TextDecodeOptions {
	return util.TextDecodeOptions{Encoding: enc, Terminated: terminated}
}

// verifyEncoding mirrors `content::verify_encoding`.
func verifyEncoding(encoding uint8, version Id3v2Version) (TextEncoding, error) {
	if version == id3v2Version2 && encoding != 0 && encoding != 1 {
		return 0, frameParseErrorMsg(None[FrameId](),
			"ID3v2.2 only supports Latin-1 and UTF-16 encodings")
	}
	if encoding > 3 {
		return 0, frameParseError(None[FrameId](), &BadTextEncodingError{})
	}
	return TextEncoding(encoding), nil
}

const (
	id3v2Version2 = 2
	id3v2Version3 = meta.Id3v2Version3
	id3v2Version4 = meta.Id3v2Version4
)

func id3v2VersionString(v Id3v2Version) string {
	switch v {
	case id3v2Version2:
		return "ID3v2.2"
	case id3v2Version3:
		return "ID3v2.3"
	default:
		return "ID3v2.4"
	}
}
