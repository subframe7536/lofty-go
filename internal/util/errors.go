package util

import "fmt"

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

func NewNotEnoughDataError(expected int) *NotEnoughDataError {
	return &NotEnoughDataError{Expected: expected, HasExpected: true}
}

var ErrNotEnoughData = &NotEnoughDataError{}

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
	if e.Err == nil {
		return "failed to reserve memory for allocation"
	}
	return e.Err.Error()
}

func (e *AllocationError) Unwrap() error { return e.Err }
