<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, errorMessage } from '../api/client'
import type { Subscription } from '../api/types'
import ProviderGlyph from '../components/ProviderGlyph.vue'
import ProviderTabs from '../components/ProviderTabs.vue'
import RouteLine from '../components/RouteLine.vue'
import RouteLegend from '../components/RouteLegend.vue'
import SegmentedFilter from '../components/SegmentedFilter.vue'
import { useFilters } from '../composables/useFilters'
import { useHub } from '../composables/useHub'
import { compareProviders, plural, providerKey, providerLabel, providerName, subscriptionStatus } from '../lib/providers'
import { effectiveExit, type EffectiveExit, type ExitKind } from '../lib/proxy'
import { href } from '../lib/router'

const hub = useHub()
const status = computed(() => hub.state.status)
const subsState = computed(() => hub.state.subs)
const subs = computed<readonly Subscription[]>(() => subsState.value.data?.subscriptions ?? [])
const defaultProxy = computed(() => subsState.value.data?.defaultProxy ?? '')

const nSubs = (n: number) => `${n} ${plural(n, 'подписка', 'подписки', 'подписок')}`
const nAccounts = (n: number) => `${n} ${plural(n, 'аккаунт', 'аккаунта', 'аккаунтов')}`

const enabledCount = computed(() => subs.value.filter((s) => !s.disabled).length)

// ── Фильтры ──────────────────────────────────────────
const { provider, items: providerItems, state, exitKind, matches, reset } = useFilters(subs, defaultProxy)

const STATE_OPTIONS = [
  { value: 'all', label: 'Все' },
  { value: 'on', label: 'Включены' },
  { value: 'off', label: 'Отключены' },
] as const

const EXIT_OPTIONS = [
  { value: 'all', label: 'Любой' },
  { value: 'custom', label: 'Свой' },
  { value: 'default', label: 'Общий' },
  { value: 'direct', label: 'Без прокси' },
  { value: 'unknown', label: 'Неизвестен' },
] as const

const visible = computed(() => subs.value.filter(matches))

// ── Панели провайдеров ───────────────────────────────
interface Row {
  sub: Subscription
  exit: EffectiveExit
  status: ReturnType<typeof subscriptionStatus>
}

interface ProviderPanel {
  key: string
  /** Все подписки провайдера, без учёта вторичных фильтров. */
  total: number
  enabled: number
  ok: number
  errors: number
  other: number
  routes: Record<ExitKind, number>
  rows: Row[]
}

const panels = computed<ProviderPanel[]>(() => {
  const map = new Map<string, ProviderPanel>()
  for (const s of subs.value) {
    const key = providerKey(s.provider)
    if (provider.value !== 'all' && key !== provider.value) continue
    let p = map.get(key)
    if (!p) {
      p = { key, total: 0, enabled: 0, ok: 0, errors: 0, other: 0, routes: { custom: 0, default: 0, direct: 0, unknown: 0 }, rows: [] }
      map.set(key, p)
    }
    const exit = effectiveExit(s, defaultProxy.value)
    const st = subscriptionStatus(s.status, s.disabled)
    p.total++
    p.routes[exit.kind]++
    if (!s.disabled) {
      p.enabled++
      if (st.tone === 'ok') p.ok++
      else if (st.tone === 'error') p.errors++
      else p.other++
    }
    if (matches(s)) p.rows.push({ sub: s, exit, status: st })
  }
  return [...map.values()].filter((p) => p.rows.length).sort((a, b) => compareProviders(a.key, b.key))
})

const ROUTE_STATS: { kind: ExitKind; label: string }[] = [
  { kind: 'custom', label: 'Свой прокси' },
  { kind: 'default', label: 'Общий прокси' },
  { kind: 'direct', label: 'Без прокси' },
  { kind: 'unknown', label: 'Маршрут неизвестен' },
]

function openInSubscriptions(key: string) {
  provider.value = key
}

// ── Точки выхода ─────────────────────────────────────
interface ExitGroup {
  exit: EffectiveExit
  subs: Subscription[]
}

function groupByExit(list: readonly Subscription[]): ExitGroup[] {
  const map = new Map<string, ExitGroup>()
  for (const s of list) {
    const exit = effectiveExit(s, defaultProxy.value)
    const g = map.get(exit.key)
    if (g) g.subs.push(s)
    else map.set(exit.key, { exit, subs: [s] })
  }
  const order = { custom: 0, default: 1, direct: 2, unknown: 3 }
  return [...map.values()].sort((a, b) => order[a.exit.kind] - order[b.exit.kind] || b.subs.length - a.subs.length)
}

const groups = computed(() => groupByExit(visible.value))

// ── Что требует внимания (по всему хабу) ─────────────
interface Warning {
  id: string
  tone: 'warn' | 'error' | 'info'
  text: string
  names?: string[]
}

const warnings = computed<Warning[]>(() => {
  const out: Warning[] = []
  const enabled = subs.value.filter((s) => !s.disabled)

  const errored = enabled.filter((s) => s.status.trim().toLowerCase() === 'error')
  if (errored.length) {
    out.push({
      id: 'errors',
      tone: 'error',
      text: `Хаб отметил ошибку у ${errored.length === 1 ? 'подписки' : 'подписок'}. Проверьте вход или перелогиньтесь.`,
      names: errored.map((s) => s.name),
    })
  }

  const direct = enabled.filter((s) => effectiveExit(s, defaultProxy.value).kind === 'direct')
  if (direct.length) {
    out.push({
      id: 'direct',
      tone: 'warn',
      text: `${nSubs(direct.length)} без прокси: запросы идут с IP хаба.`,
      names: direct.map((s) => s.name),
    })
  }

  for (const g of groupByExit(subs.value)) {
    if (g.exit.kind === 'direct' || g.exit.kind === 'unknown') continue
    const byProvider = new Map<string, string[]>()
    for (const s of g.subs) {
      if (s.disabled) continue
      const p = providerName(s.provider)
      byProvider.set(p, [...(byProvider.get(p) ?? []), s.name])
    }
    for (const [p, names] of byProvider) {
      if (names.length < 2) continue
      out.push({
        id: `shared-${g.exit.key}-${p}`,
        tone: 'warn',
        text: `${names.length} ${plural(names.length, 'аккаунт', 'аккаунта', 'аккаунтов')} ${p} выходят через один адрес ${g.exit.label}: совпадает адрес прокси. Фактический IP выхода не проверялся.`,
        names,
      })
    }
  }

  const perProvider = new Map<string, number>()
  for (const s of enabled) {
    const p = providerName(s.provider)
    perProvider.set(p, (perProvider.get(p) ?? 0) + 1)
  }
  for (const [p, n] of perProvider) {
    if (n === 1) {
      out.push({
        id: `single-${p}`,
        tone: 'info',
        text: `У ${p} одна включённая подписка: когда кончится лимит, переключиться будет не на что.`,
      })
    }
  }
  return out
})

// ── Модели ───────────────────────────────────────────
const models = ref<{ loading: boolean; error: string; ids: string[] | null; raw: string }>({
  loading: false,
  error: '',
  ids: null,
  raw: '',
})

function extractIds(data: unknown): string[] | null {
  const list = Array.isArray(data)
    ? data
    : data && typeof data === 'object' && Array.isArray((data as { data?: unknown }).data)
      ? (data as { data: unknown[] }).data
      : data && typeof data === 'object' && Array.isArray((data as { models?: unknown }).models)
        ? (data as { models: unknown[] }).models
        : null
  if (!list) return null
  const ids = list
    .map((m) => (typeof m === 'string' ? m : m && typeof m === 'object' ? ((m as { id?: unknown; name?: unknown }).id ?? (m as { name?: unknown }).name) : null))
    .filter((x): x is string => typeof x === 'string')
  return [...new Set(ids)].sort((a, b) => a.localeCompare(b))
}

async function loadModels() {
  models.value = { loading: true, error: '', ids: null, raw: '' }
  try {
    const data = await api.models()
    const ids = extractIds(data)
    models.value = {
      loading: false,
      error: '',
      ids,
      raw: ids ? '' : JSON.stringify(data, null, 2).slice(0, 4000),
    }
  } catch (err) {
    models.value = { loading: false, error: errorMessage(err), ids: null, raw: '' }
  }
}

onMounted(loadModels)
</script>

<template>
  <div class="page">
    <header class="page__head">
      <div class="page__title">
        <h1 tabindex="-1">Обзор</h1>
        <span v-if="subsState.data" class="page__meta">
          <span>{{ nSubs(subs.length) }}</span>
          <span>{{ enabledCount }} {{ plural(enabledCount, 'включена', 'включены', 'включено') }}</span>
        </span>
      </div>
      <div class="page__actions">
        <a class="btn" :href="href('subscriptions')">Настроить маршруты</a>
        <a class="btn btn--primary" :href="href('add')">Добавить аккаунт</a>
      </div>
    </header>

    <!-- Состояние хаба: ошибки показываем сразу под заголовком. -->
    <div v-if="status.error" class="notice notice--error" role="alert">
      <span class="notice__title">Панель ProxCli не отвечает</span>
      <span>{{ status.error }}</span>
      <div class="notice__actions">
        <button type="button" class="btn btn--small" :disabled="status.loading" @click="hub.refreshAll()">Проверить снова</button>
      </div>
    </div>
    <div v-else-if="status.data && !status.data.connected" class="notice notice--error">
      <span class="notice__title">ProxCli не достучался до CLIProxyAPI</span>
      <span>{{ status.data.error || 'Сервер хаба не ответил.' }}</span>
      <span class="muted">
        Проверьте, что сервис запущен (<code>systemctl --user status cliproxyapi</code> или <code>brew services list</code>) и что ProxCli запущен с верным адресом хаба и ключом управления (<code>management.secret-key</code>).
      </span>
    </div>

    <div class="controls">
      <ProviderTabs v-model="provider" :items="providerItems" :total="subs.length" />
      <div class="filters">
        <SegmentedFilter v-model="state" label="Состояние" show-label :options="STATE_OPTIONS" />
        <SegmentedFilter v-model="exitKind" label="Выход" show-label :options="EXIT_OPTIONS" />
        <p class="hubinfo" aria-live="polite">
          <template v-if="status.data">
            <span class="dot" :class="status.data.connected ? 'tone--ok' : 'tone--error'" aria-hidden="true" />
            <span class="visually-hidden">{{ status.data.connected ? 'Хаб на связи:' : 'Хаб не отвечает:' }}</span>
            <span class="mono hubinfo__addr">{{ status.data.upstream || 'адрес хаба не задан' }}</span>
            <span v-if="status.data.version" class="hubinfo__ver">CLIProxyAPI {{ status.data.version }}</span>
          </template>
          <span v-else-if="!status.error" class="skeleton" style="width: 22ch" />
        </p>
      </div>
    </div>

    <section v-if="warnings.length" class="panel attention" aria-labelledby="warn-title">
      <div class="panel__head">
        <h2 id="warn-title" class="panel__title">
          Требует внимания <span class="panel__meta">{{ warnings.length }}</span>
        </h2>
      </div>
      <ul class="attention__list">
        <li v-for="w in warnings" :key="w.id" class="attention__item" :class="`attention__item--${w.tone}`">
          <span class="attention__mark" aria-hidden="true" />
          <div class="attention__body">
            <span>{{ w.text }}</span>
            <span v-if="w.names?.length" class="attention__names">
              <span v-for="n in w.names" :key="n" class="mono">{{ n }}</span>
            </span>
          </div>
        </li>
      </ul>
    </section>

    <!-- Подписки по провайдерам -->
    <div v-if="subsState.error && !subsState.data" class="notice notice--error" role="alert">
      <span class="notice__title">Список подписок не загрузился</span>
      <span>{{ subsState.error }}</span>
      <div class="notice__actions">
        <button type="button" class="btn btn--small" @click="hub.loadSubscriptions()">Загрузить снова</button>
      </div>
    </div>

    <div v-else-if="!subsState.data" class="panel ppanel" aria-busy="true">
      <div class="ppanel__head"><span class="skeleton" style="width: 16ch; height: 18px" /></div>
      <div class="ppanel__summary"><span class="skeleton" style="width: 10ch; height: 40px" /></div>
      <ul class="accts">
        <li v-for="i in 3" :key="i" class="acct">
          <span class="skeleton" style="width: 80%" />
          <span class="skeleton" style="width: 70%" />
          <span class="skeleton" style="width: 90%" />
        </li>
      </ul>
      <span class="visually-hidden">Загрузка подписок</span>
    </div>

    <div v-else-if="!subs.length" class="panel empty">
      <h2>На хабе пока нет подписок</h2>
      <p class="muted">Добавьте аккаунт Claude или Codex и сразу задайте ему собственный прокси.</p>
      <div><a class="btn btn--primary" :href="href('add')">Добавить аккаунт</a></div>
    </div>

    <div v-else-if="!panels.length" class="panel empty">
      <h2>Ничего не найдено</h2>
      <p class="muted">Под выбранные фильтры не подходит ни одна подписка.</p>
      <div><button type="button" class="btn" @click="reset">Сбросить фильтры</button></div>
    </div>

    <template v-else>
      <section
        v-for="p in panels"
        :key="p.key"
        class="panel ppanel"
        :aria-labelledby="`pp-${p.key}`"
      >
        <header class="ppanel__head">
          <h2 :id="`pp-${p.key}`" class="panel__title">
            <ProviderGlyph :provider="p.key" class="ppanel__glyph" />
            {{ providerLabel(p.key) }}
            <span class="panel__meta">{{ nAccounts(p.total) }}</span>
          </h2>
          <a class="btn btn--quiet btn--small" :href="href('subscriptions')" @click="openInSubscriptions(p.key)">
            Настроить<span class="visually-hidden"> подписки {{ providerLabel(p.key) }}</span>
          </a>
        </header>

        <div class="ppanel__summary">
          <p class="figure">
            <span class="figure__num">{{ p.enabled }}</span>
            <span class="figure__of">из {{ p.total }} {{ plural(p.enabled, 'включена', 'включены', 'включено') }}</span>
          </p>
          <dl class="stats">
            <div class="stat">
              <dt>Работают</dt>
              <dd>{{ p.ok }}</dd>
            </div>
            <div class="stat">
              <dt>С ошибкой</dt>
              <dd :class="{ 'tone--error': p.errors }">{{ p.errors }}</dd>
            </div>
            <div v-if="p.other" class="stat">
              <dt>Другой статус</dt>
              <dd>{{ p.other }}</dd>
            </div>
            <template v-for="r in ROUTE_STATS" :key="r.kind">
              <div v-if="r.kind !== 'unknown' || p.routes.unknown" class="stat" :class="`stat--${r.kind}`">
                <dt>{{ r.label }}</dt>
                <dd>{{ p.routes[r.kind] }}</dd>
              </div>
            </template>
          </dl>
        </div>

        <ul class="accts" :aria-label="`Аккаунты ${providerLabel(p.key)}`">
          <li v-for="r in p.rows" :key="r.sub.id" class="acct" :class="{ 'acct--off': r.sub.disabled }">
            <span class="acct__status" :class="`tone--${r.status.tone}`">
              <span class="dot" aria-hidden="true" />
              <span class="acct__label">{{ r.status.label }}</span>
            </span>
            <span class="acct__who">
              <span class="acct__name">{{ r.sub.name || r.sub.id }}</span>
              <span class="acct__email">{{ r.sub.email || 'почта не указана' }}</span>
            </span>
            <RouteLine class="acct__route" :kind="r.exit.kind" :label="r.exit.label" />
            <span class="acct__prefix">
              <template v-if="r.sub.prefix">
                <span class="visually-hidden">Префикс моделей: </span><span class="mono">{{ r.sub.prefix }}/</span>
              </template>
            </span>
          </li>
        </ul>
        <p v-if="p.rows.length < p.total" class="ppanel__hidden">
          Скрыто фильтрами: {{ p.total - p.rows.length }}
        </p>
      </section>

      <RouteLegend />
    </template>

    <!-- Точки выхода -->
    <section v-if="subsState.data && groups.length" class="panel" aria-labelledby="exits-title">
      <div class="panel__head">
        <h2 id="exits-title" class="panel__title">
          Точки выхода <span class="panel__meta">{{ groups.length }}</span>
        </h2>
        <p class="muted small">Через какой адрес подписки обращаются к провайдеру.</p>
      </div>
      <ul class="exits">
        <li v-for="g in groups" :key="g.exit.key" class="exit" :class="`exit--${g.exit.kind}`">
          <div class="exit__head">
            <RouteLine :kind="g.exit.kind" :label="g.exit.label" />
            <span class="exit__count">{{ nSubs(g.subs.length) }}</span>
          </div>
          <ul class="exit__subs">
            <li v-for="s in g.subs" :key="s.id" class="exit__sub" :class="{ 'exit__sub--off': s.disabled }">
              <ProviderGlyph :provider="s.provider" :size="14" />
              <span class="exit__name">{{ s.name }}</span>
              <span class="muted">{{ providerLabel(providerKey(s.provider)) }}</span>
              <span v-if="s.disabled" class="muted">отключена</span>
            </li>
          </ul>
        </li>
      </ul>
    </section>

    <!-- Модели -->
    <section class="panel" aria-labelledby="models-title">
      <div class="panel__head">
        <h2 id="models-title" class="panel__title">
          Модели
          <span v-if="models.ids" class="panel__meta">{{ models.ids.length }}</span>
        </h2>
        <p class="muted small">Что CLIProxyAPI сейчас отдаёт клиентам по <code>/v1/models</code>.</p>
      </div>
      <div class="models">
        <p v-if="models.loading" class="muted" aria-busy="true">Загружаю список моделей…</p>
        <div v-else-if="models.error" class="notice notice--error" role="alert">
          <span>Список моделей не получен: {{ models.error }}</span>
          <div class="notice__actions">
            <button type="button" class="btn btn--small" @click="loadModels">Запросить снова</button>
          </div>
        </div>
        <template v-else-if="models.ids">
          <p v-if="!models.ids.length" class="muted">Хаб не вернул ни одной модели. Обычно это значит, что нет включённых подписок.</p>
          <details v-else class="models__details">
            <summary>Показать {{ models.ids.length }} {{ plural(models.ids.length, 'модель', 'модели', 'моделей') }}</summary>
            <ul class="models__list">
              <li v-for="id in models.ids" :key="id" class="mono">{{ id }}</li>
            </ul>
          </details>
        </template>
        <details v-else-if="models.raw" class="models__details">
          <summary>Ответ в нестандартном формате</summary>
          <pre class="models__raw">{{ models.raw }}</pre>
        </details>
      </div>
    </section>
  </div>
</template>

<style scoped>
.controls {
  display: grid;
  gap: var(--space-3);
}

.hubinfo {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
  min-width: 0;
  max-width: none;
  color: var(--ink-2);
  font-size: var(--fs-md);
}
.hubinfo__addr {
  color: var(--ink);
  overflow-wrap: anywhere;
}
.hubinfo__ver {
  color: var(--ink-3);
  white-space: nowrap;
}

/* ── Требует внимания ───────────────────────────── */

.attention__list {
  display: grid;
  margin: 0;
  padding: 0 var(--space-5) var(--space-4);
  list-style: none;
}
.attention__item {
  --c: var(--ink-3);
  display: grid;
  grid-template-columns: 7px minmax(0, 1fr);
  gap: 14px;
  padding: 10px 0;
  border-top: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.attention__item:first-child {
  border-top: 0;
}
.attention__item--warn {
  --c: var(--route-direct);
}
.attention__item--error {
  --c: var(--err);
}
.attention__mark {
  width: 7px;
  height: 7px;
  margin-top: 8px;
  border-radius: 50%;
  background: var(--c);
}
.attention__body {
  display: grid;
  gap: 2px;
}
.attention__names {
  display: flex;
  flex-wrap: wrap;
  gap: 2px var(--space-3);
  color: var(--ink-2);
  font-size: var(--fs-sm);
}

/* ── Панель провайдера ──────────────────────────── */

.ppanel {
  display: grid;
}
.ppanel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-5) 0;
}
.ppanel__head .panel__title {
  align-items: center;
}
.ppanel__glyph {
  margin-right: 2px;
}

.ppanel__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4) var(--space-6);
  padding: var(--space-3) var(--space-5) var(--space-5);
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
  font-size: var(--fs-md);
}
.stat dd {
  margin: 0;
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.stat--custom dd {
  color: var(--route-custom);
}
.stat--default dd {
  color: var(--route-default);
}
.stat--direct dd {
  color: var(--route-direct);
}
.stat--unknown dd {
  color: var(--ink-2);
}

.accts {
  display: grid;
  margin: 0;
  padding: 0 var(--space-5) var(--space-3);
  list-style: none;
  border-top: 1px solid var(--rule);
}
.acct {
  display: grid;
  grid-template-columns: 180px minmax(0, 1.1fr) minmax(0, 1.6fr) minmax(64px, auto);
  align-items: center;
  gap: var(--space-4);
  padding: 12px 0;
  border-top: 1px solid var(--rule);
}
.acct:first-child {
  border-top: 0;
}
.acct--off .acct__who,
.acct--off .acct__route {
  opacity: 0.55;
}
.acct__status {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  font-size: var(--fs-sm);
  min-width: 0;
}
.acct__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.acct__who {
  display: grid;
  min-width: 0;
}
.acct__name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.acct__email {
  font-size: var(--fs-sm);
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.acct__prefix {
  justify-self: end;
  color: var(--ink-2);
  font-size: var(--fs-sm);
}

.ppanel__hidden {
  padding: 0 var(--space-5) var(--space-4);
  color: var(--ink-3);
  font-size: var(--fs-sm);
}


/* ── Точки выхода ───────────────────────────────── */

.exits {
  margin: 0;
  padding: 0 var(--space-5) var(--space-3);
  list-style: none;
}
.exit {
  --c: var(--route-direct);
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-3) var(--space-5);
  padding: var(--space-4) 0;
  border-top: 1px solid var(--rule);
}
.exit--custom {
  --c: var(--route-custom);
}
.exit--default {
  --c: var(--route-default);
}
.exit--unknown {
  --c: var(--ink-3);
}
.exit__head {
  display: grid;
  align-content: start;
  gap: var(--space-1);
  min-width: 0;
}
.exit__count {
  padding-left: calc(40px + var(--space-2));
  font-size: var(--fs-sm);
  color: var(--ink-2);
}
.exit__subs {
  display: grid;
  gap: 2px;
  margin: 0;
  padding: 0 0 0 var(--space-4);
  list-style: none;
  border-left: 1px solid color-mix(in srgb, var(--c) 55%, transparent);
}
.exit__sub {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  column-gap: 10px;
  padding: 3px 0;
  font-size: var(--fs-sm);
}
.exit__sub--off .exit__name {
  color: var(--ink-3);
}
.exit__name {
  font-weight: 500;
  overflow-wrap: anywhere;
}

/* ── Модели ─────────────────────────────────────── */

.models {
  padding: 0 var(--space-5) var(--space-5);
}
.models__details summary {
  cursor: pointer;
  width: fit-content;
  font-weight: 500;
  border-radius: 6px;
}
.models__list {
  columns: 3 16rem;
  column-gap: var(--space-5);
  margin: var(--space-3) 0 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
  color: var(--ink-2);
}
.models__list li {
  padding: 3px 0;
  break-inside: avoid;
  overflow-wrap: anywhere;
}
.models__raw {
  margin: var(--space-3) 0 0;
  padding: var(--space-4);
  overflow: auto;
  max-height: 320px;
  background: var(--ground);
  border-radius: var(--r-control);
  font-size: var(--fs-sm);
}

.empty {
  display: grid;
  gap: var(--space-3);
  justify-items: start;
  padding: var(--space-6);
}

@media (max-width: 1180px) {
  .acct {
    grid-template-columns: 150px minmax(0, 1fr) minmax(0, 1.3fr);
  }
  .acct__prefix {
    display: none;
  }
}

@media (max-width: 900px) {
  .hubinfo {
    margin-left: 0;
    flex-wrap: wrap;
  }
  .ppanel__head,
  .ppanel__summary,
  .accts,
  .exits,
  .models,
  .attention__list {
    padding-left: var(--space-4);
    padding-right: var(--space-4);
  }
  .ppanel__head {
    padding-top: var(--space-4);
  }
  .figure__num {
    font-size: 2.4rem;
  }
  .stats {
    gap: var(--space-3) var(--space-5);
  }
  .acct {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas:
      'who status'
      'route route';
    gap: 6px var(--space-3);
  }
  .acct__who {
    grid-area: who;
  }
  .acct__status {
    grid-area: status;
  }
  .acct__route {
    grid-area: route;
  }
  .acct__route :deep(.route__exit) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .exit {
    grid-template-columns: 1fr;
  }
  .exit__subs {
    margin-left: 19px;
  }
  .empty {
    padding: var(--space-5);
  }
}
</style>
