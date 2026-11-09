import assert from 'node:assert/strict'
import { test } from 'node:test'
import { rngSeedError } from './rngSeed.ts'

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
