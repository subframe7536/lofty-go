package meta

import (
	"bytes"
	"io"
	"os"
)

// Probe is used to probe a file for its metadata.
type Probe struct {
	inner   io.ReadSeeker
	fTy     Option[FileType]
	options Option[ParseOptions]
}

// NewProbe creates a new [Probe].
func NewProbe(reader io.ReadSeeker) *Probe {
	return &Probe{inner: reader}
}

// NewProbeFromPath creates a [Probe] from a path, mirroring Rust's
// `Probe::open`: the file type is guessed from the path's extension.
//
// The probe owns the open file; call [Probe.Close] when done.
func NewProbeFromPath(path string) (*Probe, error) {
	f, err := os.Open(path) // #nosec G304 -- library opens caller-supplied paths by design
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	var fTy Option[FileType]
	if ft, ok := FileTypeFromPath(path); ok {
		fTy = Some(ft)
	}
	return &Probe{inner: f, fTy: fTy}, nil
}

// Close closes the underlying reader when it is an [io.Closer] (e.g. a probe
// created by [NewProbeFromPath]).
func (p *Probe) Close() error {
	if c, ok := p.inner.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// ProbeWithFileType creates a new [Probe] with the [FileType] already known.

func ProbeWithFileType(reader io.ReadSeeker, fileType FileType) *Probe {
	return &Probe{inner: reader, fTy: Some(fileType)}
}

// FileType returns the [FileType], if set.
func (p *Probe) FileType() Option[FileType] { return p.fTy }

// SetFileType sets the file type.
func (p *Probe) SetFileType(fileType FileType) *Probe {
	p.fTy = Some(fileType)
	return p
}

// Options sets the [ParseOptions].
func (p *Probe) Options(options ParseOptions) *Probe {
	p.options = Some(options)
	return p
}

// IntoInner returns the inner reader.
func (p *Probe) IntoInner() io.ReadSeeker { return p.inner }

// GuessFileType attempts to get the [FileType] based on the data in the reader.
//
// On success, the file type will be replaced.
//
// NOTE: The chance for succeeding is influenced by [ParseOptions].
// Be sure to set it with [Probe.Options] prior to calling this method.
// Some files may require more than the default [ParseOptions.DefaultMaxJunkBytes]
// to be detected successfully.
func (p *Probe) GuessFileType() (*Probe, error) {
	maxJunkBytes := DefaultMaxJunkBytes
	if o, ok := p.options.Get(); ok {
		maxJunkBytes = o.MaxJunkBytes
	}
	// Rust's `max_junk_bytes` is a usize; a negative Go value means "no junk".
	if maxJunkBytes < 0 {
		maxJunkBytes = 0
	}

	fTy, err := p.guessInner(maxJunkBytes)
	if err != nil {
		return nil, err
	}
	if _, ok := p.fTy.Get(); !ok {
		p.fTy = fTy
	} else if ft, ok := fTy.Get(); ok {
		p.fTy = Some(ft)
	}
	return p, nil
}

func (p *Probe) guessInner(maxJunkBytes int) (Option[FileType], error) {
	var buf [36]byte

	startingPosition, err := p.inner.Seek(0, io.SeekCurrent)
	if err != nil {
		return None[FileType](), err
	}
	bufLen, err := io.ReadFull(p.inner, buf[:])
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return None[FileType](), err
	}
	if _, err := p.inner.Seek(startingPosition, io.SeekStart); err != nil {
		return None[FileType](), err
	}
	bufSlice := buf[:bufLen]

	// Give custom resolvers priority
	if getGlobalOptions().UseCustomResolvers {
		if f, ok := guessCustomResolvers(bufSlice); ok {
			return Some(f), nil
		}
	}

	guess, ok := fileTypeFromBufferInner(bufSlice)
	if !ok {
		return None[FileType](), nil
	}

	if guess.determined {
		return Some(guess.fileType), nil
	}
	if guess.id3Size > 0 || !guess.maybeJunk {
		// The file starts with an ID3v2 tag.
		positionAfterId3, err := p.inner.Seek(int64(10+guess.id3Size), io.SeekCurrent)
		if err != nil {
			return None[FileType](), err
		}
		var ident [4]byte
		n, err := io.ReadFull(p.inner, ident[:])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return None[FileType](), err
		}
		identN := ident[:n]
		if _, err := p.inner.Seek(positionAfterId3, io.SeekStart); err != nil {
			return None[FileType](), err
		}

		var result Option[FileType]
		switch {
		case bytes.HasPrefix(identN, []byte("MAC")):
			result = Some(FileTypeApe)
		case string(identN) == "fLaC":
			result = Some(FileTypeFlac)
		case string(identN) == "MPCK" || bytes.HasPrefix(identN, []byte("MP+")):
			result = Some(FileTypeMpc)
		default:
			result, err = p.checkMpegOrAac(maxJunkBytes)
			if err != nil {
				return None[FileType](), err
			}
		}
		if _, err := p.inner.Seek(startingPosition, io.SeekStart); err != nil {
			return None[FileType](), err
		}
		return result, nil
	}

	// The stream starts with potential junk data.
	result, err := p.checkMpegOrAac(maxJunkBytes)
	if err != nil {
		return None[FileType](), err
	}
	if _, err := p.inner.Seek(startingPosition, io.SeekStart); err != nil {
		return None[FileType](), err
	}
	return result, nil
}

// checkMpegOrAac searches for an MPEG/AAC frame sync, which may be preceded by
// junk bytes.
func (p *Probe) checkMpegOrAac(maxJunkBytes int) (Option[FileType], error) {
	found, err := searchForFrameSyncSeeking(p.inner, int64(maxJunkBytes))
	if err != nil {
		return None[FileType](), err
	}
	if found < 0 {
		return None[FileType](), nil
	}

	// Seek back to the start of the frame sync to check for AAC vs MPEG.
	if _, err := p.inner.Seek(-2, io.SeekCurrent); err != nil {
		return None[FileType](), err
	}
	var buf [2]byte
	if _, err := io.ReadFull(p.inner, buf[:]); err != nil {
		return None[FileType](), err
	}

	if buf[1]&0b10000 > 0 && buf[1]&0b110 == 0 {
		return Some(FileTypeAac), nil
	}
	return Some(FileTypeMpeg), nil
}

// Read attempts to extract a [TaggedFile] from the reader.
//
// If `read_properties` is false, the properties will be zeroed out.
//
// # Errors
//
//   - No file type (this expects the file type to have been set already)
//   - The reader contains invalid data
func (p *Probe) Read() (*TaggedFile, error) {
	options := p.options.OrElse(NewParseOptions())

	fTy, ok := p.fTy.Get()
	if !ok {
		return nil, &FileParseError{Source: &UnknownFormatError{}}
	}
	// Like Rust's `read_inner`, reading begins at the reader's current
	// position; `GuessFileType` restores it after sniffing.
	return ReadFormat(fTy, p.inner, options)
}

// ReadFormat dispatches to the registered format reader.
func ReadFormat(fTy FileType, reader io.ReadSeeker, options ParseOptions) (*TaggedFile, error) {
	if fTy.kind == fileTypeCustom {
		if !getGlobalOptions().UseCustomResolvers {
			return nil, &FileParseError{Source: &UnknownFormatError{}}
		}
		return lookupResolver(fTy.customName).ReadFrom(reader, options)
	}
	registryMu.RLock()
	r, ok := formatReaders[fTy.kind]
	registryMu.RUnlock()
	if ok {
		tf, err := r(reader, options)
		if err != nil {
			return nil, err
		}
		return tf, nil
	}
	return nil, &FileParseError{Source: &UnknownFormatError{}}
}

// formatReader reads a [TaggedFile] from a reader.
type formatReader = FormatReader

var formatReaders = map[fileTypeKind]formatReader{}

// ReadBound attempts to extract a [BoundTaggedFile] from a [FileLike].
//
// Like Rust's `Probe::read_bound`, the probe's file type (set with
// [Probe.SetFileType], [ProbeWithFileType] or [NewProbeFromPath], or found by
// [Probe.GuessFileType]) is used as-is and nothing is sniffed. Only when no file
// type is set is it guessed from `file`'s content. `file` is used as the reader
// (when nil, the probe's own reader is used if it is a [FileLike]), read from
// its current position, and rewound afterwards.
func (p *Probe) ReadBound(file FileLike) (*BoundTaggedFile, error) {
	options := p.options.OrElse(NewParseOptions())
	if file == nil {
		fl, ok := p.inner.(FileLike)
		if !ok {
			return nil, &FileParseError{Source: &UnknownFormatError{}}
		}
		file = fl
	}
	fTy, ok := p.fTy.Get()
	if !ok {
		return BoundTaggedFileFrom(file, options)
	}
	taggedFile, err := ReadFormat(fTy, file, options)
	if err != nil {
		return nil, err
	}
	return newBoundTaggedFile(file, taggedFile)
}

// ReadFromPath reads a [TaggedFile] from a path.
//
// # Errors
//
//   - The file type could not be determined
//   - The file is an unsupported format
//   - The file could not be opened
//   - The reader contains invalid data
func ReadFromPath(path string, parseOptions ...ParseOptions) (*TaggedFile, error) {
	options := NewParseOptions()
	if len(parseOptions) > 0 {
		options = parseOptions[0]
	}
	f, err := os.Open(path) // #nosec G304 -- library opens caller-supplied paths by design
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	defer f.Close()

	if ft, ok := FileTypeFromPath(path); ok {
		return ReadFormat(ft, f, options)
	}

	p, err := NewProbe(f).Options(options).GuessFileType()
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, &FileParseError{Source: err}
	}
	return p.Options(options).Read()
}

// ReadFrom reads a [TaggedFile] from a reader.
func ReadFrom(reader io.ReadSeeker, parseOptions ...ParseOptions) (*TaggedFile, error) {
	options := NewParseOptions()
	if len(parseOptions) > 0 {
		options = parseOptions[0]
	}
	p, err := NewProbe(reader).Options(options).GuessFileType()
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	return p.Read()
}

// readTaggedFileFromFileLike reads a [TaggedFile] from a [FileLike], detecting
// the format from its content.
func readTaggedFileFromFileLike(file FileLike, parseOptions ParseOptions) (*TaggedFile, error) {
	p, err := NewProbe(file).Options(parseOptions).GuessFileType()
	if err != nil {
		return nil, &FileParseError{Source: err}
	}
	return p.Read()
}
