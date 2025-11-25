import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import { useTheme } from '@mui/material/styles'
import { X } from 'lucide-react'
import MDEditor from '@uiw/react-md-editor'
import '@uiw/react-md-editor/markdown-editor.css'
import { BlobProvider, PDFDownloadLink } from '@react-pdf/renderer'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import useDebounce from '../../hooks/useDebounce'
import { PdfDocument } from './PdfRenderer'

export interface ToPDFProps {
  markdown: string
  filename?: string
  onClose?: () => void
}

const ToPDF: FC<ToPDFProps> = ({ markdown: initialMarkdown, filename = 'export.pdf', onClose }) => {
  const theme = useTheme()
  const lightTheme = useLightTheme()
  const account = useAccount()
  const [markdown, setMarkdown] = useState(initialMarkdown)
  const debouncedMarkdown = useDebounce(markdown, 1000)

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          p: 2,
          borderBottom: theme.palette.mode === 'light' ? '1px solid #e0e0e0' : '1px solid #444',
        }}
      >
        <Typography variant="h6">Export to PDF</Typography>
        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>          
          {onClose && (
            <IconButton onClick={onClose} size="small">
              <X size={18} />
            </IconButton>
          )}
        </Box>
      </Box>

      <Box
        sx={{
          flex: 1,
          display: 'flex',
          flexDirection: 'row',
          overflow: 'hidden',
          minHeight: 0,
        }}
      >
        <Box
          sx={{
            width: '50%',
            borderRight: theme.palette.mode === 'light' ? '1px solid #e0e0e0' : '1px solid #444',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <Box
            sx={{
              flex: 1,
              overflow: 'auto',
              ...lightTheme.scrollbar,
            }}
          >
            <MDEditor
              value={markdown}
              onChange={(value) => setMarkdown(value || '')}
              preview="edit"
              hideToolbar={false}
              data-color-mode={theme.palette.mode}
              visibleDragbar={false}
              height="100%"
            />
          </Box>
        </Box>

        <Box
          sx={{
            width: '50%',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            backgroundColor: '#525659', // Dark background for PDF viewer
          }}
        >
           <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
             <BlobProvider document={<PdfDocument markdown={debouncedMarkdown} serverConfig={account.serverConfig} />}>
               {((params: any) => {
                 const { url, loading, error } = params
                 if (loading) {
                   return (
                     <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'white' }}>
                       <Typography>Generating PDF preview...</Typography>
                     </Box>
                   )
                 }
                 if (error) {
                   return (
                     <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#ff8a80' }}>
                       <Typography>Error generating PDF: {error.message}</Typography>
                     </Box>
                   )
                 }
                 if (!url) {
                    return (
                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'white' }}>
                        <Typography>No PDF generated</Typography>
                      </Box>
                    )
                 }
                return (
                  <iframe
                    key={url}
                    src={url}
                    style={{ width: '100%', height: '100%', border: 'none' }}
                    title="PDF Preview"
                  />
                )
               }) as any}
             </BlobProvider>
           </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default ToPDF