package id3v2

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/subframe7536/lofty-go/internal/util"
)

// ChapterFrame is an ID3v2 "CHAP" frame.
type ChapterFrame struct {
	header FrameHeader
	// Id is the chapter element ID.
	Id string
	// Times are the start and end times of this chapter in milliseconds.
	StartTime, EndTime uint32
	// Offsets are the start and end byte offsets of the chapter's audio content.
	StartOffset, EndOffset uint32
	// Children are extra frames embedded into the chapter.
	Children *FrameList
}

// NewChapterFrame creates a new [ChapterFrame].
func NewChapterFrame(id string, startTime, endTime, startOffset, endOffset uint32, children *FrameList) *ChapterFrame {
	if children == nil {
		children = NewFrameList()
	}
	return &ChapterFrame{
		header:      NewFrameHeader(FrameIdValid("CHAP"), FrameFlags{}),
		Id:          id,
		StartTime:   startTime,
		EndTime:     endTime,
		StartOffset: startOffset,
		EndOffset:   endOffset,
		Children:    children,
	}
}

func (f *ChapterFrame) ID() FrameId            { return f.header.ID }
func (f *ChapterFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *ChapterFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *ChapterFrame) frameEqual(other Frame) bool {
	o, ok := other.(*ChapterFrame)
	return ok && f.Id == o.Id
}

func (f *ChapterFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *ChapterFrame) frameName() string          { return "Chapter" }

// Title returns the title of this chapter (a TIT2 child).
func (f *ChapterFrame) Title() (string, bool) { return f.Children.GetText(FrameIdValid("TIT2")) }

// Subtitle returns the subtitle of this chapter (a TIT3 child).
func (f *ChapterFrame) Subtitle() (string, bool) { return f.Children.GetText(FrameIdValid("TIT3")) }

func (f *ChapterFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	if ctx.version == id3v2Version2 {
		return nil, frameEncodingUnsupportedVersion(ctx.version)
	}
	var content bytes.Buffer
	idBytes, err := util.TextEncodingLatin1.Encode(f.Id, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	content.Write(idBytes)
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], f.StartTime)
	content.Write(tmp[:])
	binary.BigEndian.PutUint32(tmp[:], f.EndTime)
	content.Write(tmp[:])
	binary.BigEndian.PutUint32(tmp[:], f.StartOffset)
	content.Write(tmp[:])
	binary.BigEndian.PutUint32(tmp[:], f.EndOffset)
	content.Write(tmp[:])
	frames := f.Children.CloneFrames()
	if ctx.version == id3v2Version3 {
		err = createItemsV3(&content, frames, ctx)
	} else {
		err = createItems(&content, frames, ctx)
	}
	if err != nil {
		return nil, err
	}
	return content.Bytes(), nil
}

func (f *ChapterFrame) cloneFrame() Frame {
	c := *f
	c.Children = f.Children.Clone()
	return &c
}

func parseChapterFrame(reader io.Reader, parseOptions ParseOptions, version Id3v2Version, frameFlags FrameFlags) (*ChapterFrame, error) {
	if version == id3v2Version2 {
		return nil, frameParseUnsupportedVersion(version)
	}
	idRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	var times [16]byte
	if _, err := io.ReadFull(reader, times[:]); err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	children, err := readAllFramesIntoList(reader, version, Id3v2TagFlags{}, parseOptions)
	if err != nil {
		return nil, err
	}
	return &ChapterFrame{
		header:      NewFrameHeader(FrameIdValid("CHAP"), frameFlags),
		Id:          idRes.Content,
		StartTime:   binary.BigEndian.Uint32(times[0:4]),
		EndTime:     binary.BigEndian.Uint32(times[4:8]),
		StartOffset: binary.BigEndian.Uint32(times[8:12]),
		EndOffset:   binary.BigEndian.Uint32(times[12:16]),
		Children:    children,
	}, nil
}

// CtocFlags are flags for a [ChapterTableOfContentsFrame].
type CtocFlags struct {
	// TopLevel indicates this TOC is at the root of the TOC tree.
	TopLevel bool
	// Ordered indicates the TOC entries are ordered.
	Ordered bool
}

// CtocFlagsFromU8 converts a byte to [CtocFlags].
func CtocFlagsFromU8(v uint8) CtocFlags {
	return CtocFlags{TopLevel: v&0b10 == 0b10, Ordered: v&0b1 == 0b1}
}

// AsU8 converts [CtocFlags] to a byte.
func (f CtocFlags) AsU8() uint8 {
	var ret uint8
	if f.Ordered {
		ret |= 0b1
	}
	if f.TopLevel {
		ret |= 0b10
	}
	return ret
}

// ChapterTableOfContentsFrame is an ID3v2 "CTOC" frame.
type ChapterTableOfContentsFrame struct {
	header FrameHeader
	// Id is the TOC element ID.
	Id string
	// TocFlags describes the behavior of this TOC frame.
	TocFlags CtocFlags
	// Entries are all [ChapterFrame] element IDs this TOC refers to.
	Entries []string
	// Children are extra frames embedded into the TOC.
	Children *FrameList
}

// NewChapterTableOfContentsFrame creates a new [ChapterTableOfContentsFrame].
func NewChapterTableOfContentsFrame(id string, flags CtocFlags, entries []string, children *FrameList) *ChapterTableOfContentsFrame {
	if children == nil {
		children = NewFrameList()
	}
	return &ChapterTableOfContentsFrame{
		header:   NewFrameHeader(FrameIdValid("CTOC"), FrameFlags{}),
		Id:       id,
		TocFlags: flags,
		Entries:  entries,
		Children: children,
	}
}

func (f *ChapterTableOfContentsFrame) ID() FrameId            { return f.header.ID }
func (f *ChapterTableOfContentsFrame) Flags() FrameFlags      { return f.header.Flags }
func (f *ChapterTableOfContentsFrame) SetFlags(fl FrameFlags) { f.header.Flags = fl }

func (f *ChapterTableOfContentsFrame) frameEqual(other Frame) bool {
	o, ok := other.(*ChapterTableOfContentsFrame)
	return ok && f.Id == o.Id
}

func (f *ChapterTableOfContentsFrame) frameIsEmpty() (bool, bool) { return false, false }
func (f *ChapterTableOfContentsFrame) frameName() string          { return "TableOfContents" }

// Title returns the title of this TOC (a TIT2 child).
func (f *ChapterTableOfContentsFrame) Title() (string, bool) {
	return f.Children.GetText(FrameIdValid("TIT2"))
}

// Subtitle returns the subtitle of this TOC (a TIT3 child).
func (f *ChapterTableOfContentsFrame) Subtitle() (string, bool) {
	return f.Children.GetText(FrameIdValid("TIT3"))
}

func (f *ChapterTableOfContentsFrame) frameBytes(ctx *frameEncodingContext) ([]byte, error) {
	if ctx.version == id3v2Version2 {
		return nil, frameEncodingUnsupportedVersion(ctx.version)
	}
	if len(f.Entries) > 0xFF {
		return nil, frameEncodingError(None[FrameId](), &TooMuchDataError{})
	}
	var content bytes.Buffer
	idBytes, err := util.TextEncodingLatin1.Encode(f.Id, true, ctx.writeOptions.LossyTextEncoding)
	if err != nil {
		return nil, frameEncodingError(None[FrameId](), err)
	}
	content.Write(idBytes)
	content.WriteByte(f.TocFlags.AsU8())
	content.WriteByte(uint8(len(f.Entries)))
	for _, entry := range f.Entries {
		entryBytes, err := util.TextEncodingLatin1.Encode(entry, true, ctx.writeOptions.LossyTextEncoding)
		if err != nil {
			return nil, frameEncodingError(None[FrameId](), err)
		}
		content.Write(entryBytes)
	}
	frames := f.Children.CloneFrames()
	if ctx.version == id3v2Version3 {
		err = createItemsV3(&content, frames, ctx)
	} else {
		err = createItems(&content, frames, ctx)
	}
	if err != nil {
		return nil, err
	}
	return content.Bytes(), nil
}

func (f *ChapterTableOfContentsFrame) cloneFrame() Frame {
	c := *f
	c.Entries = append([]string(nil), f.Entries...)
	c.Children = f.Children.Clone()
	return &c
}

func parseChapterTableOfContentsFrame(reader io.Reader, parseOptions ParseOptions, version Id3v2Version, frameFlags FrameFlags) (*ChapterTableOfContentsFrame, error) {
	if version == id3v2Version2 {
		return nil, frameParseUnsupportedVersion(version)
	}
	idRes, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
	if err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	var flagBytes [2]byte
	if _, err := io.ReadFull(reader, flagBytes[:]); err != nil {
		return nil, frameParseError(None[FrameId](), err)
	}
	flags := CtocFlagsFromU8(flagBytes[0])
	entryCount := flagBytes[1]
	var entries []string
	for i := 0; i < int(entryCount); i++ {
		entry, err := decodeText(reader, textDecodeOpts(util.TextEncodingLatin1, true))
		if err != nil {
			return nil, frameParseError(None[FrameId](), err)
		}
		entries = append(entries, entry.Content)
	}
	children, err := readAllFramesIntoList(reader, version, Id3v2TagFlags{}, parseOptions)
	if err != nil {
		return nil, err
	}
	return &ChapterTableOfContentsFrame{
		header:   NewFrameHeader(FrameIdValid("CTOC"), frameFlags),
		Id:       idRes.Content,
		TocFlags: flags,
		Entries:  entries,
		Children: children,
	}, nil
}
