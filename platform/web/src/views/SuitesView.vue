<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

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
const weekdays = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六']
const error = ref('')
const busy = ref(false)
let mounted = true

const caseNames = computed(() => new Map(cases.value.map((item) => [item.id, item.name])))
const availableCases = computed(() => cases.value.filter((item) => !selected.value.includes(item.id)))
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
    const suite = await api.post('/api/suites', { name: name.value })
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
  scheduleForm.value = null
  await loadHistory()
  const response = await api.get(schedulePath.value)
  applySchedule(response.schedule)
}

function saveConcurrency() {
  return request(async () => {
    const saved = await api.patch(suitePath.value, { max_concurrency: maxConcurrency.value })
    suites.value.find((suite) => suite.id === saved.id).max_concurrency = saved.max_concurrency
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

function moveUp(index) {
  const [item] = selected.value.splice(index, 1)
  selected.value.splice(index - 1, 0, item)
}

async function loadSummary(id) {
  batchId.value = id
  summary.value = null
  summary.value = await api.get(`/api/suite-runs/${encodeURIComponent(id)}`)
}

function enqueue() {
  return request(async () => {
    const caseIds = [...selected.value]
    const batch = await api.post(runPath.value, { case_ids: caseIds })
    if (!mounted) return
    suites.value.find((suite) => suite.id === suiteId.value).case_ids = caseIds
    queuedId.value = batch.id
    history.value.unshift(batch)
    await loadSummary(batch.id)
  })
}

onMounted(() => request(async () => {
  const [suiteList, caseList] = await Promise.all([
    api.get('/api/suites'),
    api.get('/api/cases?status=approved'),
  ])
  suites.value = suiteList
  cases.value = caseList
}))
onUnmounted(() => { mounted = false })
</script>

<template>
  <form class="card" aria-labelledby="suite-create" action="/api/suites" method="post" @submit.prevent="createSuite">
    <h3 id="suite-create">创建套件</h3>
    <label for="suite-name">套件名称（必填）</label>
    <div class="actions">
      <input id="suite-name" v-model.trim="name" name="name" required pattern=".*\S.*" :disabled="busy" />
      <button class="primary" type="submit" :disabled="busy">创建</button>
    </div>
  </form>

  <form class="card" aria-labelledby="suite-run" :action="runPath" method="post" @submit.prevent="enqueue">
    <h3 id="suite-run">套件入队</h3>
    <label for="suite-id">套件（必选）</label>
    <select id="suite-id" :value="suiteId" name="suite_id" required :disabled="busy || !suites.length"
      @change="request(() => selectSuite(suites.find((suite) => suite.id === $event.target.value)))">
      <option disabled value="">{{ suites.length ? '选择套件' : '暂无套件' }}</option>
      <option v-for="suite in suites" :key="suite.id" :value="suite.id">{{ suite.name }}</option>
    </select>
    <label for="suite-case">已定稿用例</label>
    <div class="actions">
      <select id="suite-case" v-model="caseId" name="case_id" :disabled="busy || !availableCases.length">
        <option disabled value="">{{ availableCases.length ? '选择用例' : '没有可添加的用例' }}</option>
        <option v-for="item in availableCases" :key="item.id" :value="item.id">{{ item.name }} · {{ item.type }}</option>
      </select>
      <button type="button" :disabled="busy || !caseId" @click="addCase">添加</button>
    </div>
    <ol v-if="selected.length" aria-label="入队顺序">
      <li v-for="(id, index) in selected" :key="index">
        {{ caseNames.get(id) || id }}
        <button type="button" :aria-label="`上移 ${caseNames.get(id) || id}`" :disabled="busy || index === 0" @click="moveUp(index)">上移</button>
        <button type="button" :aria-label="`移除 ${caseNames.get(id) || id}`" :disabled="busy" @click="selected.splice(index, 1)">移除</button>
      </li>
    </ol>
    <button class="primary submit" type="submit" :disabled="busy || !suiteId || !selected.length">按顺序入队</button>
    <p v-if="queuedId" role="status">已入队，批次 ID {{ queuedId }}</p>
  </form>

  <form v-if="suiteId" class="card" aria-labelledby="suite-concurrency-label" :action="suitePath" method="post" @submit.prevent="saveConcurrency">
    <label id="suite-concurrency-label" for="suite-concurrency">批次并发上限</label>
    <div class="actions">
      <input id="suite-concurrency" v-model.number="maxConcurrency" name="max_concurrency" type="number" min="1" step="1" required :disabled="busy" />
      <button type="submit" :disabled="busy">保存并发上限</button>
    </div>
  </form>

  <form v-if="scheduleForm" class="card" aria-labelledby="suite-schedule" :action="schedulePath" method="post" @submit.prevent="saveSchedule">
    <h3 id="suite-schedule">套件每周计划</h3>
    <div class="actions">
      <div>
        <label for="suite-weekday">星期</label>
        <select id="suite-weekday" v-model.number="scheduleForm.weekday" name="weekday" :disabled="busy">
          <option v-for="(weekday, index) in weekdays" :key="weekday" :value="index">{{ weekday }}</option>
        </select>
      </div>
      <div>
        <label for="suite-schedule-time">开始时间（必填）</label>
        <input id="suite-schedule-time" v-model="scheduleForm.time" name="time" type="time" required :disabled="busy" />
      </div>
    </div>
    <div class="schedule-switch">
      <input id="suite-schedule-enabled" v-model="scheduleForm.enabled" name="enabled" type="checkbox" :disabled="busy" />
      <label for="suite-schedule-enabled">启用</label>
    </div>
    <div class="actions">
      <button class="primary" type="submit" :disabled="busy">保存计划</button>
      <button v-if="schedule" type="button" :disabled="busy" @click="deleteSchedule">删除计划</button>
    </div>
    <p role="status">
      <template v-if="schedule?.next_run_at">
        下次触发：<time :datetime="schedule.next_run_at">{{ schedule.next_run_at.replace('T', ' ') }}</time>
      </template>
      <template v-else>{{ schedule ? '计划已停用' : '未设置周计划' }}</template>
    </p>
  </form>

  <section class="card" aria-labelledby="suite-results">
    <h3 id="suite-results">批次结果</h3>
    <ul v-if="history.length" aria-label="运行历史">
      <li v-for="batch in history" :key="batch.id">
        <StatusTag :value="batch.status" />
        <button type="button" :disabled="busy" @click="request(() => loadSummary(batch.id))">{{ batch.id }}</button>
      </li>
    </ul>
    <form action="/api/suite-runs/" aria-labelledby="suite-results" @submit.prevent="request(() => loadSummary(batchId))">
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
        <template #case_id="{ row }">{{ caseNames.get(row.case_id) || row.case_id }}</template>
        <template #status="{ row }"><StatusTag :value="row.status" /></template>
        <template #passed="{ row }">{{ row.passed == null ? '—' : row.passed ? '是' : '否' }}</template>
        <template #needs_review="{ row }">{{ row.needs_review ? '是' : '否' }}</template>
      </DataTable>
    </template>
  </section>
  <p v-if="busy" role="status">处理中…</p>
  <p v-if="error" class="error" role="alert">{{ error }}</p>
</template>

<style scoped>
.card { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 14px; margin-bottom: 14px; }
h3 { font-size: 15px; margin: 0 0 12px; }
label { display: block; margin: 0 0 4px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
input, select { width: min(100%, 340px); }
select { font: inherit; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 7px 10px; }
#suite-id { margin-bottom: 12px; }
input:focus-visible, select:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
li { margin-bottom: 6px; }
li button { margin-inline-start: 6px; }
.submit { margin-top: 12px; }
.schedule-switch { display: flex; align-items: center; gap: 6px; margin-block: 12px; }
.schedule-switch input { width: auto; accent-color: var(--accent); }
.schedule-switch label { margin: 0; }
button:disabled { opacity: .55; cursor: default; }
.error { color: var(--bad); }
</style>
