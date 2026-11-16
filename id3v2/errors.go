package id3v2

import (
	"fmt"

	meta "github.com/subframe7536/lofty-go/internal/meta"
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

// Id3v2HeaderErrorKind is the kind of an [Id3v2HeaderError], mirroring the
// variants of Rust's `Id3v2HeaderError`.
type Id3v2HeaderErrorKind int

const (
	// Id3v2HeaderErrorOther is an unclassified header error described by Message.
	Id3v2HeaderErrorOther Id3v2HeaderErrorKind = iota
	// Id3v2HeaderBadVersion is an invalid ID3v2 version (`BadId3v2Version`);
	// Major and Minor hold the version bytes.
	Id3v2HeaderBadVersion
	// Id3v2HeaderV2Compression is a compressed ID3v2.2 tag (`V2Compression`).
	Id3v2HeaderV2Compression
	// Id3v2HeaderBadExtendedHeaderSize is an extended header with an invalid size.
	Id3v2HeaderBadExtendedHeaderSize
	// Id3v2HeaderBadExtendedHeaderFlags is an extended header with reserved flags.
	Id3v2HeaderBadExtendedHeaderFlags
)

// Id3v2HeaderError is the kind of error that can occur while parsing an ID3v2 header.
type Id3v2HeaderError struct {
	Kind Id3v2HeaderErrorKind
	// Major and Minor are the version bytes for [Id3v2HeaderBadVersion].
	Major, Minor uint8
	// Message overrides the default text for the kind, if set.
	Message string
}

func (e *Id3v2HeaderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	switch e.Kind {
	case Id3v2HeaderBadVersion:
		return fmt.Sprintf("found an invalid version (v%d.%d), expected any major revision in: (2, 3, 4)", e.Major, e.Minor)
	case Id3v2HeaderV2Compression:
		return "encountered a compressed ID3v2.2 tag"
	case Id3v2HeaderBadExtendedHeaderSize:
		return "encountered an extended header with an invalid size"
	case Id3v2HeaderBadExtendedHeaderFlags:
		return "encountered an extended header with a invalid flags"
	}
	return "invalid ID3v2 header"
}

var (
	errBadExtendedHeaderSize  = &Id3v2HeaderError{Kind: Id3v2HeaderBadExtendedHeaderSize}
	errBadExtendedHeaderFlags = &Id3v2HeaderError{Kind: Id3v2HeaderBadExtendedHeaderFlags}
	errV2Compression          = &Id3v2HeaderError{Kind: Id3v2HeaderV2Compression}
)

// FrameIdParseError is returned when converting a string to a [FrameId] fails.
type FrameIdParseError struct {
	IdBytes []byte
}

func (e *FrameIdParseError) Error() string {
	return fmt.Sprintf("failed to parse a frame ID: 0x%x", e.IdBytes)
}

// FrameParseError is returned when an ID3v2 frame fails to parse.
type FrameParseError struct {
	ID      meta.Option[FrameId]
	Message string
	Source  error
}

func (e *FrameParseError) Error() string {
	if id, ok := e.ID.Get(); ok {
		return fmt.Sprintf("failed to parse frame '%s'", id.String())
	}
	return "failed to parse a frame"
}
func (e *FrameParseError) Unwrap() error { return e.Source }

func frameParseError(id meta.Option[FrameId], source error) *FrameParseError {
	return &FrameParseError{ID: id, Source: source}
}

func frameParseErrorMsg(id meta.Option[FrameId], message string) *FrameParseError {
	return &FrameParseError{ID: id, Message: message}
}

func frameParseUndersized(id FrameId) *FrameParseError {
	return frameParseErrorMsg(meta.Some(id), "frame isn't long enough to extract the necessary information")
}

func frameParseMissingDLI(id FrameId) *FrameParseError {
	return frameParseErrorMsg(meta.Some(id), "encountered an encrypted frame without a data length indicator")
}

func frameParseInvalidLanguage(language [3]byte) *FrameParseError {
	return frameParseErrorMsg(meta.None[FrameId](),
		fmt.Sprintf("invalid frame language found: %q (expected 3 ascii characters)", language))
}

func frameParseUnsupportedVersion(version Id3v2Version) *FrameParseError {
	return frameParseErrorMsg(meta.None[FrameId](),
		"attempted to read frame in version "+id3v2VersionString(version))
}

// FrameEncodingError is returned when an ID3v2 frame fails to encode.
type FrameEncodingError struct {
	ID      meta.Option[FrameId]
	Message string
	Source  error
}

func (e *FrameEncodingError) Error() string {
	if id, ok := e.ID.Get(); ok {
		return fmt.Sprintf("failed to write frame '%s'", id.String())
	}
	return "failed to write a frame"
}
func (e *FrameEncodingError) Unwrap() error { return e.Source }

func frameEncodingError(id meta.Option[FrameId], source error) *FrameEncodingError {
	return &FrameEncodingError{ID: id, Source: source}
}

func frameEncodingErrorMsg(id meta.Option[FrameId], message string) *FrameEncodingError {
	return &FrameEncodingError{ID: id, Message: message}
}

func frameEncodingMissingDLI(id FrameId) *FrameEncodingError {
	return frameEncodingErrorMsg(meta.Some(id), "encountered an encrypted frame without a data length indicator")
}

func frameEncodingInvalidLanguage(language [3]byte) *FrameEncodingError {
	return frameEncodingErrorMsg(meta.None[FrameId](),
		fmt.Sprintf("invalid frame language found: %q (expected 3 ascii characters)", language))
}

func frameEncodingUnsupportedVersion(version Id3v2Version) *FrameEncodingError {
	return frameEncodingErrorMsg(meta.None[FrameId](),
		"attempted to write frame in version "+id3v2VersionString(version))
}

// SynchOverflowError is returned when an integer is too large to create a synchsafe representation.
type SynchOverflowError struct{}

func (e *SynchOverflowError) Error() string {
	return "integer too large to create a synchsafe representation"
}

const synchsafeMax = 0x0FFFFFFF

func synchsafe32Checked(v uint32) (uint32, error) {
	if v > synchsafeMax {
		return 0, &SynchOverflowError{}
	}
	return (v & 0x7F) | ((v & (0x7F << 7)) << 1) | ((v & (0x7F << 14)) << 2) | ((v & (0x7F << 21)) << 3), nil
}

// BadTimestampFormatError is returned when decoding a [TimestampFormat] with an invalid type.
type BadTimestampFormatError struct{}

func (e *BadTimestampFormatError) Error() string {
	return "encountered an invalid timestamp format in a synchronized frame"
}

// BadSyncTextContentTypeError is returned for an invalid content type in a [SynchronizedTextFrame].
type BadSyncTextContentTypeError struct{}

func (e *BadSyncTextContentTypeError) Error() string {
	return "encountered an invalid synchronized text content type"
}

// BadRva2ChannelTypeError is returned for an invalid channel type in an RVA2 frame.
type BadRva2ChannelTypeError struct{}

func (e *BadRva2ChannelTypeError) Error() string {
	return "encountered invalid channel type in RVA2 frame"
}
