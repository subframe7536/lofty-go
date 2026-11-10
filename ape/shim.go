package ape

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
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
	MimeType        = meta.MimeType
	TaggedFile      = meta.TaggedFile
	VerifiedFile    = meta.VerifiedFile
	FileProperties  = meta.FileProperties
	FileType        = meta.FileType

	Id3v1Tag   = id3v1.Id3v1Tag
	Lyrics3Tag = lyrics3.Lyrics3Tag

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
	NotEnoughDataError  = meta.NotEnoughDataError
	SizeMismatchError   = meta.SizeMismatchError
	UnknownFormatError  = meta.UnknownFormatError
	UnsupportedTagError = meta.UnsupportedTagError
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

var FileTypeApe = meta.FileTypeApe

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

var (
	findId3v1   = id3v1.FindId3v1
	findLyrics3 = lyrics3.FindLyrics3
	findId3v2   = id3v2.FindId3v2

	WriteId3v1           = id3v1.WriteId3v1
	WriteId3v2           = id3v2.WriteId3v2
	Id3v1TagFromTag      = id3v1.Id3v1TagFromTag
	RegisterFormatReader = meta.RegisterFormatReader
	RegisterFormatWriter = meta.RegisterFormatWriter
	RegisterTagDumper    = meta.RegisterTagDumper
	SaveToPath           = meta.SaveToPath
	RemoveTagFromPath    = meta.RemoveTagFromPath
	SaveTagFields        = meta.SaveTagFields
	NewTaggedFile        = meta.NewTaggedFile
)

type (
	ItemValueKind = meta.ItemValueKind
)

const (
	ItemKeyDiscNumber    = meta.ItemKeyDiscNumber
	ItemKeyDiscTotal     = meta.ItemKeyDiscTotal
	ItemKeyTrackNumber   = meta.ItemKeyTrackNumber
	ItemKeyTrackTotal    = meta.ItemKeyTrackTotal
	ItemKeyUnknown       = meta.ItemKeyUnknown
	ItemValueKindLocator = meta.ItemValueKindLocator
)

var (
	ItemValueLocator = meta.ItemValueLocator
	ItemValueText    = meta.ItemValueText
)

const (
	ItemValueKindBinary = meta.ItemValueKindBinary
	TagTypeApe          = meta.TagTypeApe
)

var NewTag = meta.NewTag

var (
	streamLen         = util.StreamLen
	itoa              = util.Itoa
	u32FromString     = util.U32FromString
	indexOfStr        = util.IndexOfStr
	tryParseTimestamp = util.TryParseTimestamp
)

const (
	TagSupportUnsupported = meta.TagSupportUnsupported
)

var (
	ApePictureTypes      = meta.ApePictureTypes
	ItemKeyOrUnknown     = meta.ItemKeyOrUnknown
	NewTagItem           = meta.NewTagItem
	NewVerifiedFile      = meta.NewVerifiedFile
	PictureFromApeBytes  = meta.PictureFromApeBytes
	VerifiedFileFromPath = meta.VerifiedFileFromPath
)

const (
	ItemValueKindText = meta.ItemValueKindText
)

var (
	NewOSFile       = meta.NewOSFile
	ItemValueBinary = meta.ItemValueBinary
)

var (
	Splice            = meta.Splice
	NewFileProperties = meta.NewFileProperties
)

type (
	ChannelMask = meta.ChannelMask
)

var (
	splitNulls = util.SplitNulls
	joinNulls  = util.JoinNulls
	utf8Decode = util.Utf8Decode
)

func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }
func optionFromPtr[T any](p *T) Option[*T]  { return util.OptionFromPtr(p) }

const (
	TagTypeID3v1 = meta.TagTypeID3v1
	TagTypeID3v2 = meta.TagTypeID3v2
)

const (
	ItemKeyAlbumTitle  = meta.ItemKeyAlbumTitle
	ItemKeyTrackArtist = meta.ItemKeyTrackArtist
	ItemKeyTrackTitle  = meta.ItemKeyTrackTitle
)

var (
	NewParseOptions = meta.NewParseOptions
	NewWriteOptions = meta.NewWriteOptions
	dumpTag         = meta.DumpTag
)

const (
	ItemKeyTrackArtists   = meta.ItemKeyTrackArtists
	PictureTypeCoverFront = meta.PictureTypeCoverFront
)

var (
	PictureUnchecked = meta.PictureUnchecked
	MimeTypeJpeg     = meta.MimeTypeJpeg
)

const (
	ItemKeyComment = meta.ItemKeyComment
	ItemKeyGenre   = meta.ItemKeyGenre
)

// Aliases for concrete-tag writes (Rust `TagExt` parity).
type (
	Id3v2Tag   = id3v2.Id3v2Tag
	TagSupport = meta.TagSupport
)

var parseId3v2Tag = id3v2.ParseId3v2Tag
