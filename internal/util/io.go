package util

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
)

// FileLike provides a set of methods to read and write to a file-like object.
//
// This is a combination of the [io.Reader], [io.Writer], [io.Seeker], plus
// truncation and length methods.
// It is used to provide implementors access to any file saving methods such as
// [AudioFile.SaveTo].
//
// Take great care in implementing this, as Lofty will assume that the
// implementations are correct. If this assumption were to be broken, files *may* become corrupted.
type FileLike interface {
	io.Reader
	io.Writer
	io.Seeker
	// Truncate truncates the storage object to the specified length.
	Truncate(size int64) error
	// Len returns the length of the storage object.
	Len() (int64, error)
}

// Splice replaces the specified range in the file with the `replacement` content.
//
// ## Performance
//
// This will grow/shrink the file as needed. When the file size changes, its contents will be
// shifted in `64KiB` chunks. Depending on the size and position of the `replacement`, this may
// result in many writes.
//
// Errors:
// * The range end is greater than the file length
// * Any I/O error from the underlying writer
func Splice(f FileLike, start, end uint64, replacement []byte) error {
	fileLen64, err := f.Len()
	if err != nil {
		return err
	}
	fileLen := uint64(fileLen64)

	if start > end {
		return errors.New("lofty: range start exceeds range end")
	}
	if end > fileLen {
		return errors.New("lofty: range extends beyond file length")
	}

	return replaceRange(f, fileLen, start, end, replacement)
}

const moveBufferSize = 64 * 1024

func replaceRange(file FileLike, fileLen, rangeStart, rangeEnd uint64, replacement []byte) error {
	oldLen := rangeEnd - rangeStart
	replacementLen := uint64(len(replacement))

	// Fast path for edits at the end of a file
	if rangeEnd == fileLen {
		if _, err := file.Seek(int64(rangeStart), io.SeekStart); err != nil {
			return err
		}
		if _, err := file.Write(replacement); err != nil {
			return err
		}
		if replacementLen < oldLen {
			return file.Truncate(int64(rangeStart + replacementLen))
		}
		return nil
	}

	buffer := make([]byte, moveBufferSize)

	switch {
	case replacementLen > oldLen:
		difference := replacementLen - oldLen
		// The ranges overlap, so move the tail backwards from EOF before writing metadata.
		if err := extendStorage(file, difference); err != nil {
			return err
		}
		if err := shiftRight(file, rangeEnd, fileLen, difference, buffer); err != nil {
			return err
		}
	case replacementLen < oldLen:
		difference := oldLen - replacementLen
		// Move forwards from the metadata boundary so writes cannot clobber unread tail data.
		if err := shiftLeft(file, rangeEnd, fileLen, difference, buffer); err != nil {
			return err
		}
		if err := file.Truncate(int64(fileLen - difference)); err != nil {
			return err
		}
	}

	if _, err := file.Seek(int64(rangeStart), io.SeekStart); err != nil {
		return err
	}
	_, err := file.Write(replacement)
	return err
}

func extendStorage(file FileLike, amount uint64) error {
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	zeros := make([]byte, moveBufferSize)
	remaining := amount
	for remaining != 0 {
		chunkLen := min(remaining, uint64(len(zeros)))
		if _, err := file.Write(zeros[:chunkLen]); err != nil {
			return err
		}
		remaining -= chunkLen
	}
	return nil
}

func shiftRight(file FileLike, start, end, amount uint64, buffer []byte) error {
	cursor := end
	for cursor > start {
		chunkLen := min(cursor-start, uint64(len(buffer)))
		source := cursor - chunkLen

		if _, err := file.Seek(int64(source), io.SeekStart); err != nil {
			return err
		}
		if _, err := io.ReadFull(file, buffer[:chunkLen]); err != nil {
			return err
		}
		if _, err := file.Seek(int64(source+amount), io.SeekStart); err != nil {
			return err
		}
		if _, err := file.Write(buffer[:chunkLen]); err != nil {
			return err
		}
		cursor = source
	}
	return nil
}

func shiftLeft(file FileLike, start, end, amount uint64, buffer []byte) error {
	cursor := start
	for cursor < end {
		chunkLen := min(end-cursor, uint64(len(buffer)))

		if _, err := file.Seek(int64(cursor), io.SeekStart); err != nil {
			return err
		}
		if _, err := io.ReadFull(file, buffer[:chunkLen]); err != nil {
			return err
		}
		if _, err := file.Seek(int64(cursor-amount), io.SeekStart); err != nil {
			return err
		}
		if _, err := file.Write(buffer[:chunkLen]); err != nil {
			return err
		}
		cursor += chunkLen
	}
	return nil
}

// OSFile adapts an [*os.File] to the [FileLike] interface.
type OSFile struct {
	*os.File
}

// Len returns the length of the file.
func (f OSFile) Len() (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// NewOSFile wraps an [*os.File] so it satisfies [FileLike].
func NewOSFile(f *os.File) OSFile { return OSFile{f} }

// BytesFile is an in-memory [FileLike], equivalent to Rust's `Cursor<Vec<u8>>`.
type BytesFile struct {
	buf []byte
	pos int64
}

// NewBytesFile creates a [BytesFile] from an initial byte slice.
// The slice is used directly (not copied), and may grow on writes.
func NewBytesFile(buf []byte) *BytesFile { return &BytesFile{buf: buf} }

// Bytes returns the current contents of the file.
func (f *BytesFile) Bytes() []byte { return f.buf }

// SetBuf replaces the contents and resets the position to 0.
func (f *BytesFile) SetBuf(buf []byte) { f.buf = buf; f.pos = 0 }

// Read implements [io.Reader].
func (f *BytesFile) Read(b []byte) (int, error) {
	if f.pos >= int64(len(f.buf)) {
		return 0, io.EOF
	}
	n := copy(b, f.buf[f.pos:])
	f.pos += int64(n)
	return n, nil
}

// Write implements [io.Writer]. Writing past the end zero-fills the gap.
func (f *BytesFile) Write(b []byte) (int, error) {
	end := f.pos + int64(len(b))
	oldLen := int64(len(f.buf))
	if end > oldLen {
		if int64(cap(f.buf)) >= end {
			f.buf = f.buf[:end]
		} else {
			nb := make([]byte, end)
			copy(nb, f.buf)
			f.buf = nb
		}
		// Rust `Cursor<Vec>` resizes with zeros — a resliced buffer may
		// still hold stale bytes beyond the old length
		for i := oldLen; i < f.pos; i++ {
			f.buf[i] = 0
		}
	}
	n := copy(f.buf[f.pos:], b)
	f.pos += int64(n)
	return n, nil
}

// Seek implements [io.Seeker].
func (f *BytesFile) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.pos + offset
	case io.SeekEnd:
		abs = int64(len(f.buf)) + offset
	default:
		return 0, errors.New("lofty: invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("lofty: negative position")
	}
	f.pos = abs
	return abs, nil
}

// Truncate implements [FileLike].
func (f *BytesFile) Truncate(size int64) error {
	if size < 0 || size > int64(len(f.buf)) {
		return errors.New("lofty: invalid truncation size")
	}
	f.buf = f.buf[:size]
	if f.pos > size {
		f.pos = size
	}
	return nil
}

// Len implements [FileLike].
func (f *BytesFile) Len() (int64, error) { return int64(len(f.buf)), nil }

// Pos returns the current position.
func (f *BytesFile) Pos() int64 { return f.pos }

// StreamLen returns the length of a seekable stream.
func StreamLen(rs io.Seeker) (uint64, error) {
	currentPos, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	if _, err := rs.Seek(currentPos, io.SeekStart); err != nil {
		return 0, err
	}
	return uint64(end), nil
}

// readF80 reads a big-endian 80-bit extended precision float.
// F80 is an 80-bit extended precision floating-point number, used in AIFF.
type F80 struct {
	signed   bool
	exponent uint16 // 15-bit exponent with a bias of 16383
	fraction uint64
}

func F80FromBEBytes(bytes [10]byte) F80 {
	signed := bytes[0]&0x80 != 0
	exponent := uint16(bytes[0]&0x7F)<<8 | uint16(bytes[1])
	fraction := binary.BigEndian.Uint64(bytes[2:])
	return F80{signed: signed, exponent: exponent, fraction: fraction}
}

func (f F80) AsF64() float64 {
	// Apple® Apple Numerics Manual, Second Edition, Table 2-7:
	var sign uint64
	if f.signed {
		sign = 1
	}

	if f.exponent == 32767 {
		if f.fraction == 0 {
			return inf(sign)
		}
		return nan(sign)
	}

	// 0 <= e <= 32766, i = 0, f = 0
	if f.fraction == 0 {
		return floatFromBits(sign << 63)
	}

	fraction := f.fraction & 0x7FFF_FFFF_FFFF_FFFF
	exponent := int16(f.exponent) - 16383 + 1023
	bits := (sign << 63) | (uint64(exponent) << 52) | (fraction >> 11)
	return floatFromBits(bits)
}

func inf(sign uint64) float64 { return math.Float64frombits((sign << 63) | 0x7FF0000000000000) }
func nan(sign uint64) float64 { return math.Float64frombits((sign << 63) | 0x7FF8000000000000) }

func floatFromBits(b uint64) float64 { return math.Float64frombits(b) }

// DivRound performs a rounded division.
//
// NOTE: If the result is less than 1, it will be rounded up to 1.
func DivRound(a, b uint64) uint64 {
	return (a + (b >> 1)) / b
}

// RevPatternSearcher searches for a pattern in a stream backwards.
type RevPatternSearcher struct {
	start      RevSearchStart
	end        RevSearchEnd
	bufferSize uint64
	pattern    []byte
	reader     io.ReadSeeker
}

type RevSearchStart int

const (
	// RevSearchFromEnd starts the search from the end of the stream.
	RevSearchFromEnd RevSearchStart = iota
	// RevSearchFromCurrent starts the search from the current position.
	RevSearchFromCurrent
)

type RevSearchEnd struct {
	kind int
	pos  uint64
}

// RevSearchEndStreamStart ends the search at the start of the stream.
var RevSearchEndStreamStart = RevSearchEnd{kind: 0}

// RevSearchEndAtPos ends the search at a specific position.
func RevSearchEndAtPos(p uint64) RevSearchEnd { return RevSearchEnd{kind: 2, pos: p} }

func NewRevPatternSearcher(reader io.ReadSeeker, pattern []byte) *RevPatternSearcher {
	return &RevPatternSearcher{
		start:      RevSearchFromEnd,
		end:        RevSearchEndStreamStart,
		bufferSize: 1024,
		pattern:    pattern,
		reader:     reader,
	}
}

func (r *RevPatternSearcher) WithStart(s RevSearchStart) *RevPatternSearcher {
	r.start = s
	return r
}

func (r *RevPatternSearcher) WithEnd(e RevSearchEnd) *RevPatternSearcher {
	r.end = e
	return r
}

// search looks for `pattern` in the stream, from the end up to the current position.
//
// If found, this will leave the reader at the start of the pattern. Otherwise, the reader will
// be at the original position.
func (r *RevPatternSearcher) Search() (bool, error) {
	if len(r.pattern) == 0 {
		return true, nil
	}

	originalPos, err := r.reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	patternLen := uint64(len(r.pattern))

	var startPos uint64
	if r.start == RevSearchFromEnd {
		end64, err := r.reader.Seek(0, io.SeekEnd)
		if err != nil {
			return false, err
		}
		startPos = uint64(end64)
	} else {
		startPos = uint64(originalPos)
	}

	var endPos uint64
	switch r.end.kind {
	case 0:
		endPos = 0
	case 1:
		endPos = uint64(originalPos)
	default:
		endPos = r.end.pos
	}

	if startPos < endPos || (startPos-endPos) < patternLen || r.bufferSize < patternLen {
		if _, err := r.reader.Seek(originalPos, io.SeekStart); err != nil {
			return false, err
		}
		return false, nil
	}

	// To handle partial matches, the `current_pos` gets decremented in "steps". Which is the
	// `buffer_size`, but with just enough room at the end for the pattern.
	overlapStep := r.bufferSize - (patternLen - 1)

	currentPos := startPos
	buf := make([]byte, r.bufferSize)

	for currentPos > endPos {
		windowSize := currentPos - endPos
		readSize := min(r.bufferSize, windowSize)

		readStart := currentPos - readSize
		if _, err := r.reader.Seek(int64(readStart), io.SeekStart); err != nil {
			return false, err
		}

		window := buf[:readSize]
		if _, err := io.ReadFull(r.reader, window); err != nil {
			return false, err
		}

		// find last occurrence of pattern in window
		if matchOffset := bytes.LastIndex(window, r.pattern); matchOffset >= 0 {
			if _, err := r.reader.Seek(int64(readStart)+int64(matchOffset), io.SeekStart); err != nil {
				return false, err
			}
			return true, nil
		}

		if readSize < overlapStep {
			break
		}
		currentPos -= overlapStep
	}

	return false, nil
}
