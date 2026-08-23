// Source options for the pickers that wire a Processor's input or a
// Worker's attachment. A terminal source is either `trigger:<id>` or
// `processor_output:<processorId>:<outputId>` — the two things an event
// can arrive from.

import { ProcessorDTO } from '../../services/helixOrgService'
import { TriggerDTO } from '../../services/triggerService'

export type SourceOption = {
  // source is the wire value, e.g. "trigger:s-gh".
  source: string
  label: string
  kind: 'trigger' | 'processor_output'
}

export function triggerSource(id: string): string {
  return `trigger:${id}`
}

export function processorOutputSource(processorID: string, outputID: string): string {
  return `processor_output:${processorID}:${outputID}`
}

// parseTriggerSource returns the trigger id, or '' when the source names
// something else.
export function parseTriggerSource(source: string): string {
  return source.startsWith('trigger:') ? source.slice('trigger:'.length) : ''
}

// buildSourceOptions lists everything an input can be wired to.
// excludeProcessorID drops a Processor's own branches so the picker
// cannot offer the self-cycle the server rejects anyway.
export function buildSourceOptions(
  triggers: TriggerDTO[],
  processors: ProcessorDTO[],
  excludeProcessorID?: string,
): SourceOption[] {
  const options: SourceOption[] = triggers.map((t) => ({
    source: triggerSource(t.id ?? ''),
    label: t.name ? `${t.name} — ${t.id}` : (t.id ?? ''),
    kind: 'trigger',
  }))
  processors.forEach((p) => {
    if (p.id === excludeProcessorID) return
    ;(p.outputs ?? []).forEach((o) => {
      options.push({
        source: o.source || processorOutputSource(p.id, o.id),
        label: `${p.name} → ${o.label || o.id}`,
        kind: 'processor_output',
      })
    })
  })
  return options
}

// sourceLabel renders a stored source for display, falling back to the
// raw handle when the thing it names is gone.
export function sourceLabel(source: string, options: SourceOption[]): string {
  return options.find((o) => o.source === source)?.label ?? source
}

// The chart draws a Trigger under its bare id and a Processor branch
// under its full handle, so a stored source maps to a chart node id and
// back. Both directions are lossless.
export function chartNodeIDForSource(source: string): string {
  const triggerID = parseTriggerSource(source)
  return triggerID || source
}

export function sourceForChartNodeID(nodeID: string): string {
  if (!nodeID) return ''
  return nodeID.startsWith('processor_output:') ? nodeID : triggerSource(nodeID)
}
