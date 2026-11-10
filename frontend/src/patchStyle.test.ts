import assert from 'node:assert/strict'
import { test } from 'node:test'
import { patchCube, patchOpacity, solidPatchOpacity, voxelStride } from './patchStyle.ts'

test('a normal patch color is see-through and an explicit alpha is kept', () => {
  assert.equal(patchOpacity(255), solidPatchOpacity)
  assert.ok(solidPatchOpacity > 0.2 && solidPatchOpacity < 0.7)
  assert.equal(patchOpacity(128), 128 / 255)
  assert.equal(patchOpacity(0), 0)
  assert.equal(patchOpacity(-1), 0)
  assert.equal(patchOpacity(Number.NaN), 0)
})

test('patch cubes leave a gap and voxels carry an alpha byte', () => {
  assert.ok(patchCube > 0.5 && patchCube < 1)
  assert.equal(voxelStride, 10)
})
