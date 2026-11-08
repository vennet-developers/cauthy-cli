#!/usr/bin/env node
// Builds the npm packages from a goreleaser dist folder:
//
//   node npm/scripts/stage.mjs <version> <dist folder> <out folder>
//
// One package per platform (@cauthyhq/cli-<os>-<cpu>) holding its binary, and
// the @cauthyhq/cli package (its bin is cauthy) with its optionalDependencies pinned to <version> and
// checksums.json holding each binary's SHA-256. Reads dist/artifacts.json,
// goreleaser's list of what it built.

import { createHash } from 'node:crypto'
import { chmodSync, copyFileSync, cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const OS = { darwin: 'darwin', linux: 'linux', windows: 'win32' }
const CPU = { amd64: 'x64', arm64: 'arm64' }
const here = path.dirname(fileURLToPath(import.meta.url))

// legal copies the repository's LICENSE and NOTICE into a package: Apache-2.0
// asks that both travel with every copy.
function legal(dir) {
  for (const f of ['LICENSE', 'NOTICE']) copyFileSync(path.join(here, '..', '..', f), path.join(dir, f))
}

export function stage(version, dist, out) {
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error(`not a version: ${version}`)
  const artifacts = JSON.parse(readFileSync(path.join(dist, 'artifacts.json'), 'utf8'))
  const binaries = artifacts.filter((a) => a.type === 'Binary' && OS[a.goos] && CPU[a.goarch])
  if (binaries.length !== 6) throw new Error(`expected 6 binaries in ${dist}/artifacts.json, found ${binaries.length}`)
  rmSync(out, { recursive: true, force: true })
  const checksums = {}
  const wrapper = JSON.parse(readFileSync(path.join(here, '..', 'cli', 'package.json'), 'utf8'))
  for (const b of binaries) {
    const key = `${OS[b.goos]}-${CPU[b.goarch]}`
    const name = `@cauthyhq/cli-${key}`
    const dir = path.join(out, `cli-${key}`)
    const exe = OS[b.goos] === 'win32' ? 'cauthy.exe' : 'cauthy'
    mkdirSync(path.join(dir, 'bin'), { recursive: true })
    copyFileSync(path.resolve(b.path), path.join(dir, 'bin', exe))
    chmodSync(path.join(dir, 'bin', exe), 0o755)
    checksums[key] = createHash('sha256').update(readFileSync(path.join(dir, 'bin', exe))).digest('hex')
    writeFileSync(path.join(dir, 'package.json'), JSON.stringify({
      name, version,
      description: `The cauthy binary for ${key}. Installed by @cauthyhq/cli; not meant to be installed alone.`,
      license: wrapper.license, repository: wrapper.repository,
      os: [OS[b.goos]], cpu: [CPU[b.goarch]], files: ['bin', 'LICENSE', 'NOTICE'], preferUnplugged: true,
    }, null, 2) + '\n')
    legal(dir)
    if (!(name in wrapper.optionalDependencies)) throw new Error(`${name} is not in @cauthyhq/cli's optionalDependencies`)
    wrapper.optionalDependencies[name] = version
  }
  wrapper.version = version
  const dir = path.join(out, 'cli')
  cpSync(path.join(here, '..', 'cli'), dir, { recursive: true, filter: (p) => !p.includes(`${path.sep}test`) })
  writeFileSync(path.join(dir, 'package.json'), JSON.stringify(wrapper, null, 2) + '\n')
  writeFileSync(path.join(dir, 'checksums.json'), JSON.stringify(checksums, null, 2) + '\n')
  legal(dir)
  return Object.keys(checksums).sort()
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [version, dist, out] = process.argv.slice(2)
  if (!version || !dist || !out) {
    process.stderr.write('usage: node npm/scripts/stage.mjs <version> <dist folder> <out folder>\n')
    process.exit(2)
  }
  const staged = stage(version, dist, out)
  process.stdout.write(`staged @cauthyhq/cli ${version} for ${staged.join(', ')} in ${out}\n`)
}
