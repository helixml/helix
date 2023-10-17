import '@inovua/reactdatagrid-community/index.css'
import React, { FC, useCallback, useMemo, useRef } from 'react'
import { useTheme, Theme } from '@mui/material/styles'
import ReactDataGrid from '@inovua/reactdatagrid-community'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'
import Loading from '../system/Loading'

const gridStyle = {
  minHeight: 400,
  border: 'none',
  boxShadow: '0px 4px 10px 0px rgba(0,0,0,0.1)',
  width: '100%',
  height: '100%',
  position: 'relative',
  flexGrow: 1,
  flexBasis: '100%',
}

const getHeaderStyle = (theme: Theme) => ({
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
})

export interface IDataGrid2_Column_Render_Params<DataType = any> {
  value: any,
  data: DataType,
}

export interface IDataGrid2_Column<DataType = any> {
  name: string,
  header: string,
  defaultFlex?: number,
  defaultWidth?: number,
  minWidth?: number,
  textAlign?: 'start' | 'center' | 'end',
  render?: (params: IDataGrid2_Column_Render_Params<DataType>) => any, 
}

interface DataGridProps<DataType = any> {
  idProperty?: string,
  rows: DataType[],
  columns: IDataGrid2_Column<DataType>[],
  sx?: SxProps,
  innerSx?: SxProps,
  loading?: boolean,
  userSelect?: boolean,
  minHeight?: number,
  rowHeight?: number,
  headerHeight?: number,
  autoSort?: boolean,
  editable?: boolean,
  onSelect?: {
    (rowIdx: number, colIdx: number): void,
  },
  onDoubleClick?: {
    (rowIdx: number): void,
  },
}

const DataGrid: FC<React.PropsWithChildren<DataGridProps>> = ({
  idProperty = 'id',
  rows,
  columns,
  sx = {},
  innerSx = {},
  userSelect = false,
  minHeight = 400,
  editable = false,
  rowHeight = 56,
  headerHeight = 40,
  loading,
  onDoubleClick,
  onSelect,
}) => {
  const onCellClick = useCallback((ev: any, cellProps: any) => {
    if(!onSelect) return
    onSelect(cellProps.rowIndex, cellProps.columnIndex)
  }, [
    onSelect,
  ])

  return (
    <Box
      className='grid-wrapper'
      sx={{
        width: '100%',
        height: '100%',
        minHeight: '100%',
        position: 'relative',
        display: 'flex',
        overflow: 'auto',
        boxShadow: '0 2px 4px 0px rgba(0,0,0,0.2)',
        borderTopLeftRadius: '12px',
        borderTopRightRadius: '12px',
        '& .InovuaReactDataGrid--theme-default-light .InovuaReactDataGrid__column-header__content': {
          fontWeight: 'lighter',
        },
        ...sx,
      }}
    >
      <Box
        className='Grid'
        sx={{
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          flexGrow: 1,
          backgroundColor: '#fff',
          '& .rdg': {
            userSelect: ( userSelect ? 'auto' : 'none' )
          },
          minHeight: `${minHeight}px`,
          ...innerSx,
        }}
      >
        <ReactDataGrid
          columnUserSelect={ editable ? false : true }
          editable={ editable }
          idProperty={ idProperty }
          columns={ columns }
          dataSource={ rows }
          onCellClick={ onCellClick }
          onCellDoubleClick={ (ev, props) => onDoubleClick && onDoubleClick(props.rowIndex) }
          headerHeight={ headerHeight }
          minRowHeight={ rowHeight }
          rowHeight={ rowHeight }
          style={ gridStyle }
          showCellBorders={ false }
        />
      </Box>
      {
        loading && (
          <Box
            sx={{
              position: 'absolute',
              width: '100%',
              height: '100%',
              left: '0px',
              top: '0px',
              backgroundColor: 'rgba(255,255,255,0.8)',
            }}
          >
            <Loading />
          </Box>
        )
      }
    </Box>
    
  )
}

export default DataGrid