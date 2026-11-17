package wavpack

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"

	meta "github.com/subframe7536/lofty-go/internal/meta"
	"github.com/subframe7536/lofty-go/internal/testutil"
)

const (
	// DSD flag + initial + final block, sample rate index 10 (48000 Hz).
	wvtDSDInitialFinal = 0x80000000 | 0x800 | 0x1000 | 10<<23
	wvtDSDInitial      = 0x80000000 | 0x800 | 10<<23
	wvtDSDFinal        = 0x80000000 | 0x1000 | 10<<23
)

// wvtBlock builds one WavPack block (32-byte header + content).
func wvtBlock(flags, total, samples uint32, content []byte) []byte {
	b := make([]byte, 32+len(content))
	copy(b, "wvpk")
	binary.LittleEndian.PutUint32(b[4:], uint32(24+len(content)))
	binary.LittleEndian.PutUint16(b[8:], 0x410)
	binary.LittleEndian.PutUint32(b[12:], total)
	binary.LittleEndian.PutUint32(b[20:], samples)
	binary.LittleEndian.PutUint32(b[24:], flags)
	copy(b[32:], content)
	return b
}

func wvtRead(t *testing.T, data []byte, mode meta.ParsingMode) (*WavPackFile, error) {
	t.Helper()
	po := meta.NewParseOptions()
	po.ParsingMode = mode
	return WavPackFileReadFrom(bytes.NewReader(data), po)
}

// wvtChannelMask returns the channel mask as the generic FileProperties
// reports it (a zero mask is "no mask").
func wvtChannelMask(f *WavPackFile) (uint32, bool) {
	fp := f.Properties.FileProperties()
	m, ok := fp.ChannelMask().Get()
	return uint32(m), ok
}

// Multichannel sub-block walk, cases traced by hand through the Rust
// get_extended_meta_info (wavpack/properties.rs). Rust returns an error for all
// three, which Strict propagates and BestAttempt turns into a `break` before the
// initial/final block handling.
func TestWvMultichannelSubBlockCursor(t *testing.T) {
	cases := []struct {
		name     string
		content  []byte
		channels uint16
		mask     uint32
	}{
		// ID_MULTICHANNEL (size 2, mono mask 0x04), then nothing: Rust
		// subtracts only the mask byte from size and skips past the block end.
		{"0D 01 01 04", []byte{0x0D, 0x01, 0x01, 0x04}, 1, 0x04},
		// odd-size element with a u16 mask, then an ID_DSD element whose
		// multiplier Rust never reaches (it eats the element's pad-byte twice).
		{"4D 02 04 FD 47 00 0E 01 12 F5 02 01 00", []byte{0x4D, 0x02, 0x04, 0xFD, 0x47, 0x00, 0x0E, 0x01, 0x12, 0xF5, 0x02, 0x01, 0x00}, 4, 18429},
		// mono mask element, then an ID_DSD element Rust cannot reach.
		{"0D 01 01 04 0E 01 03 00", []byte{0x0D, 0x01, 0x01, 0x04, 0x0E, 0x01, 0x03, 0x00}, 1, 0x04},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := wvtBlock(wvtDSDInitialFinal, 1000, 1000, c.content)
			if _, err := wvtRead(t, data, meta.ParsingModeStrict); err == nil {
				t.Fatal("strict: expected SizeMismatch-style error (Rust get_extended_meta_info)")
			}
			f, err := wvtRead(t, data, meta.ParsingModeBestAttempt)
			if err != nil {
				t.Fatalf("best-attempt: unexpected error %v", err)
			}
			if f.Properties.SampleRate != 48000 {
				t.Errorf("sample rate = %d, want 48000 (DSD multiplier must not be applied)", f.Properties.SampleRate)
			}
			if f.Properties.Channels != c.channels {
				t.Errorf("channels = %d, want %d", f.Properties.Channels, c.channels)
			}
			if m, ok := wvtChannelMask(f); !ok || m != c.mask {
				t.Errorf("channel mask = %d (set=%v), want %d", m, ok, c.mask)
			}
			if f.Properties.Duration != 0 {
				t.Errorf("duration = %v, want 0 (block loop breaks before the initial block)", f.Properties.Duration)
			}
		})
	}
}

// Two blocks: the first sets a stereo-ish mask, the second's multichannel
// element sets mask 0. Rust assigns the mask unconditionally, and FileProperties
// maps ChannelMask(0) to None.
func TestWvZeroChannelMaskOverwrites(t *testing.T) {
	first := []byte{0x0D, 0x01, 0x02, 0x03, 0x00, 0x00}
	second := []byte{0x0D, 0x01, 0x02, 0x00, 0x00, 0x00}
	data := append(wvtBlock(wvtDSDInitial, 2000, 1000, first), wvtBlock(wvtDSDFinal, 2000, 1000, second)...)
	f, err := wvtRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := wvtChannelMask(f); ok {
		t.Fatalf("channel mask = %d, want none (zero mask overwrites the earlier one)", m)
	}
}

// Rust wavpack/read.rs has no up-front "wvpk" check; properties.rs breaks on
// the first non-wvpk block in non-strict modes and errors in Strict. Hand-traced
// results for 64 bytes of 'x':
//
//	properties off: Ok in every mode
//	properties on, Strict: error
//	properties on, BestAttempt/Relaxed: Ok with empty properties
func TestWvNoWvpkMagic(t *testing.T) {
	junk := bytes.Repeat([]byte("x"), 64)
	for _, mode := range []meta.ParsingMode{meta.ParsingModeStrict, meta.ParsingModeBestAttempt, meta.ParsingModeRelaxed} {
		for _, readProps := range []bool{false, true} {
			po := meta.NewParseOptions()
			po.ParsingMode = mode
			po.ReadProperties = readProps
			f, err := WavPackFileReadFrom(bytes.NewReader(junk), po)
			wantErr := readProps && mode == meta.ParsingModeStrict
			if (err != nil) != wantErr {
				t.Fatalf("mode=%v readProps=%v: err=%v, want error=%v", mode, readProps, err, wantErr)
			}
			if err == nil && f.Properties.SampleRate != 0 {
				t.Fatalf("mode=%v readProps=%v: expected empty properties", mode, readProps)
			}
		}
	}
}

// Rust wavpack/tests: non_standard_rate.wv (taglib test_wavpack.rs)
// asserts audio_bitrate == 0 for this fixture.
func TestWvNonStandardRateProperties(t *testing.T) {
	path := testutil.Path("lofty-rs", "lofty", "tests", "taglib", "data", "non_standard_rate.wv")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	f, err := WavPackFileReadFrom(fh, meta.NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Properties.Duration.Milliseconds(); got != 3675 {
		t.Errorf("duration = %d ms, want 3675", got)
	}
	if f.Properties.AudioBitrate != 0 {
		t.Errorf("audio bitrate = %d, want 0", f.Properties.AudioBitrate)
	}
	if f.Properties.Channels != 2 || f.Properties.BitDepth != 16 || !f.Properties.IsLossless {
		t.Errorf("props = %+v", f.Properties)
	}
}

// Rust stores the channel count as a u16 ("unlimited" streams) and truncates
// to u8 only in `From<WavPackProperties> for FileProperties`.
func TestWvChannelsAreU16(t *testing.T) {
	// ID_MULTICHANNEL, 6-byte element: channel byte 44 + high nibble 1 (+1) =
	// 301 channels with a 24-bit mask; two bytes of slack for Rust's skip.
	first := []byte{0x0D, 0x03, 0x2C, 0x01, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00}
	data := append(wvtBlock(wvtDSDInitial, 2000, 1000, first), wvtBlock(wvtDSDFinal, 2000, 1000, nil)...)
	f, err := wvtRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.Channels != 301 {
		t.Fatalf("channels = %d, want 301", f.Properties.Channels)
	}
	if got := f.GetProperties().Channels().OrZero(); got != uint8(301&0xFF) {
		t.Fatalf("generic channels = %d, want %d", got, uint8(301&0xFF))
	}
}

// Rust computes duration/bitrate with saturating casts; a huge stream must not
// depend on the platform's float-to-int conversion.
func TestWvBitrateSaturates(t *testing.T) {
	// One sample at 192 kHz (~5 µs) over a 5 MiB stream: the bitrate overflows u32.
	flags := uint32(0x800 | 0x1000 | 14<<23) // initial + final, rate index 14
	data := append(wvtBlock(flags, 1, 1, nil), make([]byte, 5<<20)...)
	f, err := wvtRead(t, data, meta.ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if f.Properties.AudioBitrate != math.MaxUint32 || f.Properties.OverallBitrate != math.MaxUint32 {
		t.Fatalf("bitrates = %d/%d, want saturated", f.Properties.AudioBitrate, f.Properties.OverallBitrate)
	}
}
