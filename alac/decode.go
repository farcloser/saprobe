package alac

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	mp4 "github.com/abema/go-mp4"

	"github.com/farcloser/saprobe"
)

// Decode reads an M4A/MP4 stream and decodes the first ALAC audio track
// to interleaved little-endian signed PCM bytes.
func Decode(rs io.ReadSeeker) ([]byte, saprobe.PCMFormat, error) {
	cookie, samples, err := findALACTrack(rs)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}

	config, err := ParseConfig(cookie)
	if err != nil {
		return nil, saprobe.PCMFormat{}, fmt.Errorf("parsing ALAC config: %w", err)
	}

	dec, err := NewDecoder(config)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}

	format := dec.Format()
	bps := format.BitDepth.BytesPerSample()

	// Rough capacity estimate (last frame may be shorter).
	pcm := make([]byte, 0, len(samples)*int(config.FrameLength)*int(config.NumChannels)*bps)

	var packetBuf []byte

	for i, s := range samples {
		if int(s.size) > len(packetBuf) {
			packetBuf = make([]byte, s.size)
		}

		packet := packetBuf[:s.size]

		if _, err := rs.Seek(int64(s.offset), io.SeekStart); err != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("seeking to sample %d at offset %d: %w", i, s.offset, err)
		}

		if _, err := io.ReadFull(rs, packet); err != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("reading sample %d: %w", i, err)
		}

		decoded, err := dec.DecodePacket(packet)
		if err != nil {
			return nil, saprobe.PCMFormat{}, fmt.Errorf("decoding packet %d: %w", i, err)
		}

		pcm = append(pcm, decoded...)
	}

	return pcm, format, nil
}

// sampleInfo holds the byte offset and size of a single encoded ALAC packet
// within the MP4 container.
type sampleInfo struct {
	offset uint64
	size   uint32
}

// findALACTrack walks the MP4 box tree to locate the first track containing
// an ALAC sample entry. It returns the magic cookie and a flat sample table.
func findALACTrack(rs io.ReadSeeker) ([]byte, []sampleInfo, error) {
	stbls, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{
		mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(),
		mp4.BoxTypeMinf(), mp4.BoxTypeStbl(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading container structure: %w", err)
	}

	for _, stbl := range stbls {
		cookie, err := extractCookie(rs, stbl)
		if err != nil {
			continue // not an ALAC track
		}

		samples, err := buildSampleTable(rs, stbl)
		if err != nil {
			return nil, nil, fmt.Errorf("building sample table: %w", err)
		}

		return cookie, samples, nil
	}

	return nil, nil, errNoALACTrack
}

const (
	alacFourCC            = "alac"
	sampleEntryHeaderSize = 8  // box header: size(4) + type(4)
	sampleEntryBaseSize   = 28 // standard AudioSampleEntry fields
	sampleEntryV1Extra    = 16 // QuickTime version 1 extra fields
	stsdPayloadHeader     = 8  // version(1) + flags(3) + entryCount(4)
)

// extractCookie reads the stsd box from stbl, finds an 'alac' sample entry,
// and extracts the raw magic cookie (ALACSpecificConfig, possibly wrapped in
// 'frma'+'alac' atoms which ParseConfig handles).
func extractCookie(rs io.ReadSeeker, stbl *mp4.BoxInfo) ([]byte, error) {
	stsds, err := mp4.ExtractBox(rs, stbl, mp4.BoxPath{mp4.BoxTypeStsd()})
	if err != nil || len(stsds) == 0 {
		return nil, errNoALACTrack
	}

	stsd := stsds[0]
	payloadSize := int(stsd.Size - stsd.HeaderSize)
	data := make([]byte, payloadSize)

	if _, err := rs.Seek(int64(stsd.Offset+stsd.HeaderSize), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking to stsd payload: %w", err)
	}

	if _, err := io.ReadFull(rs, data); err != nil {
		return nil, fmt.Errorf("reading stsd payload: %w", err)
	}

	if len(data) < stsdPayloadHeader {
		return nil, errNoALACTrack
	}

	entryCount := binary.BigEndian.Uint32(data[4:8])
	pos := stsdPayloadHeader

	for range entryCount {
		if pos+sampleEntryHeaderSize > len(data) {
			break
		}

		entrySize := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if entrySize < sampleEntryHeaderSize+sampleEntryBaseSize || pos+entrySize > len(data) {
			pos += entrySize

			continue
		}

		if string(data[pos+4:pos+8]) != alacFourCC {
			pos += entrySize

			continue
		}

		// Found ALAC sample entry. Determine cookie start from QT version field.
		// Layout after 8-byte box header: reserved(6) + dataRefIdx(2) + version(2) + ...
		// Version is at offset 8 within the payload (i.e., pos + headerSize + 8).
		version := binary.BigEndian.Uint16(data[pos+sampleEntryHeaderSize+8 : pos+sampleEntryHeaderSize+10])

		skip := sampleEntryHeaderSize + sampleEntryBaseSize
		if version == 1 {
			skip += sampleEntryV1Extra
		}

		cookieStart := pos + skip
		cookieEnd := pos + entrySize

		if cookieStart >= cookieEnd {
			return nil, errInvalidCookie
		}

		return data[cookieStart:cookieEnd], nil
	}

	return nil, errNoALACTrack
}

// buildSampleTable constructs a flat list of sample offsets and sizes from
// the stco/co64, stsc, and stsz boxes within the given stbl box.
func buildSampleTable(rs io.ReadSeeker, stbl *mp4.BoxInfo) ([]sampleInfo, error) {
	chunkOffsets, err := readChunkOffsets(rs, stbl)
	if err != nil {
		return nil, err
	}

	stscEntries, err := readStsc(rs, stbl)
	if err != nil {
		return nil, err
	}

	entrySizes, constantSize, sampleCount, err := readStsz(rs, stbl)
	if err != nil {
		return nil, err
	}

	samples := make([]sampleInfo, 0, sampleCount)
	sampleIdx := 0

	for chunkIdx := range chunkOffsets {
		spc := lookupSamplesPerChunk(stscEntries, uint32(chunkIdx+1)) // stsc uses 1-based chunk numbers
		offset := chunkOffsets[chunkIdx]

		for s := uint32(0); s < spc && sampleIdx < int(sampleCount); s++ {
			var size uint32
			if constantSize != 0 {
				size = constantSize
			} else {
				size = entrySizes[sampleIdx]
			}

			samples = append(samples, sampleInfo{offset: offset, size: size})
			offset += uint64(size)
			sampleIdx++
		}
	}

	return samples, nil
}

func readChunkOffsets(rs io.ReadSeeker, stbl *mp4.BoxInfo) ([]uint64, error) {
	// Try 32-bit stco first.
	if boxes, err := mp4.ExtractBoxWithPayload(rs, stbl,
		mp4.BoxPath{mp4.BoxTypeStco()}); err == nil && len(boxes) > 0 {
		if stco, ok := boxes[0].Payload.(*mp4.Stco); ok {
			offsets := make([]uint64, len(stco.ChunkOffset))
			for i, off := range stco.ChunkOffset {
				offsets[i] = uint64(off)
			}

			return offsets, nil
		}
	}

	// Fall back to 64-bit co64.
	boxes, err := mp4.ExtractBoxWithPayload(rs, stbl, mp4.BoxPath{mp4.BoxTypeCo64()})
	if err != nil || len(boxes) == 0 {
		return nil, errors.New("alac: no chunk offset box (stco/co64)")
	}

	co64, ok := boxes[0].Payload.(*mp4.Co64)
	if !ok {
		return nil, errors.New("alac: invalid co64 payload")
	}

	return co64.ChunkOffset, nil
}

func readStsc(rs io.ReadSeeker, stbl *mp4.BoxInfo) ([]mp4.StscEntry, error) {
	boxes, err := mp4.ExtractBoxWithPayload(rs, stbl, mp4.BoxPath{mp4.BoxTypeStsc()})
	if err != nil || len(boxes) == 0 {
		return nil, errors.New("alac: no stsc box")
	}

	stsc, ok := boxes[0].Payload.(*mp4.Stsc)
	if !ok {
		return nil, errors.New("alac: invalid stsc payload")
	}

	return stsc.Entries, nil
}

func readStsz(rs io.ReadSeeker, stbl *mp4.BoxInfo) ([]uint32, uint32, uint32, error) {
	boxes, err := mp4.ExtractBoxWithPayload(rs, stbl, mp4.BoxPath{mp4.BoxTypeStsz()})
	if err != nil || len(boxes) == 0 {
		return nil, 0, 0, errors.New("alac: no stsz box")
	}

	stsz, ok := boxes[0].Payload.(*mp4.Stsz)
	if !ok {
		return nil, 0, 0, errors.New("alac: invalid stsz payload")
	}

	return stsz.EntrySize, stsz.SampleSize, stsz.SampleCount, nil
}

// lookupSamplesPerChunk finds the samples-per-chunk count for a 1-based
// chunk number from the stsc run-length table.
func lookupSamplesPerChunk(entries []mp4.StscEntry, chunkNumber uint32) uint32 {
	var spc uint32

	for _, e := range entries {
		if e.FirstChunk > chunkNumber {
			break
		}

		spc = e.SamplesPerChunk
	}

	return spc
}
