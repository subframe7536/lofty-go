package mpc

import (
	"bytes"
	"os"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
)

// sv8Packet encodes an SV8 packet: key + varint size (covering key, size
// bytes, and payload) + payload.
func sv8Packet(key string, payload []byte) []byte {
	// Encode size as an MPC magnitude varint (7 bits/byte, MSB continuation).
	// Iterate since the varint's own length is part of the size.
	size := len(payload) + 2
	for {
		n := size
		var enc [9]byte
		i := len(enc)
		for {
			i--
			enc[i] = byte(n & 0x7F)
			n >>= 7
			if n == 0 {
				break
			}
		}
		vb := enc[i:]
		total := len(payload) + 2 + len(vb)
		if total == size {
			pkt := []byte(key)
			for j := 0; j < len(vb)-1; j++ {
				pkt = append(pkt, vb[j]|0x80)
			}
			pkt = append(pkt, vb[len(vb)-1])
			return append(pkt, payload...)
		}
		size = total
	}
}

// sv8Stream builds a minimal SV8 stream: SH + caller-provided packets + SE.
func sv8Stream(middle ...[]byte) []byte {
	// SH body: CRC(4) + version(1) + sample_count varint(44100=0x82 0xD8 0x44)
	// + beginning_silence varint(0) + 2 flag bytes (sr idx 0, 2ch)
	sh := []byte{0, 0, 0, 0, 8, 0x82, 0xD8, 0x44, 0x00, 0x00, 0x10}
	stream := []byte("MPCK")
	stream = append(stream, sv8Packet("SH", sh)...)
	for _, p := range middle {
		stream = append(stream, p...)
	}
	return append(stream, sv8Packet("SE", nil)...)
}

func TestMpcSv8Basic(t *testing.T) {
	rg := make([]byte, 9)
	data := sv8Stream(sv8Packet("RG", rg), sv8Packet("AP", make([]byte, 20000)))
	f, err := MusepackFileReadFrom(bytes.NewReader(data), meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.SampleRate != 44100 || f.Properties.Channels != 2 || f.Properties.Version != 8 {
		t.Fatalf("props = %+v", f.Properties)
	}
	if f.Properties.Duration == 0 || f.Properties.OverallBitrate == 0 {
		t.Fatalf("props = %+v", f.Properties)
	}
}

// Rust reads packet bodies through the capacity-bounded PacketReader: an RG
// packet whose declared payload is too small errors instead of reading into
// the next packet.
func TestMpcSv8TruncatedRg(t *testing.T) {
	// Declared RG capacity of 4 bytes; body read needs 9.
	pkt := []byte("RG")
	pkt = append(pkt, byte(4+3)) // size = key(2)+size(1)+4
	pkt = append(pkt, 0, 0, 0, 0)
	data := sv8Stream(pkt)
	po := meta.NewParseOptions()
	if _, err := MusepackFileReadFrom(bytes.NewReader(data), po); err == nil {
		t.Fatal("expected bounded-read error for truncated RG packet")
	}
}

// An invalid packet key ends the scan like Rust's `while let Ok(...)` —
// strict mode then errors only because mandatory packets are missing.
func TestMpcSv8BadKeyEndsScan(t *testing.T) {
	// The AP packet is never read (the bad key ends the scan first) — it
	// just pads the stream past 32 bytes so Rust's trailing `seek(-32)`
	// for the APE probe doesn't fail the read outright.
	data := sv8Stream([]byte{'a', 'a'}, sv8Packet("AP", make([]byte, 64)))
	po := meta.NewParseOptions()
	po.ParsingMode = meta.ParsingModeBestAttempt
	// SE was never reached, but non-strict mode doesn't require it
	f, err := MusepackFileReadFrom(bytes.NewReader(data), po)
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.Version != 8 {
		t.Fatalf("version = %d", f.Properties.Version)
	}
}

// segfault2.mpc is a 6-byte "MPCK"+"SH" stub: Rust's end-of-file APE probe
// (`seek(SeekFrom::Current(-32))`) lands before position 0 and the I/O
// error propagates, failing the whole read. Go must propagate too rather
// than silently succeeding with zeroed properties.
func TestSegfault2ShortFileErrors(t *testing.T) {
	f, err := os.Open(testutil.Path("lofty-rs", "lofty", "tests", "taglib", "data", "segfault2.mpc"))
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	defer f.Close()
	if _, err := MusepackFileReadFrom(f, meta.NewParseOptions()); err == nil {
		t.Fatal("expected error on truncated MPCK file")
	}
}
