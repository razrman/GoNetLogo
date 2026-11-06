package nl

import (
	"crypto/rand"
	"fmt"
)

// The web World tab shows the RNG seed as a 6-digit base-64 number.
// The alphabet is the standard base64 digit set from RFC 4648 section 4,
// in that order:
//
//	A B C D E F G H I J K L M N O P Q R S T U V W X Y Z
//	a b c d e f g h i j k l m n o p q r s t u v w x y z
//	0 1 2 3 4 5 6 7 8 9 + /
//
// A is 0, B is 1, ..., Z is 25, a is 26, ..., z is 51, 0 is 52, ...,
// 9 is 61, + is 62, and / is 63. The first character is the most
// significant digit, so the integer is
//
//	d0*64^5 + d1*64^4 + d2*64^3 + d3*64^2 + d4*64 + d5
//
// and the range of six digits is exactly 0 .. 2^36-1. DecodeRNGSeed
// produces the integer passed to World.Seed, the same path as the
// random-seed primitive. MersenneTwisterFast.setSeed keeps only the
// low 32 bits of that integer.
const (
	RNGSeedAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	RNGSeedDigits   = 6
	// rngSeedMax is one past the largest 6-digit value, 64^6 = 2^36.
	rngSeedMax = int64(1) << (6 * RNGSeedDigits)
)

var rngSeedIndex = func() [256]int8 {
	var idx [256]int8
	for i := range idx {
		idx[i] = -1
	}
	for i := 0; i < len(RNGSeedAlphabet); i++ {
		idx[RNGSeedAlphabet[i]] = int8(i)
	}
	return idx
}()

// EncodeRNGSeed writes n as six base64 digits. n must be in 0 .. 2^36-1.
// Values outside that range are an error; they are not truncated.
func EncodeRNGSeed(n int64) (string, error) {
	if n < 0 || n >= rngSeedMax {
		return "", fmt.Errorf("RNG seed integer %d is outside 0..%d", n, rngSeedMax-1)
	}
	var b [RNGSeedDigits]byte
	for i := RNGSeedDigits - 1; i >= 0; i-- {
		b[i] = RNGSeedAlphabet[n&63]
		n >>= 6
	}
	return string(b[:]), nil
}

// DecodeRNGSeed parses a 6-digit base64 seed. The text must be exactly
// six characters, each from RNGSeedAlphabet. Anything else is rejected.
func DecodeRNGSeed(s string) (int64, error) {
	if len(s) != RNGSeedDigits {
		return 0, fmt.Errorf("RNG seed must be 6 characters, not %d", len(s))
	}
	var n int64
	for i := 0; i < RNGSeedDigits; i++ {
		d := rngSeedIndex[s[i]]
		if d < 0 {
			return 0, fmt.Errorf("RNG seed must use only the base64 alphabet A-Za-z0-9+/")
		}
		n = n<<6 | int64(d)
	}
	return n, nil
}

// FormatRNGSeed encodes the low 36 bits of seed, which is the whole
// value a 6-digit seed can hold. A clock seed from NewWorld is wider
// than that; the text still round-trips through DecodeRNGSeed and Seed
// to the same generator state, because setSeed uses the low 32 bits.
func FormatRNGSeed(seed int64) string {
	text, err := EncodeRNGSeed(seed & (rngSeedMax - 1))
	if err != nil {
		panic(err)
	}
	return text
}

// GenerateRNGSeed draws a new 6-digit seed from crypto/rand.
// The world's Mersenne Twister is not read or advanced.
func GenerateRNGSeed() (string, error) {
	var buf [5]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate RNG seed: %w", err)
	}
	// Five bytes is 40 bits. The low 36 are one uniform 6-digit value.
	n := int64(buf[0]&0x0f)<<32 |
		int64(buf[1])<<24 |
		int64(buf[2])<<16 |
		int64(buf[3])<<8 |
		int64(buf[4])
	return EncodeRNGSeed(n)
}
