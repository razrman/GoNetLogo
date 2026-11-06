package nl

import (
	"math"
	"testing"
)

// The expected values in this file were produced by compiling NetLogo
// 6.4.0's own org.nlogo.api.MersenneTwisterFast.java (netlogo-core/src/
// main/api) with JDK 17 and printing its output, including raw double bits
// and StrictMath.log. They pin the Go port to the Java generator bit for
// bit.

// TestMersenneNetLogoUpstreamVectors repeats NetLogo's own
// MersenneTwisterFastTests.
func TestMersenneNetLogoUpstreamVectors(t *testing.T) {
	r := NewMersenneTwister(78645)
	for round := 0; round < 2; round++ {
		for i, want := range []int32{35398, 43344, 62618} {
			if got := r.NextIntN(100000); got != want {
				t.Fatalf("round %d nextInt(100000) #%d = %d, want %d", round, i, got, want)
			}
		}
		r.SetSeed(78645)
	}
	r = NewMersenneTwister(91773)
	for round := 0; round < 2; round++ {
		for i, want := range []float64{0.25234274343179397, 0.5371597936195429, 0.5418160939231073} {
			if got := r.NextDouble(); got != want {
				t.Fatalf("round %d nextDouble #%d = %v, want %v", round, i, got, want)
			}
		}
		r.SetSeed(91773)
	}
}

func TestMersenneNextLongN(t *testing.T) {
	cases := map[int64][]int64{
		1:          {81, 88, 80, 90, 48, 28, 39, 46, 8, 26},
		42:         {1, 55, 67, 74, 56, 51, 57, 74, 67, 21},
		0:          {83, 16, 65, 99, 49, 81, 87, 32, 22, 28},
		78645:      {4, 68, 63, 92, 53, 84, 59, 36, 71, 42},
		91773:      {86, 0, 50, 72, 22, 39, 45, 45, 97, 71},
		-7:         {42, 21, 64, 97, 93, 66, 11, 32, 23, 7},
		2147483647: {37, 90, 36, 92, 59, 17, 99, 40, 61, 46},
		-1 << 31:   {86, 13, 8, 90, 19, 60, 38, 95, 88, 75},
		1 << 31:    {86, 13, 8, 90, 19, 60, 38, 95, 88, 75}, // only the low 32 bits count
		1<<32 + 1:  {81, 88, 80, 90, 48, 28, 39, 46, 8, 26}, // same as seed 1
	}
	for seed, want := range cases {
		r := NewMersenneTwister(seed)
		for i, w := range want {
			if got := r.NextLongN(100); got != w {
				t.Fatalf("seed %d nextLong(100) #%d = %d, want %d", seed, i, got, w)
			}
		}
	}
	// Large bounds exercise the sign-extended 64-bit assembly and the
	// rejection test.
	r := NewMersenneTwister(1)
	bounds := []int64{1, 3, 1 << 40, math.MaxInt64, (math.MaxInt64 / 3) * 2, 4611686018427387905}
	want := []int64{0, 2, 547883475396, 2788525751209594690, 1353584196101614048, 851673220871563328,
		0, 0, 837380193932, 4969707177562598026, 3866387005719533746, 1885739157146505342}
	for i, w := range want {
		if got := r.NextLongN(bounds[i%len(bounds)]); got != w {
			t.Fatalf("nextLong(%d) #%d = %d, want %d", bounds[i%len(bounds)], i, got, w)
		}
	}
}

func TestMersenneNextIntN(t *testing.T) {
	r := NewMersenneTwister(1)
	bounds := []int32{1, 2, 3, 7, 14, 100, 360, 1024, 1 << 30, 2147483647, 1000000007}
	want := []int32{0, 1, 0, 5, 1, 56, 5, 1023, 157577939, 506997216, 198295624,
		0, 0, 2, 4, 6, 51, 340, 551, 908719429, 900213375, 672749746,
		0, 1, 1, 6, 13, 26, 134, 547, 719909004, 1962718498, 896155243}
	for i, w := range want {
		if got := r.NextIntN(bounds[i%len(bounds)]); got != w {
			t.Fatalf("nextInt(%d) #%d = %d, want %d", bounds[i%len(bounds)], i, got, w)
		}
	}
}

// TestMersenneNextIntRefill checks raw words across the 624-word refill.
func TestMersenneNextIntRefill(t *testing.T) {
	r := NewMersenneTwister(1)
	want := map[int]int32{
		0: 1791095845, 50: -452660932, 100: 83180453, 500: 1726676,
		550: -1383834647, 600: -808060760, 621: -1974376874, 622: -388112460,
		623: 2006116153, 624: 1104314680, 625: 939235918, 629: 2062779089,
		650: 2049304928,
	}
	for i := 0; i <= 650; i++ {
		got := r.NextInt()
		if w, ok := want[i]; ok && got != w {
			t.Fatalf("nextInt() #%d = %d, want %d", i, got, w)
		}
	}
}

func TestMersenneNextDoubleAndGaussianBits(t *testing.T) {
	cases := []struct {
		seed   int64
		double []uint64
		gauss  []uint64
	}{
		{1,
			[]uint64{0x3fdab07d0ff4780e, 0x3fe70ce5f775e0a2, 0x3f1dfb20ccc38000, 0x3fd3596abffc11e8, 0x3fc2c8e5a78e0a78},
			[]uint64{0xbfe3938220b10c84, 0x3ff9fd518a165ffe, 0xbff12ae125af3e4e, 0xbfe0e6c879c711cb, 0xc002698d23c348bc}},
		{42,
			[]uint64{0x3fd7f8771cbea3da, 0x3fee6c406977aca8, 0x3fe76c7e8e3ccea2, 0x3fe32835d4c65960, 0x3fc3f86b2e44430c},
			[]uint64{0xbfc1b2a500224593, 0x3fdfca2a1989110c, 0x3ff85e5492a67b9f, 0x3fe4b9dd4bf7231b, 0xbfcdf832fce50abd}},
	}
	for _, c := range cases {
		r := NewMersenneTwister(c.seed)
		for i, w := range c.double {
			if got := math.Float64bits(r.NextDouble()); got != w {
				t.Fatalf("seed %d nextDouble #%d bits %x, want %x", c.seed, i, got, w)
			}
		}
		r.SetSeed(c.seed)
		for i, w := range c.gauss {
			if got := math.Float64bits(r.NextGaussian()); got != w {
				t.Fatalf("seed %d nextGaussian #%d bits %x, want %x", c.seed, i, got, w)
			}
		}
	}
}

// TestMersenneReseedClearsGaussian checks setSeed drops the cached second
// Gaussian, as MersenneTwisterFast does.
func TestMersenneReseedClearsGaussian(t *testing.T) {
	r := NewMersenneTwister(1)
	r.NextGaussian() // caches the pair's second value
	r.SetSeed(1)
	if got := math.Float64bits(r.NextGaussian()); got != 0xbfe3938220b10c84 {
		t.Fatalf("first Gaussian after reseed bits %x", got)
	}
}

// TestStrictLog checks the fdlibm port against Java's StrictMath.log.
func TestStrictLog(t *testing.T) {
	cases := []struct {
		x    float64
		bits uint64
	}{
		{1, 0},
		{2, 0x3fe62e42fefa39ef},
		{0.5, 0xbfe62e42fefa39ef},
		{1e-300, 0xc085963447f87fb5},
		{4.9e-324, 0xc0874385446d71c3},
		{1e300, 0x4085963447f87fb5},
		{2.2250738585072014e-308, 0xc086232bdd7abcd2},
		{1.0000001, 0x3e7ad7f2847b6492},
		{0.9999999, 0xbe7ad7f2b1049b9f},
		{math.E, 0x3ff0000000000000},
		{10, 0x40026bb1bbb55516},
	}
	for _, c := range cases {
		if got := math.Float64bits(strictLog(c.x)); got != c.bits {
			t.Errorf("strictLog(%v) bits %x, want %x", c.x, got, c.bits)
		}
	}
	if !math.IsInf(strictLog(0), -1) || !math.IsNaN(strictLog(-1)) {
		t.Fatal("strictLog edge cases")
	}
	// -log(nextDouble) for seed 1, as random-exponential 1 computes it.
	r := NewMersenneTwister(1)
	for i, w := range []uint64{0x3febfcdb4d05fefb, 0x3fd4fed3ff065a14, 0x402226f4cce42f76, 0x3ff323bf8fb506be} {
		if got := math.Float64bits(-1.0 * strictLog(r.NextDouble())); got != w {
			t.Fatalf("exp #%d bits %x, want %x", i, got, w)
		}
	}
}
