package nl

import (
	"strings"
	"testing"
)

func TestRNGSeedEncodeDecode(t *testing.T) {
	cases := []struct {
		text string
		n    int64
	}{
		{"AAAAAA", 0},
		{"AAAAAB", 1},
		{"AAAAA/", 63},
		{"AAAABA", 64},
		{"AAAABB", 65},
		{"BAAAAA", 1 << 30},
		{"CAAAAA", 1 << 31},
		{"//////", (1 << 36) - 1},
	}
	for _, tc := range cases {
		got, err := DecodeRNGSeed(tc.text)
		if err != nil {
			t.Fatalf("decode %q: %v", tc.text, err)
		}
		if got != tc.n {
			t.Fatalf("decode %q = %d, want %d", tc.text, got, tc.n)
		}
		back, err := EncodeRNGSeed(tc.n)
		if err != nil {
			t.Fatalf("encode %d: %v", tc.n, err)
		}
		if back != tc.text {
			t.Fatalf("encode %d = %q, want %q", tc.n, back, tc.text)
		}
	}

	for n := int64(0); n < 4096; n++ {
		text, err := EncodeRNGSeed(n)
		if err != nil {
			t.Fatal(err)
		}
		if len(text) != RNGSeedDigits {
			t.Fatalf("encode %d length %d", n, len(text))
		}
		got, err := DecodeRNGSeed(text)
		if err != nil || got != n {
			t.Fatalf("round trip %d -> %q -> %d, %v", n, text, got, err)
		}
	}
}

func TestRNGSeedRejectsInvalid(t *testing.T) {
	bad := []string{
		"",
		"A",
		"AAAAA",
		"AAAAAAA",
		"AAAAA=",
		"AAAAA ",
		" AAAAA",
		"aaaaa ",
		"AAAAA\n",
		"AAA*AA",
		"////// ",
		"AAAAAé",
	}
	for _, s := range bad {
		if _, err := DecodeRNGSeed(s); err == nil {
			t.Fatalf("decode %q succeeded", s)
		}
	}
	if _, err := DecodeRNGSeed("AAAAA="); err == nil || !strings.Contains(err.Error(), "alphabet") {
		t.Fatalf("padding error %v", err)
	}
	if _, err := DecodeRNGSeed("AAAAA"); err == nil || !strings.Contains(err.Error(), "6 characters") {
		t.Fatalf("length error %v", err)
	}
	if _, err := EncodeRNGSeed(-1); err == nil {
		t.Fatal("encode -1 succeeded")
	}
	if _, err := EncodeRNGSeed(1 << 36); err == nil {
		t.Fatal("encode 2^36 succeeded")
	}
}

func TestFormatRNGSeedKeepsLowBits(t *testing.T) {
	if got := FormatRNGSeed(1); got != "AAAAAB" {
		t.Fatalf("format 1 = %q", got)
	}
	if got := FormatRNGSeed(-1); got != "//////" {
		t.Fatalf("format -1 = %q", got)
	}
	const wide = int64(0x123456789abcdef)
	text := FormatRNGSeed(wide)
	n, err := DecodeRNGSeed(text)
	if err != nil {
		t.Fatal(err)
	}
	if n != wide&(1<<36-1) {
		t.Fatalf("format %#x -> %d, want low 36 bits", wide, n)
	}
	// setSeed uses the low 32 bits, so formatting and reseeding restarts
	// the same sequence the wide seed was already producing.
	a := NewWorld(0, 0, 0, 0, false, false)
	b := NewWorld(0, 0, 0, 0, false, false)
	a.Seed(wide)
	b.Seed(n)
	for i := 0; i < 5; i++ {
		gotA, err := a.RandomInt(1000)
		if err != nil {
			t.Fatal(err)
		}
		gotB, err := b.RandomInt(1000)
		if err != nil {
			t.Fatal(err)
		}
		if gotA != gotB {
			t.Fatalf("draw %d: formatted seed %d, original %d", i, gotB, gotA)
		}
	}
}

func TestDecodeSeedMatchesRandomSeed(t *testing.T) {
	// "AAAAAB" is 1. random 100 after random-seed 1 is 81.
	n, err := DecodeRNGSeed("AAAAAB")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("decoded %d", n)
	}
	w := NewWorld(-1, 1, -1, 1, true, true)
	w.Seed(n)
	got, err := w.RandomInt(100)
	if err != nil {
		t.Fatal(err)
	}
	if got != 81 {
		t.Fatalf("random 100 after seed text AAAAAB = %d, want 81", got)
	}

	// A value above the signed 32-bit range still follows setSeed's low 32 bits.
	text := "CAAAAA" // 2^31
	n, err = DecodeRNGSeed(text)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1<<31 {
		t.Fatalf("decode %q = %d", text, n)
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
			t.Fatalf("draw %d: full seed %d, low 32 bits %d", i, a, b)
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
		n, err := DecodeRNGSeed(text)
		if err != nil {
			t.Fatalf("generated %q: %v", text, err)
		}
		back, err := EncodeRNGSeed(n)
		if err != nil || back != text {
			t.Fatalf("generated %q reencodes as %q (%v)", text, back, err)
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
	n, err := DecodeRNGSeed(text)
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
