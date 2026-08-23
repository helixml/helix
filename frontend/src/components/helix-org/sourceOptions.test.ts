import { describe, expect, it } from 'vitest'
import {
  buildSourceOptions,
  chartNodeIDForSource,
  parseTriggerSource,
  sourceForChartNodeID,
  sourceLabel,
} from './sourceOptions'

const triggers = [
  { id: 's-gh', name: 'GitHub', kind: 'github' },
  { id: 's-cron', kind: 'cron' },
]
const processors = [
  { id: 'p-1', name: 'Router', input_source: 'trigger:s-gh', kind: 'filter', outputs: [
    { id: 'po-vip', label: 'VIP', source: 'processor_output:p-1:po-vip' },
    { id: 'po-rest' },
  ] },
] as any

describe('buildSourceOptions', () => {
  it('lists Triggers and every Processor branch', () => {
    const options = buildSourceOptions(triggers, processors)
    expect(options.map((o) => o.source)).toEqual([
      'trigger:s-gh',
      'trigger:s-cron',
      'processor_output:p-1:po-vip',
      'processor_output:p-1:po-rest',
    ])
    expect(options[0].label).toBe('GitHub — s-gh')
    // A Trigger with no name still needs a label to pick from.
    expect(options[1].label).toBe('s-cron')
    expect(options[2].label).toBe('Router → VIP')
    expect(options[3].label).toBe('Router → po-rest')
  })

  it('drops a Processor own branches so the picker cannot offer a self-cycle', () => {
    const options = buildSourceOptions(triggers, processors, 'p-1')
    expect(options.every((o) => o.kind === 'trigger')).toBe(true)
  })
})

describe('sourceLabel', () => {
  it('falls back to the raw handle when the source is gone', () => {
    const options = buildSourceOptions(triggers, processors)
    expect(sourceLabel('trigger:s-gone', options)).toBe('trigger:s-gone')
  })
})

describe('chart node id round trip', () => {
  it('maps a Trigger source to its bare id and back', () => {
    expect(chartNodeIDForSource('trigger:s-gh')).toBe('s-gh')
    expect(sourceForChartNodeID('s-gh')).toBe('trigger:s-gh')
    expect(parseTriggerSource('trigger:s-gh')).toBe('s-gh')
  })

  it('leaves a Processor branch handle intact in both directions', () => {
    expect(chartNodeIDForSource('processor_output:p-1:po-vip')).toBe('processor_output:p-1:po-vip')
    expect(sourceForChartNodeID('processor_output:p-1:po-vip')).toBe('processor_output:p-1:po-vip')
    expect(parseTriggerSource('processor_output:p-1:po-vip')).toBe('')
  })

  it('maps an empty node id to an empty source, which disconnects the input', () => {
    expect(sourceForChartNodeID('')).toBe('')
    expect(chartNodeIDForSource('')).toBe('')
  })
})
