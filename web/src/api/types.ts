export type ProxyMode = 'inherit' | 'direct' | 'custom'
export type OAuthProvider = 'claude' | 'codex'

export interface HubStatus {
  demo?: boolean
  connected: boolean
  upstream: string
  version?: string
  error?: string
}

export interface Subscription {
  id: string
  name: string
  provider: string
  email: string
  status: string
  disabled: boolean
  proxyMode: ProxyMode
  /** Пароль, если он есть, приходит замаскированным как `***`. */
  proxyUrl: string
  prefix: string
  proxyKnown?: boolean
  proxyConfigured?: boolean
}

export interface SubscriptionsResponse {
  subscriptions: Subscription[]
  defaultProxy: string
}

export interface SubscriptionPatch {
  name: string
  proxyMode: ProxyMode
  /** Не передаётся, если адрес не менялся: сервер сохранит текущий. */
  proxyUrl?: string
  prefix?: string
}

export interface OAuthStartRequest {
  provider: OAuthProvider
  proxyMode: ProxyMode
  proxyUrl?: string
}

export interface OAuthStartResponse {
  url: string
  state: string
}

export interface OAuthStatusResponse {
  status: string
  error?: string
}

export interface OkResponse {
  ok: true
}

// ── Квоты, история и маршрутизация ─────────────────────────

/** Наблюдённое окно лимита подписки. Процент — доля окна, а не токены. */
export interface QuotaWindow {
  id: string
  model?: string
  remainingPercent: number | null
  resetAt: string | null
  exhausted: boolean
}

export interface QuotaAccount {
  authId: string
  provider: string
  disabled: boolean
  /** Нулевая дата Go (`0001-01-01…`) означает «наблюдений не было». */
  observedAt: string
  source: string
  windows: QuotaWindow[] | null
  error?: string
}

export interface QuotaSnapshot {
  version: number
  updatedAt: string
  accounts: QuotaAccount[] | null
}

export interface QuotasResponse {
  snapshot: QuotaSnapshot
  /** Фоновый сборщик запущен. Не означает, что обновление идёт прямо сейчас. */
  collecting: boolean
  /** Ошибка последнего цикла сбора, пустая строка — без ошибки. */
  error: string
  staleAfterSeconds: number
}

export interface RefreshResponse {
  ok: boolean
  message?: string
}

export type HistoryRange = '6h' | '24h' | '7d' | '30d'

export interface TokenCounts {
  input: number
  output: number
  reasoning: number
  cacheRead: number
  cacheWrite: number
  total: number
}

export interface UsageBucket {
  at: string
  requests: number
  failed: number
  tokens: TokenCounts
}

export interface RoutingDecision {
  at: string
  session: string
  authId: string
  previousAuthId?: string
  provider: string
  model: string
  reason: string
}

export interface HistoryResponse {
  from: string
  to: string
  buckets: UsageBucket[] | null
  quotas: QuotaAccount[] | null
  decisions: RoutingDecision[] | null
  truncated: boolean
}

export interface RoutingAccount {
  authId: string
  provider: string
  priority: number
  resetAt: string | null
  reason: string
}

export interface UsageObserverStatus {
  active: boolean
  updatedAt?: string
  dropped?: number
  writeErrors?: number
  queued?: number
  lastHandledUnix?: number
}

export interface RoutingStatus {
  active: boolean
  enabled: boolean
  strategy?: string
  policy?: string
  updatedAt?: string
  error?: string
  accounts?: RoutingAccount[] | null
  usageObserver?: UsageObserverStatus
}
