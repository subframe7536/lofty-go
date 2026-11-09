package lofty

import (
	"os"
	"strconv"
	"time"
)

// --- small helpers ---

func durationFromSeconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) (int, error) { return strconv.Atoi(s) }

func indexOfStr(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func latin1EncodeMust(s string) []byte {
	b, err := latin1Encode(s, true)
	if err != nil {
		out := make([]byte, len(s))
		for i := 0; i < len(s); i++ {
			if s[i] < 0x80 {
				out[i] = s[i]
			} else {
				out[i] = '?'
			}
		}
		return out
	}
	return b
}

func optionFromPtr[T any](p *T) Option[*T] {
	if p == nil {
		return None[*T]()
	}
	return Some(p)
}

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

// saveToPath opens `path` and writes `tag` of `tagType` via the file's write path.
func saveToPath(path string, f AudioFile, tagType TagType, writeOptions WriteOptions) error {
	osFile, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer osFile.Close()
	verified, err := newVerifiedFile(NewOSFile(osFile), writeOptions.ParseOptions)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	tf := f.TaggedFile()
	tag := tf.Tag(tagType)
	if tag == nil {
		tag = NewTag(tagType)
	}
	return writeTag(tag, verified, writeOptions)
}

// removeTagFromPath removes `tagType` from the file at `path`.
func removeTagFromPath(path string, tagType TagType) error {
	osFile, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	defer osFile.Close()
	verified, err := newVerifiedFile(NewOSFile(osFile), NewParseOptions())
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	return removeTag(tagType, verified, NewWriteOptions())
}

// --- generic MPEG-4 ftyp check used by AacFile too ---

// readU32BE reads a big-endian u32.
