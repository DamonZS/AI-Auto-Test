<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'

const props = defineProps({ projectId: { type: String, default: '' } })
const emit = defineEmits(['open-project', 'open-project-view', 'projects-changed'])
const rows = ref([])
const query = ref('')
const lifecycleFilter = ref('')
const selectedId = ref(props.projectId)
const detail = ref(null)
const form = ref(null)
const editing = ref(false)
const loading = ref(true)
const detailLoading = ref(false)
const saving = ref(false)
const error = ref('')
const detailError = ref('')
const notice = ref('')
const lifecycleLabels = { planning: '规划中', active: '进行中', completed: '已完成', archived: '已归档' }

const destinations = [
  { view: 'requirements', label: '需求', metric: 'requirements' },
  { view: 'versions', label: '版本', metric: 'versions' },
  { view: 'environments', label: '环境', metric: 'environments' },
  { view: 'cases', label: '用例', metric: 'cases' },
  { view: 'runs', label: '执行', metric: 'runs' },
  { view: 'bugs', label: '未关闭缺陷', metric: 'open_bugs' },
]
const filtered = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return rows.value.filter((row) => (!lifecycleFilter.value || row.lifecycle === lifecycleFilter.value)
    && [row.name, row.code, row.lead, row.base_url, row.allowed_hosts].some((value) => value?.toLowerCase().includes(keyword)))
})
const detailPath = computed(() => selectedId.value ? `/api/projects/${encodeURIComponent(selectedId.value)}` : '')
const formPath = computed(() => editing.value ? detailPath.value : '/api/projects')

async function load() {
  loading.value = true
  try {
    rows.value = await api.get('/api/projects')
  } catch (failure) {
    error.value = failure.message
  } finally {
    loading.value = false
  }
}

function selectProject(id) {
  form.value = null
  notice.value = ''
  selectedId.value = id
}

function openForm(project = {}) {
  editing.value = Boolean(project.id)
  form.value = {
    name: project.name || '', base_url: project.base_url || '', allowed_hosts: project.allowed_hosts || '',
    code: project.code || '', lead: project.lead || '', description: project.description || '', lifecycle: project.lifecycle || 'active',
  }
  error.value = ''
  notice.value = ''
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    if (editing.value) {
      const project = detail.value
      const changes = Object.fromEntries(Object.entries(form.value).filter(([field, value]) => project[field] !== value))
      await api.patch(formPath.value, changes)
      if (selectedId.value === project.id) detail.value = { ...project, ...changes }
    } else {
      const { id } = await api.post(formPath.value, form.value)
      selectedId.value = id
      query.value = ''
      lifecycleFilter.value = ''
    }
    form.value = null
    notice.value = editing.value ? '项目已更新' : '项目已创建'
    emit('projects-changed')
    await load()
  } catch (failure) {
    error.value = failure.message
  } finally {
    saving.value = false
  }
}

watch(() => props.projectId, selectProject)
watch(detailPath, async (path, _, onCleanup) => {
  let active = true
  onCleanup(() => { active = false })
  detail.value = null
  detailError.value = ''
  detailLoading.value = Boolean(path)
  if (!path) return
  try {
    const project = await api.get(path)
    if (active) detail.value = project
  } catch (failure) {
    if (active) detailError.value = failure.message
  } finally {
    if (active) detailLoading.value = false
  }
}, { immediate: true })
onMounted(load)
</script>

<template>
  <div class="hub">
    <header class="hub-toolbar">
      <search class="hub-field hub-grow">
        <label for="project-search">搜索项目</label>
        <input id="project-search" v-model="query" name="query" type="search" placeholder="名称、代号或负责人" />
      </search>
      <label class="hub-field">项目状态
        <select v-model="lifecycleFilter"><option value="">全部状态</option><option v-for="(label, value) in lifecycleLabels" :key="value" :value="value">{{ label }}</option></select>
      </label>
      <button type="button" class="primary" :disabled="saving" @click="openForm()">新建项目</button>
    </header>

    <form v-if="form" class="hub-panel hub-form" :action="formPath" method="post" aria-labelledby="project-form-title" @submit.prevent="save">
      <h3 id="project-form-title">{{ editing ? '编辑项目' : '新建项目' }}</h3>
      <div class="hub-fields">
        <label class="hub-field" for="project-name">项目名称 *
          <input id="project-name" v-model.trim="form.name" name="name" required />
        </label>
        <label class="hub-field" for="project-url">目标地址 *
          <input id="project-url" v-model.trim="form.base_url" name="base_url" type="url" placeholder="https://…" required />
        </label>
        <label class="hub-field" for="project-hosts">白名单主机（逗号分隔）
          <input id="project-hosts" v-model.trim="form.allowed_hosts" name="allowed_hosts" />
        </label>
        <label class="hub-field">项目代号<input v-model.trim="form.code" name="code" maxlength="40" /></label>
        <label class="hub-field">负责人<input v-model.trim="form.lead" name="lead" maxlength="80" /></label>
        <label class="hub-field">项目状态
          <select v-model="form.lifecycle" name="lifecycle"><option v-for="(label, value) in lifecycleLabels" :key="value" :value="value">{{ label }}</option></select>
        </label>
      </div>
      <label class="hub-field project-description">项目说明<textarea v-model.trim="form.description" name="description" rows="3" maxlength="2000" /></label>
      <div class="hub-actions">
        <button type="submit" class="primary" :disabled="saving">{{ saving ? '保存中…' : '保存项目' }}</button>
        <button type="button" :disabled="saving" @click="form = null">取消</button>
      </div>
    </form>
    <p v-if="error" class="hub-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="hub-notice" role="status">{{ notice }}</p>

    <div class="project-layout" :class="{ split: selectedId }">
      <section class="project-list" aria-label="项目列表">
        <p v-if="loading" role="status">正在读取项目…</p>
        <p v-else-if="!filtered.length">{{ query.trim() || lifecycleFilter ? '没有匹配的项目' : '还没有项目' }}</p>
        <ul v-else>
          <li v-for="row in filtered" :key="row.id">
            <button type="button" class="project-row" :aria-pressed="selectedId === row.id" :disabled="saving" @click="selectProject(row.id)">
              <strong>{{ row.name }}</strong>
              <span>{{ [row.code, lifecycleLabels[row.lifecycle], row.lead].filter(Boolean).join(' · ') }}</span>
            </button>
          </li>
        </ul>
      </section>

      <section v-if="selectedId" class="hub-panel project-detail" aria-label="项目详情" :aria-busy="detailLoading">
        <p v-if="detailLoading" role="status">正在读取项目详情…</p>
        <p v-else-if="detailError" class="hub-error" role="alert">{{ detailError }}</p>
        <template v-else-if="detail">
          <header class="hub-toolbar">
            <h3 class="hub-grow">{{ detail.name }}</h3>
            <button type="button" :disabled="saving" @click="openForm(detail)">编辑</button>
            <button type="button" :disabled="saving" @click="emit('open-project', detail.id)">进入项目</button>
          </header>
          <dl class="project-meta">
            <dt>状态</dt><dd>{{ lifecycleLabels[detail.lifecycle] }}</dd>
            <dt>代号</dt><dd>{{ detail.code || '—' }}</dd>
            <dt>负责人</dt><dd>{{ detail.lead || '—' }}</dd>
            <dt>目标地址</dt><dd>{{ detail.base_url }}</dd>
            <dt>白名单</dt><dd>{{ detail.allowed_hosts || '未设置' }}</dd>
            <dt>创建时间</dt><dd>{{ detail.created_at }}</dd>
          </dl>
          <p v-if="detail.description" class="project-note">{{ detail.description }}</p>
          <div class="project-progress">
            <span id="project-progress-label">用例审批进度</span>
            <template v-if="detail.metrics.cases">
              <strong>{{ detail.metrics.approved_cases }} / {{ detail.metrics.cases }}</strong>
              <progress aria-labelledby="project-progress-label" :value="detail.metrics.approved_cases" :max="detail.metrics.cases" />
            </template>
            <span v-else>暂无用例</span>
          </div>
          <nav class="project-metrics" aria-label="项目页面">
            <button v-for="item in destinations" :key="item.view" type="button"
              @click="emit('open-project-view', { projectId: detail.id, view: item.view })">
              <span>{{ item.label }}</span>
              <strong>{{ detail.metrics[item.metric] }}</strong>
              <small v-if="item.view === 'runs'">{{ detail.metrics.passed_runs }} 次通过</small>
            </button>
          </nav>
        </template>
      </section>
    </div>
  </div>
</template>

<style scoped src="./project-hubs.css"></style>
<style scoped>
.project-layout { display: grid; gap: 14px; }
.project-layout.split { grid-template-columns: minmax(0, 16rem) minmax(0, 1fr); align-items: start; }
.project-list { min-inline-size: 0; }
.project-list p { margin: 0; color: var(--muted); }
.project-list ul { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 16rem), 16rem)); gap: 10px; list-style: none; padding: 0; margin: 0; }
.project-row { display: grid; align-content: start; gap: 4px; inline-size: 100%; block-size: 100%; padding: 10px 12px; text-align: start; overflow-wrap: anywhere; }
.project-row span { color: var(--muted); }
.project-row[aria-pressed='true'] { border-color: var(--accent); background: #eef1fe; }
.project-detail { display: grid; gap: 16px; }
.project-note { margin: 0; white-space: pre-wrap; color: var(--muted); }
.project-description textarea { font: inherit; border: 1px solid var(--line); border-radius: 8px; padding: 8px; resize: vertical; }
.project-meta { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 4px 14px; margin: 0; }
.project-meta dt { color: var(--muted); }
.project-meta dd { margin: 0; overflow-wrap: anywhere; }
.project-progress { display: grid; grid-template-columns: 1fr auto; gap: 4px 12px; }
.project-progress progress { grid-column: 1 / -1; width: 100%; accent-color: var(--accent); }
.project-metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(110px, 1fr)); gap: 8px; }
.project-metrics button { display: flex; flex-direction: column; align-items: start; }
.project-metrics strong { font-size: 19px; }
.project-metrics small { color: var(--muted); }
@media (max-width: 820px) { .project-layout.split { grid-template-columns: minmax(0, 1fr); } }
</style>
