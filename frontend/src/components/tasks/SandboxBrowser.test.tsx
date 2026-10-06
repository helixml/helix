import '@testing-library/jest-dom/vitest'

import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const previewMocks = vi.hoisted(() => ({
  refetch: vi.fn(),
  create: vi.fn(),
  config: vi.fn(),
}))

vi.mock('../../services/userService', () => ({
  useGetConfig: () => previewMocks.config(),
}))

vi.mock('../../services/sessionPreviewService', () => ({
  useSessionPreviewTokens: () => ({
    data: [{
      id: 'vhr_test',
      hostname: 'share-blue-fox.dev.localhost',
      url: 'https://share-blue-fox.dev.localhost:8080',
      port: 8080,
    }],
    refetch: previewMocks.refetch,
  }),
  useCreateSessionPreviewToken: () => ({
    isPending: false,
    mutateAsync: previewMocks.create,
  }),
}))

import SandboxBrowser from './SandboxBrowser'

describe('SandboxBrowser', () => {
  beforeEach(() => {
    window.localStorage.clear()
    previewMocks.refetch.mockReset()
    previewMocks.create.mockReset()
    previewMocks.config.mockReset()
    previewMocks.config.mockReturnValue({
      data: { dev_subdomain: 'dev.localhost', preview_url_https: false },
      isLoading: false,
    })
  })

  it('automatically opens the saved address on mount', async () => {
    window.localStorage.setItem(
      'helix.sandboxBrowser.url.ses_test',
      'http://localhost:8080/docs/deploy-sovereign-server',
    )

    render(<SandboxBrowser sessionId="ses_test" />)

    const frame = await screen.findByTitle(
      'Sandbox browser: http://localhost:8080/docs/deploy-sovereign-server',
    )
    expect(frame).toHaveAttribute(
      'src',
      'http://share-blue-fox.dev.localhost:8080/docs/deploy-sovereign-server',
    )
    expect(screen.getByRole('textbox', { name: 'Sandbox browser address' }))
      .toHaveValue('http://localhost:8080/docs/deploy-sovereign-server')
    expect(previewMocks.refetch).not.toHaveBeenCalled()
    expect(previewMocks.create).not.toHaveBeenCalled()
  })

  it('automatically opens the saved address after switching sessions', async () => {
    window.localStorage.setItem(
      'helix.sandboxBrowser.url.ses_first',
      'http://localhost:8080/first',
    )
    window.localStorage.setItem(
      'helix.sandboxBrowser.url.ses_second',
      'http://localhost:8080/second',
    )

    const { rerender } = render(<SandboxBrowser sessionId="ses_first" />)
    await screen.findByTitle('Sandbox browser: http://localhost:8080/first')

    rerender(<SandboxBrowser sessionId="ses_second" />)

    await screen.findByTitle('Sandbox browser: http://localhost:8080/second')
    expect(screen.getByRole('textbox', { name: 'Sandbox browser address' }))
      .toHaveValue('http://localhost:8080/second')
  })

  it('opens a localhost path through the existing preview hostname', async () => {
    render(<SandboxBrowser sessionId="ses_test" />)

    const address = screen.getByRole('textbox', { name: 'Sandbox browser address' })
    fireEvent.change(address, { target: { value: 'http://localhost:8080/docs?q=one' } })
    fireEvent.submit(address.closest('form')!)

    const frame = await screen.findByTitle('Sandbox browser: http://localhost:8080/docs?q=one')
    expect(frame).toHaveAttribute(
      'src',
      'http://share-blue-fox.dev.localhost:8080/docs?q=one',
    )
    expect(frame).toHaveStyle('color-scheme: dark')
    expect(screen.getByRole('link', { name: 'Open browser preview in new tab' })).toHaveAttribute(
      'href',
      'http://share-blue-fox.dev.localhost:8080/docs?q=one',
    )
    expect(screen.getByRole('link', { name: 'Open browser preview in new tab' })).toHaveAttribute(
      'target',
      '_blank',
    )
    expect(previewMocks.refetch).not.toHaveBeenCalled()
    expect(previewMocks.create).not.toHaveBeenCalled()
    await waitFor(() => expect(window.localStorage.getItem('helix.sandboxBrowser.url.ses_test'))
      .toBe('http://localhost:8080/docs?q=one'))
  })

  it('shows required environment settings when previews are not configured', () => {
    previewMocks.config.mockReturnValue({
      data: { dev_subdomain: '', preview_url_https: true },
      isLoading: false,
    })

    render(<SandboxBrowser sessionId="ses_test" />)

    expect(screen.getByText('Sandbox browser previews are not configured')).toBeInTheDocument()
    expect(screen.getByText(/DEV_SUBDOMAIN=preview\.example\.com/)).toBeInTheDocument()
    expect(screen.getByText(/PREVIEW_URL_HTTPS=true/)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: 'Sandbox browser address' })).not.toBeInTheDocument()
    expect(previewMocks.create).not.toHaveBeenCalled()
  })

  it('opens multiple tabs and supports the file-tab close actions', async () => {
    render(<SandboxBrowser sessionId="ses_test" />)

    await screen.findByTitle('Sandbox browser: http://localhost:8080/')
    expect(screen.getAllByRole('tab')).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: 'New browser tab' }))
    fireEvent.click(screen.getByRole('button', { name: 'New browser tab' }))
    expect(screen.getAllByRole('tab')).toHaveLength(3)

    fireEvent.contextMenu(screen.getAllByRole('tab')[0])
    fireEvent.click(screen.getByRole('menuitem', { name: 'Close others' }))
    expect(screen.getAllByRole('tab')).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: 'New browser tab' }))
    fireEvent.click(screen.getByRole('button', { name: 'New browser tab' }))
    fireEvent.contextMenu(screen.getAllByRole('tab')[0])
    fireEvent.click(screen.getByRole('menuitem', { name: 'Close to the right' }))
    expect(screen.getAllByRole('tab')).toHaveLength(1)

    fireEvent.contextMenu(screen.getAllByRole('tab')[0])
    fireEvent.click(screen.getByRole('menuitem', { name: 'Close all' }))
    expect(screen.queryAllByRole('tab')).toHaveLength(0)
    expect(screen.getByText('All browser tabs are closed.')).toBeInTheDocument()
  })

  it('keeps each tab preview mounted when switching tabs', async () => {
    render(<SandboxBrowser sessionId="ses_test" />)

    const firstAddress = screen.getByRole('textbox', { name: 'Sandbox browser address' })
    fireEvent.change(firstAddress, { target: { value: 'http://localhost:8080/first' } })
    fireEvent.submit(firstAddress.closest('form')!)
    const firstFrame = await screen.findByTitle('Sandbox browser: http://localhost:8080/first')

    fireEvent.click(screen.getByRole('button', { name: 'New browser tab' }))
    const secondAddress = screen.getByRole('textbox', { name: 'Sandbox browser address' })
    fireEvent.change(secondAddress, { target: { value: 'http://localhost:8080/second' } })
    fireEvent.submit(secondAddress.closest('form')!)
    const secondFrame = await screen.findByTitle('Sandbox browser: http://localhost:8080/second')

    expect(secondFrame).toBeVisible()
    expect(firstFrame).not.toBeVisible()
    fireEvent.click(screen.getAllByRole('tab')[0])
    expect(firstFrame).toBeVisible()
    expect(secondFrame).not.toBeVisible()
  })

  it('tracks iframe navigation and reloads the current path', async () => {
    render(<SandboxBrowser sessionId="ses_test" />)

    const address = screen.getByRole('textbox', { name: 'Sandbox browser address' })
    fireEvent.change(address, { target: { value: 'http://localhost:8080/' } })
    fireEvent.submit(address.closest('form')!)
    const firstFrame = await screen.findByTitle('Sandbox browser: http://localhost:8080/')

    act(() => {
      window.dispatchEvent(new MessageEvent('message', {
        data: {
          type: 'helix:sandbox-browser:navigate',
          href: 'http://share-blue-fox.dev.localhost:8080/dashboard?q=one#status',
          navigationType: 'push',
        },
        origin: 'http://share-blue-fox.dev.localhost:8080',
        source: (firstFrame as HTMLIFrameElement).contentWindow,
      }))
    })

    await waitFor(() => expect(address).toHaveValue(
      'http://localhost:8080/dashboard?q=one#status',
    ))
    const navigatedFrame = screen.getByTitle(
      'Sandbox browser: http://localhost:8080/dashboard?q=one#status',
    )
    expect(navigatedFrame).toBe(firstFrame)
    expect(navigatedFrame).toHaveAttribute(
      'src',
      'http://share-blue-fox.dev.localhost:8080/',
    )
    expect(window.localStorage.getItem('helix.sandboxBrowser.url.ses_test'))
      .toBe('http://localhost:8080/dashboard?q=one#status')

    fireEvent.click(screen.getByRole('button', { name: 'Reload browser preview' }))
    const reloadedFrame = await screen.findByTitle(
      'Sandbox browser: http://localhost:8080/dashboard?q=one#status',
    )
    expect(reloadedFrame).toHaveAttribute(
      'src',
      'http://share-blue-fox.dev.localhost:8080/dashboard?q=one#status',
    )
  })

  it('ignores navigation messages that do not match the preview origin', async () => {
    render(<SandboxBrowser sessionId="ses_test" />)

    const address = screen.getByRole('textbox', { name: 'Sandbox browser address' })
    fireEvent.change(address, { target: { value: 'http://localhost:8080/' } })
    fireEvent.submit(address.closest('form')!)
    const frame = await screen.findByTitle('Sandbox browser: http://localhost:8080/')

    act(() => {
      window.dispatchEvent(new MessageEvent('message', {
        data: {
          type: 'helix:sandbox-browser:navigate',
          href: 'https://example.com/phishing',
          navigationType: 'push',
        },
        origin: 'https://example.com',
        source: (frame as HTMLIFrameElement).contentWindow,
      }))
    })

    expect(address).toHaveValue('http://localhost:8080/')
  })
})
