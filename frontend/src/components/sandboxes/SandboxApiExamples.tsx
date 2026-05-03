import React, { FC, useMemo, useState } from 'react'
import Accordion from '@mui/material/Accordion'
import AccordionDetails from '@mui/material/AccordionDetails'
import AccordionSummary from '@mui/material/AccordionSummary'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import Stack from '@mui/material/Stack'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import { Prism as SyntaxHighlighterPrism } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { Check, Copy } from 'lucide-react'

const SyntaxHighlighter = SyntaxHighlighterPrism as unknown as React.FC<any>

interface SandboxApiExamplesProps {
  orgId: string
  name?: string
  runtime: string
  vcpus: number
  memoryMb: number
  timeoutSeconds: number
  persistent: boolean
  // apiKey is the first organization API key. When undefined, the env-var
  // export snippet falls back to a placeholder and a "create one in settings"
  // hint.
  apiKey?: string
}

type Lang = 'curl' | 'javascript' | 'python'

// Prism language id for each tab. curl is just bash with the curl CLI.
const PRISM_LANG: Record<Lang, string> = {
  curl: 'bash',
  javascript: 'javascript',
  python: 'python',
}

const SECTIONS = [
  { id: 'create', title: 'Create a sandbox', defaultExpanded: true },
  { id: 'run', title: 'Run a command', defaultExpanded: false },
  { id: 'logs', title: 'Stream command logs', defaultExpanded: false },
  { id: 'upload', title: 'Upload a file', defaultExpanded: false },
  { id: 'download', title: 'Download a file', defaultExpanded: false },
  { id: 'delete', title: 'Delete a sandbox', defaultExpanded: false },
] as const

type SectionId = (typeof SECTIONS)[number]['id']

const maskApiKey = (apiKey?: string) => {
  if (!apiKey) {
    return '<PASTE_ORG_API_KEY>'
  }

  return `${apiKey.slice(0, 8)}...`
}

const buildEnvironmentSnippet = (lang: Lang, origin: string, apiKey: string) => {
  if (lang === 'javascript') {
    return `const HELIX_URL = process.env.HELIX_URL ?? '${origin}'
const HELIX_API_KEY = process.env.HELIX_API_KEY // ${apiKey}

if (!HELIX_API_KEY) {
  throw new Error('HELIX_API_KEY is required')
}`
  }

  if (lang === 'python') {
    return `import os

HELIX_URL = os.environ.get("HELIX_URL", "${origin}")
HELIX_API_KEY = os.environ["HELIX_API_KEY"]  # ${apiKey}`
  }

  return `export HELIX_URL="${origin}"
export HELIX_API_KEY="${apiKey}"`
}

interface Examples {
  curl: string
  javascript: string
  python: string
}

const buildExamples = (params: SandboxApiExamplesProps): Record<SectionId, Examples> => {
  const orgId = params.orgId || '<ORG_ID>'
  const sandboxId = '<SANDBOX_ID>'
  const cmdId = '<COMMAND_ID>'
  const remotePath = '/home/retro/work/example.txt'
  const localPath = './example.txt'

  const createBody = JSON.stringify(
    {
      name: params.name || 'my-sandbox',
      runtime: params.runtime,
      vcpus: params.vcpus,
      memory_mb: params.memoryMb,
      timeout_seconds: params.timeoutSeconds,
      persistent: params.persistent,
    },
    null,
    2,
  )

  return {
    create: {
      curl: `curl -X POST "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes" \\
  -H "Authorization: Bearer $HELIX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${createBody}'`,
      javascript: `const res = await fetch(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes\`,
  {
    method: 'POST',
    headers: {
      Authorization: \`Bearer \${HELIX_API_KEY}\`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(${createBody}),
  },
)
const sandbox = await res.json()
console.log(sandbox.id)`,
      python: `import requests

resp = requests.post(
    f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes",
    headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
    json=${createBody},
)
sandbox = resp.json()
print(sandbox["id"])`,
    },
    run: {
      curl: `curl -X POST "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands" \\
  -H "Authorization: Bearer $HELIX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"cmd":"ls","args":["-la","/home"],"detached":true}'`,
      javascript: `const res = await fetch(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands\`,
  {
    method: 'POST',
    headers: {
      Authorization: \`Bearer \${HELIX_API_KEY}\`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      cmd: 'ls',
      args: ['-la', '/home'],
      detached: true,
    }),
  },
)
const { id: commandId } = await res.json()`,
      python: `resp = requests.post(
    f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands",
    headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
    json={"cmd": "ls", "args": ["-la", "/home"], "detached": True},
)
command_id = resp.json()["id"]`,
    },
    logs: {
      curl: `# Stream stdout + stderr as Server-Sent Events.
curl -N "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands/${cmdId}/logs?stream=both&follow=1" \\
  -H "Authorization: Bearer $HELIX_API_KEY"`,
      javascript: `// Use EventSource for live log streaming.
const es = new EventSource(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands/${cmdId}/logs?stream=both&follow=1\`,
  // EventSource doesn't accept custom headers — pass the API key
  // in a query param or proxy the request server-side.
)
es.addEventListener('stdout', (e) => process.stdout.write(JSON.parse(e.data)))
es.addEventListener('stderr', (e) => process.stderr.write(JSON.parse(e.data)))
es.addEventListener('end', () => es.close())`,
      python: `import sseclient

resp = requests.get(
    f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/commands/${cmdId}/logs",
    headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
    params={"stream": "both", "follow": 1},
    stream=True,
)
for event in sseclient.SSEClient(resp).events():
    print(event.event, event.data)`,
    },
    upload: {
      curl: `curl -X PUT "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files?path=${remotePath}" \\
  -H "Authorization: Bearer $HELIX_API_KEY" \\
  --data-binary @${localPath}`,
      javascript: `const data = await fs.promises.readFile('${localPath}')
await fetch(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files?path=\${encodeURIComponent('${remotePath}')}\`,
  {
    method: 'PUT',
    headers: { Authorization: \`Bearer \${HELIX_API_KEY}\` },
    body: data,
  },
)`,
      python: `with open("${localPath}", "rb") as f:
    requests.put(
        f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files",
        headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
        params={"path": "${remotePath}"},
        data=f,
    )`,
    },
    download: {
      curl: `curl -o ${localPath} "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files?path=${remotePath}" \\
  -H "Authorization: Bearer $HELIX_API_KEY"`,
      javascript: `const res = await fetch(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files?path=\${encodeURIComponent('${remotePath}')}\`,
  { headers: { Authorization: \`Bearer \${HELIX_API_KEY}\` } },
)
const buffer = Buffer.from(await res.arrayBuffer())
await fs.promises.writeFile('${localPath}', buffer)`,
      python: `resp = requests.get(
    f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files",
    headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
    params={"path": "${remotePath}"},
)
with open("${localPath}", "wb") as f:
    f.write(resp.content)`,
    },
    delete: {
      curl: `curl -X DELETE "$HELIX_URL/api/v1/organizations/${orgId}/sandboxes/${sandboxId}" \\
  -H "Authorization: Bearer $HELIX_API_KEY"`,
      javascript: `await fetch(
  \`\${HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}\`,
  {
    method: 'DELETE',
    headers: { Authorization: \`Bearer \${HELIX_API_KEY}\` },
  },
)`,
      python: `requests.delete(
    f"{HELIX_URL}/api/v1/organizations/${orgId}/sandboxes/${sandboxId}",
    headers={"Authorization": f"Bearer {HELIX_API_KEY}"},
)`,
    },
  }
}

const CodeBlock: FC<{ code: string; lang: Lang }> = ({ code, lang }) => {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    } catch {
      // ignore — clipboard may be unavailable
    }
  }

  return (
    <Box
      sx={{
        position: 'relative',
        borderRadius: 1,
        border: '1px solid',
        borderColor: 'divider',
        overflow: 'hidden',
        '&:hover .copy-btn': { opacity: 1 },
      }}
    >
      <SyntaxHighlighter
        language={PRISM_LANG[lang]}
        style={oneDark}
        wrapLongLines={false}
        customStyle={{
          margin: 0,
          padding: '12px',
          fontSize: 12,
          lineHeight: 1.5,
          background: 'rgba(0, 0, 0, 0.35)',
          borderRadius: 0,
        }}
        codeTagProps={{ style: { fontFamily: 'monospace' } }}
      >
        {code}
      </SyntaxHighlighter>
      <Tooltip title={copied ? 'Copied' : 'Copy'}>
        <IconButton
          className="copy-btn"
          size="small"
          onClick={handleCopy}
          sx={{
            position: 'absolute',
            top: 4,
            right: 4,
            opacity: 0.6,
            transition: 'opacity 0.15s ease',
            color: '#e5e5e5',
            bgcolor: 'rgba(0, 0, 0, 0.3)',
            '&:hover': { bgcolor: 'rgba(0, 0, 0, 0.5)' },
          }}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
        </IconButton>
      </Tooltip>
    </Box>
  )
}

const SandboxApiExamples: FC<SandboxApiExamplesProps> = (props) => {
  const [lang, setLang] = useState<Lang>('curl')
  const examples = useMemo(() => buildExamples(props), [props])

  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const apiRefHref = `${origin}/api-reference#tag/Sandboxes`
  const apiKeyValue = maskApiKey(props.apiKey)
  const environmentSnippet = buildEnvironmentSnippet(lang, origin, apiKeyValue)

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minWidth: 0 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
          API examples
        </Typography>
        <Link
          href={apiRefHref}
          target="_blank"
          rel="noopener noreferrer"
          sx={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 0.5,
            fontSize: '0.75rem',
            textDecoration: 'none',
          }}
        >
          Full reference
          <OpenInNewIcon sx={{ fontSize: 12 }} />
        </Link>
      </Box>

      <Tabs
        value={lang}
        onChange={(_, v: Lang) => setLang(v)}
        sx={{ minHeight: 32, mb: 1, '& .MuiTab-root': { minHeight: 32, py: 0.5, fontSize: '0.75rem' } }}
      >
        <Tab value="curl" label="curl" />
        <Tab value="javascript" label="JavaScript" />
        <Tab value="python" label="Python" />
      </Tabs>

      <Box sx={{ flex: 1, overflowY: 'auto', pr: 0.5 }}>
        <Stack spacing={1}>
          <Box>
            <Typography
              variant="caption"
              sx={{
                fontWeight: 600,
                color: 'text.secondary',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
                display: 'block',
                mb: 0.5,
              }}
            >
              Environment
            </Typography>
            <CodeBlock code={environmentSnippet} lang={lang} />
            {!props.apiKey && (
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                No org API key found — create one in organization settings, then paste it where shown.
              </Typography>
            )}
          </Box>

          {SECTIONS.map((section) => (
            <Accordion
              key={section.id}
              defaultExpanded={section.defaultExpanded}
              disableGutters
              square
              elevation={0}
              sx={{
                bgcolor: 'transparent',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
                '&::before': { display: 'none' },
                '& + &': { mt: 0 },
              }}
            >
              <AccordionSummary
                expandIcon={<ExpandMoreIcon fontSize="small" />}
                sx={{
                  minHeight: 36,
                  px: 1.5,
                  '& .MuiAccordionSummary-content': { my: 0.5 },
                }}
              >
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 600,
                    color: 'text.secondary',
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                  }}
                >
                  {section.title}
                </Typography>
              </AccordionSummary>
              <AccordionDetails sx={{ p: 1.5, pt: 0 }}>
                <CodeBlock code={examples[section.id][lang]} lang={lang} />
              </AccordionDetails>
            </Accordion>
          ))}
        </Stack>
      </Box>
    </Box>
  )
}

export default SandboxApiExamples
