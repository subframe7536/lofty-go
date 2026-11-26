package flac

import (
	"bytes"
	"errors"
	"io"

	"github.com/subframe7536/lofty-go/internal/util"
)

// Metadata block types (Rust `flac/block.rs`).
const (
	blockIDStreamInfo    byte = 0
	blockIDPadding       byte = 1
	blockIDSeekTable     byte = 3
	blockIDVorbisComment byte = 4
	blockIDPicture       byte = 6
)

const blockHeaderSize = 4

// flacBlock is a FLAC metadata block. `content` is empty when the block's
// payload was skipped by the read predicate.
type flacBlock struct {
	ty      byte
	last    bool
	content []byte
	start   uint64
	end     uint64
}

// readFlacBlock reads one metadata block at the current position, mirroring
// Rust's `Block::read`: the payload is only buffered when `predicate(type)`
// holds, and is otherwise seeked over.
func readFlacBlock(r io.ReadSeeker, predicate func(ty byte) bool) (*flacBlock, error) {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	var hdr [blockHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, eofToUnexpected(err)
	}
	b := &flacBlock{
		ty:    hdr[0] & 0x7F,
		last:  hdr[0]&0x80 != 0,
		start: uint64(start), // #nosec G115 -- start is a Seek stream position, always non-negative
	}
	size := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])

	if predicate(b.ty) {
		b.content, err = util.TryByteSlice(size)
		if err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, b.content); err != nil {
			return nil, eofToUnexpected(err)
		}
	} else if _, err := r.Seek(int64(size), io.SeekCurrent); err != nil {
		return nil, err
	}

	b.end = b.start + uint64(size) + blockHeaderSize // #nosec G115 -- size is parsed from a 24-bit big-endian field, so it is at most 0xFFFFFF and non-negative
	return b, nil
}

// marshal serializes the block with the given "last" flag.
func (b *flacBlock) marshal(last bool) []byte {
	out := make([]byte, 0, blockHeaderSize+len(b.content))
	first := b.ty & 0x7F
	if last {
		first |= 0x80
	}
	n := len(b.content)
	out = append(out, first, byte(n>>16), byte(n>>8), byte(n)) // #nosec G115 -- block content is validated against flacMaxBlockSize (24-bit) before marshaling, so no bits are lost
	return append(out, b.content...)
}

// verifyFlac checks the "fLaC" marker and reads the mandatory STREAMINFO block
// (Rust `verify_flac`).
func verifyFlac(r io.ReadSeeker) (*flacBlock, error) {
	var marker [4]byte
	if _, err := io.ReadFull(r, marker[:]); err != nil {
		return nil, &FlacParseError{Source: eofToUnexpected(err)}
	}
	if !bytes.Equal(marker[:], flacPreamble) {
		return nil, &FlacParseError{Message: `file missing "fLaC" stream marker`}
	}

	block, err := readFlacBlock(r, func(byte) bool { return true })
	if err != nil {
		return nil, &FlacParseError{Source: err}
	}
	if block.ty != blockIDStreamInfo {
		return nil, &FlacParseError{Message: "file missing mandatory STREAMINFO block"}
	}
	return block, nil
}

// eofToUnexpected maps a bare EOF to io.ErrUnexpectedEOF, like Rust's
// `read_exact`/`read_u8` failures.
func eofToUnexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
