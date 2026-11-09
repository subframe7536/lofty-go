package lofty

import (
	"bytes"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- src/properties/tests.rs ---

var minimalAssets = filepath.Join("lofty-rs", "lofty", "tests", "files", "assets", "minimal")

func readAsset(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(minimalAssets, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func eqMs(t *testing.T, got time.Duration, wantMs int64) {
	t.Helper()
	if got.Milliseconds() != wantMs {
		t.Errorf("duration = %dms, want %dms", got.Milliseconds(), wantMs)
	}
}

func eq[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func binaryBigU128(b []byte) string {
	return new(big.Int).SetBytes(b).String()
}

func TestPropertiesAac(t *testing.T) {
	f, err := AacFileReadFrom(bytes.NewReader(readAsset(t, "full_test.aac")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, MpegVersion4)
	eq(t, "audio_object_type", p.AudioObjectType, AudioObjectTypeAacLowComplexity)
	eqMs(t, p.Duration, 1474)
	eq(t, "overall_bitrate", p.OverallBitrate, 117)
	eq(t, "audio_bitrate", p.AudioBitrate, 117)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
	eq(t, "channel_mask", p.ChannelMask.OrZero(), ChannelMaskStereo)
	eq(t, "copyright", p.Copyrighted, false)
	eq(t, "original", p.Original, false)
}

func TestPropertiesAiff(t *testing.T) {
	f, err := AiffFileReadFrom(bytes.NewReader(readAsset(t, "full_test.aiff")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 1542)
	eq(t, "audio_bitrate", p.AudioBitrate, 1536)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "sample_size", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
	eq(t, "compression_type", p.CompressionType.IsNone(), true)
}

func TestPropertiesApe(t *testing.T) {
	f, err := ApeFileReadFrom(bytes.NewReader(readAsset(t, "full_test.ape")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, uint16(3990))
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 361)
	eq(t, "audio_bitrate", p.AudioBitrate, 360)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
}

func TestPropertiesDsf(t *testing.T) {
	f, err := DsfFileReadFrom(bytes.NewReader(readAsset(t, "full_test.dsf")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "format_id", p.FormatId, DsfFormatIdDsdRaw)
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 5697)
	eq(t, "audio_bitrate", p.AudioBitrate, 5645)
	eq(t, "sample_rate", p.SampleRate, 2822400)
	eq(t, "bit_depth", p.BitDepth, 1)
	eq(t, "channels", p.Channels, 2)
	eq(t, "channel_mask", p.ChannelMask.OrZero(), ChannelMaskStereo)
	eq(t, "sample_count", p.SampleCount, uint64(4030505))
	eq(t, "block_size", p.BlockSize, uint32(4096))
}

func TestPropertiesFlac(t *testing.T) {
	f, err := FlacFileReadFrom(bytes.NewReader(readAsset(t, "full_test.flac")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate.OrZero(), uint32(321))
	eq(t, "audio_bitrate", p.AudioBitrate, 275)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
	eq(t, "signature", binaryBigU128(p.Signature[:]), "164506065180489231127156351872182799315")
}

func TestPropertiesMp1(t *testing.T) {
	f, err := MpegFileReadFrom(bytes.NewReader(readAsset(t, "full_test.mp1")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, MpegVersion1)
	eq(t, "layer", p.Layer, MpegLayer1)
	eq(t, "channel_mode", p.ChannelMode, ChannelModeStereo)
	eq(t, "copyright", p.Copyrighted, false)
	eq(t, "original", p.Original, true)
	eqMs(t, p.Duration, 588)
	eq(t, "overall_bitrate", p.OverallBitrate, 384)
	eq(t, "audio_bitrate", p.AudioBitrate, 384)
	eq(t, "sample_rate", p.SampleRate, 32000)
	eq(t, "channels", p.Channels, 2)
}

func TestPropertiesMp2(t *testing.T) {
	f, err := MpegFileReadFrom(bytes.NewReader(readAsset(t, "full_test.mp2")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, MpegVersion1)
	eq(t, "layer", p.Layer, MpegLayer2)
	eq(t, "channel_mode", p.ChannelMode, ChannelModeStereo)
	eq(t, "copyright", p.Copyrighted, false)
	eq(t, "original", p.Original, true)
	eqMs(t, p.Duration, 1440)
	eq(t, "overall_bitrate", p.OverallBitrate, 384)
	eq(t, "audio_bitrate", p.AudioBitrate, 384)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
}

func TestPropertiesMp3(t *testing.T) {
	f, err := MpegFileReadFrom(bytes.NewReader(readAsset(t, "full_test.mp3")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, MpegVersion1)
	eq(t, "layer", p.Layer, MpegLayer3)
	eq(t, "channel_mode", p.ChannelMode, ChannelModeStereo)
	eq(t, "copyright", p.Copyrighted, false)
	eq(t, "original", p.Original, false)
	eqMs(t, p.Duration, 1464)
	eq(t, "overall_bitrate", p.OverallBitrate, 64)
	eq(t, "audio_bitrate", p.AudioBitrate, 62)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
}

func TestPropertiesMp4Aac(t *testing.T) {
	f, err := Mp4FileReadFrom(bytes.NewReader(readAsset(t, "m4a_codec_aac.m4a")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "codec", p.Codec, Mp4CodecAAC)
	eq(t, "extended_audio_object_type", p.ExtendedAudioObjectType.OrZero(), AudioObjectTypeAacLowComplexity)
	eqMs(t, p.Duration, 1449)
	eq(t, "overall_bitrate", p.OverallBitrate, 135)
	eq(t, "audio_bitrate", p.AudioBitrate, 124)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
	eq(t, "drm", p.IsDrmProtected, false)
	eq(t, "ftyp", p.Ftyp, "M4A ")
}

func TestPropertiesMp4Alac(t *testing.T) {
	f, err := Mp4FileReadFrom(bytes.NewReader(readAsset(t, "m4a_codec_alac.m4a")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "codec", p.Codec, Mp4CodecALAC)
	eq(t, "extended_audio_object_type", p.ExtendedAudioObjectType.IsNone(), true)
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 331)
	eq(t, "audio_bitrate", p.AudioBitrate, 326)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
	eq(t, "ftyp", p.Ftyp, "M4A ")
}

func TestPropertiesMp4Als(t *testing.T) {
	f, err := Mp4FileReadFrom(bytes.NewReader(readAsset(t, "mp4_codec_als.mp4")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "codec", p.Codec, Mp4CodecAAC)
	eq(t, "extended_audio_object_type", p.ExtendedAudioObjectType.OrZero(), AudioObjectTypeAudioLosslessCoding)
	eqMs(t, p.Duration, 1429)
	eq(t, "overall_bitrate", p.OverallBitrate, 1083)
	eq(t, "audio_bitrate", p.AudioBitrate, 1078)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
	eq(t, "ftyp", p.Ftyp, "mp42")
}

func TestPropertiesMp4Flac(t *testing.T) {
	f, err := Mp4FileReadFrom(bytes.NewReader(readAsset(t, "mp4_codec_flac.mp4")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "codec", p.Codec, Mp4CodecFLAC)
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 280)
	eq(t, "audio_bitrate", p.AudioBitrate, 275)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
	eq(t, "ftyp", p.Ftyp, "isom")
}

func TestPropertiesMpcSv5(t *testing.T) {
	f, err := MusepackFileReadFrom(bytes.NewReader(readAsset(t, "mpc_sv5.mpc")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, uint8(5))
	eqMs(t, p.Duration, 26347)
	eq(t, "average_bitrate", p.OverallBitrate, 119)
	eq(t, "channels", p.Channels, 2)
	eq(t, "frame_count", p.FrameCount, uint32(1009))
	eq(t, "mid_side_stereo", p.MidSideStereo, true)
	eq(t, "max_band", p.MaxBand, uint8(31))
	eq(t, "sample_rate", p.SampleRate, 44100)
}

func TestPropertiesMpcSv7(t *testing.T) {
	f, err := MusepackFileReadFrom(bytes.NewReader(readAsset(t, "mpc_sv7.mpc")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, uint8(7))
	eqMs(t, p.Duration, 1440)
	eq(t, "average_bitrate", p.OverallBitrate, 86)
	eq(t, "channels", p.Channels, 2)
	eq(t, "frame_count", p.FrameCount, uint32(60))
	eq(t, "intensity_stereo", p.IntensityStereo, false)
	eq(t, "mid_side_stereo", p.MidSideStereo, true)
	eq(t, "max_band", p.MaxBand, uint8(26))
	eq(t, "profile", p.Profile, MpcProfileStandard)
	eq(t, "link", p.Link, MpcLinkVeryLowStartOrEnd)
	eq(t, "sample_freq", p.SampleRate, 48000)
	eq(t, "max_level", p.MaxLevel, uint16(0))
	eq(t, "title_gain", p.TitleGain, int16(0))
	eq(t, "title_peak", p.TitlePeak, uint16(0))
	eq(t, "album_gain", p.AlbumGain, int16(0))
	eq(t, "album_peak", p.AlbumPeak, uint16(0))
	eq(t, "true_gapless", p.TrueGapless, true)
	eq(t, "last_frame_length", p.LastFrameLength, uint16(578))
	eq(t, "fast_seeking_safe", p.FastSeekingSafe, false)
	eq(t, "encoder_version", p.EncoderVersion, uint8(192))
}

func TestPropertiesMpcSv8(t *testing.T) {
	f, err := MusepackFileReadFrom(bytes.NewReader(readAsset(t, "mpc_sv8.mpc")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, uint8(8))
	eqMs(t, p.Duration, 1428)
	eq(t, "average_bitrate", p.OverallBitrate, 82)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
}

func TestPropertiesOpus(t *testing.T) {
	f, err := OggOpusFileReadFrom(bytes.NewReader(readAsset(t, "full_test.opus")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 120)
	eq(t, "audio_bitrate", p.AudioBitrate, 120)
	eq(t, "channels", p.Channels, 2)
	eq(t, "channel_mask", p.ChannelMask.OrZero(), ChannelMaskStereo)
	eq(t, "version", p.Version, uint32(1))
	eq(t, "input_sample_rate", p.InputSampleRate, 48000)
	eq(t, "output_gain", p.OutputGainDb, float32(0))
}

func TestPropertiesSpeex(t *testing.T) {
	f, err := OggSpeexFileReadFrom(bytes.NewReader(readAsset(t, "full_test.spx")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eqMs(t, p.Duration, 1469)
	eq(t, "version", p.Version, uint32(1))
	eq(t, "sample_rate", p.SampleRate, 32000)
	eq(t, "mode", p.Mode, uint32(2))
	eq(t, "channels", p.Channels, 2)
	eq(t, "vbr", p.Vbr, false)
	eq(t, "overall_bitrate", p.OverallBitrate, 32)
	eq(t, "audio_bitrate", p.AudioBitrate, 29)
	eq(t, "nominal_bitrate", p.BitrateNominal, int32(29600))
}

func TestPropertiesVorbis(t *testing.T) {
	f, err := OggVorbisFileReadFrom(bytes.NewReader(readAsset(t, "full_test.ogg")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eqMs(t, p.Duration, 1451)
	eq(t, "overall_bitrate", p.OverallBitrate, 96)
	eq(t, "audio_bitrate", p.AudioBitrate, 112)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
	eq(t, "version", p.Version, uint32(0))
	eq(t, "bitrate_maximum", p.BitrateMax, int32(0))
	eq(t, "bitrate_nominal", p.BitrateNominal, int32(112000))
	eq(t, "bitrate_minimum", p.BitrateMin, int32(0))
}

func TestPropertiesWav(t *testing.T) {
	f, err := WavFileReadFrom(bytes.NewReader(readAsset(t, "wav_format_pcm.wav")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "format", p.Format, uint16(1))
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 1542)
	eq(t, "audio_bitrate", p.AudioBitrate, 1536)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "channels", p.Channels, 2)
	eq(t, "channel_mask", p.ChannelMask.IsNone(), true)
}

func TestPropertiesWavPack(t *testing.T) {
	f, err := WavPackFileReadFrom(bytes.NewReader(readAsset(t, "full_test.wv")), NewParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	p := f.Properties
	eq(t, "version", p.Version, uint16(1040))
	eqMs(t, p.Duration, 1428)
	eq(t, "overall_bitrate", p.OverallBitrate, 598)
	eq(t, "audio_bitrate", p.AudioBitrate, 597)
	eq(t, "sample_rate", p.SampleRate, 48000)
	eq(t, "channels", p.Channels, 2)
	eq(t, "channel_mask", p.ChannelMask.OrZero(), ChannelMaskStereo)
	eq(t, "bit_depth", p.BitDepth, 16)
	eq(t, "lossless", p.IsLossless, true)
}
