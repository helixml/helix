import { describe, expect, it } from 'vitest'

import { extractErrorMessage } from './useErrorCallback'

describe('extractErrorMessage', () => {
  it('serializes an object response body to a string', () => {
    const message = extractErrorMessage({
      response: { data: { status: 524, detail: 'A timeout occurred' } },
    })

    expect(message).toBe('{"status":524,"detail":"A timeout occurred"}')
    expect(() => message.toLowerCase()).not.toThrow()
  })
})
