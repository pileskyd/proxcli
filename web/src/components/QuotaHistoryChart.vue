<script setup lang="ts">
/**
 * История остатка одного окна квоты: линия на аккаунт, только наблюдённые точки.
 * Паузы между наблюдениями и неизвестный процент рвут линию — ничего не дорисовываем.
 */
import { computed, ref } from 'vue'
import { useElementWidth } from '../composables/useElementWidth'
import { formatMoment, formatPercent, pointAt, resetJumps, segments, timeTicks, type QuotaPoint } from '../lib/quota'

export interface QuotaChartSeries {
  key: string
  label: string
  color: string
  dash: string
  points: QuotaPoint[]
}

const props = defineProps<{
  series: QuotaChartSeries[]
  from: number
  to: number
  now: number
  maxGap: number
  highlight: string | null
  label: string
}>()

const box = ref<HTMLElement | null>(null)
const width = useElementWidth(box)
const height = computed(() => (width.value < 560 ? 210 : 260))
const M = { top: 26, right: 14, bottom: 28, left: 44 }

const plotW = computed(() => Math.max(10, width.value - M.left - M.right))
const plotH = computed(() => height.value - M.top - M.bottom)
const x = (t: number) => M.left + ((t - props.from) / (props.to - props.from)) * plotW.value
const y = (v: number) => M.top + (1 - v / 100) * plotH.value

const ticks = computed(() => timeTicks(props.from, props.to, Math.max(2, Math.floor(plotW.value / 90))))
const Y_TICKS = [0, 25, 50, 75, 100]

/** Подсвеченная серия рисуется последней, остальные приглушаются. */
const ordered = computed(() => {
  const list = [...props.series]
  if (props.highlight) list.sort((a, b) => Number(a.key === props.highlight) - Number(b.key === props.highlight))
  return list
})

const focusKey = computed(() => props.highlight ?? (props.series.length === 1 ? props.series[0]!.key : null))

const drawn = computed(() =>
  ordered.value.map((s) => {
    const segs = segments(
      s.points.filter((p) => p.t >= props.from && p.t <= props.to),
      props.maxGap,
    )
    const paths = segs.filter((g) => g.length > 1).map((g) => g.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(''))
    const dots = segs.filter((g) => g.length === 1).map((g) => g[0]!)
    const areas =
      s.key === focusKey.value
        ? segs
            .filter((g) => g.length > 1)
            .map((g) => {
              const line = g.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join('')
              return `${line}L${x(g[g.length - 1]!.t).toFixed(1)},${y(0)}L${x(g[0]!.t).toFixed(1)},${y(0)}Z`
            })
        : []
    const lastSeg = segs[segs.length - 1]
    const last = lastSeg ? lastSeg[lastSeg.length - 1]! : null
    const jumps = s.key === focusKey.value ? resetJumps(s.points, props.maxGap).filter((j) => j.t >= props.from).slice(-12) : []
    const dim = props.highlight !== null && props.highlight !== s.key
    return { s, paths, dots, areas, last, jumps, dim }
  }),
)

const hasPoints = computed(() => props.series.some((s) => s.points.some((p) => p.t >= props.from && p.t <= props.to)))

// ── Наведение ─────────────────────────────────────────
const hoverT = ref<number | null>(null)

function onMove(e: PointerEvent) {
  const rect = (e.currentTarget as SVGElement).getBoundingClientRect()
  const px = e.clientX - rect.left
  if (px < M.left || px > M.left + plotW.value) {
    hoverT.value = null
    return
  }
  hoverT.value = props.from + ((px - M.left) / plotW.value) * (props.to - props.from)
}

const hover = computed(() => {
  const t = hoverT.value
  if (t === null) return null
  const rows = props.series.map((s) => {
    const p = pointAt(s.points, t, props.maxGap)
    return { key: s.key, label: s.label, color: s.color, point: p }
  })
  const px = x(t)
  return { t, px, rows, flip: px > M.left + plotW.value * 0.62 }
})

const gradId = `qh-${Math.random().toString(36).slice(2, 8)}`
const focusColor = computed(() => props.series.find((s) => s.key === focusKey.value)?.color ?? 'currentColor')
</script>

<template>
  <div ref="box" class="qchart">
    <svg
      :width="width"
      :height="height"
      :viewBox="`0 0 ${width} ${height}`"
      role="img"
      :aria-label="label"
      @pointermove="onMove"
      @pointerleave="hoverT = null"
    >
      <defs>
        <linearGradient :id="gradId" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" :style="{ stopColor: focusColor, stopOpacity: 0.26 }" />
          <stop offset="100%" :style="{ stopColor: focusColor, stopOpacity: 0 }" />
        </linearGradient>
      </defs>

      <g class="qchart__grid">
        <template v-for="v in Y_TICKS" :key="v">
          <line :x1="M.left" :x2="M.left + plotW" :y1="y(v)" :y2="y(v)" :class="{ 'qchart__base': v === 0 }" />
          <text v-if="v % 50 === 0" :x="M.left - 10" :y="y(v) + 4" text-anchor="end">{{ v }}%</text>
        </template>
        <template v-for="tk in ticks" :key="tk.t">
          <line class="qchart__vt" :x1="x(tk.t)" :x2="x(tk.t)" :y1="M.top" :y2="y(0)" />
          <text :x="x(tk.t)" :y="height - 8" text-anchor="middle">{{ tk.label }}</text>
        </template>
      </g>

      <g v-for="d in drawn" :key="d.s.key" :class="{ 'qchart__dim': d.dim }" :style="{ color: d.s.color }">
        <path v-for="(a, i) in d.areas" :key="`a${i}`" :d="a" :fill="`url(#${gradId})`" stroke="none" />
        <path
          v-for="(p, i) in d.paths"
          :key="`p${i}`"
          :d="p"
          fill="none"
          stroke="currentColor"
          :stroke-width="d.s.key === focusKey ? 2 : 1.6"
          :stroke-dasharray="d.s.dash || undefined"
          stroke-linejoin="round"
          stroke-linecap="round"
        />
        <circle v-for="(p, i) in d.dots" :key="`d${i}`" :cx="x(p.t)" :cy="y(p.v)" r="2.6" fill="currentColor" />
        <circle v-if="d.last" :cx="x(d.last.t)" :cy="y(d.last.v)" r="3.6" fill="currentColor" stroke="var(--surface)" stroke-width="1.5" />
        <text
          v-for="j in d.jumps"
          :key="`j${j.t}`"
          class="qchart__jump"
          :x="x(j.t)"
          :y="Math.max(M.top - 8, y(j.v) - 9)"
          text-anchor="middle"
        >
          +{{ Math.round(j.delta) }}%
        </text>
      </g>

      <g v-if="hover" class="qchart__cross">
        <line :x1="hover.px" :x2="hover.px" :y1="M.top - 6" :y2="y(0)" />
        <template v-for="r in hover.rows" :key="r.key">
          <circle
            v-if="r.point && r.point.v !== null"
            :cx="x(r.point.t)"
            :cy="y(r.point.v)"
            r="4"
            :style="{ fill: r.color }"
            stroke="var(--surface)"
            stroke-width="2"
          />
        </template>
      </g>
    </svg>

    <p v-if="!hasPoints" class="qchart__empty">Нет наблюдений этого окна за выбранный период.</p>

    <div
      v-if="hover"
      class="qchart__tip"
      :class="{ 'qchart__tip--flip': hover.flip }"
      :style="{ left: `${hover.px}px` }"
      aria-hidden="true"
    >
      <span class="qchart__tip-time">{{ formatMoment(hover.t, now) }}</span>
      <span v-for="r in hover.rows" :key="r.key" class="qchart__tip-row">
        <span class="qchart__swatch" :style="{ background: r.color }" />
        <span class="qchart__tip-name">{{ r.label }}</span>
        <span class="qchart__tip-val">
          <template v-if="!r.point">нет наблюдения</template>
          <template v-else-if="r.point.v === null">неизвестно</template>
          <template v-else>{{ formatPercent(r.point.v) }}</template>
        </span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.qchart {
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
.qchart__grid line {
  stroke: #1c1c1c;
  stroke-width: 1;
}
.qchart__grid .qchart__base {
  stroke: #2c2c2c;
}
.qchart__grid .qchart__vt {
  stroke: #151515;
}
.qchart__grid text {
  fill: var(--ink-3);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.qchart__dim {
  opacity: 0.28;
}
.qchart__jump {
  fill: var(--ink-2);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
}
.qchart__cross line {
  stroke: #5a5a5a;
  stroke-width: 1;
}
.qchart__empty {
  position: absolute;
  inset: 0 0 28px 44px;
  display: grid;
  place-items: center;
  max-width: none;
  color: var(--ink-3);
  font-size: var(--fs-sm);
  pointer-events: none;
}
.qchart__tip {
  position: absolute;
  top: 0;
  z-index: 2;
  display: grid;
  gap: 3px;
  min-width: 190px;
  max-width: 300px;
  margin-left: 14px;
  padding: 8px 11px;
  border: 1px solid var(--rule-strong);
  border-radius: 8px;
  background: #121212;
  font-size: var(--fs-sm);
  pointer-events: none;
  box-shadow: 0 10px 28px -12px #000;
}
.qchart__tip--flip {
  transform: translateX(calc(-100% - 28px));
}
.qchart__tip-time {
  color: var(--ink-2);
}
.qchart__tip-row {
  display: grid;
  grid-template-columns: 10px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}
.qchart__swatch {
  width: 10px;
  height: 3px;
  border-radius: 2px;
}
.qchart__tip-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.qchart__tip-val {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}
</style>
