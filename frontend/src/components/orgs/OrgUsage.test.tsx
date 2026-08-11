import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { UsageProviderIcon } from './OrgUsage'

describe('UsageProviderIcon', () => {
  it('uses the Helix logo for globally managed inference providers', () => {
    const { container } = render(<UsageProviderIcon provider="helix/ds4-flash-node06" size={18} />)

    const image = container.querySelector('img')
    expect(image).not.toBeNull()
    expect(image?.getAttribute('src')).toContain('logo.png')
    expect(image).toHaveStyle({ width: '18px', height: '18px' })
  })
})
