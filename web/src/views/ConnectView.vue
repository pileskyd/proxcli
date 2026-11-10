<script setup lang="ts">
import { computed, ref } from 'vue'
import CodeBlock from '../components/CodeBlock.vue'
import ProviderGlyph from '../components/ProviderGlyph.vue'
import { useHub } from '../composables/useHub'

const hub = useHub()

// Ключ живёт только в памяти этой вкладки: никакого localStorage, никаких запросов.
const hubUrl = ref('')
const accessKey = ref('')
const showKeyInput = ref(false)
const revealInConfigs = ref(false)
const separateDirs = ref(false)
const websockets = ref(true)

const HUB_PLACEHOLDER = '<адрес хаба>'
const KEY_PLACEHOLDER = '<ключ доступа>'

const upstream = computed(() => hub.state.status.data?.upstream ?? '')

const urlError = computed(() => {
  const v = hubUrl.value.trim()
  if (!v) return ''
  if (!/^https?:\/\/[^\s/]+/i.test(v)) return 'Адрес должен начинаться с https:// или http://'
  return ''
})

const base = computed(() => {
  const v = hubUrl.value.trim().replace(/\/+$/, '')
  return v && !urlError.value ? v : HUB_PLACEHOLDER
})

const isLocalhost = computed(() => /^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])/i.test(hubUrl.value.trim()))

function keyFor(reveal: boolean): string {
  const k = accessKey.value.trim()
  if (!k) return KEY_PLACEHOLDER
  return reveal ? k : '•'.repeat(Math.min(Math.max(k.length, 8), 24))
}

const tomlString = (v: string) => JSON.stringify(v)
const shellQuote = (v: string) => `'${v.replace(/'/g, `'\\''`)}'`

function claudeJson(reveal: boolean): string {
  const settings: Record<string, unknown> = {
    env: {
      ANTHROPIC_BASE_URL: base.value,
      ANTHROPIC_AUTH_TOKEN: keyFor(reveal),
    },
  }
  return JSON.stringify(settings, null, 2)
}

function codexToml(reveal: boolean): string {
  const v1 = base.value === HUB_PLACEHOLDER ? `${HUB_PLACEHOLDER}/v1` : `${base.value}/v1`
  const lines = [
    'model_provider = "cliproxy"',
    '',
    '[model_providers.cliproxy]',
    'name = "CLIProxyAPI"',
    `base_url = ${tomlString(v1)}`,
    'wire_api = "responses"',
    'requires_openai_auth = false',
  ]
  if (websockets.value) lines.push('supports_websockets = true')
  lines.push(`experimental_bearer_token = ${tomlString(keyFor(reveal))}`)
  return lines.join('\n')
}

function checkCmd(reveal: boolean): string {
  return `curl -s ${shellQuote(`${base.value}/v1/models`)} \\\n  -H ${shellQuote(`Authorization: Bearer ${keyFor(reveal)}`)}`
}

const claudePath = computed(() => (separateDirs.value ? '~/.claude-hub/settings.json' : '~/.claude/settings.json'))
const codexPath = computed(() => (separateDirs.value ? '~/.codex-hub/config.toml' : '~/.codex/config.toml'))

const launch = `CLAUDE_CONFIG_DIR=~/.claude-hub claude\nCODEX_HOME=~/.codex-hub codex`
const tailscale = 'tailscale serve --bg --https=8317 http://127.0.0.1:8317'

const keyHidden = computed(() => accessKey.value.trim() !== '' && !revealInConfigs.value)
</script>

<template>
  <div class="page">
    <header class="page__head">
      <div>
        <h1 tabindex="-1">Подключение клиентов</h1>
        <p class="page__lede">
          Готовые настройки для Claude Code и Codex на рабочих машинах. Подставьте адрес хаба и ключ доступа, затем скопируйте файлы.
        </p>
      </div>
    </header>

    <section class="panel inputs" aria-labelledby="inputs-title">
      <h2 id="inputs-title" class="visually-hidden">Параметры</h2>

      <div class="field">
        <label class="field__label" for="hub-url">Адрес хаба для клиентов</label>
        <input
          id="hub-url"
          v-model="hubUrl"
          class="input input--mono"
          type="url"
          inputmode="url"
          autocomplete="off"
          spellcheck="false"
          placeholder="https://box.tailnet.ts.net:8317"
          :aria-invalid="urlError ? 'true' : undefined"
          aria-describedby="hub-url-hint"
        />
        <span v-if="urlError" class="field__error">{{ urlError }}</span>
        <span id="hub-url-hint" class="field__hint">
          Обычно это адрес Tailscale. Адрес <code>127.0.0.1</code> работает только на самом хабе.
          <button v-if="upstream && !hubUrl" type="button" class="linkish" @click="hubUrl = upstream">
            Подставить {{ upstream }}
          </button>
        </span>
        <span v-if="isLocalhost" class="field__hint warn">С других машин этот адрес недоступен.</span>
      </div>

      <div class="field">
        <label class="field__label" for="access-key">Ключ доступа</label>
        <div class="pass">
          <input
            id="access-key"
            v-model="accessKey"
            class="input input--mono"
            :type="showKeyInput ? 'text' : 'password'"
            autocomplete="off"
            spellcheck="false"
            placeholder="из access.api-keys в config.yaml хаба"
            aria-describedby="access-key-hint"
          />
          <button
            type="button"
            class="btn btn--quiet btn--small pass__toggle"
            :aria-pressed="showKeyInput"
            aria-controls="access-key"
            @click="showKeyInput = !showKeyInput"
          >
            {{ showKeyInput ? 'Скрыть' : 'Показать' }}
          </button>
        </div>
        <span id="access-key-hint" class="field__hint">
          Ключ остаётся в этой вкладке: панель его не сохраняет и никуда не отправляет. Без ключа в конфиге будет заглушка.
        </span>
      </div>

      <fieldset class="opts">
        <legend class="field__label">Параметры</legend>
        <label class="check">
          <input v-model="separateDirs" type="checkbox" />
          <span>Отдельные каталоги настроек, чтобы обычные Claude Code и Codex остались как есть</span>
        </label>
        <label class="check">
          <input v-model="websockets" type="checkbox" />
          <span>WebSocket для Codex, работает быстрее</span>
        </label>
        <label class="check">
          <input v-model="revealInConfigs" type="checkbox" :disabled="!accessKey.trim()" />
          <span>Показывать ключ в блоках ниже</span>
        </label>
      </fieldset>
    </section>

    <p v-if="keyHidden" class="notice small">На экране ключ скрыт, но кнопка «Копировать» кладёт в буфер настоящий ключ.</p>

    <section class="section" aria-labelledby="claude-title">
      <div class="section__head">
        <h2 id="claude-title" class="glyph-title"><ProviderGlyph provider="claude" />Claude Code</h2>
        <p class="muted">Сделайте копию текущего файла, если он есть, и замените или дополните его этим содержимым.</p>
      </div>
      <CodeBlock :title="claudePath" :code="claudeJson(revealInConfigs)" :copy-text="claudeJson(true)" />
    </section>

    <section class="section" aria-labelledby="codex-title">
      <div class="section__head">
        <h2 id="codex-title" class="glyph-title"><ProviderGlyph provider="codex" />Codex</h2>
        <p class="muted">
          В T3 Code для Codex выберите «Use existing CLI»: вариант «Continue with ChatGPT» идёт мимо хаба.
        </p>
      </div>
      <CodeBlock :title="codexPath" :code="codexToml(revealInConfigs)" :copy-text="codexToml(true)" />
    </section>

    <section v-if="separateDirs" class="section" aria-labelledby="launch-title">
      <h2 id="launch-title">Запуск с отдельными каталогами</h2>
      <CodeBlock title="Терминал" :code="launch" />
    </section>

    <section class="section" aria-labelledby="check-title">
      <div class="section__head">
        <h2 id="check-title">Проверка с рабочей машины</h2>
        <p class="muted">Хаб должен вернуть JSON со списком моделей. Ответ 401 значит, что ключ не совпал.</p>
      </div>
      <CodeBlock title="Терминал" :code="checkCmd(revealInConfigs)" :copy-text="checkCmd(true)" />
    </section>

    <section class="section" aria-labelledby="ts-title">
      <div class="section__head">
        <h2 id="ts-title">Открыть хаб для своих устройств</h2>
        <p class="muted">CLIProxyAPI слушает только 127.0.0.1. Tailscale публикует его внутри вашей сети по HTTPS, наружу он не виден.</p>
      </div>
      <CodeBlock title="Терминал на хабе" :code="tailscale" />
    </section>
  </div>
</template>

<style scoped>
.glyph-title {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.inputs {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4) var(--space-5);
  padding: var(--space-5);
}
.opts {
  grid-column: 1 / -1;
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
  min-width: 0;
}
.opts legend {
  padding: 0;
  margin-bottom: var(--space-2);
}
.pass {
  position: relative;
}
.pass .input {
  padding-right: 92px;
}
.pass__toggle {
  position: absolute;
  top: 4px;
  right: 4px;
}
.warn {
  color: var(--route-direct);
}

@media (max-width: 720px) {
  .inputs {
    grid-template-columns: 1fr;
    padding: var(--space-4);
  }
}
</style>
