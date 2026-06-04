#!/usr/bin/env node
// Fails the build if any source file imports from `@mui/icons-material`
// without a deep specifier. Barrel imports inflate the dev bundle by ~7,000
// modules and make Vite cold-start very slow.
//
// Wrong:
//   import { Add, Close } from '@mui/icons-material'
//   import * as Icons from '@mui/icons-material'
//
// Right:
//   import Add from '@mui/icons-material/Add'
//   import Close from '@mui/icons-material/Close'

import { promises as fs } from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(new URL('..', import.meta.url).pathname, 'src')

const SKIP_DIRS = new Set(['node_modules', 'dist', '.next', 'build'])
const EXTS = new Set(['.ts', '.tsx'])

const BARREL_RE = /from\s+['"]@mui\/icons-material['"]/

async function* walk(dir) {
  const entries = await fs.readdir(dir, { withFileTypes: true })
  for (const entry of entries) {
    if (SKIP_DIRS.has(entry.name)) continue
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      yield* walk(full)
    } else if (EXTS.has(path.extname(entry.name))) {
      yield full
    }
  }
}

const violations = []
for await (const file of walk(ROOT)) {
  const content = await fs.readFile(file, 'utf8')
  const lines = content.split('\n')
  lines.forEach((line, idx) => {
    if (BARREL_RE.test(line)) {
      violations.push({ file: path.relative(process.cwd(), file), line: idx + 1, text: line.trim() })
    }
  })
}

if (violations.length > 0) {
  console.error('Barrel imports from @mui/icons-material are forbidden — use deep imports instead.')
  console.error('  Wrong:  import { Add } from "@mui/icons-material"')
  console.error('  Right:  import Add from "@mui/icons-material/Add"')
  console.error('')
  for (const v of violations) {
    console.error(`  ${v.file}:${v.line}  ${v.text}`)
  }
  console.error(`\n${violations.length} violation(s)`)
  process.exit(1)
}

console.log('OK — no barrel @mui/icons-material imports')
