package alac

import "encoding/binary"

// Matrix unmix and output byte formatting.
// Ported from matrix_dec.c.
//
// All output is interleaved little-endian signed PCM.

// --- Stereo unmix (channel pair) ---

func writeStereo16(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 2
	offset := chanIdx * 2

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			pos := offset + idx*stride
			binary.LittleEndian.PutUint16(out[pos:], uint16(int16(left)))
			binary.LittleEndian.PutUint16(out[pos+2:], uint16(int16(right)))
		}
	} else {
		for idx := range numSamples {
			pos := offset + idx*stride
			binary.LittleEndian.PutUint16(out[pos:], uint16(int16(mixU[idx])))
			binary.LittleEndian.PutUint16(out[pos+2:], uint16(int16(mixV[idx])))
		}
	}
}

func writeStereo20(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 3
	offset := chanIdx * 3

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]
			left <<= 4
			right <<= 4

			pos := offset + idx*stride
			out[pos+0] = byte(left)
			out[pos+1] = byte(left >> 8)
			out[pos+2] = byte(left >> 16)
			out[pos+3] = byte(right)
			out[pos+4] = byte(right >> 8)
			out[pos+5] = byte(right >> 16)
		}
	} else {
		for idx := range numSamples {
			pos := offset + idx*stride
			val := mixU[idx] << 4
			out[pos+0] = byte(val)
			out[pos+1] = byte(val >> 8)
			out[pos+2] = byte(val >> 16)

			val = mixV[idx] << 4
			out[pos+3] = byte(val)
			out[pos+4] = byte(val >> 8)
			out[pos+5] = byte(val >> 16)
		}
	}
}

//revive:disable-next-line:argument-limit
func writeStereo24(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 3
	offset := chanIdx * 3
	shift := bytesShifted * 8

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			pos := offset + idx*stride
			out[pos+0] = byte(left)
			out[pos+1] = byte(left >> 8)
			out[pos+2] = byte(left >> 16)
			out[pos+3] = byte(right)
			out[pos+4] = byte(right >> 8)
			out[pos+5] = byte(right >> 16)
		}
	} else {
		for idx := range numSamples {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			pos := offset + idx*stride
			out[pos+0] = byte(left)
			out[pos+1] = byte(left >> 8)
			out[pos+2] = byte(left >> 16)
			out[pos+3] = byte(right)
			out[pos+4] = byte(right >> 8)
			out[pos+5] = byte(right >> 16)
		}
	}
}

//revive:disable-next-line:argument-limit
func writeStereo32(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 4
	offset := chanIdx * 4
	shift := bytesShifted * 8

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			pos := offset + idx*stride
			binary.LittleEndian.PutUint32(out[pos:], uint32(left))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(right))
		}
	} else {
		for idx := range numSamples {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			pos := offset + idx*stride
			binary.LittleEndian.PutUint32(out[pos:], uint32(left))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(right))
		}
	}
}

// --- Mono output (single channel) ---

func writeMono16(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 2
	offset := chanIdx * 2

	for idx := range numSamples {
		pos := offset + idx*stride
		binary.LittleEndian.PutUint16(out[pos:], uint16(int16(mixU[idx])))
	}
}

func writeMono20(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 3
	offset := chanIdx * 3

	for idx := range numSamples {
		pos := offset + idx*stride
		val := mixU[idx] << 4
		out[pos+0] = byte(val)
		out[pos+1] = byte(val >> 8)
		out[pos+2] = byte(val >> 16)
	}
}

func writeMono24(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 3
	offset := chanIdx * 3
	shift := bytesShifted * 8

	for idx := range numSamples {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		pos := offset + idx*stride
		out[pos+0] = byte(val)
		out[pos+1] = byte(val >> 8)
		out[pos+2] = byte(val >> 16)
	}
}

func writeMono32(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 4
	offset := chanIdx * 4
	shift := bytesShifted * 8

	for idx := range numSamples {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		pos := offset + idx*stride
		binary.LittleEndian.PutUint32(out[pos:], uint32(val))
	}
}
