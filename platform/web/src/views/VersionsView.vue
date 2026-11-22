<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const rows = ref([])
const projects = ref([])
const projectId = ref('')
const form = ref({ name: '', commit_ref: '', impact: '' })
const error = ref('')

const projectName = computed(() => (id) => projects.value.find((p) => p.id === id)?.name || '—')

function impactTags(json) {
  try { return JSON.parse(json || '[]') } catch { return [] }
}

async function load() {
  projects.value = await api.get('/api/projects')
  rows.value = await api.get('/api/versions')
  if (!projectId.value && projects.value.length) projectId.value = projects.value[0].id
}

async function add() {
  if (!form.value.name || !projectId.value) return
  error.value = ''
  try {
    await api.post('/api/versions', {
      project_id: projectId.value,
      name: form.value.name,
      commit_ref: form.value.commit_ref,
      impact: form.value.impact.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
    })
    form.value = { name: '', commit_ref: '', impact: '' }
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
      <input v-model="form.name" placeholder="版本名 v1.2.0" class="name" />
      <input v-model="form.commit_ref" placeholder="commit / tag" class="commit" />
      <input v-model="form.impact" placeholder="影响范围，逗号分隔" class="impact" />
      <button class="primary" @click="add">登记</button>
    </div>
  </header>
  <p v-if="error" class="error">{{ error }}</p>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'name', label: '版本', width: '18%' },
      { key: 'commit_ref', label: '提交', width: '18%' },
      { key: 'impact_json', label: '影响范围' },
      { key: 'project_id', label: '项目', width: '16%' },
      { key: 'created_at', label: '创建时间', width: '18%' },
    ]"
    empty="还没有版本"
  >
    <template #project_id="{ row }">{{ projectName(row.project_id) }}</template>
    <template #impact_json="{ row }">
      <span v-for="t in impactTags(row.impact_json)" :key="t" class="chip">{{ t }}</span>
    </template>
  </DataTable>
</template>

<style scoped>
.head { margin-bottom: 16px; }
.form { display: flex; gap: 8px; }
select, input { font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.name { width: 150px; }
.commit { width: 170px; }
.impact { flex: 1; }
.chip { display: inline-block; background: #f1f1f4; border-radius: 4px; padding: 1px 6px; margin-right: 4px; font-size: 12px; color: var(--muted); }
.error { color: var(--bad); }
</style>
