# Saprobe

> * a pure Go audio decoder supporting MP3, FLAC, ALAC, and OggVorbis.
> * [Saprobes are fungi involved in saprotrophic nutrition, a process of chemoheterotrophic extracellular digestion of organic matter](https://en.wikipedia.org/wiki/Saprobe)

![Saprobe](logo.jpg)

## Purpose

AFAIC, there is no simple decoding tool written in pure Go that support these formats.

Saprobe provides that.

Our ALAC implementation is a homegrown port of the Apple library to Go (with some help from github.com/abema/go-mp4
for boxes parsing).

FLAC, OggVorbis, and MP3 are provided by the following awesome libraries that we just wrap and instrument:
- github.com/hajimehoshi/go-mp3 (Apache)
- github.com/jfreymuth/oggvorbis (MIT)
- github.com/mewkiz/flac (Unlicense)

## Installation

On macOS:

```bash
brew install --head farcloser/mycota/saprobe
```

Others need Go installed and configured, then:
```bash
go install github.com/farcloser/saprobe/cmd/saprobe@latest
```

## Usage

```bash
# Just decode.
saprobe decode my_audio_file > decoded.wav
# Or
saprobe decode -o decoded.wav my_audio_file

# Just get the format info.
saprobe decode --info my_audio_file

# Default bit depth is to use the native from the source.
saprobe decode --bit-depth=[12|24|32] --info my_audio_file
```

## Quality and support

FLAC, ALAC, and MP3 implementations have been tested on a large number of files, conclusively producing bit for bit
identical content for FLAC and ALAC, and within rounding errors tolerance range for MP3.

See [QA](docs/QA.md) for more details.

Tier-1:
* ALAC: DONE. Actively maintained
* FLAC: DONE. Actively maintained
* Opus: TODO. No solution right now. Accept WASM as escape hatch? Implement from scratch?

Tier-2:
* AAC: TODO. No solution right now. Likely need implement from scratch.
* DSD: TODO. Clusterfuck.
  * Presumably need DAC capabilities detection for the purists and stuff in DoP.
  * Decoding to 24-bit/352.8kHz PCM for hardware without DoP support. No Go implem. Must implement from scratch.

Tier-3:
* MP3: DONE. Here because you can't avoid it, but unlikely to receive much love (already added proper gapless support on top
of hajimehoshi/go-mp3). It just works, and the format is dead anyhow, so...
* OggVorbis: DONE. Barely tested (only have a few files). Similar to MP3 situation (better format, but still a dead pony)
