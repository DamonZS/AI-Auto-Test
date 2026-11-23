<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({
  projectId: { type: String, default: '' },
  selectedRun: { type: String, default: '' },
})
const emit = defineEmits(['open-project-view'])
const runStatuses = { queued: '排队中', running: '执行中', passed: '通过', failed: '失败', error: '异常', needs_review: '需复核' }
const screenshotErrors = { limit: '已达截图上限', too_large: '截图过大', unavailable: '截图不可用' }
const rows = ref([])
const statusFilter = ref('')
const runSearch = ref('')
const loading = ref(false)
const loadError = ref('')
const detail = ref(null)
const detailError = ref('')
const detailDialog = ref(null)
const evidenceSearch = ref('')
const steps = computed(() => detail.value?.steps ?? [])
const hasScreenshots = computed(() => steps.value.some((step) => step.screenshot_jpeg || step.screenshot_error))
const visibleRuns = computed(() => rows.value.filter((row) => {
  const statusMatches = !statusFilter.value || row.status === statusFilter.value
    || (statusFilter.value === 'needs_review' && row.needs_review)
  return statusMatches && matchesSearch(runSearch.value, [row.case_name, row.id, row.case_id, row.suite_run_id, row.report, row.fallback_reason])
}))
const visibleSteps = computed(() => steps.value.filter((step) => matchesSearch(evidenceSearch.value, [
  step.step, step.action, step.status, runStatuses[step.status],
  step.screenshot_jpeg ? '截图' : screenshotErrors[step.screenshot_error],
])))
let selectedRun = null
let detailTimer
let mounted = true

function matchesSearch(search, fields) {
  const keyword = search.toLowerCase()
  return !keyword || fields.some((field) => String(field ?? '').toLowerCase().includes(keyword))
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const query = props.projectId ? `?${new URLSearchParams({ project_id: props.projectId })}` : ''
    rows.value = await api.get(`/api/runs${query}`)
    return true
  } catch (problem) {
    loadError.value = problem.message
    return false
  } finally {
    loading.value = false
  }
}

async function openDetail(row) {
  clearTimeout(detailTimer)
  selectedRun = row.id
  detail.value = row
  detailError.value = ''
  evidenceSearch.value = ''
  await nextTick()
  if (selectedRun !== row.id) return
  if (!detailDialog.value.open) detailDialog.value.showModal()
  await refreshDetail(row.id)
}

async function refreshDetail(runId) {
  try {
    const result = await api.get(`/api/runs/${encodeURIComponent(runId)}`)
    if (selectedRun !== runId) return
    if (props.projectId && !detail.value.case_id) {
      const query = new URLSearchParams({ project_id: props.projectId })
      const projectCases = await api.get(`/api/cases?${query}`)
      if (selectedRun !== runId) return
      if (!projectCases.some((item) => item.id === result.case_id)) throw new Error('此执行记录不属于当前项目')
    }
    detail.value = result
    const { steps, ...summary } = result
    rows.value = rows.value.map((row) => row.id === runId ? { ...row, ...summary } : row)
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
  detailDialog.value?.close()
}

async function downloadReport() {
  const runId = detail.value.id
  detailError.value = ''
  try {
    const url = URL.createObjectURL(await api.download(`/api/runs/${encodeURIComponent(runId)}/report`))
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
  const loaded = await load()
  if (mounted && loaded && props.selectedRun) {
    await openDetail(rows.value.find((row) => row.id === props.selectedRun) || { id: props.selectedRun })
  }
})
onUnmounted(() => {
  mounted = false
  closeDetail()
})
</script>

<template>
  <header class="head">
    <label for="run-search">搜索执行记录
      <input id="run-search" v-model.trim="runSearch" type="search" placeholder="用例、执行 ID、批次 ID 或报告" />
    </label>
    <label for="run-status">执行状态
      <select id="run-status" v-model="statusFilter">
        <option value="">全部状态</option>
        <option v-for="(label, status) in runStatuses" :key="status" :value="status">{{ label }}</option>
      </select>
    </label>
    <button type="button" :disabled="loading" @click="load">刷新队列</button>
  </header>
  <p v-if="loadError" class="error" role="alert">无法加载执行记录：{{ loadError }}</p>
  <p v-if="loading && !rows.length" role="status">正在加载执行记录…</p>
  <DataTable
    v-if="visibleRuns.length"
    :rows="visibleRuns"
    :columns="[
      { key: 'case_name', label: '用例', width: '24%' },
      { key: 'case_type', label: '类型', width: '8%' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'self_healed', label: '自愈', width: '8%' },
      { key: 'token_in', label: 'Token', width: '12%' },
      { key: 'duration_ms', label: '耗时', width: '10%' },
      { key: 'started_at', label: '开始时间', width: '18%' },
      { key: 'evidence', label: '证据' },
    ]"
    clickable
    @row-click="openDetail"
  >
    <template #status="{ row }"><StatusTag v-if="row.status !== 'needs_review'" :value="row.status" /><span v-else>需复核</span></template>
    <template #case_type="{ row }">{{ { ui: 'UI 自动化', api: '接口', perf: '性能', functional: '手工功能', security: '防御性审查' }[row.case_type] || row.case_type }}</template>
    <template #self_healed="{ row }">{{ row.self_healed ? '♻' : '—' }}</template>
    <template #token_in="{ row }">{{ row.token_in ? row.token_in + ' / ' + row.token_out : '—' }}</template>
    <template #duration_ms="{ row }">{{ ms(row.duration_ms) }}</template>
    <template #evidence="{ row }">
      <button type="button" class="view-evidence" :aria-label="`查看 ${row.case_name} 的执行证据`" @click.stop="openDetail(row)">查看证据</button>
    </template>
  </DataTable>
  <section v-else-if="!loading && !loadError" class="empty">
    <template v-if="rows.length">
      <p role="status">没有符合筛选条件的执行记录。</p>
      <button type="button" @click="runSearch = ''; statusFilter = ''">清除筛选</button>
    </template>
    <template v-else>
      <p role="status">{{ projectId ? '当前项目暂无执行记录。' : '暂无执行记录。' }}</p>
      <div class="actions">
        <button class="primary" type="button" @click="emit('open-project-view', { projectId, view: 'suites' })">运行套件</button>
        <button type="button" @click="emit('open-project-view', { projectId, view: 'cases' })">前往用例</button>
      </div>
    </template>
  </section>

  <dialog ref="detailDialog" class="drawer" aria-labelledby="run-detail-title" @cancel.prevent="closeDetail" @click.self="closeDetail">
    <div v-if="detail" class="panel">
      <header class="detail-head">
        <h3 id="run-detail-title">{{ detail.case_name || '执行证据' }}</h3>
        <button type="button" autofocus @click="closeDetail">关闭</button>
      </header>
      <p v-if="detailError" class="error" role="alert">{{ detailError }}</p>
      <p v-else-if="!detail.steps" role="status">正在加载执行证据…</p>
      <dl v-if="detail.case_id">
        <dt>状态</dt><dd role="status"><StatusTag v-if="detail.status !== 'needs_review'" :value="detail.status" /><span v-else>需复核</span></dd>
        <dt>执行 ID</dt><dd>{{ detail.id }}</dd>
        <template v-if="detail.suite_run_id"><dt>批次 ID</dt><dd>{{ detail.suite_run_id }}</dd></template>
        <template v-if="['functional', 'security'].includes(detail.case_type)"><dt>验证方式</dt><dd>人工核验</dd></template>
        <template v-else>
          <template v-if="detail.scenario"><dt>场景</dt><dd><details><summary>查看用例场景</summary><pre>{{ detail.scenario }}</pre></details></dd></template>
          <dt>耗时</dt><dd>{{ ms(detail.duration_ms) }}</dd>
          <template v-if="detail.token_in || detail.token_out"><dt>Token</dt><dd>{{ detail.token_in }} / {{ detail.token_out }}</dd></template>
        </template>
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
      <section v-if="detail.steps" aria-labelledby="run-steps-heading">
        <h4 id="run-steps-heading">执行步骤</h4>
        <label v-if="steps.length" class="evidence-filter" for="run-step-search">搜索步骤
          <input id="run-step-search" v-model.trim="evidenceSearch" type="search" placeholder="序号、动作、状态或截图" />
        </label>
        <p v-if="!steps.length" class="hint" role="status">{{ ['queued', 'running'].includes(detail.status) ? '等待步骤证据。' : '此执行没有步骤证据。' }}</p>
        <p v-else-if="!visibleSteps.length" class="hint" role="status">没有符合条件的步骤。</p>
        <div v-else class="step-table" role="region" aria-labelledby="run-steps-heading" tabindex="0">
          <table>
            <caption>{{ visibleSteps.length }} / {{ steps.length }} 个步骤</caption>
            <thead><tr><th scope="col">序号</th><th scope="col">动作</th><th scope="col">状态</th><th scope="col">耗时</th><th v-if="hasScreenshots" scope="col">截图</th></tr></thead>
            <tbody>
              <tr v-for="step in visibleSteps" :key="step.step">
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
      </section>
      <section v-if="detail.case_id" aria-labelledby="run-report-heading">
        <h4 id="run-report-heading">报告</h4>
        <pre v-if="detail.report">{{ detail.report }}</pre>
        <p v-else class="hint">暂无报告。</p>
        <button type="button" @click="downloadReport">下载 HTML 报告</button>
      </section>
    </div>
  </dialog>
</template>

<style scoped>
.head, .actions { display: flex; align-items: end; flex-wrap: wrap; gap: 10px; }
.head { margin-bottom: 14px; }
.head label, .evidence-filter { display: grid; gap: 4px; font-size: 13px; }
.head input { width: min(100%, 300px); }
.head select { font: inherit; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 7px 10px; }
.view-evidence { white-space: nowrap; }
.empty { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 18px; }
.empty p { margin: 0 0 12px; }
.error { color: var(--bad); }
.hint { color: var(--muted); font-size: 13px; }
.drawer { position: fixed; inset: 0 0 0 auto; margin: 0; inline-size: min(100%, 540px); max-inline-size: none; block-size: 100%; max-block-size: none; padding: 0; border: 0; color: var(--text); background: var(--panel); }
.drawer::backdrop { background: rgba(20,20,24,0.28); }
.panel { min-block-size: 100%; padding: 22px; }
.detail-head { display: flex; align-items: start; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.panel h3 { margin: 0; font-size: 15px; overflow-wrap: anywhere; }
.panel h4 { font-size: 14px; margin: 0 0 8px; }
.evidence-filter { margin-bottom: 10px; }
dl { display: grid; grid-template-columns: 84px 1fr; gap: 10px 12px; font-size: 13px; margin: 0 0 20px; }
dt { color: var(--muted); }
dd { margin: 0; word-break: break-word; }
pre { background: #f7f7f9; border: 1px solid var(--line); border-radius: 8px; padding: 12px; font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; margin: 0 0 18px; }
.step-table { overflow: auto; margin-bottom: 18px; }
table { width: 100%; border-collapse: collapse; font-size: 12px; }
caption { text-align: start; color: var(--muted); margin-bottom: 8px; }
th, td { text-align: start; padding: 8px 6px; border-bottom: 1px solid var(--line); }
.step-screenshot { display: block; max-inline-size: min(55vw, 240px); block-size: auto; margin-block-start: 6px; }
</style>
