# lofty-go — Port Tracker

Go port of [lofty-rs](https://github.com/Serial-ATA/lofty-rs) (audio metadata
library). Reference source lives in `lofty-rs/`; test assets are reused from
`lofty-rs/lofty/tests/files/assets/`.

## Layout

Single package `lofty` at the module root (Go requires same-package files in
one directory; format code relies on shared unexported helpers). Files are
grouped by prefix:

| Prefix / file        | Rust counterpart                          | Contents                                         |
|----------------------|--------------------------------------------|--------------------------------------------------|
| `config.go`          | `config/`                                  | ParseOptions, WriteOptions, GlobalOptions        |
| `errors.go`          | `error.rs`                                 | LoftyError, FileParse/FileEncoding/Tag errors    |
| `option.go`          | —                                          | `Option[T]` (Rust `Option`)                      |
| `util_*.go`          | `util/{alloc,io,text}.rs`                  | alloc limits, FileLike/Splice, text encodings    |
| `picture.go`         | `picture.rs`                               | Picture, MimeType, PictureInformation            |
| `properties.go`      | `properties/mod.rs`                        | FileProperties, ChannelMask                      |
| `timestamp.go`       | `tag/items/timestamp.rs`                   | Timestamp (partial ISO-8601)                     |
| `popularimeter.go`   | `tag/items/popularimeter.rs`               | POPM/star ratings                                |
| `tag_core.go`        | `tag/mod.rs`                               | Tag, accessors                                   |
| `tag_item.go`        | `tag/item.rs`                              | TagItem, ItemValue, Lang                         |
| `tag_itemkey.go`     | `tag/item.rs` + format key maps            | ItemKey + all format key mappings                |
| `tag_support.go`     | `tag/tag_type.rs`                          | TagType, TagSupport matrix                       |
| `tag_write.go`       | `tag/utils.rs`                             | write/dump/remove dispatch + format writers      |
| `tag_id3v1.go`       | `id3/v1/`                                  | ID3v1 tag + genres                               |
| `tag_id3v2.go`       | `id3/v2/`                                  | ID3v2 via `bogem/id3v2`                          |
| `tag_lyrics3.go`     | `id3/lyrics3/`                             | Lyrics3 v1/v2                                    |
| `file_*.go`          | `file/`, `probe.rs`, `resolve.rs`          | FileType, Probe, TaggedFile, resolvers           |
| `format_*.go`        | `<format>/`                                | Per-format file readers/writers                  |
| `files_test.go`      | `lofty/tests/files/*.rs`                   | Ported tests (reuses Rust assets)                |

## Format status

| Format    | Read | Write | Rust reference        | Test parity                     |
|-----------|:----:|:-----:|------------------------|---------------------------------|
| MPEG (MP3)|  ✅  |  ✅   | `mpeg/`                | `files/mpeg.rs` — 6 tests ✅    |
| ID3v1     |  ✅  |  ✅   | `id3/v1/`              | covered by mpeg/ape tests       |
| ID3v2     |  ✅  |  ✅   | `id3/v2/` (bogem)      | `id3v2_tag_test.go` — 41 pass, 4 known-gap skips |
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
| `id3/v2/tag/tests.rs`       | 45       | ✅ 41 pass, 4 skipped known-gaps (below). `from_tag` merge matrix ported: multi-TXXX keys merge `\0` by desc, ReplayGain last-wins, COMM/USLT merge `\0` by (lang,desc), generic `T*` merge `\0` by frame ID, TBPM/`W*`/flags/UFID/POPM dedup first-wins (POPM by email, UFID by owner, TXXX by desc), flags normalized to 0/1, ITUNESADVISORY validated u8→AdvisoryRating, MB-recording-id ASCII check, Movement\*→MVIN text-merge (nonstandard frame IDs), `0/total` pair defaults. Parse side: COMM/USLT `\0`-split + lang, TXXX empty-desc retained as binary, unknown `T*` frames decoded as text, invalid timestamps retained, pair frames retained on `split_pair` failure |

## Known gaps (skipped tests — bogem delegation limits)

- `TestFlagItemConversion` (PCST): flag value `"0"` doesn't round-trip through
  bogem's frame handling.
- `TestRemoveId3v24FramesOnId3v23Save`: when every frame is discarded for
  v2.3 output, bogem serializes < 10 bytes (no valid empty-tag header).
- `TestUndersizedFrames`: Rust's frame walker skips undersized COMM and
  reaches trailing TPE1; bogem's parse aborts — a manual v2.3/v2.4 frame
  walk (like the v2.2 path) would be needed for full parity.
- `TestTrimEndNullsWhenReadingFrameContent`: trailing-NUL trim not applied
  to the `TEXT` (writer) frame's decoded text.

## Not ported (Rust-internal, no portable equivalent)

- `id3/v2` specialized frame items (SYLT/CTOC/CHAP/RVA2/ETCO/UFID-owner
  variants/OWNE/GEOB/TXXX-kv): Rust exposes concrete `Frame` types; bogem
  owns these in this port. `ItemValue` stays Text/Locator/Binary like Rust.
- MPC SV8 nested `StreamHeader`/`ReplayGain`/`EncoderInfo` property structs.
- `tag/utils.rs` `dump_to` byte-level API (covered indirectly via
  `SaveFile`/`writeTagTo`).

## Notes

- `bogem/id3v2` lacks ID3v2 padding support — tags are written without
  `WriteOptions.PreferredPadding` for now.
- ID3v2 unknown frames round-trip as `ItemKeyUnknown` binary items keyed by
  frame ID in `TagItem.Description`.
- ~~`Lyrics3v2` write path~~ — implemented (before ID3v1, replace/strip).
- MP4 write replaces/creates `ilst` but does not relocate `mdat`/`moov`.
