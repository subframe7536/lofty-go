package util

import (
	"errors"
	"strings"
	"testing"
)

// --- src/tag/items/timestamp.rs ---

func tsExpected() Timestamp {
	return Timestamp{
		Year:   2024,
		Month:  Some(uint8(6)),
		Day:    Some(uint8(3)),
		Hour:   Some(uint8(14)),
		Minute: Some(uint8(8)),
		Second: Some(uint8(49)),
	}
}

func tsParse(t *testing.T, content string, mode ParsingMode) (Option[Timestamp], error) {
	t.Helper()
	return TimestampParse(strings.NewReader(content), mode)
}

func TestTimestampDecode(t *testing.T) {
	got, err := tsParse(t, "2024-06-03T14:08:49", ParsingModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := got.Get(); !ok || ts != tsExpected() {
		t.Fatalf("got %v", got)
	}
}

func TestTimestampDecodeNoZero(t *testing.T) {
	got, err := tsParse(t, "2024-6-3T14:8:49", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := got.Get(); !ok || ts != tsExpected() {
		t.Fatalf("got %v", got)
	}
}

func TestTimestampDecodeZeroSubstitution(t *testing.T) {
	got, err := tsParse(t, "2024- 6- 3T14: 8:49", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := got.Get(); !ok || ts != tsExpected() {
		t.Fatalf("got %v", got)
	}
}

func TestTimestampEncode(t *testing.T) {
	if got := tsExpected().String(); got != "2024-06-03T14:08:49" {
		t.Fatalf("got %q", got)
	}
}

func TestTimestampEncodeInvalid(t *testing.T) {
	ts := tsExpected()
	// Hour, minute, and second depend on day
	ts.Day = None[uint8]()
	if got := len(ts.String()); got != 7 {
		t.Fatalf("len = %d", got)
	}
}

func tsBroken() []struct {
	in string
	ts Timestamp
} {
	y24 := Timestamp{Year: 2024}
	return []struct {
		in string
		ts Timestamp
	}{
		{"2024-", y24},
		{"2024-06-", Timestamp{Year: 2024, Month: Some(uint8(6))}},
		{"2024--", y24},
		{"2024-  -", y24},
		{"2024-06-03T", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}},
		{"2024:06", y24},
		{"2024-0-", Timestamp{Year: 2024, Month: Some(uint8(0))}},
	}
}

func TestTimestampRejectBrokenStrict(t *testing.T) {
	for _, tc := range tsBroken() {
		if _, err := tsParse(t, tc.in, ParsingModeStrict); err == nil {
			t.Errorf("%q: expected error", tc.in)
		}
	}
}

func TestTimestampAcceptBrokenBestAttempt(t *testing.T) {
	for _, tc := range tsBroken() {
		got, err := tsParse(t, tc.in, ParsingModeBestAttempt)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if ts, ok := got.Get(); !ok || ts != tc.ts {
			t.Errorf("%q: got %v, want %v", tc.in, got, tc.ts)
		}
	}
}

func TestTimestampDecodePartial(t *testing.T) {
	for _, tc := range []struct {
		in string
		ts Timestamp
	}{
		{"2024", Timestamp{Year: 2024}},
		{"2024-06", Timestamp{Year: 2024, Month: Some(uint8(6))}},
		{"2024-06-03", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}},
		{"2024-06-03T14", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3)), Hour: Some(uint8(14))}},
		{"2024-06-03T14:08", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3)), Hour: Some(uint8(14)), Minute: Some(uint8(8))}},
		{"2024-06-03T14:08:49", tsExpected()},
	} {
		got, err := tsParse(t, tc.in, ParsingModeStrict)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if ts, ok := got.Get(); !ok || ts != tc.ts {
			t.Fatalf("%q: got %v, want %v", tc.in, got, tc.ts)
		}
	}
}

func TestTimestampEmpty(t *testing.T) {
	got, err := tsParse(t, "", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSome() {
		t.Fatal("expected None")
	}
	if _, err := tsParse(t, "", ParsingModeStrict); err == nil {
		t.Fatal("strict: expected error")
	}
}

func TestTimestampNoSeparators(t *testing.T) {
	got, err := tsParse(t, "20240603T140849", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := got.Get(); !ok || ts != tsExpected() {
		t.Fatalf("got %v", got)
	}
}

func TestTimestampDecodePartialNoSeparators(t *testing.T) {
	for _, tc := range []struct {
		in string
		ts Timestamp
	}{
		{"2024", Timestamp{Year: 2024}},
		{"202406", Timestamp{Year: 2024, Month: Some(uint8(6))}},
		{"20240603", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}},
		{"20240603T14", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3)), Hour: Some(uint8(14))}},
		{"20240603T1408", Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3)), Hour: Some(uint8(14)), Minute: Some(uint8(8))}},
		{"20240603T140849", tsExpected()},
	} {
		got, err := tsParse(t, tc.in, ParsingModeBestAttempt)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if ts, ok := got.Get(); !ok || ts != tc.ts {
			t.Fatalf("%q: got %v, want %v", tc.in, got, tc.ts)
		}
	}
}

func TestTimestampNoTimeMarker(t *testing.T) {
	if _, err := tsParse(t, "2024-06-03 14:08:49", ParsingModeStrict); err == nil {
		t.Fatal("strict: expected error")
	}
	got, err := tsParse(t, "2024-06-03 14:08:49", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	want := Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}
	if ts, ok := got.Get(); !ok || ts != want {
		t.Fatalf("got %v", got)
	}
}

func TestTimestampWhitespace(t *testing.T) {
	if _, err := tsParse(t, "\t\t\t2024-06-03", ParsingModeStrict); err == nil {
		t.Fatal("strict: expected error")
	}
	got, err := tsParse(t, "\t\t\t2024-06-03", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	want := Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}
	if ts, ok := got.Get(); !ok || ts != want {
		t.Fatalf("got %v", got)
	}
}

// failingTSReader fails every read with err.
type failingTSReader struct{ err error }

func (r failingTSReader) Read([]byte) (int, error) { return 0, r.err }

// Rust: u8::is_ascii_whitespace() is false for 0x0B, so a leading vertical tab is
// not skipped and the year segment rejects it in every mode. Form feed is skipped.
func TestTimestampVerticalTabNotWhitespace(t *testing.T) {
	if _, err := tsParse(t, "\x0b2024-06-03", ParsingModeBestAttempt); err == nil {
		t.Fatal("leading vertical tab: expected an error")
	}
	got, err := tsParse(t, "\x0c2024-06-03", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	want := Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}
	if ts, ok := got.Get(); !ok || ts != want {
		t.Fatalf("form feed: got %v", got)
	}
}

// Rust: only UnexpectedEof on the first read means "no timestamp" (Ok(None) /
// Strict Empty). Any other read error is returned as an I/O error.
func TestTimestampReadErrors(t *testing.T) {
	boom := errors.New("boom")
	for _, mode := range []ParsingMode{ParsingModeStrict, ParsingModeBestAttempt, ParsingModeRelaxed} {
		_, err := TimestampParse(failingTSReader{boom}, mode)
		var tpe *TimestampParseError
		if !errors.As(err, &tpe) || tpe.Kind != TimestampParseErrorIo || !errors.Is(err, boom) {
			t.Errorf("mode %v: got %v, want I/O error wrapping boom", mode, err)
		}
	}

	// End of input is still "empty".
	got, err := TimestampParse(strings.NewReader(""), ParsingModeBestAttempt)
	if err != nil || got.IsSome() {
		t.Errorf("BestAttempt at EOF: got %v, %v; want None, nil", got, err)
	}
	if _, err := TimestampParse(strings.NewReader(""), ParsingModeStrict); err == nil {
		t.Error("Strict at EOF: expected an error")
	}
}

func TestTimestampDotSeparators(t *testing.T) {
	if _, err := tsParse(t, "2024.06.03", ParsingModeStrict); err == nil {
		t.Fatal("strict: expected error")
	}
	got, err := tsParse(t, "2024.06.03", ParsingModeBestAttempt)
	if err != nil {
		t.Fatal(err)
	}
	want := Timestamp{Year: 2024, Month: Some(uint8(6)), Day: Some(uint8(3))}
	if ts, ok := got.Get(); !ok || ts != want {
		t.Fatalf("got %v", got)
	}
}
