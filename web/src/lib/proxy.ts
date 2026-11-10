import type { ProxyMode, Subscription } from '../api/types'

export const PROXY_SCHEMES = ['socks5h', 'socks5', 'http', 'https'] as const
export type ProxyScheme = (typeof PROXY_SCHEMES)[number]

/** Так сервер маскирует сохранённый пароль в proxyUrl. */
export const MASKED_PASSWORD = '***'

export interface ParsedProxy {
  scheme: ProxyScheme
  host: string
  port: string
  username: string
  password: string
}

const URL_RE =
  /^(socks5h?|https?):\/\/(?:([^:@/\s]*)(?::([^@/\s]*))?@)?(\[[0-9a-fA-F:.]+\]|[^:/@\s[\]]+)(?::(\d{1,5}))?\/?$/i

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export function parseProxyUrl(raw: string): ParsedProxy | null {
  const m = URL_RE.exec(raw.trim())
  if (!m) return null
  const [, scheme = '', user = '', pass, host = '', port = ''] = m
  return {
    scheme: scheme.toLowerCase() as ProxyScheme,
    host,
    port,
    username: safeDecode(user),
    // Маску не декодируем: '***' — признак сохранённого пароля, а не сам пароль.
    password: pass === undefined ? '' : pass === MASKED_PASSWORD ? MASKED_PASSWORD : safeDecode(pass),
  }
}

export function buildProxyUrl(p: ParsedProxy): string {
  let auth = ''
  if (p.username || p.password) {
    auth = encodeURIComponent(p.username)
    if (p.password) auth += `:${encodeURIComponent(p.password)}`
    auth += '@'
  }
  return `${p.scheme}://${auth}${p.host.trim()}:${p.port.trim()}`
}

/** Для показа: пароль никогда не выводится, даже замаскированный сервером. */
export function displayProxyUrl(raw: string): string {
  const p = parseProxyUrl(raw)
  if (!p) return raw.replace(/:\/\/([^:@/]*):[^@/]*@/, '://$1:•••@')
  const auth = p.username ? `${p.username}${p.password ? ':•••' : ''}@` : p.password ? '•••@' : ''
  return `${p.scheme}://${auth}${p.host}${p.port ? `:${p.port}` : ''}`
}

export function hasStoredPassword(raw: string): boolean {
  return parseProxyUrl(raw)?.password === MASKED_PASSWORD
}

// ── Черновик редактора ──────────────────────────────────────────────

export interface ProxyDraft {
  mode: ProxyMode
  scheme: ProxyScheme
  host: string
  port: string
  username: string
  /** Новый пароль. Пустая строка при replacePassword=true означает «без пароля». */
  password: string
  /** На сервере уже есть пароль (пришёл как `***`). */
  storedPassword: boolean
  /** Пользователь решил заменить сохранённый пароль. */
  replacePassword: boolean
  /** Исходные значения для сравнения. */
  initial: {
    mode: ProxyMode
    proxy: ParsedProxy | null
    raw: string
  }
}

export function createDraft(mode: ProxyMode, proxyUrl: string): ProxyDraft {
  const parsed = proxyUrl ? parseProxyUrl(proxyUrl) : null
  const stored = parsed?.password === MASKED_PASSWORD
  return {
    mode,
    scheme: parsed?.scheme ?? 'socks5h',
    host: parsed?.host ?? '',
    port: parsed?.port ?? '',
    username: parsed?.username ?? '',
    password: stored ? '' : (parsed?.password ?? ''),
    storedPassword: stored,
    replacePassword: !stored,
    initial: { mode, proxy: parsed, raw: proxyUrl },
  }
}

/** Заполнить поля из вставленного целиком URL. Возвращает false, если строка не похожа на URL прокси. */
export function applyPastedUrl(draft: ProxyDraft, raw: string): boolean {
  const p = parseProxyUrl(raw)
  if (!p) return false
  draft.scheme = p.scheme
  draft.host = p.host
  draft.port = p.port
  draft.username = p.username
  if (p.password && p.password !== MASKED_PASSWORD) {
    draft.password = p.password
    draft.replacePassword = true
  } else if (!p.password) {
    draft.password = ''
    draft.replacePassword = true
  }
  return true
}

export type DraftField = 'host' | 'port' | 'password' | 'url'
export type DraftErrors = Partial<Record<DraftField, string>>

export type DraftResult =
  | { ok: true; proxyMode: ProxyMode; proxyUrl?: string; changed: boolean }
  | { ok: false; errors: DraftErrors }

function addressChanged(d: ProxyDraft): boolean {
  const p = d.initial.proxy
  if (!p) return true
  return (
    p.scheme !== d.scheme ||
    p.host !== d.host.trim() ||
    p.port !== d.port.trim() ||
    p.username !== d.username
  )
}

export function validateAddress(d: ProxyDraft): DraftErrors {
  const errors: DraftErrors = {}
  const host = d.host.trim()
  if (!host) errors.host = 'Укажите адрес прокси.'
  else if (/[\s/@]/.test(host) || (host.includes(':') && !host.startsWith('['))) {
    errors.host = 'Только имя хоста или IP, без схемы и порта. IPv6 — в квадратных скобках.'
  }
  const port = d.port.trim()
  if (!port) errors.port = 'Укажите порт.'
  else if (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535) {
    errors.port = 'Порт — число от 1 до 65535.'
  }
  return errors
}

/**
 * Превращает черновик в тело PATCH.
 * Неизменённый адрес с замаскированным паролем не отправляется:
 * custom без proxyUrl означает «сохранить текущее значение».
 */
export function resolveDraft(d: ProxyDraft): DraftResult {
  if (d.mode !== 'custom') {
    return { ok: true, proxyMode: d.mode, changed: d.mode !== d.initial.mode }
  }

  const wasCustomWithUrl = d.initial.mode === 'custom' && d.initial.proxy !== null
  const passwordTouched = d.storedPassword ? d.replacePassword : d.password !== (d.initial.proxy?.password ?? '')
  const changedAddress = addressChanged(d)

  if (wasCustomWithUrl && !changedAddress && !passwordTouched) {
    return { ok: true, proxyMode: 'custom', changed: false }
  }

  // Режим «свой», но на сервере есть URL, который мы не смогли разобрать, и поля пусты.
  if (d.initial.mode === 'custom' && d.initial.raw && !d.initial.proxy && !d.host.trim()) {
    return { ok: false, errors: { url: 'Сохранённый адрес не распознан. Введите прокси заново.' } }
  }

  const errors = validateAddress(d)
  if (d.storedPassword && !d.replacePassword && changedAddress) {
    errors.password =
      'Адрес изменился, поэтому введите пароль ещё раз: сервер не показывает сохранённый пароль и не может перенести его.'
  }
  if (Object.keys(errors).length) return { ok: false, errors }

  const url = buildProxyUrl({
    scheme: d.scheme,
    host: d.host,
    port: d.port,
    username: d.username,
    password: d.password,
  })
  return { ok: true, proxyMode: 'custom', proxyUrl: url, changed: true }
}

// ── Эффективный выход подписки ──────────────────────────────────────

export type ExitKind = 'custom' | 'default' | 'direct' | 'unknown'

export interface EffectiveExit {
  kind: ExitKind
  /** Ключ для группировки: одинаковые выходы — один ключ. */
  key: string
  label: string
}

export function effectiveExit(
  sub: Pick<Subscription, 'proxyMode' | 'proxyUrl' | 'proxyConfigured' | 'proxyKnown'>,
  defaultProxy: string,
): EffectiveExit {
  if (sub.proxyKnown === false) return { kind: 'unknown', key: 'unknown', label: 'Маршрут неизвестен — задайте его в ProxCli' }
  if (sub.proxyMode === 'custom') {
    if (sub.proxyUrl) {
      const label = displayProxyUrl(sub.proxyUrl)
      return { kind: 'custom', key: `proxy:${label}`, label }
    }
    if (sub.proxyConfigured) {
      return { kind: 'custom', key: `proxy:hidden`, label: 'Свой прокси (адрес скрыт сервером)' }
    }
    return { kind: 'direct', key: 'direct', label: 'Свой прокси не задан, запросы идут с IP хаба' }
  }
  if (sub.proxyMode === 'inherit' && defaultProxy && !['direct', 'none'].includes(defaultProxy)) {
    const label = displayProxyUrl(defaultProxy)
    return { kind: 'default', key: `proxy:${label}`, label }
  }
  return { kind: 'direct', key: 'direct', label: 'IP хаба' }
}

export const MODE_LABEL: Record<ProxyMode, string> = {
  inherit: 'Общий прокси',
  direct: 'Без прокси',
  custom: 'Свой прокси',
}
