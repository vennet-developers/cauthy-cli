'use strict'
// Finds the cauthy binary npm installed for this platform, and checks it is
// the one this release built: its SHA-256 must equal the one in
// checksums.json, written into this package when it was published. No
// network, no install script: npm installed the one optional dependency
// whose os and cpu match, with its own integrity check.

const { createHash } = require('node:crypto')
const { readFileSync } = require('node:fs')
const path = require('node:path')

const PACKAGES = {
  'darwin-arm64': '@cauthyhq/cli-darwin-arm64',
  'darwin-x64': '@cauthyhq/cli-darwin-x64',
  'linux-arm64': '@cauthyhq/cli-linux-arm64',
  'linux-x64': '@cauthyhq/cli-linux-x64',
  'win32-arm64': '@cauthyhq/cli-win32-arm64',
  'win32-x64': '@cauthyhq/cli-win32-x64',
}

function binaryName(platform) {
  return platform === 'win32' ? 'cauthy.exe' : 'cauthy'
}

function sha256(file) {
  return createHash('sha256').update(readFileSync(file)).digest('hex')
}

/**
 * The path of the verified binary.
 * @param {{platform: string, arch: string, resolve?: (id: string) => string, checksums?: Record<string, string>}} o
 */
function resolveBinary(o) {
  const key = `${o.platform}-${o.arch}`
  const pkg = PACKAGES[key]
  if (!pkg) {
    throw new Error(`no cauthy binary for ${key}: install it with go install github.com/vennet-developers/cauthy-cli/cmd/cauthy@latest`)
  }
  const resolve = o.resolve ?? ((id) => require.resolve(id))
  const checksums = o.checksums ?? JSON.parse(readFileSync(path.join(__dirname, '..', 'checksums.json'), 'utf8'))
  let file
  try {
    file = resolve(`${pkg}/bin/${binaryName(o.platform)}`)
  } catch {
    throw new Error(`${pkg} is not installed: npm skipped it (--omit=optional or --no-optional?). Install again with optional dependencies.`)
  }
  const want = checksums[key]
  if (!want) throw new Error(`this @cauthyhq/cli package carries no checksum for ${key}: it was not published by the release workflow`)
  const got = sha256(file)
  if (got !== want) {
    throw new Error(`the cauthy binary at ${file} does not match this release (sha256 ${got}, expected ${want}): reinstall, and report it if it persists`)
  }
  return file
}

module.exports = { PACKAGES, binaryName, resolveBinary, sha256 }
