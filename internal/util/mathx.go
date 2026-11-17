package util

import (
	"math"
	"math/bits"
	"time"
)

// DurationFromMillis converts milliseconds to a Duration, saturating at the
// maximum representable Duration (Rust's Duration is u64 seconds + nanos).
func DurationFromMillis(ms uint64) time.Duration {
	const maxMs = uint64(math.MaxInt64 / int64(time.Millisecond))
	if ms > maxMs {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(ms) * time.Millisecond
}

// SaturatingMul returns a*b, clamped to math.MaxUint64 on overflow.
func SaturatingMul(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {
		return math.MaxUint64
	}
	return lo
}

// MulDiv computes a*b/c using a 128-bit intermediate (Rust `u128` math),
// saturating at math.MaxUint64. A zero divisor yields 0.
func MulDiv(a, b, c uint64) uint64 {
	if c == 0 {
		return 0
	}
	hi, lo := bits.Mul64(a, b)
	if hi >= c {
		return math.MaxUint64
	}
	q, _ := bits.Div64(hi, lo, c)
	return q
}

// MulDivRound computes round(a*b/c) like [DivRound] but with a 128-bit
// intermediate, saturating at math.MaxUint64. A zero divisor yields 0.
func MulDivRound(a, b, c uint64) uint64 {
	if c == 0 {
		return 0
	}
	hi, lo := bits.Mul64(a, b)
	lo, carry := bits.Add64(lo, c>>1, 0)
	hi += carry
	if hi >= c {
		return math.MaxUint64
	}
	q, _ := bits.Div64(hi, lo, c)
	return q
}

// F64ToU32 converts like Rust's saturating `as u32` (NaN -> 0).
func F64ToU32(f float64) uint32 {
	switch {
	case f != f, f <= 0:
		return 0
	case f >= math.MaxUint32:
		return math.MaxUint32
	}
	return uint32(f)
}
