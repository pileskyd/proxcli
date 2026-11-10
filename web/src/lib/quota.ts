/**
 * Чистые функции экрана «Квоты и расход». Никаких оценок и проекций:
 * только то, что пришло от хаба, и явное «неизвестно» там, где данных нет.
 */
import type { HistoryRange, QuotaAccount, QuotaWindow, RoutingDecision, TokenCounts, UsageBucket } from '../api/types'

export const HOUR = 3_600_000
const MINUTE = 60_000
const DAY = 24 * HOUR

export const RANGE_MS: Record<HistoryRange, number> = {
  '6h': 6 * HOUR,
  '24h': DAY,
  '7d': 7 * DAY,
  '30d': 30 * DAY,
}

/** Время из JSON. Нулевая дата Go и мусор — `null`, а не 1 января 1 года. */
export function parseTime(raw: string | null | undefined): number | null {
  if (!raw) return null
  const t = Date.parse(raw)
  if (!Number.isFinite(t) || t <= 0) return null
  return t
}

export function isPercent(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v)
}

// ── Окна ────────────────────────────────────────────────

const WINDOW_TITLES: Record<string, string> = {
  '5h': '5 часов',
  '7d': '7 дней',
  '7d-sonnet': '7 дней, Sonnet',
  '7d-opus': '7 дней, Opus',
  primary: 'Основное окно',
  secondary: 'Второе окно',
}

/** Человеческое имя окна. Неизвестный ID показываем как есть. */
export function windowTitle(id: string): string {
  return WINDOW_TITLES[id] ?? id
}

const WINDOW_ORDER = ['5h', '7d', '7d-sonnet', '7d-opus', 'primary', 'secondary']

export function compareWindows(a: string, b: string): number {
  const ia = WINDOW_ORDER.indexOf(a)
  const ib = WINDOW_ORDER.indexOf(b)
  return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.localeCompare(b)
}

export type WindowState = 'exhausted' | 'unknown' | 'reset-passed' | 'ok'

/** Состояние окна на момент `now`. Прошедший reset не означает «снова 100%». */
export function windowState(w: QuotaWindow, now: number): WindowState {
  const reset = parseTime(w.resetAt)
  if (reset !== null && reset <= now) return 'reset-passed'
  if (w.exhausted || (isPercent(w.remainingPercent) && w.remainingPercent <= 0)) return 'exhausted'
  if (!isPercent(w.remainingPercent)) return 'unknown'
  return 'ok'
}

export function clampPercent(v: number): number {
  return Math.min(100, Math.max(0, v))
}

// ── Источник и свежесть ─────────────────────────────────

export function sourceLabel(source: string): string {
  switch (source) {
    case 'provider-usage-api':
      return 'usage API провайдера'
    case 'response-headers':
      return 'заголовки ответов'
    case '':
      return 'источник неизвестен'
    default:
      return source
  }
}

export type Freshness = { kind: 'never' } | { kind: 'fresh' | 'stale' | 'future'; ageMs: number }

export function freshness(observedAt: string, now: number, staleAfterSeconds: number): Freshness {
  const t = parseTime(observedAt)
  if (t === null) return { kind: 'never' }
  const ageMs = now - t
  if (ageMs < -MINUTE) return { kind: 'future', ageMs }
  return { kind: ageMs > staleAfterSeconds * 1000 ? 'stale' : 'fresh', ageMs: Math.max(0, ageMs) }
}

export interface AccountSummary {
  /** Есть свежее общее окно с положительным остатком и ни одно общее окно не исчерпано. */
  available: boolean
  exhausted: boolean
  unknown: boolean
  /** Ближайший будущий сброс общего окна с подтверждённым остатком. */
  soonestReset: number | null
}

/**
 * Сводка аккаунта для счётчиков панели. Модельные окна (Sonnet/Opus) не влияют на
 * общую доступность — так же их трактует контроллер приоритетов.
 */
export function summarizeAccount(a: QuotaAccount, now: number, staleAfterSeconds: number): AccountSummary {
  const fresh = freshness(a.observedAt, now, staleAfterSeconds).kind === 'fresh'
  const general = (a.windows ?? []).filter((w) => !w.model)
  let exhausted = false
  let positive = false
  let soonestReset: number | null = null
  if (fresh) {
    for (const w of general) {
      const st = windowState(w, now)
      if (st === 'exhausted') exhausted = true
      if (st === 'ok') {
        positive = true
        const reset = parseTime(w.resetAt)
        if (reset !== null && (soonestReset === null || reset < soonestReset)) soonestReset = reset
      }
    }
  }
  const available = !a.disabled && fresh && positive && !exhausted
  return {
    available,
    exhausted: !a.disabled && exhausted,
    unknown: !a.disabled && !available && !exhausted,
    soonestReset: exhausted ? null : soonestReset,
  }
}

// ── История остатка ─────────────────────────────────────

export interface QuotaPoint {
  t: number
  /** Остаток в процентах; `null` — окно было в наблюдении, но процент неизвестен. */
  v: number | null
  resetAt: number | null
  exhausted: boolean
}

/**
 * Точки остатка по аккаунтам для одного окна, по возрастанию времени.
 * Повторы одного наблюдения схлопываются; ничего не интерполируется.
 */
export function quotaSeries(quotas: readonly QuotaAccount[], windowId: string): Map<string, QuotaPoint[]> {
  const out = new Map<string, QuotaPoint[]>()
  for (const a of quotas) {
    const t = parseTime(a.observedAt)
    if (t === null) continue
    const w = (a.windows ?? []).find((x) => x.id === windowId)
    if (!w) continue
    const v = isPercent(w.remainingPercent) ? clampPercent(w.remainingPercent) : w.exhausted ? 0 : null
    const list = out.get(a.authId) ?? []
    const last = list[list.length - 1]
    if (last && last.t === t) continue
    list.push({ t, v, resetAt: parseTime(w.resetAt), exhausted: w.exhausted })
    out.set(a.authId, list)
  }
  for (const list of out.values()) list.sort((x, y) => x.t - y.t)
  return out
}

/** ID окон, встречающихся в истории и текущем снимке, в стабильном порядке. */
export function windowIds(...sources: readonly (readonly QuotaAccount[])[]): string[] {
  const ids = new Set<string>()
  for (const list of sources) for (const a of list) for (const w of a.windows ?? []) ids.add(w.id)
  return [...ids].sort(compareWindows)
}

/**
 * Непрерывные участки линии. Разрыв — неизвестный процент или пауза между
 * наблюдениями дольше `maxGapMs`: что было в паузе, мы не знаем.
 */
export function segments(points: readonly QuotaPoint[], maxGapMs: number): { t: number; v: number }[][] {
  const out: { t: number; v: number }[][] = []
  let cur: { t: number; v: number }[] = []
  let prevT = -Infinity
  for (const p of points) {
    if (p.v === null || p.t - prevT > maxGapMs) {
      if (cur.length) out.push(cur)
      cur = []
    }
    if (p.v !== null) cur.push({ t: p.t, v: p.v })
    prevT = p.t
  }
  if (cur.length) out.push(cur)
  return out
}

/** Максимальная пауза между наблюдениями, которую ещё соединяем линией. */
export function maxGapFor(rangeMs: number): number {
  return Math.max(20 * MINUTE, rangeMs / 120)
}

export interface ResetJump {
  t: number
  v: number
  delta: number
}

/** Наблюдённые скачки остатка вверх после смены времени сброса. */
export function resetJumps(points: readonly QuotaPoint[], maxGapMs: number): ResetJump[] {
  const out: ResetJump[] = []
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1]!
    const b = points[i]!
    if (a.v === null || b.v === null || b.t - a.t > maxGapMs) continue
    const resetChanged = a.resetAt !== b.resetAt
    const delta = b.v - a.v
    if (resetChanged && delta >= 5) out.push({ t: b.t, v: b.v, delta })
  }
  return out
}

/** Последнее наблюдение не позже `t` и не дальше `maxGapMs` от него. */
export function pointAt(points: readonly QuotaPoint[], t: number, maxGapMs: number): QuotaPoint | null {
  let found: QuotaPoint | null = null
  for (const p of points) {
    if (p.t > t) break
    found = p
  }
  if (!found || t - found.t > maxGapMs) return null
  return found
}

// ── Расход ──────────────────────────────────────────────

export type UsageMetric = 'total' | 'input' | 'output' | 'reasoning' | 'cacheRead' | 'cacheWrite' | 'requests'

export const USAGE_METRICS: readonly { value: UsageMetric; label: string }[] = [
  { value: 'total', label: 'Всего токенов' },
  { value: 'input', label: 'Ввод без кеша' },
  { value: 'output', label: 'Вывод' },
  { value: 'cacheRead', label: 'Чтение кеша' },
  { value: 'cacheWrite', label: 'Запись кеша' },
  { value: 'requests', label: 'Запросы' },
]

export function bucketValue(b: UsageBucket, metric: UsageMetric): number {
  const n = metric === 'requests' ? b.requests : b.tokens?.[metric]
  return typeof n === 'number' && Number.isFinite(n) && n > 0 ? n : 0
}

export interface UsageTotals extends TokenCounts {
  requests: number
  failed: number
}

/** Сумма одного счётчика по часам периода. Разные счётчики между собой не складываются. */
export function usageTotals(buckets: readonly UsageBucket[]): UsageTotals {
  const t: UsageTotals = { requests: 0, failed: 0, input: 0, output: 0, reasoning: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
  for (const b of buckets) {
    t.requests += bucketValue(b, 'requests')
    t.failed += typeof b.failed === 'number' && b.failed > 0 ? b.failed : 0
    for (const k of ['input', 'output', 'reasoning', 'cacheRead', 'cacheWrite', 'total'] as const) t[k] += bucketValue(b, k)
  }
  return t
}

// ── Маршрутизация ───────────────────────────────────────

const REASONS: Record<string, string> = {
  'soonest-reset': 'Ближайший сброс',
  'unknown-quota': 'Квота неизвестна',
  'quota-exhausted': 'Квота исчерпана',
  disabled: 'Отключён',
  'first-observed-session': 'Сессия впервые замечена на аккаунте',
  'observed-account-change': 'Сессия замечена на другом аккаунте',
}

export function reasonLabel(reason: string): string {
  if (reason.startsWith('priority-updated:')) {
    const rest = reason.slice('priority-updated:'.length)
    return `Приоритет обновлён: ${(REASONS[rest] ?? rest).toLowerCase()}`
  }
  return REASONS[reason] ?? reason
}

export function decisionKind(d: RoutingDecision): 'priority' | 'session' | 'other' {
  if (d.reason.startsWith('priority-updated')) return 'priority'
  if (d.reason === 'first-observed-session' || d.reason === 'observed-account-change') return 'session'
  return 'other'
}

// ── Форматирование ──────────────────────────────────────

export function formatPercent(v: number): string {
  if (v > 0 && v < 1) return '<1%'
  if (v < 100 && v > 99) return '>99%'
  return `${Math.round(v)}%`
}

const nf = new Intl.NumberFormat('ru-RU')

export function formatCount(n: number): string {
  if (n >= 1e9) return `${trim(n / 1e9)} млрд`
  if (n >= 1e6) return `${trim(n / 1e6)} млн`
  if (n >= 1e4) return `${trim(n / 1e3)} тыс.`
  return nf.format(n)
}

export function formatExact(n: number): string {
  return nf.format(n)
}

function trim(x: number): string {
  return (x >= 100 ? Math.round(x).toString() : x.toFixed(1).replace(/\.0$/, '')).replace('.', ',')
}

/** «2 ч 14 мин», «3 дн 4 ч», «45 с». */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s} с`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} мин`
  const h = Math.floor(m / 60)
  if (h < 48) return m % 60 ? `${h} ч ${m % 60} мин` : `${h} ч`
  const d = Math.floor(h / 24)
  return h % 24 ? `${d} дн ${h % 24} ч` : `${d} дн`
}

export function formatAgo(ms: number): string {
  return ms < 30_000 ? 'только что' : `${formatDuration(ms)} назад`
}

const timeFmt = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' })
const dayFmt = new Intl.DateTimeFormat('ru-RU', { weekday: 'short', day: 'numeric', month: 'short' })
const shortDayFmt = new Intl.DateTimeFormat('ru-RU', { weekday: 'short', day: 'numeric' })

function sameDay(a: number, b: number): boolean {
  const x = new Date(a)
  const y = new Date(b)
  return x.getFullYear() === y.getFullYear() && x.getMonth() === y.getMonth() && x.getDate() === y.getDate()
}

/** «сегодня 14:05», «завтра 09:00», «пт, 14 нояб. 09:00». */
export function formatMoment(t: number, now: number): string {
  const time = timeFmt.format(t)
  if (sameDay(t, now)) return `сегодня ${time}`
  if (sameDay(t, now + DAY)) return `завтра ${time}`
  if (sameDay(t, now - DAY)) return `вчера ${time}`
  return `${dayFmt.format(t)} ${time}`
}

// ── Оси графиков ────────────────────────────────────────

export interface Tick {
  t: number
  label: string
}

const TICK_STEPS = [HOUR, 2 * HOUR, 3 * HOUR, 6 * HOUR, 12 * HOUR, DAY, 2 * DAY, 3 * DAY, 5 * DAY, 7 * DAY]

/** Метки времени по локальным границам часов и суток, не больше `maxTicks`. */
export function timeTicks(from: number, to: number, maxTicks: number): Tick[] {
  const span = to - from
  if (!(span > 0) || maxTicks < 1) return []
  const step = TICK_STEPS.find((s) => span / s <= maxTicks) ?? TICK_STEPS[TICK_STEPS.length - 1]!
  const out: Tick[] = []
  const start = new Date(from)
  if (step < DAY) {
    start.setMinutes(0, 0, 0)
    const hours = step / HOUR
    while (start.getTime() < from || start.getHours() % hours !== 0) start.setHours(start.getHours() + 1)
  } else {
    start.setHours(0, 0, 0, 0)
    if (start.getTime() < from) start.setDate(start.getDate() + 1)
  }
  for (const d = start; d.getTime() <= to; ) {
    const t = d.getTime()
    const midnight = d.getHours() === 0 && d.getMinutes() === 0
    out.push({ t, label: step >= DAY || (midnight && span > 12 * HOUR) ? shortDayFmt.format(t) : timeFmt.format(t) })
    if (step < DAY) d.setHours(d.getHours() + step / HOUR)
    else d.setDate(d.getDate() + step / DAY)
  }
  return out
}

/** «Красивая» верхняя граница оси и шаг для неотрицательных значений. */
export function niceScale(max: number, ticks = 4): { max: number; step: number } {
  if (!(max > 0)) return { max: 1, step: 1 }
  const raw = max / ticks
  const mag = 10 ** Math.floor(Math.log10(raw))
  const norm = raw / mag
  const step = (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 2.5 ? 2.5 : norm <= 5 ? 5 : 10) * mag
  return { max: Math.ceil(max / step) * step, step }
}
