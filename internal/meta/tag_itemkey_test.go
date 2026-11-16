package meta

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

type rustMapEntry struct {
	keys []string
	vars []string
}

// parseRustMaps extracts the gen_map! blocks from the vendored Rust source.
func parseRustMaps(t *testing.T) map[string][]rustMapEntry {
	t.Helper()
	b, err := os.ReadFile("../../lofty-rs/lofty/src/tag/item.rs")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]rustMapEntry{}
	cur := ""
	inBlock := false
	keyRe := regexp.MustCompile(`"([^"]*)"`)
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "gen_map!(") {
			inBlock, cur = true, ""
			continue
		}
		if !inBlock {
			continue
		}
		if s == ");" {
			inBlock = false
			continue
		}
		if cur == "" {
			if strings.HasSuffix(s, ";") && !strings.Contains(s, "=>") {
				cur = strings.TrimSuffix(s, ";")
			}
			continue
		}
		if !strings.Contains(s, "=>") {
			continue
		}
		parts := strings.SplitN(s, "=>", 2)
		right := parts[1]
		if i := strings.Index(right, "//"); i >= 0 {
			right = right[:i]
		}
		right = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(right), ","))
		var e rustMapEntry
		for _, m := range keyRe.FindAllStringSubmatch(parts[0], -1) {
			e.keys = append(e.keys, strings.ReplaceAll(m[1], `\u{a9}`, "\u00a9"))
		}
		for _, v := range strings.Split(right, "|") {
			e.vars = append(e.vars, strings.TrimSpace(v))
		}
		out[cur] = append(out[cur], e)
	}
	return out
}

// TestItemKeyMapsMatchRust compares all six per-format key maps against the
// gen_map! entries of lofty-rs/lofty/src/tag/item.rs: from_key (last duplicate
// wins, first variant of `A | B` wins, ASCII case-insensitive), map_key (first
// matching entry, its first key) and supported_keys.
func TestItemKeyMapsMatchRust(t *testing.T) {
	rust := parseRustMaps(t)
	if len(rust) != 6 {
		t.Fatalf("parsed %d Rust maps, want 6", len(rust))
	}
	names := map[string]ItemKey{}
	for k := ItemKey(0); k < ItemKeyUnknown; k++ {
		names[k.String()] = k
	}
	tagFor := map[string]TagType{
		"AIFF_TEXT_MAP": TagTypeAiffText, "APE_MAP": TagTypeApe, "ID3V2_MAP": TagTypeID3v2,
		"ILST_MAP": TagTypeMp4Ilst, "RIFF_INFO_MAP": TagTypeRiffInfo, "VORBIS_MAP": TagTypeVorbisComments,
	}
	for mname, ents := range rust {
		tt, ok := tagFor[mname]
		if !ok {
			t.Fatalf("unexpected Rust map %s", mname)
		}
		exp := map[string]string{}
		raw := map[string]string{}
		for _, e := range ents {
			for _, k := range e.keys {
				lk := asciiLower(k)
				if prev, ok := raw[lk]; ok && prev != k {
					t.Errorf("%s: %q and %q collide ignoring ASCII case", mname, prev, k)
				}
				raw[lk] = k
				exp[lk] = e.vars[0]
			}
			for _, v := range e.vars {
				if _, ok := names[v]; !ok {
					t.Errorf("%s: Rust variant %s missing in Go", mname, v)
				}
			}
		}
		for lk, v := range exp {
			for _, key := range []string{raw[lk], strings.ToUpper(lk), lk} {
				got, ok := ItemKeyFromKey(tt, key)
				if !ok || got.String() != v {
					t.Errorf("%s from_key(%q) = %v,%v want %s", mname, key, got, ok, v)
				}
			}
		}
		for name, k := range names {
			want, found := "", false
			for _, e := range ents {
				for _, v := range e.vars {
					if v == name && !found {
						want, found = e.keys[0], true
					}
				}
			}
			got, ok := k.MapKey(tt)
			if ok != found || got != want {
				t.Errorf("%s map_key(%s) = %q,%v want %q,%v", mname, name, got, ok, want, found)
			}
		}
		var wantSup []string
		seen := map[string]bool{}
		for _, e := range ents {
			for _, v := range e.vars {
				if !seen[v] {
					seen[v] = true
					wantSup = append(wantSup, v)
				}
			}
		}
		gotSup := ItemKeySupportedKeys(tt)
		if len(gotSup) != len(wantSup) {
			t.Errorf("%s: supported len %d want %d", mname, len(gotSup), len(wantSup))
			continue
		}
		for i := range gotSup {
			if gotSup[i].String() != wantSup[i] {
				t.Errorf("%s: supported[%d] = %s want %s", mname, i, gotSup[i], wantSup[i])
			}
		}
	}
	for _, tt := range []TagType{TagTypeID3v1, TagTypeLyrics3, TagTypeLyrics3v2} {
		if _, ok := ItemKeyFromKey(tt, "TITLE"); ok {
			t.Errorf("from_key on %v should be none", tt)
		}
		if _, ok := ItemKeyTrackTitle.MapKey(tt); ok {
			t.Errorf("map_key on %v should be none", tt)
		}
	}
}

// Rust folds ASCII case only (`eq_ignore_ascii_case`); U+212A KELVIN SIGN must
// not match "K".
func TestItemKeyFromKeyASCIIFoldOnly(t *testing.T) {
	if k, ok := ItemKeyFromKey(TagTypeVorbisComments, "key"); !ok || k != ItemKeyInitialKey {
		t.Fatalf("ascii fold failed: %v %v", k, ok)
	}
	if k, ok := ItemKeyFromKey(TagTypeVorbisComments, "\u212Aey"); ok {
		t.Fatalf("KELVIN SIGN matched %v", k)
	}
	if got := asciiLower("ABC\u212A\u00a9Z"); got != "abc\u212A\u00a9z" {
		t.Fatalf("asciiLower = %q", got)
	}
}
