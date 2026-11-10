<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { api, errorMessage } from '../api/client'
import type { Subscription, SubscriptionPatch } from '../api/types'
import { useHub } from '../composables/useHub'
import { useToasts } from '../composables/useToasts'
import { createDraft, effectiveExit, resolveDraft, type DraftErrors, type ProxyDraft } from '../lib/proxy'
import { providerKey, providerLabel, subscriptionStatus } from '../lib/providers'
import ProviderGlyph from './ProviderGlyph.vue'
import ProxyFields from './ProxyFields.vue'
import RouteLine from './RouteLine.vue'
import ToggleSwitch from './ToggleSwitch.vue'

const props = defineProps<{
  sub: Subscription
  defaultProxy: string
}>()

const hub = useHub()
const { push } = useToasts()

const exit = computed(() => effectiveExit(props.sub, props.defaultProxy))
const status = computed(() => subscriptionStatus(props.sub.status, props.sub.disabled))
const title = computed(() => props.sub.name || props.sub.id)
const domId = computed(() => `sub-${props.sub.id.replace(/[^a-zA-Z0-9_-]/g, '_')}`)

// ── Включение / отключение ───────────────────────────
const toggling = ref(false)
const toggleError = ref('')

async function toggle(enabled: boolean) {
  toggling.value = true
  toggleError.value = ''
  try {
    await api.setDisabled(props.sub.name, !enabled)
    hub.patchLocal(props.sub.name, { disabled: !enabled })
    push(enabled ? `Подписка ${title.value} включена` : `Подписка ${title.value} отключена`)
  } catch (err) {
    toggleError.value = `Не удалось ${enabled ? 'включить' : 'отключить'}: ${errorMessage(err)}`
  } finally {
    toggling.value = false
  }
}

// ── Редактирование ──────────────────────────────────
const editing = ref(false)
const draft = ref<ProxyDraft>(createDraft('inherit', ''))
const prefix = ref('')
const errors = ref<DraftErrors>({})
const saveError = ref('')
const saving = ref(false)
const editor = ref<HTMLElement | null>(null)
const editBtn = ref<HTMLButtonElement | null>(null)

function openEditor() {
  draft.value = createDraft(props.sub.proxyKnown === false ? 'custom' : props.sub.proxyMode, props.sub.proxyKnown === false ? '' : props.sub.proxyUrl)
  prefix.value = props.sub.prefix
  errors.value = {}
  saveError.value = ''
  editing.value = true
  nextTick(() => editor.value?.querySelector<HTMLInputElement>('input:checked')?.focus())
}

function closeEditor() {
  editing.value = false
  nextTick(() => editBtn.value?.focus())
}

async function save() {
  saveError.value = ''
  const res = resolveDraft(draft.value)
  if (!res.ok) {
    errors.value = res.errors
    nextTick(() => editor.value?.querySelector<HTMLElement>('[aria-invalid="true"], [role="alert"]')?.focus())
    return
  }
  errors.value = {}
  const newPrefix = prefix.value.trim().replace(/^\/+|\/+$/g, '')
  const prefixChanged = newPrefix !== props.sub.prefix

  if (!res.changed && !prefixChanged) {
    push('Изменений нет, сохранять нечего')
    closeEditor()
    return
  }

  const patch: SubscriptionPatch = { name: props.sub.name, proxyMode: res.proxyMode }
  if (res.proxyUrl !== undefined) patch.proxyUrl = res.proxyUrl
  if (prefixChanged) patch.prefix = newPrefix

  saving.value = true
  try {
    await api.updateSubscription(patch)
    hub.patchLocal(props.sub.name, { proxyMode: res.proxyMode, prefix: newPrefix })
    push(`Маршрут подписки ${title.value} сохранён`)
    closeEditor()
    // Сервер вернёт адрес с замаскированным паролем.
    void hub.loadSubscriptions()
  } catch (err) {
    saveError.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <li class="row" :class="[`row--${exit.kind}`, { 'row--off': sub.disabled, 'row--open': editing }]">
    <div class="row__main">
      <ToggleSwitch
        class="row__switch"
        :checked="!sub.disabled"
        :busy="toggling"
        :label="`Подписка ${title} ${sub.disabled ? 'отключена' : 'включена'}`"
        @change="toggle"
      />

      <div class="row__who">
        <span class="row__name">{{ title }}</span>
        <span class="row__email">{{ sub.email || 'почта не указана' }}</span>
      </div>

      <div class="row__meta">
        <span class="row__provider">
          <ProviderGlyph :provider="sub.provider" :size="15" />
          {{ providerLabel(providerKey(sub.provider)) }}
        </span>
        <span class="row__status" :class="`tone--${status.tone}`"><span class="dot" aria-hidden="true" />{{ status.label }}</span>
        <span v-if="sub.prefix" class="row__prefix" :title="`Префикс моделей: ${sub.prefix}`">
          <span class="visually-hidden">Префикс моделей: </span><span class="mono">{{ sub.prefix }}/</span>
        </span>
      </div>

      <RouteLine class="row__route" :kind="exit.kind" :label="exit.label" />

      <button
        ref="editBtn"
        type="button"
        class="btn btn--small row__edit"
        :aria-expanded="editing"
        :aria-controls="`${domId}-editor`"
        @click="editing ? closeEditor() : openEditor()"
      >
        {{ editing ? 'Закрыть' : 'Настроить' }}
      </button>
    </div>

    <p v-if="toggleError" class="notice notice--error row__notice" role="alert">{{ toggleError }}</p>

    <form
      v-if="editing"
      :id="`${domId}-editor`"
      ref="editor"
      class="editor"
      novalidate
      :aria-label="`Маршрут подписки ${title}`"
      @submit.prevent="save"
      @keydown.esc="closeEditor"
    >
      <ProxyFields v-model="draft" :id-base="domId" :default-proxy="defaultProxy" :errors="errors" />

      <div class="field editor__prefix">
        <label class="field__label" :for="`${domId}-prefix`">Префикс моделей <span class="muted">(необязательно)</span></label>
        <input
          :id="`${domId}-prefix`"
          v-model="prefix"
          class="input input--mono"
          type="text"
          autocomplete="off"
          spellcheck="false"
          placeholder="work"
          :aria-describedby="`${domId}-prefix-hint`"
        />
        <span :id="`${domId}-prefix-hint`" class="field__hint">
          Запрос к модели <code>{{ (prefix.trim() || 'префикс') }}/имя-модели</code> пойдёт только через эту подписку.
        </span>
      </div>

      <div v-if="saveError" class="notice notice--error" role="alert" tabindex="-1">
        <span class="notice__title">Маршрут не сохранён</span>
        <span>{{ saveError }}</span>
      </div>

      <div class="editor__actions">
        <button type="submit" class="btn btn--primary" :disabled="saving">
          {{ saving ? 'Сохраняю…' : 'Сохранить маршрут' }}
        </button>
        <button type="button" class="btn btn--quiet" :disabled="saving" @click="closeEditor">Отмена</button>
      </div>
    </form>
  </li>
</template>

<style scoped>
.row {
  border-top: 1px solid var(--rule);
}
.row:first-child {
  border-top: 0;
}
.row--open {
  background: #121212;
}
.row:first-child.row--open {
  border-radius: var(--r-panel) var(--r-panel) 0 0;
}
.row:last-child.row--open {
  border-radius: 0 0 var(--r-panel) var(--r-panel);
}
.row:only-child.row--open {
  border-radius: var(--r-panel);
}

.row__main {
  display: grid;
  grid-template-columns: 36px minmax(0, 1.25fr) minmax(0, 0.9fr) minmax(0, 1.5fr) auto;
  align-items: center;
  gap: var(--space-4);
  padding: 14px var(--space-5);
}

.row--off .row__who,
.row--off .row__meta,
.row--off .row__route {
  opacity: 0.55;
}

.row__who {
  display: grid;
  min-width: 0;
}
.row__name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row__email {
  font-size: var(--fs-sm);
  color: var(--ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  column-gap: var(--space-4);
  row-gap: 2px;
  font-size: var(--fs-sm);
  min-width: 0;
}
.row__provider {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}
.row__status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}
.row__prefix {
  color: var(--ink-2);
}

.row__notice {
  margin: 0 var(--space-5) var(--space-3);
}

.editor {
  display: grid;
  gap: var(--space-5);
  padding: var(--space-2) var(--space-5) var(--space-5);
  margin-left: calc(36px + var(--space-4));
}
.editor__prefix {
  max-width: 420px;
}
.editor__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

@media (max-width: 1080px) {
  .row__main {
    grid-template-columns: 36px minmax(0, 1fr) auto;
    grid-template-areas:
      'switch who edit'
      '. meta meta'
      '. route route';
    row-gap: var(--space-2);
  }
  .row__switch {
    grid-area: switch;
  }
  .row__who {
    grid-area: who;
  }
  .row__meta {
    grid-area: meta;
  }
  .row__route {
    grid-area: route;
  }
  .row__edit {
    grid-area: edit;
  }
  .editor {
    margin-left: 0;
  }
  .row__route :deep(.route__exit) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
}

@media (max-width: 520px) {
  .row__main {
    padding: 12px var(--space-4);
    column-gap: var(--space-3);
  }
  .editor {
    padding: var(--space-2) var(--space-4) var(--space-4);
  }
  .row__notice {
    margin: 0 var(--space-4) var(--space-3);
  }
}
</style>
