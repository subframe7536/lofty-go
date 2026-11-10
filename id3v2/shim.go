package id3v2

// Package id3v2 implements the ID3v2 tagging format (v2.2, v2.3, v2.4).

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
)

type (
	Id3v2Version = meta.Id3v2Version
	TextEncoding = meta.TextEncoding
)

const (
	Id3v2Version2 = meta.Id3v2Version2
	Id3v2Version3 = meta.Id3v2Version3
	Id3v2Version4 = meta.Id3v2Version4
)

const (
	TextEncodingLatin1  = meta.TextEncodingLatin1
	TextEncodingUTF16   = meta.TextEncodingUTF16
	TextEncodingUTF16BE = meta.TextEncodingUTF16BE
	TextEncodingUTF8    = meta.TextEncodingUTF8
)
