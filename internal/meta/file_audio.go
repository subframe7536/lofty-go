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

// SaveToPath opens `path` and saves every tag of `f` to it, mirroring Rust's
// `AudioFile::save_to_path`.
func SaveToPath(path string, f AudioFile, writeOptions WriteOptions) error {
	osFile, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	err = f.SaveFile(NewOSFile(osFile), writeOptions)
	if cerr := osFile.Close(); cerr != nil && err == nil {
		return &FileEncodingError{Source: cerr}
	}
	return err
}

// RemoveTagFromPath removes `tagType` from the file at `path`, taking the file's
// format from the path's extension like Rust's `TagType::remove_from_path`.
func RemoveTagFromPath(path string, tagType TagType) error {
	return tagType.RemoveFromPath(path, NewWriteOptions())
}
