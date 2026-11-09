package lofty

import (
	"strconv"
	"strings"
	"sync"
)

// StarRating is a whole-number star rating.
type StarRating uint8

const (
	// StarRatingOne is one star.
	StarRatingOne StarRating = 1
	// StarRatingTwo is two stars.
	StarRatingTwo StarRating = 2
	// StarRatingThree is three stars.
	StarRatingThree StarRating = 3
	// StarRatingFour is four stars.
	StarRatingFour StarRating = 4
	// StarRatingFive is five stars.
	StarRatingFive StarRating = 5
)

// RatingProvider is a rating provider for [Popularimeter]s.
type RatingProvider interface {
	// SupportsEmail reports whether this provider should be used for the given email.
	SupportsEmail(email string) bool
	// DefaultEmail is the default email to use if none is provided.
	DefaultEmail() string
	// Rate converts a [StarRating] to a format-specific integer.
	Rate(tagType TagType, rating StarRating) uint8
	// ConvertRaw converts a format-specific rating to a [StarRating].
	ConvertRaw(tagType TagType, rating uint8) StarRating
}

var (
	customProviderMu sync.RWMutex
	customProvider   RatingProvider
)

// SetCustomRatingProvider applies a custom rating provider globally.
//
// By default, the fallback provider will be [DefaultRatingProvider].
// It returns false if a custom provider has already been set.
func SetCustomRatingProvider(provider RatingProvider) bool {
	customProviderMu.Lock()
	defer customProviderMu.Unlock()
	if customProvider != nil {
		return false
	}
	customProvider = provider
	return true
}

func getCustomProvider() RatingProvider {
	customProviderMu.RLock()
	defer customProviderMu.RUnlock()
	if customProvider != nil {
		return customProvider
	}
	return DefaultRatingProvider{}
}

// DefaultRatingProvider is the default [RatingProvider] used as a fallback for unknown
// ratings. This is equivalent to the [MusicBeeProvider], except it supports *all* emails.
type DefaultRatingProvider struct{}

// SupportsEmail always returns true.
func (DefaultRatingProvider) SupportsEmail(string) bool { return true }

// DefaultEmail returns [MusicBeeProvider]'s default email.
func (DefaultRatingProvider) DefaultEmail() string { return MusicBeeProvider{}.DefaultEmail() }

// Rate delegates to [MusicBeeProvider].
func (DefaultRatingProvider) Rate(tagType TagType, rating StarRating) uint8 {
	return MusicBeeProvider{}.Rate(tagType, rating)
}

// ConvertRaw delegates to [MusicBeeProvider].
func (DefaultRatingProvider) ConvertRaw(tagType TagType, rating uint8) StarRating {
	return MusicBeeProvider{}.ConvertRaw(tagType, rating)
}

// MusicBeeProvider is a MusicBee-style rating provider.
type MusicBeeProvider struct{}

// SupportsEmail reports whether this provider handles `email`.
func (MusicBeeProvider) SupportsEmail(email string) bool {
	return email == PopularimeterMusicBeeEmail
}

// DefaultEmail returns the MusicBee email.
func (MusicBeeProvider) DefaultEmail() string { return PopularimeterMusicBeeEmail }

// Rate converts a [StarRating] to a format-specific integer.
func (MusicBeeProvider) Rate(tagType TagType, rating StarRating) uint8 {
	if tagType == TagTypeID3v2 {
		switch rating {
		case StarRatingOne:
			return 1
		case StarRatingTwo:
			return 64
		case StarRatingThree:
			return 128
		case StarRatingFour:
			return 196
		case StarRatingFive:
			return 255
		}
	}
	return uint8(rating) * 20
}

// ConvertRaw converts a format-specific rating to a [StarRating].
func (MusicBeeProvider) ConvertRaw(tagType TagType, rating uint8) StarRating {
	if tagType == TagTypeID3v2 {
		switch {
		case rating <= 1:
			return StarRatingOne
		case rating <= 64:
			return StarRatingTwo
		case rating <= 128:
			return StarRatingThree
		case rating <= 196:
			return StarRatingFour
		default:
			return StarRatingFive
		}
	}
	switch {
	case rating <= 20:
		return StarRatingOne
	case rating <= 40:
		return StarRatingTwo
	case rating <= 60:
		return StarRatingThree
	case rating <= 80:
		return StarRatingFour
	default:
		return StarRatingFive
	}
}

// WindowsMediaPlayerProvider is a Windows Media Player-style rating provider.
type WindowsMediaPlayerProvider struct{}

// SupportsEmail reports whether this provider handles `email`.
func (WindowsMediaPlayerProvider) SupportsEmail(email string) bool {
	return email == PopularimeterWMPEmail
}

// DefaultEmail returns the WMP email.
func (WindowsMediaPlayerProvider) DefaultEmail() string { return PopularimeterWMPEmail }

// Rate converts a [StarRating] — WMP only supports ID3v2 ratings.
func (WindowsMediaPlayerProvider) Rate(_ TagType, rating StarRating) uint8 {
	return MusicBeeProvider{}.Rate(TagTypeID3v2, rating)
}

// ConvertRaw converts a format-specific rating.
func (WindowsMediaPlayerProvider) ConvertRaw(_ TagType, rating uint8) StarRating {
	return MusicBeeProvider{}.ConvertRaw(TagTypeID3v2, rating)
}

// PicardProvider is a MusicBrainz Picard-style rating provider.
type PicardProvider struct{}

// SupportsEmail reports whether this provider handles `email`.
func (PicardProvider) SupportsEmail(email string) bool {
	return email == PopularimeterPicardEmail
}

// DefaultEmail returns the Picard email.
func (PicardProvider) DefaultEmail() string { return PopularimeterPicardEmail }

// Rate converts a [StarRating] to a format-specific integer.
func (PicardProvider) Rate(tagType TagType, rating StarRating) uint8 {
	stars := uint8(rating)
	if tagType == TagTypeID3v2 {
		return stars * 51
	}
	return stars * 5
}

// ConvertRaw converts a format-specific rating to a [StarRating].
func (PicardProvider) ConvertRaw(tagType TagType, rating uint8) StarRating {
	if tagType == TagTypeID3v2 {
		switch {
		case rating <= 51:
			return StarRatingOne
		case rating <= 102:
			return StarRatingTwo
		case rating <= 153:
			return StarRatingThree
		case rating <= 204:
			return StarRatingFour
		default:
			return StarRatingFive
		}
	}
	switch {
	case rating <= 5:
		return StarRatingOne
	case rating <= 10:
		return StarRatingTwo
	case rating <= 15:
		return StarRatingThree
	case rating <= 20:
		return StarRatingFour
	default:
		return StarRatingFive
	}
}

// Well-known rating provider emails.
const (
	PopularimeterWMPEmail      = "Windows Media Player 9 Series"
	PopularimeterMusicBeeEmail = "MusicBee"
	PopularimeterPicardEmail   = "users@musicbrainz.org"
)

// Popularimeter is a generic user rating and play counter.
type Popularimeter struct {
	email          Option[string]
	ratingProvider RatingProvider
	// Rating is the star rating provided by the user.
	Rating StarRating
	// PlayCounter is the number of times the user has played the song.
	//
	// NOTE: This is only supported in ID3v2.
	PlayCounter uint64
}

// PopularimeterCustom creates a new [Popularimeter] using the custom [RatingProvider].
func PopularimeterCustom(email string, rating StarRating, playCounter uint64) Popularimeter {
	return Popularimeter{
		email:          Some(email),
		ratingProvider: getCustomProvider(),
		Rating:         rating,
		PlayCounter:    playCounter,
	}
}

func popularimeterMapped(email string, tagType TagType, rate uint8, playCounter uint64) (Popularimeter, bool) {
	var provider RatingProvider
	switch email {
	case PopularimeterWMPEmail:
		provider = WindowsMediaPlayerProvider{}
	case PopularimeterMusicBeeEmail:
		provider = MusicBeeProvider{}
	case PopularimeterPicardEmail:
		provider = PicardProvider{}
	default:
		provider = getCustomProvider()
		if email == "" {
			email = provider.DefaultEmail()
		}
		if !provider.SupportsEmail(email) {
			return Popularimeter{}, false
		}
	}
	return Popularimeter{
		email:          stringToOption(email),
		ratingProvider: provider,
		Rating:         provider.ConvertRaw(tagType, rate),
		PlayCounter:    playCounter,
	}, true
}

// Email returns the email associated with this rating.
func (p Popularimeter) Email() string {
	if e, ok := p.email.Get(); ok {
		return e
	}
	return p.ratingProvider.DefaultEmail()
}

// MappedValue converts the rating to a format-specific integer.
func (p Popularimeter) MappedValue(tagType TagType) uint8 {
	return p.ratingProvider.Rate(tagType, p.Rating)
}

// PopularimeterFromStr parses a popularimeter from a string of the form
// `email|rating|counter`.
func PopularimeterFromStr(s string) (Popularimeter, error) {
	parts := strings.SplitN(s, "|", 3)
	if len(parts) < 3 {
		return Popularimeter{}, &SizeMismatchError{}
	}
	ratingNum, err := strconv.Atoi(parts[1])
	if err != nil {
		return Popularimeter{}, err
	}
	var star StarRating
	switch ratingNum {
	case 1:
		star = StarRatingOne
	case 2:
		star = StarRatingTwo
	case 3:
		star = StarRatingThree
	case 4:
		star = StarRatingFour
	case 5:
		star = StarRatingFive
	default:
		return Popularimeter{}, &SizeMismatchError{}
	}
	counter, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return Popularimeter{}, err
	}

	var provider RatingProvider
	email := parts[0]
	switch email {
	case PopularimeterWMPEmail:
		provider = WindowsMediaPlayerProvider{}
	case PopularimeterMusicBeeEmail:
		provider = MusicBeeProvider{}
	case PopularimeterPicardEmail:
		provider = PicardProvider{}
	default:
		provider = getCustomProvider()
		if !provider.SupportsEmail(email) {
			return Popularimeter{}, &SizeMismatchError{}
		}
	}

	return Popularimeter{
		email:          stringToOption(email),
		ratingProvider: provider,
		Rating:         star,
		PlayCounter:    counter,
	}, nil
}

// PopularimeterWindowsMediaPlayer creates a new Windows Media Player-style rating.
func PopularimeterWindowsMediaPlayer(rating StarRating, playCounter uint64) Popularimeter {
	return Popularimeter{
		email:          Some(PopularimeterWMPEmail),
		ratingProvider: WindowsMediaPlayerProvider{},
		Rating:         rating,
		PlayCounter:    playCounter,
	}
}

// PopularimeterMusicBee creates a new MusicBee-style rating.
func PopularimeterMusicBee(rating StarRating, playCounter uint64) Popularimeter {
	return Popularimeter{
		email:          Some(PopularimeterMusicBeeEmail),
		ratingProvider: MusicBeeProvider{},
		Rating:         rating,
		PlayCounter:    playCounter,
	}
}

// PopularimeterPicard creates a new MusicBrainz Picard-style rating.
func PopularimeterPicard(rating StarRating, playCounter uint64) Popularimeter {
	return Popularimeter{
		email:          Some(PopularimeterPicardEmail),
		ratingProvider: PicardProvider{},
		Rating:         rating,
		PlayCounter:    playCounter,
	}
}

func (p Popularimeter) String() string {
	email, _ := p.email.Get()
	return email + "|" + strconv.Itoa(int(p.Rating)) + "|" + strconv.FormatUint(p.PlayCounter, 10)
}
