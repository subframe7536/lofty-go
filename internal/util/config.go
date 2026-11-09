package util

import "sync"

// ParsingMode is the parsing strictness mode.
//
// This can be set with [Probe.Options].
type ParsingMode int

const (
	// ParsingModeBestAttempt is the default mode, less eager to error on recoverably malformed input.
	//
	// This mode will attempt to fill in any holes where possible in otherwise valid, spec-compliant input.
	//
	// NOTE: A readable input does *not* necessarily make it writable.
	//
	// Examples of behavior:
	// * Unable to decode text - If valid otherwise, the field will be replaced by an empty string and the parser moves on
	// * Unable to determine the sample rate - The sample rate will be 0
	ParsingModeBestAttempt ParsingMode = iota
	// ParsingModeStrict will eagerly error on invalid input.
	//
	// This mode will eagerly error on any non spec-compliant input.
	//
	// Examples of behavior:
	// * Unable to decode text - The parser will error and the entire input is discarded
	// * Unable to determine the sample rate - The parser will error and the entire input is discarded
	ParsingModeStrict
	// ParsingModeRelaxed is the least eager to error, may produce invalid/partial output.
	//
	// This mode will discard any invalid fields, and ignore the majority of non-fatal errors.
	//
	// If the input is malformed, the resulting tags may be incomplete, and the properties zeroed.
	//
	// Examples of behavior:
	// * Unable to decode text - The entire item is discarded and the parser moves on
	// * Unable to determine the sample rate - The sample rate will be 0
	ParsingModeRelaxed
)

// ParseOptions controls how Lofty parses a file.
//
// If you plan to write back to a file, you should reuse these options in
// [WriteOptions.ParseOptions] for consistency.
type ParseOptions struct {
	// ReadProperties controls whether or not to read the audio properties (default true).
	ReadProperties bool
	// ReadTags controls whether or not to read the tags (default true).
	ReadTags bool
	// ParsingMode is the parsing strictness mode (default [ParsingModeBestAttempt]).
	ParsingMode ParsingMode
	// MaxJunkBytes is the maximum number of allowed junk bytes to search.
	//
	// Some information may be surrounded by junk bytes, such as tag padding remnants. This sets the
	// maximum number of junk/unrecognized bytes Lofty will search for required information before
	// giving up (default 1024).
	MaxJunkBytes int
	// ReadCoverArt controls whether or not to read cover art (default true).
	ReadCoverArt bool
	// ImplicitConversions controls whether or not to perform implicit conversions (default true).
	//
	// Implicit conversions are conversions that are not explicitly defined by the spec, but are
	// commonly used.
	//
	// ⚠ **Warning** ⚠
	//
	// Turning this off may cause some [Accessor] methods to return nothing.
	// Lofty makes some assumptions about the data, if they are broken, the caller will have more
	// responsibility.
	//
	// Examples include:
	//
	// * Converting the outdated MP4 `gnre` atom to a `©gen` atom
	// * Combining the ID3v2.3 `TYER`, `TDAT`, and `TIME` frames into a single `TDRC` frame
	//
	// Examples of what this does *not* include:
	//
	// * Converting a Vorbis `COVERART` field to `METADATA_BLOCK_PICTURE`
	//   - This is a non-standard field, with a well-defined conversion. Lofty will not support
	//     the non-standard `COVERART` for [Picture]s.
	ImplicitConversions bool
}

// DefaultMaxJunkBytes is the default number of junk bytes to read.
const DefaultMaxJunkBytes = 1024

// NewParseOptions creates a new [ParseOptions] with default values.
func NewParseOptions() ParseOptions {
	return ParseOptions{
		ReadProperties:      true,
		ReadTags:            true,
		ParsingMode:         ParsingModeBestAttempt,
		MaxJunkBytes:        DefaultMaxJunkBytes,
		ReadCoverArt:        true,
		ImplicitConversions: true,
	}
}

// WriteOptions controls how Lofty writes to a file.
//
// This acts as a dumping ground for all sorts of format-specific settings. As such, this is best
// used as an application global config that gets set once.
type WriteOptions struct {
	// PreferredPadding is the preferred padding size in bytes.
	//
	// If the tag format being written supports padding, this will be the size of the padding
	// in bytes.
	//
	// NOTES:
	// * Not all tag formats support padding
	// * The actual padding size may be different from this value, depending on tag size limitations
	//
	// A value of 0 disables padding entirely (default 1024).
	PreferredPadding uint32

	// RemoveOthers controls whether to remove all other tags when writing.
	//
	// If set to true, only the tag being written will be kept in the file (default false).
	RemoveOthers bool

	// RespectReadOnly controls whether to respect read-only tag items.
	//
	// Some tag formats allow for items to be marked as read-only. If set to `true`, these items
	// will take priority over newly created tag items (default true).
	//
	// NOTE: In the case of APE tags, one can mark the entire tag as read-only. This will append
	// the existing tag items to the new tag.
	RespectReadOnly bool

	// UppercaseId3v2Chunk controls whether to uppercase the ID3v2 chunk name.
	//
	// When dealing with RIFF/AIFF files, some software may expect the ID3v2 chunk name to be
	// lowercase.
	//
	// NOTE: The vast majority of software will be able to read both upper and lowercase
	// chunk names (default true).
	UppercaseId3v2Chunk bool

	// UseID3v23 controls whether or not to use ID3v2.3 when saving [TagTypeId3v2]
	// or [ID3v2Tag].
	//
	// By default, Lofty will save ID3v2.4 tags. This option allows you to save ID3v2.3 tags instead
	// (default false).
	UseID3v23 bool

	// LossyTextEncoding controls whether to replace invalid characters when writing strings.
	//
	// Some tag formats are restricted to certain [TextEncoding]s, which may restrict the available
	// character set.
	//
	// If this is enabled, any invalid characters will be replaced with `'?'` (e.g `lфfty` in
	// [TextEncodingLatin1] will return `l?fty`) (default true).
	//
	// If this is disabled, any writes with non-representable characters will return a
	// [TextEncodingError].
	LossyTextEncoding bool

	// ImplicitTruncation controls whether to truncate text fields when they exceed tag limits.
	//
	// Some tag formats have abnormally small text field limits that may be easy
	// to exceed (e.g., [ID3v1Tag]).
	//
	// If this is enabled, any fields that exceed the limit will simply be truncated to fit
	// (default true).
	//
	// If this is disabled, any writes with exceedingly large text fields will return a
	// [TooMuchDataError].
	ImplicitTruncation bool

	// ParseOptions are the parse options for this write.
	//
	// This *should* be the same set of [ParseOptions] used for the initial read.
	ParseOptions ParseOptions
}

// DefaultPreferredPadding is the default preferred padding size in bytes.
const DefaultPreferredPadding = 1024

// NewWriteOptions creates a new [WriteOptions] with default values.
func NewWriteOptions() WriteOptions {
	return WriteOptions{
		PreferredPadding:    DefaultPreferredPadding,
		RemoveOthers:        false,
		RespectReadOnly:     true,
		UppercaseId3v2Chunk: true,
		UseID3v23:           false,
		LossyTextEncoding:   true,
		ImplicitTruncation:  true,
		ParseOptions:        NewParseOptions(),
	}
}

// GlobalOptions control all interactions with Lofty.
//
// In the Rust version these are thread-local; in Go they are process-global,
// protected by a mutex.
type GlobalOptions struct {
	// UseCustomResolvers controls whether or not to check registered custom resolvers
	// (default true).
	//
	// See also: [RegisterCustomResolver]
	UseCustomResolvers bool

	// AllocationLimit is the maximum number of bytes to allocate for any single tag item.
	//
	// This is a safety measure to prevent allocating too much memory for a single tag item. If a tag
	// item exceeds this limit, the allocator will return [AllocationError] (default 16 MiB).
	AllocationLimit int

	// PreserveFormatSpecificItems controls whether or not to preserve format-specific items.
	//
	// When converting a tag from its concrete format (ex. [ID3v2Tag]) to a [Tag], this option
	// controls whether to preserve any special items that are unique to the concrete tag.
	//
	// This will store an extra immutable tag alongside the generic [Tag], which will be merged
	// back into the concrete tag when converting back (default true).
	PreserveFormatSpecificItems bool
}

// DefaultAllocationLimit is the default allocation limit for any single tag item.
const DefaultAllocationLimit = 16 * 1024 * 1024

// NewGlobalOptions creates a new [GlobalOptions] with default values.
func NewGlobalOptions() GlobalOptions {
	return GlobalOptions{
		UseCustomResolvers:          true,
		AllocationLimit:             DefaultAllocationLimit,
		PreserveFormatSpecificItems: true,
	}
}

var (
	globalOptionsMu sync.RWMutex
	globalOptions   = NewGlobalOptions()
)

// ApplyGlobalOptions applies the given [GlobalOptions].
func ApplyGlobalOptions(options GlobalOptions) {
	globalOptionsMu.Lock()
	globalOptions = options
	globalOptionsMu.Unlock()
}

func GetGlobalOptions() GlobalOptions {
	globalOptionsMu.RLock()
	o := globalOptions
	globalOptionsMu.RUnlock()
	return o
}
