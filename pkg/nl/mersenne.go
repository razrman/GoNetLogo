package nl

import (
	"math"
	"sync/atomic"
)

// MersenneTwister is a port of Sean Luke's MersenneTwisterFast as shipped
// in NetLogo (org.nlogo.api.MersenneTwisterFast, NetLogo 6.4.0). NetLogo
// uses it for the world RNG, so every method here consumes 32-bit words
// exactly as the Java code does. Given the same seed, the same calls give
// the same numbers NetLogo Desktop gives.
//
// Only the methods NetLogo primitives use are ported. Do not add helpers
// that read bits in a different pattern; that breaks NetLogo parity.
type MersenneTwister struct {
	mt  [mtN]uint32
	mti int

	nextNextGaussian     float64
	haveNextNextGaussian bool

	// draws counts 32-bit words taken from the generator. It is nil for a
	// generator that does not need counting.
	draws *atomic.Uint64
}

const (
	mtN         = 624
	mtM         = 397
	mtMatrixA   = 0x9908b0df
	mtUpperMask = 0x80000000
	mtLowerMask = 0x7fffffff
	mtTemperB   = 0x9d2c5680
	mtTemperC   = 0xefc60000
)

// NewMersenneTwister returns a generator seeded like
// new MersenneTwisterFast(seed).
func NewMersenneTwister(seed int64) *MersenneTwister {
	r := &MersenneTwister{}
	r.SetSeed(seed)
	return r
}

// SetSeed matches MersenneTwisterFast.setSeed(long). Only the low 32 bits
// of seed are used. The cached Gaussian is discarded.
func (r *MersenneTwister) SetSeed(seed int64) {
	r.haveNextNextGaussian = false
	r.nextNextGaussian = 0
	r.mt[0] = uint32(seed)
	for i := 1; i < mtN; i++ {
		prev := r.mt[i-1]
		r.mt[i] = 1812433253*(prev^(prev>>30)) + uint32(i)
	}
	r.mti = mtN
}

// next32 returns the next tempered 32-bit word. Java treats it as a signed
// int; callers convert where the Java code relies on the sign.
func (r *MersenneTwister) next32() uint32 {
	if r.mti >= mtN {
		mt := &r.mt
		var y uint32
		kk := 0
		for ; kk < mtN-mtM; kk++ {
			y = (mt[kk] & mtUpperMask) | (mt[kk+1] & mtLowerMask)
			mt[kk] = mt[kk+mtM] ^ (y >> 1) ^ ((y & 1) * mtMatrixA)
		}
		for ; kk < mtN-1; kk++ {
			y = (mt[kk] & mtUpperMask) | (mt[kk+1] & mtLowerMask)
			mt[kk] = mt[kk+(mtM-mtN)] ^ (y >> 1) ^ ((y & 1) * mtMatrixA)
		}
		y = (mt[mtN-1] & mtUpperMask) | (mt[0] & mtLowerMask)
		mt[mtN-1] = mt[mtM-1] ^ (y >> 1) ^ ((y & 1) * mtMatrixA)
		r.mti = 0
	}
	y := r.mt[r.mti]
	r.mti++
	y ^= y >> 11
	y ^= (y << 7) & mtTemperB
	y ^= (y << 15) & mtTemperC
	y ^= y >> 18
	if r.draws != nil {
		r.draws.Add(1)
	}
	return y
}

// NextInt matches nextInt(): a uniformly distributed signed 32-bit int.
func (r *MersenneTwister) NextInt() int32 {
	return int32(r.next32())
}

// NextIntN matches nextInt(int n): uniform in [0, n). n must be in
// (0, 2^31). It panics otherwise, as Java throws.
func (r *MersenneTwister) NextIntN(n int32) int32 {
	if n <= 0 {
		panic("MersenneTwister.NextIntN: n must be positive")
	}
	if n&-n == n { // power of two
		y := r.next32()
		return int32((int64(n) * int64(y>>1)) >> 31)
	}
	for {
		bits := int32(r.next32() >> 1)
		val := bits % n
		// Java's overflow check: bits - val + (n-1) < 0 in int arithmetic.
		if bits-val+(n-1) >= 0 {
			return val
		}
	}
}

// NextLongN matches nextLong(long n): uniform in [0, n). n must be > 0.
func (r *MersenneTwister) NextLongN(n int64) int64 {
	if n <= 0 {
		panic("MersenneTwister.NextLongN: n must be positive")
	}
	for {
		y := r.next32()
		z := r.next32()
		// Java computes (((long) y) << 32) + z with z a signed int, so z is
		// sign-extended before the add, then shifts right unsigned.
		sum := int64(int32(y))<<32 + int64(int32(z))
		bits := int64(uint64(sum) >> 1)
		val := bits % n
		// Java's overflow check in long arithmetic. Go integer overflow
		// wraps the same way.
		if bits-val+(n-1) >= 0 {
			return val
		}
	}
}

// NextDouble matches nextDouble(): 26 high bits of one word and 27 of the
// next, scaled into [0, 1).
func (r *MersenneTwister) NextDouble() float64 {
	y := r.next32()
	z := r.next32()
	return float64(uint64(y>>6)<<27+uint64(z>>5)) / float64(uint64(1)<<53)
}

// NextGaussian matches nextGaussian(): the polar Box-Muller method with
// one cached value, using StrictMath (fdlibm) log and a correctly rounded
// sqrt. Products are wrapped in float64() so the compiler cannot fuse
// them into FMA instructions, which Java never does.
func (r *MersenneTwister) NextGaussian() float64 {
	if r.haveNextNextGaussian {
		r.haveNextNextGaussian = false
		return r.nextNextGaussian
	}
	var v1, v2, s float64
	for {
		v1 = float64(2*r.NextDouble()) - 1
		v2 = float64(2*r.NextDouble()) - 1
		s = float64(v1*v1) + float64(v2*v2)
		if s < 1 && s != 0 {
			break
		}
	}
	multiplier := math.Sqrt(float64(-2*strictLog(s)) / s)
	r.nextNextGaussian = v2 * multiplier
	r.haveNextNextGaussian = true
	return v1 * multiplier
}

// strictLog is fdlibm's __ieee754_log (e_log.c), which is what Java's
// StrictMath.log computes. Go's math.Log may use an assembly
// implementation that differs in the last bit, so NetLogo parity needs
// this port.
func strictLog(x float64) float64 {
	const (
		ln2Hi = 6.93147180369123816490e-01 // 0x3fe62e42 fee00000
		ln2Lo = 1.90821492927058770002e-10 // 0x3dea39ef 35793c76
		two54 = 1.80143985094819840000e+16 // 0x43500000 00000000
		lg1   = 6.666666666666735130e-01   // 0x3FE55555 55555593
		lg2   = 3.999999999940941908e-01   // 0x3FD99999 9997FA04
		lg3   = 2.857142874366239149e-01   // 0x3FD24924 94229359
		lg4   = 2.222219843214978396e-01   // 0x3FCC71C5 1D8E78AF
		lg5   = 1.818357216161805012e-01   // 0x3FC74664 96CB03DE
		lg6   = 1.531383769920937332e-01   // 0x3FC39A09 D078C69F
		lg7   = 1.479819860511658591e-01   // 0x3FC2F112 DF3E5244
		zero  = 0.0
	)
	bits := math.Float64bits(x)
	hx := int32(bits >> 32)
	lx := uint32(bits)

	k := int32(0)
	if hx < 0x00100000 { // x < 2**-1022
		if (hx&0x7fffffff)|int32(lx) == 0 {
			return math.Inf(-1) // log(+-0) = -inf
		}
		if hx < 0 {
			return math.NaN() // log(-#) = NaN
		}
		k -= 54
		x = float64(x * two54) // subnormal number, scale up x
		hx = int32(math.Float64bits(x) >> 32)
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	k += (hx >> 20) - 1023
	hx &= 0x000fffff
	i := (hx + 0x95f64) & 0x100000
	// normalize x or x/2
	x = math.Float64frombits(uint64(uint32(hx|(i^0x3ff00000)))<<32 | uint64(uint32(math.Float64bits(x))))
	k += i >> 20
	f := x - 1.0
	if (0x000fffff & (2 + hx)) < 3 { // |f| < 2**-20
		if f == zero {
			if k == 0 {
				return zero
			}
			dk := float64(k)
			return float64(dk*ln2Hi) + float64(dk*ln2Lo)
		}
		R := float64(float64(f*f) * (0.5 - float64(0.33333333333333333*f)))
		if k == 0 {
			return f - R
		}
		dk := float64(k)
		return float64(dk*ln2Hi) - ((R - float64(dk*ln2Lo)) - f)
	}
	s := f / (2.0 + f)
	dk := float64(k)
	z := float64(s * s)
	i = hx - 0x6147a
	w := float64(z * z)
	j := 0x6b851 - hx
	t1 := float64(w * (lg2 + float64(w*(lg4+float64(w*lg6)))))
	t2 := float64(z * (lg1 + float64(w*(lg3+float64(w*(lg5+float64(w*lg7)))))))
	i |= j
	R := t2 + t1
	if i > 0 {
		hfsq := float64(float64(0.5*f) * f)
		if k == 0 {
			return f - (hfsq - float64(s*(hfsq+R)))
		}
		return float64(dk*ln2Hi) - ((hfsq - (float64(s*(hfsq+R)) + float64(dk*ln2Lo))) - f)
	}
	if k == 0 {
		return f - float64(s*(f-R))
	}
	return float64(dk*ln2Hi) - ((float64(s*(f-R)) - float64(dk*ln2Lo)) - f)
}
