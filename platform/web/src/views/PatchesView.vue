<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const rows = ref([])
const error = ref('')

async function load() {
  rows.value = await api.get('/api/patches')
}

async function decide(row, decision) {
  error.value = ''
  try {
    await api.post(`/api/patches/${row.id}/decide`, { decision, reviewer: '当前用户' })
    await load()
  } catch (e) { error.value = e.message }
}

onMounted(load)
</script>

<template>
  <header class="head">
    <p class="hint">自愈只生成候选补丁，批准后才进入定位器库；不批准不影响用例本身</p>
    <button @click="load">刷新</button>
  </header>
  <p v-if="error" class="error">{{ error }}</p>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'run_id', label: '执行', width: '16%' },
      { key: 'step_seq', label: '步骤', width: '7%' },
      { key: 'old_selector', label: '原选择器' },
      { key: 'new_selector', label: '新选择器' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'actions', label: '', width: '16%' },
    ]"
    empty="没有待审批补丁"
  >
    <template #run_id="{ row }"><code>{{ row.run_id }}</code></template>
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #actions="{ row }">
      <div v-if="row.status === 'pending'" class="actions">
        <button class="primary" @click="decide(row, 'approve')">批准</button>
        <button @click="decide(row, 'reject')">拒绝</button>
      </div>
    </template>
  </DataTable>
</template>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.hint { color: var(--muted); font-size: 12px; margin: 0; }
.actions { display: flex; gap: 6px; }
.actions button { padding: 3px 9px; font-size: 12px; }
code { font-size: 12px; color: var(--muted); }
.error { color: var(--bad); }
</style>
