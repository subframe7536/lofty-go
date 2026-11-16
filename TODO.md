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
| ID3v2     |  ✅  |  ✅   | `id3/v2/` (native)     | `id3v2/` pkg — 90+ pass, no skips |
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
| `id3/v2/items/*.rs` tests   | 30       | ✅ fixture decode/encode for ATXT/CHAP/CTOC/GEOB/ETCO/TIPL/OWNE/POPM/PRIV/RVA2/SYLT (Latin-1 + UTF-16 variants), `ctoc_many_roots` (single top-level CTOC survives a dump), `issue_204` (empty UFID owner: strict rejects, best-attempt tolerates), `write_popm` counter width, `pairs.rs` whitespace/empty number handling |
| `taglib/test_id3v2.rs`      | ~50      | ✅ all non-`#[ignore]`d cases ported: unsynch decode, UTF-8→UTF-16 downgrade for v2.3, UTF-16(BE) delimiters, per-frame parse/render for APIC/GEOB/POPM/RVA2/UFID/W\*/WXXX/OWNE/SYLT/ETCO/COMM/PRIV/TXXX, save-UTF-16-comment, genre updates (v2.3 parens + v2.4), date updates (v2.2 TYE→TDRC), v2.4→v2.3 downgrade, broken compressed length, W000, delete-frame, save-strip-no-ID3v1-migration, CHAP/CTOC embedded frames, CTOC with 129 children. Empty/`#[ignore]`d markers skipped (property interface, podcast PCST, iTunes v2.4 size, broken-frame1, popm to_string, empty/duplicate-tag markers) |

## Behavioral parity fixes (test-audit, round 2)

Second-pass audit (matching `#[test_log::test]` — the first pass missed the
entire `tests/` tree) surfaced two real bugs and more ported coverage:

- **UTF-16 description termination**: encodings 1/2 need a pair-aligned
  `00 00` scan after the description or the field that follows is
  corrupted — APIC picture data loses its first byte, COMM/USLT/TXXX text
  loses its BOM. The native parser handles this directly (BOM-aware
  pair-aligned terminator scan per Rust `read_to_terminator`).
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
- `TestRemoveId3v24FramesOnId3v23Save`: an `Id3v2Tag` with no writable
  frames serializes to zero bytes (Rust `create_tag` parity).
- `TestUndersizedFrames`: the native frame walker (`id3v2/read.go`)
  enforces Rust's `ParsedFrame::read` semantics — undersized/invalid frames
  are skipped in relaxed mode and parsing continues to later frames.
- `TestTrimEndNullsWhenReadingFrameContent`: trailing-NUL trim applies and
  the TXXX `"Writer"` description maps to `ItemKeyWriter`.

## API-parity fixes (audit round 3)

- **Concrete-tag `TagExt` parity**: all nine concrete tags now expose
  `DumpTo`/`SaveTo`/`SaveToPath`/`RemoveFrom`/`RemoveFromPath`
  (`VorbisCommentsTag`, `ApeTag`, `Id3v1Tag`, `Id3v2Tag`, `Lyrics3Tag`,
  `Lyrics3v2Tag`, `RiffInfoTag`, `AiffTextTag`, `Ilst`). Saves verify the
  destination format and apply the Rust `!is_writable && !is_empty` gate;
  empty writes strip read-only tags. Format-specific placement (ID3v2 in
  WAV/AIFF chunks and at the DSF metadata pointer, VorbisComments in Ogg
  streams and FLAC blocks) dispatches through
  `meta.RegisterConcreteTagWriter` — the `TagWriteExt` equivalent — so
  vendor strings, pictures, and unknown keys survive concrete saves.
- `(*Id3v2Tag).SaveToVerified` mirrors `TagWriteExt::save_to` (the
  unchecked internal entry point AudioFile save paths call); file-level
  `SaveFile` uses it so read-only ID3v2 still round-trips like Rust.
- `VorbisCommentsTag` gained the `OggPictureStorage` API
  (`SetPicture`/`RemovePicture`/`RemovePictureType`/`RemovePictures`),
  `TakeItems`, and `RemoveKey` now returns the removed values.
- File structs store concrete tags like Rust (`Option<Id3v2Tag>` etc.):
  `ID3v2` on Mpeg/Flac/Wav/Aiff/Dsf/Aac/Mpc/Ape, `ID3v1` on Aac/Mpc/
  WavPack, `Ape` on Mpc/WavPack; `ApeFile.Ape` is a single slot (v2
  preferred) matching Rust's `ape_tag`.
- `TaggedFile.PrimaryTagMut` no longer inserts; only
  `PrimaryTagMutOrInsert` creates a missing primary tag.
- Exported `id3v2.UpgradeV2`/`UpgradeV3`, `meta.Id3v2Version2`,
  `meta.NewProbeFromPath` (Rust `Probe::open`), `TagItem.Consume`,
  `(*Id3v2Tag).Tag`/`Remove`/`RemoveKey`, `(*ApeTag).Clear`/`DumpTo`.
- Typed enums: `mp4.DataType` (+ `DataTypeOther`), `mp4.AdvisoryRating`,
  `iff.WavFormat` (+ `WavFormatOther`), `mpeg.Emphasis`;
  `MpegProperties.ModeExtension` is `Option[uint8]` (Some only for joint
  stereo) and `Emphasis` is `Option[Emphasis]` like Rust.
- `Id3v2TagToTag`/`(*Id3v2Tag).Tag` is non-destructive (splits a cloned
  frame list) so repeated conversion can't drain `t.Frames`.

## Semantic-parity fixes (audit round 4 — subagent-assisted diff vs lofty-rs)

- **MPEG**: V2/V2.5 Layer-2 bitrate table corrected (was a copy of the V2
  Layer-1 table — wrong bitrates/frame-lengths for all non-V1 L2 files);
  `findNextFrame` resumes scanning at the end of a rejected candidate like
  `cmp_header`; the Xing/VBRI probe propagates seek/`read_exact` errors;
  CBR `audio_bitrate` is reported even when no last frame is found;
  `FindLyrics3` passes the literal `true` read flag; `FindId3v1`,
  `FindId3v2`, `ParseId3v2Tag`, and `ReadApeTag` errors propagate instead of
  being swallowed; the APE tail subtracts via `checked_sub`
  (`SizeMismatchError`) and accepts `pos == size`.
- **Musepack**: SV8 frequency table extended to 8 entries (was an
  index-out-of-range panic for sample-rate index ≥ 4); non-"MPCK" 4-byte
  signatures fall back to the SV4-6 path (was `FakeTagError`); all property
  parsing is gated on `ReadProperties` like Rust; SV7 reads only the 22
  bytes it needs (29-byte read could fail on short files) and enforces
  `version & 0x0F == 7`; `p.Version` comes from the SH packet's stored byte;
  `streamLen - id3v2Size` is a checked subtraction (`SizeMismatchError`).
- **WavPack**: `ID_MULTICHANNEL` decode matches Rust's `s = size - 2`
  switch (was wrong mask/channels for size ≥ 5 and missing the size ≥ 6
  error); channels store as u16→u8 truncation (not 0xFF saturation);
  bit-depth uses `saturating_sub`; block-content `read_exact` errors and
  short large-size/odd-pad reads propagate like Rust; `ChannelMask` is
  surfaced in `FileProperties`.
- **DSF**: `DSD ` chunk size must be 28 (read and write); `overall_bitrate`
  uses the declared `total_file_size`; `audio_bitrate` uses the u8-truncated
  bit depth; the ID3v2 metadata pointer path uses `FindId3v2` (junk scan)
  and propagates parse errors; the write path mirrors Rust's
  `write_to_dsf` (pointer/size bookkeeping, `ptr > total` error, no-op on
  empty tag with no pointer, append at declared size).
- **IFF/WAV**: `overall_bitrate` uses the chunk-walk end position (not
  whole-file length); `fact` chunks < 4 bytes error; a zero-length `fmt `
  no longer blocks a later valid one; channels truncate to u8 before the
  zero check and the fallback bit depth uses `saturating_mul(8)`; empty
  LIST/INFO and all-empty AIFF text yield `None` tags;
  `RiffInfoTag.Tag()` trims NULs and `RiffInfoTagFromTag` drops
  `ItemKeyUnknown` items like Rust's `merge_tag`.
- **IFF writes**: all four chunk scans now use `iffCorrectPosition`'s pad
  peek instead of blind `size&1` (a "pad" byte that starts a valid FourCC
  is no longer eaten); `aiffWriteId3v2Chunk` matches `"id3 "` too, inserts
  at the declared FORM end, and errors on `file_len < stream_size`;
  `aiffWriteTextChunks` requires a `COMM` chunk (`missing_comm`) and
  splices at `comm_end_pos`/the first existing text chunk like Rust.
- **Vorbis comments**: vendor bytes are UTF-8-validated (strict errors,
  relaxed recovers via UTF-16 code units); `METADATA_BLOCK_PICTURE` and
  deprecated `COVERART` decode errors propagate in strict mode; `COVERART`
  is case-insensitive, uses `PictureTypeOther`, guesses MIME with an
  `"image/"` fallback, and stores default `PictureInformation`; generic
  keys must satisfy `valid_vorbis_comments_key` (0x20–0x7D minus `=`),
  values must be valid UTF-8, and `BINARY` gets no special handling (all
  matching Rust's `read_comments`).
- **Shared**: `ape.ReadEndTags` no longer scans for Lyrics3 (Rust's
  aac/musepack/wavpack readers check only ID3v1 + APE); `BytesFile.Write`
  zero-fills gaps like `Cursor<Vec<u8>>`; Latin-1 `ValidUpTo` reports char
  indices.

## Semantic-parity fixes (audit round 5 — Ogg duration + IFF/MPC gaps)

- **Ogg**: `oggLastGranulePos` was rewritten as Rust's `find_last_page` —
  an overlapping-chunk backward scan from EOF down to the position after
  the codec header packets (captured before the Vorbis setup read), with
  full page validation (`OggS`, version 0, ≥1 segment, complete lacing
  table + body) and no serial filtering. Duration and bitrate now come
  from the last valid page's granule position regardless of serial.
- **IFF/WAV write**: `wavWriteRiffInfoTag` now walks chunks inside the
  declared RIFF size like `write_riff_info` — `SizeMismatchError` when the
  file is shorter than declared, `Chunks::next`-equivalent fourcc/oversize
  strict-mode checks, pad bytes counted in `remaining`, and the replaced
  LIST consumes exactly `8+size` (a pad byte is left in place like Rust).
  `wavInsertChunk` errors `TooMuchDataError` when the new RIFF size
  exceeds u32::MAX; `riffInfoListBytes` mirrors `create_riff_info`'s
  u32::MAX check. `wavWriteId3v2Chunk`'s scan is bounded by the declared
  RIFF size.
- **RIFF INFO popularimeter**: `merge_tag` (`RiffInfoTagFromTag`) keeps the
  raw popm text; the `mapped_value` u8 conversion applies only in the
  generic-Tag write/dump paths via a new `tagitemsIntoRiff` (Rust
  `tagitems_into_riff`) — fixing `TestPopmConversions` RiffInfo
  round-trip and matching Rust's two-path semantics.
- **AIFF**: `aiffParseComt` reads through a `LimitedReader` bounded by the
  declared COMT size (overruns error in all modes like Rust's `Take`-based
  `Chunk`); `aiffWriteTextChunks`'s scan has `Chunks::next` fourcc/oversize
  strict-mode checks; the registered `AiffText` dumper emits full
  `create_text_chunks` bytes; generic-Tag write/dump uses
  `aiffTextFromGeneric` (`comments: None` like `AiffTextChunksRef`) so COMT
  comments only survive concrete-tag writes.
- **Musepack SV8**: packet-body reads go through a capacity-bounded
  `LimitedReader` (Rust `PacketReader`); `RG`/`EI` bodies are parsed so
  truncated packets error instead of silently skipping; all
  `PacketReader::next` errors (bad key, bad varint, short read) just end
  the scan like `while let Ok(...)` — no strict-mode error for bad keys;
  strict missing-packet checks run in Rust's order (SH→RG→audio→SE) and
  are fully gated on `ReadProperties`; `mpcReplayGain` computes in f32;
  SV4-6 keeps the raw header `average_bitrate` when `frame_count == 0`.

## Semantic-parity fixes (audit round 6 — internal/meta + internal/util diff)

- `internal/meta/tag_itemkey.go`: Rust `gen_map!` semantics are now exact —
  separate duplicate entries are last-wins on key lookup (`"TRCK"` →
  `TrackTotal`, `"TPOS"` → `DiscTotal`, `"MVIN"` → `MovementTotal`,
  `"TEXT"` → `Lyricist`, APE `"Track"`/`"Disc"` → totals, ilst
  `"trkn"`/`"disk"` → totals), while `"K" => A|B|C` multi-variant entries
  resolve to the first variant (`"TIPL"` → `Producer`, `"TPUB"` →
  `Publisher`, `"TSSE"` → `EncoderSettings`, APE `"Year"` →
  `RecordingDate`, ilst `"©lyr"` → `Lyrics`). `keyMapEntry` gained an
  `aliases` field; `byKey` inserts overwrite like `HashMap::insert`.
- `internal/meta/file_type.go`: committed `FORM`/`OggS`/`RIFF` buffers that
  fail the inner signature check now yield undetermined like Rust's match
  arms — the `ftyp`/`MPCK` fallback guesses are skipped.
- `internal/meta/tag_write.go`: `UnsupportedTagError` failures now carry
  the file format (Rust `.with_format`).
- `internal/meta/picture.go`: unmapped APE "Cover Art" keys now produce
  Rust's `Undefined(0)` equivalent — `AsApeKey` returns false so they're
  skipped on write instead of being renamed to "Cover Art (Other)". APE
  picture descriptions are Latin-1→UTF-8 transcoded like Rust's
  `char::from(u8)`.
- `internal/util/timestamp.go`: the first timestamp segment yielding 0
  bytes now stops all further segment parsing (Rust `read_segment!`
  `break`) — e.g. a bad month no longer leaves a parseable day.
- New test: `TestItemKeyFromKeyLastWins` pins the duplicate-key outcomes.

### Still-open parity gaps (documented)

- MP4: no 32→64-bit extended-box promotion for >4 GiB writes (returns
  `TooMuchDataError`). Rust promotes via `AtomWriter::insert`; Go's
  splice-based writer would need a buffered rewrite — a clean error is
  kept as the degradation.
- Rust's WAV JUNK-chunk write optimization is a TODO upstream too
  (Go rewrites — valid bytes, different layout).
- Ogg `oggPackets` was removed — superseded by `OggReadCountPackets`
  (round 7).

## Not ported (Rust-internal, no portable equivalent)

- MPC SV8 nested `StreamHeader`/`ReplayGain`/`EncoderInfo` property structs.
- `tag/utils.rs` `dump_to` byte-level API (covered indirectly via
  `SaveFile`/`writeTagTo`).

## Performance

`bench_test.go` has read/write benchmarks (`go test -run XXX -bench .`). The
dominant cost was syscall count, not CPU — every format's junk scan and tag
walk did byte-at-a-time `Read`s on the raw `*os.File` (matching Rust's
`reader.bytes()`, which is equally syscall-heavy upstream). Fixes:

- `internal/util/scan.go` `ChunkedScan` — chunked (8 KiB) seeker scan with
  carry overlap; used by `FindId3v2`/`FindId3v2InJunk` (ID3 magic in junk),
  `util.SearchForFrameSync`/`SearchForFrameSyncSeeking` (probe),
  `aacSearchFrameSync`, `mpeg.findNextFrame`, and the MPEG leading-zero
  skip. Non-seekable readers keep the byte-loop fallback to preserve the
  "positioned right after the match" contract.
- `id3v2.ParseId3v2Tag` buffers tag content (≤ `AllocationLimit`; lazy
  `LimitReader` for larger tags and for already-memory readers) — identical
  end position since the drain consumed the limited region anyway.
- `mp4` buffers the `moov` box and walks it via `offsetReadSeeker`, which
  keeps positions file-absolute while making inner seeks memory ops.
- `unsyncReader` (v2.3 tag-wide desync) filters in bulk instead of 2-byte
  reads per byte.
- APE item-key read scans for NUL in 64-byte chunks with seek-back.

Results (M4, ~400ms runs): FLAC 324µs→8µs, APE 342µs→18µs, M4A 36µs→11µs,
WavPack 19µs→15µs; MP3/Opus/WAV already ~10µs. `BenchmarkWriteMP3Tag` is
disk-bound (~120µs: temp file + splice rewrite of the audio tail).

## Notes

- ID3v2 is a **native port** — `Id3v2Tag`/`FrameList`/`Frame` with typed
  frames (`TextInformationFrame`, `CommentFrame`, `KeyValueFrame`, `CHAP`/
  `CTOC`, `POPM`, `UFID`, `RVA2`, `ETCO`, `PRIV`, `OWNE`, `APIC`,
  `TimestampFrame`, `ExtendedText/UrlFrame`, `UrlLinkFrame`, `BinaryFrame`
  + standalone `SynchronizedTextFrame`/`GeneralEncapsulatedObject`/
  `AudioTextFrame` helpers). bogem is no longer in the read/write path; it
  remains as a test-only parse oracle in `id3v2/tag_test.go` and
  `xfmt_test.go`.
- Unconvertible frames live in `Tag.CompanionTag().(*Id3v2Tag)` (Rust
  `SplitTagRemainder`), not as `ItemKeyUnknown` binary items.
- ID3v2 padding: `PreferredPadding` honored except when `Footer` is set
  (Rust `verify_tag_size`); CRC written for v2.3+v2.4 extended headers;
  footers supported on stream writes (not chunk inserts).
- ~~`Lyrics3v2` write path~~ — implemented (before ID3v1, replace/strip).
- MP4 write replaces/creates `ilst` and relocates chunk offsets — Rust's
  `update_offsets`: after a size-changing splice, `stco`/`co64` entries and
  `moof`/`tfhd` base offsets at/after the edit point shift by the delta
  (`mp4UpdateChunkOffsets`). Created `udta`/`meta` atoms go at their
  parent's data start like Rust; `free`-atom padding (0x01-filled) absorbs
  shrinkage ≥8 or replaces a removed `ilst` when padding is enabled;
  non-full `meta` atoms are detected (`mp4MetaIsFull`). The write path
  runs Rust's `verify_mp4` (ftyp required) and the `ContextualAtom`
  validation (`mp4ValidateAtomTree`): every important container
  (moov/udta/moof/trak/mdia/minf/stbl) must tile exactly or the write
  fails; all scans use the write options' parsing mode.
- MP4 read parity: `ParseIlst` buffers the declared `ilst` body up front
  (truncated body = hard error in all modes like Rust's `read_exact`),
  parses over a bounded `offsetReadSeeker` cursor; header-level errors
  end the outer scan (`while let Ok` semantics, freeform included) while
  `data`-level errors propagate; u24 data-type codes; multiple `udta`
  `ilst` atoms merge via `Insert`; stts `specifies_duration` matches
  `!(len==1 && duration==1)` (0 entries → true); zero-length `mdat`
  still yields `overall_bitrate`; moov/moov-child/udta/meta scans are
  bound- and mode-aware (`mp4ReadBoxBound`) with Rust's
  `(len - header_len) > reader_size` oversize tolerance.

## Fixture differential run (Rust vs Go dumpers, 157 files)

A Rust dumper (`lofty-rs/lofty/examples/dump.rs`, `Probe::open(path).read()`)
and a Go equivalent were run over the whole fixture corpus
(`tests/files/assets` + `tests/taglib/data`) and diffed. Fixes this round:

- **MP4 `ParseIlst` no longer rewinds the cursor to the parent atom end**
  after the `data`-child scan. Rust's `AtomReader` monotonically decrements
  `remaining_size`; a `data` child declaring size 0 extends to the ilst
  buffer end and permanently ends the outer atom scan
  (`infloop.m4a`: Rust keeps only trkn/disk/tmpo + upgraded ©gen).
- **MPEG `findNextFrame` rewritten** to Rust's two-step semantics: scan for
  the raw 11-bit sync, then read the 4-byte header. An invalid candidate
  resumes the scan at `sync+4` — a valid header overlapped by an invalid
  candidate is skipped (`invalid-frames3.mp3`: first frame is at 0x7dd,
  not 0x3c9 → 157 ms not 183 ms). `cmp_header` failure resumes at
  `candidate+frameLen`; Undetermined ends the search.
- **`ape.ReadEndTags` propagates the trailing `seek(-32)` error** like
  Rust's `seek(Current(-32))?` — sub-32-byte files now fail the whole read
  (`segfault2.mpc` → `EINVAL` parity).
- **AAC no longer scans for APE/`-32`** — Rust's `aac/read.rs` only checks
  ID3v1; the shared `ReadEndTags` call was wrong for it.
- `NewTaggedFile` preserves empty tags (Rust only `retain(!is_empty)`s on
  `BoundTaggedFile::save`); RIFF INFO lists merge across multiple
  `LIST/INFO` chunks; channel masks propagate in MPEG/AAC/Ogg properties;
  Opus reports `input_sample_rate` and validates channel-mapping family;
  FLAC uses integer-ms math with `Some(0)` bitrates; `Mp4Properties` uses
  `Option` fields for unset values.

Remaining diffs are all *intentional*:

- `empty_vorbis.oga` / `no-tags.3g2`: Rust's `read_from_path` has no
  content-probe fallback and errors on unrecognized extensions; Go's
  `ReadFromPath` sniffs and correctly reads both — a strict superset
  (verified identical output when the files are renamed to `.ogg`/`.mp4`).
- `longloop.ape`: documented APE `remaining_size` deviation — Rust's
  accounting bug reads past the tag and errors; Go's accurate accounting
  accepts the malformed file.
- MP4 >4 GiB writes (`TooMuchDataError` instead of extended-box
  promotion) and WAV JUNK-chunk optimization (Rust TODO upstream).

## Semantic-parity fixes (audit round 7 — Ogg + VorbisComments read/write)

- **Ogg header read** (`oggReadTagged`): single `OggReadCountPackets` call
  for all header packets after a one-packet codec sniff + rewind (3
  Vorbis, 2 Opus/Speex) — matching `Packets::read_count` and Rust's
  `header_end` bound for `find_last_page`. `oggPackets` deleted.
  `verify_signature` is a hard error in every mode; the comment signature
  is checked only under `ReadTags`.
- **`OggReadCountPackets`**: a page-header failure (`oggHeaderError`) ends
  the packet scan like `while let Ok(header)`; body errors still
  propagate; fewer than `count` packets → "not enough data"
  (`PageError::NotEnoughData`).
- **`readOggPage`**: rejects `nsegs < 1` (`BadSegmentCount`); header vs
  body failures are distinguished so `oggLastGranulePos` retries only on
  MissingMagic/InvalidVersion/BadSegmentCount (a truncated last-page body
  aborts the search → 1000ms bitrate fallback, duration unset).
- **Speex validation**: fixed 80-byte header, `version <= 1`, mono/stereo
  only, `vbr` requires field `== 1` (Rust `read_properties`).
- **Vorbis ident**: ≥28 bytes, error only under `ReadProperties`.
- **Ogg write vendor**: truncated vendor is a hard error (Rust's
  unconditional `read_u32` + `read_exact`); an invalid-UTF-8 vendor
  becomes "" (`String::from_utf8` fallback); the file's retained vendor
  always wins (`tag.vendor = vendor_str`) — applied via `WithVendor` so
  the concrete `SaveFile` path doesn't mutate the caller's tag.
- **Vorbis values**: trailing NULs trimmed like `utf8_decode_str` — item
  values and TRACKNUMBER/DISCNUMBER before the `n/total` split.
- **`VorbisCommentsTag.InsertPicture`**: `PictureInformation` extraction
  errors now propagate. Extraction runs before the Icon/OtherIcon removal,
  so a failure leaves the tag unchanged — deliberate deviation from Rust's
  remove-then-error order.
- **Generic→concrete conversion**: `VorbisCommentsTagFromTag` /
  `VorbisCommentsTagForWriteRef` drop `ItemKeyUnknown` items (Rust
  `map_key -> None => continue`) instead of storing them under their
  description; new `pushMapped` helper. `Push`/`Insert` keep the
  description fallback as public conveniences.
- **FLAC ID3v2**: `SaveTo`/`SaveToPath` gate on `checkTagSupport` (Rust
  `TagExt::save_to`); `SaveToVerified` is the ungated internal path
  (`TagWriteExt::save_to`); the generic FLAC ID3v2 writer always writes an
  empty tag — stripping any existing one (Rust `create_tag` empty →
  splice-removal).
- New synthesized-byte tests: `ogg/format_test.go` (shared-page header
  read, truncated/bogus last page, Speex validation, short ident, vendor
  write rules) and `vorbis/tag_test.go` (NUL trim, picture-info error,
  unknown-item drop).
