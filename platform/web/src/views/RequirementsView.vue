<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const emit = defineEmits(['generate-cases'])

const rows = ref([])
const projects = ref([])
const versions = ref([])
const projectId = ref('')
const form = ref({ code: '', title: '', content: '' })
const editingId = ref('')
const fileInput = ref(null)
const file = ref(null)
const useSample = ref(false)
const historyId = ref('')
const history = ref([])
const revisionNo = ref('')
const versionId = ref('')
const impact = ref(null)
const busy = ref(false)
const error = ref('')
const message = ref('')

const sampleCsv = 'code,title,content\nDEMO-01,登录成功,使用有效账号登录后进入工作台\nDEMO-02,登录失败,密码错误时显示提示'
const projectRows = computed(() => rows.value.filter((row) => row.project_id === projectId.value))
const projectVersions = computed(() => versions.value.filter((version) => version.project_id === projectId.value))
const historyTitle = computed(() => projectRows.value.find((row) => row.id === historyId.value)?.title)
const selectedRevision = computed(() => history.value.find((revision) => revision.revision_no === revisionNo.value))
const requirementPath = (id) => `/api/requirements/${encodeURIComponent(id)}`

async function refreshRows() {
  rows.value = await api.get('/api/requirements')
}

async function run(action) {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await action()
  } catch (err) {
    error.value = err.message
  } finally {
    busy.value = false
  }
}

function resetForm() {
  editingId.value = ''
  form.value = { code: '', title: '', content: '' }
}

function resetImport() {
  file.value = null
  useSample.value = false
  if (fileInput.value) fileInput.value.value = ''
}

function changeProject() {
  resetForm()
  resetImport()
  historyId.value = ''
  history.value = []
  revisionNo.value = ''
  versionId.value = ''
  impact.value = null
  error.value = ''
  message.value = ''
}

function edit(row) {
  editingId.value = row.id
  form.value = { code: row.code ?? '', title: row.title, content: row.content ?? '' }
}

async function loadHistory(id) {
  history.value = await api.get(`${requirementPath(id)}/history`)
  historyId.value = id
  revisionNo.value = history.value.at(-1)?.revision_no ?? ''
}

function save() {
  return run(async () => {
    if (!form.value.title) throw new Error('请输入标题')
    const id = editingId.value
    if (id) {
      await api.patch(requirementPath(id), form.value)
    } else {
      await api.post('/api/requirements', { ...form.value, project_id: projectId.value })
    }
    resetForm()
    await refreshRows()
    if (id && historyId.value === id) await loadHistory(id)
    message.value = id ? '已保存修订' : '已录入需求'
  })
}

function importRequirements() {
  return run(async () => {
    let filename = '需求示例.csv'
    let text = sampleCsv
    if (!useSample.value) {
      const selectedFile = file.value
      if (!selectedFile) throw new Error('请选择文件或载入示例 CSV')
      if (selectedFile.size > 1024 * 1024) throw new Error('文件不能超过 1 MiB')
      filename = selectedFile.name
      const bytes = await selectedFile.arrayBuffer()
      try {
        text = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
      } catch {
        throw new Error('文件必须使用 UTF-8 编码')
      }
    }
    const { ids } = await api.post('/api/requirements/import', { project_id: projectId.value, filename, text })
    resetImport()
    await refreshRows()
    message.value = `已导入 ${ids.length} 条需求`
  })
}

function linkRevision() {
  return run(async () => {
    await api.put(`${requirementPath(historyId.value)}/revisions/${revisionNo.value}/versions/${encodeURIComponent(versionId.value)}`)
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
  projectId.value = projectList[0]?.id ?? ''
}))
</script>

<template>
  <header class="head">
    <div class="field">
      <label for="requirement-project">项目</label>
      <select id="requirement-project" v-model="projectId" :disabled="busy || !projects.length" @change="changeProject">
        <option v-if="!projects.length" value="">暂无项目</option>
        <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
      </select>
    </div>
  </header>
  <p v-if="error" class="error" role="alert">{{ error }}</p>
  <p v-if="message" role="status">{{ message }}</p>
  <section v-if="projectId" class="panel">
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
      <button v-if="editingId" type="button" :disabled="busy" @click="resetForm">取消</button>
    </form>
  </section>
  <section v-if="projectId" class="panel">
    <h2 id="requirement-import-heading">文件导入</h2>
    <form class="form" aria-labelledby="requirement-import-heading" @submit.prevent="importRequirements">
      <div class="field">
        <label for="requirement-file">CSV / Markdown / TXT（UTF-8）</label>
        <input id="requirement-file" ref="fileInput" name="file" type="file" accept=".csv,.md,.markdown,.txt" :disabled="busy"
          @change="file = $event.target.files[0]; useSample = false" />
      </div>
      <button type="button" :disabled="busy" @click="resetImport(); useSample = true">载入示例 CSV</button>
      <button class="primary" type="submit" :disabled="busy">{{ useSample ? '导入示例' : '导入文件' }}</button>
    </form>
    <pre v-if="useSample">{{ sampleCsv }}</pre>
  </section>
  <DataTable
    :rows="projectRows"
    :columns="[
      { key: 'code', label: '编号', width: '12%' },
      { key: 'title', label: '标题', width: '26%' },
      { key: 'content', label: '内容' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'actions', label: '操作', width: '18%' },
    ]"
    empty="还没有需求"
  >
    <template #content="{ row }"><div class="text">{{ row.content }}</div></template>
    <template #actions="{ row }">
      <div class="actions">
        <button type="button" :disabled="busy" @click="edit(row)">编辑</button>
        <button type="button" :disabled="busy" @click="run(() => loadHistory(row.id))">历史</button>
        <button type="button" :disabled="busy" @click="emit('generate-cases', { projectId: row.project_id, requirementId: row.id, versionId: versionId || undefined })">生成用例</button>
      </div>
    </template>
  </DataTable>
  <section v-if="historyId" class="panel">
    <h2>修订历史 · {{ historyTitle }}</h2>
    <div class="field">
      <label for="requirement-revision">修订</label>
      <select id="requirement-revision" v-model="revisionNo" :disabled="busy || !history.length">
        <option v-for="revision in history" :key="revision.revision_no" :value="revision.revision_no">
          第 {{ revision.revision_no }} 版 · {{ revision.created_at }}
        </option>
      </select>
    </div>
    <template v-if="selectedRevision">
      <p>{{ selectedRevision.code }} {{ selectedRevision.title }}</p>
      <div class="text">{{ selectedRevision.content }}</div>
    </template>
    <p v-else>暂无修订</p>
  </section>
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
      <button v-if="selectedRevision" type="button" :disabled="busy || !versionId" @click="linkRevision">关联所选修订</button>
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
.head { margin-bottom: 16px; }
.panel { margin: 16px 0; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }
h2 { margin: 0 0 12px; font-size: 16px; }
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
