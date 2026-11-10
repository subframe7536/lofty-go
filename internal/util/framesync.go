package util

import "io"

// VerifyFrameSync verifies a 2-byte MPEG/AAC frame sync.
func VerifyFrameSync(b [2]byte) bool {
	// 11 bits of 1, or MPEG-2.5's "looser" sync handled in header parse.
	return b[0] == 0xFF && (b[1]&0xE0) == 0xE0
}

func frameSyncMatch(window []byte) int {
	for i := 0; i+1 < len(window); i++ {
		if window[i] == 0xFF && (window[i+1]&0xE0) == 0xE0 {
			return i
		}
	}
	return -1
}

// searchForFrameSync finds the first frame sync position in the reader (for probing).
// Returns (-1, nil) when no sync was found.
func SearchForFrameSync(reader io.Reader) (int64, error) {
	if rs, ok := reader.(io.ReadSeeker); ok {
		return SearchForFrameSyncSeeking(rs, -1)
	}
	var buf [1]byte
	var prev byte
	idx := int64(0)
	for {
		_, err := reader.Read(buf[:])
		if err != nil {
			if err == io.EOF {
				return -1, nil
			}
			return 0, err
		}
		if prev == 0xFF && (buf[0]&0xE0) == 0xE0 {
			return idx - 1, nil
		}
		prev = buf[0]
		idx++
	}
}

// SearchForFrameSyncSeeking is the [io.ReadSeeker] variant of
// [SearchForFrameSync], scanning at most max bytes (max < 0 means until
// EOF). On a match the reader is positioned just past the sync, matching
// [SearchForFrameSync]'s contract.
func SearchForFrameSyncSeeking(r io.ReadSeeker, max int64) (int64, error) {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	offset, found, err := ChunkedScan(r, 1, max, frameSyncMatch)
	if err != nil {
		return 0, err
	}
	if !found {
		return -1, nil
	}
	if _, err := r.Seek(start+offset+2, io.SeekStart); err != nil {
		return 0, err
	}
	return offset, nil
}
