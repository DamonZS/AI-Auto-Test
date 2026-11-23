<script setup>
import { computed, ref, watch } from 'vue'
import { api } from '../api'
import DataTable from './DataTable.vue'

const props = defineProps({ versions: { type: Array, required: true } })
const baselineId = ref('')
const targetId = ref('')
const comparison = ref(null)
const loading = ref(false)
const error = ref('')
const baseline = computed(() => props.versions.find((version) => version.id === baselineId.value))
const target = computed(() => props.versions.find((version) => version.id === targetId.value))

watch(() => props.versions, (versions) => {
  if (!baseline.value) baselineId.value = versions[1]?.id || ''
  if (!target.value) targetId.value = versions[0]?.id || ''
}, { immediate: true })
watch([baselineId, targetId], () => {
  comparison.value = null
  error.value = ''
})

function linkedRevisions({ changes }) {
  const revisions = new Map()
  for (const change of changes) {
    const previous = revisions.get(change.requirement_id)
    if (!previous || change.current.revision_no > previous.current.revision_no) {
      revisions.set(change.requirement_id, change)
    }
  }
  return revisions
}

async function compare() {
  if (baselineId.value === targetId.value) {
    error.value = '请选择两个不同的版本'
    return
  }

  const base = baseline.value
  const next = target.value
  comparison.value = null
  error.value = ''
  loading.value = true
  try {
    const [baseImpact, targetImpact] = await Promise.all([base, next].map((version) =>
      api.get(`/api/versions/${encodeURIComponent(version.id)}/requirement-impact`)))
    const before = linkedRevisions(baseImpact)
    const after = linkedRevisions(targetImpact)
    const rows = []
    for (const id of new Set([...before.keys(), ...after.keys()])) {
      const oldLink = before.get(id)
      const newLink = after.get(id)
      const oldRevision = oldLink?.current.revision_no
      const newRevision = newLink?.current.revision_no
      if (oldRevision === newRevision) continue
      const { current, affected_case_count } = newLink || oldLink
      rows.push({
        id, title: [current.code, current.title].filter(Boolean).join(' · '),
        change: !oldLink ? '新增关联' : !newLink ? '移除关联' : '修订变化',
        baseline: oldRevision ? `R${oldRevision}` : '—',
        target: newRevision ? `R${newRevision}` : '—',
        affected_case_count,
      })
    }
    comparison.value = { base, target: next, rows }
  } catch (failure) {
    error.value = failure.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="hub-panel hub-form" aria-labelledby="compare-title">
    <h3 id="compare-title">版本比较</h3>
    <p v-if="versions.length < 2">登记两个版本后即可比较</p>
    <template v-else>
      <form class="hub-toolbar" aria-labelledby="compare-title" @submit.prevent="compare">
        <label class="hub-field hub-grow" for="release-baseline">基准版本
          <select id="release-baseline" v-model="baselineId" name="baseline" :disabled="loading" required>
            <option v-for="version in versions" :key="version.id" :value="version.id">{{ version.name }}</option>
          </select>
        </label>
        <label class="hub-field hub-grow" for="release-target">目标版本
          <select id="release-target" v-model="targetId" name="target" :disabled="loading" required>
            <option v-for="version in versions" :key="version.id" :value="version.id">{{ version.name }}</option>
          </select>
        </label>
        <button type="submit" :disabled="loading">{{ loading ? '比较中…' : '比较' }}</button>
      </form>
      <p v-if="error" class="hub-error" role="alert">{{ error }}</p>
      <template v-if="comparison">
        <div class="hub-fields">
          <div class="revision" v-for="(version, index) in [comparison.base, comparison.target]" :key="version.id">
            <strong>{{ index === 0 ? '基准' : '目标' }} · {{ version.name }}</strong>
            <code>{{ version.commit_ref || '未登记提交引用' }}</code>
            <span>{{ version.impact.join('、') || '未登记影响范围' }}</span>
          </div>
        </div>
        <p role="status">{{ comparison.rows.length }} 项需求关联变化，涉及 {{ comparison.rows.reduce((total, row) => total + row.affected_case_count, 0) }} 个用例</p>
        <DataTable :rows="comparison.rows" :columns="[
          { key: 'title', label: '受影响需求' },
          { key: 'change', label: '变化' },
          { key: 'baseline', label: '基准修订' },
          { key: 'target', label: '目标修订' },
          { key: 'affected_case_count', label: '关联用例' },
        ]" empty="两个版本的已关联需求修订一致" />
      </template>
    </template>
  </section>
</template>

<style scoped src="../views/project-hubs.css"></style>
<style scoped>
.revision { display: grid; gap: 3px; padding: 10px; background: var(--bg); border-radius: 6px; overflow-wrap: anywhere; }
.revision span { color: var(--muted); }
</style>
