<script setup>
import { computed, nextTick, onMounted, ref, useId } from 'vue'
import { api } from '../api'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ projectId: { type: String, default: '' } })
const emit = defineEmits(['open-run'])
const dialogTitle = useId()
const dialog = ref(null)
const rows = ref([])
const projects = ref([])
const versions = ref([])
const selection = ref('')
const bug = ref(null)
const filters = ref({ project_id: props.projectId, status: '', severity: '', q: '' })
const loading = ref(true)
const detailLoading = ref(false)
const saving = ref(false)
const listError = ref('')
const detailError = ref('')
const formError = ref('')
const editor = ref('')
const form = ref({})
const transition = ref({})
const nextAction = ref('')
let listRequest = 0
let detailRequest = 0

const STATUSES = {
  new: '新建', confirmed: '已确认', fixing: '修复中', fixed: '已修复',
  verified: '已验证', closed: '已关闭', reopened: '重新打开', rejected: '已拒绝',
}
const ACTIONS = {
  confirmed: '确认缺陷', fixing: '开始修复', fixed: '标记已修复',
  verified: '验证通过', closed: '关闭缺陷', reopened: '重新打开', rejected: '拒绝缺陷',
}
const SEVERITIES = { S1: '致命', S2: '严重', S3: '一般', S4: '轻微' }
const PRIORITIES = ['P0', 'P1', 'P2', 'P3']
const DESCRIPTION = {
  repro_steps: '复现步骤', expected_result: '预期结果', actual_result: '实际结果', evidence: '补充证据',
}
const projectNames = computed(() => Object.fromEntries(projects.value.map(({ id, name }) => [id, name])))
const scopedVersions = computed(() => versions.value.filter(({ project_id }) =>
  project_id === (editor.value === 'transition' ? bug.value?.project_id : form.value.project_id)))
const heading = computed(() => ({
  create: '提交缺陷', edit: '编辑缺陷', transition: ACTIONS[transition.value.to],
}[editor.value]))
const bugPath = (id) => `/api/bugs/${encodeURIComponent(id)}`

async function selectBug(id) {
  const request = ++detailRequest
  selection.value = id
  bug.value = null
  detailError.value = ''
  detailLoading.value = Boolean(id)
  if (!id) return
  try {
    const detail = await api.get(bugPath(id))
    if (request === detailRequest) {
      bug.value = detail
      nextAction.value = detail.available_transitions.length === 1 ? detail.available_transitions[0] : ''
    }
  } catch (problem) {
    if (request === detailRequest) detailError.value = problem.message
  } finally {
    if (request === detailRequest) detailLoading.value = false
  }
}

async function loadList(preferredID = selection.value) {
  const request = ++listRequest
  loading.value = true
  listError.value = ''
  const params = new URLSearchParams(Object.entries(filters.value).filter(([, value]) => value))
  try {
    const result = await api.get(`/api/bugs?${params}`)
    if (request !== listRequest) return
    rows.value = result
    selectBug(result.some(({ id }) => id === preferredID) ? preferredID : result[0]?.id || '')
  } catch (problem) {
    if (request === listRequest) listError.value = problem.message
  } finally {
    if (request === listRequest) loading.value = false
  }
}

async function openEditor(mode) {
  formError.value = ''
  editor.value = mode
  if (mode === 'transition') {
    transition.value = { to: nextAction.value, note: '', fixed_version_id: '' }
  } else {
    const source = mode === 'edit' ? bug.value : null
    form.value = {
      project_id: source?.project_id || filters.value.project_id || projects.value[0]?.id || '',
      title: source?.title || '', severity: source?.severity || 'S3', priority: source?.priority || 'P2',
      assignee: source?.assignee || '', discovered_version_id: source?.discovered_version_id || '',
      ...Object.fromEntries(Object.keys(DESCRIPTION).map((key) => [key, source?.[key] || ''])),
    }
  }
  await nextTick()
  dialog.value.showModal()
}

async function save() {
  if (editor.value === 'transition' && !transition.value.note.trim()) return
  saving.value = true
  formError.value = ''
  try {
    let id = bug.value?.id
    if (editor.value === 'create') {
      ({ id } = await api.post('/api/bugs', form.value))
      filters.value = { project_id: form.value.project_id, status: '', severity: '', q: '' }
    } else if (editor.value === 'edit') {
      const { project_id, ...changes } = form.value
      await api.patch(bugPath(id), changes)
    } else {
      const { to, note, fixed_version_id } = transition.value
      await api.post(`${bugPath(id)}/transition`, { to, note, ...(to === 'fixed' && { fixed_version_id }) })
    }
    dialog.value.close()
    await loadList(id)
  } catch (problem) {
    formError.value = problem.message
  } finally {
    saving.value = false
  }
}

async function retest(id) {
  saving.value = true
  detailError.value = ''
  try {
    const { run_id } = await api.post(`${bugPath(id)}/retest`, {})
    emit('open-run', run_id)
  } catch (problem) {
    if (selection.value === id) detailError.value = problem.message
  } finally {
    saving.value = false
  }
}

async function refresh() {
  await loadList()
  try {
    [projects.value, versions.value] = await Promise.all([api.get('/api/projects'), api.get('/api/versions')])
  } catch (problem) {
    listError.value = problem.message
  }
}

onMounted(refresh)
</script>

<template>
  <div class="bug-workspace">
    <header class="toolbar">
      <form class="filters" aria-label="筛选缺陷" @submit.prevent="loadList()">
        <select v-if="!projectId" v-model="filters.project_id" aria-label="项目" @change="loadList()">
          <option value="">全部项目</option>
          <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
        </select>
        <select v-model="filters.status" aria-label="状态" @change="loadList()">
          <option value="">全部状态</option>
          <option v-for="(label, status) in STATUSES" :key="status" :value="status">{{ label }}</option>
        </select>
        <select v-model="filters.severity" aria-label="严重级别" @change="loadList()">
          <option value="">全部级别</option>
          <option v-for="(label, level) in SEVERITIES" :key="level" :value="level">{{ level }} {{ label }}</option>
        </select>
        <div class="filter-search">
          <input v-model.trim="filters.q" type="search" aria-label="搜索编号或标题" placeholder="搜索编号或标题" />
          <button type="submit">搜索</button>
        </div>
      </form>
      <div class="toolbar-actions">
        <button type="button" :disabled="loading" @click="refresh">刷新</button>
        <button type="button" class="primary" :disabled="!projects.length || saving" @click="openEditor('create')">提交缺陷</button>
      </div>
    </header>
    <p v-if="listError" class="error" role="alert">{{ listError }}</p>

    <div class="bug-board">
      <section class="bug-list" aria-label="缺陷列表" :aria-busy="loading">
        <header class="list-heading"><h2>缺陷列表</h2><span>{{ rows.length }}</span></header>
        <p v-if="loading" class="empty" role="status">正在加载缺陷…</p>
        <p v-else-if="!rows.length && !listError" class="empty">没有符合条件的缺陷</p>
        <ul v-else class="items">
          <li v-for="row in rows" :key="row.id">
            <button type="button" class="bug-row" :aria-current="selection === row.id ? 'true' : undefined" @click="selectBug(row.id)">
              <span class="row-meta"><code>{{ row.bug_code }}</code><StatusTag :value="row.status" /></span>
              <strong>{{ row.title }}</strong>
              <span class="row-meta"><span>{{ projectNames[row.project_id] || row.project_id }}</span><span class="level" :data-level="row.severity">{{ row.severity }}</span></span>
            </button>
          </li>
        </ul>
      </section>

      <section class="bug-detail" aria-label="缺陷详情" :aria-busy="detailLoading || loading">
        <p v-if="detailLoading || loading" class="empty" role="status">正在加载详情…</p>
        <template v-else>
          <p v-if="detailError" class="error" role="alert">{{ detailError }}</p>
          <template v-if="bug">
            <header class="detail-heading">
              <div class="detail-kicker"><code>{{ bug.bug_code }}</code><StatusTag :value="bug.status" /></div>
              <h2>{{ bug.title }}</h2>
              <div class="detail-actions">
                <template v-if="bug.available_transitions.length">
                  <select v-model="nextAction" aria-label="选择状态操作">
                    <option value="" disabled>选择状态操作</option>
                    <option v-for="action in bug.available_transitions" :key="action" :value="action">{{ ACTIONS[action] }}</option>
                  </select>
                  <button type="button" class="primary" :disabled="!nextAction || saving" @click="openEditor('transition')">继续</button>
                </template>
                <button type="button" :disabled="saving" @click="openEditor('edit')">编辑缺陷</button>
                <template v-if="bug.run_id">
                  <button type="button" :disabled="saving" @click="retest(bug.id)">复测</button>
                  <button type="button" @click="emit('open-run', bug.run_id)">执行记录</button>
                </template>
              </div>
            </header>

            <dl class="metadata">
              <div><dt>项目</dt><dd>{{ bug.project_name || '—' }}</dd></div>
              <div><dt>处理人</dt><dd>{{ bug.assignee || '未指派' }}</dd></div>
              <div><dt>级别 / 优先级</dt><dd><span class="level" :data-level="bug.severity">{{ bug.severity }}</span> / {{ bug.priority }}</dd></div>
              <div><dt>发现版本</dt><dd>{{ bug.discovered_version_name || '—' }}</dd></div>
              <div><dt>最近修复版本</dt><dd>{{ bug.fixed_version_name || '—' }}</dd></div>
              <div><dt>复开次数</dt><dd>{{ bug.reopen_count }}</dd></div>
            </dl>

            <div class="detail-content">
              <div class="description">
                <section v-for="(label, field) in DESCRIPTION" v-show="field !== 'evidence' || bug.evidence" :key="field">
                  <h3>{{ label }}</h3>
                  <p>{{ bug[field] || '未填写' }}</p>
                </section>
              </div>
              <section class="history">
                <h3>流转历史</h3>
                <p v-if="!bug.history.length" class="muted">暂无流转记录</p>
                <ol v-else>
                  <li v-for="event in bug.history" :key="event.id">
                    <strong v-if="!event.from_status">创建缺陷</strong>
                    <strong v-else-if="event.from_status === event.to_status">编辑资料</strong>
                    <strong v-else>{{ STATUSES[event.from_status] || event.from_status }} → {{ STATUSES[event.to_status] || event.to_status }}</strong>
                    <div class="event-meta"><span>{{ event.actor || '—' }}</span><time :datetime="event.at.replace(' ', 'T') + 'Z'">{{ event.at }} UTC</time></div>
                    <p v-if="event.note">{{ event.note }}</p>
                  </li>
                </ol>
              </section>
            </div>
          </template>
          <button v-else-if="detailError" type="button" @click="selectBug(selection)">重新加载</button>
          <p v-else class="empty">选择一条缺陷查看详情</p>
        </template>
      </section>
    </div>

    <dialog ref="dialog" class="bug-dialog" :aria-labelledby="dialogTitle" @cancel="saving && $event.preventDefault()">
      <form @submit.prevent="save">
        <header><h2 :id="dialogTitle">{{ heading }}</h2><code v-if="editor !== 'create'">{{ bug?.bug_code }}</code></header>
        <template v-if="editor === 'transition'">
          <div class="transition-label"><StatusTag :value="bug?.status" /><span aria-hidden="true">→</span><StatusTag :value="transition.to" /></div>
          <label v-if="transition.to === 'fixed'">修复版本<select v-model="transition.fixed_version_id">
            <option value="">不关联版本</option>
            <option v-for="version in scopedVersions" :key="version.id" :value="version.id">{{ version.name }}</option>
          </select></label>
          <label>操作备注<textarea v-model.trim="transition.note" rows="4" required autofocus /></label>
        </template>
        <template v-else>
          <div class="form-grid">
            <label v-if="editor === 'create' && !projectId">项目<select v-model="form.project_id" required @change="form.discovered_version_id = ''">
              <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
            </select></label>
            <label>发现版本<select v-model="form.discovered_version_id">
              <option value="">不关联版本</option>
              <option v-for="version in scopedVersions" :key="version.id" :value="version.id">{{ version.name }}</option>
            </select></label>
            <label class="full">标题<input v-model.trim="form.title" required autofocus /></label>
            <label>处理人<input v-model.trim="form.assignee" placeholder="姓名或团队" /></label>
            <label>级别<select v-model="form.severity">
              <option v-for="(label, level) in SEVERITIES" :key="level" :value="level">{{ level }} {{ label }}</option>
            </select></label>
            <label>优先级<select v-model="form.priority"><option v-for="priority in PRIORITIES" :key="priority">{{ priority }}</option></select></label>
          </div>
          <label v-for="(label, field) in DESCRIPTION" :key="field">{{ label }}<textarea v-model="form[field]" :rows="field === 'repro_steps' ? 4 : 2" /></label>
        </template>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer>
          <button type="button" :disabled="saving" @click="dialog.close()">取消</button>
          <button type="submit" class="primary" :disabled="saving || (editor === 'transition' && !transition.note)">
            {{ saving ? '保存中…' : editor === 'transition' ? '确认操作' : '保存缺陷' }}
          </button>
        </footer>
      </form>
    </dialog>
  </div>
</template>

<style scoped>
.bug-workspace { container-type: inline-size; }
.toolbar, .filters, .filter-search, .toolbar-actions, .detail-kicker, .detail-actions, .row-meta, .transition-label {
  display: flex; flex-wrap: wrap; align-items: center; gap: 8px;
}
.toolbar { justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.toolbar-actions { margin-inline-start: auto; }
.filter-search { flex-wrap: nowrap; }
.filter-search input { inline-size: min(13rem, 52vw); }
select, textarea {
  min-inline-size: 0; max-inline-size: 100%; min-block-size: 34px; padding: 6px 9px;
  border: 1px solid var(--line); border-radius: 7px; background: var(--panel); color: var(--text); font: inherit;
}
.filters select { max-inline-size: 14rem; }
.bug-board { display: grid; grid-template-columns: minmax(15rem, .65fr) minmax(0, 1.75fr); gap: 14px; align-items: start; }
.bug-list, .bug-detail {
  min-inline-size: 0; border: 1px solid var(--line); border-radius: 13px; background: var(--panel); overflow-wrap: anywhere;
}
.bug-list { background: color-mix(in srgb, var(--bg) 48%, var(--panel)); }
.list-heading { display: flex; justify-content: space-between; align-items: center; padding: 13px 15px; border-bottom: 1px solid var(--line); }
.list-heading h2, .list-heading span { margin: 0; font-size: 13px; font-weight: 600; }
.list-heading span, .muted, code, .empty, .event-meta { color: var(--muted); }
.items { list-style: none; padding: 6px; margin: 0; max-block-size: 68vh; overflow: auto; }
.items li + li { border-top: 1px solid var(--line); }
.bug-row {
  display: grid; gap: 5px; inline-size: 100%; border: 1px solid transparent; border-inline-start: 3px solid transparent;
  border-radius: 7px; margin: 2px 0; padding: 9px 10px; background: transparent; text-align: start; overflow-wrap: anywhere;
}
.bug-row[aria-current] { background: var(--accent-soft, #eef2ff); border-inline-start-color: var(--accent); }
.bug-row:hover { background: var(--bg); }
.bug-row[aria-current]:hover { background: var(--accent-soft, #eef2ff); }
.row-meta { justify-content: space-between; color: var(--muted); font-size: 12px; }
.level { font-size: 12px; font-weight: 700; }
.level[data-level='S1'], .level[data-level='S2'] { color: var(--bad); }
.bug-detail { padding: 19px 22px; min-block-size: 19rem; }
.detail-heading { padding-block-end: 15px; }
.detail-heading h2 { margin: 7px 0 13px; font-size: 23px; line-height: 1.28; }
.metadata {
  display: grid; grid-template-columns: repeat(auto-fit, minmax(7rem, 1fr)); gap: 12px 15px;
  margin: 0 0 19px; padding: 13px; border-radius: 8px; background: var(--bg);
}
.metadata dt { color: var(--muted); font-size: 12px; }
.metadata dd { margin: 3px 0 0; font-weight: 600; }
.detail-content { display: grid; grid-template-columns: minmax(0, 1.35fr) minmax(12rem, 1fr); gap: 17px; }
.description section + section { margin-top: 13px; }
.description h3, .history h3 { margin: 0 0 5px; font-size: 14px; }
.description p, .history p { margin: 0; white-space: pre-wrap; }
.description p { padding: 9px 11px; border: 1px solid var(--line); border-radius: 8px; }
.history { border-inline-start: 1px solid var(--line); padding-inline-start: 15px; }
.history ol { list-style: none; margin: 11px 0 0 4px; padding: 0; }
.history li { position: relative; border-inline-start: 1px solid var(--line); padding: 0 0 14px 12px; }
.history li:last-child { border-color: transparent; padding-bottom: 0; }
.history li::before { content: ''; position: absolute; inset: 6px auto auto -4px; inline-size: 7px; block-size: 7px; border-radius: 50%; background: var(--accent); }
.event-meta { display: flex; flex-wrap: wrap; gap: 2px 8px; font-size: 12px; margin: 2px 0; }
.empty { padding: 15px; margin: 0; }
.error { color: var(--bad); }
.bug-dialog {
  inline-size: min(40rem, calc(100% - 24px)); max-block-size: calc(100dvh - 32px); padding: 0;
  border: 1px solid var(--line); border-radius: 13px; color: var(--text); background: var(--panel);
}
.bug-dialog:modal { overscroll-behavior: contain; }
.bug-dialog::backdrop { background: #111d40aa; overflow: hidden; overscroll-behavior: contain; }
.bug-dialog form { display: grid; padding: 18px; gap: 10px; }
.bug-dialog header, .bug-dialog footer { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
.bug-dialog h2 { margin: 0; font-size: 19px; }
.bug-dialog footer { justify-content: flex-end; }
.bug-dialog label { display: grid; min-inline-size: 0; gap: 4px; }
.bug-dialog textarea { resize: vertical; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
.full { grid-column: 1 / -1; }
@container (max-width: 53rem) {
  .detail-content { grid-template-columns: minmax(0, 1fr); }
  .history { border-inline-start: 0; padding-inline-start: 0; border-top: 1px solid var(--line); padding-top: 13px; }
}
@container (max-width: 39rem) {
  .bug-board { grid-template-columns: minmax(0, 1fr); }
  .items { max-block-size: 16rem; }
  .bug-detail { padding: 12px; }
}
</style>
