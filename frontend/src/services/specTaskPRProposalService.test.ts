import { describe, expect, it } from 'vitest'
import { projectHasPullRequests } from './specTaskPRProposalService'

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
