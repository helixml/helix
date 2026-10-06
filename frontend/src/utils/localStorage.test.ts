import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SELECTED_ORG_STORAGE_KEY,
  clearSelectedOrg,
  getSelectedOrg,
  setSelectedOrg,
} from './localStorage'

describe('selected org storage', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('round-trips the selected org slug', () => {
    setSelectedOrg('acme')
    expect(getSelectedOrg()).toBe('acme')
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBe('acme')
  })

  it('reports undefined when nothing is stored', () => {
    expect(getSelectedOrg()).toBeUndefined()
  })

  it('reports undefined rather than an empty string', () => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, '')
    expect(getSelectedOrg()).toBeUndefined()
  })

  it('clears the stored org', () => {
    setSelectedOrg('acme')
    clearSelectedOrg()
    expect(getSelectedOrg()).toBeUndefined()
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBeNull()
  })

  // Remembering the org is a convenience. Private mode / quota errors must not
  // take down sign-in or sign-out.
  it('survives storage being unavailable', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })

    expect(() => setSelectedOrg('acme')).not.toThrow()
    expect(getSelectedOrg()).toBeUndefined()
    expect(() => clearSelectedOrg()).not.toThrow()
  })
})
