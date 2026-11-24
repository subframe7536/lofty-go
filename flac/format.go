package flac

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	"github.com/subframe7536/lofty-go/id3v2"
	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/vorbis"
)

var flacPreamble = []byte("fLaC")

// flacMaxBlockSize is Rust's `Block::MAX_CONTENT_SIZE` (a 24-bit length).
const flacMaxBlockSize = 0xFFFFFF

// FlacParseError is returned when a FLAC file fails to parse.
type FlacParseError struct {
	Message string
	Source  error
}

func (e *FlacParseError) Error() string {
	if e.Message != "" {
		return "FLAC: " + e.Message
	}
	return "failed to parse FLAC file"
}
func (e *FlacParseError) Unwrap() error { return e.Source }

// FlacProperties are the audio properties of a FLAC file.
type FlacProperties struct {
	Duration       time.Duration
	OverallBitrate uint32
	AudioBitrate   uint32
	SampleRate     uint32
	BitDepth       uint8
	Channels       uint8
	// Signature is the MD5 of the unencoded audio data from STREAMINFO.
	Signature [16]byte
}

// FileProperties converts to generic [meta.FileProperties].
func (p FlacProperties) FileProperties() meta.FileProperties {
	return meta.NewFileProperties(
		p.Duration,
		meta.Some(p.OverallBitrate),
		meta.Some(p.AudioBitrate),
		meta.Some(p.SampleRate),
		meta.Some(p.BitDepth),
		meta.Some(p.Channels),
		meta.None[meta.ChannelMask](),
	)
}

// FlacFile is a FLAC file.
type FlacFile struct {
	Properties     FlacProperties
	VorbisComments meta.Option[*vorbis.VorbisCommentsTag]
	ID3v2          meta.Option[*id3v2.Id3v2Tag]
	pictures       []meta.PictureInfoPair
}

// Pictures returns the file's pictures.
func (f *FlacFile) Pictures() []meta.PictureInfoPair { return f.pictures }

// SetPictures replaces the file's pictures.
func (f *FlacFile) SetPictures(pictures []meta.PictureInfoPair) { f.pictures = pictures }

// InsertPicture is [FlacFile.InsertPictureReplacing] without the removed
// picture.
func (f *FlacFile) InsertPicture(p meta.Picture, info meta.Option[meta.PictureInformation]) error {
	_, err := f.InsertPictureReplacing(p, info)
	return err
}

// InsertPictureReplacing appends a picture to the file, mirroring Rust's
// `OggPictureStorage::insert_picture`. When `info` is unset, the information is
// extracted from the picture data and an extraction failure is returned.
// According to the spec there can only be one picture of type Icon and one of
// type OtherIcon, so an existing picture of the same type is removed and
// returned.
//
// Unlike Rust, the extraction runs before the existing icon is removed, so a
// failure leaves the file unchanged.
func (f *FlacFile) InsertPictureReplacing(p meta.Picture, info meta.Option[meta.PictureInformation]) (meta.Option[meta.PictureInfoPair], error) {
	i, ok := info.Get()
	if !ok {
		var err error
		i, err = meta.PictureInformationFromPicture(&p)
		if err != nil {
			return meta.None[meta.PictureInfoPair](), err
		}
	}
	removed := meta.None[meta.PictureInfoPair]()
	if ty := p.PicType(); ty == meta.PictureTypeIcon || ty == meta.PictureTypeOtherIcon {
		for idx, existing := range f.pictures {
			if existing.Picture.PicType() == ty {
				removed = meta.Some(f.RemovePicture(idx))
				break
			}
		}
	}
	f.pictures = append(f.pictures, meta.PictureInfoPair{Picture: p, Info: i})
	return removed, nil
}

// SetPicture replaces the picture at `index`, mirroring Rust
// `OggPictureStorage::set_picture`: an out-of-range index appends the picture
// instead.
func (f *FlacFile) SetPicture(index int, picture meta.Picture, info meta.PictureInformation) {
	if index < 0 || index >= len(f.pictures) {
		_ = f.InsertPicture(picture, meta.Some(info))
		return
	}
	f.pictures[index] = meta.PictureInfoPair{Picture: picture, Info: info}
}

// RemovePicture removes and returns the picture at `index`, mirroring Rust
// `OggPictureStorage::remove_picture`.
//
// Unlike Rust, an out-of-range index does not panic; it removes nothing and
// returns a zero value.
func (f *FlacFile) RemovePicture(index int) meta.PictureInfoPair {
	if index < 0 || index >= len(f.pictures) {
		return meta.PictureInfoPair{}
	}
	pic := f.pictures[index]
	f.pictures = append(f.pictures[:index], f.pictures[index+1:]...)
	return pic
}

// RemovePictureType removes all pictures of `pictureType`, mirroring Rust
// `OggPictureStorage::remove_picture_type`.
func (f *FlacFile) RemovePictureType(pictureType meta.PictureType) {
	var kept []meta.PictureInfoPair
	for _, p := range f.pictures {
		if p.Picture.PicType() != pictureType {
			kept = append(kept, p)
		}
	}
	f.pictures = kept
}

// RemovePictures removes and returns all of the file's pictures.
func (f *FlacFile) RemovePictures() []meta.PictureInfoPair {
	pics := f.pictures
	f.pictures = nil
	return pics
}

// flacFileError wraps a source in the FLAC file parse error (Rust's
// `From<FlacParseError> for FileParseError`).
func flacFileError(src error) error {
	return &meta.FileParseError{FileType: meta.FileTypeFlac, Source: &FlacParseError{Source: src}}
}

// FlacFileReadFrom reads a [FlacFile] from a reader.
func FlacFileReadFrom(reader io.ReadSeeker, parseOptions meta.ParseOptions) (*FlacFile, error) {
	var file FlacFile

	// It is possible for a FLAC file to contain an ID3v2 tag. Locating it
	// leaves the reader at the end of the whole tag (footer included).
	res, err := id3v2.LocateId3v2(reader, parseOptions.ReadTags, parseOptions)
	if err != nil {
		return nil, &meta.FileParseError{Source: err}
	}
	if res != nil && parseOptions.ReadTags {
		t, err := res.Parse(parseOptions)
		if err != nil {
			return nil, &meta.FileParseError{Source: &meta.TagParseError{TagType: meta.TagTypeID3v2, HasTagType: true, Source: err}}
		}
		file.ID3v2 = meta.Some(t)
	}

	streamInfo, err := verifyFlac(reader)
	if err != nil {
		return nil, &meta.FileParseError{FileType: meta.FileTypeFlac, Source: err}
	}
	// Rust measures the block including its 4-byte header.
	if streamInfo.end-streamInfo.start < 18 {
		return nil, flacFileError(&meta.SizeMismatchError{})
	}

	var vc *vorbis.VorbisCommentsTag
	var pictures []meta.PictureInfoPair

	last := streamInfo.last
	for !last {
		block, err := readFlacBlock(reader, func(ty byte) bool {
			return (ty == blockIDVorbisComment && parseOptions.ReadTags) ||
				(ty == blockIDPicture && parseOptions.ReadCoverArt)
		})
		if err != nil {
			return nil, flacFileError(err)
		}
		last = block.last

		switch {
		case block.ty == blockIDVorbisComment && parseOptions.ReadTags:
			// Only the last VORBIS_COMMENT block wins; strict mode rejects
			// duplicates.
			if vc != nil && parseOptions.ParsingMode == meta.ParsingModeStrict {
				return nil, flacFileError(&FlacParseError{Message: "file contains multiple `VORBIS_COMMENT` blocks"})
			}
			// Vendor string length (4) + field count (4)
			if len(block.content) < 8 {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, flacFileError(util.NewNotEnoughDataError(8))
				}
				continue
			}
			t, err := vorbis.ParseVorbisComments(block.content, parseOptions)
			if err != nil {
				return nil, &meta.FileParseError{Source: &vorbis.VorbisCommentsParseError{Source: err}}
			}
			vc = t
		case block.ty == blockIDPicture && parseOptions.ReadCoverArt:
			// Pictures are stored on the file, not the tag — gated on
			// `read_cover_art` only (Rust `flac::read`). The picture header
			// fields total 28 bytes.
			if len(block.content) < 28 {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, flacFileError(util.NewNotEnoughDataError(28))
				}
				continue
			}
			pic, info, err := meta.PictureFromFlacBytes(block.content, false, parseOptions.ParsingMode)
			if err != nil {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, flacFileError(err)
				}
				continue
			}
			pictures = append(pictures, meta.PictureInfoPair{Picture: pic, Info: info})
		}
	}

	file.VorbisComments = util.OptionFromPtr(vc)
	file.pictures = pictures
	if !parseOptions.ReadProperties {
		return &file, nil
	}

	current, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, flacFileError(err)
	}
	end, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, flacFileError(err)
	}
	// In the event that a block lies about its size, the current position
	// could be completely wrong.
	if current > end {
		return nil, flacFileError(&meta.SizeMismatchError{})
	}

	props, err := readFlacProperties(streamInfo.content, uint64(end-current), uint64(end))
	if err != nil {
		return nil, flacFileError(err)
	}
	file.Properties = props
	return &file, nil
}

// readFlacProperties parses a STREAMINFO payload (Rust `flac::properties::read_properties`).
func readFlacProperties(streamInfo []byte, streamLength, fileLength uint64) (FlacProperties, error) {
	// Min/max block size (4), min/max frame size (6), the packed
	// rate/channels/bit depth/total samples word (4), the remaining total
	// samples (4) and the MD5 signature (16).
	if len(streamInfo) < 34 {
		return FlacProperties{}, io.ErrUnexpectedEOF
	}

	info := binary.BigEndian.Uint32(streamInfo[10:])
	sampleRate := info >> 12
	p := FlacProperties{
		SampleRate: sampleRate,
		BitDepth:   uint8((info>>4)&0b11111) + 1,
		Channels:   uint8((info>>9)&7) + 1,
	}
	totalSamples := uint64(info&0xF)<<32 | uint64(binary.BigEndian.Uint32(streamInfo[14:]))
	copy(p.Signature[:], streamInfo[18:34])

	if sampleRate > 0 && totalSamples > 0 {
		length := totalSamples * 1000 / uint64(sampleRate)
		p.Duration = util.DurationFromMillis(length)

		if length > 0 && fileLength > 0 && streamLength > 0 {
			p.OverallBitrate = uint32(util.SaturatingMul(fileLength, 8) / length)
			p.AudioBitrate = uint32(util.SaturatingMul(streamLength, 8) / length)
		}
	}
	return p, nil
}

// FileType implements meta.AudioFile.
func (f *FlacFile) FileType() meta.FileType { return meta.FileTypeFlac }

// GetProperties implements meta.AudioFile.
func (f *FlacFile) GetProperties() *meta.FileProperties {
	fp := f.Properties.FileProperties()
	return &fp
}

// ContainsTag implements meta.AudioFile.
func (f *FlacFile) ContainsTag() bool {
	return f.VorbisComments.IsSome() || f.ID3v2.IsSome() || len(f.pictures) > 0
}

// ContainsTagType implements meta.AudioFile.
func (f *FlacFile) ContainsTagType(tagType meta.TagType) bool {
	switch tagType {
	case meta.TagTypeVorbisComments:
		return f.VorbisComments.IsSome() || len(f.pictures) > 0
	case meta.TagTypeID3v2:
		return f.ID3v2.IsSome()
	}
	return false
}

// TaggedFile converts to a [meta.TaggedFile]. Tag order follows the Rust struct
// declaration order: ID3v2, VorbisComments.
func (f *FlacFile) TaggedFile() *meta.TaggedFile {
	var tags []*meta.Tag
	if t, ok := f.ID3v2.Get(); ok {
		tags = append(tags, t.Tag())
	}
	if t, ok := f.VorbisComments.Get(); ok {
		g := t.Tag()
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	} else if len(f.pictures) > 0 {
		g := meta.NewTag(meta.TagTypeVorbisComments)
		for _, pic := range f.pictures {
			g.PushPicture(pic.Picture)
		}
		tags = append(tags, g)
	}
	return meta.NewTaggedFile(meta.FileTypeFlac, tags, *f.GetProperties())
}

// SavePath saves the file's tags back to the path.
func (f *FlacFile) SavePath(path string, writeOptions meta.WriteOptions) error {
	return meta.SaveToPath(path, f, writeOptions)
}

// SaveFile writes tags to an open file.
func (f *FlacFile) SaveFile(file meta.FileLike, writeOptions meta.WriteOptions) error {
	if t, ok := f.ID3v2.Get(); ok {
		if !t.IsEmpty() {
			return &meta.FileEncodingError{FileType: meta.FileTypeFlac, Source: &meta.UnsupportedTagError{}}
		}
		if err := t.SaveToVerified(meta.NewVerifiedFileOf(meta.FileTypeFlac, file), writeOptions); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &meta.FileEncodingError{FileType: meta.FileTypeFlac, Source: err}
		}
	}
	if vc, ok := f.VorbisComments.Get(); ok {
		// Rust's `FlacFile::write_to` writes the stored (key, value) pairs
		// verbatim (vendor separate, no synthesized ENCODER comment) and
		// chains the tag's pictures with the file's pictures.
		pics := make([]meta.PictureInfoPair, 0, len(vc.Pictures())+len(f.pictures))
		pics = append(pics, vc.Pictures()...)
		pics = append(pics, f.pictures...)
		return flacWriteVorbisCommentsPics(file, vc, pics, writeOptions)
	}
	if len(f.pictures) == 0 {
		// Rust's write_to is a no-op when the tag is absent and there are no
		// pictures to store; it does not strip the existing VC block.
		return nil
	}
	// A dummy VC tag carrying only the file's pictures (vendor "").
	return flacWriteVorbisCommentsPics(file, vorbis.NewVorbisCommentsTag(), f.pictures, writeOptions)
}

// RemoveFromPath removes a tag type.
func (f *FlacFile) RemoveFromPath(path string, tagType meta.TagType) error {
	return meta.RemoveTagFromPath(path, tagType)
}

// flacScanMetadata rewinds the file past any leading ID3v2 tag and scans the
// FLAC metadata blocks, keeping every block except VORBIS_COMMENT and PICTURE
// verbatim. It mirrors the shared preamble of Rust's `flac::write::write_to_inner`.
type flacScanMetadata struct {
	metaStart         int64
	metaEnd           int64
	kept              [][]byte // raw marshaled blocks to preserve (without last flag)
	hasPadding        bool
	hasBlocksToRemove bool
	vendor            string
	foundVC           bool
}

func flacWriteScan(file meta.FileLike, parseOptions meta.ParseOptions) (*flacScanMetadata, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, &meta.FileEncodingError{Source: err}
	}
	// We don't need the ID3v2 tag, but locating it leaves the reader at its end
	if _, err := id3v2.LocateId3v2(file, false, parseOptions); err != nil {
		return nil, &meta.FileEncodingError{Source: err}
	}

	// Rust maps these through `FileParseError`, which converts into a
	// `FileEncodingError` wrapping it.
	scanErr := func(src error) error {
		var fe *FlacParseError
		if !errors.As(src, &fe) {
			src = &FlacParseError{Source: src}
		}
		return &meta.FileEncodingError{
			FileType: meta.FileTypeFlac,
			Source:   &meta.FileParseError{FileType: meta.FileTypeFlac, Source: src},
		}
	}

	streamInfo, err := verifyFlac(file)
	if err != nil {
		return nil, scanErr(err)
	}

	scan := &flacScanMetadata{
		metaStart: int64(streamInfo.start),
		metaEnd:   int64(streamInfo.end),
		// The last block is determined later
		kept: [][]byte{streamInfo.marshal(false)},
	}

	last := streamInfo.last
	for !last {
		// PICTURE payloads are never buffered, they are all removed.
		block, err := readFlacBlock(file, func(ty byte) bool {
			switch ty {
			case blockIDPicture:
				scan.hasBlocksToRemove = true
				return false
			case blockIDPadding:
				scan.hasPadding = true
			}
			return true
		})
		if err != nil {
			return nil, scanErr(err)
		}
		last = block.last
		scan.metaEnd = int64(block.end)

		switch block.ty {
		case blockIDPicture:
			continue
		case blockIDVorbisComment:
			// Retain the original vendor string (Rust errors on a truncated
			// or out-of-range vendor length and ignores invalid UTF-8).
			scan.hasBlocksToRemove = true
			scan.foundVC = true
			data := block.content
			if len(data) < 4 {
				return nil, &meta.FileEncodingError{Source: io.ErrUnexpectedEOF}
			}
			vlen := binary.LittleEndian.Uint32(data)
			if uint64(vlen) > uint64(len(data)-4) {
				return nil, &meta.FileEncodingError{Source: &meta.SizeMismatchError{}}
			}
			if vb := data[4 : 4+int(vlen)]; utf8.Valid(vb) {
				scan.vendor = string(vb)
			} else {
				// TODO: Error on strict? (Rust warning only)
				scan.vendor = ""
			}
			continue
		}
		// Keep the remaining blocks (SEEKTABLE, PADDING, etc.) verbatim
		scan.kept = append(scan.kept, block.marshal(false))
	}
	return scan, nil
}

// flacWriteTag replaces the VORBIS_COMMENT + PICTURE blocks in a FLAC file,
// preserving all other metadata blocks and the audio frames.
func flacWriteTag(file meta.FileLike, tag *meta.Tag, writeOptions meta.WriteOptions) error {
	pics := make([]meta.PictureInfoPair, 0, len(tag.Pictures()))
	for _, p := range tag.Pictures() {
		info, err := meta.PictureInformationFromPicture(&p)
		if err != nil {
			info = meta.PictureInformation{}
		}
		pics = append(pics, meta.PictureInfoPair{Picture: p, Info: info})
	}
	return flacWriteTagPics(file, tag, pics, writeOptions)
}

// flacWriteTagPics is [flacWriteTag] with explicit [meta.PictureInformation] for
// each picture block.
func flacWriteTagPics(file meta.FileLike, tag *meta.Tag, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	scan, err := flacWriteScan(file, writeOptions.ParseOptions)
	if err != nil {
		return err
	}
	vc := vorbis.VorbisCommentsTagForWritePics(tag, "", false)
	// Rust's `write_to_inner` replaces the given vendor with the file's own
	// when a VORBIS_COMMENT block already exists.
	if scan.foundVC {
		vc = vc.WithVendor(scan.vendor)
	}
	// Rust's `has_comments` counts only the item iterator — a tag containing
	// only pictures writes no VC block.
	writeVC := len(vc.Items()) > 0
	if !scan.hasBlocksToRemove && !writeVC && len(pics) == 0 {
		return nil
	}
	return flacWriteBlocksAt(file, scan, vc, writeVC, pics, writeOptions)
}

// flacWriteVorbisComments writes a concrete [vorbis.VorbisCommentsTag] into a FLAC
// file, mirroring Rust `flac::write::write_to_inner`.
func flacWriteVorbisComments(file meta.FileLike, vc *vorbis.VorbisCommentsTag, writeOptions meta.WriteOptions) error {
	return flacWriteVorbisCommentsPics(file, vc, vc.Pictures(), writeOptions)
}

// flacWriteVorbisCommentsPics is [flacWriteVorbisComments] with an explicit
// picture list (the stored comments are written verbatim, pictures separately).
func flacWriteVorbisCommentsPics(file meta.FileLike, vc *vorbis.VorbisCommentsTag, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	scan, err := flacWriteScan(file, writeOptions.ParseOptions)
	if err != nil {
		return err
	}
	if scan.foundVC {
		vc = vc.WithVendor(scan.vendor)
	}
	writeVC := len(vc.Items()) > 0
	if !scan.hasBlocksToRemove && !writeVC && len(pics) == 0 {
		return nil
	}
	return flacWriteBlocksAt(file, scan, vc, writeVC, pics, writeOptions)
}

func flacWriteBlocksAt(file meta.FileLike, scan *flacScanMetadata, vc *vorbis.VorbisCommentsTag, writeVC bool, pics []meta.PictureInfoPair, writeOptions meta.WriteOptions) error {
	// FLAC stores its comments in a block whose body excludes pictures (those
	// get their own PICTURE blocks), unlike Ogg's METADATA_BLOCK_PICTURE.
	vcData, err := vorbis.EncodeVorbisCommentsItems(vc)
	if err != nil {
		return &meta.FileEncodingError{Source: &meta.TagEncodingError{TagType: meta.TagTypeVorbisComments, HasTagType: true, Source: err}}
	}
	kept := scan.kept
	if writeVC {
		if len(vcData) > flacMaxBlockSize {
			return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
		}
		kept = append(kept, (&flacBlock{ty: blockIDVorbisComment, content: vcData}).marshal(false))
	}
	for _, pic := range pics {
		data := pic.Picture.AsFlacBytes(pic.Info, false)
		if len(data) > flacMaxBlockSize {
			return &meta.FileEncodingError{Source: &meta.TooMuchDataError{}}
		}
		kept = append(kept, (&flacBlock{ty: blockIDPicture, content: data}).marshal(false))
	}

	// If the file lacks a PADDING block and padding is preferred, add one last
	if !scan.hasPadding && writeOptions.PreferredPadding > 0 {
		var blocksLen uint64
		for _, b := range kept {
			blocksLen += uint64(len(b))
		}
		oldMetaLen := uint64(scan.metaEnd - scan.metaStart)
		var available uint64
		if oldMetaLen > blocksLen+4 {
			available = oldMetaLen - blocksLen - 4
		}
		padLen := available
		if uint64(writeOptions.PreferredPadding) > padLen {
			padLen = uint64(writeOptions.PreferredPadding)
		}
		if padLen > flacMaxBlockSize {
			padLen = flacMaxBlockSize
		}
		padding, err := util.TryByteSlice(int(padLen))
		if err != nil {
			return &meta.FileEncodingError{FileType: meta.FileTypeFlac, Source: err}
		}
		// `PADDING` always goes last
		kept = append(kept, (&flacBlock{ty: blockIDPadding, content: padding}).marshal(false))
	}

	// Assemble the new metadata region
	var metaBuf bytes.Buffer
	for i, block := range kept {
		if i == len(kept)-1 {
			block[0] |= 0x80 // last flag
		}
		metaBuf.Write(block)
	}

	return meta.Splice(file, uint64(scan.metaStart), uint64(scan.metaEnd), metaBuf.Bytes())
}

func init() {
	meta.RegisterFormatReader(meta.FileTypeFlac, func(rs io.ReadSeeker, opts meta.ParseOptions) (*meta.TaggedFile, error) {
		f, err := FlacFileReadFrom(rs, opts)
		if err != nil {
			return nil, err
		}
		return f.TaggedFile(), nil
	})
	meta.RegisterFormatWriter(meta.FileTypeFlac, func(file *meta.VerifiedFile, tag *meta.Tag, writeOptions meta.WriteOptions) error {
		f := file.File()
		switch tag.TagType() {
		case meta.TagTypeVorbisComments:
			return flacWriteTag(f, tag, writeOptions)
		case meta.TagTypeID3v2:
			// Rust's FLAC writer treats ID3v2 as removable-only: it writes an
			// empty tag, which strips any existing one.
			return id3v2.WriteId3v2(f, meta.NewTag(meta.TagTypeID3v2), writeOptions)
		}
		return &meta.FileEncodingError{Source: &meta.UnsupportedTagError{}}
	})
	meta.RegisterConcreteTagWriter(meta.FileTypeFlac, meta.TagTypeVorbisComments, func(file *meta.VerifiedFile, tag any, writeOptions meta.WriteOptions) error {
		return flacWriteVorbisComments(file.File(), tag.(*vorbis.VorbisCommentsTag), writeOptions)
	})
}
