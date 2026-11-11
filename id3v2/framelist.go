package id3v2

import (
	"strings"

	meta "github.com/subframe7536/lofty-go/internal/meta"

	"github.com/subframe7536/lofty-go/internal/util"
)

// onePerTagFrameIDs are frame IDs that can only appear once in a tag.
var onePerTagFrameIDs = map[string]bool{
	"MCDI": true, "ETCO": true, "MLLT": true, "SYTC": true, "RVRB": true,
	"PCNT": true, "RBUF": true, "POSS": true, "OWNE": true, "SEEK": true,
	"ASPI": true,
}

// FrameList is a list of ID3v2 [Frame]s that handles deduplication.
//
// This is the backing storage for [Id3v2Tag], [ChapterFrame], and
// [ChapterTableOfContentsFrame].
type FrameList struct {
	frames []Frame
}

// NewFrameList creates a new, empty [FrameList].
func NewFrameList() *FrameList { return &FrameList{} }

// Frames returns the frames in the list.
func (l *FrameList) Frames() []Frame { return l.frames }

// SetFrames replaces the list's frames.
func (l *FrameList) SetFrames(frames []Frame) { l.frames = frames }

// Len returns the number of frames in the list.
func (l *FrameList) Len() int { return len(l.frames) }

// IsEmpty reports whether the list is empty.
func (l *FrameList) IsEmpty() bool { return len(l.frames) == 0 }

// Get returns the first frame with the given ID.
func (l *FrameList) Get(id FrameId) Frame {
	for _, f := range l.frames {
		if f.ID() == id {
			return f
		}
	}
	return nil
}

// GetText returns the text for a frame, if it is a [TextInformationFrame].
func (l *FrameList) GetText(id FrameId) (string, bool) {
	if tf, ok := l.Get(id).(*TextInformationFrame); ok {
		return tf.Value, true
	}
	return "", false
}

// GetTexts returns all of the values for a text frame (ID3v2.4 multi-value).
func (l *FrameList) GetTexts(id FrameId) ([]string, bool) {
	if tf, ok := l.Get(id).(*TextInformationFrame); ok {
		return strings.Split(tf.Value, "\x00"), true
	}
	return nil, false
}

// GetUserText returns the text for a user-defined (TXXX) frame by description.
func (l *FrameList) GetUserText(description string) (string, bool) {
	for _, f := range l.frames {
		if f.ID().String() != "TXXX" {
			continue
		}
		if tf, ok := f.(*ExtendedTextFrame); ok && tf.Description == description {
			return tf.Content, true
		}
	}
	return "", false
}

// InsertUserText inserts a new user-defined text frame (TXXX), replacing any
// TXXX frame with the same description.
func (l *FrameList) InsertUserText(description, content string) Frame {
	return l.Insert(NewExtendedTextFrame(util.TextEncodingUTF8, description, content))
}

// RemoveUserText removes a user-defined text frame (TXXX) by its description.
func (l *FrameList) RemoveUserText(description string) Frame {
	for i, f := range l.frames {
		if tf, ok := f.(*ExtendedTextFrame); ok && tf.Description == description {
			l.frames = append(l.frames[:i], l.frames[i+1:]...)
			return f
		}
	}
	return nil
}

// Insert inserts a frame into the list, replacing any frame that compares
// equal (same ID for most types; identifying field for others).
func (l *FrameList) Insert(frame Frame) Frame {
	if onePerTagFrameIDs[frame.ID().String()] {
		// Rust `remove` drains every frame with the ID; only the first is returned
		var ret Frame
		kept := l.frames[:0]
		for _, f := range l.frames {
			if f.ID() == frame.ID() {
				if ret == nil {
					ret = f
				}
				continue
			}
			kept = append(kept, f)
		}
		l.frames = kept
		l.Push(frame)
		return ret
	}

	for i, f := range l.frames {
		if f.frameEqual(frame) {
			replaced := f
			l.frames = append(l.frames[:i], l.frames[i+1:]...)
			l.Push(frame)
			return replaced
		}
	}
	l.Push(frame)
	return nil
}

// Push appends a frame without deduplication.
func (l *FrameList) Push(frame Frame) { l.frames = append(l.frames, frame) }

// TakeFirst removes and returns the first frame with the given ID.
func (l *FrameList) TakeFirst(id FrameId) Frame {
	for i, f := range l.frames {
		if f.ID() == id {
			l.frames = append(l.frames[:i], l.frames[i+1:]...)
			return f
		}
	}
	return nil
}

// Remove removes all frames with the given ID and returns them.
func (l *FrameList) Remove(id FrameId) []Frame {
	var removed []Frame
	kept := l.frames[:0]
	for _, f := range l.frames {
		if f.ID() == id {
			removed = append(removed, f)
		} else {
			kept = append(kept, f)
		}
	}
	l.frames = kept
	return removed
}

// RemovePictureType removes all pictures of the given type.
func (l *FrameList) RemovePictureType(pictureType meta.PictureType) {
	l.Retain(func(f Frame) bool {
		if pf, ok := f.(*AttachedPictureFrame); ok {
			return pf.Picture.PicType() != pictureType
		}
		return true
	})
}

// Clear removes all frames from the list.
func (l *FrameList) Clear() { l.frames = nil }

// Retain keeps only the frames that make predicate return true.
func (l *FrameList) Retain(predicate func(Frame) bool) {
	kept := l.frames[:0]
	for _, f := range l.frames {
		if predicate(f) {
			kept = append(kept, f)
		}
	}
	l.frames = kept
}

// RetainMut is like [FrameList.Retain] but allows the predicate to mutate frames.
func (l *FrameList) RetainMut(predicate func(Frame) bool) { l.Retain(predicate) }

// InsertPicture inserts a [meta.Picture] as an APIC frame, replacing an existing
// picture of type Icon or OtherIcon.
func (l *FrameList) InsertPicture(picture meta.Picture) Frame {
	var ret Frame
	if picture.PicType() == meta.PictureTypeIcon || picture.PicType() == meta.PictureTypeOtherIcon {
		for i, f := range l.frames {
			if pf, ok := f.(*AttachedPictureFrame); ok && pf.Picture.PicType() == picture.PicType() {
				ret = f
				l.frames = append(l.frames[:i], l.frames[i+1:]...)
				break
			}
		}
	}
	l.Push(NewAttachedPictureFrame(util.TextEncodingUTF8, picture))
	return ret
}

// Clone returns a copy of the list with cloned frames.
func (l *FrameList) Clone() *FrameList {
	out := &FrameList{frames: make([]Frame, len(l.frames))}
	for i, f := range l.frames {
		out.frames[i] = f.cloneFrame()
	}
	return out
}

// CloneFrames returns cloned frames for writing (mirrors Rust Frame::borrow).
func (l *FrameList) CloneFrames() []Frame {
	out := make([]Frame, len(l.frames))
	for i, f := range l.frames {
		out[i] = f.cloneFrame()
	}
	return out
}
