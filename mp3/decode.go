// Package mp3 decodes MP3 audio to raw PCM using a pure-Go decoder.
package mp3

import (
	"errors"
	"fmt"
	"io"

	gomp3 "github.com/hajimehoshi/go-mp3"

	"github.com/farcloser/saprobe"
)

const channels = 2 // go-mp3 always decodes to stereo

// Decode reads an MP3 stream and decodes it to interleaved little-endian signed 16-bit PCM bytes.
// The output is always stereo (2 channels) at the source sample rate.
func Decode(rs io.ReadSeeker) ([]byte, saprobe.PCMFormat, error) {
	decoder, err := gomp3.NewDecoder(rs)
	if err != nil {
		return nil, saprobe.PCMFormat{}, fmt.Errorf("creating mp3 decoder: %w", err)
	}

	format := saprobe.PCMFormat{
		SampleRate: decoder.SampleRate(),
		BitDepth:   saprobe.Depth16,
		Channels:   channels,
	}

	// Pre-allocate output buffer when total length is known.
	var buf []byte
	if length := decoder.Length(); length > 0 {
		buf = make([]byte, 0, length)
	}

	chunk := make([]byte, 32*1024)

	for {
		readN, readErr := decoder.Read(chunk)
		if readN > 0 {
			buf = append(buf, chunk[:readN]...)
		}

		if errors.Is(readErr, io.EOF) {
			break
		}

		if readErr != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("decoding mp3: %w", readErr)
		}
	}

	return buf, format, nil
}
