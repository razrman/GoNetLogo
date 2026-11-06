// Six-digit base64 seed for the World tab. The alphabet is RFC 4648
// section 4, and the first character is the most significant digit.
// Decoding to an integer and seeding lives on the server.
export const rngSeedAlphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

export function rngSeedError(seed: string): string | null {
  if (seed.length !== 6) {
    return `RNG seed must be 6 characters, not ${seed.length}.`
  }
  for (const ch of seed) {
    if (!rngSeedAlphabet.includes(ch)) {
      return 'RNG seed must use only the base64 alphabet A-Za-z0-9+/.'
    }
  }
  return null
}
