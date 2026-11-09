package lofty

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

// FakeTagError arises when a tag is expected (e.g. an "ID3 " chunk in a WAV file), but isn't found.
type FakeTagError struct{}

func (e *FakeTagError) Error() string { return "expected a tag, found invalid data" }

// TooMuchDataError arises when attempting to read/write an abnormally large amount of data.
type TooMuchDataError struct{}

func (e *TooMuchDataError) Error() string {
	return "attempted to read/write an abnormally large amount of data"
}

// NotEnoughDataError arises when attempting to parse an item, but there isn't enough data available.
type NotEnoughDataError struct {
	// Expected is the expected size in bytes, 0 if unknown.
	Expected    int
	HasExpected bool
}

func newNotEnoughDataError(expected int) *NotEnoughDataError {
	return &NotEnoughDataError{Expected: expected, HasExpected: true}
}

var errNotEnoughData = &NotEnoughDataError{}

func (e *NotEnoughDataError) Error() string {
	if e.HasExpected {
		return fmt.Sprintf("not enough data in reader (expected %d bytes)", e.Expected)
	}
	return "not enough data in reader"
}

// SizeMismatchError arises when the size of an item is written as one value, but that size is
// either too big or small to be valid within the bounds of that item.
type SizeMismatchError struct{}

func (e *SizeMismatchError) Error() string {
	return "encountered an invalid item size, either too big or too small to be valid"
}

// UnknownFormatError arises when the format of the input cannot be guessed.
type UnknownFormatError struct{}

func (e *UnknownFormatError) Error() string {
	return "no format could be determined from the provided file"
}

// UnsupportedTagError arises when writing a tag to a [FileType] that doesn't support it.
type UnsupportedTagError struct{}

func (e *UnsupportedTagError) Error() string {
	return "attempted to write a tag to a format that does not support it"
}

// AllocationErrorKind describes why an allocation failed.
type AllocationErrorKind int

const (
	// AllocationLimitExceeded means the requested allocation exceeds
	// [GlobalOptions.AllocationLimit].
	AllocationLimitExceeded AllocationErrorKind = iota
	// AllocationReserveError means the requested allocation could not be reserved.
	AllocationReserveError
)

// AllocationError is returned when a memory allocation fails.
type AllocationError struct {
	Kind AllocationErrorKind
	Err  error
}

func (e *AllocationError) Error() string {
	if e.Kind == AllocationLimitExceeded {
		return "attempted allocation would exceed global limits"
	}
	return e.Err.Error()
}

func (e *AllocationError) Unwrap() error { return e.Err }
