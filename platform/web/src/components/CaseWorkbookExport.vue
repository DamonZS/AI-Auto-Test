<script setup>
import { onMounted, onUnmounted, ref } from 'vue'

const props = defineProps({
  name: { type: String, default: '' },
  scenario: { type: [Object, String], required: true },
})
const available = ref(Boolean(window.caseWorkbook))
const busy = ref(false)
const error = ref('')

function workbookReady() {
  available.value = true
}

onMounted(() => window.addEventListener('case-workbook-ready', workbookReady))
onUnmounted(() => window.removeEventListener('case-workbook-ready', workbookReady))

function exportWorkbook() {
  error.value = ''
  busy.value = true
  const scenario = typeof props.scenario === 'string' ? props.scenario : JSON.stringify(props.scenario)
  window.caseWorkbook.exportWorkbook(props.name, scenario, (result) => {
    busy.value = false
    if (result.error) error.value = result.error
  })
}
</script>

<template>
  <template v-if="available">
    <button type="button" :disabled="busy" @click="exportWorkbook">导出 XLSX</button>
    <span v-if="error" class="error" role="alert">{{ error }}</span>
  </template>
</template>
