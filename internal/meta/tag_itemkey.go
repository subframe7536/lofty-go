package meta

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

// asciiLower lowercases ASCII letters only, like Rust's `eq_ignore_ascii_case` /
// `to_ascii_lowercase` (strings.ToLower would also fold e.g. U+212A KELVIN SIGN).
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// keyMapEntry is a single key → ItemKey mapping. Multiple keys may map to the
// same ItemKey; the first listed key is the preferred ("standard") one.
type keyMapEntry struct {
	keys    []string
	itemKey ItemKey
	// Additional [ItemKey]s that map to keys[0] on write but lose to
	// itemKey on read (Rust's `"K" => A | B | C` gen_map! entries).
	aliases []ItemKey
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
			// last entry wins for a given key (Rust HashMap::insert)
			m.byKey[asciiLower(k)] = e.itemKey
		}
		for _, v := range append([]ItemKey{e.itemKey}, e.aliases...) {
			if !seen[v] {
				seen[v] = true
				m.supportedKeys = append(m.supportedKeys, v)
			}
		}
	}
	for _, e := range entries {
		if len(e.keys) == 0 {
			continue
		}
		for _, v := range append([]ItemKey{e.itemKey}, e.aliases...) {
			if _, exists := m.byItemKey[v]; !exists {
				m.byItemKey[v] = e.keys[0]
			}
		}
	}
	return m
}

// itemKeyForKey returns the [ItemKey] for a format-specific key (case-insensitive).
func (m *keyMap) itemKeyForKey(key string) (ItemKey, bool) {
	k, ok := m.byKey[asciiLower(key)]
	return k, ok
}

// keyForItemKey returns the preferred format-specific key for an [ItemKey].
func (m *keyMap) keyForItemKey(k ItemKey) (string, bool) {
	s, ok := m.byItemKey[k]
	return s, ok
}

var (
	aiffTextMap = newKeyMap([]keyMapEntry{
		{keys: []string{"NAME"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"AUTH"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"(c) "}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"ANNO"}, itemKey: ItemKeyComment},
	})

	apeMap = newKeyMap([]keyMapEntry{
		{keys: []string{"Album"}, itemKey: ItemKeyAlbumTitle},
		{keys: []string{"DiscSubtitle"}, itemKey: ItemKeySetSubtitle},
		{keys: []string{"Grouping"}, itemKey: ItemKeyContentGroup},
		{keys: []string{"Title"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"Subtitle"}, itemKey: ItemKeyTrackSubtitle},
		{keys: []string{"WORKTITLE"}, itemKey: ItemKeyWork},
		{keys: []string{"MOVEMENTNAME"}, itemKey: ItemKeyMovement},
		{keys: []string{"MOVEMENT"}, itemKey: ItemKeyMovementNumber},
		{keys: []string{"MOVEMENTTOTAL"}, itemKey: ItemKeyMovementTotal},
		{keys: []string{"RELEASECOUNTRY"}, itemKey: ItemKeyReleaseCountry},
		{keys: []string{"ALBUMSORT"}, itemKey: ItemKeyAlbumTitleSortOrder},
		{keys: []string{"ALBUMARTISTSORT"}, itemKey: ItemKeyAlbumArtistSortOrder},
		{keys: []string{"TITLESORT"}, itemKey: ItemKeyTrackTitleSortOrder},
		{keys: []string{"ARTISTSORT"}, itemKey: ItemKeyTrackArtistSortOrder},
		{keys: []string{"Album Artist", "ALBUMARTIST"}, itemKey: ItemKeyAlbumArtist},
		{keys: []string{"ALBUMARTISTS", "ALBUM ARTISTS"}, itemKey: ItemKeyAlbumArtists},
		{keys: []string{"Artist"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"Artists"}, itemKey: ItemKeyTrackArtists},
		{keys: []string{"Arranger"}, itemKey: ItemKeyArranger},
		{keys: []string{"Writer"}, itemKey: ItemKeyWriter},
		{keys: []string{"Composer"}, itemKey: ItemKeyComposer},
		{keys: []string{"Conductor"}, itemKey: ItemKeyConductor},
		{keys: []string{"Director"}, itemKey: ItemKeyDirector},
		{keys: []string{"Engineer"}, itemKey: ItemKeyEngineer},
		{keys: []string{"Lyricist"}, itemKey: ItemKeyLyricist},
		{keys: []string{"DjMixer"}, itemKey: ItemKeyMixDj},
		{keys: []string{"Mixer"}, itemKey: ItemKeyMixEngineer},
		{keys: []string{"Performer"}, itemKey: ItemKeyPerformer},
		{keys: []string{"Producer"}, itemKey: ItemKeyProducer},
		{keys: []string{"Label"}, itemKey: ItemKeyLabel},
		{keys: []string{"MixArtist"}, itemKey: ItemKeyRemixer},
		{keys: []string{"Disc"}, itemKey: ItemKeyDiscNumber},
		{keys: []string{"Disc"}, itemKey: ItemKeyDiscTotal},
		{keys: []string{"Track"}, itemKey: ItemKeyTrackNumber},
		{keys: []string{"Track"}, itemKey: ItemKeyTrackTotal},
		// For some reason, the ecosystem agreed on the key "Year", even for full date strings.
		{keys: []string{"Year"}, itemKey: ItemKeyRecordingDate, aliases: []ItemKey{ItemKeyYear}},
		{keys: []string{"ORIGINALYEAR"}, itemKey: ItemKeyOriginalReleaseDate},
		{keys: []string{"RELEASEDATE"}, itemKey: ItemKeyReleaseDate},
		{keys: []string{"ISRC"}, itemKey: ItemKeyIsrc},
		{keys: []string{"Barcode"}, itemKey: ItemKeyBarcode},
		{keys: []string{"ACOUSTID_ID"}, itemKey: ItemKeyAcoustId},
		{keys: []string{"ACOUSTID_FINGERPRINT"}, itemKey: ItemKeyAcoustIdFingerprint},
		{keys: []string{"CatalogNumber"}, itemKey: ItemKeyCatalogNumber},
		{keys: []string{"Compilation"}, itemKey: ItemKeyFlagCompilation},
		{keys: []string{"Media"}, itemKey: ItemKeyOriginalMediaType},
		{keys: []string{"EncodedBy"}, itemKey: ItemKeyEncodedBy},
		{keys: []string{"REPLAYGAIN_ALBUM_GAIN"}, itemKey: ItemKeyReplayGainAlbumGain},
		{keys: []string{"REPLAYGAIN_ALBUM_PEAK"}, itemKey: ItemKeyReplayGainAlbumPeak},
		{keys: []string{"REPLAYGAIN_TRACK_GAIN"}, itemKey: ItemKeyReplayGainTrackGain},
		{keys: []string{"REPLAYGAIN_TRACK_PEAK"}, itemKey: ItemKeyReplayGainTrackPeak},
		{keys: []string{"Genre"}, itemKey: ItemKeyGenre},
		{keys: []string{"Color"}, itemKey: ItemKeyColor},
		{keys: []string{"Mood"}, itemKey: ItemKeyMood},
		{keys: []string{"Copyright"}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"Comment"}, itemKey: ItemKeyComment},
		{keys: []string{"language"}, itemKey: ItemKeyLanguage},
		{keys: []string{"Script"}, itemKey: ItemKeyScript},
		{keys: []string{"Lyrics"}, itemKey: ItemKeyLyrics},
		{keys: []string{"UnsyncedLyrics"}, itemKey: ItemKeyUnsyncLyrics},
		{keys: []string{"MUSICBRAINZ_TRACKID"}, itemKey: ItemKeyMusicBrainzRecordingId},
		{keys: []string{"MUSICBRAINZ_RELEASETRACKID"}, itemKey: ItemKeyMusicBrainzTrackId},
		{keys: []string{"MUSICBRAINZ_ALBUMID"}, itemKey: ItemKeyMusicBrainzReleaseId},
		{keys: []string{"MUSICBRAINZ_RELEASEGROUPID"}, itemKey: ItemKeyMusicBrainzReleaseGroupId},
		{keys: []string{"MUSICBRAINZ_ARTISTID"}, itemKey: ItemKeyMusicBrainzArtistId},
		{keys: []string{"MUSICBRAINZ_ALBUMARTISTID"}, itemKey: ItemKeyMusicBrainzReleaseArtistId},
		{keys: []string{"MUSICBRAINZ_WORKID"}, itemKey: ItemKeyMusicBrainzWorkId},
		{keys: []string{"MUSICBRAINZ_ALBUMTYPE"}, itemKey: ItemKeyMusicBrainzReleaseType},
	})

	id3v2Map = newKeyMap([]keyMapEntry{
		{keys: []string{"TALB"}, itemKey: ItemKeyAlbumTitle},
		{keys: []string{"TSST"}, itemKey: ItemKeySetSubtitle},
		{keys: []string{"TIT1"}, itemKey: ItemKeyContentGroup},
		{keys: []string{"GRP1"}, itemKey: ItemKeyAppleId3v2ContentGroup},
		{keys: []string{"TIT2"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"TIT3"}, itemKey: ItemKeyTrackSubtitle},
		{keys: []string{"TOAL"}, itemKey: ItemKeyOriginalAlbumTitle},
		{keys: []string{"TOPE"}, itemKey: ItemKeyOriginalArtist},
		{keys: []string{"TOLY"}, itemKey: ItemKeyOriginalLyricist},
		{keys: []string{"TSOA"}, itemKey: ItemKeyAlbumTitleSortOrder},
		{keys: []string{"TSO2"}, itemKey: ItemKeyAlbumArtistSortOrder},
		{keys: []string{"TSOT"}, itemKey: ItemKeyTrackTitleSortOrder},
		{keys: []string{"TSOP"}, itemKey: ItemKeyTrackArtistSortOrder},
		{keys: []string{"TSOC"}, itemKey: ItemKeyComposerSortOrder},
		{keys: []string{"TPE2", "ALBUMARTIST", "ALBUM ARTIST"}, itemKey: ItemKeyAlbumArtist},
		{keys: []string{"ALBUMARTISTS", "ALBUM ARTISTS"}, itemKey: ItemKeyAlbumArtists},
		{keys: []string{"TPE1"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"ARTISTS"}, itemKey: ItemKeyTrackArtists},
		{keys: []string{"TEXT"}, itemKey: ItemKeyWriter},
		{keys: []string{"TCOM"}, itemKey: ItemKeyComposer},
		{keys: []string{"TPE3"}, itemKey: ItemKeyConductor},
		{keys: []string{"DIRECTOR"}, itemKey: ItemKeyDirector},
		{keys: []string{"TEXT"}, itemKey: ItemKeyLyricist},
		{keys: []string{"TIPL"}, itemKey: ItemKeyProducer, aliases: []ItemKey{ItemKeyArranger, ItemKeyEngineer, ItemKeyMixDj, ItemKeyMixEngineer}},
		{keys: []string{"TPUB"}, itemKey: ItemKeyPublisher, aliases: []ItemKey{ItemKeyLabel}},
		{keys: []string{"TRSN"}, itemKey: ItemKeyInternetRadioStationName},
		{keys: []string{"TRSO"}, itemKey: ItemKeyInternetRadioStationOwner},
		{keys: []string{"TPE4"}, itemKey: ItemKeyRemixer},
		{keys: []string{"TPOS"}, itemKey: ItemKeyDiscNumber},
		{keys: []string{"TPOS"}, itemKey: ItemKeyDiscTotal},
		{keys: []string{"TRCK"}, itemKey: ItemKeyTrackNumber},
		{keys: []string{"TRCK"}, itemKey: ItemKeyTrackTotal},
		{keys: []string{"POPM"}, itemKey: ItemKeyPopularimeter},
		{keys: []string{"ITUNESADVISORY"}, itemKey: ItemKeyParentalAdvisory},
		{keys: []string{"TDRC"}, itemKey: ItemKeyRecordingDate},
		{keys: []string{"TDOR"}, itemKey: ItemKeyOriginalReleaseDate},
		{keys: []string{"TSRC"}, itemKey: ItemKeyIsrc},
		{keys: []string{"BARCODE"}, itemKey: ItemKeyBarcode},
		{keys: []string{"Acoustid Id"}, itemKey: ItemKeyAcoustId},
		{keys: []string{"Acoustid Fingerprint"}, itemKey: ItemKeyAcoustIdFingerprint},
		{keys: []string{"CATALOGNUMBER"}, itemKey: ItemKeyCatalogNumber},
		// ID3v2.4: TXXX:WORK (Apple uses TIT1/ContentGroup, see GRP1/AppleId3v2ContentGroup)
		{keys: []string{"WORK"}, itemKey: ItemKeyWork},
		{keys: []string{"MVNM"}, itemKey: ItemKeyMovement},
		{keys: []string{"MVIN"}, itemKey: ItemKeyMovementNumber},
		{keys: []string{"MVIN"}, itemKey: ItemKeyMovementTotal},
		{keys: []string{"TCMP"}, itemKey: ItemKeyFlagCompilation},
		{keys: []string{"PCST"}, itemKey: ItemKeyFlagPodcast},
		{keys: []string{"TOWN"}, itemKey: ItemKeyFileOwner},
		{keys: []string{"TDTG"}, itemKey: ItemKeyTaggingTime},
		{keys: []string{"TLEN"}, itemKey: ItemKeyLength},
		{keys: []string{"TOFN"}, itemKey: ItemKeyOriginalFileName},
		{keys: []string{"TMED"}, itemKey: ItemKeyOriginalMediaType},
		{keys: []string{"TENC"}, itemKey: ItemKeyEncodedBy},
		{keys: []string{"TSSE"}, itemKey: ItemKeyEncoderSettings, aliases: []ItemKey{ItemKeyEncoderSoftware}},
		{keys: []string{"TDEN"}, itemKey: ItemKeyEncodingTime},
		{keys: []string{"REPLAYGAIN_ALBUM_GAIN"}, itemKey: ItemKeyReplayGainAlbumGain},
		{keys: []string{"REPLAYGAIN_ALBUM_PEAK"}, itemKey: ItemKeyReplayGainAlbumPeak},
		{keys: []string{"REPLAYGAIN_TRACK_GAIN"}, itemKey: ItemKeyReplayGainTrackGain},
		{keys: []string{"REPLAYGAIN_TRACK_PEAK"}, itemKey: ItemKeyReplayGainTrackPeak},
		{keys: []string{"WOAF"}, itemKey: ItemKeyAudioFileUrl},
		{keys: []string{"WOAS"}, itemKey: ItemKeyAudioSourceUrl},
		{keys: []string{"WCOM"}, itemKey: ItemKeyCommercialInformationUrl},
		{keys: []string{"WCOP"}, itemKey: ItemKeyCopyrightUrl},
		{keys: []string{"WOAR"}, itemKey: ItemKeyTrackArtistUrl},
		{keys: []string{"WORS"}, itemKey: ItemKeyRadioStationUrl},
		{keys: []string{"WPAY"}, itemKey: ItemKeyPaymentUrl},
		{keys: []string{"WPUB"}, itemKey: ItemKeyPublisherUrl},
		{keys: []string{"TCON"}, itemKey: ItemKeyGenre},
		{keys: []string{"TKEY"}, itemKey: ItemKeyInitialKey},
		{keys: []string{"COLOR"}, itemKey: ItemKeyColor},
		{keys: []string{"TMOO"}, itemKey: ItemKeyMood},
		{keys: []string{"TBPM"}, itemKey: ItemKeyIntegerBpm},
		{keys: []string{"TCOP"}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"TDES"}, itemKey: ItemKeyPodcastDescription},
		{keys: []string{"TCAT"}, itemKey: ItemKeyPodcastSeriesCategory},
		{keys: []string{"WFED"}, itemKey: ItemKeyPodcastUrl},
		{keys: []string{"TDRL"}, itemKey: ItemKeyReleaseDate},
		{keys: []string{"TGID"}, itemKey: ItemKeyPodcastGlobalUniqueId},
		{keys: []string{"TKWD"}, itemKey: ItemKeyPodcastKeywords},
		{keys: []string{"COMM"}, itemKey: ItemKeyComment},
		{keys: []string{"TLAN"}, itemKey: ItemKeyLanguage},
		// See rust docs: SYLT has its own standard; only UnsyncLyrics is mapped.
		{keys: []string{"USLT"}, itemKey: ItemKeyUnsyncLyrics},
		{keys: []string{"UFID"}, itemKey: ItemKeyMusicBrainzRecordingId},
		{keys: []string{"MusicBrainz Release Track Id"}, itemKey: ItemKeyMusicBrainzTrackId},
		{keys: []string{"MusicBrainz Album Id"}, itemKey: ItemKeyMusicBrainzReleaseId},
		{keys: []string{"MusicBrainz Release Group Id"}, itemKey: ItemKeyMusicBrainzReleaseGroupId},
		{keys: []string{"MusicBrainz Artist Id"}, itemKey: ItemKeyMusicBrainzArtistId},
		{keys: []string{"MusicBrainz Album Artist Id"}, itemKey: ItemKeyMusicBrainzReleaseArtistId},
		{keys: []string{"MusicBrainz Work Id"}, itemKey: ItemKeyMusicBrainzWorkId},
		{keys: []string{"MusicBrainz Album Type"}, itemKey: ItemKeyMusicBrainzReleaseType},
		{keys: []string{"MusicBrainz Album Release Country"}, itemKey: ItemKeyReleaseCountry},
	})

	ilstMap = newKeyMap([]keyMapEntry{
		{keys: []string{"©alb"}, itemKey: ItemKeyAlbumTitle},
		{keys: []string{"----:com.apple.iTunes:DISCSUBTITLE"}, itemKey: ItemKeySetSubtitle},
		{keys: []string{"tvsh"}, itemKey: ItemKeyShowName},
		{keys: []string{"©grp"}, itemKey: ItemKeyContentGroup},
		{keys: []string{"©nam"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"----:com.apple.iTunes:SUBTITLE"}, itemKey: ItemKeyTrackSubtitle},
		{keys: []string{"©wrk"}, itemKey: ItemKeyWork},
		{keys: []string{"©mvn"}, itemKey: ItemKeyMovement},
		{keys: []string{"©mvi"}, itemKey: ItemKeyMovementNumber},
		{keys: []string{"©mvc"}, itemKey: ItemKeyMovementTotal},
		{keys: []string{"soal"}, itemKey: ItemKeyAlbumTitleSortOrder},
		{keys: []string{"soaa"}, itemKey: ItemKeyAlbumArtistSortOrder},
		{keys: []string{"sonm"}, itemKey: ItemKeyTrackTitleSortOrder},
		{keys: []string{"soar"}, itemKey: ItemKeyTrackArtistSortOrder},
		{keys: []string{"sosn"}, itemKey: ItemKeyShowNameSortOrder},
		{keys: []string{"soco"}, itemKey: ItemKeyComposerSortOrder},
		{keys: []string{"aART"}, itemKey: ItemKeyAlbumArtist},
		{keys: []string{"----:com.apple.iTunes:ALBUMARTISTS"}, itemKey: ItemKeyAlbumArtists},
		{keys: []string{"©ART"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"----:com.apple.iTunes:ARTISTS"}, itemKey: ItemKeyTrackArtists},
		{keys: []string{"©wrt"}, itemKey: ItemKeyComposer},
		{keys: []string{"©dir"}, itemKey: ItemKeyDirector},
		{keys: []string{"----:com.apple.iTunes:CONDUCTOR"}, itemKey: ItemKeyConductor},
		{keys: []string{"----:com.apple.iTunes:ENGINEER"}, itemKey: ItemKeyEngineer},
		{keys: []string{"----:com.apple.iTunes:LYRICIST"}, itemKey: ItemKeyLyricist},
		{keys: []string{"----:com.apple.iTunes:DJMIXER"}, itemKey: ItemKeyMixDj},
		{keys: []string{"----:com.apple.iTunes:MIXER"}, itemKey: ItemKeyMixEngineer},
		{keys: []string{"----:com.apple.iTunes:PRODUCER"}, itemKey: ItemKeyProducer},
		{keys: []string{"----:com.apple.iTunes:LABEL"}, itemKey: ItemKeyLabel},
		{keys: []string{"----:com.apple.iTunes:REMIXER"}, itemKey: ItemKeyRemixer},
		{keys: []string{"disk"}, itemKey: ItemKeyDiscNumber},
		{keys: []string{"disk"}, itemKey: ItemKeyDiscTotal},
		{keys: []string{"trkn"}, itemKey: ItemKeyTrackNumber},
		{keys: []string{"trkn"}, itemKey: ItemKeyTrackTotal},
		{keys: []string{"rate"}, itemKey: ItemKeyPopularimeter},
		{keys: []string{"rtng"}, itemKey: ItemKeyParentalAdvisory},
		{keys: []string{"©day"}, itemKey: ItemKeyRecordingDate},
		{keys: []string{"----:com.apple.iTunes:ORIGINALDATE"}, itemKey: ItemKeyOriginalReleaseDate},
		{keys: []string{"----:com.apple.iTunes:RELEASEDATE"}, itemKey: ItemKeyReleaseDate},
		{keys: []string{"----:com.apple.iTunes:ISRC"}, itemKey: ItemKeyIsrc},
		{keys: []string{"----:com.apple.iTunes:BARCODE"}, itemKey: ItemKeyBarcode},
		{keys: []string{"----:com.apple.iTunes:Acoustid Id"}, itemKey: ItemKeyAcoustId},
		{keys: []string{"----:com.apple.iTunes:Acoustid Fingerprint"}, itemKey: ItemKeyAcoustIdFingerprint},
		{keys: []string{"----:com.apple.iTunes:CATALOGNUMBER"}, itemKey: ItemKeyCatalogNumber},
		{keys: []string{"cpil"}, itemKey: ItemKeyFlagCompilation},
		{keys: []string{"pcst"}, itemKey: ItemKeyFlagPodcast},
		{keys: []string{"----:com.apple.iTunes:MEDIA"}, itemKey: ItemKeyOriginalMediaType},
		{keys: []string{"©enc"}, itemKey: ItemKeyEncodedBy},
		{keys: []string{"©too"}, itemKey: ItemKeyEncoderSoftware},
		{keys: []string{"©gen"}, itemKey: ItemKeyGenre},
		{keys: []string{"----:com.apple.iTunes:COLOR"}, itemKey: ItemKeyColor},
		{keys: []string{"----:com.apple.iTunes:MOOD"}, itemKey: ItemKeyMood},
		{keys: []string{"tmpo"}, itemKey: ItemKeyIntegerBpm},
		{keys: []string{"----:com.apple.iTunes:BPM"}, itemKey: ItemKeyBpm},
		{keys: []string{"----:com.apple.iTunes:initialkey"}, itemKey: ItemKeyInitialKey},
		{keys: []string{"----:com.apple.iTunes:replaygain_album_gain"}, itemKey: ItemKeyReplayGainAlbumGain},
		{keys: []string{"----:com.apple.iTunes:replaygain_album_peak"}, itemKey: ItemKeyReplayGainAlbumPeak},
		{keys: []string{"----:com.apple.iTunes:replaygain_track_gain"}, itemKey: ItemKeyReplayGainTrackGain},
		{keys: []string{"----:com.apple.iTunes:replaygain_track_peak"}, itemKey: ItemKeyReplayGainTrackPeak},
		{keys: []string{"cprt"}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"----:com.apple.iTunes:LICENSE"}, itemKey: ItemKeyLicense},
		{keys: []string{"ldes"}, itemKey: ItemKeyPodcastDescription},
		{keys: []string{"catg"}, itemKey: ItemKeyPodcastSeriesCategory},
		{keys: []string{"purl"}, itemKey: ItemKeyPodcastUrl},
		{keys: []string{"egid"}, itemKey: ItemKeyPodcastGlobalUniqueId},
		{keys: []string{"keyw"}, itemKey: ItemKeyPodcastKeywords},
		{keys: []string{"©cmt"}, itemKey: ItemKeyComment},
		{keys: []string{"desc"}, itemKey: ItemKeyDescription},
		{keys: []string{"----:com.apple.iTunes:LANGUAGE"}, itemKey: ItemKeyLanguage},
		{keys: []string{"----:com.apple.iTunes:SCRIPT"}, itemKey: ItemKeyScript},
		{keys: []string{"©lyr"}, itemKey: ItemKeyLyrics, aliases: []ItemKey{ItemKeyUnsyncLyrics}},
		{keys: []string{"xid "}, itemKey: ItemKeyAppleXid},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Track Id"}, itemKey: ItemKeyMusicBrainzRecordingId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Release Track Id"}, itemKey: ItemKeyMusicBrainzTrackId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Album Id"}, itemKey: ItemKeyMusicBrainzReleaseId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Release Group Id"}, itemKey: ItemKeyMusicBrainzReleaseGroupId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Artist Id"}, itemKey: ItemKeyMusicBrainzArtistId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Album Artist Id"}, itemKey: ItemKeyMusicBrainzReleaseArtistId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Work Id"}, itemKey: ItemKeyMusicBrainzWorkId},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Album Type"}, itemKey: ItemKeyMusicBrainzReleaseType},
		{keys: []string{"----:com.apple.iTunes:MusicBrainz Album Release Country"}, itemKey: ItemKeyReleaseCountry},
	})

	riffInfoMap = newKeyMap([]keyMapEntry{
		{keys: []string{"IPRD"}, itemKey: ItemKeyAlbumTitle},
		{keys: []string{"INAM"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"IART"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"IWRI"}, itemKey: ItemKeyWriter},
		{keys: []string{"IMUS"}, itemKey: ItemKeyComposer},
		{keys: []string{"IPRO"}, itemKey: ItemKeyProducer},
		{keys: []string{"IPRT", "ITRK"}, itemKey: ItemKeyTrackNumber},
		{keys: []string{"IFRM"}, itemKey: ItemKeyTrackTotal},
		{keys: []string{"IRTD"}, itemKey: ItemKeyPopularimeter},
		{keys: []string{"ICRD"}, itemKey: ItemKeyRecordingDate},
		{keys: []string{"TLEN"}, itemKey: ItemKeyLength},
		{keys: []string{"ISRF"}, itemKey: ItemKeyOriginalMediaType},
		{keys: []string{"ITCH"}, itemKey: ItemKeyEncodedBy},
		{keys: []string{"ISFT"}, itemKey: ItemKeyEncoderSoftware},
		{keys: []string{"IGNR"}, itemKey: ItemKeyGenre},
		{keys: []string{"ICOP"}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"ICMT"}, itemKey: ItemKeyComment},
		{keys: []string{"ILNG"}, itemKey: ItemKeyLanguage},
		{keys: []string{"ICNT"}, itemKey: ItemKeyReleaseCountry},
	})

	vorbisMap = newKeyMap([]keyMapEntry{
		{keys: []string{"ALBUM"}, itemKey: ItemKeyAlbumTitle},
		{keys: []string{"DISCSUBTITLE"}, itemKey: ItemKeySetSubtitle},
		{keys: []string{"GROUPING"}, itemKey: ItemKeyContentGroup},
		{keys: []string{"TITLE"}, itemKey: ItemKeyTrackTitle},
		{keys: []string{"SUBTITLE"}, itemKey: ItemKeyTrackSubtitle},
		{keys: []string{"WORK"}, itemKey: ItemKeyWork},
		{keys: []string{"MOVEMENTNAME"}, itemKey: ItemKeyMovement},
		{keys: []string{"MOVEMENT"}, itemKey: ItemKeyMovementNumber},
		{keys: []string{"MOVEMENTTOTAL"}, itemKey: ItemKeyMovementTotal},
		{keys: []string{"RELEASECOUNTRY"}, itemKey: ItemKeyReleaseCountry},
		{keys: []string{"ALBUMSORT"}, itemKey: ItemKeyAlbumTitleSortOrder},
		{keys: []string{"ALBUMARTISTSORT"}, itemKey: ItemKeyAlbumArtistSortOrder},
		{keys: []string{"TITLESORT"}, itemKey: ItemKeyTrackTitleSortOrder},
		{keys: []string{"ARTISTSORT"}, itemKey: ItemKeyTrackArtistSortOrder},
		{keys: []string{"ALBUMARTIST", "ALBUM ARTIST"}, itemKey: ItemKeyAlbumArtist},
		{keys: []string{"ALBUMARTISTS", "ALBUM ARTISTS"}, itemKey: ItemKeyAlbumArtists},
		{keys: []string{"ARTIST"}, itemKey: ItemKeyTrackArtist},
		{keys: []string{"ARTISTS"}, itemKey: ItemKeyTrackArtists},
		{keys: []string{"ARRANGER"}, itemKey: ItemKeyArranger},
		{keys: []string{"AUTHOR", "WRITER"}, itemKey: ItemKeyWriter},
		{keys: []string{"COMPOSER"}, itemKey: ItemKeyComposer},
		{keys: []string{"CONDUCTOR"}, itemKey: ItemKeyConductor},
		{keys: []string{"DIRECTOR"}, itemKey: ItemKeyDirector},
		{keys: []string{"ENGINEER"}, itemKey: ItemKeyEngineer},
		{keys: []string{"LYRICIST"}, itemKey: ItemKeyLyricist},
		{keys: []string{"DJMIXER"}, itemKey: ItemKeyMixDj},
		{keys: []string{"MIXER"}, itemKey: ItemKeyMixEngineer},
		{keys: []string{"PERFORMER"}, itemKey: ItemKeyPerformer},
		{keys: []string{"PRODUCER"}, itemKey: ItemKeyProducer},
		{keys: []string{"PUBLISHER"}, itemKey: ItemKeyPublisher},
		{keys: []string{"LABEL", "ORGANIZATION"}, itemKey: ItemKeyLabel},
		{keys: []string{"REMIXER", "MIXARTIST"}, itemKey: ItemKeyRemixer},
		{keys: []string{"DISCNUMBER"}, itemKey: ItemKeyDiscNumber},
		{keys: []string{"DISCTOTAL", "TOTALDISCS"}, itemKey: ItemKeyDiscTotal},
		{keys: []string{"TRACKNUMBER"}, itemKey: ItemKeyTrackNumber},
		{keys: []string{"TRACKTOTAL", "TOTALTRACKS"}, itemKey: ItemKeyTrackTotal},
		{keys: []string{"RATING"}, itemKey: ItemKeyPopularimeter},
		{keys: []string{"DATE"}, itemKey: ItemKeyRecordingDate},
		{keys: []string{"YEAR"}, itemKey: ItemKeyYear},
		{keys: []string{"ORIGINALDATE", "ORIGINALYEAR"}, itemKey: ItemKeyOriginalReleaseDate},
		{keys: []string{"RELEASEDATE"}, itemKey: ItemKeyReleaseDate},
		{keys: []string{"ISRC"}, itemKey: ItemKeyIsrc},
		{keys: []string{"BARCODE"}, itemKey: ItemKeyBarcode},
		{keys: []string{"ACOUSTID_ID"}, itemKey: ItemKeyAcoustId},
		{keys: []string{"ACOUSTID_FINGERPRINT"}, itemKey: ItemKeyAcoustIdFingerprint},
		{keys: []string{"CATALOGNUMBER"}, itemKey: ItemKeyCatalogNumber},
		{keys: []string{"COMPILATION"}, itemKey: ItemKeyFlagCompilation},
		{keys: []string{"MEDIA"}, itemKey: ItemKeyOriginalMediaType},
		{keys: []string{"ENCODEDBY", "ENCODED-BY", "ENCODED_BY"}, itemKey: ItemKeyEncodedBy},
		{keys: []string{"ENCODER"}, itemKey: ItemKeyEncoderSoftware},
		{keys: []string{"ENCODING", "ENCODERSETTINGS"}, itemKey: ItemKeyEncoderSettings},
		{keys: []string{"REPLAYGAIN_ALBUM_GAIN"}, itemKey: ItemKeyReplayGainAlbumGain},
		{keys: []string{"REPLAYGAIN_ALBUM_PEAK"}, itemKey: ItemKeyReplayGainAlbumPeak},
		{keys: []string{"REPLAYGAIN_TRACK_GAIN"}, itemKey: ItemKeyReplayGainTrackGain},
		{keys: []string{"REPLAYGAIN_TRACK_PEAK"}, itemKey: ItemKeyReplayGainTrackPeak},
		{keys: []string{"R128_TRACK_GAIN"}, itemKey: ItemKeyR128TrackGain},
		{keys: []string{"R128_ALBUM_GAIN"}, itemKey: ItemKeyR128AlbumGain},
		{keys: []string{"GENRE"}, itemKey: ItemKeyGenre},
		{keys: []string{"COLOR"}, itemKey: ItemKeyColor},
		{keys: []string{"MOOD"}, itemKey: ItemKeyMood},
		{keys: []string{"BPM"}, itemKey: ItemKeyBpm},
		{keys: []string{"INITIALKEY", "KEY"}, itemKey: ItemKeyInitialKey},
		{keys: []string{"COPYRIGHT"}, itemKey: ItemKeyCopyrightMessage},
		{keys: []string{"LICENSE"}, itemKey: ItemKeyLicense},
		{keys: []string{"COMMENT"}, itemKey: ItemKeyComment},
		{keys: []string{"DESCRIPTION"}, itemKey: ItemKeyDescription},
		{keys: []string{"LANGUAGE"}, itemKey: ItemKeyLanguage},
		{keys: []string{"SCRIPT"}, itemKey: ItemKeyScript},
		{keys: []string{"LYRICS"}, itemKey: ItemKeyLyrics},
		{keys: []string{"UNSYNCEDLYRICS"}, itemKey: ItemKeyUnsyncLyrics},
		{keys: []string{"MUSICBRAINZ_TRACKID"}, itemKey: ItemKeyMusicBrainzRecordingId},
		{keys: []string{"MUSICBRAINZ_RELEASETRACKID"}, itemKey: ItemKeyMusicBrainzTrackId},
		{keys: []string{"MUSICBRAINZ_ALBUMID"}, itemKey: ItemKeyMusicBrainzReleaseId},
		{keys: []string{"MUSICBRAINZ_RELEASEGROUPID"}, itemKey: ItemKeyMusicBrainzReleaseGroupId},
		{keys: []string{"MUSICBRAINZ_ARTISTID"}, itemKey: ItemKeyMusicBrainzArtistId},
		{keys: []string{"MUSICBRAINZ_ALBUMARTISTID"}, itemKey: ItemKeyMusicBrainzReleaseArtistId},
		{keys: []string{"MUSICBRAINZ_WORKID"}, itemKey: ItemKeyMusicBrainzWorkId},
		{keys: []string{"RELEASETYPE"}, itemKey: ItemKeyMusicBrainzReleaseType},
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

// ItemKeyOrUnknown maps a format-specific key string to an [ItemKey],
// falling back to [ItemKeyUnknown].
func ItemKeyOrUnknown(tagType TagType, key string) ItemKey {
	if ik, ok := ItemKeyFromKey(tagType, key); ok {
		return ik
	}
	return ItemKeyUnknown
}
