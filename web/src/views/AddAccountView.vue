<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { ApiError, api, errorMessage, isAbort } from '../api/client'
import type { OAuthProvider, OAuthStartRequest } from '../api/types'
import ProviderGlyph from '../components/ProviderGlyph.vue'
import ProxyFields from '../components/ProxyFields.vue'
import { useHub } from '../composables/useHub'
import { useToasts } from '../composables/useToasts'
import { createDraft, resolveDraft, type DraftErrors, type ProxyDraft } from '../lib/proxy'
import { href } from '../lib/router'

type Phase = 'form' | 'starting' | 'waiting' | 'done' | 'failed' | 'expired'

const POLL_MS = 2000
const POLL_LIMIT_MS = 10 * 60 * 1000
const MAX_POLL_FAILURES = 5

const hub = useHub()
const { push } = useToasts()

const provider = ref<OAuthProvider>('claude')
const draft = ref<ProxyDraft>(createDraft('custom', ''))
const errors = ref<DraftErrors>({})

const phase = ref<Phase>('form')
const formError = ref('')
const setupNote = ref('')
const loginUrl = ref('')
const loginState = ref('')
const failure = ref('')
const pollWarning = ref('')
const startedAt = ref(0)
const now = ref(Date.now())

const callbackUrl = ref('')
const callbackError = ref('')
const callbackBusy = ref(false)

const providers: { id: OAuthProvider; title: string; text: string; port: number }[] = [
  { id: 'claude', title: 'Claude', text: 'Подписка Claude Pro или Max.', port: 54545 },
  { id: 'codex', title: 'Codex', text: 'Подписка ChatGPT с доступом к Codex.', port: 1455 },
]
const current = computed(() => providers.find((p) => p.id === provider.value) ?? providers[0]!)

let pollCtl: AbortController | null = null
let pollTimer = 0
let clockTimer = 0
let failures = 0

function stopPolling() {
  pollCtl?.abort()
  pollCtl = null
  window.clearTimeout(pollTimer)
  window.clearInterval(clockTimer)
}

onBeforeUnmount(stopPolling)

const elapsed = computed(() => {
  const s = Math.max(0, Math.floor((now.value - startedAt.value) / 1000))
  const m = Math.floor(s / 60)
  return `${m}:${String(s % 60).padStart(2, '0')}`
})

async function start() {
  formError.value = ''
  setupNote.value = ''
  const res = resolveDraft(draft.value)
  if (!res.ok) {
    errors.value = res.errors
    nextTick(() => document.querySelector<HTMLElement>('.add [aria-invalid="true"]')?.focus())
    return
  }
  errors.value = {}

  const body: OAuthStartRequest = { provider: provider.value, proxyMode: res.proxyMode }
  if (res.proxyUrl) body.proxyUrl = res.proxyUrl

  phase.value = 'starting'
  try {
    const data = await api.oauthStart(body)
    if (!data?.url || !data?.state) throw new Error('Хаб не вернул ссылку для входа.')
    if (!/^https?:\/\//i.test(data.url)) throw new Error('Хаб вернул ссылку для входа в неожиданном формате.')
    loginUrl.value = data.url
    loginState.value = data.state
    callbackUrl.value = ''
    callbackError.value = ''
    failure.value = ''
    pollWarning.value = ''
    startedAt.value = Date.now()
    now.value = Date.now()
    failures = 0
    phase.value = 'waiting'
    clockTimer = window.setInterval(() => (now.value = Date.now()), 1000)
    schedulePoll()
    nextTick(() => document.getElementById('login-step')?.focus())
  } catch (err) {
    phase.value = 'form'
    if (err instanceof ApiError && err.status === 409) {
      setupNote.value = err.message
    } else {
      formError.value = errorMessage(err)
    }
  }
}

function schedulePoll() {
  window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(poll, POLL_MS)
}

async function poll() {
  if (phase.value !== 'waiting') return
  if (Date.now() - startedAt.value > POLL_LIMIT_MS) {
    stopPolling()
    phase.value = 'expired'
    return
  }
  pollCtl = new AbortController()
  try {
    const res = await api.oauthStatus(loginState.value, pollCtl.signal)
    failures = 0
    pollWarning.value = ''
    const s = (res?.status ?? '').toLowerCase()
    if (s === 'ok' || s === 'success' || s === 'done' || s === 'completed') {
      finish()
      return
    }
    if (s === 'error' || s === 'failed') {
      stopPolling()
      failure.value = res.error || 'Хаб сообщил об ошибке входа без подробностей.'
      phase.value = 'failed'
      return
    }
  } catch (err) {
    if (isAbort(err)) return
    failures++
    pollWarning.value = `Не удаётся узнать статус входа: ${errorMessage(err)}`
    if (failures >= MAX_POLL_FAILURES) {
      stopPolling()
      failure.value = `Связь с панелью потеряна: ${errorMessage(err)}`
      phase.value = 'failed'
      return
    }
  }
  schedulePoll()
}

function finish() {
  stopPolling()
  phase.value = 'done'
  push(`Аккаунт ${current.value.title} добавлен`)
  void hub.loadSubscriptions()
  nextTick(() => document.getElementById('done-title')?.focus())
}

function cancel() {
  stopPolling()
  phase.value = 'form'
  loginUrl.value = ''
  loginState.value = ''
}

function reset() {
  cancel()
  draft.value = createDraft('custom', '')
  setupNote.value = ''
  formError.value = ''
  failure.value = ''
}

async function sendCallback() {
  callbackError.value = ''
  const value = callbackUrl.value.trim()
  if (!value) {
    callbackError.value = 'Вставьте адрес из адресной строки браузера.'
    return
  }
  if (!/^https?:\/\//i.test(value) || !/[?&]code=/.test(value)) {
    callbackError.value = 'В адресе нет параметра code. Скопируйте адрес целиком, вместе с ?code=…&state=…'
    return
  }
  callbackBusy.value = true
  try {
    await api.oauthCallback(provider.value, value)
    push('Адрес передан хабу, жду завершения входа')
    // Хаб сам завершит обмен кода; опрос статуса подтвердит результат.
    if (phase.value === 'waiting') {
      window.clearTimeout(pollTimer)
      void poll()
    }
  } catch (err) {
    callbackError.value = errorMessage(err)
  } finally {
    callbackBusy.value = false
  }
}

const copyState = ref('')
async function copyUrl() {
  try {
    await navigator.clipboard.writeText(loginUrl.value)
    copyState.value = 'Ссылка скопирована'
  } catch {
    copyState.value = 'Не удалось скопировать, выделите ссылку вручную'
  }
  window.setTimeout(() => (copyState.value = ''), 2400)
}

const locked = computed(() => phase.value !== 'form')
</script>

<template>
  <div class="page add">
    <header class="page__head">
      <div>
        <h1 tabindex="-1">Добавить аккаунт</h1>
        <p class="page__lede">
          Вход выполняется на хабе через OAuth. Выберите выход заранее: выбранный прокси будет использоваться при обмене OAuth-кода и последующих запросах.
        </p>
      </div>
    </header>

    <ol class="panel steps">
      <li class="step">
        <h2 class="step__title" id="step-provider">Провайдер</h2>
        <div class="choice" role="radiogroup" aria-labelledby="step-provider">
          <label v-for="p in providers" :key="p.id" class="choice__opt" :class="{ 'choice__opt--on': provider === p.id }">
            <input v-model="provider" type="radio" name="oauth-provider" :value="p.id" :disabled="locked" />
            <span class="choice__title"><ProviderGlyph :provider="p.id" />{{ p.title }}</span>
            <span class="choice__text">{{ p.text }}</span>
          </label>
        </div>
      </li>

      <li class="step">
        <h2 class="step__title">Выход для входа</h2>
        <p class="muted step__text">
          Обычно это тот же прокси, что будет у подписки. Пароль прокси уходит только на хаб и в браузере не сохраняется.
        </p>
        <fieldset class="bare" :disabled="locked">
          <ProxyFields
            v-model="draft"
            id-base="oauth"
            :default-proxy="hub.state.subs.data?.defaultProxy ?? ''"
            :errors="errors"
            legend="Через что выполнить вход"
          />
        </fieldset>

        <div v-if="setupNote" class="notice notice--warn" role="alert">
          <span class="notice__title">Выбранный режим входа недоступен</span>
          <span>{{ setupNote }}</span>
          <span class="muted">Для собственного прокси выберите «Свой прокси»: он применяется уже при входе.</span>
        </div>
        <div v-if="formError" class="notice notice--error" role="alert">
          <span class="notice__title">Вход не начался</span>
          <span>{{ formError }}</span>
        </div>

        <div v-if="phase === 'form' || phase === 'starting'" class="step__actions">
          <button type="button" class="btn btn--primary" :disabled="phase === 'starting'" @click="start">
            {{ phase === 'starting' ? 'Запрашиваю ссылку…' : `Начать вход в ${current.title}` }}
          </button>
        </div>
      </li>

      <li class="step" :class="{ 'step--idle': phase === 'form' || phase === 'starting' }">
        <h2 id="login-step" class="step__title" tabindex="-1">Вход в браузере</h2>

        <p v-if="phase === 'form' || phase === 'starting'" class="muted step__text">
          Здесь появится ссылка на страницу входа {{ current.title }}.
        </p>

        <template v-else-if="phase === 'waiting'">
          <p class="step__text">Откройте ссылку, войдите в аккаунт и подтвердите доступ. Панель сама заметит, когда вход завершится.</p>
          <div class="link">
            <a class="btn btn--primary" :href="loginUrl" target="_blank" rel="noopener noreferrer">Открыть страницу входа</a>
            <button type="button" class="btn" @click="copyUrl">Скопировать ссылку</button>
            <span class="small muted" role="status">{{ copyState }}</span>
          </div>
          <label class="visually-hidden" for="login-url">Ссылка для входа</label>
          <input id="login-url" class="input input--mono" type="text" readonly :value="loginUrl" @focus="($event.target as HTMLInputElement).select()" />

          <p class="waiting" role="status">
            <span class="waiting__dot" aria-hidden="true" />
            Жду подтверждения от хаба, прошло {{ elapsed }}
          </p>
          <p v-if="pollWarning" class="notice notice--warn">{{ pollWarning }}</p>

          <details class="manual">
            <summary>Браузер показал ошибку на localhost после входа</summary>
            <div class="manual__body">
              <p class="small">
                Так бывает, когда хаб на другой машине: провайдер возвращает браузер на
                <code>localhost:{{ current.port }}</code>, а там никто не слушает. Скопируйте адрес из адресной строки целиком и передайте его хабу.
              </p>
              <div class="field">
                <label class="field__label" for="callback-url">Адрес после входа</label>
                <textarea
                  id="callback-url"
                  v-model="callbackUrl"
                  class="textarea"
                  rows="3"
                  spellcheck="false"
                  :placeholder="`http://localhost:${current.port}/callback?code=…&state=…`"
                  :aria-invalid="callbackError ? 'true' : undefined"
                  :aria-describedby="callbackError ? 'callback-err' : undefined"
                />
                <span v-if="callbackError" id="callback-err" class="field__error" role="alert">{{ callbackError }}</span>
              </div>
              <div>
                <button type="button" class="btn" :disabled="callbackBusy" @click="sendCallback">
                  {{ callbackBusy ? 'Передаю…' : 'Передать адрес хабу' }}
                </button>
              </div>
            </div>
          </details>

          <div>
            <button type="button" class="btn btn--quiet" @click="cancel">Прекратить ожидание</button>
          </div>
        </template>

        <div v-else-if="phase === 'done'" class="notice notice--ok">
          <span id="done-title" class="notice__title" tabindex="-1">Аккаунт {{ current.title }} добавлен</span>
          <span>Проверьте маршрут новой подписки: если хаб не закрепил прокси автоматически, задайте его вручную.</span>
          <div class="notice__actions">
            <a class="btn btn--primary btn--small" :href="href('subscriptions')">Открыть подписки</a>
            <button type="button" class="btn btn--small" @click="reset">Добавить ещё один</button>
          </div>
        </div>

        <div v-else-if="phase === 'failed'" class="notice notice--error" role="alert">
          <span class="notice__title">Вход не завершён</span>
          <span>{{ failure }}</span>
          <div class="notice__actions">
            <button type="button" class="btn btn--small" @click="cancel">Попробовать снова</button>
          </div>
        </div>

        <div v-else-if="phase === 'expired'" class="notice notice--warn" role="alert">
          <span class="notice__title">Ожидание остановлено через 10 минут</span>
          <span>Если вы уже вошли, обновите список подписок. Иначе начните вход заново: старая ссылка могла истечь.</span>
          <div class="notice__actions">
            <button type="button" class="btn btn--small" @click="hub.loadSubscriptions()">Обновить подписки</button>
            <button type="button" class="btn btn--small" @click="cancel">Начать заново</button>
          </div>
        </div>
      </li>
    </ol>

    <aside class="aside small">
      <p>
        Хаб без браузера? Войти можно и в терминале на хабе:
        <code>./cli-proxy-api -claude-login -no-browser</code> или <code>./cli-proxy-api -codex-device-login</code>.
        Такой вход пойдёт без выбранного здесь прокси.
      </p>
    </aside>
  </div>
</template>

<style scoped>
.steps {
  display: grid;
  gap: 0;
  margin: 0;
  padding: var(--space-6) var(--space-6) var(--space-2);
  list-style: none;
  counter-reset: step;
  max-width: 900px;
}

/* Шаги — настоящая последовательность, поэтому номера здесь уместны. */
.step {
  position: relative;
  display: grid;
  gap: var(--space-4);
  padding: 0 0 var(--space-6) 48px;
  counter-increment: step;
}
.step::before {
  content: counter(step);
  position: absolute;
  left: 0;
  top: -3px;
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #fafafa;
  color: var(--on-ink);
  font-weight: 600;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.step::after {
  content: '';
  position: absolute;
  left: 13.5px;
  top: 32px;
  bottom: 6px;
  border-left: 1px solid var(--rule-strong);
}
.step:last-child::after {
  display: none;
}
.step--idle::before {
  background: var(--surface-3);
  color: var(--ink-2);
}
.step--idle .step__title {
  color: var(--ink-2);
}
.step__title {
  font-size: var(--fs-lg);
  outline: none;
}
.step__text {
  margin-top: calc(-1 * var(--space-2));
}
.step__actions {
  display: flex;
  gap: var(--space-2);
}

.bare {
  margin: 0;
  padding: 0;
  border: 0;
  min-width: 0;
}

.choice {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
  max-width: 560px;
}
.choice__opt {
  position: relative;
  display: grid;
  align-content: start;
  gap: 4px;
  padding: var(--space-3) var(--space-3) var(--space-3) calc(var(--space-3) + 24px);
  border: 1px solid var(--rule-strong);
  border-radius: var(--r-control);
  background: #0a0a0a;
  cursor: pointer;
  transition: border-color 120ms ease, background-color 120ms ease;
}
.choice__opt:hover {
  border-color: #3d3d3d;
}
.choice__opt input {
  position: absolute;
  left: var(--space-3);
  top: calc(var(--space-3) + 5px);
  margin: 0;
  accent-color: #fafafa;
}
.choice__opt--on,
.choice__opt--on:hover {
  border-color: #6a6a6a;
  background: var(--surface-2);
}
.choice__opt:has(input:focus-visible) {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}
.choice__opt:has(input:disabled) {
  cursor: default;
  opacity: 0.7;
}
.choice__title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.choice__text {
  font-size: var(--fs-sm);
  color: var(--ink-2);
}

.link {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.waiting {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-variant-numeric: tabular-nums;
}
.waiting__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ok);
  animation: skeleton 1.2s ease-in-out infinite;
}

.manual {
  border: 1px solid var(--rule-strong);
  border-radius: var(--r-control);
  background: #0a0a0a;
}
.manual summary {
  padding: var(--space-3) var(--space-4);
  cursor: pointer;
  font-weight: 500;
}
.manual__body {
  display: grid;
  gap: var(--space-3);
  padding: 0 var(--space-4) var(--space-4);
}

.aside {
  max-width: 900px;
  padding: 0 var(--space-6);
  color: var(--ink-2);
}

@media (max-width: 720px) {
  .steps {
    padding: var(--space-5) var(--space-4) 0;
  }
  .aside {
    padding: 0;
  }
}
@media (max-width: 560px) {
  .step {
    padding-left: 40px;
  }
  .choice {
    grid-template-columns: 1fr;
  }
}
</style>
