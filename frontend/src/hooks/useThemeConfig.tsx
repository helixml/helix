import {
  ITheme,
  getThemeName,
  THEMES,
} from '../themes'

export const useTheme = (): ITheme => {
  const themeName = getThemeName()
  return THEMES[themeName] || THEMES.aria
}

export default useTheme