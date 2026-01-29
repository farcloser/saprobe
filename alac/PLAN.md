# ALAC Decoder - Implementation Plan

## Overview

Pure Go port of the Apple ALAC decoder (Apache 2.0).
Source reference: `haustorium/_/alac/codec/`

## Decoding Pipeline

```
ALAC Packet
  → BitBuffer (bit-level I/O)
  → Element tag dispatch (SCE/CPE/LFE/FIL/DSE/END)
  → Per element:
      → Read frame header (partial flag, shift, escape)
      → If compressed:
          → Adaptive Golomb entropy decode (ag_dec.c)
          → Dynamic predictor (dp_dec.c)
      → If uncompressed (escape):
          → Read raw PCM samples
      → Stereo: matrix unmix (matrix_dec.c)
      → Mono: copy predictor to output
  → Interleaved little-endian signed PCM bytes
```

## File Layout

| File           | C Source              | Purpose                                    |
|----------------|-----------------------|--------------------------------------------|
| doc.go         | -                     | Package documentation                      |
| errors.go      | -                     | Error sentinels                            |
| config.go      | ALACAudioTypes.h      | Config, BitDepth, PCMFormat, ParseConfig   |
| bitbuffer.go   | ALACBitUtilities.c    | Bit-level reader                           |
| golomb.go      | ag_dec.c, aglib.h     | Adaptive Golomb-Rice entropy decoder       |
| predictor.go   | dp_dec.c, dplib.h     | Dynamic linear predictor (FIR filter)      |
| matrix.go      | matrix_dec.c          | Stereo unmix + output byte formatting      |
| decoder.go     | ALACDecoder.cpp       | Decoder struct, packet decode, element dispatch |

## Public API

```go
type Config struct { ... }       // From magic cookie (ALACSpecificConfig)
type BitDepth uint8              // 16, 20, 24, 32
type PCMFormat struct { ... }    // SampleRate, BitDepth, Channels

func ParseConfig(cookie []byte) (Config, error)

type Decoder struct { ... }
func NewDecoder(config Config) *Decoder
func (d *Decoder) DecodePacket(packet []byte) ([]byte, error)
func (d *Decoder) Format() PCMFormat
```

## Output Format

- Interleaved little-endian signed PCM bytes
- 16-bit → 2 bytes/sample, 20-bit → 3 bytes/sample (left-aligned in 24-bit),
  24-bit → 3 bytes/sample, 32-bit → 4 bytes/sample
- Matches saprobe/flac output conventions and haustorium consumer expectations

## Key Decisions

1. **BitBuffer padding**: 4 zero bytes appended to prevent out-of-bounds on near-end reads
2. **Shift buffer**: Separate allocation (not aliased to predictor buffer like Apple C code)
3. **Endian**: Always LE output (no HBYTE/LBYTE macros needed)
4. **Error handling**: Return errors, never panic
5. **Bit depths**: Full support for 16, 20, 24, 32
6. **Channels**: Full support for 1-8 (mono, stereo, surround, 7.1)

## Supported Configurations

- Bit depths: 16, 20, 24, 32
- Channels: 1-8 (all ALAC channel layouts)
- Element types: SCE (mono), CPE (stereo pair), LFE (subwoofer), FIL, DSE, END
- Compressed and uncompressed (escape) frames
- Partial frames
- Shift-bit encoding (24/32-bit LSB stripping)
- Double prediction pass (modeU/modeV != 0)
