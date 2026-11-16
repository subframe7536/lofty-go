package util

// Unsynchsafe32 converts 4 synchsafe bytes (big-endian, 7 bits each) to a u32.
//
// Like Rust's `SynchsafeInteger::unsynch`, the most significant bit of every
// byte is discarded, so the result is always at most 0x0FFFFFFF. This is used
// for ID3v2 tag and frame sizes, which may carry bytes with the high bit set in
// malformed input.
func Unsynchsafe32(b []byte) uint32 {
	return (uint32(b[0]&0x7F) << 21) | (uint32(b[1]&0x7F) << 14) | (uint32(b[2]&0x7F) << 7) | uint32(b[3]&0x7F)
}

// Synchsafe32 converts a u32 to 4 synchsafe bytes (big-endian, 7 bits each).
//
// Precondition: v must be at most 0x0FFFFFFF (28 bits). Rust's `synch()` rejects
// larger values with SynchOverflowError; this function has no error return, so
// bits 28 and above are silently dropped. Production writers check the range
// first (see id3v2 synchsafe32Checked).
func Synchsafe32(v uint32) [4]byte {
	return [4]byte{
		byte(v>>21) & 0x7F,
		byte(v>>14) & 0x7F,
		byte(v>>7) & 0x7F,
		byte(v) & 0x7F,
	}
}
