import React, { FC, useState, useMemo, useCallback } from 'react'
import ReactMarkdown, { Components } from 'react-markdown'
import {
  Box,
  Typography,
  Alert,
  Chip,
  Stack,
  Card,
  CardContent,
  Collapse,
  IconButton,
  CircularProgress,
  TextField,
  InputAdornment,
  Tab,
  Tabs,
  List,
  ListItemButton,
  ListItemText,
  Select,
  MenuItem,
  useMediaQuery,
  useTheme,
} from '@mui/material'
import {
  Brain,
  BookOpen,
  Search as SearchIcon,
  GitCommit,
  X as CloseIcon,
  Key,
  Plug,
  Copy,
  Eye,
  EyeOff,
  ChevronDown,
  ChevronRight,
  FileText,
  FolderTree,
  Terminal,
  Sparkles,
  Type as TypeIcon,
} from 'lucide-react'
import remarkGfm from 'remark-gfm'
import MermaidDiagram from '../widgets/MermaidDiagram'

import {
  useKoditCommits,
  useKoditStatus,
  useKoditRescan,
  useKoditWikiTree,
  useKoditWikiPage,
  useKoditSemanticSearch,
  useKoditKeywordSearch,
  useKoditGrep,
  useKoditFiles,
  useKoditFileContent,
  KODIT_SUBTYPE_COMMIT_DESCRIPTION,
  KoditWikiTreeNode,
  KoditFileResult,
  KoditGrepResult,
  KoditFileEntry,
  KoditSearchMeta,
  KoditGrepMeta,
  KoditFilesMeta,
} from '../../services/koditService'
import useDebounce from '../../hooks/useDebounce'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetUserAPIKeys } from '../../services/userService'
import KoditStatusPill from './KoditStatusPill'

interface CodeIntelligenceTabProps {
  repository: any
  enrichments: any[]
  repoId: string
  commitSha?: string
}

type SubTab = 'wiki' | 'search' | 'changelog' | 'connect'

// Shared markdown styles
const markdownContentStyles = {
  '& p': { margin: '0 0 1em 0', '&:last-child': { margin: 0 } },
  '& ul, & ol': { margin: '0 0 1em 0', paddingLeft: '1.5em' },
  '& li': { margin: '0.3em 0' },
  '& code': {
    backgroundColor: 'rgba(0, 0, 0, 0.1)',
    padding: '0.2em 0.4em',
    borderRadius: '3px',
    fontSize: '0.9em',
    fontFamily: 'monospace',
  },
  '& pre': {
    backgroundColor: 'rgba(0, 0, 0, 0.1)',
    padding: '1em',
    borderRadius: '4px',
    overflow: 'auto',
    fontSize: '0.85em',
    fontFamily: 'monospace',
  },
  '& h1, & h2, & h3, & h4, & h5, & h6': {
    margin: '1.2em 0 0.5em 0',
    fontWeight: 600,
    '&:first-of-type': { marginTop: 0 },
  },
  '& a': {
    color: '#00d5ff',
    textDecoration: 'none',
    '&:hover': { textDecoration: 'underline' },
  },
  '& table': {
    borderCollapse: 'collapse',
    width: '100%',
    margin: '1em 0',
  },
  '& th, & td': {
    border: '1px solid rgba(255, 255, 255, 0.1)',
    padding: '0.5em 0.75em',
    textAlign: 'left' as const,
  },
  '& th': {
    backgroundColor: 'rgba(0, 0, 0, 0.1)',
    fontWeight: 600,
  },
  '& blockquote': {
    borderLeft: '3px solid rgba(0, 213, 255, 0.4)',
    margin: '1em 0',
    padding: '0.5em 1em',
    color: 'text.secondary',
  },
}

const markdownComponents: Components = {
  code: ({ className, children, node, ...props }) => {
    const match = /language-(\w+)/.exec(className || '')
    const language = match ? match[1] : ''
    const codeContent = String(children).replace(/\n$/, '')
    const isBlock = node?.position && codeContent.includes('\n') || !!language

    if (language === 'mermaid') {
      return <MermaidDiagram code={codeContent} />
    }

    if (!isBlock) {
      return <code className={className} {...props}>{children}</code>
    }

    return (
      <Box
        component="pre"
        sx={{
          backgroundColor: 'rgba(0, 0, 0, 0.1)',
          padding: '1em',
          borderRadius: '4px',
          overflow: 'auto',
          fontSize: '0.85em',
          fontFamily: 'monospace',
        }}
      >
        <code className={className} {...props}>
          {children}
        </code>
      </Box>
    )
  },
}

// ─── Wiki Sub-tab ───────────────────────────────────────────────────────────────

const WikiSubTab: FC<{ repoId: string; enabled: boolean }> = ({ repoId, enabled }) => {
  const theme = useTheme()
  const isMobile = useMediaQuery(theme.breakpoints.down('md'))
  const { data: wikiTree, isLoading: treeLoading, error: treeError } = useKoditWikiTree(repoId, { enabled })
  const [selectedPath, setSelectedPath] = useState<string>('')

  // Auto-select first page when tree loads, using the link from the API if available
  const effectivePath = selectedPath || findFirstPagePath(wikiTree || [])

  // Extract wiki page path from a links.page URL (e.g. "/api/v1/.../wiki-page?path=foo.md" -> "foo")
  const extractWikiPath = useCallback((node: KoditWikiTreeNode): string => {
    if (node.links?.page) {
      try {
        const url = new URL(node.links.page, window.location.origin)
        const p = url.searchParams.get('path')
        if (p) return p.replace(/\.md$/, '')
      } catch { /* fall through */ }
    }
    return node.path?.replace(/\.md$/, '') || node.slug
  }, [])

  const { data: wikiPage, isLoading: pageLoading } = useKoditWikiPage(repoId, effectivePath, {
    enabled: enabled && !!effectivePath,
  })

  // Build a path index from the wiki tree so we can resolve slugs to full paths
  const pathIndex = useMemo(() => {
    const index: Record<string, string> = {}
    function walk(nodes: KoditWikiTreeNode[]) {
      for (const node of nodes) {
        const path = node.path?.replace(/\.md$/, '') || node.slug
        index[node.slug] = path
        if (node.children) walk(node.children)
      }
    }
    walk(wikiTree || [])
    return index
  }, [wikiTree])

  // Intercept clicks on wiki-internal links
  const handleContentClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement
    const anchor = target.closest('a')
    if (!anchor) return

    const href = anchor.getAttribute('href') || ''
    // Skip external URLs and anchor-only links
    if (href.startsWith('http://') || href.startsWith('https://') || href.startsWith('#')) return
    // Skip links that look like API paths (from old rewriting)
    if (href.startsWith('/api/')) {
      e.preventDefault()
      // Try to extract the wiki page path from the URL
      const match = href.match(/\/wiki\/(.+?)(?:\.md)?$/)
      if (match) {
        setSelectedPath(match[1])
      }
      return
    }

    // This is a relative wiki link (slug or path)
    e.preventDefault()
    let pagePath = href.replace(/\.md$/, '').replace(/^\//, '')

    // Try to resolve via path index if it's a bare slug
    if (pathIndex[pagePath]) {
      pagePath = pathIndex[pagePath]
    }

    setSelectedPath(pagePath)
  }, [pathIndex])

  // Flatten tree for mobile dropdown (must be before early returns to satisfy hooks rules)
  const flatPages = useMemo(() => {
    const result: { path: string; title: string; depth: number }[] = []
    function walk(nodes: KoditWikiTreeNode[], depth: number) {
      for (const node of nodes) {
        const path = extractWikiPath(node)
        result.push({ path, title: node.title, depth })
        if (node.children) walk(node.children, depth + 1)
      }
    }
    walk(wikiTree || [], 0)
    return result
  }, [wikiTree, extractWikiPath])

  if (treeLoading) {
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 4 }}>
        <CircularProgress size={20} />
        <Typography variant="body2" color="text.secondary">Loading wiki...</Typography>
      </Box>
    )
  }

  if (treeError || !wikiTree || wikiTree.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 8 }}>
        <BookOpen size={48} color="#656d76" style={{ marginBottom: 16, opacity: 0.5 }} />
        <Typography variant="h6" color="text.secondary" gutterBottom>
          Wiki not available yet
        </Typography>
        <Typography variant="body2" color="text.secondary">
          The wiki is generated automatically during indexing. Check back once indexing completes.
        </Typography>
      </Box>
    )
  }

  return (
    <Box>
      {/* Mobile: dropdown page selector */}
      {isMobile ? (
        <Select
          value={effectivePath}
          onChange={(e) => setSelectedPath(e.target.value as string)}
          size="small"
          fullWidth
          sx={{ mb: 2 }}
        >
          {flatPages.map((p) => (
            <MenuItem key={p.path} value={p.path} sx={{ pl: 2 + p.depth * 2 }}>
              {p.title}
            </MenuItem>
          ))}
        </Select>
      ) : null}

      <Box sx={{ display: 'flex', gap: 3 }}>
        {/* Desktop: sidebar navigation */}
        {!isMobile && (
          <Box
            sx={{
              width: 260,
              minWidth: 260,
              borderRight: '1px solid',
              borderColor: 'divider',
              pr: 2,
              position: 'sticky',
              top: 64,
              alignSelf: 'flex-start',
              maxHeight: 'calc(100vh - 80px)',
              overflow: 'auto',
            }}
          >
            <WikiTreeNav
              nodes={wikiTree}
              selectedPath={effectivePath}
              onSelect={setSelectedPath}
              extractPath={extractWikiPath}
              depth={0}
            />
          </Box>
        )}

        {/* Page content */}
        <Box sx={{ flex: 1 }} onClick={handleContentClick}>
          {pageLoading ? (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 4 }}>
              <CircularProgress size={20} />
              <Typography variant="body2" color="text.secondary">Loading page...</Typography>
            </Box>
          ) : wikiPage ? (
            <Box sx={markdownContentStyles}>
              <Typography variant="h4" sx={{ fontWeight: 600, mb: 3 }}>
                {wikiPage.title}
              </Typography>
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {wikiPage.content}
              </ReactMarkdown>
            </Box>
          ) : (
            <Typography variant="body2" color="text.secondary">
              Select a page from the sidebar.
            </Typography>
          )}
        </Box>
      </Box>
    </Box>
  )
}

function findFirstPagePath(nodes: KoditWikiTreeNode[]): string {
  if (nodes.length === 0) return ''
  // Prefer to extract from the API link if available
  const first = nodes[0]
  if (first.links?.page) {
    try {
      const url = new URL(first.links.page, window.location.origin)
      const p = url.searchParams.get('path')
      if (p) return p.replace(/\.md$/, '')
    } catch { /* fall through */ }
  }
  return first.path?.replace(/\.md$/, '') || first.slug
}

const WikiTreeNav: FC<{
  nodes: KoditWikiTreeNode[]
  selectedPath: string
  onSelect: (path: string) => void
  extractPath: (node: KoditWikiTreeNode) => string
  depth: number
}> = ({ nodes, selectedPath, onSelect, extractPath, depth }) => {
  return (
    <List dense disablePadding>
      {nodes.map((node) => {
        const pagePath = extractPath(node)
        const isSelected = selectedPath === pagePath
        const hasChildren = node.children && node.children.length > 0

        return (
          <React.Fragment key={node.slug}>
            <ListItemButton
              selected={isSelected}
              onClick={() => onSelect(pagePath)}
              sx={{
                pl: 1.5 + depth * 2,
                py: 0.5,
                borderRadius: 1,
                mb: 0.25,
                '&.Mui-selected': {
                  bgcolor: 'rgba(0, 213, 255, 0.12)',
                  '&:hover': { bgcolor: 'rgba(0, 213, 255, 0.18)' },
                },
              }}
            >
              <ListItemText
                primary={node.title}
                primaryTypographyProps={{
                  variant: 'body2',
                  fontSize: '0.85rem',
                  fontWeight: isSelected ? 600 : 400,
                  noWrap: true,
                }}
              />
            </ListItemButton>
            {hasChildren && (
              <WikiTreeNav
                nodes={node.children!}
                selectedPath={selectedPath}
                onSelect={onSelect}
                extractPath={extractPath}
                depth={depth + 1}
              />
            )}
          </React.Fragment>
        )
      })}
    </List>
  )
}

// ─── Search Sub-tab ─────────────────────────────────────────────────────────────

type SearchTool = 'semantic' | 'keyword' | 'grep' | 'ls' | 'read'

const searchTools: { id: SearchTool; label: string; icon: typeof SearchIcon; description: string; placeholder: string }[] = [
  { id: 'semantic', label: 'Semantic Search', icon: Sparkles, description: 'Find code by meaning using AI embeddings', placeholder: 'Describe what you\'re looking for...' },
  { id: 'keyword', label: 'Keyword Search', icon: TypeIcon, description: 'BM25 full-text keyword search', placeholder: 'Enter keywords...' },
  { id: 'grep', label: 'Grep', icon: Terminal, description: 'Regex pattern matching via git grep', placeholder: 'Enter regex pattern...' },
  { id: 'ls', label: 'List Files', icon: FolderTree, description: 'List files matching a glob pattern', placeholder: 'e.g. **/*.go, src/**/*.ts' },
  { id: 'read', label: 'Read File', icon: FileText, description: 'Read the contents of a file', placeholder: 'e.g. cmd/main.go' },
]

// Extract a file path from a links.file_content URL. Falls back to the given path if
// the URL doesn't contain a recognisable `path=` query parameter.
function extractPathFromLink(link: string | undefined, fallback: string): string {
  if (!link) return fallback
  try {
    const url = new URL(link, window.location.origin)
    return url.searchParams.get('path') || fallback
  } catch {
    return fallback
  }
}

const SearchSubTab: FC<{ repoId: string; enabled: boolean }> = ({ repoId, enabled }) => {
  const [activeTool, setActiveTool] = useState<SearchTool>('semantic')
  const [searchInput, setSearchInput] = useState('')
  const [globFilter, setGlobFilter] = useState('')
  const debouncedInput = useDebounce(searchInput, 400)
  const debouncedGlob = useDebounce(globFilter, 400)
  const [selectedFile, setSelectedFile] = useState<string>('')

  const hasQuery = debouncedInput.trim().length > 0

  const { data: semanticResponse, isLoading: semanticLoading } = useKoditSemanticSearch(
    repoId, debouncedInput, 20, undefined,
    { enabled: enabled && activeTool === 'semantic' && hasQuery }
  )
  const { data: keywordResponse, isLoading: keywordLoading } = useKoditKeywordSearch(
    repoId, debouncedInput, 20, undefined,
    { enabled: enabled && activeTool === 'keyword' && hasQuery }
  )
  const { data: grepResponse, isLoading: grepLoading } = useKoditGrep(
    repoId, debouncedInput, debouncedGlob || undefined, 50,
    { enabled: enabled && activeTool === 'grep' && hasQuery }
  )
  const { data: filesResponse, isLoading: filesLoading } = useKoditFiles(
    repoId, debouncedInput || '**/*',
    { enabled: enabled && activeTool === 'ls' && hasQuery }
  )
  const { data: fileContent, isLoading: fileLoading } = useKoditFileContent(
    repoId, selectedFile || debouncedInput,
    { enabled: enabled && activeTool === 'read' && (!!selectedFile || hasQuery) }
  )

  const semanticResults = semanticResponse?.data ?? []
  const semanticMeta = semanticResponse?.meta
  const keywordResults = keywordResponse?.data ?? []
  const keywordMeta = keywordResponse?.meta
  const grepResults = grepResponse?.data ?? []
  const grepMeta = grepResponse?.meta
  const fileList = filesResponse?.data ?? []
  const filesMeta = filesResponse?.meta

  const activeToolDef = searchTools.find(t => t.id === activeTool)!
  const isLoading = activeTool === 'semantic' ? semanticLoading
    : activeTool === 'keyword' ? keywordLoading
    : activeTool === 'grep' ? grepLoading
    : activeTool === 'ls' ? filesLoading
    : fileLoading

  const handleToolChange = (tool: SearchTool) => {
    setActiveTool(tool)
    setSearchInput('')
    setGlobFilter('')
    setSelectedFile('')
  }

  return (
    <Box>
      {/* Tool selector */}
      <Box sx={{ display: 'flex', gap: 1, mb: 3, flexWrap: 'wrap' }}>
        {searchTools.map((tool) => {
          const Icon = tool.icon
          const isActive = activeTool === tool.id
          return (
            <Chip
              key={tool.id}
              icon={<Icon size={14} />}
              label={tool.label}
              onClick={() => handleToolChange(tool.id)}
              variant={isActive ? 'filled' : 'outlined'}
              color={isActive ? 'primary' : 'default'}
              sx={{
                cursor: 'pointer',
                fontWeight: isActive ? 600 : 400,
              }}
            />
          )
        })}
      </Box>

      {/* Description */}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {activeToolDef.description}
      </Typography>

      {/* Input area */}
      <Box sx={{ display: 'flex', gap: 1, mb: 3 }}>
        <TextField
          size="small"
          placeholder={activeToolDef.placeholder}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          fullWidth
          sx={{ maxWidth: 600 }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                {React.createElement(activeToolDef.icon, { size: 18 })}
              </InputAdornment>
            ),
            endAdornment: searchInput ? (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => { setSearchInput(''); setSelectedFile('') }} edge="end">
                  <CloseIcon size={16} />
                </IconButton>
              </InputAdornment>
            ) : undefined,
            sx: { fontFamily: activeTool === 'grep' || activeTool === 'read' || activeTool === 'ls' ? 'monospace' : undefined },
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && activeTool === 'read' && searchInput.trim()) {
              setSelectedFile(searchInput.trim())
            }
          }}
        />
        {activeTool === 'grep' && (
          <TextField
            size="small"
            placeholder="File filter (e.g. *.go)"
            value={globFilter}
            onChange={(e) => setGlobFilter(e.target.value)}
            sx={{ width: 200 }}
            InputProps={{
              sx: { fontFamily: 'monospace' },
            }}
          />
        )}
      </Box>

      {/* Loading indicator */}
      {isLoading && hasQuery && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <CircularProgress size={16} />
          <Typography variant="body2" color="text.secondary">Searching...</Typography>
        </Box>
      )}

      {/* Results */}
      {activeTool === 'semantic' && hasQuery && !semanticLoading && (
        <SearchResultsList results={semanticResults} meta={semanticMeta} onFileClick={(path) => { setActiveTool('read'); setSearchInput(path); setSelectedFile(path) }} />
      )}
      {activeTool === 'keyword' && hasQuery && !keywordLoading && (
        <SearchResultsList results={keywordResults} meta={keywordMeta} onFileClick={(path) => { setActiveTool('read'); setSearchInput(path); setSelectedFile(path) }} />
      )}
      {activeTool === 'grep' && hasQuery && !grepLoading && (
        <GrepResultsList results={grepResults} meta={grepMeta} onFileClick={(path) => { setActiveTool('read'); setSearchInput(path); setSelectedFile(path) }} />
      )}
      {activeTool === 'ls' && hasQuery && !filesLoading && (
        <FileListResults files={fileList} meta={filesMeta} onFileClick={(path) => { setActiveTool('read'); setSearchInput(path); setSelectedFile(path) }} />
      )}
      {activeTool === 'read' && (selectedFile || hasQuery) && !fileLoading && fileContent && (
        <FileContentView content={fileContent} />
      )}
      {activeTool === 'read' && (selectedFile || hasQuery) && !fileLoading && !fileContent && (
        <Alert severity="info">File not found or empty.</Alert>
      )}

      {/* Empty state */}
      {!hasQuery && !selectedFile && (
        <Box sx={{ textAlign: 'center', py: 6 }}>
          <SearchIcon size={40} color="#656d76" style={{ marginBottom: 12, opacity: 0.4 }} />
          <Typography variant="body1" color="text.secondary">
            {activeToolDef.description}
          </Typography>
        </Box>
      )}
    </Box>
  )
}

// Search results (semantic + keyword)
const SearchResultsList: FC<{
  results: KoditFileResult[]
  meta?: KoditSearchMeta
  onFileClick: (path: string) => void
}> = ({ results, meta, onFileClick }) => {
  if (results.length === 0) {
    return <Alert severity="info">No results found.</Alert>
  }

  const count = meta?.count ?? results.length
  const limitHit = meta && count >= meta.limit

  return (
    <Stack spacing={1}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Chip label={`${count} result${count !== 1 ? 's' : ''}`} size="small" />
        {limitHit && (
          <Typography variant="caption" color="text.secondary">
            (limit reached)
          </Typography>
        )}
      </Box>
      {results.map((r, i) => {
        const filePath = extractPathFromLink(r.links?.file_content, r.path)
        return (
          <Card key={i} variant="outlined" sx={{ '&:hover': { borderColor: 'primary.main' } }}>
            <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
                {r.language && <Chip label={r.language} size="small" sx={{ fontSize: '0.7rem', height: 20 }} />}
                {filePath && (
                  <Typography
                    variant="caption"
                    sx={{
                      fontFamily: 'monospace',
                      cursor: 'pointer',
                      color: '#00d5ff',
                      '&:hover': { textDecoration: 'underline' },
                    }}
                    onClick={() => onFileClick(filePath)}
                  >
                    {filePath}
                  </Typography>
                )}
                {r.lines && <Typography variant="caption" color="text.secondary">{r.lines}</Typography>}
                <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto' }}>
                  {r.score > 0 ? `score: ${r.score.toFixed(3)}` : ''}
                </Typography>
              </Box>
              <Box
                component="pre"
                sx={{
                  fontSize: '0.8rem',
                  lineHeight: 1.5,
                  fontFamily: 'monospace',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  margin: 0,
                  color: 'text.secondary',
                  maxHeight: 200,
                  overflow: 'auto',
                }}
              >
                {r.preview}
              </Box>
            </CardContent>
          </Card>
        )
      })}
    </Stack>
  )
}

// Grep results
const GrepResultsList: FC<{
  results: KoditGrepResult[]
  meta?: KoditGrepMeta
  onFileClick: (path: string) => void
}> = ({ results, meta, onFileClick }) => {
  if (results.length === 0) {
    return <Alert severity="info">No matches found.</Alert>
  }

  const count = meta?.count ?? results.length
  const limitHit = meta && count >= meta.limit

  return (
    <Stack spacing={1}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Chip label={`${count} file${count !== 1 ? 's' : ''}`} size="small" />
        {limitHit && (
          <Typography variant="caption" color="text.secondary">
            (limit reached)
          </Typography>
        )}
      </Box>
      {results.map((r, i) => {
        const filePath = extractPathFromLink(r.links?.file_content, r.path)
        return (
          <Card key={i} variant="outlined" sx={{ '&:hover': { borderColor: 'primary.main' } }}>
            <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <Typography
                  variant="body2"
                  sx={{ fontFamily: 'monospace', fontWeight: 600, cursor: 'pointer', color: '#00d5ff', '&:hover': { textDecoration: 'underline' } }}
                  onClick={() => onFileClick(filePath)}
                >
                  {filePath}
                </Typography>
                {r.language && <Chip label={r.language} size="small" sx={{ fontSize: '0.7rem', height: 20 }} />}
                <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto' }}>
                  {r.matches.length} match{r.matches.length !== 1 ? 'es' : ''}
                </Typography>
              </Box>
              <Box
                component="pre"
                sx={{
                  fontSize: '0.8rem',
                  lineHeight: 1.5,
                  fontFamily: 'monospace',
                  whiteSpace: 'pre-wrap',
                  margin: 0,
                  color: 'text.secondary',
                  maxHeight: 200,
                  overflow: 'auto',
                }}
              >
                {r.matches.slice(0, 10).map((m, j) => (
                  <Box key={j} component="span" sx={{ display: 'block' }}>
                    <Box component="span" sx={{ color: 'text.disabled', userSelect: 'none', mr: 1 }}>
                      {String(m.line).padStart(4)}
                    </Box>
                    {m.content}
                  </Box>
                ))}
                {r.matches.length > 10 && (
                  <Box component="span" sx={{ color: 'text.disabled', fontStyle: 'italic' }}>
                    ... and {r.matches.length - 10} more matches
                  </Box>
                )}
              </Box>
            </CardContent>
          </Card>
        )
      })}
    </Stack>
  )
}

// File list results
const FileListResults: FC<{
  files: KoditFileEntry[]
  meta?: KoditFilesMeta
  onFileClick: (path: string) => void
}> = ({ files, meta, onFileClick }) => {
  if (files.length === 0) {
    return <Alert severity="info">No files found.</Alert>
  }

  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  const count = meta?.count ?? files.length

  return (
    <Stack spacing={0}>
      <Chip label={`${count} file${count !== 1 ? 's' : ''}`} size="small" sx={{ alignSelf: 'flex-start', mb: 1 }} />
      {files.map((f, i) => {
        const filePath = extractPathFromLink(f.links?.file_content, f.path)
        return (
          <Box
            key={i}
            onClick={() => onFileClick(filePath)}
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              py: 0.5,
              px: 1,
              cursor: 'pointer',
              borderRadius: 1,
              '&:hover': { bgcolor: 'rgba(0, 213, 255, 0.08)' },
            }}
          >
            <FileText size={14} style={{ opacity: 0.5, flexShrink: 0 }} />
            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.85rem', flex: 1 }}>
              {filePath}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
              {formatSize(f.size)}
            </Typography>
          </Box>
        )
      })}
    </Stack>
  )
}

// File content viewer
const FileContentView: FC<{ content: { path: string; content: string; commit_sha: string } }> = ({ content }) => {
  const lines = content.content.split('\n')
  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <FileText size={16} />
        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontWeight: 600 }}>
          {content.path}
        </Typography>
        {content.commit_sha && (
          <Chip label={content.commit_sha.substring(0, 7)} size="small" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', height: 20 }} />
        )}
        <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto' }}>
          {lines.length} lines
        </Typography>
      </Box>
      <Box
        component="pre"
        sx={{
          fontSize: '0.8rem',
          lineHeight: 1.6,
          fontFamily: 'monospace',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          margin: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.1)',
          borderRadius: 1,
          p: 2,
          maxHeight: 'calc(100vh - 400px)',
          overflow: 'auto',
        }}
      >
        {lines.map((line, i) => (
          <Box key={i} component="span" sx={{ display: 'block' }}>
            <Box component="span" sx={{ color: 'text.disabled', userSelect: 'none', mr: 2, display: 'inline-block', width: 40, textAlign: 'right' }}>
              {i + 1}
            </Box>
            {line}
          </Box>
        ))}
      </Box>
    </Box>
  )
}

// ─── Changelog Sub-tab ──────────────────────────────────────────────────────────

/**
 * Extract a short summary from markdown content: first non-empty line, stripped
 * of leading heading markers, truncated to ~120 chars.
 */
function extractSummary(markdown: string): string {
  const lines = markdown.split('\n')
  for (const line of lines) {
    const trimmed = line.replace(/^#+\s*/, '').trim()
    if (trimmed) {
      return trimmed.length > 120 ? trimmed.substring(0, 117) + '...' : trimmed
    }
  }
  return 'No description'
}

const ChangelogSubTab: FC<{ enrichments: any[] }> = ({ enrichments }) => {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const commitDescriptions = useMemo(() => {
    return enrichments
      .filter((e: any) => e.attributes?.subtype === KODIT_SUBTYPE_COMMIT_DESCRIPTION)
      .sort((a: any, b: any) => {
        const dateA = new Date(a.attributes?.updated_at || 0).getTime()
        const dateB = new Date(b.attributes?.updated_at || 0).getTime()
        return dateB - dateA
      })
  }, [enrichments])

  if (commitDescriptions.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 8 }}>
        <GitCommit size={48} color="#656d76" style={{ marginBottom: 16, opacity: 0.5 }} />
        <Typography variant="h6" color="text.secondary" gutterBottom>
          No commit descriptions yet
        </Typography>
        <Typography variant="body2" color="text.secondary">
          AI-generated commit descriptions will appear here as commits are indexed.
        </Typography>
      </Box>
    )
  }

  return (
    <Box sx={{ position: 'relative', pl: 4 }}>
      {/* Vertical git graph line */}
      <Box
        sx={{
          position: 'absolute',
          left: 11,
          top: 8,
          bottom: 8,
          width: 2,
          bgcolor: 'divider',
        }}
      />

      {commitDescriptions.map((entry: any, index: number) => {
        const content = entry.attributes?.content || ''
        const date = entry.attributes?.updated_at
          ? new Date(entry.attributes.updated_at).toLocaleDateString(undefined, {
              year: 'numeric', month: 'short', day: 'numeric',
            })
          : ''
        const commitSha = entry.commit_sha
        const entryId = entry.id || String(index)
        const isExpanded = expandedId === entryId
        const summary = extractSummary(content)

        return (
          <Box key={entryId} sx={{ position: 'relative', mb: 0.5 }}>
            {/* Commit dot on the line */}
            <Box
              sx={{
                position: 'absolute',
                left: -25,
                top: 12,
                width: 12,
                height: 12,
                borderRadius: '50%',
                bgcolor: isExpanded ? 'primary.main' : 'background.paper',
                border: '2px solid',
                borderColor: isExpanded ? 'primary.main' : 'text.disabled',
                zIndex: 1,
              }}
            />

            {/* Clickable entry */}
            <Box
              onClick={() => setExpandedId(isExpanded ? null : entryId)}
              sx={{
                cursor: 'pointer',
                py: 1,
                px: 2,
                borderRadius: 1,
                '&:hover': { bgcolor: 'rgba(255,255,255,0.04)' },
              }}
            >
              {/* Header row: sha + summary + date */}
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                {isExpanded ? <ChevronDown size={14} style={{ opacity: 0.5, flexShrink: 0 }} /> : <ChevronRight size={14} style={{ opacity: 0.5, flexShrink: 0 }} />}
                {commitSha && (
                  <Typography
                    component="span"
                    sx={{ fontFamily: 'monospace', fontSize: '0.8rem', color: 'primary.main', fontWeight: 600, flexShrink: 0 }}
                  >
                    {commitSha.substring(0, 7)}
                  </Typography>
                )}
                <Typography
                  variant="body2"
                  sx={{
                    flex: 1,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    fontWeight: isExpanded ? 600 : 400,
                  }}
                >
                  {summary}
                </Typography>
                {date && (
                  <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>
                    {date}
                  </Typography>
                )}
              </Box>
            </Box>

            {/* Expanded detail */}
            <Collapse in={isExpanded}>
              <Box
                sx={{
                  ml: 4,
                  mt: 1,
                  mb: 2,
                  pl: 2,
                  borderLeft: '2px solid',
                  borderColor: 'primary.main',
                  ...markdownContentStyles,
                }}
              >
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                  {content}
                </ReactMarkdown>
              </Box>
            </Collapse>
          </Box>
        )
      })}
    </Box>
  )
}

// ─── Connect Sub-tab ────────────────────────────────────────────────────────────

const ConnectSubTab: FC = () => {
  const { data: apiKeys = [] } = useGetUserAPIKeys()
  const userApiKey = (apiKeys as any)?.[0]?.key || ''
  const [showApiKey, setShowApiKey] = useState(false)
  const snackbar = useSnackbar()
  const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''

  const handleCopy = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    snackbar.success(`${label} copied to clipboard`)
  }

  return (
    <Box sx={{ maxWidth: 700 }}>
      <Typography variant="h6" sx={{ fontWeight: 600, mb: 1 }}>
        Connect External MCP Clients
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Connect your local coding agents (Claude Code, Cursor, Cline, Roo Code, Codex, Gemini CLI, Qwen Code, Zed, etc.)
        to access this repository's code intelligence via the Kodit MCP server.
      </Typography>

      <Stack spacing={3}>
        <Box>
          <Typography variant="subtitle2" sx={{ mb: 0.5 }}>MCP Endpoint URL</Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <TextField
              size="small"
              fullWidth
              value={`${baseUrl}/api/v1/mcp/kodit`}
              InputProps={{
                readOnly: true,
                sx: { fontFamily: 'monospace', fontSize: '0.85rem', bgcolor: 'rgba(0, 0, 0, 0.1)' },
              }}
            />
            <IconButton
              size="small"
              onClick={() => handleCopy(`${baseUrl}/api/v1/mcp/kodit`, 'MCP endpoint URL')}
              sx={{ color: '#00d5ff' }}
            >
              <Copy size={16} />
            </IconButton>
          </Box>
        </Box>

        <Box>
          <Typography variant="subtitle2" sx={{ mb: 0.5 }}>Authentication</Typography>
          {userApiKey ? (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <TextField
                size="small"
                fullWidth
                type={showApiKey ? 'text' : 'password'}
                value={userApiKey}
                InputProps={{
                  readOnly: true,
                  sx: { fontFamily: 'monospace', fontSize: '0.85rem', bgcolor: 'rgba(0, 0, 0, 0.1)' },
                  startAdornment: (
                    <Box sx={{ display: 'flex', alignItems: 'center', mr: 1 }}>
                      <Key size={16} color="#00d5ff" />
                    </Box>
                  ),
                }}
              />
              <IconButton size="small" onClick={() => setShowApiKey(!showApiKey)} sx={{ color: '#00d5ff' }}>
                {showApiKey ? <EyeOff size={16} /> : <Eye size={16} />}
              </IconButton>
              <IconButton size="small" onClick={() => handleCopy(userApiKey, 'API key')} sx={{ color: '#00d5ff' }}>
                <Copy size={16} />
              </IconButton>
            </Box>
          ) : (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 1.5, bgcolor: 'rgba(0, 0, 0, 0.1)', borderRadius: 1 }}>
              <Key size={16} color="#00d5ff" />
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                Use your Helix API key in the <code>Authorization: Bearer &lt;api_key&gt;</code> header
              </Typography>
            </Box>
          )}
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
            Add as <code>Authorization: Bearer &lt;api_key&gt;</code> header
          </Typography>
        </Box>

        <Alert severity="info" sx={{ fontSize: '0.8rem' }}>
          <Typography variant="body2">
            <strong>Quick Setup:</strong> In your MCP client configuration, add this server with the URL above and your API key.
            The Kodit MCP server provides tools for semantic code search, wiki documentation, and more.
          </Typography>
        </Alert>
      </Stack>
    </Box>
  )
}

// ─── Main Component ─────────────────────────────────────────────────────────────

const CodeIntelligenceTab: FC<CodeIntelligenceTabProps> = ({ repository, enrichments, repoId, commitSha }) => {
  const snackbar = useSnackbar()
  const [subTab, setSubTab] = useState<SubTab>('wiki')

  const { data: koditStatusData, isLoading: koditStatusLoading, error: koditStatusError } = useKoditStatus(repoId, {
    enabled: !!repoId && repository.kodit_indexing,
  })
  const rescanMutation = useKoditRescan(repoId)
  const { data: commits = [] } = useKoditCommits(repoId, 50, { enabled: !!repoId && repository.kodit_indexing })

  const getRescanCommitSha = (): string | null => {
    if (commitSha) return commitSha
    if (commits.length > 0 && (commits[0] as any)?.id) return (commits[0] as any).id
    return null
  }

  const handleRescan = () => {
    const sha = getRescanCommitSha()
    if (!sha) {
      snackbar.error('No commit available to rescan')
      return
    }
    rescanMutation.mutate(sha, {
      onSuccess: () => snackbar.success('Code intelligence rescan triggered'),
      onError: (error: any) => snackbar.error(error?.message || 'Failed to trigger rescan'),
    })
  }

  if (!repository.kodit_indexing) {
    return (
      <Alert severity="info" sx={{ mb: 4 }}>
        Code Intelligence is not enabled for this repository. Enable it in the Settings tab to start indexing.
      </Alert>
    )
  }

  return (
    <Box sx={{
      maxWidth: 1200,
      '& a': { color: '#00d5ff', textDecoration: 'none', '&:hover': { textDecoration: 'underline' }, '&:visited': { color: '#00d5ff' } },
    }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2, flexWrap: 'wrap' }}>
        <Brain size={24} />
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          Code Intelligence
        </Typography>
        <KoditStatusPill
          data={koditStatusData}
          isLoading={koditStatusLoading}
          error={koditStatusError}
          onRefresh={handleRescan}
          isRefreshing={rescanMutation.isPending}
        />
      </Box>

      {koditStatusData?.message && (
        <Alert
          severity={koditStatusData.status === 'failed' ? 'error' : koditStatusData.status === 'completed_with_errors' ? 'warning' : 'info'}
          sx={{ mb: 2 }}
        >
          {koditStatusData.message}
        </Alert>
      )}

      {/* Sub-tab navigation */}
      <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
        <Tabs
          value={subTab}
          onChange={(_, v) => setSubTab(v)}
          sx={{
            minHeight: 40,
            '& .MuiTab-root': { minHeight: 40, textTransform: 'none', fontWeight: 500 },
          }}
        >
          <Tab icon={<BookOpen size={16} />} iconPosition="start" label="Wiki" value="wiki" />
          <Tab icon={<SearchIcon size={16} />} iconPosition="start" label="Search" value="search" />
          <Tab icon={<GitCommit size={16} />} iconPosition="start" label="Changelog" value="changelog" />
          <Tab icon={<Plug size={16} />} iconPosition="start" label="Connect" value="connect" />
        </Tabs>
      </Box>

      {/* Sub-tab content */}
      {subTab === 'wiki' && (
        <WikiSubTab repoId={repoId} enabled={!!repoId && repository.kodit_indexing} />
      )}
      {subTab === 'search' && (
        <SearchSubTab repoId={repoId} enabled={!!repoId && repository.kodit_indexing} />
      )}
      {subTab === 'changelog' && (
        <ChangelogSubTab enrichments={enrichments} />
      )}
      {subTab === 'connect' && (
        <ConnectSubTab />
      )}
    </Box>
  )
}

export default CodeIntelligenceTab
export { WikiSubTab, SearchSubTab, ChangelogSubTab, ConnectSubTab }
