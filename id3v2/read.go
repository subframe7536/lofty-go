package id3v2

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"reflect"
	"strings"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// unsyncReader streams unsynchronised ID3v2 content, dropping the 0x00 that
// follows every 0xFF (mirrors Rust's UnsynchronizedStream).
type unsyncReader struct {
	r  io.Reader
	ff bool // last emitted byte was 0xFF; a following 0x00 is skipped
}

func (u *unsyncReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		m, err := u.r.Read(p)
		data := p[:m]
		w := 0
		for i := 0; i < len(data); i++ {
			b := data[i]
			if u.ff && b == 0x00 {
				u.ff = false
				continue
			}
			u.ff = b == 0xFF
			p[w] = b
			w++
		}
		if w > 0 || err != nil {
			return w, err
		}
		if m == 0 {
			return 0, nil
		}
	}
}

// parsedFrameResult mirrors Rust's `ParsedFrame` enum.
type parsedFrameResult int

const (
	parsedFrameNext parsedFrameResult = iota
	parsedFrameSkip
	parsedFrameEof
)

// ParseId3v2Tag reads the tag content (after the header was consumed) into a
// native [Id3v2Tag]. Mirrors Rust `parse_id3v2`.
func ParseId3v2Tag(reader io.Reader, header Id3v2Header, parseOptions meta.ParseOptions) (*Id3v2Tag, error) {
	contentSize := int64(header.Size - header.ExtendedSize)
	var r io.Reader
	switch reader.(type) {
	case *bytes.Reader, *strings.Reader:
		// Already memory-backed; Rust's lazy restriction is just as cheap.
		r = io.LimitReader(reader, contentSize)
	default:
		// Buffer the tag content when it fits the allocation limit: the
		// frame walk performs many tiny reads, which would each be a
		// syscall on a raw file. The whole limited region is consumed
		// either way.
		if contentSize > 0 {
			if buf, aerr := util.TryByteSlice(int(contentSize)); aerr == nil {
				n, rerr := io.ReadFull(reader, buf)
				if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
					return nil, &Id3v2ParseError{Source: rerr}
				}
				r = bytes.NewReader(buf[:n])
			}
		}
		if r == nil {
			r = io.LimitReader(reader, contentSize)
		}
	}

	// Prior to ID3v2.4 the tag-wide unsynchronisation flag meant the entire
	// tag content was unsynchronised.
	if header.Flags.Unsynchronisation && header.Version < id3v2Version4 {
		r = &unsyncReader{r: r}
	}

	tag, err := readAllFramesIntoTag(r, header, parseOptions)
	if err != nil {
		return nil, &Id3v2ParseError{Source: err}
	}
	// Consume any remaining tag content (padding, discarded frames) so callers
	// are positioned at the tag's end.
	_, _ = io.Copy(io.Discard, r)

	// Construct TDRC frame from TYER, TDAT, and TIME frames
	if parseOptions.ImplicitConversions && header.Version == id3v2Version3 {
		if err := constructTdrcFromV3(tag, parseOptions.ParsingMode); err != nil {
			return nil, &Id3v2ParseError{Source: err}
		}
	}

	return tag, nil
}

func constructTdrcFromV3(tag *Id3v2Tag, parseMode meta.ParsingMode) error {
	tyer := FrameIdValid("TYER")
	tdat := FrameIdValid("TDAT")
	time_ := FrameIdValid("TIME")

	yearFrame := tag.Frames.Remove(FrameIdValid("TDRC"))
	if len(yearFrame) == 0 {
		return nil
	}
	yf, ok := yearFrame[0].(*TimestampFrame)
	if !ok {
		tag.Frames.Insert(yearFrame[0])
		return nil
	}
	if yf.Timestamp.Month.IsSome() {
		// Not a spec-compliant TYER frame; some encoders write a full date in TYER
		// (upstream lofty-rs issue #794). Still allow TDAT and TIME to overwrite it
		// in non-strict modes.
		if parseMode == meta.ParsingModeStrict {
			return frameParseErrorMsg(meta.Some(tyer), "invalid year frame, likely contains a full timestamp")
		}
	}

	tdrc := yf.Timestamp
	dateUsed, timeUsed := false, false

	if date, ok := tag.Frames.GetText(tdat); ok && len(date) == 4 && isASCII(date) && isASCIIDigits(date) {
		day, err1 := atoiU8(date[:2])
		month, err2 := atoiU8(date[2:])
		if err1 && err2 {
			tdrc.Month = meta.Some(month)
			tdrc.Day = meta.Some(day)
			dateUsed = true

			if t, ok := tag.Frames.GetText(time_); ok && len(t) == 4 && isASCII(t) && isASCIIDigits(t) {
				hour, err3 := atoiU8(t[:2])
				minute, err4 := atoiU8(t[2:])
				if err3 && err4 {
					tdrc.Hour = meta.Some(hour)
					tdrc.Minute = meta.Some(minute)
					timeUsed = true
				}
			}
		}
	}

	tag.Frames.Insert(NewTimestampFrame(FrameIdValid("TDRC"), yf.Encoding, tdrc))
	if dateUsed {
		tag.Frames.Remove(tdat)
	}
	if timeUsed {
		tag.Frames.Remove(time_)
	}
	return nil
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoiU8(s string) (uint8, bool) {
	n, err := util.Atoi(s)
	if err != nil || n < 0 || n > 255 {
		return 0, false
	}
	return uint8(n), true
}

func readAllFramesIntoTag(reader io.Reader, header Id3v2Header, parseOptions meta.ParseOptions) (*Id3v2Tag, error) {
	tag := NewId3v2Tag()
	tag.SetOriginalVersion(header.Version)
	tag.SetFlags(header.Flags)
	frames, err := readAllFramesIntoList(reader, header.Version, header.Flags, parseOptions)
	if err != nil {
		return nil, err
	}
	tag.Frames = frames
	return tag, nil
}

func readAllFramesIntoList(reader io.Reader, version Id3v2Version, flags Id3v2TagFlags, parseOptions meta.ParseOptions) (*FrameList, error) {
	list := NewFrameList()
	for {
		frame, result, err := parsedFrameRead(reader, version, flags, parseOptions)
		if err != nil {
			return nil, err
		}
		switch result {
		case parsedFrameNext:
			frameValueIsEmpty, known := frame.frameIsEmpty()
			if replaced := list.Insert(frame); replaced != nil {
				replacedEmpty, replacedKnown := replaced.frameIsEmpty()
				if known && replacedKnown && frameValueIsEmpty && !replacedEmpty {
					// Keep the frame with the non-empty content
					list.Insert(replaced)
				}
			}
		case parsedFrameEof:
			return list, nil
		}
	}
}

// parseV2Header parses an ID3v2.2 frame header (3-char ID, 3-byte size).
func parseV2Header(reader io.Reader) (FrameId, uint32, bool, error) {
	var header [6]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return FrameId{}, 0, false, nil
	}
	if header[0] == 0 {
		return FrameId{}, 0, false, nil
	}
	size := uint32(header[3])<<16 | uint32(header[4])<<8 | uint32(header[5])
	idStr := string(header[:3])
	if upgraded, ok := UpgradeV2(idStr); ok {
		idStr = upgraded
	}
	id, err := ParseFrameId(idStr)
	if err != nil {
		return FrameId{}, size, false, err
	}
	return id, size, true, nil
}

// parseFrameHeader parses an ID3v2.3/2.4 frame header.
func parseFrameHeader(reader io.Reader, synchsafe bool, parseOptions meta.ParseOptions) (FrameId, FrameFlags, uint32, bool, error) {
	var header [10]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return FrameId{}, FrameFlags{}, 0, false, nil
	}
	if header[0] == 0 {
		return FrameId{}, FrameFlags{}, 0, false, nil
	}

	size := binary.BigEndian.Uint32(header[4:8])
	if synchsafe {
		size = util.Unsynchsafe32(header[4:8])
	}

	// Some apps make v3 tags with v2 frame IDs
	idEnd := 4
	invalidV2Frame := false
	if header[3] == 0 && !synchsafe {
		invalidV2Frame = true
		idEnd = 3
	}

	idStr, err := utf8DecodeStrChecked(header[:idEnd])
	if err == nil {
		// Rust's `utf8_decode_str` also trims trailing NULs, so a v2.4 ID such
		// as "ABC\0" becomes the outdated 3-character ID "ABC".
		idStr = util.TrimEndNulls(idStr)
	}
	if err != nil {
		// size is still returned so the caller can skip the frame content
		return FrameId{}, FrameFlags{}, size, false, &FrameIdParseError{IdBytes: append([]byte(nil), header[:idEnd]...)}
	}

	var id string
	switch {
	case invalidV2Frame:
		if upgraded, ok := UpgradeV2(idStr); ok {
			id = upgraded
		} else {
			id = idStr
		}
	case !synchsafe && parseOptions.ImplicitConversions:
		if upgraded, ok := UpgradeV3(idStr); ok {
			id = upgraded
		} else {
			id = idStr
		}
	default:
		id = idStr
	}
	frameID, err := ParseFrameId(id)
	if err != nil {
		return FrameId{}, FrameFlags{}, size, false, err
	}

	flags16 := binary.BigEndian.Uint16(header[8:10])
	var flags FrameFlags
	if synchsafe {
		flags = ParseFrameFlagsId3v24(flags16)
	} else {
		flags = ParseFrameFlagsId3v23(flags16)
	}
	return frameID, flags, size, true, nil
}

func utf8DecodeStrChecked(b []byte) (string, error) {
	for _, c := range b {
		if c > 0x7F {
			return "", &FrameIdParseError{IdBytes: append([]byte(nil), b...)}
		}
	}
	return string(b), nil
}

// parsedFrameRead mirrors Rust `ParsedFrame::read`.
func parsedFrameRead(reader io.Reader, version Id3v2Version, tagFlags Id3v2TagFlags, parseOptions meta.ParseOptions) (Frame, parsedFrameResult, error) {
	var id FrameId
	var flags FrameFlags
	var size uint32
	var ok bool
	var err error

	switch version {
	case id3v2Version2:
		id, size, ok, err = parseV2Header(reader)
		flags = FrameFlags{}
	case id3v2Version3:
		id, flags, size, ok, err = parseFrameHeader(reader, false, parseOptions)
	default:
		id, flags, size, ok, err = parseFrameHeader(reader, true, parseOptions)
	}
	if err != nil {
		if parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, 0, frameParseError(meta.None[FrameId](), err)
		}
		// Skip this frame and continue reading
		if serr := skipFrameContent(reader, size); serr != nil {
			return nil, 0, frameParseError(meta.None[FrameId](), serr)
		}
		return nil, parsedFrameSkip, nil
	}
	if !ok {
		return nil, parsedFrameEof, nil
	}

	if !parseOptions.ReadCoverArt && id == FrameIdValid("APIC") {
		if serr := skipFrameContent(reader, size); serr != nil {
			return nil, 0, frameParseError(meta.Some(id), serr)
		}
		return nil, parsedFrameSkip, nil
	}

	// See `ParseId3v2Tag`
	if tagFlags.Unsynchronisation && version > id3v2Version3 {
		flags.Unsynchronisation = true
	}

	if size == 0 {
		if parseOptions.ParsingMode == meta.ParsingModeStrict {
			return nil, 0, frameParseUndersized(id)
		}
		if serr := skipFrameContent(reader, size); serr != nil {
			return nil, 0, frameParseError(meta.Some(id), serr)
		}
		return nil, parsedFrameSkip, nil
	}

	// Get the encryption method symbol
	if flags.Encryption.IsSome() {
		if size < 1 {
			return nil, 0, frameParseUndersized(id)
		}
		var b [1]byte
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return nil, 0, frameParseError(meta.Some(id), err)
		}
		flags.Encryption = meta.Some(b[0])
		size--
	}

	// Get the group identifier
	if flags.GroupingIdentity.IsSome() {
		if size < 1 {
			return nil, 0, frameParseUndersized(id)
		}
		var b [1]byte
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return nil, 0, frameParseError(meta.Some(id), err)
		}
		flags.GroupingIdentity = meta.Some(b[0])
		size--
	}

	// Get the real data length
	if flags.DataLengthIndicator.IsSome() || flags.Compression {
		if size < 4 {
			return nil, 0, frameParseUndersized(id)
		}
		var lb [4]byte
		if _, err := io.ReadFull(reader, lb[:]); err != nil {
			return nil, 0, frameParseError(meta.Some(id), err)
		}
		flags.DataLengthIndicator = meta.Some(util.Unsynchsafe32(lb[:]))
		size -= 4
	}

	if size == 0 {
		return nil, 0, frameParseUndersized(id)
	}

	// Encrypted frames need a data length indicator; check before reading
	// (Rust `handle_encryption`).
	if flags.Encryption.IsSome() && flags.DataLengthIndicator.IsNone() {
		return nil, 0, frameParseMissingDLI(id)
	}

	// Restrict to the frame content. Like Rust's `reader.take(size)`, a frame
	// whose declared size runs past the end of the tag is truncated rather
	// than rejected: only what the tag actually contains is read.
	content, err := readFrameContent(reader, size)
	if err != nil {
		return nil, 0, frameParseError(meta.Some(id), err)
	}

	// Flags applied in order: unsynchronization -> compression -> encryption
	if flags.Unsynchronisation {
		content = Id3v2Deunsync(content)
	}
	// The content is a private copy (or a private sub-slice of one), so nested
	// frames (CHAP/CTOC children) can be handed sub-slices instead of copies.
	contentDepth := readerDepth(reader) + 1
	var contentReader io.Reader = &ownedReader{buf: content, depth: contentDepth}
	var zlibReader *lazyZlibReader
	if flags.Compression {
		// Decompression is lazy, like Rust's `ZlibDecoder`: a corrupt stream
		// surfaces as a read error, which parsers that tolerate a short read
		// keep tolerating. Output beyond the allocation limit is not
		// tolerated, see the check below.
		zlibReader = &lazyZlibReader{src: content, depth: contentDepth}
		contentReader = zlibReader
	}
	if flags.Encryption.IsSome() {
		// Nothing further we can do with encrypted frames. Rust reads exactly
		// `size` bytes here (`read_exact`), so a short frame is an error.
		data, err := util.TryByteSlice(int(size))
		if err != nil {
			return nil, 0, frameParseError(meta.Some(id), err)
		}
		if _, err := io.ReadFull(contentReader, data); err != nil {
			return nil, 0, frameParseError(meta.Some(id), err)
		}
		return NewBinaryFrame(id, data).withFlags(flags), parsedFrameNext, nil
	}

	frame, result, err := parseFrame(contentReader, id, size, flags, version, parseOptions)
	if zlibReader != nil && zlibReader.allocErr != nil {
		return nil, 0, frameParseError(meta.Some(id), zlibReader.allocErr)
	}
	if err == nil && result == parsedFrameSkip {
		// Rust's `skip_frame(None, reader, size)?`: draining can only fail for
		// a corrupt compressed stream.
		if _, derr := io.Copy(io.Discard, contentReader); derr != nil {
			return nil, 0, frameParseError(meta.None[FrameId](), derr)
		}
	}
	return frame, result, err
}

// upperBoundReader is implemented by readers that can cheaply tell how many
// bytes at most remain.
type upperBoundReader interface {
	remainingUpperBound() int64
}

func (u *unsyncReader) remainingUpperBound() int64 {
	if n, ok := readerUpperBound(u.r); ok {
		return n
	}
	return -1
}

// readerUpperBound returns an upper bound on the bytes left in r, if known.
func readerUpperBound(r io.Reader) (int64, bool) {
	switch v := r.(type) {
	case *bytes.Reader:
		return int64(v.Len()), true
	case *strings.Reader:
		return int64(v.Len()), true
	case *io.LimitedReader:
		// The limit alone says nothing about how much data really exists
		// (it may come from a forged header), so it is only a bound when the
		// wrapped reader knows its own length.
		if n, ok := readerUpperBound(v.R); ok {
			if v.N < n {
				n = v.N
			}
			return n, true
		}
	case upperBoundReader:
		if n := v.remainingUpperBound(); n >= 0 {
			return n, true
		}
	}
	return 0, false
}

// readFrameContent reads up to size bytes, stopping early at the end of the
// available data. It never allocates more than the data actually available.
func readFrameContent(r io.Reader, size uint32) ([]byte, error) {
	n := int64(size)
	if o, ok := r.(*ownedReader); ok {
		return o.take(n), nil
	}
	if avail, ok := readerUpperBound(r); ok {
		if avail < n {
			n = avail
		}
		buf := make([]byte, n)
		m, err := io.ReadFull(r, buf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = nil
		}
		return buf[:m], err
	}
	return io.ReadAll(io.LimitReader(r, n))
}

// lazyZlibReader decompresses src on first read, mirroring Rust's lazily
// initialized `flate2::read::ZlibDecoder`.
//
// The decompressed output is bounded by the global allocation limit; exceeding
// it fails with a [util.AllocationError] (also kept in allocErr, since
// parsers tolerate other read errors as short reads).
type lazyZlibReader struct {
	src      []byte
	depth    int
	r        io.Reader
	err      error
	allocErr error
	total    int64
	init     bool
}

func (z *lazyZlibReader) nestingDepth() int { return z.depth }

func (z *lazyZlibReader) Read(p []byte) (int, error) {
	if !z.init {
		z.init = true
		zr, err := zlib.NewReader(bytes.NewReader(z.src))
		if err != nil {
			z.err = err
		} else {
			z.r = zr
		}
	}
	if z.err != nil {
		return 0, z.err
	}
	limit := int64(util.GetGlobalOptions().AllocationLimit)
	// Allow one byte past the limit so an overflow is detectable.
	if room := limit - z.total + 1; int64(len(p)) > room {
		p = p[:room]
	}
	n, err := z.r.Read(p)
	z.total += int64(n)
	if z.total > limit {
		n -= int(z.total - limit)
		z.total = limit
		z.allocErr = &util.AllocationError{Kind: util.AllocationLimitExceeded}
		z.err = z.allocErr
		return n, z.err
	}
	if err != nil && err != io.EOF {
		z.err = err
	}
	return n, err
}

// ownedReader reads from a byte slice that is private to the frame being
// parsed, so nested frames can take sub-slices of it without copying.
type ownedReader struct {
	buf   []byte
	off   int
	depth int
}

func (o *ownedReader) nestingDepth() int { return o.depth }

func (o *ownedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if o.off >= len(o.buf) {
		return 0, io.EOF
	}
	n := copy(p, o.buf[o.off:])
	o.off += n
	return n, nil
}

func (o *ownedReader) ReadByte() (byte, error) {
	if o.off >= len(o.buf) {
		return 0, io.EOF
	}
	b := o.buf[o.off]
	o.off++
	return b, nil
}

func (o *ownedReader) remainingUpperBound() int64 { return int64(len(o.buf) - o.off) }

// take returns up to n of the remaining bytes as a capacity-limited sub-slice.
func (o *ownedReader) take(n int64) []byte {
	if rem := int64(len(o.buf) - o.off); n > rem {
		n = rem
	}
	end := o.off + int(n)
	s := o.buf[o.off:end:end]
	o.off = end
	return s
}

// readerDepth reports how many CHAP/CTOC-style container levels enclose the
// frames read from r (0 for the tag's top-level frame list).
func readerDepth(r io.Reader) int {
	if d, ok := r.(interface{ nestingDepth() int }); ok {
		return d.nestingDepth()
	}
	return 0
}

func (f *BinaryFrame) withFlags(flags FrameFlags) *BinaryFrame {
	f.SetFlags(flags)
	return f
}

func skipFrameContent(reader io.Reader, size uint32) error {
	_, err := io.CopyN(io.Discard, reader, int64(size))
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// id3v2MinContentSize mirrors `verify_size` minimum content sizes per version.
func id3v2MinContentSize(id string, version Id3v2Version) uint32 {
	switch id {
	case "APIC":
		if version == id3v2Version2 {
			return 6
		}
		return 4
	case "TXXX", "WXXX":
		return 2
	case "COMM", "USLT":
		return 5
	case "TIPL", "TMCL":
		return 3
	case "OWNE":
		return 10
	case "POPM":
		return 6
	case "CHAP":
		return 17
	case "CTOC":
		return 3
	case "UFID", "RVA2", "ETCO", "PRIV":
		return 1
	}
	// Rust matches on `starts_with`, which also covers outdated (3-char) IDs
	if len(id) > 0 && (id[0] == 'T' || id[0] == 'W' || id == "GRP1" || id == "MVNM" || id == "MVIN") {
		return 1
	}
	return 0
}

// parseFrame mirrors `parse_frame` + `parse_content` dispatch.
func parseFrame(reader io.Reader, id FrameId, size uint32, flags FrameFlags, version Id3v2Version, parseOptions meta.ParseOptions) (Frame, parsedFrameResult, error) {
	if minSize := id3v2MinContentSize(id.String(), version); size < minSize {
		if parseOptions.ParsingMode == meta.ParsingModeRelaxed {
			return nil, parsedFrameSkip, nil
		}
		return nil, 0, setFrameErrID(frameParseError(meta.None[FrameId](), util.NewNotEnoughDataError(int(minSize))), id)
	}

	var frame Frame
	var parsed bool
	var err error
	optional := func(f Frame, e error) {
		if e != nil {
			err = e
			return
		}
		if f == nil || reflect.ValueOf(f).IsNil() {
			return
		}
		frame, parsed = f, true
	}

	switch id.String() {
	case "APIC":
		f, e := parseAttachedPictureFrame(reader, flags, version)
		optional(f, e)
	case "TXXX":
		f, e := parseExtendedTextFrame(reader, flags, version)
		optional(f, e)
	case "WXXX":
		f, e := parseExtendedUrlFrame(reader, flags, version)
		optional(f, e)
	case "COMM":
		f, e := parseCommentFrame(reader, flags, version)
		optional(f, e)
	case "USLT":
		f, e := parseUnsynchronizedTextFrame(reader, flags, version)
		optional(f, e)
	case "TIPL", "TMCL":
		f, e := parseKeyValueFrame(reader, id, flags, version)
		optional(f, e)
	case "UFID":
		f, e := parseUniqueFileIdentifierFrame(reader, flags, parseOptions.ParsingMode)
		optional(f, e)
	case "RVA2":
		f, e := parseRelativeVolumeAdjustmentFrame(reader, flags, parseOptions.ParsingMode)
		optional(f, e)
	case "OWNE":
		f, e := parseOwnershipFrame(reader, flags)
		optional(f, e)
	case "ETCO":
		f, e := parseEventTimingCodesFrame(reader, flags)
		optional(f, e)
	case "PRIV":
		f, e := parsePrivateFrame(reader, flags)
		optional(f, e)
	case "TDEN", "TDOR", "TDRC", "TDRL", "TDTG":
		f, e := parseTimestampFrame(reader, id, flags, parseOptions.ParsingMode)
		optional(f, e)
	case "POPM":
		f, e := parsePopularimeterFrame(reader, flags)
		optional(f, e)
	case "CHAP":
		f, e := parseChapterFrame(reader, parseOptions, version, flags)
		optional(f, e)
	case "CTOC":
		f, e := parseChapterTableOfContentsFrame(reader, parseOptions, version, flags)
		optional(f, e)
	default:
		s := id.String()
		switch {
		case len(s) > 0 && (s[0] == 'T' || s == "GRP1" || s == "MVNM" || s == "MVIN"):
			f, e := parseTextInformationFrame(reader, id, flags, version)
			optional(f, e)
		case len(s) > 0 && s[0] == 'W':
			f, e := parseUrlLinkFrame(reader, id, flags)
			optional(f, e)
		default:
			f, e := parseBinaryFrame(reader, id, flags)
			optional(f, e)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	if !parsed {
		return nil, parsedFrameSkip, nil
	}
	return frame, parsedFrameNext, nil
}
