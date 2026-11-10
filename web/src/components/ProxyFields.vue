<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProxyMode } from '../api/types'
import {
  PROXY_SCHEMES,
  applyPastedUrl,
  displayProxyUrl,
  type DraftErrors,
  type ProxyDraft,
} from '../lib/proxy'

const draft = defineModel<ProxyDraft>({ required: true })

const props = defineProps<{
  idBase: string
  defaultProxy: string
  errors?: DraftErrors
  /** Подпись группы режимов. */
  legend?: string
}>()

const showPassword = ref(false)
const pasteNote = ref('')

const options = computed<{ mode: ProxyMode; title: string; text: string }[]>(() => [
  {
    mode: 'custom',
    title: 'Свой прокси',
    text: 'Подписка ходит к провайдеру только через указанный адрес.',
  },
  {
    mode: 'inherit',
    title: 'Общий прокси',
    text: props.defaultProxy
      ? `Прокси хаба по умолчанию: ${displayProxyUrl(props.defaultProxy)}`
      : 'Общий прокси на хабе не задан, поэтому запросы пойдут с IP хаба.',
  },
  {
    mode: 'direct',
    title: 'Без прокси',
    text: 'Запросы идут напрямую с IP хаба.',
  },
])

const preview = computed(() => {
  const d = draft.value
  if (!d.host.trim()) return ''
  const user = d.username ? `${d.username}${hasPassword.value ? ':•••' : ''}@` : hasPassword.value ? '•••@' : ''
  return `${d.scheme}://${user}${d.host.trim()}${d.port.trim() ? `:${d.port.trim()}` : ''}`
})

const hasPassword = computed(() => {
  const d = draft.value
  return d.storedPassword && !d.replacePassword ? true : d.password.length > 0
})

function onPaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text') ?? ''
  if (!/^\s*[a-z0-9]+:\/\//i.test(text)) return
  event.preventDefault()
  if (applyPastedUrl(draft.value, text)) {
    pasteNote.value = 'Адрес разобран: протокол, хост, порт, логин и пароль заполнены.'
  } else {
    pasteNote.value = 'Не удалось разобрать адрес. Поддерживаются socks5h://, socks5://, http:// и https://.'
  }
}

function startReplace() {
  draft.value.replacePassword = true
  draft.value.password = ''
}

function keepStored() {
  draft.value.replacePassword = false
  draft.value.password = ''
  showPassword.value = false
}

const id = (s: string) => `${props.idBase}-${s}`
</script>

<template>
  <div class="proxy">
    <fieldset class="modes">
      <legend class="field__label">{{ legend ?? 'Выход в интернет' }}</legend>
      <label
        v-for="opt in options"
        :key="opt.mode"
        class="mode"
        :class="[`mode--${opt.mode}`, { 'mode--on': draft.mode === opt.mode }]"
      >
        <input v-model="draft.mode" type="radio" class="mode__radio" :name="id('mode')" :value="opt.mode" />
        <span class="mode__title">{{ opt.title }}</span>
        <span class="mode__text">{{ opt.text }}</span>
      </label>
    </fieldset>

    <div v-if="draft.mode === 'custom'" class="custom">
      <p v-if="errors?.url" class="notice notice--error" role="alert">{{ errors.url }}</p>

      <div class="grid grid--addr">
        <div class="field">
          <label class="field__label" :for="id('scheme')">Протокол</label>
          <select :id="id('scheme')" v-model="draft.scheme" class="select">
            <option v-for="s in PROXY_SCHEMES" :key="s" :value="s">{{ s }}</option>
          </select>
        </div>
        <div class="field">
          <label class="field__label" :for="id('host')">Хост</label>
          <input
            :id="id('host')"
            v-model.trim="draft.host"
            class="input input--mono"
            type="text"
            inputmode="url"
            autocomplete="off"
            spellcheck="false"
            placeholder="proxy.example.net"
            :aria-invalid="errors?.host ? 'true' : undefined"
            :aria-describedby="`${id('host-hint')}${errors?.host ? ` ${id('host-err')}` : ''}`"
            @paste="onPaste"
          />
          <span :id="id('host-err')" v-if="errors?.host" class="field__error">{{ errors.host }}</span>
        </div>
        <div class="field">
          <label class="field__label" :for="id('port')">Порт</label>
          <input
            :id="id('port')"
            v-model.trim="draft.port"
            class="input input--mono"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            placeholder="1080"
            :aria-invalid="errors?.port ? 'true' : undefined"
            :aria-describedby="errors?.port ? id('port-err') : undefined"
          />
          <span :id="id('port-err')" v-if="errors?.port" class="field__error">{{ errors.port }}</span>
        </div>
      </div>
      <p :id="id('host-hint')" class="field__hint">
        Можно вставить в поле «Хост» готовый адрес вида <code>socks5h://логин:пароль@хост:порт</code>, он разложится по полям.
      </p>
      <p v-if="pasteNote" class="field__hint" role="status">{{ pasteNote }}</p>

      <div class="grid grid--auth">
        <div class="field">
          <label class="field__label" :for="id('user')">Логин <span class="muted">(если нужен)</span></label>
          <input
            :id="id('user')"
            v-model="draft.username"
            class="input input--mono"
            type="text"
            autocomplete="off"
            spellcheck="false"
          />
        </div>

        <div class="field">
          <template v-if="draft.storedPassword && !draft.replacePassword">
            <span class="field__label">Пароль</span>
            <div class="stored">
              <span>Сохранён на хабе</span>
              <button type="button" class="btn btn--small" @click="startReplace">Заменить пароль</button>
            </div>
          </template>
          <template v-else>
            <label class="field__label" :for="id('pass')">Пароль <span v-if="!draft.storedPassword" class="muted">(если нужен)</span></label>
            <div class="pass">
              <input
                :id="id('pass')"
                v-model="draft.password"
                class="input input--mono"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="new-password"
                spellcheck="false"
                :aria-invalid="errors?.password ? 'true' : undefined"
                :aria-describedby="errors?.password ? id('pass-err') : undefined"
              />
              <button
                type="button"
                class="btn btn--quiet btn--small pass__toggle"
                :aria-pressed="showPassword"
                :aria-controls="id('pass')"
                @click="showPassword = !showPassword"
              >
                {{ showPassword ? 'Скрыть' : 'Показать' }}
              </button>
            </div>
            <span v-if="draft.storedPassword" class="field__hint">
              Пустое поле уберёт пароль.
              <button type="button" class="linkish" @click="keepStored">Оставить прежний</button>
            </span>
          </template>
          <span :id="id('pass-err')" v-if="errors?.password" class="field__error">{{ errors.password }}</span>
        </div>
      </div>

      <p v-if="preview" class="preview">
        <span class="muted">Адрес выхода:</span> <span class="mono">{{ preview }}</span>
      </p>
    </div>
  </div>
</template>

<style scoped>
.proxy {
  display: grid;
  gap: var(--space-4);
}

.modes {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
  min-width: 0;
}
.modes legend {
  padding: 0;
  margin-bottom: var(--space-2);
}

.mode {
  --c: var(--route-direct);
  --w: var(--route-direct-wash);
  position: relative;
  display: grid;
  align-content: start;
  gap: 2px;
  padding: var(--space-3) var(--space-3) var(--space-3) calc(var(--space-3) + 22px);
  border: 1px solid var(--rule-strong);
  border-radius: var(--r-control);
  background: #0a0a0a;
  cursor: pointer;
  transition: border-color 120ms ease, background-color 120ms ease;
}
.mode:hover {
  border-color: #3d3d3d;
}
.mode--custom {
  --c: var(--route-custom);
  --w: var(--route-custom-wash);
}
.mode--inherit {
  --c: var(--route-default);
  --w: var(--route-default-wash);
}
.mode--on,
.mode--on:hover {
  border-color: color-mix(in srgb, var(--c) 60%, transparent);
  background: var(--w);
}
.mode__radio {
  position: absolute;
  top: calc(var(--space-3) + 3px);
  left: var(--space-3);
  margin: 0;
  width: 14px;
  height: 14px;
  accent-color: var(--c);
}
.mode:has(.mode__radio:focus-visible) {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}
.mode__title {
  font-weight: 600;
}
.mode__text {
  font-size: var(--fs-sm);
  color: var(--ink-2);
  overflow-wrap: anywhere;
}

.custom {
  display: grid;
  gap: var(--space-3);
  padding-left: var(--space-4);
  border-left: 1px solid color-mix(in srgb, var(--route-custom) 55%, transparent);
}

.grid {
  display: grid;
  gap: var(--space-3);
}
.grid--addr {
  grid-template-columns: 120px minmax(0, 1fr) 110px;
}
.grid--auth {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.stored {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  min-height: 38px;
  padding: 0 4px 0 10px;
  border: 1px dashed var(--rule-strong);
  border-radius: var(--r-control);
  font-size: var(--fs-sm);
  color: var(--ink-2);
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


.preview {
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}

@media (max-width: 720px) {
  .modes {
    grid-template-columns: 1fr;
  }
  .grid--addr {
    grid-template-columns: 1fr 96px;
  }
  .grid--addr > :first-child {
    grid-column: 1 / -1;
  }
  .grid--auth {
    grid-template-columns: 1fr;
  }
  .custom {
    padding-left: var(--space-3);
  }
}
</style>
