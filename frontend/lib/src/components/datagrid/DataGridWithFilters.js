"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Container_1 = __importDefault(require("@mui/material/Container"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const DataGridWithFilters = ({ autoScroll = false, filterWidth = 300, filters, datagrid, pagination, }) => {
    return ((0, jsx_runtime_1.jsxs)(Container_1.default, Object.assign({ disableGutters: true, maxWidth: "xl", sx: {
            height: '100%',
            display: 'flex',
            flexDirection: 'row',
            px: 1,
            pt: 1,
        } }, { children: [(0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ className: "data", sx: {
                    flexGrow: 1,
                    height: '100%',
                    flexBasis: `calc(100% - ${filterWidth}px)`,
                    display: 'flex',
                    flexDirection: 'column',
                    pr: 1,
                    mr: 1,
                } }, { children: [(0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ className: "grid", sx: Object.assign({ display: 'flex' }, (autoScroll
                            ? {
                                height: '1px',
                                flexGrow: 1,
                                overflowY: 'auto',
                                mb: 1,
                            }
                            : {
                                flexGrow: 1,
                                mb: 1,
                            })) }, { children: datagrid })), pagination && ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ className: "pagination", sx: {
                            flexGrow: 0,
                            mt: 1,
                            mb: 1,
                        } }, { children: pagination })))] })), filters && ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ className: "filters", sx: {
                    flexGrow: 0,
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'flex-start',
                    alignItems: 'center',
                    width: `${filterWidth}px`,
                    maxWidth: `${filterWidth}px`,
                    minWidth: `${filterWidth}px`,
                    height: '100%',
                } }, { children: filters })))] })));
};
exports.default = DataGridWithFilters;
//# sourceMappingURL=DataGridWithFilters.js.map