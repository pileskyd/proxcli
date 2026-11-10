import { ref } from 'vue'

export interface Toast {
  id: number
  text: string
  tone: 'ok' | 'error'
}

const toasts = ref<Toast[]>([])
let seq = 0

export function useToasts() {
  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }
  function push(text: string, tone: Toast['tone'] = 'ok') {
    const id = ++seq
    toasts.value = [...toasts.value.slice(-2), { id, text, tone }]
    window.setTimeout(() => dismiss(id), tone === 'error' ? 8000 : 4000)
  }
  return { toasts, push, dismiss }
}
