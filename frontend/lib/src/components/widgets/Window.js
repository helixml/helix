"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const Dialog_1 = __importDefault(require("@mui/material/Dialog"));
const DialogContent_1 = __importDefault(require("@mui/material/DialogContent"));
const DialogTitle_1 = __importDefault(require("@mui/material/DialogTitle"));
const DialogActions_1 = __importDefault(require("@mui/material/DialogActions"));
const Button_1 = __importDefault(require("@mui/material/Button"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const Window = ({ leftButtons, rightButtons, buttons, withCancel, loading = false, submitTitle = 'Save', cancelTitle = 'Cancel', background = '#fff', open, title, size = 'md', children, compact = false, noScroll = false, fullHeight = false, noActions = false, onCancel, onSubmit, disabled = false, }) => {
    const closeWindow = (0, react_1.useCallback)(() => {
        onCancel && onCancel();
    }, [
        onCancel,
    ]);
    return ((0, jsx_runtime_1.jsxs)(Dialog_1.default, Object.assign({ open: open, onClose: closeWindow, fullWidth: true, maxWidth: size, sx: {
            '& .MuiDialog-paper': Object.assign(Object.assign({ backgroundColor: background }, (fullHeight && {
                height: '100%',
            })), (noScroll && {
                overflowX: 'hidden!important',
                overflowY: 'hidden!important',
            })),
        } }, { children: [title && ((0, jsx_runtime_1.jsx)(DialogTitle_1.default, Object.assign({ sx: {
                    padding: 1,
                } }, { children: title }))), (0, jsx_runtime_1.jsx)(DialogContent_1.default, Object.assign({ sx: Object.assign(Object.assign({}, (compact && {
                    padding: '0px!important',
                })), (noScroll && {
                    overflowX: 'hidden!important',
                    overflowY: 'hidden!important',
                })) }, { children: children })), !noActions && ((0, jsx_runtime_1.jsx)(DialogActions_1.default, { children: (0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ component: "div", sx: {
                        width: '100%',
                        display: 'flex',
                        flexDirection: 'row',
                    } }, { children: [(0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                                flexGrow: 0,
                            } }, { children: leftButtons })), (0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ component: "div", sx: {
                                flexGrow: 1,
                                textAlign: 'right',
                            } }, { children: [withCancel && ((0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ sx: {
                                        marginLeft: 2,
                                    }, type: "button", variant: "outlined", onClick: closeWindow }, { children: cancelTitle }))), onSubmit && ((0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ sx: {
                                        marginLeft: 2,
                                    }, type: "button", variant: "contained", color: "primary", disabled: disabled || loading ? true : false, onClick: onSubmit }, { children: submitTitle }))), rightButtons || buttons] }))] })) }))] })));
};
exports.default = Window;
//# sourceMappingURL=Window.js.map