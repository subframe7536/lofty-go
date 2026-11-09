package lofty

import (
	"io"
	"sync"
)

// FileResolver is the interface for custom file resolvers.
//
// See [RegisterCustomResolver].
type FileResolver interface {
	// Extension returns the extension of the file type, without the leading dot.
	Extension() string
	// Guess attempts to determine whether `buf` is of the custom file type.
	Guess(buf []byte) bool
	// PrimaryTagType returns the resolver's primary [TagType].
	PrimaryTagType() TagType
	// TagSupport describes how the resolver supports `tagType`.
	TagSupport(tagType TagType) TagSupport
	// ReadFrom reads a [TaggedFile] from the reader.
	ReadFrom(reader io.ReadSeeker, options ParseOptions) (*TaggedFile, error)
}

var (
	customResolversMu sync.RWMutex
	customResolvers   = map[string]FileResolver{}
)

// RegisterCustomResolver registers a [FileResolver].
//
// A [FileResolver] is used when the provided file type is not one of the
// supported [FileType] variants. These resolvers can be disabled globally with
// [GlobalOptions.UseCustomResolvers].
func RegisterCustomResolver(name string, resolver FileResolver) {
	customResolversMu.Lock()
	defer customResolversMu.Unlock()
	if _, exists := customResolvers[name]; exists {
		panic("lofty: resolver `" + name + "` already exists!")
	}
	customResolvers[name] = resolver
}

// UnregisterCustomResolver removes a previously registered custom resolver.
func UnregisterCustomResolver(name string) {
	customResolversMu.Lock()
	defer customResolversMu.Unlock()
	delete(customResolvers, name)
}

// lookupResolver returns the resolver for a custom file type name.
// If unregistered, it panics — matching the Rust API's contract that
// FileType::Custom requires a registered resolver.
func lookupResolver(name string) FileResolver {
	customResolversMu.RLock()
	defer customResolversMu.RUnlock()
	if r, ok := customResolvers[name]; ok {
		return r
	}
	panic("lofty: no registered custom resolver for file type " + name)
}

func lookupResolverByExtension(ext string) (FileType, bool) {
	customResolversMu.RLock()
	defer customResolversMu.RUnlock()
	for name, r := range customResolvers {
		if r.Extension() == ext {
			return FileTypeCustom(name), true
		}
	}
	return FileType{}, false
}

func guessCustomResolvers(buf []byte) (FileType, bool) {
	customResolversMu.RLock()
	defer customResolversMu.RUnlock()
	for name, r := range customResolvers {
		if r.Guess(buf) {
			return FileTypeCustom(name), true
		}
	}
	return FileType{}, false
}
