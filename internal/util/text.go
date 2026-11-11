package util

import (
	"fmt"
	"io"
	"unicode/utf16"
	"unicode/utf8"
)

// TextEncodingError is returned when encoding text fails.
type TextEncodingError struct {
	// Encoding is the target text encoding.
	Encoding TextEncoding
	// ValidUpTo is the byte index in the provided string up to which the encoding was valid.
	ValidUpTo int
}

func (e *TextEncodingError) Error() string {
	return fmt.Sprintf("invalid %s sequence from index %d", e.Encoding, e.ValidUpTo)
}

// TextDecodingError is returned when decoding text fails.
type TextDecodingError struct {
	// Encoding is the target text encoding.
	Encoding TextEncoding
	// ValidUpTo is the byte index in the provided string up to which the input was valid, if available.
	ValidUpTo Option[int]
	// Message is an optional descriptive message.
	Message string
	// Source is the underlying error, if any.
	Source error
}

func (e *TextDecodingError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("failed to decode %s sequence: %s", e.Encoding, e.Message)
	}
	return fmt.Sprintf("failed to decode %s sequence", e.Encoding)
}

func (e *TextDecodingError) Unwrap() error { return e.Source }

func utf16BadBOMError() *TextDecodingError {
	return &TextDecodingError{
		Encoding:  TextEncodingUTF16,
		ValidUpTo: Some(0),
		Message:   "UTF-16 string has an invalid byte order mark",
	}
}

func utf16BadLengthError() *TextDecodingError {
	return &TextDecodingError{
		Encoding:  TextEncodingUTF16,
		ValidUpTo: Some(0),
		Message:   "UTF-16 string has an invalid length (< 2)",
	}
}

func utf16OddLengthError() *TextDecodingError {
	return &TextDecodingError{
		Encoding:  TextEncodingUTF16,
		ValidUpTo: Some(0),
		Message:   "UTF-16 string has an odd length",
	}
}

// BadTextEncodingError is returned when an invalid text encoding is encountered in an ID3v2 frame.
//
// This is **NOT** the same as [TextDecodingError], which describes errors within the
// actual text. This simply means that an ID3v2 frame specifies a text encoding that does
// not exist.
type BadTextEncodingError struct{}

func (e *BadTextEncodingError) Error() string {
	return "ID3v2 frame specifies invalid text encoding"
}

// TextEncoding is the text encoding for use in ID3v2 frames.
type TextEncoding uint8

const (
	// TextEncodingLatin1 is ISO-8859-1.
	TextEncodingLatin1 TextEncoding = 0
	// TextEncodingUTF16 is UTF-16 with a byte order mark.
	TextEncodingUTF16 TextEncoding = 1
	// TextEncodingUTF16BE is UTF-16 big endian.
	TextEncodingUTF16BE TextEncoding = 2
	// TextEncodingUTF8 is UTF-8.
	TextEncodingUTF8 TextEncoding = 3
)

func (e TextEncoding) String() string {
	switch e {
	case TextEncodingLatin1:
		return "Latin-1"
	case TextEncodingUTF16:
		return "UTF-16"
	case TextEncodingUTF16BE:
		return "UTF-16 BE"
	case TextEncodingUTF8:
		return "UTF-8"
	}
	return "unknown"
}

// ID3v2.4 introduced two new text encodings.
//
// When writing ID3v2.3, we just substitute with UTF-16.
func (e TextEncoding) ToID3v23() TextEncoding {
	switch e {
	case TextEncodingUTF8, TextEncodingUTF16BE:
		return TextEncodingUTF16
	default:
		return e
	}
}

// VerifyLatin1 reports whether every character in text is representable in Latin-1.
func VerifyLatin1(text string) bool {
	for _, c := range text {
		if c > 255 {
			return false
		}
	}
	return true
}

// Encode encodes `text` in this encoding. If `terminated`, a null terminator is appended.
// If `lossy`, unrepresentable characters are replaced with '?'; otherwise a
// [TextEncodingError] is returned.
func (e TextEncoding) Encode(text string, terminated, lossy bool) ([]byte, error) {
	switch e {
	case TextEncodingLatin1:
		out := make([]byte, 0, len(text))
		i := 0 // Rust reports `valid_up_to` as a character index
		for _, c := range text {
			if c <= 255 {
				out = append(out, byte(c))
			} else if lossy {
				out = append(out, '?')
			} else {
				return nil, &TextEncodingError{Encoding: e, ValidUpTo: i}
			}
			i++
		}
		if terminated {
			out = append(out, 0)
		}
		return out, nil
	case TextEncodingUTF16:
		return Utf16Encode(text, NativeEndian(), true, terminated), nil
	case TextEncodingUTF16BE:
		return Utf16Encode(text, BigEndian, false, terminated), nil
	case TextEncodingUTF8:
		out := []byte(text)
		if terminated {
			out = append(out, 0)
		}
		return out, nil
	}
	return nil, &BadTextEncodingError{}
}

type ByteOrder int

const (
	LittleEndian ByteOrder = iota
	BigEndian
)

// host byte order — on effectively all supported Go platforms this is little-endian.
func NativeEndian() ByteOrder { return LittleEndian }

func Utf16Encode(text string, order ByteOrder, bom, terminated bool) []byte {
	var encoded []byte
	if bom {
		encoded = AppendUint16(encoded, 0xFEFF, order)
	}
	for _, ch := range utf16.Encode([]rune(text)) {
		encoded = AppendUint16(encoded, ch, order)
	}
	if terminated {
		encoded = append(encoded, 0, 0)
	}
	return encoded
}

func AppendUint16(b []byte, v uint16, order ByteOrder) []byte {
	if order == BigEndian {
		return append(b, byte(v>>8), byte(v))
	}
	return append(b, byte(v), byte(v>>8))
}

// DecodeTextResult is the result of decoding text.
type DecodeTextResult struct {
	Content   string
	BytesRead int
	BOM       [2]byte
}

// TextOrNone returns the content, or an empty Option if it is empty.
func (r DecodeTextResult) TextOrNone() Option[string] {
	if r.Content == "" {
		return None[string]()
	}
	return Some(r.Content)
}

// TextDecodeOptions specifies how to decode the provided text.
type TextDecodeOptions struct {
	Encoding   TextEncoding
	Terminated bool
	BOM        [2]byte
}

func DecodeText(reader io.Reader, options TextDecodeOptions) (DecodeTextResult, error) {
	var rawBytes []byte
	var bytesRead int

	if options.Terminated {
		b, termLen := ReadToTerminator(reader, options.Encoding)
		if len(b) == 0 {
			return DecodeTextResult{BytesRead: termLen}, nil
		}
		bytesRead = len(b) + termLen
		rawBytes = b
	} else {
		b, err := io.ReadAll(reader)
		if err != nil {
			return DecodeTextResult{}, &TextDecodingError{
				Encoding: options.Encoding,
				Source:   err,
			}
		}
		if len(b) == 0 {
			return DecodeTextResult{}, nil
		}
		bytesRead = len(b)
		rawBytes = b
	}

	var bom [2]byte
	var readString string
	switch options.Encoding {
	case TextEncodingLatin1:
		readString = Latin1Decode(rawBytes)
	case TextEncodingUTF16:
		if len(rawBytes) < 2 {
			return DecodeTextResult{}, utf16BadLengthError()
		}
		if len(rawBytes)%2 != 0 {
			return DecodeTextResult{}, utf16OddLengthError()
		}
		if options.BOM == [2]byte{0, 0} {
			bom = [2]byte{rawBytes[0], rawBytes[1]}
		} else {
			bom = options.BOM
		}
		var err error
		switch bom {
		case [2]byte{0xFE, 0xFF}:
			readString, err = Utf16DecodeBytes(rawBytes, BigEndian)
		case [2]byte{0xFF, 0xFE}:
			readString, err = Utf16DecodeBytes(rawBytes, LittleEndian)
		default:
			return DecodeTextResult{}, utf16BadBOMError()
		}
		if err != nil {
			return DecodeTextResult{}, err
		}
	case TextEncodingUTF16BE:
		var err error
		readString, err = Utf16DecodeBytes(rawBytes, BigEndian)
		if err != nil {
			return DecodeTextResult{}, err
		}
	case TextEncodingUTF8:
		var err error
		readString, err = Utf8Decode(rawBytes)
		if err != nil {
			return DecodeTextResult{}, err
		}
	}

	return DecodeTextResult{
		Content:   readString,
		BytesRead: bytesRead,
		BOM:       bom,
	}, nil
}

func ReadToTerminator(reader io.Reader, encoding TextEncoding) ([]byte, int) {
	var textBytes []byte
	terminatorLen := 0
	br, ok := reader.(io.ByteReader)
	if !ok {
		br = &ByteReaderAdapter{r: reader}
	}

	switch encoding {
	case TextEncodingLatin1, TextEncodingUTF8:
		for {
			b, err := br.ReadByte()
			if err != nil {
				break
			}
			if b == 0 {
				terminatorLen = 1
				break
			}
			textBytes = append(textBytes, b)
		}
	case TextEncodingUTF16, TextEncodingUTF16BE:
		for {
			b1, err1 := br.ReadByte()
			b2, err2 := br.ReadByte()
			if err1 != nil || err2 != nil {
				break
			}
			if b1 == 0 && b2 == 0 {
				terminatorLen = 2
				break
			}
			textBytes = append(textBytes, b1, b2)
		}
	}

	return textBytes, terminatorLen
}

// ByteReaderAdapter wraps an io.Reader as an io.ByteReader. Only suitable for
// use in contexts where the underlying reader is a bytes.Buffer-like type or
// where unread bytes are never needed.
type ByteReaderAdapter struct {
	r   io.Reader
	buf [1]byte
}

func (a *ByteReaderAdapter) ReadByte() (byte, error) {
	n, err := a.r.Read(a.buf[:])
	if n == 0 {
		if err == nil {
			return 0, io.EOF
		}
		return 0, err
	}
	return a.buf[0], nil
}

func Latin1Decode(bytes []byte) string {
	runes := make([]rune, len(bytes))
	for i, b := range bytes {
		runes[i] = rune(b)
	}
	return TrimEndNulls(string(runes))
}

func Latin1Encode(s string, lossy bool) ([]byte, error) {
	out := make([]byte, 0, len(s))
	// Rust reports `valid_up_to` as a character index, not a byte index
	i := 0
	for _, c := range s {
		if c <= 255 {
			out = append(out, byte(c))
		} else if lossy {
			out = append(out, '?')
		} else {
			return nil, &TextEncodingError{Encoding: TextEncodingLatin1, ValidUpTo: i}
		}
		i++
	}
	return out, nil
}

func Utf8Decode(b []byte) (string, error) {
	if !utf8.Valid(b) {
		validUpTo := Utf8ErrorIndex(b)
		return "", &TextDecodingError{Encoding: TextEncodingUTF8, ValidUpTo: Some(validUpTo)}
	}
	return TrimEndNulls(string(b)), nil
}

func Utf8ErrorIndex(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			return i
		}
		i += size
	}
	return len(b)
}

func Utf8DecodeStr(b []byte) (string, error) { return Utf8Decode(b) }

func Utf16DecodeBytes(bytes []byte, order ByteOrder) (string, error) {
	if len(bytes) == 0 {
		return "", nil
	}
	words := make([]uint16, 0, len(bytes)/2)
	for i := 0; i+1 < len(bytes); i += 2 {
		// In ID3v2, it is possible to have multiple UTF-16 strings separated by null.
		// This also makes it possible for us to encounter multiple BOMs in a single string.
		// We must filter them out.
		if bytes[i] == 0xFF && bytes[i+1] == 0xFE || bytes[i] == 0xFE && bytes[i+1] == 0xFF {
			continue
		}
		var w uint16
		if order == BigEndian {
			w = uint16(bytes[i])<<8 | uint16(bytes[i+1])
		} else {
			w = uint16(bytes[i]) | uint16(bytes[i+1])<<8
		}
		words = append(words, w)
	}
	runes := utf16.Decode(words)
	if !Utf16Valid(words) {
		return "", &TextDecodingError{Encoding: TextEncodingUTF16}
	}
	return TrimEndNulls(string(runes)), nil
}

func Utf16Valid(words []uint16) bool {
	for i := 0; i < len(words); i++ {
		w := words[i]
		if w >= 0xD800 && w <= 0xDBFF {
			if i+1 >= len(words) {
				return false
			}
			w2 := words[i+1]
			if w2 < 0xDC00 || w2 > 0xDFFF {
				return false
			}
			i++
		} else if w >= 0xDC00 && w <= 0xDFFF {
			return false
		}
	}
	return true
}

// utf16DecodeTerminatedMaybeBOM reads a null-terminated UTF-16 string that may or may not
// have a BOM.
//
// This is needed for ID3v2, as some encoders will encode *only* the first string in a frame with a
// BOM, and the others are assumed to have the same byte order.
//
// This is seen in frames like SYLT, COMM, and USLT, where the description will be the only string
// with a BOM.
//
// If no BOM is present, the string will be decoded using `order`.
// Returns (string, bytesRead).
func Utf16DecodeTerminatedMaybeBom(reader io.Reader, order ByteOrder) (string, int, error) {
	rawText, terminatorLen := ReadToTerminator(reader, TextEncodingUTF16)

	bytesRead := len(rawText) + terminatorLen
	var decoded string
	var err error
	switch {
	case len(rawText) >= 2 && rawText[0] == 0xFF && rawText[1] == 0xFE:
		decoded, err = Utf16DecodeBytes(rawText[2:], LittleEndian)
	case len(rawText) >= 2 && rawText[0] == 0xFE && rawText[1] == 0xFF:
		decoded, err = Utf16DecodeBytes(rawText[2:], BigEndian)
	default:
		decoded, err = Utf16DecodeBytes(rawText, order)
	}
	if err != nil {
		return "", 0, err
	}
	return decoded, bytesRead, nil
}

func TrimEndNulls(text string) string {
	end := len(text)
	for end > 0 && text[end-1] == 0 {
		end--
	}
	return text[:end]
}

// AsciiNumber attempts to parse a u64 from an ascii digit string (e.g., "123").
//
// NOTE: This will parse the `input` until it encounters a non-ascii digit. Be sure to only provide
//
//	the digit string.
//
// Returns (number of digits, product).
func AsciiNumber(input []byte) (int, uint64) {
	numDigits := 0
	var product uint64
	for _, c := range input {
		if c < '0' || c > '9' {
			break
		}
		numDigits++
		product = product*10 + uint64(c-'0')
	}
	return numDigits, product
}
