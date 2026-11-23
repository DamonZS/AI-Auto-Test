<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import CoveragePanel from '../components/CoveragePanel.vue'
import ReleaseComparison from '../components/ReleaseComparison.vue'

const props = defineProps({ projectId: { type: String, default: '' } })
const rows = ref([])
const projects = ref([])
const projectId = ref(props.projectId)
const form = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

const projectVersions = computed(() => rows.value.filter((row) => row.project_id === projectId.value)
  .map((row) => ({ ...row, impact: impactTags(row.impact_json) })))

function impactTags(json) {
  try {
    const parsed = JSON.parse(json || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

async function load() {
  loading.value = true
  try {
    const [projectRows, versionRows] = await Promise.all([api.get('/api/projects'), api.get('/api/versions')])
    projects.value = projectRows
    rows.value = versionRows
    if (!projectId.value) projectId.value = props.projectId || projectRows[0]?.id || ''
  } catch (failure) {
    error.value = failure.message
  } finally {
    loading.value = false
  }
}

function openForm() {
  form.value = { name: '', commit_ref: '', impact: '' }
  error.value = ''
  notice.value = ''
}

async function add() {
  saving.value = true
  error.value = ''
  try {
    await api.post('/api/versions', {
      project_id: projectId.value,
      name: form.value.name,
      commit_ref: form.value.commit_ref,
      impact: form.value.impact.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean),
    })
    form.value = null
    notice.value = '版本已登记'
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
  error.value = ''
  notice.value = ''
})
onMounted(load)
</script>

<template>
  <div class="hub">
    <header class="hub-toolbar">
      <label v-if="!props.projectId" class="hub-field hub-grow" for="versions-project">项目
        <select id="versions-project" v-model="projectId" name="project_id" :disabled="loading || saving">
          <option v-if="!projects.length" value="">暂无项目</option>
          <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
        </select>
      </label>
      <button type="button" class="primary" :disabled="loading || saving || !projects.some((project) => project.id === projectId)" @click="openForm">登记版本</button>
    </header>
    <form v-if="form" class="hub-panel hub-form" action="/api/versions" method="post" aria-labelledby="version-form-title" @submit.prevent="add">
      <h3 id="version-form-title">登记版本</h3>
      <div class="hub-fields">
        <label class="hub-field" for="version-name">版本名称 *
          <input id="version-name" v-model.trim="form.name" name="name" placeholder="v1.2.0" required />
        </label>
        <label class="hub-field" for="version-ref">提交引用
          <input id="version-ref" v-model.trim="form.commit_ref" name="commit_ref" placeholder="提交、分支或标签" />
        </label>
        <label class="hub-field" for="version-impact">影响范围（逗号分隔）
          <input id="version-impact" v-model.trim="form.impact" name="impact" />
        </label>
      </div>
      <div class="hub-actions">
        <button type="submit" class="primary" :disabled="saving">{{ saving ? '保存中…' : '保存版本' }}</button>
        <button type="button" :disabled="saving" @click="form = null">取消</button>
      </div>
    </form>
    <p v-if="error" class="hub-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="hub-notice" role="status">{{ notice }}</p>
    <p v-if="loading" role="status">正在读取版本…</p>
    <template v-else>
      <DataTable
        :rows="projectVersions"
        :columns="[
          { key: 'name', label: '版本' },
          { key: 'commit_ref', label: '提交引用' },
          { key: 'impact', label: '影响范围' },
          { key: 'created_at', label: '登记时间' },
        ]"
        empty="项目还没有版本"
      >
        <template #commit_ref="{ row }"><code class="version-ref">{{ row.commit_ref || '—' }}</code></template>
        <template #impact="{ row }">
          <span v-if="!row.impact.length">—</span>
          <span v-for="(tag, index) in row.impact" :key="index" class="chip">{{ tag }}</span>
        </template>
      </DataTable>
    </template>
    <ReleaseComparison :key="projectId" :versions="projectVersions" />
    <CoveragePanel :versions="projectVersions" />
  </div>
</template>

<style scoped src="./project-hubs.css"></style>
<style scoped>
.version-ref { overflow-wrap: anywhere; }
.chip { display: inline-block; background: var(--bg); border-radius: 4px; padding: 1px 6px; margin-right: 4px; font-size: 12px; color: var(--muted); }
</style>
