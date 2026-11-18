import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { SessionResourcesView, buildResourceRows, formatBytes } from './SessionResourcesPanel'
import type { SessionResources } from '../../services/sessionResourcesService'

const GiB = 1024 ** 3

const resources = (overrides: Partial<SessionResources['summary']> = {}, filesystems: SessionResources['filesystems'] = []): SessionResources => ({
  session_id: 'ses_1',
  summary: {
    from: 0, to: 60000, samples: 3,
    cpu_limit_cores: 12, cpu_cores_p50: 0.5, cpu_cores_p95: 3.25, cpu_cores_max: 4,
    cpu_throttled_pct: 0, memory_limit_bytes: 24 * GiB, memory_max_bytes: 3 * GiB, memory_peak_bytes: 4 * GiB,
    cpu_pressure_pct: 1.5, memory_pressure_pct: 0, io_pressure_pct: 93.9,
    io_read_bytes: GiB, io_write_bytes: 0.5 * GiB, memory_high_events: 0, oom_kills: 2,
    ...overrides,
  },
  points: [
    { timestamp: 30000, cpu_cores: 0.5, cpu_throttled_pct: 0, memory_bytes: 2 * GiB, cpu_pressure_pct: 1, memory_pressure_pct: 0, io_pressure_pct: 90, io_read_bps: 1024, io_write_bps: 0, disk_used_bytes: 0, oom_kills: 0 },
    { timestamp: 60000, cpu_cores: 4, cpu_throttled_pct: 0, memory_bytes: 3 * GiB, cpu_pressure_pct: 2, memory_pressure_pct: 0, io_pressure_pct: 97, io_read_bps: 2048, io_write_bps: 512, disk_used_bytes: 0, oom_kills: 2 },
  ],
  filesystems,
})

describe('SessionResourcesView', () => {
  it('shows what the sandbox needs against its limits and where it stalls', () => {
    render(<SessionResourcesView resources={resources()} />)
    expect(screen.getByText('3.25 cores')).toBeTruthy()
    expect(screen.getByText('max 4.00 · limit 12.0')).toBeTruthy()
    expect(screen.getAllByText('3.0 GB').length).toBeGreaterThan(0)
    expect(screen.getByText('limit 24 GB')).toBeTruthy()
    expect(screen.getByText('94%')).toBeTruthy()
    expect(screen.getByText('2 OOM kills')).toBeTruthy()
  })

  it('says when there is no limit and when disk use is not per session', () => {
    render(
      <SessionResourcesView
        resources={resources({ cpu_limit_cores: 0, memory_limit_bytes: 0 }, [
          { paths: '/home/retro/work, /workspace', dedicated: false, size_bytes: 1000 * GiB, used_bytes: 700 * GiB, inodes: 100, inodes_used: 25 },
        ])}
      />,
    )
    expect(screen.getByText('max 4.00 · no limit')).toBeTruthy()
    expect(screen.getByText('shared host volume')).toBeTruthy()
    expect(screen.getByText(/per-session disk use is not measured/)).toBeTruthy()
  })

  it('charts usage per point', () => {
    const rows = buildResourceRows(resources())
    expect(rows[1].cpu).toEqual({ used: 4 })
    expect(rows[1].memory).toEqual({ used: 3 * GiB })
    expect(rows[1].stalls.io).toBe(97)
  })

  it('explains an empty window', () => {
    render(<SessionResourcesView resources={{ ...resources({ samples: 0 }), points: [] }} />)
    expect(screen.getByText(/No sandbox samples in this window/)).toBeTruthy()
  })

  it('does not call a window with samples but no intervals empty', () => {
    render(<SessionResourcesView resources={{ ...resources({ samples: 1 }), points: [] }} />)
    expect(screen.getByText(/1 sample in this window, not yet enough/)).toBeTruthy()
  })

  it('formats bytes', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(24 * GiB)).toBe('24 GB')
  })
})
