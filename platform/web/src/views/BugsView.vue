<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const rows = ref([])
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
  try {
    await api.post(`/api/bugs/${bug.id}/transition`, { to: bug.status, severity, actor: '当前用户' })
  } catch {
    error.value = '同一状态不能直接改级，请走状态流转'
    return
  }
  await load()
}

onMounted(load)
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
  </header>
  <p v-if="error" class="error">{{ error }}</p>

  <DataTable
    :rows="filtered"
    :columns="[
      { key: 'bug_code', label: '编号', width: '14%' },
      { key: 'title', label: '标题' },
      { key: 'severity', label: '级别', width: '8%' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'reopen_count', label: '复开', width: '7%' },
      { key: 'actions', label: '', width: '26%' },
    ]"
    empty="还没有缺陷"
  >
    <template #bug_code="{ row }"><code>{{ row.bug_code }}</code></template>
    <template #severity="{ row }">
      <select class="sev" :value="row.severity" @change="setSeverity(row, $event.target.value)">
        <option v-for="s in ['S1', 'S2', 'S3', 'S4']" :key="s">{{ s }}</option>
      </select>
    </template>
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #actions="{ row }">
      <div class="actions">
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
select { font: inherit; padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.sev { padding: 2px 6px; font-size: 12px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions button { padding: 3px 9px; font-size: 12px; }
code { font-size: 12px; color: var(--muted); }
.error { color: var(--bad); }
</style>
