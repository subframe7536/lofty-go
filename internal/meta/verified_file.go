package meta

import (
	"io"
	"os"
)

// VerifiedFile wraps a [FileLike] with a predetermined [FileType].
//
// Used for tag writes to prevent repeated format verification.
type VerifiedFile struct {
	format FileType
	file   FileLike
}

func NewVerifiedFile(file FileLike, parseOptions ParseOptions) (*VerifiedFile, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	probe := NewProbe(file).Options(parseOptions)
	if _, err := probe.GuessFileType(); err != nil {
		return nil, &FileParseError{Source: err}
	}
	if ft, ok := probe.FileType().Get(); ok {
		return &VerifiedFile{format: ft, file: file}, nil
	}
	return nil, &FileParseError{Source: &UnknownFormatError{}}
}

// VerifiedFileFromPath opens `path` for reading and writing and determines its
// format from the file's content (like Rust's `VerifiedFile::new` as used by
// `TagExt::save_to_path`). The extension is ignored; see
// [VerifiedFileFromPathByExtension] for the extension-typed variant.
func VerifiedFileFromPath(path string, parseOptions ParseOptions) (*VerifiedFile, *os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, &FileParseError{Source: err}
	}
	vf, err := NewVerifiedFile(OSFile{File: f}, parseOptions)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return vf, f, nil
}

// VerifiedFileFromPathByExtension opens `path` for reading and writing and takes
// its format from the path's extension only, with no content sniffing. This is
// Rust's `VerifiedFile::new_from_path` (via `Probe::open_with`), used by
// `TagType::remove_from_path`. An unrecognized or missing extension yields an
// [UnknownFormatError].
func VerifiedFileFromPathByExtension(path string) (*VerifiedFile, *os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, &FileParseError{Source: err}
	}
	ft, ok := FileTypeFromPath(path)
	if !ok {
		f.Close()
		return nil, nil, &FileParseError{Source: &UnknownFormatError{}}
	}
	return &VerifiedFile{format: ft, file: OSFile{File: f}}, f, nil
}

func (v *VerifiedFile) Format() FileType { return v.format }
func (v *VerifiedFile) File() FileLike   { return v.file }

// NewVerifiedFileOf wraps `file` with a predetermined `format`, skipping
// probe verification. Used by write paths that already know the format.
func NewVerifiedFileOf(format FileType, file FileLike) *VerifiedFile {
	return &VerifiedFile{format: format, file: file}
}
