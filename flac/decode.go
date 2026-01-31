package flac

import (
	"errors"
	"fmt"
	"io"
	"slices"

	goflac "github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"

	"github.com/mycophonic/primordium/fault"

	"github.com/mycophonic/saprobe"
)

//nolint:gochecknoglobals
var flacBitDepths = []saprobe.BitDepth{
	saprobe.Depth4,
	saprobe.Depth8,
	saprobe.Depth12,
	saprobe.Depth16,
	saprobe.Depth20,
	saprobe.Depth24,
	saprobe.Depth32,
}

// ErrBitDepth is returned when a FLAC stream has an unsupported bit depth.
var ErrBitDepth = errors.New("unsupported bit depth")

// Decode reads a FLAC stream and decodes it to interleaved little-endian signed PCM bytes.
// Native bit depth is preserved (16-bit FLAC produces s16le, 24-bit produces s24le, etc.).
func Decode(rs io.ReadSeeker) ([]byte, saprobe.PCMFormat, error) {
	stream, err := goflac.New(rs)
	if err != nil {
		return nil, saprobe.PCMFormat{}, fmt.Errorf("%w: %w", fault.ErrReadFailure, err)
	}
	defer stream.Close()

	info := stream.Info
	nChannels := int(info.NChannels)

	bitDepth := saprobe.BitDepth(info.BitsPerSample)
	if !slices.Contains(flacBitDepths, bitDepth) {
		return nil, saprobe.PCMFormat{}, ErrBitDepth
	}

	bytesPerSample := bitDepth.BytesPerSample()

	format := saprobe.PCMFormat{
		SampleRate: int(info.SampleRate),
		BitDepth:   bitDepth,
		Channels:   uint(nChannels), //nolint:gosec // nChannels comes from uint8, always fits in uint.
	}

	// Pre-allocate output buffer when total sample count is known.
	var buf []byte

	var offset int

	knownSize := info.NSamples > 0
	if knownSize {
		buf = make([]byte, int(info.NSamples)*nChannels*bytesPerSample)
	}

	// Scratch buffer for one interleaved frame (reused when total size is unknown).
	var scratch []byte

	for {
		audioFrame, parseErr := stream.ParseNext()
		if errors.Is(parseErr, io.EOF) {
			break
		}

		if parseErr != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("%w: %w", fault.ErrReadFailure, parseErr)
		}

		blockSize := int(audioFrame.BlockSize)
		frameBytes := blockSize * nChannels * bytesPerSample

		if knownSize {
			// Write directly into the pre-allocated output buffer.
			interleave(buf[offset:offset+frameBytes], audioFrame.Subframes, blockSize, nChannels, bitDepth)
			offset += frameBytes
		} else {
			// Unknown total size: use scratch buffer and append.
			if cap(scratch) < frameBytes {
				scratch = make([]byte, frameBytes)
			} else {
				scratch = scratch[:frameBytes]
			}

			interleave(scratch, audioFrame.Subframes, blockSize, nChannels, bitDepth)
			buf = append(buf, scratch...) //nolint:makezero // Only reached when knownSize is false (buf is nil).
		}
	}

	return buf, format, nil
}

// interleave writes decoded subframe samples into dst as interleaved little-endian signed PCM.
func interleave(dst []byte, subframes []*frame.Subframe, blockSize, nChannels int, depth saprobe.BitDepth) {
	pos := 0

	switch depth {
	case saprobe.Depth4, saprobe.Depth8:
		// 4-bit sign-extended to 8-bit, 8-bit native. Both stored as 1 byte.
		for i := range blockSize {
			for ch := range nChannels {
				dst[pos] = byte(int8(subframes[ch].Samples[i])) //nolint:gosec // Intentional int32-to-int8 truncation.
				pos++
			}
		}
	case saprobe.Depth12, saprobe.Depth16:
		// 12-bit sign-extended to 16-bit, 16-bit native. Both stored as 2 bytes LE.
		if nChannels == 2 {
			left := subframes[0].Samples
			right := subframes[1].Samples

			for i := range blockSize {
				l := left[i]
				r := right[i]
				dst[pos] = byte(l)
				dst[pos+1] = byte(l >> 8)
				dst[pos+2] = byte(r)
				dst[pos+3] = byte(r >> 8)
				pos += 4
			}
		} else {
			for i := range blockSize {
				for ch := range nChannels {
					s := subframes[ch].Samples[i]
					dst[pos] = byte(s)
					dst[pos+1] = byte(s >> 8)
					pos += 2
				}
			}
		}
	case saprobe.Depth20, saprobe.Depth24:
		// 20-bit sign-extended to 24-bit, 24-bit native. Both stored as 3 bytes LE.
		for i := range blockSize {
			for ch := range nChannels {
				s := subframes[ch].Samples[i]
				dst[pos] = byte(s)
				dst[pos+1] = byte(s >> 8)
				dst[pos+2] = byte(s >> 16)
				pos += 3
			}
		}
	case saprobe.Depth32:
		for i := range blockSize {
			for ch := range nChannels {
				s := subframes[ch].Samples[i]
				dst[pos] = byte(s)
				dst[pos+1] = byte(s >> 8)
				dst[pos+2] = byte(s >> 16)
				dst[pos+3] = byte(s >> 24)
				pos += 4
			}
		}
	default:
		panic(fmt.Sprintf("flac: interleave called with unsupported bit depth %d", depth))
	}
}
