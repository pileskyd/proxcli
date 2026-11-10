<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppIcon, { type IconName } from './components/AppIcon.vue'
import BrandMark from './components/BrandMark.vue'
import { useHub } from './composables/useHub'
import { useToasts } from './composables/useToasts'
import { ROUTES, href, route, type RouteName } from './lib/router'
import AddAccountView from './views/AddAccountView.vue'
import ConnectView from './views/ConnectView.vue'
import HelpView from './views/HelpView.vue'
import OverviewView from './views/OverviewView.vue'
import QuotasView from './views/QuotasView.vue'
import SubscriptionsView from './views/SubscriptionsView.vue'

const hub = useHub()
const { toasts, dismiss } = useToasts()

const views = {
  overview: OverviewView,
  quotas: QuotasView,
  subscriptions: SubscriptionsView,
  add: AddAccountView,
  connect: ConnectView,
  help: HelpView,
} as const

const view = computed(() => views[route.value.name])

const ICONS: Record<RouteName, IconName> = {
  overview: 'overview',
  quotas: 'quota',
  subscriptions: 'subscriptions',
  add: 'add',
  connect: 'connect',
  help: 'help',
}

// Навигация разбита на группы: работа с аккаунтами и настройка клиентов.
const GROUPS: RouteName[][] = [['overview', 'quotas', 'subscriptions', 'add'], ['connect', 'help']]
const navGroups = computed(() =>
  GROUPS.map((names) => names.map((n) => ROUTES.find((r) => r.name === n)!).filter(Boolean)),
)

const subsCount = computed(() => hub.state.subs.data?.subscriptions.length ?? null)

const hubLine = computed(() => {
  const s = hub.state.status
  if (s.error) return { tone: 'error', text: 'Панель не отвечает', detail: 'Сервер ProxCli на :8787 недоступен' }
  if (!s.data) return { tone: 'pending', text: 'Проверяю хаб…', detail: '' }
  if (s.data.connected) return { tone: 'ok', text: 'Хаб на связи', detail: s.data.upstream }
  return { tone: 'down', text: 'Хаб недоступен', detail: s.data.upstream || 'адрес не задан' }
})

// Проверка статуса: при открытии, раз в 30 с на видимой вкладке и при возврате на вкладку.
let timer = 0
function onVisible() {
  if (document.visibilityState === 'visible') void hub.loadStatus()
}
onMounted(() => {
  void hub.refreshAll()
  timer = window.setInterval(() => {
    if (document.visibilityState === 'visible') void hub.loadStatus()
  }, 30_000)
  document.addEventListener('visibilitychange', onVisible)
})
onBeforeUnmount(() => {
  window.clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisible)
})

// ── Боковая панель на узком экране ───────────────────────
const drawerOpen = ref(false)
const menuBtn = ref<HTMLButtonElement | null>(null)
const rail = ref<HTMLElement | null>(null)

function openDrawer() {
  drawerOpen.value = true
  nextTick(() => rail.value?.querySelector<HTMLElement>('[aria-current="page"], .nav__link')?.focus())
}
function closeDrawer(returnFocus = true) {
  if (!drawerOpen.value) return
  drawerOpen.value = false
  if (returnFocus) nextTick(() => menuBtn.value?.focus())
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && drawerOpen.value) closeDrawer()
}
// Если окно расширили при открытом меню, закрываем его: на широком экране панель всегда видна.
const wide = window.matchMedia('(min-width: 901px)')
function onWide(e: MediaQueryListEvent) {
  if (e.matches) closeDrawer(false)
}
onMounted(() => {
  document.addEventListener('keydown', onKey)
  wide.addEventListener('change', onWide)
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  wide.removeEventListener('change', onWide)
})

// При смене экрана переносим фокус на заголовок — для клавиатуры и скринридеров.
const main = ref<HTMLElement | null>(null)
watch(
  () => route.value.name,
  () => {
    document.title = `${route.value.title} · ProxCli`
    closeDrawer(false)
    nextTick(() => {
      main.value?.querySelector<HTMLElement>('h1')?.focus({ preventScroll: true })
      window.scrollTo({ top: 0 })
    })
  },
)
document.title = `${route.value.title} · ProxCli`

const refreshing = computed(() => hub.state.status.loading || hub.state.subs.loading)
</script>

<template>
  <a class="skip" href="#main">Перейти к содержимому</a>

  <div class="mbar">
    <button
      ref="menuBtn"
      type="button"
      class="btn btn--icon"
      aria-controls="rail"
      :aria-expanded="drawerOpen"
      aria-label="Открыть меню"
      @click="openDrawer"
    >
      <AppIcon name="menu" />
    </button>
    <a class="brand" :href="href('overview')" aria-label="ProxCli, обзор">
      <BrandMark />
      <span class="brand__name">ProxCli</span>
    </a>
    <span class="mbar__dot" :class="`hub--${hubLine.tone}`" :title="hubLine.text">
      <span class="dot" aria-hidden="true" />
      <span class="visually-hidden">{{ hubLine.text }}</span>
    </span>
    <button
      type="button"
      class="btn btn--icon"
      :class="{ 'is-spinning': refreshing }"
      :disabled="refreshing"
      :aria-label="refreshing ? 'Обновляю данные' : 'Обновить данные'"
      @click="hub.refreshAll()"
    >
      <AppIcon name="refresh" />
    </button>
  </div>

  <div class="shell">
    <div v-if="drawerOpen" class="scrim" aria-hidden="true" @click="closeDrawer()" />

    <aside id="rail" ref="rail" class="rail" :class="{ 'rail--open': drawerOpen }" aria-label="Боковая панель">
      <div class="rail__top">
        <a class="brand" :href="href('overview')" aria-label="ProxCli, обзор">
          <BrandMark />
          <span class="brand__name">ProxCli</span>
        </a>
        <button type="button" class="btn btn--icon rail__close" aria-label="Закрыть меню" @click="closeDrawer()">
          <AppIcon name="close" />
        </button>
      </div>

      <nav class="nav" aria-label="Разделы">
        <ul v-for="(group, gi) in navGroups" :key="gi" class="nav__group">
          <li v-for="r in group" :key="r.name">
            <a class="nav__link" :href="href(r.name)" :aria-current="route.name === r.name ? 'page' : undefined">
              <AppIcon :name="ICONS[r.name]" />
              <span class="nav__text">{{ r.title }}</span>
              <span v-if="r.name === 'subscriptions' && subsCount !== null" class="nav__count">
                <span class="visually-hidden">, всего </span>{{ subsCount }}
              </span>
            </a>
          </li>
        </ul>
      </nav>

      <div class="rail__foot">
        <div class="hubline" :class="`hub--${hubLine.tone}`" role="status" aria-live="polite">
          <span class="dot hubline__dot" aria-hidden="true" />
          <span class="hubline__text">
            <span class="hubline__title">{{ hubLine.text }}</span>
            <span v-if="hubLine.detail" class="hubline__detail mono">{{ hubLine.detail }}</span>
            <span v-if="hub.state.status.data?.version" class="hubline__detail">
              CLIProxyAPI {{ hub.state.status.data.version }}
            </span>
          </span>
        </div>
      </div>
    </aside>

    <div class="work" :inert="drawerOpen || undefined">
      <div class="utility">
        <button
          type="button"
          class="btn btn--icon"
          :class="{ 'is-spinning': refreshing }"
          :disabled="refreshing"
          :aria-label="refreshing ? 'Обновляю данные' : 'Обновить данные'"
          :title="refreshing ? 'Обновляю…' : 'Обновить данные'"
          @click="hub.refreshAll()"
        >
          <AppIcon name="refresh" />
        </button>
        <a class="btn btn--icon" :href="href('help')" aria-label="Справка" title="Справка">
          <AppIcon name="help" />
        </a>
      </div>

      <main id="main" ref="main" class="main">
        <p v-if="hub.state.status.data?.demo" class="demo-banner" role="note">Демонстрационные данные — реальные аккаунты и запросы не используются</p>
        <component :is="view" />
      </main>
    </div>
  </div>

  <div class="toasts" role="region" aria-label="Уведомления" aria-live="polite">
    <div v-for="t in toasts" :key="t.id" class="toast" :class="`toast--${t.tone}`">
      <span class="dot toast__dot" aria-hidden="true" />
      <span>{{ t.text }}</span>
      <button type="button" class="toast__close" aria-label="Закрыть уведомление" @click="dismiss(t.id)">
        <AppIcon name="close" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.demo-banner { margin: 0 0 20px; padding: 9px 14px; border: 1px solid #4d4236; border-radius: 8px; color: #d6b993; background: #17130f; font-size: 12px; }

.skip {
  position: absolute;
  left: var(--space-3);
  top: -100px;
  z-index: 40;
  padding: var(--space-2) var(--space-3);
  background: #fafafa;
  color: var(--on-ink);
  border-radius: var(--r-control);
}
.skip:focus {
  top: var(--space-3);
}

.shell {
  display: grid;
  grid-template-columns: var(--rail) minmax(0, 1fr);
  min-height: 100vh;
}

/* ── Боковая панель ─────────────────────────────── */

.rail {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  height: 100vh;
  padding: 20px 12px 16px;
  background: var(--ground);
  overflow-y: auto;
}

.rail__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px;
}
.rail__close {
  display: none;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  text-decoration: none;
  border-radius: var(--r-control);
}
.brand__name {
  font-size: var(--fs-lg);
  font-weight: 650;
  letter-spacing: -0.02em;
}

.nav {
  display: grid;
  gap: 18px;
}
.nav__group {
  display: grid;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.nav__link {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 42px;
  padding: 0 12px;
  border-radius: 9px;
  color: var(--ink);
  font-size: var(--fs-lg);
  text-decoration: none;
  transition: background-color 120ms ease;
}
.nav__link .icon {
  color: var(--ink-2);
}
.nav__link:hover {
  background: var(--surface-2);
}
.nav__link[aria-current='page'] {
  background: var(--surface-3);
  font-weight: 500;
}
.nav__link[aria-current='page'] .icon {
  color: var(--ink);
}
.nav__link:focus-visible {
  outline-offset: -2px;
  border-radius: 9px;
}
.nav__text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nav__count {
  margin-left: auto;
  color: var(--ink-2);
  font-size: var(--fs-md);
  font-variant-numeric: tabular-nums;
}

.rail__foot {
  margin-top: auto;
  padding: 0 8px;
}

.hub--ok {
  color: var(--ok);
}
.hub--down {
  color: var(--route-direct);
}
.hub--error {
  color: var(--err);
}
.hub--pending {
  color: var(--ink-3);
}

.hubline {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.hubline__dot {
  margin-top: 7px;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 18%, transparent);
}
.hub--pending .hubline__dot {
  animation: skeleton 1.4s ease-in-out infinite;
}
.hubline__text {
  display: grid;
  min-width: 0;
}
.hubline__title {
  color: var(--ink);
  font-size: var(--fs-sm);
  font-weight: 500;
}
.hubline__detail {
  font-size: var(--fs-xs);
  color: var(--ink-3);
  overflow-wrap: anywhere;
}

/* ── Рабочая область ────────────────────────────── */

.work {
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.utility {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  padding: 24px 40px 4px;
}
.is-spinning .icon {
  animation: spin 0.9s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.main {
  min-width: 0;
}

.mbar {
  display: none;
}

/* ── Уведомления ────────────────────────────────── */

.toasts {
  position: fixed;
  right: var(--space-5);
  bottom: var(--space-5);
  z-index: 50;
  display: grid;
  gap: var(--space-2);
  max-width: min(420px, calc(100vw - 2 * var(--space-4)));
}
.toast {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 8px 10px 14px;
  border: 1px solid var(--rule-strong);
  border-radius: var(--r-control);
  background: var(--surface-2);
  color: var(--ink);
  font-size: var(--fs-sm);
  box-shadow: 0 12px 32px -12px #000;
}
.toast__dot {
  margin-top: 7px;
  color: var(--ok);
}
.toast--error {
  border-color: color-mix(in srgb, var(--err) 40%, transparent);
}
.toast--error .toast__dot {
  color: var(--err);
}
.toast__close {
  display: grid;
  place-items: center;
  margin-left: auto;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: none;
  color: var(--ink-2);
  cursor: pointer;
}
.toast__close .icon {
  width: 15px;
  height: 15px;
}
.toast__close:hover {
  color: var(--ink);
  background: var(--surface-3);
}

/* ── Узкий экран: панель уезжает в выдвижное меню ── */

@media (max-width: 900px) {
  .shell {
    grid-template-columns: 1fr;
  }
  .mbar {
    position: sticky;
    top: 0;
    z-index: 20;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 12px;
    background: color-mix(in srgb, var(--ground) 88%, transparent);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--rule);
  }
  .mbar__dot {
    margin-left: auto;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
  }
  .rail {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 30;
    width: min(var(--rail), 84vw);
    border-right: 1px solid var(--rule);
    transform: translateX(-100%);
    visibility: hidden;
    transition:
      transform 200ms ease,
      visibility 0s linear 200ms;
  }
  .rail--open {
    transform: none;
    visibility: visible;
    transition: transform 200ms ease;
  }
  .rail__close {
    display: inline-flex;
  }
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 25;
    background: rgb(0 0 0 / 0.6);
  }
  .utility {
    display: none;
  }
  .main {
    padding-top: var(--space-5);
  }
}
</style>
