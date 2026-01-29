package detect

import (
	"fmt"
	"io"
)

// Codec represents a recognized audio codec.
type Codec uint8

const (
	// Unknown indicates the file format was not recognized.
	Unknown Codec = iota
	// FLAC is the Free Lossless Audio Codec.
	FLAC
	// ALAC is the Apple Lossless Audio Codec (inside an M4A/MP4 container).
	ALAC
	// MP3 is MPEG-1/2 Audio Layer III.
	MP3
	// Vorbis is Ogg Vorbis.
	Vorbis
)

// String returns the human-readable name of the codec.
func (c Codec) String() string {
	switch c {
	case Unknown:
		return "unknown"
	case FLAC:
		return "FLAC"
	case ALAC:
		return "ALAC"
	case MP3:
		return "MP3"
	case Vorbis:
		return "Vorbis"
	}

	return "unknown"
}

// headerSize is the minimum number of bytes needed to identify any supported codec.
// FLAC: 4 bytes at offset 0 ("fLaC").
// ALAC: 4 bytes at offset 4 ("ftyp" in an M4A/MP4 container).
// MP3:  3 bytes at offset 0 ("ID3") or 2-byte MPEG sync word (0xFF 0xE0 mask).
// OGG:  4 bytes at offset 0 ("OggS").
const (
	headerSize = 8

	// mpegSyncByte is the first byte of an MPEG audio frame sync word.
	mpegSyncByte = 0xFF
	// mpegSyncMask masks the upper 3 bits of the second byte in the sync word.
	mpegSyncMask = 0xE0
)

// Identify reads the header from rs and returns the detected audio codec.
// The reader position is reset to the start before returning.
func Identify(reader io.ReadSeeker) (Codec, error) {
	var header [headerSize]byte

	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Unknown, fmt.Errorf("reading header: %w", err)
	}

	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return Unknown, fmt.Errorf("seeking to start: %w", err)
	}

	// FLAC: first four bytes are "fLaC".
	if string(header[:4]) == "fLaC" {
		return FLAC, nil
	}

	// Ogg container (Vorbis): first four bytes are "OggS".
	if string(header[:4]) == "OggS" {
		return Vorbis, nil
	}

	// M4A/MP4 container (ALAC): bytes 4-7 are "ftyp".
	if string(header[4:8]) == "ftyp" {
		return ALAC, nil
	}

	// MP3: ID3v2 tag header starts with "ID3".
	if string(header[:3]) == "ID3" {
		return MP3, nil
	}

	// MP3: MPEG frame sync word (11 set bits).
	//nolint:gosec // header is a fixed [8]byte array
	if header[0] == mpegSyncByte && header[1]&mpegSyncMask == mpegSyncMask {
		return MP3, nil
	}

	return Unknown, nil
}
