import '@inovua/reactdatagrid-community/index.css';
import React, { FC } from 'react';
import { SxProps } from '@mui/system';
export interface IDataGrid2_Column_Render_Params<DataType = any> {
    value: any;
    data: DataType;
}
export interface IDataGrid2_Column<DataType = any> {
    name: string;
    header: string;
    defaultFlex?: number;
    defaultWidth?: number;
    minWidth?: number;
    textAlign?: 'start' | 'center' | 'end';
    render?: (params: IDataGrid2_Column_Render_Params<DataType>) => any;
}
interface DataGridProps<DataType = any> {
    idProperty?: string;
    rows: DataType[];
    columns: IDataGrid2_Column<DataType>[];
    sx?: SxProps;
    innerSx?: SxProps;
    loading?: boolean;
    userSelect?: boolean;
    minHeight?: number;
    rowHeight?: number;
    headerHeight?: number;
    autoSort?: boolean;
    editable?: boolean;
    onRowsChange?: {
        (rows: any[]): void;
    };
    onSelect?: {
        (rowIdx: number, colIdx: number): void;
    };
    onDoubleClick?: {
        (rowIdx: number): void;
    };
}
declare const DataGrid: FC<React.PropsWithChildren<DataGridProps>>;
export default DataGrid;
//# sourceMappingURL=DataGrid.d.ts.map