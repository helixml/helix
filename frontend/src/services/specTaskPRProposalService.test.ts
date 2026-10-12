import { describe, expect, it } from 'vitest'
import { projectHasPullRequests, proposalsToReveal } from './specTaskPRProposalService'

describe('projectHasPullRequests', () => {
  it('is true when any repository is external', () => {
    expect(projectHasPullRequests([{}, { external_url: 'https://github.com/a/b' }])).toBe(true)
    expect(projectHasPullRequests([{ is_external: true }])).toBe(true)
  })

  it('is false for Helix-hosted repositories only', () => {
    expect(projectHasPullRequests([{}, { is_external: false }])).toBe(false)
    expect(projectHasPullRequests([])).toBe(false)
  })
})

describe('proposalsToReveal', () => {
  const p = (id: string, status: string) => ({ id, status } as any)

  it('reveals proposals that need the user, each only once', () => {
    const proposals = [
      p('prp_pending', 'pending'),
      p('prp_failed', 'failed'),
      p('prp_waiting', 'approved'),
      p('prp_done', 'opened'),
      p('prp_no', 'rejected'),
    ]
    expect(proposalsToReveal(proposals, new Set())).toEqual(['prp_pending', 'prp_failed'])
    expect(proposalsToReveal(proposals, new Set(['prp_pending']))).toEqual(['prp_failed'])
    expect(proposalsToReveal(proposals, new Set(['prp_pending', 'prp_failed']))).toEqual([])
  })
})
