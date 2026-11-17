import assert from 'node:assert/strict'
import { test } from 'node:test'
import { viewUpdateChoices, viewUpdatesFromModel } from './viewUpdates.ts'

test('the view updates dropdown offers on ticks and continuous', () => {
  assert.deepEqual(
    viewUpdateChoices.map((choice) => choice.label),
    ['on ticks', 'continuous'],
  )
  assert.deepEqual(
    viewUpdateChoices.map((choice) => choice.value),
    ['ticks', 'continuous'],
  )
})

test('a loaded model selects its own update mode', () => {
  assert.equal(viewUpdatesFromModel('ticks'), 'ticks')
  assert.equal(viewUpdatesFromModel('on ticks'), 'ticks')
  assert.equal(viewUpdatesFromModel(1), 'ticks')
  assert.equal(viewUpdatesFromModel('1'), 'ticks')
  assert.equal(viewUpdatesFromModel('continuous'), 'continuous')
  assert.equal(viewUpdatesFromModel(0), 'continuous')
  assert.equal(viewUpdatesFromModel('0'), 'continuous')
  assert.equal(viewUpdatesFromModel(undefined), 'continuous')
  assert.equal(viewUpdatesFromModel(null), 'continuous')
})
