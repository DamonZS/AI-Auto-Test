<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ projectId: { type: String, default: '' } })
const emit = defineEmits(['open-project-view'])
const suites = ref([])
const cases = ref([])
const name = ref('')
const suiteId = ref('')
const maxConcurrency = ref(1)
const caseId = ref('')
const selected = ref([])
const batchId = ref('')
const queuedId = ref('')
const history = ref([])
const summary = ref(null)
const schedule = ref(null)
const scheduleForm = ref(null)
const orderNotice = ref('')
const weekdays = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六']
const error = ref('')
const busy = ref(false)
let mounted = true

const casesById = computed(() => new Map(cases.value.map((item) => [item.id, item])))
const activeSuite = computed(() => suites.value.find((suite) => suite.id === suiteId.value))
const suiteProjectId = computed(() => props.projectId || activeSuite.value?.project_id || '')
const approvedCases = computed(() => cases.value.filter((item) => item.status === 'approved'
  && !['functional', 'security'].includes(item.type)
  && (!suiteProjectId.value || item.project_id === suiteProjectId.value)))
const approvedCaseIds = computed(() => new Set(approvedCases.value.map((item) => item.id)))
const availableCases = computed(() => approvedCases.value.filter((item) => !selected.value.includes(item.id)))
const canEnqueue = computed(() => selected.value.length > 0 && selected.value.every((id) => approvedCaseIds.value.has(id)))
const projectQuery = computed(() => props.projectId ? `?${new URLSearchParams({ project_id: props.projectId })}` : '')
const suitePath = computed(() => `/api/suites/${encodeURIComponent(suiteId.value)}`)
const runPath = computed(() => `${suitePath.value}/run`)
const schedulePath = computed(() => `${suitePath.value}/schedule`)

async function request(action) {
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (err) {
    error.value = err.message
  } finally {
    busy.value = false
  }
}

function createSuite() {
  return request(async () => {
    const suite = await api.post('/api/suites', {
      name: name.value,
      ...(props.projectId && { project_id: props.projectId }),
    })
    suites.value.unshift(suite)
    name.value = ''
    await selectSuite(suite)
  })
}

async function loadHistory() {
  history.value = await api.get(`/api/suites/${encodeURIComponent(suiteId.value)}/runs`)
}

async function selectSuite(suite) {
  suiteId.value = suite.id
  maxConcurrency.value = suite.max_concurrency
  selected.value = [...suite.case_ids]
  caseId.value = ''
  queuedId.value = ''
  batchId.value = ''
  summary.value = null
  history.value = []
  schedule.value = null
  scheduleForm.value = null
  orderNotice.value = ''
  const [, response] = await Promise.all([loadHistory(), api.get(schedulePath.value)])
  applySchedule(response.schedule)
}

function saveConcurrency() {
  return request(async () => {
    const saved = await api.patch(suitePath.value, { max_concurrency: maxConcurrency.value })
    const suite = suites.value.find((suite) => suite.id === saved.id)
    suite.max_concurrency = saved.max_concurrency
  })
}

function applySchedule(current) {
  schedule.value = current
  scheduleForm.value = current
    ? { enabled: current.enabled, weekday: current.weekday, time: `${String(current.hour).padStart(2, '0')}:${String(current.minute).padStart(2, '0')}` }
    : { enabled: true, weekday: 1, time: '09:00' }
}

function saveSchedule() {
  return request(async () => {
    const { enabled, weekday, time } = scheduleForm.value
    const [hour, minute] = time.split(':').map(Number)
    applySchedule(await api.put(schedulePath.value, { enabled, weekday, hour, minute }))
  })
}

function deleteSchedule() {
  return request(async () => {
    await api.delete(schedulePath.value)
    applySchedule(null)
  })
}

function addCase() {
  selected.value.push(caseId.value)
  caseId.value = ''
}

function moveCase(index, offset) {
  const position = index + offset
  const [item] = selected.value.splice(index, 1)
  selected.value.splice(position, 0, item)
  orderNotice.value = `${casesById.value.get(item)?.name || item}已移到第 ${position + 1} 位`
}

async function loadSummary(id) {
  batchId.value = id
  summary.value = null
  const result = await api.get(`/api/suite-runs/${encodeURIComponent(id)}`)
  if (props.projectId && !result.runs.every((run) => casesById.value.has(run.case_id))) {
    throw new Error('此批次包含当前项目之外的用例')
  }
  summary.value = result
}

function enqueue() {
  return request(async () => {
    const caseIds = [...selected.value]
    const batch = await api.post(runPath.value, { case_ids: caseIds })
    if (!mounted) return
    activeSuite.value.case_ids = caseIds
    queuedId.value = batch.id
    history.value.unshift(batch)
    await loadSummary(batch.id)
  })
}

onMounted(() => request(async () => {
  const [suiteList, caseList] = await Promise.all([
    api.get(`/api/suites${projectQuery.value}`),
    api.get(`/api/cases${projectQuery.value}`),
  ])
  suites.value = suiteList
  cases.value = caseList
  if (suiteList.length) await selectSuite(suiteList[0])
}))
onUnmounted(() => { mounted = false })
</script>

<template>
  <p v-if="error" class="error" role="alert">{{ error }}</p>
  <form class="card" aria-labelledby="suite-create" action="/api/suites" method="post" @submit.prevent="createSuite">
    <h3 id="suite-create">创建套件</h3>
    <label for="suite-name">套件名称</label>
    <div class="actions">
      <input id="suite-name" v-model.trim="name" name="name" required pattern=".*\S.*" :disabled="busy" />
      <button :class="{ primary: !suites.length }" type="submit" :disabled="busy">创建套件</button>
    </div>
    <p v-if="!suites.length && !busy && !error" class="hint">{{ projectId ? '当前项目暂无套件。' : '暂无套件。' }}创建后可组合已定稿用例，批量执行或按周调度。</p>
  </form>

  <form v-if="suites.length" class="card" aria-labelledby="suite-run" :action="runPath" method="post" @submit.prevent="enqueue">
    <h3 id="suite-run">套件用例</h3>
    <label for="suite-id">套件</label>
    <select id="suite-id" :value="suiteId" name="suite_id" required :disabled="busy || !suites.length"
      @change="request(() => selectSuite(suites.find((suite) => suite.id === $event.target.value)))">
      <option v-for="suite in suites" :key="suite.id" :value="suite.id">{{ suite.name }} · {{ suite.case_ids.length }} 个用例</option>
    </select>
    <label v-if="availableCases.length" for="suite-case">已定稿用例</label>
    <div v-if="availableCases.length" class="actions">
      <select id="suite-case" v-model="caseId" name="case_id" :disabled="busy">
        <option disabled value="">选择用例</option>
        <option v-for="item in availableCases" :key="item.id" :value="item.id">{{ item.name }} · {{ item.type }}</option>
      </select>
      <button type="button" :disabled="busy || !caseId" @click="addCase">添加用例</button>
    </div>
    <div v-if="!approvedCases.length" class="actions">
      <p class="hint">暂无已定稿用例。</p>
      <button type="button" @click="emit('open-project-view', { projectId: suiteProjectId, view: 'cases' })">前往用例</button>
    </div>
    <ol v-if="selected.length" aria-label="入队顺序">
      <li v-for="(id, index) in selected" :key="id">
        <div class="case-row">
          <span>{{ casesById.get(id)?.name || id }} <small v-if="!approvedCaseIds.has(id)" class="error">不可入队</small></span>
          <div class="actions">
            <button type="button" :aria-label="`上移 ${casesById.get(id)?.name || id}`" :disabled="busy" :aria-disabled="index === 0" @click="index > 0 && moveCase(index, -1)">上移</button>
            <button type="button" :aria-label="`下移 ${casesById.get(id)?.name || id}`" :disabled="busy" :aria-disabled="index === selected.length - 1" @click="index < selected.length - 1 && moveCase(index, 1)">下移</button>
            <button type="button" :aria-label="`移除 ${casesById.get(id)?.name || id}`" :disabled="busy" @click="selected.splice(index, 1)">移除</button>
          </div>
        </div>
      </li>
    </ol>
    <p class="order-notice" role="status">{{ orderNotice }}</p>
    <p v-if="selected.length && !canEnqueue" class="hint">移除不可入队的用例后再提交。</p>
    <p v-else-if="!selected.length && approvedCases.length" class="hint">添加至少一个用例后即可入队。</p>
    <p v-else-if="selected.length" class="hint">用例按列表顺序入队，按已保存的并发上限执行。</p>
    <div class="actions submit">
      <button class="primary" type="submit" :disabled="busy || !suiteId || !canEnqueue">保存用例并入队</button>
      <button v-if="queuedId" type="button" @click="emit('open-project-view', { projectId: suiteProjectId, view: 'runs' })">查看执行记录</button>
    </div>
    <p v-if="queuedId" role="status">已入队，批次 ID {{ queuedId }}</p>
  </form>

  <div v-if="activeSuite" class="suite-settings">
    <form class="card" aria-labelledby="suite-concurrency-label" :action="suitePath" method="post" @submit.prevent="saveConcurrency">
      <h3 id="suite-concurrency-label">执行并发</h3>
      <label for="suite-concurrency">批次并发上限</label>
      <div class="actions">
        <input id="suite-concurrency" v-model.number="maxConcurrency" name="max_concurrency" type="number" min="1" step="1" required :disabled="busy" />
        <button type="submit" :disabled="busy || activeSuite.max_concurrency === maxConcurrency">保存并发</button>
      </div>
      <p class="hint">新批次最多同时运行 {{ activeSuite.max_concurrency }} 个用例。</p>
    </form>

    <form v-if="scheduleForm" class="card" aria-labelledby="suite-schedule" :action="schedulePath" method="post" @submit.prevent="saveSchedule">
      <h3 id="suite-schedule">每周计划</h3>
      <div class="actions">
        <div>
          <label for="suite-weekday">星期</label>
          <select id="suite-weekday" v-model.number="scheduleForm.weekday" name="weekday" :disabled="busy">
            <option v-for="(weekday, index) in weekdays" :key="weekday" :value="index">{{ weekday }}</option>
          </select>
        </div>
        <div>
          <label for="suite-schedule-time">开始时间（服务器时间）</label>
          <input id="suite-schedule-time" v-model="scheduleForm.time" name="time" type="time" required :disabled="busy" />
        </div>
      </div>
      <div class="schedule-switch">
        <input id="suite-schedule-enabled" v-model="scheduleForm.enabled" name="enabled" type="checkbox" :disabled="busy" />
        <label for="suite-schedule-enabled">启用</label>
      </div>
      <div class="actions">
        <button type="submit" :disabled="busy">保存计划</button>
        <button v-if="schedule" type="button" :disabled="busy" @click="deleteSchedule">删除计划</button>
      </div>
      <p role="status">
        <template v-if="schedule?.next_run_at">
          下次触发：<time :datetime="schedule.next_run_at">{{ schedule.next_run_at.replace('T', ' ') }}</time>
        </template>
        <template v-else>{{ schedule ? '计划已停用' : '未设置周计划' }}</template>
      </p>
      <p class="hint">{{ activeSuite.case_ids.length ? '计划使用最近一次「保存用例并入队」提交的用例和顺序。' : '先保存用例并入队，周计划才有用例可执行。' }}</p>
    </form>
  </div>

  <section v-if="suiteId || !projectId" class="card" aria-labelledby="suite-results">
    <h3 id="suite-results">批次结果</h3>
    <ul v-if="history.length" aria-label="运行历史">
      <li v-for="batch in history" :key="batch.id">
        <StatusTag v-if="batch.status !== 'needs_review'" :value="batch.status" />
        <small v-else>需复核</small>
        <button type="button" :disabled="busy" @click="request(() => loadSummary(batch.id))">查看批次 {{ batch.id }}</button>
      </li>
    </ul>
    <p v-else-if="suiteId && !busy" class="hint">此套件暂无批次。保存用例并入队后，结果会显示在这里。</p>
    <form action="/api/suite-runs/" aria-labelledby="suite-results" @submit.prevent="request(() => Promise.all([loadSummary(batchId), suiteId && loadHistory()]))">
      <label for="suite-batch">批次 ID</label>
      <div class="actions">
        <input id="suite-batch" v-model.trim="batchId" name="id" required pattern=".*\S.*" :disabled="busy" />
        <button type="submit" :disabled="busy">查询 / 刷新</button>
      </div>
    </form>
    <template v-if="summary">
      <p role="status">批次 {{ summary.id }} · 并发上限 {{ summary.max_concurrency }} · 共 {{ summary.total }} · 通过 {{ summary.passed }} · 失败 {{ summary.failed }} · 需复核 {{ summary.needs_review }}</p>
      <DataTable :rows="summary.runs" :columns="[
        { key: 'case_id', label: '用例' },
        { key: 'status', label: '状态' },
        { key: 'passed', label: '通过' },
        { key: 'needs_review', label: '需复核' },
      ]">
        <template #case_id="{ row }">{{ casesById.get(row.case_id)?.name || row.case_id }}</template>
        <template #status="{ row }"><StatusTag v-if="row.status !== 'needs_review'" :value="row.status" /><span v-else>需复核</span></template>
        <template #passed="{ row }">{{ row.passed == null ? '—' : row.passed ? '是' : '否' }}</template>
        <template #needs_review="{ row }">{{ row.needs_review ? '是' : '否' }}</template>
      </DataTable>
    </template>
  </section>
  <p v-if="busy" role="status">处理中…</p>
</template>

<style scoped>
.card { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 14px; margin-bottom: 14px; }
h3 { font-size: 15px; margin: 0 0 12px; }
label { display: block; margin: 0 0 4px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.hint { margin: 10px 0 0; color: var(--muted); font-size: 13px; }
.actions .hint { margin: 0; }
.suite-settings { display: grid; gap: 0 14px; grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr)); }
.case-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.order-notice { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
input, select { width: min(100%, 340px); }
select { font: inherit; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 7px 10px; }
#suite-id { margin-bottom: 12px; }
input:focus-visible, select:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
li { margin-bottom: 6px; }
.submit { margin-top: 12px; }
.schedule-switch { display: flex; align-items: center; gap: 6px; margin-block: 12px; }
.schedule-switch input { width: auto; accent-color: var(--accent); }
.schedule-switch label { margin: 0; }
button[aria-disabled='true'] { opacity: .55; cursor: not-allowed; }
.error { color: var(--bad); }
</style>
