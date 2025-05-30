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
  }
} 