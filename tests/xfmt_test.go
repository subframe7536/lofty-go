package lofty_test

import (
	"bytes"
	"testing"

	bogem "github.com/bogem/id3v2/v2"

	"github.com/subframe7536/lofty-go/id3v2"

	. "github.com/subframe7536/lofty-go"
)

func xfmtId3v2Dump(t *testing.T, tag *Tag, useV23 bool) []byte {
	t.Helper()
	wo := NewWriteOptions()
	wo.UseID3v23 = useV23
	encoded, err := id3v2.Id3v2Serialize(tag, wo)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	return encoded
}

func id3v2Reparse(t *testing.T, b []byte, opts ParseOptions) *Tag {
	return xfmtId3v2Reparse(t, b, opts)
}

func xfmtId3v2Reparse(t *testing.T, b []byte, opts ParseOptions) *Tag {
	t.Helper()
	r := bytes.NewReader(b)
	header, err := id3v2.ParseId3v2HeaderRaw(r)
	if err != nil {
		t.Fatalf("header re-parse failed: %v", err)
	}
	tag, err := id3v2.ParseId3v2(r, header, opts)
	if err != nil {
		t.Fatalf("tag re-parse failed: %v", err)
	}
	return tag
}

func bogemFrames(t *testing.T, b []byte) *bogem.Tag {
	t.Helper()
	btag, err := bogem.ParseReader(bytes.NewReader(b), bogem.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	return btag
}

func TestItunesAdvisoryRoundtrip(t *testing.T) {
	ilst := NewIlst()
	ilst.SetAdvisoryRating(1)

	tag := ilst.Tag()
	encoded := xfmtId3v2Dump(t, tag, false)
	btag := bogemFrames(t, encoded)

	found := false
	for _, f := range btag.GetFrames("TXXX") {
		if udtf, ok := f.(bogem.UserDefinedTextFrame); ok && udtf.Description == "ITUNESADVISORY" {
			if udtf.Value != "1" {
				t.Fatalf("ITUNESADVISORY = %q", udtf.Value)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("ITUNESADVISORY TXXX missing")
	}

	re := xfmtId3v2Reparse(t, encoded, NewParseOptions())
	back := IlstFromTag(re)
	if v, ok := back.AdvisoryRating(); !ok || v != 1 {
		t.Fatalf("advisory = %v", v)
	}
}
