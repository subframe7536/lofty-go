package meta_test

// Test-only bridges: unqualified aliases to internal/util, internal/meta and
// format-package helpers that the meta_test package files use. No non-test
// file references them, so they live in a _test.go file.

import (
	"github.com/subframe7536/lofty-go/ape"
	"github.com/subframe7536/lofty-go/id3v1"
	"github.com/subframe7536/lofty-go/id3v2"
	"github.com/subframe7536/lofty-go/iff"
	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/mp4"
	"github.com/subframe7536/lofty-go/vorbis"

	// Real Ogg writer for TestOggIssue37Write; registers FileTypeVorbis only,
	// so it cannot clash with the MPEG/FLAC stubs in helpers_test.go.
	_ "github.com/subframe7536/lofty-go/ogg"
)

var (
	tryParseTimestamp = util.TryParseTimestamp
	writeTag          = meta.WriteTag
	id3v2TagFromTag   = id3v2.Id3v2TagFromTag
	id3v2TagToTag     = id3v2.Id3v2TagToTag
)

var (
	testAsset     = testutil.Fixture
	tempFile      = testutil.TempFile
	toolInstalled = testutil.ToolInstalled
)

type (
	AiffTextTag       = iff.AiffTextTag
	RiffInfoTag       = iff.RiffInfoTag
	ApeTag            = ape.ApeTag
	Id3v1Tag          = id3v1.Id3v1Tag
	VorbisCommentsTag = vorbis.VorbisCommentsTag
)

var (
	AiffTextTagFromTag        = iff.AiffTextTagFromTag
	RiffInfoTagFromTag        = iff.RiffInfoTagFromTag
	ApeTagFromTag             = ape.ApeTagFromTag
	Id3v1TagFromTag           = id3v1.Id3v1TagFromTag
	VorbisCommentsTagFromTag  = vorbis.VorbisCommentsTagFromTag
	VorbisCommentsTagForWrite = vorbis.VorbisCommentsTagForWrite
	ParseIlst                 = mp4.ParseIlst
	EncodeMp4Ilst             = mp4.EncodeMp4Ilst
)

var noProps = meta.ParseOptions{
	ReadProperties:      false,
	ReadTags:            true,
	ParsingMode:         meta.ParsingModeBestAttempt,
	MaxJunkBytes:        1024,
	ReadCoverArt:        true,
	ImplicitConversions: true,
}
