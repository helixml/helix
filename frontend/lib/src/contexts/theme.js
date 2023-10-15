"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.ThemeProviderWrapper = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const styles_1 = require("@mui/material/styles");
const useThemeConfig_1 = __importDefault(require("../hooks/useThemeConfig"));
const ThemeProviderWrapper = ({ children }) => {
    const themeConfig = (0, useThemeConfig_1.default)();
    const theme = (0, react_1.useMemo)(() => {
        return (0, styles_1.createTheme)({
            palette: {
                primary: {
                    main: themeConfig.primary,
                },
                secondary: {
                    main: themeConfig.secondary,
                }
            }
        });
    }, [
        themeConfig,
    ]);
    return ((0, jsx_runtime_1.jsx)(styles_1.ThemeProvider, Object.assign({ theme: theme }, { children: children })));
};
exports.ThemeProviderWrapper = ThemeProviderWrapper;
//# sourceMappingURL=theme.js.map