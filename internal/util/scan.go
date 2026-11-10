package util

import "io"

// chunkScanSize is the read granularity used by [ChunkedScan].
const chunkScanSize = 8192

// ChunkedScan reads r in chunks of at most 8 KiB, looking for a match
// identified by matchFn. Each call receives a window made of the trailing
// `carry` bytes of the previous window followed by newly read data (so a
// match may straddle a chunk boundary); matchFn returns the index within
// that window where a match begins, or -1 for no match.
//
// At most max bytes are scanned: max < 0 means unbounded (scan until EOF),
// max == 0 scans nothing. The returned offset is relative to the position
// r was at when called.
//
// The reader's position on return is unspecified — callers that need an
// exact position must Seek.
func ChunkedScan(r io.ReadSeeker, carry int, max int64, matchFn func(window []byte) int) (int64, bool, error) {
	var buf [chunkScanSize]byte
	var tail []byte
	scanned := int64(0) // new bytes read from the region
	for max < 0 || scanned < max {
		want := int64(len(buf))
		if max >= 0 && want > max-scanned {
			want = max - scanned
		}
		if want <= 0 {
			break
		}
		n, rerr := r.Read(buf[:want])
		if n <= 0 {
			if rerr != nil && rerr != io.EOF {
				return 0, false, rerr
			}
			break
		}
		var window []byte
		if len(tail) == 0 {
			window = buf[:n]
		} else {
			window = make([]byte, len(tail)+n)
			copy(window, tail)
			copy(window[len(tail):], buf[:n])
		}
		if idx := matchFn(window); idx >= 0 {
			return scanned - int64(len(tail)) + int64(idx), true, nil
		}
		if carry > 0 {
			if len(window) >= carry {
				tail = append(tail[:0], window[len(window)-carry:]...)
			} else {
				tail = append(tail[:0], window...)
			}
		}
		scanned += int64(n)
		if n < int(want) || rerr == io.EOF {
			break
		}
	}
	return 0, false, nil
}
