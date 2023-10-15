"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
require("@inovua/reactdatagrid-community/index.css");
const react_1 = require("react");
const styles_1 = require("@mui/material/styles");
const reactdatagrid_community_1 = __importDefault(require("@inovua/reactdatagrid-community"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const Loading_1 = __importDefault(require("../system/Loading"));
const gridStyle = {
    minHeight: 400,
    border: 'none',
    boxShadow: '0px 4px 10px 0px rgba(0,0,0,0.1)',
    width: '100%',
    height: '100%',
    position: 'relative',
    flexGrow: 1,
    flexBasis: '100%',
};
const getHeaderStyle = (theme) => ({
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    border: 'none',
    fontSize: '0.8rem',
    fontWeight: 500,
    textTransform: 'uppercase',
    fontFamily: '"Open Sans", sans-serif, Arial, Helvetica',
    paddingLeft: '0.2rem',
    paddingRight: '0.2rem',
    height: '40px',
});
const DataGrid = ({ idProperty = 'id', rows, columns, sx = {}, innerSx = {}, userSelect = false, minHeight = 400, autoSort = false, editable = false, rowHeight = 56, headerHeight = 40, loading, onDoubleClick, onRowsChange, onSelect, }) => {
    const theme = (0, styles_1.useTheme)();
    const useHeaderStyle = (0, react_1.useMemo)(() => {
        return getHeaderStyle(theme);
    }, [
        theme,
    ]);
    const useColumns = (0, react_1.useMemo)(() => {
        return columns.map((col, i) => {
            return Object.assign({}, col, {
                headerProps: {
                    style: useHeaderStyle,
                },
            });
        });
    }, [
        columns,
        useHeaderStyle,
    ]);
    const onCellClick = (0, react_1.useCallback)((ev, cellProps) => {
        if (!onSelect)
            return;
        onSelect(cellProps.rowIndex, cellProps.columnIndex);
    }, [
        onSelect,
    ]);
    return ((0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ className: 'grid-wrapper', sx: Object.assign({ width: '100%', height: '100%', minHeight: '100%', position: 'relative', display: 'flex', overflow: 'auto', boxShadow: '0 2px 4px 0px rgba(0,0,0,0.2)', borderTopLeftRadius: '12px', borderTopRightRadius: '12px', '& .InovuaReactDataGrid--theme-default-light .InovuaReactDataGrid__column-header__content': {
                fontWeight: 'lighter',
            } }, sx) }, { children: [(0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ className: 'Grid', sx: Object.assign({ display: 'flex', flexDirection: 'column', height: '100%', flexGrow: 1, backgroundColor: '#fff', '& .rdg': {
                        userSelect: (userSelect ? 'auto' : 'none')
                    }, minHeight: `${minHeight}px` }, innerSx) }, { children: (0, jsx_runtime_1.jsx)(reactdatagrid_community_1.default, { columnUserSelect: editable ? false : true, editable: editable, idProperty: idProperty, columns: columns, dataSource: rows, onCellClick: onCellClick, onCellDoubleClick: (ev, props) => onDoubleClick && onDoubleClick(props.rowIndex), headerHeight: headerHeight, minRowHeight: rowHeight, rowHeight: rowHeight, style: gridStyle, showCellBorders: false }) })), loading && ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                    position: 'absolute',
                    width: '100%',
                    height: '100%',
                    left: '0px',
                    top: '0px',
                    backgroundColor: 'rgba(255,255,255,0.8)',
                } }, { children: (0, jsx_runtime_1.jsx)(Loading_1.default, {}) })))] })));
};
exports.default = DataGrid;
//# sourceMappingURL=DataGrid.js.map