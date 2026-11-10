import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { TurtleShape } from './types.ts'
import {
  ShapeLibrary,
  builtinKind,
  canonicalShapeName,
  geometryExtent,
  groupTurtles,
  maxRadiusNear,
  shapeScale,
  type ShapeTurtle,
} from './turtleShape.ts'

function shape(partial: Partial<TurtleShape> & Pick<TurtleShape, 'name'>): TurtleShape {
  return { rotatable: true, elements: [], ...partial }
}

function polygon(points: number[], marked = true): TurtleShape['elements'][number] {
  return { kind: 'polygon', filled: true, marked, r: 0, g: 0, b: 0, points, x: 0, y: 0, w: 0, h: 0 }
}

function circle(x: number, y: number, d: number, marked = true): TurtleShape['elements'][number] {
  return { kind: 'circle', filled: true, marked, r: 20, g: 30, b: 40, points: [], x, y, w: d, h: d }
}

const arrowHead = polygon([150, 5, 40, 250, 150, 205, 260, 250])

function library(): ShapeLibrary {
  const lib = new ShapeLibrary()
  lib.sync([
    shape({ name: 'default', elements: [arrowHead] }),
    shape({ name: 'airplane', elements: [polygon([150, 0, 135, 15, 120, 60, 180, 60])] }),
    shape({ name: 'arrow', elements: [arrowHead] }),
    shape({ name: 'circle', rotatable: false, elements: [circle(0, 0, 300)] }),
    shape({ name: 'square', rotatable: false, elements: [polygon([30, 30, 270, 30, 270, 270, 30, 270])] }),
    shape({ name: 'person', rotatable: false, elements: [circle(110, 5, 80), polygon([105, 90, 120, 195, 90, 285, 210, 285])] }),
    shape({ name: 'sheep', rotatable: false, elements: [polygon([15, 75, 210, 75, 210, 225, 15, 225])] }),
    shape({ name: 'butterfly', elements: [polygon([150, 165, 209, 199, 225, 225, 150, 240])] }),
    shape({ name: 'plant', rotatable: false, elements: [polygon([135, 90, 165, 90, 165, 300, 135, 300])] }),
    shape({
      name: 'badge',
      elements: [polygon([150, 50, 50, 250, 250, 250]), { ...polygon([40, 40, 80, 40, 80, 80, 40, 80], false), r: 255, g: 0, b: 0 }],
    }),
    shape({ name: 'circle 2', rotatable: false, elements: [circle(0, 0, 300), circle(30, 30, 240, false)] }),
  ])
  return lib
}

test('builtin names map to the NetLogo 3D solids', () => {
  assert.equal(builtinKind('Default'), 'dart')
  assert.equal(builtinKind('arrow'), 'dart')
  assert.equal(builtinKind('circle'), 'sphere')
  assert.equal(builtinKind('dot'), 'dot')
  assert.equal(builtinKind('square'), 'cube')
  assert.equal(builtinKind('triangle'), 'cone')
  assert.equal(builtinKind('cylinder'), 'cylinder')
  assert.equal(builtinKind('line'), 'line')
  assert.equal(builtinKind('line half'), 'line-half')
  assert.equal(builtinKind('line-half'), 'line-half')
  assert.equal(builtinKind('car'), 'car')
  assert.equal(builtinKind('circle 2'), null)
  assert.equal(builtinKind('square 2'), null)
  assert.equal(builtinKind('person'), null)
  assert.equal(canonicalShapeName('  Face   Happy '), 'face happy')
})

test('the default shape set resolves to distinct geometry', () => {
  const lib = library()
  const dart = lib.resolve('default')
  const arrow = lib.resolve('ARROW')
  const sphere = lib.resolve('circle')
  const cube = lib.resolve('square')
  const cone = lib.resolve('triangle')
  const cyl = lib.resolve('cylinder')
  const line = lib.resolve('line')
  const half = lib.resolve('line half')
  const dot = lib.resolve('dot')
  const car = lib.resolve('car')
  const keys = new Set([dart.key, sphere.key, cube.key, cone.key, cyl.key, line.key, half.key, dot.key, car.key])
  assert.equal(keys.size, 9)
  assert.equal(arrow.key, dart.key)
  assert.equal(arrow.marked, dart.marked)

  const dartBox = geometryExtent(dart.marked)
  const coneBox = geometryExtent(cone.marked)
  // The turtle is a flattened pyramid: thinner in z than it is wide.
  // The triangle cone is round, so its ends stay wide.
  assert.ok(dartBox.max[2] - dartBox.min[2] < dartBox.max[0] - dartBox.min[0])
  assert.ok(Math.abs(coneBox.max[0] - coneBox.min[0] - (coneBox.max[2] - coneBox.min[2])) < 0.05)
  assert.ok(maxRadiusNear(cone.marked, 0.45) > 0.35)
  assert.ok(maxRadiusNear(cyl.marked, 0.45) > 0.4)
  assert.ok(maxRadiusNear(sphere.marked, 0.45) < 0.25)

  const sphereBox = geometryExtent(sphere.marked)
  const dotBox = geometryExtent(dot.marked)
  assert.ok(sphereBox.max[0] > 0.45 && sphereBox.max[0] < 0.55)
  assert.ok(dotBox.max[0] > 0.15 && dotBox.max[0] < 0.25)

  const cubeBox = geometryExtent(cube.marked)
  assert.ok(cubeBox.max[0] > 0.49 && cubeBox.max[1] > 0.49 && cubeBox.max[2] > 0.49)

  const lineBox = geometryExtent(line.marked)
  const halfBox = geometryExtent(half.marked)
  assert.ok(lineBox.max[1] - lineBox.min[1] > 0.9)
  assert.ok(halfBox.max[1] - halfBox.min[1] < 0.6)
  assert.ok(halfBox.min[1] > -0.05 && halfBox.max[1] < 0.55)

  const carBox = geometryExtent(car.marked)
  assert.ok(carBox.count > 20)
  assert.ok(carBox.max[1] > 0.3 && carBox.max[2] > 0.2)
  lib.dispose()
})

test('vector shapes extrude when rotatable and face the camera otherwise', () => {
  const lib = library()
  const plane = lib.resolve('airplane')
  assert.equal(plane.builtin, null)
  assert.equal(plane.billboard, false)
  const planeBox = geometryExtent(plane.marked)
  assert.ok(planeBox.count > 3)
  const thick = planeBox.max[2] - planeBox.min[2]
  assert.ok(thick > 0.1 && thick < 0.25)

  const person = lib.resolve('person')
  const sheep = lib.resolve('sheep')
  const plant = lib.resolve('plant')
  assert.equal(person.billboard, true)
  assert.equal(sheep.billboard, true)
  assert.equal(plant.billboard, true)
  const personBox = geometryExtent(person.marked)
  // A billboard is flat along y (the normal) and stands up along z.
  assert.ok(Math.abs(personBox.max[1]) < 0.05 && Math.abs(personBox.min[1]) < 0.05)
  assert.ok(personBox.max[2] - personBox.min[2] > 0.4)

  const badge = lib.resolve('badge')
  assert.ok(badge.marked)
  assert.ok(badge.fixed)
  const fixed = geometryExtent(badge.fixed)
  assert.ok(fixed.count > 0)
  lib.dispose()
})

test('a custom shape is used, and circle 2 is not the sphere', () => {
  const lib = library()
  const badge = lib.resolve('badge')
  const box = geometryExtent(badge.marked)
  // Editor triangle (150,50)-(50,250)-(250,250) spans about two thirds of a patch.
  assert.ok(box.max[0] > 0.3 && box.min[0] < -0.3)
  assert.ok(box.max[1] > 0.3 && box.min[1] < -0.3)
  const ring = lib.resolve('circle 2')
  assert.equal(ring.builtin, null)
  assert.notEqual(ring.key, lib.resolve('circle').key)
  assert.equal(lib.resolve('no-such-shape').key, lib.resolve('default').key)
  assert.equal(lib.resolve('').key, lib.resolve('default').key)
  lib.dispose()
})

test('an empty library still draws the builtin solids and falls back to the dart', () => {
  const lib = new ShapeLibrary()
  lib.sync([])
  assert.equal(lib.resolve('circle').builtin, 'sphere')
  assert.equal(lib.resolve('square').builtin, 'cube')
  assert.equal(lib.resolve('wolf').builtin, 'dart')
  // The same list does not throw the cache away.
  assert.equal(lib.sync([]), false)
  assert.equal(lib.sync(null), false)
  const again = lib.resolve('circle')
  assert.equal(again.marked, lib.resolve('circle').marked)
  lib.dispose()
})

test('shape changes move a turtle between buckets and hidden turtles drop out', () => {
  const lib = library()
  const turtles: ShapeTurtle[] = [
    { shape: 'circle', hidden: false },
    { shape: 'square', hidden: false },
    { shape: 'person', hidden: true },
    { shape: 'default', hidden: false },
  ]
  const first = groupTurtles(turtles, lib)
  assert.equal(first.get(lib.resolve('circle').key)?.length, 1)
  assert.equal(first.get(lib.resolve('square').key)?.length, 1)
  assert.equal(first.get(lib.resolve('person').key), undefined)
  assert.equal(first.get(lib.resolve('default').key)?.length, 1)

  turtles[0].shape = 'square'
  turtles[2].hidden = false
  turtles[2].shape = 'sheep'
  const second = groupTurtles(turtles, lib)
  assert.equal(second.get(lib.resolve('circle').key), undefined)
  assert.equal(second.get(lib.resolve('square').key)?.length, 2)
  assert.equal(second.get(lib.resolve('sheep').key)?.length, 1)
  assert.equal(second.get(lib.resolve('default').key)?.length, 1)
  lib.dispose()
})

test('size zero draws at size 1 and a negative size keeps its sign', () => {
  assert.equal(shapeScale(0), 1)
  assert.equal(shapeScale(Number.NaN), 1)
  assert.equal(shapeScale(2.5), 2.5)
  assert.equal(shapeScale(-1.5), -1.5)
})
