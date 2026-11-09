package lofty

import "strings"

// ItemKey is a format-agnostic representation of a tag key.
type ItemKey uint16

const (
	// Titles
	ItemKeyAlbumTitle ItemKey = iota
	ItemKeySetSubtitle
	ItemKeyShowName
	ItemKeyContentGroup
	// AppleId3v2ContentGroup is the Apple alternative to [ItemKeyContentGroup] stored
	// in a "GRP1" frame rather than "TIT1".
	ItemKeyAppleId3v2ContentGroup
	ItemKeyTrackTitle
	ItemKeyTrackSubtitle

	// Original names
	ItemKeyOriginalAlbumTitle
	ItemKeyOriginalArtist
	ItemKeyOriginalLyricist

	// Sorting
	ItemKeyAlbumTitleSortOrder
	ItemKeyAlbumArtistSortOrder
	ItemKeyTrackTitleSortOrder
	ItemKeyTrackArtistSortOrder
	ItemKeyShowNameSortOrder
	ItemKeyComposerSortOrder

	// People & Organizations
	ItemKeyAlbumArtist
	ItemKeyAlbumArtists
	ItemKeyTrackArtist
	ItemKeyTrackArtists
	ItemKeyArranger
	ItemKeyWriter
	ItemKeyComposer
	ItemKeyConductor
	ItemKeyDirector
	ItemKeyEngineer
	ItemKeyLyricist
	ItemKeyMixDj
	ItemKeyMixEngineer
	ItemKeyPerformer
	ItemKeyProducer
	ItemKeyPublisher
	ItemKeyLabel
	ItemKeyInternetRadioStationName
	ItemKeyInternetRadioStationOwner
	ItemKeyRemixer

	// Counts & Indexes
	ItemKeyDiscNumber
	ItemKeyDiscTotal
	ItemKeyTrackNumber
	ItemKeyTrackTotal

	// Ratings
	ItemKeyPopularimeter
	ItemKeyParentalAdvisory

	// Dates
	ItemKeyRecordingDate
	ItemKeyYear
	ItemKeyReleaseDate
	ItemKeyOriginalReleaseDate

	// Identifiers
	ItemKeyIsrc
	ItemKeyBarcode
	ItemKeyAcoustId
	ItemKeyAcoustIdFingerprint
	ItemKeyCatalogNumber
	ItemKeyWork
	ItemKeyMovement
	ItemKeyMovementNumber
	ItemKeyMovementTotal
	ItemKeyReleaseCountry

	// MusicBrainz
	ItemKeyMusicBrainzRecordingId
	ItemKeyMusicBrainzTrackId
	ItemKeyMusicBrainzReleaseId
	ItemKeyMusicBrainzReleaseGroupId
	ItemKeyMusicBrainzArtistId
	ItemKeyMusicBrainzReleaseArtistId
	ItemKeyMusicBrainzWorkId
	ItemKeyMusicBrainzReleaseType

	// Flags
	ItemKeyFlagCompilation
	ItemKeyFlagPodcast

	// File information
	ItemKeyFileOwner
	ItemKeyTaggingTime
	ItemKeyLength
	ItemKeyOriginalFileName
	ItemKeyOriginalMediaType

	// Encoder information
	ItemKeyEncodedBy
	ItemKeyEncoderSoftware
	ItemKeyEncoderSettings
	ItemKeyEncodingTime

	// ReplayGain
	ItemKeyReplayGainAlbumGain
	ItemKeyReplayGainAlbumPeak
	ItemKeyReplayGainTrackGain
	ItemKeyReplayGainTrackPeak
	ItemKeyR128TrackGain
	ItemKeyR128AlbumGain

	// URLs
	ItemKeyAudioFileUrl
	ItemKeyAudioSourceUrl
	ItemKeyCommercialInformationUrl
	ItemKeyCopyrightUrl
	ItemKeyTrackArtistUrl
	ItemKeyRadioStationUrl
	ItemKeyPaymentUrl
	ItemKeyPublisherUrl

	// Style
	ItemKeyGenre
	ItemKeyInitialKey
	ItemKeyColor
	ItemKeyMood
	ItemKeyBpm
	ItemKeyIntegerBpm

	// Legal
	ItemKeyCopyrightMessage
	ItemKeyLicense

	// Podcast
	ItemKeyPodcastDescription
	ItemKeyPodcastSeriesCategory
	ItemKeyPodcastUrl
	ItemKeyPodcastGlobalUniqueId
	ItemKeyPodcastKeywords

	// Miscellaneous
	ItemKeyComment
	ItemKeyDescription
	ItemKeyLanguage
	ItemKeyScript
	ItemKeyLyrics
	ItemKeyUnsyncLyrics

	// Vendor-specific
	ItemKeyAppleXid

	// ItemKeyUnknown is a placeholder for items with a key not defined in [ItemKey].
	// The value is stored in [TagItem.Key].
	ItemKeyUnknown
)

func (k ItemKey) String() string {
	if int(k) < len(itemKeyVariantNames) {
		return itemKeyVariantNames[k]
	}
	return "Unknown"
}

// itemKeyVariantNames is populated in init for debugging/Display purposes.
var itemKeyVariantNames [ItemKeyUnknown + 1]string

func init() {
	for i := ItemKey(0); i <= ItemKeyUnknown; i++ {
		itemKeyVariantNames[i] = itemKeyVariantName(i)
	}
}

// IsNumeric reports whether the key *should* represent a numeric value.
func (k ItemKey) IsNumeric() bool {
	switch k {
	case ItemKeyDiscNumber, ItemKeyDiscTotal, ItemKeyTrackNumber, ItemKeyTrackTotal,
		ItemKeyParentalAdvisory, ItemKeyYear, ItemKeyBarcode, ItemKeyMovementNumber,
		ItemKeyMovementTotal, ItemKeyLength, ItemKeyBpm, ItemKeyIntegerBpm:
		return true
	}
	return false
}

// IsFlag reports whether the key *should* represent a boolean flag value.
func (k ItemKey) IsFlag() bool {
	return k == ItemKeyFlagCompilation || k == ItemKeyFlagPodcast
}

// IsTimestamp reports whether the key *should* represent a timestamp value.
func (k ItemKey) IsTimestamp() bool {
	switch k {
	case ItemKeyRecordingDate, ItemKeyReleaseDate, ItemKeyOriginalReleaseDate,
		ItemKeyTaggingTime, ItemKeyEncodingTime:
		return true
	}
	return false
}

// keyMapEntry is a single key → ItemKey mapping. Multiple keys may map to the
// same ItemKey; the first listed key is the preferred ("standard") one.
type keyMapEntry struct {
	keys    []string
	itemKey ItemKey
}

// keyMap provides bidirectional format key <-> [ItemKey] mapping.
type keyMap struct {
	entries       []keyMapEntry
	supportedKeys []ItemKey
	// key -> itemKey (case-insensitive lookup)
	byKey map[string]ItemKey
	// itemKey -> preferred key
	byItemKey map[ItemKey]string
}

func newKeyMap(entries []keyMapEntry) *keyMap {
	m := &keyMap{
		entries:   entries,
		byKey:     make(map[string]ItemKey),
		byItemKey: make(map[ItemKey]string),
	}
	seen := make(map[ItemKey]bool)
	for _, e := range entries {
		for _, k := range e.keys {
			lk := strings.ToLower(k)
			// first registered wins for a given key
			if _, exists := m.byKey[lk]; !exists {
				m.byKey[lk] = e.itemKey
			}
			if !seen[e.itemKey] {
				seen[e.itemKey] = true
				m.supportedKeys = append(m.supportedKeys, e.itemKey)
			}
		}
	}
	for _, e := range entries {
		if len(e.keys) > 0 {
			if _, exists := m.byItemKey[e.itemKey]; !exists {
				m.byItemKey[e.itemKey] = e.keys[0]
			}
		}
	}
	return m
}

// itemKeyForKey returns the [ItemKey] for a format-specific key (case-insensitive).
func (m *keyMap) itemKeyForKey(key string) (ItemKey, bool) {
	k, ok := m.byKey[strings.ToLower(key)]
	return k, ok
}

// keyForItemKey returns the preferred format-specific key for an [ItemKey].
func (m *keyMap) keyForItemKey(k ItemKey) (string, bool) {
	s, ok := m.byItemKey[k]
	return s, ok
}

var (
	aiffTextMap = newKeyMap([]keyMapEntry{
		{[]string{"NAME"}, ItemKeyTrackTitle},
		{[]string{"AUTH"}, ItemKeyTrackArtist},
		{[]string{"(c) "}, ItemKeyCopyrightMessage},
		{[]string{"ANNO"}, ItemKeyComment},
	})

	apeMap = newKeyMap([]keyMapEntry{
		{[]string{"Album"}, ItemKeyAlbumTitle},
		{[]string{"DiscSubtitle"}, ItemKeySetSubtitle},
		{[]string{"Grouping"}, ItemKeyContentGroup},
		{[]string{"Title"}, ItemKeyTrackTitle},
		{[]string{"Subtitle"}, ItemKeyTrackSubtitle},
		{[]string{"WORKTITLE"}, ItemKeyWork},
		{[]string{"MOVEMENTNAME"}, ItemKeyMovement},
		{[]string{"MOVEMENT"}, ItemKeyMovementNumber},
		{[]string{"MOVEMENTTOTAL"}, ItemKeyMovementTotal},
		{[]string{"RELEASECOUNTRY"}, ItemKeyReleaseCountry},
		{[]string{"ALBUMSORT"}, ItemKeyAlbumTitleSortOrder},
		{[]string{"ALBUMARTISTSORT"}, ItemKeyAlbumArtistSortOrder},
		{[]string{"TITLESORT"}, ItemKeyTrackTitleSortOrder},
		{[]string{"ARTISTSORT"}, ItemKeyTrackArtistSortOrder},
		{[]string{"Album Artist", "ALBUMARTIST"}, ItemKeyAlbumArtist},
		{[]string{"ALBUMARTISTS", "ALBUM ARTISTS"}, ItemKeyAlbumArtists},
		{[]string{"Artist"}, ItemKeyTrackArtist},
		{[]string{"Artists"}, ItemKeyTrackArtists},
		{[]string{"Arranger"}, ItemKeyArranger},
		{[]string{"Writer"}, ItemKeyWriter},
		{[]string{"Composer"}, ItemKeyComposer},
		{[]string{"Conductor"}, ItemKeyConductor},
		{[]string{"Director"}, ItemKeyDirector},
		{[]string{"Engineer"}, ItemKeyEngineer},
		{[]string{"Lyricist"}, ItemKeyLyricist},
		{[]string{"DjMixer"}, ItemKeyMixDj},
		{[]string{"Mixer"}, ItemKeyMixEngineer},
		{[]string{"Performer"}, ItemKeyPerformer},
		{[]string{"Producer"}, ItemKeyProducer},
		{[]string{"Label"}, ItemKeyLabel},
		{[]string{"MixArtist"}, ItemKeyRemixer},
		{[]string{"Disc"}, ItemKeyDiscNumber},
		{[]string{"Disc"}, ItemKeyDiscTotal},
		{[]string{"Track"}, ItemKeyTrackNumber},
		{[]string{"Track"}, ItemKeyTrackTotal},
		// For some reason, the ecosystem agreed on the key "Year", even for full date strings.
		{[]string{"Year"}, ItemKeyRecordingDate},
		{[]string{"Year"}, ItemKeyYear},
		{[]string{"ORIGINALYEAR"}, ItemKeyOriginalReleaseDate},
		{[]string{"RELEASEDATE"}, ItemKeyReleaseDate},
		{[]string{"ISRC"}, ItemKeyIsrc},
		{[]string{"Barcode"}, ItemKeyBarcode},
		{[]string{"ACOUSTID_ID"}, ItemKeyAcoustId},
		{[]string{"ACOUSTID_FINGERPRINT"}, ItemKeyAcoustIdFingerprint},
		{[]string{"CatalogNumber"}, ItemKeyCatalogNumber},
		{[]string{"Compilation"}, ItemKeyFlagCompilation},
		{[]string{"Media"}, ItemKeyOriginalMediaType},
		{[]string{"EncodedBy"}, ItemKeyEncodedBy},
		{[]string{"REPLAYGAIN_ALBUM_GAIN"}, ItemKeyReplayGainAlbumGain},
		{[]string{"REPLAYGAIN_ALBUM_PEAK"}, ItemKeyReplayGainAlbumPeak},
		{[]string{"REPLAYGAIN_TRACK_GAIN"}, ItemKeyReplayGainTrackGain},
		{[]string{"REPLAYGAIN_TRACK_PEAK"}, ItemKeyReplayGainTrackPeak},
		{[]string{"Genre"}, ItemKeyGenre},
		{[]string{"Color"}, ItemKeyColor},
		{[]string{"Mood"}, ItemKeyMood},
		{[]string{"Copyright"}, ItemKeyCopyrightMessage},
		{[]string{"Comment"}, ItemKeyComment},
		{[]string{"language"}, ItemKeyLanguage},
		{[]string{"Script"}, ItemKeyScript},
		{[]string{"Lyrics"}, ItemKeyLyrics},
		{[]string{"UnsyncedLyrics"}, ItemKeyUnsyncLyrics},
		{[]string{"MUSICBRAINZ_TRACKID"}, ItemKeyMusicBrainzRecordingId},
		{[]string{"MUSICBRAINZ_RELEASETRACKID"}, ItemKeyMusicBrainzTrackId},
		{[]string{"MUSICBRAINZ_ALBUMID"}, ItemKeyMusicBrainzReleaseId},
		{[]string{"MUSICBRAINZ_RELEASEGROUPID"}, ItemKeyMusicBrainzReleaseGroupId},
		{[]string{"MUSICBRAINZ_ARTISTID"}, ItemKeyMusicBrainzArtistId},
		{[]string{"MUSICBRAINZ_ALBUMARTISTID"}, ItemKeyMusicBrainzReleaseArtistId},
		{[]string{"MUSICBRAINZ_WORKID"}, ItemKeyMusicBrainzWorkId},
		{[]string{"MUSICBRAINZ_ALBUMTYPE"}, ItemKeyMusicBrainzReleaseType},
	})

	id3v2Map = newKeyMap([]keyMapEntry{
		{[]string{"TALB"}, ItemKeyAlbumTitle},
		{[]string{"TSST"}, ItemKeySetSubtitle},
		{[]string{"TIT1"}, ItemKeyContentGroup},
		{[]string{"GRP1"}, ItemKeyAppleId3v2ContentGroup},
		{[]string{"TIT2"}, ItemKeyTrackTitle},
		{[]string{"TIT3"}, ItemKeyTrackSubtitle},
		{[]string{"TOAL"}, ItemKeyOriginalAlbumTitle},
		{[]string{"TOPE"}, ItemKeyOriginalArtist},
		{[]string{"TOLY"}, ItemKeyOriginalLyricist},
		{[]string{"TSOA"}, ItemKeyAlbumTitleSortOrder},
		{[]string{"TSO2"}, ItemKeyAlbumArtistSortOrder},
		{[]string{"TSOT"}, ItemKeyTrackTitleSortOrder},
		{[]string{"TSOP"}, ItemKeyTrackArtistSortOrder},
		{[]string{"TSOC"}, ItemKeyComposerSortOrder},
		{[]string{"TPE2", "ALBUMARTIST", "ALBUM ARTIST"}, ItemKeyAlbumArtist},
		{[]string{"ALBUMARTISTS", "ALBUM ARTISTS"}, ItemKeyAlbumArtists},
		{[]string{"TPE1"}, ItemKeyTrackArtist},
		{[]string{"ARTISTS"}, ItemKeyTrackArtists},
		{[]string{"TEXT"}, ItemKeyLyricist},
		{[]string{"TCOM"}, ItemKeyComposer},
		{[]string{"TPE3"}, ItemKeyConductor},
		{[]string{"DIRECTOR"}, ItemKeyDirector},
		{[]string{"TEXT"}, ItemKeyWriter},
		{[]string{"TIPL"}, ItemKeyProducer},
		{[]string{"TIPL"}, ItemKeyArranger},
		{[]string{"TIPL"}, ItemKeyEngineer},
		{[]string{"TIPL"}, ItemKeyMixDj},
		{[]string{"TIPL"}, ItemKeyMixEngineer},
		{[]string{"TPUB"}, ItemKeyPublisher},
		{[]string{"TPUB"}, ItemKeyLabel},
		{[]string{"TRSN"}, ItemKeyInternetRadioStationName},
		{[]string{"TRSO"}, ItemKeyInternetRadioStationOwner},
		{[]string{"TPE4"}, ItemKeyRemixer},
		{[]string{"TPOS"}, ItemKeyDiscNumber},
		{[]string{"TPOS"}, ItemKeyDiscTotal},
		{[]string{"TRCK"}, ItemKeyTrackNumber},
		{[]string{"TRCK"}, ItemKeyTrackTotal},
		{[]string{"POPM"}, ItemKeyPopularimeter},
		{[]string{"ITUNESADVISORY"}, ItemKeyParentalAdvisory},
		{[]string{"TDRC"}, ItemKeyRecordingDate},
		{[]string{"TDOR"}, ItemKeyOriginalReleaseDate},
		{[]string{"TSRC"}, ItemKeyIsrc},
		{[]string{"BARCODE"}, ItemKeyBarcode},
		{[]string{"Acoustid Id"}, ItemKeyAcoustId},
		{[]string{"Acoustid Fingerprint"}, ItemKeyAcoustIdFingerprint},
		{[]string{"CATALOGNUMBER"}, ItemKeyCatalogNumber},
		// ID3v2.4: TXXX:WORK (Apple uses TIT1/ContentGroup, see GRP1/AppleId3v2ContentGroup)
		{[]string{"WORK"}, ItemKeyWork},
		{[]string{"MVNM"}, ItemKeyMovement},
		{[]string{"MVIN"}, ItemKeyMovementNumber},
		{[]string{"MVIN"}, ItemKeyMovementTotal},
		{[]string{"TCMP"}, ItemKeyFlagCompilation},
		{[]string{"PCST"}, ItemKeyFlagPodcast},
		{[]string{"TOWN"}, ItemKeyFileOwner},
		{[]string{"TDTG"}, ItemKeyTaggingTime},
		{[]string{"TLEN"}, ItemKeyLength},
		{[]string{"TOFN"}, ItemKeyOriginalFileName},
		{[]string{"TMED"}, ItemKeyOriginalMediaType},
		{[]string{"TENC"}, ItemKeyEncodedBy},
		{[]string{"TSSE"}, ItemKeyEncoderSettings},
		{[]string{"TSSE"}, ItemKeyEncoderSoftware},
		{[]string{"TDEN"}, ItemKeyEncodingTime},
		{[]string{"REPLAYGAIN_ALBUM_GAIN"}, ItemKeyReplayGainAlbumGain},
		{[]string{"REPLAYGAIN_ALBUM_PEAK"}, ItemKeyReplayGainAlbumPeak},
		{[]string{"REPLAYGAIN_TRACK_GAIN"}, ItemKeyReplayGainTrackGain},
		{[]string{"REPLAYGAIN_TRACK_PEAK"}, ItemKeyReplayGainTrackPeak},
		{[]string{"WOAF"}, ItemKeyAudioFileUrl},
		{[]string{"WOAS"}, ItemKeyAudioSourceUrl},
		{[]string{"WCOM"}, ItemKeyCommercialInformationUrl},
		{[]string{"WCOP"}, ItemKeyCopyrightUrl},
		{[]string{"WOAR"}, ItemKeyTrackArtistUrl},
		{[]string{"WORS"}, ItemKeyRadioStationUrl},
		{[]string{"WPAY"}, ItemKeyPaymentUrl},
		{[]string{"WPUB"}, ItemKeyPublisherUrl},
		{[]string{"TCON"}, ItemKeyGenre},
		{[]string{"TKEY"}, ItemKeyInitialKey},
		{[]string{"COLOR"}, ItemKeyColor},
		{[]string{"TMOO"}, ItemKeyMood},
		{[]string{"TBPM"}, ItemKeyIntegerBpm},
		{[]string{"TCOP"}, ItemKeyCopyrightMessage},
		{[]string{"TDES"}, ItemKeyPodcastDescription},
		{[]string{"TCAT"}, ItemKeyPodcastSeriesCategory},
		{[]string{"WFED"}, ItemKeyPodcastUrl},
		{[]string{"TDRL"}, ItemKeyReleaseDate},
		{[]string{"TGID"}, ItemKeyPodcastGlobalUniqueId},
		{[]string{"TKWD"}, ItemKeyPodcastKeywords},
		{[]string{"COMM"}, ItemKeyComment},
		{[]string{"TLAN"}, ItemKeyLanguage},
		// See rust docs: SYLT has its own standard; only UnsyncLyrics is mapped.
		{[]string{"USLT"}, ItemKeyUnsyncLyrics},
		{[]string{"UFID"}, ItemKeyMusicBrainzRecordingId},
		{[]string{"MusicBrainz Release Track Id"}, ItemKeyMusicBrainzTrackId},
		{[]string{"MusicBrainz Album Id"}, ItemKeyMusicBrainzReleaseId},
		{[]string{"MusicBrainz Release Group Id"}, ItemKeyMusicBrainzReleaseGroupId},
		{[]string{"MusicBrainz Artist Id"}, ItemKeyMusicBrainzArtistId},
		{[]string{"MusicBrainz Album Artist Id"}, ItemKeyMusicBrainzReleaseArtistId},
		{[]string{"MusicBrainz Work Id"}, ItemKeyMusicBrainzWorkId},
		{[]string{"MusicBrainz Album Type"}, ItemKeyMusicBrainzReleaseType},
		{[]string{"MusicBrainz Album Release Country"}, ItemKeyReleaseCountry},
	})

	ilstMap = newKeyMap([]keyMapEntry{
		{[]string{"©alb"}, ItemKeyAlbumTitle},
		{[]string{"----:com.apple.iTunes:DISCSUBTITLE"}, ItemKeySetSubtitle},
		{[]string{"tvsh"}, ItemKeyShowName},
		{[]string{"©grp"}, ItemKeyContentGroup},
		{[]string{"©nam"}, ItemKeyTrackTitle},
		{[]string{"----:com.apple.iTunes:SUBTITLE"}, ItemKeyTrackSubtitle},
		{[]string{"©wrk"}, ItemKeyWork},
		{[]string{"©mvn"}, ItemKeyMovement},
		{[]string{"©mvi"}, ItemKeyMovementNumber},
		{[]string{"©mvc"}, ItemKeyMovementTotal},
		{[]string{"soal"}, ItemKeyAlbumTitleSortOrder},
		{[]string{"soaa"}, ItemKeyAlbumArtistSortOrder},
		{[]string{"sonm"}, ItemKeyTrackTitleSortOrder},
		{[]string{"soar"}, ItemKeyTrackArtistSortOrder},
		{[]string{"sosn"}, ItemKeyShowNameSortOrder},
		{[]string{"soco"}, ItemKeyComposerSortOrder},
		{[]string{"aART"}, ItemKeyAlbumArtist},
		{[]string{"----:com.apple.iTunes:ALBUMARTISTS"}, ItemKeyAlbumArtists},
		{[]string{"©ART"}, ItemKeyTrackArtist},
		{[]string{"----:com.apple.iTunes:ARTISTS"}, ItemKeyTrackArtists},
		{[]string{"©wrt"}, ItemKeyComposer},
		{[]string{"©dir"}, ItemKeyDirector},
		{[]string{"----:com.apple.iTunes:CONDUCTOR"}, ItemKeyConductor},
		{[]string{"----:com.apple.iTunes:ENGINEER"}, ItemKeyEngineer},
		{[]string{"----:com.apple.iTunes:LYRICIST"}, ItemKeyLyricist},
		{[]string{"----:com.apple.iTunes:DJMIXER"}, ItemKeyMixDj},
		{[]string{"----:com.apple.iTunes:MIXER"}, ItemKeyMixEngineer},
		{[]string{"----:com.apple.iTunes:PRODUCER"}, ItemKeyProducer},
		{[]string{"----:com.apple.iTunes:LABEL"}, ItemKeyLabel},
		{[]string{"----:com.apple.iTunes:REMIXER"}, ItemKeyRemixer},
		{[]string{"disk"}, ItemKeyDiscNumber},
		{[]string{"disk"}, ItemKeyDiscTotal},
		{[]string{"trkn"}, ItemKeyTrackNumber},
		{[]string{"trkn"}, ItemKeyTrackTotal},
		{[]string{"rate"}, ItemKeyPopularimeter},
		{[]string{"rtng"}, ItemKeyParentalAdvisory},
		{[]string{"©day"}, ItemKeyRecordingDate},
		{[]string{"----:com.apple.iTunes:ORIGINALDATE"}, ItemKeyOriginalReleaseDate},
		{[]string{"----:com.apple.iTunes:RELEASEDATE"}, ItemKeyReleaseDate},
		{[]string{"----:com.apple.iTunes:ISRC"}, ItemKeyIsrc},
		{[]string{"----:com.apple.iTunes:BARCODE"}, ItemKeyBarcode},
		{[]string{"----:com.apple.iTunes:Acoustid Id"}, ItemKeyAcoustId},
		{[]string{"----:com.apple.iTunes:Acoustid Fingerprint"}, ItemKeyAcoustIdFingerprint},
		{[]string{"----:com.apple.iTunes:CATALOGNUMBER"}, ItemKeyCatalogNumber},
		{[]string{"cpil"}, ItemKeyFlagCompilation},
		{[]string{"pcst"}, ItemKeyFlagPodcast},
		{[]string{"----:com.apple.iTunes:MEDIA"}, ItemKeyOriginalMediaType},
		{[]string{"©enc"}, ItemKeyEncodedBy},
		{[]string{"©too"}, ItemKeyEncoderSoftware},
		{[]string{"©gen"}, ItemKeyGenre},
		{[]string{"----:com.apple.iTunes:COLOR"}, ItemKeyColor},
		{[]string{"----:com.apple.iTunes:MOOD"}, ItemKeyMood},
		{[]string{"tmpo"}, ItemKeyIntegerBpm},
		{[]string{"----:com.apple.iTunes:BPM"}, ItemKeyBpm},
		{[]string{"----:com.apple.iTunes:initialkey"}, ItemKeyInitialKey},
		{[]string{"----:com.apple.iTunes:replaygain_album_gain"}, ItemKeyReplayGainAlbumGain},
		{[]string{"----:com.apple.iTunes:replaygain_album_peak"}, ItemKeyReplayGainAlbumPeak},
		{[]string{"----:com.apple.iTunes:replaygain_track_gain"}, ItemKeyReplayGainTrackGain},
		{[]string{"----:com.apple.iTunes:replaygain_track_peak"}, ItemKeyReplayGainTrackPeak},
		{[]string{"cprt"}, ItemKeyCopyrightMessage},
		{[]string{"----:com.apple.iTunes:LICENSE"}, ItemKeyLicense},
		{[]string{"ldes"}, ItemKeyPodcastDescription},
		{[]string{"catg"}, ItemKeyPodcastSeriesCategory},
		{[]string{"purl"}, ItemKeyPodcastUrl},
		{[]string{"egid"}, ItemKeyPodcastGlobalUniqueId},
		{[]string{"keyw"}, ItemKeyPodcastKeywords},
		{[]string{"©cmt"}, ItemKeyComment},
		{[]string{"desc"}, ItemKeyDescription},
		{[]string{"----:com.apple.iTunes:LANGUAGE"}, ItemKeyLanguage},
		{[]string{"----:com.apple.iTunes:SCRIPT"}, ItemKeyScript},
		{[]string{"©lyr"}, ItemKeyLyrics},
		{[]string{"©lyr"}, ItemKeyUnsyncLyrics},
		{[]string{"xid "}, ItemKeyAppleXid},
		{[]string{"----:com.apple.iTunes:MusicBrainz Track Id"}, ItemKeyMusicBrainzRecordingId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Release Track Id"}, ItemKeyMusicBrainzTrackId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Album Id"}, ItemKeyMusicBrainzReleaseId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Release Group Id"}, ItemKeyMusicBrainzReleaseGroupId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Artist Id"}, ItemKeyMusicBrainzArtistId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Album Artist Id"}, ItemKeyMusicBrainzReleaseArtistId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Work Id"}, ItemKeyMusicBrainzWorkId},
		{[]string{"----:com.apple.iTunes:MusicBrainz Album Type"}, ItemKeyMusicBrainzReleaseType},
		{[]string{"----:com.apple.iTunes:MusicBrainz Album Release Country"}, ItemKeyReleaseCountry},
	})

	riffInfoMap = newKeyMap([]keyMapEntry{
		{[]string{"IPRD"}, ItemKeyAlbumTitle},
		{[]string{"INAM"}, ItemKeyTrackTitle},
		{[]string{"IART"}, ItemKeyTrackArtist},
		{[]string{"IWRI"}, ItemKeyWriter},
		{[]string{"IMUS"}, ItemKeyComposer},
		{[]string{"IPRO"}, ItemKeyProducer},
		{[]string{"IPRT", "ITRK"}, ItemKeyTrackNumber},
		{[]string{"IFRM"}, ItemKeyTrackTotal},
		{[]string{"IRTD"}, ItemKeyPopularimeter},
		{[]string{"ICRD"}, ItemKeyRecordingDate},
		{[]string{"TLEN"}, ItemKeyLength},
		{[]string{"ISRF"}, ItemKeyOriginalMediaType},
		{[]string{"ITCH"}, ItemKeyEncodedBy},
		{[]string{"ISFT"}, ItemKeyEncoderSoftware},
		{[]string{"IGNR"}, ItemKeyGenre},
		{[]string{"ICOP"}, ItemKeyCopyrightMessage},
		{[]string{"ICMT"}, ItemKeyComment},
		{[]string{"ILNG"}, ItemKeyLanguage},
		{[]string{"ICNT"}, ItemKeyReleaseCountry},
	})

	vorbisMap = newKeyMap([]keyMapEntry{
		{[]string{"ALBUM"}, ItemKeyAlbumTitle},
		{[]string{"DISCSUBTITLE"}, ItemKeySetSubtitle},
		{[]string{"GROUPING"}, ItemKeyContentGroup},
		{[]string{"TITLE"}, ItemKeyTrackTitle},
		{[]string{"SUBTITLE"}, ItemKeyTrackSubtitle},
		{[]string{"WORK"}, ItemKeyWork},
		{[]string{"MOVEMENTNAME"}, ItemKeyMovement},
		{[]string{"MOVEMENT"}, ItemKeyMovementNumber},
		{[]string{"MOVEMENTTOTAL"}, ItemKeyMovementTotal},
		{[]string{"RELEASECOUNTRY"}, ItemKeyReleaseCountry},
		{[]string{"ALBUMSORT"}, ItemKeyAlbumTitleSortOrder},
		{[]string{"ALBUMARTISTSORT"}, ItemKeyAlbumArtistSortOrder},
		{[]string{"TITLESORT"}, ItemKeyTrackTitleSortOrder},
		{[]string{"ARTISTSORT"}, ItemKeyTrackArtistSortOrder},
		{[]string{"ALBUMARTIST", "ALBUM ARTIST"}, ItemKeyAlbumArtist},
		{[]string{"ALBUMARTISTS", "ALBUM ARTISTS"}, ItemKeyAlbumArtists},
		{[]string{"ARTIST"}, ItemKeyTrackArtist},
		{[]string{"ARTISTS"}, ItemKeyTrackArtists},
		{[]string{"ARRANGER"}, ItemKeyArranger},
		{[]string{"AUTHOR", "WRITER"}, ItemKeyWriter},
		{[]string{"COMPOSER"}, ItemKeyComposer},
		{[]string{"CONDUCTOR"}, ItemKeyConductor},
		{[]string{"DIRECTOR"}, ItemKeyDirector},
		{[]string{"ENGINEER"}, ItemKeyEngineer},
		{[]string{"LYRICIST"}, ItemKeyLyricist},
		{[]string{"DJMIXER"}, ItemKeyMixDj},
		{[]string{"MIXER"}, ItemKeyMixEngineer},
		{[]string{"PERFORMER"}, ItemKeyPerformer},
		{[]string{"PRODUCER"}, ItemKeyProducer},
		{[]string{"PUBLISHER"}, ItemKeyPublisher},
		{[]string{"LABEL", "ORGANIZATION"}, ItemKeyLabel},
		{[]string{"REMIXER", "MIXARTIST"}, ItemKeyRemixer},
		{[]string{"DISCNUMBER"}, ItemKeyDiscNumber},
		{[]string{"DISCTOTAL", "TOTALDISCS"}, ItemKeyDiscTotal},
		{[]string{"TRACKNUMBER"}, ItemKeyTrackNumber},
		{[]string{"TRACKTOTAL", "TOTALTRACKS"}, ItemKeyTrackTotal},
		{[]string{"RATING"}, ItemKeyPopularimeter},
		{[]string{"DATE"}, ItemKeyRecordingDate},
		{[]string{"YEAR"}, ItemKeyYear},
		{[]string{"ORIGINALDATE", "ORIGINALYEAR"}, ItemKeyOriginalReleaseDate},
		{[]string{"RELEASEDATE"}, ItemKeyReleaseDate},
		{[]string{"ISRC"}, ItemKeyIsrc},
		{[]string{"BARCODE"}, ItemKeyBarcode},
		{[]string{"ACOUSTID_ID"}, ItemKeyAcoustId},
		{[]string{"ACOUSTID_FINGERPRINT"}, ItemKeyAcoustIdFingerprint},
		{[]string{"CATALOGNUMBER"}, ItemKeyCatalogNumber},
		{[]string{"COMPILATION"}, ItemKeyFlagCompilation},
		{[]string{"MEDIA"}, ItemKeyOriginalMediaType},
		{[]string{"ENCODEDBY", "ENCODED-BY", "ENCODED_BY"}, ItemKeyEncodedBy},
		{[]string{"ENCODER"}, ItemKeyEncoderSoftware},
		{[]string{"ENCODING", "ENCODERSETTINGS"}, ItemKeyEncoderSettings},
		{[]string{"REPLAYGAIN_ALBUM_GAIN"}, ItemKeyReplayGainAlbumGain},
		{[]string{"REPLAYGAIN_ALBUM_PEAK"}, ItemKeyReplayGainAlbumPeak},
		{[]string{"REPLAYGAIN_TRACK_GAIN"}, ItemKeyReplayGainTrackGain},
		{[]string{"REPLAYGAIN_TRACK_PEAK"}, ItemKeyReplayGainTrackPeak},
		{[]string{"R128_TRACK_GAIN"}, ItemKeyR128TrackGain},
		{[]string{"R128_ALBUM_GAIN"}, ItemKeyR128AlbumGain},
		{[]string{"GENRE"}, ItemKeyGenre},
		{[]string{"COLOR"}, ItemKeyColor},
		{[]string{"MOOD"}, ItemKeyMood},
		{[]string{"BPM"}, ItemKeyBpm},
		{[]string{"INITIALKEY", "KEY"}, ItemKeyInitialKey},
		{[]string{"COPYRIGHT"}, ItemKeyCopyrightMessage},
		{[]string{"LICENSE"}, ItemKeyLicense},
		{[]string{"COMMENT"}, ItemKeyComment},
		{[]string{"DESCRIPTION"}, ItemKeyDescription},
		{[]string{"LANGUAGE"}, ItemKeyLanguage},
		{[]string{"SCRIPT"}, ItemKeyScript},
		{[]string{"LYRICS"}, ItemKeyLyrics},
		{[]string{"UNSYNCEDLYRICS"}, ItemKeyUnsyncLyrics},
		{[]string{"MUSICBRAINZ_TRACKID"}, ItemKeyMusicBrainzRecordingId},
		{[]string{"MUSICBRAINZ_RELEASETRACKID"}, ItemKeyMusicBrainzTrackId},
		{[]string{"MUSICBRAINZ_ALBUMID"}, ItemKeyMusicBrainzReleaseId},
		{[]string{"MUSICBRAINZ_RELEASEGROUPID"}, ItemKeyMusicBrainzReleaseGroupId},
		{[]string{"MUSICBRAINZ_ARTISTID"}, ItemKeyMusicBrainzArtistId},
		{[]string{"MUSICBRAINZ_ALBUMARTISTID"}, ItemKeyMusicBrainzReleaseArtistId},
		{[]string{"MUSICBRAINZ_WORKID"}, ItemKeyMusicBrainzWorkId},
		{[]string{"RELEASETYPE"}, ItemKeyMusicBrainzReleaseType},
	})
)

// keyMapForTagType returns the key map for a [TagType], if the tag type has
// keyed fields (ID3v1 and Lyrics3 do not).
func keyMapForTagType(tagType TagType) *keyMap {
	switch tagType {
	case TagTypeAiffText:
		return aiffTextMap
	case TagTypeApe:
		return apeMap
	case TagTypeID3v2:
		return id3v2Map
	case TagTypeMp4Ilst:
		return ilstMap
	case TagTypeRiffInfo:
		return riffInfoMap
	case TagTypeVorbisComments:
		return vorbisMap
	}
	return nil
}

// ItemKeySupportedKeys returns all supported keys for a given [TagType].
func ItemKeySupportedKeys(tagType TagType) []ItemKey {
	switch tagType {
	case TagTypeID3v1:
		return id3v1ValidItemKeys
	case TagTypeLyrics3:
		return lyrics3v1ValidItemKeys
	case TagTypeLyrics3v2:
		return lyrics3v2ValidItemKeys
	}
	if m := keyMapForTagType(tagType); m != nil {
		return m.supportedKeys
	}
	return nil
}

// ItemKeyFromKey maps a format-specific key to an [ItemKey], if a variant exists.
//
// NOTE: If used with ID3v2, this will only check against the ID3v2.4 keys.
// To use a v2 or v3 key, see upgradeV2 and upgradeV3.
func ItemKeyFromKey(tagType TagType, key string) (ItemKey, bool) {
	if m := keyMapForTagType(tagType); m != nil {
		return m.itemKeyForKey(key)
	}
	return 0, false
}

// MapKey maps the [ItemKey] to a format-specific key.
func (k ItemKey) MapKey(tagType TagType) (string, bool) {
	if m := keyMapForTagType(tagType); m != nil {
		return m.keyForItemKey(k)
	}
	return "", false
}

// itemKeyVariantName returns the CamelCase name of the variant for Display.
func itemKeyVariantName(k ItemKey) string {
	names := [...]string{
		"AlbumTitle", "SetSubtitle", "ShowName", "ContentGroup", "AppleId3v2ContentGroup",
		"TrackTitle", "TrackSubtitle", "OriginalAlbumTitle", "OriginalArtist", "OriginalLyricist",
		"AlbumTitleSortOrder", "AlbumArtistSortOrder", "TrackTitleSortOrder", "TrackArtistSortOrder",
		"ShowNameSortOrder", "ComposerSortOrder", "AlbumArtist", "AlbumArtists", "TrackArtist",
		"TrackArtists", "Arranger", "Writer", "Composer", "Conductor", "Director", "Engineer",
		"Lyricist", "MixDj", "MixEngineer", "Performer", "Producer", "Publisher", "Label",
		"InternetRadioStationName", "InternetRadioStationOwner", "Remixer", "DiscNumber",
		"DiscTotal", "TrackNumber", "TrackTotal", "Popularimeter", "ParentalAdvisory",
		"RecordingDate", "Year", "ReleaseDate", "OriginalReleaseDate", "Isrc", "Barcode",
		"AcoustId", "AcoustIdFingerprint", "CatalogNumber", "Work", "Movement", "MovementNumber",
		"MovementTotal", "ReleaseCountry", "MusicBrainzRecordingId", "MusicBrainzTrackId",
		"MusicBrainzReleaseId", "MusicBrainzReleaseGroupId", "MusicBrainzArtistId",
		"MusicBrainzReleaseArtistId", "MusicBrainzWorkId", "MusicBrainzReleaseType",
		"FlagCompilation", "FlagPodcast", "FileOwner", "TaggingTime", "Length",
		"OriginalFileName", "OriginalMediaType", "EncodedBy", "EncoderSoftware",
		"EncoderSettings", "EncodingTime", "ReplayGainAlbumGain", "ReplayGainAlbumPeak",
		"ReplayGainTrackGain", "ReplayGainTrackPeak", "R128TrackGain", "R128AlbumGain",
		"AudioFileUrl", "AudioSourceUrl", "CommercialInformationUrl", "CopyrightUrl",
		"TrackArtistUrl", "RadioStationUrl", "PaymentUrl", "PublisherUrl", "Genre",
		"InitialKey", "Color", "Mood", "Bpm", "IntegerBpm", "CopyrightMessage", "License",
		"PodcastDescription", "PodcastSeriesCategory", "PodcastUrl", "PodcastGlobalUniqueId",
		"PodcastKeywords", "Comment", "Description", "Language", "Script", "Lyrics",
		"UnsyncLyrics", "AppleXid", "Unknown",
	}
	if int(k) < len(names) {
		return names[k]
	}
	return "Unknown"
}
