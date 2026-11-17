package meta

import (
	"io"
	"sync"
)

// registryMu guards the format/tag registries below. Registration normally
// happens in package init(), but lookups may run concurrently with late
// registrations (e.g. from tests or user code).
var registryMu sync.RWMutex

// FormatFileWriter writes a tag into a verified file of a specific format.
type FormatFileWriter func(file *VerifiedFile, tag *Tag, writeOptions WriteOptions) error

var formatWriters = map[fileTypeKind]FormatFileWriter{}

// FormatReader reads a [TaggedFile] from a reader.
type FormatReader func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error)

// FormatWriter writes a tag into a file of a specific format.
type FormatWriter func(file FileLike, format FileType, tag *Tag, writeOptions WriteOptions) error

// RegisterFormat registers a format's reader and writer for `fileType`.
// Format packages call this from their init(); it panics on duplicates,
// mirroring [RegisterCustomResolver].
func RegisterFormat(fileType FileType, reader FormatReader, writer FormatWriter) {
	RegisterFormatReader(fileType, reader)
	RegisterFormatWriter(fileType, func(v *VerifiedFile, tag *Tag, wo WriteOptions) error {
		return writer(v.File(), v.Format(), tag, wo)
	})
}

// RegisterFormatReader registers only the reader for `fileType`.
func RegisterFormatReader(fileType FileType, reader FormatReader) {
	if fileType.kind == fileTypeCustom {
		panic("lofty: RegisterFormatReader cannot be used for custom file types")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := formatReaders[fileType.kind]; exists {
		panic("lofty: format `" + fileType.String() + "` already registered!")
	}
	formatReaders[fileType.kind] = reader
}

// RegisterFormatWriter registers only the writer for `fileType`. It panics if
// a writer is already registered for the file type.
func RegisterFormatWriter(fileType FileType, writer FormatFileWriter) {
	if fileType.kind == fileTypeCustom {
		panic("lofty: RegisterFormatWriter cannot be used for custom file types")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := formatWriters[fileType.kind]; exists {
		panic("lofty: format writer for `" + fileType.String() + "` already registered!")
	}
	formatWriters[fileType.kind] = writer
}

// TagDumper writes a tag of a specific type to a writer.
type TagDumper func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error

// RegisterTagDumper registers a [TagDumper] for `tagType`.
// Tag implementation packages call this from their init(); it panics on
// duplicates, mirroring [RegisterCustomResolver].
func RegisterTagDumper(tagType TagType, dumper TagDumper) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := tagDumpers[tagType]; exists {
		panic("lofty: tag dumper for `" + tagType.String() + "` already registered!")
	}
	tagDumpers[tagType] = dumper
}

// tagDumper writes a tag of a specific type to a writer.
type tagDumper = TagDumper

var tagDumpers = map[TagType]tagDumper{}

// ConcreteTagWriter writes a format-specific (native) tag into a verified
// file, mirroring the per-format dispatch Rust's `TagWriteExt::save_to`
// implementations perform (e.g. ID3v2 inside a WAV/AIFF chunk or at the end
// of a DSF file, VorbisComments inside an Ogg stream or FLAC metadata).
//
// `tag` is the concrete tag type owned by `tagType` (`*Id3v2Tag` byte payload,
// `*VorbisCommentsTag`, ...); the meaning is defined per tag type by the
// registering format package.
type ConcreteTagWriter func(file *VerifiedFile, tag any, writeOptions WriteOptions) error

var concreteTagWriters = map[fileTypeKind]map[TagType]ConcreteTagWriter{}

// RegisterConcreteTagWriter registers a [ConcreteTagWriter] for `tagType` on
// `fileType`. Format packages call this from init() for tag types whose
// on-disk placement is format-specific.
func RegisterConcreteTagWriter(fileType FileType, tagType TagType, writer ConcreteTagWriter) {
	if fileType.kind == fileTypeCustom {
		panic("lofty: RegisterConcreteTagWriter cannot be used for custom file types")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	inner := concreteTagWriters[fileType.kind]
	if inner == nil {
		inner = map[TagType]ConcreteTagWriter{}
		concreteTagWriters[fileType.kind] = inner
	}
	if _, exists := inner[tagType]; exists {
		panic("lofty: concrete tag writer for `" + tagType.String() + "` on `" + fileType.String() + "` already registered!")
	}
	inner[tagType] = writer
}

// WriteConcreteTag invokes the registered [ConcreteTagWriter] for the
// verified file's format and `tagType`, reporting whether one exists.
func WriteConcreteTag(file *VerifiedFile, tagType TagType, tag any, writeOptions WriteOptions) (bool, error) {
	registryMu.RLock()
	w, ok := concreteTagWriters[file.Format().kind][tagType]
	registryMu.RUnlock()
	if ok {
		return true, w(file, tag, writeOptions)
	}
	return false, nil
}

// WriteTag writes a tag to a file of the verified format.
func WriteTag(tag *Tag, file *VerifiedFile, writeOptions WriteOptions) error {
	format := file.Format()
	// Rust's write_tag hits `unreachable!` for custom formats; returning
	// an error is the Go-port equivalent.
	if format.kind == fileTypeCustom {
		return &FileEncodingError{FileType: format, Source: &UnsupportedTagError{}}
	}
	support := format.TagSupport(tag.tagType)
	// Rust tag_ext::save_to: empty tag writes are allowed for read-only
	// formats, since they'll be stripped; non-empty writes are rejected.
	if support == TagSupportUnsupported || (!support.IsWritable() && !tag.IsEmpty()) {
		return &FileEncodingError{FileType: format, Source: &UnsupportedTagError{}}
	}
	registryMu.RLock()
	w, ok := formatWriters[format.kind]
	registryMu.RUnlock()
	if ok {
		err := w(file, tag, writeOptions)
		if err != nil {
			if fe, ok := err.(*FileEncodingError); ok {
				return fe.withFormat(format)
			}
			return &FileEncodingError{FileType: format, Source: err}
		}
		return nil
	}
	return &FileEncodingError{FileType: format, Source: &UnsupportedTagError{}}
}

// DumpTag writes a tag to a writer (tag data only).
func DumpTag(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
	registryMu.RLock()
	d, ok := tagDumpers[tag.tagType]
	registryMu.RUnlock()
	if ok {
		err := d(tag, writer, writeOptions)
		if err != nil {
			if te, ok := err.(*TagEncodingError); ok {
				return te
			}
			return &TagEncodingError{TagType: tag.tagType, HasTagType: true, Source: err}
		}
		return nil
	}
	return nil
}

// RemoveTag removes a tag of `tagType` from a verified file.
func RemoveTag(tagType TagType, file *VerifiedFile, writeOptions WriteOptions) error {
	return WriteTag(NewTag(tagType), file, writeOptions)
}

// SaveTagFields writes each of the file's present tag fields, in order, via
// the format's writer — mirroring the per-field `save_to` calls Rust's
// generated AudioFile impl performs. Empty tags remove the on-disk tag.
func SaveTagFields(file FileLike, format FileType, tags []*Tag, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	vf := &VerifiedFile{format: format, file: file}
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
		if err := WriteTag(tag, vf, writeOptions); err != nil {
			return err
		}
	}
	return nil
}
