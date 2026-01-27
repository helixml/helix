import { Theme as MuiTheme } from '@mui/material/styles';

declare module '@mui/material/styles' {
  interface Theme extends MuiTheme {
    chartGradientStart: string;
    chartGradientEnd: string;
    chartGradientStartOpacity: number;
    chartGradientEndOpacity: number;
    chartHighlightGradientStart: string;
    chartHighlightGradientEnd: string;
    chartHighlightGradientStartOpacity: number;
    chartHighlightGradientEndOpacity: number;
    chartActionGradientStart: string;
    chartActionGradientEnd: string;
    chartActionGradientStartOpacity: number;
    chartActionGradientEndOpacity: number;
    chartErrorGradientStart: string;
    chartErrorGradientEnd: string;
    chartErrorGradientStartOpacity: number;
    chartErrorGradientEndOpacity: number;
  }
  interface ThemeOptions {
    chartGradientStart?: string;
    chartGradientEnd?: string;
    chartGradientStartOpacity?: number;
    chartGradientEndOpacity?: number;
    chartHighlightGradientStart?: string;
    chartHighlightGradientEnd?: string;
    chartHighlightGradientStartOpacity?: number;
    chartHighlightGradientEndOpacity?: number;
    chartActionGradientStart?: string;
    chartActionGradientEnd?: string;
    chartActionGradientStartOpacity?: number;
    chartActionGradientEndOpacity?: number;
    chartErrorGradientStart?: string;
    chartErrorGradientEnd?: string;
    chartErrorGradientStartOpacity?: number;
    chartErrorGradientEndOpacity?: number;
  }
  interface TypographyVariants {
    fontFamilyMono: string;
  }
  interface TypographyVariantsOptions {
    fontFamilyMono?: string;
  }
} 