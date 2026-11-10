<script setup lang="ts">
defineProps<{
  checked: boolean
  label: string
  busy?: boolean
}>()
const emit = defineEmits<{ change: [value: boolean] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    class="switch"
    :class="{ 'switch--on': checked, 'switch--busy': busy }"
    :aria-checked="checked"
    :aria-label="label"
    :aria-busy="busy || undefined"
    :disabled="busy"
    @click="emit('change', !checked)"
  >
    <span class="switch__knob" aria-hidden="true" />
  </button>
</template>

<style scoped>
.switch {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: var(--surface-3);
  cursor: pointer;
  transition: background-color 140ms ease, border-color 140ms ease;
}
.switch__knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #8a8a8a;
  transition: transform 160ms ease, background-color 140ms ease;
}
.switch--on {
  background: #fafafa;
}
.switch--on .switch__knob {
  transform: translateX(16px);
  background: #0a0a0a;
}
.switch--busy {
  cursor: progress;
  opacity: 0.6;
}
.switch:focus-visible {
  border-radius: 999px;
}
</style>
