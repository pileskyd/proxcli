import { reactive, readonly } from 'vue'
import { api, errorMessage, isAbort } from '../api/client'
import type { HubStatus, Subscription, SubscriptionsResponse } from '../api/types'

interface Resource<T> {
  data: T | null
  loading: boolean
  error: string | null
  /** Время последней успешной загрузки. */
  loadedAt: number | null
}

function resource<T>(): Resource<T> {
  return { data: null, loading: false, error: null, loadedAt: null }
}

const state = reactive({
  status: resource<HubStatus>(),
  subs: resource<SubscriptionsResponse>(),
})

let statusCtl: AbortController | null = null
let subsCtl: AbortController | null = null

async function loadStatus(): Promise<void> {
  statusCtl?.abort()
  const ctl = new AbortController()
  statusCtl = ctl
  state.status.loading = true
  try {
    state.status.data = await api.status(ctl.signal)
    state.status.error = null
    state.status.loadedAt = Date.now()
  } catch (err) {
    if (isAbort(err)) return
    state.status.error = errorMessage(err)
  } finally {
    if (statusCtl === ctl) state.status.loading = false
  }
}

async function loadSubscriptions(): Promise<void> {
  subsCtl?.abort()
  const ctl = new AbortController()
  subsCtl = ctl
  state.subs.loading = true
  try {
    const data = await api.subscriptions(ctl.signal)
    state.subs.data = {
      defaultProxy: data?.defaultProxy ?? '',
      subscriptions: Array.isArray(data?.subscriptions) ? data.subscriptions : [],
    }
    state.subs.error = null
    state.subs.loadedAt = Date.now()
  } catch (err) {
    if (isAbort(err)) return
    state.subs.error = errorMessage(err)
  } finally {
    if (subsCtl === ctl) state.subs.loading = false
  }
}

function refreshAll(): Promise<void> {
  return Promise.all([loadStatus(), loadSubscriptions()]).then(() => undefined)
}

/** Локально применить изменение, подтверждённое сервером, без мигания списка. */
function patchLocal(name: string, patch: Partial<Subscription>): void {
  const list = state.subs.data?.subscriptions
  if (!list) return
  const item = list.find((s) => s.name === name)
  if (item) Object.assign(item, patch)
}

export function useHub() {
  return {
    state: readonly(state),
    loadStatus,
    loadSubscriptions,
    refreshAll,
    patchLocal,
  }
}
