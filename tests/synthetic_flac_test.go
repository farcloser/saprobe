package tests_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/farcloser/saprobe"
	"github.com/farcloser/saprobe/flac"
)

// flacBitDepths that the standalone flac encoder supports.
//
//nolint:gochecknoglobals
var flacBitDepths = []int{8, 16, 24, 32}

// ffmpegFlacBitDepths that ffmpeg's FLAC encoder supports (s16 and s32 sample formats).
// s32 stores 24-bit in 32-bit container.
//
//nolint:gochecknoglobals
var ffmpegFlacBitDepths = []int{16, 24}

// flacSampleRates covers the full range of commonly used sample rates.
//
//nolint:gochecknoglobals
var flacSampleRates = []int{
	8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000,
}

// flacChannelCounts covers mono and stereo.
//
//nolint:gochecknoglobals
var flacChannelCounts = []int{1, 2}

// TestFLACDecode_FlacEncoder tests all bit depth × sample rate × channel combinations
// encoded with the standalone flac binary and decoded by saprobe.
// Reference decode uses ffmpeg.
func TestFLACDecode_FlacEncoder(t *testing.T) {
	t.Parallel()

	flacBin, err := exec.LookPath("flac")
	if err != nil {
		t.Skip("standalone flac binary not found")
	}

	for _, bitDepth := range flacBitDepths {
		for _, sampleRate := range flacSampleRates {
			for _, channels := range flacChannelCounts {
				name := fmt.Sprintf("%dbit_%dHz_%dch", bitDepth, sampleRate, channels)

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					runFlacTest(t, flacBin, bitDepth, sampleRate, channels)
				})
			}
		}
	}
}

// TestFLACDecode_FFmpegEncoder tests bit depth × sample rate × channel combinations
// encoded with ffmpeg and decoded by saprobe.
// Reference decode uses ffmpeg.
func TestFLACDecode_FFmpegEncoder(t *testing.T) {
	t.Parallel()

	for _, bitDepth := range ffmpegFlacBitDepths {
		for _, sampleRate := range flacSampleRates {
			for _, channels := range flacChannelCounts {
				name := fmt.Sprintf("%dbit_%dHz_%dch", bitDepth, sampleRate, channels)

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					runFlacFFmpegTest(t, bitDepth, sampleRate, channels)
				})
			}
		}
	}
}

// runFlacTest encodes with the standalone flac binary, decodes with saprobe and ffmpeg,
// then compares bit-for-bit.
func runFlacTest(t *testing.T, flacBin string, bitDepth, sampleRate, channels int) {
	t.Helper()

	tmpDir := t.TempDir()

	srcPCM := generateWhiteNoise(sampleRate, bitDepth, channels, 1)
	srcPath := filepath.Join(tmpDir, "source.raw")

	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	encPath := filepath.Join(tmpDir, "encoded.flac")

	if err := flacBinaryEncode(flacBin, srcPath, encPath, bitDepth, sampleRate, channels); err != nil {
		t.Fatalf("flac encode: %v", err)
	}

	compareFlacDecode(t, srcPCM, encPath, bitDepth, sampleRate, channels)
}

// runFlacFFmpegTest encodes with ffmpeg, decodes with saprobe and ffmpeg,
// then compares bit-for-bit.
func runFlacFFmpegTest(t *testing.T, bitDepth, sampleRate, channels int) {
	t.Helper()

	tmpDir := t.TempDir()

	srcPCM := generateWhiteNoise(sampleRate, bitDepth, channels, 1)
	srcPath := filepath.Join(tmpDir, "source.raw")

	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	encPath := filepath.Join(tmpDir, "encoded.flac")

	if err := flacFFmpegEncode(srcPath, encPath, bitDepth, sampleRate, channels); err != nil {
		t.Fatalf("ffmpeg flac encode: %v", err)
	}

	compareFlacDecode(t, srcPCM, encPath, bitDepth, sampleRate, channels)
}

// compareFlacDecode decodes a FLAC file with both saprobe and ffmpeg, then compares
// against each other and against the original source PCM.
func compareFlacDecode(t *testing.T, srcPCM []byte, encPath string, bitDepth, sampleRate, channels int) {
	t.Helper()

	// Decode with saprobe.
	saprobePCM, format, err := decodeFlacFile(encPath)
	if err != nil {
		t.Fatalf("saprobe decode: %v", err)
	}

	// Verify format metadata.
	if format.SampleRate != sampleRate {
		t.Errorf("sample rate: got %d, want %d", format.SampleRate, sampleRate)
	}

	if int(format.BitDepth) != bitDepth {
		t.Errorf("bit depth: got %d, want %d", format.BitDepth, bitDepth)
	}

	if int(format.Channels) != channels {
		t.Errorf("channels: got %d, want %d", format.Channels, channels)
	}

	// Decode reference with ffmpeg.
	refPCM, err := flacFFmpegDecodeRaw(encPath, bitDepth, channels)
	if err != nil {
		t.Fatalf("ffmpeg decode: %v", err)
	}

	// Compare saprobe vs ffmpeg.
	if len(refPCM) != len(saprobePCM) {
		t.Errorf("saprobe vs ffmpeg length mismatch: ffmpeg=%d, saprobe=%d", len(refPCM), len(saprobePCM))
	}

	compareLosslessSamples(t, "saprobe vs ffmpeg", refPCM, saprobePCM, bitDepth, channels)

	// Compare saprobe vs original source PCM.
	if len(srcPCM) != len(saprobePCM) {
		t.Errorf("saprobe vs source length mismatch: source=%d, saprobe=%d", len(srcPCM), len(saprobePCM))
	}

	compareLosslessSamples(t, "saprobe vs source", srcPCM, saprobePCM, bitDepth, channels)
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
		return fmt.Errorf("flac: %w\n%s", err, output)
	}

	return nil
}

// flacFFmpegEncode encodes raw PCM to FLAC using ffmpeg.
// Only supports 16-bit (s16) and 24-bit (s32 container).
func flacFFmpegEncode(srcPath, dstPath string, bitDepth, sampleRate, channels int) error {
	inputFmt := rawPCMFormat(bitDepth)

	// ffmpeg FLAC encoder sample format: s16 for 16-bit, s32 for 24-bit.
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
		return fmt.Errorf("ffmpeg: %w\n%s", err, output)
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
