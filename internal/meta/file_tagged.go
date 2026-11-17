package meta

import (
	"io"
	"os"
	"slices"
)

// TaggedFile is a collection of [Tag]s and [FileProperties] parsed from a file.
type TaggedFile struct {
	fileType FileType
	// tags holds the file's tags.
	tags []*Tag
	// properties are the file's audio properties.
	properties FileProperties
}

// NewTaggedFile creates a [TaggedFile] from a file type, tags, and properties.
func NewTaggedFile(fileType FileType, tags []*Tag, properties FileProperties) *TaggedFile {
	return &TaggedFile{fileType: fileType, tags: tags, properties: properties}
}

// FileType returns the [FileType].
func (t *TaggedFile) FileType() FileType { return t.fileType }

// Properties returns the file's audio properties.
func (t *TaggedFile) Properties() FileProperties { return t.properties }

// Tags returns the file's tags.
func (t *TaggedFile) Tags() []*Tag { return t.tags }

// TagTypes returns the file's tag types.
func (t *TaggedFile) TagTypes() []TagType {
	tts := make([]TagType, len(t.tags))
	for i, tag := range t.tags {
		tts[i] = tag.tagType
	}
	return tts
}

// PrimaryTag returns the file's primary [TagType].
//
// A file's primary tag type is determined by its format.
func (t *TaggedFile) PrimaryTag() *Tag {
	primary := t.fileType.PrimaryTagType()
	for _, tag := range t.tags {
		if tag.tagType == primary {
			return tag
		}
	}
	return nil
}

// PrimaryTagMut returns the file's primary tag, or nil if it doesn't exist.
func (t *TaggedFile) PrimaryTagMut() *Tag { return t.PrimaryTag() }

// PrimaryTagMutOrInsert inserts a new tag of the file's primary tag type if it
// doesn't exist, otherwise returns the existing tag.
func (t *TaggedFile) PrimaryTagMutOrInsert() *Tag {
	if tag := t.PrimaryTag(); tag != nil {
		return tag
	}
	tag := NewTag(t.fileType.PrimaryTagType())
	t.tags = append(slices.Clip(t.tags), tag)
	return tag
}

// FirstTag returns the first tag in the file.
func (t *TaggedFile) FirstTag() *Tag {
	if len(t.tags) == 0 {
		return nil
	}
	return t.tags[0]
}

// Tag returns the first tag of a specific [TagType].
func (t *TaggedFile) Tag(tagType TagType) *Tag {
	for _, tag := range t.tags {
		if tag.tagType == tagType {
			return tag
		}
	}
	return nil
}

// ContainsTag reports whether the file contains a tag of the specified [TagType].
func (t *TaggedFile) ContainsTag(tagType TagType) bool { return t.Tag(tagType) != nil }

// InsertTag inserts a [Tag] into the file.
//
// This will replace any tag of the same type, and the replaced tag is
// returned. The tag is appended after the remaining tags.
//
// NOTE: This does nothing (and returns nil) if the [FileType] cannot read the
// [TagType]. See [FileType.TagSupport].
func (t *TaggedFile) InsertTag(tag *Tag) *Tag {
	if !t.fileType.TagSupport(tag.tagType).IsReadable() {
		return nil
	}
	replaced := t.removeFirst(tag.tagType)
	t.tags = append(slices.Clip(t.tags), tag)
	return replaced
}

// removeFirst removes and returns the first tag of the given type.
func (t *TaggedFile) removeFirst(tagType TagType) *Tag {
	for i, tag := range t.tags {
		if tag.tagType == tagType {
			kept := make([]*Tag, 0, len(t.tags)-1)
			kept = append(kept, t.tags[:i]...)
			kept = append(kept, t.tags[i+1:]...)
			t.tags = kept
			return tag
		}
	}
	return nil
}

// Remove removes the provided tag, returning true if it was found.
//
// The tag list is rebuilt rather than edited in place, so slices previously
// returned by [TaggedFile.Tags] (or shared with a copy of the file) are not
// modified.
func (t *TaggedFile) Remove(target *Tag) bool {
	for i, tag := range t.tags {
		if tag == target {
			kept := make([]*Tag, 0, len(t.tags)-1)
			kept = append(kept, t.tags[:i]...)
			kept = append(kept, t.tags[i+1:]...)
			t.tags = kept
			return true
		}
	}
	return false
}

// RemoveTagType removes the first tag of the specified [TagType], returning
// it, or nil if the file has no such tag (Rust `TaggedFileExt::remove`).
func (t *TaggedFile) RemoveTagType(tagType TagType) *Tag {
	return t.removeFirst(tagType)
}

// RemoveTags removes all tags of the specified [TagType].
func (t *TaggedFile) RemoveTags(tagType TagType) {
	kept := make([]*Tag, 0, len(t.tags))
	for _, tag := range t.tags {
		if tag.tagType != tagType {
			kept = append(kept, tag)
		}
	}
	t.tags = kept
}

// Clear clears the tags from the file, retaining only the [FileProperties].
func (t *TaggedFile) Clear() { t.tags = nil }

// SupportsTagType reports whether the [FileType] supports the [TagType].
func (t *TaggedFile) SupportsTagType(tagType TagType) bool {
	return t.fileType.TagSupport(tagType).IsReadable()
}

// ChangeFileType changes the [FileType] of the [TaggedFile].
//
// NOTES:
//   - This removes any tag the new format cannot read. See [FileType.TagSupport].
//   - This resets the [FileProperties].
func (t *TaggedFile) ChangeFileType(fileType FileType) {
	t.fileType = fileType
	t.properties = FileProperties{}
	kept := make([]*Tag, 0, len(t.tags))
	for _, tag := range t.tags {
		if fileType.TagSupport(tag.tagType).IsReadable() {
			kept = append(kept, tag)
		}
	}
	t.tags = kept
}

// Conflicts returns all pairs of conflicting [TagType]s present in the file.
//
// Each conflicting pair is reported once, with the tag that comes first in the
// file first.
func (t *TaggedFile) Conflicts() [][2]TagType {
	var conflicts [][2]TagType
	for i, tag := range t.tags {
		for _, other := range t.tags[i+1:] {
			if tag.tagType.ConflictsWith(other.tagType) {
				conflicts = append(conflicts, [2]TagType{tag.tagType, other.tagType})
			}
		}
	}
	return conflicts
}

// Save writes all tags to the file.
//
// Like Rust's `TaggedFile::save_to`, the destination's format is determined
// from its content for every tag, so a tag is only written if the real
// destination format supports it (otherwise an [UnsupportedTagError] is
// returned).
func (t *TaggedFile) Save(file FileLike, writeOptions WriteOptions) error {
	// Rust's save_to refuses to write when conflicting tag types are present
	if conflicts := t.Conflicts(); len(conflicts) > 0 {
		return &FileEncodingError{Source: &ConflictingTagError{First: conflicts[0][0], Second: conflicts[0][1]}}
	}
	for _, tag := range t.tags {
		// Users aren't necessarily aware of any read-only tags they happen to
		// read; skip them rather than error (Rust `save_to`).
		if !t.fileType.TagSupport(tag.tagType).IsWritable() {
			continue
		}
		// NewVerifiedFile rewinds the file and probes its real format.
		vf, err := NewVerifiedFile(file, writeOptions.ParseOptions)
		if err != nil {
			return &FileEncodingError{Source: err}
		}
		if err := WriteTag(tag, vf, writeOptions); err != nil {
			return err
		}
		// Rewind so the next iteration starts from the beginning, and so the
		// caller's handle is left in a usable state (Rust consumes the handle,
		// so this is unobservable there).
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	return nil
}

// SaveToPath writes all tags to the file at `path`, which is opened for
// reading and writing (Rust `AudioFile::save_to_path`).
//
// [BoundTaggedFile] inherits this method, which, like Rust, writes to `path`
// rather than to its bound file.
func (t *TaggedFile) SaveToPath(path string, writeOptions WriteOptions) (err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = &FileEncodingError{Source: cerr}
		}
	}()
	return t.Save(NewOSFile(f), writeOptions)
}

// BoundTaggedFile is a [TaggedFile] that is bound to a [FileLike].
type BoundTaggedFile struct {
	TaggedFile
	file FileLike
}

// NewBoundTaggedFile creates a new [BoundTaggedFile] from a [FileLike].
func NewBoundTaggedFile(file FileLike, taggedFile *TaggedFile) *BoundTaggedFile {
	return &BoundTaggedFile{TaggedFile: *taggedFile, file: file}
}

// BoundTaggedFileFrom creates a [BoundTaggedFile] by reading from a [FileLike].
func BoundTaggedFileFrom(file FileLike, parseOptions ParseOptions) (*BoundTaggedFile, error) {
	taggedFile, err := readTaggedFileFromFileLike(file, parseOptions)
	if err != nil {
		return nil, err
	}
	return newBoundTaggedFile(file, taggedFile)
}

// newBoundTaggedFile rewinds `file` (like Rust's `BoundTaggedFile::read_from`)
// and binds it to `taggedFile`.
func newBoundTaggedFile(file FileLike, taggedFile *TaggedFile) (*BoundTaggedFile, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, &FileParseError{FileType: taggedFile.fileType, Source: err}
	}
	return &BoundTaggedFile{TaggedFile: *taggedFile, file: file}, nil
}

// IntoInner returns the underlying [FileLike].
func (b *BoundTaggedFile) IntoInner() FileLike { return b.file }

// Save writes all tags to the bound file.
func (b *BoundTaggedFile) Save(writeOptions WriteOptions) error {
	if err := b.TaggedFile.Save(b.file, writeOptions); err != nil {
		return err
	}
	// Rust `BoundTaggedFile::save` drops emptied tags afterwards.
	kept := make([]*Tag, 0, len(b.tags))
	for _, tag := range b.tags {
		if !tag.IsEmpty() {
			kept = append(kept, tag)
		}
	}
	b.tags = kept
	return nil
}
