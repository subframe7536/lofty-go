package util

import "io"

// chunkScanSize is the read granularity used by [ChunkedScan].
const chunkScanSize = 8192

// maxEmptyReads bounds consecutive (0, nil) reads before a fill gives up with
// [io.ErrNoProgress], mirroring [bufio.Reader].
const maxEmptyReads = 100

// scanReadFull reads until p is full or an error occurs. Unlike a single Read,
// a short read does not end the fill: the returned count is only short when err
// is non-nil. Bytes returned together with an error are kept.
func scanReadFull(r io.Reader, p []byte) (int, error) {
	n, empty := 0, 0
	for n < len(p) {
		m, err := r.Read(p[n:])
		n += m
		if err != nil {
			return n, err
		}
		if m > 0 {
			empty = 0
			continue
		}
		empty++
		if empty >= maxEmptyReads {
			return n, io.ErrNoProgress
		}
	}
	return n, nil
}

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
// Short reads are not treated as end of input: each window is filled
// completely unless the reader reports EOF or an error. Bytes returned with an
// error are still searched before the error is returned. A non-EOF read error
// is returned to the caller; io.EOF ends the scan without error.
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
		n, rerr := scanReadFull(r, buf[:want])
		if n > 0 {
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
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return 0, false, rerr
		}
		scanned += int64(n)
	}
	return 0, false, nil
}
