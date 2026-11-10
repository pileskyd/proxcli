<script setup lang="ts" generic="T extends string">
defineProps<{
  label: string
  options: readonly { value: T; label: string }[]
  /** Показать подпись внутри сегмента, как у вторичных фильтров. */
  showLabel?: boolean
}>()

const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="seg" role="group" :aria-label="label">
    <span v-if="showLabel" class="seg__label" aria-hidden="true">{{ label }}</span>
    <button
      v-for="o in options"
      :key="o.value"
      type="button"
      class="seg__btn"
      :aria-pressed="model === o.value"
      @click="model = o.value"
    >
      {{ o.label }}
    </button>
  </div>
</template>
