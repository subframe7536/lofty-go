package flac

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

type (
	Tag                      = meta.Tag
	TagItem                  = meta.TagItem
	ItemKey                  = meta.ItemKey
	TagType                  = meta.TagType
	Picture                  = meta.Picture
	PictureInfoPair          = meta.PictureInfoPair
	PictureInformation       = meta.PictureInformation
	TaggedFile               = meta.TaggedFile
	VerifiedFile             = meta.VerifiedFile
	FileProperties           = meta.FileProperties
	FileType                 = meta.FileType
	VorbisCommentsTag        = vorbis.VorbisCommentsTag
	VorbisCommentsParseError = vorbis.VorbisCommentsParseError
	Id3v2Header              = id3v2.Id3v2Header

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
	VorbisCommentsTagFromTag      = vorbis.VorbisCommentsTagFromTag
	WriteId3v2                    = id3v2.WriteId3v2
	FindId3v2                     = id3v2.FindId3v2
	ParseId3v2                    = id3v2.ParseId3v2
	findId3v2                     = id3v2.FindId3v2
	RegisterFormatReader          = meta.RegisterFormatReader
	RegisterFormatWriter          = meta.RegisterFormatWriter
	RegisterTagDumper             = meta.RegisterTagDumper
	SaveToPath                    = meta.SaveToPath
	RemoveTagFromPath             = meta.RemoveTagFromPath
	SaveTagFields                 = meta.SaveTagFields
	NewTaggedFile                 = meta.NewTaggedFile
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

type (
	ChannelMask = meta.ChannelMask
)

const (
	TagTypeID3v2 = meta.TagTypeID3v2
)

var (
	NewFileProperties             = meta.NewFileProperties
	PictureFromFlacBytes          = meta.PictureFromFlacBytes
	PictureInformationFromPicture = meta.PictureInformationFromPicture
)

const (
	TagTypeVorbisComments = meta.TagTypeVorbisComments
)

var (
	FileTypeFlac = meta.FileTypeFlac
	NewTag       = meta.NewTag
)

var Splice = meta.Splice

var durationFromSeconds = util.DurationFromSeconds

func optionFromPtr[T any](p *T) Option[*T]  { return util.OptionFromPtr(p) }
func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

// Aliases for concrete-tag writes (Rust `TagExt` parity).
type (
	Id3v2Tag = id3v2.Id3v2Tag
)

var (
	NewVerifiedFile           = meta.NewVerifiedFile
	NewVerifiedFileOf         = meta.NewVerifiedFileOf
	VerifiedFileFromPath      = meta.VerifiedFileFromPath
	WriteConcreteTag          = meta.WriteConcreteTag
	RegisterConcreteTagWriter = meta.RegisterConcreteTagWriter
)

var parseId3v2Tag = id3v2.ParseId3v2Tag
