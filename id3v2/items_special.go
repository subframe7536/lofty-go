package id3v2

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/subframe7536/lofty-go/internal/util"
)

// TimestampFormat is the unit used for [SynchronizedTextFrame] timestamps.
type TimestampFormat uint8

const (
	// TimestampFormatMPEG — the unit is MPEG frames.
	TimestampFormatMPEG TimestampFormat = 1
	// TimestampFormatMS — the unit is milliseconds.
	TimestampFormatMS TimestampFormat = 2
)

// TimestampFormatFromU8 converts a byte to a [TimestampFormat].
func TimestampFormatFromU8(b uint8) (TimestampFormat, error) {
	switch b {
	case 1:
		return TimestampFormatMPEG, nil
	case 2:
		return TimestampFormatMS, nil
	default:
		return 0, &BadTimestampFormatError{}
	}
}

// SyncTextContentType is the type of text stored in a [SynchronizedTextFrame].
type SyncTextContentType uint8

const (
	SyncTextOther         SyncTextContentType = 0
	SyncTextLyrics        SyncTextContentType = 1
	SyncTextTranscription SyncTextContentType = 2
	SyncTextPartName      SyncTextContentType = 3
	SyncTextEvents        SyncTextContentType = 4
	SyncTextChord         SyncTextContentType = 5
	SyncTextTrivia        SyncTextContentType = 6
	SyncTextWebpageURL    SyncTextContentType = 7
	SyncTextImageURL      SyncTextContentType = 8
)

// SyncTextContentTypeFromU8 converts a byte to a [SyncTextContentType].
func SyncTextContentTypeFromU8(b uint8) (SyncTextContentType, error) {
	if b <= 8 {
		return SyncTextContentType(b), nil
	}
	return 0, &BadSyncTextContentTypeError{}
}

// SynchronizedTextFrame is an ID3v2 "SYLT" frame.
//
// SYLT frames are stored as [BinaryFrame] by default; use
// [ParseSynchronizedTextFrame] to decode the binary content and
// [SynchronizedTextFrame.AsBytes] to re-encode it.
type SynchronizedTextFrame struct {
	header          FrameHeader
	Encoding        TextEncoding
	Language        [3]byte
	TimestampFormat TimestampFormat
	ContentType     SyncTextContentType
	Description     Option[string]
	content         []SyncTextItem
}

// SyncTextItem is a single (timestamp, text) pair in a [SynchronizedTextFrame].
type SyncTextItem struct {
	Timestamp uint32
	Text      string
}

// NewSynchronizedTextFrame creates a new [SynchronizedTextFrame].
func NewSynchronizedTextFrame(encoding TextEncoding, language [3]byte, format TimestampFormat, contentType SyncTextContentType, description Option[string], content []SyncTextItem) *SynchronizedTextFrame {
	return &SynchronizedTextFrame{
		header:          NewFrameHeader(FrameIdValid("SYLT"), FrameFlags{}),
		Encoding:        encoding,
		Language:        language,
		TimestampFormat: format,
		ContentType:     contentType,
		Description:     description,
		content:         content,
	}
}

func (f *SynchronizedTextFrame) ID() FrameId            { return f.header.ID }
func (f *SynchronizedTextFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *SynchronizedTextFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *SynchronizedTextFrame) frameEqual(other Frame) bool {
	o, ok := other.(*SynchronizedTextFrame)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Encoding == o.Encoding && f.Language == o.Language &&
		f.TimestampFormat == o.TimestampFormat && f.ContentType == o.ContentType &&
		f.Description == o.Description && syncTextItemsEqual(f.content, o.content)
}

func syncTextItemsEqual(a, b []SyncTextItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (f *SynchronizedTextFrame) frameIsEmpty() (bool, bool) { return len(f.content) == 0, true }
func (f *SynchronizedTextFrame) frameName() string          { return "SynchronizedText" }

// Items returns the (timestamp, text) pairs.
func (f *SynchronizedTextFrame) Items() []SyncTextItem { return f.content }

// SetItems replaces the (timestamp, text) pairs.
func (f *SynchronizedTextFrame) SetItems(items []SyncTextItem) { f.content = items }

func (f *SynchronizedTextFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	return f.AsBytes(ctx.writeOptions)
}

// AsBytes converts a [SynchronizedTextFrame] to an ID3v2 SYLT frame byte slice
// (no frame header).
func (f *SynchronizedTextFrame) AsBytes(writeOptions WriteOptions) ([]byte, error) {
	for _, c := range f.Language {
		if !isASCIIAlphabetic(c) {
			return nil, frameEncodingInvalidLanguage(f.Language)
		}
	}
	encoding := f.Encoding
	if writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	data := []byte{uint8(f.Encoding)}
	data = append(data, f.Language[:]...)
	data = append(data, uint8(f.TimestampFormat), uint8(f.ContentType))
	desc, _ := f.Description.Get()
	descBytes, err := encoding.Encode(desc, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	data = append(data, descBytes...)
	for _, item := range f.content {
		tb, err := encoding.Encode(item.Text, true, writeOptions.LossyTextEncoding)
		if err != nil {
			return nil, frameEncodingError(None[FrameId](), err)
		}
		data = append(data, tb...)
		var tb32 [4]byte
		binary.BigEndian.PutUint32(tb32[:], item.Timestamp)
		data = append(data, tb32[:]...)
	}
	if uint64(len(data)) > uint64(^uint32(0)) {
		return nil, frameEncodingError(None[FrameId](), &TooMuchDataError{})
	}
	return data, nil
}

func (f *SynchronizedTextFrame) cloneFrame() Frame {
	c := *f
	c.content = append([]SyncTextItem(nil), f.content...)
	return &c
}

// ParseSynchronizedTextFrame reads a [SynchronizedTextFrame] from SYLT frame
// content bytes.
func ParseSynchronizedTextFrame(data []byte, frameFlags FrameFlags) (*SynchronizedTextFrame, error) {
	ret, err := func() (*SynchronizedTextFrame, error) {
		if len(data) < 7 {
			return nil, frameParseUndersized(FrameIdValid("SYLT"))
		}
		if data[0] > 3 {
			return nil, frameParseError(None[FrameId](), &BadTextEncodingError{})
		}
		encoding := TextEncoding(data[0])
		var language [3]byte
		copy(language[:], data[1:4])
		for _, c := range language {
			if !isASCIIAlphabetic(c) {
				return nil, frameParseInvalidLanguage(language)
			}
		}
		tf, err := TimestampFormatFromU8(data[4])
		if err != nil {
			return nil, frameParseError(None[FrameId](), err)
		}
		ct, err := SyncTextContentTypeFromU8(data[5])
		if err != nil {
			return nil, frameParseError(None[FrameId](), err)
		}

		cursor := bytes.NewReader(data[6:])
		descRes, err := decodeText(cursor, textDecodeOpts(encoding, true))
		if err != nil {
			return nil, frameParseError(None[FrameId](), err)
		}

		var endianness util.ByteOrder
		hasEndianness := false
		if encoding == util.TextEncodingUTF16 {
			switch descRes.BOM {
			case [2]byte{0xFF, 0xFE}:
				endianness, hasEndianness = util.LittleEndian, true
			case [2]byte{0xFE, 0xFF}:
				endianness, hasEndianness = util.BigEndian, true
			}
		}

		pos := 0
		total := (len(data) - 6) - (len(data[6:]) - cursor.Len())

		var content []SyncTextItem
		for pos < total {
			var text string
			var read int
			if hasEndianness {
				text, read, err = util.Utf16DecodeTerminatedMaybeBom(cursor, endianness)
				if err != nil {
					return nil, frameParseError(None[FrameId](), err)
				}
			} else {
				var res util.DecodeTextResult
				res, err = decodeText(cursor, textDecodeOpts(encoding, true))
				if err != nil {
					return nil, frameParseError(None[FrameId](), err)
				}
				read = res.BytesRead
				text = res.Content
			}
			pos += read
			var tb [4]byte
			if _, err := io.ReadFull(cursor, tb[:]); err != nil {
				return nil, frameParseError(None[FrameId](), err)
			}
			pos += 4
			content = append(content, SyncTextItem{Timestamp: binary.BigEndian.Uint32(tb[:]), Text: text})
		}

		var desc Option[string]
		if descRes.Content != "" {
			desc = Some(descRes.Content)
		}
		return &SynchronizedTextFrame{
			header:          NewFrameHeader(FrameIdValid("SYLT"), frameFlags),
			Encoding:        encoding,
			Language:        language,
			TimestampFormat: tf,
			ContentType:     ct,
			Description:     desc,
			content:         content,
		}, nil
	}()
	if err != nil {
		return nil, setFrameErrID(err, FrameIdValid("SYLT"))
	}
	return ret, nil
}

// --- GeneralEncapsulatedObject ("GEOB") ---

// GeneralEncapsulatedObject allows for encapsulation of any file type inside
// an ID3v2 tag ("GEOB" frame, stored as [BinaryFrame] by default).
type GeneralEncapsulatedObject struct {
	header     FrameHeader
	Encoding   TextEncoding
	MimeType   Option[string]
	FileName   Option[string]
	Descriptor Option[string]
	Data       []byte
}

// NewGeneralEncapsulatedObject creates a new [GeneralEncapsulatedObject].
func NewGeneralEncapsulatedObject(encoding TextEncoding, mimeType, fileName, descriptor Option[string], data []byte) *GeneralEncapsulatedObject {
	return &GeneralEncapsulatedObject{
		header:     NewFrameHeader(FrameIdValid("GEOB"), FrameFlags{}),
		Encoding:   encoding,
		MimeType:   mimeType,
		FileName:   fileName,
		Descriptor: descriptor,
		Data:       data,
	}
}

func (f *GeneralEncapsulatedObject) ID() FrameId            { return f.header.ID }
func (f *GeneralEncapsulatedObject) Flags() FrameFlags      { return f.header.Flags }
func (f *GeneralEncapsulatedObject) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *GeneralEncapsulatedObject) frameEqual(other Frame) bool {
	o, ok := other.(*GeneralEncapsulatedObject)
	return ok && f.header.ID == o.header.ID && f.header.Flags == o.header.Flags &&
		f.Encoding == o.Encoding && f.MimeType == o.MimeType && f.FileName == o.FileName &&
		f.Descriptor == o.Descriptor && bytes.Equal(f.Data, o.Data)
}

func (f *GeneralEncapsulatedObject) frameIsEmpty() (bool, bool) { return len(f.Data) == 0, true }
func (f *GeneralEncapsulatedObject) frameName() string          { return "Binary" }

func (f *GeneralEncapsulatedObject) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	return f.AsBytes(ctx.writeOptions)
}

// AsBytes converts a [GeneralEncapsulatedObject] to an ID3v2 GEOB frame byte
// slice (no frame header).
func (f *GeneralEncapsulatedObject) AsBytes(writeOptions WriteOptions) ([]byte, error) {
	encoding := f.Encoding
	if writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	bytes_ := []byte{uint8(encoding)}
	if mt, ok := f.MimeType.Get(); ok {
		bytes_ = append(bytes_, mt...)
	}
	bytes_ = append(bytes_, 0)
	fn, _ := f.FileName.Get()
	fnBytes, err := encoding.Encode(fn, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	bytes_ = append(bytes_, fnBytes...)
	desc, _ := f.Descriptor.Get()
	descBytes, err := encoding.Encode(desc, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	bytes_ = append(bytes_, descBytes...)
	return append(bytes_, f.Data...), nil
}

func (f *GeneralEncapsulatedObject) cloneFrame() Frame {
	c := *f
	c.Data = append([]byte(nil), f.Data...)
	return &c
}

// ParseGeneralEncapsulatedObject reads a [GeneralEncapsulatedObject] from GEOB
// frame content bytes.
func ParseGeneralEncapsulatedObject(data []byte, frameFlags FrameFlags) (*GeneralEncapsulatedObject, error) {
	if len(data) < 4 {
		return nil, frameParseUndersized(FrameIdValid("GEOB"))
	}
	if data[0] > 3 {
		return nil, frameParseError(None[FrameId](), &BadTextEncodingError{})
	}
	encoding := TextEncoding(data[0])
	cursor := bytes.NewReader(data[1:])
	mimeRes, err := decodeText(cursor, textDecodeOpts(util.TextEncodingLatin1, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	fnRes, err := decodeText(cursor, textDecodeOpts(encoding, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	descRes, err := decodeText(cursor, textDecodeOpts(encoding, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	rest, err := io.ReadAll(cursor)
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	return &GeneralEncapsulatedObject{
		header:     NewFrameHeader(FrameIdValid("GEOB"), frameFlags),
		Encoding:   encoding,
		MimeType:   mimeRes.TextOrNone(),
		FileName:   fnRes.TextOrNone(),
		Descriptor: descRes.TextOrNone(),
		Data:       rest,
	}, nil
}

// --- AudioTextFrame ("ATXT") ---

// AudioTextFrameFlags are flags for an ID3v2 audio-text frame.
type AudioTextFrameFlags struct {
	// Scrambling — the scrambling method has been applied to the audio data.
	Scrambling bool
}

// AudioTextFrameFlagsFromU8 reads ATXT frame flags from a byte.
func AudioTextFrameFlagsFromU8(b uint8) AudioTextFrameFlags {
	return AudioTextFrameFlags{Scrambling: b&0x01 > 0}
}

// AsU8 converts [AudioTextFrameFlags] to a byte.
func (f AudioTextFrameFlags) AsU8() uint8 {
	if f.Scrambling {
		return 1
	}
	return 0
}

// AudioTextFrame is an ID3v2 "ATXT" frame (stored as [BinaryFrame] by default).
type AudioTextFrame struct {
	header         FrameHeader
	Encoding       TextEncoding
	MimeType       string
	AudioFlags     AudioTextFrameFlags
	EquivalentText string
	AudioData      []byte
}

// NewAudioTextFrame creates a new [AudioTextFrame].
func NewAudioTextFrame(encoding TextEncoding, mimeType string, flags AudioTextFrameFlags, equivalentText string, audioData []byte) *AudioTextFrame {
	return &AudioTextFrame{
		header:         NewFrameHeader(FrameIdValid("ATXT"), FrameFlags{}),
		Encoding:       encoding,
		MimeType:       mimeType,
		AudioFlags:     flags,
		EquivalentText: equivalentText,
		AudioData:      audioData,
	}
}

func (f *AudioTextFrame) ID() FrameId            { return f.header.ID }
func (f *AudioTextFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *AudioTextFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *AudioTextFrame) frameEqual(other Frame) bool {
	o, ok := other.(*AudioTextFrame)
	return ok && f.EquivalentText == o.EquivalentText
}

func (f *AudioTextFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *AudioTextFrame) frameName() string          { return "Binary" }

func (f *AudioTextFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	return f.AsBytes(ctx.writeOptions)
}

// AsBytes converts an [AudioTextFrame] to an ID3v2 ATXT frame byte slice
// (no frame header).
func (f *AudioTextFrame) AsBytes(writeOptions WriteOptions) ([]byte, error) {
	encoding := f.Encoding
	if writeOptions.UseID3v23 {
		encoding = encoding.ToID3v23()
	}
	content := []byte{uint8(encoding)}
	mime, err := util.TextEncodingLatin1.Encode(f.MimeType, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	content = append(content, mime...)
	content = append(content, f.AudioFlags.AsU8())
	et, err := encoding.Encode(f.EquivalentText, true, writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	content = append(content, et...)
	return append(content, f.AudioData...), nil
}

func (f *AudioTextFrame) cloneFrame() Frame {
	c := *f
	c.AudioData = append([]byte(nil), f.AudioData...)
	return &c
}

// ParseAudioTextFrame reads an [AudioTextFrame] from ATXT frame content bytes.
func ParseAudioTextFrame(bytes_ []byte, frameFlags FrameFlags) (*AudioTextFrame, error) {
	if len(bytes_) < 4 {
		return nil, frameParseUndersized(FrameIdValid("ATXT"))
	}
	content := bytes.NewReader(bytes_)
	var encByte [1]byte
	if _, err := io.ReadFull(content, encByte[:]); err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	if encByte[0] > 3 {
		return nil, frameParseError(None[FrameId](), &BadTextEncodingError{})
	}
	encoding := TextEncoding(encByte[0])
	mimeRes, err := decodeText(content, textDecodeOpts(util.TextEncodingLatin1, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	var flagByte [1]byte
	if _, err := io.ReadFull(content, flagByte[:]); err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	flags := AudioTextFrameFlagsFromU8(flagByte[0])
	etRes, err := decodeText(content, textDecodeOpts(encoding, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	rest, err := io.ReadAll(content)
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	return &AudioTextFrame{
		header:         NewFrameHeader(FrameIdValid("ATXT"), frameFlags),
		Encoding:       encoding,
		MimeType:       mimeRes.Content,
		AudioFlags:     flags,
		EquivalentText: etRes.Content,
		AudioData:      rest,
	}, nil
}

var scramblingTable = func() [127]byte {
	var table [127]byte
	table[0] = 0xFE
	i := 0
	for {
		b := table[i]
		bit7 := (b >> 7) & 1
		bit6 := (b >> 6) & 1
		bit5 := (b >> 5) & 1
		bit4 := (b >> 4) & 1
		bit3 := (b >> 3) & 1
		bit2 := (b >> 2) & 1
		bit1 := (b >> 1) & 1
		bit0 := b & 1
		newByte := ((bit6 ^ bit5) << 7) + ((bit5 ^ bit4) << 6) + ((bit4 ^ bit3) << 5) +
			((bit3 ^ bit2) << 4) + ((bit2 ^ bit1) << 3) + ((bit1 ^ bit0) << 2) +
			((bit7 ^ bit5) << 1) + (bit6 ^ bit4)
		if newByte == 0xFE {
			break
		}
		i++
		table[i] = newByte
	}
	return table
}()

// Scramble scrambles/unscrambles the audio clip of an ATXT frame in place.
func Scramble(audioData []byte) {
	idx := 0
	for i := range audioData {
		audioData[i] ^= scramblingTable[idx]
		if idx == 126 {
			idx = 0
		} else {
			idx++
		}
	}
}
