import { describe, expect, it } from 'vitest'

import { centeredCreatedNodePosition } from './chartNodeGeometry'

describe('centeredCreatedNodePosition', () => {
  it.each([
    ['bot', { x: 390, y: 252 }],
    ['topic', { x: 410, y: 260 }],
    ['processor', { x: 390, y: 261 }],
    ['asset', { x: 390, y: 250 }],
  ] as const)('centers a new %s node at the canvas click', (kind, expected) => {
    expect(centeredCreatedNodePosition(kind, { x: 500, y: 300 })).toEqual(expected)
  })
})
