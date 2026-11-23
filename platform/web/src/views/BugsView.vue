<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const emit = defineEmits(['open-run'])
const rows = ref([])
const projects = ref([])
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ project_id: '', title: '', severity: 'S3', priority: 'P2', evidence: '' })
const statusFilter = ref('')
const severityFilter = ref('')
const error = ref('')

const NEXT = {
  new: ['confirmed', 'rejected'],
  reopened: ['confirmed', 'rejected'],
  confirmed: ['fixing', 'rejected'],
  fixing: ['fixed'],
  fixed: ['verified', 'reopened'],
  verified: ['closed', 'reopened'],
  closed: ['reopened'],
}
const TO_LABEL = {
  confirmed: '确认', rejected: '拒绝', fixing: '开始修复', fixed: '标记已修复',
  verified: '验证通过', closed: '关闭', reopened: '重新打开',
}

const filtered = computed(() =>
  rows.value.filter((b) =>
    (!statusFilter.value || b.status === statusFilter.value) &&
    (!severityFilter.value || b.severity === severityFilter.value)),
)

async function load() {
  rows.value = await api.get('/api/bugs')
}

async function move(bug, to) {
  error.value = ''
  try {
    await api.post(`/api/bugs/${bug.id}/transition`, { to, actor: '当前用户', note: '' })
    await load()
  } catch (e) { error.value = e.message }
}

async function setSeverity(bug, severity) {
  error.value = ''
  try {
    await api.post(`/api/bugs/${bug.id}/transition`, { to: bug.status, severity, actor: '当前用户' })
  } catch (problem) { error.value = problem.message }
  finally { await load() }
}

async function create() {
  busy.value = true
  error.value = ''
  try {
    await api.post('/api/bugs', form.value)
    form.value = { project_id: form.value.project_id, title: '', severity: 'S3', priority: 'P2', evidence: '' }
    showCreate.value = false
    await load()
  } catch (problem) { error.value = problem.message }
  finally { busy.value = false }
}

async function retest(bug) {
  busy.value = true
  error.value = ''
  try {
    const { run_id } = await api.post(`/api/bugs/${bug.id}/retest`, {})
    emit('open-run', run_id)
  } catch (problem) { error.value = problem.message }
  finally { busy.value = false }
}

onMounted(async () => {
  try {
    projects.value = await api.get('/api/projects')
    form.value.project_id = projects.value[0]?.id || ''
    await load()
  } catch (problem) { error.value = problem.message }
})
</script>

<template>
  <header class="head">
    <select v-model="statusFilter">
      <option value="">全部状态</option>
      <option v-for="(l, k) in { new: '新建', confirmed: '已确认', fixing: '修复中', fixed: '已修复', verified: '已验证', closed: '已关闭', reopened: '重新打开' }" :key="k" :value="k">{{ l }}</option>
    </select>
    <select v-model="severityFilter">
      <option value="">全部级别</option>
      <option value="S1">S1 致命</option>
      <option value="S2">S2 严重</option>
      <option value="S3">S3 一般</option>
      <option value="S4">S4 轻微</option>
    </select>
    <button @click="load">刷新</button>
    <button type="button" :aria-expanded="showCreate" @click="showCreate = !showCreate">提交缺陷</button>
  </header>
  <p v-if="error" class="error" role="alert">{{ error }}</p>
  <form v-if="showCreate" class="defect-form" aria-label="提交缺陷" @submit.prevent="create">
    <label>项目<select v-model="form.project_id" required>
      <option value="" disabled>请选择项目</option>
      <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
    </select></label>
    <label>标题<input v-model.trim="form.title" required /></label>
    <label>级别<select v-model="form.severity"><option v-for="severity in ['S1', 'S2', 'S3', 'S4']" :key="severity">{{ severity }}</option></select></label>
    <label>优先级<select v-model="form.priority"><option v-for="priority in ['P0', 'P1', 'P2', 'P3']" :key="priority">{{ priority }}</option></select></label>
    <label>复现步骤与证据<textarea v-model="form.evidence" rows="3" /></label>
    <button type="submit" class="primary" :disabled="busy">保存缺陷</button>
  </form>

  <DataTable
    :rows="filtered"
    :columns="[
      { key: 'bug_code', label: '编号', width: '14%' },
      { key: 'title', label: '标题' },
      { key: 'evidence', label: '证据' },
      { key: 'severity', label: '级别', width: '8%' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'reopen_count', label: '复开', width: '7%' },
      { key: 'actions', label: '', width: '26%' },
    ]"
    empty="还没有缺陷"
  >
    <template #bug_code="{ row }"><code>{{ row.bug_code }}</code></template>
    <template #evidence="{ row }"><details v-if="row.evidence"><summary>查看证据</summary><p>{{ row.evidence }}</p></details></template>
    <template #severity="{ row }">
      <select class="sev" :value="row.severity" @change="setSeverity(row, $event.target.value)">
        <option v-for="s in ['S1', 'S2', 'S3', 'S4']" :key="s">{{ s }}</option>
      </select>
    </template>
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #actions="{ row }">
      <div class="actions">
        <button v-if="row.run_id" type="button" @click="emit('open-run', row.run_id)">执行记录</button>
        <button v-if="row.run_id" type="button" :disabled="busy" @click="retest(row)">复测</button>
        <button
          v-for="to in NEXT[row.status] || []"
          :key="to"
          :class="{ primary: to === 'closed' || to === 'fixed' }"
          @click="move(row, to)"
        >{{ TO_LABEL[to] }}</button>
      </div>
    </template>
  </DataTable>
</template>

<style scoped>
.head { display: flex; gap: 8px; margin-bottom: 14px; }
.defect-form { display: flex; flex-wrap: wrap; gap: 10px; align-items: end; margin-bottom: 14px; }
.defect-form label { display: grid; gap: 4px; }
select { font: inherit; padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.sev { padding: 2px 6px; font-size: 12px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions button { padding: 3px 9px; font-size: 12px; }
code { font-size: 12px; color: var(--muted); }
.error { color: var(--bad); }
</style>
