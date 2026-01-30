package flac

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	goflac "github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"

	"github.com/farcloser/saprobe"
)

var errBitDepth = errors.New("unsupported bit depth")

// Decode reads a FLAC stream and decodes it to interleaved little-endian signed PCM bytes.
// Native bit depth is preserved (16-bit FLAC produces s16le, 24-bit produces s24le, etc.).
func Decode(rs io.ReadSeeker) ([]byte, saprobe.PCMFormat, error) {
	stream, err := goflac.New(rs)
	if err != nil {
		return nil, saprobe.PCMFormat{}, fmt.Errorf("opening flac: %w", err)
	}
	defer stream.Close()

	info := stream.Info
	nChannels := int(info.NChannels)

	bitDepth, err := saprobe.ToBitDepth(info.BitsPerSample)
	if err != nil {
		return nil, saprobe.PCMFormat{}, fmt.Errorf("%w: %w", errBitDepth, err)
	}

	bytesPerSample := bitDepth.BytesPerSample()

	format := saprobe.PCMFormat{
		SampleRate: int(info.SampleRate),
		BitDepth:   bitDepth,
		Channels:   uint(nChannels), //nolint:gosec // nChannels comes from uint8, always fits in uint.
	}

	// Pre-allocate output buffer when total sample count is known.
	var buf []byte
	if info.NSamples > 0 {
		//nolint:gosec // NSamples (uint64) fits in int for any real audio file.
		buf = make([]byte, 0, int(info.NSamples)*nChannels*bytesPerSample)
	}

	// Scratch buffer for one interleaved frame (reused across iterations).
	var scratch []byte

	for {
		audioFrame, parseErr := stream.ParseNext()
		if errors.Is(parseErr, io.EOF) {
			break
		}

		if parseErr != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("decoding frame: %w", parseErr)
		}

		blockSize := int(audioFrame.BlockSize)
		frameBytes := blockSize * nChannels * bytesPerSample

		if cap(scratch) < frameBytes {
			scratch = make([]byte, frameBytes)
		} else {
			scratch = scratch[:frameBytes]
		}

		interleave(scratch, audioFrame.Subframes, blockSize, nChannels, bitDepth)
		buf = append(buf, scratch...)
	}

	return buf, format, nil
}

// interleave writes decoded subframe samples into dst as interleaved little-endian signed PCM.
func interleave(dst []byte, subframes []*frame.Subframe, blockSize, nChannels int, depth saprobe.BitDepth) {
	pos := 0

	switch depth {
	case saprobe.Depth8:
		for i := range blockSize {
			for ch := range nChannels {
				dst[pos] = byte(int8(subframes[ch].Samples[i])) //nolint:gosec // Intentional int32-to-int8 truncation.
				pos++
			}
		}
	case saprobe.Depth16:
		for i := range blockSize {
			for ch := range nChannels {
				binary.LittleEndian.PutUint16(
					dst[pos:],
					uint16(int16(subframes[ch].Samples[i])), //nolint:gosec // Intentional int32-to-int16 truncation.
				)
				pos += 2
			}
		}
	case saprobe.Depth24:
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
				binary.LittleEndian.PutUint32(
					dst[pos:],
					uint32(subframes[ch].Samples[i]), //nolint:gosec // int32-to-uint32 reinterpretation.
				)
				pos += 4
			}
		}
	case saprobe.Depth20:
		fallthrough
	default:
		panic(fmt.Sprintf("flac: interleave called with unsupported bit depth %d", depth))
	}
}
