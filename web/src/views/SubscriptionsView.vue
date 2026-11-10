<script setup lang="ts">
import { computed } from 'vue'
import type { Subscription } from '../api/types'
import AppIcon from '../components/AppIcon.vue'
import ProviderTabs from '../components/ProviderTabs.vue'
import RouteLegend from '../components/RouteLegend.vue'
import SegmentedFilter from '../components/SegmentedFilter.vue'
import SubscriptionRow from '../components/SubscriptionRow.vue'
import { useFilters } from '../composables/useFilters'
import { useHub } from '../composables/useHub'
import { plural } from '../lib/providers'
import { displayProxyUrl } from '../lib/proxy'
import { href } from '../lib/router'

const hub = useHub()
const subsState = computed(() => hub.state.subs)
const all = computed<readonly Subscription[]>(() => subsState.value.data?.subscriptions ?? [])
const defaultProxy = computed(() => subsState.value.data?.defaultProxy ?? '')

const { provider, items, state, exitKind, query, matches, active: filtersActive, reset } = useFilters(all, defaultProxy)

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

const filtered = computed(() => all.value.filter(matches))

const countText = computed(() => {
  const n = filtered.value.length
  const total = all.value.length
  const word = plural(total, 'подписки', 'подписок', 'подписок')
  return filtersActive.value ? `Показано ${n} из ${total} ${word}` : `${total} ${plural(total, 'подписка', 'подписки', 'подписок')}`
})
</script>

<template>
  <div class="page">
    <header class="page__head">
      <div class="page__title">
        <h1 tabindex="-1">Подписки</h1>
        <span class="page__meta" role="status" aria-live="polite">{{ subsState.data ? countText : '' }}</span>
      </div>
      <div class="page__actions">
        <button type="button" class="btn" :disabled="subsState.loading" @click="hub.loadSubscriptions()">
          <AppIcon name="refresh" />
          {{ subsState.loading ? 'Обновляю…' : 'Обновить список' }}
        </button>
        <a class="btn btn--primary" :href="href('add')">Добавить аккаунт</a>
      </div>
    </header>

    <p class="page__lede lede">
      У каждой подписки свой выход в интернет. Настройте его, чтобы провайдер видел аккаунт с постоянного адреса.
    </p>

    <div class="controls">
      <ProviderTabs v-model="provider" :items="items" :total="all.length" />
      <div class="filters" role="search" aria-label="Фильтр подписок">
        <SegmentedFilter v-model="state" label="Состояние" show-label :options="STATE_OPTIONS" />
        <SegmentedFilter v-model="exitKind" label="Выход" show-label :options="EXIT_OPTIONS" />
        <div class="search">
          <label class="visually-hidden" for="sub-search">Поиск по имени, почте или префиксу</label>
          <input
            id="sub-search"
            v-model="query"
            class="input"
            type="search"
            placeholder="Поиск по имени, почте или префиксу"
            autocomplete="off"
          />
        </div>
      </div>
    </div>

    <div class="strip">
      <div class="default-proxy" :class="{ 'default-proxy--unset': !defaultProxy }">
        <template v-if="subsState.data">
          <span class="default-proxy__label">Общий прокси хаба</span>
          <span v-if="defaultProxy" class="mono default-proxy__value">{{ displayProxyUrl(defaultProxy) }}</span>
          <span v-else class="default-proxy__value">не задан: CLIProxyAPI использует окружение процесса или IP хаба</span>
        </template>
        <span v-else class="skeleton" style="width: 28ch" />
      </div>
      <p class="strip__note">
        Маршруты отображаются по настройкам, сохранённым через ProxCli. Изменения вне панели не отслеживаются. Для импортированных аккаунтов задайте маршрут явно.
      </p>
    </div>

    <div v-if="subsState.error" class="notice notice--error" role="alert">
      <span class="notice__title">{{ subsState.data ? 'Список не обновился' : 'Список подписок не загрузился' }}</span>
      <span>{{ subsState.error }}</span>
      <div class="notice__actions">
        <button type="button" class="btn btn--small" @click="hub.loadSubscriptions()">Загрузить снова</button>
      </div>
    </div>

    <ul v-if="!subsState.data && !subsState.error" class="panel list" aria-busy="true">
      <li v-for="i in 3" :key="i" class="ghost">
        <span class="skeleton" style="width: 36px; height: 20px; border-radius: 999px" />
        <span class="skeleton" style="width: 70%" />
        <span class="skeleton" style="width: 50%" />
        <span class="skeleton" style="width: 80%" />
      </li>
      <li class="visually-hidden">Загрузка подписок</li>
    </ul>

    <div v-else-if="subsState.data && !all.length" class="panel empty">
      <h2>Подписок пока нет</h2>
      <p class="muted">Войдите в аккаунт Claude или Codex через хаб. Прокси для входа и для работы подписки можно выбрать сразу.</p>
      <div><a class="btn btn--primary" :href="href('add')">Добавить аккаунт</a></div>
    </div>

    <div v-else-if="subsState.data && !filtered.length" class="panel empty">
      <h2>Ничего не найдено</h2>
      <p class="muted">Под эти условия не подходит ни одна подписка.</p>
      <div><button type="button" class="btn" @click="reset">Сбросить фильтры</button></div>
    </div>

    <ul v-else-if="subsState.data" class="panel list" aria-label="Список подписок">
      <SubscriptionRow v-for="s in filtered" :key="s.id" :sub="s" :default-proxy="defaultProxy" />
    </ul>

    <RouteLegend />
  </div>
</template>

<style scoped>
.lede {
  margin-top: calc(-1 * var(--space-3));
}

.controls {
  display: grid;
  gap: var(--space-3);
}

.search {
  flex: 1 1 260px;
  max-width: 420px;
  margin-left: auto;
}
.search .input {
  min-height: 38px;
  background: var(--surface);
  border-color: transparent;
}
.search .input:hover {
  border-color: var(--rule-strong);
}

.strip {
  display: grid;
  grid-template-columns: minmax(0, auto) minmax(0, 1fr);
  align-items: center;
  gap: var(--space-2) var(--space-5);
}
.default-proxy {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 2px var(--space-3);
  padding: 10px var(--space-4);
  border: 1px dashed color-mix(in srgb, var(--route-default) 45%, transparent);
  border-radius: var(--r-control);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}
.default-proxy--unset {
  border-color: color-mix(in srgb, var(--route-direct) 40%, transparent);
}
.default-proxy__label {
  font-weight: 500;
}
.default-proxy__value {
  color: var(--ink-2);
}
.strip__note {
  color: var(--ink-3);
  font-size: var(--fs-sm);
}

.list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.ghost {
  display: grid;
  grid-template-columns: 36px 1.2fr 0.9fr 1.5fr;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-top: 1px solid var(--rule);
}
.ghost:first-child {
  border-top: 0;
}

.empty {
  display: grid;
  justify-items: start;
  gap: var(--space-3);
  padding: var(--space-6);
}


@media (max-width: 900px) {
  .strip {
    grid-template-columns: 1fr;
  }
  .search {
    flex-basis: 100%;
    max-width: none;
    margin-left: 0;
  }
}
@media (max-width: 560px) {
  .ghost {
    grid-template-columns: 36px 1fr;
  }
  .empty {
    padding: var(--space-5);
  }
}
</style>
