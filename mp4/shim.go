package mp4

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

type (
	Tag                = meta.Tag
	TagItem            = meta.TagItem
	ItemKey            = meta.ItemKey
	ItemValue          = meta.ItemValue
	TagType            = meta.TagType
	Timestamp          = meta.Timestamp
	Picture            = meta.Picture
	PictureType        = meta.PictureType
	PictureInformation = meta.PictureInformation
	TaggedFile         = meta.TaggedFile
	VerifiedFile       = meta.VerifiedFile
	FileProperties     = meta.FileProperties
	FileType           = meta.FileType
	Popularimeter      = meta.Popularimeter
	MimeType           = meta.MimeType

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
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

var (
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

type (
	ChannelMask = meta.ChannelMask
)

const (
	TagTypeMp4Ilst = meta.TagTypeMp4Ilst
)

var (
	FileTypeMp4       = meta.FileTypeMp4
	NewFileProperties = meta.NewFileProperties
)

const (
	PictureTypeOther = meta.PictureTypeOther
)

var (
	Splice         = meta.Splice
	ItemKeyFromKey = meta.ItemKeyFromKey
	NewTag         = meta.NewTag
)

const (
	ItemKeyPopularimeter = meta.ItemKeyPopularimeter
)

var (
	ItemValueText       = meta.ItemValueText
	NewTagItem          = meta.NewTagItem
	PopularimeterMapped = meta.PopularimeterMapped
)

const (
	ItemKeyDiscNumber       = meta.ItemKeyDiscNumber
	ItemKeyDiscTotal        = meta.ItemKeyDiscTotal
	ItemKeyParentalAdvisory = meta.ItemKeyParentalAdvisory
	ItemKeyTrackNumber      = meta.ItemKeyTrackNumber
	ItemKeyTrackTotal       = meta.ItemKeyTrackTotal
)

const (
	ItemKeyFlagCompilation = meta.ItemKeyFlagCompilation
	ItemKeyUnknown         = meta.ItemKeyUnknown
)

var (
	divRound         = util.DivRound
	itoa             = util.Itoa
	getGlobalOptions = util.GetGlobalOptions
)

func optionFromPtr[T any](p *T) Option[*T]  { return util.OptionFromPtr(p) }
func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

const ItemKeyFlagPodcast = meta.ItemKeyFlagPodcast

var (
	PopularimeterFromStr = meta.PopularimeterFromStr
	MimeTypePng          = meta.MimeTypePng
	MimeTypeBmp          = meta.MimeTypeBmp
	MimeTypeGif          = meta.MimeTypeGif
	u32FromString        = util.U32FromString
	utf16Encode          = util.Utf16Encode
	bigEndian            = util.BigEndian
)

func newNotEnoughDataError(expected int) *meta.NotEnoughDataError {
	return util.NewNotEnoughDataError(expected)
}

var (
	MimeTypeJpeg      = meta.MimeTypeJpeg
	PictureUnchecked  = meta.PictureUnchecked
	tryParseTimestamp = util.TryParseTimestamp
	utf16DecodeBytes  = util.Utf16DecodeBytes
)

const (
	ItemKeyAlbumArtist = meta.ItemKeyAlbumArtist
	ItemKeyAlbumTitle  = meta.ItemKeyAlbumTitle
	ItemKeyComment     = meta.ItemKeyComment
	ItemKeyTrackArtist = meta.ItemKeyTrackArtist
	ItemKeyTrackTitle  = meta.ItemKeyTrackTitle
)

var (
	NewParseOptions = meta.NewParseOptions
	NewWriteOptions = meta.NewWriteOptions
)

const (
	ItemKeyContentGroup = meta.ItemKeyContentGroup
	ItemKeyGenre        = meta.ItemKeyGenre
	ItemKeyMood         = meta.ItemKeyMood
	ItemKeyComposer     = meta.ItemKeyComposer
	ItemKeyConductor    = meta.ItemKeyConductor
	ItemKeyTrackArtists = meta.ItemKeyTrackArtists
)

var NewOSFile = meta.NewOSFile
