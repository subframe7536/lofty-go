package meta

import (
	"os"
	"path/filepath"
	"strings"
)

var extensions = [...]string{
	"aac", "ape", "aiff", "aif", "afc", "aifc", "dsf", "mp3", "mp2", "mp1", "wav",
	"wv", "opus", "flac", "ogg", "mp4", "m4a", "m4b", "m4p", "m4r", "m4v", "3gp",
	"mpc", "mp+", "mpp", "spx",
}

// Extensions returns a list of common audio extensions.
//
// This contains a bunch of common extensions for all supported [FileType]s, and
// can be used as a filter when scanning directories. The returned slice is a
// copy and may be modified freely.
func Extensions() []string { return append([]string(nil), extensions[:]...) }

// FileType is the type of file read.
type FileType struct {
	kind       fileTypeKind
	customName string
}

type fileTypeKind uint8

const (
	fileTypeAac fileTypeKind = iota + 1
	fileTypeAiff
	fileTypeApe
	fileTypeDsf
	fileTypeFlac
	fileTypeMpeg
	fileTypeMp4
	fileTypeMpc
	fileTypeOpus
	fileTypeVorbis
	fileTypeSpeex
	fileTypeWav
	fileTypeWavPack
	fileTypeCustom
)

var (
	// FileTypeAac is Advanced Audio Coding (AAC/ADTS).
	FileTypeAac = FileType{kind: fileTypeAac}
	// FileTypeAiff is Audio Interchange File Format.
	FileTypeAiff = FileType{kind: fileTypeAiff}
	// FileTypeApe is Monkey's Audio.
	FileTypeApe = FileType{kind: fileTypeApe}
	// FileTypeDsf is DSD Stream File.
	FileTypeDsf = FileType{kind: fileTypeDsf}
	// FileTypeFlac is Free Lossless Audio Codec.
	FileTypeFlac = FileType{kind: fileTypeFlac}
	// FileTypeMpeg is MPEG-1/2 Audio (MP1, MP2, MP3).
	FileTypeMpeg = FileType{kind: fileTypeMpeg}
	// FileTypeMp4 is MPEG-4 Audio (M4A, M4B, etc.).
	FileTypeMp4 = FileType{kind: fileTypeMp4}
	// FileTypeMpc is Musepack.
	FileTypeMpc = FileType{kind: fileTypeMpc}
	// FileTypeOpus is Opus.
	FileTypeOpus = FileType{kind: fileTypeOpus}
	// FileTypeVorbis is Ogg Vorbis.
	FileTypeVorbis = FileType{kind: fileTypeVorbis}
	// FileTypeSpeex is Speex.
	FileTypeSpeex = FileType{kind: fileTypeSpeex}
	// FileTypeWav is Waveform Audio.
	FileTypeWav = FileType{kind: fileTypeWav}
	// FileTypeWavPack is WavPack.
	FileTypeWavPack = FileType{kind: fileTypeWavPack}
)

// FileTypeCustom creates a custom [FileType] for a name registered through
// [RegisterCustomResolver].
func FileTypeCustom(name string) FileType { return FileType{kind: fileTypeCustom, customName: name} }

// FileTypeVariants returns all non-custom [FileType] variants. The returned
// slice is a copy and may be modified freely.
func FileTypeVariants() []FileType {
	return []FileType{
		FileTypeAac, FileTypeAiff, FileTypeApe, FileTypeDsf, FileTypeFlac, FileTypeMpeg,
		FileTypeMp4, FileTypeMpc, FileTypeOpus, FileTypeVorbis, FileTypeSpeex,
		FileTypeWav, FileTypeWavPack,
	}
}

func (t FileType) String() string {
	switch t.kind {
	case fileTypeAac:
		return "AAC"
	case fileTypeAiff:
		return "AIFF"
	case fileTypeApe:
		return "APE"
	case fileTypeDsf:
		return "DSF"
	case fileTypeFlac:
		return "FLAC"
	case fileTypeMpeg:
		return "MPEG"
	case fileTypeMp4:
		return "MP4"
	case fileTypeMpc:
		return "MPC"
	case fileTypeOpus:
		return "OPUS"
	case fileTypeVorbis:
		return "VORBIS"
	case fileTypeSpeex:
		return "SPEEX"
	case fileTypeWav:
		return "WAV"
	case fileTypeWavPack:
		return "WAVPACK"
	default:
		return "CUSTOM:" + t.customName
	}
}

// IsCustom reports whether this is a custom file type.
func (t FileType) IsCustom() bool { return t.kind == fileTypeCustom }

// CustomName returns the custom file type's name.
func (t FileType) CustomName() string { return t.customName }

// Equal reports whether two file types are the same.
func (t FileType) Equal(other FileType) bool {
	return t.kind == other.kind && t.customName == other.customName
}

// PrimaryTagType returns the file type's "primary" [TagType], or the one most
// likely to be used in the target format.
func (t FileType) PrimaryTagType() TagType {
	switch t.kind {
	case fileTypeAac, fileTypeAiff, fileTypeDsf, fileTypeMpeg, fileTypeWav:
		return TagTypeID3v2
	case fileTypeApe, fileTypeMpc, fileTypeWavPack:
		return TagTypeApe
	case fileTypeFlac, fileTypeOpus, fileTypeVorbis, fileTypeSpeex:
		return TagTypeVorbisComments
	case fileTypeMp4:
		return TagTypeMp4Ilst
	default:
		return lookupResolver(t.customName).PrimaryTagType()
	}
}

// TagSupport describes how this [FileType] supports the given [TagType].
func (t FileType) TagSupport(tagType TagType) TagSupport {
	if t.kind == fileTypeCustom {
		return lookupResolver(t.customName).TagSupport(tagType)
	}
	supported, readOnly := tagSupportFor(tagType)
	if !supported[t.kind] {
		return TagSupportUnsupported
	}
	if readOnly[t.kind] {
		return TagSupportReadOnly
	}
	return TagSupportReadWrite
}

// tagSupportFor returns (supported, readOnly) bitmaps indexed by fileTypeKind.
func tagSupportFor(tagType TagType) ([fileTypeCustom + 1]bool, [fileTypeCustom + 1]bool) {
	var supported, readOnly [fileTypeCustom + 1]bool
	set := func(m *[fileTypeCustom + 1]bool, kinds ...fileTypeKind) {
		for _, k := range kinds {
			m[k] = true
		}
	}
	switch tagType {
	case TagTypeApe:
		set(&supported, fileTypeApe, fileTypeMpeg, fileTypeMpc, fileTypeWavPack)
	case TagTypeID3v1:
		set(&supported, fileTypeAac, fileTypeApe, fileTypeMpeg, fileTypeWavPack, fileTypeMpc)
		set(&readOnly, fileTypeMpc)
	case TagTypeID3v2:
		set(&supported, fileTypeAac, fileTypeAiff, fileTypeDsf, fileTypeMpeg, fileTypeWav, fileTypeApe, fileTypeFlac, fileTypeMpc)
		set(&readOnly, fileTypeApe, fileTypeFlac, fileTypeMpc)
	case TagTypeLyrics3, TagTypeLyrics3v2:
		set(&supported, fileTypeMpeg)
	case TagTypeMp4Ilst:
		set(&supported, fileTypeMp4)
	case TagTypeVorbisComments:
		set(&supported, fileTypeFlac, fileTypeOpus, fileTypeSpeex, fileTypeVorbis)
	case TagTypeRiffInfo:
		set(&supported, fileTypeWav)
	case TagTypeAiffText:
		set(&supported, fileTypeAiff)
	}
	return supported, readOnly
}

// FileTypeFromExt attempts to extract a [FileType] from an extension.
func FileTypeFromExt(ext string) (FileType, bool) {
	ext = asciiLower(ext)

	// Give custom resolvers priority
	if getGlobalOptions().UseCustomResolvers {
		if ft, ok := lookupResolverByExtension(ext); ok {
			return ft, true
		}
	}

	switch ext {
	case "aac":
		return FileTypeAac, true
	case "ape":
		return FileTypeApe, true
	case "aiff", "aif", "afc", "aifc":
		return FileTypeAiff, true
	case "dsf":
		return FileTypeDsf, true
	case "mp3", "mp2", "mp1":
		return FileTypeMpeg, true
	case "wav", "wave":
		return FileTypeWav, true
	case "wv":
		return FileTypeWavPack, true
	case "opus":
		return FileTypeOpus, true
	case "flac":
		return FileTypeFlac, true
	case "ogg":
		return FileTypeVorbis, true
	case "mp4", "m4a", "m4b", "m4p", "m4r", "m4v", "3gp":
		return FileTypeMp4, true
	case "mpc", "mp+", "mpp":
		return FileTypeMpc, true
	case "spx":
		return FileTypeSpeex, true
	}
	return FileType{}, false
}

// FileTypeFromPath attempts to determine a [FileType] from a path.
//
// The extension follows Rust's `Path::extension`: it is taken from the final
// path component using the OS path separators, a leading dot alone (".mp3")
// does not start an extension, and a path without an extension (or with an
// empty one, "name.") yields no type.
func FileTypeFromPath(p string) (FileType, bool) {
	ext, ok := pathExtension(p)
	if !ok {
		return FileType{}, false
	}
	return FileTypeFromExt(ext)
}

// pathExtension mirrors Rust's `Path::extension`, additionally treating an
// empty extension as absent.
func pathExtension(p string) (string, bool) {
	if p == "" {
		return "", false
	}
	// Strip trailing separators like Rust's component iteration does.
	for len(p) > 0 && os.IsPathSeparator(p[len(p)-1]) {
		p = p[:len(p)-1]
	}
	if p == "" {
		return "", false
	}
	base := filepath.Base(p)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "", false
	}
	i := strings.LastIndexByte(base, '.')
	if i <= 0 || i == len(base)-1 {
		return "", false
	}
	return base[i+1:], true
}

// FileTypeFromBuffer attempts to extract a [FileType] from a buffer.
//
// NOTES:
//   - This **will not** search past tags at the start of the buffer.
//     For this behavior, use [Probe.GuessFileType].
func FileTypeFromBuffer(buf []byte) (FileType, bool) {
	if r, ok := fileTypeFromBufferInner(buf); ok {
		if r.determined {
			return r.fileType, true
		}
	}
	return FileType{}, false
}

// fileTypeGuessResult is the internal result of a guess.
type fileTypeGuessResult struct {
	fileType   FileType
	determined bool
	// id3Size is set when the stream starts with an ID3v2 tag.
	id3Size uint32
	// maybeJunk is true when the stream starts with potential junk data.
	maybeJunk bool
}

func fileTypeFromBufferInner(buf []byte) (fileTypeGuessResult, bool) {
	if len(buf) == 0 {
		return fileTypeGuessResult{}, false
	}
	if f, ok := quickTypeGuess(buf); ok {
		return fileTypeGuessResult{fileType: f, determined: true}, true
	}
	// Special case for ID3 — the bare minimum size for an ID3v2 header is 10 bytes.
	if len(buf) >= 10 && string(buf[:3]) == "ID3" {
		return fileTypeGuessResult{determined: false, id3Size: unsynchsafe32(buf[6:10])}, true
	}
	return fileTypeGuessResult{determined: false, maybeJunk: true}, true
}

func quickTypeGuess(buf []byte) (FileType, bool) {
	switch buf[0] {
	case 'M':
		if len(buf) >= 3 && string(buf[:3]) == "MAC" {
			return FileTypeApe, true
		}
	case 'D':
		if len(buf) >= 4 && string(buf[:4]) == "DSD " {
			return FileTypeDsf, true
		}
	case 0xFF:
		if len(buf) >= 2 && verifyFrameSync([2]byte{buf[0], buf[1]}) {
			// ADTS and MPEG frame headers are way too similar.
			// If we have a version 1 (0b11) or version 2 (0b10) MPEG frame AND a layer
			// of 0b00, we can assume we have an ADTS header.
			if buf[1]&0b10000 > 0 && buf[1]&0b110 == 0 {
				return FileTypeAac, true
			}
			return FileTypeMpeg, true
		}
	case 'F':
		if len(buf) >= 12 && string(buf[:4]) == "FORM" {
			id := string(buf[8:12])
			if id == "AIFF" || id == "AIFC" {
				return FileTypeAiff, true
			}
			// Rust's match arm yields None here — the ftyp/MPCK fallbacks
			// are skipped for committed FORM buffers.
			return FileType{}, false
		}
	case 'O':
		if len(buf) >= 36 && string(buf[:4]) == "OggS" {
			if string(buf[29:35]) == "vorbis" {
				return FileTypeVorbis, true
			}
			if string(buf[28:36]) == "OpusHead" {
				return FileTypeOpus, true
			}
			if string(buf[28:36]) == "Speex   " {
				return FileTypeSpeex, true
			}
			return FileType{}, false
		}
	case 'f':
		if len(buf) >= 4 && string(buf[:4]) == "fLaC" {
			return FileTypeFlac, true
		}
	case 'R':
		if len(buf) >= 12 && string(buf[:4]) == "RIFF" {
			if string(buf[8:12]) == "WAVE" {
				return FileTypeWav, true
			}
			return FileType{}, false
		}
	case 'w':
		if len(buf) >= 4 && string(buf[:4]) == "wvpk" {
			return FileTypeWavPack, true
		}
	}
	if len(buf) >= 8 && string(buf[4:8]) == "ftyp" {
		return FileTypeMp4, true
	}
	if (len(buf) >= 4 && string(buf[:4]) == "MPCK") || (len(buf) >= 3 && string(buf[:3]) == "MP+") {
		return FileTypeMpc, true
	}
	return FileType{}, false
}
