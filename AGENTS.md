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
Progress is tracked in `TODO.md` — update it when you finish a module.

## Layout (intentional)

Everything is one package `lofty` at the repo root. Format code shares many
unexported helpers, so files stay grouped by name prefix, not directories:

| Prefix | Rust counterpart | Contents |
|---|---|---|
| `config.go`, `errors.go`, `option.go` | `config/`, `error.rs` | ParseOptions/WriteOptions/GlobalOptions, error types, `Option[T]` |
| `util_*.go` | `util/{alloc,io,text}.rs` | alloc limits, FileLike/Splice, text codecs (UTF-8/16, Latin-1) |
| `picture.go`, `properties.go`, `timestamp.go`, `popularimeter.go` | same-named items | shared models |
| `tag_*.go` | `tag/`, `id3/`, `ape/tag`, `ogg/tag`, `iff/*/tag` | generic `Tag`/`TagItem`/`ItemKey` model + format tag impls |
| `ilst.go` | `mp4/ilst/` | first-class `Ilst`/`Atom`/`AtomData` model |
| `file_*.go` | `file/`, `probe.rs`, `resolve.rs` | FileType, Probe, TaggedFile, resolvers |
| `format_*.go` | `<format>/` | per-format file readers/writers |
| `*_test.go` | Rust `tests/` + `src/**/tests.rs` | ported test cases |

## Dependencies (deliberate — reuse over reimplement)

- `github.com/bogem/id3v2/v2` — ID3v2 frame parse/write
- `github.com/go-flac/go-flac/v2` — FLAC metadata blocks

MP4 box traversal, Ogg page read/write, and Latin-1/UTF-16 codecs are all
hand-rolled in `format_mp4.go`, `format_ogg.go`, and `util_text.go`.

Manual code is used wherever Lofty semantics diverge from the dependency
(e.g. `id3v2TextFrameNoTerm`, `popmFramer`, `urlFramer`, `wxxxFramer` custom
Framers in `tag_id3v2.go`, `id3v2ManualDescFrame` for description-terminated
UTF-16 frames bogem corrupts, and the whole Ogg page-rebuild writer).

## Port-parity rules that matter

When porting Rust behavior, mirror these exactly — they were the source of
most debugging:

- **Companion tags.** `Tag` carries an unexported `companionTag` (`any`).
  Split/merge per format: on read, unconvertible frames stay in the
  companion (`PreserveFormatSpecificItems` global option); on write they
  are replayed. Patterns to copy: `vorbisCommentsSplitTag` (tag_vorbis.go),
  `Ilst.Tag()`/`IlstFromTag` (ilst.go), ID3v2 binary items (`tag_id3v2.go`).
- **Merge vs dedup on write** (Rust `from_tag` + `Frame` eq semantics):
  multi-valued keys merge `\0` into one frame; single-valued keys dedup
  by ID first-wins. See `id3v2MultiTxxxKeys`/`id3v2SingleTxxxKeys`/
  `id3v2SingleTextKeys`/`langMerge`/`textMerge`/`seenFrame` in
  `genericTagToBogem`. POPM dedups by email, UFID by owner, TXXX by desc.
- **N/T pair frames** (TRCK/TPOS/MVIN): `splitn(2, ['\0','/'])`, trim each,
  both must parse as u32 else the whole frame is retained (`splitNumPair`).
  Write uses `0/total` when only a total exists.
- **Strict vs relaxed**: `ParsingModeStrict/BestAttempt/Relaxed` must
  change failure-vs-skip behavior, not just error wrapping.
- **Trailing NULs / BOM**: strip per-encoding; each `\0`-separated segment
  may carry its own UTF-16 BOM (`utf16DecodeBytes` strips all of them).
- **No trailing text terminator**: bogem appends one; Rust doesn't. Use
  `id3v2TextFrameNoTerm` — this is required for byte-identical round-trips.
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

## Fixture paths

- General: `lofty-rs/lofty/tests/files/assets/`
- Tag assets: `lofty-rs/lofty/tests/tags/assets/` (e.g. `id3v2/`, `apev2/`)
- TagLib corpus: `lofty-rs/lofty/tests/taglib/data/`
- ID3v1: `lofty-rs/lofty/tests/tags/id3v1/assets/` (+ `generation.log`)

## Known gaps (see TODO.md)

None currently — the former ID3v2 bogem-delegation gaps were closed by a
manual v2.3/v2.4 frame walker (`id3v2SanitizeTag` in `tag_id3v2.go`,
mirroring Rust `ParsedFrame::read`) plus `id3v2Serialize` (empty-tag
headers + `PreferredPadding`). MP4 writes update `stco`/`co64`/`tfhd`
chunk offsets via `mp4UpdateChunkOffsets` in `format_mp4.go` (Rust
`AtomWriterCompanion::update_offsets` parity).

## Conventions

- Match Rust names where possible: `Option[T]`, `Some/None`, `IsSome/
  IsNone`, `ok bool` returns, `XxxFromStr` parsers, `TagType*` constants.
- Error types mirror `LoftyError`/`FileParseError`/`TagError` in errors.go.
- No doc files beyond README/AGENTS/TODO unless asked.
- Don't commit test binaries or `dbg_*` files.
