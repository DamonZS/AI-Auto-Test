<script setup>
defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  tone: { type: String, default: '' },
})
</script>

<template>
  <div class="card" :class="tone">
    <div class="value">{{ value }}</div>
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
.value { font-size: 24px; font-weight: 600; letter-spacing: -0.02em; }
.label { color: var(--muted); font-size: 12px; margin-top: 4px; }
.card.warn .value { color: var(--bad); }
.card.ok .value { color: var(--ok); }
</style>
