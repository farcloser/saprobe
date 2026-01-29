//nolint:gochecknoglobals
package tests_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/farcloser/saprobe"
	"github.com/farcloser/saprobe/alac"
	"github.com/farcloser/saprobe/flac"
	"github.com/farcloser/saprobe/mp3"
	"github.com/farcloser/saprobe/vorbis"
)

// Codec configurations to test.
type codecConfig struct {
	name       string
	ext        string
	sampleRate int
	bitDepth   int  // input bit depth for encoding (0 = codec decides)
	lossy      bool // lossy codecs allow small sample differences between decoders
	ffmpegArgs []string
	decoder    func(path string) ([]byte, saprobe.PCMFormat, error)
}

// FLAC: lossless, supports 16/24-bit at various sample rates.
var flacConfigs = []codecConfig{
	// 16-bit
	{
		name: "flac_16bit_44100", ext: "flac", sampleRate: 44100, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s16"}, decoder: decodeFlac,
	},
	{
		name: "flac_16bit_48000", ext: "flac", sampleRate: 48000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s16"}, decoder: decodeFlac,
	},
	{
		name: "flac_16bit_96000", ext: "flac", sampleRate: 96000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s16"}, decoder: decodeFlac,
	},
	{
		name: "flac_16bit_192000", ext: "flac", sampleRate: 192000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s16"}, decoder: decodeFlac,
	},
	// 24-bit
	{
		name: "flac_24bit_44100", ext: "flac", sampleRate: 44100, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s32"}, decoder: decodeFlac,
	},
	{
		name: "flac_24bit_48000", ext: "flac", sampleRate: 48000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s32"}, decoder: decodeFlac,
	},
	{
		name: "flac_24bit_96000", ext: "flac", sampleRate: 96000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s32"}, decoder: decodeFlac,
	},
	{
		name: "flac_24bit_192000", ext: "flac", sampleRate: 192000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "flac", "-sample_fmt", "s32"}, decoder: decodeFlac,
	},
}

// ALAC: lossless, supports 16/24-bit at various sample rates.
var alacConfigs = []codecConfig{
	// 16-bit
	{
		name: "alac_16bit_44100", ext: "m4a", sampleRate: 44100, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"}, decoder: decodeAlac,
	},
	{
		name: "alac_16bit_48000", ext: "m4a", sampleRate: 48000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"}, decoder: decodeAlac,
	},
	{
		name: "alac_16bit_96000", ext: "m4a", sampleRate: 96000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"}, decoder: decodeAlac,
	},
	{
		name: "alac_16bit_192000", ext: "m4a", sampleRate: 192000, bitDepth: 16,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"}, decoder: decodeAlac,
	},
	// 24-bit
	{
		name: "alac_24bit_44100", ext: "m4a", sampleRate: 44100, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s32p"}, decoder: decodeAlac,
	},
	{
		name: "alac_24bit_48000", ext: "m4a", sampleRate: 48000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s32p"}, decoder: decodeAlac,
	},
	{
		name: "alac_24bit_96000", ext: "m4a", sampleRate: 96000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s32p"}, decoder: decodeAlac,
	},
	{
		name: "alac_24bit_192000", ext: "m4a", sampleRate: 192000, bitDepth: 24,
		ffmpegArgs: []string{"-c:a", "alac", "-sample_fmt", "s32p"}, decoder: decodeAlac,
	},
}

// Vorbis: lossy, decodes to 16-bit (or float, but we compare as 16-bit).
var vorbisConfigs = []codecConfig{
	{
		name: "vorbis_44100", ext: "ogg", sampleRate: 44100, bitDepth: 16, lossy: true,
		ffmpegArgs: []string{"-c:a", "libvorbis", "-q:a", "6"}, decoder: decodeVorbis,
	},
	{
		name: "vorbis_48000", ext: "ogg", sampleRate: 48000, bitDepth: 16, lossy: true,
		ffmpegArgs: []string{"-c:a", "libvorbis", "-q:a", "6"}, decoder: decodeVorbis,
	},
}

// MP3: lossy, supports limited sample rates, decodes to 16-bit.
var mp3Configs = []codecConfig{
	{
		name: "mp3_44100", ext: "mp3", sampleRate: 44100, bitDepth: 16, lossy: true,
		ffmpegArgs: []string{"-c:a", "libmp3lame", "-b:a", "320k"}, decoder: decodeMp3,
	},
	{
		name: "mp3_48000", ext: "mp3", sampleRate: 48000, bitDepth: 16, lossy: true,
		ffmpegArgs: []string{"-c:a", "libmp3lame", "-b:a", "320k"}, decoder: decodeMp3,
	},
}

func TestFLACDecode(t *testing.T) {
	t.Parallel()

	for _, cfg := range flacConfigs {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			runSyntheticTest(t, cfg)
		})
	}
}

func TestALACDecode(t *testing.T) {
	t.Parallel()

	for _, cfg := range alacConfigs {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			runSyntheticTest(t, cfg)
		})
	}
}

func TestVorbisDecode(t *testing.T) {
	t.Parallel()

	for _, cfg := range vorbisConfigs {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			runSyntheticTest(t, cfg)
		})
	}
}

func TestMP3Decode(t *testing.T) {
	t.Parallel()

	for _, cfg := range mp3Configs {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			runSyntheticTest(t, cfg)
		})
	}
}

func runSyntheticTest(t *testing.T, cfg codecConfig) {
	t.Helper()

	tmpDir := t.TempDir()

	// Generate source PCM (white noise, 1 second, stereo).
	srcPCM := generateWhiteNoise(cfg.sampleRate, cfg.bitDepth, 2, 1)
	srcPath := filepath.Join(tmpDir, "source.raw")

	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// Encode with ffmpeg.
	encPath := filepath.Join(tmpDir, "encoded."+cfg.ext)
	if err := ffmpegEncode(srcPath, encPath, cfg); err != nil {
		// Skip if encoder not available (e.g., libvorbis not compiled in).
		if strings.Contains(err.Error(), "Unknown encoder") ||
			strings.Contains(err.Error(), "Encoder not found") {
			t.Skipf("encoder not available: %v", err)
		}

		t.Fatalf("ffmpeg encode: %v", err)
	}

	// Decode with ffmpeg (reference).
	ffmpegPCM, err := ffmpegDecode(encPath, cfg.bitDepth)
	if err != nil {
		t.Fatalf("ffmpeg decode: %v", err)
	}

	// Decode with saprobe.
	saprobePCM, format, err := cfg.decoder(encPath)
	if err != nil {
		t.Fatalf("saprobe decode: %v", err)
	}

	// Verify format.
	if format.SampleRate != cfg.sampleRate {
		t.Errorf("sample rate: got %d, want %d", format.SampleRate, cfg.sampleRate)
	}

	if int(format.BitDepth) != cfg.bitDepth {
		t.Errorf("bit depth: got %d, want %d", format.BitDepth, cfg.bitDepth)
	}

	if format.Channels != 2 {
		t.Errorf("channels: got %d, want 2", format.Channels)
	}

	// Compare PCM data.
	if len(ffmpegPCM) != len(saprobePCM) {
		t.Errorf("length mismatch: ffmpeg=%d, saprobe=%d", len(ffmpegPCM), len(saprobePCM))
	}

	if cfg.lossy {
		// For lossy codecs, different decoders may produce slightly different sample values
		// due to floating-point rounding differences in synthesis filterbanks.
		// We compare samples (not bytes) and allow small differences.
		compareLossySamples(t, ffmpegPCM, saprobePCM, cfg.bitDepth)
	} else {
		// For lossless codecs, output must match exactly.
		compareLosslessSamples(t, ffmpegPCM, saprobePCM, cfg.bitDepth)
	}
}

// compareLosslessSamples requires exact byte match for lossless codecs.
func compareLosslessSamples(t *testing.T, ffmpegPCM, saprobePCM []byte, bitDepth int) {
	t.Helper()

	minLen := min(len(ffmpegPCM), len(saprobePCM))
	differences := 0
	firstDiff := -1

	for i := range minLen {
		if ffmpegPCM[i] != saprobePCM[i] {
			differences++

			if firstDiff == -1 {
				firstDiff = i
			}
		}
	}

	if differences > 0 {
		bytesPerSample := bitDepth / 8
		if bitDepth == 24 {
			bytesPerSample = 3
		}

		sampleIndex := firstDiff / bytesPerSample / 2 // stereo
		t.Errorf("PCM mismatch: %d differing bytes (%.2f%%), first diff at byte %d (sample %d)",
			differences, float64(differences)/float64(minLen)*100, firstDiff, sampleIndex)

		showDiffs(t, ffmpegPCM, saprobePCM, bitDepth, 5)
	}
}

// compareLossySamples allows small differences between decoders for lossy codecs.
// Different MP3/Vorbis decoders use different floating-point implementations,
// resulting in ±1-2 LSB differences per sample.
func compareLossySamples(t *testing.T, ffmpegPCM, saprobePCM []byte, bitDepth int) {
	t.Helper()

	// Only 16-bit lossy codecs are currently supported.
	if bitDepth != 16 {
		t.Errorf("lossy comparison only supports 16-bit, got %d-bit", bitDepth)

		return
	}

	numSamples := min(len(ffmpegPCM), len(saprobePCM)) / 2

	const maxDiffPerSample = 2 // Allow ±2 difference per 16-bit sample

	largeDiffs := 0
	maxDiff := int16(0)

	for i := range numSamples {
		ffSample := int16(binary.LittleEndian.Uint16(ffmpegPCM[i*2:]))
		spSample := int16(binary.LittleEndian.Uint16(saprobePCM[i*2:]))

		diff := ffSample - spSample
		if diff < 0 {
			diff = -diff
		}

		if diff > maxDiffPerSample {
			largeDiffs++
		}

		if diff > maxDiff {
			maxDiff = diff
		}
	}

	// Allow up to 1% of samples to have larger differences (codec edge cases).
	maxLargeDiffs := numSamples / 100
	if largeDiffs > maxLargeDiffs {
		t.Errorf("lossy PCM mismatch: %d samples (%.2f%%) differ by more than ±%d, max diff=%d",
			largeDiffs, float64(largeDiffs)/float64(numSamples)*100, maxDiffPerSample, maxDiff)
		showDiffs(t, ffmpegPCM, saprobePCM, bitDepth, 5)
	}
}

// generateWhiteNoise creates random PCM data.
func generateWhiteNoise(sampleRate, bitDepth, channels, durationSec int) []byte {
	numSamples := sampleRate * durationSec * channels
	bytesPerSample := bitDepth / 8

	if bitDepth == 24 {
		bytesPerSample = 3
	}

	buf := make([]byte, numSamples*bytesPerSample)

	// Use a simple PRNG for reproducibility.
	seed := uint64(0x12345678)

	for i := range numSamples {
		// xorshift64
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17

		offset := i * bytesPerSample

		switch bitDepth {
		case 16:
			// Scale to 16-bit range, leave some headroom.
			val := int16((seed % 60000) - 30000)
			binary.LittleEndian.PutUint16(buf[offset:], uint16(val))
		case 24:
			// Scale to 24-bit range.
			val := int32((seed % 14000000) - 7000000)
			buf[offset] = byte(val)
			buf[offset+1] = byte(val >> 8)
			buf[offset+2] = byte(val >> 16)
		case 32:
			val := int32((seed % 1800000000) - 900000000)
			binary.LittleEndian.PutUint32(buf[offset:], uint32(val))

		default:
		}
	}

	return buf
}

// ffmpegEncode encodes raw PCM to the target format.
func ffmpegEncode(srcPath, dstPath string, cfg codecConfig) error {
	sampleFmt := "s16le"

	switch cfg.bitDepth {
	case 24:
		sampleFmt = "s24le"
	case 32:
		sampleFmt = "s32le"

	default:
	}

	args := []string{
		"-y",
		"-f", sampleFmt,
		"-ar", fmt.Sprintf("%d", cfg.sampleRate),
		"-ac", "2",
		"-i", srcPath,
	}
	args = append(args, cfg.ffmpegArgs...)
	args = append(args, dstPath)

	cmd := exec.Command("ffmpeg", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w\n%s", err, output)
	}

	return nil
}

// ffmpegDecode decodes an audio file to raw PCM using ffmpeg.
func ffmpegDecode(srcPath string, bitDepth int) ([]byte, error) {
	sampleFmt := "s16le"

	switch bitDepth {
	case 24:
		sampleFmt = "s24le"
	case 32:
		sampleFmt = "s32le"

	default:
	}

	cmd := exec.Command("ffmpeg",
		"-i", srcPath,
		"-f", sampleFmt,
		"-acodec", "pcm_"+sampleFmt[:3]+"le",
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

// Decoder wrappers.
func decodeFlac(path string) ([]byte, saprobe.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}
	defer f.Close()

	return flac.Decode(f)
}

func decodeAlac(path string) ([]byte, saprobe.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}
	defer f.Close()

	return alac.Decode(f)
}

func decodeVorbis(path string) ([]byte, saprobe.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}
	defer f.Close()

	return vorbis.Decode(f)
}

func decodeMp3(path string) ([]byte, saprobe.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, saprobe.PCMFormat{}, err
	}
	defer f.Close()

	return mp3.Decode(f)
}

// showDiffs prints the first N differing samples for debugging.
func showDiffs(t *testing.T, ffmpegPCM, saprobePCM []byte, bitDepth, maxDiffs int) {
	t.Helper()

	bytesPerSample := bitDepth / 8
	if bitDepth == 24 {
		bytesPerSample = 3
	}

	frameSize := bytesPerSample * 2 // stereo
	shown := 0

	for i := 0; i < min(len(ffmpegPCM), len(saprobePCM))-frameSize && shown < maxDiffs; i += frameSize {
		ffmpegFrame := ffmpegPCM[i : i+frameSize]
		saprobeFrame := saprobePCM[i : i+frameSize]

		if !bytes.Equal(ffmpegFrame, saprobeFrame) {
			sampleIdx := i / frameSize
			t.Logf("Sample %d: ffmpeg=%v, saprobe=%v", sampleIdx, ffmpegFrame, saprobeFrame)

			shown++
		}
	}
}
