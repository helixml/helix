import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import HelixOrgOverviewCard from './HelixOrgOverviewCard'

describe('HelixOrgOverviewCard', () => {
  it('gives the ID row the full card width below the title and status', () => {
    render(
      <HelixOrgOverviewCard
        title="ubuntu-1"
        id="a-82297faa-9916-4ab5-a4d9-881acca0d389"
        icon={<span>server</span>}
        status={<span>reachable</span>}
        idAction={<button>copy</button>}
      />,
    )

    const card = screen.getByTestId('helix-org-overview-card')
    const idRow = screen.getByTestId('helix-org-overview-id-row')
    expect(idRow.parentElement).toBe(card)
    expect(idRow).toHaveStyle({ width: '100%' })
    expect(idRow).toHaveTextContent('a-82297faa-9916-4ab5-a4d9-881acca0d389')
  })
})
