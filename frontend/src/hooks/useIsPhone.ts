import { useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'

/**
 * True on a phone-width viewport.
 *
 * Distinct from `useIsBigScreen`, which switches at `lg` (1200px) and governs
 * whether the nav drawer is permanent or temporary. A 1000px-wide laptop window
 * gets the temporary drawer but is not a phone: it has a pointer, room for a
 * 300px sidebar, and working hover. This hook is the narrower question — is
 * there no hover and no horizontal room — and answers it at `sm` (600px).
 */
const useIsPhone = (): boolean => {
  const theme = useTheme()
  return useMediaQuery(theme.breakpoints.down('sm'))
}

export default useIsPhone
