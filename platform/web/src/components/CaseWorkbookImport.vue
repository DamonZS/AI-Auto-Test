<script setup>
import { onMounted, onUnmounted, ref } from 'vue'

const emit = defineEmits(['imported', 'legacy-imported'])
const available = ref(Boolean(window.caseWorkbook))
const busy = ref(false)
const error = ref('')
const legacyCases = ref([])
const selected = ref(0)

function workbookReady() {
  available.value = true
}

onMounted(() => window.addEventListener('case-workbook-ready', workbookReady))
onUnmounted(() => window.removeEventListener('case-workbook-ready', workbookReady))

function importWorkbook() {
  error.value = ''
  busy.value = true
  window.caseWorkbook.importWorkbook((result) => {
    busy.value = false
    if (result.cancelled) return
    legacyCases.value = result.legacyCases || []
    selected.value = 0
    if (result.error) error.value = result.error
    else if (!result.legacyCases) emit('imported', result)
    else if (legacyCases.value.length === 1) importLegacy()
  })
}

function importLegacy() {
  emit('legacy-imported', legacyCases.value[selected.value])
}
</script>

<template>
  <template v-if="available">
    <button type="button" :disabled="busy" @click="importWorkbook">导入 XLSX</button>
    <template v-if="legacyCases.length > 1">
      <select v-model="selected" name="legacy-workbook-case" aria-label="旧版工作簿用例">
        <option v-for="(item, index) in legacyCases" :key="index" :value="index">
          {{ item.name }} · {{ item.url }}
        </option>
      </select>
      <button type="button" :disabled="busy" @click="importLegacy">载入所选用例</button>
    </template>
    <span v-if="error" class="error" role="alert">{{ error }}</span>
  </template>
</template>
