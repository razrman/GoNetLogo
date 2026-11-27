import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  geometryUsable,
  interfaceBreakpoint,
  layoutPaneWidth,
  layoutScale,
  parseLayoutChoice,
  placeInterface,
  readableFontPx,
  resolveLayout,
  type Box,
} from './interfaceLayout.ts'

const view: Box = { x: 280, y: 10, w: 312, h: 312 }
const setup: Box = { x: 16, y: 20, w: 94, h: 33 }
const slider: Box = { x: 16, y: 70, w: 234, h: 33 }

test('automatic layout follows the pane, and the toggle overrides it', () => {
  assert.equal(interfaceBreakpoint, 860)
  assert.equal(parseLayoutChoice(null), 'auto')
  assert.equal(parseLayoutChoice('nope'), 'auto')
  assert.equal(resolveLayout('auto', 1100, true, false), 'netlogo')
  assert.equal(resolveLayout('auto', 859, true, false), 'column')
  assert.equal(resolveLayout('netlogo', 400, true, false), 'netlogo')
  assert.equal(resolveLayout('column', 1400, true, false), 'column')
  assert.equal(resolveLayout('netlogo', 1400, false, false), 'column')
  assert.equal(resolveLayout('auto', 1100, false, true), 'netlogo')
  assert.equal(layoutPaneWidth(1200, 700), 700)
  assert.equal(layoutPaneWidth(640, 1400), 640)
})

test('a model with no view box, or a huge one, is not placed on the canvas', () => {
  assert.equal(geometryUsable(null, [setup]), false)
  assert.equal(geometryUsable({ x: 0, y: 0, w: 0, h: 0 }, [setup]), false)
  assert.equal(geometryUsable({ ...view, w: 5000 }, [setup]), false)
  assert.equal(geometryUsable(view, [{ x: 50000, y: 0, w: 10, h: 10 }]), false)
  assert.equal(geometryUsable(view, [{ x: Number.NaN, y: 0, w: 10, h: 10 }]), false)
  // Overlap, and a widget just outside the view, still use the canvas.
  assert.equal(geometryUsable(view, [setup, setup, { x: -20, y: 400, w: 80, h: 30 }]), true)
})

test('the canvas keeps model positions and scales with the pane', () => {
  const placed = placeInterface(view, [slider, setup])
  assert.ok(placed.view)
  assert.equal(placed.items[1]!.x - placed.items[0]!.x, setup.x - slider.x)
  assert.equal(placed.items[1]!.y - placed.items[0]!.y, setup.y - slider.y)
  assert.equal(placed.view!.w, view.w)
  assert.ok(placed.view!.x > placed.items[0]!.x)
  const wide = layoutScale(placed.width, placed.height, placed.width * 2, placed.height * 3)
  assert.equal(wide, 2)
  const narrow = layoutScale(placed.width, placed.height, placed.width / 2, placed.height)
  assert.equal(narrow, 0.5)
  assert.equal(layoutScale(placed.width, placed.height, 0, 400), 0)
  assert.equal(readableFontPx(1), 13)
  assert.equal(readableFontPx(2), 18)
  assert.equal(readableFontPx(0.4), 12)
})
