<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const rows = ref([])
const projects = ref([])
const projectId = ref('')
const form = ref({ name: '', base_url: '', source: 'platform' })
const error = ref('')

const projectName = computed(() => (id) => projects.value.find((p) => p.id === id)?.name || '—')

async function load() {
  projects.value = await api.get('/api/projects')
  rows.value = await api.get('/api/environments')
  if (!projectId.value && projects.value.length) projectId.value = projects.value[0].id
}

async function add() {
  if (!form.value.name || !form.value.base_url) return
  error.value = ''
  try {
    await api.post('/api/environments', { ...form.value, project_id: projectId.value })
    form.value = { name: '', base_url: '', source: 'platform' }
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
      <input v-model="form.name" placeholder="环境名 预发 / 测试" class="name" />
      <input v-model="form.base_url" placeholder="https://..." class="url" />
      <select v-model="form.source">
        <option value="platform">平台选定</option>
        <option value="mcp">MCP 提交</option>
      </select>
      <button class="primary" @click="add">添加</button>
    </div>
  </header>
  <p v-if="error" class="error">{{ error }}</p>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'name', label: '环境', width: '18%' },
      { key: 'base_url', label: '地址' },
      { key: 'source', label: '来源', width: '14%' },
      { key: 'status', label: '状态', width: '12%' },
      { key: 'project_id', label: '项目', width: '16%' },
    ]"
    empty="还没有环境"
  >
    <template #project_id="{ row }">{{ projectName(row.project_id) }}</template>
    <template #source="{ row }">{{ row.source === 'mcp' ? 'MCP 提交' : '平台选定' }}</template>
  </DataTable>
</template>

<style scoped>
.head { margin-bottom: 16px; }
.form { display: flex; gap: 8px; }
select, input { font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.name { width: 180px; }
.url { flex: 1; }
.error { color: var(--bad); }
</style>
