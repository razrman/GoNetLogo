import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  dialogSize,
  lastModelStorageKey,
  normalizeModelPath,
  parentFolder,
  readLastModelPath,
  writeLastModelPath,
  type KeyValueStore,
} from './libraryMemory.ts'

function memory(): KeyValueStore {
  const data = new Map<string, string>()
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
  }
}

test('parent folder of a nested model is the directory to reopen', () => {
  assert.equal(parentFolder('library/Sample Models/Art/Fire.nlogo'), 'library/Sample Models/Art')
  assert.equal(parentFolder('library/Fire.nlogo'), 'library')
  assert.equal(parentFolder('Fire.nlogo'), '')
  assert.equal(parentFolder('library\\Sample Models\\Art\\Fire.nlogo'), 'library/Sample Models/Art')
})

test('unusable paths do not choose a folder', () => {
  assert.equal(parentFolder(''), null)
  assert.equal(parentFolder('..'), null)
  assert.equal(parentFolder('library/../../etc/passwd'), null)
  assert.equal(parentFolder('/etc/passwd'), null)
  assert.equal(parentFolder('C:/models/Fire.nlogo'), null)
  assert.equal(parentFolder('library/./Fire.nlogo'), null)
  assert.equal(normalizeModelPath('  library/Fire.nlogo  '), 'library/Fire.nlogo')
})

test('the last model path round-trips and rejects a path outside the library', () => {
  const store = memory()
  assert.equal(readLastModelPath(store), null)
  writeLastModelPath('library/Sample Models/Art/Fire.nlogo', store)
  assert.equal(readLastModelPath(store), 'library/Sample Models/Art/Fire.nlogo')
  assert.equal(store.getItem(lastModelStorageKey), 'library/Sample Models/Art/Fire.nlogo')

  writeLastModelPath('../secret.nlogo', store)
  assert.equal(readLastModelPath(store), 'library/Sample Models/Art/Fire.nlogo')

  store.setItem(lastModelStorageKey, 'library/../../x.nlogo')
  assert.equal(readLastModelPath(store), null)
})

test('a stored dialog size stays inside the viewport', () => {
  assert.deepEqual(dialogSize(440, 480, 1200, 800), { width: 440, height: 480 })
  assert.deepEqual(dialogSize(2000, 2000, 500, 400), { width: 468, height: 368 })
  assert.deepEqual(dialogSize(10, 10, 1200, 800), { width: 280, height: 280 })
  assert.deepEqual(dialogSize(10, 10, 200, 180), { width: 168, height: 160 })
})
