package alac

import (
	"encoding/binary"
	"fmt"
)

// Config holds ALAC decoder configuration parsed from the magic cookie.
type Config struct {
	FrameLength   uint32
	BitDepth      uint8
	NumChannels   uint8
	PB            uint8
	MB            uint8
	KB            uint8
	MaxRun        uint16
	MaxFrameBytes uint32
	AvgBitRate    uint32
	SampleRate    uint32
}

const configSize = 24

// ParseConfig reads an ALACSpecificConfig from a magic cookie byte slice.
// Handles legacy wrappers ('frma' and 'alac' atoms).
func ParseConfig(cookie []byte) (Config, error) {
	data := cookie

	// Skip 'frma' atom if present: [size:4][type:'frma'][format:'alac']
	if len(data) >= 12 && data[4] == 'f' && data[5] == 'r' && data[6] == 'm' && data[7] == 'a' {
		data = data[12:]
	}

	// Skip 'alac' atom header if present: [size:4][type:'alac'][version:4]
	if len(data) >= 12 && data[4] == 'a' && data[5] == 'l' && data[6] == 'a' && data[7] == 'c' {
		data = data[12:]
	}

	if len(data) < configSize {
		return Config{}, errInvalidCookie
	}

	compatibleVersion := data[4]
	if compatibleVersion > 0 {
		return Config{}, fmt.Errorf("%w: %d", errUnsupportedVersion, compatibleVersion)
	}

	return Config{
		FrameLength:   binary.BigEndian.Uint32(data[0:4]),
		BitDepth:      data[5],
		PB:            data[6],
		MB:            data[7],
		KB:            data[8],
		NumChannels:   data[9],
		MaxRun:        binary.BigEndian.Uint16(data[10:12]),
		MaxFrameBytes: binary.BigEndian.Uint32(data[12:16]),
		AvgBitRate:    binary.BigEndian.Uint32(data[16:20]),
		SampleRate:    binary.BigEndian.Uint32(data[20:24]),
	}, nil
}
