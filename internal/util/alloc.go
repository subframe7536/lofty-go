package util

// TryMake creates a slice of `size` elements, honoring
// [GlobalOptions.AllocationLimit]. Equivalent to Rust's `try_vec!`.
func TryMake[T any](size int) ([]T, error) {
	if size == 0 {
		return nil, nil
	}
	if size > GetGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, size), nil
}

// TryByteSlice creates a zeroed byte slice of `size` honoring the allocation limit.
func TryByteSlice(size int) ([]byte, error) { return TryMake[byte](size) }

// TryCapSlice creates an empty slice with the given capacity honoring the allocation limit.
func TryCapSlice[T any](capacity int) ([]T, error) {
	if capacity > GetGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, 0, capacity), nil
}
