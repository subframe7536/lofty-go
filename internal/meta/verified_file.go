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

func (v *VerifiedFile) Format() FileType { return v.format }
func (v *VerifiedFile) File() FileLike   { return v.file }

// NewVerifiedFileOf wraps `file` with a predetermined `format`, skipping
// probe verification. Used by write paths that already know the format.
func NewVerifiedFileOf(format FileType, file FileLike) *VerifiedFile {
	return &VerifiedFile{format: format, file: file}
}
