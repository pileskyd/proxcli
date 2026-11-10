<script setup lang="ts">
/** Часовые корзины расхода: один счётчик за раз, без сложения разных счётчиков. */
import { computed, ref } from 'vue'
import type { UsageBucket } from '../api/types'
import { useElementWidth } from '../composables/useElementWidth'
import { HOUR, bucketValue, formatCount, formatExact, formatMoment, niceScale, parseTime, timeTicks, type UsageMetric } from '../lib/quota'

const props = defineProps<{
  buckets: readonly UsageBucket[]
  metric: UsageMetric
  from: number
  to: number
  now: number
  color: string
  label: string
}>()

const box = ref<HTMLElement | null>(null)
const width = useElementWidth(box)
const height = computed(() => (width.value < 560 ? 170 : 200))
const M = { top: 14, right: 14, bottom: 28, left: 56 }

const plotW = computed(() => Math.max(10, width.value - M.left - M.right))
const plotH = computed(() => height.value - M.top - M.bottom)
const x = (t: number) => M.left + ((t - props.from) / (props.to - props.from)) * plotW.value

const bars = computed(() =>
  props.buckets
    .map((b) => ({ b, t: parseTime(b.at), v: bucketValue(b, props.metric) }))
    .filter((r): r is { b: UsageBucket; t: number; v: number } => r.t !== null && r.t + HOUR > props.from && r.t <= props.to),
)

const scale = computed(() => niceScale(Math.max(0, ...bars.value.map((r) => r.v))))
const y = (v: number) => M.top + (1 - v / scale.value.max) * plotH.value
const yTicks = computed(() => {
  const out: number[] = []
  for (let v = 0; v <= scale.value.max + 1e-9; v += scale.value.step) out.push(v)
  return out
})
const ticks = computed(() => timeTicks(props.from, props.to, Math.max(2, Math.floor(plotW.value / 90))))

const barW = computed(() => {
  const w = (HOUR / (props.to - props.from)) * plotW.value
  return w > 4 ? w - Math.min(3, w * 0.22) : Math.max(1, w)
})

const rects = computed(() =>
  bars.value.map((r) => {
    const left = Math.max(M.left, x(r.t))
    const top = y(r.v)
    const failed = props.metric === 'requests' && r.b.failed > 0 ? Math.min(r.b.failed, r.v) : 0
    return { key: r.t, t: r.t, b: r.b, x: left, w: barW.value, y: top, h: y(0) - top, failTop: failed ? y(failed) : 0, failH: failed ? y(0) - y(failed) : 0 }
  }),
)

// ── Наведение ─────────────────────────────────────────
const hoverKey = ref<number | null>(null)

function onMove(e: PointerEvent) {
  const rect = (e.currentTarget as SVGElement).getBoundingClientRect()
  const px = e.clientX - rect.left
  if (px < M.left || px > M.left + plotW.value) {
    hoverKey.value = null
    return
  }
  const t = props.from + ((px - M.left) / plotW.value) * (props.to - props.from)
  const hit = bars.value.find((r) => t >= r.t && t < r.t + HOUR)
  hoverKey.value = hit ? hit.t : null
}

const hover = computed(() => {
  const r = rects.value.find((x) => x.key === hoverKey.value)
  if (!r) return null
  const px = r.x + r.w / 2
  return { r, px, flip: px > M.left + plotW.value * 0.62 }
})

const ROWS: { k: UsageMetric; label: string }[] = [
  { k: 'total', label: 'Всего токенов' },
  { k: 'input', label: 'Ввод без кеша' },
  { k: 'output', label: 'Вывод' },
  { k: 'reasoning', label: 'Рассуждение' },
  { k: 'cacheRead', label: 'Чтение кеша' },
  { k: 'cacheWrite', label: 'Запись кеша' },
]
</script>

<template>
  <div ref="box" class="uchart">
    <svg
      :width="width"
      :height="height"
      :viewBox="`0 0 ${width} ${height}`"
      role="img"
      :aria-label="label"
      @pointermove="onMove"
      @pointerleave="hoverKey = null"
    >
      <g class="uchart__grid">
        <template v-for="v in yTicks" :key="v">
          <line :x1="M.left" :x2="M.left + plotW" :y1="y(v)" :y2="y(v)" :class="{ 'uchart__base': v === 0 }" />
          <text :x="M.left - 10" :y="y(v) + 4" text-anchor="end">{{ formatCount(v) }}</text>
        </template>
        <text v-for="tk in ticks" :key="tk.t" :x="x(tk.t)" :y="height - 8" text-anchor="middle">{{ tk.label }}</text>
      </g>
      <g :style="{ color }">
        <template v-for="r in rects" :key="r.key">
          <rect
            :x="r.x"
            :y="r.y"
            :width="r.w"
            :height="Math.max(0, r.h)"
            rx="1.5"
            fill="currentColor"
            :opacity="hoverKey === null || hoverKey === r.key ? 0.9 : 0.4"
          />
          <rect v-if="r.failH" :x="r.x" :y="r.failTop" :width="r.w" :height="r.failH" rx="1.5" fill="var(--err)" />
        </template>
      </g>
    </svg>

    <p v-if="!rects.length" class="uchart__empty">За период нет записанных запросов.</p>

    <div
      v-if="hover"
      class="uchart__tip"
      :class="{ 'uchart__tip--flip': hover.flip }"
      :style="{ left: `${hover.px}px` }"
      aria-hidden="true"
    >
      <span class="uchart__tip-time">{{ formatMoment(hover.r.t, now) }}, час</span>
      <span class="uchart__tip-row">
        <span>Запросы</span>
        <span>{{ formatExact(hover.r.b.requests) }}<template v-if="hover.r.b.failed">, ошибок {{ formatExact(hover.r.b.failed) }}</template></span>
      </span>
      <span v-for="row in ROWS" :key="row.k" class="uchart__tip-row" :class="{ 'uchart__tip-row--on': row.k === metric }">
        <span>{{ row.label }}</span>
        <span>{{ formatExact(bucketValue(hover.r.b, row.k)) }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.uchart {
  position: relative;
  min-width: 0;
}
svg {
  display: block;
  max-width: 100%;
  height: auto;
  overflow: visible;
  touch-action: pan-y;
}
.uchart__grid line {
  stroke: #1c1c1c;
}
.uchart__grid .uchart__base {
  stroke: #2c2c2c;
}
.uchart__grid text {
  fill: var(--ink-3);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.uchart__empty {
  position: absolute;
  inset: 0 0 28px 56px;
  display: grid;
  place-items: center;
  max-width: none;
  color: var(--ink-3);
  font-size: var(--fs-sm);
  pointer-events: none;
}
.uchart__tip {
  position: absolute;
  top: 0;
  z-index: 2;
  display: grid;
  gap: 2px;
  min-width: 210px;
  margin-left: 14px;
  padding: 8px 11px;
  border: 1px solid var(--rule-strong);
  border-radius: 8px;
  background: #121212;
  font-size: var(--fs-sm);
  pointer-events: none;
  box-shadow: 0 10px 28px -12px #000;
}
.uchart__tip--flip {
  transform: translateX(calc(-100% - 28px));
}
.uchart__tip-time {
  color: var(--ink-2);
  margin-bottom: 2px;
}
.uchart__tip-row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  color: var(--ink-2);
  font-variant-numeric: tabular-nums;
}
.uchart__tip-row > span:last-child {
  color: var(--ink);
}
.uchart__tip-row--on {
  color: var(--ink);
  font-weight: 500;
}
</style>
