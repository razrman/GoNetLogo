import assert from 'node:assert/strict'
import { test } from 'node:test'
import { penRadius, visiblePenTrails, type TrailMark } from './penTrail.ts'
import type { WrapBounds } from './wrapSegment.ts'

function bounds(wrap = false): WrapBounds {
  const axis = { min: -16.5, max: 16.5, wrap }
  return { x: { ...axis }, y: { ...axis }, z: { ...axis } }
}

function stroke(partial: Partial<TrailMark> & Pick<TrailMark, 'x' | 'y' | 'z' | 'x2' | 'y2' | 'z2'>): TrailMark {
  return {
    pen: true,
    erase: false,
    size: 1,
    r: 200,
    g: 0,
    b: 0,
    ...partial,
  }
}

test('pen radius matches a half-patch stroke at pen-size 1', () => {
  assert.ok(Math.abs(penRadius(1) - 0.275) < 1e-9)
  assert.equal(penRadius(0), penRadius(1))
})

test('a 3D pen stroke is kept, including its z, and a stamp is not a trail', () => {
  const trails = visiblePenTrails(
    [
      stroke({ x: 0, y: 0, z: 0, x2: 0, y2: 0, z2: 2, size: 3, r: 10, g: 20, b: 30 }),
      { pen: false, erase: false, x: 1, y: 1, z: 1, x2: 1, y2: 1, z2: 1, size: 1, r: 0, g: 0, b: 0 },
    ],
    bounds(),
  )
  assert.equal(trails.length, 1)
  assert.equal(trails[0].az, 0)
  assert.equal(trails[0].bz, 2)
  assert.equal(trails[0].size, 3)
  assert.equal(trails[0].r, 10)
})

test('pen-erase along a stroke removes it, and ink drawn afterward stays', () => {
  const line = stroke({ x: 0, y: 0, z: 0, x2: 2, y2: 0, z2: 0 })
  const erase = stroke({ x: 0, y: 0, z: 0, x2: 2, y2: 0, z2: 0, erase: true })
  const after = stroke({ x: 0, y: 1, z: 0, x2: 0, y2: 1, z2: 2, g: 80 })
  const trails = visiblePenTrails([line, erase, after], bounds())
  assert.equal(trails.length, 1)
  assert.equal(trails[0].az, 0)
  assert.equal(trails[0].bz, 2)
  assert.equal(trails[0].g, 80)
})

test('pen-erase of the middle of a trail leaves both ends', () => {
  const line = stroke({ x: 0, y: 0, z: 1, x2: 4, y2: 0, z2: 1 })
  const erase = stroke({ x: 1.5, y: 0, z: 1, x2: 2.5, y2: 0, z2: 1, erase: true })
  const trails = visiblePenTrails([line, erase], bounds())
  assert.ok(trails.length >= 2, `pieces ${trails.length}`)
  const left = trails.some((p) => Math.max(p.ax, p.bx) < 1.6)
  const right = trails.some((p) => Math.min(p.ax, p.bx) > 2.4)
  assert.ok(left, `no left end in ${JSON.stringify(trails)}`)
  assert.ok(right, `no right end in ${JSON.stringify(trails)}`)
  for (const p of trails) {
    const lo = Math.min(p.ax, p.bx)
    const hi = Math.max(p.ax, p.bx)
    assert.ok(hi < 1.7 || lo > 2.3, `piece still covers the erased middle: ${lo}..${hi}`)
    assert.equal(p.az, 1)
    assert.equal(p.bz, 1)
  }
})

test('a wrapped pen stroke is split at the wall instead of cutting across', () => {
  const trails = visiblePenTrails(
    [stroke({ x: 15, y: 0, z: 0, x2: -15, y2: 0, z2: 1 })],
    bounds(true),
  )
  assert.equal(trails.length, 2)
  assert.ok(trails[0].ax > 14)
  assert.ok(trails[1].bx < -14)
})

test('clear-drawing is an empty layer', () => {
  assert.deepEqual(visiblePenTrails([], bounds()), [])
})
