import assert from 'node:assert/strict'
import { test } from 'node:test'
import { worldGeometryChanged, worldSizeLockedMessage } from './runLock.ts'
import type { WorldView } from './types.ts'

const live: WorldView = {
  minPxcor: -2,
  maxPxcor: 2,
  minPycor: -2,
  maxPycor: 2,
  patchSize: 12,
  wrapX: true,
  wrapY: false,
  rngSeed: 'AAAAAA',
}

test('a seed-only edit is not a world-size change', () => {
  const next = { ...live, rngSeed: 'AAAAAB' }
  assert.equal(worldGeometryChanged(next, live), false)
  assert.equal(worldSizeLockedMessage.includes('model is running'), false)
})

test('size, patch size, and wrapping count as a world change', () => {
  assert.equal(worldGeometryChanged({ ...live, maxPxcor: 3 }, live), true)
  assert.equal(worldGeometryChanged({ ...live, patchSize: 13 }, live), true)
  assert.equal(worldGeometryChanged({ ...live, wrapY: true }, live), true)
})

test('3D depth and z wrapping count, and a flat world ignores them', () => {
  const box: WorldView = { ...live, threeD: true, minPzcor: -1, maxPzcor: 1, wrapZ: true }
  assert.equal(worldGeometryChanged({ ...box, maxPzcor: 2 }, box), true)
  assert.equal(worldGeometryChanged({ ...box, wrapZ: false }, box), true)
  assert.equal(worldGeometryChanged({ ...box, rngSeed: 'BBBBBB' }, box), false)
  assert.equal(worldGeometryChanged({ ...live, minPzcor: -4, maxPzcor: 4, wrapZ: true }, live), false)
})
