import { FC, ReactElement } from 'react'
import Box from '@mui/material/Box'
import Tooltip, { TooltipProps } from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import { Bot, Sparkles } from 'lucide-react'

import OpenAILogo from '../providers/logos/openai'
import gooseMark from '../../assets/harness/goose.svg?no-inline'
import qwenCodeMark from '../../assets/harness/qwen-code.svg?no-inline'
import zedAgentMark from '../../assets/harness/zed-agent.svg'

export type AgentHarnessVariant = 'long' | 'short'

type HarnessMeta = {
  label: string
  color: string
}

const harnessMeta: Record<string, HarnessMeta> = {
  claude_code: { label: 'Claude Code', color: '#d97757' },
  codex_cli: { label: 'Codex', color: '' },
  gemini_cli: { label: 'Gemini CLI', color: '#4285f4' },
  qwen_code: { label: 'Qwen Code', color: '#6d44e8' },
  goose_code: { label: 'Goose', color: '' },
  zed_agent: { label: 'Zed Agent', color: '' },
}

export const getAgentHarnessLabel = (runtime?: string): string => {
  if (!runtime) return 'Zed Agent'
  return harnessMeta[runtime]?.label
    ?? runtime.split('_').map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

type AgentHarnessSource = {
  config?: {
    helix?: {
      assistants?: Array<{
        code_agent_runtime?: string
        code_agent_credential_type?: string
        claude_subscription_model?: string
        model?: string
        generation_model?: string
      }>
    }
  }
}

export const getAgentHarnessRuntime = (app?: AgentHarnessSource): string =>
  app?.config?.helix?.assistants?.[0]?.code_agent_runtime || 'zed_agent'

export const getAgentHarnessModel = (app?: AgentHarnessSource): string => {
  const assistant = app?.config?.helix?.assistants?.[0]
  if (!assistant) return ''
  if (assistant.code_agent_runtime === 'claude_code'
    && assistant.code_agent_credential_type === 'subscription'
    && assistant.claude_subscription_model) {
    return assistant.claude_subscription_model
  }
  return assistant.model || assistant.generation_model || ''
}

const ClaudeMark: FC<{ size: number; color: string }> = ({ size, color }) => (
  <svg width={size} height={size} viewBox="0 0 256 257" fill="none" aria-hidden="true">
    <path
      fill={color}
      d="m50.228 170.321 50.357-28.257.843-2.463-.843-1.361h-2.462l-8.426-.518-28.775-.778-24.952-1.037-24.175-1.296-6.092-1.297L0 125.796l.583-3.759 5.12-3.434 7.324.648 16.202 1.101 24.304 1.685 17.629 1.037 26.118 2.722h4.148l.583-1.685-1.426-1.037-1.101-1.037-25.147-17.045-27.22-18.017-14.258-10.37-7.713-5.25-3.888-4.925-1.685-10.758 7-7.713 9.397.649 2.398.648 9.527 7.323 20.35 15.75L94.817 91.9l3.889 3.24 1.555-1.102.195-.777-1.75-2.917-14.453-26.118-15.425-26.572-6.87-11.018-1.814-6.61c-.648-2.723-1.102-4.991-1.102-7.778l7.972-10.823L71.42 0 82.05 1.426l4.472 3.888 6.61 15.101 10.694 23.786 16.591 32.34 4.861 9.592 2.592 8.879.973 2.722h1.685v-1.556l1.36-18.211 2.528-22.36 2.463-28.776.843-8.1 4.018-9.722 7.971-5.25 6.222 2.981 5.12 7.324-.713 4.73-3.046 19.768-5.962 30.98-3.889 20.739h2.268l2.593-2.593 10.499-13.934 17.628-22.036 7.778-8.749 9.073-9.657 5.833-4.601h11.018l8.1 12.055-3.628 12.443-11.342 14.388-9.398 12.184-13.48 18.147-8.426 14.518.778 1.166 2.01-.194 30.46-6.481 16.462-2.982 19.637-3.37 8.88 4.148.971 4.213-3.5 8.62-20.998 5.184-24.628 4.926-36.682 8.685-.454.324.519.648 16.526 1.555 7.065.389h17.304l32.21 2.398 8.426 5.574 5.055 6.805-.843 5.184-12.962 6.611-17.498-4.148-40.83-9.721-14-3.5h-1.944v1.167l11.666 11.406 21.387 19.314 26.767 24.887 1.36 6.157-3.434 4.86-3.63-.518-23.526-17.693-9.073-7.972-20.545-17.304h-1.36v1.814l4.73 6.935 25.017 37.59 1.296 11.536-1.814 3.76-6.481 2.268-7.13-1.297-14.647-20.544-15.1-23.138-12.185-20.739-1.49.843-7.194 77.448-3.37 3.953-7.778 2.981-6.48-4.925-3.436-7.972 3.435-15.749 4.148-20.544 3.37-16.333 3.046-20.285 1.815-6.74-.13-.454-1.49.194-15.295 20.999-23.267 31.433-18.406 19.702-4.407 1.75-7.648-3.954.713-7.064 4.277-6.286 25.47-32.405 15.36-20.092 9.917-11.6-.065-1.686h-.583L44.07 198.125l-12.055 1.555-5.185-4.86.648-7.972 2.463-2.593 20.35-13.999-.064.065Z"
    />
  </svg>
)

const MaskMark: FC<{ runtime: string; src: string; size: number; color: string }> = ({ runtime, src, size, color }) => (
  <Box
    component="span"
    aria-hidden="true"
    data-harness-mark={runtime}
    sx={{
      width: size,
      height: size,
      display: 'inline-block',
      bgcolor: color,
      maskImage: `url("${src}")`,
      maskMode: 'alpha',
      maskPosition: 'center',
      maskRepeat: 'no-repeat',
      maskSize: 'contain',
      WebkitMaskImage: `url("${src}")`,
      WebkitMaskPosition: 'center',
      WebkitMaskRepeat: 'no-repeat',
      WebkitMaskSize: 'contain',
    }}
  />
)

const HarnessMark: FC<{ runtime: string; size: number; color: string }> = ({ runtime, size, color }) => {
  if (runtime === 'claude_code') return <ClaudeMark size={size} color={color} />
  if (runtime === 'codex_cli') {
    return <OpenAILogo width={size} height={size} style={{ color }} aria-hidden="true" />
  }
  if (runtime === 'gemini_cli') return <Sparkles size={size} color={color} aria-hidden="true" />
  if (runtime === 'qwen_code') return <MaskMark runtime={runtime} src={qwenCodeMark} size={size} color={color} />
  if (runtime === 'goose_code') return <MaskMark runtime={runtime} src={gooseMark} size={size} color={color} />
  if (runtime === 'zed_agent') return <MaskMark runtime={runtime} src={zedAgentMark} size={size} color={color} />
  return <Bot size={size} color={color} aria-hidden="true" />
}

export const AgentHarness: FC<{
  runtime?: string
  variant?: AgentHarnessVariant
  size?: number
  showTooltip?: boolean
  tooltipPlacement?: TooltipProps['placement']
}> = ({
  runtime = 'zed_agent',
  variant = 'long',
  size = 16,
  showTooltip = true,
  tooltipPlacement = 'bottom',
}) => {
  const theme = useTheme()
  const meta = harnessMeta[runtime] ?? { label: getAgentHarnessLabel(runtime), color: '#94a3b8' }
  const markColor = meta.color || theme.palette.text.primary
  const content: ReactElement = (
    <Box
      component="span"
      role={variant === 'short' ? 'img' : undefined}
      aria-label={variant === 'short' ? meta.label : undefined}
      sx={{ display: 'inline-flex', alignItems: 'center', gap: 1, minWidth: 0 }}
    >
      <Box component="span" sx={{ display: 'inline-flex', flexShrink: 0 }}>
        <HarnessMark runtime={runtime} size={size} color={markColor} />
      </Box>
      {variant === 'long' && (
        <Typography component="span" variant="body2" color="text.secondary" noWrap>
          {meta.label}
        </Typography>
      )}
    </Box>
  )

  return variant === 'short' && showTooltip
    ? <Tooltip title={meta.label} placement={tooltipPlacement} disableInteractive>{content}</Tooltip>
    : content
}

export default AgentHarness
