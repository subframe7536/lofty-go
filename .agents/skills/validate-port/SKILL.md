---
name: validate-port
description: Validate the lofty-rs Go port — run the build/test/lint quality gates, the facade drift check, and the Rust↔Go fixture differential. Use after any port-parity change or when asked to verify the port.
---

# Validate the lofty-rs → lofty-go port

This skill **reports** — it does not fix. Run every check independently,
record pass/fail for each, and present a summary table at the end. A failing
check does not stop the run; gather all results first. If the user asked for
fixes, only fix after reporting.

All commands run from the repo root (`/Users/subf/Developer/go/lofty-go`).
`golangci-lint`, `staticcheck`, `gopls`, `govulncheck` live in
`$(go env GOPATH)/bin` — prepend it to `PATH` if a command is not found.

## 1. Static gates

```bash
go build ./...
go vet ./...
golangci-lint run ./...   # must report 0 issues
staticcheck ./...
gopls check ./*.go        # root facade only; may report nothing on clean tree
govulncheck ./...         # optional if govulncheck is installed
```

## 2. Tests

```bash
go test ./...             # ~15s
```

If a test fails, rerun it focused with `go test -run TestX -v ./<pkg>` and
capture the failure output verbatim.

## 3. Facade drift check

`facade_gen.go` is generated — verify it is in sync:

```bash
cp facade_gen.go /tmp/facade_before.go
go run ./scripts/facadegen
diff /tmp/facade_before.go facade_gen.go   # empty diff = in sync
```

Any diff means someone changed a public symbol without regenerating — report
the drift (do not just accept the regenerated file as "fixed").

## 4. Fixture differential (Rust vs Go)

Compares `meta.ReadFromPath` output against lofty-rs' own dump example over
the vendored test corpus.

### Go side

```bash
go build -o /tmp/godump-bin ./scripts/godump
```

### Rust side

A prebuilt binary usually exists at `lofty-rs/target/debug/examples/dump`.
If it does, use it. If missing, check whether
`lofty-rs/lofty/examples/dump.rs` exists; if not, create it as below and
build with `cargo build --manifest-path lofty-rs/Cargo.toml --example dump`
(requires the vendored crate's deps to be resolvable — vendored tree may be
offline; if cargo fails, report the differential as SKIPPED, not failed).

```rust
use lofty::file::{AudioFile, TaggedFileExt};
use lofty::probe::Probe;
use lofty::tag::ItemValue;

fn opt<T: ToString>(o: Option<T>) -> String {
    o.map(|v| v.to_string()).unwrap_or_else(|| "-".into())
}

fn main() {
    for path in std::env::args().skip(1) {
        println!("FILE {path}");
        let tagged = match Probe::open(&path).expect("open").read() {
            Ok(t) => t,
            Err(_) => { println!("ERR"); continue; }
        };
        println!("TYPE {:?}", tagged.file_type());
        let p = tagged.properties();
        println!(
            "PROPS dur={} overall={} audio={} rate={} depth={} ch={} mask={}",
            p.duration().as_millis(),
            opt(p.overall_bitrate()),
            opt(p.audio_bitrate()),
            opt(p.sample_rate()),
            opt(p.bit_depth()),
            opt(p.channels()),
            p.channel_mask().map(|m| format!("{m:?}")).unwrap_or_else(|| "-".into()),
        );
        for tag in tagged.tags() {
            println!("TAG {:?}", tag.tag_type());
            let mut items: Vec<_> = tag.items().iter().collect();
            items.sort_by_key(|i| (i.key().to_string(), value_repr(i.value())));
            for i in items {
                println!("  ITEM {:?} = {}", i.key(), value_repr(i.value()));
            }
            for pic in tag.pictures() {
                println!(
                    "  PIC {} {:?} {}",
                    pic.pic_type() as u8,
                    pic.mime_type().map(|m| format!("{m:?}")).unwrap_or_else(|| "-".into()),
                    pic.data().len()
                );
            }
        }
    }
}

fn value_repr(v: &ItemValue) -> String {
    match v {
        ItemValue::Text(t) => format!("T:{t:?}"),
        ItemValue::Locator(l) => format!("L:{l:?}"),
        ItemValue::Binary(b) => format!("B:{}", b.len()),
    }
}
```

Note: this source is a reconstruction — if its output format diverges from
`scripts/godump`'s header comment, fix the Rust source, not the Go side.
Items must be sorted by key then value (the Rust dump sorted items and
godump matches that convention).

### Corpus

Enumerate every fixture under the vendored test dirs (order by path):

```bash
find lofty-rs/lofty/tests -type f \
  \( -path '*/files/*' -o -path '*/tags/*' -o -path '*/taglib/*' \) \
  ! -name '*.rs' ! -name '*.log' | sort > /tmp/corpus.txt
wc -l /tmp/corpus.txt   # 157 files as of the last full run
```

### Run

```bash
while read -r f; do ./lofty-rs/target/debug/examples/dump "$f"; done \
  < /tmp/corpus.txt > /tmp/rust_dump.txt 2>&1
while read -r f; do /tmp/godump-bin "$f"; done \
  < /tmp/corpus.txt > /tmp/go_dump.txt 2>&1
diff /tmp/rust_dump.txt /tmp/go_dump.txt > /tmp/dump_diff.txt
```

### Expected output

The only permitted diffs are these three files where Rust prints `ERR` but
Go successfully reads — all documented intentional deviations (see
`AGENTS.md` "APE remaining size" and `TODO.md`): Go sniffs content where
Rust's `read_from_path` rejects by extension / upstream bug.

- `lofty-rs/lofty/tests/taglib/data/empty_vorbis.oga`
- `lofty-rs/lofty/tests/taglib/data/longloop.ape`
- `lofty-rs/lofty/tests/taglib/data/no-tags.3g2`

Verify each diff hunk maps to exactly those `FILE` lines (grep the line
numbers from `diff` back into the dump files). **Any other diff is a parity
regression** — report the file, the Rust line, and the Go line.

## 5. Report

End with a table:

| Check | Result |
|---|---|
| `go build` | PASS/FAIL |
| `go vet` | PASS/FAIL |
| `go test ./...` | PASS/FAIL (n failures) |
| `golangci-lint` | PASS/FAIL (n issues) |
| `staticcheck` | PASS/FAIL |
| `gopls` | PASS/FAIL/SKIP |
| `govulncheck` | PASS/FAIL/SKIP |
| facade drift | CLEAN/DRIFT |
| differential | CLEAN / N unexpected diffs / SKIPPED |

For each failure include the first error verbatim and the file/line it came
from. For differential failures include the fixture path and both dump
lines.

## Parity rules worth remembering

When a diff or test failure appears, cross-check `AGENTS.md` — most
surprising behaviors are documented intentional deviations (APE
remaining-size accounting, MP4 >4 GiB writes returning `TooMuchDataError`,
content sniffing vs extension, Go-specific safety deviations). Do not
"correct" an intentional deviation into parity.
