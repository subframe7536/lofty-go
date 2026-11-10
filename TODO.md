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
  non-full `meta` atoms are detected (`mp4MetaIsFull`).
