import { describe, expect, it } from 'vitest'

import { artifactMutationData } from './artifactService'

describe('artifactMutationData', () => {
  it('omits absent multipart fields instead of serializing undefined', () => {
    expect(artifactMutationData({
      name: 'Viewer smoke test',
      visibility: 'project',
    })).toEqual({
      name: 'Viewer smoke test',
      visibility: 'project',
    })
  })
})
