import { useTheme, Breakpoint } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'

const useIsBigScreen = ({
  breakpoint = 'lg',
}: {
  breakpoint?: number | Breakpoint,
} = {}) => {
  const theme = useTheme()
  const bigScreen = !useMediaQuery(theme.breakpoints.down(breakpoint))
  return bigScreen
}

export default useIsBigScreen