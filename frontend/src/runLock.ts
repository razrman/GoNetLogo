import type { WorldView } from './types'

// Shown when Apply would rebuild the grid while go is running.
// The RNG seed is not part of this: random-seed is allowed during a run.
export const worldSizeLockedMessage = 'Stop the model before changing the world.'

// worldGeometryChanged reports a new size, patch size, or wrapping.
// A seed-only edit is false, so Apply can reseed without stopping go.
export function worldGeometryChanged(next: WorldView, live: WorldView | null | undefined): boolean {
  if (!live) return true
  if (
    next.minPxcor !== live.minPxcor ||
    next.maxPxcor !== live.maxPxcor ||
    next.minPycor !== live.minPycor ||
    next.maxPycor !== live.maxPycor ||
    next.patchSize !== live.patchSize ||
    next.wrapX !== live.wrapX ||
    next.wrapY !== live.wrapY
  ) {
    return true
  }
  if (next.threeD || live.threeD) {
    if ((next.minPzcor ?? 0) !== (live.minPzcor ?? 0)) return true
    if ((next.maxPzcor ?? 0) !== (live.maxPzcor ?? 0)) return true
    if ((next.wrapZ ?? false) !== (live.wrapZ ?? false)) return true
  }
  return false
}
