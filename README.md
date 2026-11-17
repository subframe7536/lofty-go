# lofty-go

A Go port of [lofty-rs](https://github.com/Serial-ATA/lofty-rs) — read and
write metadata for audio files.

Supports the same generic tag model as Lofty: a format-agnostic
`Tag`/`TagItem`/`ItemKey` layer over format-specific tags, with unknown
metadata preserved through *companion tags* on conversion.

**Requires Go 1.27+**

```bash
go get github.com/subframe7536/lofty-go
```

## Quick start

```go
import "github.com/subframe7536/lofty-go"

// Read: one call gives you file type, properties, and all tags.
f, err := lofty.ReadFromPath("song.mp3", lofty.NewParseOptions())
if err != nil {
	log.Fatal(err)
}
fmt.Println(f.FileType())                // e.g. FileTypeMpeg
fmt.Println(f.Properties().Duration())   // audio properties

tag := f.PrimaryTag()                  // may be nil if the file has no tag
if tag != nil {
	title, _ := tag.Title()            // accessors return (value, ok)
	artist, _ := tag.Artist()
	fmt.Println(title, artist)

	// Write: mutate the generic tag, save back to the same file.
	tag.InsertText(lofty.ItemKeyTrackArtist, "New Artist")
	if err := tag.SaveToPath("song.mp3", lofty.NewWriteOptions()); err != nil {
		log.Fatal(err)
	}
}
```

Everything lives under `lofty.` — format packages exist for advanced use
(e.g. `id3v2.Tag`, `mp4.Ilst`), but the root facade re-exports the whole
public API and activates all format registrations on import.

## Configuration

Lofty supports process-wide configuration via `lofty.ApplyGlobalOptions`:

```go
opts := lofty.NewGlobalOptions()
opts.AllocationLimit = 32 * 1024 * 1024 // 32 MiB allocation limit per item
lofty.ApplyGlobalOptions(opts)
```

Unlike Rust (which uses thread-local options), Go's `GlobalOptions` are process-global
and guarded by a mutex. Call `ApplyGlobalOptions` during application initialization
(`main()` or `init()`) before performing concurrent parse or write operations.

## Supported formats

| Format | Tags | Read | Write |
|---|---|---|---|
| MP3 (MPEG) | ID3v1, ID3v2 (2.2/2.3/2.4), Lyrics3 v1/v2, APEv2 | ✅ | ✅ |
| FLAC | Vorbis comments, pictures, ID3v2 | ✅ | ✅ |
| Ogg Vorbis / Opus / Speex | Vorbis comments | ✅ | ✅ |
| MP4 / M4A | `ilst` atoms (incl. freeform `----`) | ✅ | ✅ |
| WAV (RIFF) | RIFF INFO, ID3v2 chunk | ✅ | ✅ |
| AIFF | text chunks, COMT comments, ID3v2 | ✅ | ✅ |
| APE | APEv2, ID3v1 | ✅ | ✅ |
| ADTS/AAC | ID3v2 | ✅ | ✅ |
| DSF | ID3v2 | ✅ | ✅ |
| Musepack (SV7–SV8) | APEv2 | ✅ | ✅ |
| WavPack | APEv2 | ✅ | read |

Audio properties (duration, bitrate, sample rate, channels, bit depth,
codec details) are parsed per format.

## Core API

| Entry point | Purpose |
|---|---|
| `lofty.ReadFromPath(path, opts)` / `ReadFrom(r, opts)` | probe + parse a file → `*TaggedFile` |
| `lofty.NewProbe(r).GuessFileType()` / `FileTypeFromPath(p)` | sniff file type without full parse |
| `TaggedFile.PrimaryTag() / Tag(tagType) / Tags()` | access tags by precedence or type |
| `Tag.InsertText(key, s)` / `GetString(key)` / `Items()` | generic item access via `ItemKey*` |
| `Tag.SaveToPath(path, opts)` | write one tag back to a file |
| `lofty.BoundTaggedFileFrom(file, opts)` → `btf.Save(opts)` | bound write (file + tags together) |
| `lofty.SaveToPath(path, f, tagType, opts)` / `RemoveTagFromPath` | AudioFile-level write/remove |
| `lofty.NewParseOptions() / NewWriteOptions()` | strict/relaxed parsing, padding, etc. |

`Option[T]` mirrors Rust's `Option` (`Some`/`None`/`IsSome`/`IsNone`);
`Timestamp`, `Popularimeter`, `Picture`, `ChannelMask`, `Lang` mirror the
corresponding Rust item types.

## Package layout

`image`-style tree — a root facade over public format packages and
internal shared layers:

```
lofty-go (package lofty)   facade: re-exports the full public API
├── internal/util          Option[T], options, errors, text codecs, IO
├── internal/meta          Tag/TagItem/TaggedFile/FileType/Probe + dispatch
├── internal/tagpos        shared ID3v1 tail locator
├── id3v1 id3v2 vorbis lyrics3        tag implementations
└── ape ogg flac mp4 iff mpeg dsf wavpack mpc aac   format readers/writers
```

Format packages self-register in `init()`; importing `lofty` activates
them all. `facade_gen.go` is generated — regenerate after adding exported
symbols to a subpackage with `go run ./scripts/facadegen`.

## Dependencies

Only one, deliberate (`go.mod`):

- `github.com/bogem/id3v2/v2` — test-only ID3v2 parse oracle (production read/write is the native `id3v2` package)

MP4 atom traversal, Ogg page read/write, and Latin-1/UTF-16 codecs are
hand-rolled where Lofty semantics diverge from off-the-shelf libraries
(companion tags, merge/dedup rules, strict vs. relaxed parsing, padding).

## Status

Feature-complete port of readers/writers and the generic tag API, with
~400 ported Rust test cases passing. Progress, coverage, and known parity
notes are tracked in [TODO.md](TODO.md). Agent/developer notes — build
commands, layout rules, port-parity gotchas — live in
[AGENTS.md](AGENTS.md).

Upstream: <https://github.com/Serial-ATA/lofty-rs> (vendored reference
copy in `lofty-rs/`; test fixtures are reused from that tree, never
copied).
