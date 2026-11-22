<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const rows = ref([])
const projects = ref([])
const projectId = ref('')
const form = ref({ code: '', title: '', content: '' })
const error = ref('')

const projectName = computed(() => (id) => projects.value.find((p) => p.id === id)?.name || '—')

async function load() {
  projects.value = await api.get('/api/projects')
  rows.value = await api.get('/api/requirements')
  if (!projectId.value && projects.value.length) projectId.value = projects.value[0].id
}

async function add() {
  if (!form.value.title || !projectId.value) return
  error.value = ''
  try {
    await api.post('/api/requirements', { ...form.value, project_id: projectId.value })
    form.value = { code: '', title: '', content: '' }
    await load()
  } catch (e) { error.value = e.message }
}

onMounted(load)
</script>

<template>
  <header class="head">
    <div class="form">
      <select v-model="projectId">
        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <input v-model="form.code" placeholder="需求编号" class="code" />
      <input v-model="form.title" placeholder="需求标题" class="title" />
      <input v-model="form.content" placeholder="需求描述" class="desc" />
      <button class="primary" @click="add">录入</button>
    </div>
  </header>
  <p v-if="error" class="error">{{ error }}</p>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'code', label: '编号', width: '12%' },
      { key: 'title', label: '标题', width: '26%' },
      { key: 'content', label: '描述' },
      { key: 'project_id', label: '项目', width: '16%' },
      { key: 'status', label: '状态', width: '10%' },
    ]"
    empty="还没有需求"
  >
    <template #project_id="{ row }">{{ projectName(row.project_id) }}</template>
  </DataTable>
</template>

<style scoped>
.head { margin-bottom: 16px; }
.form { display: flex; gap: 8px; }
select, input { font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.code { width: 120px; }
.title { width: 220px; }
.desc { flex: 1; }
.error { color: var(--bad); }
</style>
