import { FC } from 'react'
import { styled } from 'goober'

export interface SearchBoxTheme {
  borderColor?: string,
  hoverBorderColor?: string,
  borderRadius?: string,
  iconPadding?: string,
  iconColor?: string,
  textPadding?: string,
  textSize?: string,
  fontFamily?: string,
}

type SearchBoxThemeRequired = Required<SearchBoxTheme>

export const DEFAULT_THEME: SearchBoxThemeRequired = {
  borderColor: 'rgba(255, 255, 255, 0.23)',
  hoverBorderColor: 'rgba(255, 255, 255, 1)',
  borderRadius: '5px',
  iconPadding: '10px',
  iconColor: 'rgba(255, 255, 255, 1)',
  textPadding: '20px',
  textSize: '20pt',
  fontFamily: 'Arial',
}

const InputContainer = styled('div')<{
  borderColor: string,
  hoverBorderColor: string,
  borderRadius: string,
  iconPadding: string,
}>((props) => `
  display: flex;
  align-items: center;
  border: 1px solid ${props.borderColor};
  border-radius: ${props.borderRadius};
  padding-right: ${props.iconPadding};
  &:hover {
    border-color: ${props.hoverBorderColor};
  }
`)

const SearchInput = styled('input')<{
  textPadding: string,
  textSize: string,
  fontFamily: string,
}>((props) => `
  flex-grow: 1;
  border: none;
  outline: none;
  padding: ${props.textPadding};
  background-color: transparent;
  color: white;
  font-family: ${props.fontFamily};
  font-size: ${props.textSize};
`)

const SearchIcon = styled('div')<{
  iconColor: string,
}>((props) => `
  background: none;
  border: none;
  cursor: pointer;
  color: ${props.iconColor};
`)

const RightArrowAdornment = () => (
  <svg
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
  >
    <path
      d="M4 12H20"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
    />
    <path
      d="M14 6L20 12L14 18"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
)

const SearchBar: FC<{
  onClick: () => void,
} & SearchBoxTheme> = ({
  onClick,
  ...theme
}) => {
  const useTheme = {
    ...DEFAULT_THEME,
    ...theme,
  }
  return (
    <InputContainer
      borderColor={useTheme.borderColor}
      hoverBorderColor={useTheme.hoverBorderColor}
      borderRadius={useTheme.borderRadius}
      iconPadding={useTheme.iconPadding}
      onClick={ onClick }
    >
      <SearchInput
        type="text"
        placeholder="Ask a question..."
        textPadding={useTheme.textPadding}
        textSize={useTheme.textSize}
        fontFamily={useTheme.fontFamily}
      />
      <SearchIcon
        iconColor={useTheme.iconColor}
      >
        <RightArrowAdornment />
      </SearchIcon>
    </InputContainer>
  )
}

export default SearchBar
