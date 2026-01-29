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
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/farcloser/saprobe/tests/testutils"
)

//nolint:gochecknoglobals
var audioPath = flag.String("audio-path", "", "directory of audio files for mass decode comparison")

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

			return helpers.Command("decode", "-o", saprobeOut, af.path)
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
	path      string // absolute path to the audio file
	relPath   string // path relative to the test root (used as test name)
	pcmFormat string // ffmpeg raw output format: "s16le", "s24le", "s32le"
	lossy     bool   // true for lossy codecs (mp3, ogg) that allow small sample differences
}

//nolint:gochecknoglobals
var supportedExts = map[string]bool{
	".flac": true,
	".m4a":  true,
	".mp3":  true,
	".ogg":  true,
}

// discoverAudioFiles recursively walks root and returns all supported audio files
// with their probed output format.
func discoverAudioFiles(t *testing.T, root string) []audioFile {
	t.Helper()

	var files []audioFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExts[ext] {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		pcmFmt, isLossy := probeOutputFormat(t, path, ext)

		files = append(files, audioFile{
			path:      path,
			relPath:   rel,
			pcmFormat: pcmFmt,
			lossy:     isLossy,
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	return files
}

// probeOutputFormat determines the ffmpeg raw PCM format string that matches
// what saprobe will produce for the given file. Returns (format, isLossy).
func probeOutputFormat(t *testing.T, path, ext string) (string, bool) {
	t.Helper()

	// Lossy codecs: saprobe always decodes to 16-bit.
	switch ext {
	case ".mp3", ".ogg":
		return "s16le", true
	}

	// Lossless codecs: probe actual bit depth with ffprobe.
	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=bits_per_raw_sample",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}

	depth, _ := strconv.Atoi(strings.TrimSpace(string(out)))

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
func compareLossy(t tig.T, prefix string, saprobeData, refData []byte) {
	t.Helper()

	// Check length match first.
	if len(saprobeData) != len(refData) {
		t.Log(fmt.Sprintf("%s LENGTH MISMATCH: saprobe=%d, ffmpeg=%d", prefix, len(saprobeData), len(refData)))
		t.Fail()

		return
	}

	// Compare 16-bit samples with tolerance.
	const maxDiffPerSample = 2

	numSamples := len(saprobeData) / 2
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
