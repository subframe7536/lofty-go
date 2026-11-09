# lofty-go — Port Tracker

Go port of [lofty-rs](https://github.com/Serial-ATA/lofty-rs) (audio metadata
library). Reference source lives in `lofty-rs/`; test assets are reused from
`lofty-rs/lofty/tests/files/assets/`.

## Layout

`image`-style package tree: a root facade plus public format packages over
internal shared layers. The public API is unchanged — `lofty.X` resolves
through generated aliases (see `AGENTS.md` for the dependency rule).

| Path                     | Rust counterpart                          | Contents |
|--------------------------|--------------------------------------------|----------|
| `internal/util`          | `util/{alloc,io,text}.rs`, `config/`       | `Option[T]`, options, errors, alloc, FileLike/Splice, text codecs |
| `internal/meta`          | `tag/`, `file/`, `probe.rs`, `resolve.rs`  | `Tag`/`TagItem`/`ItemKey`, `TaggedFile`, `FileType`, `Probe`, registries, read/write dispatch |
| `internal/tagpos`        | —                                          | ID3v1 tail locator (breaks id3v1↔lyrics3 cycle) |
| `internal/testutil`      | —                                          | fixture path + shared test helpers |
| `id3v1`,`id3v2`,`vorbis`,`lyrics3` | `id3/v1`, `id3/v2`, `ogg/tag`, `id3/lyrics3` | tag implementations |
| `ape`,`ogg`,`flac`,`mp4`,`iff`,`mpeg`,`dsf`,`wavpack`,`mpc`,`aac` | `<format>/` | file readers/writers; `mp4` includes `Ilst`/`Atom` |
| root (`package lofty`)   | —                                          | pure facade: `facade_gen.go` (regen: `go run ./scripts/facadegen`) + `shim_util.go` |
| `*_test.go`              | Rust `tests/` + `src/**/tests.rs`          | ported tests, distributed to owning package |

## Format status

| Format    | Read | Write | Rust reference        | Test parity                     |
|-----------|:----:|:-----:|------------------------|---------------------------------|
| MPEG (MP3)|  ✅  |  ✅   | `mpeg/`                | `files/mpeg.rs` — 6 tests ✅    |
| ID3v1     |  ✅  |  ✅   | `id3/v1/`              | covered by mpeg/ape tests       |
| ID3v2     |  ✅  |  ✅   | `id3/v2/` (bogem)      | `id3v2_tag_test.go` — 45+ pass, no skips |
| Lyrics3   |  ✅  |  ✅   | `id3/lyrics3/`         | `lyrics3_test.go` — 13 ✅      |
| APE file  |  ✅  |  ✅   | `ape/`                 | `files/ape.rs` — 2 tests ✅     |
| APE tag   |  ✅  |  ✅   | `ape/tag/`             | covered by mpeg/ape tests       |
| VorbisComments |  ✅  |  ✅   | `vorbiscomments` shared| vendor string ↔ EncoderSoftware |
| FLAC      |  ✅  |  ✅   | `flac/` (go-flac)      | 5 tests ✅ (strict multi-VC, vendor, write) |
| Ogg Vorbis|  ✅  |  ✅   | `ogg/` (manual pager)  | read+write ✅                   |
| Ogg Opus  |  ✅  |  ✅   | `ogg/`                 | read+write ✅                   |
| Ogg Speex |  ✅  |  ✅   | `ogg/`                 | read+write ✅                   |
| MP4/ilst  |  ✅  |  ✅   | `mp4/` (manual atoms)  | `ilst_test.go` — 23 ✅          |
| WAV       |  ✅  |  ✅   | `iff/wav/`             | read ✅; RIFF INFO + id3 chunk writers |
| AIFF      |  ✅  |  ✅   | `iff/aiff/`            | read ✅; text-chunk + ID3 writers |
| AAC/ADTS  |  ✅  |  ✅   | `aac/`                 | 2 tests ✅ incl. junk regression |
| DSF       |  ✅  |  ✅   | `dsf/`                 | read ✅; ID3v2-at-EOS writer     |
| Musepack  |  ✅  |  ✅   | `musepack/`            | SV7+SV8 read ✅                  |
| WavPack   |  ✅  |  ✅   | `wavpack/`             | read ✅                          |

Legend: ✅ done · 🚧 in progress · ⬜ not started

## Core checklist

- [x] Errors (FileParse/FileEncoding/Tag errors)
- [x] Config: ParseOptions, WriteOptions, GlobalOptions
- [x] Util: alloc limits, FileLike/Splice/Truncate, UTF-8/16/Latin-1 text codecs
- [x] Picture + MIME detection (PNG/JPEG/GIF/BMP/TIFF)
- [x] FileProperties + ChannelMask
- [x] Tag/TagItem/ItemValue/ItemKey + all format key maps
- [x] Accessor, Timestamp, Popularimeter, Lang
- [x] FileType detection, Probe, resolvers, TaggedFile, BoundTaggedFile
- [x] AudioFile interface, save/remove write paths
- [x] go build + go vet + go test clean

## Test porting status

| Rust test file              | Go tests | Status |
|-----------------------------|----------|--------|
| `files/mpeg.rs`             | 6        | ✅ passing (read, junk-between-frames, issue_82, issue_87, write, save_to_id3v2) |
| `files/ape.rs`              | 4        | ✅ passing (read, write, remove APEv2/ID3v1) |
| `files/flac.rs`             | 9        | ✅ passing (multi-VC strict/best-attempt, no-props/no-tags, vendor, write, remove VC, stream-info-last, app-block-last, id3v2) |
| `files/ogg.rs`              | 8        | ✅ passing (opus/vorbis/speex read+write, remove VC, issue_37, issue_499) |
| `files/mp4.rs`              | 8        | ✅ passing (+ extended atom sizes preserved on write) |
| `files/wav.rs`/`aiff.rs`    | 8        | ✅ passing (reads, writes, removes, empty id3v2 ×2, zero-size no-props) |
| `files/aac.rs`/`dsf.rs`/`mpc.rs`/`wavpack.rs` | 9 | ✅ passing (reads, junk-between, dsf write/remove, mpc/wv removes, zero-size) |
| `files/zero_sized.rs`       | 7        | ✅ passing (strict err + no-props ok; vorbis/opus/speex invalid per Rust) |
| `id3/lyrics3/v2/tests.rs`   | 4        | ✅ passing (write, remove, requires-id3v1, strip-with-id3v1) |
| `files/resize.rs`           | 1        | ✅ passing (21 subtests, ffmpeg-gated) |
| `tag/tests.rs`, conversions | 10       | ✅ accessor tests + supported-items consistency (aiff/ape/id3v1/id3v2/ilst/riffinfo/vorbis) + issue_37 ogg write |
| `picture/tests.rs`          | 8        | ✅ mime detection, PNG/JPEG info, APE/FLAC byte round-trips, extractors |
| `taglib/*` property/robustness | ~35   | ✅ mpeg (xing/vbri/cbr props, garbage, dup id3v2, fuzzed), wav, aiff, ape (399/396/390), mp4 (aac/alac), wavpack (dsd/non-standard-rate/fuzzed), mpc (sv7/sv8), dsf, ogg (props+page CRC), vorbis, flac, aac — Rust-`#[ignore]`d cases skipped |
| `taglib/test_flac.rs` pics  | 4        | ✅ `FlacPicture{Picture, Info}` pairs, insert/replace/remove pictures, stored PictureInformation |
| `taglib/test_flac.rs` rest  | 6        | ✅ repeated save, multi-values, invalid keys, zero-pad 2, remove xiph field, empty id3v2, empty seektable |
| `taglib/test_xiphcomment.rs`| 8        | ✅ raw-key API (`Get`/`GetAll`/`InsertKV`/`PushKV`/`RemoveKey`), accessors, VC picture pairs, lowercase fields |
| `taglib/test_apetag.rs`     | 3        | ✅ ApeItem key validation, IsEmpty, `ApeTag` accessor/number-pair/date API |
| `taglib/test_id3v1.rs`      | 1        | ✅ genres table |
| `taglib/test_ogg/opus` rest | 5        | ✅ simple write, split-packet comments (60k), Opus read/write comments, vendor retained |
| `taglib/test_flacpicture.rs`| 2        | ✅ parse + byte-exact as_flac_bytes pass-through |
| `ogg/tag.rs`                | 15       | ✅ parse test.vorbis, re-read, tag↔VC both ways, multi-value round-trip, zero.vorbis, issue_60, INITIALKEY/KEY, skip cover art, implicit-conv on/off (track+disc pairs), leading-0, companion round-trip |
| `properties/tests.rs`       | 20       | ✅ all format property fixtures (aac/aiff/ape/dsf/flac/mpeg×3/mp4×4/mpc×3/opus/speex/vorbis/wav/wavpack) — AAC avg-bitrate+validated-frame model, APE position-quirk file_length, MP4 dfLa block header, MPC SV4-6 fallback + SV8 packet-sum stream_length |
| `util/io.rs`                | 6        | ✅ rev-search-from-current, replace-range, save-to-file/vec byte-identical (APE flag bits + no text terminator) |
| `util/*` (math/text/alloc)  | ~20      | ✅ div_round, f80, synchsafe encode/decode, pairs via ApeTag.Insert, alloc limits |
| `timestamp.rs`              | 10       | ✅ all Timestamp parse/format/validation cases |
| `probe.rs`                  | ~30      | ✅ content+extension file-type detection for all formats |
| `resolve.rs`                | 3        | ✅ custom resolver register/dup-panic/read |
| `mpeg/header.rs`            | ~10      | ✅ header parse, rev-search, sync verification |
| `popm conversions`          | 4        | ✅ Popularimeter round-trip via id3v2/vorbis/riffinfo/ilst |
| `tag/item.rs` maps          | 2        | ✅ ItemKey↔format key maps exhaustive |
| `ape/tag/mod.rs`            | 18       | ✅ parse/re-read, tag↔ape, track/disc N-T pair semantics, skip cover art, accessor suite |
| `id3/v1/tag.rs`             | 3        | ✅ Id3v1Tag `Date`/`SetDate`/`RemoveDate`/`DumpTo`, year round-trip, lossy Latin-1, truncation, item consistency |
| `iff/wav/tag` + `iff/aiff/tag` | 15    | ✅ RiffInfo parse/re-read/`DumpTo`/companion round-trip; AIFF COMT comments (timestamp+marker), padded/unpadded/invalid-padding modes, text-chunk `DumpTo`, companion round-trip, zero-sized chunks |
| `mp4/ilst/tests.rs`         | 23       | ✅ First-class `Ilst`/`Atom`/`AtomData`/`AtomIdent` model on `Mp4File.Ilst`; split/merge companion (unknown atoms preserved); `Insert` merge + `ReplaceAtom`; full Accessor suite (title/artist/album/genre/comment/track/disk/date/flags/advisory); `gnre`→©gen upgrade + discard rules; `free`-atom padding to avoid offset updates; invalid-ident/encoding data-drop semantics |
| `lyrics3/v1` + `v2`         | 13       | ✅ `Lyrics3Tag` (v1) + `Lyrics3v2Tag` (v2) concrete models on `MpegFile`; IND flags, IMG images (`filename\|\|desc\|\|[mm:ss]`), field truncation limits, strict/lenient parse modes, companion round-trip, 0xFF char rejection, reverse-search `findLyrics3` |
| `id3/v2/header.rs`          | 6        | ✅ `Id3v2Header` extended-header fields (`ExtendedSize`, `Crc`, `Restrictions`/`TagRestrictions`); v3 CRC + v4 restriction parsing; strict/best-attempt malformed-header tests |
| `util/text.rs`              | 2        | ✅ UTF-16 decode (BOM/BE/LE/no-BOM), UTF-16/UTF-8 encode |
| `id3/v2/tag/tests.rs`       | 45       | ✅ all passing (see below). `from_tag` merge matrix ported: multi-TXXX keys merge `\0` by desc, ReplayGain last-wins, COMM/USLT merge `\0` by (lang,desc), generic `T*` merge `\0` by frame ID, TBPM/`W*`/flags/UFID/POPM dedup first-wins (POPM by email, UFID by owner, TXXX by desc), flags normalized to 0/1, ITUNESADVISORY validated u8→AdvisoryRating, MB-recording-id ASCII check, Movement\*→MVIN text-merge (nonstandard frame IDs), `0/total` pair defaults. Parse side: COMM/USLT `\0`-split + lang, TXXX empty-desc retained as binary, unknown `T*` frames decoded as text, invalid timestamps retained, pair frames retained on `split_pair` failure |

## Behavioral parity fixes (test-audit, round 2)

Second-pass audit (matching `#[test_log::test]` — the first pass missed the
entire `tests/` tree) surfaced two real bugs and more ported coverage:

- **bogem `ReadText` UTF-16 bug**: `bufReader.ReadText` reads one byte past
  the `00 00` terminator for encoding 1 (and can detect a misaligned
  terminator for encoding 2), corrupting whatever field follows the
  description — APIC picture data lost its first byte, COMM/USLT/TXXX text
  lost its BOM. `id3v2SanitizeTag` now holds APIC/COMM/USLT/TXXX frames with
  encoding ∈ {1,2} out of bogem and `id3v2ManualDescFrame` converts them
  (same conversions as the bogem cases, BOM-aware pair-aligned terminator
  scan per Rust `read_to_terminator`).
- **Tag order in `TaggedFile()`**: AAC (`[ID3v2, ID3v1]` not `[ID3v1,
  ID3v2]`), MPEG (APE was second; now ID3v2→ID3v1→Lyrics3→Lyrics3v2→APE),
  APE (`[ID3v1, ID3v2, APEv2, APEv1]`), FLAC (`[ID3v2, VC]`), MPC
  (`[ID3v2, ID3v1, APE]`) — now all follow the Rust struct declaration
  order, which `tags()[0]` assertions rely on.
- **Fuzz corpus sweep**: `TestFuzzCorpusReadFrom` replays all ~55 files in
  `tests/fuzz/assets/` through each format's `ReadFrom` — panics recovered
  per file, 3-second bound per Rust `oom_test`.
- Ported/extended: `TestTaglibId3v2SaveStripNoId3v1Migration` (empty tag →
  splice-removal, no ID3v1 migration), `TestTaglibId3v2ParseApicUtf16Bom`,
  synchdata `test_decode{1..4}` cases, `mac-399-id3v2.ape` properties
  variant, AAC junk-between-frames EncoderSettings + ID3v1 album/title.

## Behavioral parity fixes (test-audit driven)

Audit method: every `#[test]` in `lofty-rs/` cross-referenced against Go
tests (word-set matching); non-ignored unmatched tests were ported or
classified. Divergences found and fixed:

- **APIC empty description**: parsed APIC desc now uses Rust `text_or_none`
  — empty → `None`, not `Some("")`.
- **APE numbers**: `ApeTagFromTag` normalizes N/T pairs via `Insert` (Rust
  `set_number`) — unparseable/empty values are dropped, not kept verbatim.
- **APE push/insert/split**: `ApeTag.Push` merges same-type text values
  with `\0` (Rust `push`); `Insert`/setters replace (Rust `insert`);
  `ApeTag.Tag()` splits `\0` text into multiple items for mapped keys.
- **ilst `intPayload`**: signed ints always 4 bytes, unsigned never emits
  3 bytes (Rust `bytes_to_occupy_uint` semantics).
- **TIPL/TMCL** (`tag_id3v2.go`): supported TIPL roles map to generic
  items; unmapped pairs held back as `ItemKeyUnknown` binary items
  (desc `TIPL`/`TMCL`). TMCL is fully retained (Rust never maps it).
  v2.4 writes TIPL+TMCL separately; v2.3 merges both into `IPLS`.
- **`TaggedFile.Save`**: `ConflictingTagError` on conflicting tag types
  (Rust `tagged_file.rs:443`); non-writable tags skipped per TagSupport.
- **`writeTag`**: read-only TagSupport rejects non-empty writes (Rust
  `tag_ext::save_to`) — e.g. FLAC + ID3v2.
- **`FlacFile.SaveFile`**: `None` VC field + no pictures → no-op (Rust
  `write_to` parity — removal is in-memory only, disk block survives).
- **APE/ID3v1 collision**: Rust `test_id3v1_collision` is `#[ignore]`d
  upstream ("we currently fall for this collision") — Go matches the
  false-positive; not a defect.

Tests ported beyond those above: `TestTiplRoundTrip`,
`TestPopmCustomProvider` (global set-once provider; placed in
`taglib_test.go` so all other POPM reads run first),
`TestTaglibMp4PropertiesWithoutBitrate` (zeroed-bitrate fallback),
`TestTaglibMp4ZeroLengthAtom`, `TestLyrics3v1Remove` + vacuous-assertion
fix in the v2 variant, `TestLyrics3TagsConflict`,
`TestLyrics3v2RequiresId3v1`, `TestId3v2Full` (v2.2/2.3/2.4 fixtures),
`TestFlacStripTags`, `TestFlacRemoveId3v2`/`TestFlacWriteId3v2ReadOnly`,
`TestOpusIssue499` (real asserts incl. unmapped `track=a5` item) +
vinyl variant, WAV/AIFF `growing_trailing_id3v2_updates_stream_size`,
`TestTagSaveWithParseOptions` (MaxJunkBytes), APE header-only/footer-only
parse tests, `TestSpecialItemsRoundtrip` (RVA2), `TestReadSv5`
(3-tag MPC), `TestIoSaveToVec`, FLAC preferred-padding ×4,
`integer_shrinking` ilst test, `retain_known_idents`, `zero_size_id3v2`,
`find_id3v2_seek_back`, ID3v1 parse/re-read/convert/year suite.

## Formerly-known gaps (now closed)

- `TestFlagItemConversion` (PCST): flag value `"0"` round-trips — the manual
  v2.3/v2.4 frame walker decodes textual unknown frames such as `PCST` and
  maps them via the standard key map.
- `TestRemoveId3v24FramesOnId3v23Save`: `id3v2Serialize` emits a bare
  10-byte ID3 header when every frame is discarded for v2.3 output (Rust
  `dump_to` parity).
- `TestUndersizedFrames`: the manual frame walker (`id3v2SanitizeTag`)
  enforces Rust's `ParsedFrame::read` semantics — undersized/invalid frames
  are skipped in relaxed mode and parsing continues to later frames.
- `TestTrimEndNullsWhenReadingFrameContent`: trailing-NUL trim applies and
  the TXXX `"Writer"` description maps to `ItemKeyWriter`.

## Not ported (Rust-internal, no portable equivalent)

- `id3/v2` specialized frame items (SYLT/CTOC/CHAP/RVA2/ETCO/UFID-owner
  variants/OWNE/GEOB/TXXX-kv): Rust exposes concrete `Frame` types; bogem
  owns these in this port. `ItemValue` stays Text/Locator/Binary like Rust.
- MPC SV8 nested `StreamHeader`/`ReplayGain`/`EncoderInfo` property structs.
- `tag/utils.rs` `dump_to` byte-level API (covered indirectly via
  `SaveFile`/`writeTagTo`).

## Notes

- ID3v2 padding: `id3v2Serialize` appends `WriteOptions.PreferredPadding`
  (Rust `verify_tag_size`: `min(padding, SYNCHSAFE_MAX - len)`).
- ID3v2 unknown frames round-trip as `ItemKeyUnknown` binary items keyed by
  frame ID in `TagItem.Description`.
- ~~`Lyrics3v2` write path~~ — implemented (before ID3v1, replace/strip).
- MP4 write replaces/creates `ilst` and relocates chunk offsets — Rust's
  `update_offsets`: after a size-changing splice, `stco`/`co64` entries and
  `moof`/`tfhd` base offsets at/after the edit point shift by the delta
  (`mp4UpdateChunkOffsets`). Created `udta`/`meta` atoms go at their
  parent's data start like Rust; `free`-atom padding (0x01-filled) absorbs
  shrinkage ≥8 or replaces a removed `ilst` when padding is enabled;
  non-full `meta` atoms are detected (`mp4MetaIsFull`).
