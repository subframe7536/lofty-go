package lyrics3

// Internal aliases bridging this package back to the lofty model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

type (
	Tag       = meta.Tag
	TagItem   = meta.TagItem
	ItemKey   = meta.ItemKey
	TagType   = meta.TagType
	Timestamp = meta.Timestamp

	Option[T any]     = meta.Option[T]
	ParseOptions      = meta.ParseOptions
	WriteOptions      = meta.WriteOptions
	ParsingMode       = meta.ParsingMode
	FileLike          = meta.FileLike
	TagParseError     = meta.TagParseError
	TagEncodingError  = meta.TagEncodingError
	FileEncodingError = meta.FileEncodingError
	FakeTagError      = meta.FakeTagError
	TooMuchDataError  = meta.TooMuchDataError
	SizeMismatchError = meta.SizeMismatchError
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

const (
	TagTypeLyrics3   = meta.TagTypeLyrics3
	TagTypeLyrics3v2 = meta.TagTypeLyrics3v2
)

const (
	ItemKeyTrackTitle  = meta.ItemKeyTrackTitle
	ItemKeyTrackArtist = meta.ItemKeyTrackArtist
	ItemKeyAlbumTitle  = meta.ItemKeyAlbumTitle
	ItemKeyComment     = meta.ItemKeyComment
	ItemKeyLyrics      = meta.ItemKeyLyrics
	ItemKeyLyricist    = meta.ItemKeyLyricist
)

var (
	ItemValueText    = meta.ItemValueText
	NewTag           = meta.NewTag
	NewTagItem       = meta.NewTagItem
	Splice           = meta.Splice
	getGlobalOptions = util.GetGlobalOptions
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

var (
	newRevPatternSearcher = util.NewRevPatternSearcher
	revSearchFromCurrent  = util.RevSearchFromCurrent
	revSearchEndAtPos     = util.RevSearchEndAtPos
	asciiNumber           = util.AsciiNumber
	latin1Encode          = util.Latin1Encode
	latin1Decode          = util.Latin1Decode
)

var (
	NewParseOptions = meta.NewParseOptions
	NewWriteOptions = meta.NewWriteOptions
)
