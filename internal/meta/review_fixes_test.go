package meta

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTaggedFileSaveToPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mp3")
	if err := os.WriteFile(path, mp3Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	tf := NewTaggedFile(FileTypeMpeg, []*Tag{textTag(TagTypeID3v2, "x")}, FileProperties{})

	resetFakeWrites()
	if err := tf.SaveToPath(path, NewWriteOptions()); err != nil {
		t.Fatalf("SaveToPath: %v", err)
	}
	if !slices.Equal(fakeWrites, []string{"MPEG:Id3v2"}) {
		t.Fatalf("writes = %v", fakeWrites)
	}

	// BoundTaggedFile inherits SaveToPath (Rust `AudioFile::save_to_path`).
	bound := NewBoundTaggedFile(NewBytesFile(mp3Bytes()), tf)
	resetFakeWrites()
	if err := bound.SaveToPath(path, NewWriteOptions()); err != nil {
		t.Fatalf("bound SaveToPath: %v", err)
	}
	if !slices.Equal(fakeWrites, []string{"MPEG:Id3v2"}) {
		t.Fatalf("bound writes = %v", fakeWrites)
	}

	// A missing path is an error, and nothing gets created.
	missing := filepath.Join(dir, "missing.mp3")
	err := tf.SaveToPath(missing, NewWriteOptions())
	var fe *FileEncodingError
	if !errors.As(err, &fe) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing path err = %v", err)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatal("SaveToPath must not create the file")
	}
}

func TestRemoveTagTypeReturnsTag(t *testing.T) {
	a := textTag(TagTypeID3v2, "a")
	b := textTag(TagTypeID3v1, "b")
	tf := NewTaggedFile(FileTypeMpeg, []*Tag{a, b}, FileProperties{})
	if got := tf.RemoveTagType(TagTypeID3v1); got != b {
		t.Fatalf("RemoveTagType = %p, want %p", got, b)
	}
	if got := tf.RemoveTagType(TagTypeID3v1); got != nil {
		t.Fatalf("second RemoveTagType = %v, want nil", got)
	}
	if tags := tf.Tags(); len(tags) != 1 || tags[0] != a {
		t.Fatalf("tags = %v", tagTypesOf(tf))
	}
}

func TestTagSliceCopyOnWrite(t *testing.T) {
	a := textTag(TagTypeID3v2, "a")
	b := textTag(TagTypeID3v1, "b")
	c := textTag(TagTypeApe, "c")
	build := func() (*TaggedFile, []*Tag) {
		backing := []*Tag{a, b, c}
		return NewTaggedFile(FileTypeMpeg, backing, FileProperties{}), backing
	}
	assertUntouched := func(name string, backing []*Tag) {
		t.Helper()
		if backing[0] != a || backing[1] != b || backing[2] != c {
			t.Fatalf("%s mutated the caller's slice: %v", name, backing)
		}
	}

	tf, backing := build()
	if !tf.Remove(a) {
		t.Fatal("Remove failed")
	}
	assertUntouched("Remove", backing)

	tf, backing = build()
	tf.RemoveTags(TagTypeID3v2)
	assertUntouched("RemoveTags", backing)
	if len(tf.Tags()) != 2 {
		t.Fatalf("RemoveTags left %v", tagTypesOf(tf))
	}

	tf, backing = build()
	tf.RemoveTagType(TagTypeID3v2)
	assertUntouched("RemoveTagType", backing)

	// Appends must not write into spare capacity shared with another copy.
	shared := make([]*Tag, 1, 4)
	shared[0] = a
	tf1 := NewTaggedFile(FileTypeMpeg, shared, FileProperties{})
	tf2 := NewTaggedFile(FileTypeMpeg, shared, FileProperties{})
	tf1.InsertTag(b)
	tf2.InsertTag(c)
	if got := tf1.Tags(); len(got) != 2 || got[1] != b {
		t.Fatalf("tf1 tags clobbered: %v", tagTypesOf(tf1))
	}
	if got := tf2.Tags(); len(got) != 2 || got[1] != c {
		t.Fatalf("tf2 tags clobbered: %v", tagTypesOf(tf2))
	}

	// BoundTaggedFile.Save drops emptied tags without touching the input slice.
	empty := NewTag(TagTypeID3v1)
	full := textTag(TagTypeID3v2, "x")
	input := []*Tag{empty, full}
	bound := NewBoundTaggedFile(NewBytesFile(mp3Bytes()), NewTaggedFile(FileTypeMpeg, input, FileProperties{}))
	if err := bound.Save(NewWriteOptions()); err != nil {
		t.Fatal(err)
	}
	if input[0] != empty || input[1] != full {
		t.Fatalf("Save mutated the shared slice: %v", input)
	}
	if got := bound.Tags(); len(got) != 1 || got[0] != full {
		t.Fatalf("bound tags after Save = %v", got)
	}
}

func TestRegisterFormatWriterDuplicatePanics(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s did not panic", name)
			}
		}()
		fn()
	}
	writer := func(*VerifiedFile, *Tag, WriteOptions) error { return nil }
	mustPanic("duplicate writer", func() { RegisterFormatWriter(FileTypeMpeg, writer) })
	mustPanic("duplicate reader", func() {
		RegisterFormatReader(FileTypeMpeg, func(io.ReadSeeker, ParseOptions) (*TaggedFile, error) { return nil, nil })
	})
	mustPanic("custom writer", func() { RegisterFormatWriter(FileTypeCustom("x"), writer) })
}

var raceTagTypeSeq atomic.Uint32

func TestRegistriesConcurrent(t *testing.T) {
	base := TagType(100)
	var wg sync.WaitGroup
	// Concurrent registrations (distinct keys across -count runs) racing with lookups.
	for i := 0; i < 4; i++ {
		tt := base + TagType(raceTagTypeSeq.Add(1))
		wg.Add(1)
		go func() {
			defer wg.Done()
			RegisterTagDumper(tt, func(*Tag, io.Writer, WriteOptions) error { return nil })
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = DumpTag(NewTag(TagTypeID3v2), io.Discard, NewWriteOptions())
				_, _ = ReadFormat(FileTypeMpeg, bytes.NewReader(mp3Bytes()), NewParseOptions())
				_, _ = WriteConcreteTag(NewVerifiedFileOf(FileTypeMpeg, NewBytesFile(mp3Bytes())), TagTypeID3v2, nil, NewWriteOptions())
			}
		}()
	}
	wg.Wait()
}

func TestVariantAccessorsReturnCopies(t *testing.T) {
	ext := Extensions()
	ext[0] = "mutated"
	if Extensions()[0] == "mutated" {
		t.Fatal("Extensions exposes internal storage")
	}
	fts := FileTypeVariants()
	fts[0] = FileTypeWav
	if FileTypeVariants()[0] != FileTypeAac {
		t.Fatal("FileTypeVariants exposes internal storage")
	}
	tts := TagTypeVariants()
	tts[0] = TagTypeAiffText
	if TagTypeVariants()[0] != TagTypeApe {
		t.Fatal("TagTypeVariants exposes internal storage")
	}
	for _, tt := range []TagType{TagTypeID3v1, TagTypeLyrics3, TagTypeLyrics3v2, TagTypeID3v2, TagTypeVorbisComments} {
		keys := ItemKeySupportedKeys(tt)
		if len(keys) == 0 {
			t.Fatalf("%v has no keys", tt)
		}
		want := slices.Clone(keys)
		keys[0] = ItemKeyUnknown
		if !slices.Equal(ItemKeySupportedKeys(tt), want) {
			t.Fatalf("ItemKeySupportedKeys(%v) exposes internal storage", tt)
		}
	}
}

func TestItemKeyVariants(t *testing.T) {
	keys := ItemKeyVariants()
	if len(keys) != int(ItemKeyUnknown) {
		t.Fatalf("len = %d, want %d", len(keys), ItemKeyUnknown)
	}
	for i, k := range keys {
		if int(k) != i {
			t.Fatalf("keys[%d] = %v", i, k)
		}
		if k == ItemKeyUnknown {
			t.Fatal("ItemKeyUnknown must not be listed")
		}
	}
	keys[0] = ItemKeyUnknown
	if ItemKeyVariants()[0] != ItemKeyAlbumTitle {
		t.Fatal("ItemKeyVariants must return a fresh slice")
	}
}

func TestTagItemEqual(t *testing.T) {
	mk := func() TagItem {
		it := NewTagItem(ItemKeyTrackTitle, ItemValueText("x"))
		it.Description = "d"
		it.Lang = LangEnglish
		return it
	}
	a, b := mk(), mk()
	if !a.Equal(b) {
		t.Fatal("identical items must be equal")
	}
	diff := mk()
	diff.SetKey(ItemKeyAlbumTitle)
	if a.Equal(diff) {
		t.Error("key differs")
	}
	diff = mk()
	diff.SetValue(ItemValueLocator("x"))
	if a.Equal(diff) {
		t.Error("value kind differs")
	}
	diff = mk()
	diff.Description = "e"
	if a.Equal(diff) {
		t.Error("description differs")
	}
	diff = mk()
	diff.Lang = LangUnknown
	if a.Equal(diff) {
		t.Error("lang differs")
	}
}

func TestPictureEqual(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 1, 2, 3)
	mk := func() Picture {
		return PictureUnchecked(slices.Clone(png)).PicType(PictureTypeCoverFront).MimeType(MimeTypePng).Description("d").Build()
	}
	a, b := mk(), mk()
	if !a.Equal(b) {
		t.Fatal("identical pictures must be equal")
	}
	for name, mutate := range map[string]func(*Picture){
		"type":      func(p *Picture) { p.SetPicType(PictureTypeCoverBack) },
		"mime":      func(p *Picture) { p.SetMimeType(MimeTypeJpeg) },
		"desc":      func(p *Picture) { p.SetDescription("other") },
		"no desc":   func(p *Picture) { p.ClearDescription() },
		"data":      func(p *Picture) { p.data[len(p.data)-1] ^= 1 },
		"data size": func(p *Picture) { p.data = p.data[:len(p.data)-1] },
	} {
		c := mk()
		mutate(&c)
		if a.Equal(c) {
			t.Errorf("%s: pictures should differ", name)
		}
	}
	// Some("") differs from None.
	e1, e2 := mk(), mk()
	e1.SetDescription("")
	e2.ClearDescription()
	if e1.Equal(e2) {
		t.Error("empty description must differ from no description")
	}
}

func TestPictureFromApeBytesCopiesData(t *testing.T) {
	raw := append([]byte("desc\x00"), 0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 9, 9)
	pic, err := PictureFromApeBytes("Cover Art (Front)", raw)
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Clone(pic.Data())
	for i := range raw {
		raw[i] = 0
	}
	if !bytes.Equal(pic.Data(), want) {
		t.Fatal("picture data aliases the input buffer")
	}
	if d, _ := pic.Description(); d != "desc" {
		t.Fatalf("description = %q", d)
	}
}

func TestAudioObjectTypeConstants(t *testing.T) {
	// Values from Rust mp4/properties.rs `AudioObjectType`.
	want := map[AudioObjectType]uint8{
		AudioObjectTypeNull:                                         0,
		AudioObjectTypeAacMain:                                      1,
		AudioObjectTypeAacLowComplexity:                             2,
		AudioObjectTypeAacScalableSampleRate:                        3,
		AudioObjectTypeAacLongTermPrediction:                        4,
		AudioObjectTypeSpectralBandReplication:                      5,
		AudioObjectTypeAACScalable:                                  6,
		AudioObjectTypeTwinVQ:                                       7,
		AudioObjectTypeCodeExcitedLinearPrediction:                  8,
		AudioObjectTypeHarmonicVectorExcitationCoding:               9,
		AudioObjectTypeTextToSpeechtInterface:                       12,
		AudioObjectTypeMainSynthetic:                                13,
		AudioObjectTypeWavetableSynthesis:                           14,
		AudioObjectTypeGeneralMIDI:                                  15,
		AudioObjectTypeAlgorithmicSynthesis:                         16,
		AudioObjectTypeErrorResilientAacLowComplexity:               17,
		AudioObjectTypeErrorResilientAacLongTermPrediction:          19,
		AudioObjectTypeErrorResilientAacScalable:                    20,
		AudioObjectTypeErrorResilientAacTwinVQ:                      21,
		AudioObjectTypeErrorResilientAacBitSlicedArithmeticCoding:   22,
		AudioObjectTypeErrorResilientAacLowDelay:                    23,
		AudioObjectTypeErrorResilientCodeExcitedLinearPrediction:    24,
		AudioObjectTypeErrorResilientHarmonicVectorExcitationCoding: 25,
		AudioObjectTypeErrorResilientHarmonicIndividualLinesNoise:   26,
		AudioObjectTypeErrorResilientParametric:                     27,
		AudioObjectTypeSinuSoidalCoding:                             28,
		AudioObjectTypeParametricStereo:                             29,
		AudioObjectTypeMpegSurround:                                 30,
		AudioObjectTypeMpegLayer1:                                   32,
		AudioObjectTypeMpegLayer2:                                   33,
		AudioObjectTypeMpegLayer3:                                   34,
		AudioObjectTypeDirectStreamTransfer:                         35,
		AudioObjectTypeAudioLosslessCoding:                          36,
		AudioObjectTypeScalableLosslessCoding:                       37,
		AudioObjectTypeScalableLosslessCodingNoneCore:               38,
		AudioObjectTypeErrorResilientAacEnhancedLowDelay:            39,
		AudioObjectTypeSymbolicMusicRepresentationSimple:            40,
		AudioObjectTypeSymbolicMusicRepresentationMain:              41,
		AudioObjectTypeUnifiedSpeechAudioCoding:                     42,
		AudioObjectTypeSpatialAudioObjectCoding:                     43,
		AudioObjectTypeLowDelayMpegSurround:                         44,
		AudioObjectTypeSpatialAudioObjectCodingDialogueEnhancement:  45,
		AudioObjectTypeAudioSync:                                    46,
	}
	// Rust has 43 variants (NULL plus 42 named; values 10, 11, 18 and 31 are unassigned).
	if len(want) != 43 {
		t.Fatalf("expected 43 distinct constants, got %d (duplicate values?)", len(want))
	}
	for k, v := range want {
		if uint8(k) != v {
			t.Errorf("constant %d != %d", k, v)
		}
	}
}
