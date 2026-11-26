package mp4

// Port of lofty-rs src/mp4/ilst — the `Ilst`/`Atom`/`AtomData` model.

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strings"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/util"
)

// AtomDataKind identifies the payload variant of an [AtomData], mirroring
// Rust's `AtomData` enum.
type AtomDataKind int8

const (
	AtomDataUTF8 AtomDataKind = iota
	AtomDataUTF16
	AtomDataBool
	AtomDataPicture
	AtomDataSignedInt
	AtomDataUnsignedInt
	AtomDataUnknown
)

// AtomData is a single `data` atom payload inside an [Atom].
type AtomData struct {
	Kind AtomDataKind
	Text string       // UTF8/UTF16
	Bool bool         // Bool
	Pic  meta.Picture // meta.Picture
	Int  int64        // SignedInt
	Uint uint64       // UnsignedInt
	Code DataType     // Unknown — the original data-type code
	Data []byte       // Unknown — raw payload
}

// DataType is the `data` atom's type code, mirroring Rust's `DataType`.
type DataType uint32

const (
	DataTypeReserved               DataType = 0
	DataTypeUtf8                   DataType = 1
	DataTypeUtf16                  DataType = 2
	DataTypeSJis                   DataType = 3
	DataTypeUtf8Sort               DataType = 4
	DataTypeUtf16Sort              DataType = 5
	DataTypeGif                    DataType = 12
	DataTypeJpeg                   DataType = 13
	DataTypePng                    DataType = 14
	DataTypeBeSignedInteger        DataType = 21
	DataTypeBeUnsignedInteger      DataType = 22
	DataTypeBeFloat32              DataType = 23
	DataTypeBeFloat64              DataType = 24
	DataTypeBmp                    DataType = 27
	DataTypeQuicktimeMetadata      DataType = 28
	DataTypeSigned8BitInteger      DataType = 65
	DataTypeBe16BitSignedInteger   DataType = 66
	DataTypeBe32BitSignedInteger   DataType = 67
	DataTypeBePointF32             DataType = 70
	DataTypeBeDimensionsF32        DataType = 71
	DataTypeBeRectF32              DataType = 72
	DataTypeBe64BitSignedInteger   DataType = 74
	DataTypeUnsigned8BitInteger    DataType = 75
	DataTypeBe16BitUnsignedInteger DataType = 76
	DataTypeBe32BitUnsignedInteger DataType = 77
	DataTypeBe64BitUnsignedInteger DataType = 78
	DataTypeAffineTransformF64     DataType = 79
)

// DataTypeOther wraps an unrecognized type code, mirroring `DataType::Other`.
func DataTypeOther(code uint32) DataType { return DataType(code) }

// AtomIdent is an atom identifier: either a four-byte code or a freeform
// `----` atom with mean/name sub-boxes.
type AtomIdent struct {
	Freeform bool
	Fourcc   [4]byte // when !Freeform
	Mean     string  // freeform
	Name     string  // freeform
}

// FourccIdent builds a four-byte atom identifier.
func FourccIdent(name string) AtomIdent {
	var i AtomIdent
	copy(i.Fourcc[:], mp4NameFromKey(name))
	return i
}

// FreeformIdent builds a `----:<mean>:<name>` identifier.
func FreeformIdent(mean, name string) AtomIdent {
	return AtomIdent{Freeform: true, Mean: mean, Name: name}
}

func ilstIdentFromString(key string) AtomIdent {
	if strings.HasPrefix(key, "----:") {
		rest := key[len("----:"):]
		mean, name := rest, ""
		if i := strings.IndexByte(rest, ':'); i >= 0 {
			mean, name = rest[:i], rest[i+1:]
		}
		return FreeformIdent(mean, name)
	}
	return FourccIdent(key)
}

// key returns the string form used for meta.ItemKey lookups.
func (i AtomIdent) key() string {
	if i.Freeform {
		return "----:" + i.Mean + ":" + i.Name
	}
	return mp4AtomName(i.Fourcc)
}

// DataType returns the [DataType] corresponding to this [AtomData] payload,
// mirroring Rust's `AtomData::data_type`.
func (d AtomData) DataType() DataType {
	switch d.Kind {
	case AtomDataUTF8:
		return DataTypeUtf8
	case AtomDataUTF16:
		return DataTypeUtf16
	case AtomDataSignedInt, AtomDataBool:
		return DataTypeBeSignedInteger
	case AtomDataUnsignedInt:
		return DataTypeBeUnsignedInteger
	case AtomDataPicture:
		m, ok := d.Pic.MimeType().Get()
		if !ok {
			return DataTypeReserved
		}
		switch m {
		case meta.MimeTypeGif:
			return DataTypeGif
		case meta.MimeTypeJpeg:
			return DataTypeJpeg
		case meta.MimeTypePng:
			return DataTypePng
		case meta.MimeTypeBmp:
			return DataTypeBmp
		default:
			return DataTypeReserved
		}
	default:
		return d.Code
	}
}

// AtomMismatchError is returned when attempting to use [Atom.Merge] with
// mismatching identifiers.
type AtomMismatchError struct{}

func (*AtomMismatchError) Error() string {
	return "attempted to use `Atom.Merge()` with mismatching identifiers"
}

// Atom is a single ilst metadata atom (one or more `data` payloads).
type Atom struct {
	Ident AtomIdent
	Data  []AtomData
}

// Merge appends the data payloads of `other` into `a`, returning
// [AtomMismatchError] if the identifiers do not match (Rust `Atom::merge`).
func (a *Atom) Merge(other Atom) error {
	if a.Ident != other.Ident {
		return &AtomMismatchError{}
	}
	a.Data = append(a.Data, other.Data...)
	return nil
}

// Flag atom identifiers matching Rust's `mp4::constants::flags`.
var (
	// Mp4FlagPodcast is the podcast flag (`pcst`).
	Mp4FlagPodcast = FourccIdent("pcst")
	// Mp4FlagGapless is the gapless playback flag (`pgap`).
	Mp4FlagGapless = FourccIdent("pgap")
	// Mp4FlagShowWork is the show work and movement flag (`shwm`).
	Mp4FlagShowWork = FourccIdent("shwm")
	// Mp4FlagHdVideo is the HD video flag (`hdvd`).
	Mp4FlagHdVideo = FourccIdent("hdvd")
	// Mp4FlagCompilation is the compilation flag (`cpil`).
	Mp4FlagCompilation = FourccIdent("cpil")
)

// Ilst is the MP4 `ilst` metadata tag.
type Ilst struct {
	atoms []Atom
}

// NewIlst returns an empty `Ilst`.
func NewIlst() *Ilst { return &Ilst{} }

// TagType returns [meta.TagTypeMp4Ilst].
func (l *Ilst) TagType() meta.TagType { return meta.TagTypeMp4Ilst }

// Len returns the number of atoms.
func (l *Ilst) Len() int { return len(l.atoms) }

// IsEmpty reports whether the tag has no atoms.
func (l *Ilst) IsEmpty() bool { return len(l.atoms) == 0 }

// Clear removes all atoms from the tag.
func (l *Ilst) Clear() { l.atoms = nil }

// Atoms returns the atom list (for iteration).
func (l *Ilst) Atoms() []Atom { return l.atoms }

// Get returns the atom with the given identifier.
func (l *Ilst) Get(ident AtomIdent) *Atom {
	for i := range l.atoms {
		if l.atoms[i].Ident == ident {
			return &l.atoms[i]
		}
	}
	return nil
}

// Contains reports whether an atom with the identifier exists.
func (l *Ilst) Contains(ident AtomIdent) bool { return l.Get(ident) != nil }

// Insert merges the atom's data into an existing atom with the same
// identifier, or appends it (Rust `Ilst::insert`). `covr` picture atoms are
// merged into a single shared atom.
func (l *Ilst) Insert(atom Atom) {
	if !atom.Ident.Freeform && atom.Ident.Fourcc == [4]byte{'c', 'o', 'v', 'r'} {
		allPics := len(atom.Data) > 0
		for _, d := range atom.Data {
			if d.Kind != AtomDataPicture {
				allPics = false
				break
			}
		}
		if allPics {
			for _, d := range atom.Data {
				l.InsertPicture(d.Pic)
			}
			return
		}
	}
	if existing := l.Get(atom.Ident); existing != nil {
		existing.Data = append(existing.Data, atom.Data...)
		return
	}
	l.atoms = append(l.atoms, atom)
}

// ReplaceAtom replaces any atom with the same identifier (Rust `replace_atom`).
func (l *Ilst) ReplaceAtom(atom Atom) {
	l.Remove(atom.Ident)
	l.atoms = append(l.atoms, atom)
}

// InsertPicture appends a picture to the shared `covr` atom.
func (l *Ilst) InsertPicture(p meta.Picture) {
	p.SetPicType(meta.PictureTypeOther)
	d := AtomData{Kind: AtomDataPicture, Pic: p}
	if a := l.Get(FourccIdent("covr")); a != nil {
		a.Data = append(a.Data, d)
		return
	}
	l.atoms = append(l.atoms, Atom{FourccIdent("covr"), []AtomData{d}})
}

// Remove deletes the atom with the identifier.
func (l *Ilst) Remove(ident AtomIdent) {
	l.removeWhere(func(a *Atom) bool { return a.Ident == ident })
}

// removeWhere deletes every atom matching the predicate (Rust
// `extract_if_exhaustive`), preserving order.
func (l *Ilst) removeWhere(match func(*Atom) bool) {
	kept := l.atoms[:0]
	for i := range l.atoms {
		if !match(&l.atoms[i]) {
			kept = append(kept, l.atoms[i])
		}
	}
	for i := len(kept); i < len(l.atoms); i++ {
		l.atoms[i] = Atom{}
	}
	l.atoms = kept
}

// Tag converts the `Ilst` to a generic [Tag] (Rust `From<Ilst> for Tag`).
// It is non-destructive; use [Ilst.SplitTag] for the destructive conversion.
func (l *Ilst) Tag() *meta.Tag {
	clone := &Ilst{atoms: cloneAtoms(l.atoms)}
	remainder, tag := clone.SplitTag()
	if util.GetGlobalOptions().PreserveFormatSpecificItems && len(remainder.atoms) > 0 {
		tag.SetCompanionTag(remainder)
	}
	return tag
}

// SplitTag destructively converts representable atoms into a generic [Tag],
// leaving unconvertible atoms (or unconvertible data payloads) in the receiver,
// mirroring Rust's `SplitTag::split_tag`. The receiver is returned as the
// remainder.
func (l *Ilst) SplitTag() (*Ilst, *meta.Tag) {
	tag := meta.NewTag(meta.TagTypeMp4Ilst)
	kept := make([]Atom, 0, len(l.atoms))

	for _, atom := range l.atoms {
		keyStr := atom.Ident.key()
		ik, mapped := meta.ItemKeyFromKey(meta.TagTypeMp4Ilst, keyStr)
		isCovr := !atom.Ident.Freeform && atom.Ident.Fourcc == [4]byte{'c', 'o', 'v', 'r'}
		if !mapped && !isCovr {
			kept = append(kept, atom)
			continue
		}

		var remaining []AtomData
		for _, d := range atom.Data {
			switch d.Kind {
			case AtomDataUTF8, AtomDataUTF16:
				if mapped {
					if ik == meta.ItemKeyPopularimeter {
						if rate, ok := u8FromString(d.Text); ok {
							if popm, ok := meta.PopularimeterMapped("", meta.TagTypeMp4Ilst, rate, 0); ok {
								tag.PushUnchecked(meta.NewTagItem(ik, meta.ItemValueText(popm.String())))
								continue
							}
						}
						remaining = append(remaining, d)
						continue
					}
					tag.PushUnchecked(meta.NewTagItem(ik, meta.ItemValueText(d.Text)))
				}
			case AtomDataBool:
				if mapped {
					text := "0"
					if d.Bool {
						text = "1"
					}
					tag.PushUnchecked(meta.NewTagItem(ik, meta.ItemValueText(text)))
				}
			case AtomDataPicture:
				if isCovr {
					tag.PushPicture(d.Pic)
				}
			case AtomDataUnknown:
				if d.Code == 0 && len(d.Data) >= 6 && !atom.Ident.Freeform {
					numKey, totKey := meta.ItemKeyTrackNumber, meta.ItemKeyTrackTotal
					fourcc := mp4AtomName(atom.Ident.Fourcc)
					if fourcc == "disk" {
						numKey, totKey = meta.ItemKeyDiscNumber, meta.ItemKeyDiscTotal
					} else if fourcc != "trkn" {
						remaining = append(remaining, d)
						continue
					}
					cur := binary.BigEndian.Uint16(d.Data[2:])
					tot := binary.BigEndian.Uint16(d.Data[4:])
					if cur > 0 {
						tag.InsertText(numKey, util.Itoa(int(cur)))
					}
					if tot > 0 {
						tag.InsertText(totKey, util.Itoa(int(tot)))
					}
					continue
				}
				remaining = append(remaining, d)
			default:
				remaining = append(remaining, d)
			}
		}
		if len(remaining) > 0 {
			kept = append(kept, Atom{Ident: atom.Ident, Data: remaining})
		}
	}
	l.atoms = kept

	// Advisory rating (rtng) — signed int or unknown bytes → ParentalAdvisory.
	if rating, ok := l.AdvisoryRating(); ok {
		tag.InsertText(meta.ItemKeyParentalAdvisory, util.Itoa(int(rating.AsU8())))
		l.Remove(FourccIdent("rtng"))
	}
	return l, tag
}

// pushConvertedAtom appends data to an atom with the same identifier in the
// converted atom list, or creates the atom (Rust conversion.rs `push_atom`).
func pushConvertedAtom(atoms []Atom, ident AtomIdent, data AtomData) []Atom {
	for i := range atoms {
		if atoms[i].Ident == ident {
			atoms[i].Data = append(atoms[i].Data, data)
			return atoms
		}
	}
	return append(atoms, Atom{Ident: ident, Data: []AtomData{data}})
}

// replaceConvertedAtom replaces the payload of an atom with the same
// identifier in the converted atom list, or creates it (Rust `replace_atom`).
func replaceConvertedAtom(atoms []Atom, ident AtomIdent, data AtomData) []Atom {
	for i := range atoms {
		if atoms[i].Ident == ident {
			atoms[i].Data = []AtomData{data}
			return atoms
		}
	}
	return append(atoms, Atom{Ident: ident, Data: []AtomData{data}})
}

// convertedTagAtoms converts generic items and pictures to atoms, mirroring
// Rust conversion.rs `from_tag`. Merging happens only within this converted
// list; companion atoms are concatenated separately by the caller.
func convertedTagAtoms(tag *meta.Tag) []Atom {
	var tracks, discs [2]meta.Option[uint16]
	atoms := make([]Atom, 0, tag.ItemCount())

	convertToUint := func(space *meta.Option[uint16], text string) {
		// Rust's convert_to_uint only overwrites the slot when parsing succeeds.
		if parsed := u16FromStringOpt(text); parsed.IsSome() {
			*space = parsed
		}
	}

	for _, item := range tag.Items() {
		// Rust discards items without a mapping (including `ItemKey::Unknown`).
		key, ok := item.Key().MapKey(meta.TagTypeMp4Ilst)
		if !ok {
			continue
		}
		text, isText := item.Value().Text()
		if !isText {
			continue
		}
		ident := ilstIdentFromString(key)
		switch item.Key() {
		case meta.ItemKeyTrackNumber:
			convertToUint(&tracks[0], text)
		case meta.ItemKeyTrackTotal:
			convertToUint(&tracks[1], text)
		case meta.ItemKeyDiscNumber:
			convertToUint(&discs[0], text)
		case meta.ItemKeyDiscTotal:
			convertToUint(&discs[1], text)
		case meta.ItemKeyFlagCompilation, meta.ItemKeyFlagPodcast:
			if b, ok := flagItemFromText(text); ok {
				atoms = replaceConvertedAtom(atoms, ident, AtomData{Kind: AtomDataBool, Bool: b})
			}
		case meta.ItemKeyParentalAdvisory:
			if v, ok := u8FromString(text); ok {
				if rating, ok := advisoryRatingFromU8(v); ok {
					atoms = replaceConvertedAtom(atoms, ident, AtomData{Kind: AtomDataSignedInt, Int: int64(rating)})
				}
			}
		case meta.ItemKeyPopularimeter:
			if popm, err := meta.PopularimeterFromStr(text); err == nil {
				atoms = pushConvertedAtom(atoms, ident, AtomData{Kind: AtomDataUTF8, Text: util.Itoa(int(popm.MappedValue(meta.TagTypeMp4Ilst)))})
			}
		default:
			atoms = pushConvertedAtom(atoms, ident, AtomData{Kind: AtomDataUTF8, Text: text})
		}
	}

	// Pictures form one covr atom inside the converted list. In particular,
	// they must not merge into a covr atom cloned from the companion tag.
	if pictures := tag.Pictures(); len(pictures) > 0 {
		data := make([]AtomData, 0, len(pictures))
		for _, pic := range pictures {
			pic.SetPicType(meta.PictureTypeOther)
			data = append(data, AtomData{Kind: AtomDataPicture, Pic: pic})
		}
		atoms = append(atoms, Atom{Ident: FourccIdent("covr"), Data: data})
	}

	createIntPair := func(fourcc string, pair [2]meta.Option[uint16]) {
		if pair[0].IsNone() && pair[1].IsNone() {
			return
		}
		data := []byte{0, 0, 0, 0, 0, 0, 0, 0}
		if v, ok := pair[0].Get(); ok {
			binary.BigEndian.PutUint16(data[2:], v)
		}
		if v, ok := pair[1].Get(); ok {
			binary.BigEndian.PutUint16(data[4:], v)
		}
		atoms = append(atoms, Atom{
			FourccIdent(fourcc),
			[]AtomData{{Kind: AtomDataUnknown, Code: 0, Data: data}},
		})
	}
	createIntPair("trkn", tracks)
	createIntPair("disk", discs)

	return atoms
}

// IlstFromTag converts a generic [Tag] to an `Ilst` (Rust `From<Tag> for Ilst`).
func IlstFromTag(tag *meta.Tag) *Ilst {
	base := NewIlst()
	if util.GetGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := tag.CompanionTag().(*Ilst); ok && comp != nil {
			base = comp.clone()
		}
	}
	return base.MergeTag(tag)
}

// MergeTag merges a generic [Tag] into this remainder, mirroring Rust's
// `MergeTag::merge_tag`. The receiver is mutated and returned.
func (l *Ilst) MergeTag(tag *meta.Tag) *Ilst {
	l.atoms = append(l.atoms, convertedTagAtoms(tag)...)
	return l
}

// clonePicture copies a picture and its data through the public builder API.
func clonePicture(p meta.Picture) meta.Picture {
	builder := meta.PictureUnchecked(bytes.Clone(p.Data())).PicType(p.PicType())
	if mimeType, ok := p.MimeType().Get(); ok {
		builder.MimeType(mimeType)
	}
	if description, ok := p.Description(); ok {
		builder.Description(description)
	}
	return builder.Build()
}

// cloneAtoms deep-copies atoms, their data slices, raw unknown payloads, and
// picture payloads so a merge can't mutate the source tag's atoms.
func cloneAtoms(atoms []Atom) []Atom {
	if atoms == nil {
		return nil
	}
	out := make([]Atom, len(atoms))
	for i, a := range atoms {
		data := make([]AtomData, len(a.Data))
		for j, d := range a.Data {
			switch d.Kind {
			case AtomDataUnknown:
				d.Data = bytes.Clone(d.Data)
			case AtomDataPicture:
				d.Pic = clonePicture(d.Pic)
			}
			data[j] = d
		}
		out[i] = Atom{Ident: a.Ident, Data: data}
	}
	return out
}

func (l *Ilst) clone() *Ilst {
	if l == nil {
		return nil
	}
	return &Ilst{atoms: cloneAtoms(l.atoms)}
}

// CloneCompanion implements [meta.CompanionCloner].
func (l *Ilst) CloneCompanion() any { return l.clone() }

// DumpTo writes the encoded `ilst` atom (with header) to a writer.
func (l *Ilst) DumpTo(writer io.Writer, writeOptions meta.WriteOptions) error {
	data, err := encodeIlst(l)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	_, err = writer.Write(data)
	return err
}

// encodeIlst encodes the `ilst` atom (with header), returning nil for empty tags.
func encodeIlst(ilst *Ilst) ([]byte, error) {
	var atoms bytes.Buffer

	writeDataAtom := func(buf *bytes.Buffer, dataType uint32, payload []byte) error {
		size, err := ilstAtomSize(16, len(payload))
		if err != nil {
			return err
		}
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], size)
		buf.Write(sz[:])
		buf.WriteString("data")
		// version (1), then the 24-bit type code
		buf.Write([]byte{0, byte(dataType >> 16), byte(dataType >> 8), byte(dataType)}) // #nosec G115 -- data type code is a small constant or validated <= 0xFFFFFF at the Unknown path; only the low 24 bits are written by design
		buf.Write([]byte{0, 0, 0, 0})
		buf.Write(payload)
		return nil
	}

	dataTypeFor := func(d AtomData) (uint32, []byte, error) {
		switch d.Kind {
		case AtomDataUTF8:
			return 1, []byte(d.Text), nil
		case AtomDataUTF16:
			return 2, utf16BEEncode(d.Text), nil
		case AtomDataBool:
			// Rust's `write_bool` stores a 1-byte signed integer (21).
			var b byte
			if d.Bool {
				b = 1
			}
			return 21, []byte{b}, nil
		case AtomDataPicture:
			code := uint32(0) // no MIME → implicit/reserved
			if mt, ok := d.Pic.MimeType().Get(); ok {
				switch mt {
				case meta.MimeTypeGif:
					code = 12
				case meta.MimeTypeJpeg:
					code = 13
				case meta.MimeTypePng:
					code = 14
				case meta.MimeTypeBmp:
					code = 27
				default:
					return 0, nil, &Mp4ParseError{Message: "attempted to write a picture with an unsupported MIME type"}
				}
			}
			return code, d.Pic.Data(), nil
		case AtomDataSignedInt:
			code, payload := intPayload(d.Int, true)
			return code, payload, nil
		case AtomDataUnsignedInt:
			code, payload := uintPayload(uint32(d.Uint)) // #nosec G115 -- mirrors Rust's u32 unsigned-int storage (parse_uint/write_unsigned_int); truncation matches what Rust can represent
			return code, payload, nil
		default:
			if uint32(d.Code) > 0xFFFFFF {
				return 0, nil, &Mp4ParseError{Message: "attempted to write a code that cannot fit in 24 bits"}
			}
			return uint32(d.Code), d.Data, nil
		}
	}

	for _, atom := range ilst.atoms {
		var a bytes.Buffer
		if atom.Ident.Freeform {
			writeSub := func(boxName string, payload []byte) error {
				size, err := ilstAtomSize(12, len(payload))
				if err != nil {
					return err
				}
				var s [4]byte
				binary.BigEndian.PutUint32(s[:], size)
				a.Write(s[:])
				a.WriteString(boxName)
				a.Write([]byte{0, 0, 0, 0})
				a.Write(payload)
				return nil
			}
			if err := writeSub("mean", []byte(atom.Ident.Mean)); err != nil {
				return nil, err
			}
			if err := writeSub("name", []byte(atom.Ident.Name)); err != nil {
				return nil, err
			}
		}
		for _, d := range atom.Data {
			code, payload, err := dataTypeFor(d)
			if err != nil {
				return nil, err
			}
			if err := writeDataAtom(&a, code, payload); err != nil {
				return nil, err
			}
		}
		atomSize, err := ilstAtomSize(8, a.Len())
		if err != nil {
			return nil, err
		}
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], atomSize)
		atoms.Write(sz[:])
		name := atom.Ident.Fourcc
		if atom.Ident.Freeform {
			name = [4]byte{'-', '-', '-', '-'}
		}
		atoms.Write(name[:])
		atoms.Write(a.Bytes())
	}
	if atoms.Len() == 0 {
		return nil, nil
	}
	ilstSize, err := ilstAtomSize(8, atoms.Len())
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], ilstSize)
	out.Write(sz[:])
	out.WriteString("ilst")
	out.Write(atoms.Bytes())
	return out.Bytes(), nil
}

// ilstAtomSize returns header+payload as a u32 atom size, or
// [meta.TooMuchDataError] if it doesn't fit.
func ilstAtomSize(header, payload int) (uint32, error) {
	total := uint64(header) + uint64(payload) // #nosec G115 -- header/payload are non-negative buffer lengths, and the sum is bounds-checked against math.MaxUint32 immediately below
	if total > math.MaxUint32 {
		return 0, &meta.TooMuchDataError{}
	}
	return uint32(total), nil
}

// intPayload packs a big-endian integer following Rust's writers: signed ints
// are always 4 bytes (`write_signed_int`), unsigned ints occupy the smallest
// of 1/2/4 bytes (`bytes_to_occupy_uint` — 3-byte values round up to 4).
func intPayload(v int64, signed bool) (uint32, []byte) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v)) // #nosec G115 -- bit-pattern-preserving cast for big-endian serialization of the int64 value
	if signed {
		// Rust stores i32; wider Go values take 8 bytes (unrepresentable in Rust)
		if v > math.MaxInt32 || v < math.MinInt32 {
			return 21, append([]byte(nil), b[:]...)
		}
		return 21, append([]byte(nil), b[4:]...)
	}
	return 22, uintPayloadBytes(uint32(v)) // #nosec G115 -- mirrors Rust's u32 unsigned-int writer (write_unsigned_int); values originate from u32 atom payloads
}

// uintPayload packs a u32 like Rust's `write_unsigned_int`.
func uintPayload(v uint32) (uint32, []byte) { return 22, uintPayloadBytes(v) }

func uintPayloadBytes(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	out := b[:]
	for len(out) > 1 && out[0] == 0 {
		out = out[1:]
	}
	if len(out) == 3 {
		out = b[:]
	}
	return append([]byte(nil), out...)
}

func u16FromStringOpt(s string) meta.Option[uint16] {
	if v, ok := util.U32FromString(s); ok && v <= 0xFFFF {
		return meta.Some(uint16(v))
	}
	return meta.None[uint16]()
}

func flagItemFromText(s string) (bool, bool) {
	switch s {
	case "0", "false":
		return false, true
	case "1", "true":
		return true, true
	}
	return false, false
}

func u8FromString(s string) (uint8, bool) {
	v, ok := util.U32FromString(s)
	if !ok || v > 0xFF {
		return 0, false
	}
	return uint8(v), true
}

// advisoryRatingFromU8 normalizes a raw rtng byte per Rust's
// `AdvisoryRating::try_from(u8)` + `as_u8` (0→0, 1|4→1, 2→2; invalid → !ok).
// AdvisoryRating is the normalized `rtng` value, mirroring Rust's
// `AdvisoryRating` (Explicit covers both the 1 and legacy 4 forms).
type AdvisoryRating uint8

const (
	AdvisoryRatingInoffensive AdvisoryRating = 0
	AdvisoryRatingExplicit    AdvisoryRating = 1
	AdvisoryRatingClean       AdvisoryRating = 2
)

// AsU8 returns the rating as it appears in the `rtng` atom.
func (r AdvisoryRating) AsU8() uint8 { return uint8(r) }

func advisoryRatingFromU8(v uint8) (AdvisoryRating, bool) {
	switch v {
	case 0:
		return AdvisoryRatingInoffensive, true
	case 1, 4:
		return AdvisoryRatingExplicit, true
	case 2:
		return AdvisoryRatingClean, true
	default:
		return 0, false
	}
}

func utf16BEEncode(s string) []byte {
	return util.Utf16Encode(s, util.BigEndian, false, false)
}

// ParseIlst parses the atoms inside an `ilst` atom body into an [Ilst].
func ParseIlst(reader io.ReadSeeker, end int64, parseOptions meta.ParseOptions) (*Ilst, error) {
	// Rust `parse_ilst` buffers the whole atom body up front
	// (try_vec + read_exact): a truncated body is a hard error in all
	// parsing modes, and every read below stays inside the declared bound.
	pos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	bodyLen := end - pos
	if bodyLen < 0 {
		return nil, &Mp4ParseError{Source: &meta.SizeMismatchError{}}
	}
	contents, err := util.TryByteSlice(int(bodyLen))
	if err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(reader, contents); err != nil {
		return nil, err
	}
	cursor := &offsetReadSeeker{r: bytes.NewReader(contents), base: pos}

	ilst := &Ilst{}
	var upgradedGnres []AtomData
	for {
		cpos, _ := cursor.Seek(0, io.SeekCurrent)
		if cpos >= end {
			break
		}
		// Rust's outer `while let Ok(Some(atom)) = ilst_reader.next()`
		// swallows every header-level error — a malformed atom (invalid
		// ident, oversize, bad freeform) just ends the scan while keeping
		// previously parsed atoms.
		atom, err := mp4ReadBoxBound(cursor, uint64(end-cpos), parseOptions.ParsingMode) // #nosec G115 -- loop above terminates when cpos >= end, so the difference is non-negative
		if err != nil || atom == nil {
			break
		}
		atomEnd := atom.pos + int64(atom.size) // #nosec G115 -- box payload size validated <= the ilst body bound by mp4ReadBoxBound, itself a buffered in-memory span
		// `free`/`skip` atoms inside ilst are ignored
		if atom.name == [4]byte{'f', 'r', 'e', 'e'} || atom.name == [4]byte{'s', 'k', 'i', 'p'} {
			if _, err := cursor.Seek(atomEnd, io.SeekStart); err != nil {
				break
			}
			continue
		}
		name := mp4AtomName(atom.name)
		if _, err := cursor.Seek(atom.pos, io.SeekStart); err != nil {
			break
		}
		var ident AtomIdent
		if name == "----" {
			// Rust `parse_freeform` runs inside `AtomInfo::read`, so its
			// errors are swallowed by `while let Ok` — the scan just ends.
			id, err := mp4ParseFreeform(cursor, atom, atomEnd, parseOptions.ParsingMode)
			if err != nil {
				break
			}
			ident = id
		} else {
			ident = AtomIdent{Fourcc: atom.name}
		}

		var datas []AtomData
		fourcc := atom.name
		isCovr := fourcc == [4]byte{'c', 'o', 'v', 'r'}
		if isCovr && !parseOptions.ReadCoverArt {
			if _, err := cursor.Seek(atomEnd, io.SeekStart); err != nil {
				break
			}
			continue
		}
		isBool := [4]byte{'c', 'p', 'i', 'l'} == fourcc || [4]byte{'h', 'd', 'v', 'd'} == fourcc ||
			[4]byte{'p', 'c', 's', 't'} == fourcc || [4]byte{'p', 'g', 'a', 'p'} == fourcc ||
			[4]byte{'s', 'h', 'w', 'm'} == fourcc

		for {
			cpos, _ := cursor.Seek(0, io.SeekCurrent)
			if cpos >= atomEnd {
				break
			}
			// Rust `parse_data_inner`: `reader.next()?` — errors here
			// propagate in all modes; `Ok(None)` ends the loop. The bound
			// is the whole remaining `ilst` body, matching AtomReader.
			ch, err := mp4ReadBoxBound(cursor, uint64(end-cpos), parseOptions.ParsingMode) // #nosec G115 -- loop above terminates when cpos >= end, so the difference is non-negative
			if err != nil {
				return nil, err
			}
			if ch == nil {
				break
			}
			childEnd := ch.pos + int64(ch.size) // #nosec G115 -- box payload size validated <= the ilst body bound by mp4ReadBoxBound, itself a buffered in-memory span
			// ch.size is the payload size; a data atom needs >= 8 (type+locale)
			if ch.size < 8 {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, util.NewNotEnoughDataError(16)
				}
				break
			}
			if string(ch.name[:]) != "data" {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &Mp4ParseError{Message: `expected atom "data" to follow name`}
				}
				if _, err := cursor.Seek(childEnd, io.SeekStart); err != nil {
					break
				}
				continue
			}
			var dh [8]byte
			if _, err := io.ReadFull(cursor, dh[:]); err != nil {
				return nil, err
			}
			// type indicator: [type_set, type ×3]; only the well-known set (0) is valid
			if dh[0] != 0 {
				if parseOptions.ParsingMode == meta.ParsingModeStrict {
					return nil, &Mp4ParseError{Message: "unknown type set in data atom"}
				}
				if _, err := cursor.Seek(childEnd, io.SeekStart); err != nil {
					break
				}
				continue
			}
			// Rust reads a u24 type code (DataType::from(read_u24))
			code := uint32(dh[1])<<16 | uint32(dh[2])<<8 | uint32(dh[3])
			contentLen := int(ch.size) - 8 // #nosec G115 -- guarded by the ch.size < 8 check above; size validated by mp4ReadBoxBound, and TryByteSlice enforces the allocation limit
			if contentLen <= 0 {
				if _, err := cursor.Seek(childEnd, io.SeekStart); err != nil {
					break
				}
				continue
			}
			payload, err := util.TryByteSlice(contentLen)
			if err != nil {
				return nil, err
			}
			if _, err := io.ReadFull(cursor, payload); err != nil {
				return nil, err
			}
			switch {
			case isCovr:
				var mt meta.MimeType
				mtSet := true
				switch code {
				case 0: // implicit type: no MIME (Rust DataType::Reserved)
					mtSet = false
				case 13:
					mt = meta.MimeTypeJpeg
				case 14:
					mt = meta.MimeTypePng
				case 12:
					mt = meta.MimeTypeGif
				case 27:
					mt = meta.MimeTypeBmp
				default:
					if parseOptions.ParsingMode == meta.ParsingModeStrict {
						return nil, &Mp4ParseError{Message: "unknown covr data type"}
					}
					// Rust discards the whole atom on an unknown covr type. It has
					// already consumed every `data` child, so skip the rest of the
					// atom before ending it.
					datas = nil
					_, _ = cursor.Seek(atomEnd, io.SeekStart)
					goto atomDone
				}
				b := meta.PictureUnchecked(payload).PicType(meta.PictureTypeOther)
				if mtSet {
					b = b.MimeType(mt)
				}
				datas = append(datas, AtomData{Kind: AtomDataPicture, Pic: b.Build()})
			case isBool:
				if len(datas) > 0 {
					// Rust keeps only the first `data` for bool atoms
					break
				}
				isTrue := false
				for _, b := range payload {
					if b != 0 {
						isTrue = true
						break
					}
				}
				datas = append(datas, AtomData{Kind: AtomDataBool, Bool: isTrue})
			default:
				d, derr := ilstInterpretAtomContent(code, payload, fourcc, parseOptions)
				if derr != nil {
					if parseOptions.ParsingMode == meta.ParsingModeStrict {
						return nil, derr
					}
					// Drop the whole atom (Rust `handle_error`)
					datas = nil
					_, _ = cursor.Seek(atomEnd, io.SeekStart)
					goto atomDone
				}
				// Rust only keeps 8-byte `plID` payloads
				if fourcc != [4]byte{'p', 'l', 'I', 'D'} || len(payload) == 8 {
					datas = append(datas, d)
				}
			}
			if _, err := cursor.Seek(childEnd, io.SeekStart); err != nil {
				break
			}
		}
	atomDone:
		// Rust's `ilst_reader` never rewinds: `while let Ok(Some(atom))`
		// reads the next header at wherever the last child read ended.
		// A `data` child that overshoots its parent (e.g. size-0 extends
		// to the ilst end) thus permanently ends the outer scan — no
		// seek back to atomEnd here.
		if len(datas) > 0 {
			// Upgraded `gnre` values are collected per-`data` and pushed
			// as ©gen after the loop (discarded if a ©gen atom exists);
			// the `gnre` atom itself is never retained.
			if fourcc == [4]byte{'g', 'n', 'r', 'e'} && parseOptions.ImplicitConversions {
				for _, d := range datas {
					if d.Kind == AtomDataUTF8 {
						upgradedGnres = append(upgradedGnres, d)
					}
				}
				continue
			}
			ilst.atoms = append(ilst.atoms, Atom{Ident: ident, Data: datas})
		}
	}
	if parseOptions.ImplicitConversions && len(upgradedGnres) > 0 {
		if ilst.Get(AtomIdent{Fourcc: [4]byte{0xA9, 'g', 'e', 'n'}}) == nil {
			ilst.atoms = append(ilst.atoms, Atom{
				Ident: AtomIdent{Fourcc: [4]byte{0xA9, 'g', 'e', 'n'}},
				Data:  upgradedGnres,
			})
		}
	}
	return ilst, nil
}

// ilstInterpretAtomContent maps a `data` payload to an [AtomData] per Rust's
// `interpret_atom_content` + `gnre`/`plID` special cases.
func ilstInterpretAtomContent(code uint32, payload []byte, fourcc [4]byte, parseOptions meta.ParseOptions) (AtomData, error) {
	// gnre: upgrade to UTF8 ©gen when implicit conversions are on
	if fourcc == [4]byte{'g', 'n', 'r', 'e'} {
		if parseOptions.ImplicitConversions && len(payload) >= 2 {
			idx := int(payload[1])
			if idx > 0 && idx < len(mp4Genres) {
				// Marker: caller upgrades the ident to ©gen
				return AtomData{Kind: AtomDataUTF8, Text: mp4Genres[idx]}, nil
			}
		}
		return AtomData{Kind: AtomDataUnknown, Code: DataType(code), Data: payload}, nil
	}
	// plID: only 8-byte payloads are kept (64-bit), others are dropped
	if fourcc == [4]byte{'p', 'l', 'I', 'D'} {
		return AtomData{Kind: AtomDataUnknown, Code: DataType(code), Data: payload}, nil
	}
	switch code {
	case 1:
		text, err := util.Utf8Decode(payload)
		if err != nil {
			return AtomData{}, &Mp4ParseError{Message: "invalid UTF-8 in data atom"}
		}
		return AtomData{Kind: AtomDataUTF8, Text: text}, nil
	case 2:
		text, err := util.Utf16DecodeBytes(payload, util.BigEndian)
		if err != nil {
			return AtomData{}, &Mp4ParseError{Source: err}
		}
		return AtomData{Kind: AtomDataUTF16, Text: text}, nil
	case 21:
		v, err := parseBEInt(payload, true)
		if err != nil {
			return AtomData{}, err
		}
		return AtomData{Kind: AtomDataSignedInt, Int: int64(v)}, nil
	case 22:
		v, err := parseBEInt(payload, false)
		if err != nil {
			return AtomData{}, err
		}
		// Rust's `parse_uint` returns a u32: don't sign-extend through int32.
		return AtomData{Kind: AtomDataUnsignedInt, Uint: uint64(uint32(v))}, nil // #nosec G115 -- deliberate zero-extension through uint32; Rust's parse_uint yields u32 so the value must not sign-extend (see comment)
	default:
		return AtomData{Kind: AtomDataUnknown, Code: DataType(code), Data: payload}, nil
	}
}

// parseBEInt decodes a 1–4-byte big-endian integer like Rust's
// `parse_int`/`parse_uint` (larger payloads are an error). For `signed`, a
// 1-byte value is treated as unsigned (Rust `i32::from(u8)`).
func parseBEInt(b []byte, signed bool) (int32, error) {
	var v int32
	switch len(b) {
	case 1:
		v = int32(b[0])
	case 2:
		if signed {
			v = int32(int16(binary.BigEndian.Uint16(b))) // #nosec G115 -- deliberate sign extension of a 16-bit big-endian value, mirroring Rust's parse_int
		} else {
			v = int32(binary.BigEndian.Uint16(b))
		}
	case 3:
		var pad byte
		if signed && b[0]&0x80 != 0 {
			pad = 0xFF
		}
		v = int32(binary.BigEndian.Uint32([]byte{pad, b[0], b[1], b[2]})) // #nosec G115 -- intentional bit reinterpretation of the sign-padded 24-bit big-endian integer, mirroring Rust's parse_int
	case 4:
		v = int32(binary.BigEndian.Uint32(b)) // #nosec G115 -- intentional bit reinterpretation of a fixed-width big-endian integer, mirroring Rust's i32::from_be_bytes in parse_int
	default:
		return 0, &Mp4ParseError{Message: `unexpected atom size for type "BE integer"`}
	}
	return v, nil
}

// mp4ParseFreeform parses a `----` atom's ordered `mean`/`name` sub-boxes,
// mirroring Rust's `parse_freeform`/`freeform_chunk` (errors propagate).
func mp4ParseFreeform(reader io.ReadSeeker, atom *mp4BoxHeader, atomEnd int64, mode meta.ParsingMode) (AtomIdent, error) {
	if atom.size < 24 {
		return AtomIdent{}, &Mp4ParseError{Source: util.NewNotEnoughDataError(24)}
	}
	if _, err := reader.Seek(atom.pos, io.SeekStart); err != nil {
		return AtomIdent{}, &Mp4ParseError{Source: err}
	}
	var values [2]string
	for i, want := range [2]string{"mean", "name"} {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		ch, err := mp4ReadBoxBound(reader, uint64(atomEnd-pos), mode) // #nosec G115 -- reader positions advance monotonically through validated child boxes bounded against atomEnd, so the difference is non-negative
		if err != nil {
			return AtomIdent{}, err
		}
		if ch == nil || string(ch.name[:]) != want {
			return AtomIdent{}, &Mp4ParseError{Message: `expected "` + want + `" chunk in freeform atom`}
		}
		if ch.size < 4 {
			return AtomIdent{}, &Mp4ParseError{Source: util.NewNotEnoughDataError(12)}
		}
		if ch.pos+int64(ch.size) > atomEnd { // #nosec G115 -- box payload size validated <= the enclosing bound by mp4ReadBoxBound, whose avail derives from int64 positions
			return AtomIdent{}, &meta.SizeMismatchError{}
		}
		if _, err := reader.Seek(4, io.SeekCurrent); err != nil {
			return AtomIdent{}, &Mp4ParseError{Source: err}
		}
		content, err := util.TryByteSlice(int(ch.size) - 4) // #nosec G115 -- guarded by the ch.size < 4 error above; size validated by mp4ReadBoxBound, and TryByteSlice enforces the allocation limit
		if err != nil {
			return AtomIdent{}, &Mp4ParseError{Source: err}
		}
		if _, err := io.ReadFull(reader, content); err != nil {
			return AtomIdent{}, &Mp4ParseError{Source: err}
		}
		v, err := util.Utf8Decode(content)
		if err != nil {
			return AtomIdent{}, &Mp4ParseError{Source: err}
		}
		values[i] = v
	}
	return FreeformIdent(values[0], values[1]), nil
}

// firstText returns the text of the atom's first UTF8/UTF16 data value.
func (l *Ilst) firstText(ident AtomIdent) (string, bool) {
	if a := l.Get(ident); a != nil {
		for _, d := range a.Data {
			if d.Kind == AtomDataUTF8 || d.Kind == AtomDataUTF16 {
				return d.Text, true
			}
		}
	}
	return "", false
}

func (l *Ilst) setText(fourcc string, value string) {
	l.ReplaceAtom(Atom{FourccIdent(fourcc), []AtomData{{Kind: AtomDataUTF8, Text: value}}})
}

// extractNumber reads a u16 from a trkn/disk `Unknown{Reserved}` pair field
// (`offset` 4 = number, 6 = total).
func (l *Ilst) extractNumber(fourcc [4]byte, offset int) (uint16, bool) {
	if a := l.Get(AtomIdent{Fourcc: fourcc}); a != nil && len(a.Data) > 0 {
		d := a.Data[0]
		if d.Kind == AtomDataUnknown && d.Code == 0 && len(d.Data) >= offset {
			return binary.BigEndian.Uint16(d.Data[offset-2:]), true
		}
	}
	return 0, false
}

func (l *Ilst) setNumber(fourcc [4]byte, offset int, value uint32) {
	var num, tot uint16
	if offset == 4 {
		num = uint16(value) // #nosec G115 -- mirrors Rust `(value as u16)` cast in set_track/set_disk; truncation is intentional parity
		if t, ok := l.extractNumber(fourcc, 6); ok {
			tot = t
		}
	} else {
		if n, ok := l.extractNumber(fourcc, 4); ok {
			num = n
		}
		tot = uint16(value) // #nosec G115 -- mirrors Rust `(value as u16)` cast in set_track_total/set_disk_total; truncation is intentional parity
	}
	data := []byte{0, 0, byte(num >> 8), byte(num), byte(tot >> 8), byte(tot)}
	l.ReplaceAtom(Atom{
		AtomIdent{Fourcc: fourcc},
		[]AtomData{{Kind: AtomDataUnknown, Code: 0, Data: data}},
	})
}

// Artist returns the "©ART" atom's text.
func (l *Ilst) Artist() (string, bool) { return l.firstText(FourccIdent("©ART")) }

// SetArtist sets the "©ART" atom.
func (l *Ilst) SetArtist(v string) { l.setText("©ART", v) }

// RemoveArtist removes the "©ART" atom.
func (l *Ilst) RemoveArtist() { l.Remove(FourccIdent("©ART")) }

// Title returns the "©nam" atom's text.
func (l *Ilst) Title() (string, bool) { return l.firstText(FourccIdent("©nam")) }

// SetTitle sets the "©nam" atom.
func (l *Ilst) SetTitle(v string) { l.setText("©nam", v) }

// RemoveTitle removes the "©nam" atom.
func (l *Ilst) RemoveTitle() { l.Remove(FourccIdent("©nam")) }

// Album returns the "©alb" atom's text.
func (l *Ilst) Album() (string, bool) { return l.firstText(FourccIdent("©alb")) }

// SetAlbum sets the "©alb" atom.
func (l *Ilst) SetAlbum(v string) { l.setText("©alb", v) }

// RemoveAlbum removes the "©alb" atom.
func (l *Ilst) RemoveAlbum() { l.Remove(FourccIdent("©alb")) }

// Genre returns the "©gen" atom's text.
func (l *Ilst) Genre() (string, bool) { return l.firstText(FourccIdent("©gen")) }

// SetGenre sets the "©gen" atom.
func (l *Ilst) SetGenre(v string) { l.setText("©gen", v) }

// RemoveGenre removes the "©gen" atom.
func (l *Ilst) RemoveGenre() { l.Remove(FourccIdent("©gen")) }

// Comment returns the "©cmt" atom's text.
func (l *Ilst) Comment() (string, bool) { return l.firstText(FourccIdent("©cmt")) }

// SetComment sets the "©cmt" atom.
func (l *Ilst) SetComment(v string) { l.setText("©cmt", v) }

// RemoveComment removes the "©cmt" atom.
func (l *Ilst) RemoveComment() { l.Remove(FourccIdent("©cmt")) }

var (
	trknFourcc = [4]byte{'t', 'r', 'k', 'n'}
	diskFourcc = [4]byte{'d', 'i', 's', 'k'}
)

// Track returns the track number stored in `trkn`.
func (l *Ilst) Track() (uint32, bool) {
	v, ok := l.extractNumber(trknFourcc, 4)
	return uint32(v), ok
}

// SetTrack writes the track number into `trkn`, preserving the total.
func (l *Ilst) SetTrack(v uint32) { l.setNumber(trknFourcc, 4, v) }

// RemoveTrack removes the `trkn` atom.
func (l *Ilst) RemoveTrack() { l.Remove(AtomIdent{Fourcc: trknFourcc}) }

// TrackTotal returns the track total stored in `trkn`.
func (l *Ilst) TrackTotal() (uint32, bool) {
	v, ok := l.extractNumber(trknFourcc, 6)
	return uint32(v), ok
}

// SetTrackTotal writes the track total into `trkn`, preserving the number.
func (l *Ilst) SetTrackTotal(v uint32) { l.setNumber(trknFourcc, 6, v) }

// RemoveTrackTotal removes the total, keeping the track number (if any).
func (l *Ilst) RemoveTrackTotal() { l.removeTotal(trknFourcc) }

// removeTotal mirrors Rust `remove_track_total`/`remove_disk_total`: read the
// number, remove the atom, and re-add it only if a number existed.
func (l *Ilst) removeTotal(fourcc [4]byte) {
	num, ok := l.extractNumber(fourcc, 4)
	l.Remove(AtomIdent{Fourcc: fourcc})
	if ok {
		l.ReplaceAtom(Atom{
			AtomIdent{Fourcc: fourcc},
			[]AtomData{{Kind: AtomDataUnknown, Code: 0, Data: []byte{0, 0, byte(num >> 8), byte(num), 0, 0}}}, // #nosec G115 -- num is a uint16 being serialized big-endian; the byte conversions are exact byte extraction
		})
	}
}

// Disk returns the disc number stored in `disk`.
func (l *Ilst) Disk() (uint32, bool) {
	v, ok := l.extractNumber(diskFourcc, 4)
	return uint32(v), ok
}

// SetDisk writes the disc number into `disk`, preserving the total.
func (l *Ilst) SetDisk(v uint32) { l.setNumber(diskFourcc, 4, v) }

// RemoveDisk removes the `disk` atom.
func (l *Ilst) RemoveDisk() { l.Remove(AtomIdent{Fourcc: diskFourcc}) }

// DiskTotal returns the disc total stored in `disk`.
func (l *Ilst) DiskTotal() (uint32, bool) {
	v, ok := l.extractNumber(diskFourcc, 6)
	return uint32(v), ok
}

// SetDiskTotal writes the disc total into `disk`, preserving the number.
func (l *Ilst) SetDiskTotal(v uint32) { l.setNumber(diskFourcc, 6, v) }

// RemoveDiskTotal removes the total, keeping the disc number (if any).
func (l *Ilst) RemoveDiskTotal() { l.removeTotal(diskFourcc) }

// Date returns the "©day" atom parsed as a [meta.Timestamp].
func (l *Ilst) Date() (meta.Timestamp, bool) {
	// Rust only accepts a UTF-8 first value (`AtomData::UTF8`).
	if a := l.Get(FourccIdent("©day")); a != nil && len(a.Data) > 0 && a.Data[0].Kind == AtomDataUTF8 {
		if ts, ok := util.TryParseTimestamp(a.Data[0].Text).Get(); ok {
			return ts, true
		}
	}
	return meta.Timestamp{}, false
}

// SetDate sets the "©day" atom.
func (l *Ilst) SetDate(v meta.Timestamp) { l.setText("©day", v.String()) }

// RemoveDate removes the "©day" atom.
func (l *Ilst) RemoveDate() { l.Remove(FourccIdent("©day")) }

// GetFlag returns a bool atom's value.
func (l *Ilst) GetFlag(ident AtomIdent) (bool, bool) {
	if a := l.Get(ident); a != nil {
		for _, d := range a.Data {
			if d.Kind == AtomDataBool {
				return d.Bool, true
			}
		}
	}
	return false, false
}

// SetFlag writes a bool atom.
func (l *Ilst) SetFlag(ident AtomIdent, value bool) {
	if !value {
		// A flag with a value of `false` is equivalent to removing it.
		l.Remove(ident)
		return
	}
	l.ReplaceAtom(Atom{ident, []AtomData{{Kind: AtomDataBool, Bool: value}}})
}

// AdvisoryRating returns the normalized parental advisory rating.
func (l *Ilst) AdvisoryRating() (AdvisoryRating, bool) {
	if a := l.Get(FourccIdent("rtng")); a != nil {
		// Rust: filter_map(u8::try_from / first byte).find_map(AdvisoryRating::try_from)
		// — `AtomData::UnsignedInteger` is ignored.
		for _, d := range a.Data {
			var raw byte
			switch d.Kind {
			case AtomDataSignedInt:
				if d.Int < 0 || d.Int > math.MaxUint8 {
					continue
				}
				raw = byte(d.Int)
			case AtomDataUnknown:
				if len(d.Data) == 0 {
					continue
				}
				raw = d.Data[0]
			default:
				continue
			}
			if rating, ok := advisoryRatingFromU8(raw); ok {
				return rating, true
			}
		}
	}
	return 0, false
}

// SetAdvisoryRating writes the `rtng` atom.
func (l *Ilst) SetAdvisoryRating(rating AdvisoryRating) {
	l.ReplaceAtom(Atom{
		FourccIdent("rtng"),
		[]AtomData{{Kind: AtomDataSignedInt, Int: int64(rating)}},
	})
}

// Retain keeps only atoms for which `keep` returns true, preserving order
// (Rust `Ilst::retain`).
func (l *Ilst) Retain(keep func(*Atom) bool) {
	l.removeWhere(func(a *Atom) bool { return !keep(a) })
}

// IsPodcast reports whether the podcast flag (`pcst`) is set.
func (l *Ilst) IsPodcast() bool { v, _ := l.GetFlag(Mp4FlagPodcast); return v }

// IsGapless reports whether the gapless playback flag (`pgap`) is set.
func (l *Ilst) IsGapless() bool { v, _ := l.GetFlag(Mp4FlagGapless); return v }

// IsShowWork reports whether the show work and movement flag (`shwm`) is set.
func (l *Ilst) IsShowWork() bool { v, _ := l.GetFlag(Mp4FlagShowWork); return v }

// IsHdVideo reports whether the HD video flag (`hdvd`) is set.
func (l *Ilst) IsHdVideo() bool { v, _ := l.GetFlag(Mp4FlagHdVideo); return v }

// IsCompilation reports whether the compilation flag (`cpil`) is set.
func (l *Ilst) IsCompilation() bool { v, _ := l.GetFlag(Mp4FlagCompilation); return v }

// Pictures returns all pictures stored in the `covr` atom, mirroring Rust's
// `Ilst::pictures`.
func (l *Ilst) Pictures() []meta.Picture {
	a := l.Get(FourccIdent("covr"))
	if a == nil {
		return nil
	}
	var pics []meta.Picture
	for _, d := range a.Data {
		if d.Kind == AtomDataPicture {
			pics = append(pics, d.Pic)
		}
	}
	return pics
}

// RemovePictures removes the `covr` atom.
func (l *Ilst) RemovePictures() {
	l.removeWhere(func(a *Atom) bool {
		return len(a.Data) > 0 && a.Data[0].Kind == AtomDataPicture
	})
}

// SaveTo writes the ilst atom into an MP4 file, mirroring Rust
// `TagExt::save_to`.
func (l *Ilst) SaveTo(file meta.FileLike, writeOptions meta.WriteOptions) error {
	vf, err := meta.NewVerifiedFile(file, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeMp4Ilst)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !l.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return mp4WriteIlstData(vf.File(), l, writeOptions)
}

// SaveToPath writes the ilst atom to a path.
func (l *Ilst) SaveToPath(path string, writeOptions meta.WriteOptions) error {
	vf, f, err := meta.VerifiedFileFromPath(path, writeOptions.ParseOptions)
	if err != nil {
		return &meta.FileEncodingError{Source: err}
	}
	defer f.Close()
	format := vf.Format()
	support := format.TagSupport(meta.TagTypeMp4Ilst)
	if support == meta.TagSupportUnsupported || (!support.IsWritable() && !l.IsEmpty()) {
		return &meta.FileEncodingError{FileType: format, Source: &meta.UnsupportedTagError{}}
	}
	return mp4WriteIlstData(vf.File(), l, writeOptions)
}

// RemoveFrom removes the ilst atom from a [meta.FileLike].
func (l *Ilst) RemoveFrom(file meta.FileLike, writeOptions meta.WriteOptions) error {
	return meta.TagTypeMp4Ilst.RemoveFrom(file, writeOptions)
}

// RemoveFromPath removes the ilst atom from a path.
func (l *Ilst) RemoveFromPath(path string, writeOptions meta.WriteOptions) error {
	return meta.TagTypeMp4Ilst.RemoveFromPath(path, writeOptions)
}
