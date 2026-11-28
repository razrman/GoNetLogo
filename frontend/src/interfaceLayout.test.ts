import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  arrangeWidgetDocument,
  autoArrange,
  geometryUsable,
  interfaceBreakpoint,
  layoutPaneWidth,
  layoutScale,
  parseGridSpacing,
  parseLayoutChoice,
  placeInterface,
  noteFontPx,
  readableFontPx,
  resolveLayout,
  snapBox,
  snapWidgetYAML,
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
  assert.equal(placed.width, 800)
  assert.equal(placed.height, 600)
  assert.equal(placed.originX, 0)
  assert.equal(placed.view!.x, view.x)
  assert.equal(placed.items[1]!.x - placed.items[0]!.x, setup.x - slider.x)
  assert.equal(placed.items[1]!.y - placed.items[0]!.y, setup.y - slider.y)
  assert.equal(placed.view!.w, view.w)
  assert.ok(placed.view!.x > placed.items[0]!.x)
  const grown = placeInterface(view, [{ x: 760, y: 10, w: 80, h: 40 }])
  assert.equal(grown.width, 840)
  assert.equal(grown.height, 600)
  const negative = placeInterface({ x: 0, y: 0, w: 100, h: 100 }, [{ x: -30, y: 10, w: 20, h: 20 }])
  assert.equal(negative.originX, -30)
  assert.equal(negative.items[0]!.x, 0)
  const wide = layoutScale(placed.width, placed.height, placed.width * 2, placed.height * 3)
  assert.equal(wide, 2)
  const narrow = layoutScale(placed.width, placed.height, placed.width / 2, placed.height)
  assert.equal(narrow, 0.5)
  assert.equal(layoutScale(placed.width, placed.height, 0, 400), 0)
  assert.equal(readableFontPx(1), 13)
  assert.equal(readableFontPx(2), 18)
  assert.equal(readableFontPx(0.4), 12)
  assert.equal(noteFontPx(11, 1), 11)
  assert.equal(noteFontPx(11, 1.5), 16.5)
  assert.equal(noteFontPx(0, 2), 24)
})

test('snapping rounds positions and refuses to shrink a widget away', () => {
  assert.equal(parseGridSpacing(null), 10)
  assert.equal(parseGridSpacing('off'), 10)
  assert.equal(parseGridSpacing('0'), 0)
  assert.deepEqual(snapBox({ x: 16, y: 23, w: 33, h: 14 }, 10), { x: 20, y: 20, w: 30, h: 20 })
  assert.deepEqual(snapBox({ x: 16, y: 23, w: 33, h: 14 }, 0), { x: 16, y: 23, w: 33, h: 14 })
  assert.equal(snapBox({ x: 4, y: 0, w: 8, h: 8 }, 40).w, 40)
  const yaml = '- kind: button\n  x: 16\n  y: 23\n  width: 33\n  height: 14\n  minPxcor: 0\n'
  const snapped = snapWidgetYAML(yaml, 10)
  assert.match(snapped, /x: 20\n/)
  assert.match(snapped, /y: 20\n/)
  assert.match(snapped, /width: 30\n/)
  assert.match(snapped, /height: 20\n/)
  assert.match(snapped, /minPxcor: 0\n/)
  assert.equal(snapWidgetYAML(yaml, 0), yaml)
})

test('auto arrange packs controls on the grid without overlaps', () => {
  const widgets = [
    { kind: 'slider', box: { x: 400, y: 10, w: 180, h: 50 } },
    { kind: 'button', box: { x: 10, y: 10, w: 90, h: 34 } },
    { kind: 'button', box: { x: 10, y: 80, w: 90, h: 34 } },
    { kind: 'note', box: { x: 0, y: 0, w: 150, h: 40 } },
  ]
  const world = { x: 280, y: 10, w: 312, h: 312 }
  const arranged = autoArrange(world, widgets, 10)
  const boxes = [arranged.view, ...arranged.widgets]
  for (const box of boxes) {
    assert.equal(box.x % 10, 0)
    assert.equal(box.y % 10, 0)
    assert.equal(box.w % 10, 0)
    assert.equal(box.h % 10, 0)
    assert.ok(box.w >= 20 && box.h >= 20)
    assert.ok(box.x >= 0 && box.y >= 0)
    assert.ok(box.x + box.w <= 800)
    assert.ok(box.y + box.h <= 600)
  }
  const sliderBox = arranged.widgets[0]!
  const buttonBox = arranged.widgets[1]!
  assert.ok(buttonBox.y < sliderBox.y || buttonBox.x < sliderBox.x)
  assert.ok(arranged.view.x >= buttonBox.x + buttonBox.w)
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      const a = boxes[i]!
      const b = boxes[j]!
      const hit = a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
      assert.equal(hit, false)
    }
  }
  const doc = [
    '- kind: view',
    '  x: 280',
    '  y: 10',
    '  width: 312',
    '  height: 312',
    '- kind: slider',
    '  x: 400',
    '  y: 10',
    '  width: 180',
    '  height: 50',
    '- kind: button',
    '  x: 10',
    '  y: 10',
    '  width: 90',
    '  height: 34',
    '- kind: button',
    '  x: 10',
    '  y: 80',
    '  width: 90',
    '  height: 34',
    '- kind: note',
    '  x: 0',
    '  y: 0',
    '  width: 150',
    '  height: 40',
    '',
  ].join('\n')
  const written = arrangeWidgetDocument(doc, widgets, world, 10)
  assert.equal(written.error, null)
  assert.match(written.text, /kind: view[\s\S]*?width: \d+/)
  assert.doesNotMatch(written.text, /x: 400/)
})
