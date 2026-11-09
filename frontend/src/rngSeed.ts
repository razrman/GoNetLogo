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

export const seedOverrideStorageKey = 'gonetlogo.world.seedOverride'

export type KeyValueStore = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

function browserStore(): KeyValueStore {
  return localStorage
}

// readSeedOverride is the checkbox preference for the next session.
// The seed number itself comes from the server.
export function readSeedOverride(store: KeyValueStore = browserStore()): boolean {
  try {
    return store.getItem(seedOverrideStorageKey) === '1'
  } catch {
    return false
  }
}

export function writeSeedOverride(on: boolean, store: KeyValueStore = browserStore()): void {
  try {
    store.setItem(seedOverrideStorageKey, on ? '1' : '0')
  } catch {
    // Private mode and disabled storage still toggle the current session.
  }
}

// seedControlState greys out the seed box and Random while override is off.
// The checkbox stays usable so it can be turned back on. The whole form
// disables all three when there is no session.
export function seedControlState(seedOverride: boolean, formDisabled: boolean) {
  return {
    seedDisabled: formDisabled || !seedOverride,
    randomDisabled: formDisabled || !seedOverride,
    overrideDisabled: formDisabled,
  }
}

// rngSeedForApply is the seed sent with Apply. Override on always sends
// it, even when the text matches the server, so a second Apply cannot
// skip the reseed. Override off omits it and leaves the model in charge.
export function rngSeedForApply(seedOverride: boolean, seed: string): string | undefined {
  if (!seedOverride) return undefined
  return seed
}
