package id3v1

import (
	"bytes"
	"io"

	"github.com/subframe7536/lofty-go/internal/meta"
)

var Id3v1TagMarker = []byte("TAG")

// Id3v1Tag is an ID3v1 tag.
type Id3v1Tag struct {
	Title       Option[string]
	Artist      Option[string]
	Album       Option[string]
	Year        Option[uint16]
	Comment     Option[string]
	TrackNumber Option[uint8]
	Genre       Option[uint8]
}

// Id3v1ParseError is returned when an ID3v1 tag fails to parse.
type Id3v1ParseError struct {
	Message string
	Source  error
}

func (e *Id3v1ParseError) Error() string {
	if e.Message != "" {
		return "ID3v1: " + e.Message
	}
	return "failed to parse ID3v1 tag"
}
func (e *Id3v1ParseError) Unwrap() error { return e.Source }

// Id3v1EncodingError is returned when an ID3v1 tag fails to encode.
type Id3v1EncodingError struct {
	Message string
	Source  error
}

func (e *Id3v1EncodingError) Error() string {
	if e.Message != "" {
		return "ID3v1: " + e.Message
	}
	return "failed to write ID3v1 tag"
}
func (e *Id3v1EncodingError) Unwrap() error { return e.Source }

// Id3v1TagParse parses an ID3v1 tag from a 128-byte slice.
func Id3v1TagParse(reader [128]byte, parseMode ParsingMode) (*Id3v1Tag, error) {
	var tag Id3v1Tag
	if !bytes.Equal(reader[:3], Id3v1TagMarker) {
		return nil, &Id3v1ParseError{Source: &FakeTagError{}}
	}
	r := reader[3:]

	tag.Title = id3v1DecodeText(r[:30])
	tag.Artist = id3v1DecodeText(r[30:60])
	tag.Album = id3v1DecodeText(r[60:90])

	year, err := id3v1TryParseYear(r[90:94], parseMode)
	if err != nil {
		return nil, err
	}
	tag.Year = year

	var commentRange []byte
	if r[122] == 0 && r[123] != 0 {
		tag.TrackNumber = Some(r[123])
		commentRange = r[94:123]
	} else {
		commentRange = r[94:124]
	}
	tag.Comment = id3v1DecodeText(commentRange)

	if r[124] < uint8(len(Id3v1Genres)) {
		tag.Genre = Some(r[124])
	}
	return &tag, nil
}

func id3v1DecodeText(data []byte) Option[string] {
	firstNullPos := len(data)
	if i := bytes.IndexByte(data, 0); i >= 0 {
		if i == 0 {
			return None[string]()
		}
		firstNullPos = i
	}
	return Some(latin1Decode(data[:firstNullPos]))
}

func id3v1TryParseYear(input []byte, parseMode ParsingMode) (Option[uint16], error) {
	numDigits, year := asciiNumber(input)
	if numDigits != 4 {
		if parseMode == ParsingModeStrict {
			return None[uint16](), &Id3v1ParseError{Message: "year contains non-digit characters"}
		}
		return None[uint16](), nil
	}
	return Some(uint16(year)), nil
}

// Tag converts the [Id3v1Tag] to a generic [Tag].
func (t *Id3v1Tag) Tag() *Tag {
	tag := NewTag(TagTypeID3v1)
	if s, ok := t.Title.Get(); ok {
		tag.InsertText(ItemKeyTrackTitle, s)
	}
	if s, ok := t.Artist.Get(); ok {
		tag.InsertText(ItemKeyTrackArtist, s)
	}
	if s, ok := t.Album.Get(); ok {
		tag.InsertText(ItemKeyAlbumTitle, s)
	}
	if y, ok := t.Year.Get(); ok {
		tag.InsertText(ItemKeyYear, itoa(int(y)))
	}
	if s, ok := t.Comment.Get(); ok {
		tag.InsertText(ItemKeyComment, s)
	}
	if n, ok := t.TrackNumber.Get(); ok {
		tag.PushUnchecked(NewTagItem(ItemKeyTrackNumber, ItemValueText(itoa(int(n)))))
	}
	if g, ok := t.Genre.Get(); ok && int(g) < len(Id3v1Genres) {
		tag.InsertText(ItemKeyGenre, Id3v1Genres[g])
	}
	return tag
}

// Date returns the year as a [Timestamp].
func (t *Id3v1Tag) Date() Option[Timestamp] {
	if y, ok := t.Year.Get(); ok {
		return Some(Timestamp{Year: y})
	}
	return None[Timestamp]()
}

// SetDate sets the year from a [Timestamp].
func (t *Id3v1Tag) SetDate(value Timestamp) { t.Year = Some(value.Year) }

// RemoveDate clears the year.
func (t *Id3v1Tag) RemoveDate() { t.Year = None[uint16]() }

// DumpTo writes the tag to a writer.
func (t *Id3v1Tag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data, err := id3v1Encode(t, writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// Id3v1TagFromTag converts a generic [Tag] to an [Id3v1Tag].
func Id3v1TagFromTag(input *Tag) *Id3v1Tag {
	input = input.Clone()
	take := func(k ItemKey) Option[string] {
		if s := input.TakeStrings(k); len(s) > 0 {
			return Some(s[0])
		}
		return None[string]()
	}
	tag := &Id3v1Tag{
		Title:   take(ItemKeyTrackTitle),
		Artist:  take(ItemKeyTrackArtist),
		Album:   take(ItemKeyAlbumTitle),
		Comment: take(ItemKeyComment),
	}
	if s, ok := input.GetString(ItemKeyYear); ok {
		if n, err := atoi(s); err == nil {
			tag.Year = Some(uint16(n))
		}
	}
	if tag.Year.IsNone() {
		if d, ok := input.Date().Get(); ok {
			tag.Year = Some(d.Year)
		}
	}
	if s, ok := input.GetString(ItemKeyTrackNumber); ok {
		if n, err := atoi(s); err == nil && n <= 255 {
			tag.TrackNumber = Some(uint8(n))
		}
	}
	if s, ok := input.GetString(ItemKeyGenre); ok {
		for i, g := range Id3v1Genres {
			if g == s {
				tag.Genre = Some(uint8(i))
				break
			}
		}
		if tag.Genre.IsNone() {
			if n, err := atoi(s); err == nil && n <= 255 && n < len(Id3v1Genres) {
				tag.Genre = Some(uint8(n))
			}
		}
	}
	return tag
}

func (t *Id3v1Tag) isEmpty() bool {
	return t.Title.IsNone() && t.Artist.IsNone() && t.Album.IsNone() &&
		t.Year.IsNone() && t.Comment.IsNone() && t.TrackNumber.IsNone() && t.Genre.IsNone()
}

// id3v1Encode encodes an ID3v1 tag to 128 bytes.
func id3v1Encode(tag *Id3v1Tag, writeOptions WriteOptions) ([]byte, error) {
	resizeString := func(value Option[string], size int) ([]byte, error) {
		field := make([]byte, 0, size)
		if val, ok := value.Get(); ok {
			if len([]rune(val)) > size && !writeOptions.ImplicitTruncation {
				return nil, &Id3v1EncodingError{Source: &TooMuchDataError{}}
			}
			encoded, err := latin1Encode(val, writeOptions.LossyTextEncoding)
			if err != nil {
				return nil, &Id3v1EncodingError{Source: err}
			}
			if len(encoded) > size {
				encoded = encoded[:size]
			}
			field = append(field, encoded...)
		}
		for len(field) < size {
			field = append(field, 0)
		}
		return field, nil
	}

	var writer bytes.Buffer
	writer.Write(Id3v1TagMarker)

	for _, f := range []struct {
		v    Option[string]
		size int
	}{
		{tag.Title, 30},
		{tag.Artist, 30},
		{tag.Album, 30},
	} {
		b, err := resizeString(f.v, f.size)
		if err != nil {
			return nil, err
		}
		writer.Write(b)
	}

	var year [4]byte
	if y, ok := tag.Year.Get(); ok {
		n := y
		if n > 9999 {
			n = 9999
		}
		for i := 3; i >= 0; i-- {
			year[i] = '0' + byte(n%10)
			n /= 10
		}
	}
	writer.Write(year[:])

	comment, err := resizeString(tag.Comment, 28)
	if err != nil {
		return nil, err
	}
	writer.Write(comment)
	writer.WriteByte(0)
	writer.WriteByte(tag.TrackNumber.OrZero())
	genre := byte(255)
	if g, ok := tag.Genre.Get(); ok {
		genre = g
	}
	writer.WriteByte(genre)
	return writer.Bytes(), nil
}

// FindId3v1 attempts to find an ID3v1 tag in the stream.
// If found, the reader is positioned at the start of the tag.
// Returns (tagOrNil, rangeStart, rangeEnd, found, err).
func FindId3v1(reader io.ReadSeeker, read bool, parseMode ParsingMode) (*Id3v1Tag, int64, int64, bool, error) {
	if _, err := reader.Seek(-128, io.SeekEnd); err != nil {
		if _, err2 := reader.Seek(0, io.SeekEnd); err2 != nil {
			return nil, 0, 0, false, err2
		}
		return nil, 0, 0, false, nil
	}
	var header [3]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, 0, 0, false, err
	}
	start, err := reader.Seek(-3, io.SeekCurrent)
	if err != nil {
		return nil, 0, 0, false, err
	}
	if !bytes.Equal(header[:], Id3v1TagMarker) {
		if _, err := reader.Seek(0, io.SeekEnd); err != nil {
			return nil, 0, 0, false, err
		}
		return nil, 0, 0, false, nil
	}
	var tag *Id3v1Tag
	if read {
		var buf [128]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return nil, 0, 0, false, err
		}
		t, err := Id3v1TagParse(buf, parseMode)
		if err != nil {
			return nil, 0, 0, false, err
		}
		tag = t
		if _, err := reader.Seek(-128, io.SeekEnd); err != nil {
			return nil, 0, 0, false, err
		}
	}
	return tag, start, start + 128, true, nil
}

func init() {
	meta.RegisterTagDumper(meta.TagTypeID3v1, func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		t := Id3v1TagFromTag(tag)
		if t.isEmpty() {
			return nil
		}
		b, err := id3v1Encode(t, writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(b)
		return err
	})
}

// WriteId3v1 writes an ID3v1 tag into a file.
func WriteId3v1(file FileLike, tag *Id3v1Tag, writeOptions WriteOptions) error {
	var existingRange [2]int64
	_, start, end, found, err := FindId3v1(file, false, writeOptions.ParseOptions.ParsingMode)
	if err != nil {
		return &FileEncodingError{Source: &TagParseError{TagType: TagTypeID3v1, HasTagType: true, Source: err}}
	}
	if found {
		existingRange = [2]int64{start, end}
	}

	if tag.isEmpty() {
		if !found {
			return nil
		}
		// Strip Lyrics3 too — Lyrics3 is an extension of ID3v1.
		lyrStart, _, _, lyrFound, err := findLyrics3(file, false, writeOptions.ParseOptions)
		if err == nil && lyrFound {
			existingRange[0] = lyrStart
		}
		return Splice(file, uint64(existingRange[0]), uint64(existingRange[1]), nil)
	}

	encoded, err := id3v1Encode(tag, writeOptions)
	if err != nil {
		return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeID3v1, HasTagType: true, Source: err}}
	}
	_, err = file.Write(encoded)
	return err
}

// --- ID3v1 genres ---

var Id3v1Genres = []string{
	"Blues", "Classic rock", "Country", "Dance", "Disco", "Funk", "Grunge", "Hip-Hop",
	"Jazz", "Metal", "New Age", "Oldies", "Other", "Pop", "R&B", "Rap", "Reggae", "Rock",
	"Techno", "Industrial", "Alternative", "Ska", "Death metal", "Pranks", "Soundtrack",
	"Euro-Techno", "Ambient", "Trip-Hop", "Vocal", "Jazz & Funk", "Fusion", "Trance",
	"Classical", "Instrumental", "Acid", "House", "Game", "Sound clip", "Gospel", "Noise",
	"Alternative Rock", "Bass", "Soul", "Punk", "Space", "Meditative", "Instrumental Pop",
	"Instrumental Rock", "Ethnic", "Gothic", "Darkwave", "Techno-Industrial", "Electronic",
	"Pop-Folk", "Eurodance", "Dream", "Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40",
	"Christian Rap", "Pop/Funk", "Jungle", "Native US", "Cabaret", "New Wave", "Psychedelic",
	"Rave", "Show tunes", "Trailer", "Lo-Fi", "Tribal", "Acid Punk", "Acid Jazz", "Polka",
	"Retro", "Musical", "Rock ’n’ Roll", "Hard Rock", "Folk", "Folk-Rock", "National Folk",
	"Swing", "Fast Fusion", "Bebop", "Latin", "Revival", "Celtic", "Bluegrass", "Avantgarde",
	"Gothic Rock", "Progressive Rock", "Psychedelic Rock", "Symphonic Rock", "Slow rock",
	"Big Band", "Chorus", "Easy Listening", "Acoustic", "Humour", "Speech", "Chanson",
	"Opera", "Chamber music", "Sonata", "Symphony", "Booty bass", "Primus", "Porn groove",
	"Satire", "Slow jam", "Club", "Tango", "Samba", "Folklore", "Ballad", "Power Ballad",
	"Rhythmic Soul", "Freestyle", "Duet", "Punk Rock", "Drum Solo", "A cappella",
	"Euro-House", "Dance Hall", "Goa music", "Drum & Bass", "Club-House", "Hardcore Techno",
	"Terror", "Indie", "BritPop", "Negerpunk", "Polsk Punk", "Beat", "Christian Gangsta Rap",
	"Heavy Metal", "Black Metal", "Crossover", "Contemporary Christian", "Christian rock",
	"Merengue", "Salsa", "Thrash Metal", "Anime", "Jpop", "Synthpop", "Abstract", "Art Rock",
	"Baroque", "Bhangra", "Big beat", "Breakbeat", "Chillout", "Downtempo", "Dub", "EBM",
	"Eclectic", "Electro", "Electroclash", "Emo", "Experimental", "Garage", "Global", "IDM",
	"Illbient", "Industro-Goth", "Jam Band", "Krautrock", "Leftfield", "Lounge", "Math Rock",
	"New Romantic", "Nu-Breakz", "Post-Punk", "Post-Rock", "Psytrance", "Shoegaze",
	"Space Rock", "Trop Rock", "World Music", "Neoclassical", "Audiobook", "Audio theatre",
	"Neue Deutsche Welle", "Podcast", "Indie-Rock", "G-Funk", "Dubstep", "Garage Rock",
	"Psybient",
}

// SaveTo writes the tag to a [FileLike], mirroring Rust `TagExt::save_to`.
func (t *Id3v1Tag) SaveTo(file FileLike, writeOptions WriteOptions) error {
	vf, err := NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(TagTypeID3v1)
	if support == TagSupportUnsupported || (!support.IsWritable() && !t.isEmpty()) {
		return &FileEncodingError{FileType: format, Source: &UnsupportedTagError{}}
	}
	return WriteId3v1(file, t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *Id3v1Tag) SaveToPath(path string, writeOptions WriteOptions) error {
	vf, f, err := VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(TagTypeID3v1)
	if support == TagSupportUnsupported || (!support.IsWritable() && !t.isEmpty()) {
		return &FileEncodingError{FileType: format, Source: &UnsupportedTagError{}}
	}
	return WriteId3v1(vf.File(), t, writeOptions)
}

// RemoveFrom removes an ID3v1 tag from a [FileLike].
func (t *Id3v1Tag) RemoveFrom(file FileLike, writeOptions WriteOptions) error {
	return TagTypeID3v1.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes an ID3v1 tag from a path.
func (t *Id3v1Tag) RemoveFromPath(path string, writeOptions WriteOptions) error {
	return TagTypeID3v1.RemoveFromPath(path, writeOptions)
}
