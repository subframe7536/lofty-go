package lofty

// Port of lofty-rs src/mp4/ilst — the `Ilst`/`Atom`/`AtomData` model.

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"
	"io"
	"strings"
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
	Text string   // UTF8/UTF16
	Bool bool     // Bool
	Pic  Picture  // Picture
	Int  int64    // SignedInt
	Uint uint64   // UnsignedInt
	Code uint8    // Unknown — the original data-type code
	Data []byte   // Unknown — raw payload
}

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

// key returns the string form used for ItemKey lookups.
func (i AtomIdent) key() string {
	if i.Freeform {
		return "----:" + i.Mean + ":" + i.Name
	}
	return mp4AtomName(i.Fourcc)
}

// Atom is a single ilst metadata atom (one or more `data` payloads).
type Atom struct {
	Ident AtomIdent
	Data  []AtomData
}

// Ilst is the MP4 `ilst` metadata tag.
type Ilst struct {
	atoms []Atom
}

// NewIlst returns an empty `Ilst`.
func NewIlst() *Ilst { return &Ilst{} }

// Len returns the number of atoms.
func (l *Ilst) Len() int { return len(l.atoms) }

// IsEmpty reports whether the tag has no atoms.
func (l *Ilst) IsEmpty() bool { return len(l.atoms) == 0 }

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
func (l *Ilst) InsertPicture(p Picture) {
	p.picType = PictureTypeOther
	d := AtomData{Kind: AtomDataPicture, Pic: p}
	if a := l.Get(FourccIdent("covr")); a != nil {
		a.Data = append(a.Data, d)
		return
	}
	l.atoms = append(l.atoms, Atom{FourccIdent("covr"), []AtomData{d}})
}

// Remove deletes the atom with the identifier.
func (l *Ilst) Remove(ident AtomIdent) {
	for i := range l.atoms {
		if l.atoms[i].Ident == ident {
			l.atoms = append(l.atoms[:i], l.atoms[i+1:]...)
			return
		}
	}
}

// Tag converts the `Ilst` to a generic [Tag] (Rust `SplitTag`/`From<Ilst> for Tag`).
func (l *Ilst) Tag() *Tag {
	tag := NewTag(TagTypeMp4Ilst)
	var remainder []Atom

	for _, atom := range l.atoms {
		keyStr := atom.Ident.key()
		ik, mapped := ItemKeyFromKey(TagTypeMp4Ilst, keyStr)
		isCovr := !atom.Ident.Freeform && atom.Ident.Fourcc == [4]byte{'c', 'o', 'v', 'r'}
		if !mapped && !isCovr {
			remainder = append(remainder, atom)
			continue
		}

		var remaining []AtomData
		for _, d := range atom.Data {
			switch d.Kind {
			case AtomDataUTF8, AtomDataUTF16:
				if mapped {
					if ik == ItemKeyPopularimeter {
						if rate, ok := u8FromString(d.Text); ok {
							if popm, ok := popularimeterMapped("", TagTypeMp4Ilst, rate, 0); ok {
								tag.PushUnchecked(NewTagItem(ik, ItemValueText(popm.String())))
								continue
							}
						}
						remaining = append(remaining, d)
						continue
					}
					tag.PushUnchecked(NewTagItem(ik, ItemValueText(d.Text)))
				}
			case AtomDataBool:
				if mapped {
					text := "0"
					if d.Bool {
						text = "1"
					}
					tag.PushUnchecked(NewTagItem(ik, ItemValueText(text)))
				}
			case AtomDataPicture:
				if isCovr {
					tag.PushPicture(d.Pic)
				}
			case AtomDataUnknown:
				if d.Code == 0 && len(d.Data) >= 6 && !atom.Ident.Freeform {
					numKey, totKey := ItemKeyTrackNumber, ItemKeyTrackTotal
					fourcc := mp4AtomName(atom.Ident.Fourcc)
					if fourcc == "disk" {
						numKey, totKey = ItemKeyDiscNumber, ItemKeyDiscTotal
					} else if fourcc != "trkn" {
						remaining = append(remaining, d)
						continue
					}
					cur := binary.BigEndian.Uint16(d.Data[2:])
					tot := binary.BigEndian.Uint16(d.Data[4:])
					if cur > 0 {
						tag.InsertText(numKey, itoa(int(cur)))
					}
					if tot > 0 {
						tag.InsertText(totKey, itoa(int(tot)))
					}
					continue
				}
				remaining = append(remaining, d)
			default:
				remaining = append(remaining, d)
			}
		}
		if len(remaining) > 0 {
			remainder = append(remainder, Atom{Ident: atom.Ident, Data: remaining})
		}
	}

	// Advisory rating (rtng) — signed int or unknown bytes → ParentalAdvisory.
	// The atom sits in `remainder` at this point (SignedInt/Unknown aren't
	// consumed by the retain loop).
	rtngIdent := FourccIdent("rtng")
	for i := range remainder {
		if remainder[i].Ident != rtngIdent {
			continue
		}
		var raw byte
		hasRaw := false
		for _, d := range remainder[i].Data {
			switch d.Kind {
			case AtomDataSignedInt:
				raw, hasRaw = byte(d.Int), true
			case AtomDataUnsignedInt:
				raw, hasRaw = byte(d.Uint), true
			case AtomDataUnknown:
				if len(d.Data) > 0 {
					raw, hasRaw = d.Data[0], true
				}
			}
			if hasRaw {
				break
			}
		}
		if hasRaw {
			if rating, ok := advisoryRatingFromU8(raw); ok {
				tag.InsertText(ItemKeyParentalAdvisory, itoa(int(rating)))
				remainder = append(remainder[:i], remainder[i+1:]...)
			}
		}
		break
	}

	if getGlobalOptions().PreserveFormatSpecificItems && len(remainder) > 0 {
		tag.companionTag = &Ilst{atoms: remainder}
	}
	return tag
}

// IlstFromTag converts a generic [Tag] to an `Ilst` (Rust `MergeTag`/`From<Tag> for Ilst`).
func IlstFromTag(tag *Tag) *Ilst {
	merged := &Ilst{}
	if getGlobalOptions().PreserveFormatSpecificItems {
		if comp, ok := tag.companionTag.(*Ilst); ok && comp != nil {
			merged = comp
		}
	}

	var tracks, discs [2]Option[uint16]

	for _, item := range tag.Items() {
		key, ok := item.key.MapKey(TagTypeMp4Ilst)
		if !ok {
			if item.key == ItemKeyUnknown &&
				(len(item.Description) == 4 || strings.HasPrefix(item.Description, "----:")) {
				key = item.Description
			} else {
				continue
			}
		}
		text, isText := item.value.IntoString()
		if !isText {
			continue
		}
		ident := ilstIdentFromString(key)
		switch item.key {
		case ItemKeyTrackNumber:
			tracks[0] = u16FromStringOpt(text)
			continue
		case ItemKeyTrackTotal:
			tracks[1] = u16FromStringOpt(text)
			continue
		case ItemKeyDiscNumber:
			discs[0] = u16FromStringOpt(text)
			continue
		case ItemKeyDiscTotal:
			discs[1] = u16FromStringOpt(text)
			continue
		case ItemKeyFlagCompilation, ItemKeyFlagPodcast:
			if b, ok := flagItemFromText(text); ok {
				merged.atoms = append(merged.atoms, Atom{ident, []AtomData{{Kind: AtomDataBool, Bool: b}}})
			}
			continue
		case ItemKeyParentalAdvisory:
			if v, ok := u8FromString(text); ok {
				if rating, ok := advisoryRatingFromU8(v); ok {
					merged.atoms = append(merged.atoms, Atom{ident, []AtomData{{Kind: AtomDataSignedInt, Int: int64(rating)}}})
				}
			}
			continue
		case ItemKeyPopularimeter:
			if popm, err := PopularimeterFromStr(text); err == nil {
				merged.atoms = append(merged.atoms, Atom{ident, []AtomData{{Kind: AtomDataUTF8, Text: itoa(int(popm.MappedValue(TagTypeMp4Ilst)))}}})
			}
			continue
		default:
			if item.key == ItemKeyUnknown {
				if _, ok := item.value.IntoBinary(); ok {
					data, _ := item.value.IntoBinary()
					merged.atoms = append(merged.atoms, Atom{ident, []AtomData{{Kind: AtomDataUnknown, Code: 0, Data: data}}})
					continue
				}
			}
			merged.atoms = append(merged.atoms, Atom{ident, []AtomData{{Kind: AtomDataUTF8, Text: text}}})
		}
	}

	// Pictures → single covr atom with multiple data
	for _, pic := range tag.Pictures() {
		merged.InsertPicture(pic)
	}

	createIntPair := func(fourcc string, pair [2]Option[uint16]) {
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
		merged.atoms = append(merged.atoms, Atom{
			FourccIdent(fourcc),
			[]AtomData{{Kind: AtomDataUnknown, Code: 0, Data: data}},
		})
	}
	createIntPair("trkn", tracks)
	createIntPair("disk", discs)

	return merged
}

// DumpTo writes the encoded `ilst` atom (with header) to a writer.
func (l *Ilst) DumpTo(writer io.Writer, writeOptions WriteOptions) error {
	data, err := encodeIlst(l)
	if err != nil {
		return &FileEncodingError{Source: err}
	}
	_, err = writer.Write(data)
	return err
}

// encodeIlst encodes the `ilst` atom (with header), returning nil for empty tags.
func encodeIlst(ilst *Ilst) ([]byte, error) {
	var atoms bytes.Buffer

	writeDataAtom := func(buf *bytes.Buffer, dataType uint8, payload []byte) {
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(16+len(payload)))
		buf.Write(sz[:])
		buf.WriteString("data")
		buf.Write([]byte{0, 0, 0, dataType})
		buf.Write([]byte{0, 0, 0, 0})
		buf.Write(payload)
	}

	dataTypeFor := func(d AtomData) (uint8, []byte) {
		switch d.Kind {
		case AtomDataUTF8:
			return 1, []byte(d.Text)
		case AtomDataUTF16:
			return 2, utf16BEEncode(d.Text)
		case AtomDataBool:
			var b byte
			if d.Bool {
				b = 1
			}
			return 21, []byte{b}
		case AtomDataPicture:
			mt, _ := d.Pic.MimeType().Get()
			code := uint8(13)
			switch mt {
			case MimeTypePng:
				code = 14
			case MimeTypeBmp:
				code = 27
			case MimeTypeGif:
				code = 12
			}
			return code, d.Pic.Data()
		case AtomDataSignedInt:
			return intPayload(d.Int, true)
		case AtomDataUnsignedInt:
			return intPayload(int64(d.Uint), false)
		default:
			return d.Code, d.Data
		}
	}

	for _, atom := range ilst.atoms {
		var a bytes.Buffer
		if atom.Ident.Freeform {
			writeSub := func(boxName string, payload []byte) {
				var s [4]byte
				binary.BigEndian.PutUint32(s[:], uint32(12+len(payload)))
				a.Write(s[:])
				a.WriteString(boxName)
				a.Write([]byte{0, 0, 0, 0})
				a.Write(payload)
			}
			writeSub("mean", []byte(atom.Ident.Mean))
			writeSub("name", []byte(atom.Ident.Name))
		}
		for _, d := range atom.Data {
			code, payload := dataTypeFor(d)
			writeDataAtom(&a, code, payload)
		}
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(a.Len()+8))
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
	var out bytes.Buffer
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(atoms.Len()+8))
	out.Write(sz[:])
	out.WriteString("ilst")
	out.Write(atoms.Bytes())
	return out.Bytes(), nil
}

// intPayload packs a big-endian integer in the smallest of 1/2/4/8 bytes and
// returns the matching data-type code (21 signed / 22 unsigned BE).
func intPayload(v int64, signed bool) (uint8, []byte) {
	code := uint8(22)
	if signed {
		code = 21
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	out := b[:]
	for len(out) > 1 && out[0] == 0 && (!signed || out[1]&0x80 == 0) {
		out = out[1:]
	}
	return code, out
}

func u16FromStringOpt(s string) Option[uint16] {
	if v, ok := u32FromString(s); ok && v <= 0xFFFF {
		return Some(uint16(v))
	}
	return None[uint16]()
}

func flagItemFromText(s string) (bool, bool) {
	switch s {
	case "0":
		return false, true
	case "1":
		return true, true
	}
	return false, false
}

func u8FromString(s string) (uint8, bool) {
	v, ok := u32FromString(s)
	if !ok || v > 0xFF {
		return 0, false
	}
	return uint8(v), true
}

// advisoryRatingFromU8 normalizes a raw rtng byte per Rust's
// `AdvisoryRating::try_from(u8)` + `as_u8` (0→0, 1|4→1, 2→2; invalid → !ok).
func advisoryRatingFromU8(v uint8) (uint8, bool) {
	switch v {
	case 0:
		return 0, true
	case 1, 4:
		return 1, true
	case 2:
		return 2, true
	default:
		return 0, false
	}
}



func utf16BEEncode(s string) []byte {
	return utf16Encode(s, bigEndian, false, false)
}

func utf16BEDecode(b []byte) string {
	s, _ := utf16DecodeBytes(b, bigEndian)
	return s
}

// parseIlst parses the atoms inside an `ilst` atom body into an [Ilst].
func parseIlst(reader io.ReadSeeker, end int64, parseOptions ParseOptions) (*Ilst, error) {
	ilst := &Ilst{}
	var upgradedGnres []AtomData
	for {
		pos, _ := reader.Seek(0, io.SeekCurrent)
		if pos >= end {
			break
		}
		atom, err := mp4ReadBox(reader)
		if err != nil {
			break
		}
		// Rust's AtomInfo validates ident bytes (0x20–0x7E or 0xA9); an
		// invalid ident fails `next()`, which ends the ilst loop while
		// keeping previously parsed atoms.
		validIdent := true
		for _, b := range atom.name {
			if b != 0xA9 && (b < 0x20 || b > 0x7E) {
				validIdent = false
				break
			}
		}
		if !validIdent {
			break
		}
		atomEnd := atom.pos + int64(atom.size)
		// `free`/`skip` atoms inside ilst are ignored
		if atom.name == [4]byte{'f', 'r', 'e', 'e'} || atom.name == [4]byte{'s', 'k', 'i', 'p'} {
			reader.Seek(atomEnd, io.SeekStart)
			continue
		}
		name := mp4AtomName(atom.name)
		if _, err := reader.Seek(atom.pos, io.SeekStart); err != nil {
			break
		}
		var ident AtomIdent
		if name == "----" {
			mean := mp4BoxPayload(reader, atomEnd, "mean")
			nameB := mp4BoxPayload(reader, atomEnd, "name")
			if mean == nil || nameB == nil {
				reader.Seek(atomEnd, io.SeekStart)
				continue
			}
			ident = FreeformIdent(string(mean), string(nameB))
			// Rust's AtomReader consumes the mean/name sub-boxes when
			// resolving the freeform ident — skip them here so the data
			// loop only sees `data` children.
			if _, err := reader.Seek(atom.pos, io.SeekStart); err != nil {
				break
			}
			for i := 0; i < 2; i++ {
				ch, err := mp4ReadBox(reader)
				if err != nil {
					break
				}
				if _, err := reader.Seek(ch.pos+int64(ch.size), io.SeekStart); err != nil {
					break
				}
			}
		} else {
			ident = AtomIdent{Fourcc: atom.name}
		}

		var datas []AtomData
		fourcc := atom.name
		isCovr := fourcc == [4]byte{'c', 'o', 'v', 'r'}
		if isCovr && !parseOptions.ReadCoverArt {
			reader.Seek(atomEnd, io.SeekStart)
			continue
		}
		isBool := [4]byte{'c', 'p', 'i', 'l'} == fourcc || [4]byte{'h', 'd', 'v', 'd'} == fourcc ||
			[4]byte{'p', 'c', 's', 't'} == fourcc || [4]byte{'p', 'g', 'a', 'p'} == fourcc ||
			[4]byte{'s', 'h', 'w', 'm'} == fourcc

		for {
			cpos, _ := reader.Seek(0, io.SeekCurrent)
			if cpos >= atomEnd {
				break
			}
			ch, err := mp4ReadBox(reader)
			if err != nil {
				break
			}
			childEnd := ch.pos + int64(ch.size)
			// ch.size is the payload size; a data atom needs >= 8 (type+locale)
			if ch.size < 8 {
				if parseOptions.ParsingMode == ParsingModeStrict {
					return nil, newNotEnoughDataError(16)
				}
				break
			}
			if string(ch.name[:]) != "data" {
				if parseOptions.ParsingMode == ParsingModeStrict {
					return nil, &Mp4ParseError{Message: `expected atom "data" to follow name`}
				}
				reader.Seek(childEnd, io.SeekStart)
				continue
			}
			var dh [8]byte
			if _, err := io.ReadFull(reader, dh[:]); err != nil {
				break
			}
			// type indicator: [type_set, type ×3]; only the well-known set (0) is valid
			if dh[0] != 0 {
				if parseOptions.ParsingMode == ParsingModeStrict {
					return nil, &Mp4ParseError{Message: "unknown type set in data atom"}
				}
				reader.Seek(childEnd, io.SeekStart)
				continue
			}
			code := dh[3]
			contentLen := int(ch.size) - 8
			if contentLen <= 0 {
				reader.Seek(childEnd, io.SeekStart)
				continue
			}
			payload, err := tryByteSlice(contentLen)
			if err != nil {
				return nil, err
			}
			if _, err := io.ReadFull(reader, payload); err != nil {
				break
			}
			switch {
			case isCovr:
				var mt MimeType
				ok := true
				switch code {
				case 13:
					mt = MimeTypeJpeg
				case 14:
					mt = MimeTypePng
				case 12:
					mt = MimeTypeGif
				case 27:
					mt = MimeTypeBmp
				default:
					ok = false
				}
				if !ok {
					if parseOptions.ParsingMode == ParsingModeStrict {
						return nil, &Mp4ParseError{Message: "unknown covr data type"}
					}
					break
				}
				datas = append(datas, AtomData{
					Kind: AtomDataPicture,
					Pic:  Picture{picType: PictureTypeOther, mimeType: Some(mt), data: payload},
				})
			case isBool:
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
					if parseOptions.ParsingMode == ParsingModeStrict {
						return nil, derr
					}
					// Drop the whole atom (Rust `handle_error`)
					datas = nil
					reader.Seek(atomEnd, io.SeekStart)
					break
				}
				datas = append(datas, d)
			}
			if _, err := reader.Seek(childEnd, io.SeekStart); err != nil {
				break
			}
		}
		reader.Seek(atomEnd, io.SeekStart)
		if len(datas) > 0 {
			// Upgraded `gnre` values are collected and pushed as ©gen after
			// the loop (discarded if a ©gen atom already exists)
			if fourcc == [4]byte{'g', 'n', 'r', 'e'} && parseOptions.ImplicitConversions && datas[0].Kind == AtomDataUTF8 {
				upgradedGnres = append(upgradedGnres, datas...)
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
func ilstInterpretAtomContent(code uint8, payload []byte, fourcc [4]byte, parseOptions ParseOptions) (AtomData, error) {
	// gnre: upgrade to UTF8 ©gen when implicit conversions are on
	if fourcc == [4]byte{'g', 'n', 'r', 'e'} {
		if parseOptions.ImplicitConversions && len(payload) >= 2 {
			idx := int(payload[1])
			if idx > 0 && idx < len(mp4Genres) {
				// Marker: caller upgrades the ident to ©gen
				return AtomData{Kind: AtomDataUTF8, Text: mp4Genres[idx]}, nil
			}
		}
		return AtomData{Kind: AtomDataUnknown, Code: code, Data: payload}, nil
	}
	// plID: always Unknown (64-bit), preserve raw
	if fourcc == [4]byte{'p', 'l', 'I', 'D'} {
		return AtomData{Kind: AtomDataUnknown, Code: code, Data: payload}, nil
	}
	switch code {
	case 1:
		if !utf8.Valid(payload) {
			return AtomData{}, &Mp4ParseError{Message: "invalid UTF-8 in data atom"}
		}
		return AtomData{Kind: AtomDataUTF8, Text: string(payload)}, nil
	case 2:
		text, err := utf16DecodeBytes(payload, bigEndian)
		if err != nil {
			return AtomData{}, &Mp4ParseError{Source: err}
		}
		return AtomData{Kind: AtomDataUTF16, Text: text}, nil
	case 21:
		return AtomData{Kind: AtomDataSignedInt, Int: parseBEInt(payload, true)}, nil
	case 22:
		return AtomData{Kind: AtomDataUnsignedInt, Uint: uint64(parseBEInt(payload, false))}, nil
	default:
		return AtomData{Kind: AtomDataUnknown, Code: code, Data: payload}, nil
	}
}

func parseBEInt(b []byte, signed bool) int64 {
	var v uint64
	for _, c := range b {
		v = (v << 8) | uint64(c)
	}
	if signed && len(b) > 0 && len(b) < 8 && b[0]&0x80 != 0 {
		// sign-extend
		shift := uint(64 - len(b)*8)
		return int64(v<<shift) >> shift
	}
	return int64(v)
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
		num = uint16(value)
		if t, ok := l.extractNumber(fourcc, 6); ok {
			tot = t
		}
	} else {
		if n, ok := l.extractNumber(fourcc, 4); ok {
			num = n
		}
		tot = uint16(value)
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

var trknFourcc = [4]byte{'t', 'r', 'k', 'n'}
var diskFourcc = [4]byte{'d', 'i', 's', 'k'}

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

// RemoveTrackTotal zeroes the total field, removing `trkn` if the number is gone.
func (l *Ilst) RemoveTrackTotal() {
	l.setNumber(trknFourcc, 6, 0)
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

// RemoveDiskTotal zeroes the total field, removing `disk` if the number is gone.
func (l *Ilst) RemoveDiskTotal() {
	l.setNumber(diskFourcc, 6, 0)
}

// Date returns the "©day" atom parsed as a [Timestamp].
func (l *Ilst) Date() (Timestamp, bool) {
	if s, ok := l.firstText(FourccIdent("©day")); ok {
		if ts, ok := tryParseTimestamp(s).Get(); ok {
			return ts, true
		}
	}
	return Timestamp{}, false
}

// SetDate sets the "©day" atom.
func (l *Ilst) SetDate(v Timestamp) { l.setText("©day", v.String()) }

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
	l.ReplaceAtom(Atom{ident, []AtomData{{Kind: AtomDataBool, Bool: value}}})
}

// AdvisoryRating returns the normalized parental advisory rating (0/1/2).
func (l *Ilst) AdvisoryRating() (uint8, bool) {
	if a := l.Get(FourccIdent("rtng")); a != nil {
		for _, d := range a.Data {
			var raw byte
			hasRaw := false
			switch d.Kind {
			case AtomDataSignedInt:
				raw, hasRaw = byte(d.Int), true
			case AtomDataUnsignedInt:
				raw, hasRaw = byte(d.Uint), true
			case AtomDataUnknown:
				if len(d.Data) > 0 {
					raw, hasRaw = d.Data[0], true
				}
			}
			if hasRaw {
				return advisoryRatingFromU8(raw)
			}
		}
	}
	return 0, false
}

// SetAdvisoryRating writes the `rtng` atom (normalized 0/1/2).
func (l *Ilst) SetAdvisoryRating(rating uint8) {
	l.ReplaceAtom(Atom{
		FourccIdent("rtng"),
		[]AtomData{{Kind: AtomDataSignedInt, Int: int64(rating)}},
	})
}

// RemovePictures removes the `covr` atom.
func (l *Ilst) RemovePictures() { l.Remove(FourccIdent("covr")) }
