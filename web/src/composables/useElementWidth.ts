import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

/** Ширина элемента в CSS-пикселях: графики рисуются 1:1, без растяжения текста. */
export function useElementWidth(el: Ref<HTMLElement | null>, fallback = 800) {
  const width = ref(fallback)
  let ro: ResizeObserver | null = null
  onMounted(() => {
    if (!el.value) return
    width.value = Math.round(el.value.clientWidth) || fallback
    ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w && Math.abs(w - width.value) >= 1) width.value = Math.round(w)
    })
    ro.observe(el.value)
  })
  onBeforeUnmount(() => ro?.disconnect())
  return width
}
