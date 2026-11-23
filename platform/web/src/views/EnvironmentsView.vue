<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const props = defineProps({ projectId: { type: String, default: '' } })
const rows = ref([])
const projects = ref([])
const versions = ref([])
const projectId = ref(props.projectId)
const levelFilter = ref('all')
const form = ref(null)
const editing = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

const levels = { '': '未分类', development: '开发', test: '测试', staging: '预发', production: '生产' }
const sources = { platform: '平台选定', mcp: 'MCP 提交' }
const visibleRows = computed(() => rows.value.filter((row) => row.project_id === projectId.value &&
  (levelFilter.value === 'all' || (row.level || '') === levelFilter.value)))
const projectVersions = computed(() => versions.value.filter((version) => version.project_id === projectId.value))
const formPath = computed(() => editing.value
  ? `/api/environments/${encodeURIComponent(editing.value.id)}`
  : `/api/projects/${encodeURIComponent(projectId.value)}/environments`)

async function load() {
  loading.value = true
  try {
    const [projectRows, environmentRows, versionRows] = await Promise.all([
      api.get('/api/projects'), api.get('/api/environments'), api.get('/api/versions'),
    ])
    projects.value = projectRows
    rows.value = environmentRows
    versions.value = versionRows
    if (!projectId.value) projectId.value = props.projectId || projectRows[0]?.id || ''
  } catch (failure) {
    error.value = failure.message
  } finally {
    loading.value = false
  }
}

function openForm(row = null) {
  editing.value = row
  form.value = {
    name: row?.name ?? '', base_url: row?.base_url ?? '', source: row?.source ?? 'platform',
    level: row?.level ?? '', version_id: row?.version_id ?? '',
  }
  notice.value = ''
  error.value = ''
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const original = editing.value
    const values = form.value
    if (original) {
      const changes = Object.fromEntries(Object.entries(values).filter(([field, value]) => (original[field] ?? '') !== value))
      await api.patch(formPath.value, changes)
    } else {
      await api.post(formPath.value, values)
    }
    if (levelFilter.value !== 'all') levelFilter.value = values.level
    form.value = null
    notice.value = original ? '环境已更新' : '环境已创建'
    await load()
  } catch (failure) {
    error.value = failure.message
  } finally {
    saving.value = false
  }
}

watch(() => props.projectId, (id) => { projectId.value = id || projects.value[0]?.id || '' })
watch(projectId, () => {
  form.value = null
  levelFilter.value = 'all'
  error.value = ''
  notice.value = ''
})
onMounted(load)
</script>

<template>
  <div class="hub">
    <header class="hub-toolbar">
      <label v-if="!props.projectId" class="hub-field hub-grow" for="environment-project">项目
        <select id="environment-project" v-model="projectId" name="project_id" :disabled="loading || saving">
          <option v-if="!projects.length" value="">暂无项目</option>
          <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
        </select>
      </label>
      <label class="hub-field hub-grow" for="environment-filter">环境级别
        <select id="environment-filter" v-model="levelFilter" name="level_filter">
          <option value="all">全部</option>
          <option v-for="(label, level) in levels" :key="level" :value="level">{{ label }}</option>
        </select>
      </label>
      <button type="button" class="primary" :disabled="loading || saving || !projects.some((project) => project.id === projectId)" @click="openForm()">新建环境</button>
    </header>
    <form v-if="form" class="hub-panel hub-form" :action="formPath" method="post" aria-labelledby="environment-form-title" @submit.prevent="save">
      <h3 id="environment-form-title">{{ editing ? '编辑环境' : '新建环境' }}</h3>
      <div class="hub-fields">
        <label class="hub-field" for="environment-name">环境名称 *
          <input id="environment-name" v-model.trim="form.name" name="name" required />
        </label>
        <label class="hub-field" for="environment-url">测试地址 *
          <input id="environment-url" v-model.trim="form.base_url" name="base_url" type="url" placeholder="https://…" required />
        </label>
        <label class="hub-field" for="environment-source">来源
          <select id="environment-source" v-model="form.source" name="source">
            <option v-if="!(form.source in sources)" :value="form.source">{{ form.source }}</option>
            <option v-for="(label, source) in sources" :key="source" :value="source">{{ label }}</option>
          </select>
        </label>
        <label class="hub-field" for="environment-level">环境级别
          <select id="environment-level" v-model="form.level" name="level">
            <option v-for="(label, level) in levels" :key="level" :value="level">{{ label }}</option>
          </select>
        </label>
        <label class="hub-field" for="environment-version">部署版本
          <select id="environment-version" v-model="form.version_id" name="version_id">
            <option value="">未关联</option>
            <option v-for="version in projectVersions" :key="version.id" :value="version.id">{{ version.name }}</option>
          </select>
        </label>
      </div>
      <div class="hub-actions">
        <button type="submit" class="primary" :disabled="saving">{{ saving ? '保存中…' : '保存环境' }}</button>
        <button type="button" :disabled="saving" @click="form = null">取消</button>
      </div>
    </form>
    <p v-if="error" class="hub-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="hub-notice" role="status">{{ notice }}</p>
    <p v-if="loading" role="status">正在读取环境…</p>
    <DataTable v-else
      :rows="visibleRows"
      :columns="[
        { key: 'name', label: '环境' },
        { key: 'level', label: '级别' },
        { key: 'version_id', label: '部署版本' },
        { key: 'base_url', label: '测试地址' },
        { key: 'source', label: '来源' },
        { key: 'status', label: '状态' },
        { key: 'actions', label: '操作' },
      ]"
      :empty="levelFilter === 'all' ? '项目还没有环境' : '此级别暂无环境'"
    >
      <template #level="{ row }">{{ levels[row.level || ''] ?? row.level }}</template>
      <template #version_id="{ row }">{{ projectVersions.find((version) => version.id === row.version_id)?.name || row.version_id || '未关联' }}</template>
      <template #base_url="{ row }"><span class="environment-url">{{ row.base_url }}</span></template>
      <template #source="{ row }">{{ sources[row.source] || row.source }}</template>
      <template #status="{ row }">{{ row.status === 'unknown' ? '未知' : row.status }}</template>
      <template #actions="{ row }">
        <button type="button" :aria-label="`编辑环境 ${row.name}`" :disabled="saving" @click="openForm(row)">编辑</button>
      </template>
    </DataTable>
  </div>
</template>

<style scoped src="./project-hubs.css"></style>
<style scoped>
.environment-url { overflow-wrap: anywhere; }
</style>
