package iff

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

type (
	Tag            = meta.Tag
	TagItem        = meta.TagItem
	ItemKey        = meta.ItemKey
	ItemValue      = meta.ItemValue
	TagType        = meta.TagType
	Timestamp      = meta.Timestamp
	TaggedFile     = meta.TaggedFile
	VerifiedFile   = meta.VerifiedFile
	FileProperties = meta.FileProperties
	FileType       = meta.FileType
	Id3v2Header    = id3v2.Id3v2Header
	ChannelMask    = meta.ChannelMask

	Option[T any]       = meta.Option[T]
	ParseOptions        = meta.ParseOptions
	WriteOptions        = meta.WriteOptions
	ParsingMode         = meta.ParsingMode
	FileLike            = meta.FileLike
	AudioFile           = meta.AudioFile
	TagParseError       = meta.TagParseError
	TagEncodingError    = meta.TagEncodingError
	FileEncodingError   = meta.FileEncodingError
	FileParseError      = meta.FileParseError
	FakeTagError        = meta.FakeTagError
	TooMuchDataError    = meta.TooMuchDataError
	UnsupportedTagError = meta.UnsupportedTagError
	TextEncoding        = meta.TextEncoding
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

var (
	ParseId3v2           = id3v2.ParseId3v2
	ParseId3v2HeaderRaw  = id3v2.ParseId3v2HeaderRaw
	WriteId3v2           = id3v2.WriteId3v2
	Id3v2Serialize       = id3v2.Id3v2Serialize
	FindId3v2            = id3v2.FindId3v2
	RegisterFormatReader = meta.RegisterFormatReader
	RegisterFormatWriter = meta.RegisterFormatWriter
	RegisterTagDumper    = meta.RegisterTagDumper
	SaveToPath           = meta.SaveToPath
	RemoveTagFromPath    = meta.RemoveTagFromPath
	SaveTagFields        = meta.SaveTagFields
	NewTaggedFile        = meta.NewTaggedFile
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

const (
	ItemKeyUnknown  = meta.ItemKeyUnknown
	TagTypeRiffInfo = meta.TagTypeRiffInfo
)

var (
	ItemKeyOrUnknown = meta.ItemKeyOrUnknown
	NewTag           = meta.NewTag
)

const (
	TagTypeAiffText = meta.TagTypeAiffText
)

var (
	ItemValueText = meta.ItemValueText
	NewTagItem    = meta.NewTagItem
)

const (
	ItemKeyCopyrightMessage = meta.ItemKeyCopyrightMessage
	ItemKeyTrackArtist      = meta.ItemKeyTrackArtist
	ItemKeyTrackTitle       = meta.ItemKeyTrackTitle
)

const (
	ItemKeyComment = meta.ItemKeyComment
)

var (
	getGlobalOptions  = util.GetGlobalOptions
	itoa              = util.Itoa
	tryParseTimestamp = util.TryParseTimestamp
	u32FromString     = util.U32FromString
	utf8Decode        = util.Utf8Decode
)

func optionFromPtr[T any](p *T) Option[*T] { return util.OptionFromPtr(p) }

var (
	NewFileProperties = meta.NewFileProperties
	divRound          = util.DivRound
	latin1Decode      = util.Latin1Decode
	trimEndNulls      = util.TrimEndNulls
)

const TagTypeID3v2 = meta.TagTypeID3v2

var (
	FileTypeWav    = meta.FileTypeWav
	Splice         = meta.Splice
	f80FromBEBytes = util.F80FromBEBytes
)

var FileTypeAiff = meta.FileTypeAiff

func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

var (
	NewParseOptions = meta.NewParseOptions
	NewWriteOptions = meta.NewWriteOptions
)

// Aliases for concrete-tag writes (Rust `TagExt` parity).
type (
	Id3v2Tag   = id3v2.Id3v2Tag
	TagSupport = meta.TagSupport
)

const (
	TagSupportUnsupported = meta.TagSupportUnsupported
)

var (
	Id3v2TagFromTag           = id3v2.Id3v2TagFromTag
	NewVerifiedFile           = meta.NewVerifiedFile
	VerifiedFileFromPath      = meta.VerifiedFileFromPath
	WriteConcreteTag          = meta.WriteConcreteTag
	RegisterConcreteTagWriter = meta.RegisterConcreteTagWriter
)

var parseId3v2Tag = id3v2.ParseId3v2Tag
