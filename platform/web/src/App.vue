<script setup>
import { computed, onMounted, ref, shallowRef } from 'vue'
import { api, getTenant, setTenant } from './api'
import OverviewView from './views/OverviewView.vue'
import ProjectsView from './views/ProjectsView.vue'
import RequirementsView from './views/RequirementsView.vue'
import VersionsView from './views/VersionsView.vue'
import EnvironmentsView from './views/EnvironmentsView.vue'
import CasesView from './views/CasesView.vue'
import SuitesView from './views/SuitesView.vue'
import RunsView from './views/RunsView.vue'
import BugsView from './views/BugsView.vue'
import PatchesView from './views/PatchesView.vue'
import TestDataView from './views/TestDataView.vue'
import SettingsView from './views/SettingsView.vue'

const NAV = [
  { key: 'overview', label: '概览', comp: OverviewView },
  { key: 'projects', label: '项目', comp: ProjectsView },
  { key: 'requirements', label: '需求', comp: RequirementsView },
  { key: 'versions', label: '版本', comp: VersionsView },
  { key: 'environments', label: '环境', comp: EnvironmentsView },
  { key: 'test-data', label: '测试数据', comp: TestDataView },
  { key: 'cases', label: '用例', comp: CasesView },
  { key: 'suites', label: '套件', comp: SuitesView },
  { key: 'runs', label: '执行记录', comp: RunsView },
  { key: 'bugs', label: '缺陷', comp: BugsView },
  { key: 'patches', label: '自愈审批', comp: PatchesView },
  { key: 'settings', label: '设置', comp: SettingsView },
]

const tenantId = ref(getTenant())
const draft = ref('')
const current = ref('overview')
const error = ref('')
const viewKey = shallowRef(0)

const view = computed(() => NAV.find((n) => n.key === current.value))

function switchTo(key) {
  current.value = key
  viewKey.value += 1
}

async function enter() {
  const id = draft.value.trim()
  if (!id) return
  error.value = ''
  setTenant(id)
  try {
    await api.get('/api/overview')
    tenantId.value = id
    switchTo('overview')
  } catch {
    error.value = '工作区不存在'
  }
}

async function createWorkspace() {
  const name = draft.value.trim() || '新租户'
  try {
    const t = await api.post('/api/tenants', { name })
    setTenant(t.id)
    tenantId.value = t.id
    draft.value = ''
    switchTo('overview')
  } catch (e) { error.value = e.message }
}

function leave() {
  localStorage.removeItem('platform.tenant')
  tenantId.value = ''
}

onMounted(() => {
  if (tenantId.value) api.get('/api/overview').catch(leave)
})
</script>

<template>
  <div v-if="!tenantId" class="gate">
    <div class="gate-box">
      <h1>测试平台</h1>
      <input v-model="draft" placeholder="工作区 ID" @keyup.enter="enter" />
      <button class="primary" @click="enter">进入</button>
      <button class="ghost" @click="createWorkspace">新建工作区</button>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>

  <div v-else class="shell">
    <aside>
      <div class="brand">测试平台</div>
      <nav>
        <button
          v-for="n in NAV"
          :key="n.key"
          :class="{ on: current === n.key }"
          @click="switchTo(n.key)"
        >{{ n.label }}</button>
      </nav>
      <button class="leave" @click="leave">切换工作区</button>
    </aside>

    <main>
      <h2>{{ view.label }}</h2>
      <component :is="view.comp" :key="viewKey" />
    </main>
  </div>
</template>

<style scoped>
.gate { display: grid; place-items: center; min-height: 100vh; }
.gate-box { width: 300px; display: flex; flex-direction: column; gap: 10px; }
.gate-box h1 { font-size: 20px; font-weight: 600; margin: 0 0 10px; }
.ghost { border: 0; background: transparent; color: var(--muted); }

.shell { display: flex; min-height: 100vh; }
aside {
  width: 196px;
  flex: 0 0 196px;
  background: var(--panel);
  border-right: 1px solid var(--line);
  padding: 18px 12px;
  display: flex;
  flex-direction: column;
}
.brand { font-weight: 600; padding: 0 8px 18px; }
nav { display: flex; flex-direction: column; gap: 2px; }
nav button {
  border: 0;
  background: transparent;
  text-align: left;
  padding: 8px 10px;
  color: var(--muted);
  border-radius: 6px;
}
nav button.on { background: #eef1fe; color: var(--accent); font-weight: 500; }
.leave { margin-top: auto; border: 0; background: transparent; color: var(--muted); text-align: left; padding: 8px 10px; font-size: 12px; }

main { flex: 1; padding: 22px 30px 40px; max-width: 1180px; }
main h2 { font-size: 17px; font-weight: 600; margin: 0 0 18px; }
.error { color: var(--bad); font-size: 12px; margin: 0; }
</style>
