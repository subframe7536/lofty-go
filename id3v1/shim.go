package id3v1

// Internal aliases bridging this package back to the lofty model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
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
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

const (
	TagTypeID3v1 = meta.TagTypeID3v1
)

const (
	ItemKeyTrackTitle  = meta.ItemKeyTrackTitle
	ItemKeyTrackArtist = meta.ItemKeyTrackArtist
	ItemKeyAlbumTitle  = meta.ItemKeyAlbumTitle
	ItemKeyComment     = meta.ItemKeyComment
	ItemKeyGenre       = meta.ItemKeyGenre
	ItemKeyTrackNumber = meta.ItemKeyTrackNumber
	ItemKeyYear        = meta.ItemKeyYear
)

var (
	ItemValueText = meta.ItemValueText
	NewTag        = meta.NewTag
	NewTagItem    = meta.NewTagItem
	Splice        = meta.Splice
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

var (
	findLyrics3  = lyrics3.FindLyrics3
	asciiNumber  = util.AsciiNumber
	latin1Encode = util.Latin1Encode
	latin1Decode = util.Latin1Decode
	itoa         = util.Itoa
	atoi         = util.Atoi
)

// Aliases for concrete-tag writes (Rust `TagExt` parity).
type (
	UnsupportedTagError = meta.UnsupportedTagError
	VerifiedFile        = meta.VerifiedFile
	TagSupport          = meta.TagSupport
)

const (
	TagSupportUnsupported = meta.TagSupportUnsupported
)

var (
	NewVerifiedFile      = meta.NewVerifiedFile
	VerifiedFileFromPath = meta.VerifiedFileFromPath
)
