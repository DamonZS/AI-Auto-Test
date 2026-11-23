<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import StatCard from '../components/StatCard.vue'
import DataTable from '../components/DataTable.vue'
import TrendChart from '../components/TrendChart.vue'

const stats = ref({})
const trends = ref({ days: [] })
const datasets = ref([])
const loading = ref(true)
const error = ref('')
const range = ref(30)
const integer = new Intl.NumberFormat('zh-CN')
const percent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 })
const signedPercent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1, signDisplay: 'exceptZero' })
const comparisonMetrics = [
  { key: 'completed_runs', label: '完成执行数' },
  { key: 'pass_rate', label: '通过率', formatter: percent },
  { key: 'new_defects', label: '新增缺陷' },
]

const days = computed(() => trends.value.days.slice(-range.value))
const dates = computed(() => days.value.map((day) => day.day))
const outcomeFields = [
  { key: 'passed', label: '通过', color: 'var(--chart-pass)' },
  { key: 'failed', label: '失败', color: 'var(--chart-fail)' },
  { key: 'needs_review', label: '待复核', color: 'var(--chart-review)' },
  { key: 'error', label: '异常', color: 'var(--chart-error)' },
]
const outcomeSeries = computed(() => {
  const series = outcomeFields.map((field) => ({
    ...field, values: days.value.map((day) => day[field.key] ?? 0),
  }))
  const other = days.value.map((day) => Math.max(0, day.total - outcomeFields.reduce((sum, field) => sum + (day[field.key] ?? 0), 0)))
  if (other.some((count) => count > 0)) series.push({ label: '其他', color: 'var(--chart-neutral)', values: other })
  return series
})
const defectSeries = computed(() => [{ label: '新增缺陷', color: 'var(--chart-fail)', values: days.value.map((day) => day.new_bugs ?? 0) }])
const rateSeries = computed(() => [{
  label: '通过率', color: 'var(--chart-primary)', values: days.value.map(({ passed, failed }) => passed + failed ? passed / (passed + failed) * 100 : null),
}])
const passRate = computed(() => {
  const passed = days.value.reduce((sum, day) => sum + day.passed, 0)
  const failed = days.value.reduce((sum, day) => sum + day.failed, 0)
  return passed + failed ? percent.format(passed / (passed + failed)) : '—'
})

const metrics = computed(() => [
  { icon: 'projects', label: '项目', value: stats.value.projects },
  { icon: 'requirements', label: '需求', value: stats.value.requirements },
  { icon: 'cases', label: '用例', value: stats.value.cases },
  { icon: 'approved', label: '已定稿用例', value: stats.value.approved, tone: 'ok' },
  { icon: 'runs', label: '执行次数', value: stats.value.runs },
  { icon: 'pass-rate', label: `近 ${range.value} 天通过率`, value: passRate.value },
  { icon: 'bugs', label: '未关闭缺陷', value: stats.value.open_bugs, tone: stats.value.open_bugs ? 'warn' : '' },
  { icon: 'patches', label: '待审批补丁', value: stats.value.patches },
  { icon: 'datasets', label: '测试数据集', value: stats.value.test_data_sets },
  { icon: 'records', label: '测试数据记录', value: stats.value.test_data_records },
])
const severities = [
  { key: 'S1', label: 'S1 致命', color: 'var(--chart-fail)' },
  { key: 'S2', label: 'S2 严重', color: 'var(--chart-orange)' },
  { key: 'S3', label: 'S3 一般', color: 'var(--chart-primary)' },
  { key: 'S4', label: 'S4 轻微', color: 'var(--chart-pass)' },
]
const severity = computed(() => {
  const counts = new Map((trends.value.bugs_by_sev || []).map((row) => [row.severity, row.n]))
  const other = [...counts].reduce((sum, [key, count]) => sum + (severities.some((item) => item.key === key) ? 0 : count), 0)
  return [...severities.map((item) => ({ ...item, count: counts.get(item.key) || 0 })),
    ...(other ? [{ key: 'other', label: '其他', color: 'var(--chart-error)', count: other }] : [])]
})
const defectTotal = computed(() => severity.value.reduce((sum, item) => sum + item.count, 0))
const severityGradient = computed(() => {
  if (!defectTotal.value) return 'var(--bg)'
  let start = 0
  const segments = severity.value.filter((item) => item.count > 0).map((item) => {
    const end = start + item.count / defectTotal.value * 100
    const stop = `${item.color} ${start}% ${end}%`
    start = end
    return stop
  })
  return `conic-gradient(${segments.join(', ')})`
})
const statusLabels = {
  new: '新建', confirmed: '已确认', fixing: '修复中', fixed: '已修复', verified: '已验证',
  closed: '已关闭', reopened: '重新打开', rejected: '已拒绝',
}
const defectStatuses = computed(() => [...(trends.value.bugs_by_stat || [])].sort((first, second) => second.n - first.n))
const largestStatus = computed(() => Math.max(1, ...defectStatuses.value.map((item) => item.n)))

const comparison = computed(() => trends.value.comparison)

function formatComparison(value, formatter) {
  if (value == null) return '—'
  return formatter ? formatter.format(value) : value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[stats.value, trends.value, datasets.value] = await Promise.all([
      api.get('/api/overview'), api.get('/api/trends'), api.get('/api/test-data/datasets'),
    ])
  } catch (problem) {
    error.value = problem.message
  } finally {
    loading.value = false
  }
}

onMounted(load)
defineExpose({ load })
</script>

<template>
  <div class="dashboard">
    <p v-if="loading" role="status" class="empty">正在读取概览…</p>
    <p v-if="error" role="alert" class="empty">{{ error }} <button type="button" @click="load">重新加载</button></p>
    <div class="grid" aria-label="工作区指标">
      <StatCard v-for="metric in metrics" :key="metric.icon" :label="metric.label" :icon="metric.icon" :tone="metric.tone || ''"
        :value="loading || error ? '—' : typeof metric.value === 'string' ? metric.value : integer.format(metric.value ?? 0)" />
    </div>
    <div v-if="!loading && !error" class="dashboard-body">
      <section class="dashboard-block" aria-labelledby="run-trends-title">
        <header class="section-heading">
          <h3 id="run-trends-title">执行趋势</h3>
          <label class="range-control">统计范围
            <select v-model.number="range"><option :value="7">近 7 天</option><option :value="14">近 14 天</option><option :value="30">近 30 天</option></select>
          </label>
        </header>
        <div class="split">
          <TrendChart title="执行结果" mode="bar" :dates="dates" :series="outcomeSeries" empty="执行完成后显示每日结果" />
          <TrendChart title="通过率趋势" percent :dates="dates" :series="rateSeries" empty="产生通过或失败结果后显示趋势" />
        </div>
      </section>
      <section class="dashboard-block" aria-labelledby="defect-distribution-title">
        <header class="section-heading"><h3 id="defect-distribution-title">缺陷分析</h3></header>
        <div class="split">
          <section class="distribution-card" aria-labelledby="defect-severity-title">
            <h4 id="defect-severity-title">严重程度分布</h4>
            <div class="severity-layout">
              <div class="donut" :style="{ background: severityGradient }" aria-hidden="true"><span>{{ integer.format(defectTotal) }}<small>缺陷总数</small></span></div>
              <dl class="severity-list">
                <div v-for="item in severity" :key="item.key">
                  <dt><span class="swatch" :style="{ background: item.color }" />{{ item.label }}</dt>
                  <dd>{{ integer.format(item.count) }} <span>{{ defectTotal ? percent.format(item.count / defectTotal) : '—' }}</span></dd>
                </div>
              </dl>
            </div>
          </section>
          <section class="distribution-card" aria-labelledby="defect-status-title">
            <h4 id="defect-status-title">状态分布</h4>
            <dl v-if="defectStatuses.length" class="status-list">
              <div v-for="item in defectStatuses" :key="item.status" class="status-row">
                <dt>{{ statusLabels[item.status] || item.status }}</dt>
                <dd><div class="status-track" aria-hidden="true"><div :style="{ width: `${item.n / largestStatus * 100}%` }" /></div><span>{{ integer.format(item.n) }}</span></dd>
              </div>
            </dl>
            <p v-else class="status-empty empty">提交缺陷后显示状态分布</p>
          </section>
        </div>
        <TrendChart class="defect-trend" title="新增缺陷趋势" :dates="dates" :series="defectSeries" empty="当前统计范围内没有新增缺陷" />
      </section>

  <section v-if="comparison">
    <h3 id="overview-monthly-trends">月度趋势 · UTC</h3>
    <div class="comparison-table" tabindex="0" role="region" aria-labelledby="overview-monthly-trends">
      <table>
        <thead>
          <tr>
            <th scope="col">指标</th>
            <th scope="col">{{ comparison.current.start.slice(0, 7) }}</th>
            <th scope="col">环比 <span>{{ comparison.mom.baseline.start.slice(0, 7) }}</span></th>
            <th scope="col">同比 <span>{{ comparison.yoy.baseline.start.slice(0, 7) }}</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="metric in comparisonMetrics" :key="metric.key">
            <th scope="row">{{ metric.label }}</th>
            <td>{{ formatComparison(comparison.current[metric.key], metric.formatter) }}</td>
            <td>{{ formatComparison(comparison.mom.change[metric.key], signedPercent) }}</td>
            <td>{{ formatComparison(comparison.yoy.change[metric.key], signedPercent) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="comparison-note">按完整自然月计算相对变化。缺少数据或基期值为 0 时显示「—」。</p>
  </section>

  <section>
    <h3>测试数据资产</h3>
    <DataTable
      :rows="datasets.slice(0, 5)"
      :columns="[
        { key: 'name', label: '数据集', width: '28%' },
        { key: 'project_name', label: '项目', width: '22%' },
        { key: 'record_count', label: '记录数', width: '14%' },
        { key: 'fields', label: '字段' },
        { key: 'updated_at', label: '更新时间', width: '22%' },
      ]"
      empty="还没有测试数据集"
    >
      <template #fields="{ row }">
        <span v-for="field in row.fields" :key="field.name" class="chip">{{ field.name }}</span>
      </template>
    </DataTable>
  </section>

    </div>
  </div>
</template>

<style scoped>
.dashboard { --chart-primary: var(--accent); --chart-pass: #1a8a62; --chart-fail: #be3c43; --chart-review: #996322; --chart-error: #6a6584; --chart-orange: #bc661f; --chart-neutral: #64748b; font-variant-numeric: tabular-nums; }
.dashboard-body > section { margin-bottom: 24px; }
.grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; margin-bottom: 24px; }
h3 { font-size: 14px; font-weight: 600; margin: 0 0 12px; }
h4 { font-size: 14px; margin: 0 0 16px; }
.section-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 10px; }
.section-heading h3 { margin: 0; font-size: 15px; }
.range-control { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 12px; }
.range-control select { min-height: 32px; font: inherit; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 4px 8px; }
.split { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.distribution-card { min-width: 0; padding: 16px; border: 1px solid var(--line); background: var(--panel); border-radius: 10px; }
.defect-trend { margin-top: 12px; }
.severity-layout { display: flex; align-items: center; justify-content: space-around; gap: 16px; min-height: 148px; }
.donut { position: relative; display: grid; place-items: center; flex: none; width: 128px; height: 128px; border-radius: 50%; box-shadow: inset 0 0 0 1px var(--line); }
.donut > span { display: grid; place-content: center; text-align: center; width: 94px; height: 94px; border-radius: 50%; background: var(--panel); font-size: 24px; font-weight: 600; }
.donut small { font-size: 11px; color: var(--muted); font-weight: 400; }
.severity-list, .status-list { margin: 0; }
.severity-list div, .status-list div.status-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 30px; }
.severity-list dt { display: flex; align-items: center; gap: 7px; white-space: nowrap; }
.severity-list dd, .status-list dd { margin: 0; text-align: end; }
.severity-list dd span { display: inline-block; color: var(--muted); min-width: 42px; }
.swatch { width: 8px; height: 8px; border-radius: 2px; }
.status-row dt { min-width: 70px; }
.status-row dd { flex: 1; display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 8px; }
.status-track { height: 8px; background: var(--bg); border-radius: 4px; }
.status-track > div { height: 100%; background: var(--chart-primary); border-radius: 4px; }
.status-empty { min-height: 148px; display: grid; place-content: center; margin: 0; }
.chip { display: inline-block; background: #f1f1f4; border-radius: 4px; padding: 1px 6px; margin-right: 4px; font-size: 12px; color: var(--muted); }
.empty { color: var(--muted); }
.comparison-table { max-inline-size: 100%; overflow: auto; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }
.comparison-table table { width: 100%; border-collapse: separate; border-spacing: 0; font-size: 13px; }
.comparison-table th, .comparison-table td { padding: 10px 14px; text-align: left; white-space: nowrap; border-bottom: 1px solid var(--line); }
.comparison-table thead th { background: #fbfbfc; color: var(--muted); font-weight: 500; }
.comparison-table thead span { display: block; }
.comparison-table tbody th { position: sticky; inset-inline-start: 0; background: var(--panel); font-weight: 500; }
.comparison-table tbody tr:last-child > * { border-bottom: 0; }
.comparison-note { margin: 8px 0 0; color: var(--muted); font-size: 12px; }
@media (max-width: 1100px) { .grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 820px) { .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .split { grid-template-columns: 1fr; } }
@media (max-width: 520px) { .severity-layout { flex-wrap: wrap; } }
</style>
