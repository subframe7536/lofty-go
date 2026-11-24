package lofty_test

// Test-only bridges: unqualified aliases to internal/util, internal/meta and
// format-package helpers that the lofty_test package files use. No non-test
// file references them, so they live in a _test.go file.

import (
	"github.com/subframe7536/lofty-go/id3v2"
	"github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
	"github.com/subframe7536/lofty-go/lyrics3"
)

const (
	littleEndian = util.LittleEndian
	bigEndian    = util.BigEndian
)

var (
	latin1Decode     = util.Latin1Decode
	utf16DecodeBytes = util.Utf16DecodeBytes
)

var (
	synchsafe32   = util.Synchsafe32
	unsynchsafe32 = util.Unsynchsafe32
)

var (
	findLyrics3 = lyrics3.FindLyrics3
	parseId3v2  = id3v2.ParseId3v2
)

var (
	readFormat = meta.ReadFormat
	writeTag   = meta.WriteTag
	removeTag  = meta.RemoveTag
)

var id3v2TagFromTag = id3v2.Id3v2TagFromTag
