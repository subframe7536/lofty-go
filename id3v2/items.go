package id3v2

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"

	meta "github.com/subframe7536/lofty-go/internal/meta"

	"github.com/subframe7536/lofty-go/internal/util"
)

// --- TextInformationFrame ("T...", excluding TXXX) ---

// TextInformationFrame is an ID3v2 text frame.
type TextInformationFrame struct {
	header   FrameHeader
	Encoding TextEncoding
	Value    string
}

// NewTextInformationFrame creates a new [TextInformationFrame].
func NewTextInformationFrame(id FrameId, encoding TextEncoding, value string) *TextInformationFrame {
	return &TextInformationFrame{
		header:   NewFrameHeader(id, FrameFlags{}),
		Encoding: encoding,
		Value:    value,
	}
}

func (f *TextInformationFrame) ID() FrameId            { return f.header.ID }
func (f *TextInformationFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *TextInformationFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *TextInformationFrame) frameEqual(other Frame) bool {
	o, ok := other.(*TextInformationFrame)
	return ok && f.header.ID == o.header.ID
}

func (f *TextInformationFrame) frameIsEmpty() (bool, bool) { return f.Value == "", true }
func (f *TextInformationFrame) frameName() string          { return "Text" }

func (f *TextInformationFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	content, err := encoding.Encode(f.Value, false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append([]byte{uint8(encoding)}, content...), nil
}

func (f *TextInformationFrame) cloneFrame() Frame {
	c := *f
	return &c
}

// parseTextInformationFrame reads a TextInformationFrame; returns nil when the
// content is empty (Rust `TextInformationFrame::parse` -> meta.Option).
func parseTextInformationFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, version Id3v2Version) (*TextInformationFrame, error) {
	var encByte [1]byte
	if _, err := io.ReadFull(reader, encByte[:]); err != nil {
		return nil, nil
	}
	encoding, err := verifyEncoding(encByte[0], version)
	if err != nil {
		return nil, setFrameErrID(err, id)
	}
	res, err := decodeText(reader, textDecodeOpts(encoding, false))
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	return &TextInformationFrame{
		header:   NewFrameHeader(id, frameFlags),
		Encoding: encoding,
		Value:    res.Content,
	}, nil
}

// --- UrlLinkFrame ("W...", excluding WXXX) ---

// UrlLinkFrame is an ID3v2 URL link frame.
type UrlLinkFrame struct {
	header  FrameHeader
	Content string
}

// NewUrlLinkFrame creates a new [UrlLinkFrame].
func NewUrlLinkFrame(id FrameId, content string) *UrlLinkFrame {
	return &UrlLinkFrame{header: NewFrameHeader(id, FrameFlags{}), Content: content}
}

func (f *UrlLinkFrame) ID() FrameId            { return f.header.ID }
func (f *UrlLinkFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *UrlLinkFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *UrlLinkFrame) frameEqual(other Frame) bool {
	o, ok := other.(*UrlLinkFrame)
	return ok && f.header.ID == o.header.ID
}

func (f *UrlLinkFrame) frameIsEmpty() (bool, bool) { return f.Content == "", true }
func (f *UrlLinkFrame) frameName() string          { return "Url" }

func (f *UrlLinkFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	b, err := util.TextEncodingLatin1.Encode(f.Content, false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return b, nil
}

func (f *UrlLinkFrame) cloneFrame() Frame {
	c := *f
	return &c
}

// Url returns the URL of the frame.
func (f *UrlLinkFrame) Url() string { return f.Content }

// SetUrl changes the URL of the frame, reporting whether it was valid Latin-1.
func (f *UrlLinkFrame) SetUrl(url string) bool {
	if util.VerifyLatin1(url) {
		f.Content = url
		return true
	}
	return false
}

func parseUrlLinkFrame(reader io.Reader, id FrameId, frameFlags FrameFlags) (*UrlLinkFrame, error) {
	res, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	if res.BytesRead == 0 {
		return nil, nil
	}
	return &UrlLinkFrame{
		header:  NewFrameHeader(id, frameFlags),
		Content: res.Content,
	}, nil
}

// --- languageFrame: shared parse/write for CommentFrame/UnsynchronizedTextFrame ---

type languageFrame struct {
	encoding    TextEncoding
	language    meta.Lang
	description string
	content     string
}

func parseLanguageFrame(reader io.Reader, version Id3v2Version) (*languageFrame, error) {
	var encByte [1]byte
	if _, err := io.ReadFull(reader, encByte[:]); err != nil {
		return nil, nil
	}
	encoding, err := verifyEncoding(encByte[0], version)
	if err != nil {
		return nil, err
	}
	var language [3]byte
	if _, err := io.ReadFull(reader, language[:]); err != nil {
		return nil, frameParseError(meta.None[FrameId](), err)
	}
	descRes, err := decodeText(reader, textDecodeOpts(encoding, true))
	if err != nil {
		return nil, frameParseError(meta.None[FrameId](), err)
	}

	var content string
	if encoding == util.TextEncodingUTF16 && (descRes.BOM == [2]byte{0xFF, 0xFE} || descRes.BOM == [2]byte{0xFE, 0xFF}) {
		order := util.LittleEndian
		if descRes.BOM == [2]byte{0xFE, 0xFF} {
			order = util.BigEndian
		}
		content, _, err = util.Utf16DecodeTerminatedMaybeBom(reader, order)
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
	} else {
		res, err := decodeText(reader, textDecodeOpts(encoding, false))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		content = res.Content
	}

	return &languageFrame{
		encoding:    encoding,
		language:    meta.Lang(language),
		description: descRes.Content,
		content:     content,
	}, nil
}

func languageFrameBytes(encoding TextEncoding, language [3]byte, description, content string, writeOptions meta.WriteOptions) ([]byte, error) {
	if writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	for _, c := range language {
		if !isASCIIAlphabetic(c) {
			return nil, frameEncodingInvalidLanguage(language)
		}
	}
	bytes_ := []byte{uint8(encoding)}
	bytes_ = append(bytes_, language[:]...)
	desc, err := encoding.Encode(description, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	bytes_ = append(bytes_, desc...)
	cont, err := encoding.Encode(content, false, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	bytes_ = append(bytes_, cont...)
	return bytes_, nil
}

func isASCIIAlphabetic(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// --- CommentFrame ("COMM") ---

// CommentFrame is an ID3v2 "COMM" frame.
type CommentFrame struct {
	header      FrameHeader
	Encoding    TextEncoding
	Language    meta.Lang
	Description string
	Content     string
}

// NewCommentFrame creates a new [CommentFrame].
func NewCommentFrame(encoding TextEncoding, language meta.Lang, description, content string) *CommentFrame {
	return &CommentFrame{
		header:      NewFrameHeader(FrameIdValid("COMM"), FrameFlags{}),
		Encoding:    encoding,
		Language:    language,
		Description: description,
		Content:     content,
	}
}

func (f *CommentFrame) ID() FrameId            { return f.header.ID }
func (f *CommentFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *CommentFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *CommentFrame) frameEqual(other Frame) bool {
	o, ok := other.(*CommentFrame)
	return ok && f.Language == o.Language && f.Description == o.Description
}

func (f *CommentFrame) frameIsEmpty() (bool, bool) { return f.Content == "", true }
func (f *CommentFrame) frameName() string          { return "Comment" }

func (f *CommentFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	return languageFrameBytes(f.Encoding, f.Language, f.Description, f.Content, ctx.writeOptions)
}

func (f *CommentFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseCommentFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*CommentFrame, error) {
	lf, err := parseLanguageFrame(reader, version)
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("COMM"))
	}
	if lf == nil {
		return nil, nil
	}
	return &CommentFrame{
		header:      NewFrameHeader(FrameIdValid("COMM"), frameFlags),
		Encoding:    lf.encoding,
		Language:    lf.language,
		Description: lf.description,
		Content:     lf.content,
	}, nil
}

// --- UnsynchronizedTextFrame ("USLT") ---

// UnsynchronizedTextFrame is an ID3v2 "USLT" frame.
type UnsynchronizedTextFrame struct {
	header      FrameHeader
	Encoding    TextEncoding
	Language    meta.Lang
	Description string
	Content     string
}

// NewUnsynchronizedTextFrame creates a new [UnsynchronizedTextFrame].
func NewUnsynchronizedTextFrame(encoding TextEncoding, language meta.Lang, description, content string) *UnsynchronizedTextFrame {
	return &UnsynchronizedTextFrame{
		header:      NewFrameHeader(FrameIdValid("USLT"), FrameFlags{}),
		Encoding:    encoding,
		Language:    language,
		Description: description,
		Content:     content,
	}
}

func (f *UnsynchronizedTextFrame) ID() FrameId            { return f.header.ID }
func (f *UnsynchronizedTextFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *UnsynchronizedTextFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *UnsynchronizedTextFrame) frameEqual(other Frame) bool {
	o, ok := other.(*UnsynchronizedTextFrame)
	return ok && f.Language == o.Language && f.Description == o.Description
}

func (f *UnsynchronizedTextFrame) frameIsEmpty() (bool, bool) { return f.Content == "", true }
func (f *UnsynchronizedTextFrame) frameName() string          { return "UnsynchronizedText" }

func (f *UnsynchronizedTextFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	return languageFrameBytes(f.Encoding, f.Language, f.Description, f.Content, ctx.writeOptions)
}

func (f *UnsynchronizedTextFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseUnsynchronizedTextFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*UnsynchronizedTextFrame, error) {
	lf, err := parseLanguageFrame(reader, version)
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("USLT"))
	}
	if lf == nil {
		return nil, nil
	}
	return &UnsynchronizedTextFrame{
		header:      NewFrameHeader(FrameIdValid("USLT"), frameFlags),
		Encoding:    lf.encoding,
		Language:    lf.language,
		Description: lf.description,
		Content:     lf.content,
	}, nil
}

// --- ExtendedTextFrame ("TXXX") ---

// ExtendedTextFrame is an ID3v2 "TXXX" frame.
type ExtendedTextFrame struct {
	header      FrameHeader
	Encoding    TextEncoding
	Description string
	Content     string
}

// NewExtendedTextFrame creates a new [ExtendedTextFrame].
func NewExtendedTextFrame(encoding TextEncoding, description, content string) *ExtendedTextFrame {
	return &ExtendedTextFrame{
		header:      NewFrameHeader(FrameIdValid("TXXX"), FrameFlags{}),
		Encoding:    encoding,
		Description: description,
		Content:     content,
	}
}

func (f *ExtendedTextFrame) ID() FrameId            { return f.header.ID }
func (f *ExtendedTextFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *ExtendedTextFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *ExtendedTextFrame) frameEqual(other Frame) bool {
	o, ok := other.(*ExtendedTextFrame)
	return ok && f.Description == o.Description
}

func (f *ExtendedTextFrame) frameIsEmpty() (bool, bool) { return f.Content == "", true }
func (f *ExtendedTextFrame) frameName() string          { return "UserText" }

func (f *ExtendedTextFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	bytes_ := []byte{uint8(encoding)}
	desc, err := encoding.Encode(f.Description, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	bytes_ = append(bytes_, desc...)
	cont, err := encoding.Encode(f.Content, false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append(bytes_, cont...), nil
}

func (f *ExtendedTextFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseExtendedTextFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*ExtendedTextFrame, error) {
	ret, err := func() (*ExtendedTextFrame, error) {
		var encByte [1]byte
		if _, err := io.ReadFull(reader, encByte[:]); err != nil {
			return nil, nil
		}
		encoding, err := verifyEncoding(encByte[0], version)
		if err != nil {
			return nil, err
		}
		descRes, err := decodeText(reader, textDecodeOpts(encoding, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}

		var frameContent string
		if encoding != util.TextEncodingUTF16 {
			res, err := decodeText(reader, textDecodeOpts(encoding, false))
			if err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			frameContent = res.Content
		} else {
			// It's possible for the description to be the only string with a BOM
			rawText, err := io.ReadAll(reader)
			if err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			if len(rawText) == 0 {
				frameContent = ""
			} else {
				// Reuse the BOM from the description as a fallback if the text
				// doesn't specify one.
				bom := descRes.BOM
				if len(rawText) >= 2 && ((rawText[0] == 0xFF && rawText[1] == 0xFE) || (rawText[0] == 0xFE && rawText[1] == 0xFF)) {
					bom = [2]byte{rawText[0], rawText[1]}
				}
				var order util.ByteOrder
				switch bom {
				case [2]byte{0xFF, 0xFE}:
					order = util.LittleEndian
				case [2]byte{0xFE, 0xFF}:
					order = util.BigEndian
				case [2]byte{0x00, 0x00}:
					return nil, frameParseError(meta.None[FrameId](), &meta.TextDecodingError{
						Encoding: util.TextEncodingUTF16,
						Message:  "UTF-16 string has an invalid byte order mark",
					})
				default:
					return nil, frameParseError(meta.None[FrameId](), &meta.TextDecodingError{
						Encoding: util.TextEncodingUTF16,
						Message:  "UTF-16 string has an invalid byte order mark",
					})
				}
				frameContent, err = util.Utf16DecodeBytes(rawText, order)
				if err != nil {
					return nil, frameParseError(meta.None[FrameId](), err)
				}
			}
		}

		return &ExtendedTextFrame{
			header:      NewFrameHeader(FrameIdValid("TXXX"), frameFlags),
			Encoding:    encoding,
			Description: descRes.Content,
			Content:     frameContent,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("TXXX"))
	}
	return ret, nil
}

// --- ExtendedUrlFrame ("WXXX") ---

// ExtendedUrlFrame is an ID3v2 "WXXX" frame.
type ExtendedUrlFrame struct {
	header      FrameHeader
	Encoding    TextEncoding
	Description string
	Content     string
}

// NewExtendedUrlFrame creates a new [ExtendedUrlFrame].
func NewExtendedUrlFrame(encoding TextEncoding, description, content string) *ExtendedUrlFrame {
	return &ExtendedUrlFrame{
		header:      NewFrameHeader(FrameIdValid("WXXX"), FrameFlags{}),
		Encoding:    encoding,
		Description: description,
		Content:     content,
	}
}

func (f *ExtendedUrlFrame) ID() FrameId            { return f.header.ID }
func (f *ExtendedUrlFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *ExtendedUrlFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *ExtendedUrlFrame) frameEqual(other Frame) bool {
	o, ok := other.(*ExtendedUrlFrame)
	return ok && f.Description == o.Description
}

func (f *ExtendedUrlFrame) frameIsEmpty() (bool, bool) { return f.Content == "", true }
func (f *ExtendedUrlFrame) frameName() string          { return "UserUrl" }

func (f *ExtendedUrlFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	bytes_ := []byte{uint8(encoding)}
	desc, err := encoding.Encode(f.Description, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	bytes_ = append(bytes_, desc...)
	cont, err := encoding.Encode(f.Content, false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append(bytes_, cont...), nil
}

func (f *ExtendedUrlFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseExtendedUrlFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*ExtendedUrlFrame, error) {
	ret, err := func() (*ExtendedUrlFrame, error) {
		var encByte [1]byte
		if _, err := io.ReadFull(reader, encByte[:]); err != nil {
			return nil, nil
		}
		encoding, err := verifyEncoding(encByte[0], version)
		if err != nil {
			return nil, err
		}
		descRes, err := decodeText(reader, textDecodeOpts(encoding, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		contentRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, false))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		return &ExtendedUrlFrame{
			header:      NewFrameHeader(FrameIdValid("WXXX"), frameFlags),
			Encoding:    encoding,
			Description: descRes.Content,
			Content:     contentRes.Content,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("WXXX"))
	}
	return ret, nil
}

// --- KeyValueFrame ("TIPL"/"TMCL"/"IPLS") ---

// KeyValueFrame is an ID3v2 key-value frame.
type KeyValueFrame struct {
	header        FrameHeader
	Encoding      TextEncoding
	KeyValuePairs [][2]string
}

// NewKeyValueFrame creates a new [KeyValueFrame].
func NewKeyValueFrame(id FrameId, encoding TextEncoding, pairs [][2]string) *KeyValueFrame {
	return &KeyValueFrame{
		header:        NewFrameHeader(id, FrameFlags{}),
		Encoding:      encoding,
		KeyValuePairs: pairs,
	}
}

func (f *KeyValueFrame) ID() FrameId            { return f.header.ID }
func (f *KeyValueFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *KeyValueFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *KeyValueFrame) frameEqual(other Frame) bool {
	o, ok := other.(*KeyValueFrame)
	if !ok {
		return false
	}
	if f.header.ID != o.header.ID || f.header.Flags != o.header.Flags || f.Encoding != o.Encoding || len(f.KeyValuePairs) != len(o.KeyValuePairs) {
		return false
	}
	for i := range f.KeyValuePairs {
		if f.KeyValuePairs[i] != o.KeyValuePairs[i] {
			return false
		}
	}
	return true
}

func (f *KeyValueFrame) frameIsEmpty() (bool, bool) { return len(f.KeyValuePairs) == 0, true }
func (f *KeyValueFrame) frameName() string          { return "KeyValue" }

func (f *KeyValueFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	content := []byte{uint8(encoding)}
	for _, kv := range f.KeyValuePairs {
		k, err := encoding.Encode(kv[0], true, ctx.writeOptions.LossyTextEncoding)
		if err != nil {
			return nil, frameEncodingError(meta.None[FrameId](), err)
		}
		content = append(content, k...)
		v, err := encoding.Encode(kv[1], true, ctx.writeOptions.LossyTextEncoding)
		if err != nil {
			return nil, frameEncodingError(meta.None[FrameId](), err)
		}
		content = append(content, v...)
	}
	return content, nil
}

func (f *KeyValueFrame) cloneFrame() Frame {
	c := *f
	c.KeyValuePairs = append([][2]string(nil), f.KeyValuePairs...)
	return &c
}

func parseKeyValueFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, version Id3v2Version) (*KeyValueFrame, error) {
	var encByte [1]byte
	if _, err := io.ReadFull(reader, encByte[:]); err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	encoding, err := verifyEncoding(encByte[0], version)
	if err != nil {
		return nil, setFrameErrID(err, id)
	}

	var values [][2]string
	opts := textDecodeOpts(encoding, true)

	// The first key/value pair may be the only string with a BOM
	firstKey, err := decodeText(reader, opts)
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	if firstKey.BytesRead == 0 {
		return nil, nil
	}
	if encoding == util.TextEncodingUTF16 {
		opts.BOM = firstKey.BOM
	}
	firstValue, err := decodeText(reader, opts)
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	values = append(values, [2]string{firstKey.Content, firstValue.Content})

	for {
		key, err := decodeText(reader, opts)
		if err != nil {
			return nil, frameParseError(meta.Some(id), err)
		}
		value, err := decodeText(reader, opts)
		if err != nil {
			return nil, frameParseError(meta.Some(id), err)
		}
		if key.BytesRead == 0 || value.BytesRead == 0 {
			break
		}
		values = append(values, [2]string{key.Content, value.Content})
	}

	return &KeyValueFrame{
		header:        NewFrameHeader(id, frameFlags),
		Encoding:      encoding,
		KeyValuePairs: values,
	}, nil
}

// --- PopularimeterFrame ("POPM") ---

// PopularimeterFrame is an ID3v2 "POPM" frame.
type PopularimeterFrame struct {
	header  FrameHeader
	Email   string
	Rating  uint8
	Counter uint64
}

// NewPopularimeterFrame creates a new [PopularimeterFrame].
func NewPopularimeterFrame(email string, rating uint8, counter uint64) *PopularimeterFrame {
	return &PopularimeterFrame{
		header:  NewFrameHeader(FrameIdValid("POPM"), FrameFlags{}),
		Email:   email,
		Rating:  rating,
		Counter: counter,
	}
}

func (f *PopularimeterFrame) ID() FrameId            { return f.header.ID }
func (f *PopularimeterFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *PopularimeterFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *PopularimeterFrame) frameEqual(other Frame) bool {
	o, ok := other.(*PopularimeterFrame)
	return ok && f.Email == o.Email
}

func (f *PopularimeterFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *PopularimeterFrame) frameName() string          { return "Popularimeter" }

func (f *PopularimeterFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	content, err := util.TextEncodingLatin1.Encode(f.Email, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	content = append(content, f.Rating)
	if f.Counter <= 0xFFFFFFFF {
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], uint32(f.Counter))
		content = append(content, cb[:]...)
	} else {
		var cb [8]byte
		binary.BigEndian.PutUint64(cb[:], f.Counter)
		i := 0
		for i < 8 && cb[i] == 0 {
			i++
		}
		content = append(content, cb[i:]...)
	}
	return content, nil
}

func (f *PopularimeterFrame) cloneFrame() Frame {
	c := *f
	return &c
}

// PopularimeterFrameFromPopularimeter creates a [PopularimeterFrame] from a generic [meta.Popularimeter].
func PopularimeterFrameFromPopularimeter(p meta.Popularimeter) *PopularimeterFrame {
	return &PopularimeterFrame{
		header:  NewFrameHeader(FrameIdValid("POPM"), FrameFlags{}),
		Email:   p.Email(),
		Rating:  p.MappedValue(meta.TagTypeID3v2),
		Counter: p.PlayCounter,
	}
}

func parsePopularimeterFrame(reader io.Reader, frameFlags FrameFlags) (*PopularimeterFrame, error) {
	ret, err := func() (*PopularimeterFrame, error) {
		email, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		var ratingByte [1]byte
		if _, err := io.ReadFull(reader, ratingByte[:]); err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		counterContent, err := io.ReadAll(reader)
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		var counter uint64
		if len(counterContent) > 8 {
			counter = ^uint64(0)
		} else {
			var cb [8]byte
			copy(cb[8-len(counterContent):], counterContent)
			counter = binary.BigEndian.Uint64(cb[:])
		}
		return &PopularimeterFrame{
			header:  NewFrameHeader(FrameIdValid("POPM"), frameFlags),
			Email:   email.Content,
			Rating:  ratingByte[0],
			Counter: counter,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("POPM"))
	}
	return ret, nil
}

// --- UniqueFileIdentifierFrame ("UFID") ---

// UniqueFileIdentifierFrame is an ID3v2 "UFID" frame.
type UniqueFileIdentifierFrame struct {
	header     FrameHeader
	Owner      string
	Identifier []byte
}

// NewUniqueFileIdentifierFrame creates a new [UniqueFileIdentifierFrame].
func NewUniqueFileIdentifierFrame(owner string, identifier []byte) *UniqueFileIdentifierFrame {
	return &UniqueFileIdentifierFrame{
		header:     NewFrameHeader(FrameIdValid("UFID"), FrameFlags{}),
		Owner:      owner,
		Identifier: identifier,
	}
}

func (f *UniqueFileIdentifierFrame) ID() FrameId            { return f.header.ID }
func (f *UniqueFileIdentifierFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *UniqueFileIdentifierFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *UniqueFileIdentifierFrame) frameEqual(other Frame) bool {
	o, ok := other.(*UniqueFileIdentifierFrame)
	return ok && f.Owner == o.Owner
}

func (f *UniqueFileIdentifierFrame) frameIsEmpty() (bool, bool) {
	return len(f.Identifier) == 0, true
}
func (f *UniqueFileIdentifierFrame) frameName() string { return "UniqueFileIdentifier" }

func (f *UniqueFileIdentifierFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	content, err := util.TextEncodingLatin1.Encode(f.Owner, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append(content, f.Identifier...), nil
}

func (f *UniqueFileIdentifierFrame) cloneFrame() Frame {
	c := *f
	c.Identifier = append([]byte(nil), f.Identifier...)
	return &c
}

func parseUniqueFileIdentifierFrame(reader io.Reader, frameFlags FrameFlags, parseMode meta.ParsingMode) (*UniqueFileIdentifierFrame, error) {
	ret, err := func() (*UniqueFileIdentifierFrame, error) {
		ownerRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		var owner string
		if valid := ownerRes.TextOrNone(); valid.IsSome() {
			owner, _ = valid.Get()
		} else if parseMode == meta.ParsingModeBestAttempt {
			owner = ""
		} else {
			return nil, frameParseErrorMsg(meta.None[FrameId](), "empty owner field")
		}
		identifier, err := io.ReadAll(reader)
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		return &UniqueFileIdentifierFrame{
			header:     NewFrameHeader(FrameIdValid("UFID"), frameFlags),
			Owner:      owner,
			Identifier: identifier,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("UFID"))
	}
	return ret, nil
}

// --- OwnershipFrame ("OWNE") ---

// OwnershipFrame is an ID3v2 "OWNE" frame.
type OwnershipFrame struct {
	header         FrameHeader
	Encoding       TextEncoding
	PricePaid      string
	DateOfPurchase string
	Seller         string
}

// NewOwnershipFrame creates a new [OwnershipFrame].
func NewOwnershipFrame(encoding TextEncoding, pricePaid, dateOfPurchase, seller string) *OwnershipFrame {
	return &OwnershipFrame{
		header:         NewFrameHeader(FrameIdValid("OWNE"), FrameFlags{}),
		Encoding:       encoding,
		PricePaid:      pricePaid,
		DateOfPurchase: dateOfPurchase,
		Seller:         seller,
	}
}

func (f *OwnershipFrame) ID() FrameId            { return f.header.ID }
func (f *OwnershipFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *OwnershipFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *OwnershipFrame) frameEqual(other Frame) bool {
	o, ok := other.(*OwnershipFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Encoding == o.Encoding && f.PricePaid == o.PricePaid &&
		f.DateOfPurchase == o.DateOfPurchase && f.Seller == o.Seller
}

func (f *OwnershipFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *OwnershipFrame) frameName() string          { return "Ownership" }

func (f *OwnershipFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	bytes_ := []byte{uint8(encoding)}
	price, err := util.TextEncodingLatin1.Encode(f.PricePaid, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	bytes_ = append(bytes_, price...)
	if len(f.DateOfPurchase) < 8 {
		return nil, frameEncodingErrorMsg(meta.Some(FrameIdValid("OWNE")),
			"`date_of_purchase` field is too short (must be 8 characters)")
	}
	bytes_ = append(bytes_, f.DateOfPurchase[:8]...)
	seller, err := encoding.Encode(f.Seller, false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append(bytes_, seller...), nil
}

func (f *OwnershipFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseOwnershipFrame(reader io.Reader, frameFlags FrameFlags) (*OwnershipFrame, error) {
	ret, err := func() (*OwnershipFrame, error) {
		var encByte [1]byte
		if _, err := io.ReadFull(reader, encByte[:]); err != nil {
			return nil, nil
		}
		if encByte[0] > 3 {
			return nil, frameParseError(meta.None[FrameId](), &meta.BadTextEncodingError{})
		}
		encoding := TextEncoding(encByte[0])
		priceRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		var dateBytes [8]byte
		if _, err := io.ReadFull(reader, dateBytes[:]); err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		dateStr, err := util.Utf8DecodeStr(dateBytes[:])
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		sellerRes, err := decodeText(reader, textDecodeOpts(encoding, false))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		return &OwnershipFrame{
			header:         NewFrameHeader(FrameIdValid("OWNE"), frameFlags),
			Encoding:       encoding,
			PricePaid:      priceRes.Content,
			DateOfPurchase: dateStr,
			Seller:         sellerRes.Content,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("OWNE"))
	}
	return ret, nil
}

// --- TimestampFrame ("TDEN"/"TDOR"/"TDRC"/"TDRL"/"TDTG") ---

// TimestampFrame is an ID3v2 timestamp frame.
type TimestampFrame struct {
	header    FrameHeader
	Encoding  TextEncoding
	Timestamp meta.Timestamp
}

// NewTimestampFrame creates a new [TimestampFrame].
func NewTimestampFrame(id FrameId, encoding TextEncoding, timestamp meta.Timestamp) *TimestampFrame {
	return &TimestampFrame{
		header:    NewFrameHeader(id, FrameFlags{}),
		Encoding:  encoding,
		Timestamp: timestamp,
	}
}

func (f *TimestampFrame) ID() FrameId            { return f.header.ID }
func (f *TimestampFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *TimestampFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *TimestampFrame) frameEqual(other Frame) bool {
	o, ok := other.(*TimestampFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Encoding == o.Encoding && f.Timestamp == o.Timestamp
}

func (f *TimestampFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *TimestampFrame) frameName() string          { return "Timestamp" }

func (f *TimestampFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	if err := f.Timestamp.Verify(); err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	encoded, err := encoding.Encode(f.Timestamp.String(), false, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append([]byte{uint8(encoding)}, encoded...), nil
}

func (f *TimestampFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseTimestampFrame(reader io.Reader, id FrameId, frameFlags FrameFlags, parseMode meta.ParsingMode) (*TimestampFrame, error) {
	var encByte [1]byte
	if _, err := io.ReadFull(reader, encByte[:]); err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	if encByte[0] > 3 {
		if parseMode != meta.ParsingModeRelaxed {
			return nil, frameParseError(meta.Some(id), &meta.BadTextEncodingError{})
		}
		return nil, nil
	}
	encoding := TextEncoding(encByte[0])
	res, err := decodeText(reader, textDecodeOpts(encoding, false))
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	tsOpt, err := util.TimestampParse(bytes.NewReader([]byte(res.Content)), parseMode)
	if err != nil {
		if parseMode != meta.ParsingModeRelaxed {
			return nil, frameParseError(meta.Some(id), err)
		}
		return nil, nil
	}
	if tsOpt.IsNone() {
		return nil, nil
	}
	ts, _ := tsOpt.Get()
	return &TimestampFrame{
		header:    NewFrameHeader(id, frameFlags),
		Encoding:  encoding,
		Timestamp: ts,
	}, nil
}

// --- EventTimingCodesFrame ("ETCO") ---

// EventType is the type of events that can occur in an [EventTimingCodesFrame].
type EventType uint8

const (
	EventTypePadding                EventType = 0x00
	EventTypeEndOfInitialSilence    EventType = 0x01
	EventTypeIntroStart             EventType = 0x02
	EventTypeMainPartStart          EventType = 0x03
	EventTypeOutroStart             EventType = 0x04
	EventTypeOutroEnd               EventType = 0x05
	EventTypeVerseStart             EventType = 0x06
	EventTypeRefrainStart           EventType = 0x07
	EventTypeInterludeStart         EventType = 0x08
	EventTypeThemeStart             EventType = 0x09
	EventTypeVariationStart         EventType = 0x0A
	EventTypeKeyChange              EventType = 0x0B
	EventTypeTimeChange             EventType = 0x0C
	EventTypeMomentaryUnwantedNoise EventType = 0x0D
	EventTypeSustainedNoise         EventType = 0x0E
	EventTypeSustainedNoiseEnd      EventType = 0x0F
	EventTypeIntroEnd               EventType = 0x10
	EventTypeMainPartEnd            EventType = 0x11
	EventTypeVerseEnd               EventType = 0x12
	EventTypeRefrainEnd             EventType = 0x13
	EventTypeThemeEnd               EventType = 0x14
	EventTypeProfanity              EventType = 0x15
	EventTypeProfanityEnd           EventType = 0x16
	EventTypeNotPredefinedSynch0    EventType = 0xE0
	EventTypeNotPredefinedSynch1    EventType = 0xE1
	EventTypeNotPredefinedSynch2    EventType = 0xE2
	EventTypeNotPredefinedSynch3    EventType = 0xE3
	EventTypeNotPredefinedSynch4    EventType = 0xE4
	EventTypeNotPredefinedSynch5    EventType = 0xE5
	EventTypeNotPredefinedSynch6    EventType = 0xE6
	EventTypeNotPredefinedSynch7    EventType = 0xE7
	EventTypeNotPredefinedSynch8    EventType = 0xE8
	EventTypeNotPredefinedSynch9    EventType = 0xE9
	EventTypeNotPredefinedSynchA    EventType = 0xEA
	EventTypeNotPredefinedSynchB    EventType = 0xEB
	EventTypeNotPredefinedSynchC    EventType = 0xEC
	EventTypeNotPredefinedSynchD    EventType = 0xED
	EventTypeNotPredefinedSynchE    EventType = 0xEE
	EventTypeNotPredefinedSynchF    EventType = 0xEF
	EventTypeAudioEnd               EventType = 0xFD
	EventTypeAudioFileEnds          EventType = 0xFE
	EventTypeReserved               EventType = 0xFF
)

// EventTypeFromU8 converts a byte to an [EventType]; undefined ranges map to
// [EventTypeReserved].
func EventTypeFromU8(b uint8) EventType {
	switch b {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16,
		0xE0, 0xE1, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9,
		0xEA, 0xEB, 0xEC, 0xED, 0xEE, 0xEF, 0xFD, 0xFE:
		return EventType(b)
	default:
		return EventTypeReserved
	}
}

// Event is an event for an [EventTimingCodesFrame].
type Event struct {
	EventType EventType
	Timestamp uint32
}

// EventTimingCodesFrame is an ID3v2 "ETCO" frame.
type EventTimingCodesFrame struct {
	header          FrameHeader
	TimestampFormat TimestampFormat
	Events          []Event
}

// NewEventTimingCodesFrame creates a new [EventTimingCodesFrame].
func NewEventTimingCodesFrame(format TimestampFormat, events []Event) *EventTimingCodesFrame {
	return &EventTimingCodesFrame{
		header:          NewFrameHeader(FrameIdValid("ETCO"), FrameFlags{}),
		TimestampFormat: format,
		Events:          events,
	}
}

func (f *EventTimingCodesFrame) ID() FrameId            { return f.header.ID }
func (f *EventTimingCodesFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *EventTimingCodesFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *EventTimingCodesFrame) frameEqual(other Frame) bool {
	o, ok := other.(*EventTimingCodesFrame)
	if !ok {
		return false
	}
	if f.header.ID != o.header.ID || f.header.Flags != o.header.Flags ||
		f.TimestampFormat != o.TimestampFormat || len(f.Events) != len(o.Events) {
		return false
	}
	for i := range f.Events {
		if f.Events[i] != o.Events[i] {
			return false
		}
	}
	return true
}

func (f *EventTimingCodesFrame) frameIsEmpty() (bool, bool) { return len(f.Events) == 0, true }
func (f *EventTimingCodesFrame) frameName() string          { return "EventTimingCodes" }

func (f *EventTimingCodesFrame) frameBytes(_ *frameEncodingContext) ([]byte, error) {
	content := []byte{uint8(f.TimestampFormat)}
	sorted := append([]Event(nil), f.Events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	for _, ev := range sorted {
		content = append(content, uint8(ev.EventType))
		var tb [4]byte
		binary.BigEndian.PutUint32(tb[:], ev.Timestamp)
		content = append(content, tb[:]...)
	}
	return content, nil
}

func (f *EventTimingCodesFrame) cloneFrame() Frame {
	c := *f
	c.Events = append([]Event(nil), f.Events...)
	return &c
}

func parseEventTimingCodesFrame(reader io.Reader, frameFlags FrameFlags) (*EventTimingCodesFrame, error) {
	ret, err := func() (*EventTimingCodesFrame, error) {
		var fmtByte [1]byte
		if _, err := io.ReadFull(reader, fmtByte[:]); err != nil {
			return nil, nil
		}
		tf, err := TimestampFormatFromU8(fmtByte[0])
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		var events []Event
		for {
			var evByte [5]byte
			if _, err := io.ReadFull(reader, evByte[:1]); err != nil {
				break
			}
			if _, err := io.ReadFull(reader, evByte[1:5]); err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			events = append(events, Event{
				EventType: EventTypeFromU8(evByte[0]),
				Timestamp: binary.BigEndian.Uint32(evByte[1:5]),
			})
		}
		sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp < events[j].Timestamp })
		return &EventTimingCodesFrame{
			header:          NewFrameHeader(FrameIdValid("ETCO"), frameFlags),
			TimestampFormat: tf,
			Events:          events,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("ETCO"))
	}
	return ret, nil
}

// --- PrivateFrame ("PRIV") ---

// PrivateFrame is an ID3v2 "PRIV" frame.
type PrivateFrame struct {
	header      FrameHeader
	Owner       string
	PrivateData []byte
}

// NewPrivateFrame creates a new [PrivateFrame].
func NewPrivateFrame(owner string, data []byte) *PrivateFrame {
	return &PrivateFrame{
		header:      NewFrameHeader(FrameIdValid("PRIV"), FrameFlags{}),
		Owner:       owner,
		PrivateData: data,
	}
}

func (f *PrivateFrame) ID() FrameId            { return f.header.ID }
func (f *PrivateFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *PrivateFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *PrivateFrame) frameEqual(other Frame) bool {
	o, ok := other.(*PrivateFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Owner == o.Owner && bytes.Equal(f.PrivateData, o.PrivateData)
}

func (f *PrivateFrame) frameIsEmpty() (bool, bool) { return len(f.PrivateData) == 0, true }
func (f *PrivateFrame) frameName() string          { return "Private" }

func (f *PrivateFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	content, err := util.TextEncodingLatin1.Encode(f.Owner, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	return append(content, f.PrivateData...), nil
}

func (f *PrivateFrame) cloneFrame() Frame {
	c := *f
	c.PrivateData = append([]byte(nil), f.PrivateData...)
	return &c
}

func parsePrivateFrame(reader io.Reader, frameFlags FrameFlags) (*PrivateFrame, error) {
	ret, err := func() (*PrivateFrame, error) {
		ownerRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, nil
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		return &PrivateFrame{
			header:      NewFrameHeader(FrameIdValid("PRIV"), frameFlags),
			Owner:       ownerRes.Content,
			PrivateData: data,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("PRIV"))
	}
	return ret, nil
}

// --- RelativeVolumeAdjustmentFrame ("RVA2") ---

// ChannelType is a channel identifier used in the RVA2 frame.
type ChannelType uint8

const (
	ChannelTypeOther        ChannelType = 0
	ChannelTypeMasterVolume ChannelType = 1
	ChannelTypeFrontRight   ChannelType = 2
	ChannelTypeFrontLeft    ChannelType = 3
	ChannelTypeBackRight    ChannelType = 4
	ChannelTypeBackLeft     ChannelType = 5
	ChannelTypeFrontCentre  ChannelType = 6
	ChannelTypeBackCentre   ChannelType = 7
	ChannelTypeSubwoofer    ChannelType = 8
)

// ChannelTypeFromU8 converts a byte to a [ChannelType].
func ChannelTypeFromU8(b uint8) (ChannelType, error) {
	if b <= 8 {
		return ChannelType(b), nil
	}
	return 0, &BadRva2ChannelTypeError{}
}

// ChannelInformation is volume adjustment information for a specific channel.
type ChannelInformation struct {
	ChannelType          ChannelType
	VolumeAdjustment     int16
	BitsRepresentingPeak uint8
	PeakVolume           meta.Option[[]byte]
}

// RelativeVolumeAdjustmentFrame is an ID3v2 "RVA2" frame.
type RelativeVolumeAdjustmentFrame struct {
	header         FrameHeader
	Identification string
	Channels       map[ChannelType]ChannelInformation
}

// NewRelativeVolumeAdjustmentFrame creates a new [RelativeVolumeAdjustmentFrame].
func NewRelativeVolumeAdjustmentFrame(identification string, channels map[ChannelType]ChannelInformation) *RelativeVolumeAdjustmentFrame {
	return &RelativeVolumeAdjustmentFrame{
		header:         NewFrameHeader(FrameIdValid("RVA2"), FrameFlags{}),
		Identification: identification,
		Channels:       channels,
	}
}

func (f *RelativeVolumeAdjustmentFrame) ID() FrameId            { return f.header.ID }
func (f *RelativeVolumeAdjustmentFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *RelativeVolumeAdjustmentFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *RelativeVolumeAdjustmentFrame) frameEqual(other Frame) bool {
	o, ok := other.(*RelativeVolumeAdjustmentFrame)
	return ok && f.Identification == o.Identification
}

func (f *RelativeVolumeAdjustmentFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *RelativeVolumeAdjustmentFrame) frameName() string          { return "RelativeVolumeAdjustment" }

func (f *RelativeVolumeAdjustmentFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	content, err := util.TextEncodingLatin1.Encode(f.Identification, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	for channelType, info := range f.Channels {
		bitsRepresentingPeak := info.BitsRepresentingPeak
		expectedPeakByteLength := (int(bitsRepresentingPeak) + 7) >> 3
		content = append(content, uint8(channelType))
		var vb [2]byte
		binary.BigEndian.PutUint16(vb[:], uint16(info.VolumeAdjustment))
		content = append(content, vb[:]...)
		if info.PeakVolume.IsNone() {
			content = append(content, 0)
			continue
		}
		peak, _ := info.PeakVolume.Get()
		if len(peak) > expectedPeakByteLength {
			bitsRepresentingPeak = 0
			for i := 0; i < len(peak) && i < 32; i++ {
				bitsRepresentingPeak += uint8(bitsLeadingOnes(peak[i]))
			}
		}
		content = append(content, bitsRepresentingPeak)
		n := len(peak)
		if n > 32 {
			n = 32
		}
		content = append(content, peak[:n]...)
	}
	return content, nil
}

func bitsLeadingOnes(b byte) int {
	n := 0
	for b&0x80 != 0 {
		n++
		b <<= 1
	}
	return n
}

func (f *RelativeVolumeAdjustmentFrame) cloneFrame() Frame {
	c := *f
	c.Channels = make(map[ChannelType]ChannelInformation, len(f.Channels))
	for k, v := range f.Channels {
		c.Channels[k] = v
	}
	return &c
}

func parseRelativeVolumeAdjustmentFrame(reader io.Reader, frameFlags FrameFlags, parseMode meta.ParsingMode) (*RelativeVolumeAdjustmentFrame, error) {
	ret, err := func() (*RelativeVolumeAdjustmentFrame, error) {
		identRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		channels := make(map[ChannelType]ChannelInformation)
		for {
			var ctByte [1]byte
			if _, err := io.ReadFull(reader, ctByte[:]); err != nil {
				break
			}
			channelType, err := ChannelTypeFromU8(ctByte[0])
			if err != nil {
				if parseMode == meta.ParsingModeBestAttempt {
					channelType = ChannelTypeOther
				} else {
					return nil, frameParseError(meta.None[FrameId](), err)
				}
			}
			var vaBytes [2]byte
			if _, err := io.ReadFull(reader, vaBytes[:]); err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			volumeAdjustment := int16(binary.BigEndian.Uint16(vaBytes[:]))
			var peakBits [1]byte
			if _, err := io.ReadFull(reader, peakBits[:]); err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			bitsRepresentingPeak := peakBits[0]
			var peakVolume meta.Option[[]byte]
			if bitsRepresentingPeak > 0 {
				peakBytes := (int(bitsRepresentingPeak) + 7) >> 3
				pv, err := util.TryByteSlice(peakBytes)
				if err != nil {
					return nil, frameParseError(meta.None[FrameId](), err)
				}
				if _, err := io.ReadFull(reader, pv); err != nil {
					return nil, frameParseError(meta.None[FrameId](), err)
				}
				peakVolume = meta.Some(pv)
			}
			channels[channelType] = ChannelInformation{
				ChannelType:          channelType,
				VolumeAdjustment:     volumeAdjustment,
				BitsRepresentingPeak: bitsRepresentingPeak,
				PeakVolume:           peakVolume,
			}
		}
		return &RelativeVolumeAdjustmentFrame{
			header:         NewFrameHeader(FrameIdValid("RVA2"), frameFlags),
			Identification: identRes.Content,
			Channels:       channels,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("RVA2"))
	}
	return ret, nil
}

// --- BinaryFrame (unknown/outdated frames) ---

// BinaryFrame is a binary fallback for unknown ID3v2 frames.
type BinaryFrame struct {
	header FrameHeader
	Data   []byte
}

// NewBinaryFrame creates a new [BinaryFrame].
func NewBinaryFrame(id FrameId, data []byte) *BinaryFrame {
	return &BinaryFrame{header: NewFrameHeader(id, FrameFlags{}), Data: data}
}

func (f *BinaryFrame) ID() FrameId            { return f.header.ID }
func (f *BinaryFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *BinaryFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *BinaryFrame) frameEqual(other Frame) bool {
	o, ok := other.(*BinaryFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		bytes.Equal(f.Data, o.Data)
}

func (f *BinaryFrame) frameIsEmpty() (bool, bool) { return len(f.Data) == 0, true }
func (f *BinaryFrame) frameName() string          { return "Binary" }

func (f *BinaryFrame) frameBytes(_ *frameEncodingContext) ([]byte, error) {
	return append([]byte(nil), f.Data...), nil
}

func (f *BinaryFrame) cloneFrame() Frame {
	c := *f
	c.Data = append([]byte(nil), f.Data...)
	return &c
}

// AsBytes returns the frame's binary content.
func (f *BinaryFrame) AsBytes() []byte {
	return append([]byte(nil), f.Data...)
}

func parseBinaryFrame(reader io.Reader, id FrameId, frameFlags FrameFlags) (*BinaryFrame, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, frameParseError(meta.Some(id), err)
	}
	return &BinaryFrame{header: NewFrameHeader(id, frameFlags), Data: data}, nil
}

// --- AttachedPictureFrame ("APIC") ---

// AttachedPictureFrame is an ID3v2 "APIC" frame.
type AttachedPictureFrame struct {
	header   FrameHeader
	Encoding TextEncoding
	Picture  meta.Picture
}

// NewAttachedPictureFrame creates a new [AttachedPictureFrame].
func NewAttachedPictureFrame(encoding TextEncoding, picture meta.Picture) *AttachedPictureFrame {
	return &AttachedPictureFrame{
		header:   NewFrameHeader(FrameIdValid("APIC"), FrameFlags{}),
		Encoding: encoding,
		Picture:  picture,
	}
}

func (f *AttachedPictureFrame) ID() FrameId            { return f.header.ID }
func (f *AttachedPictureFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *AttachedPictureFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *AttachedPictureFrame) frameEqual(other Frame) bool {
	o, ok := other.(*AttachedPictureFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Encoding == o.Encoding && picturesEqual(f.Picture, o.Picture)
}

func picturesEqual(a, b meta.Picture) bool {
	if a.PicType() != b.PicType() {
		return false
	}
	am, aok := a.MimeType().Get()
	bm, bok := b.MimeType().Get()
	if aok != bok || (aok && am != bm) {
		return false
	}
	ad, _ := a.Description()
	bd, _ := b.Description()
	return ad == bd && bytes.Equal(a.Data(), b.Data())
}

func (f *AttachedPictureFrame) frameIsEmpty() (bool, bool) { return len(f.Picture.Data()) == 0, true }
func (f *AttachedPictureFrame) frameName() string          { return "Picture" }

func (f *AttachedPictureFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	encoding := f.Encoding
	if ctx.writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	data := []byte{uint8(encoding)}
	if mt, ok := f.Picture.MimeType().Get(); ok {
		data = append(data, mt.AsStr()...)
	}
	data = append(data, 0)
	data = append(data, f.Picture.PicType().AsU8())
	desc, _ := f.Picture.Description()
	descBytes, err := encoding.Encode(desc, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(meta.None[FrameId](), err)
	}
	data = append(data, descBytes...)
	data = append(data, f.Picture.Data()...)
	if uint64(len(data)) > uint64(^uint32(0)) {
		return nil, frameEncodingError(meta.None[FrameId](), &meta.TooMuchDataError{})
	}
	return data, nil
}

func (f *AttachedPictureFrame) cloneFrame() Frame {
	c := *f
	return &c
}

func parseAttachedPictureFrame(reader io.Reader, frameFlags FrameFlags, version Id3v2Version) (*AttachedPictureFrame, error) {
	ret, err := func() (*AttachedPictureFrame, error) {
		var encByte [1]byte
		if _, err := io.ReadFull(reader, encByte[:]); err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		if encByte[0] > 3 {
			return nil, frameParseError(meta.None[FrameId](), &meta.BadTextEncodingError{})
		}
		encoding := TextEncoding(encByte[0])

		var mimeType *meta.MimeType
		if version == id3v2Version2 {
			var format [3]byte
			if _, err := io.ReadFull(reader, format[:]); err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			switch string(format[:]) {
			case "PNG":
				m := meta.MimeTypePng
				mimeType = &m
			case "JPG":
				m := meta.MimeTypeJpeg
				mimeType = &m
			default:
				return nil, frameParseErrorMsg(meta.Some(FrameIdValid("APIC")),
					"unexpected picture format \""+string(format[:])+"\"")
			}
		} else {
			mimeRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
			if err != nil {
				return nil, frameParseError(meta.None[FrameId](), err)
			}
			if ms := mimeRes.TextOrNone(); ms.IsSome() {
				s, _ := ms.Get()
				m := meta.MimeTypeFromStr(s)
				mimeType = &m
			}
		}

		var typeByte [1]byte
		if _, err := io.ReadFull(reader, typeByte[:]); err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}
		picType := meta.PictureType(typeByte[0])

		descRes, err := decodeText(reader, textDecodeOpts(encoding, true))
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, frameParseError(meta.None[FrameId](), err)
		}

		pb := meta.PictureUnchecked(data).PicType(picType)
		if mimeType != nil {
			pb = pb.MimeType(*mimeType)
		}
		if ds := descRes.TextOrNone(); ds.IsSome() {
			d, _ := ds.Get()
			pb = pb.Description(d)
		}

		return &AttachedPictureFrame{
			header:   NewFrameHeader(FrameIdValid("APIC"), frameFlags),
			Encoding: encoding,
			Picture:  pb.Build(),
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("APIC"))
	}
	return ret, nil
}

// setFrameErrID overwrites the frame ID on a parse/encoding error if unset.
func setFrameErrID(err error, id FrameId) error {
	switch e := err.(type) {
	case *FrameParseError:
		if e.ID.IsNone() {
			e.ID = meta.Some(id)
		}
	case *FrameEncodingError:
		if e.ID.IsNone() {
			e.ID = meta.Some(id)
		}
	}
	return err
}
