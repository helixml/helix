"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useTheme = void 0;
const themes_1 = require("../themes");
const useTheme = () => {
    const themeName = (0, themes_1.getThemeName)();
    return themes_1.THEMES[themeName] || themes_1.THEMES.aria;
};
exports.useTheme = useTheme;
exports.default = exports.useTheme;
//# sourceMappingURL=useThemeConfig.js.map