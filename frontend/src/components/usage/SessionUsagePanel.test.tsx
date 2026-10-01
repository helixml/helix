import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { SessionUsageView, buildUsageRows, formatCost, formatSeconds, formatTokens } from './SessionUsagePanel'
import type { SessionUsage } from '../../services/sessionUsageService'

// Real /api/v1/sessions/{id}/usage response from a DHRE broker bot instance
// (calls trimmed to six; summary and turns as returned).
const usage: SessionUsage = {
  "session_id": "ses_01m3c4q3xa9crdsvak92km31ex",
  "summary": {
    "calls": 18,
    "prompt_tokens": 567130,
    "completion_tokens": 11644,
    "cache_read_tokens": 523776,
    "cache_write_tokens": 0,
    "cache_hit_ratio": 0.9235554458413415,
    "total_cost": 0.038513900000000004,
    "llm_ms": 117275,
    "ttft_p50_ms": 2765,
    "ttft_p90_ms": 3304,
    "duration_p50_ms": 3648,
    "duration_p90_ms": 11885,
    "models": [
      "glm-5.3-flash"
    ]
  },
  "calls": [
    {
      "created": "2026-09-25T11:20:42.309705Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 3456,
      "time_to_first_token_ms": 668,
      "prompt_tokens": 641,
      "completion_tokens": 458,
      "cache_read_tokens": 0,
      "cache_write_tokens": 0,
      "total_cost": 0.00032514999999999996
    },
    {
      "created": "2026-09-25T11:20:42.573058Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 5884,
      "time_to_first_token_ms": 3304,
      "prompt_tokens": 15044,
      "completion_tokens": 375,
      "cache_read_tokens": 0,
      "cache_write_tokens": 0,
      "total_cost": 0.0024441000000000003
    },
    {
      "created": "2026-09-25T11:20:48.505876Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 2938,
      "time_to_first_token_ms": 978,
      "prompt_tokens": 15478,
      "completion_tokens": 307,
      "cache_read_tokens": 15360,
      "cache_write_tokens": 0,
      "total_cost": 0.0009392000000000001
    },
    {
      "created": "2026-09-25T11:20:55.366036Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 9637,
      "time_to_first_token_ms": 3275,
      "prompt_tokens": 21532,
      "completion_tokens": 894,
      "cache_read_tokens": 15616,
      "cache_write_tokens": 0,
      "total_cost": 0.0021152
    },
    {
      "created": "2026-09-25T11:24:39.663147Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 3648,
      "time_to_first_token_ms": 2949,
      "prompt_tokens": 50071,
      "completion_tokens": 99,
      "cache_read_tokens": 49408,
      "cache_write_tokens": 0,
      "total_cost": 0.00261935
    },
    {
      "created": "2026-09-25T11:24:43.687953Z",
      "interaction_id": "n/a",
      "model": "glm-5.3-flash",
      "duration_ms": 8150,
      "time_to_first_token_ms": 2899,
      "prompt_tokens": 50951,
      "completion_tokens": 816,
      "cache_read_tokens": 49920,
      "cache_write_tokens": 0,
      "total_cost": 0.00305865
    }
  ],
  "turns": [
    {
      "interaction_id": "int_01m3c4qh749nfq58epsw2j71wb",
      "prompt": "Hi! I'd like to register my real estate agency with Dubai Ho",
      "started": "2026-09-25T11:20:39.396981Z",
      "completed": "2026-09-25T11:21:29.084022Z",
      "state": "complete",
      "calls": 9,
      "prompt_tokens": 204567,
      "completion_tokens": 3130,
      "cache_read_tokens": 174336,
      "cache_hit_ratio": 0.852219566205693,
      "llm_ms": 43447,
      "total_cost": 0.01481645
    },
    {
      "interaction_id": "int_01m3c4wq9fj1wmtvc49hnkez46",
      "prompt": "199143 \u2014 no branch on the letter, use Main Branch. The other",
      "started": "2026-09-25T11:23:29.455287Z",
      "completed": "2026-09-25T11:24:52.209372Z",
      "state": "complete",
      "calls": 9,
      "prompt_tokens": 362563,
      "completion_tokens": 8514,
      "cache_read_tokens": 349440,
      "cache_hit_ratio": 0.9638049111464766,
      "llm_ms": 73828,
      "total_cost": 0.02369745
    }
  ]
}

describe('SessionUsagePanel', () => {
  it('shows spend, tokens, cache hit and LLM time for the session', () => {
    render(<SessionUsageView usage={usage} />)
    expect(screen.getByText('$0.0385')).toBeTruthy()
    expect(screen.getAllByText('578.8k').length).toBeGreaterThan(0)
    expect(screen.getAllByText('92%').length).toBeGreaterThan(0)
    expect(screen.getByText('1m 57s')).toBeTruthy()
    expect(screen.getByText('Per turn')).toBeTruthy()
    expect(screen.getAllByRole('row')).toHaveLength(1 + usage.turns.length)
  })

  it('says cost is n/a when the model has no price', () => {
    render(<SessionUsageView usage={{ ...usage, summary: { ...usage.summary, total_cost: 0 } }} />)
    expect(screen.getByText('n/a')).toBeTruthy()
    expect(screen.getByText('no price for this model')).toBeTruthy()
  })

  it('handles a session with no LLM calls yet', () => {
    const empty: SessionUsage = { ...usage, calls: [], turns: [], summary: { ...usage.summary, calls: 0 } }
    render(<SessionUsageView usage={empty} />)
    expect(screen.getByText('No LLM calls recorded for this session yet.')).toBeTruthy()
  })

  it('builds per-call chart rows: uncached input, cache hit ratio, seconds', () => {
    const rows = buildUsageRows(usage)
    expect(rows.tokens).toHaveLength(usage.calls.length)
    const c = usage.calls[1]
    expect(rows.tokens[1].input).toBe(Math.max(c.prompt_tokens - c.cache_read_tokens, 0))
    expect(rows.latency[1].duration).toBeCloseTo(c.duration_ms / 1000)
    expect(rows.cache.every(r => r.hit >= 0 && r.hit <= 1)).toBe(true)
  })

  it('formats values compactly', () => {
    expect(formatTokens(567130)).toBe('567.1k')
    expect(formatTokens(1_500_000)).toBe('1.5M')
    expect(formatCost(0.0385139)).toBe('$0.0385')
    expect(formatCost(12.5)).toBe('$12.50')
    expect(formatSeconds(117275)).toBe('1m 57s')
    expect(formatSeconds(2765)).toBe('2.8s')
  })
})
