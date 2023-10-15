"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Visibility_1 = __importDefault(require("@mui/icons-material/Visibility"));
const DataGrid_1 = __importDefault(require("./DataGrid"));
const JsonWindowLink_1 = __importDefault(require("../widgets/JsonWindowLink"));
const columns = [
    {
        name: 'created_at',
        header: 'Date',
        defaultFlex: 1,
        render: ({ data }) => {
            return ((0, jsx_runtime_1.jsx)("div", { children: new Date(data.created).toLocaleString() }));
        }
    },
    {
        name: 'id',
        header: 'ID',
        defaultFlex: 1,
    },
    {
        name: 'state',
        header: 'State',
        defaultFlex: 1,
    },
    {
        name: 'actions',
        header: 'Actions',
        minWidth: 100,
        defaultWidth: 100,
        textAlign: 'end',
        render: ({ data }) => {
            return ((0, jsx_runtime_1.jsx)(JsonWindowLink_1.default, Object.assign({ data: data }, { children: (0, jsx_runtime_1.jsx)(Visibility_1.default, {}) })));
        }
    },
];
const JobDataGrid = ({ jobs, loading, }) => {
    return ((0, jsx_runtime_1.jsx)(DataGrid_1.default, { autoSort: true, userSelect: true, rows: jobs, columns: columns, loading: loading }));
};
exports.default = JobDataGrid;
//# sourceMappingURL=Job.js.map