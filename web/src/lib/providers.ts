const NAMES: Record<string, string> = {
  claude: 'Claude',
  anthropic: 'Claude',
  codex: 'Codex',
  openai: 'OpenAI',
  gemini: 'Gemini',
  'gemini-cli': 'Gemini CLI',
  qwen: 'Qwen',
  iflow: 'iFlow',
  antigravity: 'Antigravity',
}

export function providerName(provider: string): string {
  const key = provider.trim().toLowerCase()
  if (!key) return 'Неизвестно'
  return NAMES[key] ?? provider
}

/** Синонимы, которые хаб может прислать для одного провайдера. */
const ALIASES: Record<string, string> = {
  anthropic: 'claude',
}

/** Канонический ключ провайдера для группировки и вкладок. Пустой провайдер — `unknown`. */
export function providerKey(provider: string): string {
  const key = provider.trim().toLowerCase()
  if (!key) return 'unknown'
  return ALIASES[key] ?? key
}

/** Подпись по каноническому ключу. */
export function providerLabel(key: string): string {
  return key === 'unknown' ? 'Неизвестно' : providerName(key)
}

const COLORS: Record<string, string> = {
  claude: 'var(--claude)',
  codex: 'var(--codex)',
  openai: '#9fd4c4',
  gemini: '#a7c1fb',
  'gemini-cli': '#a7c1fb',
  antigravity: '#7fa3ff',
  qwen: '#b9a3f5',
  iflow: '#e9c46a',
}

export function providerColor(key: string): string {
  return COLORS[key] ?? (key === 'unknown' ? 'var(--ink-3)' : '#c4c4c4')
}

/** Порядок вкладок: Claude, Codex, затем остальные по имени, «Неизвестно» в конце. */
export function compareProviders(a: string, b: string): number {
  const rank = (k: string) => (k === 'claude' ? 0 : k === 'codex' ? 1 : k === 'unknown' ? 3 : 2)
  return rank(a) - rank(b) || providerLabel(a).localeCompare(providerLabel(b))
}

export interface StatusInfo {
  label: string
  tone: 'ok' | 'muted' | 'error' | 'neutral'
}

export function subscriptionStatus(status: string, disabled: boolean): StatusInfo {
  if (disabled) return { label: 'Отключена', tone: 'muted' }
  switch (status.trim().toLowerCase()) {
    case 'active':
    case 'ok':
    case 'ready':
      return { label: 'Работает', tone: 'ok' }
    case 'error':
    case 'unavailable':
    case 'invalid':
      return { label: 'Ошибка', tone: 'error' }
    case 'disabled':
      return { label: 'Отключена', tone: 'muted' }
    case 'cooldown':
    case 'rate_limited':
      return { label: 'Ждёт сброса лимита', tone: 'neutral' }
    case '':
      return { label: 'Статус неизвестен', tone: 'neutral' }
    default:
      return { label: status, tone: 'neutral' }
  }
}

export function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod10 === 1 && mod100 !== 11) return one
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few
  return many
}
