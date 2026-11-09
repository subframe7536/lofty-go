package util

// unsynchsafe32 converts 4 synchsafe bytes (7 bits each) to a u32.
// Unsynchsafe32 converts 4 synchsafe bytes (7 bits each) to a u32.
func Unsynchsafe32(b []byte) uint32 {
	return (uint32(b[0]) << 21) | (uint32(b[1]) << 14) | (uint32(b[2]) << 7) | uint32(b[3])
}

// synchsafe32 converts a u32 to 4 synchsafe bytes.
// Synchsafe32 converts a u32 to 4 synchsafe bytes.
func Synchsafe32(v uint32) [4]byte {
	return [4]byte{
		byte(v>>21) & 0x7F,
		byte(v>>14) & 0x7F,
		byte(v>>7) & 0x7F,
		byte(v) & 0x7F,
	}
}
