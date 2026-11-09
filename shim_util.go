package lofty

// Internal aliases to internal/util so that files still in the root package
// keep their original unqualified names until they move to their own
// subpackages.

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
	tryParseTimestamp = util.TryParseTimestamp
	synchsafe32       = util.Synchsafe32
	unsynchsafe32     = util.Unsynchsafe32
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

var genericTagToBogem = id3v2.GenericTagToBogem
