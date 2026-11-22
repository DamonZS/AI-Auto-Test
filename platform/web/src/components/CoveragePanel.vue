<script setup>
import { computed, nextTick, ref, useId, watch } from 'vue'
import { api } from '../api'

const props = defineProps({ versions: { type: Array, required: true } })
const panelId = useId()
const versionId = ref('')
const format = ref('go-coverprofile')
const rows = ref([])
const loading = ref(false)
const importing = ref(false)
const status = ref('')
const error = ref('')
const errorSummary = ref(null)
const selectedVersion = computed(() => props.versions.find((version) => version.id === versionId.value) ?? props.versions[0])
const endpoint = computed(() => selectedVersion.value ? `/api/versions/${encodeURIComponent(selectedVersion.value.id)}/coverage` : '')
const formats = { 'go-coverprofile': 'Go coverprofile', cobertura: 'Cobertura XML' }
const percent = new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 })

function displayCount(metric) {
  if (!metric?.total) return '—'
  return `${metric.covered} / ${metric.total}（${percent.format(metric.covered / metric.total)}）`
}

function errorMessage(failure) {
  try { return JSON.parse(failure.message).error || failure.message } catch { return failure.message }
}

watch(endpoint, async (path, _, onCleanup) => {
  let active = true
  onCleanup(() => { active = false })
  rows.value = []
  status.value = ''
  error.value = ''
  loading.value = Boolean(path)
  if (!path) return
  try {
    const snapshots = await api.get(path)
    if (active) rows.value = snapshots
  } catch (failure) {
    if (active) error.value = errorMessage(failure)
  } finally {
    if (active) loading.value = false
  }
}, { immediate: true })

async function submit(event) {
  const path = endpoint.value
  const form = event.currentTarget
  const data = new FormData(form)
  const report = data.get('report')
  status.value = ''
  error.value = ''
  importing.value = true
  try {
    if (report.size > 1024 * 1024) throw new Error('覆盖率报告超过 1 MiB')
    const snapshot = await api.post(path, { format: data.get('format'), content: await report.text() })
    if (path === endpoint.value) {
      rows.value = [...rows.value.filter((item) => item.format !== snapshot.format), snapshot]
        .sort((left, right) => left.format.localeCompare(right.format))
      form.elements.namedItem('report').value = ''
      status.value = '覆盖率已导入'
    }
  } catch (failure) {
    if (path === endpoint.value) {
      error.value = errorMessage(failure)
      await nextTick()
      errorSummary.value?.focus()
    }
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <section class="coverage-panel" :aria-labelledby="`${panelId}-heading`">
    <h2 :id="`${panelId}-heading`">白盒覆盖率</h2>
    <p v-if="!selectedVersion">还没有可导入覆盖率的版本</p>
    <form v-else :key="endpoint" :action="endpoint" method="post" :aria-labelledby="`${panelId}-heading`" @submit.prevent="submit">
      <div class="field">
        <label :for="`${panelId}-version`">版本</label>
        <select :id="`${panelId}-version`" :value="selectedVersion.id" name="version" :disabled="importing" @change="versionId = $event.target.value">
          <option v-for="version in versions" :key="version.id" :value="version.id">{{ version.name }}</option>
        </select>
      </div>
      <div class="field">
        <label :for="`${panelId}-format`">格式</label>
        <select :id="`${panelId}-format`" v-model="format" name="format" :disabled="importing">
          <option v-for="(label, value) in formats" :key="value" :value="value">{{ label }}</option>
        </select>
      </div>
      <div class="field">
        <label :for="`${panelId}-report`">覆盖率报告（最多 1 MiB）</label>
        <input :id="`${panelId}-report`" type="file" name="report" required :disabled="importing"
          :accept="format === 'cobertura' ? '.xml' : '.out,.coverprofile,.cov,.txt'" />
      </div>
      <button type="submit" class="primary" :disabled="loading || importing">{{ importing ? '导入中…' : '导入覆盖率' }}</button>
    </form>
    <p v-if="error" ref="errorSummary" class="error" role="alert" tabindex="-1">{{ error }}</p>
    <p v-if="status || loading" role="status">{{ loading ? '正在读取覆盖率…' : status }}</p>
    <p v-if="selectedVersion && !loading && !error && !rows.length">此版本暂无覆盖率记录</p>
    <table v-if="rows.length">
      <thead><tr><th scope="col">格式</th><th scope="col">语句</th><th scope="col">行</th><th scope="col">分支</th></tr></thead>
      <tbody>
        <tr v-for="row in rows" :key="row.format">
          <th scope="row">{{ formats[row.format] }}</th>
          <td>{{ displayCount(row.statements) }}</td>
          <td>{{ displayCount(row.lines) }}</td>
          <td>{{ displayCount(row.branches) }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.coverage-panel { margin-top: 20px; padding: 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
h2 { margin: 0 0 12px; font-size: 16px; }
form { display: flex; align-items: end; flex-wrap: wrap; gap: 8px 12px; }
.field { display: grid; gap: 4px; }
select, input { font: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
table { width: 100%; margin-top: 12px; border-collapse: collapse; text-align: left; }
td, th { padding: 8px 10px; border-bottom: 1px solid var(--line); }
.error { color: var(--bad); }
</style>
