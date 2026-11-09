package lofty

import "io"

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
	// Empty tags are not reported (matches Rust's retain(!is_empty)).
	var kept []*Tag
	for _, tag := range tags {
		if tag != nil && !tag.IsEmpty() {
			kept = append(kept, tag)
		}
	}
	return &TaggedFile{fileType: fileType, tags: kept, properties: properties}
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

// PrimaryTagMut returns the file's primary tag, creating it if missing.
func (t *TaggedFile) PrimaryTagMut() *Tag {
	if tag := t.PrimaryTag(); tag != nil {
		return tag
	}
	tag := NewTag(t.fileType.PrimaryTagType())
	t.tags = append(t.tags, tag)
	return tag
}

// PrimaryTagMutOrInsert inserts a new tag of the file's primary tag type if it
// doesn't exist, otherwise returns the existing tag.
//
// This is equivalent to [TaggedFile.PrimaryTagMut].
func (t *TaggedFile) PrimaryTagMutOrInsert() *Tag { return t.PrimaryTagMut() }

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
// This will replace any tag of the same type.
func (t *TaggedFile) InsertTag(tag *Tag) {
	t.RemoveTagType(tag.tagType)
	t.tags = append(t.tags, tag)
}

// Remove removes the provided tag, returning true if it was found.
func (t *TaggedFile) Remove(target *Tag) bool {
	for i, tag := range t.tags {
		if tag == target {
			t.tags = append(t.tags[:i], t.tags[i+1:]...)
			return true
		}
	}
	return false
}

// RemoveTagType removes the first tag of the specified [TagType].
func (t *TaggedFile) RemoveTagType(tagType TagType) {
	for i, tag := range t.tags {
		if tag.tagType == tagType {
			t.tags = append(t.tags[:i], t.tags[i+1:]...)
			return
		}
	}
}

// RemoveTags removes all tags of the specified [TagType].
func (t *TaggedFile) RemoveTags(tagType TagType) {
	kept := t.tags[:0]
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
// NOTE: This is a no-op for [FileType]s that don't support the tags present.
func (t *TaggedFile) ChangeFileType(fileType FileType) { t.fileType = fileType }

// Conflicts returns all pairs of conflicting [TagType]s present in the file.
func (t *TaggedFile) Conflicts() [][2]TagType {
	var conflicts [][2]TagType
	for i, tag := range t.tags {
		for j, other := range t.tags {
			if i == j {
				continue
			}
			if tag.tagType.ConflictsWith(other.tagType) {
				conflicts = append(conflicts, [2]TagType{tag.tagType, other.tagType})
			}
		}
	}
	return conflicts
}

// Save writes all tags to the file.
func (t *TaggedFile) Save(file FileLike, writeOptions WriteOptions) error {
	// The file type is already known; no need to re-verify
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	vf := &verifiedFile{format: t.fileType, file: file}
	for _, tag := range t.tags {
		if err := writeTag(tag, vf, writeOptions); err != nil {
			return err
		}
		if _, err := vf.file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
	}
	return nil
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
	return &BoundTaggedFile{TaggedFile: *taggedFile, file: file}, nil
}

// IntoInner returns the underlying [FileLike].
func (b *BoundTaggedFile) IntoInner() FileLike { return b.file }

// Save writes all tags to the bound file.
func (b *BoundTaggedFile) Save(writeOptions WriteOptions) error {
	return b.TaggedFile.Save(b.file, writeOptions)
}
