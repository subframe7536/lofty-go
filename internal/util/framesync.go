package util

import "io"

// VerifyFrameSync verifies a 2-byte MPEG/AAC frame sync.
func VerifyFrameSync(b [2]byte) bool {
	// 11 bits of 1, or MPEG-2.5's "looser" sync handled in header parse.
	return b[0] == 0xFF && (b[1]&0xE0) == 0xE0
}

// searchForFrameSync finds the first frame sync position in the reader (for probing).
// Returns (-1, nil) when no sync was found.
func SearchForFrameSync(reader io.Reader) (int64, error) {
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
