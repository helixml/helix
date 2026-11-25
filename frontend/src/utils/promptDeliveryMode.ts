/**
 * How a prompt reaches an agent that is mid-turn:
 * - steer: delivered into the running turn at its next step; nothing is cancelled
 * - queue: waits until the running turn finishes
 * - interrupt: cancels the running turn, then sends
 *
 * On the wire a prompt carries two booleans (interrupt, steer); they are never
 * both true.
 */
export type PromptDeliveryMode = 'steer' | 'queue' | 'interrupt'

export interface PromptDeliveryFlags {
  interrupt?: boolean
  steer?: boolean
}

export function deliveryModeOf(entry: PromptDeliveryFlags): PromptDeliveryMode {
  if (entry.steer) return 'steer'
  // Entries predating the interrupt flag were interrupts.
  return entry.interrupt !== false ? 'interrupt' : 'queue'
}

export function deliveryFlags(mode: PromptDeliveryMode): { interrupt: boolean; steer: boolean } {
  return { interrupt: mode === 'interrupt', steer: mode === 'steer' }
}

/** Next mode when cycling a queued entry's mode chip. Skips steer when the agent can't. */
export function nextDeliveryMode(mode: PromptDeliveryMode, steeringAvailable: boolean): PromptDeliveryMode {
  const order: PromptDeliveryMode[] = steeringAvailable
    ? ['steer', 'queue', 'interrupt']
    : ['queue', 'interrupt']
  const index = order.indexOf(mode)
  return order[(index + 1) % order.length]
}

export const DELIVERY_MODE_LABEL: Record<PromptDeliveryMode, string> = {
  steer: 'Steer',
  queue: 'Queue',
  interrupt: 'Interrupt',
}

export const DELIVERY_MODE_DESCRIPTION: Record<PromptDeliveryMode, string> = {
  steer: 'Delivered to the agent at its next step, without stopping its current work',
  queue: 'Sent after the agent finishes its current turn',
  interrupt: 'Stops the agent’s current turn, then sends',
}
