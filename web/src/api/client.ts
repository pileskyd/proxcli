import type {
  HistoryRange,
  HistoryResponse,
  HubStatus,
  OAuthProvider,
  OAuthStartRequest,
  OAuthStartResponse,
  OAuthStatusResponse,
  OkResponse,
  QuotasResponse,
  RefreshResponse,
  RoutingStatus,
  SubscriptionPatch,
  SubscriptionsResponse,
} from './types'

export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const PANEL_DOWN =
  'Сервер ProxCli не отвечает. Запустите его (по умолчанию 127.0.0.1:8787) и обновите данные.'

function errorFromBody(data: unknown): string | null {
  if (data && typeof data === 'object' && 'error' in data) {
    const value = (data as { error: unknown }).error
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return null
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (init.body !== undefined) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(path, { ...init, headers: { ...headers, ...(init.headers as Record<string, string> | undefined) } })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(PANEL_DOWN, 0)
  }

  const text = await res.text()
  let data: unknown = null
  let parsed = false
  if (text) {
    try {
      data = JSON.parse(text)
      parsed = true
    } catch {
      parsed = false
    }
  }

  if (!res.ok) {
    const fromBody = errorFromBody(data)
    if (fromBody) throw new ApiError(fromBody, res.status)
    if (res.status === 502 || res.status === 503 || res.status === 504) {
      throw new ApiError(PANEL_DOWN, res.status)
    }
    throw new ApiError(`Сервер ответил кодом ${res.status} без описания ошибки.`, res.status)
  }

  if (text && !parsed) {
    throw new ApiError('Сервер вернул ответ не в формате JSON. Проверьте, что /api проксируется на ProxCli.', res.status)
  }
  return data as T
}

const json = (body: unknown) => JSON.stringify(body)

export const api = {
  status: (signal?: AbortSignal) => request<HubStatus>('/api/status', { signal }),

  subscriptions: (signal?: AbortSignal) =>
    request<SubscriptionsResponse>('/api/subscriptions', { signal }),

  updateSubscription: (patch: SubscriptionPatch) =>
    request<OkResponse>('/api/subscriptions', { method: 'PATCH', body: json(patch) }),

  setDisabled: (name: string, disabled: boolean) =>
    request<OkResponse>('/api/subscriptions/status', {
      method: 'PATCH',
      body: json({ name, disabled }),
    }),

  oauthStart: (body: OAuthStartRequest) =>
    request<OAuthStartResponse>('/api/oauth/start', { method: 'POST', body: json(body) }),

  oauthStatus: (state: string, signal?: AbortSignal) =>
    request<OAuthStatusResponse>(`/api/oauth/status?state=${encodeURIComponent(state)}`, { signal }),

  oauthCallback: (provider: OAuthProvider, url: string) =>
    request<OkResponse>('/api/oauth/callback', { method: 'POST', body: json({ provider, url }) }),

  models: (signal?: AbortSignal) => request<unknown>('/api/models', { signal }),

  quotas: (signal?: AbortSignal) => request<QuotasResponse>('/api/quotas', { signal }),

  refreshTelemetry: () =>
    request<RefreshResponse>('/api/telemetry/refresh', { method: 'POST', body: json({}) }),

  history: (params: { range: HistoryRange; provider?: string; authId?: string }, signal?: AbortSignal) => {
    const q = new URLSearchParams({ range: params.range })
    if (params.provider) q.set('provider', params.provider)
    if (params.authId) q.set('authId', params.authId)
    return request<HistoryResponse>(`/api/history?${q.toString()}`, { signal })
  },

  routing: (signal?: AbortSignal) => request<RoutingStatus>('/api/routing', { signal }),
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error && err.message) return err.message
  return 'Неизвестная ошибка.'
}

export function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}
