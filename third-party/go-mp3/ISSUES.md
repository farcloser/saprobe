# go-mp3 Fork: Known Issues

Diagnosed from real-world MP3 decoding failures.

## 1. No free bitrate support

`frameheader.Read` (`frameheader.go:302-305`) rejects BitrateIndex==0 as a fatal error.
Free bitrate MP3 frames exist in the wild. When the sync scanner encounters one
(real or false sync), it returns a hard error instead of continuing to scan.

**Triggered by**: 77 White.mp3 — malformed ID3 tag leaves 3840 bytes of orphaned
tag data between declared tag end and first audio frame. Scanner finds false sync
`0xffff0300` (MPEG1 Layer I, free bitrate) at position 37370. Real audio starts at 41209.

## 2. No Layer I/II support — fatal error instead of sync recovery

`frame.Read` (`frame.go:84-85`) rejects Layer!=Layer3 as a fatal error.
This decoder only supports Layer III, which is fine, but when the sync scanner
finds a false sync with Layer I/II bits, it should continue scanning instead of dying.

**Triggered by**: Part 3.mp3 — after 29018 valid Layer III frames, ~13.5KB of
trailing metadata (APE/LYRICS3 tags + ID3v1) follows. The sync scanner slides
755 bytes into this trailing data and finds false sync `0xfff6da33` (MPEG2 Layer I)
at position 30403784. Fatal error kills `ensureFrameStartsAndLength()`.

## 3. No MPEG 2.5 support — fatal error instead of sync recovery

`frame.Read` (`frame.go:80-81`) rejects Version2.5 as a fatal error.
Same problem as #2 — should continue scanning, not die.

## 4. `skipTags` is naive

`source.go:skipTags()` only handles ONE tag at file start (either "ID3" or "TAG").
Problems:
- Doesn't handle malformed tag sizes (tag data extending beyond declared boundary)
- Doesn't handle multiple consecutive tags
- No awareness of trailing tags (ID3v1, APE, LYRICS3 at end of file)
- No handling of padding between tag and first audio frame

## 5. No trailing metadata handling

`ensureFrameStartsAndLength()` (`decode.go:172`) scans frames to EOF with no
concept of "end of audio data." After the last valid frame, trailing metadata
(ID3v1 at filesize-128, APE tags, LYRICS3 tags) gets fed to the sync scanner
which can find false syncs and die.

## 6. Sync recovery is fragile

The sync scanner in `frameheader.Read` (`frameheader.go:280-297`) loops while
`!header.IsValid()`. Once a header passes `IsValid()`, the function exits —
either returning the header or a fatal error. There is no retry mechanism.

`IsValid()` checks: sync word, version!=reserved, bitrateIndex!=15,
samplingFrequency!=reserved, layer!=reserved, emphasis!=2. But it does NOT check:
- BitrateIndex==0 (free format) — checked later, fatal error
- Layer!=Layer3 — checked in frame.Read, fatal error
- Version==2.5 — checked in frame.Read, fatal error

Any false sync passing `IsValid()` that fails these later checks kills the decoder.