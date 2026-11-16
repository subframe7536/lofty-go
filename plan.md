# Plan: finish the interrupted Ogg / Vorbis Comments parity pass

**Status: DONE (2026-11-15).** All steps implemented; `go test ./...`,
`golangci-lint`, and `staticcheck` clean; `facade_gen.go` regenerated;
`TODO.md`/`AGENTS.md` updated. Kept for reference.

Picks up from Paseo session `2f97fc9` ("review the lofty-rs port, using
sonnet/haiku 5.5 as subagent", 2026-11-14 → 2026-11-15). That session was
cut off partway through an edit to `ogg/format.go`. The working tree has 41
modified files (+3732/−1126) and 8 new test files, none of them committed.

## Current state

- **The build is broken** (`go build ./...`):
  - `ogg/format.go:9` — `"unicode/utf8"` is imported but unused. The write
    path's vendor check (step 3) is supposed to use it.
  - `ogg/format.go:249` — `serial` is declared and not used in
    `oggReadTagged`.
- Already done earlier in the session (in the tree, verified):
  - FLAC + ID3v2: `Id3v2Tag.SaveTo`/`SaveToPath` run `checkTagSupport` (Rust
    `TagExt::save_to` gate). `SaveToVerified` skips the gate (Rust
    `TagWriteExt::save_to`). The FLAC generic ID3v2 writer always writes an
    empty tag, which strips any existing one (`flac/format.go:551`).
  - `internal/meta` unit tests, `wavpack/format_test.go`, `vorbis/tag.go`
    rework, `scripts/facadegen`, and fixes across format packages.

## Ogg findings: what is done and what is left

Labels from the session's review checkpoint: 3-packet read, Speex
validation, COVERARTMIME, SetPicture/InsertPicture, stored
PictureInformation, base64 strict, truncated page, granule math, 28-byte
Vorbis ident, key spelling.

### Already applied in `ogg/format.go`

- Sentinel page errors (`errOggMissingMagic`, `errOggInvalidVersion`,
  `errOggBadSegmentCount`), plus an `oggHeaderError` wrapper that keeps
  header failures separate from body failures.
- `readOggPage`: rejects pages with `nsegs < 1` (Rust `BadSegmentCount`).
  Errors while reading the header or segment table are wrapped as
  `oggHeaderError`. Body read errors propagate unwrapped.
- `oggLastGranulePos` (Rust `find_last_page`): searches backward from EOF to
  `headerEnd`. It skips candidates only on MissingMagic, InvalidVersion or
  BadSegmentCount. Any other error, such as a truncated last-page body,
  aborts the search, so the caller falls back to the 1000 ms default.
- `oggReadTagged`: sniffs the codec from packet 0, rewinds, then reads all
  header packets in one `OggReadCountPackets` call (3 for Vorbis, 2 for
  Opus/Speex), so `headerEnd` matches Rust. Also in place:
  - `verify_signature` is a hard error.
  - The comment signature is checked only when `ReadTags` is set.
  - Vorbis ident needs at least 28 bytes.
  - Speex checks: at least 80 bytes, version ≤ 1, mono/stereo only, and
    `vbr` is true only when the field is `== 1`.
  - Properties are parsed only when `ReadProperties` is set.
- `oggPackets` has been removed.

### Still to do

1. **Fix the compile errors.**
   - `oggReadTagged`: change `serial, firstGranule, err := oggFirstPage(...)`
     to `_, firstGranule, err := ...`. The read path doesn't need the serial.
   - The `unicode/utf8` import gets used by step 3. Remove it only if step 3
     ends up not needing it.

2. **`OggReadCountPackets`: treat header errors as end of stream.**
   This matches ogg_pager `Packets::read_count`, which loops with
   `while let Ok(header)`.
   - If `readOggPage` returns an `*oggHeaderError` (check with `errors.As`),
     break out of the loop.
   - Any other error (body read) still returns the error.
   - After the loop, if `len(packets) < count`, return
     `&OggParseError{Message: "not enough data"}` (Rust
     `PageError::NotEnoughData`).
   - The write path needs no other change, because the loop already returns
     once `count` packets are complete.

3. **Write path vendor handling (`oggWriteVorbisCommentsImpl`).**
   - Truncated vendor is an error, as in Rust (`read_u32` plus a `read_exact`
     of `vendor_len`). If `len(comment) < off+4` or
     `vlen > len(comment)-off-4`, return
     `&meta.FileEncodingError{Source: &OggParseError{Source: io.ErrUnexpectedEOF}}`.
     Today the code silently uses `""`.
   - Invalid UTF-8 vendor: Rust's `String::from_utf8` fails and it uses `""`.
     Use `utf8.Valid(b)` and fall back to `""`.
   - The file's vendor always wins. Rust's `write` sets
     `tag.vendor = vendor_str` before building the packet, for both the
     concrete and generic paths. After `vc := build(vendor)`, apply
     `vc = vc.WithVendor(vendor)`. Use the shallow copy so the concrete
     `SaveFile` path doesn't mutate the caller's tag.
   - Remove the dead `restPos` / `_ = restPos`.

4. **Update the doc comment on `oggWriteVorbisCommentsImpl`.** The `build`
   callback's vendor is now authoritative.

## Vorbis Comments findings (`vorbis/tag.go`): still to do

5. **Trim trailing NULs on comment values.** Rust's `utf8_decode_str` trims
   trailing `\0`.
   - In `ParseVorbisComments`, for the `TRACKNUMBER`/`DISCNUMBER` and
     `default` branches: validate UTF-8 first (keep the current
     strict/skip behavior), then `value = util.TrimEndNulls(value)` before
     splitting on `/` or storing.
   - Keys are not trimmed.
   - The vendor already goes through `util.Utf8Decode`. Confirm it trims.
     If not, trim there too.

6. **`InsertPicture`: propagate the info-extraction error**, as Rust's
   `OggPictureStorage::insert_picture` does. When `info` is None and
   `meta.PictureInformationFromPicture` fails, return the error and don't
   append. Do the Icon/OtherIcon removal only after info succeeds, so a
   failure leaves the tag unchanged.
   - `SetPicture` keeps `_ = t.InsertPicture(...)` for out-of-range indices.
     Rust `unwrap()`s and panics there; Go not panicking is a deliberate
     safety deviation. Add a short note in the existing comment.

7. **Drop unmapped items in generic → concrete conversion.** In Rust,
   `merge_tag` and `create_vorbis_comments_ref` do
   `map_key(TagType::VorbisComments)` and `continue` on None.
   - Today `VorbisCommentsTagFromTag` and `VorbisCommentsTagForWriteRef` call
     `vc.Push(item)`. Through `entryForItem`, that stores
     `ItemKeyUnknown` items under their `Description`, which Rust never
     does.
   - Add an unexported `pushMapped(item)` that only accepts items where
     `item.Key().MapKey(meta.TagTypeVorbisComments)` is ok. Use it in both
     functions.
   - Leave the public `Push`/`Insert` as they are. They are Go-only
     conveniences; Rust's concrete API is string-keyed (`PushKV`/`InsertKV`).
   - Risk: root tests (`tag_test.go:225`, `:429`) or round-trip tests may
     depend on unknown items surviving. Companion entries are merged
     separately and are not affected. If a test breaks, check whether it
     encodes Rust behavior before changing the test.

## Confirmed non-issues (no change, recorded so they aren't re-audited)

- **COVERARTMIME**: Rust has no special handling. It falls through to
  `valid_vorbis_comments_key` and is stored as an ordinary item. Go does the
  same.
- **base64 strict**: `strictBase64Decode` (padding required, zero trailing
  bits, rejects CR/LF) matches `data_encoding::BASE64`.
- **Stored PictureInformation**: concrete tags keep parsed info and write it
  back. The generic `Tag` path recomputes info with `unwrap_or_default`.
  Both match Rust.
- **Granule math**: same as Rust. Go guards `lengthMs > 0` before dividing,
  where Rust would panic on divide-by-zero for a 1-sample Opus stream.
  Deliberate.
- **Key spelling**: TRACKNUMBER/DISCNUMBER are normalized to uppercase (Rust
  does `String::from("TRACKNUMBER")`). Other keys keep their original
  spelling. `RATING:` prefix matching is case-sensitive. All match.
- **Paginate**: `oggPaginate` matches ogg_pager `paginate`, including the
  nil-page case at `len % (255*32) == 0`.
- **Opus channel count 0**: Rust `expect` panics; Go returns a parse error.
  Deliberate, already commented.
- **`PreferredPadding == 0`**: Go disables padding, while Rust `Some(0)` pads
  to the page boundary. Deliberate (Go has no Option here), already
  documented.
- **`VorbisCommentsTagFromTag` vendor param**: every caller passes `""`, so
  the companion vendor is kept unless an EncoderSoftware item exists. Same
  outcome as Rust.

## Verification

1. `go build ./... && go vet ./...`
2. Focused:
   `go test ./ogg ./vorbis ./flac ./id3v2 ./internal/meta -count=1`, then
   `go test -run 'Ogg|Vorbis|Opus|Speex|ZeroSized|Flac' -v ./ -count=1`
3. Add regression tests in `ogg`/`vorbis` (synthesized byte slices, no new
   fixtures):
   - A Vorbis stream whose comment and setup packets share one page. This
     was broken by the old 2+1 packet read and works with the single
     `read_count`.
   - A truncated last page: duration falls back to the 1000 ms path, not
     the previous page's granule.
   - Speex: version 2 is rejected, channels=3 is rejected, a 79-byte header
     is rejected.
   - A Vorbis ident shorter than 28 bytes errors with `ReadProperties` and
     is accepted without it.
   - A comment value `"Foo\x00\x00"` reads back as `"Foo"`, and
     `"3\x00"` as TRACKNUMBER parses to 3.
   - `InsertPicture` with undecodable image data and no info returns an
     error and leaves the tag unchanged.
   - Generic Tag with an `ItemKeyUnknown` item: not emitted by
     `VorbisCommentsTagFromTag` or `VorbisCommentsTagForWriteRef`.
   - Write with a truncated vendor errors. Write with an invalid-UTF-8
     vendor produces an empty vendor. Writing a concrete tag with its own
     vendor emits the file's vendor and leaves the caller's tag unchanged.
4. Full gate: `go test ./...`, `golangci-lint fmt ./...`,
   `golangci-lint run ./...`, `staticcheck ./...`
5. If any exported symbol changed: `go run ./scripts/facadegen` and confirm
   `facade_gen.go` builds.

## Wrap-up

- Update `TODO.md` (Ogg/Vorbis rows plus any deviations above that aren't
  already listed).
- Add any new port-parity rule to `AGENTS.md` if it is broadly relevant,
  e.g. "trailing-NUL trim on Vorbis values".
- The whole working tree is uncommitted, including the earlier session's
  work. Commit only when asked.
