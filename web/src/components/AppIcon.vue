<script setup lang="ts">
/** Линейные иконки 24×24, нарисованные вручную под тон интерфейса. */
export type IconName =
  | 'overview'
  | 'subscriptions'
  | 'add'
  | 'connect'
  | 'help'
  | 'refresh'
  | 'menu'
  | 'close'
  | 'external'
  | 'chevron'
  | 'all'
  | 'quota'

defineProps<{ name: IconName }>()
</script>

<template>
  <svg
    class="icon"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <template v-if="name === 'overview'">
      <rect x="3.5" y="3.5" width="7" height="7" rx="1.6" />
      <rect x="13.5" y="3.5" width="7" height="7" rx="1.6" />
      <rect x="3.5" y="13.5" width="7" height="7" rx="1.6" />
      <rect x="13.5" y="13.5" width="7" height="7" rx="1.6" />
    </template>
    <template v-else-if="name === 'subscriptions'">
      <circle cx="8" cy="8" r="3" />
      <path d="M3 19.5c0-2.8 2.2-5 5-5s5 2.2 5 5" />
      <path d="M15 5.2a3 3 0 0 1 0 5.6M17.5 14.8c2 .7 3.5 2.6 3.5 4.7" />
    </template>
    <template v-else-if="name === 'add'">
      <circle cx="10" cy="8" r="3.5" />
      <path d="M3.5 20c0-3.3 2.9-6 6.5-6 1.2 0 2.3.3 3.2.8" />
      <path d="M18 14v6M15 17h6" />
    </template>
    <template v-else-if="name === 'connect'">
      <path d="M9 3.5v4M15 3.5v4" />
      <path d="M6.5 7.5h11v3a5.5 5.5 0 0 1-11 0z" />
      <path d="M12 16v4.5" />
    </template>
    <template v-else-if="name === 'help'">
      <circle cx="12" cy="12" r="8.5" />
      <path d="M9.6 9.4a2.5 2.5 0 0 1 4.8.9c0 1.7-2.4 2.1-2.4 3.6" />
      <path d="M12 17h.01" stroke-width="2.2" />
    </template>
    <template v-else-if="name === 'refresh'">
      <path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3" />
      <path d="M19.5 4.5v4h-4" />
    </template>
    <template v-else-if="name === 'menu'">
      <path d="M4 7h16M4 12h16M4 17h16" />
    </template>
    <template v-else-if="name === 'close'">
      <path d="M6 6l12 12M18 6L6 18" />
    </template>
    <template v-else-if="name === 'external'">
      <path d="M14 4.5h5.5V10" />
      <path d="M19.5 4.5L11 13" />
      <path d="M17.5 14v4a1.5 1.5 0 0 1-1.5 1.5H6A1.5 1.5 0 0 1 4.5 18V8A1.5 1.5 0 0 1 6 6.5h4" />
    </template>
    <template v-else-if="name === 'chevron'">
      <path d="M9 6l6 6-6 6" />
    </template>
    <template v-else-if="name === 'quota'">
      <path d="M4.2 16.5a8.5 8.5 0 1 1 15.6 0" />
      <path d="M12 13.5l3.6-4.1" />
      <circle cx="12" cy="14" r="1.2" />
      <path d="M7 19.5h10" />
    </template>
    <template v-else-if="name === 'all'">
      <path d="M9.5 9.5V7a2.5 2.5 0 1 0-2.5 2.5h10A2.5 2.5 0 1 0 14.5 7v10a2.5 2.5 0 1 0 2.5-2.5H7a2.5 2.5 0 1 0 2.5 2.5z" />
    </template>
  </svg>
</template>
