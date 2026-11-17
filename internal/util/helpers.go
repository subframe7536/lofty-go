package util

import (
	"bytes"
	"strconv"
)

// --- small helpers ---

func Itoa(n int) string { return strconv.Itoa(n) }

func Atoi(s string) (int, error) { return strconv.Atoi(s) }

func IndexOfStr(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func OptionFromPtr[T any](p *T) Option[*T] {
	if p == nil {
		return None[*T]()
	}
	return Some(p)
}

// U32FromString parses a u32 like Rust's `str::parse::<u32>()`.
//
// Exactly one leading '+' is accepted. A '-' is not a sign for an unsigned
// type, so "-5" and "-0" are rejected. Whitespace, an empty string, a lone "+",
// and values above u32::MAX are rejected.
func U32FromString(s string) (uint32, bool) {
	digits := s
	if len(digits) > 0 && digits[0] == '+' {
		digits = digits[1:]
	}
	n, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// FlagItem normalizes a flag item value ("0"/"1"/"true"/"false"), -1 if invalid.
//
// Like Rust's `flag_item`, the match is exact: no trimming and no case folding.
func FlagItem(s string) int {
	switch s {
	case "1", "true":
		return 1
	case "0", "false":
		return 0
	}
	return -1
}

// AdvisoryRatingFromU8 normalizes a raw rtng byte per Rust's
// `AdvisoryRating::try_from(u8)` + `as_u8` (0->0, 1|4->1, 2->2; invalid -> !ok).
func AdvisoryRatingFromU8(v uint8) (uint8, bool) {
	switch v {
	case 0:
		return 0, true
	case 1, 4:
		return 1, true
	case 2:
		return 2, true
	default:
		return 0, false
	}
}

func SplitNulls(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func JoinNulls(parts []string) string {
	var b bytes.Buffer
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(p)
	}
	return b.String()
}
