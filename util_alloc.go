package lofty

// tryMake creates a slice of `size` elements, honoring
// [GlobalOptions.AllocationLimit]. Equivalent to Rust's `try_vec!`.
func tryMake[T any](size int) ([]T, error) {
	if size == 0 {
		return nil, nil
	}
	if size > getGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, size), nil
}

// tryByteSlice creates a zeroed byte slice of `size` honoring the allocation limit.
func tryByteSlice(size int) ([]byte, error) { return tryMake[byte](size) }

// tryCapSlice creates an empty slice with the given capacity honoring the allocation limit.
func tryCapSlice[T any](capacity int) ([]T, error) {
	if capacity > getGlobalOptions().AllocationLimit {
		return nil, &AllocationError{Kind: AllocationLimitExceeded}
	}
	return make([]T, 0, capacity), nil
}
