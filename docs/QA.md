# QA

## Methodology

Uncompress with ffmpeg.

Uncompress with saprobe.

Compare bit for bit match. Pass or fail.

## Run 1

```bash
go test -timeout 1h ./tests/ -run TestMassDecode -audio-path  /Volumes/Anisotope/gill/Terrible\,\ Terrible\ Music
```

```bash
ALAC: 56
FLAC: 467
MP3: 0
Time: 1172.010s
```

100% success.

## Run 2

```bash
go test -timeout 3h ./tests/ -run TestMassDecode -audio-path  /Volumes/Anisotope/gill/sweep.zorn
```

```bash
ALAC: 107
FLAC: 0
MP3: 2905
Time: 14935.919s
```

100% success.

## Run 3

```bash


```


## OggVorbis

Success on... 14 files.

Do you folks have ogg files?

Please run: `go test -v -timeout 24h ./tests/ -run TestMassDecode -audio-path MEDIA_DIRECTORY`.