package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/mycophonic/saprobe"
)

// WAV format constants.
const (
	wavFormatPCM        = 1
	wavFormatIEEEFloat  = 3
	wavFormatExtensible = 0xFFFE
)

// GUID for PCM in WAVEFORMATEXTENSIBLE.
var wavGUIDPCM = [16]byte{
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00,
	0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71,
}

var (
	ErrNotWAV          = errors.New("not a WAV file")
	ErrUnsupportedFmt  = errors.New("unsupported WAV format")
	ErrNoFmtChunk      = errors.New("missing fmt chunk")
	ErrNoDataChunk     = errors.New("missing data chunk")
	ErrInvalidBitDepth = errors.New("invalid bit depth")
)

// Decode reads a WAV file and returns raw PCM samples.
func Decode(rs io.ReadSeeker) ([]byte, saprobe.PCMFormat, error) {
	var format saprobe.PCMFormat

	// Read RIFF header
	var riffHeader [12]byte
	if _, err := io.ReadFull(rs, riffHeader[:]); err != nil {
		return nil, format, fmt.Errorf("reading RIFF header: %w", err)
	}

	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, format, ErrNotWAV
	}

	// Parse chunks
	var pcmData []byte

	fmtFound := false

	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(rs, chunkHeader[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, format, fmt.Errorf("reading chunk header: %w", err)
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			if err := parseFmtChunk(rs, chunkSize, &format); err != nil {
				return nil, format, err
			}

			fmtFound = true

		case "data":
			pcmData = make([]byte, chunkSize)
			if _, err := io.ReadFull(rs, pcmData); err != nil {
				return nil, format, fmt.Errorf("reading PCM data: %w", err)
			}

		default:
			// Skip unknown chunks
			if _, err := rs.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return nil, format, fmt.Errorf("skipping chunk %s: %w", chunkID, err)
			}
		}

		// Chunks are word-aligned (pad byte if odd size)
		if chunkSize%2 == 1 {
			if _, err := rs.Seek(1, io.SeekCurrent); err != nil {
				return nil, format, fmt.Errorf("seeking past pad byte: %w", err)
			}
		}
	}

	if !fmtFound {
		return nil, format, ErrNoFmtChunk
	}

	if pcmData == nil {
		return nil, format, ErrNoDataChunk
	}

	return pcmData, format, nil
}

func parseFmtChunk(rs io.ReadSeeker, size uint32, format *saprobe.PCMFormat) error {
	if size < 16 {
		return ErrUnsupportedFmt
	}

	var buf [40]byte // Max size for WAVEFORMATEXTENSIBLE

	toRead := min(size, 40)

	if _, err := io.ReadFull(rs, buf[:toRead]); err != nil {
		return fmt.Errorf("reading fmt chunk: %w", err)
	}

	// Skip remaining bytes if chunk is larger than what we consumed.
	if size > 40 {
		if _, err := rs.Seek(int64(size-40), io.SeekCurrent); err != nil {
			return fmt.Errorf("skipping fmt chunk tail: %w", err)
		}
	}

	audioFormat := binary.LittleEndian.Uint16(buf[0:2])
	channels := binary.LittleEndian.Uint16(buf[2:4])
	sampleRate := binary.LittleEndian.Uint32(buf[4:8])
	// byteRate at buf[8:12] - not needed
	// blockAlign at buf[12:14] - not needed
	bitsPerSample := binary.LittleEndian.Uint16(buf[14:16])

	switch audioFormat {
	case wavFormatPCM:
		// Standard PCM, we're good

	case wavFormatExtensible:
		if size < 40 {
			return ErrUnsupportedFmt
		}
		// validBitsPerSample at buf[18:20]
		// channelMask at buf[20:24]
		// SubFormat GUID at buf[24:40]
		var subFormat [16]byte
		copy(subFormat[:], buf[24:40])

		if subFormat != wavGUIDPCM {
			return ErrUnsupportedFmt // Not PCM (could be float, etc.)
		}

	case wavFormatIEEEFloat:
		return ErrUnsupportedFmt // Could support, but you asked for PCM

	default:
		return ErrUnsupportedFmt
	}

	format.SampleRate = int(sampleRate)
	format.Channels = uint(channels)

	switch bitsPerSample {
	case 16, 24, 32:
		format.BitDepth = saprobe.BitDepth(bitsPerSample)
	default:
		return fmt.Errorf("%w: %d", ErrInvalidBitDepth, bitsPerSample)
	}

	return nil
}

// Encode writes PCM samples as a WAV file.
func Encode(w io.Writer, pcm []byte, format saprobe.PCMFormat) error {
	switch format.BitDepth {
	case 16, 24, 32:
		// Valid
	default:
		return fmt.Errorf("%w: %d (must be 16, 24, or 32)", ErrInvalidBitDepth, format.BitDepth)
	}

	channels := uint16(format.Channels)
	sampleRate := uint32(format.SampleRate)
	bitsPerSample := uint16(format.BitDepth)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := uint32(len(pcm))

	// Use WAVEFORMATEXTENSIBLE for >2 channels or >16 bits
	useExtensible := channels > 2 || bitsPerSample > 16

	if useExtensible {
		return writeWAVExtensible(w, pcm, channels, sampleRate, bitsPerSample, byteRate, blockAlign, dataSize)
	}

	return writeWAVSimple(w, pcm, channels, sampleRate, bitsPerSample, byteRate, blockAlign, dataSize)
}

func writeWAVSimple(
	w io.Writer,
	pcm []byte,
	channels uint16,
	sampleRate uint32,
	bitsPerSample uint16,
	byteRate uint32,
	blockAlign uint16,
	dataSize uint32,
) error {
	var header [44]byte

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], dataSize+36)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(header[20:22], wavFormatPCM)
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("writing WAV header: %w", err)
	}

	if _, err := w.Write(pcm); err != nil {
		return fmt.Errorf("writing PCM data: %w", err)
	}

	return nil
}

func writeWAVExtensible(
	w io.Writer,
	pcm []byte,
	channels uint16,
	sampleRate uint32,
	bitsPerSample uint16,
	byteRate uint32,
	blockAlign uint16,
	dataSize uint32,
) error {
	// WAVEFORMATEXTENSIBLE: fmt chunk is 40 bytes instead of 16
	fmtChunkSize := uint32(40)
	headerSize := 12 + 8 + fmtChunkSize + 8 // RIFF + fmt header + fmt data + data header
	fileSize := headerSize + dataSize - 8   // -8 for RIFF header not counted

	var header [68]byte // 12 + 8 + 40 + 8

	// RIFF header
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(fileSize))
	copy(header[8:12], "WAVE")

	// fmt chunk header
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], fmtChunkSize)

	// WAVEFORMATEX part
	binary.LittleEndian.PutUint16(header[20:22], wavFormatExtensible)
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	binary.LittleEndian.PutUint16(header[36:38], 22) // cbSize: extra bytes after WAVEFORMATEX

	// WAVEFORMATEXTENSIBLE extension
	binary.LittleEndian.PutUint16(header[38:40], bitsPerSample) // validBitsPerSample
	binary.LittleEndian.PutUint32(header[40:44], channelMask(channels))
	copy(header[44:60], wavGUIDPCM[:])

	// data chunk header
	copy(header[60:64], "data")
	binary.LittleEndian.PutUint32(header[64:68], dataSize)

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("writing WAV header: %w", err)
	}

	if _, err := w.Write(pcm); err != nil {
		return fmt.Errorf("writing PCM data: %w", err)
	}

	return nil
}

// channelMask returns standard channel mask for common configurations.
func channelMask(channels uint16) uint32 {
	switch channels {
	case 1:
		return 0x4 // FC
	case 2:
		return 0x3 // FL | FR
	case 4:
		return 0x33 // FL | FR | BL | BR
	case 6:
		return 0x3F // FL | FR | FC | LFE | BL | BR (5.1)
	case 8:
		return 0x63F // 7.1
	default:
		return 0 // Unspecified
	}
}
