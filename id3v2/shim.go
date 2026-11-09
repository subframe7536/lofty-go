package id3v2

// Internal aliases bridging this package back to the shared model and the
// internal util helpers, keeping the ported code close to the original
// single-package layout.

import (
	"github.com/subframe7536/lofty-go/id3v1"
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
	MimeType        = meta.MimeType
	Popularimeter   = meta.Popularimeter

	Option[T any]        = meta.Option[T]
	ParseOptions         = meta.ParseOptions
	WriteOptions         = meta.WriteOptions
	ParsingMode          = meta.ParsingMode
	FileLike             = meta.FileLike
	TagParseError        = meta.TagParseError
	TagEncodingError     = meta.TagEncodingError
	FileEncodingError    = meta.FileEncodingError
	TooMuchDataError     = meta.TooMuchDataError
	FakeTagError         = meta.FakeTagError
	TextEncoding         = meta.TextEncoding
	TextDecodingError    = meta.TextDecodingError
	BadTextEncodingError = meta.BadTextEncodingError
)

const (
	ParsingModeBestAttempt = meta.ParsingModeBestAttempt
	ParsingModeStrict      = meta.ParsingModeStrict
	ParsingModeRelaxed     = meta.ParsingModeRelaxed
)

func Some[T any](val T) Option[T] { return meta.Some(val) }
func None[T any]() Option[T]      { return meta.None[T]() }

type (
	Id3v2Version = meta.Id3v2Version
)

const (
	ItemKeyArranger    = meta.ItemKeyArranger
	ItemKeyConductor   = meta.ItemKeyConductor
	ItemKeyDirector    = meta.ItemKeyDirector
	ItemKeyEngineer    = meta.ItemKeyEngineer
	ItemKeyMixDj       = meta.ItemKeyMixDj
	ItemKeyMixEngineer = meta.ItemKeyMixEngineer
	ItemKeyProducer    = meta.ItemKeyProducer
)

const (
	ItemKeyComposer  = meta.ItemKeyComposer
	ItemKeyLyricist  = meta.ItemKeyLyricist
	ItemKeyPerformer = meta.ItemKeyPerformer
	ItemKeyPublisher = meta.ItemKeyPublisher
	ItemKeyRemixer   = meta.ItemKeyRemixer
	ItemKeyWriter    = meta.ItemKeyWriter
)

const (
	ItemKeyAcoustId            = meta.ItemKeyAcoustId
	ItemKeyAcoustIdFingerprint = meta.ItemKeyAcoustIdFingerprint
	ItemKeyAlbumArtists        = meta.ItemKeyAlbumArtists
	ItemKeyCatalogNumber       = meta.ItemKeyCatalogNumber
	ItemKeyDiscNumber          = meta.ItemKeyDiscNumber
	ItemKeyDiscTotal           = meta.ItemKeyDiscTotal
	ItemKeyTrackArtists        = meta.ItemKeyTrackArtists
	ItemKeyTrackNumber         = meta.ItemKeyTrackNumber
	ItemKeyTrackTotal          = meta.ItemKeyTrackTotal
)

const (
	ItemKeyBarcode                    = meta.ItemKeyBarcode
	ItemKeyMusicBrainzArtistId        = meta.ItemKeyMusicBrainzArtistId
	ItemKeyMusicBrainzReleaseArtistId = meta.ItemKeyMusicBrainzReleaseArtistId
	ItemKeyMusicBrainzReleaseGroupId  = meta.ItemKeyMusicBrainzReleaseGroupId
	ItemKeyMusicBrainzReleaseId       = meta.ItemKeyMusicBrainzReleaseId
	ItemKeyMusicBrainzReleaseType     = meta.ItemKeyMusicBrainzReleaseType
	ItemKeyMusicBrainzTrackId         = meta.ItemKeyMusicBrainzTrackId
	ItemKeyMusicBrainzWorkId          = meta.ItemKeyMusicBrainzWorkId
	ItemKeyReleaseCountry             = meta.ItemKeyReleaseCountry
)

const (
	ItemKeyColor               = meta.ItemKeyColor
	ItemKeyIntegerBpm          = meta.ItemKeyIntegerBpm
	ItemKeyReplayGainAlbumGain = meta.ItemKeyReplayGainAlbumGain
	ItemKeyReplayGainAlbumPeak = meta.ItemKeyReplayGainAlbumPeak
	ItemKeyReplayGainTrackGain = meta.ItemKeyReplayGainTrackGain
	ItemKeyReplayGainTrackPeak = meta.ItemKeyReplayGainTrackPeak
	ItemKeyWork                = meta.ItemKeyWork
)

const (
	ItemKeyGenre          = meta.ItemKeyGenre
	ItemKeyMovementNumber = meta.ItemKeyMovementNumber
	ItemKeyMovementTotal  = meta.ItemKeyMovementTotal
	TagTypeID3v2          = meta.TagTypeID3v2
)

var (
	ItemValueText = meta.ItemValueText
	NewTag        = meta.NewTag
	NewTagItem    = meta.NewTagItem
)

const ItemKeyUnknown = meta.ItemKeyUnknown

var (
	ItemKeyFromKey       = meta.ItemKeyFromKey
	ItemValueBinary      = meta.ItemValueBinary
	TimestampParseString = meta.TimestampParseString
	unsynchsafe32        = util.Unsynchsafe32
)

type byteOrder = util.ByteOrder

const (
	ItemKeyComment                = meta.ItemKeyComment
	ItemKeyMusicBrainzRecordingId = meta.ItemKeyMusicBrainzRecordingId
	ItemKeyPopularimeter          = meta.ItemKeyPopularimeter
	ItemKeyUnsyncLyrics           = meta.ItemKeyUnsyncLyrics
)

var (
	ItemValueLocator = meta.ItemValueLocator
	MimeTypeFromStr  = meta.MimeTypeFromStr
	PictureUnchecked = meta.PictureUnchecked
)

const (
	TextEncodingUTF16   = meta.TextEncodingUTF16
	TextEncodingUTF16BE = meta.TextEncodingUTF16BE
)

var (
	id3v1Genres         = id3v1.Id3v1Genres
	popularimeterMapped = meta.PopularimeterMapped
	atoi                = util.Atoi
	latin1Decode        = util.Latin1Decode
	utf16DecodeBytes    = util.Utf16DecodeBytes
)

const (
	bigEndian    = util.BigEndian
	littleEndian = util.LittleEndian
)

const (
	ItemKeyParentalAdvisory = meta.ItemKeyParentalAdvisory
	ItemValueKindBinary     = meta.ItemValueKindBinary
	ItemValueKindLocator    = meta.ItemValueKindLocator
)

var PopularimeterFromStr = meta.PopularimeterFromStr

var (
	itoa                 = util.Itoa
	latin1EncodeMust     = util.Latin1EncodeMust
	advisoryRatingFromU8 = util.AdvisoryRatingFromU8
)

var LangUnknown = meta.LangUnknown

const (
	ItemKeyOriginalReleaseDate = meta.ItemKeyOriginalReleaseDate
	ItemKeyRecordingDate       = meta.ItemKeyRecordingDate
)

var Splice = meta.Splice

var (
	MimeTypeJpeg = meta.MimeTypeJpeg
	MimeTypePng  = meta.MimeTypePng
)

var (
	flagItem     = util.FlagItem
	synchsafe32  = util.Synchsafe32
	trimEndNulls = util.TrimEndNulls
)

func tryByteSlice(size int) ([]byte, error) { return util.TryByteSlice(size) }

var (
	utf16Encode  = util.Utf16Encode
	latin1Encode = util.Latin1Encode
)

var (
	splitNulls = util.SplitNulls
	joinNulls  = util.JoinNulls
)

const (
	ItemKeyAlbumTitle     = meta.ItemKeyAlbumTitle
	ItemKeyLength         = meta.ItemKeyLength
	ItemKeyTrackArtist    = meta.ItemKeyTrackArtist
	ItemKeyTrackTitle     = meta.ItemKeyTrackTitle
	PictureTypeCoverFront = meta.PictureTypeCoverFront
)

var (
	NewParseOptions = meta.NewParseOptions
	NewWriteOptions = meta.NewWriteOptions
)

const (
	ItemKeyAlbumArtist     = meta.ItemKeyAlbumArtist
	ItemKeyContentGroup    = meta.ItemKeyContentGroup
	ItemKeyFlagCompilation = meta.ItemKeyFlagCompilation
	ItemKeyFlagPodcast     = meta.ItemKeyFlagPodcast
	ItemKeyMood            = meta.ItemKeyMood
	ItemKeyTaggingTime     = meta.ItemKeyTaggingTime
)
