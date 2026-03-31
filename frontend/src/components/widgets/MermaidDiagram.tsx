import React, { Suspense } from 'react'
import { Box, CircularProgress } from '@mui/material'

const Impl = React.lazy(() => import('./MermaidDiagramImpl'))

function MermaidDiagram(props: any) {
  return (
    <Suspense fallback={
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 100, bgcolor: 'rgba(0, 0, 0, 0.1)', borderRadius: 2 }}>
        <CircularProgress size={24} />
      </Box>
    }>
      <Impl {...props} />
    </Suspense>
  )
}

/**
 * Extract Mermaid code blocks from markdown content
 * Returns array of mermaid code strings
 */
export function extractMermaidDiagrams(content: string): string[] {
  if (!content) return []

  const mermaidRegex = /```mermaid\s*([\s\S]*?)```/gi
  const matches: string[] = []
  let match

  while ((match = mermaidRegex.exec(content)) !== null) {
    if (match[1]?.trim()) {
      matches.push(match[1].trim())
    }
  }

  return matches
}

/**
 * Check if content contains any Mermaid diagrams
 */
export function hasMermaidDiagram(content: string): boolean {
  if (!content) return false
  return /```mermaid/i.test(content)
}

export default MermaidDiagram
