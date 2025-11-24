import React, { FC, useState, useRef } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import { useTheme } from '@mui/material/styles'
import { Download, X } from 'lucide-react'
import MDEditor from '@uiw/react-md-editor'
import '@uiw/react-md-editor/markdown-editor.css'
import html2canvas from 'html2canvas'
import { jsPDF } from 'jspdf'
import generatePDF from 'react-to-pdf';
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'

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
  const targetRef = useRef<HTMLDivElement>(null)
  const [isGenerating, setIsGenerating] = useState(false)

  const handleDownload = async () => {
    if (!targetRef.current) return
    
    try {
      setIsGenerating(true)

      generatePDF(targetRef, {filename: filename})
      
    } catch (error) {
      console.error('Error generating PDF:', error)
    } finally {
      setIsGenerating(false)
    }
  }

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
          <Button
            variant="contained"
            color="secondary"
            startIcon={<Download size={18} />}
            onClick={handleDownload}
            disabled={isGenerating}
            sx={{ mr: 1 }}
          >
            {isGenerating ? 'Generating...' : 'Download PDF'}
          </Button>
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
          }}
        >
          <Box
            sx={{
              flex: 1,
              overflow: 'auto',
              p: 3,
              ...lightTheme.scrollbar,
              backgroundColor: '#ffffff', // Always white background for PDF preview
              color: '#000000', // Always black text
            }}
          >
            <div ref={targetRef} style={{ padding: '20px', backgroundColor: '#ffffff', color: '#000000' }}>
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[rehypeRaw]}
                components={{
                  img: ({node, ...props}) => (
                    <img 
                      {...props} 
                      style={{ maxWidth: '100%' }} 
                      src={props.src?.startsWith('http') ? props.src : 
                           props.src ? `${account.serverConfig?.filestore_prefix}/${props.src}?redirect_urls=true` : ''}
                    />
                  ),
                  p: ({node, ...props}) => <div {...props} style={{ marginBottom: '16px', color: '#000000' }} />,
                  h1: ({node, ...props}) => <h1 {...props} style={{ color: '#000000', marginBottom: '0.5em' }} />,
                  h2: ({node, ...props}) => <h2 {...props} style={{ color: '#000000', marginBottom: '0.5em' }} />,
                  h3: ({node, ...props}) => <h3 {...props} style={{ color: '#000000', marginBottom: '0.5em' }} />,
                  h4: ({node, ...props}) => <h4 {...props} style={{ color: '#000000', marginBottom: '0.5em' }} />,
                  li: ({node, ...props}) => <li {...props} style={{ color: '#000000' }} />,
                  a: ({node, ...props}) => <a {...props} style={{ color: '#1976d2' }} />,
                  code: ({node, className, children, ...props}) => {
                    const match = /language-(\w+)/.exec(className || '')
                    return match ? (
                      <pre style={{ 
                        backgroundColor: '#f5f5f5', 
                        padding: '1em', 
                        borderRadius: '4px', 
                        overflowX: 'auto',
                        color: '#000000'
                      }}>
                        <code className={className} {...props}>
                          {children}
                        </code>
                      </pre>
                    ) : (
                      <code className={className} style={{ 
                        backgroundColor: '#f5f5f5', 
                        padding: '0.2em 0.4em', 
                        borderRadius: '3px',
                        color: '#000000'
                      }} {...props}>
                        {children}
                      </code>
                    )
                  }
                }}
              >
                {markdown}
              </ReactMarkdown>
            </div>
          </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default ToPDF
