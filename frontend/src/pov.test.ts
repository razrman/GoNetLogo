import assert from 'node:assert/strict'
import { test } from 'node:test'
import { applyPov, turtleAxes, type PovTurtle, type Vec3 } from './pov.ts'

const cam: Vec3 = { x: 0, y: -10, z: 8 }
const origin: Vec3 = { x: 0, y: 0, z: 0 }

function turtle(partial: Partial<PovTurtle> = {}): PovTurtle {
  return { x: 1, y: 2, z: 3, heading: 0, pitch: 0, roll: 0, ...partial }
}

function near(got: number, want: number) {
  assert.ok(Math.abs(got - want) < 1e-9, `${got} != ${want}`)
}

test('heading 0 looks north, heading 90 looks east, pitch 90 looks up', () => {
  const north = turtleAxes(0, 0, 0)
  near(north.forward.x, 0)
  near(north.forward.y, 1)
  near(north.forward.z, 0)
  near(north.up.z, 1)
  const east = turtleAxes(90, 0, 0)
  near(east.forward.x, 1)
  near(east.forward.y, 0)
  const up = turtleAxes(0, 90, 0)
  near(up.forward.z, 1)
  near(up.forward.y, 0)
})

test('watch keeps the camera and faces the turtle', () => {
  const pose = applyPov('watch', turtle(), cam, origin, { x: 1, y: 1, z: 1 })
  assert.deepEqual(pose.position, cam)
  assert.deepEqual(pose.target, { x: 1, y: 2, z: 3 })
  assert.equal(pose.lockOrbit, false)
  assert.equal(pose.followOffset, null)
})

test('follow orbits around the turtle and stays with it when it moves', () => {
  const first = applyPov('follow', turtle(), cam, origin, null)
  assert.deepEqual(first.target, { x: 1, y: 2, z: 3 })
  assert.deepEqual(first.position, { x: 1, y: -8, z: 11 })
  assert.deepEqual(first.followOffset, { x: 0, y: -10, z: 8 })
  assert.equal(first.lockOrbit, false)
  const moved = applyPov('follow', turtle({ x: 4 }), first.position, first.target, first.followOffset)
  assert.deepEqual(moved.target, { x: 4, y: 2, z: 3 })
  assert.deepEqual(moved.position, { x: 4, y: -8, z: 11 })
})

test('ride puts the camera on the turtle looking along its heading', () => {
  const east = applyPov('ride', turtle({ heading: 90, pitch: 0, roll: 0 }), cam, origin, null)
  assert.deepEqual(east.position, { x: 1, y: 2, z: 3 })
  near(east.target.x, 2)
  near(east.target.y, 2)
  near(east.target.z, 3)
  near(east.up.z, 1)
  assert.equal(east.lockOrbit, true)
  const sky = applyPov('ride', turtle({ heading: 0, pitch: 90 }), cam, origin, null)
  near(sky.target.x, 1)
  near(sky.target.y, 2)
  near(sky.target.z, 4)
})

test('reset-perspective leaves the camera for the observer to restore', () => {
  const pose = applyPov('', turtle(), cam, origin, { x: 0, y: -10, z: 8 })
  assert.deepEqual(pose.position, cam)
  assert.deepEqual(pose.target, origin)
  assert.equal(pose.followOffset, null)
  assert.equal(pose.lockOrbit, false)
  const gone = applyPov('follow', null, cam, origin, { x: 1, y: 1, z: 1 })
  assert.deepEqual(gone.position, cam)
  assert.equal(gone.followOffset, null)
})
