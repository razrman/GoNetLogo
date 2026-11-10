import assert from 'node:assert/strict'
import { test } from 'node:test'
import { labelAnchors, type LabelSource, type LabelTurtle } from './labels3d.ts'

function turtle(partial: Partial<LabelTurtle> & Pick<LabelTurtle, 'who'>): LabelTurtle {
  return { x: 0, y: 0, z: 0, size: 1, hidden: false, ...partial }
}

test('turtle, patch, and link labels keep their 3D anchors', () => {
  const labels: LabelSource[] = [
    { text: 'me', who: 1, z: 9, r: 1, g: 2, b: 3 },
    { text: 'floor', px: 2, py: -3, z: 4, r: 0, g: 0, b: 200 },
    { text: 'tie', lx: 0.5, ly: 1.5, z: -2, r: 10, g: 20, b: 30 },
    { text: '', who: 1, r: 0, g: 0, b: 0 },
  ]
  const anchors = labelAnchors(labels, [turtle({ who: 1, x: 4, y: 6, z: 3, size: 2 })])
  assert.equal(anchors.length, 3)
  assert.deepEqual(anchors[0], { text: 'me', x: 5, y: 5, z: 3, r: 1, g: 2, b: 3 })
  assert.deepEqual(anchors[1], { text: 'floor', x: 2.5, y: -3.5, z: 4, r: 0, g: 0, b: 200 })
  assert.deepEqual(anchors[2], { text: 'tie', x: 0.5, y: 1.5, z: -2, r: 10, g: 20, b: 30 })
})

test('a hidden turtle and a missing turtle drop their labels', () => {
  const labels: LabelSource[] = [
    { text: 'gone', who: 4, z: 1, r: 0, g: 0, b: 0 },
    { text: 'hid', who: 5, z: 1, r: 0, g: 0, b: 0 },
  ]
  const anchors = labelAnchors(labels, [turtle({ who: 5, hidden: true })])
  assert.equal(anchors.length, 0)
})
