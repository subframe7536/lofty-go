package meta

import "time"

// FileProperties contains various *immutable* audio properties.
type FileProperties struct {
	duration       time.Duration
	overallBitrate Option[uint32]
	audioBitrate   Option[uint32]
	sampleRate     Option[uint32]
	bitDepth       Option[uint8]
	channels       Option[uint8]
	channelMask    Option[ChannelMask]
}

// NewFileProperties creates a new [FileProperties].
func NewFileProperties(
	duration time.Duration,
	overallBitrate, audioBitrate, sampleRate Option[uint32],
	bitDepth, channels Option[uint8],
	channelMask Option[ChannelMask],
) FileProperties {
	return FileProperties{
		duration:       duration,
		overallBitrate: overallBitrate,
		audioBitrate:   audioBitrate,
		sampleRate:     sampleRate,
		bitDepth:       bitDepth,
		channels:       channels,
		channelMask:    channelMask,
	}
}

// Duration of the audio.
func (p FileProperties) Duration() time.Duration { return p.duration }

// OverallBitrate returns the overall bitrate (kbps).
func (p FileProperties) OverallBitrate() Option[uint32] { return p.overallBitrate }

// AudioBitrate returns the audio bitrate (kbps).
func (p FileProperties) AudioBitrate() Option[uint32] { return p.audioBitrate }

// SampleRate returns the sample rate (Hz).
func (p FileProperties) SampleRate() Option[uint32] { return p.sampleRate }

// BitDepth returns the bits per sample (usually 16 or 24 bit).
func (p FileProperties) BitDepth() Option[uint8] { return p.bitDepth }

// Channels returns the channel count.
func (p FileProperties) Channels() Option[uint8] { return p.channels }

// ChannelMask returns the channel mask.
func (p FileProperties) ChannelMask() Option[ChannelMask] { return p.channelMask }

// IsEmpty reports whether all properties are unset. Used for tests.
func (p FileProperties) IsEmpty() bool {
	_, ob := p.overallBitrate.Get()
	_, ab := p.audioBitrate.Get()
	_, sr := p.sampleRate.Get()
	_, bd := p.bitDepth.Get()
	_, ch := p.channels.Get()
	_, cm := p.channelMask.Get()
	return p.duration == 0 &&
		(!ob || p.overallBitrate.OrZero() == 0) &&
		(!ab || p.audioBitrate.OrZero() == 0) &&
		(!sr || p.sampleRate.OrZero() == 0) &&
		(!bd || p.bitDepth.OrZero() == 0) &&
		(!ch || p.channels.OrZero() == 0) &&
		!cm
}

// ChannelMask is a mask of (at least) 18 bits, one for each channel.
//
// * Standard speaker channels: <https://en.wikipedia.org/wiki/Surround_sound#Channel_notation>
// * CAF channel bitmap: <https://developer.apple.com/library/archive/documentation/MusicAudio/Reference/CAFSpec/CAF_spec/CAF_spec.html#//apple_ref/doc/uid/TP40001862-CH210-BCGBHHHI>
// * WAV default channel ordering: <https://learn.microsoft.com/en-us/previous-versions/windows/hardware/design/dn653308(v=vs.85)>
// * FFmpeg: <https://ffmpeg.org/doxygen/trunk/group__channel__masks.html>
type ChannelMask uint32

const (
	// ChannelMaskFrontLeft is the front left speaker.
	ChannelMaskFrontLeft ChannelMask = 1 << 0
	// ChannelMaskFrontRight is the front right speaker.
	ChannelMaskFrontRight ChannelMask = 1 << 1
	// ChannelMaskFrontCenter is the front center speaker.
	ChannelMaskFrontCenter ChannelMask = 1 << 2
	// ChannelMaskLowFrequency is the low frequency effects (subwoofer).
	ChannelMaskLowFrequency ChannelMask = 1 << 3
	// ChannelMaskBackLeft is the back left speaker (surround left).
	ChannelMaskBackLeft ChannelMask = 1 << 4
	// ChannelMaskBackRight is the back right speaker (surround right).
	ChannelMaskBackRight ChannelMask = 1 << 5
	// ChannelMaskFrontLeftOfCenter is the front left of center speaker.
	ChannelMaskFrontLeftOfCenter ChannelMask = 1 << 6
	// ChannelMaskFrontRightOfCenter is the front right of center speaker.
	ChannelMaskFrontRightOfCenter ChannelMask = 1 << 7
	// ChannelMaskBackCenter is the back center speaker.
	ChannelMaskBackCenter ChannelMask = 1 << 8
	// ChannelMaskSideLeft is the side left speaker.
	ChannelMaskSideLeft ChannelMask = 1 << 9
	// ChannelMaskSideRight is the side right speaker.
	ChannelMaskSideRight ChannelMask = 1 << 10
	// ChannelMaskTopCenter is the top center speaker.
	ChannelMaskTopCenter ChannelMask = 1 << 11
	// ChannelMaskTopFrontLeft is the top front left speaker.
	ChannelMaskTopFrontLeft ChannelMask = 1 << 12
	// ChannelMaskTopFrontCenter is the top front center speaker.
	ChannelMaskTopFrontCenter ChannelMask = 1 << 13
	// ChannelMaskTopFrontRight is the top front right speaker.
	ChannelMaskTopFrontRight ChannelMask = 1 << 14
	// ChannelMaskTopBackLeft is the top back left speaker.
	ChannelMaskTopBackLeft ChannelMask = 1 << 15
	// ChannelMaskTopBackCenter is the top back center speaker.
	ChannelMaskTopBackCenter ChannelMask = 1 << 16
	// ChannelMaskTopBackRight is the top back right speaker.
	ChannelMaskTopBackRight ChannelMask = 1 << 17
)

// ChannelMaskMono is a single front center channel.
const ChannelMaskMono = ChannelMaskFrontCenter

// ChannelMaskStereo is the front left+right channels.
const ChannelMaskStereo = ChannelMaskFrontLeft | ChannelMaskFrontRight

// ChannelMaskLinearSurround is the front left+right+center channels.
const ChannelMaskLinearSurround = ChannelMaskFrontLeft | ChannelMaskFrontRight | ChannelMaskFrontCenter

// Bits returns the bit mask.
func (c ChannelMask) Bits() uint32 { return uint32(c) }

// ChannelMaskFromOpusChannels creates a channel mask from the number of channels in an Opus file.
//
// See <https://datatracker.ietf.org/doc/html/rfc7845#section-5.1.1.2> for the mapping.
func ChannelMaskFromOpusChannels(channels uint8) Option[ChannelMask] {
	switch channels {
	case 1:
		return Some(ChannelMaskMono)
	case 2:
		return Some(ChannelMaskStereo)
	case 3:
		return Some(ChannelMaskLinearSurround)
	case 4:
		return Some(ChannelMaskFrontLeft | ChannelMaskFrontRight | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 5:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 6:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight | ChannelMaskLowFrequency)
	case 7:
		return Some(ChannelMaskLinearSurround | ChannelMaskSideLeft | ChannelMaskSideRight | ChannelMaskBackCenter | ChannelMaskLowFrequency)
	case 8:
		return Some(ChannelMaskLinearSurround | ChannelMaskSideLeft | ChannelMaskSideRight | ChannelMaskBackLeft | ChannelMaskBackRight | ChannelMaskLowFrequency)
	}
	return None[ChannelMask]()
}

// ChannelMaskFromMp4Channels creates a channel mask from the number of channels in an MP4 file.
//
// See <https://wiki.multimedia.cx/index.php/MPEG-4_Audio#Channel_Configurations> for the mapping.
func ChannelMaskFromMp4Channels(channels uint8) Option[ChannelMask] {
	switch channels {
	case 1:
		return Some(ChannelMaskMono)
	case 2:
		return Some(ChannelMaskStereo)
	case 3:
		return Some(ChannelMaskLinearSurround)
	case 4:
		return Some(ChannelMaskFrontLeft | ChannelMaskFrontRight | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 5:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 6:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight | ChannelMaskLowFrequency)
	case 7:
		return Some(ChannelMaskLinearSurround | ChannelMaskSideLeft | ChannelMaskSideRight | ChannelMaskBackLeft | ChannelMaskBackRight | ChannelMaskLowFrequency)
	}
	return None[ChannelMask]()
}

// ChannelMaskFromDsfChannelType creates a channel mask from a DSF channel type.
//
// See <https://dsd-guide.com/sites/default/files/white-papers/DSFFileFormatSpec_E.pdf> for the mapping.
func ChannelMaskFromDsfChannelType(channelType uint8) Option[ChannelMask] {
	switch channelType {
	case 1:
		return Some(ChannelMaskMono)
	case 2:
		return Some(ChannelMaskStereo)
	case 3:
		return Some(ChannelMaskLinearSurround)
	case 4:
		return Some(ChannelMaskFrontLeft | ChannelMaskFrontRight | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 5:
		return Some(ChannelMaskLinearSurround | ChannelMaskLowFrequency)
	case 6:
		return Some(ChannelMaskLinearSurround | ChannelMaskBackLeft | ChannelMaskBackRight)
	case 7:
		return Some(ChannelMaskLinearSurround | ChannelMaskLowFrequency | ChannelMaskBackLeft | ChannelMaskBackRight)
	}
	return None[ChannelMask]()
}
