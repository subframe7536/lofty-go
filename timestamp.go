package lofty

import (
	"bytes"
	"fmt"
	"io"
)

// Timestamp is a subset of the ISO 8601 timestamp format.
type Timestamp struct {
	Year   uint16
	Month  Option[uint8]
	Day    Option[uint8]
	Hour   Option[uint8]
	Minute Option[uint8]
	Second Option[uint8]
}

// TimestampMaxLength is the maximum length of a timestamp in bytes.
const TimestampMaxLength = 19

var timestampSeparators = [4]byte{'-', '.', 'T', ':'}

func (t Timestamp) String() string {
	s := fmt.Sprintf("%04d", t.Year)
	if m, ok := t.Month.Get(); ok {
		s += fmt.Sprintf("-%02d", m)
		if d, ok := t.Day.Get(); ok {
			s += fmt.Sprintf("-%02d", d)
			if h, ok := t.Hour.Get(); ok {
				s += fmt.Sprintf("T%02d", h)
				if mi, ok := t.Minute.Get(); ok {
					s += fmt.Sprintf(":%02d", mi)
					if se, ok := t.Second.Get(); ok {
						s += fmt.Sprintf(":%02d", se)
					}
				}
			}
		}
	}
	return s
}

// Compare returns -1/0/1 comparing two timestamps field by field.
func (t Timestamp) Compare(o Timestamp) int {
	cmpOpt := func(a, b Option[uint8]) int {
		av, aok := a.Get()
		bv, bok := b.Get()
		switch {
		case !aok && !bok:
			return 0
		case !aok:
			return -1
		case !bok:
			return 1
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
		return 0
	}
	if t.Year < o.Year {
		return -1
	}
	if t.Year > o.Year {
		return 1
	}
	for _, c := range []int{
		cmpOpt(t.Month, o.Month), cmpOpt(t.Day, o.Day), cmpOpt(t.Hour, o.Hour),
		cmpOpt(t.Minute, o.Minute), cmpOpt(t.Second, o.Second),
	} {
		if c != 0 {
			return c
		}
	}
	return 0
}

// TimestampParseErrorKind is the kind of [TimestampParseError].
type TimestampParseErrorKind int

const (
	// TimestampParseErrorEmpty is an empty timestamp string.
	TimestampParseErrorEmpty TimestampParseErrorKind = iota
	// TimestampParseErrorBadYear is an invalid year field (e.g. less than 4 digits).
	TimestampParseErrorBadYear
	// TimestampParseErrorMissingSeparator means a field is missing a separator.
	TimestampParseErrorMissingSeparator
	// TimestampParseErrorSegmentTooShort means a field was shorter than expected.
	TimestampParseErrorSegmentTooShort
	// TimestampParseErrorContainsSpaces (strict mode only) means a segment contains spaces.
	TimestampParseErrorContainsSpaces
	// TimestampParseErrorUnexpectedChar means an unexpected character was encountered.
	TimestampParseErrorUnexpectedChar
	// TimestampParseErrorFieldTooLarge means a field failed verification when encoding.
	TimestampParseErrorFieldTooLarge
	// TimestampParseErrorIo is an I/O failure.
	TimestampParseErrorIo
)

// TimestampField is a field of a [Timestamp].
type TimestampField int

const (
	TimestampFieldYear TimestampField = iota
	TimestampFieldMonth
	TimestampFieldDay
	TimestampFieldHour
	TimestampFieldMinute
	TimestampFieldSecond
)

// TimestampParseError is an error that can occur while parsing [Timestamp]s.
type TimestampParseError struct {
	Kind   TimestampParseErrorKind
	Char   rune
	Field  TimestampField
	Source error
}

func (e *TimestampParseError) Error() string {
	switch e.Kind {
	case TimestampParseErrorEmpty:
		return "empty timestamp"
	case TimestampParseErrorBadYear:
		return "encountered an invalid year length (should be 4 digits)"
	case TimestampParseErrorMissingSeparator:
		return "expected a separator"
	case TimestampParseErrorSegmentTooShort:
		return "timestamp segment too short"
	case TimestampParseErrorContainsSpaces:
		return "timestamp contains spaces"
	case TimestampParseErrorUnexpectedChar:
		return fmt.Sprintf("unexpected character %q", e.Char)
	case TimestampParseErrorFieldTooLarge:
		return fmt.Sprintf("field %d exceeds its limits", e.Field)
	default:
		return e.Source.Error()
	}
}

func (e *TimestampParseError) Unwrap() error { return e.Source }

// TimestampParse parses a [Timestamp] from a reader.
//
// NOTES:
//   - When not using [ParsingModeStrict], this will skip any leading whitespace.
//   - Afterwards, this will take [TimestampMaxLength] bytes from the reader.
//     Ensure that it only contains the timestamp.
func TimestampParse(reader io.Reader, parseMode ParsingMode) (Option[Timestamp], error) {
	var c [1]byte
	var first byte
	if _, err := reader.Read(c[:]); err != nil {
		if parseMode == ParsingModeStrict {
			return None[Timestamp](), &TimestampParseError{Kind: TimestampParseErrorEmpty}
		}
		return None[Timestamp](), nil
	}
	first = c[0]

	if parseMode != ParsingModeStrict {
		for isASCIIWhitespace(first) {
			if _, err := reader.Read(c[:]); err != nil {
				return None[Timestamp](), &TimestampParseError{Kind: TimestampParseErrorIo, Source: err}
			}
			first = c[0]
		}
	}

	var timestamp Timestamp
	content := []byte{first}
	rest, err := io.ReadAll(io.LimitReader(reader, TimestampMaxLength-1))
	if err != nil {
		return None[Timestamp](), &TimestampParseError{Kind: TimestampParseErrorIo, Source: err}
	}
	content = append(content, rest...)

	// It is valid for a timestamp to contain no separators, but this will lower our
	// tolerance for common mistakes. We ignore the "T" separator here because it is
	// **ALWAYS** required.
	containsSeparators := false
	for _, b := range content {
		if b != 'T' && (b == '-' || b == '.' || b == ':') {
			containsSeparators = true
			break
		}
	}

	rdr := &byteCursor{b: content}

	year, bytesRead, err := timestampSegment(rdr, 4, nil, parseMode)
	if err != nil {
		return None[Timestamp](), err
	}
	if bytesRead != 4 {
		if parseMode == ParsingModeStrict {
			return None[Timestamp](), &TimestampParseError{Kind: TimestampParseErrorBadYear}
		}
		return None[Timestamp](), nil
	}
	timestamp.Year = year
	if rdr.len() == 0 {
		return Some(timestamp), nil
	}

	sepMd := []byte(nil)
	if containsSeparators {
		sepMd = timestampSeparators[:2]
	}
	sepHour := timestampSeparators[2:3]
	sepMinSec := []byte(nil)
	if containsSeparators {
		sepMinSec = timestampSeparators[3:4]
	}

	readSeg := func(sep []byte) (Option[uint8], error) {
		v, n, err := timestampSegment(rdr, 2, sep, parseMode)
		if err != nil {
			return None[uint8](), err
		}
		if n == 0 {
			return None[uint8](), nil
		}
		return Some(uint8(v)), nil
	}

	if timestamp.Month, err = readSeg(sepMd); err != nil {
		return None[Timestamp](), err
	}
	if timestamp.Day, err = readSeg(sepMd); err != nil {
		return None[Timestamp](), err
	}
	if timestamp.Hour, err = readSeg(sepHour); err != nil {
		return None[Timestamp](), err
	}
	if timestamp.Minute, err = readSeg(sepMinSec); err != nil {
		return None[Timestamp](), err
	}
	if timestamp.Second, err = readSeg(sepMinSec); err != nil {
		return None[Timestamp](), err
	}

	return Some(timestamp), nil
}

// TimestampParseString parses a [Timestamp] from a string.
func TimestampParseString(s string) (Timestamp, error) {
	ts, err := TimestampParse(bytes.NewReader([]byte(s)), ParsingModeBestAttempt)
	if err != nil {
		return Timestamp{}, err
	}
	if t, ok := ts.Get(); ok {
		return t, nil
	}
	return Timestamp{}, &TimestampParseError{Kind: TimestampParseErrorEmpty}
}

// byteCursor is a forward-only cursor over a byte slice.
type byteCursor struct {
	b []byte
}

func (c *byteCursor) len() int { return len(c.b) }

func (c *byteCursor) read() (byte, bool) {
	if len(c.b) == 0 {
		return 0, false
	}
	b := c.b[0]
	c.b = c.b[1:]
	return b, true
}

func (c *byteCursor) advance(n int) { c.b = c.b[n:] }

// timestampSegment reads a timestamp segment of up to `size` digits.
// Returns (value, bytesConsumed, error). (0, 0, nil) means "stop parsing".
func timestampSegment(content *byteCursor, size int, sep []byte, parseMode ParsingMode) (uint16, int, error) {
	if content.len() == 0 {
		return 0, 0, nil
	}

	if sep != nil {
		b, ok := content.read()
		if !ok {
			return 0, 0, &TimestampParseError{Kind: TimestampParseErrorIo, Source: io.EOF}
		}
		pos := -1
		for i, s := range sep {
			if s == b {
				pos = i
				break
			}
		}
		if pos < 0 {
			if parseMode == ParsingModeStrict {
				return 0, 0, &TimestampParseError{Kind: TimestampParseErrorMissingSeparator}
			}
			return 0, 0, nil
		}
		// The first separator in the list is the only *strictly* valid one.
		if pos > 0 && parseMode == ParsingModeStrict {
			return 0, 0, &TimestampParseError{Kind: TimestampParseErrorUnexpectedChar, Char: rune(b)}
		}
	}

	if content.len() < size {
		if parseMode == ParsingModeStrict {
			return 0, 0, &TimestampParseError{Kind: TimestampParseErrorSegmentTooShort}
		}
		return 0, 0, nil
	}

	var num *uint16
	byteCount := 0
	for _, b := range content.b[:size] {
		// Common spec violation: Timestamps may use spaces instead of zeros.
		if b == ' ' {
			if parseMode == ParsingModeStrict {
				return 0, 0, &TimestampParseError{Kind: TimestampParseErrorContainsSpaces}
			}
			byteCount++
			continue
		}
		if b < '0' || b > '9' {
			// Timestamps in the wild may not use a zero or a space, so "06", "6", and " 6"
			// are all treated as valid. Check if we are eating into the next segment's separator.
			isSep := false
			for _, s := range timestampSeparators {
				if s == b {
					isSep = true
				}
			}
			if sep != nil && isSep && parseMode != ParsingModeStrict {
				break
			}
			return 0, 0, &TimestampParseError{Kind: TimestampParseErrorUnexpectedChar, Char: rune(b)}
		}
		n := uint16(0)
		if num != nil {
			n = *num
		}
		n = n*10 + uint16(b-'0')
		num = &n
		byteCount++
	}

	if num == nil {
		return 0, 0, nil
	}
	content.advance(byteCount)
	return *num, byteCount, nil
}

// Verify ensures that all fields are within bounds (e.g. year <= 9999,
// month <= 12, etc.), and that every field has its parent.
func (t Timestamp) Verify() error {
	if t.Year > 9999 {
		return &TimestampParseError{Kind: TimestampParseErrorFieldTooLarge, Field: TimestampFieldYear}
	}
	verify := func(field TimestampField, f Option[uint8], limit uint8, parentSet bool) error {
		if v, ok := f.Get(); ok {
			if parentSet && v <= limit {
				return nil
			}
			return &TimestampParseError{Kind: TimestampParseErrorFieldTooLarge, Field: field}
		}
		return nil
	}
	if err := verify(TimestampFieldMonth, t.Month, 12, true); err != nil {
		return err
	}
	_, monthSet := t.Month.Get()
	if err := verify(TimestampFieldDay, t.Day, 31, monthSet); err != nil {
		return err
	}
	_, daySet := t.Day.Get()
	if err := verify(TimestampFieldHour, t.Hour, 23, daySet); err != nil {
		return err
	}
	_, hourSet := t.Hour.Get()
	if err := verify(TimestampFieldMinute, t.Minute, 59, hourSet); err != nil {
		return err
	}
	_, minSet := t.Minute.Get()
	return verify(TimestampFieldSecond, t.Second, 59, minSet)
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == 0x0B || b == 0x0C
}

// tryParseTimestamp attempts to parse `input` as a [Timestamp] in relaxed mode.
func tryParseTimestamp(input string) Option[Timestamp] {
	ts, err := TimestampParse(bytes.NewReader([]byte(input)), ParsingModeRelaxed)
	if err != nil {
		return None[Timestamp]()
	}
	return ts
}
