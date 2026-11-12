import assert from 'node:assert/strict'
import { test } from 'node:test'
import { assignBitmapSize, bitmapPixels, blitSource, syncBitmap } from './canvasSurface.ts'

test('bitmap pixels follow the device pixel ratio and stay at least one pixel', () => {
  assert.deepEqual(bitmapPixels(276, 150, 2), { width: 552, height: 300 })
  assert.deepEqual(bitmapPixels(276, 150, 1), { width: 276, height: 150 })
  assert.deepEqual(bitmapPixels(0, 0, 1), { width: 1, height: 1 })
  assert.deepEqual(bitmapPixels(10, 10, Number.NaN), { width: 10, height: 10 })
})

// The fitness plot used to assign canvas.width on every frame. That resets
// the bitmap even when the value is unchanged, and the world view's cans
// and grid then flashed inside the plot. A repaint at the same size must
// not write the size back.
test('a plot repaint does not reallocate the bitmap when the size is unchanged', () => {
  let writes = 0
  const canvas = {
    _w: 552,
    _h: 300,
    get width() {
      return this._w
    },
    set width(v: number) {
      writes++
      this._w = v
    },
    get height() {
      return this._h
    },
    set height(v: number) {
      writes++
      this._h = v
    },
  }
  const same = bitmapPixels(276, 150, 2)
  assert.equal(assignBitmapSize(canvas, same.width, same.height), false)
  assert.equal(writes, 0)
  assert.equal(canvas.width, 552)
  assert.equal(canvas.height, 300)

  const grown = syncBitmap(canvas, 300, 150, 2)
  // Width changed, so both dimensions are written once. Height was already 300.
  assert.equal(writes, 2)
  assert.equal(canvas.width, 600)
  assert.equal(canvas.height, 300)
  assert.deepEqual(grown, { width: 300, height: 150 })
  syncBitmap(canvas, 300, 150, 2)
  assert.equal(writes, 2)
})

test('the view does not blit a canvas onto itself', () => {
  const view = { name: 'world' }
  const patches = { name: 'patches' }
  const drawing = { name: 'drawing' }
  assert.equal(blitSource(view, view), null)
  assert.equal(blitSource(view, patches), patches)
  assert.equal(blitSource(view, drawing), drawing)
  assert.notEqual(patches, drawing)
})
