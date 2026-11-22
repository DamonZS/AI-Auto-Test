<script setup>
defineProps({
  rows: { type: Array, default: () => [] },
  columns: { type: Array, required: true },
  empty: { type: String, default: '暂无数据' },
  clickable: { type: Boolean, default: false },
})
defineEmits(['row-click'])
</script>

<template>
  <div class="wrap">
    <table v-if="rows.length">
      <thead>
        <tr><th v-for="c in columns" :key="c.key" :style="{ width: c.width }">{{ c.label }}</th></tr>
      </thead>
      <tbody>
        <tr
          v-for="(r, i) in rows"
          :key="r.id || i"
          :class="{ clickable }"
          @click="clickable && $emit('row-click', r)"
        >
          <td v-for="c in columns" :key="c.key">
            <slot :name="c.key" :row="r">{{ r[c.key] ?? '—' }}</slot>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-else class="empty">{{ empty }}</p>
  </div>
</template>

<style scoped>
.wrap { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; overflow: auto; max-inline-size: 100%; }
table { width: 100%; min-width: 620px; border-collapse: collapse; font-size: 13px; }
th {
  text-align: left;
  font-weight: 500;
  color: var(--muted);
  padding: 10px 14px;
  border-bottom: 1px solid var(--line);
  background: #fbfbfc;
  white-space: nowrap;
}
td { padding: 11px 14px; border-bottom: 1px solid #f0f0f2; }
tbody tr:last-child td { border-bottom: 0; }
tr.clickable { cursor: pointer; }
tr.clickable:hover td { background: #fafbff; }
.empty { color: var(--muted); padding: 28px 14px; margin: 0; text-align: center; }
</style>
