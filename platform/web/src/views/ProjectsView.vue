<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const rows = ref([])
const form = ref({ name: '', base_url: '', allowed_hosts: '' })
const error = ref('')

async function load() {
  rows.value = await api.get('/api/projects')
}

async function add() {
  if (!form.value.name || !form.value.base_url) return
  error.value = ''
  try {
    await api.post('/api/projects', form.value)
    form.value = { name: '', base_url: '', allowed_hosts: '' }
    await load()
  } catch (e) { error.value = e.message }
}

onMounted(load)
</script>

<template>
  <header class="head">
    <div class="form">
      <input v-model="form.name" placeholder="项目名" />
      <input v-model="form.base_url" placeholder="目标地址 https://..." />
      <input v-model="form.allowed_hosts" placeholder="白名单主机，逗号分隔" />
      <button class="primary" @click="add">添加</button>
    </div>
  </header>
  <p v-if="error" class="error">{{ error }}</p>
  <DataTable
    :rows="rows"
    :columns="[
      { key: 'name', label: '项目', width: '22%' },
      { key: 'base_url', label: '地址' },
      { key: 'allowed_hosts', label: '白名单', width: '24%' },
      { key: 'created_at', label: '创建时间', width: '18%' },
    ]"
    empty="还没有项目"
  />
</template>

<style scoped>
.head { margin-bottom: 16px; }
.form { display: flex; gap: 8px; }
.form input:first-child { width: 180px; }
.form input:nth-child(2) { flex: 1; }
.form input:nth-child(3) { width: 240px; }
.error { color: var(--bad); }
</style>
