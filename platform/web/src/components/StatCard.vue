<script setup>
import AppIcon from './AppIcon.vue'

defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  tone: { type: String, default: '' },
  icon: { type: String, default: '' },
})
</script>

<template>
  <div class="card" :class="tone">
    <div class="card-top">
      <div class="value">{{ value }}</div>
      <AppIcon v-if="icon" :name="icon" class="card-icon" />
    </div>
    <div class="label">{{ label }}</div>
  </div>
</template>

<style scoped>
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 16px 18px;
}
.card-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.card-icon { color: var(--muted); }
.value { font-size: 24px; font-weight: 600; letter-spacing: -0.02em; }
.label { color: var(--muted); font-size: 12px; margin-top: 4px; }
.card.warn .value { color: var(--bad); }
.card.ok .value { color: var(--ok); }
</style>
