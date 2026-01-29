package alac

import "encoding/binary"

// Matrix unmix and output byte formatting.
// Ported from matrix_dec.c.
//
// All output is interleaved little-endian signed PCM.

// --- Stereo unmix (channel pair) ---

func writeStereo16(out []byte, u, v []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 2
	offset := chanIdx * 2

	if mixRes != 0 {
		for i := range numSamples {
			l := u[i] + v[i] - ((mixRes * v[i]) >> mixBits)
			r := l - v[i]

			pos := offset + i*stride
			binary.LittleEndian.PutUint16(out[pos:], uint16(int16(l)))
			binary.LittleEndian.PutUint16(out[pos+2:], uint16(int16(r)))
		}
	} else {
		for i := range numSamples {
			pos := offset + i*stride
			binary.LittleEndian.PutUint16(out[pos:], uint16(int16(u[i])))
			binary.LittleEndian.PutUint16(out[pos+2:], uint16(int16(v[i])))
		}
	}
}

func writeStereo20(out []byte, u, v []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 3
	offset := chanIdx * 3

	if mixRes != 0 {
		for i := range numSamples {
			l := u[i] + v[i] - ((mixRes * v[i]) >> mixBits)
			r := l - v[i]
			l <<= 4
			r <<= 4

			pos := offset + i*stride
			out[pos+0] = byte(l)
			out[pos+1] = byte(l >> 8)
			out[pos+2] = byte(l >> 16)
			out[pos+3] = byte(r)
			out[pos+4] = byte(r >> 8)
			out[pos+5] = byte(r >> 16)
		}
	} else {
		for i := range numSamples {
			pos := offset + i*stride
			val := u[i] << 4
			out[pos+0] = byte(val)
			out[pos+1] = byte(val >> 8)
			out[pos+2] = byte(val >> 16)

			val = v[i] << 4
			out[pos+3] = byte(val)
			out[pos+4] = byte(val >> 8)
			out[pos+5] = byte(val >> 16)
		}
	}
}

func writeStereo24(out []byte, u, v []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 3
	offset := chanIdx * 3
	shift := bytesShifted * 8

	if mixRes != 0 {
		for i := range numSamples {
			l := u[i] + v[i] - ((mixRes * v[i]) >> mixBits)
			r := l - v[i]

			if bytesShifted != 0 {
				l = (l << shift) | int32(shiftBuf[i*2+0])
				r = (r << shift) | int32(shiftBuf[i*2+1])
			}

			pos := offset + i*stride
			out[pos+0] = byte(l)
			out[pos+1] = byte(l >> 8)
			out[pos+2] = byte(l >> 16)
			out[pos+3] = byte(r)
			out[pos+4] = byte(r >> 8)
			out[pos+5] = byte(r >> 16)
		}
	} else {
		for i := range numSamples {
			l := u[i]
			r := v[i]

			if bytesShifted != 0 {
				l = (l << shift) | int32(shiftBuf[i*2+0])
				r = (r << shift) | int32(shiftBuf[i*2+1])
			}

			pos := offset + i*stride
			out[pos+0] = byte(l)
			out[pos+1] = byte(l >> 8)
			out[pos+2] = byte(l >> 16)
			out[pos+3] = byte(r)
			out[pos+4] = byte(r >> 8)
			out[pos+5] = byte(r >> 16)
		}
	}
}

func writeStereo32(out []byte, u, v []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 4
	offset := chanIdx * 4
	shift := bytesShifted * 8

	if mixRes != 0 {
		for i := range numSamples {
			l := u[i] + v[i] - ((mixRes * v[i]) >> mixBits)
			r := l - v[i]

			if bytesShifted != 0 {
				l = (l << shift) | int32(shiftBuf[i*2+0])
				r = (r << shift) | int32(shiftBuf[i*2+1])
			}

			pos := offset + i*stride
			binary.LittleEndian.PutUint32(out[pos:], uint32(l))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(r))
		}
	} else {
		for i := range numSamples {
			l := u[i]
			r := v[i]

			if bytesShifted != 0 {
				l = (l << shift) | int32(shiftBuf[i*2+0])
				r = (r << shift) | int32(shiftBuf[i*2+1])
			}

			pos := offset + i*stride
			binary.LittleEndian.PutUint32(out[pos:], uint32(l))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(r))
		}
	}
}

// --- Mono output (single channel) ---

func writeMono16(out []byte, u []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 2
	offset := chanIdx * 2

	for i := range numSamples {
		pos := offset + i*stride
		binary.LittleEndian.PutUint16(out[pos:], uint16(int16(u[i])))
	}
}

func writeMono20(out []byte, u []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 3
	offset := chanIdx * 3

	for i := range numSamples {
		pos := offset + i*stride
		val := u[i] << 4
		out[pos+0] = byte(val)
		out[pos+1] = byte(val >> 8)
		out[pos+2] = byte(val >> 16)
	}
}

func writeMono24(out []byte, u []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 3
	offset := chanIdx * 3
	shift := bytesShifted * 8

	for i := range numSamples {
		val := u[i]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[i])
		}

		pos := offset + i*stride
		out[pos+0] = byte(val)
		out[pos+1] = byte(val >> 8)
		out[pos+2] = byte(val >> 16)
	}
}

func writeMono32(out []byte, u []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 4
	offset := chanIdx * 4
	shift := bytesShifted * 8

	for i := range numSamples {
		val := u[i]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[i])
		}

		pos := offset + i*stride
		binary.LittleEndian.PutUint32(out[pos:], uint32(val))
	}
}
