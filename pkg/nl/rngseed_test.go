package nl

import (
	"strconv"
	"testing"
)

func TestRNGSeedParseRoundTrip(t *testing.T) {
	cases := []struct {
		text string
		n    int64
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"2147483647", 1<<31 - 1},
		{"2147483648", 1 << 31},
		{"4294967295", int64(^uint32(0))},
	}
	for _, tc := range cases {
		got, err := ParseRNGSeed(tc.text)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.text, err)
		}
		if got != tc.n {
			t.Fatalf("parse %q = %d, want %d", tc.text, got, tc.n)
		}
		if back := FormatRNGSeed(tc.n); back != tc.text {
			t.Fatalf("format %d = %q, want %q", tc.n, back, tc.text)
		}
	}
	for n := int64(0); n < 4096; n++ {
		text := FormatRNGSeed(n)
		got, err := ParseRNGSeed(text)
		if err != nil || got != n {
			t.Fatalf("round trip %d -> %q -> %d, %v", n, text, got, err)
		}
	}
}

func TestRNGSeedRejectsInvalid(t *testing.T) {
	bad := []string{
		"",
		"-1",
		"-0",
		"1.5",
		"1.0",
		"abc",
		"12a",
		"4294967296",
		"9999999999",
		"+1",
		" 1",
		"1 ",
		"1e2",
		"0x10",
	}
	for _, s := range bad {
		if _, err := ParseRNGSeed(s); err == nil {
			t.Fatalf("parse %q succeeded", s)
		}
	}
}

func TestFormatRNGSeedKeepsLow32Bits(t *testing.T) {
	if got := FormatRNGSeed(1); got != "1" {
		t.Fatalf("format 1 = %q", got)
	}
	// random-seed -1 uses the same 32 bits as the unsigned seed 2^32-1.
	if got := FormatRNGSeed(-1); got != "4294967295" {
		t.Fatalf("format -1 = %q", got)
	}
	if got := FormatRNGSeed(int64(int32(-2147483648))); got != "2147483648" {
		t.Fatalf("format min int32 = %q", got)
	}
	const wide = int64(0x123456789abcdef)
	text := FormatRNGSeed(wide)
	n, err := ParseRNGSeed(text)
	if err != nil {
		t.Fatal(err)
	}
	if n != wide&0xffffffff {
		t.Fatalf("format %#x -> %d, want low 32 bits", wide, n)
	}
	// setSeed uses the low 32 bits, so formatting and reseeding restarts
	// the same sequence the wide seed was already producing.
	a := NewWorld(0, 0, 0, 0, false, false)
	b := NewWorld(0, 0, 0, 0, false, false)
	a.Seed(wide)
	b.Seed(n)
	for i := 0; i < 5; i++ {
		gotA, err := a.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		gotB, err := b.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		if gotA != gotB {
			t.Fatalf("draw %d: formatted seed %d, original %d", i, gotB, gotA)
		}
	}
}

func TestParseSeedMatchesRandomSeed(t *testing.T) {
	// "1" is random-seed 1, and random 100 then yields 81.
	n, err := ParseRNGSeed("1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("parsed %d", n)
	}
	w := NewWorld(-1, 1, -1, 1, true, true)
	w.Seed(n)
	got, err := w.RandomInt(100)
	if err != nil {
		t.Fatal(err)
	}
	if got != 81 {
		t.Fatalf("random 100 after seed text 1 = %d, want 81", got)
	}

	// An unsigned value at or above 2^31 is the same generator state as
	// the signed seed random-seed accepts for those bits.
	hiText := "2147483648" // 2^31
	n, err = ParseRNGSeed(hiText)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1<<31 {
		t.Fatalf("parse %q = %d", hiText, n)
	}
	hi := NewWorld(0, 0, 0, 0, false, false)
	lo := NewWorld(0, 0, 0, 0, false, false)
	hi.Seed(n)
	lo.Seed(int64(int32(uint32(n))))
	for i := 0; i < 4; i++ {
		a, err := hi.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		b, err := lo.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("draw %d: unsigned seed %d, signed low 32 bits %d", i, a, b)
		}
	}

	top := NewWorld(0, 0, 0, 0, false, false)
	neg := NewWorld(0, 0, 0, 0, false, false)
	top.Seed(int64(^uint32(0))) // 4294967295
	neg.Seed(-1)
	for i := 0; i < 4; i++ {
		a, err := top.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		b, err := neg.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("draw %d: 4294967295 -> %d, random-seed -1 -> %d", i, a, b)
		}
	}
}

func TestGenerateRNGSeed(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 24; i++ {
		text, err := GenerateRNGSeed()
		if err != nil {
			t.Fatal(err)
		}
		n, err := ParseRNGSeed(text)
		if err != nil {
			t.Fatalf("generated %q: %v", text, err)
		}
		if n < 0 || n > int64(^uint32(0)) {
			t.Fatalf("generated %q = %d", text, n)
		}
		if FormatRNGSeed(n) != strconv.FormatInt(n, 10) {
			t.Fatalf("generated %q formats as %s", text, FormatRNGSeed(n))
		}
		seen[text] = true
	}
	if len(seen) < 2 {
		t.Fatalf("generator returned a single value %v", seen)
	}

	fresh := NewWorld(0, 4, 0, 4, false, false)
	fresh.Seed(1)
	before, err := fresh.RandomInt(100)
	if err != nil {
		t.Fatal(err)
	}
	if before != 81 {
		t.Fatalf("pre-generate draw %d", before)
	}
	// Further draws move the sequence. Generating a seed must not read
	// this generator, and applying the new text restarts from that seed.
	if _, err := fresh.RandomInt(100); err != nil {
		t.Fatal(err)
	}
	text, err := GenerateRNGSeed()
	if err != nil {
		t.Fatal(err)
	}
	n, err := ParseRNGSeed(text)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Seed(n)
	got, err := fresh.RandomInt(50)
	if err != nil {
		t.Fatal(err)
	}
	other := NewWorld(0, 4, 0, 4, false, false)
	other.Seed(n)
	want, err := other.RandomInt(50)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("after generate %q: random 50 = %d, want %d", text, got, want)
	}
}
