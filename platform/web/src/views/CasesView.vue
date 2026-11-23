<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'
import CaseGenerationPanel from '../components/CaseGenerationPanel.vue'

const props = defineProps({ generationTarget: { type: Object, default: null } })
const emit = defineEmits(['open-run'])
const rows = ref([])
const projects = ref([])
const requirements = ref([])
const environments = ref([])
const versions = ref([])
const datasets = ref([])
const dataRecords = ref([])
const projectId = ref(props.generationTarget?.projectId || '')
const runVersion = ref(props.generationTarget?.versionId || '')
const showGeneration = ref(!!props.generationTarget)
const generationTarget = ref(props.generationTarget)
const statusFilter = ref('')
const form = ref({ name: '', type: 'ui', scenario: '', requirement_id: '', environment_id: '', expected_source: 'requirement', dataset_id: '', data_row_id: '' })
const error = ref('')
const runSubmitting = ref('')
const editing = ref(null)
const structured = ref(false)
const steps = ref([])
const scheduleEditing = ref(null)
const scheduleExists = ref(false)
const scheduleForm = ref({ enabled: true, weekday: 1, time: '09:00' })

const columns = [
  { key: 'name', label: '用例', width: '20%' },
  { key: 'type', label: '类型', width: '8%' },
  { key: 'scenario', label: '场景' },
  { key: 'generation_mode', label: '来源', width: '10%' },
  { key: 'version_no', label: '版本', width: '7%' },
  { key: 'status', label: '状态', width: '9%' },
  { key: 'actions', label: '', width: '22%' },
]

const TYPE_LABEL = { ui: 'UI', api: '接口', perf: '性能' }
const MODE_LABEL = { manual: '手工', sampling: 'Agent', task_pack: '任务包', platform_llm: '平台模型' }
const ELEMENT_ACTIONS = ['click', 'fill', 'select', 'check', 'uncheck']
const TEXT_ACTIONS = ['wait_text', 'assert_text']

function encodeSteps(items) {
  return JSON.stringify({ steps: items.map((step) => {
    const { frame, row, ...fields } = step
    return { ...fields, ...(frame?.trim() && { frame: frame.trim() }), ...(row?.trim() && { row: row.trim() }) }
  }) })
}

const filtered = computed(() => {
  return rows.value.filter((r) => {
    if (projectId.value && r.project_id !== projectId.value) return false
    if (statusFilter.value && r.status !== statusFilter.value) return false
    return true
  })
})
const projectDatasets = computed(() => datasets.value.filter((dataset) => dataset.project_id === projectId.value))
const selectedDataFields = computed(() => datasets.value.find((dataset) => dataset.id === form.value.dataset_id)?.fields || [])

async function load() {
  ;[projects.value, requirements.value, environments.value, datasets.value, versions.value] = await Promise.all([
    api.get('/api/projects'),
    api.get('/api/requirements'),
    api.get('/api/environments'),
    api.get('/api/test-data/datasets'),
    api.get('/api/versions'),
  ])
  rows.value = await api.get('/api/cases')
  if (!projectId.value && projects.value.length) projectId.value = projects.value[0].id
}

async function loadDataRecords() {
  form.value.data_row_id = ''
  dataRecords.value = form.value.dataset_id
    ? await api.get(`/api/test-data/datasets/${form.value.dataset_id}/records`)
    : []
}

function changeProject() {
  generationTarget.value = null
  runVersion.value = ''
  form.value.requirement_id = ''
  form.value.environment_id = ''
  form.value.dataset_id = ''
  form.value.data_row_id = ''
  dataRecords.value = []
}

async function importScenario(event) {
  const [file] = event.target.files
  if (!file) return
  error.value = ''
  try {
    const scenario = JSON.parse(await file.text())
    if (!Array.isArray(scenario?.steps) || !scenario.steps.length || scenario.steps.some((step) => typeof step?.action !== 'string')) {
      throw new Error('文件必须包含结构化 UI 步骤')
    }
    form.value.type = 'ui'
    form.value.name ||= file.name.replace(/\.json$/i, '')
    structured.value = true
    steps.value = scenario.steps
  } catch (problem) {
    error.value = problem.message
  } finally {
    event.target.value = ''
  }
}

async function add() {
  const scenario = structured.value && form.value.type === 'ui' ? encodeSteps(steps.value) : form.value.scenario
  if (!form.value.name || !scenario) return
  error.value = ''
  try {
    await api.post('/api/cases', { ...form.value, scenario, project_id: projectId.value })
    form.value = { name: '', type: 'ui', scenario: '', requirement_id: '', environment_id: '', expected_source: 'requirement', dataset_id: '', data_row_id: '' }
    dataRecords.value = []
    structured.value = false
    steps.value = []
    await load()
  } catch (e) { error.value = e.message }
}

function toggleStructured(target) {
  if (target === 'new') {
    structured.value = !structured.value
    if (structured.value) steps.value = []
    return
  }
  editing.value.structured = !editing.value.structured
  if (editing.value.structured) {
    try {
      const scenario = JSON.parse(editing.value.scenario)
      editing.value.steps = Array.isArray(scenario.steps) ? scenario.steps : []
    } catch {
      editing.value.steps = []
    }
  }
}

function setEditScenarioMode(event) {
  if (editing.value.structured === (event.target.value === 'true')) return
  toggleStructured('edit')
}

function addStep(items) {
  items.push({ action: 'navigate', url: '' })
}

function removeStep(index) {
  steps.value.splice(index, 1)
}

async function saveEdit() {
  const scenario = editing.value.structured ? encodeSteps(editing.value.steps) : editing.value.scenario
  if (!scenario.trim()) return
  error.value = ''
  try {
    await api.patch(`/api/cases/${editing.value.id}`, { scenario })
    editing.value = null
    await load()
  } catch (e) { error.value = e.message }
}

async function review(row, decision) {
  error.value = ''
  const body = { decision, reviewer: '当前用户' }
  if (decision === 'reject') body.scenario = row.scenario
  try {
    await api.post(`/api/cases/${row.id}/review`, body)
    await load()
  } catch (e) { error.value = e.message }
}

async function run(row) {
  error.value = ''
  runSubmitting.value = row.id
  try {
    const { run_id } = await api.post(`/api/cases/${row.id}/run`, { version_id: runVersion.value })
    emit('open-run', run_id)
  } catch (e) { error.value = e.message }
  finally { runSubmitting.value = '' }
}

async function openSchedule(row) {
  error.value = ''
  try {
    const response = await api.get(`/api/cases/${row.id}/schedule`)
    const current = response.schedule
    scheduleExists.value = current !== null
    scheduleForm.value = current
      ? { enabled: current.enabled, weekday: current.weekday, time: `${String(current.hour).padStart(2, '0')}:${String(current.minute).padStart(2, '0')}` }
      : { enabled: true, weekday: 1, time: '09:00' }
    scheduleEditing.value = row
  } catch (e) { error.value = e.message }
}

async function saveSchedule() {
  const [hour, minute] = scheduleForm.value.time.split(':').map(Number)
  try {
    await api.put(`/api/cases/${scheduleEditing.value.id}/schedule`, {
      enabled: scheduleForm.value.enabled,
      weekday: scheduleForm.value.weekday,
      hour,
      minute,
    })
    scheduleExists.value = true
    scheduleEditing.value = null
  } catch (e) { error.value = e.message }
}

async function deleteSchedule() {
  try {
    await api.delete(`/api/cases/${scheduleEditing.value.id}/schedule`)
    scheduleExists.value = false
    scheduleEditing.value = null
  } catch (e) { error.value = e.message }
}

function openEdit(row) {
  editing.value = { ...row, structured: false, steps: [] }
  if (isStructuredUI(row)) toggleStructured('edit')
}

function isStructuredUI(row) {
  if (row.type !== 'ui') return false
  try { return Array.isArray(JSON.parse(row.scenario)?.steps) } catch { return false }
}

onMounted(load)
</script>

<template>
  <header class="head">
    <div class="form">
      <select v-model="projectId" @change="changeProject">
        <option value="">全部项目</option>
        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <button type="button" :disabled="!projectId" :aria-expanded="showGeneration" @click="showGeneration = !showGeneration">按需求生成用例</button>
      <select v-model="statusFilter">
        <option value="">全部状态</option>
        <option value="draft">草稿</option>
        <option value="in_review">待审核</option>
        <option value="approved">已定稿</option>
      </select>
      <input v-model="form.name" placeholder="用例名" class="name" />
      <select v-model="form.type">
        <option value="ui">UI</option>
        <option value="api">接口</option>
        <option value="perf">性能</option>
      </select>
      <input v-if="!structured || form.type !== 'ui'" v-model="form.scenario" aria-label="场景描述" placeholder="场景描述" class="scenario" />
      <button v-if="form.type === 'ui'" type="button" @click="toggleStructured('new')">{{ structured ? '自然语言场景' : '结构化步骤' }}</button>
      <label v-if="form.type === 'ui'">导入录制脚本<input type="file" name="scenario-file" accept=".json,application/json" @change="importScenario" /></label>
      <select v-model="form.requirement_id" title="关联需求">
        <option value="">无关联需求</option>
        <option v-for="r in requirements.filter((item) => item.project_id === projectId)" :key="r.id" :value="r.id">{{ r.code || r.title }}</option>
      </select>
      <select v-model="form.environment_id" aria-label="测试环境">
        <option value="">项目默认环境</option>
        <option v-for="environment in environments.filter((item) => item.project_id === projectId)" :key="environment.id" :value="environment.id">{{ environment.name }}</option>
      </select>
      <select v-model="form.dataset_id" aria-label="绑定测试数据集" @change="loadDataRecords">
        <option value="">不绑定数据集</option>
        <option v-for="dataset in projectDatasets" :key="dataset.id" :value="dataset.id">{{ dataset.name }}</option>
      </select>
      <select v-if="form.dataset_id" v-model="form.data_row_id" aria-label="绑定数据行">
        <option value="">运行时选择数据行</option>
        <option v-for="record in dataRecords" :key="record.id" :value="record.id">第 {{ record.row_no }} 行</option>
      </select>
      <button class="primary" @click="add">新增</button>
    </div>
    <section v-if="structured && form.type === 'ui'" class="step-editor" aria-label="新用例步骤">
      <p v-if="selectedDataFields.length" class="muted">可用数据字段 {{ selectedDataFields.map((field) => field.name).join('、') }}</p>
      <div v-for="(step, index) in steps" :key="index" class="step-row">
        <label>动作
          <select v-model="step.action" :name="`new-step-${index}-action`">
            <option value="navigate">打开页面</option><option value="click">点击</option>
            <option value="fill">填写</option><option value="select">选择下拉选项</option>
            <option value="check">勾选</option><option value="uncheck">取消勾选</option><option value="wait_text">等待文本</option>
            <option value="assert_text">断言文本</option><option value="assert_url">断言地址</option>
          </select>
        </label>
        <label v-if="step.action === 'navigate'">URL<input v-model="step.url" :name="`new-step-${index}-url`" required /></label>
        <template v-if="ELEMENT_ACTIONS.includes(step.action)">
          <label>角色<input v-model="step.role" :name="`new-step-${index}-role`" required placeholder="button / textbox" /></label>
          <label>名称<input v-model="step.name" :name="`new-step-${index}-name`" required /></label>
        </template>
        <label v-if="['fill', 'select'].includes(step.action)">数据字段<input v-model="step.data_key" :name="`new-step-${index}-data-key`" required /></label>
        <label v-if="TEXT_ACTIONS.includes(step.action)">文本<input v-model="step.text" :name="`new-step-${index}-text`" required /></label>
        <template v-if="ELEMENT_ACTIONS.includes(step.action) || TEXT_ACTIONS.includes(step.action)">
          <label>iframe 测试 ID<input v-model="step.frame" :name="`new-step-${index}-frame`" /></label>
          <label>目标行文本<input v-model="step.row" :name="`new-step-${index}-row`" /></label>
        </template>
        <template v-if="step.action === 'assert_url'">
          <label>URL<input v-model="step.expected" :name="`new-step-${index}-url`" required /></label>
          <label>匹配方式<select v-model="step.match" :name="`new-step-${index}-match`"><option value="contains">包含</option><option value="equals">完全一致</option></select></label>
        </template>
        <button type="button" @click="removeStep(index)">移除</button>
      </div>
      <button type="button" @click="addStep(steps)">添加步骤</button>
    </section>
    <p class="hint">草稿 → 待审核 → 已定稿；只有已定稿才用于正式回归</p>
    <label v-if="projectId">执行版本
      <select v-model="runVersion" name="run-version">
        <option value="">不关联版本</option>
        <option v-for="version in versions.filter((item) => item.project_id === projectId)" :key="version.id" :value="version.id">{{ version.name }}</option>
      </select>
    </label>
    <CaseGenerationPanel
      v-if="showGeneration && projectId && requirements.length"
      :key="projectId"
      :project-id="projectId"
      :requirements="requirements"
      :initial-requirement-id="generationTarget?.requirementId"
      :initial-version-id="generationTarget?.versionId"
      @generated="load"
    />
    <p v-else-if="showGeneration && projectId" class="hint">先在「需求」中添加或导入项目需求。</p>
  </header>
  <p v-if="error" class="error" role="alert">{{ error }}</p>

  <DataTable :rows="filtered" :columns="columns" empty="还没有用例">
    <template #type="{ row }"><span class="mono">{{ TYPE_LABEL[row.type] || row.type }}</span></template>
    <template #generation_mode="{ row }"><span class="muted">{{ MODE_LABEL[row.generation_mode] || row.generation_mode }}</span></template>
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #actions="{ row }">
      <div class="actions">
        <button v-if="row.status === 'draft'" @click="review(row, 'submit_review')">提交审核</button>
        <template v-if="row.status === 'in_review'">
          <button class="primary" @click="review(row, 'approve')">通过</button>
          <button @click="review(row, 'reject')">退回</button>
        </template>
        <button v-if="row.status === 'approved'" :disabled="!!runSubmitting" @click="run(row)">{{ isStructuredUI(row) ? '回放' : '执行' }}</button>
        <button v-if="row.status === 'approved'" @click="openSchedule(row)">计划</button>
        <button @click="openEdit(row)">查看</button>
      </div>
    </template>
  </DataTable>

  <div v-if="editing" class="drawer" @click.self="editing = null">
    <form class="panel" @submit.prevent="saveEdit">
      <h3>{{ editing.name }}</h3>
      <dl>
        <dt>类型</dt><dd>{{ TYPE_LABEL[editing.type] }}</dd>
        <dt>期望来源</dt><dd>{{ editing.expected_source }}</dd>
        <dt>生成方式</dt><dd>{{ MODE_LABEL[editing.generation_mode] }}</dd>
        <dt>版本</dt><dd>v{{ editing.version_no }}</dd>
        <dt>审核人</dt><dd>{{ editing.reviewer || '—' }}</dd>
      </dl>
      <label v-if="editing.type === 'ui'" class="scenario-mode">场景格式
        <select :value="editing.structured" @change="setEditScenarioMode"><option :value="false">自然语言</option><option :value="true">结构化步骤</option></select>
      </label>
      <textarea v-if="!editing.structured" v-model="editing.scenario" name="scenario" aria-label="场景描述" required rows="7" />
      <section v-else class="step-editor" aria-label="编辑用例步骤">
        <div v-for="(step, index) in editing.steps" :key="index" class="step-row">
          <label>动作
            <select v-model="step.action" :name="`edit-step-${index}-action`">
              <option value="navigate">打开页面</option><option value="click">点击</option>
              <option value="fill">填写</option><option value="select">选择下拉选项</option>
              <option value="check">勾选</option><option value="uncheck">取消勾选</option><option value="wait_text">等待文本</option>
              <option value="assert_text">断言文本</option><option value="assert_url">断言地址</option>
            </select>
          </label>
          <label v-if="step.action === 'navigate'">URL<input v-model="step.url" :name="`edit-step-${index}-url`" required /></label>
          <template v-if="ELEMENT_ACTIONS.includes(step.action)">
            <label>角色<input v-model="step.role" :name="`edit-step-${index}-role`" required placeholder="button / textbox" /></label>
            <label>名称<input v-model="step.name" :name="`edit-step-${index}-name`" required /></label>
          </template>
          <label v-if="['fill', 'select'].includes(step.action)">数据字段<input v-model="step.data_key" :name="`edit-step-${index}-data-key`" required /></label>
          <label v-if="TEXT_ACTIONS.includes(step.action)">文本<input v-model="step.text" :name="`edit-step-${index}-text`" required /></label>
          <template v-if="ELEMENT_ACTIONS.includes(step.action) || TEXT_ACTIONS.includes(step.action)">
            <label>iframe 测试 ID<input v-model="step.frame" :name="`edit-step-${index}-frame`" /></label>
            <label>目标行文本<input v-model="step.row" :name="`edit-step-${index}-row`" /></label>
          </template>
          <template v-if="step.action === 'assert_url'">
            <label>URL<input v-model="step.expected" :name="`edit-step-${index}-url`" required /></label>
            <label>匹配方式<select v-model="step.match" :name="`edit-step-${index}-match`"><option value="contains">包含</option><option value="equals">完全一致</option></select></label>
          </template>
          <button v-if="editing.status === 'draft'" type="button" @click="editing.steps.splice(index, 1)">移除</button>
        </div>
        <button v-if="editing.status === 'draft'" type="button" @click="addStep(editing.steps)">添加步骤</button>
      </section>
      <div class="schedule-actions"><button type="button" @click="editing = null">关闭</button><button v-if="editing.status === 'draft'" class="primary" type="submit">保存场景</button></div>
    </form>
  </div>

  <div v-if="scheduleEditing" class="drawer" @click.self="scheduleEditing = null">
    <form class="panel" @submit.prevent="saveSchedule">
      <h3>每周计划 · {{ scheduleEditing.name }}</h3>
      <label class="schedule-field">
        <span>星期</span>
        <select v-model.number="scheduleForm.weekday">
          <option :value="0">星期日</option><option :value="1">星期一</option>
          <option :value="2">星期二</option><option :value="3">星期三</option>
          <option :value="4">星期四</option><option :value="5">星期五</option>
          <option :value="6">星期六</option>
        </select>
      </label>
      <label class="schedule-field">
        <span>开始时间</span>
        <input v-model="scheduleForm.time" type="time" required />
      </label>
      <label class="schedule-switch">
        <input v-model="scheduleForm.enabled" type="checkbox" />
        <span>启用</span>
      </label>
      <div class="schedule-actions">
        <button v-if="scheduleExists" type="button" @click="deleteSchedule">删除计划</button>
        <button type="button" @click="scheduleEditing = null">取消</button>
        <button class="primary" type="submit">保存</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.head { margin-bottom: 6px; }
.form { display: flex; gap: 8px; flex-wrap: wrap; }
select, input { font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.name { width: 150px; }
.scenario { flex: 1; min-width: 180px; }
.step-editor { display: grid; gap: 8px; margin: 10px 0; }
.step-row { display: flex; align-items: end; flex-wrap: wrap; gap: 8px; padding: 10px; border: 1px solid var(--line); border-radius: 6px; }
.step-row label, .scenario-mode { display: grid; gap: 4px; color: var(--muted); font-size: 12px; }
.step-row input, .step-row select, .scenario-mode select, .panel textarea { min-width: 130px; font: inherit; padding: 7px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); color: inherit; }
.panel textarea { width: 100%; resize: vertical; margin: 10px 0; }
.hint { color: var(--muted); font-size: 12px; margin: 8px 0 14px; }
.actions { display: flex; gap: 6px; }
.actions button { padding: 3px 9px; font-size: 12px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }
.muted { color: var(--muted); }
.error { color: var(--bad); }
.schedule-field { display: flex; flex-direction: column; gap: 6px; margin-bottom: 16px; font-size: 13px; }
.schedule-field > span { color: var(--muted); }
.schedule-field select, .schedule-field input { font: inherit; padding: 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.schedule-switch { display: flex; align-items: center; gap: 8px; margin: 4px 0 22px; }
.schedule-actions { display: flex; justify-content: flex-end; gap: 8px; }
.drawer { position: fixed; inset: 0; background: rgba(20,20,24,0.28); display: flex; justify-content: flex-end; }
.panel { width: 380px; background: var(--panel); padding: 22px; overflow: auto; }
.panel h3 { margin: 0 0 16px; font-size: 15px; }
dl { display: grid; grid-template-columns: 84px 1fr; gap: 10px 12px; font-size: 13px; margin: 0 0 22px; }
dt { color: var(--muted); }
dd { margin: 0; word-break: break-word; }
</style>
