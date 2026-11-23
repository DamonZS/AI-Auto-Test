<script setup>
import { computed, ref, useId, watch } from 'vue'
import { api } from '../api'

const props = defineProps({
  projectId: { type: String, required: true },
  requirements: { type: Array, required: true },
  initialRequirementId: { type: String, default: '' },
  initialVersionId: { type: String, default: '' },
})
const emit = defineEmits(['generated'])
const labelId = useId()
const requirementId = ref('')
const versionId = ref('')
const type = ref('ui')
const busy = ref(false)
const error = ref('')
const result = ref(null)
const projectRequirements = computed(() => props.requirements.filter((item) => item.project_id === props.projectId))

watch(() => [props.projectId, props.initialRequirementId, props.initialVersionId], () => {
  requirementId.value = props.initialRequirementId
  versionId.value = props.initialRequirementId ? props.initialVersionId : ''
  error.value = ''
  result.value = null
}, { immediate: true })

function changeRequirement() {
  versionId.value = ''
  error.value = ''
  result.value = null
}

async function generate() {
  busy.value = true
  error.value = ''
  result.value = null
  try {
    result.value = await api.post('/api/cases/generate', {
      project_id: props.projectId,
      requirement_id: requirementId.value,
      type: type.value,
      ...(versionId.value && { version_id: versionId.value }),
    })
    emit('generated', result.value)
  } catch (problem) {
    error.value = problem.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="generation-panel" action="/api/cases/generate" method="post" :aria-labelledby="labelId" @submit.prevent="generate">
    <h3 :id="labelId">从需求生成用例</h3>
    <fieldset class="generation-fields" :disabled="busy">
      <legend class="sr-only">生成选项</legend>
      <label :for="`${labelId}-requirement`">需求</label>
      <select :id="`${labelId}-requirement`" v-model="requirementId" name="requirement_id" required @change="changeRequirement">
        <option disabled value="">请选择需求</option>
        <option v-for="item in projectRequirements" :key="item.id" :value="item.id">
          {{ item.code ? `${item.code} · ${item.title}` : item.title }}
        </option>
      </select>
      <fieldset class="generation-types">
        <legend>类型</legend>
        <label><input v-model="type" type="radio" name="type" value="ui" /> UI</label>
        <label><input v-model="type" type="radio" name="type" value="api" /> 接口</label>
      </fieldset>
      <label v-if="initialVersionId && requirementId === initialRequirementId">
        <input type="checkbox" :checked="!!versionId" @change="versionId = $event.target.checked ? initialVersionId : ''" />
        使用预选项目版本的需求修订
      </label>
      <div><button type="submit" class="primary">{{ busy ? '正在生成…' : '生成草稿' }}</button></div>
    </fieldset>
    <p v-if="error" class="generation-error" role="alert">{{ error }}</p>
    <p v-if="result" role="status">已从需求修订 {{ result.revision_no }} 生成 {{ result.cases.length }} 条草稿，审批后可运行。</p>
  </form>
</template>

<style scoped>
.generation-panel { display: grid; gap: 10px; max-width: 420px; }
.generation-panel h3, .generation-panel p { margin: 0; }
.generation-fields, .generation-types { padding: 0; border: 0; margin: 0; min-width: 0; }
.generation-fields { display: grid; gap: 6px; }
.generation-types { display: flex; gap: 12px; margin-block: 4px; }
.generation-types legend { padding: 0; }
.generation-panel select { font: inherit; color: inherit; padding: 7px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.generation-error { color: var(--bad); }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
</style>
