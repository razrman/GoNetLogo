import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  readSeedOverride,
  rngSeedError,
  rngSeedForApply,
  seedOverrideStorageKey,
  writeSeedOverride,
  type KeyValueStore,
} from './rngSeed.ts'

test('0 and 4294967295 are accepted', () => {
  assert.equal(rngSeedError('0'), null)
  assert.equal(rngSeedError('4294967295'), null)
  assert.equal(rngSeedError('1'), null)
  assert.equal(rngSeedError('2147483648'), null)
})

test('negatives, decimals, letters, overflow, and empty are rejected', () => {
  assert.match(rngSeedError(''), /0 to 4294967295/)
  assert.match(rngSeedError('-1'), /0 to 4294967295/)
  assert.match(rngSeedError('-0'), /0 to 4294967295/)
  assert.match(rngSeedError('1.5'), /0 to 4294967295/)
  assert.match(rngSeedError('1.0'), /0 to 4294967295/)
  assert.match(rngSeedError('abc'), /0 to 4294967295/)
  assert.match(rngSeedError('12a'), /0 to 4294967295/)
  assert.match(rngSeedError('4294967296'), /0 to 4294967295/)
  assert.match(rngSeedError(' 1'), /0 to 4294967295/)
  assert.match(rngSeedError('+1'), /0 to 4294967295/)
})

test('override on always sends the seed, including an unchanged one', () => {
  assert.equal(rngSeedForApply(true, '1'), '1')
  assert.equal(rngSeedForApply(true, '4294967295'), '4294967295')
  assert.equal(rngSeedForApply(false, '1'), undefined)
})

function memory(): KeyValueStore {
  const data = new Map<string, string>()
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
  }
}

test('the seed override checkbox is remembered without storing the seed', () => {
  const store = memory()
  assert.equal(readSeedOverride(store), false)
  writeSeedOverride(true, store)
  assert.equal(store.getItem(seedOverrideStorageKey), '1')
  assert.equal(readSeedOverride(store), true)
  writeSeedOverride(false, store)
  assert.equal(readSeedOverride(store), false)
})
