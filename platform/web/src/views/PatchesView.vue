<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ selectedRun: { type: String, default: '' } })
const emit = defineEmits(['open-run', 'open-case-history'])
const rows = ref([])
const error = ref('')
const loading = ref(false)
const deciding = ref('')
const runSearch = ref(props.selectedRun)
const visibleRows = computed(() => rows.value.filter((row) => row.run_id.toLowerCase().includes(runSearch.value.toLowerCase())))

async function load() {
  error.value = ''
  loading.value = true
  try {
    rows.value = await api.get('/api/patches')
  } catch (problem) {
    error.value = problem.message
  } finally {
    loading.value = false
  }
}

async function decide(row, decision) {
  error.value = ''
  deciding.value = row.id
  try {
    await api.post(`/api/patches/${row.id}/decide`, { decision, reviewer: '当前用户' })
    await load()
  } catch (problem) {
    error.value = problem.message
  } finally {
    deciding.value = ''
  }
}

onMounted(load)
</script>

<template>
  <header class="head">
    <label for="patch-run-search">按执行 ID 搜索租户补丁
      <input id="patch-run-search" v-model.trim="runSearch" type="search" />
    </label>
    <button type="button" :disabled="loading || !!deciding" @click="load">刷新</button>
  </header>
  <p class="hint">批准自愈补丁会创建新的用例版本。</p>
  <p v-if="error" class="error" role="alert">{{ error }}</p>
  <p v-if="loading && !rows.length" role="status">正在加载候选补丁…</p>
  <DataTable
    v-else-if="rows.length || !error"
    :rows="visibleRows"
    :columns="[
      { key: 'run_id', label: '执行', width: '16%' },
      { key: 'case_name', label: '用例' },
      { key: 'step_seq', label: '步骤', width: '7%' },
      { key: 'role', label: '控件角色' },
      { key: 'old_selector', label: '原选择器' },
      { key: 'new_selector', label: '候选选择器' },
      { key: 'status', label: '审批状态', width: '10%' },
      { key: 'applied_version_no', label: '应用版本' },
      { key: 'actions', label: '', width: '16%' },
    ]"
    :empty="runSearch ? '没有匹配的补丁' : '暂无候选补丁'"
  >
    <template #run_id="{ row }">
      <button type="button" class="run-link" :aria-label="`查看执行 ${row.run_id}`" @click="emit('open-run', row.run_id, row.project_id)">{{ row.run_id }}</button>
    </template>
    <template #case_name="{ row }">
      <button v-if="row.case_id" type="button" class="run-link"
        @click="emit('open-case-history', { caseId: row.case_id, projectId: row.project_id })">
        {{ row.case_name || row.case_id }}
      </button>
      <span v-else>{{ row.case_name || '—' }}</span>
    </template>
    <template #applied_version_no="{ row }">{{ row.applied_version_no ? `v${row.applied_version_no}` : '—' }}</template>
    <template #old_selector="{ row }"><code>{{ row.old_selector || '未记录' }}</code></template>
    <template #new_selector="{ row }"><code>{{ row.new_selector || '未记录' }}</code></template>
    <template #status="{ row }"><span v-if="row.status === 'approved'" class="approved">已批准</span><StatusTag v-else :value="row.status" /></template>
    <template #actions="{ row }">
      <div v-if="row.status === 'pending'" class="actions">
        <button type="button" class="primary" :disabled="loading || !!deciding" @click="decide(row, 'approve')">批准</button>
        <button type="button" :disabled="loading || !!deciding" @click="decide(row, 'reject')">拒绝</button>
      </div>
    </template>
  </DataTable>
</template>

<style scoped>
.head { display: flex; flex-wrap: wrap; gap: 10px; align-items: end; margin-bottom: 14px; }
.head label { display: grid; gap: 4px; font-size: 13px; }
.hint { color: var(--muted); font-size: 13px; margin: 0 0 14px; }
.actions { display: flex; gap: 6px; }
.actions button { padding: 3px 9px; font-size: 12px; }
code { font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; }
.run-link { overflow-wrap: anywhere; text-align: start; }
.approved { display: inline-block; padding: 2px 8px; border-radius: 999px; background: #e8f5ee; color: var(--ok); font-size: 12px; white-space: nowrap; }
.error { color: var(--bad); }
</style>
