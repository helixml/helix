import { describe, expect, it } from 'vitest'
import type { Node } from '@xyflow/react'

import { reconcileGraphNodes } from './HelixOrgChart'

describe('org chart refresh reconciliation', () => {
  it('preserves measured handle geometry while applying refreshed server data', () => {
    const current: Node[] = [{
      id: 'bot:report',
      type: 'bot',
      position: { x: 10, y: 20 },
      measured: { width: 440, height: 192 },
      selected: true,
      data: { status: 'stopped' },
    }]
    const refreshed: Node[] = [{
      id: 'bot:report',
      type: 'bot',
      position: { x: 30, y: 40 },
      data: { status: 'running' },
    }]

    expect(reconcileGraphNodes(current, refreshed)).toEqual([{
      ...refreshed[0],
      measured: { width: 440, height: 192 },
      selected: true,
    }])
  })

  it('keeps the live position while a refresh lands during a drag', () => {
    const current: Node[] = [{
      id: 'bot:report',
      position: { x: 55, y: 65 },
      dragging: true,
      data: {},
    }]
    const refreshed: Node[] = [{
      id: 'bot:report',
      position: { x: 10, y: 20 },
      data: { status: 'running' },
    }]

    expect(reconcileGraphNodes(current, refreshed)[0]).toMatchObject({
      position: { x: 55, y: 65 },
      dragging: true,
      data: { status: 'running' },
    })
  })
})
