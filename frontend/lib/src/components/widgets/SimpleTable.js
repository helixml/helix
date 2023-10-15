"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Box_1 = __importDefault(require("@mui/material/Box"));
const Table_1 = __importDefault(require("@mui/material/Table"));
const TableBody_1 = __importDefault(require("@mui/material/TableBody"));
const TableCell_1 = __importDefault(require("@mui/material/TableCell"));
const TableHead_1 = __importDefault(require("@mui/material/TableHead"));
const TableRow_1 = __importDefault(require("@mui/material/TableRow"));
const TableContainer_1 = __importDefault(require("@mui/material/TableContainer"));
const Paper_1 = __importDefault(require("@mui/material/Paper"));
const SimpleTable = ({ fields, data, compact = false, withContainer = false, hideHeader = false, hideHeaderIfEmpty = false, actionsTitle = 'Actions', actionsFieldClassname, onRowClick, getActions, }) => {
    const table = ((0, jsx_runtime_1.jsxs)(Table_1.default, Object.assign({ size: compact ? 'small' : 'medium' }, { children: [(!hideHeader && (!hideHeaderIfEmpty || data.length > 0)) && ((0, jsx_runtime_1.jsx)(TableHead_1.default, { children: (0, jsx_runtime_1.jsxs)(TableRow_1.default, { children: [fields.map((field, i) => {
                            return ((0, jsx_runtime_1.jsx)(TableCell_1.default, Object.assign({ align: field.numeric ? 'right' : 'left' }, { children: field.title }), i));
                        }), getActions ? ((0, jsx_runtime_1.jsx)(TableCell_1.default, Object.assign({ align: 'right' }, { children: actionsTitle }))) : null] }) })), (0, jsx_runtime_1.jsx)(TableBody_1.default, { children: data.map((dataRow, i) => {
                    return ((0, jsx_runtime_1.jsxs)(TableRow_1.default, Object.assign({ hover: true, onClick: e => {
                            if (!onRowClick)
                                return;
                            onRowClick(dataRow);
                        }, tabIndex: -1 }, { children: [fields.map((field, i) => {
                                return ((0, jsx_runtime_1.jsx)(TableCell_1.default, Object.assign({ align: field.numeric ? 'right' : 'left', className: field.className, style: field.style }, { children: dataRow[field.name] }), i));
                            }), getActions ? ((0, jsx_runtime_1.jsx)(TableCell_1.default, Object.assign({ align: 'right', className: actionsFieldClassname || '' }, { children: getActions(dataRow) }))) : null] }), i));
                }) })] })));
    const renderTable = withContainer ? ((0, jsx_runtime_1.jsx)(TableContainer_1.default, Object.assign({ component: Paper_1.default }, { children: table }))) : table;
    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: { width: '100%' } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: { overflowX: 'auto' } }, { children: renderTable })) })));
};
exports.default = SimpleTable;
//# sourceMappingURL=SimpleTable.js.map