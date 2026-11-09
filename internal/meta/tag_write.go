package meta

import "io"

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
	if _, exists := formatReaders[fileType.kind]; exists {
		panic("lofty: format `" + fileType.String() + "` already registered!")
	}
	formatReaders[fileType.kind] = reader
}

// RegisterFormatWriter registers only the writer for `fileType`.
func RegisterFormatWriter(fileType FileType, writer FormatFileWriter) {
	if fileType.kind == fileTypeCustom {
		panic("lofty: RegisterFormatWriter cannot be used for custom file types")
	}
	formatWriters[fileType.kind] = writer
}

// TagDumper writes a tag of a specific type to a writer.
type TagDumper func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error

// RegisterTagDumper registers a [TagDumper] for `tagType`.
// Tag implementation packages call this from their init(); it panics on
// duplicates, mirroring [RegisterCustomResolver].
func RegisterTagDumper(tagType TagType, dumper TagDumper) {
	if _, exists := tagDumpers[tagType]; exists {
		panic("lofty: tag dumper for `" + tagType.String() + "` already registered!")
	}
	tagDumpers[tagType] = dumper
}

// tagDumper writes a tag of a specific type to a writer.
type tagDumper = TagDumper

var tagDumpers = map[TagType]tagDumper{}

// WriteTag writes a tag to a file of the verified format.
func WriteTag(tag *Tag, file *VerifiedFile, writeOptions WriteOptions) error {
	format := file.Format()
	if format.kind == fileTypeCustom {
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	support := format.TagSupport(tag.tagType)
	// Rust tag_ext::save_to: empty tag writes are allowed for read-only
	// formats, since they'll be stripped; non-empty writes are rejected.
	if support == TagSupportUnsupported || (!support.IsWritable() && !tag.IsEmpty()) {
		return &FileEncodingError{Source: &UnsupportedTagError{}}
	}
	if w, ok := formatWriters[format.kind]; ok {
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
	if d, ok := tagDumpers[tag.tagType]; ok {
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
