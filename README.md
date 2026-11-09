# lofty-go

A Go port of [lofty-rs](https://github.com/Serial-ATA/lofty-rs) — read and
write metadata for audio files.

Supports the same generic tag model as Lofty: a format-agnostic
`Tag`/`TagItem`/`ItemKey` layer over format-specific tags, with unknown
metadata preserved through *companion tags* on conversion.

## Supported formats

| Format | Tags | Read | Write |
|---|---|---|---|
| MP3 (MPEG) | ID3v1, ID3v2 (2.2/2.3/2.4), Lyrics3 v1/v2, APEv2 | ✅ | ✅ |
| FLAC | Vorbis comments, pictures, ID3v2 | ✅ | ✅ |
| Ogg Vorbis / Opus / Speex | Vorbis comments | ✅ | ✅ |
| MP4 / M4A / AAC-in-MP4 | `ilst` atoms (incl. freeform `----`) | ✅ | ✅ |
| WAV (RIFF) | RIFF INFO, ID3v2 chunk | ✅ | ✅ |
| AIFF | text chunks, COMT comments, ID3v2 | ✅ | ✅ |
| APE | APEv2, ID3v1 | ✅ | ✅ |
| ADTS/AAC | ID3v2 | ✅ | ✅ |
| DSF | ID3v2 | ✅ | ✅ |
| Musepack (SV4–SV8) | APEv2 | ✅ | ✅ |
| WavPack | APEv2 | ✅ | read |

Audio properties (duration, bitrate, sample rate, channels, bit depth,
codec details) are parsed per format.

## Example

```go
f, err := lofty.ReadFromPath("song.mp3", lofty.NewParseOptions())
if err != nil { log.Fatal(err) }

tag := f.PrimaryTag()
fmt.Println(tag.Title(), tag.Artist(), tag.Album())
fmt.Println(f.Properties().Duration())

tag.InsertText(lofty.ItemKeyTrackArtist, "New Artist")
if err := f.SaveFile(lofty.NewWriteOptions()); err != nil { log.Fatal(err) }
```

## Design notes

- Single package `lofty` at the module root; files are grouped by name
  prefix (format readers share unexported helpers).
- Mature libraries are used where they match Lofty semantics
  (`bogem/id3v2`, `go-flac`, `abema/go-mp4`, `mccoyst/ogg`); Lofty-specific
  behavior (companion tags, merge/dedup rules, strict vs. relaxed parsing,
  padding) is implemented on top.
- `Option[T]` mirrors Rust's `Option`; `Timestamp`, `Popularimeter`,
  `Picture`, `ChannelMask`, `Lang` mirror Rust's item types.

## Status

Feature-complete port of readers/writers and the generic tag API, with
~400 ported Rust test cases passing. Progress, coverage, and the handful
of known parity gaps are tracked in [TODO.md](TODO.md). Agent/developer
notes live in [AGENTS.md](AGENTS.md).

Upstream: https://github.com/Serial-ATA/lofty-rs (vendored reference copy
in `lofty-rs/`).
