<script setup>
import AppIcon from './AppIcon.vue'

defineProps({ page: { type: String, required: true } })
const emit = defineEmits(['navigate'])
const stages = [
  { key: 'requirements', label: '需求与评审' },
  { key: 'cases', label: '用例设计' },
  { key: 'suites', label: '回归套件' },
  { key: 'runs', label: '执行与证据' },
  { key: 'bugs', label: '缺陷复测' },
  { key: 'workbench', label: '流程与报告' },
]
</script>

<template>
  <ol class="journey" aria-label="项目测试流程">
    <li v-for="stage in stages" :key="stage.key">
      <button type="button" :aria-current="page === stage.key ? 'step' : undefined" @click="emit('navigate', stage.key)">
        <AppIcon :name="stage.key" />{{ stage.label }}
      </button>
    </li>
  </ol>
</template>

<style scoped>
.journey { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; padding: 8px; margin: 0 0 20px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel); list-style: none; }
.journey button { display: flex; align-items: center; gap: 7px; background: transparent; border-color: transparent; font-size: 12px; white-space: nowrap; }
.journey button:hover { background: var(--bg); }
.journey button[aria-current] { background: var(--accent-soft); color: var(--accent); border-color: var(--accent-soft); font-weight: 600; }
</style>
