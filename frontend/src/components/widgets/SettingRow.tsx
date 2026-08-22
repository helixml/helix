import { FC, ReactNode } from 'react'
import { Box, Typography } from '@mui/material'
import { SystemCssProperties, Theme } from '@mui/system'

interface SettingRowProps {
  /** What the setting is called. Replaces the control's own floating label. */
  title: string
  /** What the setting does, or why it is unavailable. */
  description?: ReactNode
  /** Marks the value as required with a trailing asterisk. */
  required?: boolean
  /**
   * Top-align the label against a tall control (a multiline field). Single-line
   * controls should stay centred so a column of rows reads as one list.
   */
  align?: 'center' | 'start'
  /** Width of the control column. */
  controlWidth?: SystemCssProperties<Theme>['width']
  /** The control, plus any adornment (a settings shortcut, a unit suffix). */
  children: ReactNode
}

/**
 * One setting: name and explanation on the left, control on the right.
 *
 * Settings pages used to stack full-width MUI fields whose floating labels sat
 * inside the input, which made a long form read as an undifferentiated column
 * of boxes — the label and its value competed for the same spot and neither
 * scanned. Splitting the two puts every name in one column and every value in
 * another, so a page of settings can be read down either side.
 *
 * Long-form prose (project guidelines, a startup script) is the exception and
 * keeps its own full-width editor — there is nothing to align it against.
 */
const SettingRow: FC<SettingRowProps> = ({
  title,
  description,
  required = false,
  align = 'center',
  controlWidth = { xs: 220, sm: 320 },
  children,
}) => (
  <Box
    sx={{
      display: 'flex',
      alignItems: align === 'start' ? 'flex-start' : 'center',
      justifyContent: 'space-between',
      gap: 2,
    }}
  >
    <Box sx={{ flex: 1, minWidth: 0, pt: align === 'start' ? 0.75 : 0 }}>
      <Typography variant="body2" sx={{ fontWeight: 600 }}>
        {title}
        {required && (
          <Box component="span" sx={{ color: 'error.main', ml: 0.25 }}>
            *
          </Box>
        )}
      </Typography>
      {description && (
        <Typography variant="caption" color="text.secondary">
          {description}
        </Typography>
      )}
    </Box>
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 0.25,
        width: controlWidth,
        flexShrink: 0,
      }}
    >
      {children}
    </Box>
  </Box>
)

export default SettingRow
