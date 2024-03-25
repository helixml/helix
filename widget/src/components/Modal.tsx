import { FC } from 'react'
import { styled } from 'goober'

export interface ModalTheme {
  backdropColor?: string,
  fontFamily?: string,
  modalWidth?: string,
  modalBorderRadius?: string,
  modalShadow?: string,
  modalBgColor?: string,
  headerTextColor?: string,
  titleFontSize?: string,
  contentPadding?: string,
  footerPadding?: string,
}

type ModalThemeRequired = Required<ModalTheme>

export const DEFAULT_THEME: ModalThemeRequired = {
  backdropColor: 'rgba(0, 0, 0, 0.5)',
  fontFamily: 'Arial',
  modalWidth: '600px',
  modalBorderRadius: '8px',
  modalShadow: '0px 3px 15px rgba(0,0,0,0.2)',
  modalBgColor: '#383838',
  headerTextColor: 'white',
  titleFontSize: '1.25rem',
  contentPadding: '20px',
  footerPadding: '20px',
}

const Backdrop = styled('div')<{
  backdropColor: string
}>((props) => `
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: ${props.backdropColor};
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
`)

const ModalWrapper = styled('div')<{
  modalWidth: string,
  modalShadow: string,
  modalBorderRadius: string,
  modalBgColor: string,
}>((props) => `
  width: ${props.modalWidth};
  border-radius: ${props.modalBorderRadius};
  box-shadow: ${props.modalShadow};
  background: ${props.modalBgColor};
  z-index: 1001;
`)

const ModalHeader = styled('div')<{
  headerTextColor: string,
  modalBorderRadius: string,
}>((props) => `
  padding: 16px;
  color: ${props.headerTextColor};
  border-top-left-radius: ${props.modalBorderRadius};
  border-top-right-radius: ${props.modalBorderRadius};
  display: flex;
  justify-content: space-between;
  align-items: center;
`)

const ModalTitle = styled('h2')<{
  titleFontSize: string
}>((props) => `
  margin: 0;
  font-size: ${props.titleFontSize};
`)

const CloseButton = styled('button')<{
  headerTextColor: string,
}>((props) => `
  background: none;
  border: none;
  color: ${props.headerTextColor};
  cursor: pointer;
  font-size: 1.25rem;
`)

const ModalContent = styled('div')<{
  contentPadding: string,
}>((props) => `
  padding: ${props.contentPadding};
  boxShadow: 24;
`)

const ModalFooter = styled('div')<{
  footerPadding: string,
}>((props) => `
  padding: ${props.footerPadding};
`)

const SimpleModal: FC<{
  open?: boolean,
  title?: string,
  onClose?: () => void,
} & ModalTheme> = ({
  open = false,
  title = 'Default Title',
  onClose = () => {},
  ...theme
}) => {
  const useTheme = {
    ...DEFAULT_THEME,
    ...theme,
  }
  if (!open) return null
  return (
    <Backdrop backdropColor={useTheme.backdropColor} onClick={ onClose }>
      <ModalWrapper
        modalWidth={useTheme.modalWidth}
        modalShadow={useTheme.modalShadow}
        modalBorderRadius={useTheme.modalBorderRadius}
        modalBgColor={useTheme.modalBgColor}
        onClick={(e) => e.stopPropagation()}
      >
        <ModalHeader
          headerTextColor={useTheme.headerTextColor}
          modalBorderRadius={useTheme.modalBorderRadius}
        >
          <ModalTitle
            titleFontSize={useTheme.titleFontSize}
          >
            {title}
          </ModalTitle>
          <CloseButton
            headerTextColor={useTheme.headerTextColor}
            onClick={onClose}
          >
            X
          </CloseButton>
        </ModalHeader>
        <ModalContent contentPadding={useTheme.contentPadding}>
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
          HERE CONTENT<br />
        </ModalContent>
        <ModalFooter footerPadding={useTheme.footerPadding}>
          FOOTER
        </ModalFooter>
      </ModalWrapper>
    </Backdrop>
  )
}

export default SimpleModal
