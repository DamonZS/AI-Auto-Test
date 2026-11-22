<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'
import StatusTag from '../components/StatusTag.vue'

const rows = ref([])
const detail = ref(null)

async function load() {
  rows.value = await api.get('/api/runs')
}

function ms(v) {
  if (!v) return '—'
  return v > 1000 ? (v / 1000).toFixed(1) + 's' : v + 'ms'
}

onMounted(load)
</script>

<template>
  <header class="head">
    <button @click="load">刷新</button>
  </header>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'case_name', label: '用例', width: '24%' },
      { key: 'case_type', label: '类型', width: '8%' },
      { key: 'status', label: '状态', width: '10%' },
      { key: 'self_healed', label: '自愈', width: '8%' },
      { key: 'token_in', label: 'Token', width: '12%' },
      { key: 'duration_ms', label: '耗时', width: '10%' },
      { key: 'started_at', label: '开始时间', width: '18%' },
    ]"
    empty="还没有执行记录"
    clickable
    @row-click="detail = $event"
  >
    <template #status="{ row }"><StatusTag :value="row.status" /></template>
    <template #case_type="{ row }">{{ { ui: 'UI', api: '接口', perf: '性能' }[row.case_type] || row.case_type }}</template>
    <template #self_healed="{ row }">{{ row.self_healed ? '♻' : '—' }}</template>
    <template #token_in="{ row }">{{ row.token_in ? row.token_in + ' / ' + row.token_out : '—' }}</template>
    <template #duration_ms="{ row }">{{ ms(row.duration_ms) }}</template>
  </DataTable>

  <div v-if="detail" class="drawer" @click.self="detail = null">
    <div class="panel">
      <h3>{{ detail.case_name }}</h3>
      <dl>
        <dt>状态</dt><dd><StatusTag :value="detail.status" /></dd>
        <dt>场景</dt><dd>{{ detail.scenario }}</dd>
        <dt>耗时</dt><dd>{{ ms(detail.duration_ms) }}</dd>
        <dt>Token</dt><dd>{{ detail.token_in }} / {{ detail.token_out }}</dd>
        <dt>需复核</dt><dd>{{ detail.needs_review ? '是' : '否' }}</dd>
      </dl>
      <h4>报告</h4>
      <pre>{{ detail.report || '暂无报告' }}</pre>
      <button @click="detail = null">关闭</button>
    </div>
  </div>
</template>

<style scoped>
.head { margin-bottom: 14px; }
.drawer { position: fixed; inset: 0; background: rgba(20,20,24,0.28); display: flex; justify-content: flex-end; }
.panel { width: 460px; background: var(--panel); padding: 22px; overflow: auto; }
.panel h3 { margin: 0 0 16px; font-size: 15px; }
.panel h4 { font-size: 12px; color: var(--muted); margin: 0 0 8px; }
dl { display: grid; grid-template-columns: 84px 1fr; gap: 10px 12px; font-size: 13px; margin: 0 0 20px; }
dt { color: var(--muted); }
dd { margin: 0; word-break: break-word; }
pre { background: #f7f7f9; border: 1px solid var(--line); border-radius: 8px; padding: 12px; font-size: 12px; white-space: pre-wrap; margin: 0 0 18px; }
</style>
