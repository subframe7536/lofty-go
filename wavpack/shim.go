package wavpack

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	ape "github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/mp4"
	"github.com/subframe7536/lofty-go/mpeg"
)

type (
	Tag             = meta.Tag
	TagItem         = meta.TagItem
	ItemKey         = meta.ItemKey
	ItemValue       = meta.ItemValue
	TagType         = meta.TagType
	Timestamp       = meta.Timestamp
	TaggedFile      = meta.TaggedFile
	VerifiedFile    = meta.VerifiedFile
	FileProperties  = meta.FileProperties
	FileType        = meta.FileType
	ChannelMask     = meta.ChannelMask
	MpegVersion     = mpeg.MpegVersion
	AudioObjectType = mp4.AudioObjectType

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

const (
	TagTypeID3v1 = meta.TagTypeID3v1
	TagTypeID3v2 = meta.TagTypeID3v2
	TagTypeApe   = meta.TagTypeApe
)

var (
	FileTypeDsf     = meta.FileTypeDsf
	FileTypeWavPack = meta.FileTypeWavPack
	FileTypeMpc     = meta.FileTypeMpc
	FileTypeAac     = meta.FileTypeAac
)

var (
	FindId3v1            = id3v1.FindId3v1
	FindId3v2            = id3v2.FindId3v2
	ParseId3v2           = id3v2.ParseId3v2
	ParseId3v2HeaderRaw  = id3v2.ParseId3v2HeaderRaw
	WriteId3v1           = id3v1.WriteId3v1
	WriteId3v2           = id3v2.WriteId3v2
	Id3v1TagFromTag      = id3v1.Id3v1TagFromTag
	ReadEndTags          = ape.ReadEndTags
	ApeWriteTo           = ape.ApeWriteTo
	ApeTagFromTag        = ape.ApeTagFromTag
	RegisterFormatReader = meta.RegisterFormatReader
	RegisterFormatWriter = meta.RegisterFormatWriter
	SaveToPath           = meta.SaveToPath
	RemoveTagFromPath    = meta.RemoveTagFromPath
	SaveTagFields        = meta.SaveTagFields
	NewTaggedFile        = meta.NewTaggedFile
	NewFileProperties    = meta.NewFileProperties
	NewTag               = meta.NewTag
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

var (
	Id3v2Serialize = id3v2.Id3v2Serialize
	Splice         = meta.Splice
)

func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

const (
	ChannelMaskMono   = meta.ChannelMaskMono
	ChannelMaskStereo = meta.ChannelMaskStereo
)

// Aliases for concrete-tag writes (Rust `TagExt` parity).
type (
	Id3v1Tag = id3v1.Id3v1Tag
	Id3v2Tag = id3v2.Id3v2Tag
	ApeTag   = ape.ApeTag
)
