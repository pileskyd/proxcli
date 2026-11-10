// Documentation/demo server only. Never contacts a real provider or reads account files.
import { resolve, sep } from 'node:path'
const root = resolve('web/dist')
const names = [
  { id: 'demo-claude-main', name: 'Claude Max — основной', email: 'main@example.invalid', provider: 'claude', host: '203.0.113.21' },
  { id: 'demo-claude-work', name: 'Claude Max — рабочий', email: 'work@example.invalid', provider: 'claude', host: '203.0.113.22' },
  { id: 'demo-codex', name: 'Codex — разработка', email: 'codex@example.invalid', provider: 'codex', host: '203.0.113.23' },
]
const iso = (ms) => new Date(ms).toISOString()
const minute = 60_000, hour = 60 * minute
function windowAt(account, time, index) {
  const duration = account.provider === 'claude' ? 5 * hour : 6 * hour
  const offset = index * hour
  const progress = ((time + offset) % duration) / duration
  return { id: account.provider === 'claude' ? '5h' : 'primary', remainingPercent: Math.round(100 - progress * 77), resetAt: iso(time + (1 - progress) * duration), exhausted: false }
}
function snapshot(time) {
  return { version: 1, updatedAt: iso(time), accounts: names.map((a, i) => ({ authId: a.id, provider: a.provider, disabled: false, observedAt: iso(time - minute), source: 'demo', windows: [windowAt(a, time, i), { id: a.provider === 'claude' ? '7d' : 'secondary', remainingPercent: [71, 43, 86][i], resetAt: iso(time + (i + 1) * 24 * hour), exhausted: false }] })) }
}
const server = Bun.serve({
  hostname: '127.0.0.1', port: Number(process.env.PROXCLI_DEMO_PORT ?? 8799),
  async fetch(request) {
    const url = new URL(request.url), time = Date.now()
    const json = (body, status = 200) => Response.json(body, { status, headers: { 'Cache-Control': 'no-store' } })
    if (url.pathname === '/api/status') return json({ connected: true, demo: true, upstream: 'http://demo-hub.local:8317', version: '8.0.63 · demo' })
    if (url.pathname === '/api/subscriptions' && request.method === 'GET') return json({ defaultProxy: '', subscriptions: names.map((a) => ({ ...a, status: 'active', disabled: false, proxyMode: 'custom', proxyUrl: `socks5h://demo:***@${a.host}:1080`, proxyKnown: true, proxyConfigured: true, prefix: '' })) })
    if (url.pathname === '/api/models') return json({ data: [{ id: 'claude-demo' }, { id: 'codex-demo' }] })
    if (url.pathname === '/api/quotas') return json({ snapshot: snapshot(time), collecting: true, error: '', staleAfterSeconds: 600 })
    if (url.pathname === '/api/routing') return json({ active: true, enabled: true, strategy: 'soonest-reset-first', policy: 'quota-strict-v1', updatedAt: iso(time), accounts: names.map((a, i) => ({ authId: a.id, provider: a.provider, priority: 10000 - i, resetAt: windowAt(a, time, i).resetAt, reason: 'soonest-reset' })), usageObserver: { active: true, updatedAt: iso(time), dropped: 0, writeErrors: 0, queued: 0, lastHandledUnix: Math.floor(time / 1000) } })
    if (url.pathname === '/api/history') {
      const duration = { '6h': 6, '24h': 24, '7d': 168, '30d': 720 }[url.searchParams.get('range') ?? '24h'] ?? 24
      const provider = url.searchParams.get('provider'), authId = url.searchParams.get('authId')
      const selected = names.filter((a) => (!provider || a.provider === provider) && (!authId || a.id === authId))
      const from = time - duration * hour, start = Math.max(from, time - 24 * hour)
      const quotas = []
      for (let t = start; t <= time; t += 5 * minute) {
        for (const a of snapshot(t).accounts) if (selected.some((n) => n.id === a.authId)) quotas.push(a)
      }
      const buckets = []
      for (let i = 0; i < Math.min(duration, 24); i++) {
        const scale = selected.length, input = (1400 + i * 35) * scale, output = (600 + i * 20) * scale
        const cacheRead = (15000 + (i % 5) * 1700) * scale, cacheWrite = (i % 6 === 0 ? 7500 : 250) * scale
        buckets.push({ at: iso(time - (Math.min(duration, 24) - i) * hour), requests: (12 + i % 7) * scale, failed: i % 9 === 0 ? 1 : 0, tokens: { input, output, reasoning: 120 * scale, cacheRead, cacheWrite, total: input + output + cacheRead + cacheWrite } })
      }
      return json({ from: iso(from), to: iso(time), buckets, quotas, decisions: selected.map((a) => ({ at: iso(time - 2 * hour), authId: a.id, provider: a.provider, session: '', model: '', reason: 'priority-updated:soonest-reset' })), truncated: false })
    }
    if (url.pathname === '/api/telemetry/refresh') return json({ ok: true, message: 'Демонстрационные данные обновлены. Реальные аккаунты не используются.' }, 202)
    if (url.pathname.startsWith('/api/')) return json({ error: 'В демонстрации изменение аккаунтов и OAuth отключены.' }, 403)
    const path = resolve(root, '.' + decodeURIComponent(url.pathname))
    if (path !== root && !path.startsWith(root + sep)) return new Response('Forbidden', { status: 403 })
    const file = Bun.file(path)
    if (await file.exists()) return new Response(file)
    return new Response(Bun.file(resolve(root, 'index.html')))
  },
})
console.log(`ProxCli demo: http://127.0.0.1:${server.port}/#/quotas — только демонстрационные данные`)
