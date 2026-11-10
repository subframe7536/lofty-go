package id3v2

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
)

// crc32IEEE mirrors Rust's crc32 — same polynomial as hash/crc32 IEEE.
var crc32Table = crc32.MakeTable(crc32.IEEE)

func crc32Of(content []byte) uint32 { return crc32.Checksum(content, crc32Table) }

// createTag mirrors Rust `write::create_tag` (ID3v2.4).
func createTag(flags Id3v2TagFlags, frames []Frame, writeOptions WriteOptions) ([]byte, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	if writeOptions.UseID3v23 {
		return createTagV3(flags, frames, writeOptions)
	}

	var id3v2 bytes.Buffer
	id3v2.WriteString("ID3")
	id3v2.Write([]byte{4, 0})
	id3v2.WriteByte(flags.asId3v24Byte())
	id3v2.Write([]byte{0, 0, 0, 0}) // Tag size (filled in later)

	var extendedHeaderSize uint32
	if flags.Crc || flags.Restrictions != nil {
		// Structure of v4 extended header:
		// Size (4) | Number of flag bytes (1) | Flags (1) | extra data
		id3v2.Write([]byte{0, 0, 0, 0, 0, 0})
		extendedHeaderSize = 6
		var extFlags uint8

		if flags.Crc {
			extFlags |= 0x20
			extendedHeaderSize += 6
			id3v2.Write([]byte{0, 0, 0, 0, 0, 0})
		}
		if flags.Restrictions != nil {
			extFlags |= 0x10
			extendedHeaderSize += 2
			id3v2.WriteByte(1)
			id3v2.WriteByte(flags.Restrictions.asByte())
		}

		// Write the actual extended header values
		out := id3v2.Bytes()
		ehSizeSynch, err := synchsafe32Checked(extendedHeaderSize)
		if err != nil {
			return nil, &Id3v2EncodingError{Source: err}
		}
		binary.BigEndian.PutUint32(out[10:14], ehSizeSynch)
		out[14] = 1
		out[15] = extFlags
	}

	headerLen := id3v2.Len()

	ctx := &frameEncodingContext{version: id3v2Version4, writeOptions: writeOptions}
	if err := createItems(&id3v2, frames, ctx); err != nil {
		return nil, &Id3v2EncodingError{Source: err}
	}

	totalTagSize, paddingSize, err := verifyTagSize(flags, id3v2.Len(), headerLen, extendedHeaderSize, writeOptions.PreferredPadding)
	if err != nil {
		return nil, err
	}

	out := id3v2.Bytes()
	sizeSynch, err := synchsafe32Checked(totalTagSize)
	if err != nil {
		return nil, &Id3v2EncodingError{Source: err}
	}
	binary.BigEndian.PutUint32(out[6:10], sizeSynch)

	if flags.Crc {
		// Normal ID3v2 header (10) + extended header v4 (6)
		const crcPos = 16
		contentStart := crcPos + 6
		if flags.Restrictions != nil {
			contentStart += 3
		}
		crc := crc32Of(out[contentStart:])
		// The CRC-32 is stored as a 35 bit synchsafe integer
		var encodedCrc [5]byte
		for i := 0; i < 5; i++ {
			encodedCrc[i] = uint8(crc>>((4-i)*7)) & 0x7F
		}
		out[crcPos] = 5
		copy(out[crcPos+1:crcPos+6], encodedCrc[:])
	}

	if flags.Footer {
		writeTagFooter(&id3v2)
	} else {
		writeTagPadding(&id3v2, paddingSize)
	}

	return id3v2.Bytes(), nil
}

// createTagV3 mirrors Rust `write::create_tag_v3` (ID3v2.3).
func createTagV3(flags Id3v2TagFlags, frames []Frame, writeOptions WriteOptions) ([]byte, error) {
	var id3v2 bytes.Buffer
	id3v2.WriteString("ID3")
	id3v2.Write([]byte{3, 0})
	id3v2.WriteByte(flags.asId3v23Byte())
	id3v2.Write([]byte{0, 0, 0, 0})

	if flags.Crc {
		// Extended header size (4) | Flags (2) | Padding size (4) | CRC (4)
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], 10)
		id3v2.Write(tmp[:])
		binary.BigEndian.PutUint16(tmp[:2], 1<<15)
		id3v2.Write(tmp[:2])
		id3v2.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	}
	var extendedHeaderSize uint32
	if flags.Crc {
		extendedHeaderSize = 10
	}

	headerLen := id3v2.Len()

	ctx := &frameEncodingContext{version: id3v2Version3, writeOptions: writeOptions}
	if err := createItemsV3(&id3v2, frames, ctx); err != nil {
		return nil, &Id3v2EncodingError{Source: err}
	}

	totalTagSize, paddingSize, err := verifyTagSize(flags, id3v2.Len(), headerLen, extendedHeaderSize, writeOptions.PreferredPadding)
	if err != nil {
		return nil, err
	}

	out := id3v2.Bytes()
	sizeSynch, err := synchsafe32Checked(totalTagSize)
	if err != nil {
		return nil, &Id3v2EncodingError{Source: err}
	}
	binary.BigEndian.PutUint32(out[6:10], sizeSynch)

	if flags.Crc {
		binary.BigEndian.PutUint32(out[16:20], paddingSize)
		binary.BigEndian.PutUint32(out[20:24], crc32Of(out[24:]))
	}

	if flags.Footer {
		writeTagFooter(&id3v2)
	} else {
		writeTagPadding(&id3v2, paddingSize)
	}

	return id3v2.Bytes(), nil
}

// verifyTagSize mirrors Rust `verify_tag_size`.
func verifyTagSize(flags Id3v2TagFlags, tagLen, headerLen int, extendedHeaderLen uint32, preferredPadding uint32) (totalSize, paddingSize uint32, err error) {
	length := uint32(tagLen - (headerLen - int(extendedHeaderLen)))
	if length > synchsafeMax {
		return 0, 0, &Id3v2EncodingError{Source: &TooMuchDataError{}}
	}
	if flags.Footer {
		preferredPadding = 0
	}
	var padding uint32
	if preferredPadding > 0 {
		remainder := synchsafeMax - length
		padding = preferredPadding
		if padding > remainder {
			padding = remainder
		}
		length += padding
	}
	return length, padding, nil
}

func writeTagFooter(id3v2 *bytes.Buffer) {
	out := id3v2.Bytes()
	// The footer is the same as the header, but with the identifier reversed
	id3v2.WriteString("3DI")
	id3v2.Write(out[3:10])
}

func writeTagPadding(id3v2 *bytes.Buffer, paddingSize uint32) {
	if paddingSize > 0 {
		id3v2.Write(make([]byte, paddingSize))
	}
}

// --- frame writing ---

// createItems mirrors `create_items` — strips outdated frames, verifies, encodes.
func createItems(w io.Writer, frames []Frame, ctx *frameEncodingContext) error {
	for _, frame := range frames {
		if !frame.ID().IsValid() {
			continue
		}
		if err := verifyFrame(frame); err != nil {
			return err
		}
		value, err := frameAsBytes(frame, ctx)
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}
		if err := writeFrame(w, frame.ID().String(), frame.Flags(), value, ctx.writeOptions); err != nil {
			return err
		}
	}
	return nil
}

// createItemsV3 mirrors `create_items_v3` — discards v4-only frames and
// converts TDOR/TDRC/TCON/TIPL/TMCL for ID3v2.3.
func createItemsV3(w io.Writer, frames []Frame, ctx *frameEncodingContext) error {
	var ipls *TextInformationFrame

	for _, frame := range frames {
		if !frame.ID().IsValid() {
			continue
		}
		if id3v23DiscardFrames[frame.ID().String()] {
			continue
		}
		if err := verifyFrame(frame); err != nil {
			return err
		}

		switch frame.ID().String() {
		case "TDOR", "TDRC":
			isTdor := frame.ID().String() == "TDOR"
			tsf, ok := frame.(*TimestampFrame)
			if !ok || tsf.Timestamp.Verify() != nil {
				continue
			}
			if isTdor {
				year := tsf.Timestamp.Year
				newFrame := NewTextInformationFrame(FrameIdValid("TORY"), tsf.Encoding.ToID3v23(), itoa(int(year)))
				newFrame.SetFlags(tsf.Flags())
				value, err := frameAsBytes(newFrame, ctx)
				if err != nil {
					return err
				}
				if value == nil {
					continue
				}
				if err := writeFrame(w, "TORY", newFrame.Flags(), value, ctx.writeOptions); err != nil {
					return err
				}
			} else {
				ts := tsf.Timestamp
				newFrames := []Frame{
					NewTextInformationFrame(FrameIdValid("TYER"), tsf.Encoding.ToID3v23(), itoa(int(ts.Year))),
				}
				if m, ok := ts.Month.Get(); ok {
					if d, ok := ts.Day.Get(); ok {
						newFrames = append(newFrames, NewTextInformationFrame(FrameIdValid("TDAT"), tsf.Encoding.ToID3v23(),
							formatTwoDigits(d)+formatTwoDigits(m)))
					}
				}
				if h, ok := ts.Hour.Get(); ok {
					if mi, ok := ts.Minute.Get(); ok {
						newFrames = append(newFrames, NewTextInformationFrame(FrameIdValid("TIME"), tsf.Encoding.ToID3v23(),
							formatTwoDigits(h)+formatTwoDigits(mi)))
					}
				}
				for _, nf := range newFrames {
					nf.SetFlags(tsf.Flags())
					value, err := frameAsBytes(nf, ctx)
					if err != nil {
						return err
					}
					if value == nil {
						continue
					}
					if err := writeFrame(w, nf.ID().String(), nf.Flags(), value, ctx.writeOptions); err != nil {
						return err
					}
				}
			}
			continue
		case "TCON":
			tf, ok := frame.(*TextInformationFrame)
			if !ok {
				continue
			}
			// ID3v2.3 cannot use null separators — wrap refs in parens
			tf.Value = id3v23GenreValue(tf.Value)
		case "TIPL", "TMCL":
			kvf, ok := frame.(*KeyValueFrame)
			if !ok {
				continue
			}
			if ipls == nil {
				ipls = NewTextInformationFrame(FrameIdValid("IPLS"), kvf.Encoding.ToID3v23(), "")
			}
			for _, kv := range kvf.KeyValuePairs {
				if ipls.Value != "" {
					ipls.Value += "\x00"
				}
				ipls.Value += kv[0] + "\x00" + kv[1]
			}
			continue
		}

		value, err := frameAsBytes(frame, ctx)
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}
		if err := writeFrame(w, frame.ID().String(), frame.Flags(), value, ctx.writeOptions); err != nil {
			return err
		}
	}

	if ipls != nil {
		value, err := frameAsBytes(ipls, ctx)
		if err != nil {
			return err
		}
		if value != nil {
			if err := writeFrame(w, "IPLS", ipls.Flags(), value, ctx.writeOptions); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatTwoDigits(v uint8) string {
	s := itoa(int(v))
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

// writeFrame mirrors `write::frame::write_frame`.
func writeFrame(w io.Writer, name string, flags FrameFlags, value []byte, writeOptions WriteOptions) error {
	if flags.Encryption.IsSome() {
		return writeEncrypted(w, name, value, flags, writeOptions)
	}
	length := uint32(len(value))
	isGrouping := flags.GroupingIdentity.IsSome()
	if isGrouping {
		length++
	}
	if err := writeFrameHeader(w, name, length, flags, writeOptions); err != nil {
		return err
	}
	if isGrouping {
		g, _ := flags.GroupingIdentity.Get()
		w.Write([]byte{g})
	}
	_, err := w.Write(value)
	return err
}

func writeEncrypted(w io.Writer, name string, value []byte, flags FrameFlags, writeOptions WriteOptions) error {
	methodSymbol, _ := flags.Encryption.Get()
	if methodSymbol <= 0x80 {
		return frameEncodingErrorMsg(Some(FrameIdValid(name)),
			"attempted to write an encrypted frame with an invalid method symbol")
	}
	if dli, ok := flags.DataLengthIndicator.Get(); ok && dli > 0 {
		if err := writeFrameHeader(w, name, uint32(len(value)+1), flags, writeOptions); err != nil {
			return err
		}
		if !writeOptions.UseID3v23 {
			var err error
			dli, err = synchsafe32Checked(dli)
			if err != nil {
				return frameEncodingError(None[FrameId](), err)
			}
		}
		var lb [4]byte
		binary.BigEndian.PutUint32(lb[:], dli)
		w.Write(lb[:])
		w.Write([]byte{methodSymbol})
		_, err := w.Write(value)
		return err
	}
	return frameEncodingMissingDLI(FrameIdValid(name))
}

func writeFrameHeader(w io.Writer, name string, length uint32, flags FrameFlags, writeOptions WriteOptions) error {
	var flagBytes uint16
	if writeOptions.UseID3v23 {
		flagBytes = flags.AsId3v23Bytes()
	} else {
		flagBytes = flags.AsId3v24Bytes()
	}
	if _, err := w.Write([]byte(name)); err != nil {
		return err
	}
	if !writeOptions.UseID3v23 {
		var err error
		length, err = synchsafe32Checked(length)
		if err != nil {
			return frameEncodingError(None[FrameId](), err)
		}
	}
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], length)
	if _, err := w.Write(tmp[:]); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(tmp[:2], flagBytes)
	_, err := w.Write(tmp[:2])
	return err
}
