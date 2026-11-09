package nl

import (
	"crypto/rand"
	"fmt"
	"math"
	"strconv"
)

// The web World tab shows the RNG seed as an unsigned decimal integer in
// 0..4294967295. That is the 32 bits MersenneTwisterFast.setSeed keeps.
// A wider value used to be accepted (six base64 digits, 0..2^36-1) and
// then truncated, so distinct text could name the same generator state.
//
// Command-center and model random-seed keep NetLogo's signed int range,
// -2147483648..2147483647. A seed whose low 32 bits are at least 2^31 is
// the same Mersenne Twister state as the negative seed with those bits:
// 4294967295 and -1, and 2147483648 and -2147483648. ParseRNGSeed is the
// World-tab path and rejects anything outside the unsigned 32-bit range.
const rngSeedMax = uint64(math.MaxUint32)

// ParseRNGSeed parses an unsigned decimal seed. The text must be one or
// more digits and the value must be in 0..4294967295. Signs, decimals,
// letters, and values above that range are rejected. Leading zeros are
// accepted and mean the same integer.
func ParseRNGSeed(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("RNG seed must be an integer from 0 to 4294967295")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("RNG seed must be an integer from 0 to 4294967295")
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n > rngSeedMax {
		return 0, fmt.Errorf("RNG seed must be an integer from 0 to 4294967295")
	}
	return int64(n), nil
}

// FormatRNGSeed writes the low 32 bits of seed as an unsigned decimal
// integer. A clock seed from NewWorld is wider than that; the text still
// round-trips through ParseRNGSeed and Seed to the same generator state,
// because setSeed uses those 32 bits. A negative seed, such as random-seed
// -1, is shown as the unsigned value of the same bits.
func FormatRNGSeed(seed int64) string {
	return strconv.FormatUint(uint64(uint32(seed)), 10)
}

// GenerateRNGSeed draws a new unsigned 32-bit seed from crypto/rand and
// returns it as a decimal integer. The world's Mersenne Twister is not
// read or advanced.
func GenerateRNGSeed() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate RNG seed: %w", err)
	}
	n := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return strconv.FormatUint(uint64(n), 10), nil
}
