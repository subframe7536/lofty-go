package meta

import "io"

// Format readers/writers live in their own packages and are not registered
// inside internal/meta tests, so register tiny fakes for MPEG and FLAC.

var (
	fakeWrites []string
	// fakeReadConsume is how many bytes the fake readers consume.
	fakeReadConsume = 7
)

func init() {
	for _, ft := range []FileType{FileTypeMpeg, FileTypeFlac} {
		RegisterFormatReader(ft, func(rs io.ReadSeeker, opts ParseOptions) (*TaggedFile, error) {
			buf := make([]byte, fakeReadConsume)
			_, _ = io.ReadFull(rs, buf)
			return NewTaggedFile(ft, nil, FileProperties{}), nil
		})
		RegisterFormatWriter(ft, func(v *VerifiedFile, tag *Tag, wo WriteOptions) error {
			fakeWrites = append(fakeWrites, v.Format().String()+":"+tag.TagType().String())
			return nil
		})
	}
}

func resetFakeWrites() { fakeWrites = nil }

// mp3Bytes is a buffer that content-sniffs as MPEG.
func mp3Bytes() []byte {
	b := make([]byte, 64)
	b[0], b[1], b[2] = 0xFF, 0xFB, 0x90
	return b
}

// flacBytes is a buffer that content-sniffs as FLAC.
func flacBytes() []byte {
	b := make([]byte, 64)
	copy(b, "fLaC")
	return b
}

func textTag(tt TagType, val string) *Tag {
	t := NewTag(tt)
	if !t.InsertText(ItemKeyTrackTitle, val) {
		panic("insert failed")
	}
	return t
}
