package mpeg

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	"github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
)

type (
	Tag            = meta.Tag
	TagItem        = meta.TagItem
	ItemKey        = meta.ItemKey
	TagType        = meta.TagType
	TaggedFile     = meta.TaggedFile
	VerifiedFile   = meta.VerifiedFile
	FileProperties = meta.FileProperties
	FileType       = meta.FileType
	ChannelMask    = meta.ChannelMask

	ApeTag       = ape.ApeTag
	Id3v1Tag     = id3v1.Id3v1Tag
	Id3v2Header  = id3v2.Id3v2Header
	Lyrics3Tag   = lyrics3.Lyrics3Tag
	Lyrics3v2Tag = lyrics3.Lyrics3v2Tag

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
	FindId3v1            = id3v1.FindId3v1
	FindLyrics3          = lyrics3.FindLyrics3
	FindId3v2            = id3v2.FindId3v2
	ParseId3v2           = id3v2.ParseId3v2
	ParseId3v2HeaderRaw  = id3v2.ParseId3v2HeaderRaw
	WriteId3v1           = id3v1.WriteId3v1
	WriteId3v2           = id3v2.WriteId3v2
	Lyrics3v1WriteTo     = lyrics3.Lyrics3v1WriteTo
	Lyrics3v2WriteTo     = lyrics3.Lyrics3v2WriteTo
	ReadApeHeader        = ape.ReadApeHeader
	ReadApeTag           = ape.ReadApeTag
	ReadApeTagWithHeader = ape.ReadApeTagWithHeader
	ApeWriteTo           = ape.ApeWriteTo
	ApeTagFromTag        = ape.ApeTagFromTag
	Id3v1TagFromTag      = id3v1.Id3v1TagFromTag
	Lyrics3TagFromTag    = lyrics3.Lyrics3TagFromTag
	Lyrics3v2TagFromTag  = lyrics3.Lyrics3v2TagFromTag
	RegisterFormatReader = meta.RegisterFormatReader
	RegisterFormatWriter = meta.RegisterFormatWriter
	SaveToPath           = meta.SaveToPath
	RemoveTagFromPath    = meta.RemoveTagFromPath
	SaveTagFields        = meta.SaveTagFields
	NewTaggedFile        = meta.NewTaggedFile
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

const (
	TagTypeApe   = meta.TagTypeApe
	TagTypeID3v2 = meta.TagTypeID3v2
)

var NewFileProperties = meta.NewFileProperties

var FileTypeMpeg = meta.FileTypeMpeg

const (
	TagTypeID3v1 = meta.TagTypeID3v1
)

const (
	TagTypeLyrics3 = meta.TagTypeLyrics3
)

const TagTypeLyrics3v2 = meta.TagTypeLyrics3v2

var (
	findId3v1   = id3v1.FindId3v1
	findLyrics3 = lyrics3.FindLyrics3
	findId3v2   = id3v2.FindId3v2
	parseId3v2  = id3v2.ParseId3v2
	divRound    = util.DivRound
)

func optionFromPtr[T any](p *T) Option[*T] { return util.OptionFromPtr(p) }

var searchForFrameSync = util.SearchForFrameSync
