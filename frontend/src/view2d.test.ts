import assert from 'node:assert/strict'
import { test } from 'node:test'
import { beginPatchBlit, canvasTransform, patchImageRect, view2dPose, worldToScreen, type View2DWorld } from './view2d.ts'

function world(partial: Partial<View2DWorld> = {}): View2DWorld {
  return {
    minPxcor: -2,
    maxPxcor: 2,
    minPycor: -2,
    maxPycor: 2,
    turtles: [{ who: 0, x: 1, y: -1, heading: 0 }],
    ...partial,
  }
}

function near(got: number, want: number) {
  assert.ok(Math.abs(got - want) < 1e-9, `${got} != ${want}`)
}

test('a free view centers the world and does not rotate it', () => {
  const pose = view2dPose(world())
  near(pose.x, 0)
  near(pose.y, 0)
  near(pose.rotation, 0)
  assert.equal(pose.spotlight, false)
  const center = worldToScreen(0, 0, pose, 10, 100, 80)
  near(center.x, 50)
  near(center.y, 40)
})

test('follow and ride center on the turtle and point its heading up', () => {
  const north = view2dPose(world({ perspective: 'follow', subject: 0 }))
  near(north.x, 1)
  near(north.y, -1)
  near(north.rotation, 0)
  assert.equal(north.spotlight, false)
  // One patch north of the turtle is above the canvas center.
  const ahead = worldToScreen(1, 0, north, 10, 200, 200)
  near(ahead.x, 100)
  near(ahead.y, 90)

  const east = view2dPose(world({ perspective: 'ride', subject: 0, turtles: [{ who: 0, x: 1, y: -1, heading: 90 }] }))
  near(east.rotation, Math.PI / 2)
  const faced = worldToScreen(2, -1, east, 10, 200, 200)
  near(faced.x, 100)
  near(faced.y, 90)
})

test('watch centers without rotating, including a patch', () => {
  const turtle = view2dPose(world({ perspective: 'watch', subject: 0 }))
  near(turtle.x, 1)
  near(turtle.y, -1)
  near(turtle.rotation, 0)
  assert.equal(turtle.spotlight, true)

  const patch = view2dPose(world({ perspective: 'watch', subject: null, subjectX: -2, subjectY: 2, subjectZ: 1 }))
  near(patch.x, -2)
  near(patch.y, 2)
  near(patch.rotation, 0)
  assert.equal(patch.spotlight, true)
  // The matrix agrees with worldToScreen.
  const m = canvasTransform(patch, 8, 40, 40)
  const p = worldToScreen(-2, 2, patch, 8, 40, 40)
  near(m[0] * -2 + m[2] * 2 + m[4], p.x)
  near(m[1] * -2 + m[3] * 2 + m[5], p.y)
})

test('beginPatchBlit maps a y-down bitmap with a positive dest height', () => {
  const calls: unknown[][] = []
  const ctx = {
    translate: (x: number, y: number) => calls.push(['translate', x, y]),
    scale: (x: number, y: number) => calls.push(['scale', x, y]),
  }
  const w = { minPxcor: -125, maxPxcor: 125, minPycor: -125, maxPycor: 125 }
  const size = beginPatchBlit(ctx, w)
  const rect = patchImageRect(w)
  near(rect.x, -125.5)
  near(rect.y, 125.5)
  near(rect.w, 251)
  near(rect.h, -251)
  // Positive height: a negative drawImage dest height flips the grid
  // under the y-up world transform.
  near(size.w, 251)
  near(size.h, 251)
  assert.deepEqual(calls, [
    ['translate', -125.5, 125.5],
    ['scale', 1, -1],
  ])
})
