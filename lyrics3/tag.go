package lyrics3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/tagpos"
	"github.com/subframe7536/lofty-go/internal/util"
)

var errNegativeSeek = errors.New("invalid seek to a negative or overflowing position")

var (
	lyrics3v1Footer = []byte("LYRICSEND")
	lyrics3v2Footer = []byte("LYRICS200")
	lyrics3v2Header = []byte("LYRICSBEGIN")
)

const (
	lyrics3v1MaxLyricsLen = 5100
	lyrics3v2TagMaxSize   = 999999
	lyrics3v2FieldMaxSize = 99999
	lyrics3v2BasicMaxSize = 250
	lyrics3v2PathMax      = 4096
)

// Lyrics3ParseError is returned when a Lyrics3 tag fails to parse.
type Lyrics3ParseError struct {
	Message string
	Source  error
}

func (e *Lyrics3ParseError) Error() string {
	if e.Message != "" {
		return "Lyrics3: " + e.Message
	}
	return "failed to parse Lyrics3 tag"
}
func (e *Lyrics3ParseError) Unwrap() error { return e.Source }

// Lyrics3v2ParseError is returned when a Lyrics3v2 tag fails to parse.
type Lyrics3v2ParseError struct {
	Message string
	Source  error
}

func (e *Lyrics3v2ParseError) Error() string {
	if e.Message != "" {
		return "Lyrics3v2: " + e.Message
	}
	return "failed to parse Lyrics3v2 tag"
}
func (e *Lyrics3v2ParseError) Unwrap() error { return e.Source }

// Lyrics3Tag is a Lyrics3 (v1) tag.
type Lyrics3Tag struct {
	Lyrics string
}

// ParseLyrics3Tag parses a complete Lyrics3v1 tag (`LYRICSBEGIN`..`LYRICSEND`).
func ParseLyrics3Tag(content []byte) (*Lyrics3Tag, error) {
	if len(content) < len(lyrics3v2Header)+len(lyrics3v1Footer) ||
		!bytes.Equal(content[:len(lyrics3v2Header)], lyrics3v2Header) ||
		!bytes.HasSuffix(content, lyrics3v1Footer) {
		return nil, &Lyrics3ParseError{Source: &meta.FakeTagError{}}
	}
	return &Lyrics3Tag{
		Lyrics: util.Latin1Decode(content[len(lyrics3v2Header) : len(content)-len(lyrics3v1Footer)]),
	}, nil
}

// Tag converts the tag to a generic [Tag].
func (t *Lyrics3Tag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeLyrics3)
	if t.Lyrics != "" {
		tag.PushUnchecked(meta.NewTagItem(meta.ItemKeyLyrics, meta.ItemValueText(t.Lyrics)))
	}
	return tag
}

// Lyrics3TagFromTag converts a generic [Tag] to a [Lyrics3Tag].
func Lyrics3TagFromTag(tag *meta.Tag) *Lyrics3Tag {
	out := &Lyrics3Tag{}
	if s, ok := tag.GetString(meta.ItemKeyLyrics); ok {
		out.Lyrics = s
	}
	return out
}

// Len returns the number of fields present.
func (t *Lyrics3Tag) Len() int {
	if t.Lyrics == "" {
		return 0
	}
	return 1
}

// IsEmpty reports whether the tag has no content.
func (t *Lyrics3Tag) IsEmpty() bool { return t.Lyrics == "" }

// DumpTo writes the encoded tag to writer.
func (t *Lyrics3Tag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := t.encode(writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func (t *Lyrics3Tag) encode(writeOptions meta.WriteOptions) ([]byte, error) {
	lyrics := t.Lyrics
	if lyrics == "" {
		return nil, nil // Stripping
	}
	if strings.Contains(lyrics, "LYRICSBEGIN") || strings.Contains(lyrics, "LYRICSEND") {
		return nil, &meta.TagEncodingError{
			TagType: meta.TagTypeLyrics3, HasTagType: true,
			Source: &Lyrics3ParseError{Message: "lyrics text contains tag markers"},
		}
	}
	enc, err := writeLatin1Field(lyrics, lyrics3v1MaxLyricsLen, writeOptions,
		&Lyrics3ParseError{Message: "lyrics text contains invalid characters"})
	if err != nil {
		return nil, &meta.TagEncodingError{TagType: meta.TagTypeLyrics3, HasTagType: true, Source: err}
	}
	out := make([]byte, 0, len(lyrics3v2Header)+len(enc)+len(lyrics3v1Footer))
	out = append(out, lyrics3v2Header...)
	out = append(out, enc...)
	out = append(out, lyrics3v1Footer...)
	return out, nil
}

// writeLatin1Field encodes a string to Latin-1, truncating to maxChars. When
// the content exceeds maxChars, meta.TooMuchDataError is returned unless
// meta.WriteOptions.ImplicitTruncation is set.
//
// Like Rust's `write_latin1_string`, encoding is lazy over the first maxChars
// characters, so characters past the cut are never checked. A character that
// is not Latin-1 fails with a [util.TextEncodingError]; the byte 0xFF (which
// Lyrics3 cannot carry) fails with encErr unless lossy encoding is enabled.
func writeLatin1Field(s string, maxChars int, writeOptions meta.WriteOptions, encErr error) ([]byte, error) {
	if utf8.RuneCountInString(s) > maxChars {
		if !writeOptions.ImplicitTruncation {
			return nil, &meta.TooMuchDataError{}
		}
		s = truncateRunes(s, maxChars)
	}
	lossy := writeOptions.LossyTextEncoding
	out := make([]byte, 0, len(s))
	for i, c := range []rune(s) {
		var b byte
		switch {
		case c <= 0xFF:
			b = byte(c)
		case lossy:
			b = '?'
		default:
			return nil, &util.TextEncodingError{Encoding: util.TextEncodingLatin1, ValidUpTo: i}
		}
		if b == 0xFF {
			if !lossy {
				return nil, encErr
			}
			b = '?'
		}
		out = append(out, b)
	}
	return out, nil
}

// truncateRunes returns the first n characters of s.
func truncateRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// Lyrics3v2TagFlags are the IND field flags of a [Lyrics3v2Tag].
type Lyrics3v2TagFlags struct {
	// Whether the `Lyrics` field contains any timestamps.
	SynchronizedLyrics bool
	// Whether the track should be inhibited from random selection.
	RandomSelection bool
}

// Lyrics3v2Image is an image file link in a [Lyrics3v2Tag].
type Lyrics3v2Image struct {
	Filename    string
	Description meta.Option[string]
	Timestamp   *meta.Timestamp
}

// Lyrics3v2Tag is a Lyrics3 v2 tag.
type Lyrics3v2Tag struct {
	Flags              Lyrics3v2TagFlags
	Lyrics             meta.Option[string]
	Info               meta.Option[string]
	Author             meta.Option[string]
	ExtendedAlbumName  meta.Option[string]
	ExtendedArtistName meta.Option[string]
	ExtendedTrackTitle meta.Option[string]
	Images             []Lyrics3v2Image
}

// ParseLyrics3v2Tag parses a complete Lyrics3v2 tag
// (`LYRICSBEGIN`..`<size>`..`LYRICS200`).
func ParseLyrics3v2Tag(content []byte, parseOptions meta.ParseOptions) (*Lyrics3v2Tag, error) {
	if len(content) < len(lyrics3v2Header)+len(lyrics3v2Footer) ||
		!bytes.Equal(content[:len(lyrics3v2Header)], lyrics3v2Header) ||
		!bytes.HasSuffix(content, lyrics3v2Footer) {
		return nil, &Lyrics3v2ParseError{Source: &meta.FakeTagError{}}
	}

	body := content[:len(content)-len(lyrics3v2Footer)]
	if len(body) < 6 {
		return nil, &Lyrics3v2ParseError{Source: &meta.FakeTagError{}}
	}

	sizeBytes, tagSize := util.AsciiNumber(body[len(body)-6:])
	if sizeBytes != 6 {
		return nil, &Lyrics3v2ParseError{Message: "non-numeric tag size"}
	}
	body = body[:len(body)-6]
	if int(tagSize) != len(body) {
		if parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, &Lyrics3v2ParseError{Source: &meta.SizeMismatchError{}}
		}
	}

	tag := &Lyrics3v2Tag{}
	fields := body[len(lyrics3v2Header):]
	mode := parseOptions.ParsingMode

	parseText := func(size, max int) (string, error) {
		if size > max && mode == meta.ParsingModeStrict {
			return "", &Lyrics3v2ParseError{Source: &meta.TooMuchDataError{}}
		}
		if len(fields) < size {
			return "", &Lyrics3v2ParseError{Source: &meta.SizeMismatchError{}}
		}
		raw := fields[:size]
		fields = fields[size:]
		if len(raw) > max {
			raw = raw[:max]
		}
		return util.Latin1Decode(raw), nil
	}

	for len(fields) > 0 {
		if len(fields) < 8 {
			return nil, &Lyrics3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		id := string(fields[:3])
		n, size64 := util.AsciiNumber(fields[3:8])
		if n != 5 {
			return nil, &Lyrics3v2ParseError{Message: "non-numeric field size"}
		}
		fields = fields[8:]
		size := int(size64)

		switch id {
		case "IND":
			ind, err := parseText(size, 3)
			if err != nil {
				return nil, err
			}
			if len(ind) != 3 {
				if mode == meta.ParsingModeStrict {
					return nil, &Lyrics3v2ParseError{Message: "invalid indication value"}
				}
				continue
			}
			indication := func(c byte) (bool, error) {
				switch c {
				case '1':
					return true, nil
				case '0':
					return false, nil
				}
				if mode == meta.ParsingModeStrict {
					return false, &Lyrics3v2ParseError{Message: "invalid indication value"}
				}
				return true, nil
			}
			if tag.Flags.SynchronizedLyrics, err = indication(ind[1]); err != nil {
				return nil, err
			}
			if tag.Flags.RandomSelection, err = indication(ind[2]); err != nil {
				return nil, err
			}
		case "LYR":
			v, err := parseText(size, lyrics3v2FieldMaxSize)
			if err != nil {
				return nil, err
			}
			tag.Lyrics = meta.Some(v)
		case "INF":
			v, err := parseText(size, lyrics3v2FieldMaxSize)
			if err != nil {
				return nil, err
			}
			tag.Info = meta.Some(v)
		case "AUT":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.Author = meta.Some(v)
		case "EAL":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedAlbumName = meta.Some(v)
		case "EAR":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedArtistName = meta.Some(v)
		case "ETT":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedTrackTitle = meta.Some(v)
		case "IMG":
			imgStr, err := parseText(size, lyrics3v2FieldMaxSize)
			if err != nil {
				return nil, err
			}
			for _, line := range rustLines(imgStr) {
				parts := strings.Split(line, "||")
				var img Lyrics3v2Image
				img.Filename = parts[0]
				if len(parts) > 1 && parts[1] != "" {
					img.Description = meta.Some(parts[1])
				} else if len(parts) <= 1 && mode == meta.ParsingModeStrict {
					return nil, &Lyrics3v2ParseError{Message: "image field missing a description"}
				}
				if len(parts) > 2 && parts[2] != "" {
					ts, err := lyrics3v2ParseTimestamp(parts[2])
					if err != nil {
						if mode == meta.ParsingModeStrict {
							return nil, err
						}
					} else {
						img.Timestamp = ts
					}
				} else if len(parts) <= 2 && mode == meta.ParsingModeStrict {
					return nil, &Lyrics3v2ParseError{Message: "image field missing a timestamp"}
				}
				tag.Images = append(tag.Images, img)
			}
		default:
			// Unknown field — skip
			if len(fields) < size {
				return nil, &Lyrics3v2ParseError{Source: &meta.SizeMismatchError{}}
			}
			fields = fields[size:]
		}
	}
	return tag, nil
}

// rustLines splits like Rust's `str::lines`: on "\n", dropping a "\r" that
// directly precedes it, with no trailing empty line. Blank lines in the middle
// are kept.
func rustLines(s string) []string {
	var lines []string
	for s != "" {
		line, rest, found := strings.Cut(s, "\n")
		if found {
			line = strings.TrimSuffix(line, "\r")
		}
		lines = append(lines, line)
		s = rest
	}
	return lines
}

// lyrics3v2ParseTimestamp parses a `[mm:ss]` timestamp.
func lyrics3v2ParseTimestamp(s string) (*meta.Timestamp, error) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	mins, secs, found := strings.Cut(s[1:len(s)-1], ":")
	if !found {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	mb, m := util.AsciiNumber([]byte(mins))
	if mb != 2 || m > 59 {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	sb, sec := util.AsciiNumber([]byte(secs))
	if sb != 2 || sec > 59 {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	return &meta.Timestamp{Minute: meta.Some(uint8(m)), Second: meta.Some(uint8(sec))}, nil
}

// Tag converts the tag to a generic [Tag]. Flags and images ride the
// companion tag when [GlobalOptions.PreserveFormatSpecificItems] is enabled.
func (t *Lyrics3v2Tag) Tag() *meta.Tag {
	tag := meta.NewTag(meta.TagTypeLyrics3v2)
	take := func(opt meta.Option[string], key meta.ItemKey) {
		if v, ok := opt.Get(); ok && v != "" {
			tag.PushUnchecked(meta.NewTagItem(key, meta.ItemValueText(v)))
		}
	}
	take(t.Lyrics, meta.ItemKeyLyrics)
	take(t.Info, meta.ItemKeyComment)
	take(t.Author, meta.ItemKeyLyricist)
	take(t.ExtendedAlbumName, meta.ItemKeyAlbumTitle)
	take(t.ExtendedArtistName, meta.ItemKeyTrackArtist)
	take(t.ExtendedTrackTitle, meta.ItemKeyTrackTitle)

	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		remainder := *t
		remainder.Lyrics, remainder.Info, remainder.Author = meta.Option[string]{}, meta.Option[string]{}, meta.Option[string]{}
		remainder.ExtendedAlbumName, remainder.ExtendedArtistName, remainder.ExtendedTrackTitle = meta.Option[string]{}, meta.Option[string]{}, meta.Option[string]{}
		if !remainder.IsEmpty() {
			tag.SetCompanionTag(&remainder)
		}
	}
	return tag
}

// Lyrics3v2TagFromTag merges a generic [Tag] into a [Lyrics3v2Tag], taking
// format-specific data back out of the companion tag when present.
func Lyrics3v2TagFromTag(input *meta.Tag) *Lyrics3v2Tag {
	var merged Lyrics3v2Tag
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*Lyrics3v2Tag); ok && comp != nil {
			merged = *comp
			if len(comp.Images) > 0 {
				merged.Images = append([]Lyrics3v2Image(nil), comp.Images...)
			}
		}
	}
	get := func(key meta.ItemKey, dst *meta.Option[string]) {
		if s, ok := input.GetString(key); ok {
			*dst = meta.Some(s)
		}
	}
	get(meta.ItemKeyLyrics, &merged.Lyrics)
	get(meta.ItemKeyComment, &merged.Info)
	get(meta.ItemKeyLyricist, &merged.Author)
	get(meta.ItemKeyAlbumTitle, &merged.ExtendedAlbumName)
	get(meta.ItemKeyTrackArtist, &merged.ExtendedArtistName)
	get(meta.ItemKeyTrackTitle, &merged.ExtendedTrackTitle)
	return &merged
}

func (t *Lyrics3v2Tag) isContentEmpty() bool {
	_, hasLyrics := t.Lyrics.Get()
	_, hasInfo := t.Info.Get()
	_, hasAuthor := t.Author.Get()
	_, hasEAL := t.ExtendedAlbumName.Get()
	_, hasEAR := t.ExtendedArtistName.Get()
	_, hasETT := t.ExtendedTrackTitle.Get()
	return !hasLyrics && !hasInfo && !hasAuthor && !hasEAL && !hasEAR && !hasETT && len(t.Images) == 0
}

// IsEmpty reports whether the tag has no fields or images. Like Rust's
// `is_empty`, the flags are not counted.
func (t *Lyrics3v2Tag) IsEmpty() bool { return t.isContentEmpty() }

// Len returns the number of fields and images present.
func (t *Lyrics3v2Tag) Len() int {
	n := len(t.Images)
	for _, o := range []meta.Option[string]{t.Lyrics, t.Info, t.Author, t.ExtendedAlbumName, t.ExtendedArtistName, t.ExtendedTrackTitle} {
		if o.IsSome() {
			n++
		}
	}
	return n
}

// Contains reports whether the given key maps to a present field.
func (t *Lyrics3v2Tag) Contains(key meta.ItemKey) bool {
	switch key {
	case meta.ItemKeyLyrics:
		return t.Lyrics.IsSome()
	case meta.ItemKeyComment:
		return t.Info.IsSome()
	case meta.ItemKeyLyricist:
		return t.Author.IsSome()
	case meta.ItemKeyAlbumTitle:
		return t.ExtendedAlbumName.IsSome()
	case meta.ItemKeyTrackArtist:
		return t.ExtendedArtistName.IsSome()
	case meta.ItemKeyTrackTitle:
		return t.ExtendedTrackTitle.IsSome()
	}
	return false
}

// Accessor methods mirroring Rust's `Accessor` impl.
func (t *Lyrics3v2Tag) Comment() meta.Option[string] { return t.Info }
func (t *Lyrics3v2Tag) SetComment(v string)          { t.Info = meta.Some(v) }
func (t *Lyrics3v2Tag) RemoveComment()               { t.Info = meta.Option[string]{} }
func (t *Lyrics3v2Tag) Artist() meta.Option[string]  { return t.ExtendedArtistName }
func (t *Lyrics3v2Tag) SetArtist(v string)           { t.ExtendedArtistName = meta.Some(v) }
func (t *Lyrics3v2Tag) RemoveArtist()                { t.ExtendedArtistName = meta.Option[string]{} }
func (t *Lyrics3v2Tag) Album() meta.Option[string]   { return t.ExtendedAlbumName }
func (t *Lyrics3v2Tag) SetAlbum(v string)            { t.ExtendedAlbumName = meta.Some(v) }
func (t *Lyrics3v2Tag) RemoveAlbum()                 { t.ExtendedAlbumName = meta.Option[string]{} }
func (t *Lyrics3v2Tag) Title() meta.Option[string]   { return t.ExtendedTrackTitle }
func (t *Lyrics3v2Tag) SetTitle(v string)            { t.ExtendedTrackTitle = meta.Some(v) }
func (t *Lyrics3v2Tag) RemoveTitle()                 { t.ExtendedTrackTitle = meta.Option[string]{} }

// DumpTo writes the encoded tag to writer.
func (t *Lyrics3v2Tag) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := t.encode(writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// len returns the encoded size of the tag's fields (for the size footer).
func (t *Lyrics3v2Tag) fieldsLen() int {
	fieldLen := func(o meta.Option[string], maxLen int) int {
		v, ok := o.Get()
		if !ok || v == "" {
			return 0
		}
		return 8 + min(len([]rune(v)), maxLen)
	}
	n := 8 + 3 // IND field
	n += fieldLen(t.Lyrics, lyrics3v2FieldMaxSize)
	n += fieldLen(t.Info, lyrics3v2FieldMaxSize)
	n += fieldLen(t.Author, lyrics3v2BasicMaxSize)
	n += fieldLen(t.ExtendedAlbumName, lyrics3v2BasicMaxSize)
	n += fieldLen(t.ExtendedArtistName, lyrics3v2BasicMaxSize)
	n += fieldLen(t.ExtendedTrackTitle, lyrics3v2BasicMaxSize)
	if len(t.Images) > 0 {
		var imgLen int
		for i, img := range t.Images {
			imgLen += min(len([]rune(img.Filename)), lyrics3v2PathMax) + 2
			if d, ok := img.Description.Get(); ok {
				imgLen += min(len([]rune(d)), lyrics3v2BasicMaxSize)
			}
			imgLen += 2
			if img.Timestamp != nil {
				imgLen += 7 // "[mm:ss]"
			}
			if i != len(t.Images)-1 {
				imgLen += 2 // "\r\n"
			}
		}
		n += 8 + min(imgLen, lyrics3v2FieldMaxSize)
	}
	return n
}

func (t *Lyrics3v2Tag) encode(writeOptions meta.WriteOptions) ([]byte, error) {
	filter := func(o meta.Option[string]) (string, bool) {
		v, ok := o.Get()
		return v, ok && v != ""
	}
	_, hasLyrics := filter(t.Lyrics)
	_, hasInfo := filter(t.Info)
	_, hasAuthor := filter(t.Author)
	_, hasEAL := filter(t.ExtendedAlbumName)
	_, hasEAR := filter(t.ExtendedArtistName)
	_, hasETT := filter(t.ExtendedTrackTitle)
	if !hasLyrics && !hasInfo && !hasAuthor && !hasEAL && !hasEAR && !hasETT && len(t.Images) == 0 {
		return nil, nil // Stripping
	}

	tagContentLen := len(lyrics3v2Header) + t.fieldsLen()
	if tagContentLen > lyrics3v2TagMaxSize {
		return nil, &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: &meta.TooMuchDataError{}}
	}

	var buf bytes.Buffer
	buf.Write(lyrics3v2Header)

	// Indications
	buf.WriteString("IND00003")
	ind := [3]byte{'0', '0', '0'}
	if _, ok := filter(t.Lyrics); ok {
		ind[0] = '1'
		if t.Flags.SynchronizedLyrics {
			ind[1] = '1'
		}
	}
	if t.Flags.RandomSelection {
		ind[2] = '1'
	}
	buf.Write(ind[:])

	writeField := func(id string, maxSize int, content meta.Option[string]) error {
		v, ok := filter(content)
		if !ok {
			return nil
		}
		contentLen := min(len([]rune(v)), maxSize)
		buf.WriteString(id)
		fmt.Fprintf(&buf, "%05d", contentLen)
		enc, err := writeLatin1Field(v, maxSize, writeOptions,
			&Lyrics3v2ParseError{Message: id + " contains invalid characters"})
		if err != nil {
			return &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: err}
		}
		buf.Write(enc)
		return nil
	}

	for _, f := range []struct {
		id  string
		max int
		opt meta.Option[string]
	}{
		{"LYR", lyrics3v2FieldMaxSize, t.Lyrics},
		{"INF", lyrics3v2FieldMaxSize, t.Info},
		{"AUT", lyrics3v2BasicMaxSize, t.Author},
		{"EAL", lyrics3v2BasicMaxSize, t.ExtendedAlbumName},
		{"EAR", lyrics3v2BasicMaxSize, t.ExtendedArtistName},
		{"ETT", lyrics3v2BasicMaxSize, t.ExtendedTrackTitle},
	} {
		if err := writeField(f.id, f.max, f.opt); err != nil {
			return nil, err
		}
	}

	if len(t.Images) > 0 {
		var imgBuf bytes.Buffer
		for i, img := range t.Images {
			fn, err := writeLatin1Field(img.Filename, lyrics3v2PathMax, writeOptions,
				&Lyrics3v2ParseError{Message: "image filename contains invalid characters"})
			if err != nil {
				return nil, &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: err}
			}
			imgBuf.Write(fn)
			imgBuf.WriteString("||")
			if d, ok := img.Description.Get(); ok {
				de, err := writeLatin1Field(d, lyrics3v2BasicMaxSize, writeOptions,
					&Lyrics3v2ParseError{Message: "image description contains invalid characters"})
				if err != nil {
					return nil, &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: err}
				}
				imgBuf.Write(de)
			}
			imgBuf.WriteString("||")
			if img.Timestamp != nil {
				mi, _ := img.Timestamp.Minute.Get()
				se, _ := img.Timestamp.Second.Get()
				fmt.Fprintf(&imgBuf, "[%02d:%02d]", min(int(mi), 59), min(int(se), 59))
			}
			if i != len(t.Images)-1 {
				imgBuf.WriteString("\r\n")
			}
		}
		if imgBuf.Len() > lyrics3v2FieldMaxSize {
			return nil, &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: &meta.TooMuchDataError{}}
		}
		buf.WriteString("IMG")
		fmt.Fprintf(&buf, "%05d", imgBuf.Len())
		buf.Write(imgBuf.Bytes())
	}

	fmt.Fprintf(&buf, "%06d", tagContentLen)
	buf.Write(lyrics3v2Footer)
	return buf.Bytes(), nil
}

// FindLyrics3 searches for a Lyrics3 tag ending right before the current
// position. On success the reader is positioned at the start of the tag.
// Returns (start, end, version, tag, found, err) where version is 1 or 2.
func FindLyrics3(reader io.ReadSeeker, read bool, parseOptions meta.ParseOptions) (int64, int64, any, bool, error) {
	end, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, nil, false, err
	}
	// Rust seeks relative to the current position, so a negative target is an
	// error rather than "no tag".
	if end < int64(len(lyrics3v1Footer)) {
		return 0, 0, nil, false, &meta.TagParseError{Source: errNegativeSeek}
	}
	if _, err := reader.Seek(end-int64(len(lyrics3v1Footer)), io.SeekStart); err != nil {
		return 0, 0, nil, false, err
	}
	var footer [9]byte
	if _, err := io.ReadFull(reader, footer[:]); err != nil {
		return 0, 0, nil, false, err
	}

	var maxSize int64
	switch {
	case bytes.Equal(footer[:], lyrics3v2Footer):
		maxSize = lyrics3v2TagMaxSize + 6 + int64(len(lyrics3v2Footer))
	case bytes.Equal(footer[:], lyrics3v1Footer):
		maxSize = int64(len(lyrics3v2Header)) + lyrics3v1MaxLyricsLen + int64(len(lyrics3v1Footer))
	default:
		return 0, 0, nil, false, nil
	}
	version := 1
	if bytes.Equal(footer[:], lyrics3v2Footer) {
		version = 2
	}

	// Search backwards for "LYRICSBEGIN" within the max tag size.
	// A matching footer without a header is a corrupt tag — Rust's
	// find_lyrics3 propagates a FakeTagError, it does not return None.
	lowerBound := end - maxSize
	if lowerBound < 0 {
		lowerBound = 0
	}
	found, err := util.NewRevPatternSearcher(reader, lyrics3v2Header).
		WithStart(util.RevSearchFromCurrent).
		WithEnd(util.RevSearchEndAtPos(uint64(lowerBound))).Search()
	if err != nil {
		return 0, 0, nil, false, err
	}
	if !found {
		if version == 2 {
			return 0, 0, nil, false, &Lyrics3v2ParseError{Source: &meta.FakeTagError{}}
		}
		return 0, 0, nil, false, &Lyrics3ParseError{Source: &meta.FakeTagError{}}
	}
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, nil, false, err
	}
	if start < lowerBound || start > end-int64(len(lyrics3v2Header)) {
		return 0, 0, nil, false, nil
	}

	var tag any
	// `read` alone decides whether the tag is parsed (Rust ignores
	// `parse_options.read_tags` here).
	if read {
		content := make([]byte, end-start)
		if _, err := reader.Seek(start, io.SeekStart); err != nil {
			return 0, 0, nil, false, err
		}
		if _, err := io.ReadFull(reader, content); err != nil {
			return 0, 0, nil, false, err
		}
		if version == 2 {
			t, err := ParseLyrics3v2Tag(content, parseOptions)
			if err != nil {
				return 0, 0, nil, false, err
			}
			tag = t
		} else {
			t, err := ParseLyrics3Tag(content)
			if err != nil {
				return 0, 0, nil, false, err
			}
			tag = t
		}
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return 0, 0, nil, false, err
	}
	return start, end, tag, true, nil
}

// Lyrics3v2WriteTo writes a Lyrics3v2 tag into a file. The file must contain
// an ID3v1 tag; the Lyrics3v2 tag is written immediately before it.
func Lyrics3v2WriteTo(file meta.FileLike, tag *Lyrics3v2Tag, writeOptions meta.WriteOptions) error {
	// Find ID3v1 — Lyrics3v2 requires it
	id3v1Start, _, found, err := tagpos.LocateId3v1(file)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if !found {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeLyrics3v2, HasTagType: true, Source: &Lyrics3v2ParseError{Message: "the destination file must contain an ID3v1 tag"}}}
	}
	// Find an existing Lyrics3 tag before it
	existingStart, existingEnd := id3v1Start, id3v1Start
	if _, err := file.Seek(id3v1Start, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	// Rust propagates `find_lyrics3` errors (e.g. a footer without a header)
	l3start, l3end, _, l3found, err := FindLyrics3(file, false, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if l3found {
		existingStart, existingEnd = l3start, l3end
	}
	encoded, err := tag.encode(writeOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return meta.Splice(file, uint64(existingStart), uint64(existingEnd), encoded)
}

// Lyrics3v1WriteTo writes a Lyrics3v1 tag into a file immediately before ID3v1.
func Lyrics3v1WriteTo(file meta.FileLike, tag *Lyrics3Tag, writeOptions meta.WriteOptions) error {
	id3v1Start, _, found, err := tagpos.LocateId3v1(file)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if !found {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeLyrics3, HasTagType: true, Source: &Lyrics3ParseError{Message: "the destination file must contain an ID3v1 tag"}}}
	}
	existingStart, existingEnd := id3v1Start, id3v1Start
	if _, err := file.Seek(id3v1Start, io.SeekStart); err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	l3start, l3end, _, l3found, err := FindLyrics3(file, false, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	if l3found {
		existingStart, existingEnd = l3start, l3end
	}
	encoded, err := tag.encode(writeOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	return meta.Splice(file, uint64(existingStart), uint64(existingEnd), encoded)
}

func init() {
	meta.RegisterTagDumper(meta.TagTypeLyrics3v2, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		data, err := Lyrics3v2TagFromTag(tag).encode(writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	meta.RegisterTagDumper(meta.TagTypeLyrics3, func(tag *meta.Tag, writer io.Writer, writeOptions meta.WriteOptions) error {
		data, err := Lyrics3TagFromTag(tag).encode(writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}

// SaveTo writes the tag into an MPEG file, mirroring Rust `TagExt::save_to`.
func (t *Lyrics3Tag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeLyrics3)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return Lyrics3v1WriteTo(file, t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *Lyrics3Tag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeLyrics3)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return Lyrics3v1WriteTo(vf.File(), t, writeOptions)
}

// RemoveFrom removes a Lyrics3v1 tag from a [meta.FileLike].
func (t *Lyrics3Tag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeLyrics3.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes a Lyrics3v1 tag from a path.
func (t *Lyrics3Tag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeLyrics3.RemoveFromPath(path, writeOptions)
}

// SaveTo writes the tag into an MPEG file, mirroring Rust `TagExt::save_to`.
func (t *Lyrics3v2Tag) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeLyrics3v2)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return Lyrics3v2WriteTo(file, t, writeOptions)
}

// SaveToPath writes the tag to a path.
func (t *Lyrics3v2Tag) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeLyrics3v2)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !t.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return Lyrics3v2WriteTo(vf.File(), t, writeOptions)
}

// RemoveFrom removes a Lyrics3v2 tag from a [meta.FileLike].
func (t *Lyrics3v2Tag) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeLyrics3v2.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes a Lyrics3v2 tag from a path.
func (t *Lyrics3v2Tag) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeLyrics3v2.RemoveFromPath(path, writeOptions)
}
