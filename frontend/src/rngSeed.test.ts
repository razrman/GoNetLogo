import assert from 'node:assert/strict'
import { test } from 'node:test'
import { rngSeedError } from './rngSeed.ts'

test('a 6-character base64 seed is accepted', () => {
  assert.equal(rngSeedError('AAAAAA'), null)
  assert.equal(rngSeedError('AAAAAB'), null)
  assert.equal(rngSeedError('zzzzzz'), null)
  assert.equal(rngSeedError('0123+/'), null)
  assert.equal(rngSeedError('//////'), null)
})

test('wrong length is rejected and not clamped', () => {
  assert.match(rngSeedError(''), /6 characters, not 0/)
  assert.match(rngSeedError('AAAAA'), /6 characters, not 5/)
  assert.match(rngSeedError('AAAAAAA'), /6 characters, not 7/)
  assert.match(rngSeedError('AAAAA '), /alphabet/)
})

test('characters outside the alphabet are rejected', () => {
  assert.match(rngSeedError('AAAAA='), /alphabet/)
  assert.match(rngSeedError('AAA*AA'), /alphabet/)
  assert.match(rngSeedError('AAAAAé'), /alphabet/)
  assert.match(rngSeedError('aaaaa\n'), /6 characters|alphabet/)
})
