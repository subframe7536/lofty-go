package util

// TryMake creates a slice of `size` elements, honoring
// [GlobalOptions.AllocationLimit]. Equivalent to Rust's `try_vec!`.
//
// A negative size is reported as [AllocationLimitExceeded] (it can never be a
// valid length; Rust's `usize` cannot express it), so it never panics.
func TryMake[T any](size int) ([]T, error) {
	if size == 0 {
		return nil, nil
	}
	if size < 0 || size > GetGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, size), nil
}

// TryByteSlice creates a zeroed byte slice of `size` honoring the allocation limit.
func TryByteSlice(size int) ([]byte, error) { return TryMake[byte](size) }

// TryCapSlice creates an empty slice with the given capacity honoring the allocation limit.
// A negative capacity is reported as [AllocationLimitExceeded], like [TryMake].
func TryCapSlice[T any](capacity int) ([]T, error) {
	if capacity < 0 || capacity > GetGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, 0, capacity), nil
}
