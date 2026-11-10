import { test, expect } from 'bun:test'
import { freshness, quotaSeries, segments, summarizeAccount, usageTotals, windowState, parseTime, resetJumps, reasonLabel, timeTicks } from '../src/lib/quota.ts'

const now = Date.parse('2026-11-10T12:00:00Z')
const iso = (ms) => new Date(ms).toISOString()

test('go zero time is "never observed", stale after threshold', () => {
  expect(parseTime('0001-01-01T00:00:00Z')).toBe(null)
  expect(freshness('0001-01-01T00:00:00Z', now, 600).kind).toBe('never')
  expect(freshness(iso(now - 5 * 60e3), now, 600).kind).toBe('fresh')
  expect(freshness(iso(now - 11 * 60e3), now, 600).kind).toBe('stale')
})

test('unknown percent and passed reset are never shown as available', () => {
  expect(windowState({ id: '5h', remainingPercent: null, resetAt: null, exhausted: false }, now)).toBe('unknown')
  expect(windowState({ id: '5h', remainingPercent: 80, resetAt: iso(now - 1000), exhausted: false }, now)).toBe('reset-passed')
  expect(windowState({ id: '5h', remainingPercent: 0, resetAt: iso(now + 1000), exhausted: false }, now)).toBe('exhausted')
  const a = { authId: 'a', provider: 'claude', disabled: false, observedAt: iso(now - 60e3), source: 'x', windows: [
    { id: '5h', remainingPercent: 40, resetAt: iso(now + 3600e3), exhausted: false },
    { id: '7d-opus', model: 'opus', remainingPercent: 0, resetAt: iso(now + 7200e3), exhausted: true },
  ] }
  const s = summarizeAccount(a, now, 600)
  expect(s.available).toBe(true) // model-specific window does not block the account
  expect(s.soonestReset).toBe(now + 3600e3)
  expect(summarizeAccount({ ...a, observedAt: iso(now - 3600e3) }, now, 600).unknown).toBe(true)
})

test('series keep gaps and unknowns instead of interpolating', () => {
  const obs = (t, v, reset) => ({ authId: 'a', provider: 'codex', disabled: false, observedAt: iso(t), source: '', windows: [{ id: 'primary', remainingPercent: v, resetAt: iso(reset), exhausted: false }] })
  const pts = quotaSeries([obs(now - 3e6, 50, now), obs(now - 2.9e6, 40, now), obs(now - 2.8e6, null, now), obs(now - 2.7e6, 30, now), obs(now - 0.5e6, 100, now + 9e6)], 'primary').get('a')
  expect(pts.length).toBe(5)
  const segs = segments(pts, 20 * 60e3)
  expect(segs.map((s) => s.length)).toEqual([2, 1, 1])
  expect(resetJumps(pts, 20 * 60e3)).toEqual([])
  expect(resetJumps(pts, 60 * 60e3)[0].delta).toBe(70)
})

test('totals sum one counter across buckets, never across counters', () => {
  const t = usageTotals([
    { at: iso(now), requests: 2, failed: 1, tokens: { input: 10, output: 5, reasoning: 0, cacheRead: 100, cacheWrite: 3, total: 15 } },
    { at: iso(now + 3600e3), requests: 1, failed: 0, tokens: { input: 1, output: 1, reasoning: 0, cacheRead: 0, cacheWrite: 0, total: 2 } },
  ])
  expect(t).toEqual({ requests: 3, failed: 1, input: 11, output: 6, reasoning: 0, cacheRead: 100, cacheWrite: 3, total: 17 })
})

test('labels and ticks', () => {
  expect(reasonLabel('priority-updated:soonest-reset')).toBe('Приоритет обновлён: ближайший сброс')
  expect(reasonLabel('observed-account-change')).not.toContain('failover')
  const ticks = timeTicks(now - 24 * 3600e3, now, 8)
  expect(ticks.length).toBeGreaterThan(3)
  expect(ticks.length).toBeLessThanOrEqual(9)
})
