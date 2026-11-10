package meta

// Internal aliases to internal/util so that files moved from the original
// single-package layout keep compiling unchanged.

import (
	"github.com/subframe7536/lofty-go/internal/util"
)

// --- option.go ---

type Option[T any] = util.Option[T]

func Some[T any](val T) Option[T] { return util.Some(val) }
func None[T any]() Option[T]      { return util.None[T]() }

var stringToOption = util.StringToOption

// --- config.go ---

type (
	ParsingMode   = util.ParsingMode
	ParseOptions  = util.ParseOptions
	WriteOptions  = util.WriteOptions
	GlobalOptions = util.GlobalOptions
)

const (
	ParsingModeBestAttempt  = util.ParsingModeBestAttempt
	ParsingModeStrict       = util.ParsingModeStrict
	ParsingModeRelaxed      = util.ParsingModeRelaxed
	DefaultMaxJunkBytes     = util.DefaultMaxJunkBytes
	DefaultPreferredPadding = util.DefaultPreferredPadding
	DefaultAllocationLimit  = util.DefaultAllocationLimit
)

var (
	NewParseOptions  = util.NewParseOptions
	NewWriteOptions  = util.NewWriteOptions
	NewGlobalOptions = util.NewGlobalOptions
	getGlobalOptions = util.GetGlobalOptions
)

// --- leaf error types ---

type (
	FakeTagError        = util.FakeTagError
	TooMuchDataError    = util.TooMuchDataError
	NotEnoughDataError  = util.NotEnoughDataError
	SizeMismatchError   = util.SizeMismatchError
	UnknownFormatError  = util.UnknownFormatError
	UnsupportedTagError = util.UnsupportedTagError
	AllocationErrorKind = util.AllocationErrorKind
	AllocationError     = util.AllocationError
)

const (
	AllocationLimitExceeded = util.AllocationLimitExceeded
	AllocationReserveError  = util.AllocationReserveError
)

var errNotEnoughData = util.ErrNotEnoughData

func newNotEnoughDataError(expected int) *NotEnoughDataError {
	return util.NewNotEnoughDataError(expected)
}

// --- util_io.go ---

type (
	FileLike  = util.FileLike
	OSFile    = util.OSFile
	BytesFile = util.BytesFile
)

var (
	Splice       = util.Splice
	NewOSFile    = util.NewOSFile
	NewBytesFile = util.NewBytesFile
)

type (
	F80                = util.F80
	RevPatternSearcher = util.RevPatternSearcher
	RevSearchStart     = util.RevSearchStart
	RevSearchEnd       = util.RevSearchEnd
)

// --- util_text.go ---

type (
	TextEncoding         = util.TextEncoding
	TextEncodingError    = util.TextEncodingError
	TextDecodingError    = util.TextDecodingError
	BadTextEncodingError = util.BadTextEncodingError
	ByteOrder            = util.ByteOrder
	DecodeTextResult     = util.DecodeTextResult
	TextDecodeOptions    = util.TextDecodeOptions
	ByteReaderAdapter    = util.ByteReaderAdapter
)

const (
	TextEncodingLatin1  = util.TextEncodingLatin1
	TextEncodingUTF16   = util.TextEncodingUTF16
	TextEncodingUTF16BE = util.TextEncodingUTF16BE
	TextEncodingUTF8    = util.TextEncodingUTF8
)

var utf8DecodeStr = util.Utf8DecodeStr

// --- util_alloc.go ---

func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

// --- util_helpers.go ---

// --- timestamp.go ---

type (
	Timestamp               = util.Timestamp
	TimestampField          = util.TimestampField
	TimestampParseError     = util.TimestampParseError
	TimestampParseErrorKind = util.TimestampParseErrorKind
)

const TimestampMaxLength = util.TimestampMaxLength

const (
	TimestampFieldYear   = util.TimestampFieldYear
	TimestampFieldMonth  = util.TimestampFieldMonth
	TimestampFieldDay    = util.TimestampFieldDay
	TimestampFieldHour   = util.TimestampFieldHour
	TimestampFieldMinute = util.TimestampFieldMinute
	TimestampFieldSecond = util.TimestampFieldSecond
)

const (
	TimestampParseErrorEmpty            = util.TimestampParseErrorEmpty
	TimestampParseErrorBadYear          = util.TimestampParseErrorBadYear
	TimestampParseErrorMissingSeparator = util.TimestampParseErrorMissingSeparator
	TimestampParseErrorSegmentTooShort  = util.TimestampParseErrorSegmentTooShort
	TimestampParseErrorContainsSpaces   = util.TimestampParseErrorContainsSpaces
	TimestampParseErrorUnexpectedChar   = util.TimestampParseErrorUnexpectedChar
	TimestampParseErrorFieldTooLarge    = util.TimestampParseErrorFieldTooLarge
	TimestampParseErrorIo               = util.TimestampParseErrorIo
)

var (
	TimestampParse       = util.TimestampParse
	TimestampParseString = util.TimestampParseString
	tryParseTimestamp    = util.TryParseTimestamp
)

// --- synchsafe / frame sync ---

var (
	unsynchsafe32             = util.Unsynchsafe32
	verifyFrameSync           = util.VerifyFrameSync
	searchForFrameSyncSeeking = util.SearchForFrameSyncSeeking
)
