<script setup>
import { computed, onMounted, onUnmounted, ref, useId } from 'vue'

const props = defineProps({
  title: { type: String, required: true },
  dates: { type: Array, required: true },
  series: { type: Array, required: true },
  mode: { type: String, default: 'line' },
  percent: Boolean,
  empty: { type: String, required: true },
})

const heading = useId()
const active = ref(null)
const chartArea = ref(null)
const width = ref(484)
const plot = computed(() => ({ left: 50, right: width.value - 12, top: 12, bottom: 154 }))
let observer
onMounted(() => {
  observer = new ResizeObserver(([entry]) => { width.value = Math.max(120, entry.contentRect.width) })
  observer.observe(chartArea.value)
})
onUnmounted(() => observer?.disconnect())
const number = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 })
const samples = computed(() => props.dates.map((day, index) => ({
  day,
  values: props.series.map((series) => series.values[index]),
})))
const hasValues = computed(() => props.percent
  ? samples.value.some(({ values }) => values.some((value) => value != null))
  : samples.value.some(({ values }) => values.some((value) => value > 0)))
const axisStep = computed(() => {
  if (props.percent) return 25
  const peak = Math.max(0, ...samples.value.map(({ values }) => values.reduce((sum, value) => sum + (value ?? 0), 0)))
  const magnitude = 10 ** Math.floor(Math.log10(Math.max(1, peak / 4)))
  return ([1, 2, 5, 10].find((step) => step * magnitude >= peak / 4) || 1) * magnitude
})
const maximum = computed(() => axisStep.value * 4)
const spacing = computed(() => (plot.value.right - plot.value.left) / Math.max(1, samples.value.length))
const barWidth = computed(() => Math.min(25, spacing.value * .67))
const highlighted = computed(() => samples.value[active.value] || null)

function x(index) { return plot.value.left + spacing.value * (index + .5) }
function y(value) { return plot.value.bottom - (value / maximum.value) * (plot.value.bottom - plot.value.top) }
function format(value) { return value == null ? '—' : `${number.format(value)}${props.percent ? '%' : ''}` }

const dateMarks = computed(() => {
  const last = samples.value.length - 1
  return [...new Set([0, Math.round(last / 3), Math.round(last * 2 / 3), last])].filter((index) => index >= 0 && index <= last)
})
const lines = computed(() => props.series.map((series) => {
  let drawing = false
  return series.values.map((value, index) => {
    if (value == null) { drawing = false; return '' }
    const command = `${drawing ? 'L' : 'M'}${x(index)},${y(value)}`
    drawing = true
    return command
  }).join(' ')
}))

function inspect(event) {
  const bounds = event.currentTarget.getBoundingClientRect()
  const position = (event.clientX - bounds.left) * width.value / bounds.width
  active.value = Math.min(samples.value.length - 1, Math.max(0, Math.floor((position - plot.value.left) / spacing.value)))
}
</script>

<template>
  <section class="chart-card" :aria-labelledby="heading">
    <h3 :id="heading">{{ title }}</h3>
    <ul class="legend" aria-label="图例">
      <li v-for="item in series" :key="item.label"><span class="swatch" :style="{ background: item.color }" />{{ item.label }}</li>
    </ul>
    <div ref="chartArea" class="chart-area">
      <svg :viewBox="`0 0 ${width} 180`" preserveAspectRatio="none" aria-hidden="true" @pointermove="inspect" @pointerleave="active = null">
        <g v-for="tick in 5" :key="tick">
          <line class="grid-line" :x1="plot.left" :x2="plot.right" :y1="y((tick - 1) * axisStep)" :y2="y((tick - 1) * axisStep)" />
          <text class="axis-label" :x="plot.left - 8" :y="y((tick - 1) * axisStep) + 4" text-anchor="end">{{ format((tick - 1) * axisStep) }}</text>
        </g>
        <template v-if="mode === 'bar'">
          <g v-for="(sample, index) in samples" :key="sample.day">
            <rect v-for="(value, seriesIndex) in sample.values" :key="seriesIndex" :x="x(index) - barWidth / 2"
              :y="y(sample.values.slice(0, seriesIndex + 1).reduce((sum, count) => sum + count, 0))"
              :width="barWidth" :height="plot.bottom - y(value)" :fill="series[seriesIndex].color" />
          </g>
        </template>
        <template v-else-if="hasValues">
          <path v-for="(path, index) in lines" :key="series[index].label" :d="path" :stroke="series[index].color" class="plot-line" />
          <g v-for="(sample, index) in samples" :key="sample.day">
            <template v-for="(value, seriesIndex) in sample.values" :key="seriesIndex">
              <circle v-if="value != null" :cx="x(index)" :cy="y(value)" :r="active === index ? 4 : 2.4" :fill="series[seriesIndex].color" />
            </template>
          </g>
        </template>
        <line v-if="highlighted && hasValues" class="guide" :x1="x(active)" :x2="x(active)" :y1="plot.top" :y2="plot.bottom" />
        <text v-for="index in dateMarks" :key="samples[index].day" class="axis-label" :x="x(index)" y="175"
          :text-anchor="index === 0 ? 'start' : index === samples.length - 1 ? 'end' : 'middle'">{{ samples[index].day.slice(5) }}</text>
      </svg>
      <p v-if="!hasValues" class="empty">{{ empty }}</p>
    </div>
    <div class="chart-footer">
      <details v-if="hasValues" class="chart-values">
        <summary>查看明细</summary>
        <table :aria-label="`${title}数据明细`">
          <thead><tr><th scope="col">日期（UTC）</th><th v-for="item in series" :key="item.label" scope="col">{{ item.label }}</th></tr></thead>
          <tbody><tr v-for="sample in samples" :key="sample.day"><th scope="row">{{ sample.day }}</th><td v-for="(value, index) in sample.values" :key="index">{{ format(value) }}</td></tr></tbody>
        </table>
      </details>
      <span v-if="highlighted && hasValues" class="readout" aria-hidden="true">
        {{ highlighted.day.slice(5) }} · {{ series.map((item, index) => `${item.label} ${format(highlighted.values[index])}`).join(' / ') }}
      </span>
    </div>
  </section>
</template>

<style scoped>
.chart-card { min-width: 0; padding: 16px; border: 1px solid var(--line); background: var(--panel); border-radius: 10px; }
h3 { font-size: 14px; margin: 0; }
.legend { display: flex; gap: 8px 14px; flex-wrap: wrap; padding: 0; list-style: none; margin: 8px 0 12px; }
.legend li { display: flex; gap: 6px; align-items: center; color: var(--muted); font-size: 12px; }
.swatch { width: 8px; height: 8px; border-radius: 2px; }
.chart-area { position: relative; height: 180px; }
svg { display: block; width: 100%; height: 100%; overflow: visible; }
.grid-line { stroke: var(--line); vector-effect: non-scaling-stroke; }
.axis-label { font-size: 11px; fill: var(--muted); }
.plot-line { fill: none; stroke-width: 2; stroke-linejoin: round; stroke-linecap: round; vector-effect: non-scaling-stroke; }
.guide { stroke: var(--muted); stroke-opacity: .55; stroke-dasharray: 3 4; vector-effect: non-scaling-stroke; }
.empty { position: absolute; inset: 0 0 20px 50px; margin: 0; display: grid; place-content: center; color: var(--muted); font-size: 13px; pointer-events: none; }
.chart-footer { min-height: 24px; margin-top: 4px; position: relative; }
.chart-values { font-size: 12px; color: var(--muted); }
.chart-values summary { cursor: pointer; width: fit-content; padding: 4px 0; }
.chart-values table { border-collapse: collapse; width: 100%; }
.chart-values th, .chart-values td { border-bottom: 1px solid var(--line); padding: 7px 4px; text-align: end; }
.chart-values :is(th, td):first-child { text-align: start; }
.readout { position: absolute; inset-inline-end: 0; top: 4px; max-width: 78%; font-size: 11px; color: var(--muted); }
@media (max-width: 560px) { .readout { display: none; } }
</style>
