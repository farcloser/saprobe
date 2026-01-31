package tests_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mycophonic/saprobe"
	"github.com/mycophonic/saprobe/flac"
)

// flacEncoderType identifies a FLAC encoder.
type flacEncoderType int

const (
	encoderSaprobe flacEncoderType = iota
	encoderFlacBinary
	encoderFFmpeg
)

// flacDecoderType identifies a FLAC decoder.
type flacDecoderType int

const (
	decoderSaprobe flacDecoderType = iota
	decoderFlacBinary
	decoderFFmpeg
)

// All encodable FLAC bit depths (4-bit excluded: no frame header bit pattern in FLAC spec).
//
//nolint:gochecknoglobals
var allFlacBitDepths = []int{8, 12, 16, 20, 24, 32}

// flacSampleRates covers the full range of commonly used sample rates.
//
//nolint:gochecknoglobals
var flacSampleRates = []int{
	8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000,
}

// flacChannelCounts covers all FLAC-supported channel counts (1 through 8).
//
//nolint:gochecknoglobals
var flacChannelCounts = []int{1, 2, 3, 4, 5, 6, 7, 8}

// encodersForBitDepth returns which encoders support the given bit depth.
func encodersForBitDepth(bitDepth int) []flacEncoderType {
	switch bitDepth {
	case 12, 20:
		return []flacEncoderType{encoderSaprobe}
	case 8, 32:
		return []flacEncoderType{encoderSaprobe, encoderFlacBinary}
	case 16, 24:
		return []flacEncoderType{encoderSaprobe, encoderFlacBinary, encoderFFmpeg}
	default:
		return nil
	}
}

// decodersForBitDepth returns which decoders support the given bit depth.
func decodersForBitDepth(bitDepth int) []flacDecoderType {
	switch bitDepth {
	case 12, 20:
		return []flacDecoderType{decoderSaprobe}
	case 8, 16, 24, 32:
		return []flacDecoderType{decoderSaprobe, decoderFlacBinary, decoderFFmpeg}
	default:
		return nil
	}
}

func encoderName(enc flacEncoderType) string {
	switch enc {
	case encoderSaprobe:
		return "saprobe"
	case encoderFlacBinary:
		return "flac"
	case encoderFFmpeg:
		return "ffmpeg"
	default:
		return "unknown"
	}
}

func decoderName(dec flacDecoderType) string {
	switch dec {
	case decoderSaprobe:
		return "saprobe"
	case decoderFlacBinary:
		return "flac"
	case decoderFFmpeg:
		return "ffmpeg"
	default:
		return "unknown"
	}
}

// ffmpegMultichannelFails reports whether ffmpeg's FLAC decoder is known to produce
// different PCM byte ordering for the given bit depth and channel count due to
// channel layout remapping. saprobe and the flac binary match source bit-for-bit
// in all these cases.
func ffmpegMultichannelFails(bitDepth, channels int) bool {
	switch bitDepth {
	case 8:
		return channels >= 3 && channels <= 4
	case 16:
		return (channels >= 3 && channels <= 4) || (channels >= 7 && channels <= 8)
	case 24:
		return channels >= 3
	case 32:
		return channels >= 3 && channels <= 6
	default:
		return false
	}
}

// TestFLACDecode tests all bit depth x encoder x sample rate x channel combinations.
// For each encoded file, all supported decoders are used and compared against the source.
func TestFLACDecode(t *testing.T) {
	t.Parallel()

	flacBin, flacBinErr := exec.LookPath("flac")

	for _, bitDepth := range allFlacBitDepths {
		encoders := encodersForBitDepth(bitDepth)
		decoders := decodersForBitDepth(bitDepth)

		for _, enc := range encoders {
			for _, sampleRate := range flacSampleRates {
				for _, channels := range flacChannelCounts {
					name := fmt.Sprintf(
						"%dbit/%s/%dHz_%dch",
						bitDepth, encoderName(enc), sampleRate, channels,
					)

					t.Run(name, func(t *testing.T) {
						t.Parallel()

						if enc == encoderFlacBinary && flacBinErr != nil {
							t.Skip("standalone flac binary not found")
						}

						runFlacTest(t, enc, decoders, flacBin, bitDepth, sampleRate, channels)
					})
				}
			}
		}
	}
}

//nolint:cyclop // Test orchestration requires many steps.
func runFlacTest(
	t *testing.T,
	enc flacEncoderType,
	decoders []flacDecoderType,
	flacBin string,
	bitDepth, sampleRate, channels int,
) {
	t.Helper()

	tmpDir := t.TempDir()
	srcPCM := generateWhiteNoise(sampleRate, bitDepth, channels, 1)
	srcPath := filepath.Join(tmpDir, "source.raw")

	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	encPath := filepath.Join(tmpDir, "encoded.flac")

	// Encode with the selected encoder.
	switch enc {
	case encoderSaprobe:
		if err := saprobeFlacEncode(srcPCM, encPath, bitDepth, sampleRate, channels); err != nil {
			t.Fatalf("saprobe encode: %v", err)
		}
	case encoderFlacBinary:
		if err := flacBinaryEncode(flacBin, srcPath, encPath, bitDepth, sampleRate, channels); err != nil {
			t.Fatalf("flac encode: %v", err)
		}
	case encoderFFmpeg:
		if err := flacFFmpegEncode(srcPath, encPath, bitDepth, sampleRate, channels); err != nil {
			t.Fatalf("ffmpeg encode: %v", err)
		}
	default:
		t.Fatalf("unknown encoder type: %d", enc)
	}

	// Decode with every supported decoder and compare against source.
	decoded := make(map[string][]byte, len(decoders))

	ffmpegSkip := ffmpegMultichannelFails(bitDepth, channels)

	for _, dec := range decoders {
		if dec == decoderFlacBinary && flacBin == "" {
			t.Log("skipping flac decoder: binary not found")

			continue
		}

		if dec == decoderFFmpeg && ffmpegSkip {
			t.Logf("skipping ffmpeg decode: known channel remapping for %dbit/%dch", bitDepth, channels)

			continue
		}

		decName := decoderName(dec)
		pcm, format := runFlacDecode(t, dec, flacBin, encPath, bitDepth, channels)

		// Verify format metadata (saprobe decoder only, others return raw bytes).
		if dec == decoderSaprobe && format != nil {
			verifyFlacFormat(t, format, sampleRate, bitDepth, channels)
		}

		// Compare decoded PCM vs original source.
		label := fmt.Sprintf("decode(%s) vs source", decName)

		if len(srcPCM) != len(pcm) {
			t.Errorf("%s length mismatch: source=%d, decoded=%d", label, len(srcPCM), len(pcm))
		}

		compareLosslessSamples(t, label, srcPCM, pcm, bitDepth, channels)

		decoded[decName] = pcm
	}

	// Cross-compare all decoder outputs against each other.
	decoderNames := make([]string, 0, len(decoded))
	for name := range decoded {
		decoderNames = append(decoderNames, name)
	}

	for idx := range decoderNames {
		for jdx := idx + 1; jdx < len(decoderNames); jdx++ {
			nameA := decoderNames[idx]
			nameB := decoderNames[jdx]
			label := fmt.Sprintf("decode(%s) vs decode(%s)", nameA, nameB)

			if len(decoded[nameA]) != len(decoded[nameB]) {
				t.Errorf("%s length mismatch: %s=%d, %s=%d",
					label, nameA, len(decoded[nameA]), nameB, len(decoded[nameB]))
			}

			compareLosslessSamples(t, label, decoded[nameA], decoded[nameB], bitDepth, channels)
		}
	}
}

func runFlacDecode(
	t *testing.T,
	dec flacDecoderType,
	flacBin, encPath string,
	bitDepth, channels int,
) ([]byte, *saprobe.PCMFormat) {
	t.Helper()

	switch dec {
	case decoderSaprobe:
		pcm, format, err := decodeFlacFile(encPath)
		if err != nil {
			t.Fatalf("saprobe decode: %v", err)
		}

		return pcm, &format
	case decoderFlacBinary:
		pcm, err := flacBinaryDecodeRaw(flacBin, encPath)
		if err != nil {
			t.Fatalf("flac decode: %v", err)
		}

		return pcm, nil
	case decoderFFmpeg:
		pcm, err := flacFFmpegDecodeRaw(encPath, bitDepth, channels)
		if err != nil {
			t.Fatalf("ffmpeg decode: %v", err)
		}

		return pcm, nil
	default:
		t.Fatalf("unknown decoder type: %d", dec)

		return nil, nil
	}
}

func verifyFlacFormat(t *testing.T, format *saprobe.PCMFormat, sampleRate, bitDepth, channels int) {
	t.Helper()

	if format.SampleRate != sampleRate {
		t.Errorf("sample rate: got %d, want %d", format.SampleRate, sampleRate)
	}

	if int(format.BitDepth) != bitDepth {
		t.Errorf("bit depth: got %d, want %d", format.BitDepth, bitDepth)
	}

	if int(format.Channels) != channels {
		t.Errorf("channels: got %d, want %d", format.Channels, channels)
	}
}

// saprobeFlacEncode encodes raw PCM to FLAC using saprobe's encoder.
func saprobeFlacEncode(srcPCM []byte, dstPath string, bitDepth, sampleRate, channels int) error {
	depth, err := saprobe.ToBitDepth(uint8(bitDepth))
	if err != nil {
		return err
	}

	format := saprobe.PCMFormat{
		SampleRate: sampleRate,
		BitDepth:   depth,
		Channels:   uint(channels),
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return flac.Encode(f, srcPCM, format)
}

// flacBinaryEncode encodes raw PCM to FLAC using the standalone flac binary.
func flacBinaryEncode(flacBin, srcPath, dstPath string, bitDepth, sampleRate, channels int) error {
	cmd := exec.Command(flacBin,
		"-f",
		"--force-raw-format",
		"--sign=signed",
		"--endian=little",
		fmt.Sprintf("--channels=%d", channels),
		fmt.Sprintf("--bps=%d", bitDepth),
		fmt.Sprintf("--sample-rate=%d", sampleRate),
		"-o", dstPath,
		srcPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("flac encode: %w\n%s", err, output)
	}

	return nil
}

// flacBinaryDecodeRaw decodes a FLAC file to raw PCM using the standalone flac binary.
func flacBinaryDecodeRaw(flacBin, srcPath string) ([]byte, error) {
	cmd := exec.Command(flacBin,
		"-d", "-f",
		"--force-raw-format",
		"--sign=signed",
		"--endian=little",
		"-o", "-",
		srcPath,
	)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("flac decode: %w\n%s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// flacFFmpegEncode encodes raw PCM to FLAC using ffmpeg.
// Only supports 16-bit (s16) and 24-bit (s32 container).
func flacFFmpegEncode(srcPath, dstPath string, bitDepth, sampleRate, channels int) error {
	inputFmt := rawPCMFormat(bitDepth)

	var sampleFmt string

	switch bitDepth {
	case 16:
		sampleFmt = "s16"
	case 24:
		sampleFmt = "s32"
	default:
		return fmt.Errorf("ffmpeg FLAC encoder does not support %d-bit", bitDepth)
	}

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", inputFmt,
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", fmt.Sprintf("%d", channels),
		"-i", srcPath,
		"-c:a", "flac",
		"-sample_fmt", sampleFmt,
		dstPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg encode: %w\n%s", err, output)
	}

	return nil
}

// flacFFmpegDecodeRaw decodes a FLAC file to raw PCM using ffmpeg.
func flacFFmpegDecodeRaw(srcPath string, bitDepth, channels int) ([]byte, error) {
	outFmt := rawPCMFormat(bitDepth)
	codec := rawPCMCodec(bitDepth)

	cmd := exec.Command("ffmpeg",
		"-i", srcPath,
		"-f", outFmt,
		"-ac", fmt.Sprintf("%d", channels),
		"-acodec", codec,
		"-",
	)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg decode: %w\n%s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// rawPCMFormat returns the ffmpeg raw format name for a given bit depth.
func rawPCMFormat(bitDepth int) string {
	switch bitDepth {
	case 8:
		return "s8"
	case 24:
		return "s24le"
	case 32:
		return "s32le"
	default:
		return "s16le"
	}
}

// rawPCMCodec returns the ffmpeg PCM codec name for a given bit depth.
func rawPCMCodec(bitDepth int) string {
	switch bitDepth {
	case 8:
		return "pcm_s8"
	case 24:
		return "pcm_s24le"
	case 32:
		return "pcm_s32le"
	default:
		return "pcm_s16le"
	}
}

func decodeFlacFile(path string) ([]byte, saprobe.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}
	defer f.Close()

	return flac.Decode(f)
}
