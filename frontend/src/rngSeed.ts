// Unsigned decimal seed for the World tab. The value is an integer in
// 0..4294967295, the 32 bits the world generator actually uses.
// Decoding to an integer and seeding lives on the server.

export const rngSeedMax = 4294967295

export function rngSeedError(seed: string): string | null {
  if (!/^[0-9]+$/.test(seed)) {
    return 'RNG seed must be an integer from 0 to 4294967295.'
  }
  const n = Number(seed)
  if (!Number.isSafeInteger(n) || n > rngSeedMax) {
    return 'RNG seed must be an integer from 0 to 4294967295.'
  }
  return null
}
