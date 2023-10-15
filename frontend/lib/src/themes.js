"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.getThemeName = exports.THEME_DOMAINS = exports.THEMES = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const Box_1 = __importDefault(require("@mui/material/Box"));
const Typography_1 = __importDefault(require("@mui/material/Typography"));
const DEFAULT_THEME_NAME = 'lilypad';
exports.THEMES = {
    lilypad: {
        company: 'Lilypad',
        url: 'https://lilypad.tech/',
        primary: '#64BEA9',
        secondary: '#8DA4BB',
        activeSections: [],
        logo: () => ((0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ sx: {
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'center',
            } }, { children: [(0, jsx_runtime_1.jsx)(Box_1.default, { component: "img", src: "/img/logo.png", alt: "Lilypad", sx: {
                        height: 40,
                    } }), (0, jsx_runtime_1.jsx)(Typography_1.default, Object.assign({ variant: "h6", sx: {
                        ml: 1,
                    } }, { children: "Lilypad" }))] }))),
    },
};
exports.THEME_DOMAINS = {
    'lilypad.tech': 'lilypad',
};
const getThemeName = () => {
    if (typeof document !== "undefined") {
        const params = new URLSearchParams(new URL(document.URL).search);
        const queryValue = params.get('theme');
        if (queryValue) {
            localStorage.setItem('theme', queryValue);
        }
    }
    const localStorageValue = localStorage.getItem('theme');
    if (localStorageValue) {
        if (exports.THEMES[localStorageValue]) {
            return localStorageValue;
        }
        else {
            localStorage.removeItem('theme');
            return DEFAULT_THEME_NAME;
        }
    }
    if (typeof document !== "undefined") {
        const domainName = exports.THEME_DOMAINS[document.location.hostname];
        if (domainName && exports.THEMES[domainName])
            return exports.THEME_DOMAINS[domainName];
        return DEFAULT_THEME_NAME;
    }
    return DEFAULT_THEME_NAME;
};
exports.getThemeName = getThemeName;
//# sourceMappingURL=themes.js.map