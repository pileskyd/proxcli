import { computed, ref, type Ref } from 'vue'
import type { Subscription } from '../api/types'
import { compareProviders, providerKey, providerName } from '../lib/providers'
import { effectiveExit, type ExitKind } from '../lib/proxy'

export type StateFilter = 'all' | 'on' | 'off'
export type ExitFilter = 'all' | ExitKind

/**
 * Фильтры общие для «Обзора» и «Подписок»: выбранная вкладка провайдера
 * и вторичные фильтры сохраняются при переходе между экранами.
 */
const provider = ref('all')
const state = ref<StateFilter>('all')
const exitKind = ref<ExitFilter>('all')
const query = ref('')

export function useFilters(subs: Ref<readonly Subscription[]>, defaultProxy: Ref<string>) {
  const items = computed(() => {
    const counts = new Map<string, number>()
    for (const s of subs.value) {
      const k = providerKey(s.provider)
      counts.set(k, (counts.get(k) ?? 0) + 1)
    }
    return [...counts.entries()]
      .map(([key, count]) => ({ key, count }))
      .sort((a, b) => compareProviders(a.key, b.key))
  })

  /** Если выбранный провайдер пропал из списка, показываем всех. */
  const activeProvider = computed({
    get: () => (provider.value === 'all' || items.value.some((i) => i.key === provider.value) ? provider.value : 'all'),
    set: (v: string) => {
      provider.value = v
    },
  })

  function matches(s: Subscription): boolean {
    if (activeProvider.value !== 'all' && providerKey(s.provider) !== activeProvider.value) return false
    if (state.value === 'on' && s.disabled) return false
    if (state.value === 'off' && !s.disabled) return false
    if (exitKind.value !== 'all' && effectiveExit(s, defaultProxy.value).kind !== exitKind.value) return false
    const q = query.value.trim().toLowerCase()
    if (!q) return true
    return [s.name, s.email, s.prefix, s.id, providerName(s.provider)].some((v) => v?.toLowerCase().includes(q))
  }

  const active = computed(
    () => query.value.trim() !== '' || activeProvider.value !== 'all' || state.value !== 'all' || exitKind.value !== 'all',
  )

  function reset() {
    provider.value = 'all'
    state.value = 'all'
    exitKind.value = 'all'
    query.value = ''
  }

  return { provider: activeProvider, items, state, exitKind, query, matches, active, reset }
}
