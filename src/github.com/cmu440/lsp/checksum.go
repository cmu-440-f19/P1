// DO NOT MODIFY THIS FILE!

package lsp

import (
	"encoding/binary"
)

// Calculate the 32-bit checksum for a given integer.
func Int2Checksum(value int) uint32 {
	return uint2Checksum(uint32(value))
}

func uint2Checksum(value uint32) uint32 {
	var sum uint32

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, value)
	lower := binary.LittleEndian.Uint16(buf[0:2])
	upper := binary.LittleEndian.Uint16(buf[2:4])
	sum += uint32(lower + upper)

	return sum
}

// Calculate the 32-bit checksum for a given byte array.
func ByteArray2Checksum(value []byte) uint32 {
	var sum uint32

	numChunks := len(value)/2 + len(value)%2 // uint16 occupies 2 bytes

	lastIdx := len(value) - 1
	for i := 0; i < numChunks; i++ {
		startIdx := i * 2
		if startIdx < lastIdx {
			chunk := value[startIdx:(startIdx + 2)]
			tmp := binary.LittleEndian.Uint16(chunk)
			sum += uint32(tmp)
		} else {
			// Pad a zero byte at the end when the byte array has odd number of bytes
			chunk := []byte{value[startIdx], 0}
			tmp := binary.LittleEndian.Uint16(chunk)
			sum += uint32(tmp)
		}
	}

	return sum
}
