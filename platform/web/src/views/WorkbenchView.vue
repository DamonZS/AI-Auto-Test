<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api'

const props = defineProps({ projectId: { type: String, default: '' } })
const emit = defineEmits(['open-project-view'])
const collection = '/api/workbench/documents'
const labels = { draft: '草稿', in_review: '待审核', approved: '已批准', rejected: '已驳回' }
const percent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 })
const templates = ref([])
const projects = ref([])
const versions = ref([])
const documents = ref([])
const create = ref({ kind: '', project_id: props.projectId, version_id: '', title: '' })
const generationContext = ref('')
const selected = ref(null)
const draft = ref(null)
const note = ref('')
const busy = ref(false)
const feedback = ref('')
const error = ref('')
const errorSummary = ref(null)
const missing = ref([])

const availableVersions = computed(() => versions.value.filter((version) => version.project_id === create.value.project_id))
const template = computed(() => templates.value.find((item) => item.kind === selected.value?.kind))
const editable = computed(() => ['draft', 'rejected'].includes(selected.value?.status))
const documentPath = computed(() => selected.value ? `${collection}/${encodeURIComponent(selected.value.id)}` : '')
const metrics = computed(() => selected.value?.metrics)
const scopedDocuments = computed(() => props.projectId
  ? documents.value.filter((document) => document.project_id === props.projectId)
  : documents.value)

function selectDocument(document) {
  selected.value = document
  const definition = templates.value.find((item) => item.kind === document.kind)
  draft.value = {
    title: document.title,
    risk_level: document.risk_level,
    fields: Object.fromEntries(definition.fields.map(({ key }) => [key, document.fields[key] || ''])),
    checks: Object.fromEntries(definition.checks.map(({ key }) => [key, { result: '', evidence: '', ...document.checks?.[key] }])),
  }
  note.value = ''
}

function accept(document) {
  documents.value = [document, ...documents.value.filter((item) => item.id !== document.id)]
  selectDocument(document)
}

async function perform(operation) {
  busy.value = true
  error.value = ''
  feedback.value = ''
  missing.value = []
  try {
    await operation()
  } catch (failure) {
    feedback.value = ''
    let detail = failure.detail
    if (!detail) {
      try { detail = JSON.parse(failure.message) } catch { detail = null }
    }
    error.value = detail?.error || failure.message
    missing.value = detail?.missing_sections || []
    await nextTick()
    errorSummary.value?.focus()
  } finally {
    busy.value = false
  }
}

async function createDocument(generate) {
  if (generate) {
    if (!generationContext.value) throw new Error('请填写功能测试背景')
    feedback.value = '正在生成草稿…'
    accept(await api.post(`${collection}/generate`, { ...create.value, context: generationContext.value }))
    generationContext.value = ''
  } else {
    feedback.value = '正在创建草稿…'
    accept(await api.post(collection, { ...create.value, fields: {} }))
  }
  create.value.title = ''
  feedback.value = generate ? '草稿已生成，请核对内容后提审' : '草稿已创建'
}

async function save(submit) {
  accept(await api.patch(documentPath.value, { ...draft.value, revision: selected.value.revision }))
  feedback.value = '草稿已保存'
  if (submit) await review('submit')
}

async function review(action) {
  const body = { action, revision: selected.value.revision, note: note.value }
  const result = await api.post(`${documentPath.value}/review`, body)
  accept(result)
  feedback.value = { submit: '已提交审核', approve: '已批准，可下载 HTML', reject: '已驳回，可重新编辑' }[action]
}

async function download() {
  const url = URL.createObjectURL(await api.download(`${documentPath.value}/export`))
  const link = document.createElement('a')
  link.href = url
  link.download = `${selected.value.kind}-${selected.value.id}.html`
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 0)
  feedback.value = '已开始下载 HTML'
}

function missingLabel(key) {
  return [...template.value.fields, ...template.value.checks].find((item) => item.key === key)?.label || key
}

function distribution(counts) {
  return Object.entries(counts).map(([level, count]) => `${level} ${count}`).join('、') || '无'
}

onMounted(() => perform(async () => {
  const [directory, projectList, versionList, records] = await Promise.all([
    api.get('/api/workbench/templates'), api.get('/api/projects'), api.get('/api/versions'), api.get(collection),
  ])
  templates.value = directory
  projects.value = projectList
  versions.value = versionList
  documents.value = records
  create.value.kind = directory[0]?.kind || ''
}))
</script>

<template>
  <div class="workbench">
    <section class="panel lifecycle" aria-label="测试产物与流程">
      <div><strong>测试准备</strong><span>申请、计划、准入与同行评审</span></div>
      <div><strong>执行跟踪</strong><span>回归执行、缺陷与日报</span></div>
      <div><strong>质量交付</strong><span>准出核验、签核与总结报告</span></div>
      <button v-if="projectId && !availableVersions.length" type="button" @click="emit('open-project-view', { projectId, view: 'versions' })">登记项目版本</button>
    </section>
    <form class="panel" :action="collection" method="post" aria-labelledby="wb-new"
      @submit.prevent="perform(() => createDocument($event.submitter?.value === 'generate'))">
      <h3 id="wb-new">新建产物</h3>
      <fieldset class="create-grid" :disabled="busy" aria-label="新建产物字段">
        <label>类型<select v-model="create.kind" name="kind" required>
          <option v-for="item in templates" :key="item.kind" :value="item.kind">{{ item.label }}</option>
        </select></label>
        <label>项目<select v-model="create.project_id" name="project_id" :disabled="!!projectId" required @change="create.version_id = ''">
          <option value="" disabled>选择项目</option>
          <option v-for="item in projects" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select></label>
        <label>版本<select v-model="create.version_id" name="version_id" required>
          <option value="" disabled>选择版本</option>
          <option v-for="item in availableVersions" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select></label>
        <label>标题<input v-model.trim="create.title" name="title" required /></label>
        <label class="wide">功能测试背景（AI 生成时必填）
          <textarea v-model.trim="generationContext" rows="3" placeholder="填写需求、变更或已确认的执行情况，请勿填写凭据" />
        </label>
        <div class="actions wide">
          <button type="submit" value="manual">创建草稿</button>
          <button type="submit" value="generate" class="primary">AI 生成草稿</button>
        </div>
      </fieldset>
    </form>

    <div v-if="error" id="wb-error" ref="errorSummary" class="error" role="alert" tabindex="-1">
      {{ error }}
      <ul v-if="missing.length"><li v-for="key in missing" :key="key">{{ missingLabel(key) }}</li></ul>
    </div>
    <div role="status" class="feedback">{{ feedback }}</div>

    <div class="columns">
      <section class="panel" aria-labelledby="wb-list">
        <h3 id="wb-list">流程产物</h3>
        <p v-if="!scopedDocuments.length && !busy">暂无流程产物</p>
        <ul class="documents">
          <li v-for="item in scopedDocuments" :key="item.id">
            <button type="button" :disabled="busy" :aria-current="selected?.id === item.id ? 'true' : undefined"
              @click="perform(async () => selectDocument(await api.get(`${collection}/${encodeURIComponent(item.id)}`)))">
              <strong>{{ item.title }}</strong>
              <span>{{ templates.find((entry) => entry.kind === item.kind)?.label }} · {{ labels[item.status] }}</span>
            </button>
          </li>
        </ul>
      </section>

      <section v-if="selected" class="panel editor" aria-labelledby="wb-editor">
        <header class="record-header">
          <div><h3 id="wb-editor">{{ template.label }}</h3>
            <span>{{ versions.find((item) => item.id === selected.version_id)?.name || selected.version_id }} · 修订 {{ selected.revision }}</span>
          </div>
          <strong>{{ labels[selected.status] }}</strong>
        </header>
        <form :action="documentPath" method="post" aria-labelledby="wb-editor" @submit.prevent="perform(() => save($event.submitter?.value === 'submit'))">
          <fieldset :disabled="busy || !editable" aria-label="产物内容">
            <label>标题<input v-model.trim="draft.title" name="title" required /></label>
            <label>风险级别<select v-model="draft.risk_level" name="risk_level">
              <option value="">未标记</option>
              <option v-for="level in ['P0', 'P1', 'P2', 'P3']" :key="level">{{ level }}</option>
            </select></label>
            <label v-for="field in template.fields" :key="field.key">{{ field.label }}
              <textarea v-if="field.type === 'textarea'" v-model="draft.fields[field.key]" :name="field.key" rows="3"
                :aria-invalid="missing.includes(field.key)" :aria-describedby="missing.includes(field.key) ? 'wb-error' : undefined" />
              <input v-else v-model="draft.fields[field.key]" :name="field.key" :type="field.type"
                :aria-invalid="missing.includes(field.key)" :aria-describedby="missing.includes(field.key) ? 'wb-error' : undefined" />
            </label>
            <fieldset v-for="check in template.checks" :key="check.key" class="check"
              :aria-describedby="missing.includes(check.key) ? 'wb-error' : undefined">
              <legend>{{ check.label }}</legend>
              <p v-if="draft.checks[check.key].guidance" class="guidance">核验建议：{{ draft.checks[check.key].guidance }}</p>
              <label>结果<select v-model="draft.checks[check.key].result" :name="`${check.key}.result`" :aria-invalid="missing.includes(check.key)">
                <option value="">未检查</option><option value="pass">通过</option><option value="fail">未通过</option>
              </select></label>
              <label>证据<textarea v-model="draft.checks[check.key].evidence" :name="`${check.key}.evidence`" rows="2"
                :aria-invalid="missing.includes(check.key) && !draft.checks[check.key].evidence" /></label>
            </fieldset>
          </fieldset>
          <div v-if="editable" class="actions">
            <button type="submit" value="save" :disabled="busy">保存草稿</button>
            <button type="submit" value="submit" class="primary" :disabled="busy">保存并提审</button>
          </div>
        </form>

        <section v-if="metrics" class="section" aria-labelledby="wb-metrics">
          <h4 id="wb-metrics">提审时执行统计</h4>
          <dl>
            <dt>用例</dt><dd>有效 {{ metrics.case_statistics.total }} · 执行 {{ metrics.case_statistics.executed }} · 通过 {{ metrics.case_statistics.passed }} · 失败 {{ metrics.case_statistics.failed }} · 阻塞 {{ metrics.case_statistics.blocked }}</dd>
            <dt>需求覆盖</dt><dd>{{ metrics.requirement_coverage.covered }} / {{ metrics.requirement_coverage.total }}</dd>
            <template v-for="coverage in metrics.whitebox_coverage" :key="coverage.format">
              <dt>白盒覆盖率 · {{ coverage.format }}</dt>
              <dd>
                <span v-if="coverage.statements">语句 {{ coverage.statements.covered }} / {{ coverage.statements.total }}</span>
                <span v-if="coverage.lines">行 {{ coverage.lines.covered }} / {{ coverage.lines.total }}</span>
                <span v-if="coverage.branches"> · 分支 {{ coverage.branches.covered }} / {{ coverage.branches.total }}</span>
              </dd>
            </template>
            <dt>执行率 / 通过率</dt><dd>{{ percent.format(metrics.execution_rate) }} / {{ percent.format(metrics.pass_rate) }}</dd>
            <dt>执行起止</dt><dd>{{ metrics.started_at ? `${metrics.started_at} 至 ${metrics.ended_at} UTC` : '无已结束执行' }}</dd>
            <dt>缺陷总数</dt><dd>{{ metrics.defect_total }}</dd>
            <dt>缺陷优先级</dt><dd>{{ distribution(metrics.defect_distribution) }}</dd>
            <dt>缺陷严重度</dt><dd>{{ distribution(metrics.severity_distribution) }}</dd>
            <dt v-if="metrics.requires_human_review">质量门禁</dt><dd v-if="metrics.requires_human_review">命中人工复核条件</dd>
          </dl>
          <h4>遗留缺陷</h4>
          <ul v-if="metrics.residual_defects.length">
            <li v-for="defect in metrics.residual_defects" :key="defect.code">{{ defect.code }} · {{ defect.title }} · {{ defect.severity }} / {{ defect.priority }} · {{ defect.status }}</li>
          </ul>
          <p v-else>无</p>
        </section>

        <form v-if="selected.status === 'in_review'" class="section" :action="`${documentPath}/review`" method="post"
          aria-labelledby="wb-review" @submit.prevent="perform(() => review($event.submitter?.value || 'approve'))">
          <h4 id="wb-review">签核</h4>
          <label>意见（驳回必填）<textarea v-model.trim="note" name="note" rows="2" :disabled="busy" /></label>
          <div class="actions">
            <button type="submit" value="reject" :disabled="busy">驳回</button>
            <button type="submit" value="approve" class="primary" :disabled="busy">批准</button>
          </div>
        </form>
        <section v-if="selected.reviewer" class="section" aria-labelledby="wb-signature">
          <h4 id="wb-signature">签核记录</h4>
          <dl><dt>审核人</dt><dd>{{ selected.reviewer }}</dd><dt>审核时间</dt><dd>{{ selected.reviewed_at }}</dd>
            <template v-if="selected.review_note"><dt>意见</dt><dd>{{ selected.review_note }}</dd></template>
          </dl>
        </section>
        <button v-if="selected.status === 'approved'" type="button" :disabled="busy" @click="perform(download)">下载 HTML</button>
      </section>
    </div>
  </div>
</template>

<style scoped>
.lifecycle { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; }
.lifecycle > div { display: grid; gap: 3px; }
.lifecycle span { color: var(--muted); font-size: 12px; }
.workbench { display: grid; gap: 14px; }
.panel { padding: 16px; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; }
.columns { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(0, 3fr); align-items: start; gap: 14px; }
.create-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); align-items: end; gap: 10px; }
.wide { grid-column: 1 / -1; }
fieldset { min-width: 0; border: 0; margin: 0; padding: 0; }
label { display: grid; gap: 4px; }
input, select, textarea { width: 100%; padding: 7px 10px; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; font: inherit; }
textarea { resize: vertical; }
h3, h4 { margin: 0 0 12px; }
.editor label { margin-bottom: 10px; }
.record-header { display: flex; justify-content: space-between; gap: 8px; margin-bottom: 12px; }
.record-header h3 { margin-bottom: 4px; }
.record-header span, .documents span { color: var(--muted); }
.check { border: 1px solid var(--line); border-radius: 6px; padding: 8px; margin-bottom: 10px; }
.guidance { margin: 0 0 10px; color: var(--muted); white-space: pre-wrap; overflow-wrap: anywhere; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
.documents { display: grid; gap: 6px; list-style: none; margin: 0; padding: 0; }
.documents button { display: grid; gap: 4px; width: 100%; text-align: start; overflow-wrap: anywhere; }
.documents button[aria-current='true'] { border-color: var(--accent); }
.section { border-top: 1px solid var(--line); margin-top: 12px; padding-top: 12px; }
dl { display: grid; grid-template-columns: max-content 1fr; gap: 5px 10px; }
dd { margin: 0; overflow-wrap: anywhere; }
.feedback:empty { position: absolute; }
.error { color: var(--bad); }
[aria-invalid='true'] { border-color: var(--bad); }
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
@media (max-width: 760px) { .columns { grid-template-columns: 1fr; } }
</style>
