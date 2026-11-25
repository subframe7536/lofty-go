package meta

import "io"

// Accessor provides common metadata field accessors across tag formats,
// mirroring Rust's `lofty::tag::Accessor`.
type Accessor interface {
	Artist() (string, bool)
	SetArtist(value string)
	RemoveArtist()

	Title() (string, bool)
	SetTitle(value string)
	RemoveTitle()

	Album() (string, bool)
	SetAlbum(value string)
	RemoveAlbum()

	Genre() (string, bool)
	SetGenre(value string)
	RemoveGenre()

	Comment() (string, bool)
	SetComment(value string)
	RemoveComment()

	Track() (uint32, bool)
	SetTrack(value uint32)
	RemoveTrack()

	TrackTotal() (uint32, bool)
	SetTrackTotal(value uint32)
	RemoveTrackTotal()

	Disk() (uint32, bool)
	SetDisk(value uint32)
	RemoveDisk()

	DiskTotal() (uint32, bool)
	SetDiskTotal(value uint32)
	RemoveDiskTotal()

	Date() Option[Timestamp]
	SetDate(value Timestamp)
	RemoveDate()
}

// TagExt defines the common persistence and inspection methods shared by tag
// types, mirroring Rust's `lofty::tag::TagExt`.
type TagExt interface {
	TagType() TagType
	Len() int
	IsEmpty() bool
	Clear()
	SaveTo(file FileLike, writeOptions WriteOptions) error
	SaveToPath(path string, writeOptions WriteOptions) error
	DumpTo(w io.Writer, writeOptions WriteOptions) error
	RemoveFrom(file FileLike, writeOptions WriteOptions) error
	RemoveFromPath(path string, writeOptions WriteOptions) error
}

// TaggedFileExt defines the common operations on [TaggedFile] and
// [BoundTaggedFile], mirroring Rust's `lofty::file::TaggedFileExt`.
type TaggedFileExt interface {
	FileType() FileType
	Properties() FileProperties
	Tags() []*Tag
	TagTypes() []TagType
	PrimaryTag() *Tag
	PrimaryTagMut() *Tag
	PrimaryTagMutOrInsert() *Tag
	FirstTag() *Tag
	Tag(tagType TagType) *Tag
	ContainsTag(tagType TagType) bool
	InsertTag(tag *Tag) *Tag
	Remove(target *Tag) bool
	RemoveTagType(tagType TagType) *Tag
	RemoveTags(tagType TagType)
	Clear()
	PrimaryTagType() TagType
	TagSupport(tagType TagType) TagSupport
	SupportsTagType(tagType TagType) bool
	ChangeFileType(fileType FileType)
	Conflicts() [][2]TagType
}
