import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ResetBotDialog from './ResetBotDialog'

const plan = vi.hoisted(() => ({
  value: undefined as any,
}))

vi.mock('../../services/helixOrgService', () => ({
  useBotResetPlan: () => ({ data: plan.value, isLoading: false, error: null }),
}))

const SESSION = {
  scope: 'session',
  available: true,
  erases: ['queued and in-flight activations (purged, not replayed)', 'current session, its conversation thread and desktop container'],
  keeps: ['bot home (/home/retro/work, including Chrome profiles)', 'other bots and other organizations'],
}
const HOME = {
  scope: 'session_and_home',
  available: true,
  erases: [...SESSION.erases, 'bot home (/home/retro/work, including Chrome profiles and logins), retired to rollback storage'],
  keeps: ['other bots and other organizations'],
}

const renderDialog = (onReset = vi.fn().mockResolvedValue(undefined), onClose = vi.fn()) => {
  render(<ResetBotDialog open botId="b-bird" botName="Birding Promoter" onClose={onClose} onReset={onReset} />)
  return { onReset, onClose }
}

describe('ResetBotDialog', () => {
  beforeEach(() => {
    plan.value = { scopes: [SESSION, HOME] }
  })

  it('lists exactly what the selected scope deletes and keeps', () => {
    renderDialog()
    const decl = screen.getByTestId('reset-declaration')
    for (const item of SESSION.erases) expect(decl).toHaveTextContent(item)
    for (const item of SESSION.keeps) expect(decl).toHaveTextContent(item)
    expect(decl).not.toHaveTextContent('retired to rollback storage')

    fireEvent.click(screen.getByLabelText('Reset session and bot home'))
    expect(screen.getByTestId('reset-declaration')).toHaveTextContent('retired to rollback storage')
  })

  it('stays disabled until the bot name is typed exactly', () => {
    const { onReset } = renderDialog()
    const button = screen.getByTestId('reset-confirm')
    expect(button).toBeDisabled()
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'b-bird' } })
    expect(button).toBeDisabled()
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'birding promoter' } })
    expect(button).toBeDisabled()
    fireEvent.click(button)
    expect(onReset).not.toHaveBeenCalled()
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'Birding Promoter' } })
    expect(button).toBeEnabled()
  })

  it('resets with the selected scope', async () => {
    const { onReset, onClose } = renderDialog()
    fireEvent.click(screen.getByLabelText('Reset session and bot home'))
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'Birding Promoter' } })
    fireEvent.click(screen.getByTestId('reset-confirm'))
    await waitFor(() => expect(onReset).toHaveBeenCalledWith('session_and_home'))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('never silently widens the scope: an unavailable scope cannot be chosen', () => {
    plan.value = {
      scopes: [{ ...SESSION, available: false, unavailable_reason: 'bot b-bird still keeps its home in session ses_1' }, HOME],
    }
    const { onReset } = renderDialog()
    expect(screen.getByText(/still keeps its home in session/)).toBeInTheDocument()
    const sessionRadio = screen.getByLabelText('Reset session') as HTMLInputElement
    expect(sessionRadio).toBeDisabled()
    // The broader scope is selected and shown in full before confirmation.
    expect(screen.getByTestId('reset-declaration')).toHaveTextContent('retired to rollback storage')
    expect(onReset).not.toHaveBeenCalled()
  })

  it('reports a failed or partial teardown and stays open', async () => {
    const failure = { response: { data: { error: 'erase bot home: retire of generation bh_1 on runner sb1 failed; the home may still exist' } } }
    const { onReset, onClose } = renderDialog(vi.fn().mockRejectedValue(failure))
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'Birding Promoter' } })
    fireEvent.click(screen.getByTestId('reset-confirm'))
    expect(await screen.findByTestId('reset-error')).toHaveTextContent('the home may still exist')
    expect(onReset).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('offers no reset while the bot home still lives in the session', () => {
    const reason = 'bot b-bird still keeps its home in session ses_1, so deleting the session would destroy it without the declared rollback copy; restart the bot once to move the home out of the session, then reset'
    plan.value = {
      scopes: [
        { ...SESSION, available: false, unavailable_reason: reason },
        { ...HOME, available: false, unavailable_reason: reason },
      ],
    }
    const { onReset } = renderDialog()
    expect(screen.getByLabelText('Reset session')).toBeDisabled()
    expect(screen.getByLabelText('Reset session and bot home')).toBeDisabled()
    fireEvent.change(screen.getByTestId('reset-confirm-input'), { target: { value: 'Birding Promoter' } })
    expect(screen.getByTestId('reset-confirm')).toBeDisabled()
    fireEvent.click(screen.getByTestId('reset-confirm'))
    expect(onReset).not.toHaveBeenCalled()
    expect(screen.getAllByText(/restart the bot once/).length).toBeGreaterThan(0)
  })

  it('offers no reset when the plan cannot be loaded', () => {
    plan.value = undefined
    renderDialog()
    expect(screen.getByTestId('reset-confirm')).toBeDisabled()
  })
})
