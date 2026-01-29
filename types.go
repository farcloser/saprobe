package saprobe

import (
	"errors"
	"fmt"
)

// BitDepth represents the bit depth of PCM audio samples.
type BitDepth uint

// Standard PCM bit depths.
const (
	Depth16 BitDepth = 16
	Depth20 BitDepth = 20
	Depth24 BitDepth = 24
	Depth32 BitDepth = 32
)

// BytesPerSample returns the number of bytes needed to store one sample.
// 20-bit samples are stored in 3 bytes (left-aligned in 24-bit).
func (d BitDepth) BytesPerSample() int {
	switch d {
	case Depth16:
		return 2
	case Depth20, Depth24:
		return 3
	case Depth32:
		return 4
	default:
		panic(fmt.Sprintf("saprobe: BytesPerSample called with unsupported bit depth %d", d))
	}
}

// PCMFormat describes the format of raw PCM audio data.
type PCMFormat struct {
	SampleRate int
	BitDepth   BitDepth
	Channels   uint
}

var errUnsupportedBitDepth = errors.New("unsupported bit depth")

// ToBitDepth converts a numeric bit depth to the BitDepth type.
func ToBitDepth(bps uint8) (BitDepth, error) {
	switch BitDepth(bps) {
	case Depth16:
		return Depth16, nil
	case Depth20:
		return Depth20, nil
	case Depth24:
		return Depth24, nil
	case Depth32:
		return Depth32, nil
	default:
		return 0, fmt.Errorf("%d-bit: %w", bps, errUnsupportedBitDepth)
	}
}
