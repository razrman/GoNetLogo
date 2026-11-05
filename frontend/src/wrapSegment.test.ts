import assert from 'node:assert/strict'
import { test } from 'node:test'
import { wrapSegments, wrapSegments2D, type Vec2, type Vec3, type WrapBounds, type WrapBounds2D } from './wrapSegment.ts'

function bounds(
  partial: { x?: boolean; y?: boolean; z?: boolean; min?: number; max?: number } = {},
): WrapBounds {
  const min = (partial.min ?? -16) - 0.5
  const max = (partial.max ?? 16) + 0.5
  const axis = (wrap: boolean | undefined) => ({ min, max, wrap: wrap ?? true })
  return { x: axis(partial.x), y: axis(partial.y), z: axis(partial.z) }
}

function near(p: Vec3, x: number, y: number, z: number) {
  assert.ok(Math.abs(p.x - x) < 1e-8, `x ${p.x} != ${x}`)
  assert.ok(Math.abs(p.y - y) < 1e-8, `y ${p.y} != ${y}`)
  assert.ok(Math.abs(p.z - z) < 1e-8, `z ${p.z} != ${z}`)
}

test('single-axis wrap splits at the wall and continues from the opposite wall', () => {
  // minPxcor -16, maxPxcor 16: walls at ±16.5, width 33.
  // 15 → -15 is 3 steps east through the wall, not 30 steps west.
  const segs = wrapSegments({ x: 15, y: 0, z: 0 }, { x: -15, y: 1, z: 2 }, bounds())
  assert.equal(segs.length, 2)
  near(segs[0].a, 15, 0, 0)
  near(segs[0].b, 16.5, 0.5, 1)
  near(segs[1].a, -16.5, 0.5, 1)
  near(segs[1].b, -15, 1, 2)
})

test('multi-axis wrap emits a piece per wall the shortest path crosses', () => {
  // Width 5, walls at ±2.5. Shortest deltas are +1 x, +2 y, +1 z,
  // crossing x and z together, then y.
  const small = bounds({ min: -2, max: 2 })
  const segs = wrapSegments({ x: 2, y: 1, z: 2 }, { x: -2, y: -2, z: -2 }, small)
  assert.equal(segs.length, 3)
  near(segs[0].a, 2, 1, 2)
  near(segs[0].b, 2.5, 2, 2.5)
  near(segs[1].a, -2.5, 2, -2.5)
  near(segs[1].b, -2.25, 2.5, -2.25)
  near(segs[2].a, -2.25, -2.5, -2.25)
  near(segs[2].b, -2, -2, -2)
})

test('no wrap keeps a single segment on the original endpoints', () => {
  const a = { x: 1, y: -2, z: 3 }
  const b = { x: 4, y: 5, z: -1 }
  const segs = wrapSegments(a, b, bounds())
  assert.equal(segs.length, 1)
  near(segs[0].a, a.x, a.y, a.z)
  near(segs[0].b, b.x, b.y, b.z)
})

test('a path that ends on the opposite wall stops at the near wall', () => {
  // -16.5 is the min wall. The short way from 15 is +1.5, which lands
  // exactly on the max wall, so the continuation has no length.
  const segs = wrapSegments({ x: 15, y: 0, z: 0 }, { x: -16.5, y: 0, z: 0 }, bounds())
  assert.equal(segs.length, 1)
  near(segs[0].a, 15, 0, 0)
  near(segs[0].b, 16.5, 0, 0)
})

test('a non-wrapping axis is not split even when the long way is shorter', () => {
  // x wraps (short hop east through the wall). y does not, so the
  // link keeps the long north-south difference.
  const segs = wrapSegments(
    { x: 2, y: 2, z: 0 },
    { x: -2, y: -2, z: 0 },
    bounds({ x: true, y: false, z: false, min: -2, max: 2 }),
  )
  assert.equal(segs.length, 2)
  near(segs[0].a, 2, 2, 0)
  near(segs[0].b, 2.5, 0, 0)
  near(segs[1].a, -2.5, 0, 0)
  near(segs[1].b, -2, -2, 0)
})

function bounds2D(
  partial: { x?: boolean; y?: boolean; min?: number; max?: number } = {},
): WrapBounds2D {
  const min = (partial.min ?? 0) - 0.5
  const max = (partial.max ?? 25) + 0.5
  const axis = (wrap: boolean | undefined) => ({ min, max, wrap: wrap ?? true })
  return { x: axis(partial.x), y: axis(partial.y) }
}

function near2(p: Vec2, x: number, y: number) {
  assert.ok(Math.abs(p.x - x) < 1e-8, `x ${p.x} != ${x}`)
  assert.ok(Math.abs(p.y - y) < 1e-8, `y ${p.y} != ${y}`)
}

test('2D x wrap splits at the wall and the last point is end 2', () => {
  // 0..25, walls at -0.5 and 25.5. 0 → 25 is one step west, not 25 east.
  const segs = wrapSegments2D({ x: 0, y: 4 }, { x: 25, y: 4 }, bounds2D())
  assert.equal(segs.length, 2)
  near2(segs[0].a, 0, 4)
  near2(segs[0].b, -0.5, 4)
  near2(segs[1].a, 25.5, 4)
  near2(segs[1].b, 25, 4)
})

test('2D wrap on both axes emits a piece per wall', () => {
  const segs = wrapSegments2D({ x: 2, y: 1 }, { x: -2, y: -2 }, bounds2D({ min: -2, max: 2 }))
  assert.equal(segs.length, 3)
  near2(segs[0].a, 2, 1)
  near2(segs[0].b, 2.5, 2)
  near2(segs[1].a, -2.5, 2)
  near2(segs[1].b, -2.25, 2.5)
  near2(segs[2].a, -2.25, -2.5)
  near2(segs[2].b, -2, -2)
})

test('2D link that does not cross a wall is one segment', () => {
  const a = { x: 3, y: 4 }
  const b = { x: 8, y: 9 }
  const segs = wrapSegments2D(a, b, bounds2D())
  assert.equal(segs.length, 1)
  near2(segs[0].a, a.x, a.y)
  near2(segs[0].b, b.x, b.y)
})

test('2D non-wrapping axis stays a straight difference', () => {
  const segs = wrapSegments2D(
    { x: 0, y: 1 },
    { x: 25, y: 20 },
    bounds2D({ x: true, y: false }),
  )
  assert.equal(segs.length, 2)
  near2(segs[0].a, 0, 1)
  near2(segs[0].b, -0.5, 10.5)
  near2(segs[1].a, 25.5, 10.5)
  near2(segs[1].b, 25, 20)
})
