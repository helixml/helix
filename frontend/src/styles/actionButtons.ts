/**
 * Shared style for inline action buttons in detail panels (spec task details,
 * share preview, tool pickers).
 *
 * The convention is one thing everywhere: a text button — no outline, no fill —
 * with a 16px Lucide icon on the left and sentence-case text. Semantic intent is
 * carried by the MUI `color` prop (`warning`, `error`), never by the variant, so
 * a destructive action reads as destructive without becoming a filled block that
 * outweighs everything around it.
 *
 * Import these rather than hand-rolling `sx` per button — that is how icon size,
 * hit area, and spacing drift apart between surfaces.
 */
export const ACTION_BUTTON_SX = {
  textTransform: 'none',
  fontSize: '0.75rem',
  fontWeight: 500,
  minHeight: 30,
  px: 1,
  // Pin the icon box in CSS rather than trusting every call site to pass the
  // same `size` prop — that is exactly how icon sizes drift apart. CSS beats
  // Lucide's width/height presentation attributes, so this wins regardless.
  '& .MuiButton-startIcon': {
    marginLeft: 0,
    marginRight: 0.75,
    '& svg': { width: 16, height: 16, strokeWidth: 1.75 },
  },
} as const

/**
 * Non-semantic actions (view, share, edit): quiet until hovered, so the
 * coloured warning/error actions stay the only things that draw the eye.
 */
export const NEUTRAL_ACTION_BUTTON_SX = {
  ...ACTION_BUTTON_SX,
  color: 'text.secondary',
  '&:hover': { color: 'text.primary' },
} as const
