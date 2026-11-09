package lofty

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// PictureParseError is returned when a [Picture] fails to parse.
type PictureParseError struct {
	Message string
	Source  error
}

func (e *PictureParseError) Error() string {
	if e.Message != "" {
		return "failed to parse picture: " + e.Message
	}
	return "failed to parse picture"
}

func (e *PictureParseError) Unwrap() error { return e.Source }

func pictureParseError(source error) *PictureParseError {
	return &PictureParseError{Source: source}
}

// UnknownImageFormatError is returned when the image format cannot be determined.
type UnknownImageFormatError struct{}

func (e *UnknownImageFormatError) Error() string { return "unable to determine image format" }

// ApePictureTypes are the common picture item keys for APE.
var ApePictureTypes = [21]string{
	"Cover Art (Other)",
	"Cover Art (Png Icon)",
	"Cover Art (Icon)",
	"Cover Art (Front)",
	"Cover Art (Back)",
	"Cover Art (Leaflet)",
	"Cover Art (Media)",
	"Cover Art (Lead Artist)",
	"Cover Art (Artist)",
	"Cover Art (Conductor)",
	"Cover Art (Band)",
	"Cover Art (Composer)",
	"Cover Art (Lyricist)",
	"Cover Art (Recording Location)",
	"Cover Art (During Recording)",
	"Cover Art (During Performance)",
	"Cover Art (Video Capture)",
	"Cover Art (Fish)",
	"Cover Art (Illustration)",
	"Cover Art (Band Logotype)",
	"Cover Art (Publisher Logotype)",
}

// MimeType of a [Picture].
type MimeType struct {
	kind    mimeTypeKind
	unknown string
}

type mimeTypeKind uint8

const (
	mimePng mimeTypeKind = iota + 1
	mimeJpeg
	mimeTiff
	mimeBmp
	mimeGif
	mimeUnknown
)

var (
	// MimeTypePng is the PNG image MIME type.
	MimeTypePng = MimeType{kind: mimePng}
	// MimeTypeJpeg is the JPEG image MIME type.
	MimeTypeJpeg = MimeType{kind: mimeJpeg}
	// MimeTypeTiff is the TIFF image MIME type.
	MimeTypeTiff = MimeType{kind: mimeTiff}
	// MimeTypeBmp is the BMP image MIME type.
	MimeTypeBmp = MimeType{kind: mimeBmp}
	// MimeTypeGif is the GIF image MIME type.
	MimeTypeGif = MimeType{kind: mimeGif}
)

// MimeTypeUnknown creates a [MimeType] for an unknown MIME type string.
func MimeTypeUnknown(s string) MimeType { return MimeType{kind: mimeUnknown, unknown: s} }

// MimeTypeFromStr gets a [MimeType] from a string.
func MimeTypeFromStr(mimeType string) MimeType {
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		return MimeTypeJpeg
	case "image/png":
		return MimeTypePng
	case "image/tiff":
		return MimeTypeTiff
	case "image/bmp":
		return MimeTypeBmp
	case "image/gif":
		return MimeTypeGif
	}
	return MimeTypeUnknown(mimeType)
}

// AsStr returns the MIME type as a string.
func (m MimeType) AsStr() string {
	switch m.kind {
	case mimeJpeg:
		return "image/jpeg"
	case mimePng:
		return "image/png"
	case mimeTiff:
		return "image/tiff"
	case mimeBmp:
		return "image/bmp"
	case mimeGif:
		return "image/gif"
	default:
		return m.unknown
	}
}

// Ext returns the file extension for the [MimeType] if it is known.
func (m MimeType) Ext() (string, bool) {
	switch m.kind {
	case mimeJpeg:
		return "jpg", true
	case mimePng:
		return "png", true
	case mimeTiff:
		return "tif", true
	case mimeBmp:
		return "bmp", true
	case mimeGif:
		return "gif", true
	}
	return "", false
}

func (m MimeType) String() string { return m.AsStr() }

// PictureType is the picture type, according to ID3v2 APIC.
type PictureType uint8

const (
	// PictureTypeOther is a picture type not covered by the remaining variants.
	PictureTypeOther PictureType = 0
	// PictureTypeIcon is a 32x32 pixels file icon (PNG only).
	PictureTypeIcon PictureType = 1
	// PictureTypeOtherIcon is another file icon.
	PictureTypeOtherIcon PictureType = 2
	// PictureTypeCoverFront is the front cover.
	PictureTypeCoverFront PictureType = 3
	// PictureTypeCoverBack is the back cover.
	PictureTypeCoverBack PictureType = 4
	// PictureTypeLeaflet is a leaflet page.
	PictureTypeLeaflet PictureType = 5
	// PictureTypeMedia is the media (e.g. label side of CD).
	PictureTypeMedia PictureType = 6
	// PictureTypeLeadArtist is the lead artist/lead performer/soloist.
	PictureTypeLeadArtist PictureType = 7
	// PictureTypeArtist is the artist/performer.
	PictureTypeArtist PictureType = 8
	// PictureTypeConductor is the conductor.
	PictureTypeConductor PictureType = 9
	// PictureTypeBand is the band/orchestra.
	PictureTypeBand PictureType = 10
	// PictureTypeComposer is the composer.
	PictureTypeComposer PictureType = 11
	// PictureTypeLyricist is the lyricist/text writer.
	PictureTypeLyricist PictureType = 12
	// PictureTypeRecordingLocation is the recording location.
	PictureTypeRecordingLocation PictureType = 13
	// PictureTypeDuringRecording is during recording.
	PictureTypeDuringRecording PictureType = 14
	// PictureTypeDuringPerformance is during performance.
	PictureTypeDuringPerformance PictureType = 15
	// PictureTypeScreenCapture is a movie/video screen capture.
	PictureTypeScreenCapture PictureType = 16
	// PictureTypeBrightFish is a bright colored fish.
	PictureTypeBrightFish PictureType = 17
	// PictureTypeIllustration is an illustration.
	PictureTypeIllustration PictureType = 18
	// PictureTypeBandLogo is a band/artist logotype.
	PictureTypeBandLogo PictureType = 19
	// PictureTypePublisherLogo is a publisher/studio logotype.
	PictureTypePublisherLogo PictureType = 20
)

// PictureTypeUndefined is an undefined picture type.
func PictureTypeUndefined(i uint8) PictureType { return PictureType(i) }

// AsU8 returns the u8 for a [PictureType] according to ID3v2 APIC.
func (p PictureType) AsU8() uint8 { return uint8(p) }

// PictureTypeFromU8 gets a [PictureType] from a u8 according to ID3v2 APIC.
func PictureTypeFromU8(b uint8) PictureType { return PictureType(b) }

// AsApeKey gets an APE item key from a [PictureType].
func (p PictureType) AsApeKey() (string, bool) {
	if int(p) < len(ApePictureTypes) {
		return ApePictureTypes[p], true
	}
	return "", false
}

// PictureTypeFromApeKey gets a [PictureType] from an APE item key.
func PictureTypeFromApeKey(key string) PictureType {
	for i, k := range ApePictureTypes {
		if k == key {
			return PictureType(i)
		}
	}
	return PictureType(0)
}

// PictureInformation describes a [Picture].
//
// This information is necessary for FLAC's `METADATA_BLOCK_PICTURE`.
// See [Picture.AsFlacBytes] for more information.
type PictureInformation struct {
	// Width is the picture's width in pixels.
	Width uint32
	// Height is the picture's height in pixels.
	Height uint32
	// ColorDepth is the picture's color depth in bits per pixel.
	ColorDepth uint32
	// NumColors is the number of colors used.
	NumColors uint32
}

// PictureInformationFromPicture attempts to extract [PictureInformation] from a [Picture].
//
// NOTE: This only supports PNG and JPEG images. If another image is provided,
// the [PictureInformation] will be zeroed out.
func PictureInformationFromPicture(picture *Picture) (PictureInformation, error) {
	data := picture.data
	if len(data) < 8 {
		return PictureInformation{}, pictureParseError(newNotEnoughDataError(8))
	}
	switch {
	case bytes.Equal(data[:4], []byte{0x89, 'P', 'N', 'G'}):
		info, err := PictureInformationFromPNG(data)
		if err != nil {
			return PictureInformation{}, nil
		}
		return info, nil
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		info, err := PictureInformationFromJPEG(data)
		if err != nil {
			return PictureInformation{}, nil
		}
		return info, nil
	}
	return PictureInformation{}, nil
}

// PictureInformationFromPNG attempts to extract [PictureInformation] from a PNG.
func PictureInformationFromPNG(data []byte) (PictureInformation, error) {
	reader := bytes.NewReader(data)

	var sig [8]byte
	if _, err := io.ReadFull(reader, sig[:]); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	if sig != [8]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A} {
		return PictureInformation{}, pictureParseError(&UnknownImageFormatError{})
	}

	var ihdr [8]byte
	if _, err := io.ReadFull(reader, ihdr[:]); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	// Verify the signature is immediately followed by the IHDR chunk
	if !bytes.HasSuffix(ihdr[:], []byte("IHDR")) {
		return PictureInformation{}, pictureParseError(&UnknownImageFormatError{})
	}

	var width, height uint32
	if err := binary.Read(reader, binary.BigEndian, &width); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	if err := binary.Read(reader, binary.BigEndian, &height); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	colorDepthB, err := reader.ReadByte()
	if err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	colorType, err := reader.ReadByte()
	if err != nil {
		return PictureInformation{}, pictureParseError(err)
	}

	colorDepth := uint32(colorDepthB)
	switch colorType {
	case 2:
		colorDepth *= 3
	case 4, 6:
		colorDepth *= 4
	}

	ret := PictureInformation{
		Width:      width,
		Height:     height,
		ColorDepth: colorDepth,
	}

	// The color type 3 (indexed-color) means there should be a "PLTE" chunk, whose data
	// can be used in the `num_colors` field.
	if colorType != 3 {
		return ret, nil
	}

	// Skip 7 bytes: Compression method (1) + Filter method (1) + Interlace method (1) + CRC (4)
	if _, err := reader.Seek(7, io.SeekCurrent); err != nil {
		return ret, nil
	}

	var chunkType [4]byte
	for {
		var size uint32
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			break
		}
		if _, err := io.ReadFull(reader, chunkType[:]); err != nil {
			break
		}
		if chunkType == [4]byte{'P', 'L', 'T', 'E'} {
			// The PLTE chunk contains 1-256 3-byte entries
			ret.NumColors = size / 3
			break
		}
		// Skip the chunk's data (size) and CRC (4 bytes)
		if _, err := reader.Seek(int64(size)+4, io.SeekCurrent); err != nil {
			break
		}
	}

	return ret, nil
}

// PictureInformationFromJPEG attempts to extract [PictureInformation] from a JPEG.
func PictureInformationFromJPEG(data []byte) (PictureInformation, error) {
	reader := bytes.NewReader(data)

	var frameMarker [4]byte
	if _, err := io.ReadFull(reader, frameMarker[:]); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}
	if frameMarker[0] != 0xFF || frameMarker[1] != 0xD8 || frameMarker[2] != 0xFF {
		return PictureInformation{}, pictureParseError(&UnknownImageFormatError{})
	}

	var sectionLen uint16
	if err := binary.Read(reader, binary.BigEndian, &sectionLen); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}

	// The length contains itself, so anything < 2 is invalid
	if sectionLen < 2 {
		return PictureInformation{}, pictureParseError(newNotEnoughDataError(2))
	}
	if _, err := reader.Seek(int64(sectionLen-2), io.SeekCurrent); err != nil {
		return PictureInformation{}, pictureParseError(err)
	}

	for {
		b, err := reader.ReadByte()
		if err != nil || b != 0xFF {
			break
		}
		marker, err := reader.ReadByte()
		if err != nil {
			break
		}
		if err := binary.Read(reader, binary.BigEndian, &sectionLen); err != nil {
			break
		}

		// This marks the SOS (Start of Scan), which is the end of the header
		if marker == 0xDA {
			break
		}

		// We are looking for a frame with a "SOFn" marker, with `n` either being 0 or 2.
		if marker == 0xC0 || marker == 0xC2 {
			precision, err := reader.ReadByte()
			if err != nil {
				break
			}
			var h, w uint16
			if err := binary.Read(reader, binary.BigEndian, &h); err != nil {
				break
			}
			if err := binary.Read(reader, binary.BigEndian, &w); err != nil {
				break
			}
			components, err := reader.ReadByte()
			if err != nil {
				break
			}
			return PictureInformation{
				Width:      uint32(w),
				Height:     uint32(h),
				ColorDepth: uint32(precision) * uint32(components),
			}, nil
		}

		if sectionLen < 2 {
			break
		}
		if _, err := reader.Seek(int64(sectionLen-2), io.SeekCurrent); err != nil {
			break
		}
	}

	return PictureInformation{}, pictureParseError(&UnknownImageFormatError{})
}

// Picture represents a picture.
type Picture struct {
	picType     PictureType
	mimeType    Option[MimeType]
	description Option[string]
	data        []byte
}

// PictureEmpty is a placeholder [Picture] for conversions.
var PictureEmpty = Picture{picType: PictureTypeOther}

// PictureFromReader creates a [Picture] from a reader.
//
// NOTES:
// * This is for reading picture data only, from an [*os.File] for example.
// * PicType will always be [PictureTypeOther], be sure to change it accordingly if
//
//	writing.
//
// # Errors
//
// * `reader` contains less than 8 bytes
// * `reader` does not contain a supported format. See [MimeType] for valid formats
func PictureFromReader(reader io.Reader) (*Picture, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, pictureParseError(err)
	}
	if len(data) < 8 {
		return nil, pictureParseError(newNotEnoughDataError(8))
	}
	mimeType, err := mimetypeFromBin(data[:8])
	if err != nil {
		return nil, pictureParseError(err)
	}
	return &Picture{
		picType:  PictureTypeOther,
		mimeType: Some(mimeType),
		data:     data,
	}, nil
}

// PictureUnchecked creates a new [PictureBuilder] with no verification.
//
// This will **not** verify `data`'s signature.
//
// This should only be used if all data has been verified beforehand.
func PictureUnchecked(data []byte) *PictureBuilder {
	return &PictureBuilder{picType: PictureTypeOther, data: data}
}

// PictureBuilder is a builder for a [Picture].
//
// This is created through [PictureUnchecked].
type PictureBuilder struct {
	picType     PictureType
	mimeType    Option[MimeType]
	description Option[string]
	data        []byte
}

// PicType sets the [PictureType] for this picture.
func (b *PictureBuilder) PicType(picType PictureType) *PictureBuilder {
	b.picType = picType
	return b
}

// MimeType sets the [MimeType] for this picture.
func (b *PictureBuilder) MimeType(mimeType MimeType) *PictureBuilder {
	b.mimeType = Some(mimeType)
	return b
}

// Description sets the description for this picture.
func (b *PictureBuilder) Description(description string) *PictureBuilder {
	b.description = Some(description)
	return b
}

// Build converts this builder into a [Picture].
func (b *PictureBuilder) Build() Picture {
	return Picture{
		picType:     b.picType,
		mimeType:    b.mimeType,
		description: b.description,
		data:        b.data,
	}
}

// PicType returns the [PictureType].
func (p *Picture) PicType() PictureType { return p.picType }

// SetPicType sets the [PictureType].
func (p *Picture) SetPicType(picType PictureType) { p.picType = picType }

// MimeType returns the [MimeType].
//
// The mime type is determined from the data, and is immutable.
func (p *Picture) MimeType() Option[MimeType] { return p.mimeType }

// mimeStr is used commonly internally.
func (p *Picture) mimeStr() string {
	if mt, ok := p.mimeType.Get(); ok {
		return mt.AsStr()
	}
	return ""
}

// Description returns the description.
func (p *Picture) Description() (string, bool) { return p.description.Get() }

// SetDescription sets the description.
func (p *Picture) SetDescription(description string) { p.description = Some(description) }

// ClearDescription removes the description.
func (p *Picture) ClearDescription() { p.description = None[string]() }

// Data returns the [Picture] data.
func (p *Picture) Data() []byte { return p.data }

// AsFlacBytes converts a [Picture] to a base64 encoded FLAC `METADATA_BLOCK_PICTURE` string.
//
// Use `encode` to convert the picture to a base64 encoded string ([RFC 4648 §4]).
//
// NOTES:
// * This does not include a key (Vorbis comments) or METADATA_BLOCK_HEADER (FLAC blocks)
// * FLAC blocks have different size requirements than OGG Vorbis/Opus, size is not checked here
// * When writing to Vorbis comments, the data **must** be base64 encoded
//
// [RFC 4648 §4]: http://www.faqs.org/rfcs/rfc4648.html
func (p *Picture) AsFlacBytes(pictureInformation PictureInformation, encode bool) []byte {
	var data []byte
	var tmp [4]byte

	binary.BigEndian.PutUint32(tmp[:], uint32(p.picType.AsU8()))
	data = append(data, tmp[:]...)

	mimeStr := p.mimeStr()
	binary.BigEndian.PutUint32(tmp[:], uint32(len(mimeStr)))
	data = append(data, tmp[:]...)
	data = append(data, mimeStr...)

	if desc, ok := p.description.Get(); ok {
		binary.BigEndian.PutUint32(tmp[:], uint32(len(desc)))
		data = append(data, tmp[:]...)
		data = append(data, desc...)
	} else {
		data = append(data, 0, 0, 0, 0)
	}

	binary.BigEndian.PutUint32(tmp[:], pictureInformation.Width)
	data = append(data, tmp[:]...)
	binary.BigEndian.PutUint32(tmp[:], pictureInformation.Height)
	data = append(data, tmp[:]...)
	binary.BigEndian.PutUint32(tmp[:], pictureInformation.ColorDepth)
	data = append(data, tmp[:]...)
	binary.BigEndian.PutUint32(tmp[:], pictureInformation.NumColors)
	data = append(data, tmp[:]...)

	binary.BigEndian.PutUint32(tmp[:], uint32(len(p.data)))
	data = append(data, tmp[:]...)
	data = append(data, p.data...)

	if encode {
		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(encoded, data)
		return encoded
	}
	return data
}

// PictureFromFlacBytes gets a [Picture] from FLAC `METADATA_BLOCK_PICTURE` bytes.
//
// NOTE: This takes both the base64 encoded string from Vorbis comments, and
// the raw data from a FLAC block, specified with `encoded`.
//
// [PictureInformation] is also returned.
func PictureFromFlacBytes(b []byte, encoded bool, parseMode ParsingMode) (Picture, PictureInformation, error) {
	if encoded {
		data, err := base64.StdEncoding.DecodeString(string(b))
		if err != nil {
			return Picture{}, PictureInformation{}, pictureParseError(err)
		}
		return pictureFromFlacBytesInner(data, parseMode)
	}
	return pictureFromFlacBytesInner(b, parseMode)
}

func pictureFromFlacBytesInner(content []byte, parseMode ParsingMode) (Picture, PictureInformation, error) {
	size := len(content)
	reader := bytes.NewReader(content)

	if size < 32 {
		return Picture{}, PictureInformation{}, pictureParseError(newNotEnoughDataError(32))
	}

	var picTy uint32
	if err := binary.Read(reader, binary.BigEndian, &picTy); err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}
	size -= 4

	// ID3v2 APIC uses a single byte for picture type.
	// Anything greater than that is probably invalid, so we just stop early.
	if picTy > 255 && parseMode == ParsingModeStrict {
		return Picture{}, PictureInformation{}, &PictureParseError{
			Message: fmt.Sprintf("unknown picture type: %d", picTy),
		}
	}

	var mimeLen uint32
	if err := binary.Read(reader, binary.BigEndian, &mimeLen); err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}
	size -= 4
	if int(mimeLen) > size {
		return Picture{}, PictureInformation{}, pictureParseError(&SizeMismatchError{})
	}

	mimeTypeStr, err := utf8DecodeStr(content[8 : 8+mimeLen])
	if err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}
	size -= int(mimeLen)
	if _, err := reader.Seek(int64(mimeLen), io.SeekCurrent); err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}

	var descLen uint32
	if err := binary.Read(reader, binary.BigEndian, &descLen); err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}
	size -= 4

	var description Option[string]
	if descLen > 0 && int(descLen) < size {
		pos := 12 + int(mimeLen)
		if desc, err := utf8DecodeStr(content[pos : pos+int(descLen)]); err == nil {
			description = Some(desc)
		}
		size -= int(descLen)
		if _, err := reader.Seek(int64(descLen), io.SeekCurrent); err != nil {
			return Picture{}, PictureInformation{}, pictureParseError(err)
		}
	}

	var width, height, colorDepth, numColors, dataLenU32 uint32
	for _, p := range []*uint32{&width, &height, &colorDepth, &numColors, &dataLenU32} {
		if err := binary.Read(reader, binary.BigEndian, p); err != nil {
			return Picture{}, PictureInformation{}, pictureParseError(err)
		}
	}
	dataLen := int(dataLenU32)
	size -= 20

	if dataLen > size {
		return Picture{}, PictureInformation{}, pictureParseError(&SizeMismatchError{})
	}

	data, err := tryByteSlice(dataLen)
	if err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}
	if _, err := io.ReadFull(reader, data); err != nil {
		return Picture{}, PictureInformation{}, pictureParseError(err)
	}

	var mimeType Option[MimeType]
	if mimeTypeStr != "" {
		mimeType = Some(MimeTypeFromStr(mimeTypeStr))
	}

	return Picture{
		picType:     PictureTypeFromU8(uint8(picTy)),
		mimeType:    mimeType,
		description: description,
		data:        data,
	}, PictureInformation{
		Width:      width,
		Height:     height,
		ColorDepth: colorDepth,
		NumColors:  numColors,
	}, nil
}

// AsApeBytes converts a [Picture] to an APE Cover Art byte slice.
//
// NOTE: This is only the picture data and description, a
// key and terminating null byte will not be prepended.
// To map a [PictureType] to an APE key see [PictureType.AsApeKey].
func (p *Picture) AsApeBytes() []byte {
	var data []byte
	if desc, ok := p.description.Get(); ok {
		data = append(data, desc...)
	}
	data = append(data, 0)
	data = append(data, p.data...)
	return data
}

// PictureFromApeBytes gets a [Picture] from an APEv2 binary item.
//
// NOTE: This function expects `bytes` to contain *only* the APE item data.
func PictureFromApeBytes(key string, b []byte) (Picture, error) {
	if len(b) == 0 {
		return Picture{}, pictureParseError(errNotEnoughData)
	}

	picType := PictureTypeFromApeKey(key)

	pos := 0
	var description Option[string]
	var descText strings.Builder

	for pos < len(b) {
		ch := b[pos]
		pos++
		if ch == 0 {
			break
		}
		descText.WriteByte(ch)
	}

	if descText.Len() > 0 {
		description = Some(descText.String())
	}

	if pos+8 > len(b) {
		return Picture{}, pictureParseError(io.ErrUnexpectedEOF)
	}
	mimeType, err := mimetypeFromBin(b[pos : pos+8])
	if err != nil {
		return Picture{}, pictureParseError(err)
	}

	return Picture{
		picType:     picType,
		mimeType:    Some(mimeType),
		description: description,
		data:        b[pos:],
	}, nil
}

func mimetypeFromBin(b []byte) (MimeType, error) {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return MimeTypePng, nil
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8:
		return MimeTypeJpeg, nil
	case len(b) >= 6 && b[0] == 'G' && b[1] == 'I' && b[2] == 'F' && b[3] == '8' && (b[4] == '7' || b[4] == '9') && b[5] == 'a':
		return MimeTypeGif, nil
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return MimeTypeBmp, nil
	case len(b) >= 4 && ((b[0] == 'I' && b[1] == 'I' && b[2] == '*' && b[3] == 0) ||
		(b[0] == 'M' && b[1] == 'M' && b[2] == 0 && b[3] == '*')):
		return MimeTypeTiff, nil
	}
	return MimeType{}, &UnknownImageFormatError{}
}
