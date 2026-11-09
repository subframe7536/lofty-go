package lyrics3

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/tagpos"
)

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
		return nil, &Lyrics3ParseError{Source: &FakeTagError{}}
	}
	return &Lyrics3Tag{
		Lyrics: latin1Decode(content[len(lyrics3v2Header) : len(content)-len(lyrics3v1Footer)]),
	}, nil
}

// Tag converts the tag to a generic [Tag].
func (t *Lyrics3Tag) Tag() *Tag {
	tag := NewTag(TagTypeLyrics3)
	if t.Lyrics != "" {
		tag.PushUnchecked(NewTagItem(ItemKeyLyrics, ItemValueText(t.Lyrics)))
	}
	return tag
}

// Lyrics3TagFromTag converts a generic [Tag] to a [Lyrics3Tag].
func Lyrics3TagFromTag(tag *Tag) *Lyrics3Tag {
	out := &Lyrics3Tag{}
	if s, ok := tag.GetString(ItemKeyLyrics); ok {
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
func (t *Lyrics3Tag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data, err := t.encode(writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func (t *Lyrics3Tag) encode(writeOptions WriteOptions) ([]byte, error) {
	lyrics := t.Lyrics
	if lyrics == "" {
		return nil, nil // Stripping
	}
	if strings.Contains(lyrics, "LYRICSBEGIN") || strings.Contains(lyrics, "LYRICSEND") {
		return nil, &TagEncodingError{
			TagType: TagTypeLyrics3, HasTagType: true,
			Source: &Lyrics3ParseError{Message: "lyrics text contains tag markers"},
		}
	}
	enc, err := writeLatin1Field(lyrics, lyrics3v1MaxLyricsLen, writeOptions,
		&Lyrics3ParseError{Message: "lyrics text contains invalid characters"})
	if err != nil {
		return nil, &TagEncodingError{TagType: TagTypeLyrics3, HasTagType: true, Source: err}
	}
	out := make([]byte, 0, len(lyrics3v2Header)+len(enc)+len(lyrics3v1Footer))
	out = append(out, lyrics3v2Header...)
	out = append(out, enc...)
	out = append(out, lyrics3v1Footer...)
	return out, nil
}

// writeLatin1Field encodes a string to Latin-1, truncating to maxChars. When
// the content exceeds maxChars, TooMuchDataError is returned unless
// WriteOptions.ImplicitTruncation is set.
func writeLatin1Field(s string, maxChars int, writeOptions WriteOptions, encErr error) ([]byte, error) {
	if len([]rune(s)) > maxChars && !writeOptions.ImplicitTruncation {
		return nil, &TooMuchDataError{}
	}
	enc, err := latin1Encode(s, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, encErr
	}
	for i, b := range enc {
		if b == 0xFF {
			if !writeOptions.LossyTextEncoding {
				return nil, encErr
			}
			enc[i] = '?'
		}
	}
	if len(enc) > maxChars {
		enc = enc[:maxChars]
	}
	return enc, nil
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
	Description Option[string]
	Timestamp   *Timestamp
}

// Lyrics3v2Tag is a Lyrics3 v2 tag.
type Lyrics3v2Tag struct {
	Flags              Lyrics3v2TagFlags
	Lyrics             Option[string]
	Info               Option[string]
	Author             Option[string]
	ExtendedAlbumName  Option[string]
	ExtendedArtistName Option[string]
	ExtendedTrackTitle Option[string]
	Images             []Lyrics3v2Image
}

// ParseLyrics3v2Tag parses a complete Lyrics3v2 tag
// (`LYRICSBEGIN`..`<size>`..`LYRICS200`).
func ParseLyrics3v2Tag(content []byte, parseOptions ParseOptions) (*Lyrics3v2Tag, error) {
	if len(content) < len(lyrics3v2Header)+len(lyrics3v2Footer) ||
		!bytes.Equal(content[:len(lyrics3v2Header)], lyrics3v2Header) ||
		!bytes.HasSuffix(content, lyrics3v2Footer) {
		return nil, &Lyrics3v2ParseError{Source: &FakeTagError{}}
	}

	body := content[:len(content)-len(lyrics3v2Footer)]
	if len(body) < 6 {
		return nil, &Lyrics3v2ParseError{Source: &FakeTagError{}}
	}

	sizeBytes, tagSize := asciiNumber(body[len(body)-6:])
	if sizeBytes != 6 {
		return nil, &Lyrics3v2ParseError{Message: "non-numeric tag size"}
	}
	body = body[:len(body)-6]
	if int(tagSize) != len(body) {
		if parseOptions.ParsingMode == ParsingModeStrict {
			return nil, &Lyrics3v2ParseError{Source: &SizeMismatchError{}}
		}
	}

	tag := &Lyrics3v2Tag{}
	fields := body[len(lyrics3v2Header):]
	mode := parseOptions.ParsingMode

	parseText := func(size, max int) (string, error) {
		if size > max && mode == ParsingModeStrict {
			return "", &Lyrics3v2ParseError{Source: &TooMuchDataError{}}
		}
		if len(fields) < size {
			return "", &Lyrics3v2ParseError{Source: &SizeMismatchError{}}
		}
		raw := fields[:size]
		fields = fields[size:]
		if len(raw) > max {
			raw = raw[:max]
		}
		return latin1Decode(raw), nil
	}

	for len(fields) > 0 {
		if len(fields) < 8 {
			return nil, &Lyrics3v2ParseError{Source: io.ErrUnexpectedEOF}
		}
		id := string(fields[:3])
		n, size64 := asciiNumber(fields[3:8])
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
				if mode == ParsingModeStrict {
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
				if mode == ParsingModeStrict {
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
			tag.Lyrics = Some(v)
		case "INF":
			v, err := parseText(size, lyrics3v2FieldMaxSize)
			if err != nil {
				return nil, err
			}
			tag.Info = Some(v)
		case "AUT":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.Author = Some(v)
		case "EAL":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedAlbumName = Some(v)
		case "EAR":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedArtistName = Some(v)
		case "ETT":
			v, err := parseText(size, lyrics3v2BasicMaxSize)
			if err != nil {
				return nil, err
			}
			tag.ExtendedTrackTitle = Some(v)
		case "IMG":
			imgStr, err := parseText(size, lyrics3v2FieldMaxSize)
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(strings.ReplaceAll(imgStr, "\r\n", "\n"), "\n") {
				if line == "" {
					continue
				}
				parts := strings.Split(line, "||")
				var img Lyrics3v2Image
				img.Filename = parts[0]
				if len(parts) > 1 && parts[1] != "" {
					img.Description = Some(parts[1])
				} else if len(parts) <= 1 && mode == ParsingModeStrict {
					return nil, &Lyrics3v2ParseError{Message: "image field missing a description"}
				}
				if len(parts) > 2 && parts[2] != "" {
					ts, err := lyrics3v2ParseTimestamp(parts[2])
					if err != nil {
						if mode == ParsingModeStrict {
							return nil, err
						}
					} else {
						img.Timestamp = ts
					}
				} else if len(parts) <= 2 && mode == ParsingModeStrict {
					return nil, &Lyrics3v2ParseError{Message: "image field missing a timestamp"}
				}
				tag.Images = append(tag.Images, img)
			}
		default:
			// Unknown field — skip
			if len(fields) < size {
				return nil, &Lyrics3v2ParseError{Source: &SizeMismatchError{}}
			}
			fields = fields[size:]
		}
	}
	return tag, nil
}

// lyrics3v2ParseTimestamp parses a `[mm:ss]` timestamp.
func lyrics3v2ParseTimestamp(s string) (*Timestamp, error) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	mins, secs, found := strings.Cut(s[1:len(s)-1], ":")
	if !found {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	mb, m := asciiNumber([]byte(mins))
	if mb != 2 || m > 59 {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	sb, sec := asciiNumber([]byte(secs))
	if sb != 2 || sec > 59 {
		return nil, &Lyrics3v2ParseError{Message: "malformed timestamp"}
	}
	return &Timestamp{Minute: Some(uint8(m)), Second: Some(uint8(sec))}, nil
}

// Tag converts the tag to a generic [Tag]. Flags and images ride the
// companion tag when [GlobalOptions.PreserveFormatSpecificItems] is enabled.
func (t *Lyrics3v2Tag) Tag() *Tag {
	tag := NewTag(TagTypeLyrics3v2)
	take := func(opt Option[string], key ItemKey) {
		if v, ok := opt.Get(); ok && v != "" {
			tag.PushUnchecked(NewTagItem(key, ItemValueText(v)))
		}
	}
	take(t.Lyrics, ItemKeyLyrics)
	take(t.Info, ItemKeyComment)
	take(t.Author, ItemKeyLyricist)
	take(t.ExtendedAlbumName, ItemKeyAlbumTitle)
	take(t.ExtendedArtistName, ItemKeyTrackArtist)
	take(t.ExtendedTrackTitle, ItemKeyTrackTitle)

	if getGlobalOptions().PreserveFormatSpecificItems {
		remainder := *t
		remainder.Lyrics, remainder.Info, remainder.Author = Option[string]{}, Option[string]{}, Option[string]{}
		remainder.ExtendedAlbumName, remainder.ExtendedArtistName, remainder.ExtendedTrackTitle = Option[string]{}, Option[string]{}, Option[string]{}
		if !remainder.IsEmpty() {
			tag.SetCompanionTag(&remainder)
		}
	}
	return tag
}

// Lyrics3v2TagFromTag merges a generic [Tag] into a [Lyrics3v2Tag], taking
// format-specific data back out of the companion tag when present.
func Lyrics3v2TagFromTag(input *Tag) *Lyrics3v2Tag {
	var merged Lyrics3v2Tag
	if getGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := input.CompanionTag().(*Lyrics3v2Tag); ok && comp != nil {
			merged = *comp
		}
	}
	take := func(key ItemKey, dst *Option[string]) {
		if strs := input.TakeStrings(key); len(strs) > 0 {
			*dst = Some(strs[0])
		}
	}
	take(ItemKeyLyrics, &merged.Lyrics)
	take(ItemKeyComment, &merged.Info)
	take(ItemKeyLyricist, &merged.Author)
	take(ItemKeyAlbumTitle, &merged.ExtendedAlbumName)
	take(ItemKeyTrackArtist, &merged.ExtendedArtistName)
	take(ItemKeyTrackTitle, &merged.ExtendedTrackTitle)
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

// IsEmpty reports whether the tag has no fields, images, or set flags.
func (t *Lyrics3v2Tag) IsEmpty() bool {
	return t.isContentEmpty() && !t.Flags.SynchronizedLyrics && !t.Flags.RandomSelection
}

// Len returns the number of fields and images present.
func (t *Lyrics3v2Tag) Len() int {
	n := len(t.Images)
	for _, o := range []Option[string]{t.Lyrics, t.Info, t.Author, t.ExtendedAlbumName, t.ExtendedArtistName, t.ExtendedTrackTitle} {
		if o.IsSome() {
			n++
		}
	}
	return n
}

// Contains reports whether the given key maps to a present field.
func (t *Lyrics3v2Tag) Contains(key ItemKey) bool {
	switch key {
	case ItemKeyLyrics:
		return t.Lyrics.IsSome()
	case ItemKeyComment:
		return t.Info.IsSome()
	case ItemKeyLyricist:
		return t.Author.IsSome()
	case ItemKeyAlbumTitle:
		return t.ExtendedAlbumName.IsSome()
	case ItemKeyTrackArtist:
		return t.ExtendedArtistName.IsSome()
	case ItemKeyTrackTitle:
		return t.ExtendedTrackTitle.IsSome()
	}
	return false
}

// Accessor methods mirroring Rust's `Accessor` impl.
func (t *Lyrics3v2Tag) Comment() Option[string] { return t.Info }
func (t *Lyrics3v2Tag) SetComment(v string)     { t.Info = Some(v) }
func (t *Lyrics3v2Tag) RemoveComment()          { t.Info = Option[string]{} }
func (t *Lyrics3v2Tag) Artist() Option[string]  { return t.ExtendedArtistName }
func (t *Lyrics3v2Tag) SetArtist(v string)      { t.ExtendedArtistName = Some(v) }
func (t *Lyrics3v2Tag) RemoveArtist()           { t.ExtendedArtistName = Option[string]{} }
func (t *Lyrics3v2Tag) Album() Option[string]   { return t.ExtendedAlbumName }
func (t *Lyrics3v2Tag) SetAlbum(v string)       { t.ExtendedAlbumName = Some(v) }
func (t *Lyrics3v2Tag) RemoveAlbum()            { t.ExtendedAlbumName = Option[string]{} }
func (t *Lyrics3v2Tag) Title() Option[string]   { return t.ExtendedTrackTitle }
func (t *Lyrics3v2Tag) SetTitle(v string)       { t.ExtendedTrackTitle = Some(v) }
func (t *Lyrics3v2Tag) RemoveTitle()            { t.ExtendedTrackTitle = Option[string]{} }

// DumpTo writes the encoded tag to writer.
func (t *Lyrics3v2Tag) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data, err := t.encode(writeOptions)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// len returns the encoded size of the tag's fields (for the size footer).
func (t *Lyrics3v2Tag) fieldsLen() int {
	fieldLen := func(o Option[string], maxLen int) int {
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

func (t *Lyrics3v2Tag) encode(writeOptions WriteOptions) ([]byte, error) {
	filter := func(o Option[string]) (string, bool) {
		v, ok := o.Get()
		return v, ok && v != ""
	}
	if t.isContentEmpty() {
		return nil, nil // Stripping
	}

	tagContentLen := len(lyrics3v2Header) + t.fieldsLen()
	if tagContentLen > lyrics3v2TagMaxSize {
		return nil, &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: &TooMuchDataError{}}
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

	writeField := func(id string, maxSize int, content Option[string]) error {
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
			return &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: err}
		}
		buf.Write(enc)
		return nil
	}

	for _, f := range []struct {
		id  string
		max int
		opt Option[string]
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
				return nil, &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: err}
			}
			imgBuf.Write(fn)
			imgBuf.WriteString("||")
			if d, ok := img.Description.Get(); ok {
				de, err := writeLatin1Field(d, lyrics3v2BasicMaxSize, writeOptions,
					&Lyrics3v2ParseError{Message: "image description contains invalid characters"})
				if err != nil {
					return nil, &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: err}
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
			return nil, &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: &TooMuchDataError{}}
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
func FindLyrics3(reader io.ReadSeeker, read bool, parseOptions ParseOptions) (int64, int64, any, bool, error) {
	end, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, nil, false, err
	}
	if end < int64(len(lyrics3v1Footer)) {
		return 0, 0, nil, false, nil
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

	// Search backwards for "LYRICSBEGIN" within the max tag size
	lowerBound := end - maxSize
	if lowerBound < 0 {
		lowerBound = 0
	}
	found, err := newRevPatternSearcher(reader, lyrics3v2Header).
		WithStart(revSearchFromCurrent).
		WithEnd(revSearchEndAtPos(uint64(lowerBound))).Search()
	if err != nil || !found {
		return 0, 0, nil, false, nil
	}
	start, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, nil, false, err
	}
	if start < lowerBound || start > end-int64(len(lyrics3v2Header)) {
		return 0, 0, nil, false, nil
	}

	var tag any
	if read && parseOptions.ReadTags {
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
func Lyrics3v2WriteTo(file FileLike, tag *Lyrics3v2Tag, writeOptions WriteOptions) error {
	// Find ID3v1 — Lyrics3v2 requires it
	id3v1Start, _, found, err := tagpos.LocateId3v1(file)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	if !found {
		return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeLyrics3v2, HasTagType: true, Source: &Lyrics3v2ParseError{Message: "the destination file must contain an ID3v1 tag"}}}
	}
	// Find an existing Lyrics3 tag before it
	existingStart, existingEnd := id3v1Start, id3v1Start
	if _, err := file.Seek(id3v1Start, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	if l3start, l3end, _, l3found, err := FindLyrics3(file, false, writeOptions.ParseOptions); err == nil && l3found {
		existingStart, existingEnd = l3start, l3end
	}
	encoded, err := tag.encode(writeOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return Splice(file, uint64(existingStart), uint64(existingEnd), encoded)
}

// Lyrics3v1WriteTo writes a Lyrics3v1 tag into a file immediately before ID3v1.
func Lyrics3v1WriteTo(file FileLike, tag *Lyrics3Tag, writeOptions WriteOptions) error {
	id3v1Start, _, found, err := tagpos.LocateId3v1(file)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	if !found {
		return &FileEncodingError{Source: &TagEncodingError{TagType: TagTypeLyrics3, HasTagType: true, Source: &Lyrics3ParseError{Message: "the destination file must contain an ID3v1 tag"}}}
	}
	existingStart, existingEnd := id3v1Start, id3v1Start
	if _, err := file.Seek(id3v1Start, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	if l3start, l3end, _, l3found, err := FindLyrics3(file, false, writeOptions.ParseOptions); err == nil && l3found {
		existingStart, existingEnd = l3start, l3end
	}
	encoded, err := tag.encode(writeOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return Splice(file, uint64(existingStart), uint64(existingEnd), encoded)
}

func init() {
	meta.RegisterTagDumper(meta.TagTypeLyrics3v2, func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		data, err := Lyrics3v2TagFromTag(tag).encode(writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	meta.RegisterTagDumper(meta.TagTypeLyrics3, func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
		data, err := Lyrics3TagFromTag(tag).encode(writeOptions)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}
