package ogg

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

type (
	Tag               = meta.Tag
	TagItem           = meta.TagItem
	ItemKey           = meta.ItemKey
	TagType           = meta.TagType
	PictureInfoPair   = meta.PictureInfoPair
	TaggedFile        = meta.TaggedFile
	VerifiedFile      = meta.VerifiedFile
	FileProperties    = meta.FileProperties
	FileType          = meta.FileType
	VorbisCommentsTag = vorbis.VorbisCommentsTag

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
	ParseVorbisComments           = vorbis.ParseVorbisComments
	EncodeVorbisComments          = vorbis.EncodeVorbisComments
	VorbisCommentsTagForWritePics = vorbis.VorbisCommentsTagForWritePics
	RegisterFormatReader          = meta.RegisterFormatReader
	RegisterFormatWriter          = meta.RegisterFormatWriter
	SaveToPath                    = meta.SaveToPath
	RemoveTagFromPath             = meta.RemoveTagFromPath
	SaveTagFields                 = meta.SaveTagFields
	NewTaggedFile                 = meta.NewTaggedFile
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

type (
	ChannelMask              = meta.ChannelMask
	VorbisCommentsParseError = vorbis.VorbisCommentsParseError
)

var (
	FileTypeOpus   = meta.FileTypeOpus
	FileTypeSpeex  = meta.FileTypeSpeex
	FileTypeVorbis = meta.FileTypeVorbis
)

var (
	ChannelMaskFromOpusChannels = meta.ChannelMaskFromOpusChannels
	NewFileProperties           = meta.NewFileProperties
)

type (
	PictureInformation = meta.PictureInformation
)

const (
	TagTypeVorbisComments = meta.TagTypeVorbisComments
)

var (
	Splice                        = meta.Splice
	PictureInformationFromPicture = meta.PictureInformationFromPicture
)

var divRound = util.DivRound

func optionFromPtr[T any](p *T) Option[*T] { return util.OptionFromPtr(p) }
