package id3v2

import (
	"io"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Public frame-level API mirroring Rust `Frame::parse` and
// `XxxFrame::as_bytes`/`parse`.

// ParseFrame parses a single frame's content from reader, mirroring Rust
// `Frame::parse` (the tag-level `parse_frame` dispatch). Returns a nil frame
// for unsupported/malformed content (Rust's `meta.Option::None`).
func ParseFrame(reader io.Reader, id FrameId, size uint32, frameFlags FrameFlags, version Id3v2Version, parseOptions meta.ParseOptions) (Frame, error) {
	f, result, err := parseFrame(reader, id, size, frameFlags, version, parseOptions)
	if err != nil || result != parsedFrameNext {
		return nil, err
	}
	return f, nil
}

// ParseTextInformationFrame mirrors Rust `TextInformationFrame::parse`.
func ParseTextInformationFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, version Id3v2Version) (*TextInformationFrame, error) {
	return parseTextInformationFrame(reader, id, frameFlags, version)
}

// AsBytes serializes the frame content, mirroring Rust `as_bytes`.
func (f *TextInformationFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseUrlLinkFrame mirrors Rust `UrlLinkFrame::parse`.
func ParseUrlLinkFrame(reader io.Reader, id FrameId, frameFlags FrameFlags) (*UrlLinkFrame, error) {
	return parseUrlLinkFrame(reader, id, frameFlags)
}

// AsBytes mirrors Rust `UrlLinkFrame::as_bytes`.
func (f *UrlLinkFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseCommentFrame mirrors Rust `CommentFrame::parse`.
func ParseCommentFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*CommentFrame, error) {
	return parseCommentFrame(reader, frameFlags, version)
}

// AsBytes mirrors Rust `CommentFrame::as_bytes`.
func (f *CommentFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseUnsynchronizedTextFrame mirrors Rust `UnsynchronizedTextFrame::parse`.
func ParseUnsynchronizedTextFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*UnsynchronizedTextFrame, error) {
	return parseUnsynchronizedTextFrame(reader, frameFlags, version)
}

// AsBytes mirrors Rust `UnsynchronizedTextFrame::as_bytes`.
func (f *UnsynchronizedTextFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseExtendedTextFrame mirrors Rust `ExtendedTextFrame::parse`.
func ParseExtendedTextFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*ExtendedTextFrame, error) {
	return parseExtendedTextFrame(reader, frameFlags, version)
}

// AsBytes mirrors Rust `ExtendedTextFrame::as_bytes`.
func (f *ExtendedTextFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseExtendedUrlFrame mirrors Rust `ExtendedUrlFrame::parse`.
func ParseExtendedUrlFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*ExtendedUrlFrame, error) {
	return parseExtendedUrlFrame(reader, frameFlags, version)
}

// AsBytes mirrors Rust `ExtendedUrlFrame::as_bytes`.
func (f *ExtendedUrlFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseKeyValueFrame mirrors Rust `KeyValueFrame::parse`.
func ParseKeyValueFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, version Id3v2Version) (*KeyValueFrame, error) {
	return parseKeyValueFrame(reader, id, frameFlags, version)
}

// AsBytes mirrors Rust `KeyValueFrame::as_bytes`.
func (f *KeyValueFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParsePopularimeterFrame mirrors Rust `PopularimeterFrame::parse`.
func ParsePopularimeterFrame(reader io.Reader, frameFlags FrameFlags) (*PopularimeterFrame, error) {
	return parsePopularimeterFrame(reader, frameFlags)
}

// AsBytes mirrors Rust `PopularimeterFrame::as_bytes`.
func (f *PopularimeterFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseUniqueFileIdentifierFrame mirrors Rust `UniqueFileIdentifierFrame::parse`.
func ParseUniqueFileIdentifierFrame(reader io.Reader, frameFlags FrameFlags, parseMode meta.ParsingMode) (*UniqueFileIdentifierFrame, error) {
	return parseUniqueFileIdentifierFrame(reader, frameFlags, parseMode)
}

// AsBytes mirrors Rust `UniqueFileIdentifierFrame::as_bytes`.
func (f *UniqueFileIdentifierFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseOwnershipFrame mirrors Rust `OwnershipFrame::parse`.
func ParseOwnershipFrame(reader io.Reader, frameFlags FrameFlags) (*OwnershipFrame, error) {
	return parseOwnershipFrame(reader, frameFlags)
}

// AsBytes mirrors Rust `OwnershipFrame::as_bytes`.
func (f *OwnershipFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseTimestampFrame mirrors Rust `TimestampFrame::parse`.
func ParseTimestampFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, parseMode meta.ParsingMode) (*TimestampFrame, error) {
	return parseTimestampFrame(reader, id, frameFlags, parseMode)
}

// AsBytes mirrors Rust `TimestampFrame::as_bytes`.
func (f *TimestampFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseEventTimingCodesFrame mirrors Rust `EventTimingCodesFrame::parse`.
func ParseEventTimingCodesFrame(reader io.Reader, frameFlags FrameFlags) (*EventTimingCodesFrame, error) {
	return parseEventTimingCodesFrame(reader, frameFlags)
}

// AsBytes mirrors Rust `EventTimingCodesFrame::as_bytes`.
func (f *EventTimingCodesFrame) AsBytes() ([]byte, error) {
	return f.frameBytes(nil)
}

// ParsePrivateFrame mirrors Rust `PrivateFrame::parse`.
func ParsePrivateFrame(reader io.Reader, frameFlags FrameFlags) (*PrivateFrame, error) {
	return parsePrivateFrame(reader, frameFlags)
}

// AsBytes mirrors Rust `PrivateFrame::as_bytes`.
func (f *PrivateFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseRelativeVolumeAdjustmentFrame mirrors Rust `RelativeVolumeAdjustmentFrame::parse`.
func ParseRelativeVolumeAdjustmentFrame(reader io.Reader, frameFlags FrameFlags, parseMode meta.ParsingMode) (*RelativeVolumeAdjustmentFrame, error) {
	return parseRelativeVolumeAdjustmentFrame(reader, frameFlags, parseMode)
}

// AsBytes mirrors Rust `RelativeVolumeAdjustmentFrame::as_bytes`.
func (f *RelativeVolumeAdjustmentFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseAttachedPictureFrame mirrors Rust `AttachedPictureFrame::parse`.
func ParseAttachedPictureFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*AttachedPictureFrame, error) {
	return parseAttachedPictureFrame(reader, frameFlags, version)
}

// AsBytes mirrors Rust `AttachedPictureFrame::as_bytes`.
func (f *AttachedPictureFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseChapterFrame mirrors Rust `ChapterFrame::parse`.
func ParseChapterFrame(reader io.Reader, parseOptions meta.ParseOptions, version Id3v2Version, frameFlags FrameFlags) (*ChapterFrame, error) {
	return parseChapterFrame(reader, parseOptions, version, frameFlags)
}

// AsBytes mirrors Rust `ChapterFrame::as_bytes`.
func (f *ChapterFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

// ParseChapterTableOfContentsFrame mirrors Rust `ChapterTableOfContentsFrame::parse`.
func ParseChapterTableOfContentsFrame(reader io.Reader, parseOptions meta.ParseOptions, version Id3v2Version, frameFlags FrameFlags) (*ChapterTableOfContentsFrame, error) {
	return parseChapterTableOfContentsFrame(reader, parseOptions, version, frameFlags)
}

// AsBytes mirrors Rust `ChapterTableOfContentsFrame::as_bytes`.
func (f *ChapterTableOfContentsFrame) AsBytes(writeOptions meta.WriteOptions) ([]byte, error) {
	return f.frameBytes(&frameEncodingContext{version: writeOptionsVersion(writeOptions), writeOptions: writeOptions})
}

func writeOptionsVersion(writeOptions meta.WriteOptions) Id3v2Version {
	if writeOptions.UseID3v23 {
		return meta.Id3v2Version3
	}
	return meta.Id3v2Version4
}
