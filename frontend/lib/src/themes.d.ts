import { ReactElement } from 'react';
export interface ITheme {
    company: string;
    url: string;
    primary: string;
    secondary: string;
    activeSections: string[];
    logo: {
        (): ReactElement;
    };
}
export declare const THEMES: Record<string, ITheme>;
export declare const THEME_DOMAINS: Record<string, string>;
export declare const getThemeName: () => string;
//# sourceMappingURL=themes.d.ts.map