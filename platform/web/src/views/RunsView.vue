<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ selectedRun: { type: String, default: '' } })
const rows = ref([])
const detail = ref(null)
const detailError = ref('')
const hasScreenshots = computed(() => detail.value?.steps?.some((step) => step.screenshot_jpeg || step.screenshot_error))
const screenshotErrors = { limit: '已达截图上限', too_large: '截图过大', unavailable: '截图不可用' }
let selectedRun = null
let detailTimer

async function load() {
  rows.value = await api.get('/api/runs')
}

async function openDetail(row) {
  clearTimeout(detailTimer)
  selectedRun = row.id
  detail.value = row
  detailError.value = ''
  await refreshDetail(row.id)
}

async function refreshDetail(runId) {
  try {
    const result = await api.get(`/api/runs/${runId}`)
    if (selectedRun !== runId) return
    detail.value = result
    const { steps, ...summary } = result
    rows.value = rows.value.map((row) => row.id === runId ? summary : row)
    if (['queued', 'running'].includes(result.status)) {
      detailTimer = setTimeout(() => refreshDetail(runId), 1500)
    }
  } catch (error) {
    if (selectedRun === runId) detailError.value = error.message
  }
}

function closeDetail() {
  clearTimeout(detailTimer)
  selectedRun = null
  detail.value = null
}

async function downloadReport() {
  const runId = detail.value.id
  detailError.value = ''
  try {
    const url = URL.createObjectURL(await api.download(`/api/runs/${runId}/report`))
    const link = document.createElement('a')
    link.href = url
    link.download = `Auto-Test-${runId}.html`
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (problem) { detailError.value = problem.message }
}

function ms(v) {
  if (v == null) return '—'
  return v > 1000 ? (v / 1000).toFixed(1) + 's' : v + 'ms'
}

onMounted(async () => {
  await load()
  if (props.selectedRun) {
    await openDetail(rows.value.find((row) => row.id === props.selectedRun) || { id: props.selectedRun })
  }
})
onUnmounted(closeDetail)
</script>

<template>
  <header class="head">
    <button @click="load">刷新</button>
  </header>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'case_name', label: '用例', width: '24%' },
      { key: 'case_type', label: '类型', width: '8%' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'self_healed', label: '自愈', width: '8%' },
      { key: 'token_in', label: 'Token', width: '12%' },
      { key: 'duration_ms', label: '耗时', width: '10%' },
      { key: 'started_at', label: '开始时间', width: '18%' },
    ]"
    empty="还没有执行记录"
    clickable
    @row-click="openDetail"
  >
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #case_type="{ row }">{{ { ui: 'UI', api: '接口', perf: '性能' }[row.case_type] || row.case_type }}</template>
    <template #self_healed="{ row }">{{ row.self_healed ? '♻' : '—' }}</template>
    <template #token_in="{ row }">{{ row.token_in ? row.token_in + ' / ' + row.token_out : '—' }}</template>
    <template #duration_ms="{ row }">{{ ms(row.duration_ms) }}</template>
  </DataTable>

  <div v-if="detail" class="drawer" @click.self="closeDetail">
    <div class="panel">
      <h3>{{ detail.case_name }}</h3>
      <dl>
        <dt>状态</dt><dd role="status"><StatusTag :value="detail.status" /></dd>
        <dt>场景</dt><dd>{{ detail.scenario }}</dd>
        <dt>耗时</dt><dd>{{ ms(detail.duration_ms) }}</dd>
        <dt>Token</dt><dd>{{ detail.token_in }} / {{ detail.token_out }}</dd>
        <dt>需复核</dt><dd>{{ detail.needs_review ? '是' : '否' }}</dd>
      </dl>
      <template v-if="detail.performance">
        <h4>性能指标</h4>
        <dl>
          <dt>请求数</dt><dd>{{ detail.performance.requests }}</dd>
          <dt>失败数</dt><dd>{{ detail.performance.failed_requests }}</dd>
          <dt>错误率</dt><dd>{{ (detail.performance.error_rate * 100).toFixed(2) }}%</dd>
          <dt>吞吐量</dt><dd>{{ detail.performance.rps }} 请求/秒</dd>
          <dt>P50 / P95 / P99</dt><dd>{{ ms(detail.performance.p50_ms) }} / {{ ms(detail.performance.p95_ms) }} / {{ ms(detail.performance.p99_ms) }}</dd>
        </dl>
      </template>
      <h4>报告</h4>
      <pre>{{ detail.report || '暂无报告' }}</pre>
      <p v-if="detailError" role="alert">{{ detailError }}</p>
      <div v-if="detail.steps?.length" class="step-table" role="region" aria-label="执行步骤" tabindex="0">
        <table>
          <caption>执行步骤</caption>
          <thead><tr><th scope="col">序号</th><th scope="col">动作</th><th scope="col">状态</th><th scope="col">耗时</th><th v-if="hasScreenshots" scope="col">截图</th></tr></thead>
          <tbody>
            <tr v-for="step in detail.steps" :key="step.step">
              <th scope="row">{{ step.step }}</th><td>{{ step.action }}</td>
              <td><StatusTag :value="step.status" /></td><td>{{ ms(step.duration_ms) }}</td>
              <td v-if="hasScreenshots">
                <details v-if="step.screenshot_jpeg">
                  <summary>查看截图</summary>
                  <img class="step-screenshot" :src="`data:image/jpeg;base64,${step.screenshot_jpeg}`"
                    :alt="`第 ${step.step} 步执行后的页面`" loading="lazy" />
                </details>
                <span v-else>{{ screenshotErrors[step.screenshot_error] || '—' }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <button type="button" @click="downloadReport">下载 HTML 报告</button>
      <button type="button" @click="closeDetail">关闭</button>
    </div>
  </div>
</template>

<style scoped>
.head { margin-bottom: 14px; }
.drawer { position: fixed; inset: 0; background: rgba(20,20,24,0.28); display: flex; justify-content: flex-end; }
.panel { width: 460px; background: var(--panel); padding: 22px; overflow: auto; }
.panel h3 { margin: 0 0 16px; font-size: 15px; }
.panel h4 { font-size: 12px; color: var(--muted); margin: 0 0 8px; }
dl { display: grid; grid-template-columns: 84px 1fr; gap: 10px 12px; font-size: 13px; margin: 0 0 20px; }
dt { color: var(--muted); }
dd { margin: 0; word-break: break-word; }
pre { background: #f7f7f9; border: 1px solid var(--line); border-radius: 8px; padding: 12px; font-size: 12px; white-space: pre-wrap; margin: 0 0 18px; }
.step-table { overflow: auto; margin-bottom: 18px; }
table { width: 100%; border-collapse: collapse; font-size: 12px; }
caption { text-align: start; color: var(--muted); margin-bottom: 8px; }
th, td { text-align: start; padding: 8px 6px; border-bottom: 1px solid var(--line); }
.step-screenshot { display: block; max-inline-size: min(55vw, 240px); block-size: auto; margin-block-start: 6px; }
</style>
