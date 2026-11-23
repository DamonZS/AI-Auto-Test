<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import StatCard from '../components/StatCard.vue'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const stats = ref({})
const trends = ref({ days: [] })
const datasets = ref([])
const percent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 })
const signedPercent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1, signDisplay: 'exceptZero' })
const comparisonMetrics = [
  { key: 'completed_runs', label: '完成执行数' },
  { key: 'pass_rate', label: '通过率', formatter: percent },
  { key: 'new_defects', label: '新增缺陷' },
]

const passRate = computed(() => {
  const d = trends.value.days || []
  const p = d.reduce((n, x) => n + (x.passed || 0), 0)
  const f = d.reduce((n, x) => n + (x.failed || 0), 0)
  return p + f === 0 ? '—' : `${Math.round((p / (p + f)) * 100)}%`
})

const maxDaily = computed(() =>
  Math.max(1, ...(trends.value.days || []).map((d) => d.total || 0)),
)

const comparison = computed(() => trends.value.comparison)

function formatComparison(value, formatter) {
  if (value == null) return '—'
  return formatter ? formatter.format(value) : value
}

async function load() {
  stats.value = await api.get('/api/overview')
  trends.value = await api.get('/api/trends')
  datasets.value = await api.get('/api/test-data/datasets')
}

onMounted(load)
defineExpose({ load })
</script>

<template>
  <div class="grid">
    <StatCard label="项目" :value="stats.projects ?? 0" />
    <StatCard label="需求" :value="stats.requirements ?? 0" />
    <StatCard label="用例" :value="stats.cases ?? 0" />
    <StatCard label="已定稿用例" :value="stats.approved ?? 0" tone="ok" />
    <StatCard label="执行次数" :value="stats.runs ?? 0" />
    <StatCard label="通过率" :value="passRate" />
    <StatCard label="未关闭缺陷" :value="stats.open_bugs ?? 0" :tone="stats.open_bugs ? 'warn' : ''" />
    <StatCard label="待审批补丁" :value="stats.patches ?? 0" />
    <StatCard label="测试数据集" :value="stats.test_data_sets ?? 0" />
    <StatCard label="测试数据记录" :value="stats.test_data_records ?? 0" />
  </div>

  <section>
    <h3>近 30 天执行</h3>
    <div v-if="trends.days?.length" class="bars">
      <div v-for="d in trends.days" :key="d.day" class="bar-col">
        <div class="bars-stack">
          <div class="seg ok" :style="{ height: ((d.passed || 0) / maxDaily) * 120 + 'px' }" />
          <div class="seg bad" :style="{ height: ((d.failed || 0) / maxDaily) * 120 + 'px' }" />
        </div>
        <span>{{ d.day.slice(5) }}</span>
      </div>
    </div>
    <p v-else class="empty">还没有执行数据</p>
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

  <section>
    <h3>缺陷分布</h3>
    <div class="split">
      <div>
        <h4>按严重程度</h4>
        <DataTable :rows="trends.bugs_by_sev || []" :columns="[{ key: 'severity', label: '级别' }, { key: 'n', label: '数量' }]" empty="暂无缺陷" />
      </div>
      <div>
        <h4>按状态</h4>
        <DataTable :rows="trends.bugs_by_stat || []" :columns="[{ key: 'status', label: '状态' }, { key: 'n', label: '数量' }]" empty="暂无缺陷">
          <template #status="{ row }"><StatusTag :value="row.status" /></template>
        </DataTable>
      </div>
    </div>
  </section>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 28px; }
section { margin-bottom: 28px; }
h3 { font-size: 14px; font-weight: 600; margin: 0 0 12px; }
h4 { font-size: 12px; font-weight: 500; color: var(--muted); margin: 0 0 8px; }
.bars { display: flex; gap: 6px; align-items: flex-end; padding: 14px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
.bar-col { display: flex; flex-direction: column; align-items: center; gap: 6px; flex: 1; }
.bars-stack { display: flex; flex-direction: column-reverse; justify-content: flex-start; height: 120px; }
.seg { width: 14px; border-radius: 2px; }
.seg.ok { background: #34a26b; }
.seg.bad { background: #d14334; }
.bar-col span { font-size: 10px; color: var(--muted); }
.split { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
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
@media (max-width: 900px) { .grid { grid-template-columns: repeat(2, 1fr); } .split { grid-template-columns: 1fr; } }
</style>
