'use strict'
const { test } = require('node:test')
const assert = require('node:assert')
const { mkdtempSync, writeFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const path = require('node:path')
const { PACKAGES, binaryName, resolveBinary, sha256 } = require('../lib/platform')

function fakeBinary(content) {
  const file = path.join(mkdtempSync(path.join(tmpdir(), 'cauthy-')), 'cauthy')
  writeFileSync(file, content)
  return file
}

test('every platform the release builds has a package', () => {
  assert.deepStrictEqual(Object.keys(PACKAGES).sort(), ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-arm64', 'win32-x64'])
  assert.strictEqual(binaryName('win32'), 'cauthy.exe')
  assert.strictEqual(binaryName('linux'), 'cauthy')
})

test('answers the binary when its checksum matches', () => {
  const file = fakeBinary('the binary')
  let asked
  const got = resolveBinary({ platform: 'linux', arch: 'x64', resolve: (id) => { asked = id; return file }, checksums: { 'linux-x64': sha256(file) } })
  assert.strictEqual(got, file)
  assert.strictEqual(asked, '@cauthyhq/cli-linux-x64/bin/cauthy')
})

test('refuses a binary that is not the one released', () => {
  const file = fakeBinary('something else')
  assert.throws(() => resolveBinary({ platform: 'linux', arch: 'x64', resolve: () => file, checksums: { 'linux-x64': '00'.repeat(32) } }), /does not match this release/)
  assert.throws(() => resolveBinary({ platform: 'linux', arch: 'x64', resolve: () => file, checksums: {} }), /carries no checksum/)
})

test('says what to do on an unknown platform or a skipped package', () => {
  assert.throws(() => resolveBinary({ platform: 'aix', arch: 'ppc64', checksums: {} }), /go install/)
  assert.throws(() => resolveBinary({ platform: 'linux', arch: 'x64', resolve: () => { throw new Error('nope') }, checksums: {} }), /optional dependencies/)
})
