package vorbis

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

type (
	Tag             = meta.Tag
	TagItem         = meta.TagItem
	ItemKey         = meta.ItemKey
	ItemValue       = meta.ItemValue
	TagType         = meta.TagType
	Timestamp       = meta.Timestamp
	Picture         = meta.Picture
	PictureType     = meta.PictureType
	PictureInfoPair = meta.PictureInfoPair
	Lang            = meta.Lang

	Option[T any]    = meta.Option[T]
	ParseOptions     = meta.ParseOptions
	WriteOptions     = meta.WriteOptions
	ParsingMode      = meta.ParsingMode
	FileLike         = meta.FileLike
	TagParseError    = meta.TagParseError
	TagEncodingError = meta.TagEncodingError
	TooMuchDataError = meta.TooMuchDataError
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

const (
	TagTypeVorbisComments = meta.TagTypeVorbisComments
)

var (
	ItemValueText       = meta.ItemValueText
	ItemValueBinary     = meta.ItemValueBinary
	ItemValueKindText   = meta.ItemValueKindText
	ItemValueKindBinary = meta.ItemValueKindBinary
	NewTag              = meta.NewTag
	NewTagItem          = meta.NewTagItem
	NewTagItemChecked   = meta.NewTagItemChecked
	ItemKeyOrUnknown    = meta.ItemKeyOrUnknown
	Splice              = meta.Splice
	getGlobalOptions    = util.GetGlobalOptions
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

var (
	itoa              = util.Itoa
	u32FromString     = util.U32FromString
	tryParseTimestamp = util.TryParseTimestamp
)

type (
	PictureInformation = meta.PictureInformation
)

const (
	ItemKeyUnknown = meta.ItemKeyUnknown
)

var PictureInformationFromPicture = meta.PictureInformationFromPicture

const (
	ItemKeyDiscNumber      = meta.ItemKeyDiscNumber
	ItemKeyEncoderSoftware = meta.ItemKeyEncoderSoftware
	ItemKeyFlagCompilation = meta.ItemKeyFlagCompilation
	ItemKeyPopularimeter   = meta.ItemKeyPopularimeter
	ItemKeyTrackNumber     = meta.ItemKeyTrackNumber
	PictureTypeCoverFront  = meta.PictureTypeCoverFront
)

var PictureUnchecked = meta.PictureUnchecked

var mimetypeFromBin = meta.MimetypeFromBin

func tryCapSlice[T any](capacity int) ([]T, error) { return util.TryCapSlice[T](capacity) }

var PictureFromFlacBytes = meta.PictureFromFlacBytes

var flagItem = util.FlagItem
