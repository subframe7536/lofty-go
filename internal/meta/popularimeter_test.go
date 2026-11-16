package meta

import "testing"

func TestZeroValuePopularimeterDoesNotPanic(t *testing.T) {
	var z Popularimeter
	if got := z.Email(); got != PopularimeterMusicBeeEmail {
		t.Errorf("zero Email() = %q", got)
	}
	_ = z.MappedValue(TagTypeID3v2)
	_ = z.String()
}

func TestRateSaturates(t *testing.T) {
	if got := (MusicBeeProvider{}).Rate(TagTypeVorbisComments, StarRating(200)); got != 255 {
		t.Errorf("MusicBee Rate(200) = %d, want 255", got)
	}
	if got := (PicardProvider{}).Rate(TagTypeID3v2, StarRating(100)); got != 255 {
		t.Errorf("Picard Rate(100) = %d, want 255", got)
	}
	if got := (PicardProvider{}).Rate(TagTypeVorbisComments, StarRating(100)); got != 255 {
		t.Errorf("Picard non-ID3v2 Rate(100) = %d, want 255", got)
	}
	// regular values unchanged
	if got := (MusicBeeProvider{}).Rate(TagTypeVorbisComments, StarRatingFive); got != 100 {
		t.Errorf("MusicBee Rate(5) = %d", got)
	}
	if got := (PicardProvider{}).Rate(TagTypeID3v2, StarRatingFive); got != 255 {
		t.Errorf("Picard ID3v2 Rate(5) = %d", got)
	}
}

func TestPopularimeterFromStrParsesLikeRust(t *testing.T) {
	ok := []struct {
		in      string
		counter uint64
		rating  StarRating
	}{
		{"MusicBee|3|5", 5, StarRatingThree},
		{"MusicBee|3|+5", 5, StarRatingThree},
		{"MusicBee|+4|0", 0, StarRatingFour},
		{"foo@example.com|1|18446744073709551615", 18446744073709551615, StarRatingOne},
		{"MusicBee|2|1|extra", 0, 0}, // placeholder, replaced below
	}
	for _, c := range ok[:4] {
		p, err := PopularimeterFromStr(c.in)
		if err != nil || p.PlayCounter != c.counter || p.Rating != c.rating {
			t.Errorf("%q: %+v %v", c.in, p, err)
		}
	}
	for _, in := range []string{
		"", "MusicBee", "MusicBee|3", "MusicBee|0|1", "MusicBee|6|1", "MusicBee|3|-1",
		"MusicBee|3|++1", "MusicBee|3|+", "MusicBee|3|", "MusicBee|3| 5", "MusicBee|x|1", "MusicBee|3|18446744073709551616",
		"MusicBee|2|1|extra",
	} {
		if _, err := PopularimeterFromStr(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}
