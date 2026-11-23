<script setup>
defineProps({
  name: { type: String, required: true },
})

const glyphs = {
  overview: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
  projects: 'M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z',
  requirements: 'M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z M14 3v6h6 M8 13h8 M8 17h6',
  versions: 'M3 4h9l9 9-8 8-10-10Z M7.5 8.5h.01',
  environments: 'M3 4h18v13H3z M12 17v4 M8 21h8',
  'test-data': 'M4 6a8 3 0 1 0 16 0 8 3 0 1 0-16 0z M4 6v12a8 3 0 0 0 16 0V6 M4 12a8 3 0 0 0 16 0',
  cases: 'M8 5H5v16h14V5h-3 M8 3h8v4H8z M8 14l2.5 2.5L16 11',
  suites: 'M3 7l9-4 9 4-9 4z M3 12l9 4 9-4 M3 17l9 4 9-4',
  workbench: 'M9 3h6v6H9z M3 15h6v6H3z M15 15h6v6h-6z M12 9v3 M6 15v-3h12v3',
  runs: 'M4 11a8 8 0 1 1 .7 4.5 M4 5v6h6 M12 8v4l3 2',
  bugs: 'M7 11a5 5 0 0 1 10 0v5a5 5 0 0 1-10 0z M8 3l2 3 M16 3l-2 3 M12 10v11 M3 8l4 3 M21 8l-4 3 M3 14h4 M17 14h4 M4 21l4-3 M20 21l-4-3',
  patches: 'M12 3l8 3v5c0 4-3 7-8 10-5-3-8-6-8-10V6z M8.5 12l2 2 5-5',
  mcp: 'M9 3v5 M15 3v5 M7 8h10v3a5 5 0 0 1-10 0z M12 16v5',
  settings: 'M5 3v4m0 4v10 M12 3v10m0 4v4 M19 3v4m0 4v10 M3 7h4v4H3z M10 13h4v4h-4z M17 7h4v4h-4z',
  approved: 'M3 12a9 9 0 1 0 18 0 9 9 0 1 0-18 0z M8 12l3 3 5-6',
  'pass-rate': 'M5 19 19 5 M4 7a3 3 0 1 0 6 0 3 3 0 1 0-6 0z M14 17a3 3 0 1 0 6 0 3 3 0 1 0-6 0z',
  datasets: 'M3 4h18v4H3z M4 8v12h16V8 M10 12h4',
  records: 'M3 4h18v16H3z M3 9h18 M3 14h18 M9 9v11',
}
</script>

<template>
  <svg
    v-if="glyphs[name]"
    class="app-icon"
    width="18"
    height="18"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.75"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path :d="glyphs[name]" />
  </svg>
</template>

<style scoped>
.app-icon { display: block; flex: none; }
</style>
