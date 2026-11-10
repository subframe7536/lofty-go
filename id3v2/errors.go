package id3v2

import "fmt"

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

// Id3v2HeaderError is the kind of error that can occur while parsing an ID3v2 header.
type Id3v2HeaderError struct {
	Message string
}

func (e *Id3v2HeaderError) Error() string { return e.Message }

var (
	errBadExtendedHeaderSize  = &Id3v2HeaderError{Message: "encountered an extended header with an invalid size"}
	errBadExtendedHeaderFlags = &Id3v2HeaderError{Message: "encountered an extended header with a invalid flags"}
	errV2Compression          = &Id3v2HeaderError{Message: "encountered a compressed ID3v2.2 tag"}
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
	ID      Option[FrameId]
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

func frameParseError(id Option[FrameId], source error) *FrameParseError {
	return &FrameParseError{ID: id, Source: source}
}

func frameParseErrorMsg(id Option[FrameId], message string) *FrameParseError {
	return &FrameParseError{ID: id, Message: message}
}

func frameParseUndersized(id FrameId) *FrameParseError {
	return frameParseErrorMsg(Some(id), "frame isn't long enough to extract the necessary information")
}

func frameParseMissingDLI(id FrameId) *FrameParseError {
	return frameParseErrorMsg(Some(id), "encountered an encrypted frame without a data length indicator")
}

func frameParseInvalidLanguage(language [3]byte) *FrameParseError {
	return frameParseErrorMsg(None[FrameId](),
		fmt.Sprintf("invalid frame language found: %q (expected 3 ascii characters)", language))
}

func frameParseUnsupportedVersion(version Id3v2Version) *FrameParseError {
	return frameParseErrorMsg(None[FrameId](),
		fmt.Sprintf("attempted to read frame in version %s", id3v2VersionString(version)))
}

// FrameEncodingError is returned when an ID3v2 frame fails to encode.
type FrameEncodingError struct {
	ID      Option[FrameId]
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

func frameEncodingError(id Option[FrameId], source error) *FrameEncodingError {
	return &FrameEncodingError{ID: id, Source: source}
}

func frameEncodingErrorMsg(id Option[FrameId], message string) *FrameEncodingError {
	return &FrameEncodingError{ID: id, Message: message}
}

func frameEncodingMissingDLI(id FrameId) *FrameEncodingError {
	return frameEncodingErrorMsg(Some(id), "encountered an encrypted frame without a data length indicator")
}

func frameEncodingInvalidLanguage(language [3]byte) *FrameEncodingError {
	return frameEncodingErrorMsg(None[FrameId](),
		fmt.Sprintf("invalid frame language found: %q (expected 3 ascii characters)", language))
}

func frameEncodingUnsupportedVersion(version Id3v2Version) *FrameEncodingError {
	return frameEncodingErrorMsg(None[FrameId](),
		fmt.Sprintf("attempted to write frame in version %s", id3v2VersionString(version)))
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
