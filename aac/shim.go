package aac

import (
	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Re-exported property types for AAC audio properties.
type (
	MpegVersion     = meta.MpegVersion
	AudioObjectType = meta.AudioObjectType
)

const (
	MpegVersion1  = meta.MpegVersion1
	MpegVersion2  = meta.MpegVersion2
	MpegVersion25 = meta.MpegVersion25
	MpegVersion4  = meta.MpegVersion4
)
