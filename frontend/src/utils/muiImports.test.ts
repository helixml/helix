import { readdirSync, readFileSync } from 'node:fs'
import { relative, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    return /\.tsx?$/.test(entry.name) ? [path] : []
  })
}

// The MUI barrels load every component or icon at runtime (the icons barrel
// alone takes ~2s in Node), and each vitest file pays that again. Import by
// path: '@mui/material/Box', '@mui/icons-material/Close'. Type-only imports
// from '@mui/material' are erased at compile time and stay allowed.
describe('MUI imports', () => {
  it('never pull a whole MUI barrel in at runtime', () => {
    const barrelValueImport = /^import\s+(?!type\b)[\w$*\s{},]*\sfrom\s+['"]@mui\/(material|icons-material)['"]/m
    const violations = sourceFiles(sourceRoot)
      .filter((path) => barrelValueImport.test(readFileSync(path, 'utf8')))
      .map((path) => relative(sourceRoot, path))

    expect(violations).toEqual([])
  })
})
