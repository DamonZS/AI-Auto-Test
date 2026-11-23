<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const path = '/api/desktop/worker'
const status = ref('')
const activating = ref(false)
const error = ref('')
let pendingToken = ''
const labels = {
  inactive: '未激活',
  active: '已激活',
  external: '已使用手动配置的凭据',
  other_workspace: '已为其他工作区激活',
}

async function load() {
  try {
    status.value = (await api.get(path)).status
  } catch (err) {
    if (err.status !== 403 && err.status !== 404) {
      error.value = '无法读取桌面 Worker 状态，请重新打开设置。'
    }
  }
}

async function activate() {
  if (activating.value) return
  activating.value = true
  error.value = ''
  try {
    pendingToken ||= (await api.post('/api/auth/service-tokens', { scope: 'worker' })).token
    status.value = (await api.post(path, { token: pendingToken })).status
    pendingToken = ''
  } catch (err) {
    if (err.status === 400) pendingToken = ''
    if (err.status === 409) await load()
    else error.value = err.message
  } finally {
    activating.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="status || error" class="desktop-worker" aria-labelledby="desktop-worker-title">
    <h3 id="desktop-worker-title">桌面 Worker</h3>
    <span role="status">{{ labels[status] }}</span>
    <button v-if="status === 'inactive'" type="button" class="primary" :disabled="activating" @click="activate">
      {{ activating ? '激活中…' : '激活 Worker' }}
    </button>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.desktop-worker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 18px 20px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
}
h3 { margin: 0; font-size: 14px; font-weight: 600; }
span { color: var(--muted); }
button { margin-left: auto; }
button:disabled { opacity: .55; cursor: wait; }
.error { flex-basis: 100%; margin: 0; color: var(--bad); }
</style>
