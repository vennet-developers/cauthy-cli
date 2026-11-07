import { test } from 'node:test'
import assert from 'node:assert'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { stage } from './stage.mjs'

function dist() {
  const dir = mkdtempSync(path.join(tmpdir(), 'dist-'))
  const artifacts = [{ type: 'Archive', goos: 'linux', goarch: 'amd64', path: 'x.tar.gz' }]
  for (const goos of ['darwin', 'linux', 'windows']) {
    for (const goarch of ['amd64', 'arm64']) {
      const p = path.join(dir, `cauthy_${goos}_${goarch}`, goos === 'windows' ? 'cauthy.exe' : 'cauthy')
      mkdirSync(path.dirname(p), { recursive: true })
      writeFileSync(p, `binary ${goos} ${goarch}`)
      artifacts.push({ type: 'Binary', goos, goarch, path: p })
    }
  }
  writeFileSync(path.join(dir, 'artifacts.json'), JSON.stringify(artifacts))
  return dir
}

test('stages six platform packages and the pinned wrapper', () => {
  const out = mkdtempSync(path.join(tmpdir(), 'npm-'))
  const keys = stage('1.0.0', dist(), out)
  assert.deepStrictEqual(keys, ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-arm64', 'win32-x64'])
  const wrapper = JSON.parse(readFileSync(path.join(out, 'cauthy', 'package.json'), 'utf8'))
  assert.strictEqual(wrapper.version, '1.0.0')
  assert.ok(Object.values(wrapper.optionalDependencies).every((v) => v === '1.0.0'))
  const sums = JSON.parse(readFileSync(path.join(out, 'cauthy', 'checksums.json'), 'utf8'))
  const win = readFileSync(path.join(out, 'cli-win32-x64', 'bin', 'cauthy.exe'))
  assert.strictEqual(sums['win32-x64'], createHash('sha256').update(win).digest('hex'))
  const pkg = JSON.parse(readFileSync(path.join(out, 'cli-win32-x64', 'package.json'), 'utf8'))
  assert.deepStrictEqual([pkg.name, pkg.os, pkg.cpu], ['@cauthyhq/cli-win32-x64', ['win32'], ['x64']])
  assert.ok(!existsSync(path.join(out, 'cauthy', 'test')))
  // Apache-2.0: every package says so and carries LICENSE and NOTICE.
  assert.strictEqual(wrapper.license, 'Apache-2.0')
  assert.strictEqual(pkg.license, 'Apache-2.0')
  for (const dir of ['cauthy', 'cli-win32-x64']) {
    assert.ok(readFileSync(path.join(out, dir, 'LICENSE'), 'utf8').includes('Apache License'))
    assert.ok(readFileSync(path.join(out, dir, 'NOTICE'), 'utf8').includes('Copyright 2026 Vennet Developers'))
  }
})

test('refuses a bad version or a partial build', () => {
  assert.throws(() => stage('v1', dist(), mkdtempSync(path.join(tmpdir(), 'npm-'))), /not a version/)
  const d = dist()
  const artifacts = JSON.parse(readFileSync(path.join(d, 'artifacts.json'), 'utf8')).slice(0, 4)
  writeFileSync(path.join(d, 'artifacts.json'), JSON.stringify(artifacts))
  assert.throws(() => stage('1.0.0', d, mkdtempSync(path.join(tmpdir(), 'npm-'))), /expected 6 binaries/)
})
