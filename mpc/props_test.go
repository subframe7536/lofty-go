package mpc

import (
	"bytes"
	"errors"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
)

// Rust musepack/sv8/properties.rs: the Stream Header, ReplayGain and Encoder
// Info packets are exposed on the properties.
func TestMpcSv8PacketFields(t *testing.T) {
	sh := []byte{
		0xDE, 0xAD, 0xBE, 0xEF, // CRC
		8,                // stream version
		0x82, 0xD8, 0x44, // sample count (44100)
		0x00, // beginning silence
		0x3F, // sample rate index 1 (48000), max used bands 32
		0x1A, // 2 channels, MS used, audio block frames 4^2
	}
	rg := []byte{1, 0x4E, 0x8F, 0x60, 0xC1, 0x12, 0x34, 0x59, 0xF5}
	ei := []byte{0x23, 1, 17, 3} // profile 17/8, PNS tool, 1.17.3

	data := []byte("MPCK")
	data = append(data, sv8Packet("SH", sh)...)
	data = append(data, sv8Packet("RG", rg)...)
	data = append(data, sv8Packet("EI", ei)...)
	data = append(data, sv8Packet("AP", make([]byte, 100))...)
	data = append(data, sv8Packet("SE", nil)...)

	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeStrict
	f, err := MusepackFileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	if f.StreamVersion() != MpcStreamVersionSv8 {
		t.Fatalf("stream version = %v", f.StreamVersion())
	}
	wantSH := MpcSv8StreamHeader{
		CRC:              0xDEADBEEF,
		StreamVersion:    8,
		SampleCount:      44100,
		SampleRate:       48000,
		MaxUsedBands:     32,
		Channels:         2,
		MSUsed:           true,
		AudioBlockFrames: 16,
	}
	if f.Properties.StreamHeader != wantSH {
		t.Fatalf("stream header = %+v, want %+v", f.Properties.StreamHeader, wantSH)
	}
	wantRG := MpcSv8ReplayGain{Version: 1, TitleGain: 0x4E8F, TitlePeak: 0x60C1, AlbumGain: 0x1234, AlbumPeak: 0x59F5}
	if f.Properties.ReplayGain != wantRG {
		t.Fatalf("replay gain = %+v, want %+v", f.Properties.ReplayGain, wantRG)
	}
	info, ok := f.Properties.EncoderInfo.Get()
	if !ok {
		t.Fatal("expected encoder info")
	}
	if want := (MpcSv8EncoderInfo{Profile: 2.125, PNSTool: true, Major: 1, Minor: 17, Build: 3}); info != want {
		t.Fatalf("encoder info = %+v, want %+v", info, want)
	}
	if f.Properties.SampleRate != 48000 || f.Properties.Channels != 2 || f.Properties.Version != 8 {
		t.Fatalf("properties = %+v", f.Properties)
	}
}

// Without an Encoder Info packet the option stays empty.
func TestMpcSv8NoEncoderInfo(t *testing.T) {
	data := sv8Stream(sv8Packet("RG", make([]byte, 9)), sv8Packet("AP", make([]byte, 100)))
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.EncoderInfo.IsSome() {
		t.Fatal("unexpected encoder info")
	}
}

func TestMpcStreamVersion(t *testing.T) {
	sv7 := mpcSv7()
	sv7[3] = 0x17 // only the low nibble is the stream version
	sv46 := make([]byte, 40)
	sv46[1] = 0x30 // stream version 6 in bits 11..20 of the first dword

	cases := []struct {
		name    string
		data    []byte
		want    MpcStreamVersion
		version uint8
	}{
		{"sv8", sv8Stream(sv8Packet("RG", make([]byte, 9)), sv8Packet("AP", make([]byte, 100))), MpcStreamVersionSv8, 8},
		{"sv7", sv7, MpcStreamVersionSv7, 7},
		{"sv4to6", sv46, MpcStreamVersionSv4to6, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := MusepackFileReadFrom(bytes.NewReader(c.data), meta.NewParseOptions())
			if err != nil {
				t.Fatal(err)
			}
			if f.StreamVersion() != c.want {
				t.Fatalf("stream version = %v, want %v", f.StreamVersion(), c.want)
			}
			if f.Properties.Version != c.version {
				t.Fatalf("properties version = %d, want %d", f.Properties.Version, c.version)
			}

			// The stream version does not depend on reading properties.
			po := meta.NewParseOptions()
			po.ReadProperties = false
			f, err = MusepackFileReadFrom(bytes.NewReader(c.data), po)
			if err != nil {
				t.Fatal(err)
			}
			if f.StreamVersion() != c.want {
				t.Fatalf("no properties: stream version = %v, want %v", f.StreamVersion(), c.want)
			}
		})
	}
}

// Rust checked_sub: a leading ID3v2 tag plus the trailing ID3v1/APE tags may
// not exceed the stream; an APE tag bigger than what is left is a size
// mismatch, not an unsigned wrap-around.
func TestMpcApeTagLargerThanStream(t *testing.T) {
	// ID3v2 tag + SV7 stream + empty APE footer + ID3v1: the footer's size
	// field reaches back five bytes into the ID3v2 tag, so it fits in the
	// file but not in the stream left after the ID3v2 tag is removed.
	id3v2 := mpcId3v24(mpcTIT2("Hello"), false, false)
	body := append(mpcSv7(), make([]byte, 64)...)
	ape := make([]byte, 32)
	copy(ape, "APETAGEX")
	ape[8] = 0xD0 // version 2000
	ape[9] = 0x07
	ape[12] = byte(len(body) + 32 + 5)
	id3v1 := make([]byte, 128)
	copy(id3v1, "TAG")
	data := append(id3v2, body...)
	data = append(data, ape...)
	data = append(data, id3v1...)

	_, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	var sm *meta.SizeMismatchError
	if !errors.As(err, &sm) {
		t.Fatalf("err = %v, want SizeMismatchError", err)
	}
}
