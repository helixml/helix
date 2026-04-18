import React from 'react'
import { isMobileOrTablet } from '../../utils/isMobileOrTablet'
import { logErrorToSession, getRecentErrors, clearErrorLog } from '../../utils/errorSessionLog'

interface Props {
  children: React.ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
  errorInfo: React.ErrorInfo | null
}

// GPU-safe inline styles — no position:fixed, no filter, no will-change, no opacity animations
const overlayStyle: React.CSSProperties = {
  position: 'absolute',
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  zIndex: 99999,
  backgroundColor: '#1a1a2e',
  color: '#e0e0e0',
  fontFamily: 'monospace',
  fontSize: '13px',
  overflow: 'auto',
  WebkitOverflowScrolling: 'touch',
  padding: '16px',
  boxSizing: 'border-box',
}

const headerStyle: React.CSSProperties = {
  borderBottom: '3px solid #e74c3c',
  paddingBottom: '12px',
  marginBottom: '12px',
}

const buttonStyle: React.CSSProperties = {
  padding: '10px 20px',
  marginRight: '10px',
  marginTop: '8px',
  border: '1px solid #555',
  borderRadius: '4px',
  backgroundColor: '#2a2a3e',
  color: '#e0e0e0',
  fontFamily: 'monospace',
  fontSize: '14px',
  cursor: 'pointer',
}

export default class ErrorBoundary extends React.Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null, errorInfo: null }
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    this.setState({ errorInfo })

    // Log to sessionStorage for post-crash recovery
    logErrorToSession(error.message, error.stack)

    // Forward to Sentry/analytics via existing global hook
    try {
      const win = window as any
      if (typeof win.emitError === 'function') {
        win.emitError(error)
      }
    } catch {
      // ignore
    }
  }

  handleCopy = () => {
    const { error, errorInfo } = this.state
    const previousErrors = getRecentErrors()
    let text = `Error: ${error?.message || 'Unknown error'}\n\n`
    if (error?.stack) text += `Stack:\n${error.stack}\n\n`
    if (errorInfo?.componentStack) text += `Component Stack:\n${errorInfo.componentStack}\n\n`
    if (previousErrors.length > 0) {
      text += `Previous errors:\n`
      previousErrors.forEach(e => {
        text += `[${e.timestamp}] ${e.message}\n`
      })
    }
    text += `\nURL: ${window.location.href}\nUA: ${navigator.userAgent}\nTime: ${new Date().toISOString()}`
    navigator.clipboard.writeText(text).catch(() => {
      // fallback: select text for manual copy
    })
  }

  handleDismiss = () => {
    clearErrorLog()
    this.setState({ hasError: false, error: null, errorInfo: null })
  }

  handleReload = () => {
    window.location.reload()
  }

  render() {
    if (!this.state.hasError) {
      return this.props.children
    }

    // On desktop, re-throw so the error propagates to dev tools
    if (!isMobileOrTablet()) {
      throw this.state.error
    }

    const { error, errorInfo } = this.state
    const previousErrors = getRecentErrors()

    return (
      <div style={overlayStyle}>
        <div style={{ ...headerStyle, display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
          <div>
            <h2 style={{ margin: 0, color: '#e74c3c', fontSize: '18px' }}>
              JavaScript Error
            </h2>
            <p style={{ margin: '4px 0 0', color: '#888', fontSize: '11px' }}>
              {new Date().toISOString()} &mdash; {window.location.pathname}
            </p>
          </div>
          <button style={{ ...buttonStyle, marginLeft: '12px', flexShrink: 0, color: '#aaa' }} onClick={this.handleDismiss}>
            ✕ Dismiss
          </button>
        </div>

        <div style={{ marginBottom: '16px' }}>
          <strong style={{ color: '#ff6b6b' }}>{error?.message || 'Unknown error'}</strong>
        </div>

        {error?.stack && (
          <pre style={{
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            backgroundColor: '#0d0d1a',
            padding: '12px',
            borderRadius: '4px',
            maxHeight: '200px',
            overflow: 'auto',
            fontSize: '11px',
            lineHeight: '1.4',
            marginBottom: '12px',
          }}>
            {error.stack}
          </pre>
        )}

        {errorInfo?.componentStack && (
          <details style={{ marginBottom: '12px' }}>
            <summary style={{ cursor: 'pointer', color: '#888' }}>Component Stack</summary>
            <pre style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              backgroundColor: '#0d0d1a',
              padding: '12px',
              borderRadius: '4px',
              maxHeight: '150px',
              overflow: 'auto',
              fontSize: '11px',
              lineHeight: '1.4',
              marginTop: '4px',
            }}>
              {errorInfo.componentStack}
            </pre>
          </details>
        )}

        {previousErrors.length > 0 && (
          <details style={{ marginBottom: '12px' }}>
            <summary style={{ cursor: 'pointer', color: '#888' }}>
              Previous errors ({previousErrors.length})
            </summary>
            <pre style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              backgroundColor: '#0d0d1a',
              padding: '12px',
              borderRadius: '4px',
              maxHeight: '150px',
              overflow: 'auto',
              fontSize: '11px',
              lineHeight: '1.4',
              marginTop: '4px',
            }}>
              {previousErrors.map(e => `[${e.timestamp}] ${e.message}`).join('\n')}
            </pre>
          </details>
        )}

        <div>
          <button style={buttonStyle} onClick={this.handleCopy}>
            Copy Error
          </button>
          <button style={{ ...buttonStyle, backgroundColor: '#e74c3c', borderColor: '#e74c3c' }} onClick={this.handleReload}>
            Reload Page
          </button>
        </div>
      </div>
    )
  }
}
