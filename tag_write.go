package lofty

import "io"

// formatWriter writes a tag into a verified file of a specific format.
type formatWriter func(file *verifiedFile, tag *Tag, writeOptions WriteOptions) error

var formatWriters = map[fileTypeKind]formatWriter{}

// tagDumper writes a tag of a specific type to a writer.
type tagDumper func(tag *Tag, writer io.Writer, writeOptions WriteOptions) error

var tagDumpers = map[TagType]tagDumper{}

// writeTag writes a tag to a file of the verified format.
func writeTag(tag *Tag, file *verifiedFile, writeOptions WriteOptions) error {
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

// dumpTag writes a tag to a writer (tag data only).
func dumpTag(tag *Tag, writer io.Writer, writeOptions WriteOptions) error {
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

// removeTag removes a tag of `tagType` from a verified file.
func removeTag(tagType TagType, file *verifiedFile, writeOptions WriteOptions) error {
	return writeTag(NewTag(tagType), file, writeOptions)
}

// saveTagFields writes each of the file's present tag fields, in order, via
// the format's writer — mirroring the per-field `save_to` calls Rust's
// generated AudioFile impl performs. Empty tags remove the on-disk tag.
func saveTagFields(file FileLike, format FileType, tags []*Tag, writeOptions WriteOptions) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return &FileEncodingError{Source: err}
	}
	vf := &verifiedFile{format: format, file: file}
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &FileEncodingError{Source: err}
		}
		if err := writeTag(tag, vf, writeOptions); err != nil {
			return err
		}
	}
	return nil
}
