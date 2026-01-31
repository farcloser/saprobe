# QA

## Methodology

Uncompress with ffmpeg.

Uncompress with saprobe.

Compare bit for bit match. Pass or fail.

## Small run

### FLAC

```bash
go test -v -timeout 24h ./tests/ -run TestMassDecode -only flac -audio-path  /Volumes/Anisotope/gill 1>./masstest-flac.log  2>&1
```

```
    mass_decode_test.go:49:   by codec: flac=467
    mass_decode_test.go:49:   by bit depth: 44100=467
    mass_decode_test.go:49:   by sample rate: 2=467
    mass_decode_test.go:49:   by channels: 16=467

PASS
ok      github.com/farcloser/saprobe/tests      707.436s
```

## Big run

### FLAC

```
ALAC/AAC: 7434
FLAC: 467
MP3: 3034
WAV: 1

Total files: 10935

Total time: 30345.057s
```

Failures:
- 193 files total
- FLAC failures: 0
- WAV: 1
- MP3 failures: 85
- M4A failures: 107


Diagnostic:
- need wav decoder...
- 


## OggVorbis

Success on... 14 files.

Do you folks have ogg files?

Please run: `go test -v -timeout 24h ./tests/ -run TestMassDecode -audio-path MEDIA_DIRECTORY`.