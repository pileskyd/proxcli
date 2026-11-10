<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { api, errorMessage, isAbort } from '../api/client'
import type {
  HistoryRange,
  HistoryResponse,
  QuotaAccount,
  QuotasResponse,
  RoutingAccount,
  RoutingDecision,
  RoutingStatus,
  Subscription,
} from '../api/types'
import AppIcon from '../components/AppIcon.vue'
import CodeBlock from '../components/CodeBlock.vue'
import ProviderGlyph from '../components/ProviderGlyph.vue'
import QuotaHistoryChart, { type QuotaChartSeries } from '../components/QuotaHistoryChart.vue'
import SegmentedFilter from '../components/SegmentedFilter.vue'
import UsageBarsChart from '../components/UsageBarsChart.vue'
import { useHub } from '../composables/useHub'
import { useToasts } from '../composables/useToasts'
import { plural, providerColor, providerKey, providerLabel } from '../lib/providers'
import {
  USAGE_METRICS,
  clampPercent,
  compareWindows,
  decisionKind,
  formatAgo,
  formatCount,
  formatDuration,
  formatExact,
  formatMoment,
  formatPercent,
  freshness,
  maxGapFor,
  parseTime,
  quotaSeries,
  reasonLabel,
  sourceLabel,
  summarizeAccount,
  usageTotals,
  windowIds,
  windowState,
  windowTitle,
  type UsageMetric,
} from '../lib/quota'
import { href } from '../lib/router'

type Prov = 'claude' | 'codex'
type Tab = 'all' | Prov
const PROVIDERS: Prov[] = ['claude', 'codex']

const hub = useHub()
const { push } = useToasts()

const nAccounts = (n: number) => `${n} ${plural(n, 'аккаунт', 'аккаунта', 'аккаунтов')}`

// ── Часы страницы: относительное время пересчитывается без запросов ──
const now = ref(Date.now())

// ── Данные ───────────────────────────────────────────
interface Res<T> {
  data: T | null
  loading: boolean
  error: string
}
const quotas = reactive<Res<QuotasResponse>>({ data: null, loading: false, error: '' })
const routing = reactive<Res<RoutingStatus>>({ data: null, loading: false, error: '' })
const history = reactive<Record<Prov, Res<HistoryResponse> & { range: HistoryRange | null }>>({
  claude: { data: null, loading: false, error: '', range: null },
  codex: { data: null, loading: false, error: '', range: null },
})

const ctl: Partial<Record<'quotas' | 'routing' | Prov, AbortController>> = {}
function take(key: keyof typeof ctl): AbortController {
  ctl[key]?.abort()
  const c = new AbortController()
  ctl[key] = c
  return c
}

async function loadQuotas(): Promise<QuotasResponse | null> {
  const c = take('quotas')
  quotas.loading = true
  try {
    const data = await api.quotas(c.signal)
    quotas.data = data
    quotas.error = ''
    return data
  } catch (err) {
    if (isAbort(err)) return null
    quotas.error = errorMessage(err)
    return null
  } finally {
    if (ctl.quotas === c) quotas.loading = false
  }
}

async function loadRouting(): Promise<void> {
  const c = take('routing')
  routing.loading = true
  try {
    routing.data = await api.routing(c.signal)
    routing.error = ''
  } catch (err) {
    if (!isAbort(err)) routing.error = errorMessage(err)
  } finally {
    if (ctl.routing === c) routing.loading = false
  }
}

async function loadHistory(p: Prov): Promise<void> {
  const c = take(p)
  const h = history[p]
  const r = range.value
  h.loading = true
  try {
    const data = await api.history({ range: r, provider: p }, c.signal)
    h.data = {
      from: data.from,
      to: data.to,
      truncated: Boolean(data.truncated),
      buckets: Array.isArray(data.buckets) ? data.buckets : [],
      quotas: Array.isArray(data.quotas) ? data.quotas : [],
      decisions: Array.isArray(data.decisions) ? data.decisions : [],
    }
    h.range = r
    h.error = ''
  } catch (err) {
    if (!isAbort(err)) h.error = errorMessage(err)
  } finally {
    if (ctl[p] === c) h.loading = false
  }
}

function loadVisibleHistory(force = false): Promise<void> {
  return Promise.all(
    visibleProviders.value.filter((p) => force || history[p].range !== range.value).map((p) => loadHistory(p)),
  ).then(() => undefined)
}

function loadAll(): Promise<void> {
  return Promise.all([loadQuotas(), loadRouting(), loadVisibleHistory(true)]).then(() => undefined)
}

// ── Фильтры ──────────────────────────────────────────
const tab = ref<Tab>('all')
const range = ref<HistoryRange>('24h')
const showEmails = ref(false)

const RANGE_OPTIONS = [
  { value: '6h', label: '6 ч' },
  { value: '24h', label: '24 ч' },
  { value: '7d', label: '7 дн' },
  { value: '30d', label: '30 дн' },
] as const

const RANGE_WORDS: Record<HistoryRange, string> = {
  '6h': 'последние 6 часов',
  '24h': 'последние 24 часа',
  '7d': 'последние 7 дней',
  '30d': 'последние 30 дней',
}

// ── Аккаунты ─────────────────────────────────────────
const subs = computed<readonly Subscription[]>(() => hub.state.subs.data?.subscriptions ?? [])
const subById = computed(() => new Map(subs.value.map((s) => [s.id, s])))
const snapshotAccounts = computed<readonly QuotaAccount[]>(() => quotas.data?.snapshot?.accounts ?? [])
const staleAfter = computed(() => quotas.data?.staleAfterSeconds || 600)

function asProv(raw: string): Prov | null {
  const k = providerKey(raw)
  return k === 'claude' || k === 'codex' ? k : null
}

/** Все известные аккаунты провайдера: из снимка квот и из списка подписок. */
function accountsOf(p: Prov): QuotaAccount[] {
  const map = new Map<string, QuotaAccount>()
  for (const a of snapshotAccounts.value) if (asProv(a.provider) === p) map.set(a.authId, a)
  for (const s of subs.value) {
    if (asProv(s.provider) !== p || map.has(s.id)) continue
    map.set(s.id, { authId: s.id, provider: p, disabled: s.disabled, observedAt: '', source: '', windows: [] })
  }
  return [...map.values()]
}

const counts = computed(() => {
  const out: Record<Prov, number> = { claude: 0, codex: 0 }
  for (const p of PROVIDERS) out[p] = accountsOf(p).length
  return out
})
const totalAccounts = computed(() => counts.value.claude + counts.value.codex)

const otherProviders = computed(() => {
  const set = new Set<string>()
  for (const s of subs.value) if (!asProv(s.provider)) set.add(providerLabel(providerKey(s.provider)))
  return [...set]
})

const visibleProviders = computed<Prov[]>(() => {
  if (tab.value !== 'all') return [tab.value]
  const present = PROVIDERS.filter((p) => counts.value[p] > 0)
  return present.length ? present : PROVIDERS
})

function accountName(id: string): string {
  const s = subById.value.get(id)
  return s?.name || id
}
function accountEmail(id: string): string {
  return subById.value.get(id)?.email ?? ''
}

const routingById = computed(() => {
  const m = new Map<string, RoutingAccount>()
  for (const a of routing.data?.accounts ?? []) m.set(a.authId, a)
  return m
})

// ── Цвета аккаунтов: оттенки цвета провайдера, затем штрихи ──
const SHADES = [0, 45, -35, 70, -55, 22]
const DASHES = ['', '7 4', '2 3']
function accountStyle(p: Prov, index: number): { color: string; dash: string } {
  const base = providerColor(p)
  const s = SHADES[index % SHADES.length]!
  const color = s === 0 ? base : s > 0 ? `color-mix(in oklab, ${base} ${100 - s}%, white)` : `color-mix(in oklab, ${base} ${100 + s}%, black)`
  return { color, dash: DASHES[Math.floor(index / SHADES.length) % DASHES.length]! }
}

// ── Выбор окна, подсветка и метрика по провайдеру ────
const selWindow = reactive<Record<Prov, string>>({ claude: '', codex: '' })
const highlight = reactive<Record<Prov, string | null>>({ claude: null, codex: null })
const metric = reactive<Record<Prov, UsageMetric>>({ claude: 'total', codex: 'total' })

function toggleHighlight(p: Prov, id: string) {
  highlight[p] = highlight[p] === id ? null : id
}

// ── Панели провайдеров ───────────────────────────────
const panels = computed(() =>
  visibleProviders.value.map((p) => {
    const t = now.value
    const accounts = accountsOf(p)
    const summaries = new Map(accounts.map((a) => [a.authId, summarizeAccount(a, t, staleAfter.value)]))
    const available = accounts.filter((a) => summaries.get(a.authId)!.available).length
    const exhausted = accounts.filter((a) => summaries.get(a.authId)!.exhausted).length
    const unknown = accounts.filter((a) => summaries.get(a.authId)!.unknown).length
    const disabled = accounts.filter((a) => a.disabled).length
    let soonest: number | null = null
    for (const s of summaries.values()) if (s.available && s.soonestReset !== null && (soonest === null || s.soonestReset < soonest)) soonest = s.soonestReset

    // Порядок строк — как у контроллера: приоритет, затем имя.
    const rows = [...accounts].sort((a, b) => {
      const pa = routingById.value.get(a.authId)?.priority ?? -Infinity
      const pb = routingById.value.get(b.authId)?.priority ?? -Infinity
      return Number(a.disabled) - Number(b.disabled) || pb - pa || accountName(a.authId).localeCompare(accountName(b.authId))
    })

    // Цвет закреплён за аккаунтом по ID, чтобы не прыгал при пересортировке.
    const h = history[p]
    const histQuotas = h.data?.quotas ?? []
    const ids = [...new Set([...accounts.map((a) => a.authId), ...histQuotas.map((a) => a.authId)])].sort()
    const styles = new Map(ids.map((id, i) => [id, accountStyle(p, i)]))

    const windows = windowIds(histQuotas, accounts)
    const win = windows.includes(selWindow[p]) ? selWindow[p] : (windows[0] ?? '')
    const from = parseTime(h.data?.from) ?? t
    const to = parseTime(h.data?.to) ?? t
    const series: QuotaChartSeries[] = []
    if (win) {
      const byAcc = quotaSeries(histQuotas, win)
      for (const id of ids) {
        const points = byAcc.get(id)
        if (!points?.length) continue
        series.push({ key: id, label: accountName(id), ...styles.get(id)!, points })
      }
    }
    const legend = ids
      .filter((id) => series.some((s) => s.key === id))
      .map((id) => {
        const cur = accounts.find((a) => a.authId === id)?.windows?.find((w) => w.id === win)
        const st = cur ? windowState(cur, t) : null
        const value =
          !cur || !st ? 'нет в снимке' : st === 'ok' ? formatPercent(clampPercent(cur.remainingPercent!)) : st === 'exhausted' ? 'исчерпано' : st === 'reset-passed' ? 'ждёт наблюдения' : 'неизвестно'
        return { id, name: accountName(id), ...styles.get(id)!, value }
      })

    const buckets = h.data?.buckets ?? []
    return {
      p,
      accounts,
      rows,
      summaries,
      available,
      exhausted,
      unknown,
      disabled,
      soonest,
      windows,
      win,
      from,
      to,
      series,
      legend,
      buckets,
      totals: usageTotals(buckets),
      maxGap: maxGapFor(to - from),
      history: h,
    }
  }),
)

// ── Журнал решений по видимым провайдерам ────────────
const decisions = computed<RoutingDecision[]>(() =>
  visibleProviders.value
    .flatMap((p) => history[p].data?.decisions ?? [])
    .sort((a, b) => (parseTime(b.at) ?? 0) - (parseTime(a.at) ?? 0))
    .slice(0, 60),
)
const historyLoaded = computed(() => visibleProviders.value.every((p) => history[p].data !== null))
const truncated = computed(() => visibleProviders.value.some((p) => history[p].data?.truncated))
const historyErrors = computed(() => visibleProviders.value.filter((p) => history[p].error).map((p) => ({ p, error: history[p].error })))

// ── Статус сбора, маршрутизации и observer ───────────
const snapshotAt = computed(() => parseTime(quotas.data?.snapshot?.updatedAt))

const observer = computed(() => routing.data?.usageObserver ?? null)
const observerLosses = computed(() => (observer.value?.dropped ?? 0) + (observer.value?.writeErrors ?? 0))
const observerSeen = computed(() => Boolean(observer.value && (observer.value.updatedAt || observer.value.lastHandledUnix)))
const routingCheckedAt = computed(() => parseTime(routing.data?.updatedAt))
const lastHandledAt = computed(() => {
  const u = observer.value?.lastHandledUnix
  return typeof u === 'number' && u > 0 ? u * 1000 : null
})

const STRATEGIES: Record<string, string> = { 'soonest-reset-first': 'новые сессии — на аккаунт с ближайшим сбросом' }

// ── Обновление ───────────────────────────────────────
type RefreshPhase = 'idle' | 'request' | 'waiting'
const phase = ref<RefreshPhase>('idle')
const refreshNote = ref('')
let pollTimer = 0
let alive = true

const wait = (ms: number) => new Promise<void>((r) => (pollTimer = window.setTimeout(r, ms)))

/**
 * POST только будит сборщик. Новый снимок ждём по смене `updatedAt`: активные
 * проверки ограничены раз в 5 минут на аккаунт, поэтому новых данных может не быть.
 */
async function refresh() {
  if (phase.value !== 'idle') return
  phase.value = 'request'
  refreshNote.value = ''
  const before = quotas.data?.snapshot?.updatedAt ?? ''
  try {
    const res = await api.refreshTelemetry()
    push(res.message || 'Обновление запрошено.')
  } catch (err) {
    phase.value = 'idle'
    push(`Обновление не запрошено: ${errorMessage(err)}`, 'error')
    return
  }
  phase.value = 'waiting'
  let changed = false
  for (let i = 0; i < 8 && alive; i++) {
    await wait(2500)
    if (!alive) return
    const data = await loadQuotas()
    if (!data) {
      if (quotas.error) break
      continue
    }
    if (data.snapshot?.updatedAt && data.snapshot.updatedAt !== before) {
      changed = true
      break
    }
  }
  if (!alive) return
  await Promise.all([loadRouting(), loadVisibleHistory(true)])
  now.value = Date.now()
  phase.value = 'idle'
  if (quotas.error) refreshNote.value = `Снимок квот не получен: ${quotas.error}`
  else if (quotas.data?.error) refreshNote.value = `Цикл сбора завершился с ошибкой: ${quotas.data.error}`
  else if (!changed) refreshNote.value = 'За 20 секунд сборщик не выпустил новый снимок. Данные подтянутся автоматически при следующем цикле.'
  else refreshNote.value = ''
}

// ── Жизненный цикл ───────────────────────────────────
let clock = 0
let ticks = 0
function onVisible() {
  if (document.visibilityState === 'visible') {
    now.value = Date.now()
    void loadQuotas()
    void loadRouting()
  }
}
onMounted(() => {
  void loadAll()
  if (!hub.state.subs.data && !hub.state.subs.loading) void hub.loadSubscriptions()
  clock = window.setInterval(() => {
    now.value = Date.now()
    if (document.visibilityState !== 'visible' || phase.value !== 'idle') return
    ticks++
    // Раз в минуту — снимок и статус; раз в 5 минут — история.
    if (ticks % 4 === 0) {
      void loadQuotas()
      void loadRouting()
    }
    if (ticks % 20 === 0) void loadVisibleHistory(true)
  }, 15_000)
  document.addEventListener('visibilitychange', onVisible)
})
onBeforeUnmount(() => {
  alive = false
  window.clearInterval(clock)
  window.clearTimeout(pollTimer)
  document.removeEventListener('visibilitychange', onVisible)
  for (const c of Object.values(ctl)) c?.abort()
})

watch([range, visibleProviders], () => void loadVisibleHistory())

// ── Подсказки для строк ──────────────────────────────
function resetText(resetAt: string | null): string {
  const t = parseTime(resetAt)
  if (t === null) return 'время сброса неизвестно'
  if (t <= now.value) return `сброс был ${formatMoment(t, now.value)}`
  return `сброс через ${formatDuration(t - now.value)}, ${formatMoment(t, now.value)}`
}

const OBSERVER_CONFIG = `routing:
  strategy: "fill-first"
  session-affinity: true
  session-affinity-ttl: "168h"
  session-affinity-rebind-on: "quota-strict"
plugins:
  enabled: true
  dir: "bin/plugins"
  configs:
    proxcli-usage:
      enabled: true`
</script>

<template>
  <div class="page">
    <header class="page__head">
      <div class="page__title">
        <h1 tabindex="-1">Квоты и расход</h1>
        <span class="page__meta">
          <span v-if="hub.state.subs.data || quotas.data">{{ nAccounts(totalAccounts) }}</span>
          <span v-if="snapshotAt">снимок {{ formatAgo(now - snapshotAt) }}</span>
        </span>
      </div>
      <div class="page__actions">
        <button type="button" class="btn" :aria-pressed="showEmails" @click="showEmails = !showEmails">
          {{ showEmails ? 'Скрыть почту' : 'Показать почту' }}
        </button>
        <button type="button" class="btn btn--primary" :class="{ 'is-busy': phase !== 'idle' }" :disabled="phase !== 'idle'" @click="refresh">
          <AppIcon name="refresh" />
          <span v-if="phase === 'request'">Запрашиваю…</span>
          <span v-else-if="phase === 'waiting'">Жду нового снимка…</span>
          <span v-else>Обновить квоты</span>
        </button>
      </div>
    </header>

    <p v-if="refreshNote" class="notice notice--warn" role="status">{{ refreshNote }}</p>

    <div class="controls">
      <div class="tabs" role="group" aria-label="Провайдер">
        <button type="button" class="tab" :aria-pressed="tab === 'all'" @click="tab = 'all'">
          <AppIcon name="all" class="tab__all" />
          <span>Все</span>
          <span class="tab__count">{{ totalAccounts }}</span>
        </button>
        <button v-for="p in PROVIDERS" :key="p" type="button" class="tab" :aria-pressed="tab === p" @click="tab = p">
          <ProviderGlyph :provider="p" />
          <span>{{ providerLabel(p) }}</span>
          <span class="tab__count">{{ counts[p] }}</span>
        </button>
      </div>
      <div class="filters">
        <SegmentedFilter v-model="range" label="Период" show-label :options="RANGE_OPTIONS" />
        <span class="controls__hint">Часовые корзины, история хранится 30 дней</span>
      </div>
    </div>

    <!-- Состояние учёта: что именно сейчас работает -->
    <section class="panel health" aria-labelledby="health-title">
      <h2 id="health-title" class="visually-hidden">Состояние учёта</h2>

      <div class="health__cell">
        <span class="health__label">Сбор квот</span>
        <template v-if="quotas.data">
          <span class="health__value">
            <span class="dot" :class="quotas.data.collecting ? 'tone--ok' : 'tone--error'" aria-hidden="true" />
            {{ quotas.data.collecting ? 'Фоновый сборщик запущен' : 'Фоновый сборщик не запущен' }}
          </span>
          <span class="health__detail">
            <template v-if="snapshotAt">Последний снимок {{ formatMoment(snapshotAt, now) }}.</template>
            <template v-else>Снимков ещё не было.</template>
            Цикл раз в минуту, активная проверка аккаунта не чаще раза в 5 минут.
          </span>
          <span v-if="quotas.data.error" class="health__error">Последний цикл: {{ quotas.data.error }}</span>
        </template>
        <span v-else-if="quotas.error" class="health__error">{{ quotas.error }}</span>
        <span v-else class="skeleton" style="width: 20ch" />
      </div>

      <div class="health__cell">
        <span class="health__label">Маршрутизация</span>
        <template v-if="routing.data">
          <span class="health__value">
            <span class="dot" :class="routing.data.active ? 'tone--ok' : 'tone--error'" aria-hidden="true" />
            {{ routing.data.active ? 'Политика применяется' : 'Политика не применяется' }}
          </span>
          <span class="health__detail">
            {{ STRATEGIES[routing.data.strategy ?? ''] ?? routing.data.strategy ?? 'стратегия неизвестна' }}<template v-if="routing.data.policy">,
              <span class="mono">{{ routing.data.policy }}</span></template>.
            <template v-if="!routing.data.enabled"> Управление приоритетами выключено.</template>
            <template v-if="routingCheckedAt"> Проверено {{ formatAgo(now - routingCheckedAt) }}.</template>
          </span>
          <span v-if="routing.data.error" class="health__error">{{ routing.data.error }}</span>
        </template>
        <span v-else-if="routing.error" class="health__error">{{ routing.error }}</span>
        <span v-else class="skeleton" style="width: 20ch" />
      </div>

      <div class="health__cell">
        <span class="health__label">Учёт запросов (usage observer)</span>
        <template v-if="routing.data">
          <span class="health__value">
            <span class="dot" :class="observer?.active ? 'tone--ok' : 'tone--error'" aria-hidden="true" />
            <template v-if="observer?.active">Пишет журнал</template>
            <template v-else-if="observerSeen">Не отвечает больше 30 секунд</template>
            <template v-else>Не подключён</template>
          </span>
          <dl v-if="observerSeen" class="health__counters">
            <div :class="{ 'is-bad': (observer?.dropped ?? 0) > 0 }">
              <dt>Потеряно событий</dt>
              <dd>{{ formatExact(observer?.dropped ?? 0) }}</dd>
            </div>
            <div :class="{ 'is-bad': (observer?.writeErrors ?? 0) > 0 }">
              <dt>Ошибок записи</dt>
              <dd>{{ formatExact(observer?.writeErrors ?? 0) }}</dd>
            </div>
            <div>
              <dt>В очереди</dt>
              <dd>{{ formatExact(observer?.queued ?? 0) }}</dd>
            </div>
          </dl>
          <span class="health__detail">
            <template v-if="lastHandledAt">Последний запрос {{ formatMoment(lastHandledAt, now) }}. </template>
            <template v-if="observerLosses > 0">История за это время неполная.</template>
            <template v-else>Учёт best-effort: при аварийной остановке очередь может потеряться.</template>
          </span>
        </template>
        <span v-else-if="routing.error" class="health__error">Статус неизвестен: {{ routing.error }}</span>
        <span v-else class="skeleton" style="width: 20ch" />
      </div>

      <p class="health__foot">
        Активная сессия переходит на другой аккаунт только при подтверждённом исчерпании квоты; сеть, 5xx и 401/403 её не переносят.
        Привязки сессий живут в памяти ядра и теряются при его перезапуске.
        <a :href="href('help')">Как это устроено</a>
      </p>
    </section>

    <!-- Observer не подключён: как включить -->
    <section v-if="routing.data && !observer?.active && !observerSeen" class="panel setup" aria-labelledby="setup-title">
      <div class="setup__text">
        <h2 id="setup-title">Подключите usage observer, чтобы видеть расход</h2>
        <p class="muted">
          Квоты собирает панель, а токены и смены аккаунтов пишет плагин <code>proxcli-usage</code> внутри ядра. Без него графики расхода пустые.
        </p>
        <ol class="setup__steps">
          <li>Соберите ядро с плагином: <code>bun run build:core</code>. Появятся <code>bin/cli-proxy-api</code> и <code>bin/plugins/proxcli-usage</code>.</li>
          <li>Проверьте блоки <code>routing</code> и <code>plugins</code> в конфиге ядра.</li>
          <li>Запустите собранное ядро: <code>bun run hub</code>. Перезапуск ядра сбрасывает привязки активных сессий: делайте это между важными диалогами.</li>
        </ol>
      </div>
      <CodeBlock title="config.yaml ядра" :code="OBSERVER_CONFIG" />
    </section>

    <div v-if="truncated" class="notice notice--warn" role="status">
      <span class="notice__title">История показана не полностью</span>
      <span>Журнал за период слишком большой для одного чтения. Выберите период короче.</span>
    </div>
    <div v-for="e in historyErrors" :key="e.p" class="notice notice--error" role="alert">
      <span class="notice__title">История {{ providerLabel(e.p) }} не загрузилась</span>
      <span>{{ e.error }}</span>
      <div class="notice__actions">
        <button type="button" class="btn btn--small" @click="loadHistory(e.p)">Загрузить снова</button>
      </div>
    </div>
    <div v-if="quotas.error && !quotas.data" class="notice notice--error" role="alert">
      <span class="notice__title">Снимок квот не загрузился</span>
      <span>{{ quotas.error }}</span>
      <div class="notice__actions">
        <button type="button" class="btn btn--small" @click="loadQuotas()">Загрузить снова</button>
      </div>
    </div>

    <div v-if="(quotas.data || hub.state.subs.data) && !totalAccounts" class="panel empty">
      <h2>Нет аккаунтов Claude или Codex</h2>
      <p class="muted">
        Квоты собираются только для подписок Claude и Codex.
        <template v-if="otherProviders.length">Сейчас на хабе: {{ otherProviders.join(', ') }}.</template>
      </p>
      <div><a class="btn btn--primary" :href="href('add')">Добавить аккаунт</a></div>
    </div>

    <!-- Панели провайдеров -->
    <section
      v-for="pn in totalAccounts ? panels : []"
      :key="pn.p"
      class="panel qpanel"
      :style="{ '--pc': providerColor(pn.p) }"
      :aria-labelledby="`qp-${pn.p}`"
      :aria-busy="pn.history.loading || undefined"
    >
      <header class="qpanel__head">
        <h2 :id="`qp-${pn.p}`" class="panel__title">
          <ProviderGlyph :provider="pn.p" />
          {{ providerLabel(pn.p) }}
          <span class="panel__meta">{{ nAccounts(pn.accounts.length) }}</span>
        </h2>
      </header>

      <div class="qpanel__summary">
        <p class="figure">
          <span class="figure__num">{{ pn.available }}</span>
          <span class="figure__of">из {{ pn.accounts.length }} с подтверждённым остатком</span>
        </p>
        <dl class="stats">
          <div class="stat">
            <dt>Ближайший сброс</dt>
            <dd>
              <template v-if="pn.soonest">через {{ formatDuration(pn.soonest - now) }}</template>
              <template v-else>неизвестен</template>
            </dd>
          </div>
          <div class="stat">
            <dt>Исчерпано</dt>
            <dd :class="{ 'tone--error': pn.exhausted }">{{ pn.exhausted }}</dd>
          </div>
          <div class="stat">
            <dt>Квота неизвестна</dt>
            <dd>{{ pn.unknown }}</dd>
          </div>
          <div v-if="pn.disabled" class="stat">
            <dt>Отключено</dt>
            <dd>{{ pn.disabled }}</dd>
          </div>
        </dl>
      </div>

      <!-- История остатка -->
      <div class="qblock">
        <div class="qblock__head">
          <h3>Остаток окна квоты</h3>
          <SegmentedFilter
            v-if="pn.windows.length > 1"
            :model-value="pn.win"
            label="Окно квоты"
            :options="pn.windows.map((id) => ({ value: id, label: windowTitle(id) }))"
            @update:model-value="(v: string) => (selWindow[pn.p] = v)"
          />
          <span v-else-if="pn.win" class="qblock__single">{{ windowTitle(pn.win) }}</span>
        </div>
        <template v-if="pn.history.data">
          <QuotaHistoryChart
            v-if="pn.series.length"
            :series="pn.series"
            :from="pn.from"
            :to="pn.to"
            :now="now"
            :max-gap="pn.maxGap"
            :highlight="highlight[pn.p]"
            :label="`Остаток окна «${windowTitle(pn.win)}» по аккаунтам ${providerLabel(pn.p)} за ${RANGE_WORDS[pn.history.range ?? range]}`"
          />
          <p v-else class="qblock__empty">
            За {{ RANGE_WORDS[pn.history.range ?? range] }} нет наблюдений квот. Точка появляется, когда сборщик получает новое наблюдение от провайдера.
          </p>
          <div v-if="pn.legend.length" class="legend" role="group" aria-label="Аккаунты на графике">
            <button
              v-for="l in pn.legend"
              :key="l.id"
              type="button"
              class="legend__item"
              :aria-pressed="highlight[pn.p] === l.id"
              @click="toggleHighlight(pn.p, l.id)"
            >
              <svg width="22" height="8" aria-hidden="true">
                <line x1="1" x2="21" y1="4" y2="4" :style="{ stroke: l.color }" stroke-width="2.2" :stroke-dasharray="l.dash || undefined" stroke-linecap="round" />
              </svg>
              <span class="legend__name">{{ l.name }}</span>
              <span class="legend__val">{{ l.value }}</span>
            </button>
          </div>
          <p class="qblock__note">Линии — только наблюдённые значения; разрыв означает паузу в наблюдениях. Прогнозов нет, окна разной длины не складываются.</p>
        </template>
        <div v-else class="skeleton qblock__skeleton" />
      </div>

      <!-- Текущие окна -->
      <div class="qblock">
        <div class="qblock__head">
          <h3>Окна сейчас</h3>
          <span class="qblock__hint">Наблюдение старше {{ Math.round(staleAfter / 60) }} минут считается устаревшим и не влияет на приоритет</span>
        </div>
        <ul class="accts" :aria-label="`Аккаунты ${providerLabel(pn.p)}`">
          <li v-for="a in pn.rows" :key="a.authId" class="acct" :class="{ 'acct--off': a.disabled }">
            <div class="acct__who">
              <span class="acct__name">{{ accountName(a.authId) }}</span>
              <span v-if="showEmails && accountEmail(a.authId)" class="acct__email">{{ accountEmail(a.authId) }}</span>
              <span v-if="a.disabled" class="acct__flag">Отключён</span>
              <span v-else-if="routingById.get(a.authId)" class="acct__route">
                {{ reasonLabel(routingById.get(a.authId)!.reason) }}<template v-if="routingById.get(a.authId)!.priority > 0">,
                  приоритет <span class="mono">{{ routingById.get(a.authId)!.priority }}</span></template>
              </span>
            </div>

            <div class="acct__windows">
              <p v-if="!a.windows?.length" class="acct__none">Окон квоты нет: наблюдений ещё не было.</p>
              <div
                v-for="w in [...(a.windows ?? [])].sort((x, y) => compareWindows(x.id, y.id))"
                :key="w.id"
                class="win"
                :class="`win--${windowState(w, now)}`"
              >
                <span class="win__title">{{ windowTitle(w.id) }}</span>
                <span class="win__bar" aria-hidden="true">
                  <span
                    v-if="windowState(w, now) === 'ok' || windowState(w, now) === 'exhausted'"
                    class="win__fill"
                    :style="{ width: `${windowState(w, now) === 'ok' ? clampPercent(w.remainingPercent!) : 0}%` }"
                  />
                </span>
                <span class="win__value">
                  <template v-if="windowState(w, now) === 'ok'">{{ formatPercent(clampPercent(w.remainingPercent!)) }} осталось</template>
                  <template v-else-if="windowState(w, now) === 'exhausted'">исчерпано</template>
                  <template v-else-if="windowState(w, now) === 'reset-passed'">сброс прошёл, ждём наблюдения</template>
                  <template v-else>остаток неизвестен</template>
                </span>
                <span class="win__reset">{{ resetText(w.resetAt) }}</span>
              </div>
            </div>

            <div class="acct__meta">
              <template v-for="f in [freshness(a.observedAt, now, staleAfter)]" :key="f.kind">
                <span v-if="f.kind === 'never'" class="acct__fresh">Наблюдений нет</span>
                <span v-else class="acct__fresh" :class="{ 'is-stale': f.kind !== 'fresh' }">
                  <template v-if="f.kind === 'future'">Время наблюдения в будущем — проверьте часы хаба</template>
                  <template v-else-if="f.kind === 'stale'">Устарело: {{ formatAgo(f.ageMs) }}</template>
                  <template v-else>Наблюдение {{ formatAgo(f.ageMs) }}</template>
                </span>
              </template>
              <span class="acct__source">{{ sourceLabel(a.source) }}</span>
            </div>

            <p v-if="a.error" class="acct__error">{{ a.error }}</p>
          </li>
        </ul>
      </div>

      <!-- Расход -->
      <div class="qblock">
        <div class="qblock__head">
          <h3>Расход по часам</h3>
          <SegmentedFilter v-model="metric[pn.p]" label="Счётчик" :options="USAGE_METRICS" />
        </div>
        <template v-if="pn.history.data">
          <UsageBarsChart
            :buckets="pn.buckets"
            :metric="metric[pn.p]"
            :from="pn.from"
            :to="pn.to"
            :now="now"
            :color="metric[pn.p] === 'cacheRead' || metric[pn.p] === 'cacheWrite' ? 'color-mix(in oklab, var(--pc) 55%, white)' : 'var(--pc)'"
            :label="`${USAGE_METRICS.find((m) => m.value === metric[pn.p])?.label} по часам, ${providerLabel(pn.p)}, ${RANGE_WORDS[pn.history.range ?? range]}`"
          />
          <dl class="totals">
            <div>
              <dt>Запросов</dt>
              <dd>
                {{ formatCount(pn.totals.requests) }}
                <span v-if="pn.totals.failed" class="totals__fail">ошибок {{ formatCount(pn.totals.failed) }}</span>
              </dd>
            </div>
            <div>
              <dt>Всего токенов</dt>
              <dd>{{ formatCount(pn.totals.total) }}</dd>
            </div>
            <div>
              <dt>Ввод</dt>
              <dd>{{ formatCount(pn.totals.input) }}</dd>
            </div>
            <div>
              <dt>Вывод</dt>
              <dd>{{ formatCount(pn.totals.output) }}</dd>
            </div>
            <div v-if="pn.totals.reasoning">
              <dt>Рассуждение</dt>
              <dd>{{ formatCount(pn.totals.reasoning) }}</dd>
            </div>
            <div>
              <dt>Чтение кеша</dt>
              <dd>{{ formatCount(pn.totals.cacheRead) }}</dd>
            </div>
            <div>
              <dt>Запись кеша</dt>
              <dd>{{ formatCount(pn.totals.cacheWrite) }}</dd>
            </div>
          </dl>
          <p class="qblock__note">
            Суммы за {{ RANGE_WORDS[pn.history.range ?? range] }} по каждому счётчику отдельно; «всего» — как сообщил провайдер.
            Пустой час — нет записанных запросов<template v-if="!observer?.active"> или observer не работал</template>.
            <template v-if="metric[pn.p] === 'requests'">Красным — запросы с ошибкой.</template>
          </p>
        </template>
        <div v-else class="skeleton qblock__skeleton" />
      </div>
    </section>

    <!-- Журнал -->
    <section v-if="totalAccounts" class="panel" aria-labelledby="log-title">
      <div class="panel__head">
        <h2 id="log-title" class="panel__title">
          Журнал маршрутизации <span v-if="decisions.length" class="panel__meta">{{ decisions.length }}</span>
        </h2>
        <p class="muted small">Смены приоритетов контроллером и наблюдения сессий по завершённым запросам. Наблюдение не доказывает причину смены аккаунта.</p>
      </div>
      <p v-if="!historyLoaded" class="log__empty muted">Загружаю журнал…</p>
      <p v-else-if="!decisions.length" class="log__empty muted">За {{ RANGE_WORDS[range] }} событий нет.</p>
      <ol v-else class="log">
        <li v-for="(d, i) in decisions" :key="`${d.at}-${d.authId}-${i}`" class="log__row" :class="`log__row--${decisionKind(d)}`">
          <time class="log__time" :datetime="d.at">{{ parseTime(d.at) ? formatMoment(parseTime(d.at)!, now) : 'время неизвестно' }}</time>
          <ProviderGlyph :provider="d.provider" :size="15" />
          <span class="log__what">{{ reasonLabel(d.reason) }}</span>
          <span class="log__who">
            <template v-if="d.previousAuthId">{{ accountName(d.previousAuthId) }} → </template>{{ accountName(d.authId) }}
          </span>
          <span class="log__model mono">{{ d.model }}</span>
          <span v-if="d.session" class="log__session mono" :title="'Хеш сессии'">#{{ d.session.slice(0, 8) }}</span>
        </li>
      </ol>
    </section>
  </div>
</template>

<style scoped>
.controls {
  display: grid;
  gap: var(--space-3);
}
.tab__all {
  width: 17px;
  height: 17px;
}
.controls__hint {
  color: var(--ink-3);
  font-size: var(--fs-sm);
}

.is-busy .icon {
  animation: spin 0.9s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* ── Состояние учёта ─────────────────────────────── */

.health {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.health__cell {
  display: grid;
  align-content: start;
  gap: 4px;
  padding: var(--space-4) var(--space-5);
  border-left: 1px solid var(--rule);
  min-width: 0;
}
.health__cell:first-child {
  border-left: 0;
}
.health__label {
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.health__value {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  font-size: var(--fs-lg);
  font-weight: 550;
}
.health__detail {
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.health__error {
  color: var(--err);
  font-size: var(--fs-sm);
}
.health__counters {
  display: flex;
  flex-wrap: wrap;
  gap: 2px var(--space-5);
  margin: 2px 0;
}
.health__counters div {
  display: flex;
  gap: 6px;
  align-items: baseline;
  font-size: var(--fs-sm);
}
.health__counters dt {
  color: var(--ink-2);
}
.health__counters dd {
  margin: 0;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.health__counters .is-bad dd {
  color: var(--err);
}
.health__foot {
  grid-column: 1 / -1;
  max-width: none;
  padding: var(--space-3) var(--space-5);
  border-top: 1px solid var(--rule);
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.health__foot a {
  color: var(--ink);
}

/* ── Подключение observer ─────────────────────────── */

.setup {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr);
  gap: var(--space-5);
  padding: var(--space-5);
}
.setup__text {
  display: grid;
  align-content: start;
  gap: var(--space-3);
}
.setup__steps {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding-left: 1.3em;
  color: var(--ink-2);
}
.setup__steps li::marker {
  color: var(--ink-3);
}

/* ── Панель провайдера ───────────────────────────── */

.qpanel {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  min-width: 0;
}
.qpanel__head {
  padding: var(--space-5) var(--space-5) 0;
}
.qpanel__head .panel__title {
  align-items: center;
}
.qpanel__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4) var(--space-6);
  padding: var(--space-3) var(--space-5) var(--space-4);
}
.figure {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin: 0;
}
.figure__num {
  font-size: var(--fs-3xl);
  font-weight: 600;
  line-height: 1;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums;
}
.figure__of {
  color: var(--ink-2);
  font-size: var(--fs-lg);
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3) 40px;
  margin: 0;
}
.stat {
  display: grid;
  gap: 2px;
}
.stat dt {
  color: var(--ink-2);
}
.stat dd {
  margin: 0;
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.qblock {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  min-width: 0;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-5) var(--space-5);
  border-top: 1px solid var(--rule);
}
.qblock__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
}
.qblock__head h3 {
  font-size: var(--fs-md);
  font-weight: 600;
}
.qblock__single,
.qblock__hint {
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.qblock__empty {
  display: grid;
  place-items: center;
  min-height: 140px;
  max-width: none;
  padding: var(--space-4);
  border: 1px dashed var(--rule-strong);
  border-radius: var(--r-control);
  color: var(--ink-2);
  text-align: center;
}
.qblock__note {
  color: var(--ink-3);
  font-size: var(--fs-sm);
  max-width: none;
}
.qblock__skeleton {
  height: 200px;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 2px var(--space-2);
}
.legend__item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.legend__item:hover {
  background: var(--surface-2);
}
.legend__item[aria-pressed='true'] {
  background: var(--surface-3);
}
.legend__name {
  max-width: 22ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.legend__val {
  color: var(--ink-2);
  font-variant-numeric: tabular-nums;
}

/* ── Аккаунты и окна ─────────────────────────────── */

.accts {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
}
.acct {
  display: grid;
  grid-template-columns: minmax(0, 220px) minmax(0, 1fr) minmax(0, 200px);
  gap: var(--space-2) var(--space-5);
  padding: 14px 0;
  border-top: 1px solid var(--rule);
}
.acct:first-child {
  border-top: 0;
  padding-top: 4px;
}
.acct--off .acct__who,
.acct--off .acct__windows {
  opacity: 0.55;
}
.acct__who {
  display: grid;
  align-content: start;
  gap: 2px;
  min-width: 0;
}
.acct__name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.acct__email,
.acct__route,
.acct__flag {
  color: var(--ink-2);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}
.acct__windows {
  display: grid;
  gap: 8px;
  min-width: 0;
}
.acct__none {
  color: var(--ink-3);
  font-size: var(--fs-sm);
}
.acct__meta {
  display: grid;
  align-content: start;
  gap: 2px;
  font-size: var(--fs-sm);
  text-align: right;
}
.acct__fresh {
  color: var(--ink);
}
.acct__fresh.is-stale {
  color: var(--route-direct);
}
.acct__source {
  color: var(--ink-3);
}
.acct__error {
  grid-column: 1 / -1;
  max-width: none;
  color: var(--err);
  font-size: var(--fs-sm);
}

.win {
  display: grid;
  grid-template-columns: 9.5rem minmax(80px, 1fr) 12.5rem;
  grid-template-areas: 'title bar value' '. reset reset';
  align-items: center;
  gap: 0 var(--space-3);
  font-size: var(--fs-sm);
}
.win__title {
  grid-area: title;
  color: var(--ink-2);
}
.win__bar {
  grid-area: bar;
  position: relative;
  height: 6px;
  border-radius: 3px;
  background: var(--surface-3);
  overflow: hidden;
}
.win__fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 3px;
  background: var(--pc);
}
.win--unknown .win__bar,
.win--reset-passed .win__bar {
  background: repeating-linear-gradient(135deg, var(--surface-3) 0 4px, transparent 4px 8px);
}
.win--exhausted .win__bar {
  background: color-mix(in srgb, var(--err) 35%, var(--surface-3));
}
.win__value {
  grid-area: value;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.win--exhausted .win__value {
  color: var(--err);
}
.win--unknown .win__value,
.win--reset-passed .win__value {
  color: var(--ink-2);
  font-weight: 400;
}
.win__reset {
  grid-area: reset;
  color: var(--ink-3);
  font-size: var(--fs-xs);
}

/* ── Итоги расхода ───────────────────────────────── */

.totals {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3) 36px;
  margin: 0;
}
.totals div {
  display: grid;
  gap: 1px;
}
.totals dt {
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.totals dd {
  margin: 0;
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.totals__fail {
  margin-left: 6px;
  color: var(--err);
  font-size: var(--fs-sm);
  font-weight: 500;
}

/* ── Журнал ──────────────────────────────────────── */

.log__empty {
  padding: 0 var(--space-5) var(--space-5);
}
.log {
  margin: 0;
  padding: 0 var(--space-5) var(--space-3);
  list-style: none;
  max-height: 420px;
  overflow-y: auto;
}
.log__row {
  display: grid;
  grid-template-columns: 11rem 15px minmax(0, 1.4fr) minmax(0, 1.2fr) minmax(0, 0.8fr) 6rem;
  align-items: center;
  gap: var(--space-3);
  padding: 9px 0;
  border-top: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.log__row:first-child {
  border-top: 0;
}
.log__time,
.log__model,
.log__session {
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.log__session {
  color: var(--ink-3);
  text-align: right;
}
.log__row--priority .log__what {
  color: var(--ink-2);
}
.log__who {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty {
  display: grid;
  gap: var(--space-3);
  justify-items: start;
  padding: var(--space-6);
}

@media (max-width: 1180px) {
  .health {
    grid-template-columns: 1fr;
  }
  .health__cell {
    border-left: 0;
    border-top: 1px solid var(--rule);
  }
  .health__cell:first-child {
    border-top: 0;
  }
  .acct {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .acct__windows {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .setup {
    grid-template-columns: 1fr;
  }
  .log__row {
    grid-template-columns: 9rem 15px minmax(0, 1fr) minmax(0, 1fr);
  }
  .log__model,
  .log__session {
    display: none;
  }
}

@media (max-width: 900px) {
  .qpanel__head,
  .qpanel__summary,
  .qblock,
  .health__cell,
  .health__foot,
  .log,
  .log__empty {
    padding-left: var(--space-4);
    padding-right: var(--space-4);
  }
  .setup {
    padding: var(--space-4);
  }
  .figure__num {
    font-size: 2.4rem;
  }
  .stats,
  .totals {
    gap: var(--space-3) var(--space-5);
  }
  .win {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas: 'title value' 'bar bar' 'reset reset';
    gap: 4px var(--space-3);
  }
  .log__row {
    grid-template-columns: 15px minmax(0, 1fr);
    gap: 2px var(--space-2);
  }
  .log__time {
    grid-column: 1 / -1;
  }
  .log__who {
    grid-column: 2;
  }
  .empty {
    padding: var(--space-5);
  }
}
</style>
