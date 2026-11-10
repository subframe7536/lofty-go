package id3v1

import (
	"bytes"
	"io"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
)

var Id3v1TagMarker = []byte("TAG")

// Id3v1Tag is an ID3v1 tag.
type Id3v1Tag struct {
	Title       meta.Option[string]
	Artist      meta.Option[string]
	Album       meta.Option[string]
	Year        meta.Option[uint16]
	Comment     meta.Option[string]
	TrackNumber meta.Option[uint8]
	Genre       meta.Option[uint8]
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
func Id3v1TagParse(reader [128]byte, parseMode meta.ParsingMode) (*Id3v1Tag, error) {
	var tag Id3v1Tag
	if !bytes.Equal(reader[:3], Id3v1TagMarker) {
		return nil, &Id3v1ParseError{Source: &meta.FakeTagError{}}
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
		tag.TrackNumber = meta.Some(r[123])
		commentRange = r[94:123]
	} else {
		commentRange = r[94:124]
	}
	tag.Comment = id3v1DecodeText(commentRange)

	if r[124] < uint8(len(Id3v1Genres)) {
		tag.Genre = meta.Some(r[124])
	}
	return &tag, nil
}

func id3v1DecodeText(data []byte) meta.Option[string] {
	firstNullPos := len(data)
	if i := bytes.IndexByte(data, 0); i >= 0 {
		if i == 0 {
			return meta.None[string]()
		}
		firstNullPos = i
	}
	return meta.Some(util.Latin1Decode(data[:firstNullPos]))
}

func id3v1TryParseYear(input []byte, parseMode meta.ParsingMode) (meta.Option[uint16], error) {
	numDigits, year := util.AsciiNumber(input)
	if numDigits != 4 {
		if parseMode == meta.ParsingModeStrict {
			return meta.None[uint16](), &Id3v1ParseError{Message: "year contains non-digit characters"}
		}
		return meta.None[uint16](), nil
	}
	return meta.Some(uint16(year)), nil
}

// Tag converts the [Id3v1Tag] to a generic [Tag].
func (t *Id3v1Tag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeID3v1)
	if s, ok := t.Title.Get(); ok {
		tag.InsertText(meta.ItemKeyTrackTitle, s)
	}
	if s, ok := t.Artist.Get(); ok {
		tag.InsertText(meta.ItemKeyTrackArtist, s)
	}
	if s, ok := t.Album.Get(); ok {
		tag.InsertText(meta.ItemKeyAlbumTitle, s)
	}
	if y, ok := t.Year.Get(); ok {
		tag.InsertText(meta.ItemKeyYear, util.Itoa(int(y)))
	}
	if s, ok := t.Comment.Get(); ok {
		tag.InsertText(meta.ItemKeyComment, s)
	}
	if n, ok := t.TrackNumber.Get(); ok {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyTrackNumber, meta.ItemValueText(util.Itoa(int(n)))))
	}
	if g, ok := t.Genre.Get(); ok && int(g) < len(Id3v1Genres) {
		tag.InsertText(meta.ItemKeyGenre, Id3v1Genres[g])
	}
	return tag
}

// Date returns the year as a [Timestamp].
func (t *Id3v1Tag) Date() meta.Option[meta.Timestamp] {
	if y, ok := t.Year.Get(); ok {
		return meta.Some(meta.Timestamp{Year: y})
	}
	return meta.None[meta.Timestamp]()
}

// SetDate sets the year from a [Timestamp].
func (t *Id3v1Tag) SetDate(value meta.Timestamp) { t.Year = meta.Some(value.Year) }

// RemoveDate clears the year.
func (t *Id3v1Tag) RemoveDate() { t.Year = meta.None[uint16]() }

// DumpTo writes the tag to a writer.
func (t *Id3v1Tag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := id3v1Encode(t, writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// Id3v1TagFromTag converts a generic [Tag] to an [Id3v1Tag].
func Id3v1TagFromTag(input *meta.Tag) *Id3v1Tag {
	input = input.Clone()
	take := func(k meta.ItemKey) meta.Option[string] {
		if s := input.TakeStrings(k); len(s) > 0 {
			return meta.Some(s[0])
		}
		return meta.None[string]()
	}
	tag := &Id3v1Tag{
		Title:   take(meta.ItemKeyTrackTitle),
		Artist:  take(meta.ItemKeyTrackArtist),
		Album:   take(meta.ItemKeyAlbumTitle),
		Comment: take(meta.ItemKeyComment),
	}
	if s, ok := input.GetString(meta.ItemKeyYear); ok {
		if n, err := util.Atoi(s); err == nil {
			tag.Year = meta.Some(uint16(n))
		}
	}
	if tag.Year.IsNone() {
		if d, ok := input.Date().Get(); ok {
			tag.Year = meta.Some(d.Year)
		}
	}
	if s, ok := input.GetString(meta.ItemKeyTrackNumber); ok {
		if n, err := util.Atoi(s); err == nil && n <= 255 {
			tag.TrackNumber = meta.Some(uint8(n))
		}
	}
	if g, ok := input.GetString(meta.ItemKeyGenre); ok {
		for i, genre := range Id3v1Genres {
			if genre == g {
				tag.Genre = meta.Some(uint8(i))
				break
			}
		}
	}
	return tag
}

func (t *Id3v1Tag) isEmpty() bool {
	return t.Title.IsNone() &&
		t.Artist.IsNone() &&
		t.Album.IsNone() &&
		t.Year.IsNone() &&
		t.Comment.IsNone() &&
		t.TrackNumber.IsNone() &&
		t.Genre.IsNone()
}

func id3v1Encode(tag *Id3v1Tag, writeOptions meta.WriteOptions) ([]byte, error) {
	encodeField := func(field meta.Option[string], len_ int, fieldName string) ([]byte, error) {
		if s, ok := field.Get(); ok {
			if len([]rune(s)) > len_ && !writeOptions.ImplicitTruncation {
				return nil, &Id3v1EncodingError{Message: fieldName + " is too long"}
			}
			enc, err := util.Latin1Encode(s, writeOptions.LossyTextEncoding)
			if err != nil {
				return nil, &Id3v1EncodingError{Source: err}
			}
			if len(enc) > len_ {
				enc = enc[:len_]
			}
			buf := make([]byte, len_)
			copy(buf, enc)
			return buf, nil
		}
		return make([]byte, len_), nil
	}

	var writer bytes.Buffer
	writer.Write(Id3v1TagMarker)

	title, err := encodeField(tag.Title, 30, "Title")
	if err != nil {
		return nil, err
	}
	writer.Write(title)

	artist, err := encodeField(tag.Artist, 30, "Artist")
	if err != nil {
		return nil, err
	}
	writer.Write(artist)

	album, err := encodeField(tag.Album, 30, "Album")
	if err != nil {
		return nil, err
	}
	writer.Write(album)

	if y, ok := tag.Year.Get(); ok {
		var buf [4]byte
		copy(buf[:], util.Itoa(int(y)))
		writer.Write(buf[:])
	} else {
		writer.Write(make([]byte, 4))
	}

	comment, err := encodeField(tag.Comment, 28, "Comment")
	if err != nil {
		return nil, err
	}
	writer.Write(comment)

	var track [2]byte
	if n, ok := tag.TrackNumber.Get(); ok {
		track[1] = n
	}
	writer.Write(track[:])

	if g, ok := tag.Genre.Get(); ok {
		writer.WriteByte(g)
	} else {
		writer.WriteByte(255)
	}

	return writer.Bytes(), nil
}

// FindId3v1 searches for an ID3v1 tag at the end of the reader.
func FindId3v1(reader io.ReadSeeker, read bool, parseMode meta.ParsingMode) (*Id3v1Tag, int64, int64, bool, error) {
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
	meta.RegisterTagDumper(meta.TagTypeID3v1, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
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
func WriteId3v1(file meta.FileLike, tag *Id3v1Tag, writeOptions meta.WriteOptions) error {
	var existingRange [2]int64
	_, start, end, found, err := FindId3v1(file, false, writeOptions.ParseOptions.ParsingMode)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v1, HasTagType: true, Source: err}}
	}
	if found {
		existingRange = [2]int64{start, end}
	}

	if tag.isEmpty() {
		if !found {
			return nil
		}
		// Strip Lyrics3 too — Lyrics3 is an extension of ID3v1.
		lyrStart, _, _, lyrFound, err := lyrics3.FindLyrics3(file, false, writeOptions.ParseOptions)
		if err == nil && lyrFound {
			existingRange[0] = lyrStart
		}
		return meta.Splice(file, uint64(existingRange[0]), uint64(existingRange[1]), nil)
	}

	encoded, err := id3v1Encode(tag, writeOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeID3v1, HasTagType: true, Source: err}}
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
func (t *Id3v1Tag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeID3v1)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.isEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return WriteId3v1(file, t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *Id3v1Tag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeID3v1)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.isEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return WriteId3v1(vf.File(), t, writeOptions)
}

// RemoveFrom removes an ID3v1 tag from a [FileLike].
func (t *Id3v1Tag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeID3v1.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes an ID3v1 tag from a path.
func (t *Id3v1Tag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeID3v1.RemoveFromPath(path, writeOptions)
}
