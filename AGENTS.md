# AGENTS.md — lofty-go

Go port of [lofty-rs](https://github.com/Serial-ATA/lofty-rs) (audio metadata
library). The vendored Rust source lives in `lofty-rs/` and is the ground
truth for semantics. Test fixtures are **reused** from the Rust tree —
never copy them.

## Build / test

```bash
go build ./...          # single package at repo root
go vet ./...
go test ./...           # ~15s; all tests in package lofty
go test -run TestX -v ./  # focused test

# Quality tooling (all live in $GOPATH/bin)
golangci-lint run ./... # .golangci.yml: standard linters + staticcheck,
                        # misspell, unparam, unconvert, perfsprint,
                        # copyloopvar, usestdlibvars; gofumpt+goimports fmt
golangci-lint fmt ./... # auto-fix formatting (gofumpt is stricter gofmt)
gopls check ./*.go      # language-server diagnostics
staticcheck ./...
govulncheck ./...
```

No codegen, no cgo. Module: `github.com/subframe7536/lofty-go` (Go 1.27+).

## Layout

The repo follows the Go-idiomatic `image`-style layout: a root facade plus
public format packages over internal shared layers. The public API is
unchanged — `lofty.X` resolves through generated aliases.

| Path | Rust counterpart | Contents |
|---|---|---|
| `internal/util` | `util/{alloc,io,text}.rs`, `config/` | `Option[T]`, Global/Parse/WriteOptions, error types, alloc limits, FileLike/Splice, text codecs, synchsafe/frame-sync helpers |
| `internal/meta` | `tag/`, `file/`, `probe.rs`, `resolve.rs` | `Tag`/`TagItem`/`ItemKey`, `TaggedFile`, `FileType`, `Probe`, format/tag registries, read/write dispatch, `verifiedFile`. Internal tests are `package meta`; ported facade-level tests of `Tag`/`Picture`/resolvers are `package meta_test` alongside them — they must NOT import `lofty`, `mpeg` or `flac`, because `helpers_test.go` init-registers stub readers/writers for MPEG+FLAC and the real registrations would panic on the duplicate |
| `internal/tagpos` | — | ID3v1 tail-locator shared by `id3v1`/`lyrics3`/`mpeg` (breaks the id3v1↔lyrics3 cycle) |
| `internal/testutil` | — | fixture path resolution + shared test helpers; tests live in the package whose internals they exercise |
| `id3v1`, `id3v2`, `vorbis`, `lyrics3` | `id3/v1`, `id3/v2`, `ogg/tag`, `id3/lyrics3` | tag implementations (`id3v2` is a native port: `tag.go` tag+header, `read.go`/`write.go` frame walk+serialize, `frame.go`/`framelist.go`/`items*.go` typed frames, `convert.go` generic-Tag split/merge) |
| `ape`, `ogg`, `flac`, `mp4`, `iff`, `mpeg`, `dsf`, `wavpack`, `mpc`, `aac` | `<format>/` | file readers/writers; `mp4` includes `Ilst`/`Atom`/`AtomData` |
| root (`package lofty`) | — | **pure facade**: `facade_gen.go` re-exports every moved public symbol (regenerate with `go run ./scripts/facadegen`); no test files at root |
| `tests/` (`package lofty_test`) | Rust `tests/` | black-box integration tests: file read/write dispatch, TagLib corpus, fuzz/bench. Dot-imports the facade so ported call sites stay unqualified; `shim_util_test.go` holds internal bridges |
| `*_test.go` | Rust `tests/` + `src/**/tests.rs` | ported test cases, distributed to their owning package; format-specific ports are external `<pkg>_test` files dot-importing the facade (e.g. `id3v2/id3v2_header_test.go`) |

Dependency rule: `internal/util` ← `internal/meta` ← format packages ← root
facade. Format packages never import the root; they register themselves in
`init()` via `meta.RegisterFormatReader`/`RegisterFormatWriter`/
`RegisterTagDumper`, so `import lofty` activates every format.

## Dependencies (deliberate — reuse over reimplement)

- `github.com/bogem/id3v2/v2` — test-only parse oracle (`bogemFrames` in
  tests); production ID3v2 read/write is the native `id3v2` package

MP4 box traversal, Ogg page read/write, FLAC metadata blocks (`flac/block.go`),
and Latin-1/UTF-16 codecs are all hand-rolled in the `mp4`, `ogg`, and `internal/util` packages.

## Port-parity rules that matter

When porting Rust behavior, mirror these exactly — they were the source of
most debugging:

- **Companion tags.** `Tag` carries an unexported `companionTag` (`any`).
  Split/merge per format: on read, unconvertible frames stay in the
  companion (`PreserveFormatSpecificItems` global option); on write they
  are replayed. Patterns to copy: `vorbisCommentsSplitTag` (vorbis pkg),
  `Ilst.Tag()`/`IlstFromTag` (mp4 pkg), `Id3v2Tag` remainder
  (`id3v2/convert.go`).
- **Merge vs dedup on write** (Rust `from_tag` + `Frame` eq semantics):
  multi-valued keys merge `\0` into one frame; single-valued keys dedup
  by ID first-wins. See `id3v2/convert.go` (`multiTxxxKeys`/
  `langMerge`/`textMerge`/`seenFrame` etc.). POPM dedups by email, UFID by
  owner, TXXX by desc, COMM/USLT by lang+desc.
- **N/T pair frames** (TRCK/TPOS/MVIN): `splitn(2, ['\0','/'])`, trim each,
  both must parse as u32 else the whole frame is retained (`splitNumPair`).
  Write uses `0/total` when only a total exists.
- **Strict vs relaxed**: `ParsingModeStrict/BestAttempt/Relaxed` must
  change failure-vs-skip behavior, not just error wrapping.
- **Trailing NULs / BOM**: strip per-encoding; each `\0`-separated segment
  may carry its own UTF-16 BOM (`utf16DecodeBytes` strips all of them).
- **No trailing text terminator**: Rust doesn't write one after the last
  text value; the native writer matches so round-trips are byte-identical.
- **Latin-1 byte 0xFF**: rejected/replaced exactly like Rust (see
  `latin1Encode` use in lyrics3/ape paths).
- **Empty vendor**: `VorbisCommentsTag` default vendor is `""` (Rust
  `VorbisComments::new()`), not a lofty string — vendor→EncoderSoftware
  injection only happens for parsed files.
- **APEv2 flags**: header `0xE0` (IS_HEADER|CONTAINS_FOOTER|CONTAINS_HEADER),
  footer `0xC0`.
- **Sizes**: `Id3v2Header.Size` is content size (excludes the 10-byte
  header); AAC `stream_len` in properties subtracts ID3v2 content size only
  (Rust quirk); APE `file_length` is the reader position after the trailing
  tag attempt, not EOF; MP4 `dfLa` payload contains a 4-byte FLAC block
  header before STREAMINFO.
- **APE remaining size**: Go subtracts `8 + len(key) + valueSize` per item
  loop iteration (actual item size); Rust `ape/tag/read.rs` only subtracts 4
  (`value_size`), which is an upstream oversight that miscounts remaining
  bytes. Go's strict accounting is an intentional deviation for robustness.
- **Ogg page errors are two-tiered** (`oggHeaderError`): header/segment-table
  failures end packet scans (`Packets::read_count`'s `while let Ok`) and are
  the only retry candidates in `find_last_page`; body errors abort. The
  comment packet's vendor is the file's — on write it always overrides the
  tag's own vendor/`EncoderSoftware`; truncated vendor = hard error,
  invalid UTF-8 → `""`.
- **Vorbis values**: trailing NULs trimmed on read (`utf8_decode_str`),
  including TRACKNUMBER/DISCNUMBER before the `n/total` split. Unknown-key
  items are dropped in generic→concrete conversion (`map_key` → None), not
  stored under their description.

## Intentional deviations from Rust (reviewed; do not "fix")

- **Content sniffing:** `ReadFromPath` falls back to content detection when the
  extension is unknown; Rust returns an error. Accounts for the differential
  diffs on `empty_vorbis.oga`, `no-tags.3g2`, `id3v2/test.atxt`,
  `id3v2/test.geob`, `id3v2/trailing_nulls.id3v24`.
- **ID3v2:** `verifyFrame` accepts OWNE/ETCO and the Go-only SYLT/GEOB/ATXT
  frames;
  COMM/USLT descriptions apply to every NUL-split item; nested CHAP/CTOC depth
  is capped at 8 and decompressed frames are bounded by `AllocationLimit`.
  `Locator` items are written to URL frames.
- **MP4:** UTF-16 atoms are written as real UTF-16BE; `dfLa` reads a fixed
  18-byte STREAMINFO; after a non-strict bad `data` child later children are
  not pre-read.
- **APE:** generic-Tag conversion routes `Cover Art` items through pictures,
  merges same-key text with NUL, merges Track/Disc pairs on write, and outputs
  items, then pairs, then pictures. After a leading APE tag Go keeps scanning
  for the MAC header (Rust effectively errors).
- **IFF:** after an invalid RIFF INFO FourCC Go seeks to the list end (Rust
  leaves the reader mid-list).
- **Ogg:** `SaveFile` with no `VorbisComments` is a no-op. `MulDivRound`
  saturates where Rust truncates `u128 as u64`.
- **FLAC:** `InsertPicture` returns only `error`; `InsertPictureReplacing`
  also returns the replaced picture.
- **Lyrics3:** generic `Tag.DumpTo` writes Lyrics3/Lyrics3v2 data (Rust writes
  nothing).

## Fixture paths

- General: `lofty-rs/lofty/tests/files/assets/`
- Tag assets: `lofty-rs/lofty/tests/tags/assets/` (e.g. `id3v2/`, `apev2/`)
- TagLib corpus: `lofty-rs/lofty/tests/taglib/data/`
- ID3v1: `lofty-rs/lofty/tests/tags/id3v1/assets/` (+ `generation.log`)

## Known gaps

ID3v2 is a full native port — `Id3v2Tag`/`FrameList`/`Frame`/`FrameId`/
`FrameFlags`, typed frames, v2.2/v2.3/v2.4 headers, footer/CRC/
restrictions, extended headers, unsynchronisation, v2.3↔v2.4 conversion,
`Id3v2TagFlags`-aware writing (`create_tag`/`create_tag_v3` in
`id3v2/write.go`). MP4 writes update `stco`/`co64`/`tfhd` chunk offsets
(Rust `AtomWriterCompanion::update_offsets` parity). MP4 write does not
currently support promoting 32-bit boxes to 64-bit extended boxes when
metadata crosses 4 GiB; attempting to write boxes >4 GiB returns
`TooMuchDataError`.

## Conventions

- Match Rust names where possible: `Option[T]`, `Some/None`, `IsSome/
  IsNone`, `ok bool` returns, `XxxFromStr` parsers, `TagType*` constants.
- Error types mirror `LoftyError`/`FileParseError`/`TagError` in errors.go.
- No doc files beyond README/AGENTS/TODO unless asked.
- Don't commit test binaries or `dbg_*` files.
