/**
 * Regression test for Helix Cyber pentest finding TST-01-293.
 *
 * The served client bundles were fingerprinted as mermaid 11.12.2 and
 * axios 1.7.9. `npm audit` against those pins reported:
 *   - mermaid >=11.0.0-alpha.1 <=11.14.0  GHSA-ghcm-xqfw-q4vr
 *     (classDef in state diagrams -> HTML injection)
 *   - axios   >=1.0.0 <1.8.2              GHSA-jr5f-v2jv-69x6
 *     (SSRF / credential leakage via absolute URL)
 *
 * This test reproduces the audit check against the lockfiles the production
 * build actually installs from (yarn.lock: Dockerfile + Drone use
 * `yarn install`; package-lock.json is kept in sync for `npm audit`), and
 * against the packages vitest resolves, so CI fails if either dependency
 * regresses into the vulnerable range.
 */
import { describe, expect, it } from 'vitest'
import fs from 'fs'
import path from 'path'
import { MERMAID_CONFIG } from '../components/widgets/mermaidConfig'

const FRONTEND_ROOT = path.resolve(__dirname, '../..')

// Minimum safe version (first release outside each advisory's range).
const MIN_SAFE: Record<string, string> = {
  mermaid: '11.14.1', // GHSA-ghcm-xqfw-q4vr affects <=11.14.0
  axios: '1.8.2', // GHSA-jr5f-v2jv-69x6 affects <1.8.2
}

const parse = (v: string): number[] =>
  v.replace(/[-+].*$/, '').split('.').map((n) => parseInt(n, 10))

const compareVersions = (a: string, b: string): number => {
  const pa = parse(a)
  const pb = parse(b)
  for (let i = 0; i < 3; i++) {
    const d = (pa[i] || 0) - (pb[i] || 0)
    if (d !== 0) return d
  }
  // Same x.y.z: a prerelease sorts before the release.
  const preA = /-/.test(a)
  const preB = /-/.test(b)
  return preA === preB ? 0 : preA ? -1 : 1
}

const yarnLockVersions = (name: string): string[] => {
  const lock = fs.readFileSync(path.join(FRONTEND_ROOT, 'yarn.lock'), 'utf8')
  const versions: string[] = []
  const entry = new RegExp(`^"?${name}@[^\\n]*:\\n\\s+version "([^"]+)"`, 'gm')
  let m: RegExpExecArray | null
  while ((m = entry.exec(lock)) !== null) versions.push(m[1])
  return versions
}

const packageLockVersions = (name: string): string[] => {
  const lock = JSON.parse(
    fs.readFileSync(path.join(FRONTEND_ROOT, 'package-lock.json'), 'utf8'),
  )
  return Object.entries(lock.packages as Record<string, { version?: string }>)
    .filter(([key]) => key === `node_modules/${name}` || key.endsWith(`/node_modules/${name}`))
    .map(([, pkg]) => pkg.version as string)
}

const installedVersion = (name: string): string =>
  JSON.parse(
    fs.readFileSync(path.join(FRONTEND_ROOT, 'node_modules', name, 'package.json'), 'utf8'),
  ).version

describe('TST-01-293: vulnerable third-party components in client bundles', () => {
  it('version comparison matches the advisory ranges', () => {
    // The fingerprinted versions from the finding must be flagged as vulnerable.
    expect(compareVersions('11.12.2', MIN_SAFE.mermaid)).toBeLessThan(0)
    expect(compareVersions('11.14.0', MIN_SAFE.mermaid)).toBeLessThan(0)
    expect(compareVersions('1.7.9', MIN_SAFE.axios)).toBeLessThan(0)
    expect(compareVersions('1.8.1', MIN_SAFE.axios)).toBeLessThan(0)
    expect(compareVersions('1.8.2', MIN_SAFE.axios)).toBe(0)
    expect(compareVersions('11.17.2', MIN_SAFE.mermaid)).toBeGreaterThan(0)
  })

  for (const [name, minSafe] of Object.entries(MIN_SAFE)) {
    it(`yarn.lock pins ${name} >= ${minSafe}`, () => {
      const versions = yarnLockVersions(name)
      expect(versions.length).toBeGreaterThan(0)
      for (const v of versions) {
        expect(compareVersions(v, minSafe), `${name}@${v} in yarn.lock`).toBeGreaterThanOrEqual(0)
      }
    })

    it(`package-lock.json pins ${name} >= ${minSafe}`, () => {
      const versions = packageLockVersions(name)
      expect(versions.length).toBeGreaterThan(0)
      for (const v of versions) {
        expect(compareVersions(v, minSafe), `${name}@${v} in package-lock.json`).toBeGreaterThanOrEqual(0)
      }
    })

    it(`installed ${name} (what gets bundled) is >= ${minSafe}`, () => {
      expect(compareVersions(installedVersion(name), minSafe)).toBeGreaterThanOrEqual(0)
    })

    it(`package.json range for ${name} cannot resolve below ${minSafe}`, () => {
      const pkg = JSON.parse(fs.readFileSync(path.join(FRONTEND_ROOT, 'package.json'), 'utf8'))
      const range: string = pkg.dependencies[name]
      expect(range).toMatch(/^\^\d+\.\d+\.\d+$/)
      expect(compareVersions(range.slice(1), minSafe)).toBeGreaterThanOrEqual(0)
    })
  }

  it('mermaid is initialised with securityLevel strict (diagram text is untrusted)', () => {
    expect(MERMAID_CONFIG.securityLevel).toBe('strict')
  })
})
