package alac

import "math/bits"

// Adaptive Golomb-Rice entropy decoder.
// Ported from ag_dec.c and aglib.h.

const (
	qbShift    = 9
	qb         = 1 << qbShift // 512
	mmulShift  = 2
	mdenShift  = qbShift - mmulShift - 1 // 6
	moff       = 1 << (mdenShift - 2)    // 16
	bitoff     = 24
	maxPrefix16    = 9
	maxPrefix32    = 9
	maxDatatype16  = 16
	nMaxMeanClamp  = 0xffff
	nMeanClampVal  = 0xffff
)

type agParams struct {
	mb, mb0 uint32
	pb      uint32
	kb      uint32
	wb      uint32
	qb      uint32
	fw, sw  uint32
	maxrun  uint32
}

func setAGParams(params *agParams, m, p, k, f, s, maxrun uint32) {
	params.mb = m
	params.mb0 = m
	params.pb = p
	params.kb = k
	params.wb = (1 << k) - 1
	params.qb = qb - p
	params.fw = f
	params.sw = s
	params.maxrun = maxrun
}

// lead returns the number of leading zeros in a 32-bit value.
// Equivalent to the Apple lead() function.
func lead(m int32) int32 {
	if m == 0 {
		return 32
	}

	return int32(bits.LeadingZeros32(uint32(m)))
}

// lg3a computes floor(log2(x+3)).
func lg3a(x int32) int32 {
	return 31 - lead(x+3)
}

// read32bit reads 4 bytes big-endian from a byte slice at the given offset.
func read32bit(buf []byte, offset int) uint32 {
	return uint32(buf[offset])<<24 | uint32(buf[offset+1])<<16 |
		uint32(buf[offset+2])<<8 | uint32(buf[offset+3])
}

// getStreamBits reads up to 32 bits from an arbitrary bit position in a byte buffer.
func getStreamBits(in []byte, bitOffset, numBits uint32) uint32 {
	byteOffset := bitOffset / 8
	load1 := read32bit(in, int(byteOffset))

	if numBits+(bitOffset&7) > 32 {
		// Need bits from a 5th byte.
		result := load1 << (bitOffset & 7)
		load2 := uint32(in[byteOffset+4])
		load2shift := 8 - (numBits + (bitOffset & 7) - 32)
		load2 >>= load2shift
		result >>= 32 - numBits
		result |= load2

		return result
	}

	result := load1 >> (32 - numBits - (bitOffset & 7))
	if numBits < 32 {
		result &= (1 << numBits) - 1
	}

	return result
}

// dynGet decodes one Golomb-coded value (16-bit variant used for zero-run counts).
func dynGet(in []byte, bitPos *uint32, m, k uint32) uint32 {
	tempBits := *bitPos

	streamLong := read32bit(in, int(tempBits>>3))
	streamLong <<= tempBits & 7

	// Count leading ones (= leading zeros in complement).
	pre := uint32(lead(int32(^streamLong)))

	if pre >= maxPrefix16 {
		pre = maxPrefix16
		tempBits += pre
		streamLong <<= pre
		result := streamLong >> (32 - maxDatatype16)
		tempBits += maxDatatype16
		*bitPos = tempBits

		return result
	}

	tempBits += pre + 1
	streamLong <<= pre + 1
	v := streamLong >> (32 - k)
	tempBits += k

	var result uint32
	if v < 2 {
		result = pre * m
		tempBits--
	} else {
		result = pre*m + v - 1
	}

	*bitPos = tempBits

	return result
}

// dynGet32Bit decodes one Golomb-coded value (32-bit variant used for sample residuals).
func dynGet32Bit(in []byte, bitPos *uint32, m uint32, k int32, maxBits int32) uint32 {
	tempBits := *bitPos

	streamLong := read32bit(in, int(tempBits>>3))
	streamLong <<= tempBits & 7

	// Count leading ones.
	result := uint32(lead(int32(^streamLong)))

	if result >= maxPrefix32 {
		result = getStreamBits(in, tempBits+maxPrefix32, uint32(maxBits))
		tempBits += maxPrefix32 + uint32(maxBits)
	} else {
		tempBits += result + 1

		if k != 1 {
			streamLong <<= result + 1
			v := streamLong >> (32 - uint32(k))

			if v >= 2 {
				result = result*m + v - 1
				tempBits += uint32(k)
			} else {
				result = result * m
				tempBits += uint32(k) - 1
			}
		}
	}

	*bitPos = tempBits

	return result
}

// dynDecomp performs adaptive Golomb-Rice entropy decoding of a sample block.
// Writes decoded prediction residuals into pc.
func dynDecomp(params *agParams, bb *bitBuffer, pc []int32, numSamples, maxSize int) error {
	in := bb.buf[bb.pos:]
	startPos := bb.bitIdx
	maxPos := uint32(bb.size) * 8
	bitPos := startPos

	mb := params.mb0
	zmode := int32(0)
	c := 0

	pbLocal := params.pb
	kbLocal := params.kb
	wbLocal := params.wb

	for c < numSamples {
		if bitPos >= maxPos {
			return errBitstreamOverrun
		}

		m := mb >> qbShift
		k := lg3a(int32(m))

		if k > int32(kbLocal) {
			k = int32(kbLocal)
		}

		m = (1 << uint32(k)) - 1

		n := dynGet32Bit(in, &bitPos, m, k, int32(maxSize))

		// Decode sign from LSB.
		ndecode := n + uint32(zmode)
		multiplier := -int32(ndecode & 1)
		multiplier |= 1
		del := int32((ndecode+1)>>1) * multiplier

		pc[c] = del
		c++

		// Update mean.
		mb = pbLocal*(n+uint32(zmode)) + mb - ((pbLocal * mb) >> qbShift)
		if n > nMaxMeanClamp {
			mb = nMeanClampVal
		}

		zmode = 0

		// Check for zero run mode.
		if (mb<<mmulShift) < qb && c < numSamples {
			zmode = 1
			k32 := lead(int32(mb)) - bitoff + int32((mb+moff)>>mdenShift)
			if k32 < 0 {
				k32 = 0
			}

			mz := ((uint32(1) << uint32(k32)) - 1) & wbLocal

			n = dynGet(in, &bitPos, mz, uint32(k32))

			if c+int(n) > numSamples {
				return errSampleOverrun
			}

			for j := uint32(0); j < n; j++ {
				pc[c] = 0
				c++
			}

			if n >= 65535 {
				zmode = 0
			}

			mb = 0
		}
	}

	bitsConsumed := bitPos - startPos
	bb.advance(bitsConsumed)

	return nil
}
