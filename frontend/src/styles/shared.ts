import { SxProps } from '@mui/system'
export const COLORS = {
  blue: '#509dfd',
  pink: '#f087e2',
  teal: '#4fe3af',
  red: '#fe6b8b',
  orange: '#ffa67a',
  light: '#ffffff',
  dark: '#222222',
  background: '#FAFBFF',
}

export const DemoFields = (): any => {
    return {
        fontSize: '3rem',
        flexBasis: '100%',
        p: 2,
        borderRadius: 3,
        m: 1,
        mb: 2,
        backgroundColor: 'white',
    }
}

export const GradientText = (): any => {
    return {
        backgroundColor: '#f087e2',
        backgroundImage: 'linear-gradient(90deg, rgba(152,68,183,1), rgba(59,173,227,1))',
        backgroundSize: '100%',
        backgroundRepeat: 'repeat',
        backgroundClip: 'text',
        WebkitBackgroundClip: 'text',
        WebkitTextFillColor: 'transparent',
        MozBackgroundClip: 'text',
        MozTextFillColor: 'transparent',
    }
}

export const BlueIconWrapper = (): any => {
    return {
        display: 'flex',
        flexWrap: 'wrap',
        flexDirection: 'row',
        justifyContent: 'flex-start',
        alignItems: 'left',
        p: 1,
    }
}

export const BlueIcon = (): any => {
    return {
        color: 'primary.main',
        borderRadius: '1rem',
        borderColor: 'primary.main',
        border: 'solid .1rem',
        fontSize: '3rem',
        padding: '1.6rem 2.4rem',
        marginRight: '2rem',
        minWidth: '60px',
        alignItems: 'baseline',
    }
}

export const SocialMediaPost = (): any => {
    return {
        textTransform: 'none',
        backgroundColor: 'white',
        border: '1px solid',
        borderColor: 'primary.main',
        borderRadius: 3,
        mb: 3,
        p: 1,
    }
}

export const NavigationLinks = (): any => {
    return {
        mb: 0,
        mr: 1,
        pr: 1,
        color: '#ffffff',
        flexBasis: 'auto',
        justifyContent: 'center',
        alignItems: 'center',
        '&:hover': {
            color: 'secondary.main',
        },
        '&:not(:last-child)::after': {
            top: 0,
            right: 0,
            position: 'absolute',
            display: 'block',
            height: '85%',
            width: '.15rem',
        },
        'a': {
            color: '#ffffff',
            fontSize: '1.2rem',
            borderRight: '1px solid #ffffff',
        },
        'a:last-child': {
            borderRight: 'none',
        },
        'a:hover': {
            color: COLORS.teal,
        },
    }
}

// Layout

export const FranchiseWrapper = (): any => {
    return {
        width: '100%',
        display: 'flex',
        flexDirection: 'column',
    }
}

export const SectionWrapper = (): any => {
    return {
        position: 'relative',
        maxWidth: '72vw',
        width: '100%',
        mx: 'auto',
        overflow: 'hidden',
        '@media (min-width: 768px)': {
            maxWidth: '85vw',
        },
        '@media (min-width: 1440px)': {
            maxWidth: '80vw',
        },
    }
}

export const XlPadding = (): any => {
    return {
        pt: 3,
        pr: 2,
        pb: 3,
        pl: 2,
        '@media (min-width: 768px)': {
            pt: 4,
            pb: 4,
        },
        '@media (min-width: 1440px)': {
            pt: 6,
            pb: 6,
        },
    }
}

// Text

export const TitleExtraLarge = (): any => {
    return {
        fontSize: '3rem',
        letterSpacing: '-0.2rem',
        marginTop: '3rem',
        marginBottom: '3rem',
        lineHeight: '100%',
        '@media (min-width: 786px)': {
            fontSize: '4rem',
        },
        '@media (min-width: 1440px)': {
            fontSize: '5rem',
        },
    }
}

export const TextLarge = (): any => {
    return {
        fontSize: '3rem',
        marginBottom: '2.7rem',
        lineHeight: '140%',
        textTransform: 'none',
        '@media (min-width: 786px)': {
            fontSize: '4rem',
        },
        '@media (min-width: 1440px)': {
            fontSize: '2.4rem',
        },
    }
}

export const TextMedium = (): any => {
    return {
        fontSize: '2.2rem',
        marginBottom: '1.8rem',
        lineHeight: '140%',
        textTransform: 'none',
        '@media (min-width: 786px)': {
            fontSize: '3rem',
        },
        '@media (min-width: 1440px)': {
            fontSize: '1.8rem',
        },
    }
}

export const TextSmall = (): any => {
    return {
        fontSize: '1.8rem',
        marginBottom: '1.62rem',
        lineHeight: '140%',
        textTransform: 'none',
        '@media (min-width: 786px)': {
            fontSize: '2rem',
        },
        '@media (min-width: 1440px)': {
            fontSize: '1.5rem',
        },
    }
}

export const TextExtraSmall = (): any => {
    return {
        fontSize: '1.4rem',
        marginBottom: '1.62rem',
        lineHeight: '140%',
        textTransform: 'none',
        '@media (min-width: 786px)': {
            fontSize: '1rem',
        },
    }
}

// Footer

export const FooterLink = (): any => {
    return {
        display: 'block',
        color: 'primary.main',
        transition: 'color 0.3s ease-in-out',
        '&:hover': {
            color: COLORS.teal,
        },
    }
}

export const mergeSX = (props: SxProps[]): SxProps => {
  return props.reduce((acc, curr) => {
    return Object.assign({}, acc, curr)
  }, {})
}