package tests_test

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/mycophonic/saprobe/tests/testutils"
)

//nolint:gochecknoglobals
var (
	audioPath = flag.String("audio-path", "", "directory of audio files for mass decode comparison")
	only      = flag.String("only", "", "filter by file extension (e.g. flac, mp3, ogg, m4a)")
)

// TestMassDecode walks a directory of audio files, decodes each with saprobe
// and ffmpeg, and compares the raw PCM output byte-for-byte.
//
// Run: go test ./tests/ -run TestMassDecode -audio-path /path/to/audio/files.
func TestMassDecode(t *testing.T) {
	t.Parallel()

	if *audioPath == "" {
		t.Skip("no -audio-path flag provided")
	}

	root := *audioPath

	files := discoverAudioFiles(t, root)
	if len(files) == 0 {
		t.Fatalf("no audio files found in %s", root)
	}

	t.Logf("discovered %d audio files in %s", len(files), root)
	logDiscoverySummary(t, files)

	testCase := testutils.Setup()
	testCase.Description = "mass decode comparison"
	// Parallelism will screw the pooch.
	testCase.NoParallel = true

	for i, af := range files {
		t.Logf("[%d/%d] queuing %s (%s)", i+1, len(files), af.relPath, af.pcmFormat)
		testCase.SubTests = append(testCase.SubTests, makeDecodeTest(af, i+1, len(files)))
	}

	testCase.Run(t)
}

func makeDecodeTest(af audioFile, index, total int) *test.Case {
	prefix := fmt.Sprintf("[%d/%d]", index, total)

	return &test.Case{
		Description: af.relPath,
		// Parallelism will screw the pooch.
		NoParallel: true,
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.T().Log(fmt.Sprintf("%s decoding reference with ffmpeg (%s): %s", prefix, af.pcmFormat, af.path))

			refOut := data.Temp().Path("ref.raw")
			helpers.Custom("ffmpeg",
				"-i", af.path,
				"-f", af.pcmFormat,
				"-y", refOut,
			).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})

			info, _ := os.Stat(refOut)
			if info != nil {
				helpers.T().Log(fmt.Sprintf("%s ffmpeg reference: %d bytes", prefix, info.Size()))
			}
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			helpers.T().Log(fmt.Sprintf("%s decoding with saprobe: %s", prefix, af.path))

			saprobeOut := data.Temp().Path("saprobe.raw")

			return helpers.Command("decode", "--raw", "-o", saprobeOut, af.path)
		},
		Expected: func(data test.Data, _ test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: expect.ExitCodeSuccess,
				Output:   comparePCMFiles(data, prefix, af.lossy),
			}
		},
	}
}

type audioFile struct {
	path       string // absolute path to the audio file
	relPath    string // path relative to the test root (used as test name)
	pcmFormat  string // ffmpeg raw output format: "s16le", "s24le", "s32le"
	lossy      bool   // true for lossy codecs (mp3, ogg) that allow small sample differences
	codec      string // ffprobe codec name (e.g. "flac", "mp3", "vorbis", "alac")
	bitDepth   string // bit depth as reported by ffprobe (e.g. "16", "24", "N/A")
	sampleRate string // sample rate in Hz (e.g. "44100", "48000")
	channels   string // number of channels (e.g. "1", "2", "6")
}

// discoverAudioFiles recursively walks root, probes every file with ffprobe,
// and returns those containing exactly one audio stream.
// Files with no audio streams are silently skipped.
// Files with multiple audio streams cause an immediate fatal error.
func discoverAudioFiles(t *testing.T, root string) []audioFile {
	t.Helper()

	var files []audioFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		// Filter by extension if -only is set.
		if *only != "" {
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if !strings.EqualFold(ext, *only) {
				return nil
			}
		}

		af, isAudio := probeAudioFile(t, path)
		if !isAudio {
			return nil
		}

		af.relPath, _ = filepath.Rel(root, path)

		files = append(files, af)

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	return files
}

// probeAudioFile runs ffprobe on path to detect audio streams.
// Returns the audioFile metadata and true if exactly one audio stream exists.
// Returns false (skip) if no audio streams are found.
// Fatals if multiple audio streams are detected.
func probeAudioFile(t *testing.T, path string) (audioFile, bool) {
	t.Helper()

	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "a",
		"-show_entries", "stream=codec_name,bits_per_raw_sample,sample_rate,channels",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		// ffprobe failed — not a media file, skip.
		return audioFile{}, false
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		// No audio streams.
		return audioFile{}, false
	}

	lines := strings.Split(raw, "\n")
	if len(lines) > 1 {
		t.Fatalf("multiple audio tracks (%d) in %s", len(lines), path)
	}

	// Single audio stream: "codec_name,bits_per_raw_sample,sample_rate,channels"
	// e.g. "flac,16,44100,2" or "mp3,N/A,44100,2" or "vorbis,N/A,44100,2"
	fields := strings.Split(lines[0], ",")
	codecName := strings.TrimSpace(fields[0])

	var bitsRaw, sampleRate, channels string
	if len(fields) > 1 {
		bitsRaw = strings.TrimSpace(fields[1])
	}

	if len(fields) > 2 {
		sampleRate = strings.TrimSpace(fields[2])
	}

	if len(fields) > 3 {
		channels = strings.TrimSpace(fields[3])
	}

	pcmFmt, lossy := codecToPCMFormat(codecName, bitsRaw)

	return audioFile{
		path:       path,
		pcmFormat:  pcmFmt,
		lossy:      lossy,
		codec:      codecName,
		bitDepth:   bitsRaw,
		sampleRate: sampleRate,
		channels:   channels,
	}, true
}

// codecToPCMFormat maps an ffprobe codec name and bit depth string to the
// ffmpeg raw PCM output format and whether the codec is lossy.
func codecToPCMFormat(codec, bitsRaw string) (string, bool) {
	// Lossy codecs: saprobe always decodes to 16-bit.
	switch codec {
	case "mp3", "vorbis":
		return "s16le", true
	}

	// Lossless codecs: use probed bit depth.
	depth, _ := strconv.Atoi(bitsRaw)

	switch depth {
	case 32:
		return "s32le", false
	case 20, 24:
		return "s24le", false
	default:
		return "s16le", false
	}
}

// comparePCMFiles returns a comparator that reads the saprobe and ffmpeg output
// files from the test's temp directory and compares them.
// For lossless codecs, comparison is byte-for-byte.
// For lossy codecs, small per-sample differences (±2) are tolerated.
//
//revive:disable:flag-parameter
func comparePCMFiles(data test.Data, prefix string, lossy bool) test.Comparator {
	return func(_ string, t tig.T) {
		t.Helper()

		saprobeData, err := os.ReadFile(data.Temp().Path("saprobe.raw"))
		if err != nil {
			t.Log(prefix + " reading saprobe output: " + err.Error())
			t.Fail()

			return
		}

		refData, err := os.ReadFile(data.Temp().Path("ref.raw"))
		if err != nil {
			t.Log(prefix + " reading ffmpeg reference: " + err.Error())
			t.Fail()

			return
		}

		t.Log(fmt.Sprintf("%s comparing: saprobe=%d bytes, ffmpeg=%d bytes", prefix, len(saprobeData), len(refData)))

		if lossy {
			compareLossy(t, prefix, saprobeData, refData)
		} else {
			compareLossless(t, prefix, saprobeData, refData)
		}
	}
}

// compareLossless requires exact byte match.
func compareLossless(t tig.T, prefix string, saprobeData, refData []byte) {
	t.Helper()

	if bytes.Equal(saprobeData, refData) {
		t.Log(prefix + " MATCH")

		return
	}

	t.Log(prefix + " MISMATCH")
	reportFirstDifference(t, prefix, saprobeData, refData)
	t.Fail()
}

// compareLossy allows small per-sample differences (±2) for lossy codecs.
// Different decoders have floating-point rounding differences in synthesis filterbanks.
// Length differences up to 1 MP3 frame (1152 stereo samples = 4608 bytes) are tolerated
// because decoders handle trailing metadata and end-of-stream differently.
func compareLossy(t tig.T, prefix string, saprobeData, refData []byte) {
	t.Helper()

	// Allow length differences up to 1 frame (1152 samples * 2 channels * 2 bytes).
	const maxLengthDiffBytes = 1152 * 2 * 2

	lengthDiff := len(saprobeData) - len(refData)
	if lengthDiff < 0 {
		lengthDiff = -lengthDiff
	}

	if lengthDiff > maxLengthDiffBytes {
		t.Log(fmt.Sprintf("%s LENGTH MISMATCH: saprobe=%d, ffmpeg=%d (diff=%d exceeds tolerance %d)",
			prefix, len(saprobeData), len(refData), lengthDiff, maxLengthDiffBytes))
		t.Fail()

		return
	}

	if lengthDiff > 0 {
		t.Log(fmt.Sprintf("%s length diff: saprobe=%d, ffmpeg=%d (±%d bytes, within tolerance)",
			prefix, len(saprobeData), len(refData), lengthDiff))
	}

	// Compare the overlapping region with per-sample tolerance.
	const maxDiffPerSample = 2

	compareLen := min(len(saprobeData), len(refData))
	numSamples := compareLen / 2
	largeDiffs := 0
	maxDiff := int16(0)

	for i := range numSamples {
		spSample := int16(binary.LittleEndian.Uint16(saprobeData[i*2:]))
		refSample := int16(binary.LittleEndian.Uint16(refData[i*2:]))

		diff := spSample - refSample
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

	// Allow up to 1% of samples to have larger differences.
	maxLargeDiffs := numSamples / 100
	if largeDiffs > maxLargeDiffs {
		t.Log(fmt.Sprintf("%s LOSSY MISMATCH: %d samples (%.2f%%) differ by more than ±%d, max diff=%d",
			prefix, largeDiffs, float64(largeDiffs)/float64(numSamples)*100, maxDiffPerSample, maxDiff))
		t.Fail()

		return
	}

	t.Log(fmt.Sprintf("%s MATCH (lossy: max diff=%d, large diffs=%d)", prefix, maxDiff, largeDiffs))
}

func logDiscoverySummary(t *testing.T, files []audioFile) {
	t.Helper()

	byCodec := map[string]int{}
	byBitDepth := map[string]int{}
	bySampleRate := map[string]int{}
	byChannels := map[string]int{}

	for _, af := range files {
		byCodec[af.codec]++
		byBitDepth[af.bitDepth]++
		bySampleRate[af.sampleRate]++
		byChannels[af.channels]++
	}

	t.Logf("--- summary: %d files ---", len(files))
	logBreakdown(t, "codec", byCodec)
	logBreakdown(t, "bit depth", byBitDepth)
	logBreakdown(t, "sample rate", bySampleRate)
	logBreakdown(t, "channels", byChannels)
}

func logBreakdown(t *testing.T, label string, counts map[string]int) {
	t.Helper()

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}

	t.Logf("  by %s: %s", label, strings.Join(parts, ", "))
}

func reportFirstDifference(t tig.T, prefix string, a, b []byte) {
	t.Helper()

	minLen := min(len(a), len(b))

	for i := range minLen {
		if a[i] != b[i] {
			t.Log(fmt.Sprintf("%s first difference at byte %d: saprobe=0x%02x ffmpeg=0x%02x", prefix, i, a[i], b[i]))

			return
		}
	}

	t.Log(fmt.Sprintf("%s identical up to byte %d, then one output is longer", prefix, minLen))
}
