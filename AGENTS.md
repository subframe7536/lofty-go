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

## Layout

The repo follows the Go-idiomatic `image`-style layout: a root facade plus
public format packages over internal shared layers. The public API is
unchanged — `lofty.X` resolves through generated aliases.

| Path | Rust counterpart | Contents |
|---|---|---|
| `internal/util` | `util/{alloc,io,text}.rs`, `config/` | `Option[T]`, Global/Parse/WriteOptions, error types, alloc limits, FileLike/Splice, text codecs, synchsafe/frame-sync helpers |
| `internal/meta` | `tag/`, `file/`, `probe.rs`, `resolve.rs` | `Tag`/`TagItem`/`ItemKey`, `TaggedFile`, `FileType`, `Probe`, format/tag registries, read/write dispatch, `verifiedFile` |
| `internal/tagpos` | — | ID3v1 tail-locator shared by `id3v1`/`lyrics3`/`mpeg` (breaks the id3v1↔lyrics3 cycle) |
| `internal/testutil` | — | fixture path resolution + shared test helpers; tests live in the package whose internals they exercise |
| `id3v1`, `id3v2`, `vorbis`, `lyrics3` | `id3/v1`, `id3/v2`, `ogg/tag`, `id3/lyrics3` | tag implementations (`id3v2` owns the bogem integration + `genericTagToBogem`) |
| `ape`, `ogg`, `flac`, `mp4`, `iff`, `mpeg`, `dsf`, `wavpack`, `mpc`, `aac` | `<format>/` | file readers/writers; `mp4` includes `Ilst`/`Atom`/`AtomData` |
| root (`package lofty`) | — | **pure facade**: `facade_gen.go` re-exports every moved public symbol (regenerate with `go run ./scripts/facadegen`); `shim_util.go` holds internal bridges for root tests |
| `*_test.go` | Rust `tests/` + `src/**/tests.rs` | ported test cases, distributed to their owning package |

Dependency rule: `internal/util` ← `internal/meta` ← format packages ← root
facade. Format packages never import the root; they register themselves in
`init()` via `meta.RegisterFormatReader`/`RegisterFormatWriter`/
`RegisterTagDumper`, so `import lofty` activates every format.

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
