package meta

import (
	"errors"
	"fmt"
)

// FileParseError is returned when parsing a file fails.
type FileParseError struct {
	// FileType is the type of the file that failed to parse.
	// The zero value indicates an unknown file type.
	FileType FileType
	// Source is the underlying error.
	Source error
}

// IsUnknownFormat reports whether this error represents an [UnknownFormatError].
func (e *FileParseError) IsUnknownFormat() bool {
	var u *UnknownFormatError
	return errors.As(e.Source, &u)
}

func (e *FileParseError) Error() string {
	if e.FileType.kind != 0 {
		return fmt.Sprintf("failed to parse %s file", e.FileType)
	}
	return "failed to parse file"
}

func (e *FileParseError) Unwrap() error { return e.Source }

// FileEncodingError is returned when writing a file fails.
type FileEncodingError struct {
	// FileType is the type of the file that failed to write.
	FileType FileType
	// Source is the underlying error.
	Source error
}

func (e *FileEncodingError) Error() string {
	if e.FileType.kind != 0 {
		return fmt.Sprintf("failed to write %s file", e.FileType)
	}
	return "failed to write to file"
}

func (e *FileEncodingError) Unwrap() error { return e.Source }

func (e *FileEncodingError) withFormat(ty FileType) *FileEncodingError {
	e.FileType = ty
	return e
}

// ConflictingTagError is returned when attempting to save conflicting tag types to the same file.
//
// See [TagType.ConflictsWith].
type ConflictingTagError struct {
	First, Second TagType
}

func (e *ConflictingTagError) Error() string {
	return fmt.Sprintf("tag type `%s` conflicts with `%s`", e.First, e.Second)
}

// TagParseError is returned when parsing a tag fails.
type TagParseError struct {
	// TagType is the type of tag that failed to parse.
	TagType TagType
	// HasTagType is false when the tag type is unknown.
	HasTagType bool
	// Source is the underlying error.
	Source error
}

func (e *TagParseError) Error() string {
	if e.HasTagType {
		return fmt.Sprintf("failed to parse %s tag", e.TagType)
	}
	return "failed to parse tag"
}

func (e *TagParseError) Unwrap() error { return e.Source }

// TagEncodingError is returned when writing a tag fails.
type TagEncodingError struct {
	// TagType is the type of tag that failed to write.
	TagType TagType
	// HasTagType is false when the tag type is unknown.
	HasTagType bool
	// Source is the underlying error.
	Source error
}

func (e *TagEncodingError) Error() string {
	if e.HasTagType {
		return fmt.Sprintf("failed to write %s tag", e.TagType)
	}
	return "failed to write tag"
}

func (e *TagEncodingError) Unwrap() error { return e.Source }
