package alac

import (
	"fmt"

	"github.com/farcloser/saprobe"
)

// Element type tags from the ALAC bitstream.
const (
	elemSCE = 0 // Single Channel Element
	elemCPE = 1 // Channel Pair Element
	elemCCE = 2 // Coupling Channel Element (unsupported)
	elemLFE = 3 // LFE Channel Element
	elemDSE = 4 // Data Stream Element
	elemPCE = 5 // Program Config Element (unsupported)
	elemFIL = 6 // Fill Element
	elemEND = 7 // End of Frame
)

const maxCoefs = 32

// Decoder decodes ALAC audio packets into interleaved LE signed PCM.
type Decoder struct {
	config      Config
	format      saprobe.PCMFormat
	mixBufferU  []int32
	mixBufferV  []int32
	predictor   []int32
	shiftBuffer []uint16
}

// NewDecoder creates a new ALAC decoder from the given configuration.
func NewDecoder(config Config) (*Decoder, error) {
	bd, err := saprobe.ToBitDepth(config.BitDepth)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBitDepth, err)
	}

	fl := int(config.FrameLength)

	return &Decoder{
		config: config,
		format: saprobe.PCMFormat{
			SampleRate: int(config.SampleRate),
			BitDepth:   bd,
			Channels:   uint(config.NumChannels),
		},
		mixBufferU:  make([]int32, fl),
		mixBufferV:  make([]int32, fl),
		predictor:   make([]int32, fl),
		shiftBuffer: make([]uint16, fl*2), // stereo worst case
	}, nil
}

// Format returns the PCM output format.
func (d *Decoder) Format() saprobe.PCMFormat {
	return d.format
}

// DecodePacket decodes a single ALAC packet into interleaved LE signed PCM bytes.
func (d *Decoder) DecodePacket(packet []byte) ([]byte, error) {
	bb := newBitBuffer(packet)
	numSamples := d.config.FrameLength
	numChan := int(d.config.NumChannels)
	bps := d.format.BitDepth.BytesPerSample()
	chanIdx := 0

	// Allocate output for max possible size (Go zero-fills, so remaining channels are silent).
	output := make([]byte, int(numSamples)*numChan*bps)

	for {
		if bb.pastEnd() {
			return nil, errBitstreamOverrun
		}

		tag := bb.readSmall(3)

		switch tag {
		case elemSCE, elemLFE:
			ns, err := d.decodeSCE(bb, output, chanIdx, numChan, numSamples)
			if err != nil {
				return nil, fmt.Errorf("alac: SCE/LFE decode: %w", err)
			}

			numSamples = ns
			chanIdx++

		case elemCPE:
			if chanIdx+2 > numChan {
				goto done
			}

			ns, err := d.decodeCPE(bb, output, chanIdx, numChan, numSamples)
			if err != nil {
				return nil, fmt.Errorf("alac: CPE decode: %w", err)
			}

			numSamples = ns
			chanIdx += 2

		case elemCCE, elemPCE:
			return nil, errUnsupportedElement

		case elemDSE:
			if err := d.skipDSE(bb); err != nil {
				return nil, err
			}

		case elemFIL:
			if err := d.skipFIL(bb); err != nil {
				return nil, err
			}

		case elemEND:
			bb.byteAlign()

			goto done
		}

		if chanIdx >= numChan {
			break
		}
	}

done:
	actualSize := int(numSamples) * numChan * bps

	return output[:actualSize], nil
}

// decodeSCE decodes a Single Channel Element (mono) or LFE element.
func (d *Decoder) decodeSCE(bb *bitBuffer, output []byte, chanIdx, numChan int, numSamples uint32) (uint32, error) {
	_ = bb.readSmall(4) // element instance tag

	// 12 unused header bits (must be 0).
	unusedHeader := bb.read(12)
	if unusedHeader != 0 {
		return 0, errInvalidHeader
	}

	headerByte := bb.read(4)
	partialFrame := headerByte >> 3
	bytesShifted := int((headerByte >> 1) & 0x3)

	if bytesShifted == 3 {
		return 0, errInvalidShift
	}

	escapeFlag := headerByte & 0x1
	chanBits := uint32(d.config.BitDepth) - uint32(bytesShifted)*8

	if partialFrame != 0 {
		numSamples = bb.read(16) << 16
		numSamples |= bb.read(16)
	}

	if escapeFlag == 0 {
		if err := d.decodeSCECompressed(bb, chanBits, bytesShifted, int(numSamples)); err != nil {
			return 0, err
		}
	} else {
		d.decodeSCEEscape(bb, chanBits, int(numSamples))

		bytesShifted = 0
	}

	// Write output.
	ns := int(numSamples)

	switch d.config.BitDepth {
	case 16:
		writeMono16(output, d.mixBufferU, chanIdx, numChan, ns)
	case 20:
		writeMono20(output, d.mixBufferU, chanIdx, numChan, ns)
	case 24:
		writeMono24(output, d.mixBufferU, chanIdx, numChan, ns, d.shiftBuffer, bytesShifted)
	case 32:
		writeMono32(output, d.mixBufferU, chanIdx, numChan, ns, d.shiftBuffer, bytesShifted)
	}

	return numSamples, nil
}

func (d *Decoder) decodeSCECompressed(bb *bitBuffer, chanBits uint32, bytesShifted, numSamples int) error {
	_ = bb.read(8) // mixBits (unused for mono)
	_ = bb.read(8) // mixRes (unused for mono)

	headerByte := bb.read(8)
	modeU := headerByte >> 4
	denShiftU := headerByte & 0xf

	headerByte = bb.read(8)
	pbFactorU := headerByte >> 5
	numU := headerByte & 0x1f

	var coefsU [maxCoefs]int16
	for i := range numU {
		coefsU[i] = int16(bb.read(16))
	}

	// Save shift bits position, skip past them.
	var shiftBits bitBuffer
	if bytesShifted != 0 {
		shiftBits = bb.copy()
		bb.advance(uint32(bytesShifted) * 8 * uint32(numSamples))
	}

	// Entropy decode.
	pb := uint32(d.config.PB)

	var agP agParams
	setAGParams(&agP, uint32(d.config.MB), (pb*pbFactorU)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := dynDecomp(&agP, bb, d.predictor, numSamples, int(chanBits)); err != nil {
		return err
	}

	// Predictor.
	if modeU != 0 {
		unpcBlock(d.predictor, d.predictor, numSamples, nil, 31, chanBits, 0)
	}

	unpcBlock(d.predictor, d.mixBufferU, numSamples, coefsU[:numU], int32(numU), chanBits, denShiftU)

	// Read shift buffer from saved position.
	if bytesShifted != 0 {
		shift := uint8(bytesShifted * 8)
		for i := range numSamples {
			d.shiftBuffer[i] = uint16(shiftBits.read(shift))
		}
	}

	return nil
}

func (d *Decoder) decodeSCEEscape(bb *bitBuffer, chanBits uint32, numSamples int) {
	shift := uint32(32) - chanBits

	if chanBits <= 16 {
		for i := range numSamples {
			val := int32(bb.read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferU[i] = val
		}
	} else {
		extraBits := chanBits - 16

		for i := range numSamples {
			val := int32(bb.read(16))
			val = (val << 16) >> shift
			d.mixBufferU[i] = val | int32(bb.read(uint8(extraBits)))
		}
	}
}

// decodeCPE decodes a Channel Pair Element (stereo).
func (d *Decoder) decodeCPE(bb *bitBuffer, output []byte, chanIdx, numChan int, numSamples uint32) (uint32, error) {
	_ = bb.readSmall(4) // element instance tag

	unusedHeader := bb.read(12)
	if unusedHeader != 0 {
		return 0, errInvalidHeader
	}

	headerByte := bb.read(4)
	partialFrame := headerByte >> 3
	bytesShifted := int((headerByte >> 1) & 0x3)

	if bytesShifted == 3 {
		return 0, errInvalidShift
	}

	escapeFlag := headerByte & 0x1
	// CPE has +1 bit for decorrelation.
	chanBits := uint32(d.config.BitDepth) - uint32(bytesShifted)*8 + 1

	if partialFrame != 0 {
		numSamples = bb.read(16) << 16
		numSamples |= bb.read(16)
	}

	var mixBits, mixRes int32

	if escapeFlag == 0 {
		var err error

		mixBits, mixRes, err = d.decodeCPECompressed(bb, chanBits, bytesShifted, int(numSamples))
		if err != nil {
			return 0, err
		}
	} else {
		chanBits = uint32(d.config.BitDepth) // Reset for escape.
		d.decodeCPEEscape(bb, chanBits, int(numSamples))

		bytesShifted = 0
	}

	// Unmix and write output.
	ns := int(numSamples)

	switch d.config.BitDepth {
	case 16:
		writeStereo16(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, ns, mixBits, mixRes)
	case 20:
		writeStereo20(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, ns, mixBits, mixRes)
	case 24:
		writeStereo24(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, ns,
			mixBits, mixRes, d.shiftBuffer, bytesShifted)
	case 32:
		writeStereo32(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, ns,
			mixBits, mixRes, d.shiftBuffer, bytesShifted)
	}

	return numSamples, nil
}

func (d *Decoder) decodeCPECompressed(
	bb *bitBuffer,
	chanBits uint32,
	bytesShifted, numSamples int,
) (int32, int32, error) {
	mixBits := int32(bb.read(8))
	mixRes := int32(int8(bb.read(8)))

	// Read U channel predictor params.
	headerByte := bb.read(8)
	modeU := headerByte >> 4
	denShiftU := headerByte & 0xf

	headerByte = bb.read(8)
	pbFactorU := headerByte >> 5
	numU := headerByte & 0x1f

	var coefsU [maxCoefs]int16
	for i := range numU {
		coefsU[i] = int16(bb.read(16))
	}

	// Read V channel predictor params.
	headerByte = bb.read(8)
	modeV := headerByte >> 4
	denShiftV := headerByte & 0xf

	headerByte = bb.read(8)
	pbFactorV := headerByte >> 5
	numV := headerByte & 0x1f

	var coefsV [maxCoefs]int16
	for i := range numV {
		coefsV[i] = int16(bb.read(16))
	}

	// Save shift bits position, skip past interleaved shift data.
	var shiftBits bitBuffer
	if bytesShifted != 0 {
		shiftBits = bb.copy()
		bb.advance(uint32(bytesShifted) * 8 * 2 * uint32(numSamples))
	}

	pb := uint32(d.config.PB)

	var agP agParams

	// Decompress and predict U channel.
	setAGParams(&agP, uint32(d.config.MB), (pb*pbFactorU)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := dynDecomp(&agP, bb, d.predictor, numSamples, int(chanBits)); err != nil {
		return 0, 0, err
	}

	if modeU != 0 {
		unpcBlock(d.predictor, d.predictor, numSamples, nil, 31, chanBits, 0)
	}

	unpcBlock(d.predictor, d.mixBufferU, numSamples, coefsU[:numU], int32(numU), chanBits, denShiftU)

	// Decompress and predict V channel.
	setAGParams(&agP, uint32(d.config.MB), (pb*pbFactorV)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := dynDecomp(&agP, bb, d.predictor, numSamples, int(chanBits)); err != nil {
		return 0, 0, err
	}

	if modeV != 0 {
		unpcBlock(d.predictor, d.predictor, numSamples, nil, 31, chanBits, 0)
	}

	unpcBlock(d.predictor, d.mixBufferV, numSamples, coefsV[:numV], int32(numV), chanBits, denShiftV)

	// Read shift buffer from saved position.
	if bytesShifted != 0 {
		shift := uint8(bytesShifted * 8)
		for i := 0; i < numSamples*2; i += 2 {
			d.shiftBuffer[i+0] = uint16(shiftBits.read(shift))
			d.shiftBuffer[i+1] = uint16(shiftBits.read(shift))
		}
	}

	return mixBits, mixRes, nil
}

func (d *Decoder) decodeCPEEscape(bb *bitBuffer, chanBits uint32, numSamples int) {
	shift := uint32(32) - chanBits

	if chanBits <= 16 {
		for i := range numSamples {
			val := int32(bb.read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferU[i] = val

			val = int32(bb.read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferV[i] = val
		}
	} else {
		extraBits := chanBits - 16

		for i := range numSamples {
			val := int32(bb.read(16))
			val = (val << 16) >> shift
			d.mixBufferU[i] = val | int32(bb.read(uint8(extraBits)))

			val = int32(bb.read(16))
			val = (val << 16) >> shift
			d.mixBufferV[i] = val | int32(bb.read(uint8(extraBits)))
		}
	}
}

// skipFIL skips a Fill Element.
func (d *Decoder) skipFIL(bb *bitBuffer) error {
	count := int16(bb.readSmall(4))
	if count == 15 {
		count += int16(bb.readSmall(8)) - 1
	}

	bb.advance(uint32(count) * 8)

	if bb.pastEnd() {
		return errBitstreamOverrun
	}

	return nil
}

// skipDSE skips a Data Stream Element.
func (d *Decoder) skipDSE(bb *bitBuffer) error {
	_ = bb.readSmall(4) // element instance tag
	dataByteAlignFlag := bb.readOne()

	count := uint16(bb.readSmall(8))
	if count == 255 {
		count += uint16(bb.readSmall(8))
	}

	if dataByteAlignFlag != 0 {
		bb.byteAlign()
	}

	bb.advance(uint32(count) * 8)

	if bb.pastEnd() {
		return errBitstreamOverrun
	}

	return nil
}
