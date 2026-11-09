package meta

import "os"

// AudioFile is the shared interface of all audio file types.
type AudioFile interface {
	// FileType returns the file's type.
	FileType() FileType
	// GetProperties returns the audio properties.
	GetProperties() *FileProperties
	// ContainsTag reports whether any tag is present.
	ContainsTag() bool
	// ContainsTagType reports whether a tag of the given type is present.
	ContainsTagType(tagType TagType) bool
	// TaggedFile converts the file to a generic [TaggedFile].
	TaggedFile() *TaggedFile
	// SaveFile writes the file's tags to the given [FileLike].
	SaveFile(file FileLike, writeOptions WriteOptions) error
	// SavePath saves the file's tags to `path`.
	SavePath(path string, writeOptions WriteOptions) error
	// RemoveFromPath removes a [TagType] from the file at `path`.
	RemoveFromPath(path string, tagType TagType) error
}

// SaveToPath opens `path` and writes `tag` of `tagType` via the file's write path.
func SaveToPath(path string, f AudioFile, tagType TagType, writeOptions WriteOptions) error {
	osFile, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer osFile.Close()
	verified, err := NewVerifiedFile(NewOSFile(osFile), writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	tf := f.TaggedFile()
	tag := tf.Tag(tagType)
	if tag == nil {
		tag = NewTag(tagType)
	}
	return WriteTag(tag, verified, writeOptions)
}

// RemoveTagFromPath removes `tagType` from the file at `path`.
func RemoveTagFromPath(path string, tagType TagType) error {
	osFile, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer osFile.Close()
	verified, err := NewVerifiedFile(NewOSFile(osFile), NewParseOptions())
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return RemoveTag(tagType, verified, NewWriteOptions())
}
