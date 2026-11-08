#!/usr/bin/env node
'use strict'
// npx @cauthyhq/cli (or cauthy, once installed): runs the verified Go binary for this platform with the same
// arguments, and exits with its code.

const { spawnSync } = require('node:child_process')
const { resolveBinary } = require('../lib/platform')

let bin
try {
  bin = resolveBinary({ platform: process.platform, arch: process.arch })
} catch (err) {
  process.stderr.write(`cauthy: ${err.message}\n`)
  process.exit(2)
}
const run = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' })
if (run.error) {
  process.stderr.write(`cauthy: ${run.error.message}\n`)
  process.exit(2)
}
if (run.signal) process.kill(process.pid, run.signal)
process.exit(run.status ?? 1)
