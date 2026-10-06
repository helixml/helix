import { readdirSync, readFileSync } from 'node:fs'
import { relative, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const allowedDirectClipboardFiles = new Set([
  'components/external-agent/DesktopStreamViewer.tsx',
  'utils/clipboard.ts',
])

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    if (!/\.tsx?$/.test(entry.name) || /\.test\.tsx?$/.test(entry.name)) return []
    return [path]
  })
}

describe('clipboard usage', () => {
  it('routes ordinary text copies through the shared clipboard helper', () => {
    const directWriteText = new RegExp(
      ['navigator', 'clipboard', 'writeText'].join('\\s*\\.\\s*'),
    )
    const violations = sourceFiles(sourceRoot)
      .map((path) => relative(sourceRoot, path))
      .filter((path) => !allowedDirectClipboardFiles.has(path))
      .filter((path) => directWriteText.test(readFileSync(resolve(sourceRoot, path), 'utf8')))

    expect(violations).toEqual([])
  })
})
