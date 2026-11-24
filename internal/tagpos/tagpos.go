// Package tagpos locates trailing metadata tags (ID3v1, etc.) without
// parsing them, so tag implementation packages can find each other's
// on-disk regions without importing each other.
package tagpos

import (
	"bytes"
	"io"
)

// Id3v1Size is the fixed size of an ID3v1 tag in bytes.
const Id3v1Size = 128

var (
	id3v1Marker = []byte("TAG")
	apePreamble = []byte("APETAGEX")
)

// LocateId3v1 checks whether the stream ends with a 128-byte ID3v1 tag,
// returning its [start, end) range. When found, the reader is left at `start`;
// otherwise it is left at EOF.
func LocateId3v1(r io.ReadSeeker) (start, end int64, found bool, err error) {
	// Peek 3 bytes before the -128 mark so the "TAG" substring inside an
	// "APETAGEX" header/footer at -131 is not mistaken for an ID3v1 tag
	// (upstream lofty-rs issue #762 / commit 700bf53d).
	if _, err = r.Seek(-(Id3v1Size + 3), io.SeekEnd); err == nil {
		var header [8]byte
		if _, err = io.ReadFull(r, header[:]); err != nil {
			return 0, 0, false, err
		}
		if bytes.Equal(header[:], apePreamble) || !bytes.Equal(header[3:6], id3v1Marker) {
			if _, err = r.Seek(0, io.SeekEnd); err != nil {
				return 0, 0, false, err
			}
			return 0, 0, false, nil
		}
		if start, err = r.Seek(-5, io.SeekCurrent); err != nil {
			return 0, 0, false, err
		}
		return start, start + Id3v1Size, true, nil
	}

	// Stream is shorter than 131 bytes, so no APE preamble collision is possible;
	// still allow standalone 128..130 byte ID3v1 streams.
	if _, err = r.Seek(-Id3v1Size, io.SeekEnd); err != nil {
		if _, err2 := r.Seek(0, io.SeekEnd); err2 != nil {
			return 0, 0, false, err2
		}
		return 0, 0, false, nil
	}
	var header [3]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		return 0, 0, false, err
	}
	if start, err = r.Seek(-3, io.SeekCurrent); err != nil {
		return 0, 0, false, err
	}
	if !bytes.Equal(header[:], id3v1Marker) {
		if _, err = r.Seek(0, io.SeekEnd); err != nil {
			return 0, 0, false, err
		}
		return 0, 0, false, nil
	}
	return start, start + Id3v1Size, true, nil
}
