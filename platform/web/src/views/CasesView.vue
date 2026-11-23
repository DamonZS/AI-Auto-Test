<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'
import CaseGenerationPanel from '../components/CaseGenerationPanel.vue'
import CaseWorkbookExport from '../components/CaseWorkbookExport.vue'
import CaseWorkbookImport from '../components/CaseWorkbookImport.vue'
import HttpScenarioFields from '../components/HttpScenarioFields.vue'

const props = defineProps({ projectId: { type: String, default: '' }, generationTarget: { type: Object, default: null }, selectedCase: { type: String, default: '' } })
const emit = defineEmits(['open-run'])
const rows = ref([])
const projects = ref([])
const requirements = ref([])
const environments = ref([])
const versions = ref([])
const datasets = ref([])
const dataRecords = ref([])
const projectId = ref(props.generationTarget?.projectId || props.projectId)
const runVersion = ref(props.generationTarget?.versionId || '')
const showGeneration = ref(!!props.generationTarget)
const generationTarget = ref(props.generationTarget)
const statusFilter = ref('')
const typeFilter = ref('')
const query = ref('')
const creating = ref(false)
const form = ref({ name: '', type: 'ui', scenario: '', requirement_id: '', environment_id: '', expected_source: 'requirement', dataset_id: '', data_row_id: '' })
const error = ref('')
const importedTarget = ref('')
const runSubmitting = ref('')
const editing = ref(null)
const returning = ref(null)
const feedback = ref('')
const reviewBusy = ref(false)
const saving = ref(false)
const manualResult = ref({ status: '', report: '', version_id: '' })
const manualSubmitting = ref(false)
const caseHistory = ref([])
const historyError = ref('')
const structured = ref(false)
const steps = ref([])
const functionalSteps = ref([])
const checklist = ref([])
const scheduleEditing = ref(null)
const scheduleExists = ref(false)
const scheduleForm = ref({ enabled: true, weekday: 1, time: '09:00' })
const canRecord = ref(Boolean(window.caseWorkbook?.recordUiCase))

function workbookReady() {
  canRecord.value = Boolean(window.caseWorkbook?.recordUiCase)
}

onMounted(() => window.addEventListener('case-workbook-ready', workbookReady))
onUnmounted(() => window.removeEventListener('case-workbook-ready', workbookReady))

function recordUiCase() {
  window.caseWorkbook.recordUiCase()
}

const columns = [
  { key: 'name', label: '用例', width: '20%' },
  { key: 'type', label: '类型', width: '8%' },
  { key: 'next', label: '下一步' },
  { key: 'generation_mode', label: '来源', width: '10%' },
  { key: 'version_no', label: '版本', width: '7%' },
  { key: 'status', label: '状态', width: '9%' },
  { key: 'actions', label: '', width: '22%' },
]

const TYPE_LABEL = { functional: '手工功能', ui: 'UI', api: '接口', perf: '性能', security: '防御性安全审查' }
const MODE_LABEL = { manual: '手工', sampling: 'Agent', task_pack: '任务包', platform_llm: '平台模型' }
const CHECKLIST = {
  authentication: '核对文档是否明确认证与权限要求',
  sensitive_data: '核对文档是否明确敏感数据的访问、存储和脱敏规则',
  configuration: '核对文档是否明确可配置项的默认值、管理权限和变更规则',
}
const ELEMENT_ACTIONS = ['click', 'fill', 'select', 'check', 'uncheck']
const TEXT_ACTIONS = ['wait_text', 'assert_text']

function encodeSteps(items) {
  return JSON.stringify({ steps: items.map((step) => {
    const { frame, row, ...fields } = step
    return { ...fields, ...(frame?.trim() && { frame: frame.trim() }), ...(row?.trim() && { row: row.trim() }) }
  }) })
}

function isExecutable(row) {
  return ['ui', 'api', 'perf'].includes(row.type)
}

function acceptsManualResult(row) {
  return row.status === 'approved' && (row.type === 'functional' || row.type === 'security')
}

function hasCurrentFeedback(row) {
  return row.status === 'draft' && !!row.review_feedback && row.review_feedback_version === row.version_no
}

function nextAction(row) {
  if (hasCurrentFeedback(row)) return '按反馈修订后重新提交'
  if (row.status === 'draft') return '核对内容后提交审核'
  if (row.status === 'in_review') return '审核内容，定稿或填写退回意见'
  if (row.status === 'approved') return isExecutable(row) ? '可执行或设置计划' : '完成人工验证后登记结果'
  return '—'
}

const filtered = computed(() => {
  return rows.value.filter((r) => {
    if (projectId.value && r.project_id !== projectId.value) return false
    const status = hasCurrentFeedback(r) ? 'returned' : r.status
    if (statusFilter.value && status !== statusFilter.value) return false
    if (typeFilter.value && r.type !== typeFilter.value) return false
    if (query.value && !r.name.toLowerCase().includes(query.value.toLowerCase())) return false
    return true
  })
})
const projectDatasets = computed(() => datasets.value.filter((dataset) => dataset.project_id === projectId.value))
const selectedDataFields = computed(() => datasets.value.find((dataset) => dataset.id === form.value.dataset_id)?.fields || [])
const projectRequirements = computed(() => requirements.value.filter((item) => item.project_id === projectId.value))

async function load() {
  ;[projects.value, requirements.value, environments.value, datasets.value, versions.value] = await Promise.all([
    api.get('/api/projects'),
    api.get('/api/requirements'),
    api.get('/api/environments'),
    api.get('/api/test-data/datasets'),
    api.get('/api/versions'),
  ])
  rows.value = await api.get('/api/cases')
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

watch(() => [props.projectId, props.generationTarget], ([scope, target]) => {
  changeProject()
  projectId.value = target?.projectId || scope
  generationTarget.value = target
  runVersion.value = target?.versionId || ''
  showGeneration.value = !!target
  creating.value = false
  editing.value = null
})

function openCreate() {
  error.value = ''
  creating.value = true
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
    importWorkbook({ name: file.name.replace(/\.json$/i, ''), scenario })
  } catch (problem) {
    error.value = problem.message
  } finally {
    event.target.value = ''
  }
}

function importWorkbook({ name, scenario }) {
  error.value = ''
  importedTarget.value = ''
  form.value.type = 'ui'
  form.value.name ||= name
  structured.value = true
  steps.value = scenario.steps
}

function importLegacyWorkbook({ name, scenario, url }) {
  error.value = ''
  form.value.type = 'ui'
  form.value.name = name
  form.value.scenario = scenario
  structured.value = false
  steps.value = []
  importedTarget.value = url
  form.value.environment_id = environments.value.find((environment) =>
    environment.project_id === projectId.value && environment.base_url === url,
  )?.id || ''
}

async function add() {
  const { type } = form.value
  const scenario = type === 'functional' ? JSON.stringify({ steps: functionalSteps.value })
    : type === 'security' ? JSON.stringify({ checklist: checklist.value })
      : structured.value && type === 'ui' ? encodeSteps(steps.value) : form.value.scenario
  if (!form.value.name || !scenario) return
  if (type === 'security' && !checklist.value.length) {
    error.value = '请选择至少一个审查项'
    return
  }
  error.value = ''
  saving.value = true
  try {
    await api.post('/api/cases', { ...form.value, scenario, project_id: projectId.value })
    form.value = { name: '', type: 'ui', scenario: '', requirement_id: '', environment_id: '', expected_source: 'requirement', dataset_id: '', data_row_id: '' }
    dataRecords.value = []
    importedTarget.value = ''
    structured.value = false
    steps.value = []
    functionalSteps.value = []
    checklist.value = []
    creating.value = false
    await load()
  } catch (e) { error.value = e.message }
  finally { saving.value = false }
}

function changeCaseType() {
  form.value.scenario = ''
  importedTarget.value = ''
  if (!isExecutable(form.value)) {
    form.value.environment_id = ''
    form.value.dataset_id = ''
    form.value.data_row_id = ''
    dataRecords.value = []
  }
  if (form.value.type === 'functional' && !functionalSteps.value.length) functionalSteps.value = [{ action: '', expected: '' }]
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
  const { type, structured, steps, checklist } = editing.value
  const scenario = type === 'functional' ? JSON.stringify({ steps })
    : type === 'security' ? JSON.stringify({ checklist })
      : structured ? encodeSteps(steps) : editing.value.scenario
  if (!scenario.trim()) return
  if (type === 'security' && !checklist.length) {
    error.value = '请选择至少一个审查项'
    return
  }
  error.value = ''
  saving.value = true
  try {
    await api.patch(`/api/cases/${editing.value.id}`, { scenario })
    editing.value = null
    await load()
  } catch (e) { error.value = e.message }
  finally { saving.value = false }
}

async function review(row, decision) {
  error.value = ''
  reviewBusy.value = true
  try {
    await api.post(`/api/cases/${row.id}/review`, { decision, ...(decision === 'reject' && { feedback: feedback.value }) })
    returning.value = null
    if (editing.value?.id === row.id) editing.value = null
    await load()
  } catch (e) { error.value = e.message }
  finally { reviewBusy.value = false }
}

function openReturn(row) {
  error.value = ''
  feedback.value = ''
  editing.value = null
  returning.value = row
}

async function revise(row) {
  error.value = ''
  reviewBusy.value = true
  try {
    await api.post(`/api/cases/${row.id}/revise-with-ai`, {})
    await load()
    const revised = rows.value.find((item) => item.id === row.id)
    if (revised) openEdit(revised)
  } catch (e) { error.value = e.message }
  finally { reviewBusy.value = false }
}

async function recordManualResult() {
  error.value = ''
  manualSubmitting.value = true
  try {
    const { run_id } = await api.post(`/api/cases/${editing.value.id}/manual-result`, manualResult.value)
    editing.value = null
    emit('open-run', run_id)
  } catch (problem) {
    error.value = problem.message
  } finally {
    manualSubmitting.value = false
  }
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
  error.value = ''
  caseHistory.value = []
  historyError.value = ''
  editing.value = { ...row, structured: false, steps: [], checklist: [] }
  manualResult.value = { status: '', report: '', version_id: row.project_id === projectId.value ? runVersion.value : '' }
  if (isStructuredUI(row)) toggleStructured('edit')
  if (row.type === 'functional' || row.type === 'security') {
    try {
      const scenario = JSON.parse(row.scenario)
      if (Array.isArray(scenario?.steps)) editing.value.steps = scenario.steps
      if (Array.isArray(scenario?.checklist)) editing.value.checklist = scenario.checklist
    } catch {
      error.value = '场景格式无效，请修正后再提交审核'
    }
    if (row.type === 'functional' && !editing.value.steps.length && row.status === 'draft') {
      editing.value.steps = [{ action: '', expected: '' }]
    }
  }
}

async function loadHistory() {
  const caseId = editing.value.id
  try {
    const history = await api.get(`/api/cases/${caseId}/history`)
    if (editing.value?.id === caseId) caseHistory.value = history.slice().reverse()
  } catch (problem) {
    if (editing.value?.id === caseId) historyError.value = problem.message
  }
}

function isStructuredUI(row) {
  if (row.type !== 'ui') return false
  try { return Array.isArray(JSON.parse(row.scenario)?.steps) } catch { return false }
}

onMounted(async () => {
  try {
    await load()
    if (props.selectedCase) {
      const selected = rows.value.find((row) => row.id === props.selectedCase)
      if (selected) openEdit(selected)
      else error.value = '用例不存在或无法访问'
    }
  } catch (problem) {
    error.value = problem.message
  }
})
</script>

<template>
  <header class="head">
    <search class="form" aria-label="筛选用例">
      <select v-if="!props.projectId" v-model="projectId" aria-label="项目" @change="changeProject">
        <option value="">全部项目</option>
        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <input v-model.trim="query" type="search" aria-label="搜索用例名称" placeholder="搜索用例名称" />
      <select v-model="typeFilter" aria-label="筛选用例类型">
        <option value="">全部类型</option>
        <option v-for="(label, value) in TYPE_LABEL" :key="value" :value="value">{{ label }}</option>
      </select>
      <select v-model="statusFilter" aria-label="筛选审核状态">
        <option value="">全部状态</option>
        <option value="draft">草稿</option>
        <option value="returned">已退回</option>
        <option value="in_review">待审核</option>
        <option value="approved">已定稿</option>
      </select>
      <button class="primary" type="button" :disabled="!projectId" @click="openCreate">新建用例</button>
      <button v-if="canRecord" type="button" @click="recordUiCase">录制 UI 自动化用例</button>
      <button type="button" :disabled="!projectId" :aria-expanded="showGeneration" aria-controls="case-generation" @click="showGeneration = !showGeneration">AI 生成</button>
    </search>
    <label v-if="projectId">执行版本
      <select v-model="runVersion" name="run-version">
        <option value="">不关联版本</option>
        <option v-for="version in versions.filter((item) => item.project_id === projectId)" :key="version.id" :value="version.id">{{ version.name }}</option>
      </select>
    </label>
    <div id="case-generation" v-if="showGeneration && projectId">
      <CaseGenerationPanel
        v-if="projectRequirements.length"
        :key="projectId"
        :project-id="projectId"
        :requirements="projectRequirements"
        :initial-requirement-id="generationTarget?.requirementId"
        :initial-version-id="generationTarget?.versionId"
        :initial-review-id="generationTarget?.analysisId"
        @generated="load"
      />
      <p v-else class="hint">先在「需求」中添加或导入项目需求。</p>
    </div>
  </header>
  <p v-if="error && !creating && !editing && !returning" class="error" role="alert">{{ error }}</p>

  <DataTable :rows="filtered" :columns="columns" empty="没有符合条件的用例">
    <template #type="{ row }"><span class="mono">{{ TYPE_LABEL[row.type] || row.type }}</span></template>
    <template #generation_mode="{ row }"><span class="muted">{{ MODE_LABEL[row.generation_mode] || row.generation_mode }}</span></template>
    <template #status="{ row }"><span v-if="hasCurrentFeedback(row)">已退回</span><StatusTag v-else :value="row.status" /></template>
    <template #next="{ row }">{{ nextAction(row) }}</template>
    <template #actions="{ row }">
      <div class="actions">
        <button v-if="hasCurrentFeedback(row)" type="button" :disabled="reviewBusy" @click="revise(row)">AI 修订</button>
        <button v-if="row.status === 'draft' && !hasCurrentFeedback(row)" type="button" :disabled="reviewBusy" @click="review(row, 'submit_review')">提交审核</button>
        <button v-if="row.status === 'approved' && isExecutable(row)" type="button" :disabled="!!runSubmitting" @click="run(row)">{{ isStructuredUI(row) ? '回放' : '执行' }}</button>
        <button v-if="row.status === 'approved' && isExecutable(row)" type="button" @click="openSchedule(row)">计划</button>
        <button type="button" @click="openEdit(row)">{{ row.status === 'draft' ? '编辑' : row.status === 'in_review' ? '审核' : '查看' }}</button>
        <CaseWorkbookExport v-if="isStructuredUI(row)" :name="row.name" :scenario="row.scenario" />
      </div>
    </template>
  </DataTable>

  <div v-if="creating" class="drawer" @click.self="!saving && (creating = false)">
    <form class="panel" action="/api/cases" method="post" aria-labelledby="create-case-title" @submit.prevent="add">
      <h3 id="create-case-title">新建用例</h3>
      <div class="form">
      <input v-model.trim="form.name" aria-label="用例名" placeholder="用例名" class="name" required />
      <select v-model="form.type" name="type" aria-label="用例类型" @change="changeCaseType">
        <option v-for="(label, value) in TYPE_LABEL" :key="value" :value="value">{{ label }}</option>
      </select>
      <input v-if="!structured && form.type === 'ui'" v-model="form.scenario" aria-label="场景描述" placeholder="场景描述" class="scenario" />
      <button v-if="form.type === 'ui'" type="button" @click="toggleStructured('new')">{{ structured ? '自然语言场景' : '结构化步骤' }}</button>
      <label v-if="form.type === 'ui'">导入录制脚本<input type="file" name="scenario-file" accept=".json,application/json" @change="importScenario" /></label>
      <CaseWorkbookImport v-if="form.type === 'ui'" @imported="importWorkbook" @legacy-imported="importLegacyWorkbook" />
      <select v-model="form.requirement_id" aria-label="关联需求">
        <option value="">无关联需求</option>
        <option v-for="r in projectRequirements" :key="r.id" :value="r.id">{{ r.code || r.title }}</option>
      </select>
      <select v-if="isExecutable(form)" v-model="form.environment_id" aria-label="测试环境">
        <option value="">项目默认环境</option>
        <option v-for="environment in environments.filter((item) => item.project_id === projectId)" :key="environment.id" :value="environment.id">{{ environment.name }}</option>
      </select>
      <select v-if="isExecutable(form)" v-model="form.dataset_id" aria-label="绑定测试数据集" @change="loadDataRecords">
        <option value="">不绑定数据集</option>
        <option v-for="dataset in projectDatasets" :key="dataset.id" :value="dataset.id">{{ dataset.name }}</option>
      </select>
      <select v-if="isExecutable(form) && form.dataset_id" v-model="form.data_row_id" aria-label="绑定数据行">
        <option value="">请选择数据行</option>
        <option v-for="record in dataRecords" :key="record.id" :value="record.id">第 {{ record.row_no }} 行</option>
      </select>
    </div>
    <p v-if="importedTarget" class="hint" role="status">原工作簿地址 {{ importedTarget }}。请核对执行环境。</p>
    <HttpScenarioFields v-if="form.type === 'api' || form.type === 'perf'" :key="form.type" v-model="form.scenario" :type="form.type" />
    <fieldset v-if="form.type === 'functional'" class="step-editor review-fields">
      <legend>手工步骤</legend>
      <div v-for="(step, index) in functionalSteps" :key="index" class="functional-step">
        <label>步骤 {{ index + 1 }}<input v-model.trim="step.action" :name="`action-${index}`" required maxlength="500" /></label>
        <label>预期结果<input v-model.trim="step.expected" :name="`expected-${index}`" required maxlength="500" /></label>
        <button v-if="functionalSteps.length > 1" type="button" @click="functionalSteps.splice(index, 1)">移除步骤 {{ index + 1 }}</button>
      </div>
      <button v-if="functionalSteps.length < 15" type="button" @click="functionalSteps.push({ action: '', expected: '' })">添加步骤</button>
    </fieldset>
    <fieldset v-if="form.type === 'security'" class="step-editor review-fields">
      <legend>人工审查清单</legend>
      <label v-for="(label, key) in CHECKLIST" :key="key"><input v-model="checklist" name="checklist" type="checkbox" :value="key" /> {{ label }}</label>
    </fieldset>
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
        <label><input v-model="step.screenshot" type="checkbox" />执行后截图</label>
        <button type="button" @click="removeStep(index)">移除</button>
      </div>
      <button type="button" @click="addStep(steps)">添加步骤</button>
    </section>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="schedule-actions">
        <button type="button" :disabled="saving" @click="creating = false">取消</button>
        <button class="primary" type="submit" :disabled="saving || !projectId || (form.dataset_id && !form.data_row_id)">保存草稿</button>
      </div>
    </form>
  </div>

  <div v-if="editing" class="drawer" @click.self="!manualSubmitting && (editing = null)">
    <div class="panel">
    <form aria-labelledby="case-detail-title" @submit.prevent="saveEdit">
      <h3 id="case-detail-title">{{ editing.name }}</h3>
      <dl>
        <dt>类型</dt><dd>{{ TYPE_LABEL[editing.type] }}</dd>
        <dt>期望来源</dt><dd>{{ editing.expected_source }}</dd>
        <dt>生成方式</dt><dd>{{ MODE_LABEL[editing.generation_mode] }}</dd>
        <dt>版本</dt><dd>v{{ editing.version_no }}</dd>
        <dt>审核人</dt><dd>{{ editing.reviewer || '—' }}</dd>
      </dl>
      <p v-if="editing.review_feedback" class="feedback">v{{ editing.review_feedback_version }} 退回反馈：{{ editing.review_feedback }}</p>
      <label v-if="editing.type === 'ui' && editing.status === 'draft'" class="scenario-mode">场景格式
        <select :value="editing.structured" @change="setEditScenarioMode"><option :value="false">自然语言</option><option :value="true">结构化步骤</option></select>
      </label>
      <HttpScenarioFields v-if="editing.type === 'api' || editing.type === 'perf'" v-model="editing.scenario" :type="editing.type" :readonly="editing.status !== 'draft'" />
      <fieldset v-else-if="editing.type === 'functional'" class="step-editor review-fields" :disabled="editing.status !== 'draft'">
        <legend>手工步骤</legend>
        <div v-for="(step, index) in editing.steps" :key="index" class="functional-step">
          <label>步骤 {{ index + 1 }}<input v-model.trim="step.action" :name="`edit-action-${index}`" required maxlength="500" /></label>
          <label>预期结果<input v-model.trim="step.expected" :name="`edit-expected-${index}`" required maxlength="500" /></label>
          <button v-if="editing.status === 'draft' && editing.steps.length > 1" type="button" @click="editing.steps.splice(index, 1)">移除步骤 {{ index + 1 }}</button>
        </div>
        <button v-if="editing.status === 'draft' && editing.steps.length < 15" type="button" @click="editing.steps.push({ action: '', expected: '' })">添加步骤</button>
      </fieldset>
      <fieldset v-else-if="editing.type === 'security'" class="step-editor review-fields" :disabled="editing.status !== 'draft'">
        <legend>人工审查清单</legend>
        <label v-for="(label, key) in CHECKLIST" :key="key"><input v-model="editing.checklist" name="checklist" type="checkbox" :value="key" /> {{ label }}</label>
      </fieldset>
      <textarea v-else-if="!editing.structured" v-model="editing.scenario" name="scenario" aria-label="场景描述" :readonly="editing.status !== 'draft'" required rows="7" />
      <fieldset v-else class="step-editor review-fields" :disabled="editing.status !== 'draft'">
        <legend>UI 步骤</legend>
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
          <label><input v-model="step.screenshot" type="checkbox" :disabled="editing.status !== 'draft'" />执行后截图</label>
          <button v-if="editing.status === 'draft'" type="button" @click="editing.steps.splice(index, 1)">移除</button>
        </div>
        <button v-if="editing.status === 'draft'" type="button" @click="addStep(editing.steps)">添加步骤</button>
      </fieldset>
      <details :open="selectedCase === editing.id || undefined" @toggle="($event) => $event.target.open && loadHistory()">
        <summary>用例修订历史</summary>
        <p v-if="historyError" role="alert" class="error">{{ historyError }}</p>
        <details v-for="revision in caseHistory" :key="revision.version_no">
          <summary>v{{ revision.version_no }} · {{ revision.created_at }}</summary>
          <pre class="revision-content">{{ revision.scenario }}</pre>
        </details>
      </details>
      <p v-if="error && !acceptsManualResult(editing)" class="error" role="alert">{{ error }}</p>
      <div class="schedule-actions">
        <button type="button" :disabled="manualSubmitting" @click="editing = null">关闭</button>
        <button v-if="hasCurrentFeedback(editing)" type="button" :disabled="reviewBusy || saving" @click="revise(editing)">{{ reviewBusy ? '正在修订…' : '按反馈 AI 修订' }}</button>
        <button v-if="editing.status === 'draft'" class="primary" type="submit" :disabled="saving || reviewBusy">保存场景</button>
        <template v-if="editing.status === 'in_review'">
          <button type="button" :disabled="reviewBusy" @click="openReturn(editing)">退回</button>
          <button type="button" class="primary" :disabled="reviewBusy" @click="review(editing, 'approve')">通过并定稿</button>
        </template>
      </div>
    </form>
    <form v-if="acceptsManualResult(editing)" class="manual-result" :action="`/api/cases/${editing.id}/manual-result`" method="post"
      aria-labelledby="manual-result-title" @submit.prevent="recordManualResult">
      <h3 id="manual-result-title">登记人工结果</h3>
      <fieldset class="review-fields" :disabled="manualSubmitting">
        <legend>验证结果</legend>
        <label><input v-model="manualResult.status" name="status" type="radio" value="passed" required /> 通过</label>
        <label><input v-model="manualResult.status" name="status" type="radio" value="failed" /> 失败</label>
      </fieldset>
      <label>验证记录<textarea v-model.trim="manualResult.report" name="report" rows="4" required /></label>
      <label>关联版本
        <select v-model="manualResult.version_id" name="version_id">
          <option value="">不关联版本</option>
          <option v-for="version in versions.filter((item) => item.project_id === editing.project_id)" :key="version.id" :value="version.id">{{ version.name }}</option>
        </select>
      </label>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button class="primary" type="submit" :disabled="manualSubmitting">{{ manualSubmitting ? '正在登记…' : '保存结果' }}</button>
    </form>
    </div>
  </div>

  <div v-if="returning" class="drawer" @click.self="!reviewBusy && (returning = null)">
    <form class="panel" :action="`/api/cases/${returning.id}/review`" method="post" aria-labelledby="return-case-title" @submit.prevent="review(returning, 'reject')">
      <h3 id="return-case-title">退回 · {{ returning.name }}</h3>
      <label for="case-feedback">需要修改的内容</label>
      <textarea id="case-feedback" v-model.trim="feedback" name="feedback" rows="5" maxlength="2000" required />
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="schedule-actions">
        <button type="button" :disabled="reviewBusy" @click="openEdit(returning); returning = null">取消</button>
        <button type="submit" class="primary" :disabled="reviewBusy">退回并保存反馈</button>
      </div>
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
.head { display: grid; gap: 12px; margin-bottom: 12px; }
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
.actions { display: flex; flex-wrap: wrap; gap: 6px; }
.actions button { padding: 3px 9px; font-size: 12px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }
.muted { color: var(--muted); }
.revision-content { white-space: pre-wrap; overflow-wrap: anywhere; }
.error { color: var(--bad); }
.schedule-field { display: flex; flex-direction: column; gap: 6px; margin-bottom: 16px; font-size: 13px; }
.schedule-field > span { color: var(--muted); }
.schedule-field select, .schedule-field input { font: inherit; padding: 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.schedule-switch { display: flex; align-items: center; gap: 8px; margin: 4px 0 22px; }
.schedule-actions { display: flex; justify-content: flex-end; gap: 8px; }
.drawer { position: fixed; inset: 0; background: rgba(20,20,24,0.28); display: flex; justify-content: flex-end; }
.panel { width: min(520px, 100%); background: var(--panel); padding: 22px; overflow: auto; }
.panel h3 { margin: 0 0 16px; font-size: 15px; }
.review-fields { padding: 0; border: 0; min-width: 0; }
.functional-step, .functional-step label { display: grid; gap: 5px; }
.functional-step { border: 1px solid var(--line); border-radius: 6px; padding: 10px; }
.feedback { overflow-wrap: anywhere; white-space: pre-wrap; }
.manual-result { display: grid; gap: 8px; margin-top: 16px; border-top: 1px solid var(--line); padding-top: 16px; }
.manual-result label { display: block; }
.manual-result fieldset label { display: inline-block; margin-inline-end: 12px; }
.manual-result textarea { margin-block: 4px 0; }
.manual-result button { justify-self: end; }
dl { display: grid; grid-template-columns: 84px 1fr; gap: 10px 12px; font-size: 13px; margin: 0 0 22px; }
dt { color: var(--muted); }
dd { margin: 0; word-break: break-word; }
</style>
