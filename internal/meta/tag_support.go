package meta

// TagSupport describes how a [TagType] is supported in a given [FileType].
//
// See [FileType.TagSupport].
type TagSupport int

const (
	// TagSupportUnsupported means the tag isn't supported in this [FileType].
	TagSupportUnsupported TagSupport = iota
	// TagSupportReadOnly means the tag type can be *read* from this [FileType], but cannot be
	// written back to it.
	//
	// For example, ID3v2 tags can be read from, but not written to FLAC files.
	TagSupportReadOnly
	// TagSupportReadWrite means the tag type can be both read from and written to this [FileType].
	TagSupportReadWrite
)

// IsReadable reports whether the tag type can be read from the file.
// This is true for both [TagSupportReadOnly] and [TagSupportReadWrite].
func (s TagSupport) IsReadable() bool {
	return s == TagSupportReadOnly || s == TagSupportReadWrite
}

// IsWritable reports whether the tag type can be written to the file.
// This is only true for [TagSupportReadWrite].
func (s TagSupport) IsWritable() bool { return s == TagSupportReadWrite }

// TagType is the tag's format.
type TagType uint8

const (
	// TagTypeApe covers both APEv1 and APEv2 as it doesn't matter much.
	TagTypeApe TagType = iota
	// TagTypeID3v1 represents an ID3v1 tag.
	TagTypeID3v1
	// TagTypeID3v2 covers all ID3v2 versions since they all get upgraded to ID3v2.4.
	TagTypeID3v2
	// TagTypeLyrics3 represents a Lyrics3 (v1) tag.
	TagTypeLyrics3
	// TagTypeLyrics3v2 represents a Lyrics3v2 tag.
	TagTypeLyrics3v2
	// TagTypeMp4Ilst represents an MP4 ilst atom.
	TagTypeMp4Ilst
	// TagTypeVorbisComments represents vorbis comments.
	TagTypeVorbisComments
	// TagTypeRiffInfo represents a RIFF INFO LIST.
	TagTypeRiffInfo
	// TagTypeAiffText represents AIFF text chunks.
	TagTypeAiffText
)

// TagTypeVariants contains all [TagType] variants.
var TagTypeVariants = []TagType{
	TagTypeApe,
	TagTypeID3v1,
	TagTypeID3v2,
	TagTypeLyrics3,
	TagTypeLyrics3v2,
	TagTypeMp4Ilst,
	TagTypeVorbisComments,
	TagTypeRiffInfo,
	TagTypeAiffText,
}

func (t TagType) String() string {
	switch t {
	case TagTypeApe:
		return "Ape"
	case TagTypeID3v1:
		return "Id3v1"
	case TagTypeID3v2:
		return "Id3v2"
	case TagTypeLyrics3:
		return "Lyrics3"
	case TagTypeLyrics3v2:
		return "Lyrics3v2"
	case TagTypeMp4Ilst:
		return "Mp4Ilst"
	case TagTypeVorbisComments:
		return "VorbisComments"
	case TagTypeRiffInfo:
		return "RiffInfo"
	case TagTypeAiffText:
		return "AiffText"
	}
	return "Unknown"
}

// ConflictsWith reports whether this [TagType] conflicts with `other`.
//
// Conflicting tag types cannot be written to the same file. One will need to be removed.
func (t TagType) ConflictsWith(other TagType) bool {
	// Only Lyrics3 and Lyrics3v2 conflict, and the relationship is commutative.
	return (t == TagTypeLyrics3 && other == TagTypeLyrics3v2) ||
		(t == TagTypeLyrics3v2 && other == TagTypeLyrics3)
}

// RemoveFromPath removes a tag of this type from a path.
//
// Like Rust's `TagType::remove_from_path`, the file's format is taken from the
// path's extension (no content sniffing).
func (t TagType) RemoveFromPath(path string, writeOptions WriteOptions) error {
	vf, f, err := VerifiedFileFromPathByExtension(path)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer f.Close()
	return t.doRemove(vf, writeOptions)
}

// RemoveFrom removes a tag of this type from a [FileLike].
func (t TagType) RemoveFrom(file FileLike, writeOptions WriteOptions) error {
	vf, err := NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return t.doRemove(vf, writeOptions)
}

func (t TagType) doRemove(file *VerifiedFile, writeOptions WriteOptions) error {
	// Read-only tags are always removable
	if !file.Format().TagSupport(t).IsReadable() {
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	return WriteTag(NewTag(t), file, writeOptions)
}
