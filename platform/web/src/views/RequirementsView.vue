<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { api } from '../api'

const props = defineProps({ projectId: { type: String, default: '' } })
const emit = defineEmits(['generate-cases'])

const rows = ref([])
const projects = ref([])
const versions = ref([])
const localProjectId = ref('')
const projectId = computed(() => props.projectId || localProjectId.value)
const entryMode = ref('')
const form = ref({ code: '', title: '', content: '' })
const editingId = ref('')
const fileInput = ref(null)
const file = ref(null)
const useSample = ref(false)
const preview = ref([])
const historyId = ref('')
const history = ref([])
const revisionNo = ref('')
const versionId = ref('')
const impact = ref(null)
const rounds = ref([])
const analysis = ref(null)
const linkedVersionIds = ref([])
const approvalVersionId = ref('')
const returning = ref(false)
const feedback = ref('')
const feedbackInput = ref(null)
const busy = ref(false)
const processing = ref('')
const error = ref('')
const message = ref('')

const sampleCsv = 'code,title,content\nDEMO-01,登录成功,使用有效账号登录后进入工作台\nDEMO-02,登录失败,密码错误时显示提示'
const statusLabels = { draft: '草稿', submitted: '待评审', returned: '已退回', approved: '已批准' }
const projectRows = computed(() => rows.value.filter((row) => row.project_id === projectId.value))
const projectVersions = computed(() => versions.value.filter((version) => version.project_id === projectId.value))
const selectedRequirement = computed(() => projectRows.value.find((row) => row.id === historyId.value))
const selectedRevision = computed(() => history.value.find((revision) => revision.revision_no === revisionNo.value))
const linkedVersions = computed(() => projectVersions.value.filter((version) => linkedVersionIds.value.includes(version.id)))
const latestRound = computed(() => analysis.value?.id === rounds.value[0]?.id)
const requirementPath = (id) => `/api/requirements/${encodeURIComponent(id)}`
const analysesPath = computed(() => `${requirementPath(historyId.value)}/point-analyses`)

async function refreshRows() {
  rows.value = await api.get('/api/requirements')
}

async function run(action, label = '') {
  busy.value = true
  processing.value = label
  error.value = ''
  message.value = ''
  try {
    await action()
  } catch (err) {
    error.value = err.message
  } finally {
    busy.value = false
    processing.value = ''
  }
}

function resetForm() {
  editingId.value = ''
  form.value = { code: '', title: '', content: '' }
}

function resetImport() {
  file.value = null
  useSample.value = false
  preview.value = []
  if (fileInput.value) fileInput.value.value = ''
}

async function changeProject() {
  entryMode.value = ''
  resetForm()
  resetImport()
  historyId.value = ''
  history.value = []
  revisionNo.value = ''
  versionId.value = ''
  impact.value = null
  resetPointSelection()
  if (projectRows.value.length) await loadHistory(projectRows.value[0].id)
}

function edit(row) {
  entryMode.value = 'manual'
  editingId.value = row.id
  form.value = { code: row.code ?? '', title: row.title, content: row.content ?? '' }
}

async function loadHistory(id) {
  const revisions = await api.get(`${requirementPath(id)}/history`)
  resetPointSelection()
  history.value = revisions
  historyId.value = id
  revisionNo.value = history.value.at(-1)?.revision_no ?? ''
  if (revisionNo.value) await loadPointRounds()
}

function resetPointSelection() {
  rounds.value = []
  analysis.value = null
  linkedVersionIds.value = []
  approvalVersionId.value = ''
  returning.value = false
  feedback.value = ''
}

async function loadPointRounds() {
  const data = await api.get(`${analysesPath.value}?revision_no=${revisionNo.value}`)
  rounds.value = data.rounds
  linkedVersionIds.value = data.version_ids
  await selectRound(rounds.value[0]?.id)
}

async function selectRound(id) {
  analysis.value = id ? await api.get(`${analysesPath.value}/${encodeURIComponent(id)}`) : null
  returning.value = false
  feedback.value = ''
  approvalVersionId.value = ''
}

function selectRevision() {
  return run(async () => {
    resetPointSelection()
    await loadPointRounds()
  })
}

function useAnalysis(result) {
  analysis.value = result
  rounds.value = [result, ...rounds.value.filter((round) => round.id !== result.id)]
  returning.value = false
  feedback.value = ''
  approvalVersionId.value = ''
}

function generatePoints(optimize = false) {
  return run(async () => {
    const result = optimize
      ? await api.post(`${analysesPath.value}/${encodeURIComponent(analysis.value.id)}/optimize`)
      : await api.post(analysesPath.value, { revision_no: revisionNo.value })
    useAnalysis(result)
    message.value = optimize ? '新一轮草稿已生成' : '功能点草稿已生成'
  }, optimize ? 'AI 正在按退回意见优化' : 'AI 正在解析需求')
}

function review(action) {
  return run(async () => {
    const result = await api.post(`${analysesPath.value}/${encodeURIComponent(analysis.value.id)}/review`, {
      action,
      ...(action === 'return' && { feedback: feedback.value }),
      ...(action === 'approve' && approvalVersionId.value && { version_id: approvalVersionId.value }),
    })
    useAnalysis(result)
    message.value = statusLabels[result.status]
    if (action === 'approve') generateCases(result)
  })
}

async function openFeedback() {
  returning.value = true
  await nextTick()
  feedbackInput.value?.focus()
}

function generateCases(result) {
  emit('generate-cases', {
    projectId: result.project_id,
    requirementId: result.requirement_id,
    analysisId: result.id,
    ...(result.version_id && { versionId: result.version_id }),
  })
}

function save() {
  return run(async () => {
    if (!form.value.title) throw new Error('请输入标题')
    const isEdit = Boolean(editingId.value)
    let id = editingId.value
    if (isEdit) {
      await api.patch(requirementPath(id), form.value)
    } else {
      const created = await api.post('/api/requirements', { ...form.value, project_id: projectId.value })
      id = created.id
    }
    resetForm()
    entryMode.value = ''
    await refreshRows()
    await loadHistory(id)
    message.value = isEdit ? '已保存修订' : '已录入需求'
  })
}

function uploadDocument(previewOnly) {
  const selected = useSample.value ? new File([sampleCsv], '需求示例.csv', { type: 'text/csv' }) : file.value
  if (!selected) throw new Error('请选择需求文件或载入示例 CSV')
  if (selected.size > 10 * 1024 * 1024) throw new Error('文件不能超过 10 MiB')
  const payload = new FormData()
  payload.append('file', selected)
  const query = new URLSearchParams({ project_id: projectId.value, preview: previewOnly ? '1' : '0' })
  return api.upload(`/api/requirements/documents?${query}`, payload)
}

function previewDocument() {
  return run(async () => {
    preview.value = []
    const { items } = await uploadDocument(true)
    preview.value = items
    message.value = `识别到 ${items.length} 条需求，请确认后导入`
  })
}

function importRequirements() {
  return run(async () => {
    const { ids } = await uploadDocument(false)
    resetImport()
    entryMode.value = ''
    await refreshRows()
    if (ids.length) await loadHistory(ids[0])
    message.value = `已导入 ${ids.length} 条需求`
  })
}

function linkRevision() {
  return run(async () => {
    await api.put(`${requirementPath(historyId.value)}/revisions/${revisionNo.value}/versions/${encodeURIComponent(versionId.value)}`)
    if (!linkedVersionIds.value.includes(versionId.value)) linkedVersionIds.value.push(versionId.value)
    impact.value = null
    message.value = '已关联修订'
  })
}

function viewImpact() {
  return run(async () => {
    impact.value = await api.get(`/api/versions/${encodeURIComponent(versionId.value)}/requirement-impact`)
  })
}

onMounted(() => run(async () => {
  const [projectList, versionList] = await Promise.all([api.get('/api/projects'), api.get('/api/versions'), refreshRows()])
  projects.value = projectList
  versions.value = versionList
  localProjectId.value = projectList[0]?.id ?? ''
  await changeProject()
}))

watch(() => props.projectId, () => {
  if (projects.value.length) run(changeProject)
})
</script>

<template>
  <header class="head">
    <div v-if="!props.projectId" class="field">
      <label for="requirement-project">项目</label>
      <select id="requirement-project" v-model="localProjectId" :disabled="busy || !projects.length" @change="run(changeProject)">
        <option v-if="!projects.length" value="">暂无项目</option>
        <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
      </select>
    </div>
    <div v-if="projectId" class="actions">
      <button type="button" :disabled="busy" :aria-expanded="entryMode === 'manual'"
        @click="resetForm(); entryMode = entryMode === 'manual' ? '' : 'manual'">新建需求</button>
      <button type="button" :disabled="busy" :aria-expanded="entryMode === 'import'"
        @click="resetImport(); entryMode = entryMode === 'import' ? '' : 'import'">导入文档</button>
    </div>
  </header>
  <div role="alert"><p v-if="error" class="error">{{ error }}</p></div>
  <div role="status">
    <p v-if="processing" class="processing"><span aria-hidden="true"></span>{{ processing }}…</p>
    <p v-else-if="message">{{ message }}</p>
  </div>
  <section v-if="projectId && entryMode === 'manual'" class="panel">
    <h2 id="requirement-form-heading">{{ editingId ? '编辑需求' : '手工录入' }}</h2>
    <form class="form" aria-labelledby="requirement-form-heading" @submit.prevent="save">
      <div class="field">
        <label for="requirement-code">编号</label>
        <input id="requirement-code" v-model="form.code" name="code" :disabled="busy" />
      </div>
      <div class="field">
        <label for="requirement-title">标题（必填）</label>
        <input id="requirement-title" v-model.trim="form.title" name="title" required :disabled="busy" />
      </div>
      <div class="field grow">
        <label for="requirement-content">内容</label>
        <textarea id="requirement-content" v-model="form.content" name="content" rows="3" :disabled="busy" />
      </div>
      <button class="primary" type="submit" :disabled="busy">{{ editingId ? '保存修订' : '录入' }}</button>
      <button type="button" :disabled="busy" @click="resetForm(); entryMode = ''">取消</button>
    </form>
  </section>
  <section v-if="projectId && entryMode === 'import'" class="panel">
    <h2 id="requirement-import-heading">上传需求说明书</h2>
    <form class="form" aria-labelledby="requirement-import-heading" @submit.prevent="previewDocument">
      <div class="field">
        <label for="requirement-file">PDF / Word（.docx）/ Excel（.xlsx）/ UTF-8 CSV / Markdown / TXT</label>
        <input id="requirement-file" ref="fileInput" name="file" type="file" accept=".pdf,.docx,.xlsx,.csv,.md,.markdown,.txt" :disabled="busy"
          @change="file = $event.target.files[0]; useSample = false; preview = []; message = ''; error = ''" />
      </div>
      <button type="button" :disabled="busy" @click="resetImport(); useSample = true">载入示例 CSV</button>
      <button type="submit" :disabled="busy || (!file && !useSample)">解析预览</button>
      <button v-if="preview.length" class="primary" type="button" :disabled="busy" @click="importRequirements">确认导入</button>
    </form>
    <div v-if="preview.length" class="document-preview">
      <details v-for="(item, index) in preview" :key="index" :open="preview.length === 1">
        <summary>{{ [item.code, item.title].filter(Boolean).join(' · ') }}</summary>
        <p class="text">{{ item.content }}</p>
      </details>
    </div>
  </section>
  <div v-if="projectId" class="workbench" :aria-busy="busy">
    <nav class="panel master" aria-labelledby="requirement-list-heading">
      <h2 id="requirement-list-heading">需求列表</h2>
      <p v-if="!projectRows.length" class="muted">{{ busy ? '正在读取需求…' : '还没有需求' }}</p>
      <ul v-else>
        <li v-for="row in projectRows" :key="row.id">
          <button type="button" :aria-current="row.id === historyId ? 'true' : undefined" :disabled="busy"
            @click="run(() => loadHistory(row.id))">
            <span v-if="row.code" class="muted">{{ row.code }}</span>
            <strong>{{ row.title }}</strong>
          </button>
        </li>
      </ul>
    </nav>
    <section v-if="selectedRequirement && selectedRevision" class="panel detail">
      <div class="detail-head">
        <h2>{{ selectedRevision.title }}</h2>
        <button type="button" :disabled="busy" @click="edit(selectedRequirement)">编辑最新修订</button>
      </div>
      <div class="field">
        <label for="requirement-revision">源修订</label>
        <select id="requirement-revision" v-model="revisionNo" :disabled="busy" @change="selectRevision">
          <option v-for="revision in history" :key="revision.revision_no" :value="revision.revision_no">
            第 {{ revision.revision_no }} 版 · {{ revision.created_at }}
          </option>
        </select>
      </div>
      <details class="source">
        <summary>查看原文</summary>
        <p class="text">{{ selectedRevision.content || '暂无正文' }}</p>
      </details>
      <div class="detail-head workflow-head">
        <h3>功能点评审</h3>
        <button type="button" :disabled="busy" @click="run(loadPointRounds)">刷新</button>
      </div>
      <div v-if="rounds.length" class="field">
        <label for="requirement-round">评审轮次</label>
        <select id="requirement-round" :value="analysis?.id" :disabled="busy"
          @change="run(() => selectRound($event.target.value))">
          <option v-for="round in rounds" :key="round.id" :value="round.id">第 {{ round.round_no }} 轮 · {{ statusLabels[round.status] }}</option>
        </select>
      </div>
      <div v-if="analysis" class="analysis">
        <div v-if="analysis.feedback" class="feedback">
          <strong>退回意见</strong>
          <p class="text">{{ analysis.feedback }}</p>
        </div>
        <ol class="points">
          <li v-for="(point, index) in analysis.points" :key="index">
            <h4>{{ point.title }}</h4>
            <p class="text">{{ point.description }}</p>
            <strong>验收标准</strong>
            <ul>
              <li v-for="(criterion, criterionIndex) in point.acceptance_criteria" :key="criterionIndex" class="text">{{ criterion }}</li>
            </ul>
          </li>
        </ol>
        <p v-if="analysis.reviewed_by || analysis.submitted_by" class="muted review-meta">
          <span v-if="analysis.submitted_by">提交 · {{ analysis.submitted_by }} · {{ analysis.submitted_at }}</span>
          <span v-if="analysis.reviewed_by">评审 · {{ analysis.reviewed_by }} · {{ analysis.reviewed_at }}</span>
        </p>
        <div v-if="analysis.status === 'submitted' && linkedVersions.length && !returning" class="field review-version">
          <label for="requirement-approval-version">用例版本（可选）</label>
          <select id="requirement-approval-version" v-model="approvalVersionId" :disabled="busy">
            <option value="">不指定版本</option>
            <option v-for="version in linkedVersions" :key="version.id" :value="version.id">{{ version.name }}</option>
          </select>
        </div>
        <div v-if="!returning" class="actions workflow-actions">
          <button v-if="latestRound && analysis.status === 'draft'" class="primary" type="button" :disabled="busy" @click="review('submit')">提交评审</button>
          <template v-if="latestRound && analysis.status === 'submitted'">
            <button class="primary" type="button" :disabled="busy" @click="review('approve')">批准并生成用例</button>
            <button type="button" :disabled="busy" @click="openFeedback">退回</button>
          </template>
          <button v-if="latestRound && analysis.status === 'returned'" class="primary" type="button" :disabled="busy" @click="generatePoints(true)">按意见优化</button>
          <button v-if="analysis.status === 'approved'" class="primary" type="button" :disabled="busy" @click="generateCases(analysis)">生成用例</button>
        </div>
        <form v-if="returning" class="feedback-form" @submit.prevent="review('return')">
          <div class="field">
            <label for="requirement-feedback">退回意见（必填）</label>
            <textarea id="requirement-feedback" ref="feedbackInput" v-model.trim="feedback" rows="3" maxlength="2000" required :disabled="busy" />
          </div>
          <div class="actions">
            <button class="primary" type="submit" :disabled="busy || !feedback">确认退回</button>
            <button type="button" :disabled="busy" @click="returning = false; feedback = ''">取消</button>
          </div>
        </form>
      </div>
      <div v-else-if="!rounds.length">
        <p class="muted">此修订暂无功能点</p>
        <button class="primary" type="button" :disabled="busy" @click="generatePoints()">AI 解析功能点</button>
      </div>
    </section>
    <section v-else class="panel detail">
      <p class="muted">{{ busy ? '正在读取详情…' : '录入或导入需求后即可开始功能点评审。' }}</p>
    </section>
  </div>
  <section v-if="projectId" class="panel">
    <h2 id="requirement-impact-heading">版本影响</h2>
    <form class="form" aria-labelledby="requirement-impact-heading" @submit.prevent="viewImpact">
      <div class="field">
        <label for="requirement-version">登记版本</label>
        <select id="requirement-version" v-model="versionId" name="version_id" required :disabled="busy || !projectVersions.length"
          @change="impact = null">
          <option value="" disabled>{{ projectVersions.length ? '请选择版本' : '暂无登记版本' }}</option>
          <option v-for="version in projectVersions" :key="version.id" :value="version.id">{{ version.name }}</option>
        </select>
      </div>
      <button type="submit" :disabled="busy || !projectVersions.length">查看影响</button>
      <button v-if="selectedRevision" type="button" :disabled="busy || !versionId || linkedVersionIds.includes(versionId)" @click="linkRevision">
        {{ linkedVersionIds.includes(versionId) ? '已关联所选修订' : '关联所选修订' }}
      </button>
    </form>
    <template v-if="impact">
      <p role="status">共影响 {{ impact.affected_case_count }} 条用例</p>
      <p v-if="!impact.changes.length">该版本暂无关联需求</p>
      <article v-for="change in impact.changes" :key="`${change.requirement_id}-${change.current.revision_no}`" class="change">
        <h3>{{ change.current.title }} · 第 {{ change.current.revision_no }} 版 · {{ change.affected_case_count }} 条用例</h3>
        <div class="comparison">
          <div>
            <strong>变更前</strong>
            <template v-if="change.previous">
              <p>{{ change.previous.code }} {{ change.previous.title }}</p>
              <div class="text">{{ change.previous.content }}</div>
            </template>
            <p v-else>首版，无上一修订</p>
          </div>
          <div>
            <strong>变更后</strong>
            <p>{{ change.current.code }} {{ change.current.title }}</p>
            <div class="text">{{ change.current.content }}</div>
          </div>
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.document-preview { max-height: 24rem; overflow: auto; margin-top: 12px; padding: 8px; border: 1px solid var(--line); }
.document-preview summary, .source summary { cursor: pointer; }
.head, .detail-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
.head { margin-bottom: 16px; }
.panel { margin: 16px 0; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }
.workbench { display: grid; grid-template-columns: minmax(11rem, 1fr) minmax(0, 3fr); align-items: start; gap: 14px; }
.workbench > .panel { margin: 0; min-width: 0; }
.master ul { margin: 0; padding: 0; }
.master li { list-style: none; }
.master li + li { margin-top: 4px; }
.master button { display: grid; width: 100%; gap: 2px; min-height: 38px; text-align: left; overflow-wrap: anywhere; background: transparent; border-color: transparent; }
.master button[aria-current] { background: var(--accent-soft); color: var(--accent); border-color: var(--line); }
.master button span, .muted { color: var(--muted); }
.master button span { font-size: 12px; }
h2, h3, h4 { margin: 0; overflow-wrap: anywhere; }
h2 { font-size: 16px; }
.panel > h2 { margin-bottom: 12px; }
h3 { font-size: 15px; }
h4 { font-size: 14px; }
.detail-head { margin-bottom: 12px; }
.source { margin-top: 12px; }
.workflow-head { border-top: 1px solid var(--line); margin-top: 16px; padding-top: 12px; }
.points { margin: 12px 0; padding-left: 2rem; }
.points > li { padding: 10px 8px; border-bottom: 1px solid var(--line); }
.points p { margin: 6px 0 8px; }
.points ul { margin-top: 4px; padding-left: 1.25rem; }
.feedback { margin-top: 12px; border-inline-start: 3px solid var(--bad); padding: 4px 10px; }
.feedback p { margin: 4px 0 0; }
.review-meta { display: flex; flex-wrap: wrap; gap: 8px 16px; font-size: 12px; }
.review-version, .feedback-form .field { margin-bottom: 10px; }
.workflow-actions:empty { display: none; }
.processing { display: flex; align-items: center; gap: 8px; color: var(--accent); }
.processing span { inline-size: 8px; block-size: 8px; background: currentColor; border-radius: 50%; }
@media (prefers-reduced-motion: no-preference) {
  .processing span { animation: pulse 800ms alternate 4; }
  @keyframes pulse { to { opacity: .35; } }
}
@media (max-width: 780px) {
  .workbench { grid-template-columns: minmax(0, 1fr); }
  .master ul { max-height: 15rem; overflow: auto; }
}
.form, .actions, .comparison { display: flex; flex-wrap: wrap; gap: 8px; }
.form { align-items: end; column-gap: 12px; }
.field { display: grid; gap: 4px; justify-items: start; }
.grow, .comparison > div { flex: 1 1 220px; min-width: 0; }
select, input, textarea { box-sizing: border-box; max-width: 100%; font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
textarea { width: 100%; resize: vertical; }
.text, pre { white-space: pre-wrap; overflow-wrap: anywhere; }
.change { border-top: 1px solid var(--line); }
.error { color: var(--bad); }
</style>
